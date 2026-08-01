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
	cangyuanE2EUpstreamTaskID = "cangyuan-private"
	cangyuanE2EVideoURL       = "https://assets.example/cangyuan.mp4"
)

type cangyuanE2EMock struct {
	mu            sync.Mutex
	requests      []mockArkRequest
	pollResponses []string
	pollIndex     int
}

func (m *cangyuanE2EMock) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	body, _ := io.ReadAll(request.Body)
	m.mu.Lock()
	m.requests = append(m.requests, mockArkRequest{
		Method: request.Method, Path: request.URL.Path,
		Authorization: request.Header.Get("Authorization"), Body: append([]byte(nil), body...),
	})
	response := ""
	switch {
	case request.Method == http.MethodPost && request.URL.Path == "/v1/videos":
		response = `{"task_id":"` + cangyuanE2EUpstreamTaskID + `","status":"queued"}`
	case request.Method == http.MethodGet && request.URL.Path == "/v1/videos/"+cangyuanE2EUpstreamTaskID:
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

func (m *cangyuanE2EMock) snapshot() []mockArkRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	requests := make([]mockArkRequest, len(m.requests))
	copy(requests, m.requests)
	return requests
}

type cangyuanE2EEnvironment struct {
	engine *gin.Engine
	mock   *cangyuanE2EMock
}

func setupCangyuanE2E(t *testing.T, pollResponses ...string) *cangyuanE2EEnvironment {
	t.Helper()
	setupSeedanceE2EDB(t)
	mock := &cangyuanE2EMock{pollResponses: pollResponses}
	server := httptest.NewServer(mock)
	t.Cleanup(server.Close)
	seedSeedanceE2EData(t, server.URL)

	channel, err := model.GetChannelById(e2eChannelID, true)
	require.NoError(t, err)
	mapping := `{"doubao-seedance-2-0-260128":"seedance-2.0-720p"}`
	channel.Type = constant.ChannelTypeCangyuan
	channel.Key = "mock-cangyuan-key"
	channel.Models = "doubao-seedance-2-0-260128"
	channel.ModelMapping = &mapping
	require.NoError(t, channel.Update())

	ratio := ratio_setting.GetModelRatioCopy()
	ratio["doubao-seedance-2-0-260128"] = 0.1
	encoded, err := common.Marshal(ratio)
	require.NoError(t, err)
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(string(encoded)))

	service.GetTaskAdaptorFunc = func(platform constant.TaskPlatform) service.TaskPollingAdaptor {
		return relay.GetTaskAdaptor(platform)
	}
	t.Cleanup(func() { service.GetTaskAdaptorFunc = nil })
	return &cangyuanE2EEnvironment{engine: seedanceE2ERouter(), mock: mock}
}

func TestCangyuanARKLifecycleAndTextRequestE2E(t *testing.T) {
	env := setupCangyuanE2E(t,
		`{"task_id":"cangyuan-private","status":"in_progress","progress":50}`,
		`{"task_id":"cangyuan-private","status":"completed","data":[{"url":"https://assets.example/cangyuan.mp4"}]}`,
	)
	requestBody := `{"model":"doubao-seedance-2-0-260128","content":[{"type":"text","text":"cangyuan text acceptance"}],"duration":8,"ratio":"16:9","resolution":"720p"}`

	status, submit := performJSONRequest(t, env.engine, http.MethodPost, "/api/v3/contents/generations/tasks", "Bearer e2e-1", requestBody)
	require.Equal(t, http.StatusOK, status, string(submit))
	var submitResponse map[string]any
	require.NoError(t, common.Unmarshal(submit, &submitResponse))
	publicID, ok := submitResponse["id"].(string)
	require.True(t, ok)
	assert.True(t, strings.HasPrefix(publicID, "task_"))
	assertCangyuanE2EPublicBody(t, submit)

	requests := env.mock.snapshot()
	require.Len(t, requests, 1)
	assert.Equal(t, http.MethodPost, requests[0].Method)
	assert.Equal(t, "/v1/videos", requests[0].Path)
	assert.Equal(t, "Bearer mock-cangyuan-key", requests[0].Authorization)
	assert.JSONEq(t, `{"model":"seedance-2.0-720p","prompt":"cangyuan text acceptance","aspect_ratio":"16:9","duration":8,"resolution":"720p"}`, string(requests[0].Body))
	assert.NotContains(t, string(requests[0].Body), `"ratio"`)

	task := pollNewAPIVideoTask(t, publicID)
	assert.Equal(t, model.TaskStatus(model.TaskStatusInProgress), task.Status)
	task = pollNewAPIVideoTask(t, publicID)
	assert.Equal(t, model.TaskStatus(model.TaskStatusSuccess), task.Status)
	assert.Equal(t, cangyuanE2EUpstreamTaskID, task.PrivateData.UpstreamTaskID)
	assert.Equal(t, cangyuanE2EVideoURL, task.PrivateData.ResultURL)

	status, single := performJSONRequest(t, env.engine, http.MethodGet, "/api/v3/contents/generations/tasks/"+publicID, "Bearer e2e-1", "")
	require.Equal(t, http.StatusOK, status, string(single))
	assertCangyuanE2ESucceededTask(t, single, publicID)

	status, list := performJSONRequest(t, env.engine, http.MethodGet, "/api/v3/contents/generations/tasks?filter.task_ids="+publicID+"&page_size=20", "Bearer e2e-1", "")
	require.Equal(t, http.StatusOK, status, string(list))
	assertCangyuanE2EPublicBody(t, list)
	assert.Contains(t, string(list), publicID)
}

func TestCangyuanRejectsReferenceMediaBeforeUpstreamAndPreConsumeE2E(t *testing.T) {
	env := setupCangyuanE2E(t)
	requestBody := `{"model":"doubao-seedance-2-0-260128","content":[{"type":"text","text":"media is unsupported"},{"type":"image_url","image_url":{"url":"https://8.8.8.8/ref.jpg"}}],"duration":8}`

	var beforeTasks int64
	var beforeUser model.User
	require.NoError(t, model.DB.Model(&model.Task{}).Count(&beforeTasks).Error)
	require.NoError(t, model.DB.First(&beforeUser, e2eUserID).Error)
	status, body := performJSONRequest(t, env.engine, http.MethodPost, "/api/v3/contents/generations/tasks", "Bearer e2e-1", requestBody)
	assert.Equal(t, http.StatusBadRequest, status, string(body))
	assert.Contains(t, string(body), `"code":"InvalidParameter.content"`)
	assert.Empty(t, env.mock.snapshot())
	var afterTasks int64
	var afterUser model.User
	require.NoError(t, model.DB.Model(&model.Task{}).Count(&afterTasks).Error)
	require.NoError(t, model.DB.First(&afterUser, e2eUserID).Error)
	assert.Equal(t, beforeTasks, afterTasks)
	assert.Equal(t, beforeUser.Quota, afterUser.Quota)
	assert.Equal(t, beforeUser.UsedQuota, afterUser.UsedQuota)
}

func TestCangyuanFailedTaskRefundsE2E(t *testing.T) {
	env := setupCangyuanE2E(t, `{"task_id":"cangyuan-private","status":"failed","error":{"code":"provider_error","message":"generation failed"}}`)
	var beforeUser model.User
	var beforeToken model.Token
	var beforeChannel model.Channel
	require.NoError(t, model.DB.First(&beforeUser, e2eUserID).Error)
	require.NoError(t, model.DB.First(&beforeToken, 1).Error)
	require.NoError(t, model.DB.First(&beforeChannel, e2eChannelID).Error)

	requestBody := `{"model":"doubao-seedance-2-0-260128","content":[{"type":"text","text":"refund failure"}],"duration":8}`
	status, submit := performJSONRequest(t, env.engine, http.MethodPost, "/api/v3/contents/generations/tasks", "Bearer e2e-1", requestBody)
	require.Equal(t, http.StatusOK, status, string(submit))
	var submitResponse map[string]any
	require.NoError(t, common.Unmarshal(submit, &submitResponse))
	publicID, ok := submitResponse["id"].(string)
	require.True(t, ok)

	var task model.Task
	require.NoError(t, model.DB.Where("task_id = ?", publicID).First(&task).Error)
	preConsumedQuota := task.Quota
	require.Positive(t, preConsumedQuota)

	service.RunTaskPollingOnce(context.Background(), nil)
	status, failed := performJSONRequest(t, env.engine, http.MethodGet, "/api/v3/contents/generations/tasks/"+publicID, "Bearer e2e-1", "")
	require.Equal(t, http.StatusOK, status, string(failed))
	assertCangyuanE2EPublicBody(t, failed)
	assert.Contains(t, string(failed), `"status":"failed"`)
	assert.Contains(t, string(failed), `"code":"provider_error"`)

	require.NoError(t, model.DB.Where("task_id = ?", publicID).First(&task).Error)
	assert.Equal(t, model.TaskStatus(model.TaskStatusFailure), task.Status)
	assert.Zero(t, task.Quota)
	var afterUser model.User
	var afterToken model.Token
	var afterChannel model.Channel
	require.NoError(t, model.DB.First(&afterUser, e2eUserID).Error)
	require.NoError(t, model.DB.First(&afterToken, 1).Error)
	require.NoError(t, model.DB.First(&afterChannel, e2eChannelID).Error)
	assert.Equal(t, beforeUser.Quota, afterUser.Quota)
	assert.Equal(t, beforeUser.UsedQuota, afterUser.UsedQuota)
	assert.Equal(t, beforeToken.UsedQuota, afterToken.UsedQuota)
	assert.Equal(t, beforeChannel.UsedQuota, afterChannel.UsedQuota)
	var refundLog model.Log
	require.NoError(t, model.LOG_DB.Where("type = ?", model.LogTypeRefund).Order("id DESC").First(&refundLog).Error)
	assert.Equal(t, preConsumedQuota, refundLog.Quota)
}

func assertCangyuanE2ESucceededTask(t *testing.T, body []byte, publicID string) {
	t.Helper()
	assertCangyuanE2EPublicBody(t, body)
	var response map[string]any
	require.NoError(t, common.Unmarshal(body, &response))
	assert.Equal(t, publicID, response["id"])
	assert.Equal(t, "doubao-seedance-2-0-260128", response["model"])
	assert.Equal(t, "succeeded", response["status"])
	assert.Equal(t, map[string]any{"video_url": cangyuanE2EVideoURL}, response["content"])
}

func assertCangyuanE2EPublicBody(t *testing.T, body []byte) {
	t.Helper()
	for _, privateValue := range []string{
		cangyuanE2EUpstreamTaskID, "seedance-2.0-720p", "mock-cangyuan-key",
		"user_id", "channel_id", `"group"`, `"quota"`, `"platform"`, `"properties"`, "upstream_model_name",
	} {
		assert.NotContains(t, string(body), privateValue)
	}
}
