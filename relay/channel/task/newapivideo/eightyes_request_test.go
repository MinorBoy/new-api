package newapivideo

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEightYesProfileUsesDocumentedTaskContract(t *testing.T) {
	adaptor := NewEightYesTaskAdaptor()

	assert.Equal(t, ChannelNameEightYes, adaptor.GetChannelName())
	assert.Empty(t, adaptor.GetModelList())
	profile := adaptor.activeProfile()
	assert.Equal(t, "/v1/videos", profile.submitPath)
	assert.Equal(t, "/v1/videos/{task_id}", profile.pollPath)
	assert.Equal(t, "application/json", profile.contentType)
	assert.Equal(t, videoRequestDialectEightYes, profile.requestDialect)
	assert.Equal(t, 5, profile.defaultDurationSeconds)
	assert.True(t, profile.requirePublicHTTPMedia)
	assert.True(t, profile.singleFrameImagesAreReferences)
}

func TestBuildEightYesRequestEncodesArkMultimodalReferences(t *testing.T) {
	request, err := parseARKRequest([]byte(`{
		"model":"client-model",
		"content":[
			{"type":"text","text":"camera pushes in"},
			{"type":"image_url","role":"first_frame","image_url":{"url":"https://8.8.8.8/first.jpg"}},
			{"type":"image_url","role":"reference_image","image_url":{"url":"https://1.1.1.1/ref.jpg"}},
			{"type":"video_url","role":"reference_video","video_url":{"url":"https://9.9.9.9/ref.mp4"}},
			{"type":"audio_url","role":"reference_audio","audio_url":{"url":"https://8.8.4.4/ref.mp3"}}
		],
		"duration":5,
		"ratio":"16:9",
		"resolution":"480p",
		"generate_audio":true,
		"seed":4294967295
	}`), eightYesProtocolProfile())
	require.NoError(t, err)

	body, err := buildEightYesRequest(request, "videos-4-mini-480p")
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"model":"videos-4-mini-480p",
		"prompt":"camera pushes in",
		"duration":5,
		"ratio":"16:9",
		"seed":4294967295,
		"referenceImages":["https://8.8.8.8/first.jpg","https://1.1.1.1/ref.jpg"],
		"referenceVideos":["https://9.9.9.9/ref.mp4"],
		"referenceAudios":["https://8.8.4.4/ref.mp3"]
	}`, string(body))
	assert.NotContains(t, string(body), "generate_audio")
	assert.NotContains(t, string(body), "resolution")
}

func TestBuildEightYesRequestOmitsAbsentOptionalFields(t *testing.T) {
	request, err := parseARKRequest([]byte(`{
		"model":"client-model",
		"content":[{"type":"text","text":"text only"}]
	}`), eightYesProtocolProfile())
	require.NoError(t, err)

	body, err := buildEightYesRequest(request, "videos-4-mini-480p")
	require.NoError(t, err)
	assert.JSONEq(t, `{"model":"videos-4-mini-480p","prompt":"text only"}`, string(body))
}

func TestEightYesRequestValidationRejectsUnsupportedOrUnsafeFields(t *testing.T) {
	tests := []struct {
		name string
		body string
		code string
	}{
		{name: "last frame", body: `{"model":"m","content":[{"type":"text","text":"text"},{"type":"image_url","role":"last_frame","image_url":{"url":"https://8.8.8.8/last.jpg"}}]}`, code: "InvalidParameter.content"},
		{name: "watermark", body: `{"model":"m","content":[{"type":"text","text":"text"}],"watermark":false}`, code: "InvalidParameter.watermark"},
		{name: "generate audio false", body: `{"model":"m","content":[{"type":"text","text":"text"}],"generate_audio":false}`, code: "InvalidParameter.generate_audio"},
		{name: "priority tier", body: `{"model":"m","content":[{"type":"text","text":"text"}],"service_tier":"priority"}`, code: "InvalidParameter.service_tier"},
		{name: "draft", body: `{"model":"m","content":[{"type":"text","text":"text"}],"draft":true}`, code: "InvalidParameter.draft"},
		{name: "tools", body: `{"model":"m","content":[{"type":"text","text":"text"}],"tools":[{"type":"x"}]}`, code: "InvalidParameter.tools"},
		{name: "callback", body: `{"model":"m","content":[{"type":"text","text":"text"}],"callback_url":"https://8.8.8.8/hook"}`, code: "InvalidParameter.callback_url"},
		{name: "seed below range", body: `{"model":"m","content":[{"type":"text","text":"text"}],"seed":-2}`, code: "InvalidParameter.seed"},
		{name: "seed above range", body: `{"model":"m","content":[{"type":"text","text":"text"}],"seed":4294967296}`, code: "InvalidParameter.seed"},
		{name: "fractional seed", body: `{"model":"m","content":[{"type":"text","text":"text"}],"seed":1.5}`, code: "InvalidParameter.seed"},
		{name: "duration overflow", body: fmt.Sprintf(`{"model":"m","content":[{"type":"text","text":"text"}],"duration":%d}`, relaycommon.MaxTaskDurationSeconds+1), code: "InvalidParameter.duration"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request, err := parseARKRequest([]byte(tt.body), eightYesProtocolProfile())
			if err == nil {
				err = validateEightYesRequest(request, "")
			}
			require.Error(t, err)
			var requestErr *arkRequestError
			require.ErrorAs(t, err, &requestErr)
			assert.Equal(t, tt.code, requestErr.Code)
		})
	}
}

func TestEightYesAdaptorUsesMappedModelAndProviderPath(t *testing.T) {
	var receivedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.EscapedPath()
		assert.Equal(t, "Bearer key", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"queued"}`))
	}))
	t.Cleanup(server.Close)

	adaptor := NewEightYesTaskAdaptor()
	adaptor.Init(&relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: server.URL, ApiKey: "key"}})
	requestURL, err := adaptor.BuildRequestURL(nil)
	require.NoError(t, err)
	assert.Equal(t, server.URL+"/v1/videos", requestURL)

	response, err := adaptor.FetchTask(server.URL, "key", map[string]any{"task_id": "upstream/task"}, "")
	require.NoError(t, err)
	_, _ = io.Copy(io.Discard, response.Body)
	require.NoError(t, response.Body.Close())
	assert.Equal(t, "/v1/videos/upstream%2Ftask", receivedPath)
}

func TestEightYesBuildRequiresCompletedProviderValidation(t *testing.T) {
	request, err := parseARKRequest([]byte(`{"model":"client","content":[{"type":"text","text":"text"}]}`), eightYesProtocolProfile())
	require.NoError(t, err)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set(common.KeySeedanceOfficialAPI, true)
	c.Set(requestStateContextKey, requestState{ARK: &request})
	_, err = NewEightYesTaskAdaptor().BuildRequestBody(c, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "videos-4-mini-480p"}})
	assert.ErrorContains(t, err, "validation")
}
