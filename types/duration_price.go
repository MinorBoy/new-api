package types

import (
	"fmt"
	"math"
	"strings"

	"github.com/shopspring/decimal"
)

const (
	DurationUnitSecond        = "second"
	DurationUnitMinute        = "minute"
	DurationSourceRequest     = "request"
	DurationScenarioNoVideo   = "no_video"
	DurationScenarioWithVideo = "with_video"
)

type DurationPriceScenario struct {
	OutputPrice            float64 `json:"output_price"`
	Unit                   string  `json:"unit"`
	RoundingStepSeconds    int     `json:"rounding_step_seconds"`
	MinimumDurationSeconds int     `json:"minimum_duration_seconds"`
	PricingVersion         string  `json:"pricing_version,omitempty"`
	Source                 string  `json:"source,omitempty"`
}

type DurationBillingBreakdown struct {
	Scenario               string `json:"scenario"`
	Resolution             string `json:"resolution"`
	PricingVersion         string `json:"pricing_version,omitempty"`
	Source                 string `json:"source,omitempty"`
	OutputSeconds          int    `json:"output_seconds"`
	BillableOutputSeconds  int    `json:"billable_output_seconds"`
	InputVideoDurationMS   int64  `json:"input_video_duration_ms,omitempty"`
	RoundingStepSeconds    int    `json:"rounding_step_seconds"`
	MinimumDurationSeconds int    `json:"minimum_duration_seconds"`
	OutputPricePerSecond   string `json:"output_price_per_second"`
	OutputCharge           string `json:"output_charge"`
	TotalCharge            string `json:"total_charge"`
	GroupRatio             string `json:"group_ratio,omitempty"`
	FinalCharge            string `json:"final_charge,omitempty"`
}

type DurationCharge struct {
	OutputSeconds          int
	BillableOutputSeconds  int
	InputVideoDurationMS   int64
	OutputPricePerSecond   decimal.Decimal
	OutputCharge           decimal.Decimal
	TotalCharge            decimal.Decimal
	Scenario               string
	Resolution             string
	PricingVersion         string
	Source                 string
	RoundingStepSeconds    int
	MinimumDurationSeconds int
}

func DurationScenarioKey(resolution, scenario string) string {
	return strings.ToLower(strings.TrimSpace(resolution)) + ":" + strings.ToLower(strings.TrimSpace(scenario))
}

type DurationPrice struct {
	Price                  float64                          `json:"price,omitempty"`
	Unit                   string                           `json:"unit"`
	RoundingStepSeconds    int                              `json:"rounding_step_seconds"`
	MinimumDurationSeconds int                              `json:"minimum_duration_seconds"`
	Scenarios              map[string]DurationPriceScenario `json:"scenarios,omitempty"`
}

func (p DurationPrice) Validate(maxSeconds int) error {
	if len(p.Scenarios) == 0 {
		if p.Price < 0 || math.IsNaN(p.Price) || math.IsInf(p.Price, 0) {
			return fmt.Errorf("duration price must be a finite non-negative number")
		}
		if p.Unit != DurationUnitSecond && p.Unit != DurationUnitMinute {
			return fmt.Errorf("duration unit must be second or minute")
		}
		if p.RoundingStepSeconds <= 0 || p.RoundingStepSeconds > maxSeconds {
			return fmt.Errorf("rounding_step_seconds must be between 1 and %d", maxSeconds)
		}
		if p.MinimumDurationSeconds < 0 || p.MinimumDurationSeconds > maxSeconds {
			return fmt.Errorf("minimum_duration_seconds must be between 0 and %d", maxSeconds)
		}
	}
	for key, scenario := range p.Scenarios {
		parts := strings.Split(key, ":")
		if len(parts) != 2 || parts[0] == "" || (parts[1] != DurationScenarioNoVideo && parts[1] != DurationScenarioWithVideo) {
			return fmt.Errorf("duration pricing scenario %q is invalid", key)
		}
		if key != DurationScenarioKey(parts[0], parts[1]) {
			return fmt.Errorf("duration pricing scenario %q must use a normalized key", key)
		}
		if err := scenario.validate(maxSeconds); err != nil {
			return fmt.Errorf("duration pricing scenario %q: %w", key, err)
		}
	}
	return nil
}

func (p DurationPriceScenario) validate(maxSeconds int) error {
	if p.OutputPrice < 0 || math.IsNaN(p.OutputPrice) || math.IsInf(p.OutputPrice, 0) {
		return fmt.Errorf("output price must be a finite non-negative number")
	}
	if p.Unit != DurationUnitSecond && p.Unit != DurationUnitMinute {
		return fmt.Errorf("unit must be second or minute")
	}
	if p.RoundingStepSeconds <= 0 || p.RoundingStepSeconds > maxSeconds {
		return fmt.Errorf("rounding_step_seconds must be between 1 and %d", maxSeconds)
	}
	if p.MinimumDurationSeconds < 0 || p.MinimumDurationSeconds > maxSeconds {
		return fmt.Errorf("minimum_duration_seconds must be between 0 and %d", maxSeconds)
	}
	if strings.TrimSpace(p.PricingVersion) == "" {
		return fmt.Errorf("pricing_version is required")
	}
	if strings.TrimSpace(p.Source) == "" {
		return fmt.Errorf("source is required")
	}
	return nil
}

func (p DurationPrice) ScenarioFor(resolution string, hasVideoInput bool) (DurationPriceScenario, bool) {
	if len(p.Scenarios) == 0 {
		return DurationPriceScenario{}, false
	}
	scenario := DurationScenarioNoVideo
	if hasVideoInput {
		scenario = DurationScenarioWithVideo
	}
	value, ok := p.Scenarios[DurationScenarioKey(resolution, scenario)]
	return value, ok
}

func (p DurationPrice) CalculateCharge(requestedSeconds int, resolution string, hasVideoInput bool, inputVideoDurationMS int64, maxSeconds int) (DurationCharge, error) {
	if err := p.Validate(maxSeconds); err != nil {
		return DurationCharge{}, err
	}
	charge := DurationCharge{OutputSeconds: requestedSeconds, InputVideoDurationMS: inputVideoDurationMS}
	outputUnitPrice := decimal.Zero
	if len(p.Scenarios) > 0 {
		scenarioName := DurationScenarioNoVideo
		if hasVideoInput {
			scenarioName = DurationScenarioWithVideo
		}
		scenario, ok := p.ScenarioFor(resolution, hasVideoInput)
		if !ok {
			return DurationCharge{}, fmt.Errorf("explicit duration pricing scenario is unavailable for resolution %s and video_input=%t", resolution, hasVideoInput)
		}
		billableSeconds, err := (DurationPrice{
			Price:                  scenario.OutputPrice,
			Unit:                   scenario.Unit,
			RoundingStepSeconds:    scenario.RoundingStepSeconds,
			MinimumDurationSeconds: scenario.MinimumDurationSeconds,
		}).BillableSeconds(requestedSeconds, maxSeconds)
		if err != nil {
			return DurationCharge{}, err
		}
		charge.BillableOutputSeconds = billableSeconds
		outputUnitPrice = decimal.NewFromFloat(scenario.OutputPrice).Div(decimal.NewFromInt(int64(scenario.UnitSeconds())))
		charge.Scenario = scenarioName
		charge.Resolution = strings.ToLower(strings.TrimSpace(resolution))
		charge.PricingVersion = scenario.PricingVersion
		charge.Source = scenario.Source
		charge.RoundingStepSeconds = scenario.RoundingStepSeconds
		charge.MinimumDurationSeconds = scenario.MinimumDurationSeconds
		if hasVideoInput && (inputVideoDurationMS <= 0 || inputVideoDurationMS > int64(maxSeconds)*1000) {
			return DurationCharge{}, fmt.Errorf("reference video duration must be between 1 ms and %d seconds", maxSeconds)
		}
		if !hasVideoInput && inputVideoDurationMS != 0 {
			return DurationCharge{}, fmt.Errorf("input video duration is present for a no-video scenario")
		}
	} else if inputVideoDurationMS != 0 || hasVideoInput {
		return DurationCharge{}, fmt.Errorf("explicit duration pricing scenario is unavailable for video input")
	} else {
		billableSeconds, err := p.BillableSeconds(requestedSeconds, maxSeconds)
		if err != nil {
			return DurationCharge{}, err
		}
		charge.BillableOutputSeconds = billableSeconds
		outputUnitPrice = decimal.NewFromFloat(p.Price).Div(decimal.NewFromInt(int64(p.UnitSeconds())))
	}
	charge.OutputPricePerSecond = outputUnitPrice
	charge.OutputCharge = outputUnitPrice.Mul(decimal.NewFromInt(int64(charge.BillableOutputSeconds)))
	charge.TotalCharge = charge.OutputCharge
	return charge, nil
}

func (p DurationPriceScenario) UnitSeconds() int {
	if p.Unit == DurationUnitMinute {
		return 60
	}
	return 1
}

func (charge DurationCharge) Breakdown() *DurationBillingBreakdown {
	if charge.Scenario == "" {
		return nil
	}
	breakdown := &DurationBillingBreakdown{
		Scenario:               charge.Scenario,
		Resolution:             charge.Resolution,
		PricingVersion:         charge.PricingVersion,
		Source:                 charge.Source,
		OutputSeconds:          charge.OutputSeconds,
		BillableOutputSeconds:  charge.BillableOutputSeconds,
		InputVideoDurationMS:   charge.InputVideoDurationMS,
		RoundingStepSeconds:    charge.RoundingStepSeconds,
		MinimumDurationSeconds: charge.MinimumDurationSeconds,
		OutputPricePerSecond:   charge.OutputPricePerSecond.String(),
		OutputCharge:           charge.OutputCharge.String(),
		TotalCharge:            charge.TotalCharge.String(),
	}
	return breakdown
}

func (p DurationPrice) UnitSeconds() int {
	if p.Unit == DurationUnitMinute {
		return 60
	}
	return 1
}

func (p DurationPrice) BillableSeconds(requested, maxSeconds int) (int, error) {
	if err := p.Validate(maxSeconds); err != nil {
		return 0, err
	}
	if requested <= 0 || requested > maxSeconds {
		return 0, fmt.Errorf("requested duration must be between 1 and %d seconds", maxSeconds)
	}

	normalized := requested
	if normalized < p.MinimumDurationSeconds {
		normalized = p.MinimumDurationSeconds
	}
	return ((normalized + p.RoundingStepSeconds - 1) / p.RoundingStepSeconds) * p.RoundingStepSeconds, nil
}
