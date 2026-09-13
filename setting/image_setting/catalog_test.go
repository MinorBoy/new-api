package image_setting

import (
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/pkg/imageprofile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveResolutionTierUsesTotalPixelsAndAutoDefaults(t *testing.T) {
	tests := []struct {
		name string
		size string
		want string
	}{
		{name: "missing size", size: "", want: "1k"},
		{name: "auto size", size: "auto", want: "1k"},
		{name: "one k boundary", size: "1024x1024", want: "1k"},
		{name: "two k boundary", size: "2048x2048", want: "2k"},
		{name: "four k boundary", size: "2880x2880", want: "4k"},
		{name: "rectangular two k by area", size: "4096x1024", want: "2k"},
		{name: "rectangular four k by area", size: "3000x2000", want: "4k"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, normalized, err := ResolveResolutionTier(tt.size)
			require.NoError(t, err)
			assert.Equal(t, ResolutionTier(tt.want), got)
			if tt.size == "" || tt.size == "auto" {
				assert.Equal(t, "", normalized)
			} else {
				assert.Equal(t, tt.size, normalized)
			}
		})
	}
}

func TestResolveResolutionTierRejectsInvalidAndOversizedInput(t *testing.T) {
	for _, size := range []string{"0x1024", "1024x0", "-1x1024", "1024", "1024*1024", "2881x2881", fmt.Sprintf("%d x 1", ^uint64(0))} {
		t.Run(size, func(t *testing.T) {
			_, _, err := ResolveResolutionTier(size)
			require.Error(t, err)
		})
	}
}

func TestResolveLegacyResolutionTierKeepsHistoricalOversizedSKUsAtFourK(t *testing.T) {
	tier, normalized, err := ResolveLegacyResolutionTier("4096x4096")
	require.NoError(t, err)
	assert.Equal(t, ResolutionTier4K, tier)
	assert.Equal(t, "4096x4096", normalized)

	_, _, err = ResolveResolutionTier("4096x4096")
	assert.Error(t, err)
}

func TestBuildTierSKUKeyUsesResolutionTier(t *testing.T) {
	assert.Equal(t, "gen-2k-high", BuildTierSKUKey(imageprofile.EndpointGenerations, ResolutionTier2K, "high"))
	assert.Equal(t, "edit-4k-low", BuildTierSKUKey(imageprofile.EndpointEdits, ResolutionTier4K, "low"))
}

func validCatalogJSON() string {
	return `{"version":1,"models":{"gpt-image-1":{"profile":"openai_images","profile_version":1,"endpoints":{"generations":{"capability":{"enabled":true,"sizes":["1024x1024"],"qualities":["medium"],"response_formats":["b64_json"],"max_n":4,"max_input_images":0,"supports_mask":false},"default_size":"1024x1024","default_quality":"medium","default_response_format":"b64_json"},"edits":{"capability":{"enabled":true,"sizes":["1024x1024"],"qualities":["medium"],"response_formats":["b64_json"],"max_n":2,"max_input_images":2,"supports_mask":true},"default_size":"1024x1024","default_quality":"medium","default_response_format":"b64_json"}},"skus":{"gen-1024x1024-medium":{"endpoint":"generations","size":"1024x1024","quality":"medium","unit":"image","sale_price_usd":"0.040000"},"edit-1024x1024-medium":{"endpoint":"edits","size":"1024x1024","quality":"medium","unit":"image","sale_price_usd":"0.060000"}}}}}`
}

func TestCatalogUpdateResolveAndSnapshot(t *testing.T) {
	t.Cleanup(func() { require.NoError(t, UpdateCatalogByJSONString(`{"version":1,"models":{}}`)) })
	require.NoError(t, UpdateCatalogByJSONString(validCatalogJSON()))
	resolved, err := Resolve(Selection{Model: "gpt-image-1", Endpoint: imageprofile.EndpointGenerations, N: 2})
	require.NoError(t, err)
	assert.Equal(t, "gen-1024x1024-medium", resolved.SKUKey)
	assert.Equal(t, "b64_json", resolved.ResponseFormat)
	assert.Equal(t, "0.040000", resolved.SalePriceUSD)
	assert.Equal(t, uint(2), resolved.N)

	snapshot := Snapshot()
	snapshot.Models["gpt-image-1"].Endpoints[imageprofile.EndpointGenerations] = EndpointCatalog{}
	assert.NotEmpty(t, Snapshot().Models["gpt-image-1"].Endpoints[imageprofile.EndpointGenerations].Capability.Sizes)
}

func TestEnsureOpenAIImage25ModelsClonesExistingImage2Matrix(t *testing.T) {
	entry := ModelEntry{
		Profile:        imageprofile.OpenAIImagesProfile,
		ProfileVersion: imageprofile.OpenAIImagesVersion,
		Endpoints: map[imageprofile.Endpoint]EndpointCatalog{
			imageprofile.EndpointGenerations: {
				Capability: imageprofile.Capability{
					Enabled:         true,
					ResolutionTiers: []string{"1k", "2k", "4k"},
					Qualities:       []string{"low", "medium", "high"},
					ResponseFormats: []string{"b64_json"},
					MaxN:            4,
				},
				DefaultSize:           "auto",
				DefaultQuality:        "medium",
				DefaultResponseFormat: "b64_json",
			},
		},
		SKUs: map[string]SKU{
			"gen-1k-low":    {Endpoint: imageprofile.EndpointGenerations, Tier: "1k", Quality: "low", Unit: "image", SalePriceUSD: "0.05"},
			"gen-1k-medium": {Endpoint: imageprofile.EndpointGenerations, Tier: "1k", Quality: "medium", Unit: "image", SalePriceUSD: "0.05"},
			"gen-1k-high":   {Endpoint: imageprofile.EndpointGenerations, Tier: "1k", Quality: "high", Unit: "image", SalePriceUSD: "0.05"},
			"gen-2k-low":    {Endpoint: imageprofile.EndpointGenerations, Tier: "2k", Quality: "low", Unit: "image", SalePriceUSD: "0.1"},
			"gen-2k-medium": {Endpoint: imageprofile.EndpointGenerations, Tier: "2k", Quality: "medium", Unit: "image", SalePriceUSD: "0.1"},
			"gen-2k-high":   {Endpoint: imageprofile.EndpointGenerations, Tier: "2k", Quality: "high", Unit: "image", SalePriceUSD: "0.1"},
			"gen-4k-low":    {Endpoint: imageprofile.EndpointGenerations, Tier: "4k", Quality: "low", Unit: "image", SalePriceUSD: "0.2"},
			"gen-4k-medium": {Endpoint: imageprofile.EndpointGenerations, Tier: "4k", Quality: "medium", Unit: "image", SalePriceUSD: "0.2"},
			"gen-4k-high":   {Endpoint: imageprofile.EndpointGenerations, Tier: "4k", Quality: "high", Unit: "image", SalePriceUSD: "0.2"},
		},
	}
	catalog := Catalog{Version: CatalogVersion, Models: map[string]ModelEntry{"gpt-image-2": entry}}

	updated, changed := EnsureOpenAIImage25Models(catalog)
	require.True(t, changed)
	for _, modelName := range OpenAIImage25Models {
		model, ok := updated.Models[modelName]
		require.True(t, ok)
		require.Len(t, model.SKUs, 9)
		assert.Equal(t, "0.2", model.SKUs["gen-4k-high"].SalePriceUSD)
	}

	updated.Models[OpenAIImage25Models[0]].SKUs["gen-1k-low"] = SKU{}
	assert.Equal(t, "0.05", updated.Models[OpenAIImage25Models[1]].SKUs["gen-1k-low"].SalePriceUSD)
	assert.Equal(t, "0.05", catalog.Models["gpt-image-2"].SKUs["gen-1k-low"].SalePriceUSD)

	_, changed = EnsureOpenAIImage25Models(updated)
	assert.False(t, changed)
}

func TestEnsureOpenAIImage25ModelsPreservesExistingIndependentEntry(t *testing.T) {
	source := ModelEntry{
		Profile:        imageprofile.OpenAIImagesProfile,
		ProfileVersion: imageprofile.OpenAIImagesVersion,
		Endpoints: map[imageprofile.Endpoint]EndpointCatalog{
			imageprofile.EndpointGenerations: {
				Capability:     imageprofile.Capability{Enabled: true, Qualities: []string{"medium"}, ResponseFormats: []string{"b64_json"}, MaxN: 1},
				DefaultQuality: "medium", DefaultResponseFormat: "b64_json",
			},
		},
		SKUs: map[string]SKU{
			"gen-1k-medium": {Endpoint: imageprofile.EndpointGenerations, Tier: "1k", Quality: "medium", Unit: "image", SalePriceUSD: "0.03"},
		},
	}
	customFlare := cloneModelEntry(source)
	customFlare.SKUs["gen-1k-medium"] = SKU{Endpoint: imageprofile.EndpointGenerations, Tier: "1k", Quality: "medium", Unit: "image", SalePriceUSD: "0.99"}
	catalog := Catalog{Version: CatalogVersion, Models: map[string]ModelEntry{
		"gpt-image-2":          source,
		OpenAIImage25Models[0]: customFlare,
	}}

	updated, changed := EnsureOpenAIImage25Models(catalog)
	require.True(t, changed)
	assert.Equal(t, "0.99", updated.Models[OpenAIImage25Models[0]].SKUs["gen-1k-medium"].SalePriceUSD)
	assert.Equal(t, "0.03", updated.Models[OpenAIImage25Models[1]].SKUs["gen-1k-medium"].SalePriceUSD)
}

func TestResolveUsesTierSKUForArbitraryImageSize(t *testing.T) {
	t.Cleanup(func() { require.NoError(t, UpdateCatalogByJSONString(`{"version":1,"models":{}}`)) })
	raw := `{"version":1,"models":{"gpt-image-2":{"profile":"openai_images","profile_version":1,"endpoints":{"generations":{"capability":{"enabled":true,"qualities":["low","medium","high"],"response_formats":["b64_json"],"max_n":4},"default_size":"auto","default_quality":"medium","default_response_format":"b64_json"}},"skus":{"gen-1k-medium":{"endpoint":"generations","tier":"1k","quality":"medium","unit":"image","sale_price_usd":"0.02"},"gen-2k-high":{"endpoint":"generations","tier":"2k","quality":"high","unit":"image","sale_price_usd":"0.04"},"gen-4k-high":{"endpoint":"generations","tier":"4k","quality":"high","unit":"image","sale_price_usd":"0.08"}}}}}`
	require.NoError(t, UpdateCatalogByJSONString(raw))
	resolved, err := Resolve(Selection{Model: "gpt-image-2", Endpoint: imageprofile.EndpointGenerations, Size: "1500x1500", Quality: "high", ResponseFormat: "b64_json", N: 1})
	require.NoError(t, err)
	assert.Equal(t, ResolutionTier2K, resolved.Tier)
	assert.Equal(t, "gen-2k-high", resolved.SKUKey)
	assert.Equal(t, "1500x1500", resolved.Size)

	resolved, err = Resolve(Selection{Model: "gpt-image-2", Endpoint: imageprofile.EndpointGenerations, Size: "auto", Quality: "medium", ResponseFormat: "b64_json", N: 1})
	require.NoError(t, err)
	assert.Equal(t, ResolutionTier1K, resolved.Tier)
	assert.Equal(t, "gen-1k-medium", resolved.SKUKey)
}

func TestResolveOmittedSizeUsesOneKTierEvenWhenDefaultSizeIsConcrete(t *testing.T) {
	t.Cleanup(func() { require.NoError(t, UpdateCatalogByJSONString(`{"version":1,"models":{}}`)) })
	raw := `{"version":1,"models":{"gpt-image-2":{"profile":"openai_images","profile_version":1,"endpoints":{"generations":{"capability":{"enabled":true,"qualities":["medium"],"response_formats":["b64_json"],"max_n":1},"default_size":"2048x2048","default_quality":"medium","default_response_format":"b64_json"}},"skus":{"gen-1k-medium":{"endpoint":"generations","tier":"1k","quality":"medium","unit":"image","sale_price_usd":"0.02"},"gen-2k-medium":{"endpoint":"generations","tier":"2k","quality":"medium","unit":"image","sale_price_usd":"0.04"}}}}}`
	require.NoError(t, UpdateCatalogByJSONString(raw))
	resolved, err := Resolve(Selection{Model: "gpt-image-2", Endpoint: imageprofile.EndpointGenerations, N: 1})
	require.NoError(t, err)
	assert.Equal(t, ResolutionTier1K, resolved.Tier)
	assert.Equal(t, "gen-1k-medium", resolved.SKUKey)
	assert.Empty(t, resolved.Size)
}

func TestCatalogRejectsInvalidDefinitionsAndPreservesPreviousSnapshot(t *testing.T) {
	t.Cleanup(func() { require.NoError(t, UpdateCatalogByJSONString(`{"version":1,"models":{}}`)) })
	require.NoError(t, UpdateCatalogByJSONString(validCatalogJSON()))
	invalid := []struct {
		name string
		raw  string
		want string
	}{
		{"duplicate options", `{"version":1,"models":{"m":{"profile":"openai_images","profile_version":1,"endpoints":{"generations":{"capability":{"enabled":true,"sizes":["1024x1024","1024x1024"],"qualities":["medium"],"response_formats":["b64_json"],"max_n":1},"default_size":"1024x1024","default_quality":"medium","default_response_format":"b64_json"}},"skus":{"gen-1024x1024-medium":{"endpoint":"generations","size":"1024x1024","quality":"medium","unit":"image","sale_price_usd":"1"}}}}}`, "duplicate"},
		{"bad price", stringsReplace(validCatalogJSON(), `"0.040000"`, `"1e-3"`), "fixed-point"},
		{"too many images", stringsReplace(validCatalogJSON(), `"max_n":4`, `"max_n":129`), "max_n"},
	}
	for _, tt := range invalid {
		t.Run(tt.name, func(t *testing.T) {
			err := UpdateCatalogByJSONString(tt.raw)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
			assert.Equal(t, "0.040000", Snapshot().Models["gpt-image-1"].SKUs["gen-1024x1024-medium"].SalePriceUSD)
		})
	}
}

func TestResolveRejectsUnsupportedRequestCombinations(t *testing.T) {
	t.Cleanup(func() { require.NoError(t, UpdateCatalogByJSONString(`{"version":1,"models":{}}`)) })
	require.NoError(t, UpdateCatalogByJSONString(validCatalogJSON()))
	tests := []struct {
		name      string
		selection Selection
		want      string
	}{
		{"zero n", Selection{Model: "gpt-image-1", Endpoint: imageprofile.EndpointGenerations}, "between 1"},
		{"unsupported format", Selection{Model: "gpt-image-1", Endpoint: imageprofile.EndpointGenerations, N: 1, ResponseFormat: "url"}, "response format"},
		{"input on generation", Selection{Model: "gpt-image-1", Endpoint: imageprofile.EndpointGenerations, N: 1, InputImages: 1}, "does not accept"},
		{"mask on generation", Selection{Model: "gpt-image-1", Endpoint: imageprofile.EndpointGenerations, N: 1, HasMask: true}, "does not support mask"},
		{"too many input images", Selection{Model: "gpt-image-1", Endpoint: imageprofile.EndpointEdits, N: 1, InputImages: 3}, "max_input_images"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Resolve(tt.selection)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func stringsReplace(value, old, new string) string {
	return strings.Replace(value, old, new, 1)
}
