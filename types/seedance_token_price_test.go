package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSeedanceTokenPriceCalculatesOfficialWithVideoCharge(t *testing.T) {
	price := SeedanceTokenPrice{Scenarios: map[string]SeedanceTokenPriceScenario{
		SeedanceTokenScenarioKey("480p", SeedanceTokenScenarioWithVideo): {
			PricePerMillion: "1.917808219178082",
			Width:           864,
			Height:          496,
			FrameRate:       24,
			PricingVersion:  "official-token-v1",
			Source:          "SRC-OFFICIAL-SEEDANCE-2-0-MINI!18",
		},
	}}

	charge, err := price.CalculateCharge(
		"480p",
		true,
		3000,
		4,
		SeedanceTokenUsage{InputTokens: 0, OutputTokens: 70308, TotalTokens: 70308},
		1_073_741_823,
	)

	require.NoError(t, err)
	assert.Equal(t, SeedanceTokenScenarioWithVideo, charge.Scenario)
	assert.Equal(t, "1.917808219178082", charge.PricePerMillion.String())
	assert.Equal(t, 0, charge.InputTokens)
	assert.Equal(t, 70308, charge.OutputTokens)
	assert.Equal(t, 70308, charge.TotalTokens)
	assert.Equal(t, "0.134837260273972589256", charge.BaseCharge.String())
	assert.Equal(t, 864, charge.Width)
	assert.Equal(t, 496, charge.Height)
	assert.Equal(t, 24, charge.FrameRate)
}

func TestSeedanceTokenPriceRejectsMissingScenario(t *testing.T) {
	price := SeedanceTokenPrice{Scenarios: map[string]SeedanceTokenPriceScenario{
		SeedanceTokenScenarioKey("480p", SeedanceTokenScenarioNoVideo): {
			PricePerMillion: "3.150684931506849",
			Width:           864,
			Height:          496,
			FrameRate:       24,
			PricingVersion:  "official-token-v1",
			Source:          "SRC-OFFICIAL-SEEDANCE-2-0-MINI!17",
		},
	}}

	_, err := price.CalculateCharge(
		"480p",
		true,
		3000,
		4,
		SeedanceTokenUsage{InputTokens: 0, OutputTokens: 70308, TotalTokens: 70308},
		1_073_741_823,
	)

	assert.ErrorContains(t, err, "scenario is unavailable")
}

func TestSeedanceTokenPriceRejectsInconsistentUsageParts(t *testing.T) {
	price := SeedanceTokenPrice{Scenarios: map[string]SeedanceTokenPriceScenario{
		SeedanceTokenScenarioKey("480p", SeedanceTokenScenarioWithVideo): {
			PricePerMillion: "1.917808219178082", Width: 864, Height: 496, FrameRate: 24,
			PricingVersion: "official-token-v1", Source: "sd官价!A1",
		},
	}}

	_, err := price.CalculateCharge(
		"480p",
		true,
		3000,
		4,
		SeedanceTokenUsage{InputTokens: 0, OutputTokens: 70308, TotalTokens: 70309},
		1_073_741_823,
	)

	assert.ErrorContains(t, err, "token usage is invalid")
}

func TestSeedanceTokenPriceRejectsInputTokenUsage(t *testing.T) {
	price := SeedanceTokenPrice{Scenarios: map[string]SeedanceTokenPriceScenario{
		SeedanceTokenScenarioKey("480p", SeedanceTokenScenarioWithVideo): {
			PricePerMillion: "1.917808219178082", Width: 864, Height: 496, FrameRate: 24,
			PricingVersion: "official-token-v1", Source: "sd官价!A1",
		},
	}}

	_, err := price.CalculateCharge(
		"480p",
		true,
		3000,
		4,
		SeedanceTokenUsage{InputTokens: 1, OutputTokens: 70308, TotalTokens: 70309},
		1_073_741_823,
	)

	assert.ErrorContains(t, err, "token usage is invalid")
}
