package service

import (
	"context"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
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

func TestConfigImportStageResolutionAcceptsStructuredActions(t *testing.T) {
	prepareConfigImportServiceDB(t)
	batch := createConfigImportStageBatch(t, 0, "line-a", "vendor-video")
	for _, businessID := range []string{"variant-a", "variant-b", "variant-c"} {
		require.NoError(t, model.DB.Create(&model.ConfigImportItem{BatchID: batch.ID, EntityType: "unresolved_variants", BusinessID: businessID, CanonicalJSON: "{}", State: string(types.ConfigImportItemStateConflict)}).Error)
	}

	_, err := UpdateConfigImportResolutions(context.Background(), 42, batch.ID, []dto.ConfigImportResolutionInput{
		{ItemBusinessID: "variant-c", Action: "exclude"},
		{ItemBusinessID: "variant-b", Action: "bind_variant"},
		{ItemBusinessID: "variant-a", Action: "split_line"},
	})
	require.NoError(t, err)
	var resolutions []model.ConfigImportResolution
	require.NoError(t, model.DB.Where("batch_id = ?", batch.ID).Order("id ASC").Find(&resolutions).Error)
	require.Equal(t, []string{"variant-a", "variant-b", "variant-c"}, []string{resolutions[0].ItemBusinessID, resolutions[1].ItemBusinessID, resolutions[2].ItemBusinessID})
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

func createConfigImportStageBatch(t *testing.T, channelID int, lineRef, upstreamModel string) model.ConfigImportBatch {
	t.Helper()
	summary, err := common.Marshal(configImportBatchSummaryStorage{ItemCounts: types.ConfigImportEntityCounts{CostRuleDrafts: 1}})
	require.NoError(t, err)
	batch := model.ConfigImportBatch{
		SchemaVersion: 1, TemplateVersion: "1", SourceSHA256: strings.Repeat("a", 64), PayloadSHA256: strings.Repeat("b", 64),
		Status: string(types.ConfigImportBatchStatusBinding), CreatedBy: 42, SummaryJSON: string(summary), BaselineJSON: "{}",
	}
	require.NoError(t, model.DB.Create(&batch).Error)

	line := types.ConfigImportChannelLine{ConfigImportAuthoritativeEntity: types.ConfigImportAuthoritativeEntity{BusinessID: lineRef}, LineRef: lineRef, ChannelRef: "channel-a", DisplayName: lineRef, ProviderTypeHint: "test", Region: "test", Protocol: "test", StatusProposal: "disabled"}
	sku := types.ConfigImportModelSKU{ConfigImportAuthoritativeEntity: types.ConfigImportAuthoritativeEntity{BusinessID: "sku-a"}, LineRef: lineRef, UpstreamModel: upstreamModel}
	draft := types.ConfigImportCostRuleDraft{ConfigImportAuthoritativeEntity: types.ConfigImportAuthoritativeEntity{BusinessID: "cost-a"}, LineRef: lineRef, UpstreamModel: upstreamModel, CostVariantKey: "default", RouteTargetRef: "target-a", CostMode: "per_request", Currency: "USD", UnitPrice: stringPointer("0.5"), BillingMultiplier: stringPointer("1"), PurchaseDiscountRatio: stringPointer("1"), RechargeExchangeRatio: stringPointer("1"), FeeRate: stringPointer("0"), CurrencyToUSDRate: stringPointer("1")}
	proposal := types.ConfigImportSaleProposal{ConfigImportAuthoritativeEntity: types.ConfigImportAuthoritativeEntity{BusinessID: "sale-a"}, ModelSKURef: sku.BusinessID, Currency: "USD", UnitPrice: stringPointer("1")}
	blueprint := types.ConfigImportRouteBlueprint{ConfigImportAuthoritativeEntity: types.ConfigImportAuthoritativeEntity{BusinessID: "route-a"}, CanonicalModel: "canonical-video", ClientModel: "canonical-video", MergeMode: types.ConfigImportRouteMergeModeMerge, Targets: []types.ConfigImportRouteTarget{{RouteTargetRef: "target-a", LineRef: lineRef, UpstreamModel: upstreamModel, SKURef: sku.BusinessID, CostVariantKey: "default", Enabled: boolPointer(false)}}}
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
		require.NoError(t, model.DB.Create(&model.ConfigImportBinding{BatchID: batch.ID, LineRef: lineRef, Action: string(types.ConfigImportBindingActionBind), ChannelID: &channelID}).Error)
	}
	return batch
}

func boolPointer(value bool) *bool { return &value }
