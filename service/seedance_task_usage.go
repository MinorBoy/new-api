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
