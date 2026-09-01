package relay

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildUnifiedImageRequestBodyPreservesPassthroughFieldsAndAppliesResolvedValues(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewBufferString(`{"model":"gpt-image-1","prompt":"a cat","n":1,"size":"256x256","provider_option":{"seed":0}}`))
	c.Request.Header.Set("Content-Type", "application/json")

	request := dto.ImageRequest{
		Model:          "provider-image",
		N:              common.GetPointer(uint(2)),
		Size:           "1024x1024",
		Quality:        "medium",
		ResponseFormat: "b64_json",
	}
	body, err := buildUnifiedImageRequestBody(c, request)
	require.NoError(t, err)

	var fields map[string]any
	require.NoError(t, common.Unmarshal(body, &fields))
	assert.Equal(t, "provider-image", fields["model"])
	assert.Equal(t, float64(2), fields["n"])
	assert.Equal(t, "1024x1024", fields["size"])
	assert.Equal(t, "medium", fields["quality"])
	assert.Equal(t, "b64_json", fields["response_format"])
	assert.Equal(t, "a cat", fields["prompt"])
	assert.Equal(t, map[string]any{"seed": float64(0)}, fields["provider_option"])
}
