package e2e

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/setting/video_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLucenARKLifecycleE2E(t *testing.T) {
	setupSeedanceE2EDB(t)
	videoConfig := config.GlobalConfig.Get(video_setting.ConfigName)
	previousVideoConfig, err := config.ConfigToMap(videoConfig)
	require.NoError(t, err)
	require.NoError(t, config.UpdateConfigFromMap(videoConfig, map[string]string{
		video_setting.KeyBase64InputEnabled: "true",
	}))
	video_setting.UpdateAndSync()
	t.Cleanup(func() {
		require.NoError(t, config.UpdateConfigFromMap(videoConfig, previousVideoConfig))
		video_setting.UpdateAndSync()
	})

	mock := &mockNewAPIVideoServer{}
	server := httptest.NewServer(mock)
	t.Cleanup(server.Close)
	seedSeedanceE2EData(t, server.URL)

	channel, err := model.GetChannelById(e2eChannelID, true)
	require.NoError(t, err)
	mapping := `{"doubao-seedance-2-0-260128":"seedance-720p-token"}`
	channel.Type = constant.ChannelTypeLucen
	channel.Key = "mock-lucen-token-key"
	channel.Models = "doubao-seedance-2-0-260128"
	channel.ModelMapping = &mapping
	channel.BaseURL = common.GetPointer(server.URL)
	require.NoError(t, channel.Update())

	ratios := ratio_setting.GetModelRatioCopy()
	ratios["doubao-seedance-2-0-260128"] = 0.1
	encoded, err := common.Marshal(ratios)
	require.NoError(t, err)
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(string(encoded)))

	service.GetTaskAdaptorFunc = func(platform constant.TaskPlatform) service.TaskPollingAdaptor {
		return relay.GetTaskAdaptor(platform)
	}
	t.Cleanup(func() { service.GetTaskAdaptorFunc = nil })
	engine := seedanceE2ERouter()

	requestBody := `{
		"model":"doubao-seedance-2-0-260128",
		"content":[
			{"type":"text","text":"multimodal Lucen acceptance"},
			{"type":"image_url","role":"reference_image","image_url":{"url":"data:image/png;base64,QUJDRA=="}},
			{"type":"video_url","role":"reference_video","video_url":{"url":"asset://video-reference-1"}},
			{"type":"audio_url","role":"reference_audio","audio_url":{"url":"data:audio/wav;base64,UklGRg=="}}
		],
		"resolution":"720p",
		"ratio":"16:9",
		"duration":10,
		"generate_audio":true,
		"watermark":false,
		"callback_url":"https://client.example/callback",
		"return_last_frame":true,
		"priority":7,
		"execution_expires_after":3600
	}`
	status, submit := performJSONRequest(t, engine, http.MethodPost, "/api/v3/contents/generations/tasks", "Bearer e2e-1", requestBody)
	require.Equal(t, http.StatusOK, status, string(submit))
	var submitResponse map[string]any
	require.NoError(t, common.Unmarshal(submit, &submitResponse))
	require.Len(t, submitResponse, 1)
	publicID, ok := submitResponse["id"].(string)
	require.True(t, ok)
	assert.True(t, strings.HasPrefix(publicID, "task_"))
	assertLucenPublicTaskBody(t, submit)

	requests := mock.snapshot()
	require.Len(t, requests, 1)
	assert.Equal(t, http.MethodPost, requests[0].Method)
	assert.Equal(t, "/v1/video/generations", requests[0].Path)
	assert.Equal(t, "Bearer mock-lucen-token-key", requests[0].Authorization)
	assert.JSONEq(t, `{
		"model":"seedance-720p-token",
		"prompt":"multimodal Lucen acceptance",
		"content":[
			{"type":"text","text":"multimodal Lucen acceptance"},
			{"type":"image_url","role":"reference_image","image_url":{"url":"data:image/png;base64,QUJDRA=="}},
			{"type":"video_url","role":"reference_video","video_url":{"url":"asset://video-reference-1"}},
			{"type":"audio_url","role":"reference_audio","audio_url":{"url":"data:audio/wav;base64,UklGRg=="}}
		],
		"generateAudio":true,
		"ratio":"16:9",
		"seconds":"10",
		"watermark":false
	}`, string(requests[0].Body))
	for _, unsupported := range []string{"duration", "resolution", "callback_url", "return_last_frame", "priority", "execution_expires_after"} {
		assert.NotContains(t, string(requests[0].Body), `"`+unsupported+`"`)
	}

	task := pollNewAPIVideoTask(t, publicID)
	assert.Equal(t, model.TaskStatus(model.TaskStatusSuccess), task.Status)
	assert.Equal(t, "https://example.com/video.mp4", task.PrivateData.ResultURL)
	require.NotNil(t, task.PrivateData.BillingContext)
	assert.Equal(t, 216900, task.PrivateData.BillingContext.BillingTokens)

	requests = mock.snapshot()
	require.Len(t, requests, 2)
	assert.Equal(t, http.MethodGet, requests[1].Method)
	assert.Equal(t, "/v1/video/generations/upstream-task", requests[1].Path)
	assert.Equal(t, "Bearer mock-lucen-token-key", requests[1].Authorization)

	status, single := performJSONRequest(t, engine, http.MethodGet, "/api/v3/contents/generations/tasks/"+publicID, "Bearer e2e-1", "")
	require.Equal(t, http.StatusOK, status, string(single))
	assertLucenARKTaskResponse(t, single, publicID)

	status, list := performJSONRequest(t, engine, http.MethodGet, "/api/v3/contents/generations/tasks?filter.task_ids="+publicID+"&page_size=20", "Bearer e2e-1", "")
	require.Equal(t, http.StatusOK, status, string(list))
	assertLucenPublicTaskBody(t, list)
	var listResponse struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	require.NoError(t, common.Unmarshal(list, &listResponse))
	require.Equal(t, 1, listResponse.Total)
	require.Len(t, listResponse.Items, 1)
	listItem, err := common.Marshal(listResponse.Items[0])
	require.NoError(t, err)
	assertLucenARKTaskResponse(t, listItem, publicID)
	assert.Len(t, mock.snapshot(), 2)
}

func assertLucenARKTaskResponse(t *testing.T, body []byte, publicID string) {
	t.Helper()
	assertLucenPublicTaskBody(t, body)
	var response map[string]any
	require.NoError(t, common.Unmarshal(body, &response))
	assert.Equal(t, publicID, response["id"])
	assert.Equal(t, "doubao-seedance-2-0-260128", response["model"])
	assert.Equal(t, "succeeded", response["status"])
	assert.Equal(t, map[string]any{"video_url": "https://example.com/video.mp4"}, response["content"])
	assert.Equal(t, map[string]any{"completion_tokens": float64(216900), "total_tokens": float64(216900)}, response["usage"])
}

func assertLucenPublicTaskBody(t *testing.T, body []byte) {
	t.Helper()
	for _, privateValue := range []string{
		"upstream-task", "seedance-720p-token", "mock-lucen-token-key", "provider-secret", "doubao-seedance-2.0",
		"user_id", "channel_id", `"group"`, `"quota"`, `"platform"`, `"properties"`, "upstream_model_name",
	} {
		assert.NotContains(t, string(body), privateValue)
	}
}
