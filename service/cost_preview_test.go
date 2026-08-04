package service

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relaydto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPreviewCostAndBilledGrossProfit(t *testing.T) {
	preview, err := PreviewChannelModelCost(PreviewCostInput{
		FinalUserQuota: 500_000, QuotaPerUnitSnapshot: "500000",
		CostMode: types.CostModePerRequest, Config: normalizedUSDPerRequest("0.2"),
		Meter: types.CostMeter{Source: types.CostMeterValidatedRequest},
	})
	require.NoError(t, err)
	assert.True(t, preview.Estimated)
	assert.Equal(t, int64(1_000_000_000), preview.RevenueNanoUSD)
	assert.Equal(t, int64(200_000_000), preview.CostNanoUSD)
	assert.Equal(t, int64(800_000_000), preview.ProfitNanoUSD)
	assert.Equal(t, int64(800_000), *preview.MarginPPM)
}

func TestPreviewChannelModelCostAllowsNegativeProfit(t *testing.T) {
	preview, err := PreviewChannelModelCost(PreviewCostInput{
		FinalUserQuota: 100_000, QuotaPerUnitSnapshot: "500000",
		CostMode: types.CostModePerRequest, Config: normalizedUSDPerRequest("0.3"),
	})
	require.NoError(t, err)
	assert.Equal(t, int64(-100_000_000), preview.ProfitNanoUSD)
	assert.Equal(t, int64(-500_000), *preview.MarginPPM)
}

func TestPreviewFinalUserQuotaUsesPerRequestQuota(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := &relaycommon.RelayInfo{PriceData: types.PriceData{Quota: 321}}

	quota, err := PreviewFinalUserQuota(ctx, info, UserBillingPreviewInput{})
	require.NoError(t, err)
	assert.Equal(t, int64(321), quota)
	assert.Nil(t, info.QuotaClamp)
}

func TestPreviewFinalUserQuotaUsesDurationBilling(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	originalQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 500_000
	t.Cleanup(func() { common.QuotaPerUnit = originalQuotaPerUnit })

	price := types.DurationPrice{
		Price:                  0.1,
		Unit:                   types.DurationUnitSecond,
		RoundingStepSeconds:    1,
		MinimumDurationSeconds: 4,
	}
	info := &relaycommon.RelayInfo{PriceData: types.PriceData{
		DurationPrice:  &price,
		GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 2},
	}}
	duration := 6

	quota, err := PreviewFinalUserQuota(ctx, info, UserBillingPreviewInput{DurationSeconds: &duration})
	require.NoError(t, err)
	assert.Equal(t, int64(600_000), quota)
}

func TestPreviewFinalUserQuotaSeedanceTokenAndDurationInputsAgree(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	originalQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 500_000
	t.Cleanup(func() { common.QuotaPerUnit = originalQuotaPerUnit })

	price := types.SeedanceTokenPrice{Scenarios: map[string]types.SeedanceTokenPriceScenario{
		types.SeedanceTokenScenarioKey("480p", types.SeedanceTokenScenarioWithVideo): {
			PricePerMillion: "1.917808219178082", Width: 864, Height: 496, FrameRate: 24,
			PricingVersion: "official-token-v1", Source: "sd官价!A1",
		},
	}}
	newInfo := func() *relaycommon.RelayInfo {
		return &relaycommon.RelayInfo{PriceData: types.PriceData{
			BillingMode:              billing_setting.BillingModeSeedanceTokens,
			SeedanceTokenPrice:       &price,
			RequestedDurationSeconds: 4,
			DurationResolution:       "480p",
			HasVideoInput:            true,
			InputVideoDurationMS:     3000,
			GroupRatioInfo:           types.GroupRatioInfo{GroupRatio: 1.25},
		}}
	}
	duration := 4

	durationQuota, err := PreviewFinalUserQuota(ctx, newInfo(), UserBillingPreviewInput{DurationSeconds: &duration})
	require.NoError(t, err)
	tokenQuota, err := PreviewFinalUserQuota(ctx, newInfo(), UserBillingPreviewInput{Usage: &relaydto.Usage{
		PromptTokens: 30132, CompletionTokens: 40176, TotalTokens: 70308,
	}, DurationSeconds: &duration})
	require.NoError(t, err)

	assert.Equal(t, int64(84_273), durationQuota)
	assert.Equal(t, durationQuota, tokenQuota)
}

func TestPreviewFinalUserQuotaUsesTextSettlement(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := &relaycommon.RelayInfo{
		OriginModelName: "text-model",
		PriceData: types.PriceData{
			ModelRatio:      1,
			CompletionRatio: 2,
			GroupRatioInfo:  types.GroupRatioInfo{GroupRatio: 1},
		},
	}
	usage := &relaydto.Usage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150}

	quota, err := PreviewFinalUserQuota(ctx, info, UserBillingPreviewInput{Usage: usage})
	require.NoError(t, err)
	assert.Equal(t, int64(200), quota)
}

func TestPreviewFinalUserQuotaUsesFrozenBillingExpression(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	expression := `tier("base", p * 2 + c * 10)`
	info := &relaycommon.RelayInfo{
		OriginModelName: "tiered-model",
		TieredBillingSnapshot: &billingexpr.BillingSnapshot{
			BillingMode:  "tiered_expr",
			ModelName:    "tiered-model",
			ExprString:   expression,
			ExprHash:     billingexpr.ExprHashString(expression),
			GroupRatio:   1,
			QuotaPerUnit: 500_000,
			ExprVersion:  1,
		},
	}
	usage := &relaydto.Usage{PromptTokens: 1_000, CompletionTokens: 100, TotalTokens: 1_100}

	quota, err := PreviewFinalUserQuota(ctx, info, UserBillingPreviewInput{Usage: usage})
	require.NoError(t, err)
	assert.Equal(t, int64(1_500), quota)
}

func normalizedUSDPerRequest(unitPrice string) types.CostRuleConfigV1 {
	return types.CostRuleConfigV1{
		Currency:              "USD",
		BillingMultiplier:     "1",
		PurchaseDiscountRatio: "1",
		RechargeExchangeRatio: "1",
		FeeRate:               "0",
		CurrencyToUSDRate:     "1",
		UnitPrice:             &unitPrice,
		NormalizedUSDPrices: types.CostRulePricesV1{
			UnitPrice: &unitPrice,
		},
	}
}
