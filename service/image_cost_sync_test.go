package service_test

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/image_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSyncImageCostRulesForChannelRetiresUnsupportedImageMatrixEntries(t *testing.T) {
	prepareImageCostSyncDB(t)
	originalCatalog := image_setting.Catalog2JSONString()
	t.Cleanup(func() { require.NoError(t, image_setting.UpdateCatalogByJSONString(originalCatalog)) })
	require.NoError(t, image_setting.UpdateCatalogByJSONString(imageCostSyncCatalogWithAlt(t)))

	mapping := `{"gpt-image-2":"vendor-image"}`
	channel := &model.Channel{
		Id: 41, Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled,
		Models: "gpt-image-2", ModelMapping: &mapping,
		OtherSettings: `{"image_profile":{"profile":"openai_images","profile_version":1,"capability_overrides":{"gpt-image-2":{"resolution_qualities":["1k:low"],"edits":false}}}}`,
	}
	require.NoError(t, model.DB.Create(channel).Error)
	activeGen := seedImageSyncRule(t, 41, "vendor-image", "gen-1k-low", types.CostRuleActive, "0.01")
	draftGen := seedImageSyncRule(t, 41, "vendor-image", "gen-2k-high", types.CostRuleDraft, "0.02")
	activeEdit := seedImageSyncRule(t, 41, "vendor-image", "edit-1k-low", types.CostRuleActive, "0.03")

	require.NoError(t, service.SyncImageCostRulesForChannel(41, 7))

	var got model.ChannelModelCostRule
	require.NoError(t, model.DB.First(&got, activeGen.ID).Error)
	assert.Equal(t, string(types.CostRuleActive), got.Status)
	got = model.ChannelModelCostRule{}
	require.NoError(t, model.DB.First(&got, draftGen.ID).Error)
	assert.Equal(t, string(types.CostRuleRetired), got.Status)
	assert.NotNil(t, got.EffectiveTo)
	got = model.ChannelModelCostRule{}
	require.NoError(t, model.DB.First(&got, activeEdit.ID).Error)
	assert.Equal(t, string(types.CostRuleRetired), got.Status)
}

func TestSyncImageCostRulesForChannelRetiresOnlyUnusedMappedModelRules(t *testing.T) {
	prepareImageCostSyncDB(t)
	originalCatalog := image_setting.Catalog2JSONString()
	t.Cleanup(func() { require.NoError(t, image_setting.UpdateCatalogByJSONString(originalCatalog)) })
	require.NoError(t, image_setting.UpdateCatalogByJSONString(imageCostSyncCatalogWithAlt(t)))

	mapping := `{"gpt-image-2":"vendor-old","gpt-image-2-alt":"vendor-shared"}`
	channel := &model.Channel{
		Id: 42, Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled,
		Models: "gpt-image-2,gpt-image-2-alt", ModelMapping: &mapping,
		OtherSettings: `{"image_profile":{"profile":"openai_images","profile_version":1}}`,
	}
	require.NoError(t, model.DB.Create(channel).Error)
	oldRule := seedImageSyncRule(t, 42, "vendor-old", "gen-1k-low", types.CostRuleActive, "0.01")
	sharedRule := seedImageSyncRule(t, 42, "vendor-shared", "gen-1k-low", types.CostRuleActive, "0.02")
	newRule := seedImageSyncRule(t, 42, "vendor-new", "gen-1k-low", types.CostRuleActive, "0.03")

	mapping = `{"gpt-image-2":"vendor-new","gpt-image-2-alt":"vendor-shared"}`
	channel.ModelMapping = &mapping
	require.NoError(t, service.SyncImageCostRulesForChannelSnapshot(channel, 7))

	var got model.ChannelModelCostRule
	require.NoError(t, model.DB.First(&got, oldRule.ID).Error)
	assert.Equal(t, string(types.CostRuleRetired), got.Status)
	got = model.ChannelModelCostRule{}
	require.NoError(t, model.DB.First(&got, sharedRule.ID).Error)
	assert.Equal(t, string(types.CostRuleActive), got.Status)
	got = model.ChannelModelCostRule{}
	require.NoError(t, model.DB.First(&got, newRule.ID).Error)
	assert.Equal(t, string(types.CostRuleActive), got.Status)
}

func TestSyncImageCostRulesForChannelDoesNotCreateMissingPrices(t *testing.T) {
	prepareImageCostSyncDB(t)
	originalCatalog := image_setting.Catalog2JSONString()
	t.Cleanup(func() { require.NoError(t, image_setting.UpdateCatalogByJSONString(originalCatalog)) })
	require.NoError(t, image_setting.UpdateCatalogByJSONString(imageCostSyncCatalogFixture))

	channel := &model.Channel{
		Id: 43, Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled,
		Models: "gpt-image-2", OtherSettings: `{"image_profile":{"profile":"openai_images","profile_version":1}}`,
	}
	require.NoError(t, model.DB.Create(channel).Error)
	require.NoError(t, service.SyncImageCostRulesForChannel(43, 7))

	var count int64
	require.NoError(t, model.DB.Model(&model.ChannelModelCostRule{}).Where("channel_id = ?", 43).Count(&count).Error)
	assert.Zero(t, count)
}

func TestSyncImageCostRulesForChannelWithoutProfileRetiresImageRules(t *testing.T) {
	prepareImageCostSyncDB(t)
	originalCatalog := image_setting.Catalog2JSONString()
	t.Cleanup(func() { require.NoError(t, image_setting.UpdateCatalogByJSONString(originalCatalog)) })
	require.NoError(t, image_setting.UpdateCatalogByJSONString(imageCostSyncCatalogFixture))

	channel := &model.Channel{Id: 44, Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Models: "gpt-image-2"}
	require.NoError(t, model.DB.Create(channel).Error)
	rule := seedImageSyncRule(t, 44, "gpt-image-2", "gen-1k-low", types.CostRuleDraft, "0.01")

	require.NoError(t, service.SyncImageCostRulesForChannel(44, 7))

	var got model.ChannelModelCostRule
	require.NoError(t, model.DB.First(&got, rule.ID).Error)
	assert.Equal(t, string(types.CostRuleRetired), got.Status)
}

func prepareImageCostSyncDB(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	previousDB := model.DB
	previousMainType := common.MainDatabaseType()
	previousLogType := common.LogDatabaseType()
	model.DB = db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, previousLogType)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.ChannelModelCostRule{}))
	t.Cleanup(func() {
		model.DB = previousDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
		require.NoError(t, sqlDB.Close())
	})
}

func seedImageSyncRule(t *testing.T, channelID int, upstreamModel, variant string, status types.CostRuleStatus, unitPrice string) model.ChannelModelCostRule {
	t.Helper()
	price := unitPrice
	config := types.CostRuleConfigV1{
		Currency: "USD", BillingMultiplier: "1", PurchaseDiscountRatio: "1",
		RechargeExchangeRatio: "1", FeeRate: "0", CurrencyToUSDRate: "1",
		UnitPrice: &price, ChargeEvent: types.CostChargeResponseSucceeded,
		MeterSource: types.CostMeterValidatedRequest,
	}
	normalized, err := service.NormalizeCostRuleConfig(types.CostModePerImage, config)
	require.NoError(t, err)
	encoded, err := common.Marshal(normalized)
	require.NoError(t, err)
	now := common.GetTimestamp()
	rule := model.ChannelModelCostRule{
		ChannelID: channelID, BillableUpstreamModel: upstreamModel, CostVariantKey: variant,
		Version: 1, Status: string(status), CostMode: string(types.CostModePerImage), SchemaVersion: 1,
		ConfigJSON: string(encoded), Source: "manual", EffectiveFrom: &now,
		CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, model.DB.Create(&rule).Error)
	return rule
}

const imageCostSyncCatalogFixture = `{"version":1,"models":{"gpt-image-2":{"profile":"openai_images","profile_version":1,"endpoints":{"generations":{"capability":{"enabled":true,"resolution_tiers":["1k","2k","4k"],"qualities":["low","medium","high"],"response_formats":["b64_json"],"max_n":4},"default_size":"auto","default_quality":"medium","default_response_format":"b64_json"},"edits":{"capability":{"enabled":true,"resolution_tiers":["1k","2k","4k"],"qualities":["low","medium","high"],"response_formats":["b64_json"],"max_n":4,"max_input_images":1,"supports_mask":true},"default_size":"auto","default_quality":"medium","default_response_format":"b64_json"}},"skus":{"gen-1k-low":{"endpoint":"generations","tier":"1k","quality":"low","unit":"image","sale_price_usd":"0.02"},"gen-1k-medium":{"endpoint":"generations","tier":"1k","quality":"medium","unit":"image","sale_price_usd":"0.025"},"gen-1k-high":{"endpoint":"generations","tier":"1k","quality":"high","unit":"image","sale_price_usd":"0.03"},"gen-2k-high":{"endpoint":"generations","tier":"2k","quality":"high","unit":"image","sale_price_usd":"0.05"},"gen-4k-high":{"endpoint":"generations","tier":"4k","quality":"high","unit":"image","sale_price_usd":"0.08"},"edit-1k-low":{"endpoint":"edits","tier":"1k","quality":"low","unit":"image","sale_price_usd":"0.02"},"edit-1k-medium":{"endpoint":"edits","tier":"1k","quality":"medium","unit":"image","sale_price_usd":"0.025"}}}}}`

func imageCostSyncCatalogWithAlt(t *testing.T) string {
	t.Helper()
	var catalog image_setting.Catalog
	require.NoError(t, common.UnmarshalJsonStr(imageCostSyncCatalogFixture, &catalog))
	catalog.Models["gpt-image-2-alt"] = catalog.Models["gpt-image-2"]
	encoded, err := common.Marshal(catalog)
	require.NoError(t, err)
	return string(encoded)
}
