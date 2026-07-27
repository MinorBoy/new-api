package service

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelrouting"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.NoError(t, model.DB.AutoMigrate(&model.Channel{}, &model.ChannelModelCostRule{}, &model.Option{}, &model.RoutingPolicy{}, &model.RouteTarget{}, &model.ConfigImportPublishAudit{}))
	channel := &model.Channel{Type: 1, Name: "supplier", Models: "vendor-video", Key: "key"}
	require.NoError(t, model.DB.Create(channel).Error)
	batch := createConfigImportStageBatch(t, channel.Id, "line-a", "vendor-video")
	canonicalModel := "seedance-2.0"
	runtimeModel := modelrouting.Seedance20
	mapping := types.ConfigImportModelMapping{ConfigImportAuthoritativeEntity: types.ConfigImportAuthoritativeEntity{BusinessID: "mapping-a"}, CanonicalModel: canonicalModel, ClientModel: canonicalModel, LineRef: "line-a", UpstreamModel: "vendor-video", SKURef: "sku-a"}
	encoded, err := common.Marshal(mapping)
	require.NoError(t, err)
	require.NoError(t, model.DB.Create(&model.ConfigImportItem{BatchID: batch.ID, EntityType: "model_mappings", BusinessID: mapping.BusinessID, CanonicalJSON: string(encoded), State: string(types.ConfigImportItemStateNew)}).Error)
	blueprint := types.ConfigImportRouteBlueprint{
		ConfigImportAuthoritativeEntity: types.ConfigImportAuthoritativeEntity{BusinessID: "route-a"},
		CanonicalModel:                  canonicalModel,
		ClientModel:                     canonicalModel,
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
			ReferenceMinimums: &types.ConfigImportReferenceBounds{},
			ReferenceLimits:   &types.ConfigImportReferenceBounds{},
		}},
	}
	encoded, err = common.Marshal(blueprint)
	require.NoError(t, err)
	require.NoError(t, model.DB.Model(&model.ConfigImportItem{}).Where("batch_id = ? AND business_id = ?", batch.ID, blueprint.BusinessID).Update("canonical_json", string(encoded)).Error)
	_, err = StageConfigImportBatch(context.Background(), 42, batch.ID)
	require.NoError(t, err)
	require.NoError(t, PublishConfigImportBatch(context.Background(), batch.ID, 42))

	var option model.Option
	require.NoError(t, model.DB.Where("key = ?", "ModelPrice").First(&option).Error)
	var prices map[string]float64
	require.NoError(t, common.UnmarshalJsonStr(option.Value, &prices))
	assert.Equal(t, 1.0, prices[runtimeModel])
	var saved model.Channel
	require.NoError(t, model.DB.First(&saved, channel.Id).Error)
	var savedMapping map[string]string
	require.NoError(t, common.UnmarshalJsonStr(saved.GetModelMapping(), &savedMapping))
	assert.Equal(t, "vendor-video", savedMapping[runtimeModel])
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
			ReferenceMinimums: &types.ConfigImportReferenceBounds{}, ReferenceLimits: &types.ConfigImportReferenceBounds{}, Enabled: boolPointer(false),
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

	_, err := StageConfigImportBatch(context.Background(), 42, batch.ID)
	var schemaErr *ConfigImportSchemaError
	require.ErrorAs(t, err, &schemaErr)
	assert.Equal(t, "BINDING_CREDENTIALS_UNCONFIRMED", schemaErr.Code)
	var savedBatch model.ConfigImportBatch
	require.NoError(t, model.DB.First(&savedBatch, batch.ID).Error)
	assert.Equal(t, string(types.ConfigImportBatchStatusBinding), savedBatch.Status)
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
	_, err := StageConfigImportBatch(context.Background(), 42, batch.ID)
	require.NoError(t, err)

	var item model.ConfigImportItem
	require.NoError(t, model.DB.Where("batch_id = ? AND entity_type = ?", batch.ID, "cost_rule_drafts").First(&item).Error)
	var rule model.ChannelModelCostRule
	require.NoError(t, model.DB.First(&rule, *item.MaterializedID).Error)
	require.NoError(t, model.DB.Create(&model.ChannelModelCostRule{ChannelID: channel.Id, BillableUpstreamModel: "unrelated-model", CostVariantKey: "default", Version: 1, Status: string(types.CostRuleActive), CostMode: "per_request", SchemaVersion: 1, ConfigJSON: `{}`}).Error)

	err = PublishConfigImportBatch(context.Background(), batch.ID, 42)
	require.NoError(t, err)
	var loaded model.ChannelModelCostRule
	require.NoError(t, model.DB.First(&loaded, rule.ID).Error)
	assert.Equal(t, string(types.CostRuleActive), loaded.Status)
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

func TestPublishConfigImportBatchActivatesDraftAndAudits(t *testing.T) {
	prepareConfigImportServiceDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Channel{}, &model.ChannelModelCostRule{}, &model.ConfigImportPublishAudit{}))
	channel := &model.Channel{Type: 1, Name: "supplier", Models: "vendor-video", Key: "key"}
	require.NoError(t, model.DB.Create(channel).Error)
	batch := createConfigImportStageBatch(t, channel.Id, "line-a", "vendor-video")
	_, err := StageConfigImportBatch(context.Background(), 42, batch.ID)
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
