package doubao

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newNativeTaskContext(t *testing.T, body string) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(common.KeySeedanceOfficialAPI, true)
	return c
}

func TestValidateNativeRequest(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		wantErr  string
		wantCode string
	}{
		{
			name: "multimodal audio requires media and keeps zero values",
			body: `{"model":"doubao-seedance-2-0-260128","content":[{"type":"image_url","image_url":{"url":"https://example.com/a.png"},"role":"reference_image"},{"type":"audio_url","audio_url":{"url":"https://example.com/a.wav"},"role":"reference_audio"},{"type":"text","text":"make a video"}],"generate_audio":false,"watermark":false,"duration":-1}`,
		},
		{
			name: "mini exact id accepts bounded request",
			body: `{"model":"doubao-seedance-2-0-mini-260615","content":[{"type":"text","text":"a cat"}],"resolution":"720p","priority":9}`,
		},
		{
			name:     "duration zero rejected",
			body:     `{"model":"doubao-seedance-2-0-260128","content":[{"type":"text","text":"a cat"}],"duration":0}`,
			wantErr:  "duration must be positive",
			wantCode: "InvalidParameter",
		},
		{
			name:     "audio only rejected",
			body:     `{"model":"doubao-seedance-2-0-260128","content":[{"type":"audio_url","audio_url":{"url":"https://example.com/a.wav"},"role":"reference_audio"}]}`,
			wantErr:  "audio input requires",
			wantCode: "InvalidParameter.content",
		},
		{
			name:    "video reference role required",
			body:    `{"model":"doubao-seedance-2-0-260128","content":[{"type":"video_url","video_url":{"url":"https://example.com/a.mp4"}}]}`,
			wantErr: "reference_video",
		},
		{
			name:    "audio reference role required",
			body:    `{"model":"doubao-seedance-2-0-260128","content":[{"type":"image_url","image_url":{"url":"https://example.com/a.png"},"role":"reference_image"},{"type":"audio_url","audio_url":{"url":"https://example.com/a.wav"}}]}`,
			wantErr: "reference_audio",
		},
		{
			name:     "fast 1080p rejected",
			body:     `{"model":"doubao-seedance-2-0-fast-260128","content":[{"type":"text","text":"a cat"}],"resolution":"1080p"}`,
			wantErr:  "not supported",
			wantCode: "InvalidParameter",
		},
		{
			name:    "seedance 1.0 fast rejects last frame",
			body:    `{"model":"doubao-seedance-1-0-pro-fast-250528","content":[{"type":"image_url","image_url":{"url":"first"},"role":"first_frame"},{"type":"image_url","image_url":{"url":"last"},"role":"last_frame"}]}`,
			wantErr: "last_frame",
		},
		{
			name:    "draft task requires id",
			body:    `{"model":"doubao-seedance-1-5-pro-251215","content":[{"type":"draft_task","draft_task":{}}]}`,
			wantErr: "draft_task.id",
		},
		{
			name:    "too many reference images rejected",
			body:    `{"model":"doubao-seedance-2-0-260128","content":[{"type":"image_url","image_url":{"url":"1"},"role":"reference_image"},{"type":"image_url","image_url":{"url":"2"},"role":"reference_image"},{"type":"image_url","image_url":{"url":"3"},"role":"reference_image"},{"type":"image_url","image_url":{"url":"4"},"role":"reference_image"},{"type":"image_url","image_url":{"url":"5"},"role":"reference_image"},{"type":"image_url","image_url":{"url":"6"},"role":"reference_image"},{"type":"image_url","image_url":{"url":"7"},"role":"reference_image"},{"type":"image_url","image_url":{"url":"8"},"role":"reference_image"},{"type":"image_url","image_url":{"url":"9"},"role":"reference_image"},{"type":"image_url","image_url":{"url":"10"},"role":"reference_image"}]}`,
			wantErr: "reference media count",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newNativeTaskContext(t, tt.body)
			info := &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
			err := (&TaskAdaptor{}).ValidateRequestAndSetAction(c, info)
			if tt.wantErr != "" {
				require.NotNil(t, err)
				require.Contains(t, err.Message, tt.wantErr)
				if tt.wantCode != "" {
					require.Equal(t, tt.wantCode, err.Code)
				}
				return
			}
			require.Nil(t, err)
			req, getErr := relaycommon.GetTaskRequest(c)
			require.NoError(t, getErr)
			require.NotNil(t, req.Metadata)
			if tt.name == "multimodal audio requires media and keeps zero values" {
				require.False(t, c.GetBool(string(constant.ContextKeyTaskGenerateAudio)))
			}
		})
	}
}

func TestValidateNativeRequestMalformedBooleanUsesStableARKError(t *testing.T) {
	testCases := []struct {
		name string
		body string
	}{
		{
			name: "generate_audio string",
			body: `{"model":"doubao-seedance-1-5-pro-251215","content":[{"type":"text","text":"malformed boolean"}],"generate_audio":"not-a-bool"}`,
		},
		{
			name: "draft object",
			body: `{"model":"doubao-seedance-1-5-pro-251215","content":[{"type":"text","text":"malformed boolean"}],"draft":{"value":true}}`,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			c := newNativeTaskContext(t, testCase.body)
			info := &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
			taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(c, info)
			require.NotNil(t, taskErr)
			require.Equal(t, "InvalidParameter", taskErr.Code)
			require.Equal(t, "request body contains invalid parameters", taskErr.Message)
			for _, internalDetail := range []string{"json:", "cannot unmarshal", "Go struct field", "Go value"} {
				require.NotContains(t, taskErr.Message, internalDetail)
			}
		})
	}
}

func TestBuildNativeRequestBodyAppliesMappedModel(t *testing.T) {
	c := newNativeTaskContext(t, `{"model":"alias","content":[{"type":"text","text":"a cat"}],"watermark":false,"duration":0,"unknown_field":{"preserve":true}}`)
	// Use a valid request for validation; the body rewrite itself must preserve
	// the explicit zero-valued duration even when the test bypasses validation.
	info := &relaycommon.RelayInfo{
		ChannelMeta:   &relaycommon.ChannelMeta{UpstreamModelName: "doubao-seedance-2-0-260128"},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
	}
	body, err := (&TaskAdaptor{}).BuildRequestBody(c, info)
	require.NoError(t, err)
	data, readErr := io.ReadAll(body)
	require.NoError(t, readErr)
	var fields map[string]json.RawMessage
	require.NoError(t, common.Unmarshal(data, &fields))
	var modelName string
	require.NoError(t, common.Unmarshal(fields["model"], &modelName))
	require.Equal(t, "doubao-seedance-2-0-260128", modelName)
	require.Contains(t, string(data), `"duration":0`)
	require.Contains(t, string(data), `"watermark":false`)
	require.Contains(t, string(data), `"unknown_field":{"preserve":true}`)
}

func TestNativeCapabilityRoutingStripsExtensionAndUsesPrevalidatedResolution(t *testing.T) {
	body := `{
		"model":"doubao-seedance-2-0-fast-260128",
		"content":[{"type":"text","text":"a cat"}],
		"resolution":"1080p",
		"routing":{"require_real_person":true}
	}`
	c := newNativeTaskContext(t, body)
	common.SetContextKey(c, constant.ContextKeyRoutingCapabilityMode, true)
	info := &relaycommon.RelayInfo{
		ChannelMeta:   &relaycommon.ChannelMeta{UpstreamModelName: "provider-fast-1080p"},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
	}
	require.Nil(t, (&TaskAdaptor{}).ValidateRequestAndSetAction(c, info))
	taskRequest, err := relaycommon.GetTaskRequest(c)
	require.NoError(t, err)
	require.NotContains(t, taskRequest.Metadata, "routing")

	requestBody, err := (&TaskAdaptor{}).BuildRequestBody(c, info)
	require.NoError(t, err)
	data, err := io.ReadAll(requestBody)
	require.NoError(t, err)
	var fields map[string]json.RawMessage
	require.NoError(t, common.Unmarshal(data, &fields))
	require.NotContains(t, fields, "routing")
	require.Contains(t, string(data), `"model":"provider-fast-1080p"`)
}
