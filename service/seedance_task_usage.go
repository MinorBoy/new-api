package service

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
)

type SeedanceTerminalFacts struct {
	DurationSeconds        int
	DurationPresent        bool
	Resolution             string
	ResolutionPresent      bool
	FramesPerSecond        int
	FramesPerSecondPresent bool
}

type SeedanceTaskUsage struct {
	InputTokens      int
	CompletionTokens int
	TotalTokens      int
}

func IsValidSeedanceUsage(completionTokens, totalTokens int64) bool {
	return completionTokens > 0 &&
		totalTokens == completionTokens &&
		completionTokens <= int64(relaycommon.MaxTokensLimit) &&
		totalTokens <= int64(relaycommon.MaxTokensLimit)
}

// IsValidSeedanceUpstreamUsage applies the shared trust boundary used by
// settlement and public task responses.
func IsValidSeedanceUpstreamUsage(completionTokens, totalTokens int64) bool {
	return IsValidSeedanceUsage(completionTokens, totalTokens)
}

func PersistedSeedanceTaskUsage(bc *model.TaskBillingContext) (SeedanceTaskUsage, bool) {
	if bc == nil || bc.UsageSnapshotVersion != model.TaskUsageSnapshotVersion1 ||
		!IsValidSeedanceUsage(int64(bc.UsageCompletionTokens), int64(bc.UsageTotalTokens)) ||
		bc.UsageInputTokens != 0 {
		return SeedanceTaskUsage{}, false
	}
	return SeedanceTaskUsage{
		InputTokens:      bc.UsageInputTokens,
		CompletionTokens: bc.UsageCompletionTokens,
		TotalTokens:      bc.UsageTotalTokens,
	}, true
}

func applySeedanceTaskUsage(task *model.Task, result *relaycommon.TaskInfo, usage SeedanceTaskUsage, source string) {
	result.CompletionTokens = usage.CompletionTokens
	result.TotalTokens = usage.TotalTokens
	result.CompletionTokensPresent = true
	result.TotalTokensPresent = true
	result.UsageSource = source
	billingContext := task.PrivateData.BillingContext
	billingContext.UsageInputTokens = usage.InputTokens
	billingContext.UsageCompletionTokens = usage.CompletionTokens
	billingContext.UsageTotalTokens = usage.TotalTokens
	billingContext.BillingTokens = usage.TotalTokens
	billingContext.UsageSource = source
}

func NormalizeSeedanceTaskUsage(task *model.Task, result *relaycommon.TaskInfo) error {
	if task == nil || result == nil || model.TaskStatus(result.Status) != model.TaskStatusSuccess {
		return nil
	}
	billingContext := task.PrivateData.BillingContext
	if billingContext == nil || billingContext.UsageProfile != model.TaskUsageProfileSeedance {
		return nil
	}
	if result.UpstreamUsageCostMeter == nil && result.BillingClamp == nil &&
		(result.CompletionTokensPresent || result.TotalTokensPresent) {
		meter := types.CostMeter{Source: types.CostMeterUpstreamUsage}
		if result.CompletionTokensPresent {
			completionTokens := int64(result.CompletionTokens)
			meter.OutputTokens = &completionTokens
			meter.CompletionTokens = &completionTokens
		}
		if result.TotalTokensPresent {
			totalTokens := int64(result.TotalTokens)
			meter.TotalTokens = &totalTokens
		}
		result.UpstreamUsageCostMeter = &meter
	}

	calculatedUsage, calculationErr := CalculateSeedanceTaskUsage(billingContext, SeedanceTerminalFacts{})
	upstreamUsageMatchesFormula := calculationErr == nil &&
		result.CompletionTokens == calculatedUsage.CompletionTokens &&
		result.TotalTokens == calculatedUsage.TotalTokens
	if result.BillingClamp == nil &&
		result.CompletionTokensPresent && result.TotalTokensPresent &&
		IsValidSeedanceUpstreamUsage(int64(result.CompletionTokens), int64(result.TotalTokens)) &&
		upstreamUsageMatchesFormula {
		applySeedanceTaskUsage(task, result, SeedanceTaskUsage{
			InputTokens:      0,
			CompletionTokens: result.CompletionTokens,
			TotalTokens:      result.TotalTokens,
		}, model.TaskUsageSourceUpstream)
		return nil
	}

	if calculationErr == nil {
		applySeedanceTaskUsage(task, result, calculatedUsage, model.TaskUsageSourceLocalCalculated)
		return nil
	}
	if usage, ok := PersistedSeedanceTaskUsage(billingContext); ok {
		applySeedanceTaskUsage(task, result, usage, model.TaskUsageSourceLocalCalculated)
		return nil
	}
	if billingContext.UsageSnapshotVersion == model.TaskUsageSnapshotVersion1 {
		return errors.New("versioned Seedance usage snapshot is unavailable")
	}
	return nil
}

func CalculateSeedanceTaskUsage(billingContext *model.TaskBillingContext, _ SeedanceTerminalFacts) (SeedanceTaskUsage, error) {
	if billingContext == nil {
		return SeedanceTaskUsage{}, fmt.Errorf("billing context is unavailable")
	}

	durationSeconds := billingContext.RequestedDurationSeconds
	if durationSeconds <= 0 || durationSeconds > relaycommon.MaxTaskDurationSeconds {
		return SeedanceTaskUsage{}, fmt.Errorf("output duration is out of range")
	}

	inputDurationMS := billingContext.InputVideoDurationMS
	if billingContext.HasVideoInput && inputDurationMS <= 0 {
		return SeedanceTaskUsage{}, fmt.Errorf("reference video duration is unavailable")
	}
	if inputDurationMS < 0 || inputDurationMS > int64(relaycommon.MaxTaskDurationSeconds)*1000 {
		return SeedanceTaskUsage{}, fmt.Errorf("reference video duration is out of range")
	}

	resolution := strings.TrimSpace(billingContext.DurationResolution)
	if resolution == "" {
		resolution = strings.TrimSpace(billingContext.Resolution)
	}
	width := 0
	height := 0
	frameRate := 0
	if billingContext.SeedanceTokenBilling != nil {
		width = billingContext.SeedanceTokenBilling.Width
		height = billingContext.SeedanceTokenBilling.Height
		frameRate = billingContext.SeedanceTokenBilling.FrameRate
	} else if billingContext.SeedanceTokenPrice != nil {
		scenario, ok := billingContext.SeedanceTokenPrice.ScenarioFor(resolution, billingContext.HasVideoInput)
		if !ok {
			return SeedanceTaskUsage{}, fmt.Errorf("frozen Seedance token pricing scenario is unavailable")
		}
		width = scenario.Width
		height = scenario.Height
		frameRate = scenario.FrameRate
	}
	if width <= 0 || height <= 0 || frameRate <= 0 {
		return SeedanceTaskUsage{}, fmt.Errorf("frozen Seedance token pricing geometry is unavailable")
	}

	inputTokens, completionTokens, totalTokens, err := EstimateSeedanceTokens(ProfitRoutingFacts{
		OutputDurationSeconds: durationSeconds,
		InputDurationMS:       inputDurationMS,
		Width:                 width,
		Height:                height,
		FrameRateNum:          int64(frameRate),
		FrameRateDen:          1,
	})
	if err != nil {
		return SeedanceTaskUsage{}, err
	}
	return SeedanceTaskUsage{
		InputTokens:      int(inputTokens),
		CompletionTokens: int(completionTokens),
		TotalTokens:      int(totalTokens),
	}, nil
}
