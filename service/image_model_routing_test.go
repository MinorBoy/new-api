package service_test

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	relaytypes "github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/image_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnifiedImageRoutingSelectsLowestCostAndPublishesIdentity(t *testing.T) {
	prepareStrictCostRoutingServiceTest(t)
	allowImageCostCoverageForTest(t)
	originalCatalog := image_setting.Catalog2JSONString()
	originalRouting := image_setting.Routing2JSONString()
	t.Cleanup(func() {
		require.NoError(t, image_setting.UpdateCatalogByJSONString(originalCatalog))
		require.NoError(t, image_setting.UpdateRoutingByJSONString(originalRouting))
	})
	require.NoError(t, image_setting.UpdateCatalogByJSONString(testImageRoutingCatalog))
	require.NoError(t, image_setting.UpdateRoutingByJSONString(`{"version":1,"default":{"strategy":"lowest_cost"}}`))
	originalGroupRatio := ratio_setting.GroupRatio2JSONString()
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"分组A":1}`))
	t.Cleanup(func() { require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalGroupRatio)) })

	seedUnifiedImageChannel(t, 11, "cheap", "cheap-image")
	seedUnifiedImageChannel(t, 12, "expensive", "expensive-image")
	seedImageCostRule(t, 11, "cheap-image", "0.02")
	seedImageCostRule(t, 12, "expensive-image", "0.01")

	request := &dto.ImageRequest{Model: "gpt-image-1", Prompt: "a lighthouse", N: imageCountPointer(2)}
	imageContext, err := service.ResolveImageRequest(request, relayconstant.RelayModeImagesGenerations)
	require.NoError(t, err)
	c := capabilitySelectionContext()
	param := &service.RetryParam{
		Ctx: c, TokenGroup: "分组A", ModelName: request.Model,
		RequestPath: "/v1/images/generations", Retry: common.GetPointer(0), ImageRequest: &imageContext,
	}
	selected, group, err := service.CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, selected)
	assert.Equal(t, "分组A", group)
	assert.Equal(t, 12, selected.Id)
	assert.Equal(t, "expensive-image", common.GetContextKeyString(c, constant.ContextKeyRoutingUpstreamModel))
	assert.Equal(t, "gen-1024x1024-medium", common.GetContextKeyString(c, constant.ContextKeyRoutingCostVariant))

	covered, coverageErr := service.CheckSelectedChannelCostCoverage(param, selected, "")
	require.NoError(t, coverageErr)
	assert.True(t, covered, "image coverage must use the selected SKU and mapped upstream model")
}

func TestUnifiedImageRoutingCostWeightedKeepsOnlyTolerancePool(t *testing.T) {
	prepareStrictCostRoutingServiceTest(t)
	allowImageCostCoverageForTest(t)
	originalCatalog := image_setting.Catalog2JSONString()
	originalRouting := image_setting.Routing2JSONString()
	t.Cleanup(func() {
		require.NoError(t, image_setting.UpdateCatalogByJSONString(originalCatalog))
		require.NoError(t, image_setting.UpdateRoutingByJSONString(originalRouting))
	})
	require.NoError(t, image_setting.UpdateCatalogByJSONString(testImageRoutingCatalog))
	require.NoError(t, image_setting.UpdateRoutingByJSONString(`{"version":1,"default":{"strategy":"cost_weighted","cost_tolerance_bps":1000}}`))
	originalGroupRatio := ratio_setting.GroupRatio2JSONString()
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"分组A":1}`))
	t.Cleanup(func() { require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalGroupRatio)) })

	seedUnifiedImageChannel(t, 31, "weighted-cheap", "weighted-cheap-image")
	seedUnifiedImageChannel(t, 32, "weighted-close", "weighted-close-image")
	seedUnifiedImageChannel(t, 33, "weighted-far", "weighted-far-image")
	seedImageCostRule(t, 31, "weighted-cheap-image", "0.01")
	seedImageCostRule(t, 32, "weighted-close-image", "0.0105")
	seedImageCostRule(t, 33, "weighted-far-image", "0.02")

	request := &dto.ImageRequest{Model: "gpt-image-1", Prompt: "a lighthouse", N: imageCountPointer(1)}
	imageContext, err := service.ResolveImageRequest(request, relayconstant.RelayModeImagesGenerations)
	require.NoError(t, err)
	c := capabilitySelectionContext()
	param := &service.RetryParam{
		Ctx: c, TokenGroup: "分组A", ModelName: request.Model,
		RequestPath: "/v1/images/generations", Retry: common.GetPointer(0), ImageRequest: &imageContext,
	}
	selected, _, err := service.CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, selected)
	assert.NotEqual(t, 33, selected.Id, "candidate outside cost tolerance must not be selected")
}

func TestValidateKnownImageChannelRequiresProfileAndPublishesIdentity(t *testing.T) {
	prepareStrictCostRoutingServiceTest(t)
	allowImageCostCoverageForTest(t)
	originalCatalog := image_setting.Catalog2JSONString()
	originalRouting := image_setting.Routing2JSONString()
	t.Cleanup(func() {
		require.NoError(t, image_setting.UpdateCatalogByJSONString(originalCatalog))
		require.NoError(t, image_setting.UpdateRoutingByJSONString(originalRouting))
	})
	require.NoError(t, image_setting.UpdateCatalogByJSONString(testImageRoutingCatalog))
	require.NoError(t, image_setting.UpdateRoutingByJSONString(`{"version":1,"default":{"strategy":"manual"}}`))
	originalGroupRatio := ratio_setting.GroupRatio2JSONString()
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"分组A":1}`))
	t.Cleanup(func() { require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalGroupRatio)) })
	seedUnifiedImageChannel(t, 21, "profiled", "profiled-image")
	seedImageCostRule(t, 21, "profiled-image", "0.01")

	request := &dto.ImageRequest{Model: "gpt-image-1", Prompt: "a lighthouse", N: imageCountPointer(1)}
	imageContext, err := service.ResolveImageRequest(request, relayconstant.RelayModeImagesGenerations)
	require.NoError(t, err)
	c := capabilitySelectionContext()
	param := &service.RetryParam{
		Ctx: c, TokenGroup: "分组A", ModelName: request.Model,
		RequestPath: "/v1/images/generations", Retry: common.GetPointer(0), ImageRequest: &imageContext,
	}

	compatible, err := service.ValidateKnownChannelForRouting(param, "分组A", 21)
	require.NoError(t, err)
	assert.True(t, compatible)
	assert.Equal(t, "profiled-image", common.GetContextKeyString(c, constant.ContextKeyRoutingUpstreamModel))
	assert.Equal(t, "gen-1024x1024-medium", common.GetContextKeyString(c, constant.ContextKeyRoutingCostVariant))

	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", 21).Update("settings", "{}").Error)
	compatible, err = service.ValidateKnownChannelForRouting(param, "分组A", 21)
	assert.False(t, compatible)
	var selectionErr *service.ChannelSelectionError
	require.ErrorAs(t, err, &selectionErr)
	assert.Equal(t, relaytypes.ErrorCodeCompatibleChannelUnavailable, selectionErr.Code)
	assert.Equal(t, http.StatusServiceUnavailable, selectionErr.StatusCode)
}

func allowImageCostCoverageForTest(t *testing.T) {
	t.Helper()
	previous := service.CostCapabilityLookup
	service.CostCapabilityLookup = func(int, string, constant.TaskPlatform) types.CostCapabilities {
		return types.CostCapabilities{
			CanResolveBillableModel: true,
			ChargeEvents:            []types.CostChargeEvent{types.CostChargeResponseSucceeded},
			MeterSources:            []types.CostMeterSource{types.CostMeterValidatedRequest, types.CostMeterUpstreamActual},
		}
	}
	t.Cleanup(func() { service.CostCapabilityLookup = previous })
}

func seedUnifiedImageChannel(t *testing.T, channelID int, name, upstreamModel string) {
	t.Helper()
	mapping := `{"gpt-image-1":"` + upstreamModel + `"}`
	settings := `{"image_profile":{"profile":"openai_images","profile_version":1}}`
	priority := int64(100)
	weight := uint(10)
	modelMapping := mapping
	otherSettings := settings
	require.NoError(t, model.DB.Create(&model.Channel{
		Id: channelID, Type: constant.ChannelTypeOpenAI, Name: name, Key: "secret",
		Status: common.ChannelStatusEnabled, Priority: &priority, Weight: &weight,
		Models: "gpt-image-1", ModelMapping: &modelMapping, OtherSettings: otherSettings,
	}).Error)
	require.NoError(t, model.DB.Create(&model.Ability{
		Group: "分组A", Model: "gpt-image-1", ChannelId: channelID, Enabled: true,
		Priority: &priority, Weight: weight,
	}).Error)
}

func seedImageCostRule(t *testing.T, channelID int, modelName, unitPrice string) {
	t.Helper()
	config := types.CostRuleConfigV1{
		Currency:              "USD",
		BillingMultiplier:     "1",
		PurchaseDiscountRatio: "1",
		RechargeExchangeRatio: "1",
		FeeRate:               "0",
		CurrencyToUSDRate:     "1",
		UnitPrice:             &unitPrice,
		ChargeEvent:           types.CostChargeResponseSucceeded,
		MeterSource:           types.CostMeterValidatedRequest,
	}
	normalized, err := service.NormalizeCostRuleConfig(types.CostModePerImage, config)
	require.NoError(t, err)
	configJSON, err := common.Marshal(normalized)
	require.NoError(t, err)
	now := common.GetTimestamp()
	require.NoError(t, model.DB.Create(&model.ChannelModelCostRule{
		ChannelID: channelID, BillableUpstreamModel: modelName,
		CostVariantKey: "gen-1024x1024-medium", Version: 1,
		Status: string(types.CostRuleActive), CostMode: string(types.CostModePerImage), SchemaVersion: 1,
		ConfigJSON: string(configJSON), Source: "manual", EffectiveFrom: &now,
		CreatedAt: now, UpdatedAt: now,
	}).Error)
}

func imageCountPointer(value uint) *uint {
	return &value
}

const testImageRoutingCatalog = `{"version":1,"models":{"gpt-image-1":{"profile":"openai_images","profile_version":1,"endpoints":{"generations":{"capability":{"enabled":true,"sizes":["1024x1024"],"qualities":["medium"],"response_formats":["b64_json"],"max_n":4},"default_size":"1024x1024","default_quality":"medium","default_response_format":"b64_json"}},"skus":{"gen-1024x1024-medium":{"endpoint":"generations","size":"1024x1024","quality":"medium","unit":"image","sale_price_usd":"0.04"}}}}}`
