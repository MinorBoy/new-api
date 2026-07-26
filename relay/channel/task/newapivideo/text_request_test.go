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

func TestCangyuanProfileAndTransport(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/videos/upstream%2Ftask", r.URL.EscapedPath())
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	adaptor := NewCangyuanTaskAdaptor()
	assert.Equal(t, "Cangyuan", adaptor.GetChannelName())
	assert.Equal(t, []string{"seedance-2.0-720p"}, adaptor.GetModelList())
	assert.Equal(t, "/v1/videos", adaptor.activeProfile().submitPath)
	assert.Equal(t, "/v1/videos/{task_id}", adaptor.activeProfile().pollPath)
	assert.Equal(t, "application/json", adaptor.activeProfile().contentType)
	assert.Equal(t, videoRequestDialectTextJSON, adaptor.activeProfile().requestDialect)

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

func TestCangyuanValidationRejectsUnsupportedRequestsBeforeBuild(t *testing.T) {
	tests := []struct {
		name string
		body string
		code string
	}{
		{
			name: "image",
			body: `{"model":"m","content":[{"type":"text","text":"text"},{"type":"image_url","image_url":{"url":"https://x/ref.png"}}]}`,
			code: "InvalidParameter.content",
		},
		{
			name: "video",
			body: `{"model":"m","content":[{"type":"text","text":"text"},{"type":"video_url","role":"reference_video","video_url":{"url":"https://x/ref.mp4"}}]}`,
			code: "InvalidParameter.content",
		},
		{
			name: "audio",
			body: `{"model":"m","content":[{"type":"text","text":"text"},{"type":"audio_url","role":"reference_audio","audio_url":{"url":"https://x/ref.mp3"}}]}`,
			code: "InvalidParameter.content",
		},
		{
			name: "draft task",
			body: `{"model":"m","content":[{"type":"text","text":"text"},{"type":"draft_task","draft_task":{"id":"task"}}]}`,
			code: "InvalidParameter.content",
		},
		{
			name: "generate audio true",
			body: `{"model":"m","content":[{"type":"text","text":"text"}],"generate_audio":true}`,
			code: "InvalidParameter.generate_audio",
		},
		{
			name: "generate audio false",
			body: `{"model":"m","content":[{"type":"text","text":"text"}],"generate_audio":false}`,
			code: "InvalidParameter.generate_audio",
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

func TestCangyuanUsesOnlySharedDurationBoundary(t *testing.T) {
	tests := []struct {
		name      string
		duration  int
		wantError bool
	}{
		{name: "minimum", duration: 1},
		{name: "maximum", duration: relaycommon.MaxTaskDurationSeconds},
		{name: "explicit zero", duration: 0, wantError: true},
		{name: "above shared maximum", duration: relaycommon.MaxTaskDurationSeconds + 1, wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := arkRequest{
				Model:    "client-model",
				Content:  []arkContent{{Type: "text", Text: "text"}},
				Duration: common.GetPointer(tt.duration),
			}
			err := validateTextVideoRequest(request, *cangyuanProtocolProfile().textRequest)
			if tt.wantError {
				var requestErr *arkRequestError
				require.ErrorAs(t, err, &requestErr)
				assert.Equal(t, "InvalidParameter.duration", requestErr.Code)
				return
			}
			assert.NoError(t, err)
		})
	}
}

func TestCangyuanDoesNotLocallyRestrictRatioOrResolution(t *testing.T) {
	request := arkRequest{
		Model:      "client-model",
		Content:    []arkContent{{Type: "text", Text: "text"}},
		Ratio:      "4:3",
		Resolution: "2160p",
	}
	assert.NoError(t, validateTextVideoRequest(request, *cangyuanProtocolProfile().textRequest))
}

func TestBuildCangyuanTextVideoRequestDefensivelyRevalidates(t *testing.T) {
	request := arkRequest{
		Model:    "client-model",
		Content:  []arkContent{{Type: "text", Text: "text"}},
		Duration: common.GetPointer(0),
	}
	body, err := buildTextVideoRequest(request, "seedance-2.0-720p", *cangyuanProtocolProfile().textRequest)
	var requestErr *arkRequestError
	require.ErrorAs(t, err, &requestErr)
	assert.Equal(t, "InvalidParameter.duration", requestErr.Code)
	assert.Nil(t, body)
}

func cangyuanValidationContext(body string) (*gin.Context, *relaycommon.RelayInfo) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(common.KeySeedanceOfficialAPI, true)
	return c, &relaycommon.RelayInfo{
		ChannelMeta:   &relaycommon.ChannelMeta{UpstreamModelName: "seedance-2.0-720p"},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
	}
}
