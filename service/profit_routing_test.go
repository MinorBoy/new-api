package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/seedancepricing"
	"github.com/QuantumNous/new-api/types"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// nano converts a USD amount to nano-USD so the eligibility tests stay readable.
func nano(usd string) int64 {
	d, err := decimal.NewFromString(usd)
	if err != nil {
		panic(err)
	}
	value, err := DecimalToNanoUSD(d)
	if err != nil {
		panic(err)
	}
	return value
}

func TestEvaluateProfitEligibilityPerRequestThreshold(t *testing.T) {
	// Channel cost is fixed at 5 yuan/req; official per-second price 0.99 yuan with a
	// 0.5 group ratio yields 2.475 yuan revenue at 5 seconds — below the 5 yuan cost.
	caseInput := ProfitRoutingInput{RevenueNanoUSD: nano("2.475"), CostNanoUSD: nano("5"), ThresholdBPS: 0}
	result := EvaluateProfitEligibility(caseInput)
	require.False(t, result.Eligible)
	assert.Equal(t, ProfitReasonMarginBelowThreshold, result.Reason)
}

func TestEvaluateProfitEligibilityPerRequestBusinessBoundaries(t *testing.T) {
	tests := []struct {
		name        string
		revenueNano int64
		costNano    int64
		threshold   int
		eligible    bool
	}{
		// 0% threshold: 5s excluded (revenue 2.475 < cost 5), 11s admitted (5.445 > 5).
		{"0% threshold 5 seconds excluded", nano("2.475"), nano("5"), 0, false},
		{"0% threshold 11 seconds admitted", nano("5.445"), nano("5"), 0, true},
		// 10% threshold: 11s excluded (margin 8.17% < 10%), 12s admitted.
		{"10% threshold 11 seconds excluded", nano("5.445"), nano("5"), 1000, false},
		{"10% threshold 12 seconds admitted", nano("5.94"), nano("5"), 1000, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EvaluateProfitEligibility(ProfitRoutingInput{RevenueNanoUSD: tt.revenueNano, CostNanoUSD: tt.costNano, ThresholdBPS: tt.threshold})
			assert.Equal(t, tt.eligible, result.Eligible)
			if !tt.eligible {
				assert.Equal(t, ProfitReasonMarginBelowThreshold, result.Reason)
			}
		})
	}
}

func TestEvaluateProfitEligibilityMarginEqualToThresholdAdmits(t *testing.T) {
	// revenue 10, cost 9 → margin 10% → exactly 1000 BPS must be admitted (>= comparison).
	result := EvaluateProfitEligibility(ProfitRoutingInput{RevenueNanoUSD: nano("10"), CostNanoUSD: nano("9"), ThresholdBPS: 1000})
	require.True(t, result.Eligible)
	require.NotNil(t, result.MarginPPM)
	assert.Equal(t, int64(100_000), *result.MarginPPM)
}

func TestEvaluateProfitEligibilityFreeCostAdmitsWhenRevenuePositive(t *testing.T) {
	result := EvaluateProfitEligibility(ProfitRoutingInput{RevenueNanoUSD: nano("1"), CostNanoUSD: 0, ThresholdBPS: 0})
	require.True(t, result.Eligible)
	require.NotNil(t, result.MarginPPM)
	assert.Equal(t, int64(1_000_000), *result.MarginPPM)
}

func TestEvaluateProfitEligibilityZeroRevenueExcludes(t *testing.T) {
	result := EvaluateProfitEligibility(ProfitRoutingInput{RevenueNanoUSD: 0, CostNanoUSD: 0, ThresholdBPS: 0})
	require.False(t, result.Eligible)
	assert.Equal(t, ProfitReasonRevenueUnknown, result.Reason)
	assert.Nil(t, result.MarginPPM)
}

func TestEvaluateProfitEligibilityOverflowExcludes(t *testing.T) {
	// An impossibly large revenue near int64 max must fail closed rather than wrap.
	maxNano := int64(1 << 62)
	result := EvaluateProfitEligibility(ProfitRoutingInput{RevenueNanoUSD: maxNano, CostNanoUSD: -maxNano, ThresholdBPS: 0})
	require.False(t, result.Eligible)
	assert.Equal(t, ProfitReasonCalculationError, result.Reason)
}

func TestEstimateSeedanceTokensUsesResolutionProfileAndFrameRate(t *testing.T) {
	// 720p: 1280x720 @ 24fps. Input 3s, output 5s.
	// input  = 3000ms * 1280 * 720 * 24 / 1024 / 1000
	// output = 5000ms * 1280 * 720 * 24 / 1024 / 1000
	profile, ok := seedancepricing.Profile("720p")
	require.True(t, ok)
	facts := ProfitRoutingFacts{
		OutputDurationSeconds: 5,
		InputDurationMS:       3000,
		Width:                 profile.Width,
		Height:                profile.Height,
		FrameRateNum:          profile.FrameRateNum,
		FrameRateDen:          profile.FrameRateDen,
	}
	inputTokens, outputTokens, totalTokens, err := EstimateSeedanceTokens(facts)
	require.NoError(t, err)

	expectedTotal := decimal.NewFromInt(3000 + 5000).
		Mul(decimal.NewFromInt(int64(profile.Width))).
		Mul(decimal.NewFromInt(int64(profile.Height))).
		Mul(decimal.NewFromInt(profile.FrameRateNum)).
		Div(decimal.NewFromInt(profile.FrameRateDen)).
		Div(decimal.NewFromInt(1024)).
		Div(decimal.NewFromInt(1000)).
		Ceil()

	assert.Zero(t, inputTokens)
	assert.Equal(t, expectedTotal.IntPart(), outputTokens)
	assert.Equal(t, expectedTotal.IntPart(), totalTokens)
}

func TestEstimateSeedanceTokensUsesCombinedVideoDurationAsCompletionUsage(t *testing.T) {
	inputTokens, outputTokens, totalTokens, err := EstimateSeedanceTokens(ProfitRoutingFacts{
		InputDurationMS:       3000,
		OutputDurationSeconds: 5,
		Width:                 864,
		Height:                496,
		FrameRateNum:          24,
		FrameRateDen:          1,
	})

	require.NoError(t, err)
	assert.Zero(t, inputTokens)
	assert.Equal(t, int64(80352), outputTokens)
	assert.Equal(t, int64(80352), totalTokens)
}

func TestEstimateSeedanceTokensTotalComputedBeforeCeil(t *testing.T) {
	// Verify the invariant: total is ceil(sum of raw input+output), which can exceed
	// ceil(input) + ceil(output). Use dimensions that produce fractional tokens.
	facts := ProfitRoutingFacts{
		OutputDurationSeconds: 1,
		InputDurationMS:       1, // 1ms produces a fractional token
		Width:                 1280,
		Height:                720,
		FrameRateNum:          24,
		FrameRateDen:          1,
	}
	_, _, total, err := EstimateSeedanceTokens(facts)
	require.NoError(t, err)
	assert.Greater(t, total, int64(0))
}

func TestEstimateSeedanceTokensRejectsUnknownResolution(t *testing.T) {
	facts := ProfitRoutingFacts{
		OutputDurationSeconds: 5,
		InputDurationMS:       3000,
		Width:                 0,
		Height:                0,
		FrameRateNum:          0,
		FrameRateDen:          0,
	}
	_, _, _, err := EstimateSeedanceTokens(facts)
	require.Error(t, err)
}

func TestEstimateSeedanceTokensRejectsOversizedResult(t *testing.T) {
	// A pathological duration/dimension combination must fail closed at MaxTokensLimit,
	// never overflow into a negative token count.
	facts := ProfitRoutingFacts{
		OutputDurationSeconds: 3_600_000,
		InputDurationMS:       3_600_000_000,
		Width:                 3840,
		Height:                2160,
		FrameRateNum:          24,
		FrameRateDen:          1,
	}
	_, _, _, err := EstimateSeedanceTokens(facts)
	require.Error(t, err)
}

func TestBuildProfitCostMeterPerRequest(t *testing.T) {
	facts := ProfitRoutingFacts{OutputDurationSeconds: 11}
	meter, err := BuildProfitCostMeter(types.CostModePerRequest, facts)
	require.NoError(t, err)
	assert.Nil(t, meter.DurationSeconds)
	assert.Nil(t, meter.TotalTokens)
}

func TestBuildProfitCostMeterPerDurationUsesRequestedSeconds(t *testing.T) {
	facts := ProfitRoutingFacts{OutputDurationSeconds: 11}
	meter, err := BuildProfitCostMeter(types.CostModePerDuration, facts)
	require.NoError(t, err)
	require.NotNil(t, meter.DurationSeconds)
	assert.Equal(t, "11", *meter.DurationSeconds)
}

func TestBuildProfitCostMeterPerTokenSubModes(t *testing.T) {
	facts := ProfitRoutingFacts{
		OutputDurationSeconds: 5,
		InputDurationMS:       3000,
		Width:                 1280,
		Height:                720,
		FrameRateNum:          24,
		FrameRateDen:          1,
	}
	inputTokens, outputTokens, totalTokens, err := EstimateSeedanceTokens(facts)
	require.NoError(t, err)
	facts.InputTokens = inputTokens
	facts.OutputTokens = outputTokens
	facts.TotalTokens = totalTokens

	tests := []struct {
		mode types.CostTokenMode
	}{
		{types.CostTokenModeTotal},
		{types.CostTokenModeCompletion},
		{types.CostTokenModeInputOutput},
	}
	for _, tt := range tests {
		t.Run(string(tt.mode), func(t *testing.T) {
			meter, err := BuildProfitCostMeter(types.CostModePerToken, facts)
			require.NoError(t, err)
			require.NotNil(t, meter.TotalTokens)
			require.NotNil(t, meter.CompletionTokens)
			require.NotNil(t, meter.InputTokens)
			require.NotNil(t, meter.OutputTokens)
			assert.Equal(t, totalTokens, *meter.TotalTokens)
			assert.Equal(t, outputTokens, *meter.CompletionTokens)
			assert.Equal(t, inputTokens, *meter.InputTokens)
			assert.Equal(t, outputTokens, *meter.OutputTokens)
		})
	}
}

func TestFilterProfitEligibleChannelsAllowsTokenPricingWithoutReferenceVideo(t *testing.T) {
	facts, err := EstimateProfitRoutingFacts("720p", 5, 0)
	require.NoError(t, err)
	require.Zero(t, facts.InputTokens)
	require.Positive(t, facts.OutputTokens)

	config, err := NormalizeCostRuleConfig(types.CostModePerToken, validTokenCostConfig(types.CostTokenModeInputOutput, types.CostMeterLocalUsage))
	require.NoError(t, err)
	configJSON, err := common.Marshal(config)
	require.NoError(t, err)
	candidate := ProfitRoutingCandidate{ChannelID: 7, PredictedUpstreamModel: "vendor-model", CostVariantKey: string(types.DefaultCostVariantKey)}
	rules := map[CostRuleCandidate]*model.ChannelModelCostRule{
		{ChannelID: candidate.ChannelID, BillableUpstreamModel: candidate.PredictedUpstreamModel, CostVariantKey: string(types.DefaultCostVariantKey)}: {
			ChannelID: candidate.ChannelID, BillableUpstreamModel: candidate.PredictedUpstreamModel,
			CostMode: string(types.CostModePerToken), SchemaVersion: 1, ConfigJSON: string(configJSON),
		},
	}

	result := FilterProfitEligibleChannels(ProfitChannelFilterInput{
		Facts: facts, RevenueNanoUSD: nano("10"), HasRevenue: true,
		Candidates: []ProfitRoutingCandidate{candidate}, MetadataState: nil,
	}, rules)

	assert.Contains(t, result.AllowedChannelIDs, candidate.ChannelID)
	assert.Empty(t, result.Exclusions)
}

func TestFilterProfitEligibleChannelsMetadataUnavailableExcludesOnlyTokenCandidates(t *testing.T) {
	facts, err := EstimateProfitRoutingFacts("720p", 5, 0)
	require.NoError(t, err)
	tokenConfig, err := NormalizeCostRuleConfig(types.CostModePerToken, validTokenCostConfig(types.CostTokenModeTotal, types.CostMeterLocalUsage))
	require.NoError(t, err)
	tokenConfigJSON, err := common.Marshal(tokenConfig)
	require.NoError(t, err)
	unitPrice := "1"
	requestConfig, err := NormalizeCostRuleConfig(types.CostModePerRequest, types.CostRuleConfigV1{
		Currency:              "USD",
		BillingMultiplier:     "1",
		PurchaseDiscountRatio: "1",
		RechargeExchangeRatio: "1",
		FeeRate:               "0",
		CurrencyToUSDRate:     "1",
		UnitPrice:             &unitPrice,
		ChargeEvent:           types.CostChargeResponseSucceeded,
	})
	require.NoError(t, err)
	requestConfigJSON, err := common.Marshal(requestConfig)
	require.NoError(t, err)

	tokenCandidate := ProfitRoutingCandidate{ChannelID: 7, PredictedUpstreamModel: "token-model", CostVariantKey: string(types.DefaultCostVariantKey)}
	requestCandidate := ProfitRoutingCandidate{ChannelID: 8, PredictedUpstreamModel: "request-model", CostVariantKey: string(types.DefaultCostVariantKey)}
	state := NewProfitRoutingRequestState(&fakeMetadataClient{
		errs: map[string]error{"https://assets.example/input.mp4?signature=secret": &VideoMetadataError{Kind: VideoMetadataUnavailable}},
	}, []string{"https://assets.example/input.mp4?signature=secret"}, 1)
	result := FilterProfitEligibleChannels(ProfitChannelFilterInput{
		Ctx:            context.Background(),
		Facts:          facts,
		RevenueNanoUSD: nano("10"),
		HasRevenue:     true,
		Candidates:     []ProfitRoutingCandidate{tokenCandidate, requestCandidate},
		MetadataState:  state,
	}, map[CostRuleCandidate]*model.ChannelModelCostRule{
		{ChannelID: tokenCandidate.ChannelID, BillableUpstreamModel: tokenCandidate.PredictedUpstreamModel, CostVariantKey: string(types.DefaultCostVariantKey)}: {
			ID: 70, ChannelID: tokenCandidate.ChannelID, BillableUpstreamModel: tokenCandidate.PredictedUpstreamModel,
			Version: 1, CostMode: string(types.CostModePerToken), SchemaVersion: 1, ConfigJSON: string(tokenConfigJSON),
		},
		{ChannelID: requestCandidate.ChannelID, BillableUpstreamModel: requestCandidate.PredictedUpstreamModel, CostVariantKey: string(types.DefaultCostVariantKey)}: {
			ID: 80, ChannelID: requestCandidate.ChannelID, BillableUpstreamModel: requestCandidate.PredictedUpstreamModel,
			Version: 1, CostMode: string(types.CostModePerRequest), SchemaVersion: 1, ConfigJSON: string(requestConfigJSON),
		},
	})

	assert.Contains(t, result.AllowedChannelIDs, requestCandidate.ChannelID)
	assert.NotContains(t, result.AllowedChannelIDs, tokenCandidate.ChannelID)
	require.Len(t, result.Exclusions, 1)
	assert.Equal(t, tokenCandidate.ChannelID, result.Exclusions[0].ChannelID)
	assert.Equal(t, ProfitReasonMetadataUnavailable, result.Exclusions[0].Reason)
	body, err := common.Marshal(result.Exclusions)
	require.NoError(t, err)
	assert.NotContains(t, string(body), "assets.example")
	assert.NotContains(t, string(body), "secret")
}

func TestCalculateTaskTokenQuotaMatchesSettlementFormula(t *testing.T) {
	// totalTokens * modelRatio * groupRatio * otherMultiplier, saturated via the shared
	// helper. The same function must drive both the asynchronous actual settlement and
	// the profit predictor so the two never drift.
	quota, clamp, err := CalculateTaskTokenQuota(1_000_000, 46.0, 0.5, 1.0)
	require.NoError(t, err)
	assert.Nil(t, clamp)
	assert.Equal(t, int(1_000_000*46*0.5), quota)
}

func TestCalculateTaskTokenQuotaRejectsNegativeInput(t *testing.T) {
	_, _, err := CalculateTaskTokenQuota(-1, 1, 1, 1)
	require.Error(t, err)
}

func TestCalculateTaskTokenQuotaRejectsInvalidRatios(t *testing.T) {
	tests := []struct {
		name            string
		modelRatio      float64
		groupRatio      float64
		otherMultiplier float64
	}{
		{"negative model ratio", -1, 1, 1},
		{"negative group ratio", 1, -1, 1},
		{"negative other multiplier", 1, 1, -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := CalculateTaskTokenQuota(1000, tt.modelRatio, tt.groupRatio, tt.otherMultiplier)
			require.Error(t, err)
		})
	}
}

func TestCalculateTaskTokenQuotaSaturatesOverflow(t *testing.T) {
	// A value beyond int32 must saturate and report a clamp, not wrap to negative.
	quota, clamp, err := CalculateTaskTokenQuota(1<<40, 1e6, 1e6, 1e6)
	require.NoError(t, err)
	require.NotNil(t, clamp)
	assert.Equal(t, common.MaxQuota, quota)
}

func TestProfitExclusionReasonsAreStable(t *testing.T) {
	// The reason enum is part of the admin diagnostics contract; values must not change.
	assert.Equal(t, ProfitExclusionReason("revenue_unknown"), ProfitReasonRevenueUnknown)
	assert.Equal(t, ProfitExclusionReason("cost_rule_missing"), ProfitReasonCostRuleMissing)
	assert.Equal(t, ProfitExclusionReason("meter_unknown"), ProfitReasonMeterUnknown)
	assert.Equal(t, ProfitExclusionReason("margin_below_threshold"), ProfitReasonMarginBelowThreshold)
	assert.Equal(t, ProfitExclusionReason("calculation_error"), ProfitReasonCalculationError)
	assert.Equal(t, ProfitExclusionReason("metadata_unavailable"), ProfitReasonMetadataUnavailable)
}

func TestPreviewRoutingRevenueFailsClosedWithoutHook(t *testing.T) {
	original := currentRoutingRevenuePreview()
	t.Cleanup(func() { SetRoutingRevenuePreview(original) })
	SetRoutingRevenuePreview(nil)
	_, err := PreviewRoutingRevenue(nil, RoutingRevenuePreviewInput{OriginModelName: "any"})
	require.Error(t, err)
}

func TestPreviewRoutingRevenueConvertsUserQuotaToNanoUSD(t *testing.T) {
	original := currentRoutingRevenuePreview()
	t.Cleanup(func() { SetRoutingRevenuePreview(original) })
	// Simulate a callback returning a final quota of 200000 (=$0.40 user revenue at
	// QuotaPerUnit=500000) and assert the service converts it to nano-USD.
	SetRoutingRevenuePreview(func(_ context.Context, _ RoutingRevenuePreviewInput) (int64, string, error) {
		return 200_000, "500000", nil
	})
	revenueNano, err := PreviewRoutingRevenue(nil, RoutingRevenuePreviewInput{OriginModelName: "any"})
	require.NoError(t, err)
	// 200000 / 500000 = 0.4 USD = 400_000_000 nano-USD.
	assert.Equal(t, int64(400_000_000), revenueNano)
}

func TestPreviewRoutingRevenuePropagatesCallbackError(t *testing.T) {
	original := currentRoutingRevenuePreview()
	t.Cleanup(func() { SetRoutingRevenuePreview(original) })
	SetRoutingRevenuePreview(func(_ context.Context, _ RoutingRevenuePreviewInput) (int64, string, error) {
		return 0, "", fmt.Errorf("pricing unavailable")
	})
	_, err := PreviewRoutingRevenue(nil, RoutingRevenuePreviewInput{OriginModelName: "any"})
	require.Error(t, err)
}
