package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/imageprofile"
	"github.com/QuantumNous/new-api/setting/image_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestMigrateImageCatalogMapsLegacySKUsToResolutionTiers(t *testing.T) {
	input := image_setting.Catalog{
		Version: 1,
		Models: map[string]image_setting.ModelEntry{
			"gpt-image-2": {
				Profile: imageprofile.OpenAIImagesProfile, ProfileVersion: imageprofile.OpenAIImagesVersion,
				Endpoints: map[imageprofile.Endpoint]image_setting.EndpointCatalog{
					imageprofile.EndpointGenerations: {
						Capability:  imageprofile.Capability{Enabled: true, Sizes: []string{"1024x1024", "2048x2048", "2880x2880"}, Qualities: []string{"medium"}, ResponseFormats: []string{"b64_json"}, MaxN: 1},
						DefaultSize: "1024x1024", DefaultQuality: "medium", DefaultResponseFormat: "b64_json",
					},
				},
				SKUs: map[string]image_setting.SKU{
					"gen-1024x1024-medium": {Endpoint: imageprofile.EndpointGenerations, Size: "1024x1024", Quality: "medium", Unit: "image", SalePriceUSD: "0.02"},
					"gen-2048x2048-medium": {Endpoint: imageprofile.EndpointGenerations, Size: "2048x2048", Quality: "medium", Unit: "image", SalePriceUSD: "0.04"},
					"gen-2880x2880-medium": {Endpoint: imageprofile.EndpointGenerations, Size: "2880x2880", Quality: "medium", Unit: "image", SalePriceUSD: "0.08"},
				},
			},
		},
	}

	result, err := MigrateImageCatalog(input)
	require.NoError(t, err)
	model := result.Catalog.Models["gpt-image-2"]
	assert.Len(t, model.SKUs, 3)
	assert.Equal(t, "0.04", model.SKUs["gen-2k-medium"].SalePriceUSD)
	assert.Equal(t, "2k", model.SKUs["gen-2k-medium"].Tier)
	assert.Empty(t, result.Conflicts)
}

func TestMigrateImageCatalogMapsHistorical4096SKUToFourK(t *testing.T) {
	input := image_setting.Catalog{Version: 1, Models: map[string]image_setting.ModelEntry{
		"gpt-image-2": {
			Profile: imageprofile.OpenAIImagesProfile, ProfileVersion: imageprofile.OpenAIImagesVersion,
			Endpoints: map[imageprofile.Endpoint]image_setting.EndpointCatalog{imageprofile.EndpointGenerations: {
				Capability:  imageprofile.Capability{Enabled: true, Sizes: []string{"1024x1024", "4096x4096"}, Qualities: []string{"medium"}, ResponseFormats: []string{"b64_json"}, MaxN: 1},
				DefaultSize: "1024x1024", DefaultQuality: "medium", DefaultResponseFormat: "b64_json",
			}},
			SKUs: map[string]image_setting.SKU{
				"gen-1024x1024-medium": {Endpoint: imageprofile.EndpointGenerations, Size: "1024x1024", Quality: "medium", Unit: "image", SalePriceUSD: "0.02"},
				"gen-4096x4096-medium": {Endpoint: imageprofile.EndpointGenerations, Size: "4096x4096", Quality: "medium", Unit: "image", SalePriceUSD: "0.08"},
			},
		},
	}}
	result, err := MigrateImageCatalog(input)
	require.NoError(t, err)
	assert.Empty(t, result.Errors)
	assert.Equal(t, "0.08", result.Catalog.Models["gpt-image-2"].SKUs["gen-4k-medium"].SalePriceUSD)
	assert.Equal(t, "4k", result.Catalog.Models["gpt-image-2"].SKUs["gen-4k-medium"].Tier)
	assert.Empty(t, result.Catalog.Models["gpt-image-2"].SKUs["gen-4k-medium"].Size)
}

func TestMigrateImageCatalogAtStartupCopiesCostsAndIsIdempotent(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	previousDB := model.DB
	previousMainType := common.MainDatabaseType()
	previousLogType := common.LogDatabaseType()
	previousMap := common.OptionMap
	model.DB = db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, previousLogType)
	common.OptionMapRWMutex.Lock()
	common.OptionMap = map[string]string{}
	common.OptionMapRWMutex.Unlock()
	require.NoError(t, db.AutoMigrate(&model.Option{}, &model.ChannelModelCostRule{}, &model.Channel{}))
	// Seeding is channel-driven: an enabled OpenAI Images channel advertises
	// the 2.5 siblings that the catalog lacks.
	require.NoError(t, db.Create(&model.Channel{
		Id: 41, Name: "vendor-image", Status: common.ChannelStatusEnabled, Type: 1,
		Models: "gpt-image-2,gpt-image-2.5-flare,gpt-image-2.5-sunburst",
		OtherSettings: `{"image_profile":{"profile":"openai_images","profile_version":1}}`,
	}).Error)
	t.Cleanup(func() {
		model.DB = previousDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousMap
		common.OptionMapRWMutex.Unlock()
		require.NoError(t, sqlDB.Close())
	})

	originalCatalog := image_setting.Catalog2JSONString()
	t.Cleanup(func() { require.NoError(t, image_setting.UpdateCatalogByJSONString(originalCatalog)) })
	legacyCatalog := `{"version":1,"models":{"gpt-image-2":{"profile":"openai_images","profile_version":1,"endpoints":{"generations":{"capability":{"enabled":true,"sizes":["1024x1024","4096x4096"],"qualities":["medium"],"response_formats":["b64_json"],"max_n":1},"default_size":"1024x1024","default_quality":"medium","default_response_format":"b64_json"}},"skus":{"gen-1024x1024-medium":{"endpoint":"generations","size":"1024x1024","quality":"medium","unit":"image","sale_price_usd":"0.03"},"gen-4096x4096-medium":{"endpoint":"generations","size":"4096x4096","quality":"medium","unit":"image","sale_price_usd":"0.08"}}}}}`
	require.NoError(t, image_setting.UpdateCatalogByJSONString(legacyCatalog))
	seedLegacyImageCostRule(t, 41, "vendor-image", "gen-1024x1024-medium", "0.008")
	seedLegacyImageCostRule(t, 41, "vendor-image", "gen-4096x4096-medium", "0.012")

	result, err := MigrateImageCatalogAtStartup()
	require.NoError(t, err)
	assert.True(t, result.CatalogChanged)
	assert.Equal(t, 2, result.CostRulesCreated)
	snapshot := image_setting.Snapshot()
	assert.Contains(t, snapshot.Models["gpt-image-2"].SKUs, "gen-1k-medium")
	assert.Contains(t, snapshot.Models["gpt-image-2"].SKUs, "gen-4k-medium")
	assert.Empty(t, snapshot.Models["gpt-image-2"].SKUs["gen-4k-medium"].Size)
	for _, modelName := range []string{"gpt-image-2.5-flare", "gpt-image-2.5-sunburst"} {
		modelEntry, ok := snapshot.Models[modelName]
		require.True(t, ok)
		assert.Equal(t, imageprofile.OpenAIImagesProfile, modelEntry.Profile)
		// Clones mirror generations into edits, so each tier SKU doubles.
		assert.Len(t, modelEntry.SKUs, 4)
		assert.Equal(t, "0.08", modelEntry.SKUs["gen-4k-medium"].SalePriceUSD)
		assert.Equal(t, "0.08", modelEntry.SKUs["edit-4k-medium"].SalePriceUSD)
		edits, ok := modelEntry.Endpoints[imageprofile.EndpointEdits]
		require.True(t, ok)
		assert.True(t, edits.Capability.Enabled)
		assert.GreaterOrEqual(t, edits.Capability.MaxInputImages, uint(1))
	}
	var migratedRules []model.ChannelModelCostRule
	require.NoError(t, db.Where("channel_id = ? AND billable_upstream_model = ? AND cost_variant_key IN ?", 41, "vendor-image", []string{"gen-1k-medium", "gen-4k-medium"}).Find(&migratedRules).Error)
	assert.Len(t, migratedRules, 2)

	second, err := MigrateImageCatalogAtStartup()
	require.NoError(t, err)
	assert.False(t, second.CatalogChanged)
	assert.Zero(t, second.CostRulesCreated)
}

func seedLegacyImageCostRule(t *testing.T, channelID int, upstreamModel, variant, unitPrice string) {
	t.Helper()
	config := types.CostRuleConfigV1{
		Currency: "USD", BillingMultiplier: "1", PurchaseDiscountRatio: "1",
		RechargeExchangeRatio: "1", FeeRate: "0", CurrencyToUSDRate: "1",
		UnitPrice: &unitPrice, ChargeEvent: types.CostChargeResponseSucceeded,
		MeterSource: types.CostMeterValidatedRequest,
	}
	normalized, err := NormalizeCostRuleConfig(types.CostModePerImage, config)
	require.NoError(t, err)
	configJSON, err := common.Marshal(normalized)
	require.NoError(t, err)
	now := common.GetTimestamp()
	require.NoError(t, model.DB.Create(&model.ChannelModelCostRule{
		ChannelID: channelID, BillableUpstreamModel: upstreamModel, CostVariantKey: variant,
		Version: 1, Status: string(types.CostRuleActive), CostMode: string(types.CostModePerImage), SchemaVersion: 1,
		ConfigJSON: string(configJSON), Source: "manual", EffectiveFrom: &now,
		CreatedAt: now, UpdatedAt: now,
	}).Error)
}

func TestMigrateImageCatalogReportsConflictingLegacyCosts(t *testing.T) {
	input := image_setting.Catalog{Version: 1, Models: map[string]image_setting.ModelEntry{
		"gpt-image-2": {
			Profile: imageprofile.OpenAIImagesProfile, ProfileVersion: imageprofile.OpenAIImagesVersion,
			Endpoints: map[imageprofile.Endpoint]image_setting.EndpointCatalog{imageprofile.EndpointGenerations: {
				Capability: imageprofile.Capability{Enabled: true, Qualities: []string{"medium"}, ResponseFormats: []string{"b64_json"}, MaxN: 1}, DefaultSize: "auto", DefaultQuality: "medium", DefaultResponseFormat: "b64_json",
			}},
			SKUs: map[string]image_setting.SKU{"gen-1k-medium": {Endpoint: imageprofile.EndpointGenerations, Tier: "1k", Quality: "medium", Unit: "image", SalePriceUSD: "0.02"}},
		},
	}}
	result, err := MigrateImageCatalog(input)
	require.NoError(t, err)
	assert.Equal(t, "0.02", result.Catalog.Models["gpt-image-2"].SKUs["gen-1k-medium"].SalePriceUSD)
}

func TestMigrateImageCatalogMapsCostRulesAndBlocksConflicts(t *testing.T) {
	input := image_setting.Catalog{Version: 1, Models: map[string]image_setting.ModelEntry{"gpt-image-2": {
		Profile: imageprofile.OpenAIImagesProfile, ProfileVersion: imageprofile.OpenAIImagesVersion,
		Endpoints: map[imageprofile.Endpoint]image_setting.EndpointCatalog{imageprofile.EndpointGenerations: {Capability: imageprofile.Capability{Enabled: true, Qualities: []string{"medium"}, ResponseFormats: []string{"b64_json"}, MaxN: 1}, DefaultSize: "auto", DefaultQuality: "medium", DefaultResponseFormat: "b64_json"}},
		SKUs:      map[string]image_setting.SKU{"gen-1k-medium": {Endpoint: imageprofile.EndpointGenerations, Tier: "1k", Quality: "medium", Unit: "image", SalePriceUSD: "0.02"}},
	}}}
	result, err := MigrateImageCatalogWithCosts(input, []ImageCatalogLegacyCost{
		{ChannelID: 41, UpstreamModel: "vendor", Endpoint: imageprofile.EndpointGenerations, SKUKey: "gen-1024x1024-medium", CostUSD: "0.008"},
		{ChannelID: 41, UpstreamModel: "vendor", Endpoint: imageprofile.EndpointGenerations, SKUKey: "gen-512x2048-medium", CostUSD: "0.012"},
	})
	require.NoError(t, err)
	assert.Empty(t, result.MappedCosts)
	require.Len(t, result.Conflicts, 1)
	assert.Equal(t, "gen-1k-medium", result.Conflicts[0].TargetSKU)
}
