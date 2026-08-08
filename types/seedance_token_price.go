package types

import (
	"fmt"
	"strings"

	"github.com/shopspring/decimal"
)

const (
	SeedanceTokenScenarioNoVideo   = "no_video"
	SeedanceTokenScenarioWithVideo = "with_video"
)

type SeedanceTokenPriceScenario struct {
	PricePerMillion string `json:"price_per_million"`
	Width           int    `json:"width"`
	Height          int    `json:"height"`
	FrameRate       int    `json:"frame_rate"`
	PricingVersion  string `json:"pricing_version"`
	Source          string `json:"source"`
}

type SeedanceTokenPrice struct {
	Scenarios map[string]SeedanceTokenPriceScenario `json:"scenarios"`
}

type SeedanceTokenUsage struct {
	InputTokens  int
	OutputTokens int
	TotalTokens  int
}

type SeedanceTokenCharge struct {
	Scenario              string
	Resolution            string
	PricePerMillion       decimal.Decimal
	InputTokens           int
	OutputTokens          int
	TotalTokens           int
	InputVideoDurationMS  int64
	OutputDurationSeconds int
	Width                 int
	Height                int
	FrameRate             int
	PricingVersion        string
	Source                string
	BaseCharge            decimal.Decimal
}

type SeedanceTokenBillingBreakdown struct {
	Scenario              string `json:"scenario"`
	Resolution            string `json:"resolution"`
	PricePerMillion       string `json:"price_per_million"`
	InputTokens           int    `json:"input_tokens"`
	OutputTokens          int    `json:"output_tokens"`
	TotalTokens           int    `json:"total_tokens"`
	InputVideoDurationMS  int64  `json:"input_video_duration_ms,omitempty"`
	OutputDurationSeconds int    `json:"output_duration_seconds"`
	Width                 int    `json:"width"`
	Height                int    `json:"height"`
	FrameRate             int    `json:"frame_rate"`
	PricingVersion        string `json:"pricing_version"`
	Source                string `json:"source"`
	BaseCharge            string `json:"base_charge"`
	GroupRatio            string `json:"group_ratio,omitempty"`
	FinalCharge           string `json:"final_charge,omitempty"`
}

func SeedanceTokenScenarioKey(resolution, scenario string) string {
	return strings.ToLower(strings.TrimSpace(resolution)) + ":" + strings.ToLower(strings.TrimSpace(scenario))
}

func (p SeedanceTokenPrice) Validate(maxTokens int) error {
	if len(p.Scenarios) == 0 {
		return fmt.Errorf("Seedance token price scenarios are required")
	}
	for key, scenario := range p.Scenarios {
		parts := strings.Split(key, ":")
		if len(parts) != 2 || parts[0] == "" ||
			(parts[1] != SeedanceTokenScenarioNoVideo && parts[1] != SeedanceTokenScenarioWithVideo) {
			return fmt.Errorf("Seedance token price scenario %q is invalid", key)
		}
		if key != SeedanceTokenScenarioKey(parts[0], parts[1]) {
			return fmt.Errorf("Seedance token price scenario %q must use a normalized key", key)
		}
		price, err := decimal.NewFromString(strings.TrimSpace(scenario.PricePerMillion))
		if err != nil || !price.IsPositive() {
			return fmt.Errorf("Seedance token price scenario %q price_per_million must be a positive decimal", key)
		}
		if scenario.Width <= 0 || scenario.Height <= 0 || scenario.FrameRate <= 0 {
			return fmt.Errorf("Seedance token price scenario %q output geometry is invalid", key)
		}
		if strings.TrimSpace(scenario.PricingVersion) == "" || strings.TrimSpace(scenario.Source) == "" {
			return fmt.Errorf("Seedance token price scenario %q audit metadata is required", key)
		}
	}
	if maxTokens <= 0 {
		return fmt.Errorf("Seedance token price max token limit must be positive")
	}
	return nil
}

func (p SeedanceTokenPrice) ScenarioFor(resolution string, hasVideoInput bool) (SeedanceTokenPriceScenario, bool) {
	scenario := SeedanceTokenScenarioNoVideo
	if hasVideoInput {
		scenario = SeedanceTokenScenarioWithVideo
	}
	value, ok := p.Scenarios[SeedanceTokenScenarioKey(resolution, scenario)]
	return value, ok
}

func (p SeedanceTokenPrice) CalculateCharge(
	resolution string,
	hasVideoInput bool,
	inputVideoDurationMS int64,
	outputDurationSeconds int,
	usage SeedanceTokenUsage,
	maxTokens int,
) (SeedanceTokenCharge, error) {
	if err := p.Validate(maxTokens); err != nil {
		return SeedanceTokenCharge{}, err
	}
	if outputDurationSeconds <= 0 {
		return SeedanceTokenCharge{}, fmt.Errorf("Seedance output duration must be positive")
	}
	if hasVideoInput && inputVideoDurationMS <= 0 {
		return SeedanceTokenCharge{}, fmt.Errorf("Seedance input video duration is required")
	}
	if !hasVideoInput && inputVideoDurationMS != 0 {
		return SeedanceTokenCharge{}, fmt.Errorf("Seedance input video duration is present for a no-video scenario")
	}
	if usage.InputTokens != 0 || usage.OutputTokens <= 0 || usage.TotalTokens > maxTokens ||
		usage.OutputTokens != usage.TotalTokens {
		return SeedanceTokenCharge{}, fmt.Errorf("Seedance token usage is invalid")
	}

	scenarioName := SeedanceTokenScenarioNoVideo
	if hasVideoInput {
		scenarioName = SeedanceTokenScenarioWithVideo
	}
	scenario, ok := p.ScenarioFor(resolution, hasVideoInput)
	if !ok {
		return SeedanceTokenCharge{}, fmt.Errorf("Seedance token price scenario is unavailable for resolution %s and video_input=%t", resolution, hasVideoInput)
	}
	pricePerMillion, _ := decimal.NewFromString(strings.TrimSpace(scenario.PricePerMillion))
	baseCharge := pricePerMillion.
		Mul(decimal.NewFromInt(int64(usage.TotalTokens))).
		Shift(-6)
	return SeedanceTokenCharge{
		Scenario:              scenarioName,
		Resolution:            strings.ToLower(strings.TrimSpace(resolution)),
		PricePerMillion:       pricePerMillion,
		InputTokens:           usage.InputTokens,
		OutputTokens:          usage.OutputTokens,
		TotalTokens:           usage.TotalTokens,
		InputVideoDurationMS:  inputVideoDurationMS,
		OutputDurationSeconds: outputDurationSeconds,
		Width:                 scenario.Width,
		Height:                scenario.Height,
		FrameRate:             scenario.FrameRate,
		PricingVersion:        scenario.PricingVersion,
		Source:                scenario.Source,
		BaseCharge:            baseCharge,
	}, nil
}

func (charge SeedanceTokenCharge) Breakdown() *SeedanceTokenBillingBreakdown {
	if charge.Scenario == "" {
		return nil
	}
	return &SeedanceTokenBillingBreakdown{
		Scenario:              charge.Scenario,
		Resolution:            charge.Resolution,
		PricePerMillion:       charge.PricePerMillion.String(),
		InputTokens:           charge.InputTokens,
		OutputTokens:          charge.OutputTokens,
		TotalTokens:           charge.TotalTokens,
		InputVideoDurationMS:  charge.InputVideoDurationMS,
		OutputDurationSeconds: charge.OutputDurationSeconds,
		Width:                 charge.Width,
		Height:                charge.Height,
		FrameRate:             charge.FrameRate,
		PricingVersion:        charge.PricingVersion,
		Source:                charge.Source,
		BaseCharge:            charge.BaseCharge.String(),
	}
}
