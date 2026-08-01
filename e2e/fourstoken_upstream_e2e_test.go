package e2e

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	fourSTokenE2EClientModel    = "doubao-seedance-2-0-260128"
	fourSTokenE2EUpstreamModel  = "4sdance_v2.0_900"
	fourSTokenE2EUpstreamTaskID = "fourstoken-private-task"
	fourSTokenE2EVideoURL       = "https://assets.example/fourstoken.mp4"
)

type fourSTokenE2EMock struct {
	mu            sync.Mutex
	requests      []mockArkRequest
	pollResponses []string
	pollIndex     int
}

func (m *fourSTokenE2EMock) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	body, _ := io.ReadAll(request.Body)
	m.mu.Lock()
	m.requests = append(m.requests, mockArkRequest{
		Method: request.Method, Path: request.URL.Path,
		Authorization: request.Header.Get("Authorization"), Body: append([]byte(nil), body...),
	})
	response := ""
	switch {
	case request.Method == http.MethodPost && request.URL.Path == "/v1/videos":
		response = `{"taskId":"` + fourSTokenE2EUpstreamTaskID + `"}`
	case request.Method == http.MethodGet && request.URL.Path == "/v1/videos/"+fourSTokenE2EUpstreamTaskID:
		if len(m.pollResponses) > 0 {
			index := min(m.pollIndex, len(m.pollResponses)-1)
			response = m.pollResponses[index]
			m.pollIndex++
		}
	}
	m.mu.Unlock()

	writer.Header().Set("Content-Type", "application/json")
	if response == "" {
		http.NotFound(writer, request)
		return
	}
	_, _ = writer.Write([]byte(response))
}

func (m *fourSTokenE2EMock) snapshot() []mockArkRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	requests := make([]mockArkRequest, len(m.requests))
	copy(requests, m.requests)
	return requests
}

type fourSTokenE2EEnvironment struct {
	engine *gin.Engine
	mock   *fourSTokenE2EMock
}

func setupFourSTokenE2E(t *testing.T, pollResponses ...string) *fourSTokenE2EEnvironment {
	t.Helper()
	setupSeedanceE2EDB(t)
	mock := &fourSTokenE2EMock{pollResponses: pollResponses}
	server := httptest.NewServer(mock)
	t.Cleanup(server.Close)
	seedSeedanceE2EData(t, server.URL)

	channel, err := model.GetChannelById(e2eChannelID, true)
	require.NoError(t, err)
	mapping := `{"` + fourSTokenE2EClientModel + `":"` + fourSTokenE2EUpstreamModel + `"}`
	channel.Type = constant.ChannelTypeFourSToken
	channel.Key = "mock-fourstoken-key"
	channel.Models = fourSTokenE2EClientModel
	channel.ModelMapping = &mapping
	require.NoError(t, channel.Update())

	ratios := ratio_setting.GetModelRatioCopy()
	ratios[fourSTokenE2EClientModel] = 0.1
	encoded, err := common.Marshal(ratios)
	require.NoError(t, err)
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(string(encoded)))

	service.GetTaskAdaptorFunc = func(platform constant.TaskPlatform) service.TaskPollingAdaptor {
		return relay.GetTaskAdaptor(platform)
	}
	t.Cleanup(func() { service.GetTaskAdaptorFunc = nil })
	return &fourSTokenE2EEnvironment{engine: seedanceE2ERouter(), mock: mock}
}

func TestFourSTokenArkLifecycleE2E(t *testing.T) {
	env := setupFourSTokenE2E(t,
		`{"id":"fourstoken-private-task","status":"running","progress":50}`,
		`{"id":"fourstoken-private-task","status":"succeeded","progress":100,"content":{"video_url":"https://assets.example/fourstoken.mp4","last_frame_url":"https://assets.example/last.jpg"}}`,
	)
	requestBody := `{
		"model":"doubao-seedance-2-0-260128",
		"content":[
			{"type":"text","text":"4stoken multimodal request"},
			{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.8/ref.jpg"}},
			{"type":"video_url","role":"reference_video","video_url":{"url":"https://1.1.1.1/ref.mp4"}},
			{"type":"audio_url","role":"reference_audio","audio_url":{"url":"https://9.9.9.9/ref.mp3"}}
		],
		"generate_audio":true,"ratio":"16:9","duration":8,
		"watermark":false,"resolution":"720p","seed":0
	}`

	status, submit := performJSONRequest(t, env.engine, http.MethodPost, "/api/v3/contents/generations/tasks", "Bearer e2e-1", requestBody)
	require.Equal(t, http.StatusOK, status, string(submit))
	var submitResponse map[string]any
	require.NoError(t, common.Unmarshal(submit, &submitResponse))
	require.Len(t, submitResponse, 1)
	publicID, ok := submitResponse["id"].(string)
	require.True(t, ok)
	assert.True(t, strings.HasPrefix(publicID, "task_"))
	assertFourSTokenPublicBody(t, submit)

	requests := env.mock.snapshot()
	require.Len(t, requests, 1)
	assert.Equal(t, http.MethodPost, requests[0].Method)
	assert.Equal(t, "/v1/videos", requests[0].Path)
	assert.Equal(t, "Bearer mock-fourstoken-key", requests[0].Authorization)
	assert.JSONEq(t, `{
		"model":"4sdance_v2.0_900",
		"content":[
			{"type":"text","text":"4stoken multimodal request"},
			{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.8/ref.jpg"}},
			{"type":"video_url","role":"reference_video","video_url":{"url":"https://1.1.1.1/ref.mp4"}},
			{"type":"audio_url","role":"reference_audio","audio_url":{"url":"https://9.9.9.9/ref.mp3"}}
		],
		"generate_audio":true,"ratio":"16:9","duration":8,
		"watermark":false,"resolution":"720p","seed":0
	}`, string(requests[0].Body))

	task := pollNewAPIVideoTask(t, publicID)
	assert.Equal(t, model.TaskStatus(model.TaskStatusInProgress), task.Status)
	task = pollNewAPIVideoTask(t, publicID)
	assert.Equal(t, model.TaskStatus(model.TaskStatusSuccess), task.Status)
	assert.Equal(t, fourSTokenE2EUpstreamTaskID, task.PrivateData.UpstreamTaskID)
	assert.Equal(t, fourSTokenE2EVideoURL, task.PrivateData.ResultURL)

	status, single := performJSONRequest(t, env.engine, http.MethodGet, "/api/v3/contents/generations/tasks/"+publicID, "Bearer e2e-1", "")
	require.Equal(t, http.StatusOK, status, string(single))
	assertFourSTokenPublicBody(t, single)
	assert.Contains(t, string(single), `"id":"`+publicID+`"`)
	assert.Contains(t, string(single), `"model":"`+fourSTokenE2EClientModel+`"`)
	assert.Contains(t, string(single), `"video_url":"`+fourSTokenE2EVideoURL+`"`)
	assert.Contains(t, string(single), `"last_frame_url":"https://assets.example/last.jpg"`)

	status, list := performJSONRequest(t, env.engine, http.MethodGet, "/api/v3/contents/generations/tasks?filter.task_ids="+publicID+"&page_size=20", "Bearer e2e-1", "")
	require.Equal(t, http.StatusOK, status, string(list))
	assertFourSTokenPublicBody(t, list)
	assert.Contains(t, string(list), publicID)
}

func TestFourSTokenInvalidRequestsHaveNoSideEffectsE2E(t *testing.T) {
	tests := []struct {
		name string
		body string
		code string
	}{
		{name: "callback", body: `{"model":"doubao-seedance-2-0-260128","content":[{"type":"text","text":"text"}],"callback_url":"https://example.com/hook"}`, code: "InvalidParameter.callback_url"},
		{name: "task id", body: `{"model":"doubao-seedance-2-0-260128","content":[{"type":"text","text":"text"},{"type":"task_id","task_id":"private"}]}`, code: "InvalidParameter.content"},
		{name: "seed overflow", body: `{"model":"doubao-seedance-2-0-260128","content":[{"type":"text","text":"text"}],"seed":4294967296}`, code: "InvalidParameter.seed"},
		{name: "private media", body: `{"model":"doubao-seedance-2-0-260128","content":[{"type":"text","text":"text"},{"type":"image_url","role":"reference_image","image_url":{"url":"http://127.0.0.1/ref.jpg"}}]}`, code: "InvalidParameter.content"},
		{name: "duration overflow", body: `{"model":"doubao-seedance-2-0-260128","content":[{"type":"text","text":"text"}],"duration":86401}`, code: "InvalidParameter.duration"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			env := setupFourSTokenE2E(t)
			var beforeTasks int64
			var beforeUser model.User
			require.NoError(t, model.DB.Model(&model.Task{}).Count(&beforeTasks).Error)
			require.NoError(t, model.DB.First(&beforeUser, e2eUserID).Error)

			status, body := performJSONRequest(t, env.engine, http.MethodPost, "/api/v3/contents/generations/tasks", "Bearer e2e-1", test.body)
			assert.Equal(t, http.StatusBadRequest, status, string(body))
			assert.Contains(t, string(body), `"code":"`+test.code+`"`)
			assert.Empty(t, env.mock.snapshot())

			var afterTasks int64
			var afterUser model.User
			require.NoError(t, model.DB.Model(&model.Task{}).Count(&afterTasks).Error)
			require.NoError(t, model.DB.First(&afterUser, e2eUserID).Error)
			assert.Equal(t, beforeTasks, afterTasks)
			assert.Equal(t, beforeUser.Quota, afterUser.Quota)
			assert.Equal(t, beforeUser.UsedQuota, afterUser.UsedQuota)
		})
	}
}

func TestFourSTokenFailedTaskRefundsExactlyOnceE2E(t *testing.T) {
	env := setupFourSTokenE2E(t,
		`{"id":"fourstoken-private-task","status":"failed","error":{"code":"provider_error","message":"generation failed"}}`,
	)
	var beforeUser model.User
	require.NoError(t, model.DB.First(&beforeUser, e2eUserID).Error)

	status, submit := performJSONRequest(t, env.engine, http.MethodPost, "/api/v3/contents/generations/tasks", "Bearer e2e-1", `{"model":"doubao-seedance-2-0-260128","content":[{"type":"text","text":"refund"}],"duration":8}`)
	require.Equal(t, http.StatusOK, status, string(submit))
	var submitResponse map[string]any
	require.NoError(t, common.Unmarshal(submit, &submitResponse))
	publicID := submitResponse["id"].(string)

	service.RunTaskPollingOnce(context.Background(), nil)
	service.RunTaskPollingOnce(context.Background(), nil)
	var task model.Task
	require.NoError(t, model.DB.Where("task_id = ?", publicID).First(&task).Error)
	assert.Equal(t, model.TaskStatus(model.TaskStatusFailure), task.Status)
	assert.Zero(t, task.Quota)
	var afterUser model.User
	require.NoError(t, model.DB.First(&afterUser, e2eUserID).Error)
	assert.Equal(t, beforeUser.Quota, afterUser.Quota)
	assert.Equal(t, beforeUser.UsedQuota, afterUser.UsedQuota)

	var refundLogs []model.Log
	require.NoError(t, model.LOG_DB.Where("type = ?", model.LogTypeRefund).Find(&refundLogs).Error)
	require.Len(t, refundLogs, 1)
}

func assertFourSTokenPublicBody(t *testing.T, body []byte) {
	t.Helper()
	for _, privateValue := range []string{
		fourSTokenE2EUpstreamTaskID, fourSTokenE2EUpstreamModel, "mock-fourstoken-key",
		"user_id", "channel_id", `"group"`, `"quota"`, `"platform"`, `"properties"`, "upstream_model_name",
	} {
		assert.NotContains(t, string(body), privateValue)
	}
}
