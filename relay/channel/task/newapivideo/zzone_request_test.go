package newapivideo

import (
	"fmt"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestZZoneProfileUsesDocumentedContract(t *testing.T) {
	adaptor := NewZZoneTaskAdaptor()
	profile := adaptor.activeProfile()

	assert.Equal(t, ChannelNameZZone, adaptor.GetChannelName())
	assert.Empty(t, adaptor.GetModelList())
	assert.Equal(t, "/v1/videos", profile.submitPath)
	assert.Equal(t, "/v1/videos/{task_id}", profile.pollPath)
	assert.Equal(t, "application/json", profile.contentType)
	assert.Equal(t, videoRequestDialectZZone, profile.requestDialect)
	assert.True(t, profile.requirePublicHTTPMedia)
}

func TestBuildZZoneRequestEncodesDocumentedFields(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "text only omits optional fields",
			body: `{"model":"client-model","content":[{"type":"text","text":"city at night"}]}`,
			want: `{"model":"imported-zzone-model","prompt":"city at night"}`,
		},
		{
			name: "complete multimedia request",
			body: `{
				"model":"client-model",
				"content":[
					{"type":"text","text":"city at night"},
					{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.8/a.png"}},
					{"type":"video_url","role":"reference_video","video_url":{"url":"https://8.8.4.4/a.mp4"}},
					{"type":"audio_url","role":"reference_audio","audio_url":{"url":"https://1.1.1.1/a.mp3"}}
				],
				"duration":15,
				"ratio":"9:16"
			}`,
			want: `{
				"model":"imported-zzone-model",
				"prompt":"city at night",
				"seconds":"15",
				"aspect_ratio":"9:16",
				"images":["https://8.8.8.8/a.png"],
				"videos":["https://8.8.4.4/a.mp4"],
				"audios":["https://1.1.1.1/a.mp3"]
			}`,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			request, err := parseARKRequest([]byte(testCase.body), zzoneProtocolProfile())
			require.NoError(t, err)
			body, err := buildZZoneRequest(request, "imported-zzone-model")
			require.NoError(t, err)
			assert.JSONEq(t, testCase.want, string(body))
		})
	}
}

func TestBuildZZoneRequestUses720pAsRoutingFactWithoutUpstreamField(t *testing.T) {
	request, err := parseARKRequest([]byte(`{
		"model":"client-model",
		"content":[{"type":"text","text":"city at night"}],
		"resolution":"720p"
	}`), zzoneProtocolProfile())
	require.NoError(t, err)

	body, err := buildZZoneRequest(request, "imported-zzone-model")
	require.NoError(t, err)
	assert.JSONEq(t, `{"model":"imported-zzone-model","prompt":"city at night"}`, string(body))
	assert.NotContains(t, string(body), `"resolution"`)
}

func TestBuildZZoneRequestRejectsUnsupportedFields(t *testing.T) {
	tests := []struct {
		name string
		body string
		code string
	}{
		{name: "480p resolution", body: `{"model":"m","content":[{"type":"text","text":"t"}],"resolution":"480p"}`, code: "InvalidParameter.resolution"},
		{name: "1080p resolution", body: `{"model":"m","content":[{"type":"text","text":"t"}],"resolution":"1080p"}`, code: "InvalidParameter.resolution"},
		{name: "4k resolution", body: `{"model":"m","content":[{"type":"text","text":"t"}],"resolution":"4k"}`, code: "InvalidParameter.resolution"},
		{name: "seed", body: `{"model":"m","content":[{"type":"text","text":"t"}],"seed":0}`, code: "InvalidParameter.seed"},
		{name: "watermark false", body: `{"model":"m","content":[{"type":"text","text":"t"}],"watermark":false}`, code: "InvalidParameter.watermark"},
		{name: "generate audio false", body: `{"model":"m","content":[{"type":"text","text":"t"}],"generate_audio":false}`, code: "InvalidParameter.generate_audio"},
		{name: "service tier default", body: `{"model":"m","content":[{"type":"text","text":"t"}],"service_tier":"default"}`, code: "InvalidParameter.service_tier"},
		{name: "draft false", body: `{"model":"m","content":[{"type":"text","text":"t"}],"draft":false}`, code: "InvalidParameter.draft"},
		{name: "empty tools", body: `{"model":"m","content":[{"type":"text","text":"t"}],"tools":[]}`, code: "InvalidParameter.tools"},
		{name: "callback", body: `{"model":"m","content":[{"type":"text","text":"t"}],"callback_url":"https://8.8.8.8/hook"}`, code: "InvalidParameter.callback_url"},
		{name: "unsupported ratio", body: `{"model":"m","content":[{"type":"text","text":"t"}],"ratio":"21:9"}`, code: "InvalidParameter.ratio"},
		{name: "duration overflow", body: fmt.Sprintf(`{"model":"m","content":[{"type":"text","text":"t"}],"duration":%d}`, relaycommon.MaxTaskDurationSeconds+1), code: "InvalidParameter.duration"},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			request, err := parseARKRequest([]byte(testCase.body), zzoneProtocolProfile())
			if err == nil {
				err = validateZZoneRequest(request)
			}
			require.Error(t, err)
			var requestErr *arkRequestError
			require.ErrorAs(t, err, &requestErr)
			assert.Equal(t, testCase.code, requestErr.Code)
		})
	}
}

func TestBuildZZoneRequestEnforcesMediaLimitsAndPublicURLs(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr bool
	}{
		{name: "documented maxima", body: zzoneMediaContent(4, 3, 1)},
		{name: "five images", body: zzoneMediaContent(5, 0, 0), wantErr: true},
		{name: "four videos", body: zzoneMediaContent(0, 4, 0), wantErr: true},
		{name: "two audios", body: zzoneMediaContent(0, 0, 2), wantErr: true},
		{name: "private image URL", body: `{"model":"m","content":[{"type":"text","text":"t"},{"type":"image_url","role":"reference_image","image_url":{"url":"http://192.168.1.2/ref.png"}}]}`, wantErr: true},
		{name: "local path", body: `{"model":"m","content":[{"type":"text","text":"t"},{"type":"image_url","role":"reference_image","image_url":{"url":"C:\\Users\\me\\ref.png"}}]}`, wantErr: true},
		{name: "data URI", body: `{"model":"m","content":[{"type":"text","text":"t"},{"type":"image_url","role":"reference_image","image_url":{"url":"data:image/png;base64,iVBORw0KGgo="}}]}`, wantErr: true},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			request, err := parseARKRequest([]byte(testCase.body), zzoneProtocolProfile())
			if err == nil {
				_, err = buildZZoneRequest(request, "imported-zzone-model")
			}
			if testCase.wantErr {
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

func zzoneMediaContent(images, videos, audios int) string {
	items := `[{"type":"text","text":"t"}`
	for index := 0; index < images; index++ {
		items += fmt.Sprintf(`,{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.%d/ref.png"}}`, index+1)
	}
	for index := 0; index < videos; index++ {
		items += fmt.Sprintf(`,{"type":"video_url","role":"reference_video","video_url":{"url":"https://8.8.4.%d/ref.mp4"}}`, index+1)
	}
	for index := 0; index < audios; index++ {
		items += fmt.Sprintf(`,{"type":"audio_url","role":"reference_audio","audio_url":{"url":"https://1.1.1.%d/ref.mp3"}}`, index+1)
	}
	return fmt.Sprintf(`{"model":"m","content":%s]}`, items)
}
