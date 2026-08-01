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

func TestFourSTokenProfile(t *testing.T) {
	adaptor := NewFourSTokenTaskAdaptor()
	assert.Equal(t, ChannelNameFourSToken, adaptor.GetChannelName())
	assert.Empty(t, adaptor.GetModelList())
	profile := adaptor.activeProfile()
	assert.Equal(t, "/v1/videos", profile.submitPath)
	assert.Equal(t, "/v1/videos/{task_id}", profile.pollPath)
	assert.Equal(t, videoRequestDialectFourSToken, profile.requestDialect)
	assert.True(t, profile.requirePublicHTTPMedia)
}

func TestBuildFourSTokenRequestPreservesContentOrderAndSnakeCaseFields(t *testing.T) {
	request, err := parseARKRequest([]byte(`{
		"model":"client-model",
		"content":[
			{"type":"text","text":"camera pushes in"},
			{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.8/ref.jpg"}},
			{"type":"video_url","role":"reference_video","video_url":{"url":"https://1.1.1.1/ref.mp4"}},
			{"type":"audio_url","role":"reference_audio","audio_url":{"url":"https://9.9.9.9/ref.mp3"}}
		],
		"generate_audio":true,
		"ratio":"16:9",
		"duration":5,
		"watermark":false,
		"resolution":"720p",
		"seed":4294967295
	}`), fourSTokenProtocolProfile())
	require.NoError(t, err)

	body, err := buildFourSTokenRequest(request, "4sdance_v2.0_900")
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"model":"4sdance_v2.0_900",
		"content":[
			{"type":"text","text":"camera pushes in"},
			{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.8/ref.jpg"}},
			{"type":"video_url","role":"reference_video","video_url":{"url":"https://1.1.1.1/ref.mp4"}},
			{"type":"audio_url","role":"reference_audio","audio_url":{"url":"https://9.9.9.9/ref.mp3"}}
		],
		"generate_audio":true,
		"ratio":"16:9",
		"duration":5,
		"watermark":false,
		"resolution":"720p",
		"seed":4294967295
	}`, string(body))
}

func TestBuildFourSTokenRequestOmitsAbsentOptionalFields(t *testing.T) {
	request, err := parseARKRequest([]byte(`{
		"model":"client-model",
		"content":[{"type":"text","text":"text only"}]
	}`), fourSTokenProtocolProfile())
	require.NoError(t, err)
	body, err := buildFourSTokenRequest(request, "4sdance_v2.0_900")
	require.NoError(t, err)
	assert.JSONEq(t, `{"model":"4sdance_v2.0_900","content":[{"type":"text","text":"text only"}]}`, string(body))
	assert.NotContains(t, string(body), "generate_audio")
	assert.NotContains(t, string(body), "watermark")
	assert.NotContains(t, string(body), "seed")
}

func TestFourSTokenRequestValidationRejectsUnsupportedFieldsAndSeedRange(t *testing.T) {
	tests := []struct {
		name string
		body string
		code string
	}{
		{name: "callback url", body: `{"model":"m","content":[{"type":"text","text":"text"}],"callback_url":"https://example.com/hook"}`, code: "InvalidParameter.callback_url"},
		{name: "task id content", body: `{"model":"m","content":[{"type":"text","text":"text"},{"type":"task_id","task_id":"private"}]}`, code: "InvalidParameter.content"},
		{name: "seed overflow", body: `{"model":"m","content":[{"type":"text","text":"text"}],"seed":4294967296}`, code: "InvalidParameter.seed"},
		{name: "seed malformed", body: `{"model":"m","content":[{"type":"text","text":"text"}],"seed":1.5}`, code: "InvalidParameter.seed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseARKRequest([]byte(tt.body), fourSTokenProtocolProfile())
			require.Error(t, err)
			var requestErr *arkRequestError
			require.ErrorAs(t, err, &requestErr)
			assert.Equal(t, tt.code, requestErr.Code)
		})
	}
}

func TestFourSTokenTransportUsesProviderPathAndEscapesTaskIDOnce(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/videos/upstream%2Ftask", r.URL.EscapedPath())
		assert.Equal(t, "Bearer key", r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	adaptor := NewFourSTokenTaskAdaptor()
	adaptor.Init(&relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: server.URL, ApiKey: "key"}})
	requestURL, err := adaptor.BuildRequestURL(nil)
	require.NoError(t, err)
	assert.Equal(t, server.URL+"/v1/videos", requestURL)
	response, err := adaptor.FetchTask(server.URL, "key", map[string]any{"task_id": "upstream/task"}, "")
	require.NoError(t, err)
	_, _ = io.Copy(io.Discard, response.Body)
	require.NoError(t, response.Body.Close())
}

func fourSTokenValidationContext(body, upstreamModel string) (*gin.Context, *relaycommon.RelayInfo) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(common.KeySeedanceOfficialAPI, true)
	return c, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: "https://api.4stoken.cn", ApiKey: "key", UpstreamModelName: upstreamModel}, OriginModelName: "client", TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
}
