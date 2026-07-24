package service

import (
	"math"
	"testing"

	"github.com/QuantumNous/new-api/types"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func stringPointer(value string) *string {
	return &value
}

func TestDecimalToNanoUSD(t *testing.T) {
	value, err := DecimalToNanoUSD(decimal.RequireFromString("1.0000000005"))
	require.NoError(t, err)
	assert.Equal(t, int64(1_000_000_001), value)

	value, err = DecimalToNanoUSD(decimal.RequireFromString("-1.0000000005"))
	require.NoError(t, err)
	assert.Equal(t, int64(-1_000_000_001), value)

	_, err = DecimalToNanoUSD(decimal.RequireFromString("9223372036.854775808"))
	assert.ErrorIs(t, err, ErrNanoUSDOverflow)

	_, err = DecimalToNanoUSD(decimal.RequireFromString("-9223372036.854775809"))
	assert.ErrorIs(t, err, ErrNanoUSDOverflow)
}

func TestNormalizeRulePrice(t *testing.T) {
	unitPrice := "10"
	config := types.CostRuleConfigV1{
		Currency: "CNY", BillingMultiplier: "1.2",
		PurchaseDiscountRatio: "0.8", RechargeExchangeRatio: "2",
		FeeRate: "0.05", CurrencyToUSDRate: "0.14",
		UnitPrice: &unitPrice,
	}
	normalized, err := NormalizeCostRuleConfig(types.CostModePerRequest, config)
	require.NoError(t, err)
	assert.Equal(t, "0.7056", *normalized.NormalizedUSDPrices.UnitPrice)
}

func TestNormalizeCostRuleConfigCanonicalizesDecimals(t *testing.T) {
	config := types.CostRuleConfigV1{
		Currency:              "usd",
		BillingMultiplier:     "01.2000",
		PurchaseDiscountRatio: "1.000",
		RechargeExchangeRatio: "2.00",
		FeeRate:               "0.0500",
		CurrencyToUSDRate:     "1.0000",
		UnitPrice:             stringPointer("10.000"),
		ChargeEvent:           types.CostChargeResponseSucceeded,
	}

	normalized, err := NormalizeCostRuleConfig(types.CostModePerRequest, config)
	require.NoError(t, err)
	assert.Equal(t, "USD", normalized.Currency)
	assert.Equal(t, "1.2", normalized.BillingMultiplier)
	assert.Equal(t, "1", normalized.PurchaseDiscountRatio)
	assert.Equal(t, "2", normalized.RechargeExchangeRatio)
	assert.Equal(t, "0.05", normalized.FeeRate)
	assert.Equal(t, "1", normalized.CurrencyToUSDRate)
	assert.Equal(t, "10", *normalized.UnitPrice)
	assert.Equal(t, "6.3", *normalized.NormalizedUSDPrices.UnitPrice)
}

func TestNormalizeCostRuleConfigRejectsNegativePrice(t *testing.T) {
	config := types.CostRuleConfigV1{
		Currency: "USD", BillingMultiplier: "1",
		PurchaseDiscountRatio: "1", RechargeExchangeRatio: "1",
		FeeRate: "0", CurrencyToUSDRate: "1",
		UnitPrice: stringPointer("-0.01"),
	}

	_, err := NormalizeCostRuleConfig(types.CostModePerRequest, config)
	require.Error(t, err)
}

func TestNormalizeCostRuleConfigRejectsUnusedPrices(t *testing.T) {
	tests := []struct {
		name   string
		mode   types.CostMode
		config types.CostRuleConfigV1
	}{
		{
			name: "free rule with a unit price",
			mode: types.CostModeFree,
			config: types.CostRuleConfigV1{
				ZeroCostReason: "provider promotion",
				UnitPrice:      stringPointer("0"),
			},
		},
		{
			name: "per-request rule with a duration price",
			mode: types.CostModePerRequest,
			config: types.CostRuleConfigV1{
				Currency: "USD", BillingMultiplier: "1",
				PurchaseDiscountRatio: "1", RechargeExchangeRatio: "1",
				FeeRate: "0", CurrencyToUSDRate: "1",
				UnitPrice:      stringPointer("1"),
				PricePerSecond: stringPointer("1"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NormalizeCostRuleConfig(tt.mode, tt.config)
			require.Error(t, err)
		})
	}
}

func TestCalculateAttemptCostSupportsAllModes(t *testing.T) {
	tests := []struct {
		name         string
		mode         types.CostMode
		config       types.CostRuleConfigV1
		meter        types.CostMeter
		originalCost string
		nanoUSD      int64
	}{
		{
			name: "free",
			mode: types.CostModeFree,
			config: types.CostRuleConfigV1{
				ZeroCostReason: "provider promotion",
			},
			originalCost: "0",
		},
		{
			name: "per request",
			mode: types.CostModePerRequest,
			config: normalizedUSDConfig(types.CostRuleConfigV1{
				UnitPrice: stringPointer("0.25"),
			}),
			originalCost: "0.25",
			nanoUSD:      250_000_000,
		},
		{
			name: "per duration explicit zero",
			mode: types.CostModePerDuration,
			config: normalizedUSDConfig(types.CostRuleConfigV1{
				PricePerSecond: stringPointer("0.2"),
			}),
			meter: types.CostMeter{
				DurationSeconds: stringPointer("0"),
			},
			originalCost: "0",
		},
		{
			name: "per total tokens",
			mode: types.CostModePerToken,
			config: normalizedUSDConfig(types.CostRuleConfigV1{
				TokenMode:       types.CostTokenModeTotal,
				TotalPerMillion: stringPointer("2"),
			}),
			meter: types.CostMeter{
				TotalTokens: int64Pointer(250_000),
			},
			originalCost: "0.5",
			nanoUSD:      500_000_000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalCost, nanoUSD, err := CalculateAttemptCost(tt.mode, tt.config, tt.meter)
			require.NoError(t, err)
			assert.Equal(t, tt.originalCost, originalCost)
			assert.Equal(t, tt.nanoUSD, nanoUSD)
		})
	}
}

func TestCalculateAttemptCostRequiresDeclaredMeter(t *testing.T) {
	config := normalizedUSDConfig(types.CostRuleConfigV1{
		PricePerSecond: stringPointer("0.2"),
	})

	_, _, err := CalculateAttemptCost(types.CostModePerDuration, config, types.CostMeter{})
	require.Error(t, err)
}

func TestCalculateAttemptCostSupportsTokenSubmodes(t *testing.T) {
	tests := []struct {
		name         string
		config       types.CostRuleConfigV1
		meter        types.CostMeter
		originalCost string
		nanoUSD      int64
	}{
		{
			name: "completion",
			config: normalizedUSDConfig(types.CostRuleConfigV1{
				TokenMode:            types.CostTokenModeCompletion,
				CompletionPerMillion: stringPointer("3"),
			}),
			meter: types.CostMeter{
				CompletionTokens: int64Pointer(200_000),
			},
			originalCost: "0.6",
			nanoUSD:      600_000_000,
		},
		{
			name: "input and output",
			config: normalizedUSDConfig(types.CostRuleConfigV1{
				TokenMode:        types.CostTokenModeInputOutput,
				InputPerMillion:  stringPointer("1"),
				OutputPerMillion: stringPointer("4"),
			}),
			meter: types.CostMeter{
				InputTokens:  int64Pointer(500_000),
				OutputTokens: int64Pointer(250_000),
			},
			originalCost: "1.5",
			nanoUSD:      1_500_000_000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalCost, nanoUSD, err := CalculateAttemptCost(types.CostModePerToken, tt.config, tt.meter)
			require.NoError(t, err)
			assert.Equal(t, tt.originalCost, originalCost)
			assert.Equal(t, tt.nanoUSD, nanoUSD)
		})
	}
}

func TestCalculateAttemptCostRejectsNegativeMeter(t *testing.T) {
	config := normalizedUSDConfig(types.CostRuleConfigV1{
		TokenMode:       types.CostTokenModeTotal,
		TotalPerMillion: stringPointer("1"),
	})

	_, _, err := CalculateAttemptCost(types.CostModePerToken, config, types.CostMeter{
		TotalTokens: int64Pointer(-1),
	})
	require.Error(t, err)
}

func TestRevenueEquivalentNanoUSD(t *testing.T) {
	got, err := RevenueEquivalentNanoUSD(500_000, "500000")
	require.NoError(t, err)
	assert.Equal(t, int64(1_000_000_000), got)
	_, err = RevenueEquivalentNanoUSD(-1, "500000")
	assert.Error(t, err)
}

func TestRevenueEquivalentNanoUSDRejectsInvalidQuotaUnit(t *testing.T) {
	for _, quotaPerUnit := range []string{"0", "-1", "invalid"} {
		t.Run(quotaPerUnit, func(t *testing.T) {
			_, err := RevenueEquivalentNanoUSD(1, quotaPerUnit)
			require.Error(t, err)
		})
	}
}

func TestGrossMarginPPM(t *testing.T) {
	margin, err := GrossMarginPPM(-500_000_000, 1_000_000_000)
	require.NoError(t, err)
	require.NotNil(t, margin)
	assert.Equal(t, int64(-500_000), *margin)

	margin, err = GrossMarginPPM(0, 0)
	require.NoError(t, err)
	assert.Nil(t, margin)

	_, err = GrossMarginPPM(1, -1)
	require.Error(t, err)
}

func TestCheckedNanoArithmetic(t *testing.T) {
	got, err := CheckedNanoAdd(-5, 3)
	require.NoError(t, err)
	assert.Equal(t, int64(-2), got)

	got, err = CheckedNanoSubtract(3, 5)
	require.NoError(t, err)
	assert.Equal(t, int64(-2), got)

	_, err = CheckedNanoAdd(math.MaxInt64, 1)
	assert.ErrorIs(t, err, ErrNanoUSDOverflow)

	_, err = CheckedNanoSubtract(math.MinInt64, 1)
	assert.ErrorIs(t, err, ErrNanoUSDOverflow)
}

func normalizedUSDConfig(config types.CostRuleConfigV1) types.CostRuleConfigV1 {
	config.Currency = "USD"
	config.BillingMultiplier = "1"
	config.PurchaseDiscountRatio = "1"
	config.RechargeExchangeRatio = "1"
	config.FeeRate = "0"
	config.CurrencyToUSDRate = "1"
	config.NormalizedUSDPrices = types.CostRulePricesV1{
		UnitPrice:            config.UnitPrice,
		PricePerSecond:       config.PricePerSecond,
		TotalPerMillion:      config.TotalPerMillion,
		CompletionPerMillion: config.CompletionPerMillion,
		InputPerMillion:      config.InputPerMillion,
		OutputPerMillion:     config.OutputPerMillion,
	}
	return config
}

func int64Pointer(value int64) *int64 {
	return &value
}
