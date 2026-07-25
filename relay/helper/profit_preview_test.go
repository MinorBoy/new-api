package helper

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
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

func costPreviewRequest(model, group string) (r dto.CostPreviewRequest) {
	r.OriginModel = model
	r.UserGroup = group
	r.RelayMode = relayconstant.RelayModeVideoSubmit
	return r
}
