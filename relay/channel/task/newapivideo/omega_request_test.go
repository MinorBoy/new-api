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

func TestOmegaAIProfile(t *testing.T) {
	adaptor := NewOmegaAITaskAdaptor()

	assert.Equal(t, ChannelNameOmegaAI, adaptor.GetChannelName())
	assert.Equal(t, []string{
		"klsdpro2-720p",
		"seedance-v2-720p",
		"dola-seedance-2.0",
		"lingjing-video-v1",
	}, adaptor.GetModelList())
	assert.Equal(t, "/v1/media/generate", adaptor.activeProfile().submitPath)
	assert.Equal(t, "/v1/tasks/{task_id}", adaptor.activeProfile().pollPath)
	assert.Equal(t, "application/json", adaptor.activeProfile().contentType)
	assert.Equal(t, videoRequestDialectOmegaMediaArrays, adaptor.activeProfile().requestDialect)
	assert.True(t, adaptor.activeProfile().requirePublicHTTPMedia)
	require.NotNil(t, adaptor.activeProfile().omegaRequest)
	assert.Equal(t, 9, adaptor.activeProfile().omegaRequest.MaxImages)
	assert.Equal(t, 3, adaptor.activeProfile().omegaRequest.MaxVideos)
	assert.Equal(t, 3, adaptor.activeProfile().omegaRequest.MaxAudios)
}

func TestBuildOmegaAIRequestPreservesMediaArraysAndPrompt(t *testing.T) {
	request, err := parseARKRequest([]byte(`{
		"model":"client-model",
		"content":[
			{"type":"text","text":"camera pushes in"},
			{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.8/ref-1.jpg"}},
			{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.4.4/ref-2.jpg"}},
			{"type":"video_url","role":"reference_video","video_url":{"url":"https://1.1.1.1/ref.mp4"}},
			{"type":"audio_url","role":"reference_audio","audio_url":{"url":"https://9.9.9.9/ref.mp3"}}
		],
		"duration":5,
		"ratio":"16:9"
	}`), omegaAIProtocolProfile())
	require.NoError(t, err)

	body, err := buildOmegaAIRequest(request, "klsdpro2-720p", *omegaAIProtocolProfile().omegaRequest)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"model":"klsdpro2-720p",
		"prompt":"camera pushes in",
		"duration":5,
		"aspect_ratio":"16:9",
		"images":["https://8.8.8.8/ref-1.jpg","https://8.8.4.4/ref-2.jpg"],
		"videos":["https://1.1.1.1/ref.mp4"],
		"audios":["https://9.9.9.9/ref.mp3"]
	}`, string(body))
}

func TestBuildOmegaAIRequestPreservesLingjingPromptReferences(t *testing.T) {
	request, err := parseARKRequest([]byte(`{
		"model":"client-model",
		"content":[
			{"type":"text","text":"让 @参考图1 中的人物走进 @参考图2 的场景"},
			{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.8/ref-1.jpg"}},
			{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.4.4/ref-2.jpg"}}
		]
	}`), omegaAIProtocolProfile())
	require.NoError(t, err)

	body, err := buildOmegaAIRequest(request, "lingjing-video-v1", *omegaAIProtocolProfile().omegaRequest)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"model":"lingjing-video-v1",
		"prompt":"让 @参考图1 中的人物走进 @参考图2 的场景",
		"images":["https://8.8.8.8/ref-1.jpg","https://8.8.4.4/ref-2.jpg"]
	}`, string(body))
}

func TestBuildOmegaAIRequestOmitsAbsentOptionalFields(t *testing.T) {
	request, err := parseARKRequest([]byte(`{
		"model":"client-model",
		"content":[{"type":"text","text":"text only"}]
	}`), omegaAIProtocolProfile())
	require.NoError(t, err)
	require.Nil(t, request.Ratio)
	require.Nil(t, request.Resolution)

	body, err := buildOmegaAIRequest(request, "seedance-v2-720p", *omegaAIProtocolProfile().omegaRequest)
	require.NoError(t, err)
	assert.JSONEq(t, `{"model":"seedance-v2-720p","prompt":"text only"}`, string(body))
	assert.NotContains(t, string(body), "aspect_ratio")
	assert.NotContains(t, string(body), "duration")
	assert.NotContains(t, string(body), "images")
}

func TestOmegaAIRequestValidation(t *testing.T) {
	images9 := strings.Repeat(`,{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.8/ref.jpg"}}`, 9)
	images10 := strings.Repeat(`,{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.8/ref.jpg"}}`, 10)
	tests := []struct {
		name          string
		body          string
		upstreamModel string
		code          string
		wantErr       bool
	}{
		{name: "nine images", body: `{"model":"m","content":[{"type":"text","text":"text"}` + images9 + `]}`, upstreamModel: "lingjing-video-v1"},
		{name: "ten images", body: `{"model":"m","content":[{"type":"text","text":"text"}` + images10 + `]}`, upstreamModel: "lingjing-video-v1", code: "InvalidParameter.content", wantErr: true},
		{name: "video on image only model", body: `{"model":"m","content":[{"type":"text","text":"text"},{"type":"video_url","role":"reference_video","video_url":{"url":"https://8.8.8.8/ref.mp4"}}]}`, upstreamModel: "seedance-v2-720p", code: "InvalidParameter.content", wantErr: true},
		{name: "audio on image only model", body: `{"model":"m","content":[{"type":"text","text":"text"},{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.8/ref.jpg"}},{"type":"audio_url","role":"reference_audio","audio_url":{"url":"https://8.8.8.8/ref.mp3"}}]}`, upstreamModel: "dola-seedance-2.0", code: "InvalidParameter.content", wantErr: true},
		{name: "video and audio on klsd", body: `{"model":"m","content":[{"type":"text","text":"text"},{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.8/ref.jpg"}},{"type":"video_url","role":"reference_video","video_url":{"url":"https://8.8.8.8/ref.mp4"}},{"type":"audio_url","role":"reference_audio","audio_url":{"url":"https://8.8.8.8/ref.mp3"}}]}`, upstreamModel: "klsdpro2-720p"},
		{name: "first frame", body: `{"model":"m","content":[{"type":"text","text":"text"},{"type":"image_url","role":"first_frame","image_url":{"url":"https://8.8.8.8/ref.jpg"}}]}`, upstreamModel: "klsdpro2-720p", code: "InvalidParameter.content", wantErr: true},
		{name: "last frame", body: `{"model":"m","content":[{"type":"text","text":"text"},{"type":"image_url","role":"last_frame","image_url":{"url":"https://8.8.8.8/ref.jpg"}}]}`, upstreamModel: "klsdpro2-720p", code: "InvalidParameter.content", wantErr: true},
		{name: "explicit empty ratio", body: `{"model":"m","content":[{"type":"text","text":"text"}],"ratio":""}`, upstreamModel: "klsdpro2-720p", code: "InvalidParameter.ratio", wantErr: true},
		{name: "resolution", body: `{"model":"m","content":[{"type":"text","text":"text"}],"resolution":"720p"}`, upstreamModel: "klsdpro2-720p", code: "InvalidParameter.resolution", wantErr: true},
		{name: "generate audio", body: `{"model":"m","content":[{"type":"text","text":"text"}],"generate_audio":false}`, upstreamModel: "klsdpro2-720p", code: "InvalidParameter.generate_audio", wantErr: true},
		{name: "watermark", body: `{"model":"m","content":[{"type":"text","text":"text"}],"watermark":false}`, upstreamModel: "klsdpro2-720p", code: "InvalidParameter.watermark", wantErr: true},
		{name: "non-default service tier", body: `{"model":"m","content":[{"type":"text","text":"text"}],"service_tier":"flex"}`, upstreamModel: "klsdpro2-720p", code: "InvalidParameter.service_tier", wantErr: true},
		{name: "draft", body: `{"model":"m","content":[{"type":"text","text":"text"}],"draft":false}`, upstreamModel: "klsdpro2-720p", code: "InvalidParameter.draft", wantErr: true},
		{name: "tools", body: `{"model":"m","content":[{"type":"text","text":"text"}],"tools":[{"type":"x"}]}`, upstreamModel: "klsdpro2-720p", code: "InvalidParameter.tools", wantErr: true},
		{name: "duration zero", body: `{"model":"m","content":[{"type":"text","text":"text"}],"duration":0}`, upstreamModel: "klsdpro2-720p", code: "InvalidParameter.duration", wantErr: true},
		{name: "duration overflow", body: `{"model":"m","content":[{"type":"text","text":"text"}],"duration":86401}`, upstreamModel: "klsdpro2-720p", code: "InvalidParameter.duration", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request, err := parseARKRequest([]byte(tt.body), omegaAIProtocolProfile())
			if err == nil {
				err = validateOmegaAIRequest(request, *omegaAIProtocolProfile().omegaRequest, tt.upstreamModel)
			}
			if !tt.wantErr {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			var requestErr *arkRequestError
			require.ErrorAs(t, err, &requestErr)
			assert.Equal(t, tt.code, requestErr.Code)
		})
	}
}

func TestOmegaAIAdaptorRequiresMappedProviderValidation(t *testing.T) {
	body := `{"model":"client","content":[{"type":"text","text":"text"}],"duration":5}`
	c, info := omegaAIValidationContext(body, "klsdpro2-720p")
	adaptor := NewOmegaAITaskAdaptor()

	require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))
	_, err := adaptor.BuildRequestBody(c, info)
	assert.ErrorContains(t, err, "validation")

	require.Nil(t, adaptor.ValidateBillingRequest(c, info))
	reader, err := adaptor.BuildRequestBody(c, info)
	require.NoError(t, err)
	built, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.JSONEq(t, `{"model":"klsdpro2-720p","prompt":"text","duration":5}`, string(built))
}

func TestOmegaAITransportUsesProfileAndEscapesTaskIDOnce(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/tasks/upstream%2Ftask", r.URL.EscapedPath())
		assert.Equal(t, "Bearer key", r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	adaptor := NewOmegaAITaskAdaptor()
	adaptor.Init(&relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: server.URL, ApiKey: "key"}})
	requestURL, err := adaptor.BuildRequestURL(nil)
	require.NoError(t, err)
	assert.Equal(t, server.URL+"/v1/media/generate", requestURL)

	response, err := adaptor.FetchTask(server.URL, "key", map[string]any{"task_id": "upstream/task"}, "")
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
}

func omegaAIValidationContext(body, upstreamModel string) (*gin.Context, *relaycommon.RelayInfo) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(common.KeySeedanceOfficialAPI, true)
	return c, &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl:    "https://omegaai.xin",
			ApiKey:            "key",
			UpstreamModelName: upstreamModel,
		},
		OriginModelName: "client",
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{},
	}
}
