package types

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDurationPriceValidate(t *testing.T) {
	valid := DurationPrice{
		Price:                  0.5,
		Unit:                   DurationUnitMinute,
		RoundingStepSeconds:    5,
		MinimumDurationSeconds: 4,
	}
	require.NoError(t, valid.Validate(3600))

	tests := map[string]DurationPrice{
		"negative price": {
			Price: -1, Unit: DurationUnitSecond, RoundingStepSeconds: 1,
		},
		"nan price": {
			Price: math.NaN(), Unit: DurationUnitSecond, RoundingStepSeconds: 1,
		},
		"infinite price": {
			Price: math.Inf(1), Unit: DurationUnitSecond, RoundingStepSeconds: 1,
		},
		"bad unit": {
			Price: 1, Unit: "hour", RoundingStepSeconds: 1,
		},
		"zero step": {
			Price: 1, Unit: DurationUnitSecond,
		},
		"large step": {
			Price: 1, Unit: DurationUnitSecond, RoundingStepSeconds: 3601,
		},
		"negative minimum": {
			Price: 1, Unit: DurationUnitSecond, RoundingStepSeconds: 1, MinimumDurationSeconds: -1,
		},
		"large minimum": {
			Price: 1, Unit: DurationUnitSecond, RoundingStepSeconds: 1, MinimumDurationSeconds: 3601,
		},
	}
	for name, rule := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Error(t, rule.Validate(3600))
		})
	}
}

func TestDurationPriceBillableSeconds(t *testing.T) {
	rule := DurationPrice{
		Price:                  1,
		Unit:                   DurationUnitSecond,
		RoundingStepSeconds:    5,
		MinimumDurationSeconds: 4,
	}
	tests := []struct {
		requested int
		expected  int
	}{
		{requested: 1, expected: 5},
		{requested: 4, expected: 5},
		{requested: 5, expected: 5},
		{requested: 6, expected: 10},
		{requested: 11, expected: 15},
	}
	for _, test := range tests {
		actual, err := rule.BillableSeconds(test.requested, 3600)
		require.NoError(t, err)
		assert.Equal(t, test.expected, actual)
	}

	_, err := rule.BillableSeconds(3601, 3600)
	assert.Error(t, err)
}

func TestDurationPriceUnitSeconds(t *testing.T) {
	assert.Equal(t, 1, (DurationPrice{Unit: DurationUnitSecond}).UnitSeconds())
	assert.Equal(t, 60, (DurationPrice{Unit: DurationUnitMinute}).UnitSeconds())
}
