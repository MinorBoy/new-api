package billing_setting

import (
	"testing"

	"github.com/QuantumNous/new-api/pkg/modelrouting"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSeedanceTokenPriceConfiguredRuleIsReturned(t *testing.T) {
	originalPrices := billingSetting.SeedanceTokenPrice
	t.Cleanup(func() { billingSetting.SeedanceTokenPrice = originalPrices })
	billingSetting.SeedanceTokenPrice = types.NewRWMap[string, types.SeedanceTokenPrice]()
	price := types.SeedanceTokenPrice{Scenarios: map[string]types.SeedanceTokenPriceScenario{
		"480p:with_video": {
			PricePerMillion: "1.917808219178082",
			Width:           864,
			Height:          496,
			FrameRate:       24,
			PricingVersion:  "official-token-v1",
			Source:          "SRC-OFFICIAL-SEEDANCE-2-0-MINI!18",
		},
	}}
	billingSetting.SeedanceTokenPrice.Set(modelrouting.Seedance20Mini, price)

	got, ok := GetSeedanceTokenPrice(modelrouting.Seedance20Mini)

	require.True(t, ok)
	assert.Equal(t, price, got)
	assert.Equal(t, BillingModeSeedanceTokens, "seedance_tokens")
}

func TestValidateSeedanceTokenPriceJSONStringProtectsImportedPrices(t *testing.T) {
	originalPrices := billingSetting.SeedanceTokenPrice
	t.Cleanup(func() { billingSetting.SeedanceTokenPrice = originalPrices })
	billingSetting.SeedanceTokenPrice = types.NewRWMap[string, types.SeedanceTokenPrice]()
	billingSetting.SeedanceTokenPrice.Set(modelrouting.Seedance20Mini, types.SeedanceTokenPrice{Scenarios: map[string]types.SeedanceTokenPriceScenario{
		"480p:with_video": {
			PricePerMillion: "1.917808219178082",
			Width:           864,
			Height:          496,
			FrameRate:       24,
			PricingVersion:  "official-token-v1",
			Source:          "SRC-OFFICIAL-SEEDANCE-2-0-MINI!18",
		},
	}})

	err := ValidateSeedanceTokenPriceJSONString(`{"doubao-seedance-2-0-mini-260615":{"scenarios":{"480p:with_video":{"price_per_million":"9","width":864,"height":496,"frame_rate":24,"pricing_version":"official-token-v1","source":"SRC-OFFICIAL-SEEDANCE-2-0-MINI!18"}}}}`)

	assert.ErrorContains(t, err, "must be updated through config import")
}
