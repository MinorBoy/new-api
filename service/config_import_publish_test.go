package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelrouting"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestPublishConfigImportBatchCreatesNormalizedRouteCandidateWithoutApplyingProposals(t *testing.T) {
	prepareConfigImportServiceDB(t)
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
	var saleItem model.ConfigImportItem
	require.NoError(t, model.DB.Where("batch_id = ? AND entity_type = ?", batch.ID, "sale_proposals").First(&saleItem).Error)
	var saleProposal types.ConfigImportSaleProposal
	require.NoError(t, common.UnmarshalJsonStr(saleItem.CanonicalJSON, &saleProposal))
	saleProposal.UnitPrice = nil
	row := 5
	saleProposal.SourceRef = "SRC-OFFICIAL-SEEDANCE-2-0"
	saleProposal.Sheet = "官方售价"
	saleProposal.Row = &row
	saleProposal.Scenario = types.SeedanceTokenScenarioNoVideo
	saleProposal.Resolution = "720p"
	saleProposal.Currency = "USD"
	saleProposal.BillingMode = billing_setting.BillingModeSeedanceTokens
	saleProposal.SeedanceTokenPrice = &types.SeedanceTokenPriceProposal{
		PricePerMillion: "1.91",
		Width:           1280,
		Height:          720,
		FrameRate:       24,
		PricingVersion:  "official-token-v1",
		Source:          "SRC-OFFICIAL-SEEDANCE-2-0!5",
	}
	saleProposal.DurationPrice = nil
	saleJSON, err := common.Marshal(saleProposal)
	require.NoError(t, err)
	require.NoError(t, model.DB.Model(&model.ConfigImportItem{}).Where("id = ?", saleItem.ID).Update("canonical_json", string(saleJSON)).Error)
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

	var optionCount int64
	require.NoError(t, model.DB.Model(&model.Option{}).Count(&optionCount).Error)
	assert.Zero(t, optionCount)
	var saved model.Channel
	require.NoError(t, model.DB.First(&saved, channel.Id).Error)
	assert.Equal(t, "vendor-video", saved.Models)
	assert.Empty(t, saved.GetModelMapping())
	var policy model.RoutingPolicy
	require.NoError(t, model.DB.Where("group_name = ? AND model = ?", "default", runtimeModel).First(&policy).Error)
	assert.False(t, policy.Enabled)
	var candidate model.RouteTarget
	require.NoError(t, model.DB.Where("policy_id = ? AND source_batch_id = ?", policy.ID, batch.ID).First(&candidate).Error)
	assert.Equal(t, channel.Id, candidate.ChannelID)
	assert.Equal(t, "vendor-video", candidate.UpstreamModel)
	assert.False(t, candidate.Enabled)
	assert.Equal(t, string(types.RouteTargetManagedByConfigImport), candidate.ManagedBy)
	var audit model.ConfigImportPublishAudit
	require.NoError(t, model.DB.Where("batch_id = ?", batch.ID).First(&audit).Error)
	assert.NotEmpty(t, audit.AfterSHA256)
	assert.NotEqual(t, audit.BeforeSHA256, audit.AfterSHA256)
	after, err := CaptureConfigImportBaseline(model.DB, batch.ID)
	require.NoError(t, err)
	assert.Equal(t, after.Hash, audit.AfterSHA256)
}

func TestPublishRouteBlueprintUsesDeclaredGroupName(t *testing.T) {
	prepareConfigImportServiceDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.ChannelModelCostRule{}, &model.Option{}, &model.RoutingPolicy{}, &model.RouteTarget{}, &model.ConfigImportPublishAudit{}))
	channel := &model.Channel{Type: 1, Name: "supplier", Models: "vendor-video", Key: "key"}
	require.NoError(t, model.DB.Create(channel).Error)
	batch := createConfigImportStageBatch(t, channel.Id, "line-a", "vendor-video")
	var routeItem model.ConfigImportItem
	require.NoError(t, model.DB.Where("batch_id = ? AND entity_type = ?", batch.ID, "route_blueprints").First(&routeItem).Error)
	var blueprint types.ConfigImportRouteBlueprint
	require.NoError(t, common.UnmarshalJsonStr(routeItem.CanonicalJSON, &blueprint))
	blueprint.GroupName = "vip"
	encoded, err := common.Marshal(blueprint)
	require.NoError(t, err)
	require.NoError(t, model.DB.Model(&routeItem).Update("canonical_json", string(encoded)).Error)
	_, err = StageConfigImportBatch(context.Background(), 42, batch.ID)
	require.NoError(t, err)
	require.NoError(t, PublishConfigImportBatch(context.Background(), batch.ID, 42))

	var policy model.RoutingPolicy
	require.NoError(t, model.DB.Where("group_name = ? AND model = ?", "vip", configImportRuntimeCanonicalModel(blueprint.CanonicalModel)).First(&policy).Error)
	assert.False(t, policy.Enabled)
}

func TestPublishConfigImportBatchDoesNotRecordPostActivationCostCoverage(t *testing.T) {
	prepareConfigImportServiceDB(t)
	require.NoError(t, model.DB.AutoMigrate(
		&model.Channel{}, &model.Ability{}, &model.ChannelModelCostRule{},
		&model.ConfigImportIssue{}, &model.ConfigImportPublishAudit{},
	))
	unrelated := &model.Channel{
		Type: 1, Name: "unrelated-unpriced", Group: "default", Status: common.ChannelStatusEnabled,
		Models: "unrelated-unpriced-model", Key: "unrelated-key",
	}
	require.NoError(t, model.DB.Create(unrelated).Error)
	require.NoError(t, unrelated.UpdateAbilities(model.DB))
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

	var issueCount int64
	require.NoError(t, model.DB.Model(&model.ConfigImportIssue{}).
		Where("batch_id = ? AND code = ?", batch.ID, "COST_COVERAGE_INCOMPLETE").Count(&issueCount).Error)
	assert.Zero(t, issueCount)
}

func TestRecordPostActivationCostCoverageResolvesHistoricalIssuesWhoseBoundChannelsAreCovered(t *testing.T) {
	prepareConfigImportServiceDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.ChannelModelCostRule{}))
	coveredChannel := &model.Channel{
		Type: 1, Name: "covered", Group: "default", Status: common.ChannelStatusEnabled,
		Models: "covered-model", Key: "covered-key",
	}
	uncoveredChannel := &model.Channel{
		Type: 1, Name: "uncovered", Group: "default", Status: common.ChannelStatusEnabled,
		Models: "uncovered-model", Key: "uncovered-key",
	}
	require.NoError(t, model.DB.Create(coveredChannel).Error)
	require.NoError(t, model.DB.Create(uncoveredChannel).Error)
	require.NoError(t, coveredChannel.UpdateAbilities(model.DB))
	require.NoError(t, uncoveredChannel.UpdateAbilities(model.DB))
	createHistoricalCoverageIssue := func(channelID int, batchPayload string) model.ConfigImportIssue {
		batch := model.ConfigImportBatch{
			SchemaVersion: 1, TemplateVersion: "v1", SourceSHA256: strings.Repeat("a", 64),
			PayloadSHA256: batchPayload, Status: string(types.ConfigImportBatchStatusPublished),
			CreatedBy: 42, SummaryJSON: "{}", BaselineJSON: "{}",
		}
		require.NoError(t, model.DB.Create(&batch).Error)
		confirmedAt := int64(1)
		require.NoError(t, model.DB.Create(&model.ConfigImportBinding{
			BatchID: batch.ID, LineRef: fmt.Sprintf("line-%d", channelID), Action: string(types.ConfigImportBindingActionBind),
			ChannelID: &channelID, CredentialsConfirmedBy: 42, CredentialsConfirmedAt: &confirmedAt,
		}).Error)
		issue := model.ConfigImportIssue{
			BatchID: batch.ID, Severity: string(types.ConfigImportIssueSeverityWarning), Code: "COST_COVERAGE_INCOMPLETE",
			Message: "Published configuration has uncovered enabled channel model mappings.", ResolutionStatus: "open",
		}
		require.NoError(t, model.DB.Create(&issue).Error)
		return issue
	}
	coveredIssue := createHistoricalCoverageIssue(coveredChannel.Id, strings.Repeat("b", 64))
	uncoveredIssue := createHistoricalCoverageIssue(uncoveredChannel.Id, strings.Repeat("c", 64))
	currentBatch := model.ConfigImportBatch{
		SchemaVersion: 1, TemplateVersion: "v1", SourceSHA256: strings.Repeat("d", 64),
		PayloadSHA256: strings.Repeat("e", 64), Status: string(types.ConfigImportBatchStatusPublished),
		CreatedBy: 42, SummaryJSON: "{}", BaselineJSON: "{}",
	}
	require.NoError(t, model.DB.Create(&currentBatch).Error)
	confirmedAt := int64(1)
	require.NoError(t, model.DB.Create(&model.ConfigImportBinding{
		BatchID: currentBatch.ID, LineRef: "line-current", Action: string(types.ConfigImportBindingActionBind),
		ChannelID: &coveredChannel.Id, CredentialsConfirmedBy: 42, CredentialsConfirmedAt: &confirmedAt,
	}).Error)
	config, err := NormalizeCostRuleConfig(types.CostModePerRequest, validPerRequestCostConfig())
	require.NoError(t, err)
	configJSON, err := common.Marshal(config)
	require.NoError(t, err)
	require.NoError(t, model.DB.Create(&model.ChannelModelCostRule{
		ChannelID: coveredChannel.Id, BillableUpstreamModel: "covered-model", CostVariantKey: "default", Version: 1,
		Status: string(types.CostRuleActive), CostMode: string(types.CostModePerRequest), SchemaVersion: 1,
		ConfigJSON: string(configJSON),
	}).Error)
	InvalidateCostCoverage(0, "", "")
	previousLookup := CostCapabilityLookup
	CostCapabilityLookup = func(int, string, constant.TaskPlatform) types.CostCapabilities { return completeCostCapabilities() }
	t.Cleanup(func() { CostCapabilityLookup = previousLookup })

	require.NoError(t, recordPostActivationCostCoverage(context.Background(), currentBatch.ID, ConfigImportRefreshKeys{ChannelIDs: []int{coveredChannel.Id}}))

	require.NoError(t, model.DB.First(&coveredIssue, coveredIssue.ID).Error)
	assert.Equal(t, "resolved", coveredIssue.ResolutionStatus)
	assert.Equal(t, "Authoritative cost coverage is complete after publish.", coveredIssue.Message)
	require.NoError(t, model.DB.First(&uncoveredIssue, uncoveredIssue.ID).Error)
	assert.Equal(t, "open", uncoveredIssue.ResolutionStatus)
}

func TestPublishConfigImportBatchDoesNotReplaceBoundChannelModelSnapshot(t *testing.T) {
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
	assert.Equal(t, "canonical-keep,upstream-keep,canonical-old,upstream-old", savedChannel.Models)
	var savedMapping map[string]string
	require.NoError(t, common.UnmarshalJsonStr(savedChannel.GetModelMapping(), &savedMapping))
	assert.Equal(t, map[string]string{
		"canonical-keep": "upstream-keep",
		"canonical-old":  "upstream-old",
	}, savedMapping)
	var abilities []model.Ability
	require.NoError(t, model.DB.Where("channel_id = ?", channel.Id).Order("model ASC").Find(&abilities).Error)
	require.Len(t, abilities, 4)
	assert.Equal(t, []string{"canonical-keep", "canonical-old", "upstream-keep", "upstream-old"}, []string{
		abilities[0].Model, abilities[1].Model, abilities[2].Model, abilities[3].Model,
	})
	require.NoError(t, model.DB.First(oldTarget, oldTarget.ID).Error)
	assert.True(t, oldTarget.Enabled)
	require.NoError(t, model.DB.First(oldRule, oldRule.ID).Error)
	assert.Equal(t, string(types.CostRuleActive), oldRule.Status)
	assert.Nil(t, oldRule.EffectiveTo)
	var candidateCount int64
	require.NoError(t, model.DB.Model(&model.RouteTarget{}).Where("source_batch_id = ?", batch.ID).Count(&candidateCount).Error)
	assert.Zero(t, candidateCount)
}

func TestPublishConfigImportModelSnapshotExcludesExplicitlyUnpricedMapping(t *testing.T) {
	prepareConfigImportServiceDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.ChannelModelCostRule{}))
	channel := &model.Channel{Type: 1, Name: "supplier", Group: "default", Models: "stale-model", Key: "key"}
	require.NoError(t, model.DB.Create(channel).Error)
	confirmedAt := common.GetTimestamp()
	require.NoError(t, model.DB.Create(&model.ConfigImportBinding{
		BatchID: 1, LineRef: "line-a", Action: string(types.ConfigImportBindingActionBind), ChannelID: &channel.Id,
		CredentialsConfirmedBy: 42, CredentialsConfirmedAt: &confirmedAt,
	}).Error)
	mapping := types.ConfigImportModelMapping{
		ConfigImportAuthoritativeEntity: types.ConfigImportAuthoritativeEntity{BusinessID: "mapping-unpriced"},
		CanonicalModel:                  "canonical-video", ClientModel: "canonical-video", LineRef: "line-a", UpstreamModel: "vendor-unpriced", SKURef: "sku-a",
	}
	mappingJSON, err := common.Marshal(mapping)
	require.NoError(t, err)
	disabled := false
	costDraft := types.ConfigImportCostRuleDraft{
		ConfigImportAuthoritativeEntity: types.ConfigImportAuthoritativeEntity{BusinessID: "cost-unpriced"},
		Enabled:                         &disabled, LineRef: "line-a", UpstreamModel: "vendor-unpriced", CostVariantKey: "480p-disabled",
		RouteTargetRef: "route-target/mapping-unpriced", CostMode: string(types.CostModePerDuration),
	}
	costJSON, err := common.Marshal(costDraft)
	require.NoError(t, err)
	items := []model.ConfigImportItem{
		{BatchID: 1, EntityType: "model_mappings", BusinessID: mapping.BusinessID, CanonicalJSON: string(mappingJSON), State: string(types.ConfigImportItemStateNew)},
		{BatchID: 1, EntityType: "cost_rule_drafts", BusinessID: costDraft.BusinessID, CanonicalJSON: string(costJSON), State: string(types.ConfigImportItemStateExcluded), ExclusionReason: "disabled by import document"},
	}

	require.NoError(t, publishConfigImportModelMappings(model.DB, items, &ConfigImportRefreshKeys{}))

	var savedChannel model.Channel
	require.NoError(t, model.DB.First(&savedChannel, channel.Id).Error)
	assert.Empty(t, savedChannel.Models)
	assert.JSONEq(t, `{}`, savedChannel.GetModelMapping())
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

func TestPublishConfigImportSaleOptionsDeletesLegacySeedancePriceMaps(t *testing.T) {
	prepareConfigImportServiceDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Option{}))
	runtimeModel := modelrouting.Seedance20
	for _, key := range []string{"ModelPrice", "ModelRatio", "CompletionRatio"} {
		require.NoError(t, model.DB.Create(&model.Option{
			Key:   key,
			Value: fmt.Sprintf(`{"%s":9,"unrelated-model":2}`, runtimeModel),
		}).Error)
	}

	refresh := ConfigImportRefreshKeys{}
	require.NoError(t, publishConfigImportSaleOptions(model.DB, []model.ConfigImportItem{
		configImportSeedanceTokenPriceSaleItem(t, "sale-no-video", runtimeModel, types.SeedanceTokenScenarioNoVideo, "1.91"),
	}, &refresh))

	for _, key := range []string{"ModelPrice", "ModelRatio", "CompletionRatio"} {
		var option model.Option
		require.NoError(t, model.DB.Where("key = ?", key).First(&option).Error)
		assert.JSONEq(t, `{"unrelated-model":2}`, option.Value, key)
		assert.Contains(t, refresh.OptionKeys, key)
	}
}

func TestPublishConfigImportSaleOptionsDeletesUnpublishedSeedanceSupplierPrices(t *testing.T) {
	prepareConfigImportServiceDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Option{}))
	providerModel := "jimeng-video-seedance-2.0-vip"
	options := map[string]string{
		"ModelPrice":                           fmt.Sprintf(`{%q:9,"unrelated-model":2}`, providerModel),
		"ModelRatio":                           fmt.Sprintf(`{%q:9,"unrelated-model":2}`, providerModel),
		"CompletionRatio":                      fmt.Sprintf(`{%q:9,"unrelated-model":2}`, providerModel),
		"billing_setting.billing_mode":         fmt.Sprintf(`{%q:"per_duration","unrelated-model":"ratio"}`, providerModel),
		"billing_setting.billing_expr":         fmt.Sprintf(`{%q:"v1:tier(\"legacy\", c)","unrelated-model":"v1:tier(\"keep\", c)"}`, providerModel),
		"billing_setting.duration_price":       fmt.Sprintf(`{%q:{"price":0.2,"unit":"second","rounding_step_seconds":1,"minimum_duration_seconds":4},"unrelated-model":{"price":0.3,"unit":"second","rounding_step_seconds":1,"minimum_duration_seconds":0}}`, providerModel),
		"billing_setting.seedance_token_price": fmt.Sprintf(`{%q:{"scenarios":{"720p:no_video":{"price_per_million":"9","width":1280,"height":720,"frame_rate":24,"pricing_version":"legacy-token-v1","source":"legacy"}}},"unrelated-model":{"scenarios":{"720p:no_video":{"price_per_million":"2","width":1280,"height":720,"frame_rate":24,"pricing_version":"keep-token-v1","source":"keep"}}}}`, providerModel),
	}
	for key, value := range options {
		require.NoError(t, model.DB.Create(&model.Option{Key: key, Value: value}).Error)
	}

	refresh := ConfigImportRefreshKeys{}
	require.NoError(t, publishConfigImportSaleOptions(model.DB, []model.ConfigImportItem{
		configImportSeedanceTokenPriceSaleItem(t, "sale-official", modelrouting.Seedance20, types.SeedanceTokenScenarioNoVideo, "1.91"),
	}, &refresh))

	for key := range options {
		var option model.Option
		require.NoError(t, model.DB.Where("key = ?", key).First(&option).Error)
		var values map[string]any
		require.NoError(t, common.UnmarshalJsonStr(option.Value, &values))
		assert.NotContains(t, values, providerModel, key)
		assert.Contains(t, values, "unrelated-model", key)
	}

	var tokenPriceOption model.Option
	require.NoError(t, model.DB.Where("key = ?", "billing_setting.seedance_token_price").First(&tokenPriceOption).Error)
	var prices map[string]types.SeedanceTokenPrice
	require.NoError(t, common.UnmarshalJsonStr(tokenPriceOption.Value, &prices))
	require.Contains(t, prices, modelrouting.Seedance20)
	assert.Contains(t, prices[modelrouting.Seedance20].Scenarios, "720p:no_video")
}

func TestPublishConfigImportSaleOptionsDeletesMappedSeedanceUpstreamAliasPrices(t *testing.T) {
	prepareConfigImportServiceDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Option{}))
	upstreamAlias := "video-2.0-mini"
	options := map[string]string{
		"ModelPrice":                           fmt.Sprintf(`{%q:9}`, upstreamAlias),
		"ModelRatio":                           fmt.Sprintf(`{%q:9}`, upstreamAlias),
		"CompletionRatio":                      fmt.Sprintf(`{%q:9}`, upstreamAlias),
		"billing_setting.billing_mode":         fmt.Sprintf(`{%q:"per_duration"}`, upstreamAlias),
		"billing_setting.billing_expr":         fmt.Sprintf(`{%q:"v1:tier(\"legacy\", c)"}`, upstreamAlias),
		"billing_setting.duration_price":       fmt.Sprintf(`{%q:{"price":0.2,"unit":"second","rounding_step_seconds":1,"minimum_duration_seconds":4}}`, upstreamAlias),
		"billing_setting.seedance_token_price": fmt.Sprintf(`{%q:{"scenarios":{"720p:no_video":{"price_per_million":"9","width":1280,"height":720,"frame_rate":24,"pricing_version":"legacy-token-v1","source":"legacy"}}}}`, upstreamAlias),
	}
	for key, value := range options {
		require.NoError(t, model.DB.Create(&model.Option{Key: key, Value: value}).Error)
	}
	mappingJSON, err := common.Marshal(types.ConfigImportModelMapping{
		CanonicalModel: modelrouting.Seedance20Mini,
		ClientModel:    modelrouting.Seedance20Mini,
		UpstreamModel:  upstreamAlias,
	})
	require.NoError(t, err)

	items := []model.ConfigImportItem{
		configImportSeedanceTokenPriceSaleItem(t, "sale-official", modelrouting.Seedance20Mini, types.SeedanceTokenScenarioNoVideo, "1.91"),
		{EntityType: "model_mappings", BusinessID: "mapping-mini", CanonicalJSON: string(mappingJSON), State: string(types.ConfigImportItemStateNew)},
	}
	require.NoError(t, publishConfigImportSaleOptions(model.DB, items, &ConfigImportRefreshKeys{}))

	for key := range options {
		var option model.Option
		require.NoError(t, model.DB.Where("key = ?", key).First(&option).Error)
		var values map[string]any
		require.NoError(t, common.UnmarshalJsonStr(option.Value, &values))
		assert.NotContains(t, values, upstreamAlias, key)
	}
}

func TestPublishConfigImportSaleOptionsDeletesAliasFromPreviouslyPublishedImport(t *testing.T) {
	prepareConfigImportServiceDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Option{}, &model.ConfigImportBatch{}, &model.ConfigImportItem{}))
	retiredAlias := "retired-video-2.0-provider"
	seedPublishedSeedanceAliasImport(t, retiredAlias)
	options := map[string]string{
		"ModelPrice":                           fmt.Sprintf(`{%q:9,"unrelated-model":2}`, retiredAlias),
		"ModelRatio":                           fmt.Sprintf(`{%q:9,"unrelated-model":2}`, retiredAlias),
		"CompletionRatio":                      fmt.Sprintf(`{%q:9,"unrelated-model":2}`, retiredAlias),
		"billing_setting.billing_mode":         fmt.Sprintf(`{%q:"per_duration","unrelated-model":"ratio"}`, retiredAlias),
		"billing_setting.billing_expr":         fmt.Sprintf(`{%q:"v1:tier(\"legacy\", c)","unrelated-model":"v1:tier(\"keep\", c)"}`, retiredAlias),
		"billing_setting.duration_price":       fmt.Sprintf(`{%q:{"price":0.2,"unit":"second","rounding_step_seconds":1,"minimum_duration_seconds":4},"unrelated-model":{"price":0.3,"unit":"second","rounding_step_seconds":1,"minimum_duration_seconds":0}}`, retiredAlias),
		"billing_setting.seedance_token_price": fmt.Sprintf(`{%q:{"scenarios":{"720p:no_video":{"price_per_million":"9","width":1280,"height":720,"frame_rate":24,"pricing_version":"legacy-token-v1","source":"legacy"}}},"unrelated-model":{"scenarios":{"720p:no_video":{"price_per_million":"2","width":1280,"height":720,"frame_rate":24,"pricing_version":"keep-token-v1","source":"keep"}}}}`, retiredAlias),
	}
	for key, value := range options {
		require.NoError(t, model.DB.Create(&model.Option{Key: key, Value: value}).Error)
	}

	require.NoError(t, publishConfigImportSaleOptions(model.DB, []model.ConfigImportItem{
		configImportSeedanceTokenPriceSaleItem(t, "sale-current", modelrouting.Seedance20, types.SeedanceTokenScenarioNoVideo, "1.91"),
	}, &ConfigImportRefreshKeys{}))

	for key := range options {
		var option model.Option
		require.NoError(t, model.DB.Where("key = ?", key).First(&option).Error)
		var values map[string]any
		require.NoError(t, common.UnmarshalJsonStr(option.Value, &values))
		assert.NotContains(t, values, retiredAlias, key)
		assert.Contains(t, values, "unrelated-model", key)
	}
}

func TestConfigImportBaselineTracksUnchangedOfficialSaleHistoricalAliases(t *testing.T) {
	prepareConfigImportServiceDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Option{}, &model.ConfigImportBatch{}, &model.ConfigImportItem{}))
	retiredAlias := "retired-videos-provider-model"
	seedPublishedSeedanceAliasImport(t, retiredAlias)
	currentBatch := model.ConfigImportBatch{
		SchemaVersion: 1, TemplateVersion: "test", SourceSHA256: strings.Repeat("c", 64),
		PayloadSHA256: strings.Repeat("d", 64), Status: string(types.ConfigImportBatchStatusStaged),
		CreatedBy: 42, BaselineJSON: "{}", SummaryJSON: "{}",
	}
	require.NoError(t, model.DB.Create(&currentBatch).Error)
	currentSale := configImportSeedanceTokenPriceSaleItem(t, "sale-current", modelrouting.Seedance20, types.SeedanceTokenScenarioNoVideo, "1.91")
	currentSale.BatchID = currentBatch.ID
	currentSale.State = string(types.ConfigImportItemStateUnchanged)
	require.NoError(t, model.DB.Create(&currentSale).Error)
	for _, key := range configImportSeedanceSaleOptionKeys {
		require.NoError(t, model.DB.Create(&model.Option{Key: key, Value: fmt.Sprintf(`{%q:9}`, retiredAlias)}).Error)
	}

	baseline, err := captureConfigImportBaseline(model.DB, currentBatch.ID)
	require.NoError(t, err)

	for _, key := range configImportSeedanceSaleOptionKeys {
		assert.Contains(t, baseline.Options, key+"|"+retiredAlias, key)
	}
}

func TestConfigImportBaselineTracksCurrentSeedanceKeysOutsideImportHistory(t *testing.T) {
	prepareConfigImportServiceDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Option{}, &model.ConfigImportBatch{}, &model.ConfigImportItem{}))
	currentBatch := model.ConfigImportBatch{
		SchemaVersion: 1, TemplateVersion: "test", SourceSHA256: strings.Repeat("e", 64),
		PayloadSHA256: strings.Repeat("f", 64), Status: string(types.ConfigImportBatchStatusStaged),
		CreatedBy: 42, BaselineJSON: "{}", SummaryJSON: "{}",
	}
	require.NoError(t, model.DB.Create(&currentBatch).Error)
	currentSale := configImportSeedanceTokenPriceSaleItem(t, "sale-current", modelrouting.Seedance20, types.SeedanceTokenScenarioNoVideo, "1.91")
	currentSale.BatchID = currentBatch.ID
	currentSale.State = string(types.ConfigImportItemStateUnchanged)
	require.NoError(t, model.DB.Create(&currentSale).Error)

	currentOnlyKey := "manual-seedance-2-0-shadow"
	for _, key := range configImportSeedanceSaleOptionKeys {
		require.NoError(t, model.DB.Create(&model.Option{Key: key, Value: fmt.Sprintf(`{%q:9,"unrelated-model":2}`, currentOnlyKey)}).Error)
	}

	baseline, err := captureConfigImportBaseline(model.DB, currentBatch.ID)
	require.NoError(t, err)

	for _, key := range configImportSeedanceSaleOptionKeys {
		assert.Contains(t, baseline.Options, key+"|"+currentOnlyKey, key)
		assert.NotContains(t, baseline.Options, key+"|unrelated-model", key)
	}
}

func TestPublishConfigImportSaleOptionsUnchangedSeedanceProposalDeletesLegacyPriceMaps(t *testing.T) {
	prepareConfigImportServiceDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Option{}))
	runtimeModel := modelrouting.Seedance20
	for _, key := range []string{"ModelPrice", "ModelRatio", "CompletionRatio"} {
		require.NoError(t, model.DB.Create(&model.Option{
			Key:   key,
			Value: fmt.Sprintf(`{"%s":9,"unrelated-model":2}`, runtimeModel),
		}).Error)
	}
	item := configImportSeedanceTokenPriceSaleItem(t, "sale-no-video", runtimeModel, types.SeedanceTokenScenarioNoVideo, "1.91")
	item.State = string(types.ConfigImportItemStateUnchanged)

	refresh := ConfigImportRefreshKeys{}
	require.NoError(t, publishConfigImportSaleOptions(model.DB, []model.ConfigImportItem{item}, &refresh))

	for _, key := range []string{"ModelPrice", "ModelRatio", "CompletionRatio"} {
		var option model.Option
		require.NoError(t, model.DB.Where("key = ?", key).First(&option).Error)
		assert.JSONEq(t, `{"unrelated-model":2}`, option.Value, key)
		assert.Contains(t, refresh.OptionKeys, key)
	}
}

func TestPublishConfigImportSaleOptionsMergesSeedanceTokenPriceScenarios(t *testing.T) {
	prepareConfigImportServiceDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Option{}))
	runtimeModel := modelrouting.Seedance20
	items := []model.ConfigImportItem{
		configImportSeedanceTokenPriceSaleItem(t, "sale-no-video", runtimeModel, types.SeedanceTokenScenarioNoVideo, "1.91"),
		configImportSeedanceTokenPriceSaleItem(t, "sale-with-video", runtimeModel, types.SeedanceTokenScenarioWithVideo, "1.23"),
	}

	refresh := ConfigImportRefreshKeys{}
	tx := model.DB.Begin()
	require.NoError(t, tx.Error)
	require.NoError(t, publishConfigImportSaleOptions(tx, items, &refresh))
	require.NoError(t, tx.Commit().Error)

	var option model.Option
	require.NoError(t, model.DB.Where("key = ?", "billing_setting.seedance_token_price").First(&option).Error)
	var prices map[string]types.SeedanceTokenPrice
	require.NoError(t, common.UnmarshalJsonStr(option.Value, &prices))
	price, found := prices[runtimeModel]
	require.True(t, found)
	require.Len(t, price.Scenarios, 2)
	assert.Equal(t, "1.91", price.Scenarios["720p:no_video"].PricePerMillion)
	assert.Equal(t, "1.23", price.Scenarios["720p:with_video"].PricePerMillion)
	assert.Equal(t, 1280, price.Scenarios["720p:no_video"].Width)
	assert.Equal(t, 24, price.Scenarios["720p:with_video"].FrameRate)
}

func TestPublishConfigImportSaleOptionsReplacesStaleSeedanceTokenScenarios(t *testing.T) {
	prepareConfigImportServiceDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Option{}))
	runtimeModel := modelrouting.Seedance20
	initialTokenPrice := fmt.Sprintf(`{
		%q:{
			"scenarios":{
				"720p:no_video":{"price_per_million":"9","width":1280,"height":720,"frame_rate":24,"pricing_version":"legacy-token-v1","source":"legacy"},
				"720p:with_video":{"price_per_million":"8","width":1280,"height":720,"frame_rate":24,"pricing_version":"legacy-token-v1","source":"legacy"}
			}
		}
	}`, runtimeModel)
	require.NoError(t, model.DB.Create(&model.Option{Key: "billing_setting.seedance_token_price", Value: initialTokenPrice}).Error)

	refresh := ConfigImportRefreshKeys{}
	require.NoError(t, publishConfigImportSaleOptions(model.DB, []model.ConfigImportItem{
		configImportSeedanceTokenPriceSaleItem(t, "sale-no-video", runtimeModel, types.SeedanceTokenScenarioNoVideo, "1.91"),
	}, &refresh))

	var option model.Option
	require.NoError(t, model.DB.Where("key = ?", "billing_setting.seedance_token_price").First(&option).Error)
	var prices map[string]types.SeedanceTokenPrice
	require.NoError(t, common.UnmarshalJsonStr(option.Value, &prices))
	price := prices[runtimeModel]
	require.Len(t, price.Scenarios, 1)
	assert.Equal(t, "1.91", price.Scenarios["720p:no_video"].PricePerMillion)
	assert.NotContains(t, price.Scenarios, "720p:with_video")
}

func TestPublishConfigImportSaleOptionsReplacesInvalidPublishedSeedanceTokenSnapshot(t *testing.T) {
	prepareConfigImportServiceDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Option{}))
	runtimeModel := modelrouting.Seedance20
	require.NoError(t, model.DB.Create(&model.Option{
		Key: "billing_setting.seedance_token_price",
		Value: fmt.Sprintf(`{%q:{"scenarios":{
			"720p:with_video":{"price_per_million":"9","width":1280,"height":720,"frame_rate":24,"pricing_version":"legacy-token-v1"}
		}}}`, runtimeModel),
	}).Error)

	err := publishConfigImportSaleOptions(model.DB, []model.ConfigImportItem{
		configImportSeedanceTokenPriceSaleItem(t, "sale-no-video", runtimeModel, types.SeedanceTokenScenarioNoVideo, "1.91"),
	}, &ConfigImportRefreshKeys{})

	require.NoError(t, err)
	var option model.Option
	require.NoError(t, model.DB.Where("key = ?", "billing_setting.seedance_token_price").First(&option).Error)
	var prices map[string]types.SeedanceTokenPrice
	require.NoError(t, common.UnmarshalJsonStr(option.Value, &prices))
	assert.Equal(t, "1.91", prices[runtimeModel].Scenarios["720p:no_video"].PricePerMillion)
	assert.NotContains(t, prices[runtimeModel].Scenarios, "720p:with_video")
}

func TestPublishConfigImportSaleOptionsOverwritesFlatDurationPrice(t *testing.T) {
	prepareConfigImportServiceDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Option{}))
	require.NoError(t, model.DB.Create(&model.Option{
		Key:   "billing_setting.duration_price",
		Value: `{"ordinary-video":{"price":0.12,"unit":"second","rounding_step_seconds":1,"minimum_duration_seconds":0}}`,
	}).Error)
	document := map[string]any{
		"staged_proposal": map[string]any{
			"option_patches": map[string]any{
				"billing_setting.duration_price": map[string]any{
					"ordinary-video": map[string]any{
						"price":                    0.18,
						"unit":                     types.DurationUnitSecond,
						"rounding_step_seconds":    1,
						"minimum_duration_seconds": 0,
					},
				},
			},
		},
	}
	encoded, err := common.Marshal(document)
	require.NoError(t, err)

	refresh := ConfigImportRefreshKeys{}
	err = publishConfigImportSaleOptions(model.DB, []model.ConfigImportItem{{
		EntityType:    "sale_proposals",
		BusinessID:    "sale-ordinary-video",
		CanonicalJSON: string(encoded),
		State:         string(types.ConfigImportItemStateNew),
	}}, &refresh)
	require.NoError(t, err)

	var option model.Option
	require.NoError(t, model.DB.Where("key = ?", "billing_setting.duration_price").First(&option).Error)
	var prices map[string]types.DurationPrice
	require.NoError(t, common.UnmarshalJsonStr(option.Value, &prices))
	price := prices["ordinary-video"]
	assert.Equal(t, 0.18, price.Price)
}

func TestPublishConfigImportSaleOptionsRejectsConflictingSeedanceTokenPriceWithoutPartialWrite(t *testing.T) {
	prepareConfigImportServiceDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Option{}))
	initialTokenPrice := `{"existing-model":{"scenarios":{"720p:no_video":{"price_per_million":"1","width":1280,"height":720,"frame_rate":24,"pricing_version":"official-token-v1","source":"official"}}}}`
	initialBillingMode := `{"existing-model":"per_duration"}`
	require.NoError(t, model.DB.Create(&model.Option{Key: "billing_setting.seedance_token_price", Value: initialTokenPrice}).Error)
	require.NoError(t, model.DB.Create(&model.Option{Key: "billing_setting.billing_mode", Value: initialBillingMode}).Error)
	runtimeModel := modelrouting.Seedance20
	items := []model.ConfigImportItem{
		configImportSeedanceTokenPriceSaleItem(t, "sale-first", runtimeModel, types.SeedanceTokenScenarioNoVideo, "1.91"),
		configImportSeedanceTokenPriceSaleItem(t, "sale-conflict", runtimeModel, types.SeedanceTokenScenarioNoVideo, "1.92"),
	}

	refresh := ConfigImportRefreshKeys{}
	err := publishConfigImportSaleOptions(model.DB, items, &refresh)
	var schemaErr *ConfigImportSchemaError
	require.ErrorAs(t, err, &schemaErr)
	assert.Equal(t, "PUBLISH_PRICING_CONFLICT", schemaErr.Code)
	assert.Empty(t, refresh.OptionKeys)

	var tokenPriceOption model.Option
	require.NoError(t, model.DB.Where("key = ?", "billing_setting.seedance_token_price").First(&tokenPriceOption).Error)
	assert.Equal(t, initialTokenPrice, tokenPriceOption.Value)
	var billingModeOption model.Option
	require.NoError(t, model.DB.Where("key = ?", "billing_setting.billing_mode").First(&billingModeOption).Error)
	assert.Equal(t, initialBillingMode, billingModeOption.Value)
	var billingExprCount int64
	require.NoError(t, model.DB.Model(&model.Option{}).Where("key = ?", "billing_setting.billing_expr").Count(&billingExprCount).Error)
	assert.Zero(t, billingExprCount)
}

func configImportSeedanceTokenPriceSaleItem(t *testing.T, businessID, runtimeModel, scenario, price string) model.ConfigImportItem {
	t.Helper()
	proposal := configImportOfficialSeedanceSaleProposalForTest()
	proposal.BusinessID = businessID
	proposal.Scenario = scenario
	proposal.SeedanceTokenPrice.PricePerMillion = price
	patches, err := configImportSaleOptionPatches(proposal, runtimeModel)
	require.NoError(t, err)
	document := map[string]any{
		"staged_proposal": map[string]any{
			"option_patches": patches,
		},
	}
	encoded, err := common.Marshal(document)
	require.NoError(t, err)
	return model.ConfigImportItem{
		EntityType:    "sale_proposals",
		BusinessID:    businessID,
		CanonicalJSON: string(encoded),
		State:         string(types.ConfigImportItemStateNew),
	}
}

func seedPublishedSeedanceAliasImport(t *testing.T, upstreamAlias string) {
	t.Helper()
	batch := model.ConfigImportBatch{
		SchemaVersion: 1, TemplateVersion: "historical", SourceSHA256: strings.Repeat("a", 64),
		PayloadSHA256: strings.Repeat("b", 64), Status: string(types.ConfigImportBatchStatusPublished),
		CreatedBy: 42, BaselineJSON: "{}", SummaryJSON: "{}",
	}
	require.NoError(t, model.DB.Create(&batch).Error)
	mappingJSON, err := common.Marshal(types.ConfigImportModelMapping{
		CanonicalModel: modelrouting.Seedance20,
		ClientModel:    modelrouting.Seedance20,
		UpstreamModel:  upstreamAlias,
	})
	require.NoError(t, err)
	require.NoError(t, model.DB.Create(&model.ConfigImportItem{
		BatchID: batch.ID, EntityType: "model_mappings", BusinessID: "historical-mapping",
		CanonicalJSON: string(mappingJSON), State: string(types.ConfigImportItemStateNew),
	}).Error)
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

func TestConfigImportRouteRowsPreservesAggregateReferenceConstraints(t *testing.T) {
	total := 12
	videoAudioTotal := 3
	videoDuration := 15
	blueprint := types.ConfigImportRouteBlueprint{
		CanonicalModel: modelrouting.Seedance20,
		Targets: []types.ConfigImportRouteTarget{{
			RouteTargetRef: "aggregate-reference-limits", LineRef: "line-a", UpstreamModel: "model-a",
			SKURef: "sku-a", CostVariantKey: "default",
			ReferenceMinimums: configImportReferenceBounds(1, 0, 0), ReferenceLimits: configImportReferenceBounds(9, 3, 3),
			ReferenceTotalMax: &total, ReferenceVideoAudioTotalMax: &videoAudioTotal,
			ReferenceVideoTotalDurationSeconds: &videoDuration,
			ReferenceModes:                     []string{"first_last_frames", "omni_reference"},
		}},
	}

	_, targets, err := configImportRouteRows(map[string]int{"line-a": 21}, blueprint)
	require.NoError(t, err)
	require.Len(t, targets, 1)
	var constraints modelrouting.Constraints
	require.NoError(t, common.UnmarshalJsonStr(targets[0].Constraints, &constraints))
	assert.Equal(t, &total, constraints.ReferenceTotalMax)
	assert.Equal(t, &videoAudioTotal, constraints.ReferenceVideoAudioTotalMax)
	assert.Equal(t, &videoDuration, constraints.ReferenceVideoTotalDurationSeconds)
	assert.Equal(t, []string{"first_last_frames", "omni_reference"}, constraints.ReferenceModes)
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
	assert.Empty(t, refresh.RoutingPolicyKeys)

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
	assert.True(t, replaced.Enabled)
	require.Len(t, replaced.Targets, 2)
	var replacement model.RouteTarget
	for _, target := range replaced.Targets {
		if target.Name == "replacement" {
			replacement = target
		}
	}
	assert.Equal(t, "replacement-model", replacement.UpstreamModel)
	assert.False(t, replacement.Enabled)
	assert.Equal(t, string(types.RouteTargetManagedByConfigImport), replacement.ManagedBy)
	require.NotNil(t, replacement.SourceBatchID)
	assert.Equal(t, int64(1), *replacement.SourceBatchID)

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
	assert.Equal(t, "preserved", skipped.Targets[0].Name)
	assert.True(t, skipped.Targets[0].Enabled)
}

func TestPublishConfigImportBatchCreatesDisabledCandidateWithoutChangingActiveConfiguration(t *testing.T) {
	prepareConfigImportServiceDB(t)
	require.NoError(t, model.DB.AutoMigrate(
		&model.Channel{}, &model.Ability{}, &model.ChannelModelCostRule{}, &model.Option{},
		&model.RoutingPolicy{}, &model.RouteTarget{}, &model.ConfigImportPublishAudit{},
	))
	existingMapping := `{"canonical-old":"upstream-old"}`
	channel := &model.Channel{
		Type: 1, Name: "supplier", Group: "default", Status: common.ChannelStatusEnabled,
		Models: "canonical-old,upstream-old", Key: "key", ModelMapping: &existingMapping,
	}
	require.NoError(t, model.DB.Create(channel).Error)
	require.NoError(t, channel.UpdateAbilities(model.DB))
	activeRule := &model.ChannelModelCostRule{
		ChannelID: channel.Id, BillableUpstreamModel: "vendor-video", CostVariantKey: "default", Version: 1,
		Status: string(types.CostRuleActive), CostMode: string(types.CostModePerRequest), SchemaVersion: 1, ConfigJSON: `{}`,
	}
	require.NoError(t, model.DB.Create(activeRule).Error)
	previousOptionValue := `{"canonical-video":9}`
	require.NoError(t, model.DB.Create(&model.Option{Key: "ModelPrice", Value: previousOptionValue}).Error)

	constraints, err := common.Marshal(modelrouting.Constraints{
		OutputResolutions: []string{"1080p"},
		Durations:         modelrouting.DurationConstraint{Values: []int{10}},
		AspectRatios:      []string{"16:9"},
		InputModes:        []modelrouting.InputMode{modelrouting.InputModeText},
		ReferenceMinimums: modelrouting.ReferenceLimits{},
		ReferenceLimits:   modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3},
	})
	require.NoError(t, err)
	policy := model.RoutingPolicy{
		GroupName: "default", Model: "canonical-video", Enabled: true,
		DefaultResolution: "1080p", DefaultDuration: 10, DefaultRatio: "16:9",
	}
	require.NoError(t, model.DB.Create(&policy).Error)
	manual := model.RouteTarget{
		PolicyID: policy.ID, ChannelID: channel.Id, Name: "manual", UpstreamModel: "upstream-old",
		CostVariantKey: "default", TargetPriority: 100, Constraints: string(constraints), Enabled: true,
		ManagedBy: string(types.RouteTargetManagedByManual),
	}
	require.NoError(t, model.DB.Create(&manual).Error)
	previousBatchID := int64(19)
	previousImported := model.RouteTarget{
		PolicyID: policy.ID, ChannelID: channel.Id, Name: "previous-import", UpstreamModel: "upstream-old",
		CostVariantKey: "default", TargetPriority: 90, Constraints: string(constraints), Enabled: true,
		ManagedBy: string(types.RouteTargetManagedByConfigImport), SourceBatchID: &previousBatchID,
	}
	require.NoError(t, model.DB.Create(&previousImported).Error)

	batch := createConfigImportStageBatch(t, channel.Id, "line-a", "vendor-video")
	mapping := types.ConfigImportModelMapping{
		ConfigImportAuthoritativeEntity: types.ConfigImportAuthoritativeEntity{BusinessID: "mapping-current"},
		CanonicalModel:                  "canonical-video", ClientModel: "canonical-video", LineRef: "line-a", UpstreamModel: "vendor-video", SKURef: "sku-a",
	}
	mappingJSON, err := common.Marshal(mapping)
	require.NoError(t, err)
	require.NoError(t, model.DB.Create(&model.ConfigImportItem{
		BatchID: batch.ID, EntityType: "model_mappings", BusinessID: mapping.BusinessID,
		CanonicalJSON: string(mappingJSON), State: string(types.ConfigImportItemStateNew),
	}).Error)

	staged, err := StageConfigImportBatch(context.Background(), 42, batch.ID)
	require.NoError(t, err)
	require.Equal(t, types.ConfigImportBatchStatusReady, staged.Status)
	var costItem model.ConfigImportItem
	require.NoError(t, model.DB.Where("batch_id = ? AND entity_type = ?", batch.ID, "cost_rule_drafts").First(&costItem).Error)
	require.NotNil(t, costItem.MaterializedID)
	var currentCostDraft model.ChannelModelCostRule
	require.NoError(t, model.DB.First(&currentCostDraft, *costItem.MaterializedID).Error)
	require.Equal(t, string(types.CostRuleDraft), currentCostDraft.Status)

	require.NoError(t, PublishConfigImportBatch(context.Background(), batch.ID, 42))

	require.NoError(t, model.DB.First(activeRule, activeRule.ID).Error)
	assert.Equal(t, string(types.CostRuleActive), activeRule.Status)
	require.NoError(t, model.DB.First(&currentCostDraft, currentCostDraft.ID).Error)
	assert.Equal(t, string(types.CostRuleDraft), currentCostDraft.Status)
	var loadedOption model.Option
	require.NoError(t, model.DB.Where("key = ?", "ModelPrice").First(&loadedOption).Error)
	assert.Equal(t, previousOptionValue, loadedOption.Value)
	var loadedChannel model.Channel
	require.NoError(t, model.DB.First(&loadedChannel, channel.Id).Error)
	assert.Equal(t, "canonical-old,upstream-old", loadedChannel.Models)
	assert.JSONEq(t, existingMapping, loadedChannel.GetModelMapping())
	require.NoError(t, model.DB.First(&manual, manual.ID).Error)
	assert.True(t, manual.Enabled)
	assert.Equal(t, string(types.RouteTargetManagedByManual), manual.ManagedBy)
	require.NoError(t, model.DB.First(&previousImported, previousImported.ID).Error)
	assert.True(t, previousImported.Enabled)
	assert.Nil(t, previousImported.RetiredAt)

	var currentCandidate model.RouteTarget
	require.NoError(t, model.DB.Where("source_batch_id = ?", batch.ID).First(&currentCandidate).Error)
	assert.False(t, currentCandidate.Enabled)
	assert.Equal(t, string(types.RouteTargetManagedByConfigImport), currentCandidate.ManagedBy)
	require.NotNil(t, currentCandidate.SourceBatchID)
	assert.Equal(t, batch.ID, *currentCandidate.SourceBatchID)
	var loadedPolicy model.RoutingPolicy
	require.NoError(t, model.DB.First(&loadedPolicy, policy.ID).Error)
	assert.True(t, loadedPolicy.Enabled)
	assert.Equal(t, "1080p", loadedPolicy.DefaultResolution)
	assert.Equal(t, 10, loadedPolicy.DefaultDuration)
	assert.Equal(t, "16:9", loadedPolicy.DefaultRatio)
	var loadedBatch model.ConfigImportBatch
	require.NoError(t, model.DB.First(&loadedBatch, batch.ID).Error)
	assert.Equal(t, string(types.ConfigImportBatchStatusPublished), loadedBatch.Status)
	assert.NotEmpty(t, loadedBatch.BaselineJSON)
	var storedBaseline ConfigImportBaseline
	require.NoError(t, common.UnmarshalJsonStr(string(loadedBatch.BaselineJSON), &storedBaseline))
	currentBaseline, err := CaptureConfigImportBaseline(model.DB, batch.ID)
	require.NoError(t, err)
	assert.Equal(t, currentBaseline.Hash, storedBaseline.Hash)
	var cacheIssueCount int64
	require.NoError(t, model.DB.Model(&model.ConfigImportIssue{}).
		Where("batch_id = ? AND code = ?", batch.ID, "CACHE_REFRESH_PENDING").Count(&cacheIssueCount).Error)
	assert.Zero(t, cacheIssueCount)
}

func configImportReferenceBounds(images, videos, audios int) *types.ConfigImportReferenceBounds {
	return &types.ConfigImportReferenceBounds{
		Images: &images,
		Videos: &videos,
		Audios: &audios,
	}
}

func TestPublishConfigImportBatchRejectsInvalidCandidateWithoutApplyingConfiguration(t *testing.T) {
	prepareConfigImportServiceDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Channel{}, &model.ChannelModelCostRule{}, &model.Option{}, &model.RoutingPolicy{}, &model.RouteTarget{}, &model.ConfigImportPublishAudit{}))
	channel := &model.Channel{Type: 1, Name: "supplier", Models: "vendor-video", Key: "key"}
	require.NoError(t, model.DB.Create(channel).Error)
	batch := createConfigImportStageBatch(t, channel.Id, "line-a", "vendor-video")
	mapping := types.ConfigImportModelMapping{
		ConfigImportAuthoritativeEntity: types.ConfigImportAuthoritativeEntity{BusinessID: "mapping-a"},
		CanonicalModel:                  "canonical-video",
		ClientModel:                     "canonical-video",
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
	var routeItem model.ConfigImportItem
	require.NoError(t, model.DB.Where("batch_id = ? AND entity_type = ?", batch.ID, "route_blueprints").First(&routeItem).Error)
	var blueprint types.ConfigImportRouteBlueprint
	require.NoError(t, common.UnmarshalJsonStr(routeItem.CanonicalJSON, &blueprint))
	blueprint.Targets[0].ReferenceLimits = nil
	encoded, err = common.Marshal(blueprint)
	require.NoError(t, err)
	require.NoError(t, model.DB.Model(&routeItem).Update("canonical_json", string(encoded)).Error)

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
	assert.Equal(t, "PUBLISH_ROUTE_REFERENCE", savedBatch.FailureCode)
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

	require.Equal(t, []string{"refresh_cache", "copy_for_binding"}, configImportAllowedActions(types.ConfigImportBatchStatusPublished, nil, []model.ConfigImportIssue{{Code: "CACHE_REFRESH_PENDING", ResolutionStatus: "open"}}))
	require.NoError(t, RetryConfigImportBatchCache(context.Background(), batch.ID, 42))
	var issue model.ConfigImportIssue
	require.NoError(t, model.DB.Where("batch_id = ? AND code = ?", batch.ID, "CACHE_REFRESH_PENDING").First(&issue).Error)
	assert.Equal(t, "resolved", issue.ResolutionStatus)
	var auditCount int64
	require.NoError(t, model.DB.Model(&model.ConfigImportPublishAudit{}).Where("batch_id = ?", batch.ID).Count(&auditCount).Error)
	assert.Equal(t, int64(1), auditCount)
}

func TestPublishConfigImportBatchKeepsActiveCostRuleMissingFromAuthoritativeSnapshot(t *testing.T) {
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
	unrelatedRule := &model.ChannelModelCostRule{ChannelID: channel.Id, BillableUpstreamModel: "unrelated-model", CostVariantKey: "default", Version: 1, Status: string(types.CostRuleActive), CostMode: "per_request", SchemaVersion: 1, ConfigJSON: `{}`}
	require.NoError(t, model.DB.Create(unrelatedRule).Error)
	_, err = StageConfigImportBatch(context.Background(), 42, batch.ID)
	require.NoError(t, err)

	var item model.ConfigImportItem
	require.NoError(t, model.DB.Where("batch_id = ? AND entity_type = ?", batch.ID, "cost_rule_drafts").First(&item).Error)
	var rule model.ChannelModelCostRule
	require.NoError(t, model.DB.First(&rule, *item.MaterializedID).Error)

	err = PublishConfigImportBatch(context.Background(), batch.ID, 42)
	require.NoError(t, err)
	var loaded model.ChannelModelCostRule
	require.NoError(t, model.DB.First(&loaded, rule.ID).Error)
	assert.Equal(t, string(types.CostRuleDraft), loaded.Status)
	var unrelatedLoaded model.ChannelModelCostRule
	require.NoError(t, model.DB.First(&unrelatedLoaded, unrelatedRule.ID).Error)
	assert.Equal(t, string(types.CostRuleActive), unrelatedLoaded.Status)
	assert.Nil(t, unrelatedLoaded.EffectiveTo)
	var loadedBatch model.ConfigImportBatch
	require.NoError(t, model.DB.First(&loadedBatch, batch.ID).Error)
	assert.Equal(t, string(types.ConfigImportBatchStatusPublished), loadedBatch.Status)
}

func TestPublishConfigImportBatchRejectsChangedCostRuleMissingFromAuthoritativeSnapshot(t *testing.T) {
	prepareConfigImportServiceDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Channel{}, &model.ChannelModelCostRule{}, &model.ConfigImportPublishAudit{}))
	channel := &model.Channel{Type: 1, Name: "supplier", Models: "vendor-video", Key: "key"}
	require.NoError(t, model.DB.Create(channel).Error)
	batch := createConfigImportStageBatch(t, channel.Id, "line-a", "vendor-video")
	obsoleteRule := &model.ChannelModelCostRule{
		ChannelID: channel.Id, BillableUpstreamModel: "obsolete-model", CostVariantKey: "default", Version: 1,
		Status: string(types.CostRuleActive), CostMode: "per_request", SchemaVersion: 1, ConfigJSON: `{}`,
	}
	require.NoError(t, model.DB.Create(obsoleteRule).Error)
	_, err := StageConfigImportBatch(context.Background(), 42, batch.ID)
	require.NoError(t, err)
	require.NoError(t, model.DB.Model(obsoleteRule).Update("config_json", `{"changed":true}`).Error)

	err = PublishConfigImportBatch(context.Background(), batch.ID, 42)
	require.ErrorIs(t, err, ErrConfigImportStale)
	require.NoError(t, model.DB.First(obsoleteRule, obsoleteRule.ID).Error)
	assert.Equal(t, string(types.CostRuleActive), obsoleteRule.Status)
}

func TestPublishConfigImportBatchKeepsCostRuleDisabledByImportActive(t *testing.T) {
	prepareConfigImportServiceDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Channel{}, &model.ChannelModelCostRule{}, &model.ConfigImportPublishAudit{}))
	channel := &model.Channel{Type: 1, Name: "supplier", Models: "vendor-video", Key: "key"}
	require.NoError(t, model.DB.Create(channel).Error)
	activeRule := &model.ChannelModelCostRule{
		ChannelID: channel.Id, BillableUpstreamModel: "vendor-video", CostVariantKey: "default", Version: 1,
		Status: string(types.CostRuleActive), CostMode: "per_request", SchemaVersion: 1, ConfigJSON: `{}`,
	}
	require.NoError(t, model.DB.Create(activeRule).Error)
	InvalidateCostCoverage(0, "", "")
	t.Cleanup(func() { InvalidateCostCoverage(0, "", "") })
	_, err := ActiveCostRule(channel.Id, "vendor-video", "default", false)
	require.NoError(t, err)
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

	var costItem model.ConfigImportItem
	require.NoError(t, model.DB.Where("batch_id = ? AND entity_type = ?", batch.ID, "cost_rule_drafts").First(&costItem).Error)
	var draft map[string]any
	require.NoError(t, common.UnmarshalJsonStr(costItem.CanonicalJSON, &draft))
	draft["enabled"] = false
	encoded, err := common.Marshal(draft)
	require.NoError(t, err)
	require.NoError(t, model.DB.Model(&costItem).Update("canonical_json", string(encoded)).Error)

	staged, err := StageConfigImportBatch(context.Background(), 42, batch.ID)
	require.NoError(t, err)
	assert.Equal(t, types.ConfigImportBatchStatusReady, staged.Status)
	require.NoError(t, PublishConfigImportBatch(context.Background(), batch.ID, 42))

	require.NoError(t, model.DB.First(activeRule, activeRule.ID).Error)
	assert.Equal(t, string(types.CostRuleActive), activeRule.Status)
	assert.Nil(t, activeRule.EffectiveTo)
	var activeCount int64
	require.NoError(t, model.DB.Model(&model.ChannelModelCostRule{}).Where(
		"channel_id = ? AND billable_upstream_model = ? AND cost_variant_key = ? AND status = ?",
		channel.Id, "vendor-video", "default", types.CostRuleActive,
	).Count(&activeCount).Error)
	assert.Equal(t, int64(1), activeCount)
	_, err = ActiveCostRule(channel.Id, "vendor-video", "default", false)
	require.NoError(t, err)
}

func TestPublishConfigImportBatchDoesNotRetireDisabledCostRuleOnSkippedLine(t *testing.T) {
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

	var costItem model.ConfigImportItem
	require.NoError(t, model.DB.Where("batch_id = ? AND entity_type = ?", batch.ID, "cost_rule_drafts").First(&costItem).Error)
	var draft map[string]any
	require.NoError(t, common.UnmarshalJsonStr(costItem.CanonicalJSON, &draft))
	draft["enabled"] = false
	encoded, err := common.Marshal(draft)
	require.NoError(t, err)
	require.NoError(t, model.DB.Model(&costItem).Update("canonical_json", string(encoded)).Error)
	_, err = UpdateConfigImportBindings(context.Background(), 42, batch.ID, []dto.ConfigImportBindingInput{{
		LineRef: "line-a", Action: types.ConfigImportBindingActionSkip,
	}})
	require.NoError(t, err)

	staged, err := StageConfigImportBatch(context.Background(), 42, batch.ID)
	require.NoError(t, err)
	assert.Equal(t, types.ConfigImportBatchStatusReady, staged.Status)
	require.NoError(t, PublishConfigImportBatch(context.Background(), batch.ID, 42))

	require.NoError(t, model.DB.First(activeRule, activeRule.ID).Error)
	assert.Equal(t, string(types.CostRuleActive), activeRule.Status)
}

func TestPublishConfigImportBatchRejectsChangedDisabledCostRule(t *testing.T) {
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

	var costItem model.ConfigImportItem
	require.NoError(t, model.DB.Where("batch_id = ? AND entity_type = ?", batch.ID, "cost_rule_drafts").First(&costItem).Error)
	var draft map[string]any
	require.NoError(t, common.UnmarshalJsonStr(costItem.CanonicalJSON, &draft))
	draft["enabled"] = false
	encoded, err := common.Marshal(draft)
	require.NoError(t, err)
	require.NoError(t, model.DB.Model(&costItem).Update("canonical_json", string(encoded)).Error)
	_, err = StageConfigImportBatch(context.Background(), 42, batch.ID)
	require.NoError(t, err)
	require.NoError(t, model.DB.Model(activeRule).Update("config_json", `{"changed":true}`).Error)

	err = PublishConfigImportBatch(context.Background(), batch.ID, 42)
	require.ErrorIs(t, err, ErrConfigImportStale)
	require.NoError(t, model.DB.First(activeRule, activeRule.ID).Error)
	assert.Equal(t, string(types.CostRuleActive), activeRule.Status)
	var savedBatch model.ConfigImportBatch
	require.NoError(t, model.DB.First(&savedBatch, batch.ID).Error)
	assert.Equal(t, string(types.ConfigImportBatchStatusStaged), savedBatch.Status)
	assert.Equal(t, "STALE_BASE_VERSION", savedBatch.FailureCode)
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

func TestPublishConfigImportBatchKeepsDraftAndAudits(t *testing.T) {
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
	assert.Equal(t, string(types.CostRuleDraft), rule.Status)
	var loadedBatch model.ConfigImportBatch
	require.NoError(t, model.DB.First(&loadedBatch, batch.ID).Error)
	assert.Equal(t, string(types.ConfigImportBatchStatusPublished), loadedBatch.Status)
	var audit model.ConfigImportPublishAudit
	require.NoError(t, model.DB.Where("batch_id = ?", batch.ID).First(&audit).Error)
	assert.Equal(t, "published", audit.Outcome)
	assert.NotEmpty(t, audit.BeforeSHA256)
	_ = common.GetTimestamp()
}
