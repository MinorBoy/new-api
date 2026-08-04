package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildDifferencesComparesStructuredDurationPrice(t *testing.T) {
	const modelName = "duration-model"
	rule := types.DurationPrice{
		Price: 0.25, Unit: types.DurationUnitMinute,
		RoundingStepSeconds: 5, MinimumDurationSeconds: 10,
	}
	local := map[string]any{
		billing_setting.BillingModeField:   map[string]string{modelName: billing_setting.BillingModePerDuration},
		billing_setting.DurationPriceField: map[string]types.DurationPrice{modelName: rule},
	}
	upstreamRule := map[string]any{
		"price": 0.25, "unit": "minute",
		"rounding_step_seconds": float64(5), "minimum_duration_seconds": float64(10),
	}
	channels := []struct {
		name string
		data map[string]any
	}{
		{name: "upstream", data: map[string]any{
			billing_setting.BillingModeField:   map[string]any{modelName: billing_setting.BillingModePerDuration},
			billing_setting.DurationPriceField: map[string]any{modelName: upstreamRule},
		}},
	}

	differences := buildDifferences(local, channels)

	assert.NotContains(t, differences, modelName)
}

func TestBuildDifferencesKeepsDurationModeAndRuleTogether(t *testing.T) {
	const modelName = "duration-model"
	rule := types.DurationPrice{
		Price: 0.25, Unit: types.DurationUnitMinute,
		RoundingStepSeconds: 5, MinimumDurationSeconds: 10,
	}
	upstreamRule := map[string]any{
		"price": 0.25, "unit": "minute",
		"rounding_step_seconds": float64(5), "minimum_duration_seconds": float64(10),
	}
	channels := []struct {
		name string
		data map[string]any
	}{
		{name: "upstream", data: map[string]any{
			billing_setting.BillingModeField:   map[string]any{modelName: billing_setting.BillingModePerDuration},
			billing_setting.DurationPriceField: map[string]any{modelName: upstreamRule},
		}},
	}

	differences := buildDifferences(map[string]any{}, channels)

	modelDiff, ok := differences[modelName]
	require.True(t, ok)
	assert.Contains(t, modelDiff, billing_setting.BillingModeField)
	assert.Contains(t, modelDiff, billing_setting.DurationPriceField)
	assert.Equal(t, rule, modelDiff[billing_setting.DurationPriceField].Upstreams["upstream"])
}

func TestFetchUpstreamRatiosConvertsDurationPriceWithoutModelPrice(t *testing.T) {
	gin.SetMode(gin.TestMode)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/pricing", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"success":true,
			"data":[{
				"model_name":"remote-duration-model",
				"quota_type":1,
				"model_price":999,
				"billing_mode":"per_duration",
				"duration_price":{
					"price":0.25,
					"unit":"minute",
					"rounding_step_seconds":5,
					"minimum_duration_seconds":10
				}
			}]
		}`))
	}))
	t.Cleanup(server.Close)

	requestBody, err := common.Marshal(map[string]any{
		"upstreams": []map[string]any{{"name": "upstream", "base_url": server.URL}},
		"timeout":   2,
	})
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/ratio_sync", bytes.NewReader(requestBody))
	c.Request.Header.Set("Content-Type", "application/json")

	FetchUpstreamRatios(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			Differences map[string]map[string]any `json:"differences"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	modelDiff, ok := response.Data.Differences["remote-duration-model"]
	require.True(t, ok)
	assert.Contains(t, modelDiff, billing_setting.BillingModeField)
	assert.Contains(t, modelDiff, billing_setting.DurationPriceField)
	assert.NotContains(t, modelDiff, "model_price")
}

func TestBuildDifferencesComparesStructuredSeedanceTokenPrice(t *testing.T) {
	const modelName = "seedance-token-model"
	price := types.SeedanceTokenPrice{Scenarios: map[string]types.SeedanceTokenPriceScenario{
		"480p:with_video": {
			PricePerMillion: "1.917808219178082",
			Width:           864,
			Height:          496,
			FrameRate:       24,
			PricingVersion:  "official-token-v1",
			Source:          "official-sheet",
		},
	}}
	local := map[string]any{
		billing_setting.BillingModeField:        map[string]string{modelName: billing_setting.BillingModeSeedanceTokens},
		billing_setting.SeedanceTokenPriceField: map[string]types.SeedanceTokenPrice{modelName: price},
	}
	upstreamPrice := map[string]any{
		"scenarios": map[string]any{
			"480p:with_video": map[string]any{
				"price_per_million": "1.917808219178082",
				"width":             float64(864),
				"height":            float64(496),
				"frame_rate":        float64(24),
				"pricing_version":   "official-token-v1",
				"source":            "official-sheet",
			},
		},
	}
	channels := []struct {
		name string
		data map[string]any
	}{
		{name: "upstream", data: map[string]any{
			billing_setting.BillingModeField:        map[string]any{modelName: billing_setting.BillingModeSeedanceTokens},
			billing_setting.SeedanceTokenPriceField: map[string]any{modelName: upstreamPrice},
		}},
	}

	differences := buildDifferences(local, channels)

	assert.NotContains(t, differences, modelName)
}

func TestFetchUpstreamRatiosConvertsSeedanceTokenPriceWithoutModelPrice(t *testing.T) {
	gin.SetMode(gin.TestMode)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/pricing", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"success":true,
			"data":[{
				"model_name":"remote-seedance-token-model",
				"quota_type":1,
				"model_price":999,
				"billing_mode":"seedance_tokens",
				"seedance_token_price":{
					"scenarios":{
						"480p:with_video":{
							"price_per_million":"1.917808219178082",
							"width":864,
							"height":496,
							"frame_rate":24,
							"pricing_version":"official-token-v1",
							"source":"official-sheet"
						}
					}
				}
			}]
		}`))
	}))
	t.Cleanup(server.Close)

	requestBody, err := common.Marshal(map[string]any{
		"upstreams": []map[string]any{{"name": "upstream", "base_url": server.URL}},
		"timeout":   2,
	})
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/ratio_sync", bytes.NewReader(requestBody))
	c.Request.Header.Set("Content-Type", "application/json")

	FetchUpstreamRatios(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			Differences map[string]map[string]any `json:"differences"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	modelDiff, ok := response.Data.Differences["remote-seedance-token-model"]
	require.True(t, ok)
	assert.Contains(t, modelDiff, billing_setting.BillingModeField)
	assert.Contains(t, modelDiff, billing_setting.SeedanceTokenPriceField)
	assert.NotContains(t, modelDiff, "model_price")
}
