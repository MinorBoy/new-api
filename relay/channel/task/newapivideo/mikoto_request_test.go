package newapivideo

import (
	"context"
	"fmt"
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

func TestMikotoProfileUsesDocumentedTaskContract(t *testing.T) {
	adaptor := NewMikotoTaskAdaptor()
	profile := adaptor.activeProfile()

	assert.Equal(t, ChannelNameMikoto, adaptor.GetChannelName())
	assert.Empty(t, adaptor.GetModelList())
	assert.Equal(t, "/v1/videos", profile.submitPath)
	assert.Equal(t, "/v1/videos/{task_id}", profile.pollPath)
	assert.Equal(t, "application/json", profile.contentType)
	assert.Equal(t, videoRequestDialectMikoto, profile.requestDialect)
	assert.Equal(t, mikotoDialectSora, mikotoRequestDialect("sora-v3-pro"))
	assert.Equal(t, mikotoDialectSeedance, mikotoRequestDialect("seedance-fast-720p"))
	assert.Equal(t, mikotoDialectUnknown, mikotoRequestDialect("unverified-model"))
}

func TestMikotoSoraRejectsReferenceVideoOutsideDocumentedDuration(t *testing.T) {
	service.SetVideoMetadataClient(mikotoVideoMetadataClient{durationMS: 2_000})
	t.Cleanup(func() { service.SetVideoMetadataClient(nil) })
	requestBody := `{"model":"client","content":[{"type":"text","text":"follow the reference"},{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.8/ref.png"}},{"type":"video_url","role":"reference_video","video_url":{"url":"https://8.8.4.4/ref.mp4"}}],"duration":8,"ratio":"16:9","resolution":"720p"}`
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", strings.NewReader(requestBody))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(common.KeySeedanceOfficialAPI, true)
	info := &relaycommon.RelayInfo{
		ChannelMeta:   &relaycommon.ChannelMeta{UpstreamModelName: "sora-v3-pro"},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
	}

	taskErr := NewMikotoTaskAdaptor().ValidateRequestAndSetAction(c, info)
	require.NotNil(t, taskErr)
	assert.Equal(t, "InvalidParameter.content", taskErr.Code)
}

type mikotoVideoMetadataClient struct {
	durationMS int64
}

func (client mikotoVideoMetadataClient) Metadata(context.Context, string) (videometa.Metadata, error) {
	return videometa.Metadata{
		DurationMS: client.durationMS, Width: 1280, Height: 720,
		FrameRateNum: 24, FrameRateDen: 1, Container: "mp4", ContentLength: 1024,
	}, nil
}

func TestMikotoAdaptorUsesProviderRequestDialect(t *testing.T) {
	requestBody := `{"model":"client","content":[{"type":"text","text":"follow the reference"},{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.8/ref.png"}}],"duration":8,"ratio":"16:9","generate_audio":false}`
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", strings.NewReader(requestBody))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(common.KeySeedanceOfficialAPI, true)
	info := &relaycommon.RelayInfo{
		ChannelMeta:   &relaycommon.ChannelMeta{UpstreamModelName: "seedance-2.0-720p"},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
	}
	adaptor := NewMikotoTaskAdaptor()

	require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))
	reader, err := adaptor.BuildRequestBody(c, info)
	require.NoError(t, err)
	body, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.JSONEq(t, `{"model":"seedance-2.0-720p","prompt":"follow the reference","duration":8,"aspect_ratio":"16:9","images":["https://8.8.8.8/ref.png"],"reference_mode":"media","generate_audio":false}`, string(body))
}

func TestBuildMikotoSoraRequestRejectsDocumentedInvalidInputs(t *testing.T) {
	tests := []struct {
		name string
		body string
		code string
	}{
		{name: "duration below minimum", body: `{"model":"m","content":[{"type":"text","text":"t"}],"duration":3,"ratio":"16:9","resolution":"720p"}`, code: "InvalidParameter.duration"},
		{name: "unsupported ratio", body: `{"model":"m","content":[{"type":"text","text":"t"}],"duration":4,"ratio":"2:1","resolution":"720p"}`, code: "InvalidParameter.ratio"},
		{name: "unsupported resolution", body: `{"model":"m","content":[{"type":"text","text":"t"}],"duration":4,"ratio":"16:9","resolution":"1080p"}`, code: "InvalidParameter.resolution"},
		{name: "data URI", body: `{"model":"m","content":[{"type":"text","text":"t"},{"type":"image_url","role":"reference_image","image_url":{"url":"data:image/png;base64,AAAA"}}],"duration":4,"ratio":"16:9","resolution":"720p"}`, code: "InvalidParameter.content"},
		{name: "private URL", body: `{"model":"m","content":[{"type":"text","text":"t"},{"type":"image_url","role":"reference_image","image_url":{"url":"http://192.168.1.2/ref.png"}}],"duration":4,"ratio":"16:9","resolution":"720p"}`, code: "InvalidParameter.content"},
		{name: "audio without image", body: `{"model":"m","content":[{"type":"text","text":"t"},{"type":"video_url","role":"reference_video","video_url":{"url":"https://8.8.8.8/ref.mp4"}},{"type":"audio_url","role":"reference_audio","audio_url":{"url":"https://1.1.1.1/ref.mp3"}}],"duration":4,"ratio":"16:9","resolution":"720p"}`, code: "InvalidParameter.content"},
		{name: "explicit service tier", body: `{"model":"m","content":[{"type":"text","text":"t"}],"duration":4,"ratio":"16:9","resolution":"720p","service_tier":"default"}`, code: "InvalidParameter.service_tier"},
		{name: "explicit draft false", body: `{"model":"m","content":[{"type":"text","text":"t"}],"duration":4,"ratio":"16:9","resolution":"720p","draft":false}`, code: "InvalidParameter.draft"},
		{name: "explicit empty tools", body: `{"model":"m","content":[{"type":"text","text":"t"}],"duration":4,"ratio":"16:9","resolution":"720p","tools":[]}`, code: "InvalidParameter.tools"},
		{name: "too many total references", body: mikotoSoraReferenceContent(9, 3, 1), code: "InvalidParameter.content"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request, err := parseARKRequest([]byte(test.body), mikotoProtocolProfile())
			if err == nil {
				_, err = buildMikotoRequest(request, "sora-v3-pro")
			}
			require.Error(t, err)
			var requestErr *arkRequestError
			require.ErrorAs(t, err, &requestErr)
			assert.Equal(t, test.code, requestErr.Code)
		})
	}
}

func mikotoSoraReferenceContent(images, videos, audios int) string {
	content := `[{"type":"text","text":"t"}`
	for index := 0; index < images; index++ {
		content += fmt.Sprintf(`,{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.%d/ref.png"}}`, index+1)
	}
	for index := 0; index < videos; index++ {
		content += fmt.Sprintf(`,{"type":"video_url","role":"reference_video","video_url":{"url":"https://8.8.4.%d/ref.mp4"}}`, index+1)
	}
	for index := 0; index < audios; index++ {
		content += fmt.Sprintf(`,{"type":"audio_url","role":"reference_audio","audio_url":{"url":"https://1.1.1.%d/ref.mp3"}}`, index+1)
	}
	return fmt.Sprintf(`{"model":"m","content":%s],"duration":4,"ratio":"16:9","resolution":"720p"}`, content)
}

func TestBuildMikotoSoraRequest(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "text only",
			body: `{"model":"client","content":[{"type":"text","text":"camera pushes in"}],"duration":8,"ratio":"16:9","resolution":"720p"}`,
			want: `{"model":"sora-v3-pro","prompt":"camera pushes in","seconds":"8","aspect_ratio":"16:9","resolution":"720p"}`,
		},
		{
			name: "first and last frames",
			body: `{"model":"client","content":[{"type":"text","text":"transition between frames"},{"type":"image_url","role":"first_frame","image_url":{"url":"https://8.8.8.8/first.png"}},{"type":"image_url","role":"last_frame","image_url":{"url":"https://8.8.4.4/last.png"}}],"duration":10,"ratio":"9:16","resolution":"720p"}`,
			want: `{"model":"sora-v3-pro","prompt":"transition between frames","seconds":"10","aspect_ratio":"9:16","resolution":"720p","image_url":"https://8.8.8.8/first.png","reference_image_urls":["https://8.8.4.4/last.png"],"video_config":{"reference_mode":"start_end"}}`,
		},
		{
			name: "reference media",
			body: `{"model":"client","content":[{"type":"text","text":"follow the references"},{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.8/ref.png"}},{"type":"video_url","role":"reference_video","video_url":{"url":"https://8.8.4.4/ref.mp4"}},{"type":"audio_url","role":"reference_audio","audio_url":{"url":"https://1.1.1.1/ref.mp3"}}],"duration":15,"ratio":"21:9","resolution":"720p"}`,
			want: `{"model":"sora-v3-pro","prompt":"follow the references","seconds":"15","aspect_ratio":"21:9","resolution":"720p","image_url":"https://8.8.8.8/ref.png","reference_videos":["https://8.8.4.4/ref.mp4"],"audio_url":"https://1.1.1.1/ref.mp3"}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request, err := parseARKRequest([]byte(test.body), mikotoProtocolProfile())
			require.NoError(t, err)
			body, err := buildMikotoRequest(request, "sora-v3-pro")
			require.NoError(t, err)
			assert.JSONEq(t, test.want, string(body))
		})
	}
}

func TestBuildMikotoSeedanceRequest(t *testing.T) {
	tests := []struct {
		name  string
		model string
		body  string
		want  string
	}{
		{
			name:  "text only omits optional fields",
			model: "seedance-fast-480p",
			body:  `{"model":"client","content":[{"type":"text","text":"camera pushes in"}],"duration":4}`,
			want:  `{"model":"seedance-fast-480p","prompt":"camera pushes in","duration":4}`,
		},
		{
			name:  "reference media preserves explicit false",
			model: "seedance-2.0-720p",
			body:  `{"model":"client","content":[{"type":"text","text":"follow the references"},{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.8/ref.png"}},{"type":"video_url","role":"reference_video","video_url":{"url":"data:video/mp4;base64,AAAA"}},{"type":"audio_url","role":"reference_audio","audio_url":{"url":"data:audio/mpeg;base64,AAAA"}}],"duration":8,"ratio":"9:16","generate_audio":false}`,
			want:  `{"model":"seedance-2.0-720p","prompt":"follow the references","duration":8,"aspect_ratio":"9:16","images":["https://8.8.8.8/ref.png"],"reference_mode":"media","referenceVideos":["data:video/mp4;base64,AAAA"],"referenceAudios":["data:audio/mpeg;base64,AAAA"],"generate_audio":false}`,
		},
		{
			name:  "first and last frames use frame mode",
			model: "seedance-fast-720p",
			body:  `{"model":"client","content":[{"type":"text","text":"transition"},{"type":"image_url","role":"first_frame","image_url":{"url":"https://8.8.8.8/first.png"}},{"type":"image_url","role":"last_frame","image_url":{"url":"https://8.8.4.4/last.png"}}],"duration":15,"ratio":"1:1"}`,
			want:  `{"model":"seedance-fast-720p","prompt":"transition","duration":15,"aspect_ratio":"1:1","images":["https://8.8.8.8/first.png","https://8.8.4.4/last.png"],"reference_mode":"frame"}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request, err := parseARKRequest([]byte(test.body), mikotoProtocolProfile())
			require.NoError(t, err)
			body, err := buildMikotoRequest(request, test.model)
			require.NoError(t, err)
			assert.JSONEq(t, test.want, string(body))
			assert.NotContains(t, string(body), `"resolution"`)
		})
	}
}

func TestMikotoDataURIEnforcesDecodedSizeAndMediaType(t *testing.T) {
	const dataURI = "data:video/mp4;base64,AAAA"

	assert.True(t, validMikotoDataURI(dataURI, "video", 3))
	assert.False(t, validMikotoDataURI(dataURI, "video", 2))
	assert.False(t, validMikotoDataURI(dataURI, "audio", 3))
}
