package helper

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newSeedreamImageContext(t *testing.T, body string) *gin.Context {
	t.Helper()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v3/images/generations", bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(common.KeySeedanceOfficialAPI, true)
	return c
}

func TestNormalizeSeedreamNativeImageRequestBoundsOutput(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantN   uint
		wantErr string
	}{
		{name: "references reduce output", body: `{"model":"doubao-seedream-5-0-lite-260128","prompt":"x","image":["a","b"],"sequential_image_generation":"auto","sequential_image_generation_options":{"max_images":15}}`, wantN: 13},
		{name: "disabled estimates one", body: `{"model":"doubao-seedream-5-0-lite-260128","prompt":"x","sequential_image_generation":"disabled"}`, wantN: 1},
		{name: "options require auto", body: `{"model":"doubao-seedream-5-0-lite-260128","prompt":"x","sequential_image_generation":"disabled","sequential_image_generation_options":{"max_images":2}}`, wantErr: "requires"},
		{name: "input capacity exhausted", body: `{"model":"doubao-seedream-5-0-lite-260128","prompt":"x","image":["1","2","3","4","5","6","7","8","9","10","11","12","13","14","15"],"sequential_image_generation":"auto"}`, wantErr: "less than 15"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newSeedreamImageContext(t, tt.body)
			req, err := GetAndValidOpenAIImageRequest(c, relayconstant.RelayModeImagesGenerations)
			require.NoError(t, err)
			err = NormalizeSeedreamNativeImageRequest(c, req)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, req.N)
			assert.Equal(t, tt.wantN, *req.N)
		})
	}
}

func TestValidateSeedreamNativeModelRequestCapabilities(t *testing.T) {
	pro := newSeedreamImageContext(t, `{"model":"alias","prompt":"x"}`)
	request := &dto.ImageRequest{Model: "alias", N: common.GetPointer(uint(1))}
	assert.NoError(t, ValidateSeedreamNativeModelRequest(pro, request, "doubao-seedream-5-0-260128"))

	proUnsupported := newSeedreamImageContext(t, `{"model":"alias","prompt":"x","stream":true}`)
	assert.Error(t, ValidateSeedreamNativeModelRequest(proUnsupported, request, "doubao-seedream-5-0-260128"))

	lite := newSeedreamImageContext(t, `{"model":"alias","prompt":"x","sequential_image_generation":"auto","sequential_image_generation_options":{"max_images":3},"stream":true}`)
	assert.NoError(t, ValidateSeedreamNativeModelRequest(lite, request, "doubao-seedream-5-0-lite-260128"))

	fourFive := newSeedreamImageContext(t, `{"model":"alias","prompt":"x","output_format":"png"}`)
	assert.Error(t, ValidateSeedreamNativeModelRequest(fourFive, request, "doubao-seedream-4-5-251128"))
}
