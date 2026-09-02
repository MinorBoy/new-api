package service_test

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/image_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestImagePricingCostSummaryUsesMappedActiveRules(t *testing.T) {
	prepareImagePricingDB(t)
	originalCatalog := image_setting.Catalog2JSONString()
	t.Cleanup(func() { require.NoError(t, image_setting.UpdateCatalogByJSONString(originalCatalog)) })
	require.NoError(t, image_setting.UpdateCatalogByJSONString(imagePricingCatalogFixture))
	seedImagePricingChannel(t, 41, `{"gpt-image-2":"vendor-cheap"}`, true)
	seedImagePricingChannel(t, 42, `{"gpt-image-2":"vendor-expensive"}`, true)
	seedActiveImageRule(t, 41, "vendor-cheap", "gen-1024x1024-medium", "0.008")
	seedActiveImageRule(t, 42, "vendor-expensive", "gen-1024x1024-medium", "0.012")

	summary, err := service.ListImagePricingCostSummary([]string{"gpt-image-2"})
	require.NoError(t, err)
	item := requireImagePricingItem(t, summary, "gpt-image-2", "gen-1024x1024-medium")
	assert.True(t, item.Known)
	assert.Equal(t, "0.008", item.MinimumCostUSD)
	assert.Equal(t, 2, item.SourceCount)
	assert.NotContains(t, string(mustMarshal(t, summary)), "secret")
}

func TestImagePricingCostSummaryIgnoresDraftRetiredAndUnmappedRules(t *testing.T) {
	prepareImagePricingDB(t)
	originalCatalog := image_setting.Catalog2JSONString()
	t.Cleanup(func() { require.NoError(t, image_setting.UpdateCatalogByJSONString(originalCatalog)) })
	require.NoError(t, image_setting.UpdateCatalogByJSONString(imagePricingCatalogFixture))
	seedImagePricingChannel(t, 51, `{"other-model":"vendor-model"}`, true)
	seedImagePricingChannel(t, 52, `{"gpt-image-2":"vendor-draft"}`, true)
	seedImagePricingChannel(t, 53, `{"gpt-image-2":"vendor-disabled"}`, false)
	seedDraftImageRule(t, 52, "vendor-draft", "gen-1024x1024-medium", "0.001")
	seedActiveImageRuleWithStatus(t, 53, "vendor-disabled", "gen-1024x1024-medium", "0.002", types.CostRuleRetired)

	summary, err := service.ListImagePricingCostSummary([]string{"gpt-image-2"})
	require.NoError(t, err)
	item := requireImagePricingItem(t, summary, "gpt-image-2", "gen-1024x1024-medium")
	assert.False(t, item.Known)
	assert.Equal(t, 0, item.SourceCount)
	assert.Empty(t, item.MinimumCostUSD)
}

func TestImagePricingCostSummaryReturnsAllCatalogSKUsForEmptyModelFilter(t *testing.T) {
	prepareImagePricingDB(t)
	originalCatalog := image_setting.Catalog2JSONString()
	t.Cleanup(func() { require.NoError(t, image_setting.UpdateCatalogByJSONString(originalCatalog)) })
	require.NoError(t, image_setting.UpdateCatalogByJSONString(imagePricingCatalogFixture))

	summary, err := service.ListImagePricingCostSummary(nil)
	require.NoError(t, err)
	assert.Equal(t, []string{
		"gpt-image-2|gen-1024x1024-medium",
		"gpt-image-2|gen-4096x4096-medium",
	}, imagePricingItemIDs(summary))
}

func prepareImagePricingDB(t *testing.T) {
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

func seedImagePricingChannel(t *testing.T, id int, mapping string, enabled bool) {
	t.Helper()
	status := common.ChannelStatusAutoDisabled
	if enabled {
		status = common.ChannelStatusEnabled
	}
	modelMapping := mapping
	require.NoError(t, model.DB.Create(&model.Channel{
		Id: id, Type: constant.ChannelTypeOpenAI, Key: "secret", Status: status,
		Models: "gpt-image-2", ModelMapping: &modelMapping,
	}).Error)
}

func seedActiveImageRule(t *testing.T, channelID int, upstreamModel, sku, unitPrice string) {
	seedActiveImageRuleWithStatus(t, channelID, upstreamModel, sku, unitPrice, types.CostRuleActive)
}

func seedDraftImageRule(t *testing.T, channelID int, upstreamModel, sku, unitPrice string) {
	seedActiveImageRuleWithStatus(t, channelID, upstreamModel, sku, unitPrice, types.CostRuleDraft)
}

func seedActiveImageRuleWithStatus(t *testing.T, channelID int, upstreamModel, sku, unitPrice string, status types.CostRuleStatus) {
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
	configJSON, err := common.Marshal(normalized)
	require.NoError(t, err)
	now := common.GetTimestamp()
	require.NoError(t, model.DB.Create(&model.ChannelModelCostRule{
		ChannelID: channelID, BillableUpstreamModel: upstreamModel, CostVariantKey: sku,
		Version: 1, Status: string(status), CostMode: string(types.CostModePerImage), SchemaVersion: 1,
		ConfigJSON: string(configJSON), Source: "manual", EffectiveFrom: &now,
		CreatedAt: now, UpdatedAt: now,
	}).Error)
}

func requireImagePricingItem(t *testing.T, summary typesImagePricingSummary, modelName, sku string) typesImagePricingItem {
	t.Helper()
	for _, item := range summary.Items {
		if item.Model == modelName && item.SKU == sku {
			return item
		}
	}
	t.Fatalf("image pricing item %s|%s not found", modelName, sku)
	return typesImagePricingItem{}
}

func imagePricingItemIDs(summary typesImagePricingSummary) []string {
	ids := make([]string, 0, len(summary.Items))
	for _, item := range summary.Items {
		ids = append(ids, item.Model+"|"+item.SKU)
	}
	return ids
}

func mustMarshal(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := common.Marshal(value)
	require.NoError(t, err)
	return encoded
}

// Aliases keep this test readable before the production DTO is introduced.
type typesImagePricingSummary = dto.ImagePricingCostSummary
type typesImagePricingItem = dto.ImagePricingCostItem

const imagePricingCatalogFixture = `{"version":1,"models":{"gpt-image-2":{"profile":"openai_images","profile_version":1,"endpoints":{"generations":{"capability":{"enabled":true,"sizes":["1024x1024","4096x4096"],"qualities":["medium"],"response_formats":["b64_json"],"max_n":4},"default_size":"1024x1024","default_quality":"medium","default_response_format":"b64_json"}},"skus":{"gen-1024x1024-medium":{"endpoint":"generations","size":"1024x1024","quality":"medium","unit":"image","sale_price_usd":"0.03"},"gen-4096x4096-medium":{"endpoint":"generations","size":"4096x4096","quality":"medium","unit":"image","sale_price_usd":"0.08"}}}}}`
