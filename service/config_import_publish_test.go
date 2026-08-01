package service

import (
	"context"
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelrouting"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestPublishConfigImportBatchAppliesSaleMappingAndRouteProposals(t *testing.T) {
	prepareConfigImportServiceDB(t)
	common.OptionMapRWMutex.Lock()
	previousOptionMap := common.OptionMap
	common.OptionMap = make(map[string]string)
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptionMap
		common.OptionMapRWMutex.Unlock()
	})
	require.NoError(t, model.DB.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.ChannelModelCostRule{}, &model.Option{}, &model.RoutingPolicy{}, &model.RouteTarget{}, &model.ConfigImportPublishAudit{}))
	channel := &model.Channel{Type: 1, Name: "supplier", Models: "vendor-video", Key: "key"}
	require.NoError(t, model.DB.Create(channel).Error)
	batch := createConfigImportStageBatch(t, channel.Id, "line-a", "vendor-video")
	canonicalModel := "seedance-2.0"
	runtimeModel := modelrouting.Seedance20
	mapping := types.ConfigImportModelMapping{ConfigImportAuthoritativeEntity: types.ConfigImportAuthoritativeEntity{BusinessID: "mapping-a"}, CanonicalModel: canonicalModel, ClientModel: "supplier-video-alias", LineRef: "line-a", UpstreamModel: "vendor-video", SKURef: "sku-a"}
	encoded, err := common.Marshal(mapping)
	require.NoError(t, err)
	require.NoError(t, model.DB.Create(&model.ConfigImportItem{BatchID: batch.ID, EntityType: "model_mappings", BusinessID: mapping.BusinessID, CanonicalJSON: string(encoded), State: string(types.ConfigImportItemStateNew)}).Error)
	blueprint := types.ConfigImportRouteBlueprint{
		ConfigImportAuthoritativeEntity: types.ConfigImportAuthoritativeEntity{BusinessID: "route-a"},
		CanonicalModel:                  canonicalModel,
		ClientModel:                     "supplier-video-alias",
		MergeMode:                       types.ConfigImportRouteMergeModeMerge,
		Targets: []types.ConfigImportRouteTarget{{
			RouteTargetRef:    "target-a",
			LineRef:           "line-a",
			UpstreamModel:     "vendor-video",
			SKURef:            "sku-a",
			CostVariantKey:    "default",
			OutputResolutions: []string{"720p"},
			DurationValues:    []int{5},
			AspectRatios:      []string{"16:9"},
			InputModes:        []string{"text"},
			ReferenceMinimums: configImportReferenceBounds(0, 0, 0),
			ReferenceLimits:   configImportReferenceBounds(9, 3, 3),
		}},
	}
	encoded, err = common.Marshal(blueprint)
	require.NoError(t, err)
	require.NoError(t, model.DB.Model(&model.ConfigImportItem{}).Where("batch_id = ? AND business_id = ?", batch.ID, blueprint.BusinessID).Update("canonical_json", string(encoded)).Error)
	_, err = StageConfigImportBatch(context.Background(), 42, batch.ID)
	require.NoError(t, err)
	scope, err := configImportBaselineScopeForBatch(model.DB, batch.ID)
	require.NoError(t, err)
	assert.Contains(t, scope.modelMappings[channel.Id], runtimeModel)
	assert.NotContains(t, scope.modelMappings[channel.Id], "supplier-video-alias")
	require.NoError(t, PublishConfigImportBatch(context.Background(), batch.ID, 42))

	var billingModeOption model.Option
	require.NoError(t, model.DB.Where("key = ?", "billing_setting.billing_mode").First(&billingModeOption).Error)
	var billingModes map[string]string
	require.NoError(t, common.UnmarshalJsonStr(billingModeOption.Value, &billingModes))
	assert.Equal(t, billing_setting.BillingModePerDuration, billingModes[runtimeModel])

	var durationPriceOption model.Option
	require.NoError(t, model.DB.Where("key = ?", "billing_setting.duration_price").First(&durationPriceOption).Error)
	var durationPrices map[string]types.DurationPrice
	require.NoError(t, common.UnmarshalJsonStr(durationPriceOption.Value, &durationPrices))
	expectedDurationPrice, err := configImportSeedanceDurationPrice(runtimeModel)
	require.NoError(t, err)
	assert.Equal(t, expectedDurationPrice, durationPrices[runtimeModel])
	var saved model.Channel
	require.NoError(t, model.DB.First(&saved, channel.Id).Error)
	var savedMapping map[string]string
	require.NoError(t, common.UnmarshalJsonStr(saved.GetModelMapping(), &savedMapping))
	assert.Equal(t, "vendor-video", savedMapping[runtimeModel])
	assert.Contains(t, saved.GetModels(), runtimeModel)
	candidates, err := model.ListRoutingCandidates("default", runtimeModel)
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	assert.Equal(t, channel.Id, candidates[0].ID)
	var policy model.RoutingPolicy
	require.NoError(t, model.DB.Where("group_name = ? AND model = ?", "default", runtimeModel).First(&policy).Error)
	assert.False(t, policy.Enabled)
	var targetCount int64
	require.NoError(t, model.DB.Model(&model.RouteTarget{}).Where("policy_id = ?", policy.ID).Count(&targetCount).Error)
	assert.Equal(t, int64(1), targetCount)
	var audit model.ConfigImportPublishAudit
	require.NoError(t, model.DB.Where("batch_id = ?", batch.ID).First(&audit).Error)
	assert.NotEmpty(t, audit.AfterSHA256)
	assert.NotEqual(t, audit.BeforeSHA256, audit.AfterSHA256)
	after, err := CaptureConfigImportBaseline(model.DB, batch.ID)
	require.NoError(t, err)
	assert.Equal(t, after.Hash, audit.AfterSHA256)
}

func TestPublishConfigImportBatchRecordsPostPublishCostCoverage(t *testing.T) {
	prepareConfigImportServiceDB(t)
	require.NoError(t, model.DB.AutoMigrate(
		&model.Channel{}, &model.Ability{}, &model.ChannelModelCostRule{},
		&model.ConfigImportIssue{}, &model.ConfigImportPublishAudit{},
	))
	channel := &model.Channel{
		Type: 1, Name: "unpriced", Group: "default", Status: common.ChannelStatusEnabled,
		Models: "unpriced-model", Key: "key",
	}
	require.NoError(t, model.DB.Create(channel).Error)
	require.NoError(t, channel.UpdateAbilities(model.DB))
	batch := createConfigImportStageBatch(t, channel.Id, "line-a", "unpriced-model")
	_, err := StageConfigImportBatch(context.Background(), 42, batch.ID)
	require.NoError(t, err)
	require.NoError(t, PublishConfigImportBatch(context.Background(), batch.ID, 42))

	var issue model.ConfigImportIssue
	require.NoError(t, model.DB.Where("batch_id = ? AND code = ?", batch.ID, "COST_COVERAGE_INCOMPLETE").First(&issue).Error)
	assert.Equal(t, string(types.ConfigImportIssueSeverityWarning), issue.Severity)
	assert.Equal(t, "open", issue.ResolutionStatus)
	assert.Contains(t, issue.Message, "1")
}

func TestPublishConfigImportBatchReplacesBoundChannelModelSnapshot(t *testing.T) {
	prepareConfigImportServiceDB(t)
	common.OptionMapRWMutex.Lock()
	previousOptionMap := common.OptionMap
	common.OptionMap = make(map[string]string)
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptionMap
		common.OptionMapRWMutex.Unlock()
	})
	require.NoError(t, model.DB.AutoMigrate(
		&model.Channel{}, &model.Ability{}, &model.ChannelModelCostRule{}, &model.Option{},
		&model.RoutingPolicy{}, &model.RouteTarget{}, &model.ConfigImportPublishAudit{},
	))
	existingMapping := `{"canonical-keep":"upstream-keep","canonical-old":"upstream-old"}`
	channel := &model.Channel{
		Type: 1, Name: "supplier", Group: "default", Status: common.ChannelStatusEnabled,
		Models: "canonical-keep,upstream-keep,canonical-old,upstream-old", Key: "key", ModelMapping: &existingMapping,
	}
	require.NoError(t, model.DB.Create(channel).Error)
	require.NoError(t, channel.UpdateAbilities(model.DB))
	oldRule := &model.ChannelModelCostRule{
		ChannelID: channel.Id, BillableUpstreamModel: "upstream-old", CostVariantKey: "default", Version: 1,
		Status: string(types.CostRuleActive), CostMode: string(types.CostModePerRequest), SchemaVersion: 1, ConfigJSON: `{}`,
	}
	require.NoError(t, model.DB.Create(oldRule).Error)
	oldTarget := &model.RouteTarget{
		PolicyID: 999, ChannelID: channel.Id, Name: "old-target", UpstreamModel: "upstream-old",
		CostVariantKey: "default", Constraints: `{}`, Enabled: true,
	}
	require.NoError(t, model.DB.Create(oldTarget).Error)

	batch := createConfigImportStageBatch(t, channel.Id, "line-a", "upstream-keep")
	var importedRoute model.ConfigImportItem
	require.NoError(t, model.DB.Where("batch_id = ? AND entity_type = ?", batch.ID, "route_blueprints").First(&importedRoute).Error)
	var skippedBlueprint types.ConfigImportRouteBlueprint
	require.NoError(t, common.UnmarshalJsonStr(importedRoute.CanonicalJSON, &skippedBlueprint))
	skippedBlueprint.MergeMode = types.ConfigImportRouteMergeModeSkip
	encodedRoute, err := common.Marshal(skippedBlueprint)
	require.NoError(t, err)
	require.NoError(t, model.DB.Model(&importedRoute).Update("canonical_json", string(encodedRoute)).Error)
	for _, mapping := range []types.ConfigImportModelMapping{
		{
			ConfigImportAuthoritativeEntity: types.ConfigImportAuthoritativeEntity{BusinessID: "mapping-keep"},
			CanonicalModel:                  "canonical-keep", ClientModel: "canonical-keep", LineRef: "line-a", UpstreamModel: "upstream-keep", SKURef: "sku-a",
		},
		{
			ConfigImportAuthoritativeEntity: types.ConfigImportAuthoritativeEntity{BusinessID: "mapping-new"},
			CanonicalModel:                  "canonical-new", ClientModel: "canonical-new", LineRef: "line-a", UpstreamModel: "upstream-new", SKURef: "sku-a",
		},
	} {
		encoded, err := common.Marshal(mapping)
		require.NoError(t, err)
		require.NoError(t, model.DB.Create(&model.ConfigImportItem{
			BatchID: batch.ID, EntityType: "model_mappings", BusinessID: mapping.BusinessID,
			CanonicalJSON: string(encoded), State: string(types.ConfigImportItemStateNew),
		}).Error)
	}

	_, err = StageConfigImportBatch(context.Background(), 42, batch.ID)
	require.NoError(t, err)
	require.NoError(t, PublishConfigImportBatch(context.Background(), batch.ID, 42))

	var savedChannel model.Channel
	require.NoError(t, model.DB.First(&savedChannel, channel.Id).Error)
	assert.Equal(t, "canonical-keep,canonical-new,upstream-keep,upstream-new", savedChannel.Models)
	var savedMapping map[string]string
	require.NoError(t, common.UnmarshalJsonStr(savedChannel.GetModelMapping(), &savedMapping))
	assert.Equal(t, map[string]string{
		"canonical-keep": "upstream-keep",
		"canonical-new":  "upstream-new",
	}, savedMapping)
	var abilities []model.Ability
	require.NoError(t, model.DB.Where("channel_id = ?", channel.Id).Order("model ASC").Find(&abilities).Error)
	require.Len(t, abilities, 4)
	assert.Equal(t, []string{"canonical-keep", "canonical-new", "upstream-keep", "upstream-new"}, []string{
		abilities[0].Model, abilities[1].Model, abilities[2].Model, abilities[3].Model,
	})
	require.NoError(t, model.DB.First(oldTarget, oldTarget.ID).Error)
	assert.False(t, oldTarget.Enabled)
	require.NoError(t, model.DB.First(oldRule, oldRule.ID).Error)
	assert.Equal(t, string(types.CostRuleRetired), oldRule.Status)
	assert.NotNil(t, oldRule.EffectiveTo)
}

func TestPublishConfigImportModelSnapshotRollsBackWithTransaction(t *testing.T) {
	prepareConfigImportServiceDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.ChannelModelCostRule{}, &model.RoutingPolicy{}, &model.RouteTarget{}))
	existingMapping := `{"canonical-old":"upstream-old"}`
	channel := &model.Channel{Type: 1, Name: "supplier", Group: "default", Models: "canonical-old,upstream-old", Key: "key", ModelMapping: &existingMapping}
	require.NoError(t, model.DB.Create(channel).Error)
	oldRule := &model.ChannelModelCostRule{
		ChannelID: channel.Id, BillableUpstreamModel: "upstream-old", CostVariantKey: "default", Version: 1,
		Status: string(types.CostRuleActive), CostMode: string(types.CostModePerRequest), SchemaVersion: 1, ConfigJSON: `{}`,
	}
	require.NoError(t, model.DB.Create(oldRule).Error)
	oldTarget := &model.RouteTarget{PolicyID: 999, ChannelID: channel.Id, Name: "old-target", UpstreamModel: "upstream-old", CostVariantKey: "default", Constraints: `{}`, Enabled: true}
	require.NoError(t, model.DB.Create(oldTarget).Error)
	batch := createConfigImportStageBatch(t, channel.Id, "line-a", "upstream-new")
	mapping := types.ConfigImportModelMapping{
		ConfigImportAuthoritativeEntity: types.ConfigImportAuthoritativeEntity{BusinessID: "mapping-new"},
		CanonicalModel:                  "canonical-new", LineRef: "line-a", UpstreamModel: "upstream-new",
	}
	encoded, err := common.Marshal(mapping)
	require.NoError(t, err)
	item := model.ConfigImportItem{BatchID: batch.ID, EntityType: "model_mappings", BusinessID: mapping.BusinessID, CanonicalJSON: string(encoded), State: string(types.ConfigImportItemStateNew)}
	require.NoError(t, model.DB.Create(&item).Error)

	err = model.DB.Transaction(func(tx *gorm.DB) error {
		require.NoError(t, publishConfigImportModelMappings(tx, []model.ConfigImportItem{item}, &ConfigImportRefreshKeys{}))
		return errors.New("force rollback")
	})
	require.ErrorContains(t, err, "force rollback")

	var savedChannel model.Channel
	require.NoError(t, model.DB.First(&savedChannel, channel.Id).Error)
	assert.Equal(t, "canonical-old,upstream-old", savedChannel.Models)
	assert.JSONEq(t, existingMapping, savedChannel.GetModelMapping())
	require.NoError(t, model.DB.First(oldRule, oldRule.ID).Error)
	assert.Equal(t, string(types.CostRuleActive), oldRule.Status)
	require.NoError(t, model.DB.First(oldTarget, oldTarget.ID).Error)
	assert.True(t, oldTarget.Enabled)
}

func TestPublishConfigImportSaleOptionsRemovesStaleBillingExpression(t *testing.T) {
	prepareConfigImportServiceDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Option{}))
	require.NoError(t, model.DB.Create(&model.Option{
		Key:   "billing_setting.billing_expr",
		Value: `{"doubao-seedance-2-0-mini-260615":"v1:tier(\"base\", c * 99)"}`,
	}).Error)
	document := map[string]any{
		"staged_proposal": map[string]any{
			"option_patches": map[string]any{
				"billing_setting.billing_expr": map[string]any{
					"doubao-seedance-2-0-mini-260615": "",
				},
			},
		},
	}
	encoded, err := common.Marshal(document)
	require.NoError(t, err)
	refresh := ConfigImportRefreshKeys{}
	tx := model.DB.Begin()
	require.NoError(t, tx.Error)
	require.NoError(t, publishConfigImportSaleOptions(tx, []model.ConfigImportItem{{
		EntityType: "sale_proposals", BusinessID: "sale-seedance", CanonicalJSON: string(encoded), State: "new",
	}}, &refresh))
	require.NoError(t, tx.Commit().Error)

	var option model.Option
	require.NoError(t, model.DB.Where("key = ?", "billing_setting.billing_expr").First(&option).Error)
	assert.JSONEq(t, `{}`, option.Value)
}

func TestConfigImportRouteRowsAssignsStablePrioritiesForImplicitTargets(t *testing.T) {
	blueprint := types.ConfigImportRouteBlueprint{
		CanonicalModel: modelrouting.Seedance20,
		Targets: []types.ConfigImportRouteTarget{
			{RouteTargetRef: "dimensio-4k", LineRef: "line-a", UpstreamModel: "model-4k", SKURef: "sku-4k", CostVariantKey: "default", ReferenceMinimums: configImportReferenceBounds(0, 0, 0), ReferenceLimits: configImportReferenceBounds(9, 3, 3)},
			{RouteTargetRef: "dimensio-720p", LineRef: "line-a", UpstreamModel: "model-720p", SKURef: "sku-720p", CostVariantKey: "default", ReferenceMinimums: configImportReferenceBounds(0, 0, 0), ReferenceLimits: configImportReferenceBounds(9, 3, 3)},
			{RouteTargetRef: "other-channel", LineRef: "line-b", UpstreamModel: "model-other", SKURef: "sku-other", CostVariantKey: "default", ReferenceMinimums: configImportReferenceBounds(0, 0, 0), ReferenceLimits: configImportReferenceBounds(9, 3, 3)},
		},
	}

	_, targets, err := configImportRouteRows(map[string]int{"line-a": 21, "line-b": 22}, blueprint)
	require.NoError(t, err)
	require.Len(t, targets, 3)
	assert.Equal(t, 1, targets[0].TargetPriority)
	assert.Equal(t, 0, targets[1].TargetPriority)
	assert.Equal(t, 0, targets[2].TargetPriority)
}

func TestConfigImportRouteRowsRejectsMissingReferenceLimits(t *testing.T) {
	blueprint := types.ConfigImportRouteBlueprint{
		CanonicalModel: modelrouting.Seedance20,
		Targets: []types.ConfigImportRouteTarget{{
			RouteTargetRef: "missing-reference-limits",
			LineRef:        "line-a",
			UpstreamModel:  "model-a",
			SKURef:         "sku-a",
			CostVariantKey: "default",
		}},
	}

	_, _, err := configImportRouteRows(map[string]int{"line-a": 21}, blueprint)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "reference limits")
}

func TestConfigImportRouteRowsPreservesExplicitPriorities(t *testing.T) {
	explicitPriority := 7
	blueprint := types.ConfigImportRouteBlueprint{
		CanonicalModel: modelrouting.Seedance20,
		Targets: []types.ConfigImportRouteTarget{
			{RouteTargetRef: "explicit", LineRef: "line-a", UpstreamModel: "model-explicit", SKURef: "sku-explicit", CostVariantKey: "default", Priority: &explicitPriority, ReferenceMinimums: configImportReferenceBounds(0, 0, 0), ReferenceLimits: configImportReferenceBounds(9, 3, 3)},
			{RouteTargetRef: "implicit", LineRef: "line-a", UpstreamModel: "model-implicit", SKURef: "sku-implicit", CostVariantKey: "default", ReferenceMinimums: configImportReferenceBounds(0, 0, 0), ReferenceLimits: configImportReferenceBounds(9, 3, 3)},
		},
	}

	_, targets, err := configImportRouteRows(map[string]int{"line-a": 21}, blueprint)
	require.NoError(t, err)
	require.Len(t, targets, 2)
	assert.Equal(t, explicitPriority, targets[0].TargetPriority)
	assert.Equal(t, 1, targets[1].TargetPriority)
}

func TestConfigImportRouteTargetPriorityOverridesCoverSeparateBlueprints(t *testing.T) {
	routeBlueprints := []configImportPublishRouteBlueprint{
		{blueprint: types.ConfigImportRouteBlueprint{
			CanonicalModel: modelrouting.Seedance20,
			Targets:        []types.ConfigImportRouteTarget{{RouteTargetRef: "blueprint-a", LineRef: "line-a", UpstreamModel: "model-a", SKURef: "sku-a", CostVariantKey: "default", ReferenceMinimums: configImportReferenceBounds(0, 0, 0), ReferenceLimits: configImportReferenceBounds(9, 3, 3)}},
		}},
		{blueprint: types.ConfigImportRouteBlueprint{
			CanonicalModel: modelrouting.Seedance20,
			Targets:        []types.ConfigImportRouteTarget{{RouteTargetRef: "blueprint-b", LineRef: "line-a", UpstreamModel: "model-b", SKURef: "sku-b", CostVariantKey: "default", ReferenceMinimums: configImportReferenceBounds(0, 0, 0), ReferenceLimits: configImportReferenceBounds(9, 3, 3)}},
		}},
	}

	overrides := configImportRouteTargetPriorityOverrides(map[string]int{"line-a": 21}, routeBlueprints)
	assert.Equal(t, 1, overrides["blueprint-a"])
	assert.Equal(t, 0, overrides["blueprint-b"])
}

func TestConfigImportMergeRouteTargetsPreservesExistingEnabledState(t *testing.T) {
	merged := configImportMergeRouteTargets(
		[]model.RouteTarget{{Name: "route-target/MAP-DIMENSIO-R83-720", Enabled: true, UpstreamModel: "previous-model"}},
		[]model.RouteTarget{{Name: "route-target/MAP-DIMENSIO-R83-720", Enabled: false, UpstreamModel: "updated-model"}},
	)

	require.Len(t, merged, 1)
	assert.True(t, merged[0].Enabled)
	assert.Equal(t, "updated-model", merged[0].UpstreamModel)
}

func TestPublishConfigImportRoutesAssignsPrioritiesAcrossBlueprints(t *testing.T) {
	prepareConfigImportServiceDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Channel{}, &model.RoutingPolicy{}, &model.RouteTarget{}))
	channel := &model.Channel{Type: 1, Name: "supplier", Models: "vendor-video", Key: "key"}
	require.NoError(t, model.DB.Create(channel).Error)
	confirmedAt := int64(1)
	require.NoError(t, model.DB.Create(&model.ConfigImportBinding{
		BatchID: 1, LineRef: "line-a", Action: string(types.ConfigImportBindingActionBind), ChannelID: &channel.Id,
		CredentialsConfirmedBy: 42, CredentialsConfirmedAt: &confirmedAt,
	}).Error)
	newTarget := func(ref, upstream string) types.ConfigImportRouteTarget {
		return types.ConfigImportRouteTarget{
			RouteTargetRef: ref, LineRef: "line-a", UpstreamModel: upstream, SKURef: ref, CostVariantKey: "default",
			OutputResolutions: []string{"720p"}, DurationValues: []int{5}, AspectRatios: []string{"16:9"}, InputModes: []string{"text"},
			ReferenceMinimums: configImportReferenceBounds(0, 0, 0), ReferenceLimits: configImportReferenceBounds(9, 3, 3), Enabled: boolPointer(false),
		}
	}
	blueprints := []types.ConfigImportRouteBlueprint{
		{CanonicalModel: modelrouting.Seedance20, Targets: []types.ConfigImportRouteTarget{newTarget("blueprint-a", "model-a")}},
		{CanonicalModel: modelrouting.Seedance20, Targets: []types.ConfigImportRouteTarget{newTarget("blueprint-b", "model-b")}},
	}
	items := make([]model.ConfigImportItem, 0, len(blueprints))
	for _, blueprint := range blueprints {
		encoded, err := common.Marshal(blueprint)
		require.NoError(t, err)
		items = append(items, model.ConfigImportItem{BatchID: 1, EntityType: "route_blueprints", BusinessID: blueprint.Targets[0].RouteTargetRef, CanonicalJSON: string(encoded), State: string(types.ConfigImportItemStateChanged)})
	}
	require.NoError(t, publishConfigImportRoutes(model.DB, items, &ConfigImportRefreshKeys{}))

	policy, err := model.GetRoutingPolicy(1)
	require.NoError(t, err)
	require.Len(t, policy.Targets, 2)
	assert.Equal(t, 1, policy.Targets[0].TargetPriority)
	assert.Equal(t, 0, policy.Targets[1].TargetPriority)
}

func TestPublishConfigImportRoutesMergesReplacesAndSkipsTargets(t *testing.T) {
	prepareConfigImportServiceDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Channel{}, &model.RoutingPolicy{}, &model.RouteTarget{}))
	channel := &model.Channel{Type: 1, Name: "supplier", Models: "vendor-video", Key: "key"}
	require.NoError(t, model.DB.Create(channel).Error)
	confirmedAt := int64(1)
	require.NoError(t, model.DB.Create(&model.ConfigImportBinding{
		BatchID: 1, LineRef: "line-a", Action: string(types.ConfigImportBindingActionBind), ChannelID: &channel.Id,
		CredentialsConfirmedBy: 42, CredentialsConfirmedAt: &confirmedAt,
	}).Error)
	constraints, err := common.Marshal(modelrouting.Constraints{
		OutputResolutions: []string{"720p"},
		Durations:         modelrouting.DurationConstraint{Values: []int{5}},
		AspectRatios:      []string{"16:9"},
		InputModes:        []modelrouting.InputMode{modelrouting.InputModeText},
	})
	require.NoError(t, err)
	policy, err := model.ReplaceRoutingPolicy(0, model.RoutingPolicy{
		GroupName: "default", Model: modelrouting.Seedance20, Enabled: true,
		DefaultResolution: "720p", DefaultDuration: 5, DefaultRatio: "16:9",
	}, []model.RouteTarget{{
		ChannelID: channel.Id, Name: "preserved", UpstreamModel: "existing-model", CostVariantKey: "default",
		TargetPriority: 100, Enabled: true, Constraints: string(constraints),
	}})
	require.NoError(t, err)

	merge := types.ConfigImportRouteBlueprint{
		CanonicalModel: modelrouting.Seedance20, ClientModel: modelrouting.Seedance20, MergeMode: types.ConfigImportRouteMergeModeMerge,
		Targets: []types.ConfigImportRouteTarget{{
			RouteTargetRef: "managed", LineRef: "line-a", UpstreamModel: "import-v1", SKURef: "sku-a", CostVariantKey: "default",
			OutputResolutions: []string{"720p"}, DurationValues: []int{5}, AspectRatios: []string{"16:9"}, InputModes: []string{"text"},
			ReferenceMinimums: configImportReferenceBounds(0, 0, 0), ReferenceLimits: configImportReferenceBounds(9, 3, 3), Enabled: boolPointer(false),
		}},
	}
	encoded, err := common.Marshal(merge)
	require.NoError(t, err)
	refresh := ConfigImportRefreshKeys{}
	require.NoError(t, publishConfigImportRoutes(model.DB, []model.ConfigImportItem{{
		BatchID: 1, EntityType: "route_blueprints", BusinessID: "merge-v1", CanonicalJSON: string(encoded), State: string(types.ConfigImportItemStateChanged),
	}}, &refresh))

	merged, err := model.GetRoutingPolicy(policy.ID)
	require.NoError(t, err)
	assert.True(t, merged.Enabled)
	require.Len(t, merged.Targets, 2)
	assert.Contains(t, refresh.RoutingPolicyKeys, model.RoutingPolicyKey{GroupName: "default", Model: modelrouting.Seedance20})

	merge.Targets[0].UpstreamModel = "import-v2"
	encoded, err = common.Marshal(merge)
	require.NoError(t, err)
	require.NoError(t, publishConfigImportRoutes(model.DB, []model.ConfigImportItem{{
		BatchID: 1, EntityType: "route_blueprints", BusinessID: "merge-v2", CanonicalJSON: string(encoded), State: string(types.ConfigImportItemStateChanged),
	}}, &ConfigImportRefreshKeys{}))
	merged, err = model.GetRoutingPolicy(policy.ID)
	require.NoError(t, err)
	require.Len(t, merged.Targets, 2)
	for _, target := range merged.Targets {
		if target.Name == "managed" {
			assert.Equal(t, "import-v2", target.UpstreamModel)
			assert.False(t, target.Enabled)
		}
	}

	replace := merge
	replace.MergeMode = types.ConfigImportRouteMergeModeReplace
	replace.Targets[0].RouteTargetRef = "replacement"
	replace.Targets[0].UpstreamModel = "replacement-model"
	encoded, err = common.Marshal(replace)
	require.NoError(t, err)
	require.NoError(t, publishConfigImportRoutes(model.DB, []model.ConfigImportItem{{
		BatchID: 1, EntityType: "route_blueprints", BusinessID: "replace", CanonicalJSON: string(encoded), State: string(types.ConfigImportItemStateChanged),
	}}, &ConfigImportRefreshKeys{}))
	replaced, err := model.GetRoutingPolicy(policy.ID)
	require.NoError(t, err)
	assert.False(t, replaced.Enabled)
	require.Len(t, replaced.Targets, 1)
	assert.Equal(t, "replacement", replaced.Targets[0].Name)

	skip := replace
	skip.MergeMode = types.ConfigImportRouteMergeModeSkip
	skip.Targets[0].RouteTargetRef = "not-published"
	encoded, err = common.Marshal(skip)
	require.NoError(t, err)
	require.NoError(t, publishConfigImportRoutes(model.DB, []model.ConfigImportItem{{
		BatchID: 1, EntityType: "route_blueprints", BusinessID: "skip", CanonicalJSON: string(encoded), State: string(types.ConfigImportItemStateChanged),
	}}, &ConfigImportRefreshKeys{}))
	skipped, err := model.GetRoutingPolicy(policy.ID)
	require.NoError(t, err)
	require.Len(t, skipped.Targets, 1)
	assert.Equal(t, "replacement", skipped.Targets[0].Name)
}

func configImportReferenceBounds(images, videos, audios int) *types.ConfigImportReferenceBounds {
	return &types.ConfigImportReferenceBounds{
		Images: &images,
		Videos: &videos,
		Audios: &audios,
	}
}

func TestPublishConfigImportBatchRollsBackSaleMappingAndCostWhenRouteFails(t *testing.T) {
	prepareConfigImportServiceDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Channel{}, &model.ChannelModelCostRule{}, &model.Option{}, &model.RoutingPolicy{}, &model.RouteTarget{}, &model.ConfigImportPublishAudit{}))
	channel := &model.Channel{Type: 1, Name: "supplier", Models: "vendor-video", Key: "key"}
	require.NoError(t, model.DB.Create(channel).Error)
	batch := createConfigImportStageBatch(t, channel.Id, "line-a", "vendor-video")
	mapping := types.ConfigImportModelMapping{
		ConfigImportAuthoritativeEntity: types.ConfigImportAuthoritativeEntity{BusinessID: "mapping-a"},
		CanonicalModel:                  modelrouting.Seedance20,
		ClientModel:                     modelrouting.Seedance20,
		LineRef:                         "line-a",
		UpstreamModel:                   "vendor-video",
		SKURef:                          "sku-a",
	}
	encoded, err := common.Marshal(mapping)
	require.NoError(t, err)
	require.NoError(t, model.DB.Create(&model.ConfigImportItem{
		BatchID: batch.ID, EntityType: "model_mappings", BusinessID: mapping.BusinessID,
		CanonicalJSON: string(encoded), State: string(types.ConfigImportItemStateNew),
	}).Error)
	_, err = StageConfigImportBatch(context.Background(), 42, batch.ID)
	require.NoError(t, err)

	err = PublishConfigImportBatch(context.Background(), batch.ID, 42)
	require.Error(t, err)
	var rule model.ChannelModelCostRule
	require.NoError(t, model.DB.Where("source = ?", "config_import").First(&rule).Error)
	assert.Equal(t, string(types.CostRuleDraft), rule.Status)
	var optionCount int64
	require.NoError(t, model.DB.Model(&model.Option{}).Where("key = ?", "ModelPrice").Count(&optionCount).Error)
	assert.Zero(t, optionCount)
	var savedChannel model.Channel
	require.NoError(t, model.DB.First(&savedChannel, channel.Id).Error)
	assert.Empty(t, savedChannel.GetModelMapping())
	var routeCount int64
	require.NoError(t, model.DB.Model(&model.RoutingPolicy{}).Count(&routeCount).Error)
	assert.Zero(t, routeCount)
	var savedBatch model.ConfigImportBatch
	require.NoError(t, model.DB.First(&savedBatch, batch.ID).Error)
	assert.Equal(t, string(types.ConfigImportBatchStatusPublishFailed), savedBatch.Status)
	assert.Equal(t, "PUBLISH_ROUTE", savedBatch.FailureCode)
	assert.Equal(t, "configuration publish transaction failed", savedBatch.FailureMessage)
}

func TestStageConfigImportBatchRejectsUnconfirmedChannelBindings(t *testing.T) {
	prepareConfigImportServiceDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Channel{}, &model.ChannelModelCostRule{}))
	channel := &model.Channel{Type: 1, Name: "supplier", Models: "vendor-video", Key: "key"}
	require.NoError(t, model.DB.Create(channel).Error)
	batch := createConfigImportStageBatch(t, channel.Id, "line-a", "vendor-video")
	require.NoError(t, model.DB.Model(&model.ConfigImportBinding{}).
		Where("batch_id = ? AND line_ref = ?", batch.ID, "line-a").
		Updates(map[string]any{"credentials_confirmed_by": 0, "credentials_confirmed_at": nil}).Error)

	detail, err := StageConfigImportBatch(context.Background(), 42, batch.ID)
	require.NoError(t, err)
	assert.Equal(t, types.ConfigImportBatchStatusBinding, detail.Status)
	var credentialIssue model.ConfigImportIssue
	require.NoError(t, model.DB.Where("batch_id = ? AND code = ? AND business_id = ?", batch.ID, "BINDING_CREDENTIALS_UNCONFIRMED", "line-a").First(&credentialIssue).Error)
	assert.Equal(t, string(types.ConfigImportIssueSeverityError), credentialIssue.Severity)
	assert.Equal(t, "line \"line-a\" requires credential confirmation before staging", credentialIssue.Message)
	assert.Equal(t, "open", credentialIssue.ResolutionStatus)
	var savedBatch model.ConfigImportBatch
	require.NoError(t, model.DB.First(&savedBatch, batch.ID).Error)
	assert.Equal(t, string(types.ConfigImportBatchStatusBinding), savedBatch.Status)

	confirmedAt := common.GetTimestamp()
	require.NoError(t, model.DB.Model(&model.ConfigImportBinding{}).
		Where("batch_id = ? AND line_ref = ?", batch.ID, "line-a").
		Updates(map[string]any{"credentials_confirmed_by": 42, "credentials_confirmed_at": confirmedAt}).Error)
	detail, err = StageConfigImportBatch(context.Background(), 42, batch.ID)
	require.NoError(t, err)
	assert.Equal(t, types.ConfigImportBatchStatusReady, detail.Status)
	require.NoError(t, model.DB.First(&credentialIssue, credentialIssue.ID).Error)
	assert.Equal(t, "resolved", credentialIssue.ResolutionStatus)
}

func TestStageConfigImportBatchRevalidatesPublishFailedBatch(t *testing.T) {
	prepareConfigImportServiceDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Channel{}, &model.ChannelModelCostRule{}))
	channel := &model.Channel{Type: 1, Name: "supplier", Models: "vendor-video", Key: "key"}
	require.NoError(t, model.DB.Create(channel).Error)
	batch := createConfigImportStageBatch(t, channel.Id, "line-a", "vendor-video")
	_, err := StageConfigImportBatch(context.Background(), 42, batch.ID)
	require.NoError(t, err)
	failedAt := common.GetTimestamp()
	require.NoError(t, model.DB.Model(&model.ConfigImportBatch{}).Where("id = ?", batch.ID).Updates(map[string]any{
		"status":          string(types.ConfigImportBatchStatusPublishFailed),
		"failure_code":    "PUBLISH_FAILED",
		"failure_message": "configuration publish transaction failed",
		"failed_at":       failedAt,
	}).Error)

	detail, err := StageConfigImportBatch(context.Background(), 42, batch.ID)
	require.NoError(t, err)
	assert.Equal(t, types.ConfigImportBatchStatusReady, detail.Status)
	var savedBatch model.ConfigImportBatch
	require.NoError(t, model.DB.First(&savedBatch, batch.ID).Error)
	assert.Empty(t, savedBatch.FailureCode)
	assert.Empty(t, savedBatch.FailureMessage)
	assert.Nil(t, savedBatch.FailedAt)
}

func TestRetryConfigImportBatchCacheDoesNotRepublishConfiguration(t *testing.T) {
	prepareConfigImportServiceDB(t)
	now := common.GetTimestamp()
	batch := &model.ConfigImportBatch{
		SchemaVersion: 1, TemplateVersion: "1", SourceSHA256: "source", PayloadSHA256: "payload",
		Status: string(types.ConfigImportBatchStatusPublished), CreatedBy: 42, PublishedAt: &now,
	}
	require.NoError(t, model.DB.Create(batch).Error)
	require.NoError(t, model.DB.Create(&model.ConfigImportPublishAudit{BatchID: batch.ID, AdminID: 42, BeforeSHA256: "before", AfterSHA256: "after", Outcome: "published"}).Error)
	require.NoError(t, markConfigImportCacheRefreshPending(context.Background(), batch.ID))

	require.Equal(t, []string{"refresh_cache"}, configImportAllowedActions(types.ConfigImportBatchStatusPublished, []model.ConfigImportIssue{{Code: "CACHE_REFRESH_PENDING", ResolutionStatus: "open"}}))
	require.NoError(t, RetryConfigImportBatchCache(context.Background(), batch.ID, 42))
	var issue model.ConfigImportIssue
	require.NoError(t, model.DB.Where("batch_id = ? AND code = ?", batch.ID, "CACHE_REFRESH_PENDING").First(&issue).Error)
	assert.Equal(t, "resolved", issue.ResolutionStatus)
	var auditCount int64
	require.NoError(t, model.DB.Model(&model.ConfigImportPublishAudit{}).Where("batch_id = ?", batch.ID).Count(&auditCount).Error)
	assert.Equal(t, int64(1), auditCount)
}

func TestPublishConfigImportBatchIgnoresUnrelatedActiveCostRule(t *testing.T) {
	prepareConfigImportServiceDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Channel{}, &model.ChannelModelCostRule{}, &model.ConfigImportPublishAudit{}))
	channel := &model.Channel{Type: 1, Name: "supplier", Models: "vendor-video", Key: "key"}
	require.NoError(t, model.DB.Create(channel).Error)
	batch := createConfigImportStageBatch(t, channel.Id, "line-a", "vendor-video")
	mapping := types.ConfigImportModelMapping{
		ConfigImportAuthoritativeEntity: types.ConfigImportAuthoritativeEntity{BusinessID: "mapping-keep"},
		CanonicalModel:                  "vendor-video", LineRef: "line-a", UpstreamModel: "vendor-video",
	}
	mappingJSON, err := common.Marshal(mapping)
	require.NoError(t, err)
	require.NoError(t, model.DB.Create(&model.ConfigImportItem{
		BatchID: batch.ID, EntityType: "model_mappings", BusinessID: mapping.BusinessID,
		CanonicalJSON: string(mappingJSON), State: string(types.ConfigImportItemStateNew),
	}).Error)
	_, err = StageConfigImportBatch(context.Background(), 42, batch.ID)
	require.NoError(t, err)

	var item model.ConfigImportItem
	require.NoError(t, model.DB.Where("batch_id = ? AND entity_type = ?", batch.ID, "cost_rule_drafts").First(&item).Error)
	var rule model.ChannelModelCostRule
	require.NoError(t, model.DB.First(&rule, *item.MaterializedID).Error)
	unrelatedRule := &model.ChannelModelCostRule{ChannelID: channel.Id, BillableUpstreamModel: "unrelated-model", CostVariantKey: "default", Version: 1, Status: string(types.CostRuleActive), CostMode: "per_request", SchemaVersion: 1, ConfigJSON: `{}`}
	require.NoError(t, model.DB.Create(unrelatedRule).Error)

	err = PublishConfigImportBatch(context.Background(), batch.ID, 42)
	require.NoError(t, err)
	var loaded model.ChannelModelCostRule
	require.NoError(t, model.DB.First(&loaded, rule.ID).Error)
	assert.Equal(t, string(types.CostRuleActive), loaded.Status)
	var unrelatedLoaded model.ChannelModelCostRule
	require.NoError(t, model.DB.First(&unrelatedLoaded, unrelatedRule.ID).Error)
	assert.Equal(t, string(types.CostRuleActive), unrelatedLoaded.Status)
	var loadedBatch model.ConfigImportBatch
	require.NoError(t, model.DB.First(&loadedBatch, batch.ID).Error)
	assert.Equal(t, string(types.ConfigImportBatchStatusPublished), loadedBatch.Status)
}

func TestPublishConfigImportBatchRejectsChangedAffectedCostRule(t *testing.T) {
	prepareConfigImportServiceDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Channel{}, &model.ChannelModelCostRule{}, &model.ConfigImportPublishAudit{}))
	channel := &model.Channel{Type: 1, Name: "supplier", Models: "vendor-video", Key: "key"}
	require.NoError(t, model.DB.Create(channel).Error)
	activeRule := &model.ChannelModelCostRule{
		ChannelID: channel.Id, BillableUpstreamModel: "vendor-video", CostVariantKey: "default", Version: 1,
		Status: string(types.CostRuleActive), CostMode: "per_request", SchemaVersion: 1, ConfigJSON: `{}`,
	}
	require.NoError(t, model.DB.Create(activeRule).Error)
	batch := createConfigImportStageBatch(t, channel.Id, "line-a", "vendor-video")
	_, err := StageConfigImportBatch(context.Background(), 42, batch.ID)
	require.NoError(t, err)

	require.NoError(t, model.DB.Model(&model.ChannelModelCostRule{}).Where("id = ?", activeRule.ID).Update("config_json", `{"changed":true}`).Error)
	err = PublishConfigImportBatch(context.Background(), batch.ID, 42)
	var schemaErr *ConfigImportSchemaError
	require.ErrorAs(t, err, &schemaErr)
	assert.Equal(t, "STALE_BASE_VERSION", schemaErr.Code)
}

func TestPublishConfigImportBatchRejectsChangedAffectedChannelSnapshot(t *testing.T) {
	prepareConfigImportServiceDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Channel{}, &model.ChannelModelCostRule{}, &model.ConfigImportPublishAudit{}))
	existingMapping := `{"canonical-keep":"upstream-keep"}`
	channel := &model.Channel{Type: 1, Name: "supplier", Models: "canonical-keep,upstream-keep", Key: "key", ModelMapping: &existingMapping}
	require.NoError(t, model.DB.Create(channel).Error)
	batch := createConfigImportStageBatch(t, channel.Id, "line-a", "upstream-keep")
	mapping := types.ConfigImportModelMapping{
		ConfigImportAuthoritativeEntity: types.ConfigImportAuthoritativeEntity{BusinessID: "mapping-keep"},
		CanonicalModel:                  "canonical-keep", LineRef: "line-a", UpstreamModel: "upstream-keep",
	}
	encoded, err := common.Marshal(mapping)
	require.NoError(t, err)
	require.NoError(t, model.DB.Create(&model.ConfigImportItem{
		BatchID: batch.ID, EntityType: "model_mappings", BusinessID: mapping.BusinessID,
		CanonicalJSON: string(encoded), State: string(types.ConfigImportItemStateNew),
	}).Error)
	_, err = StageConfigImportBatch(context.Background(), 42, batch.ID)
	require.NoError(t, err)
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", channel.Id).Update("models", "canonical-keep,upstream-keep,concurrent-model").Error)

	err = PublishConfigImportBatch(context.Background(), batch.ID, 42)
	require.ErrorIs(t, err, ErrConfigImportStale)
	var schemaErr *ConfigImportSchemaError
	require.ErrorAs(t, err, &schemaErr)
	assert.Equal(t, "STALE_BASE_VERSION", schemaErr.Code)
	var savedChannel model.Channel
	require.NoError(t, model.DB.First(&savedChannel, channel.Id).Error)
	assert.Equal(t, "canonical-keep,upstream-keep,concurrent-model", savedChannel.Models)
}

func TestPublishConfigImportBatchActivatesDraftAndAudits(t *testing.T) {
	prepareConfigImportServiceDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Channel{}, &model.ChannelModelCostRule{}, &model.ConfigImportPublishAudit{}))
	channel := &model.Channel{Type: 1, Name: "supplier", Models: "vendor-video", Key: "key"}
	require.NoError(t, model.DB.Create(channel).Error)
	batch := createConfigImportStageBatch(t, channel.Id, "line-a", "vendor-video")
	mapping := types.ConfigImportModelMapping{
		ConfigImportAuthoritativeEntity: types.ConfigImportAuthoritativeEntity{BusinessID: "mapping-keep"},
		CanonicalModel:                  "vendor-video", LineRef: "line-a", UpstreamModel: "vendor-video",
	}
	mappingJSON, err := common.Marshal(mapping)
	require.NoError(t, err)
	require.NoError(t, model.DB.Create(&model.ConfigImportItem{
		BatchID: batch.ID, EntityType: "model_mappings", BusinessID: mapping.BusinessID,
		CanonicalJSON: string(mappingJSON), State: string(types.ConfigImportItemStateNew),
	}).Error)
	_, err = StageConfigImportBatch(context.Background(), 42, batch.ID)
	require.NoError(t, err)
	err = PublishConfigImportBatch(context.Background(), batch.ID, 42)
	require.NoError(t, err)

	var item model.ConfigImportItem
	require.NoError(t, model.DB.Where("batch_id = ? AND entity_type = ?", batch.ID, "cost_rule_drafts").First(&item).Error)
	var rule model.ChannelModelCostRule
	require.NoError(t, model.DB.First(&rule, *item.MaterializedID).Error)
	assert.Equal(t, string(types.CostRuleActive), rule.Status)
	var loadedBatch model.ConfigImportBatch
	require.NoError(t, model.DB.First(&loadedBatch, batch.ID).Error)
	assert.Equal(t, string(types.ConfigImportBatchStatusPublished), loadedBatch.Status)
	var audit model.ConfigImportPublishAudit
	require.NoError(t, model.DB.Where("batch_id = ?", batch.ID).First(&audit).Error)
	assert.Equal(t, "published", audit.Outcome)
	assert.NotEmpty(t, audit.BeforeSHA256)
	_ = common.GetTimestamp()
}
