package types

import (
	"fmt"
	"math"
)

const (
	DurationUnitSecond    = "second"
	DurationUnitMinute    = "minute"
	DurationSourceRequest = "request"
)

type DurationPrice struct {
	Price                  float64 `json:"price"`
	Unit                   string  `json:"unit"`
	RoundingStepSeconds    int     `json:"rounding_step_seconds"`
	MinimumDurationSeconds int     `json:"minimum_duration_seconds"`
}

func (p DurationPrice) Validate(maxSeconds int) error {
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
	return nil
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
