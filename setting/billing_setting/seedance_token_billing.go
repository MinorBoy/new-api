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

func GetSeedanceTokenPrice(model string) (types.SeedanceTokenPrice, bool) {
	return billingSetting.SeedanceTokenPrice.Get(model)
}

func GetSeedanceTokenPriceCopy() map[string]types.SeedanceTokenPrice {
	return billingSetting.SeedanceTokenPrice.ReadAll()
}

func ValidateSeedanceTokenPriceJSONString(raw string) error {
	var prices map[string]types.SeedanceTokenPrice
	if err := common.UnmarshalJsonStr(raw, &prices); err != nil {
		return fmt.Errorf("invalid Seedance token price JSON: %w", err)
	}
	if prices == nil {
		return fmt.Errorf("Seedance token price must be a JSON object")
	}
	for model, price := range prices {
		if strings.TrimSpace(model) == "" {
			return fmt.Errorf("Seedance token price model name cannot be empty")
		}
		if err := price.Validate(relaycommon.MaxTokensLimit); err != nil {
			return fmt.Errorf("invalid Seedance token price for %s: %w", model, err)
		}
	}
	currentPrices := GetSeedanceTokenPriceCopy()
	for model, current := range currentPrices {
		if seedancepricing.Family(model) == "" {
			continue
		}
		proposed, exists := prices[model]
		if !exists {
			return fmt.Errorf("Seedance token price for %s cannot be removed outside config import", model)
		}
		if !reflect.DeepEqual(current, proposed) {
			return fmt.Errorf("Seedance token price for %s must be updated through config import", model)
		}
	}
	for model := range prices {
		if seedancepricing.Family(model) == "" {
			continue
		}
		if _, exists := currentPrices[model]; !exists {
			return fmt.Errorf("Seedance token price for %s must be created through config import", model)
		}
	}
	return nil
}

func SeedanceTokenPrice2JSONString() string {
	data, err := common.Marshal(GetSeedanceTokenPriceCopy())
	if err != nil {
		return "{}"
	}
	return string(data)
}
