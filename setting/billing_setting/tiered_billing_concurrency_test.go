package billing_setting

import (
	"testing"

	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConcurrentBillingConfigUpdateAndRead(t *testing.T) {
	original, err := config.ConfigToMap(&billingSetting)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, config.UpdateConfigFromMap(&billingSetting, original))
	})

	require.NoError(t, config.UpdateConfigFromMap(&billingSetting, map[string]string{
		BillingModeField:   `{"race-model":"ratio"}`,
		BillingExprField:   `{"race-model":"old expression"}`,
		DurationPriceField: `{"race-model":{"price":1,"unit":"second","rounding_step_seconds":1,"minimum_duration_seconds":0}}`,
	}))

	start := make(chan struct{})
	ready := make(chan struct{}, 2)
	updateDone := make(chan error, 1)
	type readResult struct {
		mode        string
		expr        string
		hasExpr     bool
		price       types.DurationPrice
		hasPrice    bool
		modes       map[string]string
		exprs       map[string]string
		prices      map[string]types.DurationPrice
		pricingSync map[string]any
	}
	readDone := make(chan readResult, 1)

	go func() {
		ready <- struct{}{}
		<-start
		updateDone <- config.UpdateConfigFromMap(&billingSetting, map[string]string{
			BillingModeField:   `{"race-model":"tiered_expr"}`,
			BillingExprField:   `{"race-model":"new expression"}`,
			DurationPriceField: `{"race-model":{"price":2,"unit":"minute","rounding_step_seconds":5,"minimum_duration_seconds":10}}`,
		})
	}()
	go func() {
		ready <- struct{}{}
		<-start
		mode := GetBillingMode("race-model")
		expr, hasExpr := GetBillingExpr("race-model")
		price, hasPrice := GetDurationPrice("race-model")
		readDone <- readResult{
			mode:        mode,
			expr:        expr,
			hasExpr:     hasExpr,
			price:       price,
			hasPrice:    hasPrice,
			modes:       GetBillingModeCopy(),
			exprs:       GetBillingExprCopy(),
			prices:      GetDurationPriceCopy(),
			pricingSync: GetPricingSyncData(map[string]any{"base": true}),
		}
	}()

	<-ready
	<-ready
	close(start)
	require.NoError(t, <-updateDone)
	result := <-readDone

	assert.Contains(t, []string{BillingModeRatio, BillingModeTieredExpr}, result.mode)
	assert.Contains(t, []string{"old expression", "new expression"}, result.expr)
	assert.True(t, result.hasExpr)
	assert.Contains(t, []float64{1, 2}, result.price.Price)
	assert.True(t, result.hasPrice)
	assert.Contains(t, result.modes, "race-model")
	assert.Contains(t, result.exprs, "race-model")
	assert.Contains(t, result.prices, "race-model")
	assert.Equal(t, true, result.pricingSync["base"])
	assert.Equal(t, BillingModePerDuration, result.modes["jimeng-video-seedance-2.0-vip"])
}

func TestBillingConfigNullKeepsMapsUsable(t *testing.T) {
	original, err := config.ConfigToMap(&billingSetting)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, config.UpdateConfigFromMap(&billingSetting, original))
	})

	require.NoError(t, config.UpdateConfigFromMap(&billingSetting, map[string]string{
		BillingModeField:   `{"boundary-model":"tiered_expr"}`,
		BillingExprField:   `{"boundary-model":"old expression"}`,
		DurationPriceField: `{"boundary-model":{"price":1,"unit":"second","rounding_step_seconds":1,"minimum_duration_seconds":0}}`,
	}))
	require.NoError(t, config.UpdateConfigFromMap(&billingSetting, map[string]string{
		BillingModeField:   `null`,
		BillingExprField:   `null`,
		DurationPriceField: `null`,
	}))

	assert.NotNil(t, billingSetting.BillingMode)
	assert.NotNil(t, billingSetting.BillingExpr)
	assert.NotNil(t, billingSetting.DurationPrice)
	assert.NotPanics(t, func() {
		assert.Equal(t, BillingModeRatio, GetBillingMode("boundary-model"))
		_, hasExpr := GetBillingExpr("boundary-model")
		assert.False(t, hasExpr)
		_, hasPrice := GetDurationPrice("boundary-model")
		assert.False(t, hasPrice)
		assert.Contains(t, GetBillingModeCopy(), "jimeng-video-seedance-2.0-vip")
		assert.Empty(t, GetBillingExprCopy())
		assert.Contains(t, GetDurationPriceCopy(), "jimeng-video-seedance-2.0-vip")
	})
}

func TestBillingConfigInvalidJSONPreservesValues(t *testing.T) {
	original, err := config.ConfigToMap(&billingSetting)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, config.UpdateConfigFromMap(&billingSetting, original))
	})

	require.NoError(t, config.UpdateConfigFromMap(&billingSetting, map[string]string{
		BillingModeField:   `{"boundary-model":"tiered_expr"}`,
		BillingExprField:   `{"boundary-model":"stable expression"}`,
		DurationPriceField: `{"boundary-model":{"price":3,"unit":"minute","rounding_step_seconds":5,"minimum_duration_seconds":10}}`,
	}))
	require.NoError(t, config.UpdateConfigFromMap(&billingSetting, map[string]string{
		BillingModeField:   `[]`,
		BillingExprField:   `[]`,
		DurationPriceField: `[]`,
	}))

	assert.Equal(t, BillingModeTieredExpr, GetBillingMode("boundary-model"))
	expr, hasExpr := GetBillingExpr("boundary-model")
	require.True(t, hasExpr)
	assert.Equal(t, "stable expression", expr)
	price, hasPrice := GetDurationPrice("boundary-model")
	require.True(t, hasPrice)
	assert.Equal(t, 3.0, price.Price)
}
