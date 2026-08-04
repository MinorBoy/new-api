package service

import (
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

type PreviewCostInput struct {
	FinalUserQuota       int64
	QuotaPerUnitSnapshot string
	CostMode             types.CostMode
	Config               types.CostRuleConfigV1
	Meter                types.CostMeter
}

type CostPreview struct {
	Estimated      bool
	OriginalCost   string
	RevenueNanoUSD int64
	CostNanoUSD    int64
	ProfitNanoUSD  int64
	MarginPPM      *int64
}

type UserBillingPreviewInput struct {
	Usage           *dto.Usage
	DurationSeconds *int
}

func PreviewFinalUserQuota(ctx *gin.Context, info *relaycommon.RelayInfo, input UserBillingPreviewInput) (int64, error) {
	if ctx == nil || info == nil {
		return 0, errors.New("billing preview context and relay info are required")
	}
	previewInfo := *info
	previewInfo.PriceData = info.PriceData
	previewInfo.QuotaClamp = nil
	if previewInfo.PriceData.FreeModel {
		return 0, nil
	}
	if previewInfo.PriceData.BillingMode == billing_setting.BillingModeSeedanceTokens {
		if previewInfo.PriceData.SeedanceTokenPrice == nil {
			return 0, errors.New("Seedance token price is unavailable")
		}
		requestedSeconds := previewInfo.PriceData.RequestedDurationSeconds
		if input.DurationSeconds != nil {
			requestedSeconds = *input.DurationSeconds
		}
		scenario, ok := previewInfo.PriceData.SeedanceTokenPrice.ScenarioFor(
			previewInfo.PriceData.DurationResolution,
			previewInfo.PriceData.HasVideoInput,
		)
		if !ok {
			return 0, fmt.Errorf(
				"Seedance token price scenario is unavailable for resolution %s and video_input=%t",
				previewInfo.PriceData.DurationResolution,
				previewInfo.PriceData.HasVideoInput,
			)
		}
		usage := types.SeedanceTokenUsage{}
		if input.Usage != nil {
			usage = types.SeedanceTokenUsage{
				InputTokens:  input.Usage.PromptTokens,
				OutputTokens: input.Usage.CompletionTokens,
				TotalTokens:  input.Usage.TotalTokens,
			}
		} else {
			inputTokens, outputTokens, totalTokens, err := EstimateSeedanceTokens(ProfitRoutingFacts{
				OutputDurationSeconds: requestedSeconds,
				InputDurationMS:       previewInfo.PriceData.InputVideoDurationMS,
				Width:                 scenario.Width,
				Height:                scenario.Height,
				FrameRateNum:          int64(scenario.FrameRate),
				FrameRateDen:          1,
			})
			if err != nil {
				return 0, err
			}
			usage = types.SeedanceTokenUsage{
				InputTokens:  int(inputTokens),
				OutputTokens: int(outputTokens),
				TotalTokens:  int(totalTokens),
			}
		}
		charge, err := previewInfo.PriceData.SeedanceTokenPrice.CalculateCharge(
			previewInfo.PriceData.DurationResolution,
			previewInfo.PriceData.HasVideoInput,
			previewInfo.PriceData.InputVideoDurationMS,
			requestedSeconds,
			usage,
			relaycommon.MaxTokensLimit,
		)
		if err != nil {
			return 0, err
		}
		finalCharge := charge.BaseCharge.Mul(decimal.NewFromFloat(previewInfo.PriceData.GroupRatioInfo.GroupRatio))
		quota, clamp := common.QuotaFromDecimalChecked(finalCharge.Mul(decimal.NewFromFloat(common.QuotaPerUnit)))
		if clamp != nil {
			return 0, fmt.Errorf("billing preview quota is out of range: %w", clamp)
		}
		return int64(quota), nil
	}

	if previewInfo.PriceData.DurationPrice != nil {
		requestedSeconds := previewInfo.PriceData.RequestedDurationSeconds
		if input.DurationSeconds != nil {
			requestedSeconds = *input.DurationSeconds
		}
		billableSeconds, err := previewInfo.PriceData.DurationPrice.BillableSeconds(requestedSeconds, relaycommon.MaxTaskDurationSeconds)
		if err != nil {
			return 0, err
		}
		quotaDecimal := decimal.NewFromFloat(previewInfo.PriceData.DurationPrice.Price).
			Mul(decimal.NewFromInt(int64(billableSeconds))).
			Div(decimal.NewFromInt(int64(previewInfo.PriceData.DurationPrice.UnitSeconds()))).
			Mul(decimal.NewFromFloat(common.QuotaPerUnit)).
			Mul(decimal.NewFromFloat(previewInfo.PriceData.GroupRatioInfo.GroupRatio))
		quotaDecimal = previewInfo.PriceData.ApplyOtherRatiosToDecimal(quotaDecimal)
		quota, clamp := common.QuotaFromDecimalChecked(quotaDecimal)
		if clamp != nil {
			return 0, fmt.Errorf("billing preview quota is out of range: %w", clamp)
		}
		return int64(quota), nil
	}

	if input.Usage == nil {
		if previewInfo.PriceData.Quota < 0 {
			return 0, errors.New("billing preview quota cannot be negative")
		}
		return int64(previewInfo.PriceData.Quota), nil
	}

	billingUsage := effectiveBillingUsage(input.Usage)
	summary := calculateTextQuotaSummary(ctx, &previewInfo, billingUsage)
	if snapshot := previewInfo.TieredBillingSnapshot; snapshot != nil {
		usedVars := billingexpr.UsedVars(snapshot.ExprString)
		tiered, quota, result := TryTieredSettle(&previewInfo, BuildTieredTokenParams(billingUsage, false, usedVars))
		if tiered {
			summary.Quota = composeTieredTextQuota(&previewInfo, summary, quota, result)
		}
	}
	if previewInfo.QuotaClamp != nil {
		return 0, fmt.Errorf("billing preview quota is out of range: %w", previewInfo.QuotaClamp)
	}
	if summary.Quota < 0 {
		return 0, errors.New("billing preview quota cannot be negative")
	}
	return int64(summary.Quota), nil
}

func PreviewChannelModelCost(input PreviewCostInput) (CostPreview, error) {
	revenueNanoUSD, err := RevenueEquivalentNanoUSD(input.FinalUserQuota, input.QuotaPerUnitSnapshot)
	if err != nil {
		return CostPreview{}, err
	}
	originalCost, costNanoUSD, err := CalculateAttemptCost(input.CostMode, input.Config, input.Meter)
	if err != nil {
		return CostPreview{}, err
	}
	profitNanoUSD, err := CheckedNanoSubtract(revenueNanoUSD, costNanoUSD)
	if err != nil {
		return CostPreview{}, err
	}
	marginPPM, err := GrossMarginPPM(profitNanoUSD, revenueNanoUSD)
	if err != nil {
		return CostPreview{}, err
	}
	return CostPreview{
		Estimated:      true,
		OriginalCost:   originalCost,
		RevenueNanoUSD: revenueNanoUSD,
		CostNanoUSD:    costNanoUSD,
		ProfitNanoUSD:  profitNanoUSD,
		MarginPPM:      marginPPM,
	}, nil
}
