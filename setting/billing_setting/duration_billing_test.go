package billing_setting

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDimensioDurationPriceDefaults(t *testing.T) {
	tests := map[string]float64{
		"jimeng-video-seedance-2.0-fast-vip": 0.48 / 7.3,
		"jimeng-video-seedance-2.0-mini":     0.39 / 7.3,
		"jimeng-video-seedance-2.0-vip":      0.62 / 7.3,
	}
	for model, expected := range tests {
		assert.Equal(t, BillingModePerDuration, GetBillingMode(model))
		rule, ok := GetDurationPrice(model)
		require.True(t, ok)
		assert.InDelta(t, expected, rule.Price, 1e-10)
		assert.Equal(t, types.DurationUnitSecond, rule.Unit)
		assert.Equal(t, 1, rule.RoundingStepSeconds)
		assert.Equal(t, 4, rule.MinimumDurationSeconds)
	}
}

func TestDurationPriceConfiguredRuleOverridesDefault(t *testing.T) {
	modelName := "jimeng-video-seedance-2.0-vip"
	originalModes := billingSetting.BillingMode
	originalPrices := billingSetting.DurationPrice
	t.Cleanup(func() {
		billingSetting.BillingMode = originalModes
		billingSetting.DurationPrice = originalPrices
	})

	billingSetting.BillingMode = types.NewRWMap[string, string]()
	billingSetting.BillingMode.Set(modelName, BillingModeRatio)
	billingSetting.DurationPrice = types.NewRWMap[string, types.DurationPrice]()
	billingSetting.DurationPrice.Set(modelName, types.DurationPrice{
		Price: 9, Unit: types.DurationUnitMinute, RoundingStepSeconds: 60,
	})

	assert.Equal(t, BillingModeRatio, GetBillingMode(modelName))
	rule, ok := GetDurationPrice(modelName)
	require.True(t, ok)
	assert.Equal(t, 9.0, rule.Price)
}

func TestDurationPriceCopiesAreIndependent(t *testing.T) {
	prices := GetDurationPriceCopy()
	prices["jimeng-video-seedance-2.0-vip"] = types.DurationPrice{Price: 99}
	rule, ok := GetDurationPrice("jimeng-video-seedance-2.0-vip")
	require.True(t, ok)
	assert.NotEqual(t, 99.0, rule.Price)
}

func TestValidateDurationPriceJSONString(t *testing.T) {
	require.NoError(t, ValidateDurationPriceJSONString(`{"video":{"price":1.5,"unit":"minute","rounding_step_seconds":5,"minimum_duration_seconds":10}}`))

	invalid := []string{
		`[]`,
		`{"":{"price":1,"unit":"second","rounding_step_seconds":1,"minimum_duration_seconds":0}}`,
		`{"video":{"price":-1,"unit":"second","rounding_step_seconds":1,"minimum_duration_seconds":0}}`,
		`{"video":{"price":1,"unit":"hour","rounding_step_seconds":1,"minimum_duration_seconds":0}}`,
		`{"video":{"price":1,"unit":"second","rounding_step_seconds":0,"minimum_duration_seconds":0}}`,
	}
	for _, raw := range invalid {
		assert.Error(t, ValidateDurationPriceJSONString(raw))
	}
}

func TestDurationPriceJSONStringIncludesEffectiveDefaults(t *testing.T) {
	var prices map[string]types.DurationPrice
	require.NoError(t, common.UnmarshalJsonStr(DurationPrice2JSONString(), &prices))
	require.Contains(t, prices, "jimeng-video-seedance-2.0-vip")
	assert.InDelta(t, 0.62/7.3, prices["jimeng-video-seedance-2.0-vip"].Price, 1e-10)
}
