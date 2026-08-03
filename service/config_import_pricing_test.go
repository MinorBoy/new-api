package service

import (
	"context"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelrouting"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigImportStagePersistsCostDraftWithoutActivatingConfiguration(t *testing.T) {
	prepareConfigImportServiceDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Channel{}, &model.ChannelModelCostRule{}))

	channel := &model.Channel{Type: 1, Name: "supplier", Models: "vendor-video", Key: "configured-outside-import"}
	require.NoError(t, model.DB.Create(channel).Error)
	batch := createConfigImportStageBatch(t, channel.Id, "line-a", "vendor-video")

	detail, err := StageConfigImportBatch(context.Background(), 42, batch.ID)
	require.NoError(t, err)
	assert.Contains(t, []types.ConfigImportBatchStatus{types.ConfigImportBatchStatusStaged, types.ConfigImportBatchStatusReady}, detail.Status)

	var drafts []model.ChannelModelCostRule
	require.NoError(t, model.DB.Where("source = ?", "config_import").Find(&drafts).Error)
	require.Len(t, drafts, 1)
	assert.Equal(t, string(types.CostRuleDraft), drafts[0].Status)
	assert.Equal(t, channel.Id, drafts[0].ChannelID)
	assert.NotContains(t, drafts[0].ConfigJSON, "derived_preview")

	var active int64
	require.NoError(t, model.DB.Model(&model.ChannelModelCostRule{}).Where("status = ?", types.CostRuleActive).Count(&active).Error)
	assert.Zero(t, active)
	assert.Empty(t, detail.Issues)
}

func TestConfigImportSeedanceSaleStagesExplicitScenarioPrice(t *testing.T) {
	var proposal types.ConfigImportSaleProposal
	require.NoError(t, common.UnmarshalJsonStr(`{
		"business_id":"sale-mini-720-with-video",
		"source_ref":"SRC-OFFICIAL-SEEDANCE-2-0-MINI",
		"sheet":"官方售价",
		"row":7,
		"model_sku_ref":"sku-mini-720",
		"scenario":"with_video",
		"resolution":"720p",
		"currency":"USD",
		"billing_mode":"per_duration",
		"duration_price":{
			"price":"0.08",
			"unit":"second",
			"rounding_step_seconds":1,
			"minimum_duration_seconds":4,
			"pricing_version":"official-sheet-v1",
			"source":"SRC-OFFICIAL-SEEDANCE-2-0-MINI!7"
		}
	}`, &proposal))

	patches, err := configImportSaleOptionPatches(proposal, "doubao-seedance-2-0-mini-260615")

	require.NoError(t, err)
	encoded, marshalErr := common.Marshal(patches)
	require.NoError(t, marshalErr)
	assert.Contains(t, string(encoded), `"720p:with_video"`)
	assert.Contains(t, string(encoded), `"output_price":0.08`)
	assert.NotContains(t, string(encoded), `"input_video_price"`)
	assert.Contains(t, string(encoded), `"pricing_version":"official-sheet-v1"`)
}

func TestConfigImportSeedanceSaleRejectsMissingOfficialPriceAuditFields(t *testing.T) {
	base := configImportOfficialSeedanceSaleProposalForTest()
	tests := []struct {
		name   string
		mutate func(*types.DurationPriceProposal)
	}{
		{name: "pricing version", mutate: func(price *types.DurationPriceProposal) { price.PricingVersion = "" }},
		{name: "source", mutate: func(price *types.DurationPriceProposal) { price.Source = "" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			proposal := base
			price := *base.DurationPrice
			proposal.DurationPrice = &price
			test.mutate(proposal.DurationPrice)

			_, err := configImportSaleOptionPatches(proposal, modelrouting.Seedance20)

			var schemaErr *ConfigImportSchemaError
			require.ErrorAs(t, err, &schemaErr)
			assert.Equal(t, "PRICING_SEEDANCE_AUDIT_REQUIRED", schemaErr.Code)
		})
	}
}

func TestConfigImportSeedanceSaleRejectsNonOfficialUSDSource(t *testing.T) {
	tests := []struct {
		name       string
		mutate     func(*types.ConfigImportSaleProposal)
		expectCode string
	}{
		{name: "currency", mutate: func(proposal *types.ConfigImportSaleProposal) { proposal.Currency = "CNY" }, expectCode: "PRICING_SEEDANCE_OFFICIAL_CURRENCY"},
		{name: "pricing version", mutate: func(proposal *types.ConfigImportSaleProposal) { proposal.DurationPrice.PricingVersion = "custom-v1" }, expectCode: "PRICING_SEEDANCE_OFFICIAL_SOURCE"},
		{name: "sheet", mutate: func(proposal *types.ConfigImportSaleProposal) { proposal.Sheet = "渠道成本" }, expectCode: "PRICING_SEEDANCE_OFFICIAL_SOURCE"},
		{name: "source reference", mutate: func(proposal *types.ConfigImportSaleProposal) { proposal.SourceRef = "SRC-CH-1" }, expectCode: "PRICING_SEEDANCE_OFFICIAL_SOURCE"},
		{name: "source row", mutate: func(proposal *types.ConfigImportSaleProposal) {
			proposal.DurationPrice.Source = "SRC-OFFICIAL-SEEDANCE-2-0!99"
		}, expectCode: "PRICING_SEEDANCE_OFFICIAL_SOURCE"},
		{name: "missing row", mutate: func(proposal *types.ConfigImportSaleProposal) { proposal.Row = nil }, expectCode: "PRICING_SEEDANCE_OFFICIAL_SOURCE"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			proposal := configImportOfficialSeedanceSaleProposalForTest()
			test.mutate(&proposal)

			_, err := configImportSaleOptionPatches(proposal, modelrouting.Seedance20)

			var schemaErr *ConfigImportSchemaError
			require.ErrorAs(t, err, &schemaErr)
			assert.Equal(t, test.expectCode, schemaErr.Code)
		})
	}
}

func configImportOfficialSeedanceSaleProposalForTest() types.ConfigImportSaleProposal {
	row := 5
	return types.ConfigImportSaleProposal{
		ConfigImportAuthoritativeEntity: types.ConfigImportAuthoritativeEntity{
			BusinessID: "sale-seedance-official",
			SourceRef:  "SRC-OFFICIAL-SEEDANCE-2-0",
			Sheet:      "官方售价",
			Row:        &row,
		},
		ModelSKURef: "sku-seedance",
		Scenario:    types.DurationScenarioNoVideo,
		Resolution:  "720p",
		Currency:    "USD",
		BillingMode: billing_setting.BillingModePerDuration,
		DurationPrice: &types.DurationPriceProposal{
			Price:                  "0.08",
			Unit:                   types.DurationUnitSecond,
			RoundingStepSeconds:    1,
			MinimumDurationSeconds: 0,
			PricingVersion:         "official-sheet-v1",
			Source:                 "SRC-OFFICIAL-SEEDANCE-2-0!5",
		},
	}
}

func TestConfigImportStageDoesNotMaterializeKeepExistingCostResolution(t *testing.T) {
	prepareConfigImportServiceDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Channel{}, &model.ChannelModelCostRule{}))
	channel := &model.Channel{Type: 1, Name: "supplier", Models: "vendor-video", Key: "configured-outside-import"}
	require.NoError(t, model.DB.Create(channel).Error)
	batch := createConfigImportStageBatch(t, channel.Id, "line-a", "vendor-video")

	_, err := UpdateConfigImportResolutions(context.Background(), 42, batch.ID, []dto.ConfigImportResolutionInput{{
		ItemBusinessID: "cost-a", Action: types.ConfigImportResolutionActionKeepExisting,
	}})
	require.NoError(t, err)

	_, err = StageConfigImportBatch(context.Background(), 42, batch.ID)
	require.NoError(t, err)

	var count int64
	require.NoError(t, model.DB.Model(&model.ChannelModelCostRule{}).Where("source = ?", "config_import").Count(&count).Error)
	assert.Zero(t, count)
	var item model.ConfigImportItem
	require.NoError(t, model.DB.Where("batch_id = ? AND business_id = ?", batch.ID, "cost-a").First(&item).Error)
	assert.Equal(t, string(types.ConfigImportItemStateUnchanged), item.State)
}

func TestConfigImportStageRollsBackCostDraftsWhenProposalStagingFails(t *testing.T) {
	prepareConfigImportServiceDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Channel{}, &model.ChannelModelCostRule{}))
	channel := &model.Channel{Type: 1, Name: "supplier", Models: "vendor-video", Key: "key"}
	require.NoError(t, model.DB.Create(channel).Error)
	batch := createConfigImportStageBatch(t, channel.Id, "line-a", "vendor-video")
	require.NoError(t, model.DB.Model(&model.ConfigImportItem{}).
		Where("batch_id = ? AND entity_type = ?", batch.ID, "sale_proposals").
		Update("canonical_json", "{").Error)

	_, err := StageConfigImportBatch(context.Background(), 42, batch.ID)
	require.Error(t, err)
	var draftCount int64
	require.NoError(t, model.DB.Model(&model.ChannelModelCostRule{}).Where("source = ?", "config_import").Count(&draftCount).Error)
	assert.Zero(t, draftCount)
}

func TestConfigImportStageMergesIdenticalNoVideoAndWithVideoCostDrafts(t *testing.T) {
	prepareConfigImportServiceDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Channel{}, &model.ChannelModelCostRule{}))
	channel := &model.Channel{Type: 1, Name: "supplier", Models: "vendor-video", Key: "key"}
	require.NoError(t, model.DB.Create(channel).Error)
	batch := createConfigImportStageBatch(t, channel.Id, "line-a", "vendor-video")

	var original model.ConfigImportItem
	require.NoError(t, model.DB.Where("batch_id = ? AND entity_type = ?", batch.ID, "cost_rule_drafts").First(&original).Error)
	var draft types.ConfigImportCostRuleDraft
	require.NoError(t, common.UnmarshalJsonStr(original.CanonicalJSON, &draft))
	draft.Scenario = "no_video"
	encoded, err := common.Marshal(draft)
	require.NoError(t, err)
	require.NoError(t, model.DB.Model(&model.ConfigImportItem{}).Where("id = ?", original.ID).Update("canonical_json", string(encoded)).Error)

	draft.BusinessID = "cost-with-video"
	draft.Scenario = "with_video"
	encoded, err = common.Marshal(draft)
	require.NoError(t, err)
	require.NoError(t, model.DB.Create(&model.ConfigImportItem{
		BatchID: batch.ID, EntityType: "cost_rule_drafts", BusinessID: draft.BusinessID, CanonicalJSON: string(encoded), State: string(types.ConfigImportItemStateNew),
	}).Error)

	_, err = StageConfigImportBatch(context.Background(), 42, batch.ID)
	require.NoError(t, err)
	var rules []model.ChannelModelCostRule
	require.NoError(t, model.DB.Where("source = ?", "config_import").Find(&rules).Error)
	require.Len(t, rules, 1)

	var stagedItems []model.ConfigImportItem
	require.NoError(t, model.DB.Where("batch_id = ? AND entity_type = ?", batch.ID, "cost_rule_drafts").Order("business_id ASC").Find(&stagedItems).Error)
	require.Len(t, stagedItems, 2)
	assert.Equal(t, stagedItems[0].MaterializedID, stagedItems[1].MaterializedID)
	assert.Contains(t, []string{string(types.ConfigImportItemStateChanged), string(types.ConfigImportItemStateUnchanged)}, stagedItems[0].State)
	assert.Contains(t, []string{string(types.ConfigImportItemStateChanged), string(types.ConfigImportItemStateUnchanged)}, stagedItems[1].State)
}

func TestConfigImportStageMergesIdenticalCostDraftsWithSameScenario(t *testing.T) {
	prepareConfigImportServiceDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Channel{}, &model.ChannelModelCostRule{}))
	channel := &model.Channel{Type: 1, Name: "supplier", Models: "vendor-video", Key: "key"}
	require.NoError(t, model.DB.Create(channel).Error)
	batch := createConfigImportStageBatch(t, channel.Id, "line-a", "vendor-video")

	var original model.ConfigImportItem
	require.NoError(t, model.DB.Where("batch_id = ? AND entity_type = ?", batch.ID, "cost_rule_drafts").First(&original).Error)
	var draft types.ConfigImportCostRuleDraft
	require.NoError(t, common.UnmarshalJsonStr(original.CanonicalJSON, &draft))
	draft.Scenario = "no_video"
	encoded, err := common.Marshal(draft)
	require.NoError(t, err)
	require.NoError(t, model.DB.Model(&model.ConfigImportItem{}).Where("id = ?", original.ID).Update("canonical_json", string(encoded)).Error)

	draft.BusinessID = "cost-no-video-duplicate"
	encoded, err = common.Marshal(draft)
	require.NoError(t, err)
	require.NoError(t, model.DB.Create(&model.ConfigImportItem{
		BatchID: batch.ID, EntityType: "cost_rule_drafts", BusinessID: draft.BusinessID, CanonicalJSON: string(encoded), State: string(types.ConfigImportItemStateNew),
	}).Error)

	detail, err := StageConfigImportBatch(context.Background(), 42, batch.ID)
	require.NoError(t, err)
	for _, issue := range detail.Issues {
		assert.NotEqual(t, "COST_VARIANT_AMBIGUOUS", issue.Code)
	}

	var rules []model.ChannelModelCostRule
	require.NoError(t, model.DB.Where("source = ?", "config_import").Find(&rules).Error)
	require.Len(t, rules, 1)

	var stagedItems []model.ConfigImportItem
	require.NoError(t, model.DB.Where("batch_id = ? AND entity_type = ?", batch.ID, "cost_rule_drafts").Order("business_id ASC").Find(&stagedItems).Error)
	require.Len(t, stagedItems, 2)
	assert.Equal(t, stagedItems[0].MaterializedID, stagedItems[1].MaterializedID)
}

func TestConfigImportStageRejectsDifferentCostDraftsWithSameVariantKey(t *testing.T) {
	prepareConfigImportServiceDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Channel{}, &model.ChannelModelCostRule{}))
	channel := &model.Channel{Type: 1, Name: "supplier", Models: "vendor-video", Key: "key"}
	require.NoError(t, model.DB.Create(channel).Error)
	batch := createConfigImportStageBatch(t, channel.Id, "line-a", "vendor-video")

	var original model.ConfigImportItem
	require.NoError(t, model.DB.Where("batch_id = ? AND entity_type = ?", batch.ID, "cost_rule_drafts").First(&original).Error)
	var draft types.ConfigImportCostRuleDraft
	require.NoError(t, common.UnmarshalJsonStr(original.CanonicalJSON, &draft))
	draft.BusinessID = "cost-different-price"
	draft.UnitPrice = stringPointer("0.75")
	encoded, err := common.Marshal(draft)
	require.NoError(t, err)
	require.NoError(t, model.DB.Create(&model.ConfigImportItem{
		BatchID: batch.ID, EntityType: "cost_rule_drafts", BusinessID: draft.BusinessID, CanonicalJSON: string(encoded), State: string(types.ConfigImportItemStateNew),
	}).Error)

	detail, err := StageConfigImportBatch(context.Background(), 42, batch.ID)
	require.NoError(t, err)
	foundAmbiguity := false
	for _, issue := range detail.Issues {
		if issue.Code != "COST_VARIANT_AMBIGUOUS" || issue.BusinessID != draft.BusinessID {
			continue
		}
		foundAmbiguity = true
		assert.Equal(t, types.ConfigImportIssueSeverityWarning, issue.Severity)
		assert.Equal(t, "multiple cost contracts share one channel/model/variant key", issue.Message)
	}
	assert.True(t, foundAmbiguity)

	var rules []model.ChannelModelCostRule
	require.NoError(t, model.DB.Where("source = ?", "config_import").Find(&rules).Error)
	require.Len(t, rules, 1)

	var conflict model.ConfigImportItem
	require.NoError(t, model.DB.Where("batch_id = ? AND business_id = ?", batch.ID, draft.BusinessID).First(&conflict).Error)
	assert.Equal(t, string(types.ConfigImportItemStateConflict), conflict.State)
	assert.Nil(t, conflict.MaterializedID)
}

func TestConfigImportPricingRecomputesDecimalsAndBlocksNegativeMargin(t *testing.T) {
	proposal := types.ConfigImportSaleProposal{
		ConfigImportAuthoritativeEntity: types.ConfigImportAuthoritativeEntity{BusinessID: "sale-a"},
		ModelSKURef:                     "sku-a", Currency: "USD", UnitPrice: stringPointer("1.2300"), MarginRatio: stringPointer("0.1"),
	}
	got, issues, err := recomputeConfigImportSaleProposal(proposal, "1.5")
	require.NoError(t, err)
	assert.Equal(t, "1.23", *got.UnitPrice)
	require.Len(t, issues, 1)
	assert.Equal(t, "PRICING_NEGATIVE_MARGIN", issues[0].Code)
	assert.Equal(t, string(types.ConfigImportIssueSeverityWarning), issues[0].Severity)
}

func TestConfigImportCostNormalizationMismatchComparesDecimalValues(t *testing.T) {
	provided := "1.00"
	serverNormalized := "1"
	draft := types.ConfigImportCostRuleDraft{NormalizedUSDUnitPrice: &provided}
	normalized := types.CostRuleConfigV1{NormalizedUSDPrices: types.CostRulePricesV1{UnitPrice: &serverNormalized}}

	assert.False(t, configImportCostNormalizationMismatch(draft, normalized))
	provided = "1.01"
	assert.True(t, configImportCostNormalizationMismatch(draft, normalized))
}

func TestConfigImportPricingStagesTokenPricesAsVersionedExpression(t *testing.T) {
	proposal := types.ConfigImportSaleProposal{
		ConfigImportAuthoritativeEntity: types.ConfigImportAuthoritativeEntity{BusinessID: "sale-token"},
		ModelSKURef:                     "sku-a",
		InputPerMillion:                 stringPointer("1.200"),
		OutputPerMillion:                stringPointer("3.400"),
	}

	recomputed, _, err := recomputeConfigImportSaleProposal(proposal, "0")
	require.NoError(t, err)
	assert.Equal(t, `v1:tier("base", p * 1.2 + c * 3.4)`, recomputed.BillingExpr)
	assert.Equal(t, billing_setting.BillingModeTieredExpr, recomputed.BillingMode)
	patches, err := configImportSaleOptionPatches(recomputed, "canonical-model")
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"canonical-model": recomputed.BillingExpr}, patches["billing_setting.billing_expr"])
}

func TestConfigImportSeedanceSaleStagesExplicitDurationPriceForMiniAliases(t *testing.T) {
	proposal := configImportOfficialSeedanceSaleProposalForTest()
	proposal.BusinessID = "sale-seedance-mini"
	proposal.ModelSKURef = "sku-mini"
	proposal.DurationPrice.Price = "0.31"

	recomputed, _, err := recomputeConfigImportSaleProposal(proposal, "0")
	require.NoError(t, err)
	patches, err := configImportSaleOptionPatches(recomputed, modelrouting.Seedance20Mini)
	require.NoError(t, err)

	miniAliases := []string{modelrouting.Seedance20Mini, "doubao-seedance-2-0-mini-260128"}
	assert.Equal(t, map[string]string{
		miniAliases[0]: billing_setting.BillingModePerDuration,
		miniAliases[1]: billing_setting.BillingModePerDuration,
	}, patches["billing_setting.billing_mode"])
	prices, ok := patches["billing_setting.duration_price"].(map[string]types.DurationPrice)
	require.True(t, ok)
	for _, modelName := range miniAliases {
		price, found := prices[modelName]
		require.True(t, found)
		scenarioPrice, found := price.Scenarios["720p:no_video"]
		require.True(t, found)
		assert.Equal(t, 0.31, scenarioPrice.OutputPrice)
		assert.Equal(t, types.DurationUnitSecond, scenarioPrice.Unit)
		assert.Equal(t, 1, scenarioPrice.RoundingStepSeconds)
	}
	assert.Equal(t, map[string]string{
		miniAliases[0]: "",
		miniAliases[1]: "",
	}, patches["billing_setting.billing_expr"])
	for _, key := range []string{"ModelPrice", "ModelRatio", "CompletionRatio"} {
		cleanup, ok := patches[key].(map[string]any)
		require.True(t, ok, key)
		assert.Equal(t, map[string]any{
			miniAliases[0]: nil,
			miniAliases[1]: nil,
		}, cleanup, key)
	}
}

func TestConfigImportSeedanceSaleRejectsMissingExplicitScenario(t *testing.T) {
	for _, proposal := range []types.ConfigImportSaleProposal{
		{
			ConfigImportAuthoritativeEntity: types.ConfigImportAuthoritativeEntity{BusinessID: "sale-seedance-duration"},
			ModelSKURef:                     "sku-mini",
			DurationPrice:                   &types.DurationPriceProposal{Price: "99", Unit: types.DurationUnitSecond, RoundingStepSeconds: 1},
		},
		{
			ConfigImportAuthoritativeEntity: types.ConfigImportAuthoritativeEntity{BusinessID: "sale-seedance-expr"},
			ModelSKURef:                     "sku-mini",
			BillingExpr:                     `v1:tier("base", c * 99)`,
		},
	} {
		_, err := configImportSaleOptionPatches(proposal, modelrouting.Seedance20Mini)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "requires resolution, scenario")
	}
}

func TestConfigImportSaleRecomputeRemovesLegacyEnabledFlag(t *testing.T) {
	enabled := false
	proposal := types.ConfigImportSaleProposal{
		ConfigImportAuthoritativeEntity: types.ConfigImportAuthoritativeEntity{BusinessID: "sale-legacy-enabled"},
		ModelSKURef:                     "sku-a",
		UnitPrice:                       stringPointer("1"),
		Enabled:                         &enabled,
	}
	recomputed, _, err := recomputeConfigImportSaleProposal(proposal, "0")
	require.NoError(t, err)
	assert.Nil(t, recomputed.Enabled)
}

func TestConfigImportPricingRejectsConflictingPricingModes(t *testing.T) {
	proposal := types.ConfigImportSaleProposal{
		ConfigImportAuthoritativeEntity: types.ConfigImportAuthoritativeEntity{BusinessID: "sale-conflict"},
		ModelSKURef:                     "sku-a",
		BillingExpr:                     `v1:tier("base", p * 1)`,
		DurationPrice:                   &types.DurationPriceProposal{Price: "1", Unit: types.DurationUnitSecond, RoundingStepSeconds: 1},
	}

	_, _, err := recomputeConfigImportSaleProposal(proposal, "0")
	requireCode(t, err, "PRICING_MODE_CONFLICT")
}

func TestConfigImportStageRejectsInvalidDurationUnit(t *testing.T) {
	prepareConfigImportServiceDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Channel{}, &model.ChannelModelCostRule{}))
	channel := &model.Channel{Type: 1, Name: "supplier", Models: "vendor-video", Key: "key"}
	require.NoError(t, model.DB.Create(channel).Error)
	batch := createConfigImportStageBatch(t, channel.Id, "line-a", "vendor-video")

	var saleItem model.ConfigImportItem
	require.NoError(t, model.DB.Where("batch_id = ? AND entity_type = ?", batch.ID, "sale_proposals").First(&saleItem).Error)
	var proposal types.ConfigImportSaleProposal
	require.NoError(t, common.UnmarshalJsonStr(saleItem.CanonicalJSON, &proposal))
	proposal.UnitPrice = nil
	proposal.DurationPrice = &types.DurationPriceProposal{Price: "1", Unit: "hour", RoundingStepSeconds: 1, MinimumDurationSeconds: 0}
	encoded, err := common.Marshal(proposal)
	require.NoError(t, err)
	require.NoError(t, model.DB.Model(&model.ConfigImportItem{}).Where("id = ?", saleItem.ID).Update("canonical_json", string(encoded)).Error)

	_, err = StageConfigImportBatch(context.Background(), 42, batch.ID)
	require.NoError(t, err)
	require.NoError(t, model.DB.First(&saleItem, saleItem.ID).Error)
	assert.Equal(t, string(types.ConfigImportItemStateConflict), saleItem.State)
	assert.Contains(t, saleItem.ConflictReason, "PRICING_DURATION_INVALID")
}

func TestConfigImportStageBlocksUnrepresentableGroupScope(t *testing.T) {
	prepareConfigImportServiceDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Channel{}, &model.ChannelModelCostRule{}, &model.Ability{}))
	channel := &model.Channel{Type: 1, Name: "supplier", Models: "vendor-video", Key: "key"}
	require.NoError(t, model.DB.Create(channel).Error)
	require.NoError(t, model.DB.Create(&model.Ability{ChannelId: channel.Id, Group: "vip", Model: "sku-a", Enabled: true}).Error)
	batch := createConfigImportStageBatch(t, channel.Id, "line-a", "vendor-video")

	var saleItem model.ConfigImportItem
	require.NoError(t, model.DB.Where("batch_id = ? AND entity_type = ?", batch.ID, "sale_proposals").First(&saleItem).Error)
	var proposal types.ConfigImportSaleProposal
	require.NoError(t, common.UnmarshalJsonStr(saleItem.CanonicalJSON, &proposal))
	proposal.SelectedGroups = []string{"default"}
	proposal.GroupPrices = map[string]string{"default": "1"}
	encoded, err := common.Marshal(proposal)
	require.NoError(t, err)
	require.NoError(t, model.DB.Model(&model.ConfigImportItem{}).Where("id = ?", saleItem.ID).Update("canonical_json", string(encoded)).Error)

	detail, err := StageConfigImportBatch(context.Background(), 42, batch.ID)
	require.NoError(t, err)
	assert.Equal(t, types.ConfigImportBatchStatusStaged, detail.Status)
	var groupScopeIssue *dto.ConfigImportIssueDetail
	for index := range detail.Issues {
		if detail.Issues[index].Code == "PRICING_GROUP_SCOPE_UNREPRESENTABLE" {
			groupScopeIssue = &detail.Issues[index]
			break
		}
	}
	require.NotNil(t, groupScopeIssue)
}

func TestConfigImportPricingReviewPersistsSelectedGroupsAndRestagingClearsScopeWarning(t *testing.T) {
	prepareConfigImportServiceDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Channel{}, &model.ChannelModelCostRule{}, &model.Ability{}))
	channel := &model.Channel{Type: 1, Name: "supplier", Models: "vendor-video", Key: "key"}
	require.NoError(t, model.DB.Create(channel).Error)
	priority := int64(0)
	weight := uint(1)
	require.NoError(t, model.DB.Create(&model.Ability{ChannelId: channel.Id, Group: "分组A", Model: "sku-a", Enabled: true, Priority: &priority, Weight: weight}).Error)
	batch := createConfigImportStageBatch(t, channel.Id, "line-a", "vendor-video")

	detail, err := StageConfigImportBatch(context.Background(), 42, batch.ID)
	require.NoError(t, err)
	require.Equal(t, types.ConfigImportBatchStatusStaged, detail.Status)

	updated, err := UpdateConfigImportPricingReview(context.Background(), 42, batch.ID, []string{"default", "分组A"})
	require.NoError(t, err)
	require.Equal(t, types.ConfigImportBatchStatusStaged, updated.Status)

	detail, err = StageConfigImportBatch(context.Background(), 42, batch.ID)
	require.NoError(t, err)
	require.Equal(t, types.ConfigImportBatchStatusReady, detail.Status)
	for _, issue := range detail.Issues {
		if issue.Code == "PRICING_GROUP_SCOPE_UNREPRESENTABLE" {
			assert.NotEqual(t, "open", issue.ResolutionStatus)
		}
	}

	var item model.ConfigImportItem
	require.NoError(t, model.DB.Where("batch_id = ? AND entity_type = ?", batch.ID, "sale_proposals").First(&item).Error)
	var proposal types.ConfigImportSaleProposal
	require.NoError(t, common.UnmarshalJsonStr(item.CanonicalJSON, &proposal))
	assert.Equal(t, []string{"default", "分组A"}, proposal.SelectedGroups)
}

func TestConfigImportStagePersistsNegativeMarginWarningGate(t *testing.T) {
	prepareConfigImportServiceDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Channel{}, &model.ChannelModelCostRule{}))
	channel := &model.Channel{Type: 1, Name: "supplier", Models: "vendor-video", Key: "key"}
	require.NoError(t, model.DB.Create(channel).Error)
	batch := createConfigImportStageBatch(t, channel.Id, "line-a", "vendor-video")
	var costItem model.ConfigImportItem
	require.NoError(t, model.DB.Where("batch_id = ? AND entity_type = ?", batch.ID, "cost_rule_drafts").First(&costItem).Error)
	updatedJSON := strings.Replace(costItem.CanonicalJSON, `"0.5"`, `"2"`, 1)
	require.NoError(t, model.DB.Model(&model.ConfigImportItem{}).Where("id = ?", costItem.ID).Update("canonical_json", updatedJSON).Error)

	detail, err := StageConfigImportBatch(context.Background(), 42, batch.ID)
	require.NoError(t, err)
	assert.Equal(t, types.ConfigImportBatchStatusStaged, detail.Status)
	var issue model.ConfigImportIssue
	require.NoError(t, model.DB.Where("batch_id = ? AND code = ?", batch.ID, "PRICING_NEGATIVE_MARGIN").First(&issue).Error)
	assert.Equal(t, string(types.ConfigImportIssueSeverityWarning), issue.Severity)
	assert.Equal(t, "open", issue.ResolutionStatus)
}

func TestConfigImportStageExcludesDisabledCostRuleDraft(t *testing.T) {
	prepareConfigImportServiceDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Channel{}, &model.ChannelModelCostRule{}))
	channel := &model.Channel{Type: 1, Name: "supplier", Models: "vendor-video", Key: "key"}
	require.NoError(t, model.DB.Create(channel).Error)
	batch := createConfigImportStageBatch(t, channel.Id, "line-a", "vendor-video")

	var item model.ConfigImportItem
	require.NoError(t, model.DB.Where("batch_id = ? AND entity_type = ?", batch.ID, "cost_rule_drafts").First(&item).Error)
	var payload map[string]any
	require.NoError(t, common.UnmarshalJsonStr(item.CanonicalJSON, &payload))
	payload["enabled"] = false
	encoded, err := common.Marshal(payload)
	require.NoError(t, err)
	require.NoError(t, model.DB.Model(&item).Update("canonical_json", string(encoded)).Error)

	_, err = StageConfigImportBatch(context.Background(), 42, batch.ID)
	require.NoError(t, err)
	require.NoError(t, model.DB.First(&item, item.ID).Error)
	assert.Equal(t, string(types.ConfigImportItemStateExcluded), item.State)
	assert.Equal(t, "disabled by import document", item.ExclusionReason)

	var count int64
	require.NoError(t, model.DB.Model(&model.ChannelModelCostRule{}).Where("channel_id = ?", channel.Id).Count(&count).Error)
	assert.Zero(t, count)
}

func TestConfigImportStageResolutionAcceptsStructuredActions(t *testing.T) {
	prepareConfigImportServiceDB(t)
	batch := createConfigImportStageBatch(t, 17, "line-a", "vendor-video")
	for _, businessID := range []string{"variant-a", "variant-b", "variant-c"} {
		require.NoError(t, model.DB.Create(&model.ConfigImportItem{BatchID: batch.ID, EntityType: "unresolved_variants", BusinessID: businessID, CanonicalJSON: "{}", State: string(types.ConfigImportItemStateConflict)}).Error)
	}

	_, err := UpdateConfigImportResolutions(context.Background(), 42, batch.ID, []dto.ConfigImportResolutionInput{
		{ItemBusinessID: "variant-c", Action: "exclude", Reason: "Duplicate supplier quote"},
		{ItemBusinessID: "variant-b", Action: "bind_variant", CostVariantKey: "default", RouteTargetRef: "target-a"},
		{ItemBusinessID: "variant-a", Action: "split_line", LineRef: "line-a"},
	})
	require.NoError(t, err)
	var resolutions []model.ConfigImportResolution
	require.NoError(t, model.DB.Where("batch_id = ?", batch.ID).Order("id ASC").Find(&resolutions).Error)
	require.Equal(t, []string{"variant-a", "variant-b", "variant-c"}, []string{resolutions[0].ItemBusinessID, resolutions[1].ItemBusinessID, resolutions[2].ItemBusinessID})
	var splitDecision, bindDecision, excludeDecision map[string]string
	require.NoError(t, common.UnmarshalJsonStr(resolutions[0].DecisionJSON, &splitDecision))
	require.NoError(t, common.UnmarshalJsonStr(resolutions[1].DecisionJSON, &bindDecision))
	require.NoError(t, common.UnmarshalJsonStr(resolutions[2].DecisionJSON, &excludeDecision))
	assert.Equal(t, map[string]string{"action": "split_line", "line_ref": "line-a"}, splitDecision)
	assert.Equal(t, map[string]string{"action": "bind_variant", "cost_variant_key": "default", "route_target_ref": "target-a"}, bindDecision)
	assert.Equal(t, map[string]string{"action": "exclude", "reason": "Duplicate supplier quote"}, excludeDecision)

	var excluded model.ConfigImportItem
	require.NoError(t, model.DB.Where("batch_id = ? AND business_id = ?", batch.ID, "variant-c").First(&excluded).Error)
	assert.Equal(t, "Duplicate supplier quote", excluded.ExclusionReason)
}

func TestConfigImportStageResolutionAppliesLineAndVariantSelections(t *testing.T) {
	prepareConfigImportServiceDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Channel{}, &model.ChannelModelCostRule{}))
	channel := &model.Channel{Type: 1, Name: "supplier", Models: "vendor-video", Key: "key"}
	require.NoError(t, model.DB.Create(channel).Error)
	batch := createConfigImportStageBatch(t, channel.Id, "line-a", "vendor-video")
	lineB := &model.Channel{Type: 1, Name: "supplier-b", Models: "vendor-video", Key: "key-b"}
	require.NoError(t, model.DB.Create(lineB).Error)
	lineBChannelID := lineB.Id
	confirmedAt := int64(2)
	require.NoError(t, model.DB.Create(&model.ConfigImportBinding{
		BatchID: batch.ID, LineRef: "line-b", Action: string(types.ConfigImportBindingActionBind), ChannelID: &lineBChannelID,
		CredentialsConfirmedBy: 42, CredentialsConfirmedAt: &confirmedAt,
	}).Error)
	variant := types.ConfigImportUnresolvedVariant{
		ConfigImportAuthoritativeEntity: types.ConfigImportAuthoritativeEntity{BusinessID: "variant-a"},
		LineRef:                         "line-a", UpstreamModel: "vendor-video", CostVariantKey: "default",
		CostRuleRefs: []string{"cost-a"}, RouteTargetRefs: []string{"target-a"},
	}
	encoded, err := common.Marshal(variant)
	require.NoError(t, err)
	require.NoError(t, model.DB.Create(&model.ConfigImportItem{BatchID: batch.ID, EntityType: "unresolved_variants", BusinessID: variant.BusinessID, CanonicalJSON: string(encoded), State: string(types.ConfigImportItemStateConflict)}).Error)

	_, err = UpdateConfigImportResolutions(context.Background(), 42, batch.ID, []dto.ConfigImportResolutionInput{{
		ItemBusinessID: variant.BusinessID, Action: types.ConfigImportResolutionActionSplitLine, LineRef: "line-b",
	}})
	require.NoError(t, err)
	_, err = StageConfigImportBatch(context.Background(), 42, batch.ID)
	require.NoError(t, err)

	var draftItem model.ConfigImportItem
	require.NoError(t, model.DB.Where("batch_id = ? AND business_id = ?", batch.ID, "cost-a").First(&draftItem).Error)
	var draft types.ConfigImportCostRuleDraft
	require.NoError(t, common.UnmarshalJsonStr(draftItem.CanonicalJSON, &draft))
	assert.Equal(t, "line-b", draft.LineRef)
	var routeItem model.ConfigImportItem
	require.NoError(t, model.DB.Where("batch_id = ? AND business_id = ?", batch.ID, "route-a").First(&routeItem).Error)
	var blueprint types.ConfigImportRouteBlueprint
	require.NoError(t, common.UnmarshalJsonStr(routeItem.CanonicalJSON, &blueprint))
	assert.Equal(t, "line-b", blueprint.Targets[0].LineRef)
}

func TestConfigImportRouteReviewPersistsMergeMode(t *testing.T) {
	prepareConfigImportServiceDB(t)
	batch := createConfigImportStageBatch(t, 17, "line-a", "vendor-video")
	detail, err := UpdateConfigImportRouteReviews(context.Background(), 42, batch.ID, []dto.ConfigImportRouteReviewInput{{
		ItemBusinessID: "route-a", MergeMode: types.ConfigImportRouteMergeModeReplace,
	}})
	require.NoError(t, err)
	require.NotNil(t, detail)
	var item model.ConfigImportItem
	require.NoError(t, model.DB.Where("batch_id = ? AND business_id = ?", batch.ID, "route-a").First(&item).Error)
	var blueprint types.ConfigImportRouteBlueprint
	require.NoError(t, common.UnmarshalJsonStr(item.CanonicalJSON, &blueprint))
	assert.Equal(t, types.ConfigImportRouteMergeModeReplace, blueprint.MergeMode)
	assert.Equal(t, string(types.ConfigImportItemStateChanged), item.State)
}

func TestConfigImportResolutionRejectsMissingStructuredFields(t *testing.T) {
	prepareConfigImportServiceDB(t)
	batch := createConfigImportStageBatch(t, 0, "line-a", "vendor-video")

	_, err := UpdateConfigImportResolutions(context.Background(), 42, batch.ID, []dto.ConfigImportResolutionInput{{
		ItemBusinessID: "cost-a", Action: types.ConfigImportResolutionActionSplitLine,
	}})
	requireCode(t, err, "SCHEMA_RESOLUTION_LINE")

	_, err = UpdateConfigImportResolutions(context.Background(), 42, batch.ID, []dto.ConfigImportResolutionInput{{
		ItemBusinessID: "cost-a", Action: types.ConfigImportResolutionActionBindVariant, CostVariantKey: "default",
	}})
	requireCode(t, err, "SCHEMA_RESOLUTION_ROUTE_TARGET")

	_, err = UpdateConfigImportResolutions(context.Background(), 42, batch.ID, []dto.ConfigImportResolutionInput{{
		ItemBusinessID: "cost-a", Action: types.ConfigImportResolutionActionExclude,
	}})
	requireCode(t, err, "SCHEMA_RESOLUTION_REASON")

	_, err = UpdateConfigImportResolutions(context.Background(), 42, batch.ID, []dto.ConfigImportResolutionInput{{
		ItemBusinessID: "cost-a", Action: types.ConfigImportResolutionActionSplitLine, LineRef: "line-a",
	}})
	requireCode(t, err, "RESOLUTION_LINE_UNBOUND")
}

func TestConfigImportStageExcludesRouteBlueprintWithSkipMode(t *testing.T) {
	prepareConfigImportServiceDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Channel{}, &model.ChannelModelCostRule{}))
	channel := &model.Channel{Type: 1, Name: "supplier", Models: "vendor-video", Key: "key"}
	require.NoError(t, model.DB.Create(channel).Error)
	batch := createConfigImportStageBatch(t, channel.Id, "line-a", "vendor-video")

	var routeItem model.ConfigImportItem
	require.NoError(t, model.DB.Where("batch_id = ? AND entity_type = ?", batch.ID, "route_blueprints").First(&routeItem).Error)
	var blueprint types.ConfigImportRouteBlueprint
	require.NoError(t, common.UnmarshalJsonStr(routeItem.CanonicalJSON, &blueprint))
	blueprint.MergeMode = types.ConfigImportRouteMergeModeSkip
	encoded, err := common.Marshal(blueprint)
	require.NoError(t, err)
	require.NoError(t, model.DB.Model(&model.ConfigImportItem{}).Where("id = ?", routeItem.ID).Update("canonical_json", string(encoded)).Error)

	_, err = StageConfigImportBatch(context.Background(), 42, batch.ID)
	require.NoError(t, err)
	require.NoError(t, model.DB.First(&routeItem, routeItem.ID).Error)
	assert.Equal(t, string(types.ConfigImportItemStateExcluded), routeItem.State)
	assert.Equal(t, "route merge mode skip", routeItem.ExclusionReason)
	assert.NotContains(t, routeItem.CanonicalJSON, "staged_proposal")
}

func TestConfigImportStageDisablesEveryStagedRouteTarget(t *testing.T) {
	prepareConfigImportServiceDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Channel{}, &model.ChannelModelCostRule{}))
	channel := &model.Channel{Type: 1, Name: "supplier", Models: "vendor-video", Key: "key"}
	require.NoError(t, model.DB.Create(channel).Error)
	batch := createConfigImportStageBatch(t, channel.Id, "line-a", "vendor-video")

	var routeItem model.ConfigImportItem
	require.NoError(t, model.DB.Where("batch_id = ? AND entity_type = ?", batch.ID, "route_blueprints").First(&routeItem).Error)
	var blueprint types.ConfigImportRouteBlueprint
	require.NoError(t, common.UnmarshalJsonStr(routeItem.CanonicalJSON, &blueprint))
	blueprint.Targets[0].Enabled = boolPointer(true)
	encoded, err := common.Marshal(blueprint)
	require.NoError(t, err)
	require.NoError(t, model.DB.Model(&model.ConfigImportItem{}).Where("id = ?", routeItem.ID).Update("canonical_json", string(encoded)).Error)

	_, err = StageConfigImportBatch(context.Background(), 42, batch.ID)
	require.NoError(t, err)
	require.NoError(t, model.DB.First(&routeItem, routeItem.ID).Error)
	var staged map[string]any
	require.NoError(t, common.UnmarshalJsonStr(routeItem.CanonicalJSON, &staged))
	proposal := staged["staged_proposal"].(map[string]any)
	targets := proposal["targets"].([]any)
	assert.Equal(t, false, targets[0].(map[string]any)["enabled"])
}

func TestConfigImportV1BaselineHashIsDeterministic(t *testing.T) {
	prepareConfigImportServiceDB(t)
	batch := createConfigImportStageBatch(t, 0, "line-a", "vendor-video")
	first, err := captureConfigImportBaseline(model.DB, batch.ID)
	require.NoError(t, err)
	second, err := captureConfigImportBaseline(model.DB, batch.ID)
	require.NoError(t, err)
	assert.Equal(t, first.Hash, second.Hash)
	assert.NotEmpty(t, first.Hash)
}

func TestConfigImportBaselineTracksFullAffectedChannelState(t *testing.T) {
	prepareConfigImportServiceDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Channel{}, &model.ChannelModelCostRule{}, &model.Option{}, &model.RoutingPolicy{}, &model.RouteTarget{}))
	modelMapping := `{"canonical-video":"vendor-video","unrelated-model":"unrelated-upstream"}`
	channel := &model.Channel{Type: 1, Name: "supplier", Models: "vendor-video", Key: "key", ModelMapping: &modelMapping}
	require.NoError(t, model.DB.Create(channel).Error)
	require.NoError(t, model.DB.Create(&model.ChannelModelCostRule{
		ChannelID: channel.Id, BillableUpstreamModel: "vendor-video", CostVariantKey: "default", Version: 1,
		Status: string(types.CostRuleActive), CostMode: "per_request", SchemaVersion: 1, ConfigJSON: `{}`,
	}).Error)
	require.NoError(t, model.DB.Create(&model.Option{Key: "ModelPrice", Value: `{"canonical-video":1,"unrelated-model":2}`}).Error)
	relevantPolicy := &model.RoutingPolicy{GroupName: "default", Model: "canonical-video"}
	unrelatedPolicy := &model.RoutingPolicy{GroupName: "default", Model: "unrelated-model"}
	require.NoError(t, model.DB.Create(relevantPolicy).Error)
	require.NoError(t, model.DB.Create(unrelatedPolicy).Error)

	batch := createConfigImportStageBatch(t, channel.Id, "line-a", "vendor-video")
	mapping := types.ConfigImportModelMapping{
		ConfigImportAuthoritativeEntity: types.ConfigImportAuthoritativeEntity{BusinessID: "mapping-a"},
		CanonicalModel:                  "canonical-video",
		ClientModel:                     "canonical-video",
		LineRef:                         "line-a",
		UpstreamModel:                   "vendor-video",
		SKURef:                          "sku-a",
	}
	mappingJSON, err := common.Marshal(mapping)
	require.NoError(t, err)
	require.NoError(t, model.DB.Create(&model.ConfigImportItem{
		BatchID: batch.ID, EntityType: "model_mappings", BusinessID: mapping.BusinessID,
		CanonicalJSON: string(mappingJSON), State: string(types.ConfigImportItemStateNew),
	}).Error)
	_, err = StageConfigImportBatch(context.Background(), 42, batch.ID)
	require.NoError(t, err)
	first, err := captureConfigImportBaseline(model.DB, batch.ID)
	require.NoError(t, err)

	require.NoError(t, model.DB.Model(&model.Option{}).Where("key = ?", "ModelPrice").Update("value", `{"canonical-video":1,"unrelated-model":3}`).Error)
	require.NoError(t, model.DB.Model(&model.RoutingPolicy{}).Where("id = ?", unrelatedPolicy.ID).Update("enabled", true).Error)
	second, err := captureConfigImportBaseline(model.DB, batch.ID)
	require.NoError(t, err)
	assert.Equal(t, first.Hash, second.Hash)

	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", channel.Id).Update("model_mapping", `{"canonical-video":"vendor-video","unrelated-model":"changed"}`).Error)
	third, err := captureConfigImportBaseline(model.DB, batch.ID)
	require.NoError(t, err)
	assert.NotEqual(t, second.Hash, third.Hash)

	require.NoError(t, model.DB.Model(&model.Option{}).Where("key = ?", "ModelPrice").Update("value", `{"canonical-video":4,"unrelated-model":3}`).Error)
	fourth, err := captureConfigImportBaseline(model.DB, batch.ID)
	require.NoError(t, err)
	assert.NotEqual(t, third.Hash, fourth.Hash)
}

func TestGetConfigImportBatchIncludesChannelModelSnapshotDiff(t *testing.T) {
	prepareConfigImportServiceDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Channel{}, &model.ChannelModelCostRule{}))
	existingMapping := `{"canonical-keep":"upstream-keep","canonical-old":"upstream-old"}`
	channel := &model.Channel{
		Type:         1,
		Name:         "supplier",
		Models:       "canonical-keep,upstream-keep,canonical-old,upstream-old",
		Key:          "key",
		ModelMapping: &existingMapping,
	}
	require.NoError(t, model.DB.Create(channel).Error)
	batch := createConfigImportStageBatch(t, channel.Id, "line-a", "upstream-keep")

	for _, mapping := range []types.ConfigImportModelMapping{
		{
			ConfigImportAuthoritativeEntity: types.ConfigImportAuthoritativeEntity{BusinessID: "mapping-keep"},
			CanonicalModel:                  "canonical-keep",
			ClientModel:                     "canonical-keep",
			LineRef:                         "line-a",
			UpstreamModel:                   "upstream-keep",
			SKURef:                          "sku-a",
		},
		{
			ConfigImportAuthoritativeEntity: types.ConfigImportAuthoritativeEntity{BusinessID: "mapping-new"},
			CanonicalModel:                  "canonical-new",
			ClientModel:                     "canonical-new",
			LineRef:                         "line-a",
			UpstreamModel:                   "upstream-new",
			SKURef:                          "sku-a",
		},
	} {
		encoded, err := common.Marshal(mapping)
		require.NoError(t, err)
		require.NoError(t, model.DB.Create(&model.ConfigImportItem{
			BatchID: batch.ID, EntityType: "model_mappings", BusinessID: mapping.BusinessID,
			CanonicalJSON: string(encoded), State: string(types.ConfigImportItemStateNew),
		}).Error)
	}

	detail, err := StageConfigImportBatch(context.Background(), 42, batch.ID)
	require.NoError(t, err)
	require.Len(t, detail.ChannelModelSnapshots, 1)
	assert.Equal(t, channel.Id, detail.ChannelModelSnapshots[0].ChannelID)
	assert.Equal(t, "supplier", detail.ChannelModelSnapshots[0].ChannelName)
	assert.Equal(t, []string{"line-a"}, detail.ChannelModelSnapshots[0].LineRefs)
	assert.Equal(t, []string{"canonical-new", "upstream-new"}, detail.ChannelModelSnapshots[0].AddedModels)
	assert.Equal(t, []string{"canonical-keep", "upstream-keep"}, detail.ChannelModelSnapshots[0].RetainedModels)
	assert.Equal(t, []string{"canonical-old", "upstream-old"}, detail.ChannelModelSnapshots[0].RemovedModels)
}

func TestConfigImportChannelModelSnapshotUnionsLinesBoundToOneChannel(t *testing.T) {
	prepareConfigImportServiceDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Channel{}))
	channel := &model.Channel{Type: 1, Name: "supplier", Models: "", Key: "key"}
	require.NoError(t, model.DB.Create(channel).Error)
	batch := createConfigImportStageBatch(t, channel.Id, "line-a", "upstream-a")
	confirmedAt := int64(1)
	require.NoError(t, model.DB.Create(&model.ConfigImportBinding{
		BatchID: batch.ID, LineRef: "line-b", Action: string(types.ConfigImportBindingActionBind), ChannelID: &channel.Id,
		CredentialsConfirmedBy: 42, CredentialsConfirmedAt: &confirmedAt,
	}).Error)
	for _, mapping := range []types.ConfigImportModelMapping{
		{ConfigImportAuthoritativeEntity: types.ConfigImportAuthoritativeEntity{BusinessID: "mapping-a"}, CanonicalModel: "canonical-a", LineRef: "line-a", UpstreamModel: "upstream-a"},
		{ConfigImportAuthoritativeEntity: types.ConfigImportAuthoritativeEntity{BusinessID: "mapping-b"}, CanonicalModel: "canonical-b", LineRef: "line-b", UpstreamModel: "upstream-b"},
	} {
		encoded, err := common.Marshal(mapping)
		require.NoError(t, err)
		require.NoError(t, model.DB.Create(&model.ConfigImportItem{
			BatchID: batch.ID, EntityType: "model_mappings", BusinessID: mapping.BusinessID,
			CanonicalJSON: string(encoded), State: string(types.ConfigImportItemStateNew),
		}).Error)
	}
	var items []model.ConfigImportItem
	require.NoError(t, model.DB.Where("batch_id = ?", batch.ID).Find(&items).Error)

	targets, err := configImportChannelModelSnapshotTargets(model.DB, items)
	require.NoError(t, err)
	require.Contains(t, targets, channel.Id)
	assert.Equal(t, map[string]struct{}{
		"canonical-a": {}, "upstream-a": {}, "canonical-b": {}, "upstream-b": {},
	}, targets[channel.Id].Models)
	assert.Equal(t, map[string]struct{}{"line-a": {}, "line-b": {}}, targets[channel.Id].LineRefs)
}

func TestConfigImportChannelModelSnapshotOmitsAmbiguousLegacyMapping(t *testing.T) {
	prepareConfigImportServiceDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Channel{}))
	channel := &model.Channel{Type: 1, Name: "supplier", Models: "", Key: "key"}
	require.NoError(t, model.DB.Create(channel).Error)
	batch := createConfigImportStageBatch(t, channel.Id, "line-a", "upstream-a")
	for _, mapping := range []types.ConfigImportModelMapping{
		{ConfigImportAuthoritativeEntity: types.ConfigImportAuthoritativeEntity{BusinessID: "mapping-a"}, CanonicalModel: "canonical-video", LineRef: "line-a", UpstreamModel: "upstream-a"},
		{ConfigImportAuthoritativeEntity: types.ConfigImportAuthoritativeEntity{BusinessID: "mapping-b"}, CanonicalModel: "canonical-video", LineRef: "line-a", UpstreamModel: "upstream-b"},
	} {
		encoded, err := common.Marshal(mapping)
		require.NoError(t, err)
		require.NoError(t, model.DB.Create(&model.ConfigImportItem{
			BatchID: batch.ID, EntityType: "model_mappings", BusinessID: mapping.BusinessID,
			CanonicalJSON: string(encoded), State: string(types.ConfigImportItemStateNew),
		}).Error)
	}
	var items []model.ConfigImportItem
	require.NoError(t, model.DB.Where("batch_id = ?", batch.ID).Find(&items).Error)

	targets, err := configImportChannelModelSnapshotTargets(model.DB, items)
	require.NoError(t, err)
	require.Contains(t, targets, channel.Id)
	assert.Equal(t, map[string]struct{}{
		"canonical-video": {}, "upstream-a": {}, "upstream-b": {},
	}, targets[channel.Id].Models)
	assert.Empty(t, targets[channel.Id].Mapping)
}

func TestConfigImportChannelModelSnapshotIncludesBoundChannelWithNoEffectiveMappings(t *testing.T) {
	prepareConfigImportServiceDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Channel{}))
	existingMapping := `{"canonical-old":"upstream-old"}`
	channel := &model.Channel{Type: 1, Name: "supplier", Models: "canonical-old,upstream-old", Key: "key", ModelMapping: &existingMapping}
	require.NoError(t, model.DB.Create(channel).Error)
	batch := createConfigImportStageBatch(t, channel.Id, "line-a", "upstream-old")

	var items []model.ConfigImportItem
	require.NoError(t, model.DB.Where("batch_id = ?", batch.ID).Find(&items).Error)
	targets, err := configImportChannelModelSnapshotTargets(model.DB, items)
	require.NoError(t, err)
	require.Contains(t, targets, channel.Id)
	assert.Empty(t, targets[channel.Id].Models)
	assert.Empty(t, targets[channel.Id].Mapping)
	assert.Equal(t, map[string]struct{}{"line-a": {}}, targets[channel.Id].LineRefs)

	diffs, err := configImportChannelModelSnapshotDiffs(model.DB, items)
	require.NoError(t, err)
	require.Len(t, diffs, 1)
	assert.Empty(t, diffs[0].AddedModels)
	assert.Empty(t, diffs[0].RetainedModels)
	assert.Equal(t, []string{"canonical-old", "upstream-old"}, diffs[0].RemovedModels)
}

func createConfigImportStageBatch(t *testing.T, channelID int, lineRef, upstreamModel string) model.ConfigImportBatch {
	t.Helper()
	summary, err := common.Marshal(configImportBatchSummaryStorage{ItemCounts: types.ConfigImportEntityCounts{CostRuleDrafts: 1}})
	require.NoError(t, err)
	batch := model.ConfigImportBatch{
		SchemaVersion: 1, TemplateVersion: "1", SourceSHA256: strings.Repeat("a", 64), PayloadSHA256: strings.Repeat("b", 64),
		Status: string(types.ConfigImportBatchStatusBinding), CreatedBy: 42, SummaryJSON: model.ConfigImportSummaryJSON(summary), BaselineJSON: "{}",
	}
	require.NoError(t, model.DB.Create(&batch).Error)

	line := types.ConfigImportChannelLine{ConfigImportAuthoritativeEntity: types.ConfigImportAuthoritativeEntity{BusinessID: lineRef}, LineRef: lineRef, ChannelRef: "channel-a", DisplayName: lineRef, ProviderTypeHint: "test", Region: "test", Protocol: "test", StatusProposal: "disabled"}
	sku := types.ConfigImportModelSKU{ConfigImportAuthoritativeEntity: types.ConfigImportAuthoritativeEntity{BusinessID: "sku-a"}, LineRef: lineRef, UpstreamModel: upstreamModel}
	draft := types.ConfigImportCostRuleDraft{ConfigImportAuthoritativeEntity: types.ConfigImportAuthoritativeEntity{BusinessID: "cost-a"}, LineRef: lineRef, UpstreamModel: upstreamModel, CostVariantKey: "default", RouteTargetRef: "target-a", CostMode: "per_request", Currency: "USD", UnitPrice: stringPointer("0.5"), BillingMultiplier: stringPointer("1"), PurchaseDiscountRatio: stringPointer("1"), RechargeExchangeRatio: stringPointer("1"), FeeRate: stringPointer("0"), CurrencyToUSDRate: stringPointer("1")}
	proposal := types.ConfigImportSaleProposal{ConfigImportAuthoritativeEntity: types.ConfigImportAuthoritativeEntity{BusinessID: "sale-a"}, ModelSKURef: sku.BusinessID, Currency: "USD", UnitPrice: stringPointer("1")}
	blueprint := types.ConfigImportRouteBlueprint{ConfigImportAuthoritativeEntity: types.ConfigImportAuthoritativeEntity{BusinessID: "route-a"}, CanonicalModel: "canonical-video", ClientModel: "canonical-video", MergeMode: types.ConfigImportRouteMergeModeMerge, Targets: []types.ConfigImportRouteTarget{{RouteTargetRef: "target-a", LineRef: lineRef, UpstreamModel: upstreamModel, SKURef: sku.BusinessID, CostVariantKey: "default", ReferenceMinimums: configImportReferenceBounds(0, 0, 0), ReferenceLimits: configImportReferenceBounds(9, 3, 3), Enabled: boolPointer(false)}}}
	for entityType, entry := range map[string]struct {
		id    string
		value any
	}{
		"channel_lines": {lineRef, line}, "model_skus": {sku.BusinessID, sku}, "cost_rule_drafts": {draft.BusinessID, draft}, "sale_proposals": {proposal.BusinessID, proposal}, "route_blueprints": {blueprint.BusinessID, blueprint},
	} {
		encoded, marshalErr := common.Marshal(entry.value)
		require.NoError(t, marshalErr)
		item := &model.ConfigImportItem{BatchID: batch.ID, EntityType: entityType, BusinessID: entry.id, CanonicalJSON: string(encoded), State: string(types.ConfigImportItemStateNew)}
		require.NoError(t, model.DB.Create(item).Error)
	}
	if channelID > 0 {
		confirmedAt := int64(1)
		require.NoError(t, model.DB.Create(&model.ConfigImportBinding{
			BatchID: batch.ID, LineRef: lineRef, Action: string(types.ConfigImportBindingActionBind), ChannelID: &channelID,
			CredentialsConfirmedBy: 42, CredentialsConfirmedAt: &confirmedAt,
		}).Error)
	}
	return batch
}

func boolPointer(value bool) *bool { return &value }

func TestConfigImportCostRuleConfigUpgradesLegacyTaskChargeEvent(t *testing.T) {
	draft := types.ConfigImportCostRuleDraft{
		CostMode:    string(types.CostModePerDuration),
		ChargeEvent: string(types.CostChargeResponseSucceeded),
	}

	config, err := configImportCostRuleConfig(draft)

	require.NoError(t, err)
	assert.Equal(t, types.CostChargeTaskSucceeded, config.ChargeEvent)
}
