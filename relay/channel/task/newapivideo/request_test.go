package newapivideo

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newOpenAIRequestContext(body, contentType string) *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/video/generations", bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", contentType)
	return c
}

func newRelayInfo() *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
}

func TestValidateOpenAIRequest(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		contentType string
		code        string
		status      int
		wantErr     bool
	}{
		{name: "json charset", body: `{"model":"m","prompt":"x"}`, contentType: "application/json; charset=utf-8"},
		{name: "multipart", body: `{"model":"m","prompt":"x"}`, contentType: "multipart/form-data; boundary=test", code: "unsupported_media_type", status: http.StatusUnsupportedMediaType, wantErr: true},
		{name: "damaged", body: `{bad`, contentType: "application/json", code: "invalid_json", status: http.StatusBadRequest, wantErr: true},
		{name: "array root", body: `[]`, contentType: "application/json", code: "invalid_request", status: http.StatusBadRequest, wantErr: true},
		{name: "missing model", body: `{"prompt":"x"}`, contentType: "application/json", code: "missing_model", status: http.StatusBadRequest, wantErr: true},
		{name: "missing prompt", body: `{"model":"m"}`, contentType: "application/json", code: "invalid_request", status: http.StatusBadRequest, wantErr: true},
		{name: "empty prompt", body: `{"model":"m","prompt":" "}`, contentType: "application/json", code: "invalid_request", status: http.StatusBadRequest, wantErr: true},
		{name: "duration zero", body: `{"model":"m","prompt":"x","duration":0}`, contentType: "application/json", code: "invalid_duration", status: http.StatusBadRequest, wantErr: true},
		{name: "duration overflow", body: `{"model":"m","prompt":"x","duration":3601}`, contentType: "application/json", code: "invalid_duration", status: http.StatusBadRequest, wantErr: true},
		{name: "seconds string", body: `{"model":"m","prompt":"x","seconds":"5"}`, contentType: "application/json"},
		{name: "seconds number", body: `{"model":"m","prompt":"x","seconds":5}`, contentType: "application/json", code: "invalid_seconds", status: http.StatusBadRequest, wantErr: true},
		{name: "conflicting duration", body: `{"model":"m","prompt":"x","duration":5,"seconds":"6"}`, contentType: "application/json", code: "invalid_duration", status: http.StatusBadRequest, wantErr: true},
		{name: "n one", body: `{"model":"m","prompt":"x","n":1}`, contentType: "application/json"},
		{name: "n huge", body: `{"model":"m","prompt":"x","n":18446744073686646784}`, contentType: "application/json", code: "invalid_n", status: http.StatusBadRequest, wantErr: true},
		{name: "metadata bypass", body: `{"model":"m","prompt":"x","duration":5,"metadata":{"duration":3601}}`, contentType: "application/json", code: "invalid_duration", status: http.StatusBadRequest, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newOpenAIRequestContext(tt.body, tt.contentType)
			err := validateOpenAIRequest(c, newRelayInfo(), []byte(tt.body))
			if tt.wantErr {
				require.NotNil(t, err)
				assert.Equal(t, tt.code, err.Code)
				assert.Equal(t, tt.status, err.StatusCode)
				return
			}
			require.Nil(t, err)
		})
	}
}

func TestBuildOpenAIRequestBodyPreservesFieldsExceptModel(t *testing.T) {
	body := `{"model":"client","prompt":"x","watermark":false,"seed":0,"duration":5.5,"unknown":{"zero":0,"flag":false}}`
	c := newOpenAIRequestContext(body, "application/json")
	info := newRelayInfo()
	require.Nil(t, validateOpenAIRequest(c, info, []byte(body)))

	out, err := buildOpenAIRequestBody(c, "provider-model")
	require.NoError(t, err)
	assert.JSONEq(t, `{"model":"provider-model","prompt":"x","watermark":false,"seed":0,"duration":5.5,"unknown":{"zero":0,"flag":false}}`, string(out))

	req, err := relaycommon.GetTaskRequest(c)
	require.NoError(t, err)
	assert.Equal(t, constant.TaskActionGenerate, info.Action)
	assert.Equal(t, "client", req.Model)
	assert.Equal(t, "x", req.Prompt)
}

func TestBuildOpenAIRequestBodyPreservesMixedMediaContent(t *testing.T) {
	body := `{"model":"client","prompt":"make a cinematic scene","content":[{"type":"text","text":"A rainy city"},{"type":"reference_image","image_url":"https://example.com/a.png"},{"type":"reference_image","image_url":"https://example.com/b.png"},{"type":"reference_video","video_url":"https://example.com/ref.mp4"},{"type":"reference_audio","audio_url":"https://example.com/ref.aac"}],"generateAudio":true,"seconds":"10"}`
	c := newOpenAIRequestContext(body, "application/json")
	require.Nil(t, validateOpenAIRequest(c, newRelayInfo(), []byte(body)))
	out, err := buildOpenAIRequestBody(c, "seedance-2.0")
	require.NoError(t, err)
	assert.JSONEq(t, `{"model":"seedance-2.0","prompt":"make a cinematic scene","content":[{"type":"text","text":"A rainy city"},{"type":"reference_image","image_url":"https://example.com/a.png"},{"type":"reference_image","image_url":"https://example.com/b.png"},{"type":"reference_video","video_url":"https://example.com/ref.mp4"},{"type":"reference_audio","audio_url":"https://example.com/ref.aac"}],"generateAudio":true,"seconds":"10"}`, string(out))
}
