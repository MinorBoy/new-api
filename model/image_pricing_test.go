package model

import (
	"testing"

	"github.com/QuantumNous/new-api/pkg/imageprofile"
	"github.com/QuantumNous/new-api/setting/image_setting"
	"github.com/stretchr/testify/require"
)

func TestImagePricingFromCatalogReturnsStableTierQualityPrices(t *testing.T) {
	catalog := image_setting.Catalog{
		Models: map[string]image_setting.ModelEntry{
			"gpt-image-2": {
				SKUs: map[string]image_setting.SKU{
					"gen-2k-high":   {Endpoint: imageprofile.EndpointGenerations, Tier: "2k", Quality: "high", Unit: "image", SalePriceUSD: "0.12"},
					"gen-1k-medium": {Endpoint: imageprofile.EndpointGenerations, Tier: "1k", Quality: "medium", Unit: "image", SalePriceUSD: "0.05"},
					"gen-1k-low":    {Endpoint: imageprofile.EndpointGenerations, Tier: "1k", Quality: "low", Unit: "image", SalePriceUSD: "0.04"},
				},
			},
		},
	}

	prices := imagePricingFromCatalog(catalog, "gpt-image-2")
	require.Equal(t, []ImagePrice{
		{Tier: "1k", Quality: "low", PriceUSD: "0.04"},
		{Tier: "1k", Quality: "medium", PriceUSD: "0.05"},
		{Tier: "2k", Quality: "high", PriceUSD: "0.12"},
	}, prices)
}
