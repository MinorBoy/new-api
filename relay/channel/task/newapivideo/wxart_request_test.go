package newapivideo

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/videometa"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type wxArtVideoMetadataClient struct {
	durations map[string]int64
}

func (c wxArtVideoMetadataClient) Metadata(_ context.Context, rawURL string) (videometa.Metadata, error) {
	return videometa.Metadata{DurationMS: c.durations[rawURL]}, nil
}

type wxArtAudioDurationResolver struct {
	durationMS int64
}

func (r wxArtAudioDurationResolver) ResolveMS(context.Context, []string) (int64, error) {
	return r.durationMS, nil
}

func TestWxArtProfileDeclaresTaskProtocol(t *testing.T) {
	profile := wxartProtocolProfile()
	assert.Equal(t, ChannelNameWxArt, profile.channelName)
	assert.Equal(t, []string{"seedance2.0", "seedance2.5"}, profile.modelList)
	assert.Equal(t, "/v1/videos", profile.submitPath)
	assert.Equal(t, "/v1/videos/{task_id}", profile.pollPath)
	assert.Equal(t, videoRequestDialectWxArt, profile.requestDialect)
	assert.Equal(t, 4, profile.defaultDurationSeconds)
}

func TestWxArtTransportUsesProviderPathsAndBearerAuth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer wx-key", r.Header.Get("Authorization"))
		assert.Equal(t, "/v1/videos/private%2Ftask", r.URL.EscapedPath())
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	adaptor := NewWxArtTaskAdaptor()
	adaptor.Init(&relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: server.URL, ApiKey: "wx-key"}})
	requestURL, err := adaptor.BuildRequestURL(nil)
	require.NoError(t, err)
	assert.Equal(t, server.URL+"/v1/videos", requestURL)
	response, err := adaptor.FetchTask(server.URL, "wx-key", map[string]any{"task_id": "private/task"}, "")
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
}

func TestWxArtModelWhitelistRejectsOtherSeedanceVariants(t *testing.T) {
	_, ok := wxArtModel("doubao-seedance-2-0-fast-260128")
	assert.False(t, ok)
	_, ok = wxArtModel("doubao-seedance-2-0-mini-260615")
	assert.False(t, ok)
}

func TestWxArtFailureContentVideoURLBecomesErrorReason(t *testing.T) {
	adaptor := NewWxArtTaskAdaptor()
	result, err := adaptor.ParseTaskResult([]byte(`{"id":"wx-private","status":"failed","content":{"video_url":"prompt rejected by WxArt"}}`))
	require.NoError(t, err)
	assert.Equal(t, string(model.TaskStatusFailure), result.Status)
	assert.Equal(t, "prompt rejected by WxArt", result.Reason)
	assert.Empty(t, result.Url)
}

func TestWxArtFailureArkResponseDoesNotExposeContentURL(t *testing.T) {
	adaptor := NewWxArtTaskAdaptor()
	task := &model.Task{
		TaskID:   "wx-public",
		Platform: "215",
		Status:   model.TaskStatusFailure,
		Properties: model.Properties{
			OriginModelName: "doubao-seedance-2-5-260628",
		},
		PrivateData: model.TaskPrivateData{UpstreamTaskID: "wx-private"},
		Data:        json.RawMessage(`{"status":"failed","content":{"video_url":"provider failure reason"}}`),
	}
	body, err := adaptor.ConvertToArkVideoTask(task)
	require.NoError(t, err)
	var response map[string]any
	require.NoError(t, common.Unmarshal(body, &response))
	assert.Nil(t, response["content"])
	errorValue, ok := response["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "provider failure reason", errorValue["message"])
}

func TestBuildWxArtRequestMapsArkContentAndOmitsAbsentOptions(t *testing.T) {
	ratio := "Auto"
	resolution := "720p"
	duration := 30
	request := arkRequest{
		Model:      "seedance2.5",
		Ratio:      &ratio,
		Resolution: &resolution,
		Duration:   &duration,
		Content: []arkContent{
			{Type: "text", Text: "a red kite"},
			{Type: "image_url", Role: "first_frame", ImageURL: &arkMedia{URL: "https://example.com/first.png"}},
			{Type: "image_url", Role: "last_frame", ImageURL: &arkMedia{URL: "https://example.com/last.png"}},
		},
	}

	body, err := buildWxArtRequest(request, "seedance2.5")
	require.NoError(t, err)
	assert.JSONEq(t, `{"model":"seedance2.5","prompt":"a red kite","ratio":"Auto","duration":30,"resolution":"720p","first_image":"https://example.com/first.png","last_image":"https://example.com/last.png"}`, string(body))
}

func TestValidateWxArtSeedance25AllowsThirtyMediaItemsPerType(t *testing.T) {
	content := []arkContent{{Type: "text", Text: "make a video"}}
	for index := 0; index < 30; index++ {
		content = append(content, arkContent{
			Type: "image_url", Role: "reference_image",
			ImageURL: &arkMedia{URL: "https://example.com/image.png"},
		})
	}
	for index := 0; index < 10; index++ {
		content = append(content,
			arkContent{Type: "video_url", Role: "reference_video", VideoURL: &arkMedia{URL: "https://example.com/video.mp4"}},
			arkContent{Type: "audio_url", Role: "reference_audio", AudioURL: &arkMedia{URL: "https://example.com/audio.wav"}},
		)
	}

	require.NoError(t, validateWxArtRequest(arkRequest{Model: "seedance2.5", Content: content}))
	content = append(content, arkContent{Type: "image_url", Role: "reference_image", ImageURL: &arkMedia{URL: "https://example.com/extra.png"}})
	assert.EqualError(t, validateWxArtRequest(arkRequest{Model: "seedance2.5", Content: content}), "InvalidParameter.content: reference media count exceeds seedance2.5 limits")
}

func TestValidateWxArtRejectsUnsupportedFieldsAndMixedFrameReferences(t *testing.T) {
	watermark := false
	request := arkRequest{
		Model: "seedance2.0",
		Content: []arkContent{
			{Type: "text", Text: "make a video"},
			{Type: "image_url", Role: "first_frame", ImageURL: &arkMedia{URL: "https://example.com/first.png"}},
			{Type: "image_url", Role: "reference_image", ImageURL: &arkMedia{URL: "https://example.com/reference.png"}},
		},
		Watermark: &watermark,
	}
	assert.EqualError(t, validateWxArtRequest(request), "InvalidParameter.watermark: watermark is not supported by WxArt")
	request.Watermark = nil
	assert.EqualError(t, validateWxArtRequest(request), "InvalidParameter.content: first/last frame content cannot mix with reference media")
}

func TestValidateWxArtFrameModeRequiresAutoRatio(t *testing.T) {
	ratio := "16:9"
	err := validateWxArtRequest(arkRequest{
		Model: "seedance2.0",
		Ratio: &ratio,
		Content: []arkContent{
			{Type: "text", Text: "make a video"},
			{Type: "image_url", Role: "first_frame", ImageURL: &arkMedia{URL: "https://example.com/first.png"}},
			{Type: "image_url", Role: "last_frame", ImageURL: &arkMedia{URL: "https://example.com/last.png"}},
		},
	})

	assert.EqualError(t, err, "InvalidParameter.ratio: first/last frame mode only supports ratio=Auto")
}

func TestValidateWxArtReferenceVideoDurationUsesMetadataBounds(t *testing.T) {
	service.SetVideoMetadataClient(wxArtVideoMetadataClient{durations: map[string]int64{
		"https://example.com/long.mp4": 16_000,
	}})
	t.Cleanup(func() { service.SetVideoMetadataClient(nil) })

	err := validateWxArtReferenceDurations(context.Background(), arkRequest{
		Model: "seedance2.0",
		Content: []arkContent{
			{Type: "text", Text: "make a video"},
			{Type: "video_url", Role: "reference_video", VideoURL: &arkMedia{URL: "https://example.com/long.mp4"}},
		},
	}, "seedance2.0")

	assert.EqualError(t, err, "InvalidParameter.content: reference video duration exceeds 15 seconds")
}

func TestValidateWxArtSeedance20RejectsShortReferenceVideo(t *testing.T) {
	service.SetVideoMetadataClient(wxArtVideoMetadataClient{durations: map[string]int64{
		"https://example.com/short.mp4": 1_000,
	}})
	t.Cleanup(func() { service.SetVideoMetadataClient(nil) })

	err := validateWxArtReferenceDurations(context.Background(), arkRequest{
		Model: "seedance2.0",
		Content: []arkContent{
			{Type: "text", Text: "make a video"},
			{Type: "video_url", Role: "reference_video", VideoURL: &arkMedia{URL: "https://example.com/short.mp4"}},
		},
	}, "seedance2.0")

	assert.EqualError(t, err, "InvalidParameter.content: reference video duration must be between 1.8 and 15 seconds")
}

func TestValidateWxArtSeedance25AudioDurationUsesMetadataBounds(t *testing.T) {
	service.SetReferenceAudioDurationResolver(wxArtAudioDurationResolver{durationMS: 1_000})
	t.Cleanup(func() { service.SetReferenceAudioDurationResolver(nil) })

	err := validateWxArtReferenceDurations(context.Background(), arkRequest{
		Model: "seedance2.5",
		Content: []arkContent{
			{Type: "text", Text: "make a video"},
			{Type: "audio_url", Role: "reference_audio", AudioURL: &arkMedia{URL: "https://example.com/short.wav"}},
		},
	}, "seedance2.5")

	assert.EqualError(t, err, "InvalidParameter.content: reference audio duration must be between 2 and 30 seconds")
}
