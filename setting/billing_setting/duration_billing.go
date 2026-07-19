package billing_setting

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
)

var defaultDurationPrice = map[string]types.DurationPrice{
	"jimeng-video-seedance-2.0-fast-vip": {
		Price: 0.48 / 7.3, Unit: types.DurationUnitSecond,
		RoundingStepSeconds: 1, MinimumDurationSeconds: 4,
	},
	"jimeng-video-seedance-2.0-mini": {
		Price: 0.39 / 7.3, Unit: types.DurationUnitSecond,
		RoundingStepSeconds: 1, MinimumDurationSeconds: 4,
	},
	"jimeng-video-seedance-2.0-vip": {
		Price: 0.62 / 7.3, Unit: types.DurationUnitSecond,
		RoundingStepSeconds: 1, MinimumDurationSeconds: 4,
	},
}

func GetDurationPrice(model string) (types.DurationPrice, bool) {
	if price, ok := billingSetting.DurationPrice.Get(model); ok {
		return price, true
	}
	price, ok := defaultDurationPrice[model]
	return price, ok
}

func GetDurationPriceCopy() map[string]types.DurationPrice {
	configuredPrices := billingSetting.DurationPrice.ReadAll()
	prices := make(map[string]types.DurationPrice, len(defaultDurationPrice)+len(configuredPrices))
	for model, price := range defaultDurationPrice {
		prices[model] = price
	}
	for model, price := range configuredPrices {
		prices[model] = price
	}
	return prices
}

func GetDefaultDurationPriceMap() map[string]types.DurationPrice {
	prices := make(map[string]types.DurationPrice, len(defaultDurationPrice))
	for model, price := range defaultDurationPrice {
		prices[model] = price
	}
	return prices
}

func ValidateDurationPriceJSONString(raw string) error {
	var prices map[string]types.DurationPrice
	if err := common.UnmarshalJsonStr(raw, &prices); err != nil {
		return fmt.Errorf("invalid duration price JSON: %w", err)
	}
	if prices == nil {
		return fmt.Errorf("duration price must be a JSON object")
	}
	for model, price := range prices {
		if strings.TrimSpace(model) == "" {
			return fmt.Errorf("duration price model name cannot be empty")
		}
		if err := price.Validate(relaycommon.MaxTaskDurationSeconds); err != nil {
			return fmt.Errorf("invalid duration price for %s: %w", model, err)
		}
	}
	return nil
}

func DurationPrice2JSONString() string {
	data, err := common.Marshal(GetDurationPriceCopy())
	if err != nil {
		return "{}"
	}
	return string(data)
}
