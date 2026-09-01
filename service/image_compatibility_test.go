package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/imageprofile"
	relaydto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/image_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImageContractHashUsesCanonicalContractAndExcludesCredentials(t *testing.T) {
	price := `{"version":1,"models":{"gpt-image-1":{"profile":"openai_images","profile_version":1,"endpoints":{"generations":{"capability":{"enabled":true,"sizes":["1024x1024"],"qualities":["medium"],"response_formats":["b64_json"],"max_n":1},"default_size":"1024x1024","default_quality":"medium","default_response_format":"b64_json"}},"skus":{"gen-1024x1024-medium":{"endpoint":"generations","size":"1024x1024","quality":"medium","unit":"image","sale_price_usd":"0.1"}}}}}`
	require.NoError(t, image_setting.UpdateCatalogByJSONString(price))

	mapping := `{"gpt-image-1":"vendor-image"}`
	channel := &model.Channel{Id: 7, Type: constant.ChannelTypeOpenAI, Key: "secret-key", Models: "gpt-image-1", ModelMapping: &mapping, BaseURL: stringPtr("https://vendor.example")}
	channel.OtherSettings = `{"image_profile":{"profile":"openai_images","profile_version":1}}`
	request := ImageRequestContext{Resolved: image_setting.ResolvedSKU{
		CatalogVersion: 1, Model: "gpt-image-1", SKUKey: "gen-1024x1024-medium", Size: "1024x1024", Quality: "medium", ResponseFormat: "b64_json", SalePriceUSD: "0.1", Endpoint: imageprofile.EndpointGenerations, N: 1,
	}}

	first, err := ResolveImageContractHash(channel, "gpt-image-1", request)
	require.NoError(t, err)
	settings := channel.GetOtherSettings()
	settings.ImageProfile.Compatibility = map[string]imageprofile.Compatibility{"gpt-image-1:generations": {Status: imageprofile.CompatibilityPassed, ProfileVersion: 1, ContractHash: first}}
	channel.SetOtherSettings(settings)
	assert.Len(t, first, 64)
	assert.NotContains(t, first, "secret-key")

	channel.Key = "another-secret"
	second, err := ResolveImageContractHash(channel, "gpt-image-1", request)
	require.NoError(t, err)
	assert.Equal(t, first, second)

	request.Resolved.Size = "1024x1024"
	request.Resolved.Quality = "medium"
	third, err := ResolveImageContractHash(channel, "gpt-image-1", request)
	require.NoError(t, err)
	assert.Equal(t, first, third)
}

func TestImageContractHashChangesWithEffectiveChannelContract(t *testing.T) {
	price := `{"version":1,"models":{"gpt-image-1":{"profile":"openai_images","profile_version":1,"endpoints":{"generations":{"capability":{"enabled":true,"sizes":["1024x1024"],"qualities":["medium"],"response_formats":["b64_json"],"max_n":1},"default_size":"1024x1024","default_quality":"medium","default_response_format":"b64_json"}},"skus":{"gen-1024x1024-medium":{"endpoint":"generations","size":"1024x1024","quality":"medium","unit":"image","sale_price_usd":"0.1"}}}}}`
	require.NoError(t, image_setting.UpdateCatalogByJSONString(price))

	mapping := `{"gpt-image-1":"vendor-image"}`
	channel := &model.Channel{
		Id: 7, Type: constant.ChannelTypeOpenAI, Key: "secret-key", Models: "gpt-image-1",
		ModelMapping: &mapping, BaseURL: stringPtr("https://vendor.example"),
		OtherSettings: `{"image_profile":{"profile":"openai_images","profile_version":1}}`,
	}
	request := ImageRequestContext{Resolved: image_setting.ResolvedSKU{
		CatalogVersion: 1, Model: "gpt-image-1", SKUKey: "gen-1024x1024-medium", Size: "1024x1024", Quality: "medium", ResponseFormat: "b64_json", SalePriceUSD: "0.1", Endpoint: imageprofile.EndpointGenerations, N: 1,
	}}
	baseline, err := ResolveImageContractHash(channel, "gpt-image-1", request)
	require.NoError(t, err)

	tests := []struct {
		name   string
		mutate func(*model.Channel)
	}{
		{name: "base URL", mutate: func(channel *model.Channel) { channel.BaseURL = stringPtr("https://other.example") }},
		{name: "channel type", mutate: func(channel *model.Channel) { channel.Type = constant.ChannelTypeCustom }},
		{name: "parameter override", mutate: func(channel *model.Channel) { channel.ParamOverride = stringPtr(`{"prompt":"vendor test"}`) }},
		{name: "header override", mutate: func(channel *model.Channel) { channel.HeaderOverride = stringPtr(`{"X-Vendor-Feature":"enabled"}`) }},
		{name: "organization", mutate: func(channel *model.Channel) { channel.OpenAIOrganization = stringPtr("org-vendor") }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := *channel
			test.mutate(&candidate)
			actual, err := ResolveImageContractHash(&candidate, "gpt-image-1", request)
			require.NoError(t, err)
			assert.NotEqual(t, baseline, actual)
		})
	}
}

func TestMergeStoredImageCompatibilityRejectsSubmittedPassedState(t *testing.T) {
	mapping := `{"gpt-image-1":"vendor-image"}`
	base := &model.Channel{Id: 1, Type: constant.ChannelTypeOpenAI, Models: "gpt-image-1", ModelMapping: &mapping, OtherSettings: `{"image_profile":{"profile":"openai_images","profile_version":1}}`}
	request := ImageRequestContext{Resolved: image_setting.ResolvedSKU{CatalogVersion: 1, Model: "gpt-image-1", SKUKey: "gen-1024x1024-medium", Size: "1024x1024", Quality: "medium", ResponseFormat: "b64_json", SalePriceUSD: "0.1", Endpoint: imageprofile.EndpointGenerations, N: 1}}
	hash, err := ResolveImageContractHash(base, "gpt-image-1", request)
	require.NoError(t, err)
	storedSettings := base.GetOtherSettings()
	storedSettings.ImageProfile.Compatibility = map[string]imageprofile.Compatibility{"gpt-image-1:generations": {Status: imageprofile.CompatibilityPassed, ProfileVersion: 1, ContractHash: hash}}
	base.SetOtherSettings(storedSettings)
	stored := base
	submitted := &model.Channel{Id: 1, Type: constant.ChannelTypeOpenAI, Models: "gpt-image-1", ModelMapping: &mapping, OtherSettings: `{"image_profile":{"profile":"openai_images","profile_version":1,"compatibility":{"gpt-image-1:generations":{"status":"passed","profile_version":1,"contract_hash":"forged"}}}}`}

	require.NoError(t, MergeStoredImageCompatibility(stored, submitted))
	assert.Contains(t, submitted.OtherSettings, hash)
	assert.NotContains(t, submitted.OtherSettings, "forged")
}

func TestMergeStoredImageCompatibilityInvalidatesChangedMapping(t *testing.T) {
	oldMapping := `{"gpt-image-1":"vendor-image"}`
	newMapping := `{"gpt-image-1":"vendor-image-v2"}`
	stored := &model.Channel{Id: 1, Type: constant.ChannelTypeOpenAI, Models: "gpt-image-1", ModelMapping: &oldMapping, OtherSettings: `{"image_profile":{"profile":"openai_images","profile_version":1,"compatibility":{"gpt-image-1:generations":{"status":"passed","profile_version":1,"contract_hash":"valid"}}}}`}
	submitted := &model.Channel{Id: 1, Type: constant.ChannelTypeOpenAI, Models: "gpt-image-1", ModelMapping: &newMapping, OtherSettings: stored.OtherSettings}

	require.NoError(t, MergeStoredImageCompatibility(stored, submitted))
	assert.NotContains(t, submitted.OtherSettings, "valid")
}

func TestRunImageCompatibilityTestVerifiesMappedModelAndResponse(t *testing.T) {
	require.NoError(t, image_setting.UpdateCatalogByJSONString(`{"version":1,"models":{"gpt-image-1":{"profile":"openai_images","profile_version":1,"endpoints":{"generations":{"capability":{"enabled":true,"sizes":["1024x1024"],"qualities":["medium"],"response_formats":["b64_json"],"max_n":1},"default_size":"1024x1024","default_quality":"medium","default_response_format":"b64_json"}},"skus":{"gen-1024x1024-medium":{"endpoint":"generations","size":"1024x1024","quality":"medium","unit":"image","sale_price_usd":"0.1"}}}}}`))
	var receivedModel string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model string `json:"model"`
		}
		require.NoError(t, common.DecodeJson(r.Body, &body))
		receivedModel = body.Model
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"b64_json":"aGVsbG8="}]}`))
	}))
	defer server.Close()
	mapping := `{"gpt-image-1":"vendor-image"}`
	channel := &model.Channel{Id: 9, Type: constant.ChannelTypeOpenAI, Key: "test-key", Models: "gpt-image-1", ModelMapping: &mapping, BaseURL: &server.URL, OtherSettings: `{"image_profile":{"profile":"openai_images","profile_version":1}}`}
	result, err := RunImageCompatibilityTest(context.Background(), channel, ImageCompatibilityTestRequest{PublicModel: "gpt-image-1", Endpoint: imageprofile.EndpointGenerations})
	require.NoError(t, err)
	assert.Equal(t, imageprofile.CompatibilityPassed, result.Status)
	assert.Equal(t, "vendor-image", receivedModel)
	assert.Len(t, result.ContractHash, 64)
}

func TestRunImageCompatibilityTestUsesChannelProxy(t *testing.T) {
	require.NoError(t, image_setting.UpdateCatalogByJSONString(`{"version":1,"models":{"gpt-image-1":{"profile":"openai_images","profile_version":1,"endpoints":{"generations":{"capability":{"enabled":true,"sizes":["1024x1024"],"qualities":["medium"],"response_formats":["b64_json"],"max_n":1},"default_size":"1024x1024","default_quality":"medium","default_response_format":"b64_json"}},"skus":{"gen-1024x1024-medium":{"endpoint":"generations","size":"1024x1024","quality":"medium","unit":"image","sale_price_usd":"0.1"}}}}}`))
	proxyUsed := false
	proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyUsed = true
		assert.Equal(t, "http://127.0.0.1:1/v1/images/generations", r.URL.String())
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"b64_json":"aGVsbG8="}]}`))
	}))
	defer proxyServer.Close()

	channel := &model.Channel{
		Id: 10, Type: constant.ChannelTypeOpenAI, Key: "test-key", Models: "gpt-image-1",
		BaseURL: stringPtr("http://127.0.0.1:1"), OtherSettings: `{"image_profile":{"profile":"openai_images","profile_version":1}}`,
	}
	channel.SetSetting(relaydto.ChannelSettings{Proxy: proxyServer.URL})

	result, err := RunImageCompatibilityTest(context.Background(), channel, ImageCompatibilityTestRequest{
		PublicModel: "gpt-image-1",
		Endpoint:    imageprofile.EndpointGenerations,
	})
	require.NoError(t, err)
	assert.True(t, proxyUsed)
	assert.Equal(t, imageprofile.CompatibilityPassed, result.Status)
}

func TestRunImageCompatibilityTestAppliesFixedHeaderOverrides(t *testing.T) {
	require.NoError(t, image_setting.UpdateCatalogByJSONString(`{"version":1,"models":{"gpt-image-1":{"profile":"openai_images","profile_version":1,"endpoints":{"generations":{"capability":{"enabled":true,"sizes":["1024x1024"],"qualities":["medium"],"response_formats":["b64_json"],"max_n":1},"default_size":"1024x1024","default_quality":"medium","default_response_format":"b64_json"}},"skus":{"gen-1024x1024-medium":{"endpoint":"generations","size":"1024x1024","quality":"medium","unit":"image","sale_price_usd":"0.1"}}}}}`))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "enabled", r.Header.Get("X-Vendor-Feature"))
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"b64_json":"aGVsbG8="}]}`))
	}))
	defer server.Close()

	headerOverride := `{"X-Vendor-Feature":"enabled","Authorization":"Bearer {api_key}"}`
	channel := &model.Channel{
		Id: 11, Type: constant.ChannelTypeOpenAI, Key: "test-key", Models: "gpt-image-1",
		BaseURL: &server.URL, HeaderOverride: &headerOverride,
		OtherSettings: `{"image_profile":{"profile":"openai_images","profile_version":1}}`,
	}
	result, err := RunImageCompatibilityTest(context.Background(), channel, ImageCompatibilityTestRequest{
		PublicModel: "gpt-image-1", Endpoint: imageprofile.EndpointGenerations,
	})
	require.NoError(t, err)
	assert.Equal(t, imageprofile.CompatibilityPassed, result.Status)
}

func TestRunImageCompatibilityTestAppliesGenerationParamOverride(t *testing.T) {
	require.NoError(t, image_setting.UpdateCatalogByJSONString(`{"version":1,"models":{"gpt-image-1":{"profile":"openai_images","profile_version":1,"endpoints":{"generations":{"capability":{"enabled":true,"sizes":["1024x1024"],"qualities":["medium"],"response_formats":["b64_json"],"max_n":1},"default_size":"1024x1024","default_quality":"medium","default_response_format":"b64_json"}},"skus":{"gen-1024x1024-medium":{"endpoint":"generations","size":"1024x1024","quality":"medium","unit":"image","sale_price_usd":"0.1"}}}}}`))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		require.NoError(t, common.DecodeJson(r.Body, &body))
		assert.Equal(t, "vendor compatibility test", body["prompt"])
		assert.Equal(t, "png", body["output_format"])
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"b64_json":"aGVsbG8="}]}`))
	}))
	defer server.Close()

	paramOverride := `{"prompt":"vendor compatibility test","output_format":"png"}`
	channel := &model.Channel{
		Id: 12, Type: constant.ChannelTypeOpenAI, Key: "test-key", Models: "gpt-image-1",
		BaseURL: &server.URL, ParamOverride: &paramOverride,
		OtherSettings: `{"image_profile":{"profile":"openai_images","profile_version":1}}`,
	}
	result, err := RunImageCompatibilityTest(context.Background(), channel, ImageCompatibilityTestRequest{
		PublicModel: "gpt-image-1", Endpoint: imageprofile.EndpointGenerations,
	})
	require.NoError(t, err)
	assert.Equal(t, imageprofile.CompatibilityPassed, result.Status)
}

func stringPtr(value string) *string { return &value }
