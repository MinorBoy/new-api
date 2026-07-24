package service

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/QuantumNous/new-api/types"
	"github.com/shopspring/decimal"
)

var (
	ErrNanoUSDOverflow   = errors.New("nano-USD amount exceeds int64 range")
	ErrInvalidCostAmount = errors.New("invalid cost amount")
)

var (
	nanoUSDScale    = decimal.NewFromInt(1_000_000_000)
	ppmScale        = decimal.NewFromInt(1_000_000)
	millionScale    = decimal.NewFromInt(1_000_000)
	maxInt64Decimal = decimal.NewFromInt(math.MaxInt64)
	minInt64Decimal = decimal.NewFromInt(math.MinInt64)
)

func NormalizeCostRuleConfig(mode types.CostMode, in types.CostRuleConfigV1) (types.CostRuleConfigV1, error) {
	if err := validateRulePriceShape(mode, in); err != nil {
		return types.CostRuleConfigV1{}, err
	}
	if mode == types.CostModeFree {
		if strings.TrimSpace(in.ZeroCostReason) == "" {
			return types.CostRuleConfigV1{}, fmt.Errorf("%w: zero-cost reason is required", ErrInvalidCostAmount)
		}
		in.ZeroCostReason = strings.TrimSpace(in.ZeroCostReason)
		in.NormalizedUSDPrices = types.CostRulePricesV1{}
		return in, nil
	}

	currency := strings.ToUpper(strings.TrimSpace(in.Currency))
	if currency == "" {
		return types.CostRuleConfigV1{}, fmt.Errorf("%w: currency is required", ErrInvalidCostAmount)
	}

	billingMultiplier, err := parsePositiveDecimal(in.BillingMultiplier, "billing multiplier", "1")
	if err != nil {
		return types.CostRuleConfigV1{}, err
	}
	purchaseDiscountRatio, err := parsePositiveDecimal(in.PurchaseDiscountRatio, "purchase discount ratio", "1")
	if err != nil {
		return types.CostRuleConfigV1{}, err
	}
	rechargeExchangeRatio, err := parsePositiveDecimal(in.RechargeExchangeRatio, "recharge exchange ratio", "1")
	if err != nil {
		return types.CostRuleConfigV1{}, err
	}
	feeRate, err := parseNonNegativeDecimal(in.FeeRate, "fee rate", "0")
	if err != nil {
		return types.CostRuleConfigV1{}, err
	}
	currencyToUSDRate, err := parsePositiveDecimal(in.CurrencyToUSDRate, "currency-to-USD rate", "")
	if err != nil {
		return types.CostRuleConfigV1{}, err
	}

	factor := billingMultiplier.
		Mul(purchaseDiscountRatio).
		Div(rechargeExchangeRatio).
		Mul(decimal.NewFromInt(1).Add(feeRate)).
		Mul(currencyToUSDRate)

	in.Currency = currency
	in.BillingMultiplier = billingMultiplier.String()
	in.PurchaseDiscountRatio = purchaseDiscountRatio.String()
	in.RechargeExchangeRatio = rechargeExchangeRatio.String()
	in.FeeRate = feeRate.String()
	in.CurrencyToUSDRate = currencyToUSDRate.String()
	in.NormalizedUSDPrices = types.CostRulePricesV1{}

	switch mode {
	case types.CostModePerRequest:
		in.UnitPrice, in.NormalizedUSDPrices.UnitPrice, err = normalizePrice(in.UnitPrice, factor, "unit price")
	case types.CostModePerDuration:
		in.PricePerSecond, in.NormalizedUSDPrices.PricePerSecond, err = normalizePrice(in.PricePerSecond, factor, "price per second")
	case types.CostModePerToken:
		switch in.TokenMode {
		case types.CostTokenModeTotal:
			in.TotalPerMillion, in.NormalizedUSDPrices.TotalPerMillion, err = normalizePrice(in.TotalPerMillion, factor, "total-token price")
		case types.CostTokenModeCompletion:
			in.CompletionPerMillion, in.NormalizedUSDPrices.CompletionPerMillion, err = normalizePrice(in.CompletionPerMillion, factor, "completion-token price")
		case types.CostTokenModeInputOutput:
			in.InputPerMillion, in.NormalizedUSDPrices.InputPerMillion, err = normalizePrice(in.InputPerMillion, factor, "input-token price")
			if err == nil {
				in.OutputPerMillion, in.NormalizedUSDPrices.OutputPerMillion, err = normalizePrice(in.OutputPerMillion, factor, "output-token price")
			}
		default:
			err = fmt.Errorf("%w: unsupported token mode %q", ErrInvalidCostAmount, in.TokenMode)
		}
	default:
		err = fmt.Errorf("%w: unsupported cost mode %q", ErrInvalidCostAmount, mode)
	}
	if err != nil {
		return types.CostRuleConfigV1{}, err
	}
	return in, nil
}

func validateRulePriceShape(mode types.CostMode, config types.CostRuleConfigV1) error {
	var hasUnusedPrice bool
	switch mode {
	case types.CostModeFree:
		hasUnusedPrice = hasConfiguredPrice(config.UnitPrice, config.PricePerSecond, config.TotalPerMillion, config.CompletionPerMillion, config.InputPerMillion, config.OutputPerMillion)
	case types.CostModePerRequest:
		hasUnusedPrice = hasConfiguredPrice(config.PricePerSecond, config.TotalPerMillion, config.CompletionPerMillion, config.InputPerMillion, config.OutputPerMillion)
	case types.CostModePerDuration:
		hasUnusedPrice = hasConfiguredPrice(config.UnitPrice, config.TotalPerMillion, config.CompletionPerMillion, config.InputPerMillion, config.OutputPerMillion)
	case types.CostModePerToken:
		switch config.TokenMode {
		case types.CostTokenModeTotal:
			hasUnusedPrice = hasConfiguredPrice(config.UnitPrice, config.PricePerSecond, config.CompletionPerMillion, config.InputPerMillion, config.OutputPerMillion)
		case types.CostTokenModeCompletion:
			hasUnusedPrice = hasConfiguredPrice(config.UnitPrice, config.PricePerSecond, config.TotalPerMillion, config.InputPerMillion, config.OutputPerMillion)
		case types.CostTokenModeInputOutput:
			hasUnusedPrice = hasConfiguredPrice(config.UnitPrice, config.PricePerSecond, config.TotalPerMillion, config.CompletionPerMillion)
		}
	}
	if hasUnusedPrice {
		return fmt.Errorf("%w: unused price fields must be empty for %q", ErrInvalidCostAmount, mode)
	}
	return nil
}

func hasConfiguredPrice(prices ...*string) bool {
	for _, price := range prices {
		if price != nil {
			return true
		}
	}
	return false
}

func CalculateAttemptCost(mode types.CostMode, config types.CostRuleConfigV1, meter types.CostMeter) (string, int64, error) {
	if mode == types.CostModeFree {
		return "0", 0, nil
	}

	var originalCost decimal.Decimal
	var normalizedCost decimal.Decimal
	var err error

	switch mode {
	case types.CostModePerRequest:
		originalCost, normalizedCost, err = calculatePricePair(config.UnitPrice, config.NormalizedUSDPrices.UnitPrice, decimal.NewFromInt(1))
	case types.CostModePerDuration:
		if meter.DurationSeconds == nil {
			return "", 0, fmt.Errorf("%w: duration meter is missing", ErrInvalidCostAmount)
		}
		duration, parseErr := parseNonNegativeDecimal(*meter.DurationSeconds, "duration", "")
		if parseErr != nil {
			return "", 0, parseErr
		}
		originalCost, normalizedCost, err = calculatePricePair(config.PricePerSecond, config.NormalizedUSDPrices.PricePerSecond, duration)
	case types.CostModePerToken:
		originalCost, normalizedCost, err = calculateTokenCost(config, meter)
	default:
		err = fmt.Errorf("%w: unsupported cost mode %q", ErrInvalidCostAmount, mode)
	}
	if err != nil {
		return "", 0, err
	}
	if originalCost.IsNegative() || normalizedCost.IsNegative() {
		return "", 0, fmt.Errorf("%w: calculated cost cannot be negative", ErrInvalidCostAmount)
	}

	nanoUSD, err := DecimalToNanoUSD(normalizedCost)
	if err != nil {
		return "", 0, err
	}
	return originalCost.String(), nanoUSD, nil
}

func DecimalToNanoUSD(amount decimal.Decimal) (int64, error) {
	rounded := amount.Mul(nanoUSDScale).Round(0)
	if rounded.GreaterThan(maxInt64Decimal) || rounded.LessThan(minInt64Decimal) {
		return 0, ErrNanoUSDOverflow
	}
	return rounded.IntPart(), nil
}

func RevenueEquivalentNanoUSD(finalQuota int64, quotaPerUnitSnapshot string) (int64, error) {
	if finalQuota < 0 {
		return 0, fmt.Errorf("%w: final quota cannot be negative", ErrInvalidCostAmount)
	}
	quotaPerUnit, err := parsePositiveDecimal(quotaPerUnitSnapshot, "quota per unit", "")
	if err != nil {
		return 0, err
	}
	revenue := decimal.NewFromInt(finalQuota).Div(quotaPerUnit)
	if revenue.IsNegative() {
		return 0, fmt.Errorf("%w: revenue cannot be negative", ErrInvalidCostAmount)
	}
	return DecimalToNanoUSD(revenue)
}

func GrossMarginPPM(profitNanoUSD, revenueNanoUSD int64) (*int64, error) {
	if revenueNanoUSD < 0 {
		return nil, fmt.Errorf("%w: revenue cannot be negative", ErrInvalidCostAmount)
	}
	if revenueNanoUSD == 0 {
		return nil, nil
	}

	margin := decimal.NewFromInt(profitNanoUSD).
		Div(decimal.NewFromInt(revenueNanoUSD)).
		Mul(ppmScale).
		Round(0)
	if margin.GreaterThan(maxInt64Decimal) || margin.LessThan(minInt64Decimal) {
		return nil, ErrNanoUSDOverflow
	}
	value := margin.IntPart()
	return &value, nil
}

func CheckedNanoAdd(left, right int64) (int64, error) {
	if (right > 0 && left > math.MaxInt64-right) || (right < 0 && left < math.MinInt64-right) {
		return 0, ErrNanoUSDOverflow
	}
	return left + right, nil
}

func CheckedNanoSubtract(left, right int64) (int64, error) {
	if (right > 0 && left < math.MinInt64+right) || (right < 0 && left > math.MaxInt64+right) {
		return 0, ErrNanoUSDOverflow
	}
	return left - right, nil
}

func calculateTokenCost(config types.CostRuleConfigV1, meter types.CostMeter) (decimal.Decimal, decimal.Decimal, error) {
	switch config.TokenMode {
	case types.CostTokenModeTotal:
		count, err := tokenCount(meter.TotalTokens, "total tokens")
		if err != nil {
			return decimal.Zero, decimal.Zero, err
		}
		return calculatePricePair(config.TotalPerMillion, config.NormalizedUSDPrices.TotalPerMillion, count.Div(millionScale))
	case types.CostTokenModeCompletion:
		count, err := tokenCount(meter.CompletionTokens, "completion tokens")
		if err != nil {
			return decimal.Zero, decimal.Zero, err
		}
		return calculatePricePair(config.CompletionPerMillion, config.NormalizedUSDPrices.CompletionPerMillion, count.Div(millionScale))
	case types.CostTokenModeInputOutput:
		inputCount, err := tokenCount(meter.InputTokens, "input tokens")
		if err != nil {
			return decimal.Zero, decimal.Zero, err
		}
		outputCount, err := tokenCount(meter.OutputTokens, "output tokens")
		if err != nil {
			return decimal.Zero, decimal.Zero, err
		}
		originalInput, normalizedInput, err := calculatePricePair(config.InputPerMillion, config.NormalizedUSDPrices.InputPerMillion, inputCount.Div(millionScale))
		if err != nil {
			return decimal.Zero, decimal.Zero, err
		}
		originalOutput, normalizedOutput, err := calculatePricePair(config.OutputPerMillion, config.NormalizedUSDPrices.OutputPerMillion, outputCount.Div(millionScale))
		if err != nil {
			return decimal.Zero, decimal.Zero, err
		}
		return originalInput.Add(originalOutput), normalizedInput.Add(normalizedOutput), nil
	default:
		return decimal.Zero, decimal.Zero, fmt.Errorf("%w: unsupported token mode %q", ErrInvalidCostAmount, config.TokenMode)
	}
}

func calculatePricePair(originalPrice, normalizedPrice *string, multiplier decimal.Decimal) (decimal.Decimal, decimal.Decimal, error) {
	if originalPrice == nil || normalizedPrice == nil {
		return decimal.Zero, decimal.Zero, fmt.Errorf("%w: price is missing", ErrInvalidCostAmount)
	}
	original, err := parseNonNegativeDecimal(*originalPrice, "original price", "")
	if err != nil {
		return decimal.Zero, decimal.Zero, err
	}
	normalized, err := parseNonNegativeDecimal(*normalizedPrice, "normalized USD price", "")
	if err != nil {
		return decimal.Zero, decimal.Zero, err
	}
	return original.Mul(multiplier), normalized.Mul(multiplier), nil
}

func tokenCount(value *int64, name string) (decimal.Decimal, error) {
	if value == nil {
		return decimal.Zero, fmt.Errorf("%w: %s meter is missing", ErrInvalidCostAmount, name)
	}
	if *value < 0 {
		return decimal.Zero, fmt.Errorf("%w: %s cannot be negative", ErrInvalidCostAmount, name)
	}
	return decimal.NewFromInt(*value), nil
}

func normalizePrice(value *string, factor decimal.Decimal, name string) (*string, *string, error) {
	if value == nil {
		return nil, nil, fmt.Errorf("%w: %s is required", ErrInvalidCostAmount, name)
	}
	price, err := parseNonNegativeDecimal(*value, name, "")
	if err != nil {
		return nil, nil, err
	}
	original := price.String()
	normalized := price.Mul(factor).String()
	return &original, &normalized, nil
}

func parsePositiveDecimal(value, name, defaultValue string) (decimal.Decimal, error) {
	if strings.TrimSpace(value) == "" {
		value = defaultValue
	}
	parsed, err := decimal.NewFromString(value)
	if err != nil || !parsed.IsPositive() {
		return decimal.Zero, fmt.Errorf("%w: %s must be a positive decimal", ErrInvalidCostAmount, name)
	}
	return parsed, nil
}

func parseNonNegativeDecimal(value, name, defaultValue string) (decimal.Decimal, error) {
	if strings.TrimSpace(value) == "" {
		value = defaultValue
	}
	parsed, err := decimal.NewFromString(value)
	if err != nil || parsed.IsNegative() {
		return decimal.Zero, fmt.Errorf("%w: %s must be a non-negative decimal", ErrInvalidCostAmount, name)
	}
	return parsed, nil
}
