package helper

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/image_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImageSKUPriceHelperFreezesSnapshotAndPreconsumeQuota(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/images/generations", nil)
	info := &relaycommon.RelayInfo{UsingGroup: "default", UserGroup: "default"}
	oldQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 500000
	t.Cleanup(func() { common.QuotaPerUnit = oldQuotaPerUnit })

	sku := image_setting.ResolvedSKU{
		CatalogVersion: 1, Model: "gpt-image-1", SKUKey: "gen-1024x1024-medium",
		Endpoint: "generations", Size: "1024x1024", Quality: "medium",
		ResponseFormat: "b64_json", SalePriceUSD: "0.125", N: 3,
	}

	priceData, err := ImageSKUPriceHelper(c, info, sku)
	require.NoError(t, err)
	assert.Equal(t, 187500, priceData.QuotaToPreConsume)
	require.NotNil(t, info.ImageBillingSnapshot)
	assert.Equal(t, int64(3), info.ImageBillingSnapshot.RequestedImages)
	assert.Equal(t, "0.125", info.ImageBillingSnapshot.UnitSalePriceUSD)
	assert.Equal(t, "1", info.ImageBillingSnapshot.GroupRatio)
}
