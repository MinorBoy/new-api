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
	omegaAIE2EClientModel          = "doubao-seedance-2-0-260128"
	omegaAIE2EImageOnlyClientModel = "doubao-seedance-2-0-fast-260128"
	omegaAIE2EUpstreamTaskID       = "omega-private-task"
	omegaAIE2EVideoURL             = "https://assets.example/omegaai.mp4"
)

type omegaAIE2EMock struct {
	mu            sync.Mutex
	requests      []mockArkRequest
	pollResponses []string
	pollIndex     int
}

func (m *omegaAIE2EMock) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	body, _ := io.ReadAll(request.Body)
	m.mu.Lock()
	m.requests = append(m.requests, mockArkRequest{
		Method: request.Method, Path: request.URL.Path,
		Authorization: request.Header.Get("Authorization"), Body: append([]byte(nil), body...),
	})
	response := ""
	switch {
	case request.Method == http.MethodPost && request.URL.Path == "/v1/media/generate":
		response = `{"task_id":"` + omegaAIE2EUpstreamTaskID + `","status":"queued"}`
	case request.Method == http.MethodGet && request.URL.Path == "/v1/tasks/"+omegaAIE2EUpstreamTaskID:
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

func (m *omegaAIE2EMock) snapshot() []mockArkRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	requests := make([]mockArkRequest, len(m.requests))
	copy(requests, m.requests)
	return requests
}

type omegaAIE2EEnvironment struct {
	engine *gin.Engine
	mock   *omegaAIE2EMock
}

func setupOmegaAIE2E(t *testing.T, pollResponses ...string) *omegaAIE2EEnvironment {
	t.Helper()
	setupSeedanceE2EDB(t)
	setupSeedanceE2EVideoMetadata(t)
	mock := &omegaAIE2EMock{pollResponses: pollResponses}
	server := httptest.NewServer(mock)
	t.Cleanup(server.Close)
	seedSeedanceE2EData(t, server.URL)

	channel, err := model.GetChannelById(e2eChannelID, true)
	require.NoError(t, err)
	mapping := `{"` + omegaAIE2EClientModel + `":"klsdpro2-720p","` + omegaAIE2EImageOnlyClientModel + `":"seedance-v2-720p"}`
	channel.Type = constant.ChannelTypeOmegaAI
	channel.Key = "mock-omegaai-key"
	channel.Models = omegaAIE2EClientModel + "," + omegaAIE2EImageOnlyClientModel
	channel.ModelMapping = &mapping
	require.NoError(t, channel.Update())

	ratios := ratio_setting.GetModelRatioCopy()
	ratios[omegaAIE2EClientModel] = 0.1
	ratios[omegaAIE2EImageOnlyClientModel] = 0.1
	encoded, err := common.Marshal(ratios)
	require.NoError(t, err)
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(string(encoded)))

	service.GetTaskAdaptorFunc = func(platform constant.TaskPlatform) service.TaskPollingAdaptor {
		return relay.GetTaskAdaptor(platform)
	}
	t.Cleanup(func() { service.GetTaskAdaptorFunc = nil })
	return &omegaAIE2EEnvironment{engine: seedanceE2ERouter(), mock: mock}
}

func TestOmegaAIARKLifecyclePreservesPublicContractE2E(t *testing.T) {
	env := setupOmegaAIE2E(t,
		`{"task_id":"omega-private-task","status":"running","progress":50}`,
		`{"task_id":"omega-private-task","status":"succeeded","progress":100,"content":{"video_url":"https://assets.example/omegaai.mp4"}}`,
	)
	var beforeUser model.User
	var beforeToken model.Token
	var beforeChannel model.Channel
	var beforeConsumeLogs int64
	require.NoError(t, model.DB.First(&beforeUser, e2eUserID).Error)
	require.NoError(t, model.DB.First(&beforeToken, 1).Error)
	require.NoError(t, model.DB.First(&beforeChannel, e2eChannelID).Error)
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).Where("type = ?", model.LogTypeConsume).Count(&beforeConsumeLogs).Error)
	requestBody := `{
		"model":"doubao-seedance-2-0-260128",
		"content":[
			{"type":"text","text":"OmegaAI multimodal acceptance"},
			{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.8/ref.jpg"}},
			{"type":"video_url","role":"reference_video","video_url":{"url":"https://1.1.1.1/ref.mp4"}},
			{"type":"audio_url","role":"reference_audio","audio_url":{"url":"https://9.9.9.9/ref.mp3"}}
		],
		"duration":8,
		"ratio":"16:9"
	}`

	status, submit := performJSONRequest(t, env.engine, http.MethodPost, "/api/v3/contents/generations/tasks", "Bearer e2e-1", requestBody)
	require.Equal(t, http.StatusOK, status, string(submit))
	var submitResponse map[string]any
	require.NoError(t, common.Unmarshal(submit, &submitResponse))
	require.Len(t, submitResponse, 1)
	publicID, ok := submitResponse["id"].(string)
	require.True(t, ok)
	assert.True(t, strings.HasPrefix(publicID, "task_"))
	assertOmegaAIE2EPublicBody(t, submit)

	requests := env.mock.snapshot()
	require.Len(t, requests, 1)
	assert.Equal(t, http.MethodPost, requests[0].Method)
	assert.Equal(t, "/v1/media/generate", requests[0].Path)
	assert.Equal(t, "Bearer mock-omegaai-key", requests[0].Authorization)
	assert.JSONEq(t, `{
		"model":"klsdpro2-720p",
		"prompt":"OmegaAI multimodal acceptance",
		"duration":8,
		"aspect_ratio":"16:9",
		"images":["https://8.8.8.8/ref.jpg"],
		"videos":["https://1.1.1.1/ref.mp4"],
		"audios":["https://9.9.9.9/ref.mp3"]
	}`, string(requests[0].Body))

	task := pollNewAPIVideoTask(t, publicID)
	assert.Equal(t, model.TaskStatus(model.TaskStatusInProgress), task.Status)
	task = pollNewAPIVideoTask(t, publicID)
	assert.Equal(t, model.TaskStatus(model.TaskStatusSuccess), task.Status)
	assert.Equal(t, omegaAIE2EUpstreamTaskID, task.PrivateData.UpstreamTaskID)
	assert.Equal(t, omegaAIE2EVideoURL, task.PrivateData.ResultURL)
	var afterSuccessUser model.User
	var afterSuccessToken model.Token
	var afterSuccessChannel model.Channel
	var afterSuccessConsumeLogs int64
	require.NoError(t, model.DB.First(&afterSuccessUser, e2eUserID).Error)
	require.NoError(t, model.DB.First(&afterSuccessToken, 1).Error)
	require.NoError(t, model.DB.First(&afterSuccessChannel, e2eChannelID).Error)
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).Where("type = ?", model.LogTypeConsume).Count(&afterSuccessConsumeLogs).Error)
	require.Positive(t, task.Quota)
	assert.Less(t, afterSuccessUser.Quota, beforeUser.Quota)
	assert.Equal(t, beforeUser.UsedQuota+task.Quota, afterSuccessUser.UsedQuota)
	assert.Equal(t, beforeToken.UsedQuota+task.Quota, afterSuccessToken.UsedQuota)
	assert.Equal(t, beforeChannel.UsedQuota+int64(task.Quota), afterSuccessChannel.UsedQuota)
	assert.Equal(t, beforeConsumeLogs+1, afterSuccessConsumeLogs)
	service.RunTaskPollingOnce(context.Background(), nil)
	var afterReplayUser model.User
	var afterReplayToken model.Token
	var afterReplayChannel model.Channel
	var afterReplayConsumeLogs int64
	require.NoError(t, model.DB.First(&afterReplayUser, e2eUserID).Error)
	require.NoError(t, model.DB.First(&afterReplayToken, 1).Error)
	require.NoError(t, model.DB.First(&afterReplayChannel, e2eChannelID).Error)
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).Where("type = ?", model.LogTypeConsume).Count(&afterReplayConsumeLogs).Error)
	assert.Equal(t, afterSuccessUser.Quota, afterReplayUser.Quota)
	assert.Equal(t, afterSuccessUser.UsedQuota, afterReplayUser.UsedQuota)
	assert.Equal(t, afterSuccessToken.UsedQuota, afterReplayToken.UsedQuota)
	assert.Equal(t, afterSuccessChannel.UsedQuota, afterReplayChannel.UsedQuota)
	assert.Equal(t, afterSuccessConsumeLogs, afterReplayConsumeLogs)

	requests = env.mock.snapshot()
	require.Len(t, requests, 3)
	for _, request := range requests[1:] {
		assert.Equal(t, http.MethodGet, request.Method)
		assert.Equal(t, "/v1/tasks/"+omegaAIE2EUpstreamTaskID, request.Path)
		assert.Equal(t, "Bearer mock-omegaai-key", request.Authorization)
	}

	status, single := performJSONRequest(t, env.engine, http.MethodGet, "/api/v3/contents/generations/tasks/"+publicID, "Bearer e2e-1", "")
	require.Equal(t, http.StatusOK, status, string(single))
	assertOmegaAIE2ESucceededTask(t, single, publicID)

	status, list := performJSONRequest(t, env.engine, http.MethodGet, "/api/v3/contents/generations/tasks?filter.task_ids="+publicID+"&page_size=20", "Bearer e2e-1", "")
	require.Equal(t, http.StatusOK, status, string(list))
	assertOmegaAIE2EPublicBody(t, list)
	assert.Contains(t, string(list), publicID)
}

func TestOmegaAIInvalidRequestsHaveNoSideEffectsE2E(t *testing.T) {
	images := strings.Repeat(`,{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.8/ref.jpg"}}`, 10)
	tests := []struct {
		name string
		body string
		code string
	}{
		{
			name: "video on image-only model",
			body: `{"model":"doubao-seedance-2-0-fast-260128","content":[{"type":"text","text":"text"},{"type":"video_url","role":"reference_video","video_url":{"url":"https://8.8.8.8/ref.mp4"}}]}`,
			code: "InvalidParameter.content",
		},
		{
			name: "frame role",
			body: `{"model":"doubao-seedance-2-0-260128","content":[{"type":"text","text":"text"},{"type":"image_url","role":"first_frame","image_url":{"url":"https://8.8.8.8/ref.jpg"}}]}`,
			code: "InvalidParameter.content",
		},
		{
			name: "ten images",
			body: `{"model":"doubao-seedance-2-0-260128","content":[{"type":"text","text":"text"}` + images + `]}`,
			code: "InvalidParameter.content",
		},
		{
			name: "duration overflow",
			body: `{"model":"doubao-seedance-2-0-260128","content":[{"type":"text","text":"text"}],"duration":86401}`,
			code: "InvalidParameter.duration",
		},
		{
			name: "generate audio",
			body: `{"model":"doubao-seedance-2-0-260128","content":[{"type":"text","text":"text"}],"generate_audio":false}`,
			code: "InvalidParameter.generate_audio",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			env := setupOmegaAIE2E(t)
			assertOmegaAIInvalidRequestDoesNotConsume(t, env, test.body, test.code)
		})
	}
}

func TestOmegaAIFailedTaskRefundsExactlyOnceE2E(t *testing.T) {
	env := setupOmegaAIE2E(t,
		`{"task_id":"omega-private-task","status":"failed","error":{"code":"provider_error","message":"generation failed"}}`,
	)
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
	service.RunTaskPollingOnce(context.Background(), nil)
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

	var refundLogs []model.Log
	require.NoError(t, model.LOG_DB.Where("type = ?", model.LogTypeRefund).Find(&refundLogs).Error)
	require.Len(t, refundLogs, 1)
	assert.Equal(t, preConsumedQuota, refundLogs[0].Quota)
}

func assertOmegaAIInvalidRequestDoesNotConsume(t *testing.T, env *omegaAIE2EEnvironment, requestBody, wantCode string) {
	t.Helper()
	var beforeTasks int64
	var beforeUser model.User
	require.NoError(t, model.DB.Model(&model.Task{}).Count(&beforeTasks).Error)
	require.NoError(t, model.DB.First(&beforeUser, e2eUserID).Error)

	status, body := performJSONRequest(t, env.engine, http.MethodPost, "/api/v3/contents/generations/tasks", "Bearer e2e-1", requestBody)
	assert.Equal(t, http.StatusBadRequest, status, string(body))
	assert.Contains(t, string(body), `"code":"`+wantCode+`"`)
	assert.Empty(t, env.mock.snapshot())

	var afterTasks int64
	var afterUser model.User
	require.NoError(t, model.DB.Model(&model.Task{}).Count(&afterTasks).Error)
	require.NoError(t, model.DB.First(&afterUser, e2eUserID).Error)
	assert.Equal(t, beforeTasks, afterTasks)
	assert.Equal(t, beforeUser.Quota, afterUser.Quota)
	assert.Equal(t, beforeUser.UsedQuota, afterUser.UsedQuota)
}

func assertOmegaAIE2ESucceededTask(t *testing.T, body []byte, publicID string) {
	t.Helper()
	assertOmegaAIE2EPublicBody(t, body)
	var response map[string]any
	require.NoError(t, common.Unmarshal(body, &response))
	assert.Equal(t, publicID, response["id"])
	assert.Equal(t, omegaAIE2EClientModel, response["model"])
	assert.Equal(t, "succeeded", response["status"])
	assert.Equal(t, map[string]any{"video_url": omegaAIE2EVideoURL}, response["content"])
}

func assertOmegaAIE2EPublicBody(t *testing.T, body []byte) {
	t.Helper()
	for _, privateValue := range []string{
		omegaAIE2EUpstreamTaskID, "klsdpro2-720p", "mock-omegaai-key",
		"user_id", "channel_id", `"group"`, `"quota"`, `"platform"`, `"properties"`, "upstream_model_name",
	} {
		assert.NotContains(t, string(body), privateValue)
	}
}
