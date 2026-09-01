package image_setting

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/pkg/imageprofile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
