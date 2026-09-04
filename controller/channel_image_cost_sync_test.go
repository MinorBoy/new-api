package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/image_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateChannelSynchronizesImageCostRules(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}, &model.RoutingPolicy{}, &model.RouteTarget{}))
	originalCatalog := image_setting.Catalog2JSONString()
	t.Cleanup(func() { require.NoError(t, image_setting.UpdateCatalogByJSONString(originalCatalog)) })
	require.NoError(t, image_setting.UpdateCatalogByJSONString(controllerImageCostSyncCatalogFixture))

	channel := &model.Channel{
		Type:          constant.ChannelTypeOpenAI,
		Name:          "image sync channel",
		Key:           "secret",
		Models:        "gpt-image-2",
		Group:         "default",
		Status:        common.ChannelStatusEnabled,
		OtherSettings: `{"image_profile":{"profile":"openai_images","profile_version":1,"capability_overrides":{"gpt-image-2":{"resolution_qualities":["1k:low","2k:high"]}}}}`,
	}
	require.NoError(t, db.Create(channel).Error)
	active := seedControllerImageCostRule(t, channel.Id, "gpt-image-2", "gen-1k-low", types.CostRuleActive, types.CostModePerImage)
	draft := seedControllerImageCostRule(t, channel.Id, "gpt-image-2", "gen-2k-high", types.CostRuleDraft, types.CostModePerImage)
	ordinary := seedControllerImageCostRule(t, channel.Id, "gpt-image-2", "request", types.CostRuleActive, types.CostModePerRequest)

	requestBody, err := common.Marshal(map[string]any{
		"id":       channel.Id,
		"settings": `{"image_profile":{"profile":"openai_images","profile_version":1,"capability_overrides":{"gpt-image-2":{"resolution_qualities":["1k:low"]}}}}`,
	})
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("role", common.RoleRootUser)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/channel/", bytes.NewReader(requestBody))
	ctx.Request.Header.Set("Content-Type", "application/json")
	UpdateChannel(ctx)

	var response struct {
		Success bool `json:"success"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success, recorder.Body.String())

	var got model.ChannelModelCostRule
	require.NoError(t, db.First(&got, active.ID).Error)
	assert.Equal(t, string(types.CostRuleActive), got.Status)
	got = model.ChannelModelCostRule{}
	require.NoError(t, db.First(&got, draft.ID).Error)
	assert.Equal(t, string(types.CostRuleRetired), got.Status)
	got = model.ChannelModelCostRule{}
	require.NoError(t, db.First(&got, ordinary.ID).Error)
	assert.Equal(t, string(types.CostRuleActive), got.Status)
}

func seedControllerImageCostRule(t *testing.T, channelID int, upstreamModel, variant string, status types.CostRuleStatus, mode types.CostMode) model.ChannelModelCostRule {
	t.Helper()
	price := "0.01"
	config := types.CostRuleConfigV1{
		Currency: "USD", BillingMultiplier: "1", PurchaseDiscountRatio: "1",
		RechargeExchangeRatio: "1", FeeRate: "0", CurrencyToUSDRate: "1",
		UnitPrice: &price, ChargeEvent: types.CostChargeResponseSucceeded,
		MeterSource: types.CostMeterValidatedRequest,
	}
	normalized, err := service.NormalizeCostRuleConfig(mode, config)
	require.NoError(t, err)
	encoded, err := common.Marshal(normalized)
	require.NoError(t, err)
	now := common.GetTimestamp()
	rule := model.ChannelModelCostRule{
		ChannelID: channelID, BillableUpstreamModel: upstreamModel, CostVariantKey: variant,
		Version: 1, Status: string(status), CostMode: string(mode), SchemaVersion: 1,
		ConfigJSON: string(encoded), Source: "manual", EffectiveFrom: &now,
		CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, model.DB.Create(&rule).Error)
	return rule
}

const controllerImageCostSyncCatalogFixture = `{"version":1,"models":{"gpt-image-2":{"profile":"openai_images","profile_version":1,"endpoints":{"generations":{"capability":{"enabled":true,"resolution_tiers":["1k","2k"],"qualities":["low","medium","high"],"response_formats":["b64_json"],"max_n":4},"default_size":"auto","default_quality":"medium","default_response_format":"b64_json"}},"skus":{"gen-1k-low":{"endpoint":"generations","tier":"1k","quality":"low","unit":"image","sale_price_usd":"0.02"},"gen-1k-medium":{"endpoint":"generations","tier":"1k","quality":"medium","unit":"image","sale_price_usd":"0.025"},"gen-2k-high":{"endpoint":"generations","tier":"2k","quality":"high","unit":"image","sale_price_usd":"0.05"}}}}}`
