package newapivideo

import (
	"fmt"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestZ5APIProfileUsesDocumentedTaskContract(t *testing.T) {
	adaptor := NewZ5APITaskAdaptor()
	profile := adaptor.activeProfile()

	assert.Equal(t, ChannelNameZ5API, adaptor.GetChannelName())
	assert.Empty(t, adaptor.GetModelList())
	assert.Equal(t, "/v1/videos", profile.submitPath)
	assert.Equal(t, "/v1/videos/{task_id}", profile.pollPath)
	assert.Equal(t, "application/json", profile.contentType)
	assert.Equal(t, videoRequestDialectZ5APIMedia, profile.requestDialect)
	assert.True(t, profile.requirePublicHTTPMedia)
}

func TestBuildZ5APIRequestEncodesFrameAndReferenceModes(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "text only",
			body: `{"model":"client","content":[{"type":"text","text":"camera pushes in"}]}`,
			want: `{"model":"sd-imported","prompt":"camera pushes in"}`,
		},
		{
			name: "first and last frame",
			body: `{"model":"client","content":[{"type":"text","text":"smooth transition"},{"type":"image_url","role":"first_frame","image_url":{"url":"https://8.8.8.8/first.png"}},{"type":"image_url","role":"last_frame","image_url":{"url":"https://8.8.4.4/last.png"}}],"duration":15,"ratio":"16:9","resolution":"720p"}`,
			want: `{"model":"sd-imported","prompt":"smooth transition","media":[{"type":"first_frame","url":"https://8.8.8.8/first.png"},{"type":"last_frame","url":"https://8.8.4.4/last.png"}],"parameters":{"resolution":"720p","ratio":"16:9","duration":15}}`,
		},
		{
			name: "multimodal references",
			body: `{"model":"client","content":[{"type":"text","text":"use @图1 @视频1 @音频1"},{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.8/ref.png"}},{"type":"video_url","role":"reference_video","video_url":{"url":"https://8.8.4.4/ref.mp4"}},{"type":"audio_url","role":"reference_audio","audio_url":{"url":"https://1.1.1.1/ref.mp3"}}],"duration":10,"ratio":"9:16"}`,
			want: `{"model":"sd-imported","prompt":"use @图1 @视频1 @音频1","media":[{"type":"reference_image","url":"https://8.8.8.8/ref.png"},{"type":"reference_video","url":"https://8.8.4.4/ref.mp4"},{"type":"reference_voice","url":"https://1.1.1.1/ref.mp3"}],"parameters":{"ratio":"9:16","duration":10}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request, err := parseARKRequest([]byte(tt.body), z5apiProtocolProfile())
			require.NoError(t, err)
			body, err := buildZ5APIRequest(request, "sd-imported")
			require.NoError(t, err)
			assert.JSONEq(t, tt.want, string(body))
		})
	}
}

func TestBuildZ5APIRequestRejectsUnsupportedFields(t *testing.T) {
	tests := []struct {
		name string
		body string
		code string
	}{
		{name: "watermark false", body: `{"model":"m","content":[{"type":"text","text":"t"}],"watermark":false}`, code: "InvalidParameter.watermark"},
		{name: "generate audio false", body: `{"model":"m","content":[{"type":"text","text":"t"}],"generate_audio":false}`, code: "InvalidParameter.generate_audio"},
		{name: "seed", body: `{"model":"m","content":[{"type":"text","text":"t"}],"seed":0}`, code: "InvalidParameter.seed"},
		{name: "callback", body: `{"model":"m","content":[{"type":"text","text":"t"}],"callback_url":"https://8.8.8.8/hook"}`, code: "InvalidParameter.callback_url"},
		{name: "draft false", body: `{"model":"m","content":[{"type":"text","text":"t"}],"draft":false}`, code: "InvalidParameter.draft"},
		{name: "flex tier", body: `{"model":"m","content":[{"type":"text","text":"t"}],"service_tier":"flex"}`, code: "InvalidParameter.service_tier"},
		{name: "unsupported ratio", body: `{"model":"m","content":[{"type":"text","text":"t"}],"ratio":"21:9"}`, code: "InvalidParameter.ratio"},
		{name: "duration overflow", body: fmt.Sprintf(`{"model":"m","content":[{"type":"text","text":"t"}],"duration":%d}`, relaycommon.MaxTaskDurationSeconds+1), code: "InvalidParameter.duration"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request, err := parseARKRequest([]byte(tt.body), z5apiProtocolProfile())
			if err == nil {
				err = validateZ5APIRequest(request)
			}
			require.Error(t, err)
			var requestErr *arkRequestError
			require.ErrorAs(t, err, &requestErr)
			assert.Equal(t, tt.code, requestErr.Code)
		})
	}
}

func TestBuildZ5APIRequestEnforcesMediaLimitsRolesAndPublicURLs(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr bool
	}{
		{name: "nine images", body: z5APIImageContent(9)},
		{name: "ten images", body: z5APIImageContent(10), wantErr: true},
		{name: "three videos", body: z5APIVideoContent(3)},
		{name: "four videos", body: z5APIVideoContent(4), wantErr: true},
		{name: "three audios with image", body: z5APIAudioContent(3)},
		{name: "four audios with image", body: z5APIAudioContent(4), wantErr: true},
		{name: "wrong image role", body: `{"model":"m","content":[{"type":"text","text":"t"},{"type":"image_url","role":"reference_video","image_url":{"url":"https://8.8.8.8/ref.png"}}]}`, wantErr: true},
		{name: "private image URL", body: `{"model":"m","content":[{"type":"text","text":"t"},{"type":"image_url","role":"reference_image","image_url":{"url":"http://192.168.1.2/ref.png"}}]}`, wantErr: true},
		{name: "data URI", body: `{"model":"m","content":[{"type":"text","text":"t"},{"type":"image_url","role":"reference_image","image_url":{"url":"data:image/png;base64,iVBORw0KGgo="}}]}`, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request, err := parseARKRequest([]byte(tt.body), z5apiProtocolProfile())
			if err == nil {
				_, err = buildZ5APIRequest(request, "sd-imported")
			}
			if tt.wantErr {
				require.Error(t, err)
				var requestErr *arkRequestError
				require.ErrorAs(t, err, &requestErr)
				assert.Equal(t, "InvalidParameter.content", requestErr.Code)
				return
			}
			require.NoError(t, err)
		})
	}
}

func z5APIImageContent(count int) string {
	items := `[{"type":"text","text":"t"}`
	for i := 0; i < count; i++ {
		items += fmt.Sprintf(`,{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.%d/ref.png"}}`, i%250+1)
	}
	return fmt.Sprintf(`{"model":"m","content":%s]}`, items)
}

func z5APIVideoContent(count int) string {
	items := `[{"type":"text","text":"t"},{"type":"image_url","role":"reference_image","image_url":{"url":"https://1.1.1.1/ref.png"}}`
	for i := 0; i < count; i++ {
		items += fmt.Sprintf(`,{"type":"video_url","role":"reference_video","video_url":{"url":"https://8.8.8.%d/ref.mp4"}}`, i%250+1)
	}
	return fmt.Sprintf(`{"model":"m","content":%s]}`, items)
}

func z5APIAudioContent(count int) string {
	items := `[{"type":"text","text":"t"},{"type":"image_url","role":"reference_image","image_url":{"url":"https://1.1.1.1/ref.png"}}`
	for i := 0; i < count; i++ {
		items += fmt.Sprintf(`,{"type":"audio_url","role":"reference_audio","audio_url":{"url":"https://8.8.8.%d/ref.mp3"}}`, i%250+1)
	}
	return fmt.Sprintf(`{"model":"m","content":%s]}`, items)
}
