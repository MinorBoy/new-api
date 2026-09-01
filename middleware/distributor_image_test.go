package middleware

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	relayhelper "github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/cost_setting"
	"github.com/QuantumNous/new-api/setting/image_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const distributorImageCatalog = `{"version":1,"models":{"gpt-image-1":{"profile":"openai_images","profile_version":1,"endpoints":{"generations":{"capability":{"enabled":true,"sizes":["1024x1024"],"qualities":["medium"],"response_formats":["b64_json"],"max_n":4},"default_size":"1024x1024","default_quality":"medium","default_response_format":"b64_json"},"edits":{"capability":{"enabled":true,"sizes":["1024x1024"],"qualities":["medium"],"response_formats":["b64_json"],"max_n":2,"max_input_images":2,"supports_mask":true},"default_size":"1024x1024","default_quality":"medium","default_response_format":"b64_json"}},"skus":{"gen-1024x1024-medium":{"endpoint":"generations","size":"1024x1024","quality":"medium","unit":"image","sale_price_usd":"0.040000"},"edit-1024x1024-medium":{"endpoint":"edits","size":"1024x1024","quality":"medium","unit":"image","sale_price_usd":"0.060000"}}}}}`

func prepareDistributorImageCatalog(t *testing.T) {
	t.Helper()
	previousCatalog := image_setting.Catalog2JSONString()
	previousRouting := image_setting.Routing2JSONString()
	require.NoError(t, image_setting.UpdateCatalogByJSONString(distributorImageCatalog))
	require.NoError(t, image_setting.UpdateRoutingByJSONString(`{"version":1,"default":{"strategy":"manual"}}`))
	t.Cleanup(func() {
		require.NoError(t, image_setting.UpdateCatalogByJSONString(previousCatalog))
		require.NoError(t, image_setting.UpdateRoutingByJSONString(previousRouting))
	})
}

func TestExtractUnifiedImageRequestContextSupportsSeedancePath(t *testing.T) {
	prepareDistributorImageCatalog(t)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v3/images/generations", bytes.NewBufferString(`{"model":"gpt-image-1","prompt":"x","quality":"medium","n":1}`))
	c.Request.Header.Set("Content-Type", "application/json")
	t.Cleanup(func() { common.CleanupBodyStorage(c) })

	context, err := extractUnifiedImageRequestContext(c, "gpt-image-1")
	require.NoError(t, err)
	require.NotNil(t, context)
	assert.Equal(t, "gen-1024x1024-medium", context.Resolved.SKUKey)

	request, err := relayhelper.GetAndValidOpenAIImageRequest(c, relayconstant.RelayModeImagesGenerations)
	require.NoError(t, err)
	assert.Equal(t, "gpt-image-1", request.Model)
}

func TestExtractUnifiedImageRequestContextUsesCatalogQualityWhenOmitted(t *testing.T) {
	prepareDistributorImageCatalog(t)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewBufferString(`{"model":"gpt-image-1","prompt":"x","n":1}`))
	c.Request.Header.Set("Content-Type", "application/json")
	t.Cleanup(func() { common.CleanupBodyStorage(c) })

	context, err := extractUnifiedImageRequestContext(c, "gpt-image-1")
	require.NoError(t, err)
	require.NotNil(t, context)
	assert.Equal(t, "medium", context.Resolved.Quality)
	assert.Equal(t, "gen-1024x1024-medium", context.Resolved.SKUKey)
}

func TestExtractUnifiedImageRequestContextResetsMultipartEditBody(t *testing.T) {
	prepareDistributorImageCatalog(t)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("model", "gpt-image-1"))
	require.NoError(t, writer.WriteField("prompt", "edit this"))
	require.NoError(t, writer.WriteField("quality", "medium"))
	require.NoError(t, writer.WriteField("n", "1"))
	part, err := writer.CreateFormFile("image", "input.png")
	require.NoError(t, err)
	_, err = part.Write([]byte("not-an-image"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", &body)
	c.Request.Header.Set("Content-Type", writer.FormDataContentType())
	t.Cleanup(func() { common.CleanupBodyStorage(c) })

	context, err := extractUnifiedImageRequestContext(c, "gpt-image-1")
	require.NoError(t, err)
	require.NotNil(t, context)
	assert.Equal(t, "edit-1024x1024-medium", context.Resolved.SKUKey)

	request, err := relayhelper.GetAndValidOpenAIImageRequest(c, relayconstant.RelayModeImagesEdits)
	require.NoError(t, err)
	assert.Equal(t, "gpt-image-1", request.Model)
	assert.Equal(t, uint(1), request.InputImageCount)
}

func TestExtractUnifiedImageRequestContextLeavesLegacyTextEditsUntouched(t *testing.T) {
	prepareDistributorImageCatalog(t)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/edits", bytes.NewBufferString(`{"model":"gpt-image-1","prompt":"edit"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	t.Cleanup(func() { common.CleanupBodyStorage(c) })

	context, err := extractUnifiedImageRequestContext(c, "gpt-image-1")
	require.NoError(t, err)
	assert.Nil(t, context)
	assert.Equal(t, relayconstant.RelayModeEdits, relayconstant.Path2RelayMode("/v1/edits"))
}

func TestDistributeImageRoutingResolvesBeforeChannelSelection(t *testing.T) {
	prepareDistributorImageRoutingTest(t)
	seedDistributorImageChannel(t, 11, "cheap", "cheap-image")
	seedDistributorImageChannel(t, 12, "expensive", "expensive-image")
	seedDistributorImageCostRule(t, 11, "cheap-image", "0.020000")
	seedDistributorImageCostRule(t, 12, "expensive-image", "0.010000")

	var (
		selectedID int
		upstream   string
		variant    string
		parsed     *dto.ImageRequest
		parseErr   error
	)
	router := gin.New()
	router.Use(BodyStorageCleanup())
	router.Use(SeedanceRequestConvert())
	router.Use(func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
		common.SetContextKey(c, constant.ContextKeyTokenGroup, "default")
		common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
		c.Next()
	})
	router.Use(Distribute())
	router.POST("/api/v3/images/generations", func(c *gin.Context) {
		selectedID = common.GetContextKeyInt(c, constant.ContextKeyChannelId)
		upstream = common.GetContextKeyString(c, constant.ContextKeyRoutingUpstreamModel)
		variant = common.GetContextKeyString(c, constant.ContextKeyRoutingCostVariant)
		parsed, parseErr = relayhelper.GetAndValidOpenAIImageRequest(c, relayconstant.RelayModeImagesGenerations)
		c.Status(http.StatusOK)
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v3/images/generations", bytes.NewBufferString(`{"model":"gpt-image-1","prompt":"x","quality":"medium","n":2}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, 12, selectedID)
	assert.Equal(t, "expensive-image", upstream)
	assert.Equal(t, "gen-1024x1024-medium", variant)
	require.NoError(t, parseErr)
	require.NotNil(t, parsed)
	assert.Equal(t, "gpt-image-1", parsed.Model)
	assert.Equal(t, uint(2), *parsed.N)
}

func prepareDistributorImageRoutingTest(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.ChannelModelCostRule{}))

	previousDB := model.DB
	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	previousLookup := service.CostCapabilityLookup
	model.DB = db
	common.MemoryCacheEnabled = false
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	service.CostCapabilityLookup = func(_ int, requestPath string, _ constant.TaskPlatform) hosttypes.CostCapabilities {
		if requestPath == "/v1/images/generations" || requestPath == "/v1/images/edits" {
			return hosttypes.CostCapabilities{
				CanResolveBillableModel: true,
				ChargeEvents:            []hosttypes.CostChargeEvent{hosttypes.CostChargeResponseSucceeded},
				MeterSources:            []hosttypes.CostMeterSource{hosttypes.CostMeterValidatedRequest, hosttypes.CostMeterUpstreamActual},
			}
		}
		return hosttypes.CostCapabilities{CanResolveBillableModel: true}
	}
	service.InvalidateCostCoverage(0, "", "")

	costConfig := config.GlobalConfig.Get(cost_setting.ConfigName)
	require.NotNil(t, costConfig)
	previousMode := cost_setting.Runtime().Mode
	require.NoError(t, config.UpdateConfigFromMap(costConfig, map[string]string{cost_setting.KeyMode: string(hosttypes.CostAccountingStrict)}))
	cost_setting.UpdateAndSync()
	previousGroupRatio := ratio_setting.GroupRatio2JSONString()
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))
	previousCatalog := image_setting.Catalog2JSONString()
	previousRouting := image_setting.Routing2JSONString()
	require.NoError(t, image_setting.UpdateCatalogByJSONString(distributorImageCatalog))
	require.NoError(t, image_setting.UpdateRoutingByJSONString(`{"version":1,"default":{"strategy":"lowest_cost"}}`))

	t.Cleanup(func() {
		require.NoError(t, config.UpdateConfigFromMap(costConfig, map[string]string{cost_setting.KeyMode: string(previousMode)}))
		cost_setting.UpdateAndSync()
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(previousGroupRatio))
		require.NoError(t, image_setting.UpdateCatalogByJSONString(previousCatalog))
		require.NoError(t, image_setting.UpdateRoutingByJSONString(previousRouting))
		service.InvalidateCostCoverage(0, "", "")
		service.CostCapabilityLookup = previousLookup
		common.MemoryCacheEnabled = previousMemoryCacheEnabled
		model.DB = previousDB
		require.NoError(t, sqlDB.Close())
	})
}

func seedDistributorImageChannel(t *testing.T, channelID int, name, upstreamModel string) {
	t.Helper()
	priority := int64(100)
	weight := uint(10)
	mapping := `{"gpt-image-1":"` + upstreamModel + `"}`
	settings := `{"image_profile":{"profile":"openai_images","profile_version":1}}`
	require.NoError(t, model.DB.Create(&model.Channel{
		Id: channelID, Type: constant.ChannelTypeOpenAI, Name: name, Key: "secret",
		Status: common.ChannelStatusEnabled, Priority: &priority, Weight: &weight,
		Models: "gpt-image-1", ModelMapping: &mapping, OtherSettings: settings,
	}).Error)
	require.NoError(t, model.DB.Create(&model.Ability{
		Group: "default", Model: "gpt-image-1", ChannelId: channelID, Enabled: true,
		Priority: &priority, Weight: weight,
	}).Error)
}

func seedDistributorImageCostRule(t *testing.T, channelID int, upstreamModel, unitPrice string) {
	t.Helper()
	config := hosttypes.CostRuleConfigV1{
		Currency:              "USD",
		BillingMultiplier:     "1",
		PurchaseDiscountRatio: "1",
		RechargeExchangeRatio: "1",
		FeeRate:               "0",
		CurrencyToUSDRate:     "1",
		UnitPrice:             &unitPrice,
		ChargeEvent:           hosttypes.CostChargeResponseSucceeded,
		MeterSource:           hosttypes.CostMeterValidatedRequest,
	}
	normalized, err := service.NormalizeCostRuleConfig(hosttypes.CostModePerImage, config)
	require.NoError(t, err)
	configJSON, err := common.Marshal(normalized)
	require.NoError(t, err)
	now := common.GetTimestamp()
	require.NoError(t, model.DB.Create(&model.ChannelModelCostRule{
		ChannelID: channelID, BillableUpstreamModel: upstreamModel,
		CostVariantKey: "gen-1024x1024-medium", Version: 1,
		Status: string(hosttypes.CostRuleActive), CostMode: string(hosttypes.CostModePerImage), SchemaVersion: 1,
		ConfigJSON: string(configJSON), Source: "test", EffectiveFrom: &now,
		CreatedAt: now, UpdatedAt: now,
	}).Error)
}
