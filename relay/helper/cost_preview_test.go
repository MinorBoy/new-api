package helper

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	relaydto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCostPreviewUserBillingQuotaUsesSelectedGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	savedPrices := ratio_setting.ModelPrice2JSONString()
	savedGroups := ratio_setting.GroupRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(savedPrices))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(savedGroups))
	})

	prices, err := common.Marshal(map[string]float64{"preview-fixed": 0.2})
	require.NoError(t, err)
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(string(prices)))
	groups, err := common.Marshal(map[string]float64{"preview-group": 2})
	require.NoError(t, err)
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(string(groups)))

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	quota, snapshot, err := PreviewUserBillingQuota(ctx, dto.CostPreviewRequest{
		OriginModel: "preview-fixed",
		UserGroup:   "preview-group",
		RelayMode:   relayconstant.RelayModeVideoSubmit,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(200_000), quota)
	assert.Equal(t, "500000", snapshot)
}

func TestCostPreviewUserBillingQuotaSupportsTextDurationAndExpressionModes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	savedPrices := ratio_setting.ModelPrice2JSONString()
	savedGroups := ratio_setting.GroupRatio2JSONString()
	savedModes := billing_setting.GetBillingModeCopy()
	savedExpressions := billing_setting.GetBillingExprCopy()
	savedDurations := billing_setting.GetDurationPriceCopy()
	savedSeedancePrices := billing_setting.GetSeedanceTokenPriceCopy()
	originalQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 500_000
	t.Cleanup(func() {
		common.QuotaPerUnit = originalQuotaPerUnit
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(savedPrices))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(savedGroups))
		restoreCostPreviewBillingConfig(t, savedModes, savedExpressions, savedDurations, savedSeedancePrices)
	})

	prices, err := common.Marshal(map[string]float64{"preview-text": 0.2})
	require.NoError(t, err)
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(string(prices)))
	groups, err := common.Marshal(map[string]float64{"preview-group": 2})
	require.NoError(t, err)
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(string(groups)))
	configureCostPreviewBilling(t,
		map[string]string{
			"preview-duration": billing_setting.BillingModePerDuration,
			"preview-tiered":   billing_setting.BillingModeTieredExpr,
		},
		map[string]string{
			"preview-tiered": `tier("base", p * 2 + c * 10)`,
		},
		map[string]types.DurationPrice{
			"preview-duration": {
				Price: 0.1, Unit: types.DurationUnitSecond,
				RoundingStepSeconds: 1, MinimumDurationSeconds: 4,
			},
		},
		nil,
	)

	tests := []struct {
		name      string
		input     dto.CostPreviewRequest
		wantQuota int64
	}{
		{
			name: "text fixed price",
			input: dto.CostPreviewRequest{
				OriginModel: "preview-text", UserGroup: "preview-group",
				RelayMode: relayconstant.RelayModeChatCompletions,
				Usage:     &relaydto.Usage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150},
			},
			wantQuota: 200_000,
		},
		{
			name: "duration price",
			input: dto.CostPreviewRequest{
				OriginModel: "preview-duration", UserGroup: "preview-group",
				RelayMode:       relayconstant.RelayModeVideoSubmit,
				DurationSeconds: costPreviewIntPointer(6),
			},
			wantQuota: 600_000,
		},
		{
			name: "tiered expression",
			input: dto.CostPreviewRequest{
				OriginModel: "preview-tiered", UserGroup: "preview-group",
				RelayMode: relayconstant.RelayModeChatCompletions,
				Usage:     &relaydto.Usage{PromptTokens: 1_000, CompletionTokens: 100, TotalTokens: 1_100},
			},
			wantQuota: 3_000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			quota, snapshot, err := PreviewUserBillingQuota(ctx, tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.wantQuota, quota)
			assert.Equal(t, "500000", snapshot)
		})
	}
}

func TestCostPreviewUserBillingQuotaSupportsSeedanceTokenAndDurationInputs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	savedPrices := ratio_setting.ModelPrice2JSONString()
	savedGroups := ratio_setting.GroupRatio2JSONString()
	savedModes := billing_setting.GetBillingModeCopy()
	savedExpressions := billing_setting.GetBillingExprCopy()
	savedDurations := billing_setting.GetDurationPriceCopy()
	savedSeedancePrices := billing_setting.GetSeedanceTokenPriceCopy()
	originalQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 500_000
	t.Cleanup(func() {
		common.QuotaPerUnit = originalQuotaPerUnit
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(savedPrices))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(savedGroups))
		restoreCostPreviewBillingConfig(t, savedModes, savedExpressions, savedDurations, savedSeedancePrices)
	})

	groups, err := common.Marshal(map[string]float64{"preview-group": 2})
	require.NoError(t, err)
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(string(groups)))
	configureCostPreviewBilling(t,
		map[string]string{"doubao-seedance-2-0-mini-260615": billing_setting.BillingModeSeedanceTokens},
		nil,
		nil,
		map[string]types.SeedanceTokenPrice{
			"doubao-seedance-2-0-mini-260615": {
				Scenarios: map[string]types.SeedanceTokenPriceScenario{
					"480p:with_video": {
						PricePerMillion: "1.917808219178082", Width: 864, Height: 496, FrameRate: 24,
						PricingVersion: "official-token-v1", Source: "sd官价!A1",
					},
				},
			},
		},
	)

	duration := 4
	baseInput := dto.CostPreviewRequest{
		OriginModel: "doubao-seedance-2-0-mini-260615", UserGroup: "preview-group",
		RelayMode: relayconstant.RelayModeVideoSubmit, DurationSeconds: &duration,
		Resolution: "480p", HasVideoInput: true, InputVideoDurationMS: 3000,
	}
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	durationQuota, snapshot, err := PreviewUserBillingQuota(ctx, baseInput)
	require.NoError(t, err)

	baseInput.Usage = &relaydto.Usage{PromptTokens: 0, CompletionTokens: 70308, TotalTokens: 70308}
	tokenQuota, _, err := PreviewUserBillingQuota(ctx, baseInput)
	require.NoError(t, err)

	assert.Equal(t, int64(134_837), durationQuota)
	assert.Equal(t, durationQuota, tokenQuota)
	assert.Equal(t, "500000", snapshot)
}

func configureCostPreviewBilling(t *testing.T, modes map[string]string, expressions map[string]string, durations map[string]types.DurationPrice, seedancePrices map[string]types.SeedanceTokenPrice) {
	t.Helper()
	modesJSON, err := common.Marshal(modes)
	require.NoError(t, err)
	expressionsJSON, err := common.Marshal(expressions)
	require.NoError(t, err)
	durationsJSON, err := common.Marshal(durations)
	require.NoError(t, err)
	seedancePricesJSON, err := common.Marshal(seedancePrices)
	require.NoError(t, err)
	require.NoError(t, config.UpdateConfigFromMap(config.GlobalConfig.Get("billing_setting"), map[string]string{
		billing_setting.BillingModeField:        string(modesJSON),
		billing_setting.BillingExprField:        string(expressionsJSON),
		billing_setting.DurationPriceField:      string(durationsJSON),
		billing_setting.SeedanceTokenPriceField: string(seedancePricesJSON),
	}))
}

func restoreCostPreviewBillingConfig(t *testing.T, modes map[string]string, expressions map[string]string, durations map[string]types.DurationPrice, seedancePrices map[string]types.SeedanceTokenPrice) {
	t.Helper()
	configureCostPreviewBilling(t, modes, expressions, durations, seedancePrices)
}

func costPreviewIntPointer(value int) *int {
	return &value
}
