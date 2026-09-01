package service

import (
	"testing"

	appconstant "github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/image_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveImageRequestUsesCatalogDefaultsAndInputContract(t *testing.T) {
	raw := `{"version":1,"models":{"gpt-image-1":{"profile":"openai_images","profile_version":1,"endpoints":{"generations":{"capability":{"enabled":true,"sizes":["1024x1024"],"qualities":["medium"],"response_formats":["b64_json"],"max_n":4},"default_size":"1024x1024","default_quality":"medium","default_response_format":"b64_json"}},"skus":{"gen-1024x1024-medium":{"endpoint":"generations","size":"1024x1024","quality":"medium","unit":"image","sale_price_usd":"0.04"}}}}}`
	original := image_setting.Catalog2JSONString()
	t.Cleanup(func() { require.NoError(t, image_setting.UpdateCatalogByJSONString(original)) })
	require.NoError(t, image_setting.UpdateCatalogByJSONString(raw))
	request := &dto.ImageRequest{Model: "gpt-image-1", Prompt: "a cat", N: pointer(uint(2))}
	context, err := ResolveImageRequest(request, relayconstant.RelayModeImagesGenerations)
	require.NoError(t, err)
	assert.Equal(t, "1024x1024", context.Resolved.Size)
	assert.Equal(t, "medium", context.Resolved.Quality)
	assert.Equal(t, uint(2), context.Resolved.N)
	assert.Equal(t, "gen-1024x1024-medium", context.Resolved.SKUKey)
}

func TestEvaluateImageChannelRequiresBindingAndRespectsMappingAndCapabilities(t *testing.T) {
	raw := `{"version":1,"models":{"public-image":{"profile":"openai_images","profile_version":1,"endpoints":{"generations":{"capability":{"enabled":true,"sizes":["1024x1024"],"qualities":["medium"],"response_formats":["b64_json"],"max_n":2},"default_size":"1024x1024","default_quality":"medium","default_response_format":"b64_json"}},"skus":{"gen-1024x1024-medium":{"endpoint":"generations","size":"1024x1024","quality":"medium","unit":"image","sale_price_usd":"0.04"}}}}}`
	original := image_setting.Catalog2JSONString()
	t.Cleanup(func() { require.NoError(t, image_setting.UpdateCatalogByJSONString(original)) })
	require.NoError(t, image_setting.UpdateCatalogByJSONString(raw))
	request, err := ResolveImageRequest(&dto.ImageRequest{Model: "public-image", Prompt: "x", N: pointer(uint(1))}, relayconstant.RelayModeImagesGenerations)
	require.NoError(t, err)

	withoutBinding := &model.Channel{Id: 1, Type: appconstant.ChannelTypeOpenAI, Models: "public-image"}
	_, err = EvaluateImageChannel(withoutBinding, "public-image", request)
	require.ErrorContains(t, err, "no image profile")

	mapping := `{"public-image":"vendor-image"}`
	withBinding := &model.Channel{Id: 2, Type: appconstant.ChannelTypeOpenAI, Models: "public-image", ModelMapping: &mapping, OtherSettings: `{"image_profile":{"profile":"openai_images","profile_version":1,"capability_overrides":{"public-image":{"sizes":["512x512"]}}}}`}
	_, err = EvaluateImageChannel(withBinding, "public-image", request)
	require.ErrorContains(t, err, "size")

	withBinding.OtherSettings = `{"image_profile":{"profile":"openai_images","profile_version":1}}`
	eligibility, err := EvaluateImageChannel(withBinding, "public-image", request)
	require.NoError(t, err)
	assert.Equal(t, "vendor-image", eligibility.UpstreamModel)
}

func TestEvaluateImageChannelHonorsExplicitEndpointDisable(t *testing.T) {
	raw := `{"version":1,"models":{"public-image":{"profile":"openai_images","profile_version":1,"endpoints":{"generations":{"capability":{"enabled":true,"sizes":["1024x1024"],"qualities":["medium"],"response_formats":["b64_json"],"max_n":1},"default_size":"1024x1024","default_quality":"medium","default_response_format":"b64_json"},"edits":{"capability":{"enabled":true,"sizes":["1024x1024"],"qualities":["medium"],"response_formats":["b64_json"],"max_n":1,"max_input_images":1,"supports_mask":true},"default_size":"1024x1024","default_quality":"medium","default_response_format":"b64_json"}},"skus":{"gen-1024x1024-medium":{"endpoint":"generations","size":"1024x1024","quality":"medium","unit":"image","sale_price_usd":"0.04"},"edit-1024x1024-medium":{"endpoint":"edits","size":"1024x1024","quality":"medium","unit":"image","sale_price_usd":"0.04"}}}}}`
	original := image_setting.Catalog2JSONString()
	t.Cleanup(func() { require.NoError(t, image_setting.UpdateCatalogByJSONString(original)) })
	require.NoError(t, image_setting.UpdateCatalogByJSONString(raw))
	request, err := ResolveImageRequest(&dto.ImageRequest{Model: "public-image", Prompt: "x", N: pointer(uint(1)), InputImageCount: 1}, relayconstant.RelayModeImagesEdits)
	require.NoError(t, err)
	mapping := `{"public-image":"vendor-image"}`
	channel := &model.Channel{Id: 3, Type: appconstant.ChannelTypeOpenAI, Models: "public-image", ModelMapping: &mapping, OtherSettings: `{"image_profile":{"profile":"openai_images","profile_version":1,"capability_overrides":{"public-image":{"edits":false}}}}`}
	_, err = EvaluateImageChannel(channel, "public-image", request)
	require.ErrorContains(t, err, "edits capability is disabled")
}

func pointer(value uint) *uint { return &value }
