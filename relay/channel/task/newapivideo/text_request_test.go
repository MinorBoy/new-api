package newapivideo

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/videometa"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCangyuanProfileAndTransport(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/videos/upstream%2Ftask", r.URL.EscapedPath())
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	adaptor := NewCangyuanTaskAdaptor()
	assert.Equal(t, "Cangyuan", adaptor.GetChannelName())
	assert.Empty(t, adaptor.GetModelList())
	assert.Equal(t, "/v1/videos", adaptor.activeProfile().submitPath)
	assert.Equal(t, "/v1/videos/{task_id}", adaptor.activeProfile().pollPath)
	assert.Equal(t, "application/json", adaptor.activeProfile().contentType)
	assert.Equal(t, videoRequestDialectCangyuanMedia, adaptor.activeProfile().requestDialect)

	adaptor.Init(&relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
		ChannelBaseUrl: server.URL,
		ApiKey:         "key",
	}})
	requestURL, err := adaptor.BuildRequestURL(nil)
	require.NoError(t, err)
	assert.Equal(t, server.URL+"/v1/videos", requestURL)

	req, err := http.NewRequest(http.MethodPost, requestURL, nil)
	require.NoError(t, err)
	require.NoError(t, adaptor.BuildRequestHeader(nil, req, nil))
	assert.Equal(t, "application/json", req.Header.Get("Content-Type"))

	resp, err := adaptor.FetchTask(server.URL, "key", map[string]any{"task_id": "upstream/task"}, "")
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
}

func TestBuildCangyuanTextVideoRequest(t *testing.T) {
	c, info := cangyuanValidationContext(`{
		"model":"client-model",
		"content":[{"type":"text","text":"camera pushes in"}],
		"duration":8,
		"ratio":"16:9",
		"resolution":"720p"
	}`)
	adaptor := NewCangyuanTaskAdaptor()
	require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))

	reader, err := adaptor.BuildRequestBody(c, info)
	require.NoError(t, err)
	body, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"model":"seedance-2.0-720p",
		"prompt":"camera pushes in",
		"aspect_ratio":"16:9",
		"duration":8,
		"resolution":"720p"
	}`, string(body))
	assert.NotContains(t, string(body), `"ratio"`)
}

func TestCangyuanOmitsUnsetOptionalFields(t *testing.T) {
	c, info := cangyuanValidationContext(`{
		"model":"client-model",
		"content":[{"type":"text","text":"camera pushes in"}]
	}`)
	adaptor := NewCangyuanTaskAdaptor()
	require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))

	reader, err := adaptor.BuildRequestBody(c, info)
	require.NoError(t, err)
	body, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.JSONEq(t, `{"model":"seedance-2.0-720p","prompt":"camera pushes in"}`, string(body))
}

func TestCangyuanBuildsReferenceMediaRequest(t *testing.T) {
	c, info := cangyuanValidationContext(`{
		"model":"client-model",
		"content":[
			{"type":"text","text":"use the reference assets"},
			{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.8/ref-1.png"}},
			{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.8/ref-2.png"}},
			{"type":"video_url","role":"reference_video","video_url":{"url":"https://8.8.8.8/ref.mp4"}},
			{"type":"audio_url","role":"reference_audio","audio_url":{"url":"https://8.8.8.8/ref.wav"}}
		],
		"duration":15,
		"ratio":"9:16",
		"resolution":"480p",
		"generate_audio":false
	}`)
	adaptor := NewCangyuanTaskAdaptor()
	require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))

	reader, err := adaptor.BuildRequestBody(c, info)
	require.NoError(t, err)
	body, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"model":"seedance-2.0-720p",
		"prompt":"use the reference assets",
		"reference_image_urls":["https://8.8.8.8/ref-1.png","https://8.8.8.8/ref-2.png"],
		"reference_videos":["https://8.8.8.8/ref.mp4"],
		"reference_audios":["https://8.8.8.8/ref.wav"],
		"aspect_ratio":"9:16",
		"duration":15,
		"resolution":"480p",
		"audio":false
	}`, string(body))
}

func TestCangyuanSD5OmitsUnsupportedReferenceMode(t *testing.T) {
	c, info := cangyuanValidationContextForModel("sd5-seedance-2.0-fast", `{
		"model":"client-model",
		"content":[
			{"type":"text","text":"use the sd5 reference assets"},
			{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.8/ref.png"}},
			{"type":"video_url","role":"reference_video","video_url":{"url":"https://8.8.8.8/ref.mp4"}}
		],
		"duration":8,
		"ratio":"9:16",
		"resolution":"720p",
		"generate_audio":false,
		"seed":0
	}`)
	adaptor := NewCangyuanTaskAdaptor()
	require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))

	reader, err := adaptor.BuildRequestBody(c, info)
	require.NoError(t, err)
	body, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"model":"sd5-seedance-2.0-fast",
		"prompt":"use the sd5 reference assets",
		"reference_image_urls":["https://8.8.8.8/ref.png"],
		"reference_videos":["https://8.8.8.8/ref.mp4"],
		"aspect_ratio":"9:16",
		"duration":8,
		"resolution":"720p",
		"generate_audio":false,
		"seed":0
	}`, string(body))
}

func TestCangyuanRequiresHTTPSForRemoteReferenceMedia(t *testing.T) {
	for _, mediaType := range []string{"image", "video", "audio"} {
		assert.False(t, validCangyuanMediaURL("http://8.8.8.8/reference", mediaType), mediaType)
	}
}

func TestCangyuanValidatesReferenceMediaDurations(t *testing.T) {
	request := arkRequest{
		Model: "client-model",
		Content: []arkContent{
			{Type: "text", Text: "use the references"},
			{Type: "image_url", Role: "reference_image", ImageURL: &arkMedia{URL: "https://8.8.8.8/ref.png"}},
			{Type: "video_url", Role: "reference_video", VideoURL: &arkMedia{URL: "https://8.8.8.8/ref.mp4"}},
			{Type: "audio_url", Role: "reference_audio", AudioURL: &arkMedia{URL: "https://8.8.8.8/ref.wav"}},
		},
	}
	profile := cangyuanRequestProfileForModel("seedance-2.0-720p")

	service.SetVideoMetadataClient(fixedCangyuanVideoMetadataClient{durationMS: 15_001})
	service.SetReferenceAudioDurationResolver(fixedCangyuanAudioDurationResolver{durationMS: 1_000})
	t.Cleanup(func() {
		service.SetVideoMetadataClient(nil)
		service.SetReferenceAudioDurationResolver(nil)
	})
	var requestErr *arkRequestError
	require.ErrorAs(t, validateCangyuanReferenceDurations(context.Background(), request, profile), &requestErr)
	assert.Equal(t, "InvalidParameter.content", requestErr.Code)

	service.SetVideoMetadataClient(fixedCangyuanVideoMetadataClient{durationMS: 5_000})
	service.SetReferenceAudioDurationResolver(fixedCangyuanAudioDurationResolver{durationMS: 15_001})
	requestErr = nil
	require.ErrorAs(t, validateCangyuanReferenceDurations(context.Background(), request, profile), &requestErr)
	assert.Equal(t, "InvalidParameter.content", requestErr.Code)
}

func TestCangyuanBuildsFirstLastFrameRequest(t *testing.T) {
	c, info := cangyuanValidationContext(`{
		"model":"client-model",
		"content":[
			{"type":"text","text":"transition between frames"},
			{"type":"image_url","role":"first_frame","image_url":{"url":"https://8.8.8.8/first.png"}},
			{"type":"image_url","role":"last_frame","image_url":{"url":"https://8.8.8.8/last.png"}}
		],
		"duration":4,
		"resolution":"720p"
	}`)
	adaptor := NewCangyuanTaskAdaptor()
	require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))

	reader, err := adaptor.BuildRequestBody(c, info)
	require.NoError(t, err)
	body, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"model":"seedance-2.0-720p",
		"prompt":"transition between frames",
		"first_image_url":"https://8.8.8.8/first.png",
		"last_image_url":"https://8.8.8.8/last.png",
		"duration":4,
		"resolution":"720p"
	}`, string(body))
}

func TestCangyuanValidationRejectsUnsupportedRequestsBeforeBuild(t *testing.T) {
	tests := []struct {
		name string
		body string
		code string
	}{
		{
			name: "five images",
			body: `{"model":"m","content":[{"type":"text","text":"text"},{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.8/1.png"}},{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.8/2.png"}},{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.8/3.png"}},{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.8/4.png"}},{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.8/5.png"}}]}`,
			code: "InvalidParameter.content",
		},
		{
			name: "four videos",
			body: `{"model":"m","content":[{"type":"text","text":"text"},{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.8/1.png"}},{"type":"video_url","role":"reference_video","video_url":{"url":"https://8.8.8.8/1.mp4"}},{"type":"video_url","role":"reference_video","video_url":{"url":"https://8.8.8.8/2.mp4"}},{"type":"video_url","role":"reference_video","video_url":{"url":"https://8.8.8.8/3.mp4"}},{"type":"video_url","role":"reference_video","video_url":{"url":"https://8.8.8.8/4.mp4"}}]}`,
			code: "InvalidParameter.content",
		},
		{
			name: "two audios",
			body: `{"model":"m","content":[{"type":"text","text":"text"},{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.8/1.png"}},{"type":"audio_url","role":"reference_audio","audio_url":{"url":"https://8.8.8.8/1.wav"}},{"type":"audio_url","role":"reference_audio","audio_url":{"url":"https://8.8.8.8/2.wav"}}]}`,
			code: "InvalidParameter.content",
		},
		{
			name: "one first frame",
			body: `{"model":"m","content":[{"type":"text","text":"text"},{"type":"image_url","role":"first_frame","image_url":{"url":"https://8.8.8.8/first.png"}}]}`,
			code: "InvalidParameter.content",
		},
		{
			name: "mixed frame and reference media",
			body: `{"model":"m","content":[{"type":"text","text":"text"},{"type":"image_url","role":"first_frame","image_url":{"url":"https://8.8.8.8/first.png"}},{"type":"image_url","role":"last_frame","image_url":{"url":"https://8.8.8.8/last.png"}},{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.8/ref.png"}}]}`,
			code: "InvalidParameter.content",
		},
		{
			name: "duration below minimum",
			body: `{"model":"m","content":[{"type":"text","text":"text"}],"duration":3}`,
			code: "InvalidParameter.duration",
		},
		{
			name: "resolution above maximum",
			body: `{"model":"m","content":[{"type":"text","text":"text"}],"resolution":"1080p"}`,
			code: "InvalidParameter.resolution",
		},
		{
			name: "draft task",
			body: `{"model":"m","content":[{"type":"text","text":"text"},{"type":"draft_task","draft_task":{"id":"task"}}]}`,
			code: "InvalidParameter.content",
		},
		{
			name: "watermark",
			body: `{"model":"m","content":[{"type":"text","text":"text"}],"watermark":true}`,
			code: "InvalidParameter.watermark",
		},
		{
			name: "draft true",
			body: `{"model":"m","content":[{"type":"text","text":"text"}],"draft":true}`,
			code: "InvalidParameter.draft",
		},
		{
			name: "draft false",
			body: `{"model":"m","content":[{"type":"text","text":"text"}],"draft":false}`,
			code: "InvalidParameter.draft",
		},
		{
			name: "non-empty tools",
			body: `{"model":"m","content":[{"type":"text","text":"text"}],"tools":[{"type":"camera_fixed"}]}`,
			code: "InvalidParameter.tools",
		},
		{
			name: "default service tier",
			body: `{"model":"m","content":[{"type":"text","text":"text"}],"service_tier":"default"}`,
			code: "InvalidParameter.service_tier",
		},
		{
			name: "non-default service tier",
			body: `{"model":"m","content":[{"type":"text","text":"text"}],"service_tier":"flex"}`,
			code: "InvalidParameter.service_tier",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, info := cangyuanValidationContext(tt.body)
			adaptor := NewCangyuanTaskAdaptor()

			taskErr := adaptor.ValidateRequestAndSetAction(c, info)
			require.NotNil(t, taskErr)
			assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
			assert.Equal(t, tt.code, taskErr.Code)
			_, buildErr := adaptor.BuildRequestBody(c, info)
			assert.Error(t, buildErr, "invalid input must not produce an upstream body")
		})
	}
}

func TestCangyuanUsesDocumentedDurationAndResolutionBoundaries(t *testing.T) {
	tests := []struct {
		name      string
		duration  int
		wantError bool
	}{
		{name: "minimum", duration: 4},
		{name: "maximum", duration: 15},
		{name: "explicit zero", duration: 0, wantError: true},
		{name: "below provider minimum", duration: 3, wantError: true},
		{name: "above provider maximum", duration: 16, wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := arkRequest{
				Model:    "client-model",
				Content:  []arkContent{{Type: "text", Text: "text"}},
				Duration: common.GetPointer(tt.duration),
			}
			err := validateCangyuanRequest(request, *cangyuanProtocolProfile().cangyuanRequest)
			if tt.wantError {
				var requestErr *arkRequestError
				require.ErrorAs(t, err, &requestErr)
				assert.Equal(t, "InvalidParameter.duration", requestErr.Code)
				return
			}
			assert.NoError(t, err)
		})
	}
	for _, resolution := range []string{"480p", "720p"} {
		request := arkRequest{Model: "client-model", Content: []arkContent{{Type: "text", Text: "text"}}, Resolution: common.GetPointer(resolution)}
		assert.NoError(t, validateCangyuanRequest(request, *cangyuanProtocolProfile().cangyuanRequest))
	}
	request := arkRequest{Model: "client-model", Content: []arkContent{{Type: "text", Text: "text"}}, Resolution: common.GetPointer("1080p")}
	var requestErr *arkRequestError
	require.ErrorAs(t, validateCangyuanRequest(request, *cangyuanProtocolProfile().cangyuanRequest), &requestErr)
	assert.Equal(t, "InvalidParameter.resolution", requestErr.Code)
}

func TestCangyuanRejectsUnsupportedRatio(t *testing.T) {
	request := arkRequest{
		Model:   "client-model",
		Content: []arkContent{{Type: "text", Text: "text"}},
		Ratio:   common.GetPointer("2:1"),
	}
	var requestErr *arkRequestError
	require.ErrorAs(t, validateCangyuanRequest(request, *cangyuanProtocolProfile().cangyuanRequest), &requestErr)
	assert.Equal(t, "InvalidParameter.ratio", requestErr.Code)
}

func TestBuildCangyuanTextVideoRequestDefensivelyRevalidates(t *testing.T) {
	request := arkRequest{
		Model:    "client-model",
		Content:  []arkContent{{Type: "text", Text: "text"}},
		Duration: common.GetPointer(0),
	}
	body, err := buildCangyuanRequest(request, "seedance-2.0-720p", *cangyuanProtocolProfile().cangyuanRequest)
	var requestErr *arkRequestError
	require.ErrorAs(t, err, &requestErr)
	assert.Equal(t, "InvalidParameter.duration", requestErr.Code)
	assert.Nil(t, body)
}

func cangyuanValidationContext(body string) (*gin.Context, *relaycommon.RelayInfo) {
	return cangyuanValidationContextForModel("seedance-2.0-720p", body)
}

func cangyuanValidationContextForModel(model, body string) (*gin.Context, *relaycommon.RelayInfo) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(common.KeySeedanceOfficialAPI, true)
	return c, &relaycommon.RelayInfo{
		ChannelMeta:   &relaycommon.ChannelMeta{UpstreamModelName: model},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
	}
}

type fixedCangyuanVideoMetadataClient struct {
	durationMS int64
}

func (client fixedCangyuanVideoMetadataClient) Metadata(context.Context, string) (videometa.Metadata, error) {
	return videometa.Metadata{
		DurationMS: client.durationMS, Width: 1280, Height: 720,
		FrameRateNum: 24, FrameRateDen: 1, Container: "mp4", ContentLength: 1024,
	}, nil
}

type fixedCangyuanAudioDurationResolver struct {
	durationMS int64
}

func (resolver fixedCangyuanAudioDurationResolver) ResolveMS(context.Context, []string) (int64, error) {
	return resolver.durationMS, nil
}
