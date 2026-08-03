package billing_setting

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/seedancepricing"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
)

func GetDurationPrice(model string) (types.DurationPrice, bool) {
	return billingSetting.DurationPrice.Get(model)
}

func GetDurationPriceCopy() map[string]types.DurationPrice {
	return billingSetting.DurationPrice.ReadAll()
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
	currentPrices := GetDurationPriceCopy()
	for model, current := range currentPrices {
		if seedancepricing.Family(model) == "" {
			continue
		}
		proposed, exists := prices[model]
		if !exists {
			return fmt.Errorf("Seedance duration price for %s cannot be removed outside config import", model)
		}
		if !reflect.DeepEqual(current, proposed) {
			return fmt.Errorf("Seedance duration price for %s must be updated through config import", model)
		}
	}
	for model := range prices {
		if seedancepricing.Family(model) == "" {
			continue
		}
		if _, exists := currentPrices[model]; !exists {
			return fmt.Errorf("Seedance duration price for %s must be created through config import", model)
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
