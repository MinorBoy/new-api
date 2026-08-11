package newapivideo

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFFLinkProfileUsesPublicURLTaskContract(t *testing.T) {
	adaptor := NewFYLinkTaskAdaptor()
	profile := adaptor.activeProfile()

	assert.Equal(t, ChannelNameFYLink, adaptor.GetChannelName())
	assert.Empty(t, adaptor.GetModelList())
	assert.Equal(t, "/v1/videos/generations", profile.submitPath)
	assert.Equal(t, "/v1/videos/jobs/{task_id}", profile.pollPath)
	assert.Equal(t, videoRequestDialectFFLink, profile.requestDialect)
	assert.True(t, profile.requirePublicHTTPMedia)
	assert.True(t, profile.preferRespondAsync)
}

func TestBuildFFLinkRequestMapsArkMediaWithoutUploading(t *testing.T) {
	request, err := parseARKRequest([]byte(`{
		"model":"client-model",
		"content":[
			{"type":"text","text":"camera move"},
			{"type":"image_url","role":"first_frame","image_url":{"url":"https://8.8.8.8/start.png"}},
			{"type":"image_url","role":"last_frame","image_url":{"url":"https://8.8.8.8/end.png"}}
		],
		"duration":8,
		"ratio":"16:9",
		"resolution":"720p",
		"generate_audio":false
	}`), fflinkProtocolProfile())
	require.NoError(t, err)

	body, err := buildFFLinkRequest(request, "seedance-2.0")
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"model":"seedance-2.0",
		"prompt":"camera move",
		"resolution":"720p",
		"duration":8,
		"aspect_ratio":"16:9",
		"audio":false,
		"start_frame_url":"https://8.8.8.8/start.png",
		"end_frame_url":"https://8.8.8.8/end.png"
	}`, string(body))
}

func TestBuildFFLinkRequestMapsReferenceGuidances(t *testing.T) {
	request, err := parseARKRequest([]byte(`{
		"model":"client-model",
		"content":[
			{"type":"text","text":"mix references"},
			{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.8/ref.png"}},
			{"type":"video_url","role":"reference_video","video_url":{"url":"https://8.8.8.8/ref.mp4"}},
			{"type":"audio_url","role":"reference_audio","audio_url":{"url":"https://8.8.8.8/ref.mp3"}}
		],
		"duration":8,
		"ratio":"16:9",
		"resolution":"720p",
		"generate_audio":true
	}`), fflinkProtocolProfile())
	require.NoError(t, err)

	body, err := buildFFLinkRequest(request, "seedance-2.0")
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, common.Unmarshal(body, &got))
	assert.Equal(t, "UPLOADED", got["guidances"].(map[string]any)["image_reference"].([]any)[0].(map[string]any)["image"].(map[string]any)["type"])
	assert.Equal(t, "https://8.8.8.8/ref.mp4", got["guidances"].(map[string]any)["video_reference_base"].([]any)[0].(map[string]any)["video"].(map[string]any)["url"])
	assert.Equal(t, "https://8.8.8.8/ref.mp3", got["guidances"].(map[string]any)["audio_reference"].([]any)[0].(map[string]any)["audio"].(map[string]any)["url"])
}

func TestFFLinkRequestRejectsUnsafeURLsAndProviderFields(t *testing.T) {
	tests := []struct {
		name string
		body string
		code string
	}{
		{name: "data URL", body: `{"model":"m","content":[{"type":"text","text":"t"},{"type":"image_url","role":"reference_image","image_url":{"url":"data:image/png;base64,abc"}}]}`, code: "InvalidParameter.content"},
		{name: "private URL", body: `{"model":"m","content":[{"type":"text","text":"t"},{"type":"image_url","role":"reference_image","image_url":{"url":"http://192.168.1.2/ref.png"}}]}`, code: "InvalidParameter.content"},
		{name: "watermark", body: `{"model":"m","content":[{"type":"text","text":"t"}],"watermark":false}`, code: "InvalidParameter.watermark"},
		{name: "seed", body: `{"model":"m","content":[{"type":"text","text":"t"}],"seed":0}`, code: "InvalidParameter.seed"},
		{name: "callback", body: `{"model":"m","content":[{"type":"text","text":"t"}],"callback_url":"https://8.8.8.8/callback"}`, code: "InvalidParameter.callback_url"},
		{name: "draft", body: `{"model":"m","content":[{"type":"text","text":"t"}],"draft":false}`, code: "InvalidParameter.draft"},
		{name: "tools", body: `{"model":"m","content":[{"type":"text","text":"t"}],"tools":[{"type":"x"}]}`, code: "InvalidParameter.tools"},
		{name: "service tier", body: `{"model":"m","content":[{"type":"text","text":"t"}],"service_tier":"flex"}`, code: "InvalidParameter.service_tier"},
		{name: "short duration", body: `{"model":"m","content":[{"type":"text","text":"t"}],"duration":3}`, code: "InvalidParameter.duration"},
		{name: "1080p over twelve seconds", body: `{"model":"seedance-2.0","content":[{"type":"text","text":"t"}],"resolution":"1080p","duration":13}`, code: "InvalidParameter.duration"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request, err := parseARKRequest([]byte(tt.body), fflinkProtocolProfile())
			if err == nil {
				err = validateFFLinkRequest(request, "seedance-2.0")
			}
			require.Error(t, err)
			var requestErr *arkRequestError
			require.ErrorAs(t, err, &requestErr)
			assert.Equal(t, tt.code, requestErr.Code)
		})
	}
}

func TestFFLinkRequestRejectsMediaLimitsAndRoles(t *testing.T) {
	request := arkRequest{Model: "m", Content: []arkContent{{Type: "text", Text: "t"}}}
	for i := 0; i < 5; i++ {
		request.Content = append(request.Content, arkContent{Type: "image_url", Role: "reference_image", ImageURL: &arkMedia{URL: "https://8.8.8.8/ref.png"}})
	}
	err := validateFFLinkRequest(request, "seedance-2.0")
	require.Error(t, err)
	var requestErr *arkRequestError
	require.ErrorAs(t, err, &requestErr)
	assert.Equal(t, "InvalidParameter.content", requestErr.Code)

	request = arkRequest{Model: "m", Content: []arkContent{{Type: "text", Text: "t"}, {Type: "image_url", Role: "reference_video", ImageURL: &arkMedia{URL: "https://8.8.8.8/ref.png"}}}}
	err = validateFFLinkRequest(request, "seedance-2.0")
	require.Error(t, err)
	require.ErrorAs(t, err, &requestErr)
	assert.Equal(t, "InvalidParameter.content", requestErr.Code)
}
