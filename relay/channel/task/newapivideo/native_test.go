package newapivideo

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestARKToUpstream(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		upstream   string
		expectBody string
	}{
		{
			name:       "text only",
			body:       `{"model":"client","content":[{"type":"text","text":"text"}]}`,
			upstream:   "provider-720p",
			expectBody: `{"model":"provider-720p","prompt":"text"}`,
		},
		{
			name:       "single first frame",
			body:       `{"model":"client","content":[{"type":"text","text":"text"},{"type":"image_url","image_url":{"url":"https://x/first.png"},"role":"first_frame"}]}`,
			upstream:   "provider-720p",
			expectBody: `{"model":"provider-720p","prompt":"text","image":"https://x/first.png"}`,
		},
		{
			name:       "first and last frames",
			body:       `{"model":"client","content":[{"type":"text","text":"text"},{"type":"image_url","image_url":{"url":"https://x/first.png"},"role":"first_frame"},{"type":"image_url","image_url":{"url":"https://x/last.png"},"role":"last_frame"}]}`,
			upstream:   "provider-720p",
			expectBody: `{"model":"provider-720p","prompt":"text","image_with_roles":[{"url":"https://x/first.png","role":"first_frame"},{"url":"https://x/last.png","role":"last_frame"}]}`,
		},
		{
			name:       "mixed reference media",
			body:       `{"model":"client","content":[{"type":"text","text":"text"},{"type":"image_url","image_url":{"url":"https://x/a.png"},"role":"reference_image"},{"type":"image_url","image_url":{"url":"https://x/b.png"},"role":"reference_image"},{"type":"video_url","video_url":{"url":"https://x/a.mp4"},"role":"reference_video"},{"type":"audio_url","audio_url":{"url":"https://x/a.mp3"},"role":"reference_audio"}],"generate_audio":true,"duration":10}`,
			upstream:   "provider-720p",
			expectBody: `{"model":"provider-720p","prompt":"text","content":[{"type":"text","text":"text"},{"type":"image_url","image_url":{"url":"https://x/a.png"},"role":"reference_image"},{"type":"image_url","image_url":{"url":"https://x/b.png"},"role":"reference_image"},{"type":"video_url","video_url":{"url":"https://x/a.mp4"},"role":"reference_video"},{"type":"audio_url","audio_url":{"url":"https://x/a.mp3"},"role":"reference_audio"}],"generateAudio":true,"seconds":"10"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request, err := parseARKRequest([]byte(tt.body))
			require.NoError(t, err)
			translated, err := arkToUpstream(request, tt.upstream, false)
			require.NoError(t, err)
			encoded, err := marshalUpstreamRequest(translated)
			require.NoError(t, err)
			assert.JSONEq(t, tt.expectBody, string(encoded))
		})
	}
}

func TestARKRequestStoresTypedState(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", nil)
	c.Request.Header.Set("Content-Type", "application/json")
	info := &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
	body := []byte(`{"model":"client","content":[{"type":"text","text":"text"}],"duration":10}`)
	require.Nil(t, validateARKRequest(c, info, body))
	state, err := getRequestState(c)
	require.NoError(t, err)
	require.NotNil(t, state.ARK)
	assert.Equal(t, "client", state.ARK.Model)
	assert.Equal(t, 10, *state.ARK.Duration)
	assert.Equal(t, constant.TaskActionGenerate, info.Action)
	taskRequest, err := relaycommon.GetTaskRequest(c)
	require.NoError(t, err)
	assert.Equal(t, "text", taskRequest.Prompt)
	assert.Equal(t, 10, taskRequest.Duration)
}

func TestARKRejects(t *testing.T) {
	tests := []struct {
		name string
		body string
		code string
	}{
		{name: "draft task", body: `{"model":"m","content":[{"type":"text","text":"text"},{"type":"draft_task","draft_task":{"id":"x"}}]}`, code: "InvalidParameter.content"},
		{name: "draft true", body: `{"model":"m","content":[{"type":"text","text":"text"}],"draft":true}`, code: "InvalidParameter.draft"},
		{name: "tools", body: `{"model":"m","content":[{"type":"text","text":"text"}],"tools":[{"type":"web_search"}]}`, code: "InvalidParameter.tools"},
		{name: "two text items", body: `{"model":"m","content":[{"type":"text","text":"one"},{"type":"text","text":"two"}]}`, code: "InvalidParameter.content"},
		{name: "missing text", body: `{"model":"m","content":[{"type":"image_url","image_url":{"url":"https://x/a.png"},"role":"first_frame"}]}`, code: "InvalidParameter.content"},
		{name: "malformed media URL", body: `{"model":"m","content":[{"type":"text","text":"text"},{"type":"image_url","image_url":{"url":"://bad"},"role":"first_frame"}]}`, code: "InvalidParameter.content"},
		{name: "unsupported role", body: `{"model":"m","content":[{"type":"text","text":"text"},{"type":"image_url","image_url":{"url":"https://x/a.png"},"role":"thumbnail"}]}`, code: "InvalidParameter.content"},
		{name: "false audio", body: `{"model":"m","content":[{"type":"text","text":"text"},{"type":"image_url","image_url":{"url":"https://x/a.png"},"role":"reference_image"},{"type":"audio_url","audio_url":{"url":"https://x/a.mp3"},"role":"reference_audio"}],"generate_audio":false}`, code: "InvalidParameter.generate_audio"},
		{name: "mixed first and reference", body: `{"model":"m","content":[{"type":"text","text":"text"},{"type":"image_url","image_url":{"url":"https://x/a.png"},"role":"first_frame"},{"type":"image_url","image_url":{"url":"https://x/b.png"},"role":"reference_image"}]}`, code: "InvalidParameter.content"},
		{name: "seed", body: `{"model":"m","content":[{"type":"text","text":"text"}],"seed":0}`, code: "InvalidParameter.seed"},
		{name: "frames", body: `{"model":"m","content":[{"type":"text","text":"text"}],"frames":1}`, code: "InvalidParameter.frames"},
		{name: "camera fixed", body: `{"model":"m","content":[{"type":"text","text":"text"}],"camera_fixed":false}`, code: "InvalidParameter.camera_fixed"},
		{name: "return last frame", body: `{"model":"m","content":[{"type":"text","text":"text"}],"return_last_frame":false}`, code: "InvalidParameter.return_last_frame"},
		{name: "priority", body: `{"model":"m","content":[{"type":"text","text":"text"}],"priority":0}`, code: "InvalidParameter.priority"},
		{name: "expiry", body: `{"model":"m","content":[{"type":"text","text":"text"}],"execution_expires_after":3600}`, code: "InvalidParameter.execution_expires_after"},
		{name: "safety identifier", body: `{"model":"m","content":[{"type":"text","text":"text"}],"safety_identifier":"x"}`, code: "InvalidParameter.safety_identifier"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request, err := parseARKRequest([]byte(tt.body))
			if err == nil {
				_, err = arkToUpstream(request, "provider-720p", false)
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.code)
		})
	}
}

func TestARKDurationAndBooleanSemantics(t *testing.T) {
	request, err := parseARKRequest([]byte(`{"model":"m","content":[{"type":"text","text":"text"}],"generate_audio":false,"duration":10}`))
	require.NoError(t, err)
	translated, err := arkToUpstream(request, "provider-720p", false)
	require.NoError(t, err)
	encoded, err := marshalUpstreamRequest(translated)
	require.NoError(t, err)
	assert.JSONEq(t, `{"model":"provider-720p","prompt":"text","generateAudio":false,"seconds":"10"}`, string(encoded))
	assert.NotContains(t, string(encoded), `"duration"`)
}

func TestARKAcceptsExplicitUnsupportedFeatureZeros(t *testing.T) {
	request, err := parseARKRequest([]byte(`{"model":"m","content":[{"type":"text","text":"text"}],"draft":false,"tools":[]}`))
	require.NoError(t, err)
	translated, err := arkToUpstream(request, "provider-720p", false)
	require.NoError(t, err)
	encoded, err := marshalUpstreamRequest(translated)
	require.NoError(t, err)
	assert.JSONEq(t, `{"model":"provider-720p","prompt":"text"}`, string(encoded))
}

func TestARKReferenceAudioEnablesOutputAudio(t *testing.T) {
	request, err := parseARKRequest([]byte(`{"model":"m","content":[{"type":"text","text":"text"},{"type":"video_url","video_url":{"url":"https://x/a.mp4"},"role":"reference_video"},{"type":"audio_url","audio_url":{"url":"https://x/a.mp3"},"role":"reference_audio"}]}`))
	require.NoError(t, err)
	translated, err := arkToUpstream(request, "provider-720p", false)
	require.NoError(t, err)
	encoded, err := marshalUpstreamRequest(translated)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), `"generateAudio":true`)
}

func TestValidateMappedResolution(t *testing.T) {
	assert.NoError(t, validateMappedResolution("720p", "seedance-720p-token"))
	assert.NoError(t, validateMappedResolution("", "provider-model"))
	assert.Error(t, validateMappedResolution("1080p", "seedance-720p-token"))
	assert.Error(t, validateMappedResolution("720p", "provider-model"))
}

func TestARKCapabilityRoutingStripsExtensionAndUsesPrevalidatedResolution(t *testing.T) {
	request, err := parseARKRequest([]byte(`{
		"model":"doubao-seedance-2-0-260128",
		"content":[{"type":"text","text":"text"}],
		"resolution":"1080p",
		"routing":{"require_real_person":true}
	}`))
	require.NoError(t, err)

	translated, err := arkToUpstream(request, "lec-feituo-seedance-2-0-my-upscaled-1080p", true)
	require.NoError(t, err)
	encoded, err := marshalUpstreamRequest(translated)
	require.NoError(t, err)
	assert.JSONEq(t, `{"model":"lec-feituo-seedance-2-0-my-upscaled-1080p","prompt":"text"}`, string(encoded))
	assert.NotContains(t, string(encoded), `"routing"`)
}
