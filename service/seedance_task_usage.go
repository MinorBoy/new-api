package service

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/seedancepricing"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

const maxSeedanceFrameRate = 240

type SeedanceTerminalFacts struct {
	DurationSeconds        int
	DurationPresent        bool
	Resolution             string
	ResolutionPresent      bool
	FramesPerSecond        int
	FramesPerSecondPresent bool
}

type SeedanceTaskUsage struct {
	CompletionTokens int
	TotalTokens      int
}

func IsValidSeedanceUsage(completionTokens, totalTokens int64) bool {
	return completionTokens > 0 &&
		totalTokens >= completionTokens &&
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
		!IsValidSeedanceUsage(int64(bc.UsageCompletionTokens), int64(bc.UsageTotalTokens)) {
		return SeedanceTaskUsage{}, false
	}
	return SeedanceTaskUsage{
		CompletionTokens: bc.UsageCompletionTokens,
		TotalTokens:      bc.UsageTotalTokens,
	}, true
}

func NormalizeSeedanceTaskUsage(task *model.Task, result *relaycommon.TaskInfo) error {
	if task == nil || result == nil || model.TaskStatus(result.Status) != model.TaskStatusSuccess {
		return nil
	}
	billingContext := task.PrivateData.BillingContext
	if billingContext == nil || billingContext.UsageProfile != model.TaskUsageProfileSeedance {
		return nil
	}

	if result.BillingClamp == nil &&
		result.CompletionTokensPresent && result.TotalTokensPresent &&
		IsValidSeedanceUpstreamUsage(int64(result.CompletionTokens), int64(result.TotalTokens)) {
		result.UsageSource = model.TaskUsageSourceUpstream
		billingContext.UsageSource = model.TaskUsageSourceUpstream
		billingContext.BillingTokens = result.CompletionTokens
		return nil
	}

	usage, err := CalculateSeedanceTaskUsage(billingContext, SeedanceTerminalFacts{
		DurationSeconds:        result.DurationSeconds,
		DurationPresent:        result.DurationPresent,
		Resolution:             result.Resolution,
		ResolutionPresent:      result.ResolutionPresent,
		FramesPerSecond:        result.FramesPerSecond,
		FramesPerSecondPresent: result.FramesPerSecondPresent,
	})
	if err != nil {
		return err
	}
	result.CompletionTokens = usage.CompletionTokens
	result.TotalTokens = usage.TotalTokens
	result.CompletionTokensPresent = true
	result.TotalTokensPresent = true
	result.UsageSource = model.TaskUsageSourceLocalCalculated
	billingContext.UsageSource = model.TaskUsageSourceLocalCalculated
	billingContext.BillingTokens = usage.CompletionTokens
	return nil
}

func CalculateSeedanceTaskUsage(billingContext *model.TaskBillingContext, terminal SeedanceTerminalFacts) (SeedanceTaskUsage, error) {
	if billingContext == nil {
		return SeedanceTaskUsage{}, fmt.Errorf("billing context is unavailable")
	}

	durationSeconds := billingContext.RequestedDurationSeconds
	if terminal.DurationPresent && terminal.DurationSeconds > 0 && terminal.DurationSeconds <= relaycommon.MaxTaskDurationSeconds {
		durationSeconds = terminal.DurationSeconds
	}
	if durationSeconds <= 0 || durationSeconds > relaycommon.MaxTaskDurationSeconds {
		return SeedanceTaskUsage{}, fmt.Errorf("output duration is out of range")
	}

	resolution := strings.TrimSpace(billingContext.Resolution)
	if terminal.ResolutionPresent {
		if _, ok := seedancepricing.Profile(terminal.Resolution); ok {
			resolution = terminal.Resolution
		}
	}
	profile, ok := seedancepricing.Profile(resolution)
	if !ok {
		return SeedanceTaskUsage{}, fmt.Errorf("output resolution is unsupported")
	}

	frameRate := profile.FrameRateNum
	if terminal.FramesPerSecondPresent && terminal.FramesPerSecond > 0 && terminal.FramesPerSecond <= maxSeedanceFrameRate {
		frameRate = int64(terminal.FramesPerSecond)
	}

	inputDurationMS := billingContext.InputVideoDurationMS
	if billingContext.HasVideoInput && inputDurationMS <= 0 {
		return SeedanceTaskUsage{}, fmt.Errorf("reference video duration is unavailable")
	}
	if inputDurationMS < 0 || inputDurationMS > int64(relaycommon.MaxTaskDurationSeconds)*1000 {
		return SeedanceTaskUsage{}, fmt.Errorf("reference video duration is out of range")
	}

	_, completionTokens, totalTokens, err := EstimateSeedanceTokens(ProfitRoutingFacts{
		OutputDurationSeconds: durationSeconds,
		InputDurationMS:       inputDurationMS,
		Width:                 profile.Width,
		Height:                profile.Height,
		FrameRateNum:          frameRate,
		FrameRateDen:          1,
	})
	if err != nil {
		return SeedanceTaskUsage{}, err
	}
	return SeedanceTaskUsage{
		CompletionTokens: int(completionTokens),
		TotalTokens:      int(totalTokens),
	}, nil
}
