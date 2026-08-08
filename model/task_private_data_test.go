package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskPrivateDataValueRetainsUserResponseAuditData(t *testing.T) {
	privateData := TaskPrivateData{UserResponseData: []byte(`{"id":"task-public"}`)}

	value, err := privateData.Value()

	require.NoError(t, err)
	require.NotNil(t, value)
	storedData, ok := value.([]byte)
	require.True(t, ok)
	var stored map[string]any
	require.NoError(t, common.Unmarshal(storedData, &stored))
	assert.Equal(t, "task-public", stored["user_response_data"].(map[string]any)["id"])
}

func TestTaskPrivateDataValueRetainsSeedanceUsageSnapshot(t *testing.T) {
	privateData := TaskPrivateData{BillingContext: &TaskBillingContext{
		InputVideoDurationMS:  2500,
		UpstreamCostMode:      string(types.CostModePerDuration),
		UsageInputTokens:      0,
		UsageCompletionTokens: 65000,
		UsageTotalTokens:      65000,
		SeedanceTokenPrice: &types.SeedanceTokenPrice{Scenarios: map[string]types.SeedanceTokenPriceScenario{
			"480p:with_video": {
				PricePerMillion: "1.917808219178082", Width: 864, Height: 496, FrameRate: 24,
				PricingVersion: "official-token-v1", Source: "SRC-OFFICIAL-SEEDANCE-2-0-MINI!18",
			},
		}},
		SeedanceTokenBilling: &types.SeedanceTokenBillingBreakdown{
			Scenario: "with_video", InputTokens: 0, OutputTokens: 65000, TotalTokens: 65000,
			FinalCharge: "0.1558219178082191625",
		},
	}}

	value, err := privateData.Value()

	require.NoError(t, err)
	require.NotNil(t, value)
	storedData, ok := value.([]byte)
	require.True(t, ok)
	var decoded TaskPrivateData
	require.NoError(t, common.Unmarshal(storedData, &decoded))
	require.NotNil(t, decoded.BillingContext)
	assert.Equal(t, int64(2500), decoded.BillingContext.InputVideoDurationMS)
	assert.Equal(t, string(types.CostModePerDuration), decoded.BillingContext.UpstreamCostMode)
	assert.Equal(t, 0, decoded.BillingContext.UsageInputTokens)
	assert.Equal(t, 65000, decoded.BillingContext.UsageCompletionTokens)
	assert.Equal(t, 65000, decoded.BillingContext.UsageTotalTokens)
	require.NotNil(t, decoded.BillingContext.SeedanceTokenPrice)
	require.NotNil(t, decoded.BillingContext.SeedanceTokenBilling)
	assert.Equal(t, "0.1558219178082191625", decoded.BillingContext.SeedanceTokenBilling.FinalCharge)
}

func TestTaskPrivateDataValueRetainsObjectStorageResult(t *testing.T) {
	privateData := TaskPrivateData{
		ResultObjectKey:         "doubao-seedance-2-0-fast/task_public.mp4",
		ResultObjectContentType: "video/mp4",
	}

	value, err := privateData.Value()
	require.NoError(t, err)
	require.NotNil(t, value)

	var decoded TaskPrivateData
	require.NoError(t, decoded.Scan(value))
	assert.Equal(t, privateData.ResultObjectKey, decoded.ResultObjectKey)
	assert.Equal(t, privateData.ResultObjectContentType, decoded.ResultObjectContentType)
}

func TestTaskPrivateDataScanKeepsLegacyResultURL(t *testing.T) {
	var decoded TaskPrivateData
	require.NoError(t, decoded.Scan([]byte(`{"result_url":"https://legacy.example/video.mp4"}`)))
	assert.Equal(t, "https://legacy.example/video.mp4", decoded.ResultURL)
	assert.Empty(t, decoded.ResultObjectKey)
}
