package newapivideo

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var expectedPaipuModels = []string{
	"lec-sz-seedance-2-0-480p",
	"lec-gongteng-seedance-2-0-720p",
	"lec-gongteng-seedance-2-0-fast-720p",
	"lec-gongteng-seedance-2-0-1080p",
	"lec-seedance-2-0",
	"lec-feituo-seedance-2-0-hn-fast-720p",
	"lec-feituo-seedance-2-0-hn-720p",
	"lec-feituo-seedance-2-0-xh-fast-933-720p",
	"lec-feituo-seedance-2-0-xh-pro-933-720p",
	"lec-feituo-seedance-2-0-ld-cvk-2",
	"lec-feituo-seedance-2-0-limited-720p",
	"lec-feituo-seedance-2-0-my-fast-upscaled-1080p",
	"lec-feituo-seedance-2-0-my-upscaled-1080p",
	"lec-seedance-videos-standard",
	"lec-seedance-videos-face-standard",
	"lec-seedance-videos-face-fast",
	"lec-seedance-videos-stable",
	"lec-seedance-videos-stable-fast",
	"lec-seedance-videos-stable-mini",
	"lec-seedance-videos-stable-720p",
	"lec-seedance-videos-fast-720p",
	"lec-seedance-videos-mini-720p",
	"lec-seedance-videos-fast",
	"lec-seedance-videos-mini",
}

func TestPaipuProfile(t *testing.T) {
	adaptor := NewPaipuTaskAdaptor()
	assert.Equal(t, "Paipu", adaptor.GetChannelName())
	assert.Equal(t, expectedPaipuModels, adaptor.GetModelList())
	assert.Equal(t, "/v1/videos", adaptor.activeProfile().submitPath)
	assert.Equal(t, "/v1/videos/{task_id}", adaptor.activeProfile().pollPath)
	assert.Equal(t, "ratio", adaptor.activeProfile().textRequest.ratioField)
}

func TestBuildPaipuTextRequest(t *testing.T) {
	request, err := parseARKRequest([]byte(`{
		"model":"doubao-seedance-2-0-260128",
		"content":[{"type":"text","text":"海边日落，固定机位"}],
		"duration":8,"ratio":"16:9","resolution":"720p"
	}`), paipuProtocolProfile())
	require.NoError(t, err)
	body, err := buildTextVideoRequest(request, "lec-gongteng-seedance-2-0-720p", *paipuProtocolProfile().textRequest)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"model":"lec-gongteng-seedance-2-0-720p","prompt":"海边日落，固定机位",
		"duration":8,"ratio":"16:9","resolution":"720p"
	}`, string(body))
}

func TestBuildPaipuTextRequestOmitsAbsentScalars(t *testing.T) {
	request, err := parseARKRequest([]byte(`{
		"model":"doubao-seedance-2-0-260128",
		"content":[{"type":"text","text":"minimal"}]
	}`), paipuProtocolProfile())
	require.NoError(t, err)
	body, err := buildTextVideoRequest(request, "lec-seedance-2-0", *paipuProtocolProfile().textRequest)
	require.NoError(t, err)
	assert.JSONEq(t, `{"model":"lec-seedance-2-0","prompt":"minimal"}`, string(body))
}

func TestPaipuResolutionSuffixValidation(t *testing.T) {
	tests := []struct {
		name       string
		model      string
		resolution string
		wantErr    bool
	}{
		{name: "480p matches", model: "lec-sz-seedance-2-0-480p", resolution: "480p"},
		{name: "480p rejects 720p", model: "lec-sz-seedance-2-0-480p", resolution: "720p", wantErr: true},
		{name: "720p matches", model: "lec-feituo-seedance-2-0-hn-720p", resolution: "720p"},
		{name: "720p rejects 1080p", model: "lec-feituo-seedance-2-0-hn-720p", resolution: "1080p", wantErr: true},
		{name: "1080p matches", model: "lec-feituo-seedance-2-0-my-upscaled-1080p", resolution: "1080p"},
		{name: "unsuffixed accepts route resolution", model: "lec-seedance-videos-standard", resolution: "720p"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := arkRequest{Model: "client", Content: []arkContent{{Type: "text", Text: "text"}}, Resolution: tt.resolution}
			err := validateTextVideoRequest(request, *paipuProtocolProfile().textRequest, tt.model)
			if tt.wantErr {
				var requestErr *arkRequestError
				require.ErrorAs(t, err, &requestErr)
				assert.Equal(t, "InvalidParameter.resolution", requestErr.Code)
				return
			}
			assert.NoError(t, err)
		})
	}
}

func TestPaipuRejectsUnsupportedRequestsBeforeBuild(t *testing.T) {
	tests := []struct {
		name string
		body string
		code string
	}{
		{name: "image", body: `{"model":"m","content":[{"type":"text","text":"text"},{"type":"image_url","image_url":{"url":"https://x/ref.png"}}]}`, code: "InvalidParameter.content"},
		{name: "video", body: `{"model":"m","content":[{"type":"text","text":"text"},{"type":"video_url","role":"reference_video","video_url":{"url":"https://x/ref.mp4"}}]}`, code: "InvalidParameter.content"},
		{name: "audio", body: `{"model":"m","content":[{"type":"text","text":"text"},{"type":"audio_url","role":"reference_audio","audio_url":{"url":"https://x/ref.mp3"}}]}`, code: "InvalidParameter.content"},
		{name: "draft task", body: `{"model":"m","content":[{"type":"text","text":"text"},{"type":"draft_task","draft_task":{"id":"task"}}]}`, code: "InvalidParameter.content"},
		{name: "generate audio", body: `{"model":"m","content":[{"type":"text","text":"text"}],"generate_audio":true}`, code: "InvalidParameter.generate_audio"},
		{name: "draft", body: `{"model":"m","content":[{"type":"text","text":"text"}],"draft":true}`, code: "InvalidParameter.draft"},
		{name: "tools", body: `{"model":"m","content":[{"type":"text","text":"text"}],"tools":[{"type":"camera_fixed"}]}`, code: "InvalidParameter.tools"},
		{name: "service tier", body: `{"model":"m","content":[{"type":"text","text":"text"}],"service_tier":"flex"}`, code: "InvalidParameter.service_tier"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", strings.NewReader(tt.body))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Set(common.KeySeedanceOfficialAPI, true)
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "lec-seedance-2-0"}, TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
			adaptor := NewPaipuTaskAdaptor()
			taskErr := adaptor.ValidateRequestAndSetAction(c, info)
			require.NotNil(t, taskErr)
			assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
			assert.Equal(t, tt.code, taskErr.Code)
			_, buildErr := adaptor.BuildRequestBody(c, info)
			assert.Error(t, buildErr)
		})
	}
}

func TestPaipuAllowsDefaultServiceTierAndOmitsItUpstream(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", strings.NewReader(`{"model":"client","content":[{"type":"text","text":"text"}],"service_tier":"default"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(common.KeySeedanceOfficialAPI, true)
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "lec-seedance-2-0"}, TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
	adaptor := NewPaipuTaskAdaptor()
	require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))
	reader, err := adaptor.BuildRequestBody(c, info)
	require.NoError(t, err)
	body, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.JSONEq(t, `{"model":"lec-seedance-2-0","prompt":"text"}`, string(body))
}

func TestPaipuValidateBillingRequestUsesMappedModel(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", strings.NewReader(`{"model":"client","content":[{"type":"text","text":"text"}],"resolution":"720p"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(common.KeySeedanceOfficialAPI, true)
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "lec-gongteng-seedance-2-0-1080p"}, TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
	adaptor := NewPaipuTaskAdaptor()
	require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))
	validationErr := adaptor.ValidateBillingRequest(c, info)
	require.NotNil(t, validationErr)
	assert.Equal(t, "InvalidParameter.resolution", validationErr.Code)
}

func TestBuildPaipuTextRequestDefensivelyRevalidates(t *testing.T) {
	request := arkRequest{Model: "client", Content: []arkContent{{Type: "text", Text: "text"}}, Resolution: "720p"}
	body, err := buildTextVideoRequest(request, "lec-gongteng-seedance-2-0-1080p", *paipuProtocolProfile().textRequest)
	var requestErr *arkRequestError
	require.ErrorAs(t, err, &requestErr)
	assert.Equal(t, "InvalidParameter.resolution", requestErr.Code)
	assert.Nil(t, body)
}

func TestBuildPaipuTextRequestUsesRatioField(t *testing.T) {
	request := arkRequest{Model: "client", Content: []arkContent{{Type: "text", Text: "text"}}, Ratio: "4:3"}
	body, err := buildTextVideoRequest(request, "lec-seedance-2-0", *paipuProtocolProfile().textRequest)
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, common.Unmarshal(body, &got))
	assert.Equal(t, "4:3", got["ratio"])
	assert.NotContains(t, got, "aspect_ratio")
}
