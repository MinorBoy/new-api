package billing_setting

import (
	"fmt"
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
	var objects map[string]map[string]any
	if err := common.UnmarshalJsonStr(raw, &objects); err == nil {
		for model, object := range objects {
			if _, exists := object["scenarios"]; exists {
				return fmt.Errorf("duration price for %s: scenarios is no longer supported", model)
			}
		}
	}
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
		if seedancepricing.Family(model) != "" {
			return fmt.Errorf("Seedance model %s does not support per_duration pricing", model)
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
