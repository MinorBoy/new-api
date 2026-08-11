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
	mikotoE2ESoraClientModel       = "client-mikoto-sora"
	mikotoE2ESoraUpstreamModel     = "sora-v3-pro"
	mikotoE2ESeedanceClientModel   = "doubao-seedance-2-0-260128"
	mikotoE2ESeedanceUpstreamModel = "seedance-2.0-720p"
	mikotoE2EKey                   = "mock-mikoto-key"
)

type mikotoE2EMock struct {
	mu             sync.Mutex
	requests       []mockArkRequest
	taskID         string
	pollResponses  []string
	pollIndex      int
	submitStatus   int
	submitResponse string
}

func (m *mikotoE2EMock) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	body, _ := io.ReadAll(request.Body)
	m.mu.Lock()
	m.requests = append(m.requests, mockArkRequest{
		Method: request.Method, Path: request.URL.Path,
		Authorization: request.Header.Get("Authorization"), Body: append([]byte(nil), body...),
	})

	status := http.StatusOK
	response := ""
	switch {
	case request.Method == http.MethodPost && request.URL.Path == "/v1/videos":
		if m.submitStatus != 0 {
			status = m.submitStatus
			response = m.submitResponse
		} else {
			response = `{"task_id":"` + m.taskID + `","status":"queued"}`
		}
	case request.Method == http.MethodGet && request.URL.Path == "/v1/videos/"+m.taskID:
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
	writer.WriteHeader(status)
	_, _ = writer.Write([]byte(response))
}

func (m *mikotoE2EMock) snapshot() []mockArkRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	requests := make([]mockArkRequest, len(m.requests))
	copy(requests, m.requests)
	return requests
}

func (m *mikotoE2EMock) submitCount() int {
	count := 0
	for _, request := range m.snapshot() {
		if request.Method == http.MethodPost && request.Path == "/v1/videos" {
			count++
		}
	}
	return count
}

func (m *mikotoE2EMock) setSubmitError(status int, response string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.submitStatus = status
	m.submitResponse = response
}

type mikotoE2EEnvironment struct {
	engine        *gin.Engine
	mock          *mikotoE2EMock
	clientModel   string
	upstreamModel string
}

func setupMikotoE2E(t *testing.T, clientModel, upstreamModel string, pollResponses ...string) *mikotoE2EEnvironment {
	t.Helper()
	setupSeedanceE2EDB(t)
	setupSeedanceE2EVideoMetadata(t)
	taskID := "mikoto-seedance-private-task"
	if upstreamModel == mikotoE2ESoraUpstreamModel {
		taskID = "mikoto-sora-private-task"
	}
	mock := &mikotoE2EMock{taskID: taskID, pollResponses: pollResponses}
	server := httptest.NewServer(mock)
	t.Cleanup(server.Close)
	seedSeedanceE2EData(t, server.URL)

	channel, err := model.GetChannelById(e2eChannelID, true)
	require.NoError(t, err)
	mapping := `{"` + clientModel + `":"` + upstreamModel + `"}`
	channel.Type = constant.ChannelTypeMikoto
	channel.Key = mikotoE2EKey
	channel.Name = "mikoto-e2e-mock"
	channel.Models = clientModel
	channel.ModelMapping = &mapping
	require.NoError(t, channel.Update())

	ratios := ratio_setting.GetModelRatioCopy()
	ratios[clientModel] = 0.1
	encoded, err := common.Marshal(ratios)
	require.NoError(t, err)
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(string(encoded)))

	service.GetTaskAdaptorFunc = func(platform constant.TaskPlatform) service.TaskPollingAdaptor {
		return relay.GetTaskAdaptor(platform)
	}
	t.Cleanup(func() { service.GetTaskAdaptorFunc = nil })
	return &mikotoE2EEnvironment{
		engine: seedanceE2ERouter(), mock: mock, clientModel: clientModel, upstreamModel: upstreamModel,
	}
}

func TestMikotoArkLifecycleProjectsBothDialectsE2E(t *testing.T) {
	tests := []struct {
		name          string
		clientModel   string
		upstreamModel string
		requestBody   string
		upstreamBody  string
		videoURL      string
		completedBody string
	}{
		{
			name:          "Sora",
			clientModel:   mikotoE2ESoraClientModel,
			upstreamModel: mikotoE2ESoraUpstreamModel,
			requestBody:   `{"model":"client-mikoto-sora","content":[{"type":"text","text":"transition between frames"},{"type":"image_url","role":"first_frame","image_url":{"url":"https://8.8.8.8/first.png"}},{"type":"image_url","role":"last_frame","image_url":{"url":"https://8.8.4.4/last.png"}}],"duration":8,"ratio":"16:9","resolution":"720p"}`,
			upstreamBody:  `{"model":"sora-v3-pro","prompt":"transition between frames","seconds":"8","aspect_ratio":"16:9","resolution":"720p","image_url":"https://8.8.8.8/first.png","reference_image_urls":["https://8.8.4.4/last.png"],"video_config":{"reference_mode":"start_end"}}`,
			videoURL:      "https://assets.example/mikoto-sora.mp4",
			completedBody: `{"task_id":"mikoto-sora-private-task","status":"completed","video_url":"https://assets.example/mikoto-sora.mp4"}`,
		},
		{
			name:          "Seedance",
			clientModel:   mikotoE2ESeedanceClientModel,
			upstreamModel: mikotoE2ESeedanceUpstreamModel,
			requestBody:   `{"model":"doubao-seedance-2-0-260128","content":[{"type":"text","text":"follow the references"},{"type":"image_url","role":"reference_image","image_url":{"url":"data:image/png;base64,AAAA"}},{"type":"video_url","role":"reference_video","video_url":{"url":"https://8.8.4.4/ref.mp4"}},{"type":"audio_url","role":"reference_audio","audio_url":{"url":"data:audio/mpeg;base64,AAAA"}}],"duration":8,"ratio":"9:16","resolution":"720p","generate_audio":false}`,
			upstreamBody:  `{"model":"seedance-2.0-720p","prompt":"follow the references","duration":8,"aspect_ratio":"9:16","images":["data:image/png;base64,AAAA"],"reference_mode":"media","referenceVideos":["https://8.8.4.4/ref.mp4"],"referenceAudios":["data:audio/mpeg;base64,AAAA"],"generate_audio":false}`,
			videoURL:      "https://assets.example/mikoto-seedance.mp4",
			completedBody: `{"task_id":"mikoto-seedance-private-task","status":"completed","content_url":"https://assets.example/mikoto-seedance.mp4"}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			taskID := "mikoto-seedance-private-task"
			if test.upstreamModel == mikotoE2ESoraUpstreamModel {
				taskID = "mikoto-sora-private-task"
			}
			env := setupMikotoE2E(t, test.clientModel, test.upstreamModel,
				`{"task_id":"`+taskID+`","status":"queued","progress":0}`,
				`{"task_id":"`+taskID+`","status":"processing","progress":50}`,
				test.completedBody,
			)
			var beforeUser model.User
			require.NoError(t, model.DB.First(&beforeUser, e2eUserID).Error)

			status, submit := performJSONRequest(t, env.engine, http.MethodPost, "/api/v3/contents/generations/tasks", "Bearer e2e-1", test.requestBody)
			require.Equal(t, http.StatusOK, status, string(submit))
			var submitted struct {
				ID string `json:"id"`
			}
			require.NoError(t, common.Unmarshal(submit, &submitted))
			require.True(t, strings.HasPrefix(submitted.ID, "task_"))
			assertMikotoPublicBody(t, submit, taskID, test.upstreamModel)

			requests := env.mock.snapshot()
			require.Len(t, requests, 1)
			assert.Equal(t, http.MethodPost, requests[0].Method)
			assert.Equal(t, "/v1/videos", requests[0].Path)
			assert.Equal(t, "Bearer "+mikotoE2EKey, requests[0].Authorization)
			assert.JSONEq(t, test.upstreamBody, string(requests[0].Body))

			task := pollNewAPIVideoTask(t, submitted.ID)
			assert.Equal(t, model.TaskStatus(model.TaskStatusQueued), task.Status)
			task = pollNewAPIVideoTask(t, submitted.ID)
			assert.Equal(t, model.TaskStatus(model.TaskStatusInProgress), task.Status)
			task = pollNewAPIVideoTask(t, submitted.ID)
			assert.Equal(t, model.TaskStatus(model.TaskStatusSuccess), task.Status)
			assert.Equal(t, taskID, task.PrivateData.UpstreamTaskID)
			assert.Equal(t, test.videoURL, task.PrivateData.ResultURL)
			require.Positive(t, task.Quota)

			var afterUser model.User
			require.NoError(t, model.DB.First(&afterUser, e2eUserID).Error)
			assert.Equal(t, beforeUser.Quota-task.Quota, afterUser.Quota)
			assert.Equal(t, beforeUser.UsedQuota+task.Quota, afterUser.UsedQuota)

			requests = env.mock.snapshot()
			require.Len(t, requests, 4)
			for _, request := range requests[1:] {
				assert.Equal(t, http.MethodGet, request.Method)
				assert.Equal(t, "/v1/videos/"+taskID, request.Path)
				assert.Equal(t, "Bearer "+mikotoE2EKey, request.Authorization)
			}

			status, single := performJSONRequest(t, env.engine, http.MethodGet, "/api/v3/contents/generations/tasks/"+submitted.ID, "Bearer e2e-1", "")
			require.Equal(t, http.StatusOK, status, string(single))
			assertMikotoSucceededTask(t, single, submitted.ID, test.clientModel, test.videoURL, taskID, test.upstreamModel)

			status, list := performJSONRequest(t, env.engine, http.MethodGet, "/api/v3/contents/generations/tasks?filter.task_ids="+submitted.ID+"&page_size=20", "Bearer e2e-1", "")
			require.Equal(t, http.StatusOK, status, string(list))
			assertMikotoPublicBody(t, list, taskID, test.upstreamModel)
			assert.Contains(t, string(list), submitted.ID)
			assert.Contains(t, string(list), test.clientModel)

			status, otherList := performJSONRequest(t, env.engine, http.MethodGet, "/api/v3/contents/generations/tasks?filter.task_ids="+submitted.ID+"&page_size=20", "Bearer other", "")
			require.Equal(t, http.StatusOK, status, string(otherList))
			assert.NotContains(t, string(otherList), submitted.ID)
		})
	}
}

func TestMikotoFailedTaskRefundsExactlyOnceE2E(t *testing.T) {
	env := setupMikotoE2E(t, mikotoE2ESoraClientModel, mikotoE2ESoraUpstreamModel,
		`{"task_id":"mikoto-sora-private-task","status":"failed","error":{"code":"provider_error","message":"generation failed for mikoto-sora-private-task"}}`,
	)
	var beforeUser model.User
	require.NoError(t, model.DB.First(&beforeUser, e2eUserID).Error)
	status, submit := performJSONRequest(t, env.engine, http.MethodPost, "/api/v3/contents/generations/tasks", "Bearer e2e-1", `{"model":"client-mikoto-sora","content":[{"type":"text","text":"refund"}],"duration":8,"ratio":"16:9","resolution":"720p"}`)
	require.Equal(t, http.StatusOK, status, string(submit))
	var submitted struct {
		ID string `json:"id"`
	}
	require.NoError(t, common.Unmarshal(submit, &submitted))

	var task model.Task
	require.NoError(t, model.DB.Where("task_id = ?", submitted.ID).First(&task).Error)
	preConsumedQuota := task.Quota
	require.Positive(t, preConsumedQuota)
	service.RunTaskPollingOnce(context.Background(), nil)
	service.RunTaskPollingOnce(context.Background(), nil)
	require.NoError(t, model.DB.Where("task_id = ?", submitted.ID).First(&task).Error)
	assert.Equal(t, model.TaskStatus(model.TaskStatusFailure), task.Status)
	assert.Zero(t, task.Quota)

	var afterUser model.User
	require.NoError(t, model.DB.First(&afterUser, e2eUserID).Error)
	assert.Equal(t, beforeUser.Quota, afterUser.Quota)
	assert.Equal(t, beforeUser.UsedQuota, afterUser.UsedQuota)
	var refundLogs []model.Log
	require.NoError(t, model.LOG_DB.Where("type = ?", model.LogTypeRefund).Find(&refundLogs).Error)
	require.Len(t, refundLogs, 1)
	assert.Equal(t, preConsumedQuota, refundLogs[0].Quota)

	status, single := performJSONRequest(t, env.engine, http.MethodGet, "/api/v3/contents/generations/tasks/"+submitted.ID, "Bearer e2e-1", "")
	require.Equal(t, http.StatusOK, status, string(single))
	assertMikotoPublicBody(t, single, env.mock.taskID, env.upstreamModel)
	status, list := performJSONRequest(t, env.engine, http.MethodGet, "/api/v3/contents/generations/tasks?filter.task_ids="+submitted.ID+"&page_size=20", "Bearer e2e-1", "")
	require.Equal(t, http.StatusOK, status, string(list))
	assertMikotoPublicBody(t, list, env.mock.taskID, env.upstreamModel)
}

func TestMikotoRejectsInvalidRequestsBeforeUpstreamE2E(t *testing.T) {
	images := strings.Repeat(`,{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.8/ref.png"}}`, 10)
	tests := []struct {
		name          string
		clientModel   string
		upstreamModel string
		body          string
	}{
		{
			name: "duration above documented maximum", clientModel: mikotoE2ESeedanceClientModel, upstreamModel: mikotoE2ESeedanceUpstreamModel,
			body: `{"model":"doubao-seedance-2-0-260128","content":[{"type":"text","text":"too long"}],"duration":16,"resolution":"720p"}`,
		},
		{
			name: "ten images", clientModel: mikotoE2ESeedanceClientModel, upstreamModel: mikotoE2ESeedanceUpstreamModel,
			body: `{"model":"doubao-seedance-2-0-260128","content":[{"type":"text","text":"too many"}` + images + `],"duration":8,"resolution":"720p"}`,
		},
		{
			name: "Sora data URI", clientModel: mikotoE2ESoraClientModel, upstreamModel: mikotoE2ESoraUpstreamModel,
			body: `{"model":"client-mikoto-sora","content":[{"type":"text","text":"data URI"},{"type":"image_url","role":"reference_image","image_url":{"url":"data:image/png;base64,AAAA"}}],"duration":8,"resolution":"720p"}`,
		},
		{
			name: "unverified upstream model", clientModel: mikotoE2ESeedanceClientModel, upstreamModel: "mikoto-unverified-model",
			body: `{"model":"doubao-seedance-2-0-260128","content":[{"type":"text","text":"unknown"}],"duration":8,"resolution":"720p"}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			env := setupMikotoE2E(t, test.clientModel, test.upstreamModel)
			var beforeTasks int64
			var beforeUser model.User
			require.NoError(t, model.DB.Model(&model.Task{}).Count(&beforeTasks).Error)
			require.NoError(t, model.DB.First(&beforeUser, e2eUserID).Error)

			status, response := performJSONRequest(t, env.engine, http.MethodPost, "/api/v3/contents/generations/tasks", "Bearer e2e-1", test.body)
			assert.Equal(t, http.StatusBadRequest, status, string(response))
			assert.Zero(t, env.mock.submitCount())

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

func TestMikotoSubmitMapsUpstreamErrorsWithoutRetryE2E(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{name: "unauthorized", status: http.StatusUnauthorized, body: `{"error":{"code":"unauthorized","message":"invalid credential"}}`},
		{name: "rate limited", status: http.StatusTooManyRequests, body: `{"error":{"code":"rate_limited","message":"slow down"}}`},
		{name: "provider failure", status: http.StatusInternalServerError, body: `{"error":{"code":"provider_error","message":"upstream failure"}}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			env := setupMikotoE2E(t, mikotoE2ESoraClientModel, mikotoE2ESoraUpstreamModel)
			env.mock.setSubmitError(test.status, test.body)
			status, response := performJSONRequest(t, env.engine, http.MethodPost, "/api/v3/contents/generations/tasks", "Bearer e2e-1", `{"model":"client-mikoto-sora","content":[{"type":"text","text":"upstream error"}],"duration":8,"ratio":"16:9","resolution":"720p"}`)
			assert.Equal(t, test.status, status, string(response))
			assert.Contains(t, string(response), `"error"`)
			assert.NotContains(t, string(response), mikotoE2EKey)
			assert.NotContains(t, string(response), env.mock.taskID)
			assert.Equal(t, 1, env.mock.submitCount())
		})
	}
}

func assertMikotoSucceededTask(t *testing.T, body []byte, publicID, clientModel, videoURL, privateID, upstreamModel string) {
	t.Helper()
	assertMikotoPublicBody(t, body, privateID, upstreamModel)
	var response map[string]any
	require.NoError(t, common.Unmarshal(body, &response))
	assert.Equal(t, publicID, response["id"])
	assert.Equal(t, clientModel, response["model"])
	assert.Equal(t, "succeeded", response["status"])
	assert.Equal(t, map[string]any{"video_url": videoURL}, response["content"])
}

func assertMikotoPublicBody(t *testing.T, body []byte, privateID, upstreamModel string) {
	t.Helper()
	for _, privateValue := range []string{
		privateID, upstreamModel, mikotoE2EKey, "user_id", "channel_id", `"group"`, `"quota"`, `"platform"`,
		`"properties"`, "upstream_model_name",
	} {
		assert.NotContains(t, string(body), privateValue)
	}
}
