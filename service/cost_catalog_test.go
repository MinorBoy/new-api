package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/types"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestListSupplierCostCatalogProjectsAllCostModes(t *testing.T) {
	prepareCostCatalogServiceDB(t)
	seedCostCatalogServiceRows(t)

	page, err := ListSupplierCostCatalog(CostCatalogFilter{Status: "active", Page: 1, PageSize: 50})
	require.NoError(t, err)
	assert.Equal(t, int64(9), page.Total)
	require.Len(t, page.Items, 9)

	free := findCatalogItem(t, page.Items, "free-model")
	assert.Empty(t, free.Prices)
	assert.Equal(t, "available", free.PriceStatus)

	perRequest := findCatalogItem(t, page.Items, "per-request-model")
	require.Len(t, perRequest.Prices, 1)
	assert.Equal(t, "unit_price", perRequest.Prices[0].Key)
	assert.Equal(t, "per_request", perRequest.Prices[0].Unit)
	assert.Equal(t, "3", perRequest.Prices[0].NativeAmount)
	assert.Equal(t, "3", perRequest.Prices[0].NormalizedUSDAmount)

	perDuration := findCatalogItem(t, page.Items, "per-duration-model")
	require.Len(t, perDuration.Prices, 1)
	assert.Equal(t, "price_per_second", perDuration.Prices[0].Key)
	assert.Equal(t, "per_second", perDuration.Prices[0].Unit)

	totalTokens := findCatalogItem(t, page.Items, "total-token-model")
	require.Len(t, totalTokens.Prices, 1)
	assert.Equal(t, "total_per_million", totalTokens.Prices[0].Key)
	assert.Equal(t, "per_million_tokens", totalTokens.Prices[0].Unit)

	completionTokens := findCatalogItem(t, page.Items, "completion-token-model")
	require.Len(t, completionTokens.Prices, 1)
	assert.Equal(t, "completion_per_million", completionTokens.Prices[0].Key)
	assert.Equal(t, "per_million_completion_tokens", completionTokens.Prices[0].Unit)

	inputOutput := findCatalogItem(t, page.Items, "input-output-token-model")
	require.Len(t, inputOutput.Prices, 2)
	assert.Equal(t, []string{"input_per_million", "output_per_million"}, []string{inputOutput.Prices[0].Key, inputOutput.Prices[1].Key})
}

func TestListSupplierCostCatalogProjectsPerImagePrice(t *testing.T) {
	prepareCostCatalogServiceDB(t)
	channel := model.Channel{Id: 7, Name: "Images", Type: 1, Key: "image-secret"}
	require.NoError(t, model.DB.Create(&channel).Error)
	config := validCatalogCostConfig("USD")
	config.UnitPrice = catalogStringPointer("0.04")
	config.MeterSource = types.CostMeterUpstreamActual
	config.ChargeEvent = types.CostChargeResponseSucceeded
	rule := catalogCostRule(t, channel.Id, "gpt-image-1", 1, types.CostRuleActive, types.CostModePerImage, config, "manual")
	require.NoError(t, model.DB.Create(&rule).Error)

	page, err := ListSupplierCostCatalog(CostCatalogFilter{Status: "active", Page: 1, PageSize: 25})
	require.NoError(t, err)
	item := findCatalogItem(t, page.Items, "gpt-image-1")
	require.Len(t, item.Prices, 1)
	assert.Equal(t, "unit_price", item.Prices[0].Key)
	assert.Equal(t, "per_image", item.Prices[0].Unit)
	assert.Equal(t, "0.04", item.Prices[0].NativeAmount)
}

func TestListSupplierCostCatalogUsesFullPerRequestPriceAndComparisonOnly(t *testing.T) {
	prepareCostCatalogServiceDB(t)
	seedCostCatalogServiceRows(t)

	page, err := ListSupplierCostCatalog(CostCatalogFilter{Status: "active", Page: 1, PageSize: 50})
	require.NoError(t, err)
	row := findCatalogItem(t, page.Items, "per-request-model")
	require.Len(t, row.Prices, 1)
	assert.Equal(t, "3", row.Prices[0].NormalizedUSDAmount)
	require.NotNil(t, row.Comparison15SEquivalentUSDPerSecond)
	assert.Equal(t, "0.2", *row.Comparison15SEquivalentUSDPerSecond)
	assert.Equal(t, "available", row.PriceStatus)
}

func TestListSupplierCostCatalogFiltersCurrencyAfterStructuredParsing(t *testing.T) {
	prepareCostCatalogServiceDB(t)
	seedCostCatalogServiceRows(t)

	page, err := ListSupplierCostCatalog(CostCatalogFilter{
		Status: "active", Currency: "cny", Page: 1, PageSize: 25,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1), page.Total)
	require.Len(t, page.Items, 1)
	assert.Equal(t, "per-duration-model", page.Items[0].BillableUpstreamModel)
	assert.Equal(t, "CNY", page.Items[0].Currency)
	assert.Equal(t, int64(1), page.Summary.ActiveRuleCount)
	assert.Zero(t, page.Summary.DraftRuleCount)
	assert.Zero(t, page.Summary.RetiredRuleCount)
}

func TestListSupplierCostCatalogReportsInvalidPricesWithoutZeroFallback(t *testing.T) {
	prepareCostCatalogServiceDB(t)
	seedCostCatalogServiceRows(t)

	page, err := ListSupplierCostCatalog(CostCatalogFilter{Status: "active", Page: 1, PageSize: 50})
	require.NoError(t, err)
	invalid := findCatalogItem(t, page.Items, "invalid-config-model")
	assert.Equal(t, "unavailable", invalid.PriceStatus)
	assert.Contains(t, invalid.Issues, "invalid_config")
	assert.Empty(t, invalid.Prices)
	assert.Nil(t, invalid.Comparison15SEquivalentUSDPerSecond)

	missing := findCatalogItem(t, page.Items, "missing-normalized-model")
	assert.Equal(t, "unavailable", missing.PriceStatus)
	assert.Contains(t, missing.Issues, "missing_normalized_price")
	for _, price := range missing.Prices {
		assert.Empty(t, price.NativeAmount)
		assert.Empty(t, price.NormalizedUSDAmount)
	}
}

func TestListSupplierCostCatalogKeepsValidPriceForMissingChannel(t *testing.T) {
	prepareCostCatalogServiceDB(t)
	seedCostCatalogServiceRows(t)

	page, err := ListSupplierCostCatalog(CostCatalogFilter{Status: "active", Page: 1, PageSize: 50})
	require.NoError(t, err)
	orphan := findCatalogItem(t, page.Items, "orphan-model")
	assert.True(t, orphan.ChannelMissing)
	assert.Equal(t, "available", orphan.PriceStatus)
	assert.Contains(t, orphan.Issues, "channel_missing")
	require.Len(t, orphan.Prices, 1)
	assert.Equal(t, "1", orphan.Prices[0].NormalizedUSDAmount)
}

func TestListSupplierCostCatalogSummaryIgnoresStatusFilter(t *testing.T) {
	prepareCostCatalogServiceDB(t)
	seedCostCatalogServiceRows(t)

	page, err := ListSupplierCostCatalog(CostCatalogFilter{Status: "draft", Page: 1, PageSize: 50})
	require.NoError(t, err)
	assert.Equal(t, int64(1), page.Total)
	require.Len(t, page.Items, 1)
	assert.Equal(t, "draft", page.Items[0].Status)
	assert.Equal(t, int64(3), page.Summary.ChannelCount)
	assert.Equal(t, int64(9), page.Summary.ActiveRuleCount)
	assert.Equal(t, int64(1), page.Summary.DraftRuleCount)
	assert.Equal(t, int64(1), page.Summary.RetiredRuleCount)
}

func TestListSupplierCostCatalogFacetsIgnoreCurrentFilters(t *testing.T) {
	prepareCostCatalogServiceDB(t)
	seedCostCatalogServiceRows(t)

	page, err := ListSupplierCostCatalog(CostCatalogFilter{
		ChannelID: 1, Status: "draft", Currency: "usd", Source: "manual", Page: 1, PageSize: 25,
	})
	require.NoError(t, err)
	require.Len(t, page.Facets.Channels, 3)
	assert.Equal(t, []string{"Alpha", "Beta", ""}, []string{
		page.Facets.Channels[0].Name, page.Facets.Channels[1].Name, page.Facets.Channels[2].Name,
	})
	assert.Equal(t, []string{"CNY", "EUR", "JPY", "USD"}, page.Facets.Currencies)
	assert.Equal(t, []string{"config_import", "manual"}, page.Facets.Sources)
}

func TestGetSupplierCostCatalogDetailReturnsConfigAndHistory(t *testing.T) {
	prepareCostCatalogServiceDB(t)
	seedCostCatalogServiceRows(t)

	page, err := ListSupplierCostCatalog(CostCatalogFilter{Status: "active", Page: 1, PageSize: 50})
	require.NoError(t, err)
	active := findCatalogItem(t, page.Items, "per-request-model")
	detail, err := GetSupplierCostCatalogDetail(active.RuleID)
	require.NoError(t, err)
	require.NotNil(t, detail.Config)
	assert.Equal(t, "USD", detail.Config.Currency)
	assert.Equal(t, active.RuleID, detail.Rule.RuleID)
	require.Len(t, detail.History, 3)
	assert.Equal(t, []int{2, 1, 0}, []int{
		detail.History[0].Version, detail.History[1].Version, detail.History[2].Version,
	})
}

func prepareCostCatalogServiceDB(t *testing.T) {
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
	t.Cleanup(func() {
		model.DB = previousDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
		require.NoError(t, sqlDB.Close())
	})
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.ChannelModelCostRule{}))
}

func seedCostCatalogServiceRows(t *testing.T) {
	t.Helper()
	channels := []model.Channel{
		{Id: 1, Name: "Alpha", Type: 10, Key: "alpha-secret"},
		{Id: 2, Name: "Beta", Type: 20, Key: "beta-secret"},
	}
	require.NoError(t, model.DB.Create(&channels).Error)

	perRequestConfig := validCatalogCostConfig("USD")
	perRequestConfig.UnitPrice = catalogStringPointer("3")
	perDurationConfig := validCatalogCostConfig("CNY")
	perDurationConfig.CurrencyToUSDRate = "0.14"
	perDurationConfig.PricePerSecond = catalogStringPointer("2")
	totalConfig := validCatalogCostConfig("USD")
	totalConfig.TokenMode = types.CostTokenModeTotal
	totalConfig.TotalPerMillion = catalogStringPointer("2")
	completionConfig := validCatalogCostConfig("USD")
	completionConfig.TokenMode = types.CostTokenModeCompletion
	completionConfig.CompletionPerMillion = catalogStringPointer("3")
	inputOutputConfig := validCatalogCostConfig("USD")
	inputOutputConfig.TokenMode = types.CostTokenModeInputOutput
	inputOutputConfig.InputPerMillion = catalogStringPointer("1")
	inputOutputConfig.OutputPerMillion = catalogStringPointer("4")
	orphanConfig := validCatalogCostConfig("JPY")
	orphanConfig.CurrencyToUSDRate = "0.01"
	orphanConfig.UnitPrice = catalogStringPointer("100")
	draftConfig := validCatalogCostConfig("USD")
	draftConfig.UnitPrice = catalogStringPointer("4")
	retiredConfig := validCatalogCostConfig("EUR")
	retiredConfig.CurrencyToUSDRate = "1.1"
	retiredConfig.UnitPrice = catalogStringPointer("5")

	rules := []model.ChannelModelCostRule{
		catalogCostRule(t, 1, "free-model", 1, types.CostRuleActive, types.CostModeFree, types.CostRuleConfigV1{ZeroCostReason: "promotional"}, "manual"),
		catalogCostRule(t, 1, "per-request-model", 1, types.CostRuleActive, types.CostModePerRequest, perRequestConfig, "config_import"),
		catalogCostRule(t, 2, "per-duration-model", 1, types.CostRuleActive, types.CostModePerDuration, perDurationConfig, "manual"),
		catalogCostRule(t, 2, "total-token-model", 1, types.CostRuleActive, types.CostModePerToken, totalConfig, "manual"),
		catalogCostRule(t, 2, "completion-token-model", 1, types.CostRuleActive, types.CostModePerToken, completionConfig, "manual"),
		catalogCostRule(t, 2, "input-output-token-model", 1, types.CostRuleActive, types.CostModePerToken, inputOutputConfig, "manual"),
		catalogRawCostRule(2, "invalid-config-model", 1, types.CostRuleActive, types.CostModePerRequest, "{", "manual"),
		catalogRawCostRule(2, "missing-normalized-model", 1, types.CostRuleActive, types.CostModePerRequest, `{"currency":"USD","unit_price":"1"}`, "manual"),
		catalogCostRule(t, 99, "orphan-model", 1, types.CostRuleActive, types.CostModePerRequest, orphanConfig, "manual"),
		catalogCostRule(t, 1, "per-request-model", 2, types.CostRuleDraft, types.CostModePerRequest, draftConfig, "manual"),
		catalogCostRule(t, 1, "per-request-model", 0, types.CostRuleRetired, types.CostModePerRequest, retiredConfig, "manual"),
	}
	for i := range rules {
		rules[i].CreatedAt = int64(100 + i)
		rules[i].UpdatedAt = int64(200 + i)
		rules[i].CreatedBy = 7
		rules[i].ActivatedBy = 8
	}
	require.NoError(t, model.DB.Create(&rules).Error)
}

func catalogCostRule(t *testing.T, channelID int, modelName string, version int, status types.CostRuleStatus, mode types.CostMode, config types.CostRuleConfigV1, source string) model.ChannelModelCostRule {
	t.Helper()
	normalized, err := NormalizeCostRuleConfig(mode, config)
	require.NoError(t, err)
	configJSON, err := common.Marshal(normalized)
	require.NoError(t, err)
	return catalogRawCostRule(channelID, modelName, version, status, mode, string(configJSON), source)
}

func catalogRawCostRule(channelID int, modelName string, version int, status types.CostRuleStatus, mode types.CostMode, configJSON, source string) model.ChannelModelCostRule {
	return model.ChannelModelCostRule{
		ChannelID: channelID, BillableUpstreamModel: modelName, CostVariantKey: string(types.DefaultCostVariantKey),
		Version: version, Status: string(status), CostMode: string(mode), SchemaVersion: 1,
		ConfigJSON: configJSON, Source: source, Note: "catalog note",
	}
}

func validCatalogCostConfig(currency string) types.CostRuleConfigV1 {
	return types.CostRuleConfigV1{
		Currency: currency, BillingMultiplier: "1", PurchaseDiscountRatio: "1",
		RechargeExchangeRatio: "1", FeeRate: "0", CurrencyToUSDRate: "1",
	}
}

func catalogStringPointer(value string) *string {
	return &value
}

func findCatalogItem(t *testing.T, items []dto.CostCatalogItem, modelName string) dto.CostCatalogItem {
	t.Helper()
	for _, item := range items {
		if item.BillableUpstreamModel == modelName {
			return item
		}
	}
	require.FailNow(t, "catalog item not found", modelName)
	return dto.CostCatalogItem{}
}
