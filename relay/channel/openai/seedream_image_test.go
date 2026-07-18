package openai

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSeedreamGeneratedImagesIsAuthoritative(t *testing.T) {
	c, recorder, resp, info := newImageTestContext(t, `{"data":[{"url":"a"},{"url":"b"},{"url":"c"}],"usage":{"generated_images":2,"output_tokens":7,"input_images":4}}`, "application/json", false)
	c.Set(common.KeySeedanceOfficialAPI, true)
	info.PriceData.UsePrice = true
	info.PriceData.AddOtherRatio("n", 3)
	usage, err := OpenaiImageHandler(c, info, resp)
	require.Nil(t, err)
	require.NotNil(t, usage)
	assert.Equal(t, 2, usage.GeneratedImages)
	assert.Equal(t, 7, usage.CompletionTokens)
	assert.Equal(t, 7, usage.TotalTokens)
	assert.Equal(t, 4, usage.InputImages)
	assert.Equal(t, 2.0, info.PriceData.OtherRatios()["n"])
	assert.Equal(t, http.StatusOK, recorder.Code)
}

func TestSeedreamGeneratedImagesZeroRemovesImageRatio(t *testing.T) {
	c, _, resp, info := newImageTestContext(t, `{"data":[{"url":"failed"}],"usage":{"generated_images":0,"output_tokens":0}}`, "application/json", false)
	c.Set(common.KeySeedanceOfficialAPI, true)
	info.PriceData.UsePrice = true
	info.PriceData.AddOtherRatio("n", 3)
	usage, err := OpenaiImageHandler(c, info, resp)
	require.Nil(t, err)
	require.NotNil(t, usage)
	assert.Equal(t, 0, usage.TotalTokens)
	assert.NotContains(t, info.PriceData.OtherRatios(), "n")
}

func TestSeedreamGeneratedImagesZeroKeepsZeroTotalTokens(t *testing.T) {
	c, _, resp, info := newImageTestContext(t, `{"data":[{"error":{"code":"content_filter"}}],"usage":{"generated_images":0,"output_tokens":123}}`, "application/json", false)
	c.Set(common.KeySeedanceOfficialAPI, true)
	info.PriceData.UsePrice = true
	info.PriceData.AddOtherRatio("n", 3)

	usage, err := OpenaiImageHandler(c, info, resp)
	require.Nil(t, err)
	require.NotNil(t, usage)
	assert.Equal(t, 123, usage.CompletionTokens)
	assert.Equal(t, 0, usage.TotalTokens)
	assert.NotContains(t, info.PriceData.OtherRatios(), "n")
}
