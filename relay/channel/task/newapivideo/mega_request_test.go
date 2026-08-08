package newapivideo

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/videometa"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type megaByAIVideoMetadataClient struct {
	durations map[string]int64
	err       error
	calls     *atomic.Int32
}

func (f megaByAIVideoMetadataClient) Metadata(_ context.Context, url string) (videometa.Metadata, error) {
	if f.calls != nil {
		f.calls.Add(1)
	}
	if f.err != nil {
		return videometa.Metadata{}, f.err
	}
	return videometa.Metadata{
		DurationMS: f.durations[url], Width: 1280, Height: 720,
		FrameRateNum: 24, FrameRateDen: 1, Container: "mp4", ContentLength: 1,
	}, nil
}

type megaByAIAudioDurationResolver struct {
	duration int64
	err      error
	calls    *atomic.Int32
}

func (f megaByAIAudioDurationResolver) ResolveMS(_ context.Context, _ []string) (int64, error) {
	if f.calls != nil {
		f.calls.Add(1)
	}
	return f.duration, f.err
}

func TestMegaByAIProfile(t *testing.T) {
	adaptor := NewMegaByAITaskAdaptor()
	assert.Equal(t, "MegaByAI", adaptor.GetChannelName())
	assert.Equal(t, []string{"videos-standard", "videos-fast", "videos-mini"}, adaptor.GetModelList())
	assert.Equal(t, "/v1/videos", adaptor.activeProfile().submitPath)
	assert.Equal(t, "/v1/videos/{task_id}", adaptor.activeProfile().pollPath)
	assert.Equal(t, "application/json", adaptor.activeProfile().contentType)
	assert.Equal(t, videoRequestDialectMegaReferenceArrays, adaptor.activeProfile().requestDialect)
	assert.False(t, adaptor.activeProfile().allowEmbeddedMedia)
	assert.Equal(t, 5, adaptor.activeProfile().defaultDurationSeconds)
}

func TestMegaByAIDefaultDurationIsFiveSeconds(t *testing.T) {
	c, info := megaByAIValidationContext(`{"model":"client","content":[{"type":"text","text":"text"}]}`)
	adaptor := NewMegaByAITaskAdaptor()
	require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))
	duration, taskErr := adaptor.EstimateDurationSeconds(c, info)
	require.Nil(t, taskErr)
	assert.Equal(t, 5, duration)

	reader, err := adaptor.BuildRequestBody(c, info)
	require.NoError(t, err)
	body, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.JSONEq(t, `{"model":"videos-mini","prompt":"text"}`, string(body))
}

func TestMegaByAITransportUsesProfile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/videos/upstream%2Ftask", r.URL.EscapedPath())
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	adaptor := NewMegaByAITaskAdaptor()
	adaptor.Init(&relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: server.URL, ApiKey: "key"}})
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

	adaptor.profile.pollPath = "/v1/videos/tasks"
	_, err = adaptor.FetchTask(server.URL, "key", map[string]any{"task_id": "task"}, "")
	assert.ErrorContains(t, err, "{task_id}")

	adaptor.profile.pollPath = "/v1/videos/{task_id}/copies/{task_id}"
	_, err = adaptor.FetchTask(server.URL, "key", map[string]any{"task_id": "task"}, "")
	assert.ErrorContains(t, err, "exactly once")
}

func TestBuildMegaByAIRequest(t *testing.T) {
	request, err := parseARKRequest([]byte(`{
		"model":"doubao-seedance-2-0-260128",
		"content":[
			{"type":"text","text":"camera pushes in"},
			{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.8/ref.jpg"}},
			{"type":"video_url","role":"reference_video","video_url":{"url":"https://8.8.8.8/ref.mp4"}},
			{"type":"audio_url","role":"reference_audio","audio_url":{"url":"https://8.8.8.8/ref.mp3"}}
		],
		"duration":8,"ratio":"16:9","resolution":"720p"
	}`), megaByAIProtocolProfile())
	require.NoError(t, err)

	body, err := buildMegaByAIRequest(request, "videos-mini")
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"model":"videos-mini","prompt":"camera pushes in","duration":8,
		"ratio":"16:9","resolution":"720p",
		"referenceImages":["https://8.8.8.8/ref.jpg"],
		"referenceVideos":["https://8.8.8.8/ref.mp4"],
		"referenceAudios":["https://8.8.8.8/ref.mp3"]
	}`, string(body))
}

func TestBuildMegaByAIRequestSupportsHighResolutions(t *testing.T) {
	for _, resolution := range []string{"1080p", "4k"} {
		t.Run(resolution, func(t *testing.T) {
			request, err := parseARKRequest([]byte(`{
				"model":"doubao-seedance-2-0-260128",
				"content":[{"type":"text","text":"high resolution video"}],
				"resolution":"`+resolution+`"
			}`), megaByAIProtocolProfile())
			require.NoError(t, err)

			body, err := buildMegaByAIRequest(request, "videos-standard")
			require.NoError(t, err)
			assert.JSONEq(t, `{
				"model":"videos-standard",
				"prompt":"high resolution video",
				"resolution":"`+resolution+`"
			}`, string(body))
		})
	}
}

func TestBuildMegaByAIRequestTreatsSingleFrameImageAsReference(t *testing.T) {
	for _, role := range []string{"", "first_frame"} {
		t.Run(fmt.Sprintf("role_%s", role), func(t *testing.T) {
			request, err := parseARKRequest([]byte(fmt.Sprintf(`{
				"model":"client","content":[
					{"type":"text","text":"text"},
					{"type":"image_url","role":%q,"image_url":{"url":"https://8.8.8.8/ref.jpg"}}
				],"duration":5}`, role)), megaByAIProtocolProfile())
			require.NoError(t, err)
			body, err := buildMegaByAIRequest(request, "videos-standard")
			require.NoError(t, err)
			assert.JSONEq(t, `{"model":"videos-standard","prompt":"text","duration":5,"referenceImages":["https://8.8.8.8/ref.jpg"]}`, string(body))
		})
	}
}

func TestBuildMegaByAIRequestAllowsSingleFrameImageWithReferenceMedia(t *testing.T) {
	for _, role := range []string{"", "first_frame"} {
		t.Run(fmt.Sprintf("role_%s", role), func(t *testing.T) {
			request, err := parseARKRequest([]byte(fmt.Sprintf(`{
				"model":"client","content":[
					{"type":"text","text":"text"},
					{"type":"image_url","role":%q,"image_url":{"url":"https://8.8.8.8/ref.jpg"}},
					{"type":"video_url","role":"reference_video","video_url":{"url":"https://8.8.8.8/ref.mp4"}},
					{"type":"audio_url","role":"reference_audio","audio_url":{"url":"https://8.8.8.8/ref.mp3"}}
				]}`, role)), megaByAIProtocolProfile())
			require.NoError(t, err)

			body, err := buildMegaByAIRequest(request, "videos-standard")
			require.NoError(t, err)
			assert.JSONEq(t, `{
				"model":"videos-standard","prompt":"text",
				"referenceImages":["https://8.8.8.8/ref.jpg"],
				"referenceVideos":["https://8.8.8.8/ref.mp4"],
				"referenceAudios":["https://8.8.8.8/ref.mp3"]
			}`, string(body))
		})
	}
}

func TestMegaByAIValidationRejectsInvalidRequestsBeforeBuild(t *testing.T) {
	images := strings.Repeat(`,{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.8/ref.jpg"}}`, 10)
	videos := strings.Repeat(`,{"type":"video_url","role":"reference_video","video_url":{"url":"https://8.8.8.8/ref.mp4"}}`, 4)
	audios := strings.Repeat(`,{"type":"audio_url","role":"reference_audio","audio_url":{"url":"https://8.8.8.8/ref.mp3"}}`, 4)
	tests := []struct {
		name string
		body string
		code string
	}{
		{name: "duration below minimum", body: `{"model":"m","content":[{"type":"text","text":"text"}],"duration":3}`, code: "InvalidParameter.duration"},
		{name: "duration above maximum", body: `{"model":"m","content":[{"type":"text","text":"text"}],"duration":16}`, code: "InvalidParameter.duration"},
		{name: "unsupported ratio", body: `{"model":"m","content":[{"type":"text","text":"text"}],"ratio":"4:3"}`, code: "InvalidParameter.ratio"},
		{name: "unsupported resolution", body: `{"model":"m","content":[{"type":"text","text":"text"}],"resolution":"1440p"}`, code: "InvalidParameter.resolution"},
		{name: "last frame", body: `{"model":"m","content":[{"type":"text","text":"text"},{"type":"image_url","role":"last_frame","image_url":{"url":"https://8.8.8.8/last.jpg"}}]}`, code: "InvalidParameter.content"},
		{name: "non HTTP media", body: `{"model":"m","content":[{"type":"text","text":"text"},{"type":"image_url","role":"reference_image","image_url":{"url":"asset://ref"}}]}`, code: "InvalidParameter.content"},
		{name: "loopback image", body: `{"model":"m","content":[{"type":"text","text":"text"},{"type":"image_url","role":"reference_image","image_url":{"url":"http://127.0.0.1/ref.jpg"}}]}`, code: "InvalidParameter.content"},
		{name: "link-local image", body: `{"model":"m","content":[{"type":"text","text":"text"},{"type":"image_url","role":"reference_image","image_url":{"url":"http://169.254.169.254/latest/meta-data"}}]}`, code: "InvalidParameter.content"},
		{name: "localhost image", body: `{"model":"m","content":[{"type":"text","text":"text"},{"type":"image_url","role":"reference_image","image_url":{"url":"http://localhost/ref.jpg"}}]}`, code: "InvalidParameter.content"},
		{name: "audio only", body: `{"model":"m","content":[{"type":"text","text":"text"},{"type":"audio_url","role":"reference_audio","audio_url":{"url":"https://8.8.8.8/ref.mp3"}}]}`, code: "InvalidParameter.content"},
		{name: "too many images", body: `{"model":"m","content":[{"type":"text","text":"text"}` + images + `]}`, code: "InvalidParameter.content"},
		{name: "too many videos", body: `{"model":"m","content":[{"type":"text","text":"text"}` + videos + `]}`, code: "InvalidParameter.content"},
		{name: "too many audios", body: `{"model":"m","content":[{"type":"text","text":"text"},{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.8/ref.jpg"}}` + audios + `]}`, code: "InvalidParameter.content"},
		{name: "audio conflict", body: `{"model":"m","content":[{"type":"text","text":"text"},{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.8/ref.jpg"}},{"type":"audio_url","role":"reference_audio","audio_url":{"url":"https://8.8.8.8/ref.mp3"}}],"generate_audio":false}`, code: "InvalidParameter.generate_audio"},
		{name: "unsupported control", body: `{"model":"m","content":[{"type":"text","text":"text"}],"callback_url":"https://8.8.8.8/callback"}`, code: "InvalidParameter.callback_url"},
		{name: "misspelled control", body: `{"model":"m","content":[{"type":"text","text":"text"}],"duraton":5}`, code: "InvalidParameter.duraton"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", strings.NewReader(tt.body))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Set(common.KeySeedanceOfficialAPI, true)
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "videos-mini"}, TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
			adaptor := NewMegaByAITaskAdaptor()

			taskErr := adaptor.ValidateRequestAndSetAction(c, info)
			require.NotNil(t, taskErr)
			assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
			assert.Equal(t, tt.code, taskErr.Code)
			_, buildErr := adaptor.BuildRequestBody(c, info)
			assert.Error(t, buildErr, "invalid input must not produce an upstream body")
		})
	}
}

func TestMegaByAIRejectsHostnameResolvingToLoopback(t *testing.T) {
	_, err := parseARKRequest([]byte(`{
		"model":"m",
		"content":[
			{"type":"text","text":"text"},
			{"type":"image_url","role":"reference_image","image_url":{"url":"https://127.0.0.1.nip.io/ref.jpg"}}
		]
	}`), megaByAIProtocolProfile())
	require.Error(t, err)
	var requestErr *arkRequestError
	require.ErrorAs(t, err, &requestErr)
	assert.Equal(t, "InvalidParameter.content", requestErr.Code)
}

func TestMegaByAIExplicitGenerateAudioWithoutReferenceAudioIsAcceptedAndNotMapped(t *testing.T) {
	for _, value := range []string{"true", "false"} {
		t.Run(value, func(t *testing.T) {
			request, err := parseARKRequest([]byte(`{"model":"m","content":[{"type":"text","text":"text"}],"generate_audio":`+value+`}`), megaByAIProtocolProfile())
			require.NoError(t, err)
			body, err := buildMegaByAIRequest(request, "videos-mini")
			require.NoError(t, err)
			assert.NotContains(t, string(body), "generateAudio")
		})
	}
}

func TestBuildMegaByAIRequestDefensivelyRevalidates(t *testing.T) {
	request := arkRequest{
		Model: "client", Duration: common.GetPointer(3),
		Content: []arkContent{{Type: "text", Text: "text"}},
	}
	_, err := buildMegaByAIRequest(request, "videos-mini")
	var requestErr *arkRequestError
	require.ErrorAs(t, err, &requestErr)
	assert.Equal(t, "InvalidParameter.duration", requestErr.Code)
}

func TestMegaByAIValidatesReferenceDurationsBeforeBuilding(t *testing.T) {
	t.Cleanup(func() { service.SetVideoMetadataClient(nil) })
	service.SetVideoMetadataClient(megaByAIVideoMetadataClient{durations: map[string]int64{
		"https://8.8.8.8/a.mp4": 9000,
		"https://8.8.8.8/b.mp4": 6000,
	}})
	service.SetReferenceAudioDurationResolver(megaByAIAudioDurationResolver{duration: 15000})
	t.Cleanup(func() { service.SetReferenceAudioDurationResolver(nil) })

	body := `{"model":"client","content":[
		{"type":"text","text":"text"},
		{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.8/ref.jpg"}},
		{"type":"video_url","role":"reference_video","video_url":{"url":"https://8.8.8.8/a.mp4"}},
		{"type":"video_url","role":"reference_video","video_url":{"url":"https://8.8.8.8/b.mp4"}},
		{"type":"audio_url","role":"reference_audio","audio_url":{"url":"https://8.8.8.8/a.wav"}}
	],"duration":8,"ratio":"1:1","resolution":"480p"}`
	c, info := megaByAIValidationContext(body)
	adaptor := NewMegaByAITaskAdaptor()
	require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))

	reader, err := adaptor.BuildRequestBody(c, info)
	require.NoError(t, err)
	upstreamBody, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.JSONEq(t, `{"model":"videos-mini","prompt":"text","duration":8,"ratio":"1:1","resolution":"480p","referenceImages":["https://8.8.8.8/ref.jpg"],"referenceVideos":["https://8.8.8.8/a.mp4","https://8.8.8.8/b.mp4"],"referenceAudios":["https://8.8.8.8/a.wav"]}`, string(upstreamBody))
}

func TestMegaByAIMediaValidationIsMemoizedAcrossRetries(t *testing.T) {
	var videoCalls atomic.Int32
	var audioCalls atomic.Int32
	service.SetVideoMetadataClient(megaByAIVideoMetadataClient{
		durations: map[string]int64{"https://8.8.8.8/a.mp4": 1000}, calls: &videoCalls,
	})
	service.SetReferenceAudioDurationResolver(megaByAIAudioDurationResolver{duration: 1000, calls: &audioCalls})
	t.Cleanup(func() {
		service.SetVideoMetadataClient(nil)
		service.SetReferenceAudioDurationResolver(nil)
	})

	body := `{"model":"client","content":[
		{"type":"text","text":"text"},
		{"type":"video_url","role":"reference_video","video_url":{"url":"https://8.8.8.8/a.mp4"}},
		{"type":"audio_url","role":"reference_audio","audio_url":{"url":"https://8.8.8.8/a.wav"}}
	]}`
	c, info := megaByAIValidationContext(body)
	adaptor := NewMegaByAITaskAdaptor()

	require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))
	require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))
	assert.Equal(t, int32(1), videoCalls.Load())
	assert.Equal(t, int32(1), audioCalls.Load())
}

func TestMegaByAIRejectsReferenceDurationOverLimit(t *testing.T) {
	tests := []struct {
		name       string
		videoError error
		videoMS    int64
		audioError error
		audioMS    int64
		wantStatus int
		wantCode   string
	}{
		{name: "video too long", videoMS: 15001, audioMS: 1000, wantStatus: http.StatusBadRequest, wantCode: "InvalidParameter.content"},
		{name: "audio too long", videoMS: 1000, audioMS: 15001, wantStatus: http.StatusBadRequest, wantCode: "InvalidParameter.content"},
		{name: "video invalid", videoError: &service.VideoMetadataError{Kind: service.VideoMetadataInvalidMedia}, audioMS: 1000, wantStatus: http.StatusBadRequest, wantCode: "InvalidParameter.content"},
		{name: "video unavailable", videoError: &service.VideoMetadataError{Kind: service.VideoMetadataUnavailable}, audioMS: 1000, wantStatus: http.StatusServiceUnavailable, wantCode: "reference_media_metadata_unavailable"},
		{name: "audio invalid", videoMS: 1000, audioError: &service.ReferenceAudioDurationError{Kind: service.ReferenceAudioInvalidMedia}, wantStatus: http.StatusBadRequest, wantCode: "InvalidParameter.content"},
		{name: "audio unavailable", videoMS: 1000, audioError: &service.ReferenceAudioDurationError{Kind: service.ReferenceAudioMetadataUnavailable}, wantStatus: http.StatusServiceUnavailable, wantCode: "reference_media_metadata_unavailable"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service.SetVideoMetadataClient(megaByAIVideoMetadataClient{durations: map[string]int64{"https://8.8.8.8/a.mp4": tt.videoMS}, err: tt.videoError})
			service.SetReferenceAudioDurationResolver(megaByAIAudioDurationResolver{duration: tt.audioMS, err: tt.audioError})
			t.Cleanup(func() {
				service.SetVideoMetadataClient(nil)
				service.SetReferenceAudioDurationResolver(nil)
			})

			body := `{"model":"client","content":[
				{"type":"text","text":"text"},
				{"type":"video_url","role":"reference_video","video_url":{"url":"https://8.8.8.8/a.mp4"}},
				{"type":"audio_url","role":"reference_audio","audio_url":{"url":"https://8.8.8.8/a.wav"}}
			],"duration":8}`
			c, info := megaByAIValidationContext(body)
			adaptor := NewMegaByAITaskAdaptor()
			taskErr := adaptor.ValidateRequestAndSetAction(c, info)
			require.NotNil(t, taskErr)
			assert.Equal(t, tt.wantStatus, taskErr.StatusCode)
			assert.Equal(t, tt.wantCode, taskErr.Code)
			_, err := adaptor.BuildRequestBody(c, info)
			assert.Error(t, err)
		})
	}
}

func TestMegaByAIBuildRequiresCompletedProviderValidation(t *testing.T) {
	request, err := parseARKRequest([]byte(`{"model":"client","content":[{"type":"text","text":"text"}],"duration":5}`), megaByAIProtocolProfile())
	require.NoError(t, err)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set(common.KeySeedanceOfficialAPI, true)
	c.Set(requestStateContextKey, requestState{ARK: &request})
	_, err = NewMegaByAITaskAdaptor().BuildRequestBody(c, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "videos-mini"}})
	assert.ErrorContains(t, err, "validation")
}

func megaByAIValidationContext(body string) (*gin.Context, *relaycommon.RelayInfo) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(common.KeySeedanceOfficialAPI, true)
	return c, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "videos-mini"}, TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
}
