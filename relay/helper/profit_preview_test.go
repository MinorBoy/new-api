package helper

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPreviewRoutingRevenueMatchesUserBillingChain asserts the routing revenue preview
// runs through the same pricing chain as PreviewUserBillingQuota. User revenue for
// profit routing must equal what the user would actually be billed, so the two paths
// share the helper and only differ in how the caller supplies the user id.
func TestPreviewRoutingRevenueMatchesUserBillingChain(t *testing.T) {
	gin.SetMode(gin.TestMode)
	savedPrices := ratio_setting.ModelPrice2JSONString()
	savedGroups := ratio_setting.GroupRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(savedPrices))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(savedGroups))
	})

	prices, err := common.Marshal(map[string]float64{"routing-fixed": 0.2})
	require.NoError(t, err)
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(string(prices)))
	groups, err := common.Marshal(map[string]float64{"routing-group": 2})
	require.NoError(t, err)
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(string(groups)))

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("id", 42)
	referenceQuota, referenceSnapshot, err := PreviewUserBillingQuota(ctx, costPreviewRequest("routing-fixed", "routing-group"))
	require.NoError(t, err)

	routingQuota, routingSnapshot, err := PreviewRoutingRevenue("routing-fixed", "routing-group", "/v1/video/generations", relayconstant.RelayModeVideoSubmit, nil, 42)
	require.NoError(t, err)

	assert.Equal(t, referenceQuota, routingQuota, "routing preview must match the user-billing preview")
	assert.Equal(t, referenceSnapshot, routingSnapshot)
	assert.Equal(t, "500000", routingSnapshot)
}

func TestPreviewRoutingRevenueAppliesExplicitGroupRatio(t *testing.T) {
	gin.SetMode(gin.TestMode)
	savedPrices := ratio_setting.ModelPrice2JSONString()
	savedGroups := ratio_setting.GroupRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(savedPrices))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(savedGroups))
	})

	prices, err := common.Marshal(map[string]float64{"routing-fixed": 0.2})
	require.NoError(t, err)
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(string(prices)))
	groups, err := common.Marshal(map[string]float64{"routing-group": 2})
	require.NoError(t, err)
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(string(groups)))

	override := 1.5
	quota, snapshot, err := PreviewRoutingRevenueWithSeedanceInputAndGroupRatio(
		"routing-fixed", "routing-group", "/v1/video/generations",
		relayconstant.RelayModeVideoSubmit, nil, 42, "", false, 0, &override,
	)

	require.NoError(t, err)
	assert.Equal(t, int64(150000), quota)
	assert.Equal(t, "500000", snapshot)
}

func TestPreviewRoutingRevenueRejectsNonPositiveGroupRatioOverride(t *testing.T) {
	gin.SetMode(gin.TestMode)
	savedPrices := ratio_setting.ModelPrice2JSONString()
	savedGroups := ratio_setting.GroupRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(savedPrices))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(savedGroups))
	})

	prices, err := common.Marshal(map[string]float64{"routing-fixed": 0.2})
	require.NoError(t, err)
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(string(prices)))
	groups, err := common.Marshal(map[string]float64{"routing-group": 2})
	require.NoError(t, err)
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(string(groups)))

	zero := 0.0
	_, _, err = PreviewRoutingRevenueWithSeedanceInputAndGroupRatio(
		"routing-fixed", "routing-group", "/v1/video/generations",
		relayconstant.RelayModeVideoSubmit, nil, 42, "", false, 0, &zero,
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "positive finite number")
}

func TestPreviewRoutingRevenueUsesOfficialSeedanceTokenFormula(t *testing.T) {
	common.OptionMapRWMutex.Lock()
	previousOptionMap := common.OptionMap
	common.OptionMap = make(map[string]string)
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptionMap
		common.OptionMapRWMutex.Unlock()
	})
	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		saved[key] = value
		return nil
	}))
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(saved))
	})
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode": `{"doubao-seedance-2-0-mini-260615":"seedance_tokens"}`,
		"billing_setting.seedance_token_price": `{"doubao-seedance-2-0-mini-260615":{"scenarios":{` +
			`"720p:no_video":{"price_per_million":"3.150684931506849","width":1280,"height":720,"frame_rate":24,"pricing_version":"official-token-v1","source":"SRC-OFFICIAL-SEEDANCE-2-0-MINI!17"},` +
			`"720p:with_video":{"price_per_million":"1.917808219178082","width":1280,"height":720,"frame_rate":24,"pricing_version":"official-token-v1","source":"SRC-OFFICIAL-SEEDANCE-2-0-MINI!18"}}}}`,
		"group_ratio_setting.group_ratio": `{"default":1}`,
	}))

	textQuota, _, err := PreviewRoutingRevenueWithSeedanceInput("doubao-seedance-2-0-mini-260615", "default", "/v1/video/generations", relayconstant.RelayModeVideoSubmit, intPointer(5), 42, "720p", false, 0)
	require.NoError(t, err)
	videoQuota, _, err := PreviewRoutingRevenueWithSeedanceInput("doubao-seedance-2-0-mini-260615", "default", "/v1/video/generations", relayconstant.RelayModeVideoSubmit, intPointer(5), 42, "720p", true, 5000)
	require.NoError(t, err)
	longVideoQuota, _, err := PreviewRoutingRevenueWithSeedanceInput("doubao-seedance-2-0-mini-260615", "default", "/v1/video/generations", relayconstant.RelayModeVideoSubmit, intPointer(5), 42, "720p", true, 15000)
	require.NoError(t, err)
	assert.Equal(t, int64(170137), textQuota)
	assert.Equal(t, int64(207123), videoQuota)
	assert.Equal(t, int64(414247), longVideoQuota)
}

func intPointer(value int) *int { return &value }

func costPreviewRequest(model, group string) (r dto.CostPreviewRequest) {
	r.OriginModel = model
	r.UserGroup = group
	r.RelayMode = relayconstant.RelayModeVideoSubmit
	return r
}
