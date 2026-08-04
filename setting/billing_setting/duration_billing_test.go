package billing_setting

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSeedanceDurationPriceHasNoSupplierDefaults(t *testing.T) {
	originalModes := billingSetting.BillingMode
	originalPrices := billingSetting.DurationPrice
	t.Cleanup(func() {
		billingSetting.BillingMode = originalModes
		billingSetting.DurationPrice = originalPrices
	})
	billingSetting.BillingMode = types.NewRWMap[string, string]()
	billingSetting.DurationPrice = types.NewRWMap[string, types.DurationPrice]()

	for _, model := range []string{
		"jimeng-video-seedance-2.0-fast-vip",
		"jimeng-video-seedance-2.0-mini",
		"jimeng-video-seedance-2.0-vip",
	} {
		assert.Equal(t, BillingModeRatio, GetBillingMode(model))
		_, ok := GetDurationPrice(model)
		assert.False(t, ok)
		assert.NotContains(t, GetBillingModeCopy(), model)
		assert.NotContains(t, GetDurationPriceCopy(), model)
	}
}

func TestDurationPriceConfiguredRuleIsReturned(t *testing.T) {
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
	modelName := "configured-duration-model"
	originalPrices := billingSetting.DurationPrice
	t.Cleanup(func() {
		billingSetting.DurationPrice = originalPrices
	})
	billingSetting.DurationPrice = types.NewRWMap[string, types.DurationPrice]()
	billingSetting.DurationPrice.Set(modelName, types.DurationPrice{
		Price: 1, Unit: types.DurationUnitSecond, RoundingStepSeconds: 1,
	})

	prices := GetDurationPriceCopy()
	prices[modelName] = types.DurationPrice{Price: 99}
	rule, ok := GetDurationPrice(modelName)
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

func TestValidateDurationPriceJSONStringRejectsDirectSeedanceAddition(t *testing.T) {
	originalPrices := billingSetting.DurationPrice
	t.Cleanup(func() { billingSetting.DurationPrice = originalPrices })
	billingSetting.DurationPrice = types.NewRWMap[string, types.DurationPrice]()

	raw := `{"doubao-seedance-2-0-260128":{"price":1,"unit":"second","rounding_step_seconds":1,"minimum_duration_seconds":0}}`
	assert.ErrorContains(t, ValidateDurationPriceJSONString(raw), "does not support per_duration pricing")
}

func TestValidateDurationPriceJSONStringRejectsRemovedScenarioContract(t *testing.T) {
	raw := `{"video":{"price":1,"unit":"second","rounding_step_seconds":1,"minimum_duration_seconds":0,"scenarios":{"720p:no_video":{"output_price":9}}}}`
	assert.ErrorContains(t, ValidateDurationPriceJSONString(raw), "scenarios is no longer supported")
}

func TestValidateBillingModeJSONStringProtectsImportedSeedanceModes(t *testing.T) {
	originalModes := billingSetting.BillingMode
	t.Cleanup(func() { billingSetting.BillingMode = originalModes })

	modelName := "doubao-seedance-2-0-260128"
	billingSetting.BillingMode = types.NewRWMap[string, string]()
	billingSetting.BillingMode.Set(modelName, BillingModePerDuration)

	require.NoError(t, ValidateBillingModeJSONString(`{"doubao-seedance-2-0-260128":"per_duration","video":"ratio"}`))
	assert.ErrorContains(t, ValidateBillingModeJSONString(`{"doubao-seedance-2-0-260128":"ratio"}`), "must be updated through config import")
	assert.ErrorContains(t, ValidateBillingModeJSONString(`{"video":"ratio"}`), "cannot be removed outside config import")

	billingSetting.BillingMode = types.NewRWMap[string, string]()
	assert.ErrorContains(t, ValidateBillingModeJSONString(`{"doubao-seedance-2-0-260128":"per_duration"}`), "must be created through config import")
}

func TestDurationPriceJSONStringExcludesRemovedSupplierDefaults(t *testing.T) {
	originalPrices := billingSetting.DurationPrice
	t.Cleanup(func() {
		billingSetting.DurationPrice = originalPrices
	})
	billingSetting.DurationPrice = types.NewRWMap[string, types.DurationPrice]()

	var prices map[string]types.DurationPrice
	require.NoError(t, common.UnmarshalJsonStr(DurationPrice2JSONString(), &prices))
	assert.Empty(t, prices)
}
