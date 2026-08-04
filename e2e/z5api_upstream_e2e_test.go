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
	z5apiE2EClientModel   = "sd-2-c6-imported"
	z5apiE2EUpstreamModel = "z5-c6"
	z5apiE2ETaskID        = "z5api-private-task"
	z5apiE2EVideoURL      = "https://assets.example/z5api.mp4"
)

type z5apiE2EMock struct {
	mu             sync.Mutex
	requests       []mockArkRequest
	pollResponses  []string
	pollIndex      int
	submitStatus   int
	submitResponse string
}

func (m *z5apiE2EMock) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	body, _ := io.ReadAll(request.Body)
	m.mu.Lock()
	m.requests = append(m.requests, mockArkRequest{
		Method: request.Method, Path: request.URL.Path,
		Authorization: request.Header.Get("Authorization"), Body: append([]byte(nil), body...),
	})
	response := ""
	switch {
	case request.Method == http.MethodPost && request.URL.Path == "/v1/videos":
		if m.submitStatus != 0 {
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(m.submitStatus)
			_, _ = writer.Write([]byte(m.submitResponse))
			m.mu.Unlock()
			return
		}
		response = `{"task_id":"` + z5apiE2ETaskID + `","status":"queued"}`
	case request.Method == http.MethodGet && request.URL.Path == "/v1/videos/"+z5apiE2ETaskID:
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

func (m *z5apiE2EMock) submitCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	count := 0
	for _, request := range m.requests {
		if request.Method == http.MethodPost && request.Path == "/v1/videos" {
			count++
		}
	}
	return count
}

func (m *z5apiE2EMock) snapshot() []mockArkRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	requests := make([]mockArkRequest, len(m.requests))
	copy(requests, m.requests)
	return requests
}

type z5apiE2EEnvironment struct {
	engine *gin.Engine
	mock   *z5apiE2EMock
}

func setupZ5APIE2E(t *testing.T, pollResponses ...string) *z5apiE2EEnvironment {
	t.Helper()
	setupSeedanceE2EDB(t)
	setupSeedanceE2EVideoMetadata(t)
	mock := &z5apiE2EMock{pollResponses: pollResponses}
	server := httptest.NewServer(mock)
	t.Cleanup(server.Close)
	seedSeedanceE2EData(t, server.URL)

	channel, err := model.GetChannelById(e2eChannelID, true)
	require.NoError(t, err)
	mapping := `{"` + z5apiE2EClientModel + `":"` + z5apiE2EUpstreamModel + `"}`
	channel.Type = constant.ChannelTypeZ5API
	channel.Key = "mock-z5api-key"
	channel.Models = z5apiE2EClientModel
	channel.ModelMapping = &mapping
	require.NoError(t, channel.Update())

	ratio := ratio_setting.GetModelRatioCopy()
	ratio[z5apiE2EClientModel] = 0.1
	encoded, err := common.Marshal(ratio)
	require.NoError(t, err)
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(string(encoded)))

	service.GetTaskAdaptorFunc = func(platform constant.TaskPlatform) service.TaskPollingAdaptor {
		return relay.GetTaskAdaptor(platform)
	}
	t.Cleanup(func() { service.GetTaskAdaptorFunc = nil })
	return &z5apiE2EEnvironment{engine: seedanceE2ERouter(), mock: mock}
}

func TestZ5APIArkLifecycleE2E(t *testing.T) {
	env := setupZ5APIE2E(t,
		`{"task_id":"z5api-private-task","status":"processing","seconds":"8"}`,
		`{"task_id":"z5api-private-task","status":"completed","object":"https://assets.example/z5api.mp4","seconds":"8"}`,
	)
	requestBody := `{"model":"sd-2-c6-imported","content":[{"type":"text","text":"z5api multimodal acceptance"},{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.8/ref.png"}},{"type":"video_url","role":"reference_video","video_url":{"url":"https://8.8.4.4/ref.mp4"}},{"type":"audio_url","role":"reference_audio","audio_url":{"url":"https://9.9.9.9/ref.mp3"}}],"duration":8,"ratio":"16:9","resolution":"720p"}`

	status, submit := performJSONRequest(t, env.engine, http.MethodPost, "/api/v3/contents/generations/tasks", "Bearer e2e-1", requestBody)
	require.Equal(t, http.StatusOK, status, string(submit))
	var submitResponse map[string]any
	require.NoError(t, common.Unmarshal(submit, &submitResponse))
	publicID, ok := submitResponse["id"].(string)
	require.True(t, ok)
	assert.True(t, strings.HasPrefix(publicID, "task_"))
	assertZ5APIPublicBody(t, submit)

	requests := env.mock.snapshot()
	require.Len(t, requests, 1)
	assert.Equal(t, "/v1/videos", requests[0].Path)
	assert.Equal(t, "Bearer mock-z5api-key", requests[0].Authorization)
	assert.JSONEq(t, `{"model":"z5-c6","prompt":"z5api multimodal acceptance","media":[{"type":"reference_image","url":"https://8.8.8.8/ref.png"},{"type":"reference_video","url":"https://8.8.4.4/ref.mp4"},{"type":"reference_voice","url":"https://9.9.9.9/ref.mp3"}],"parameters":{"resolution":"720p","ratio":"16:9","duration":8}}`, string(requests[0].Body))

	task := pollNewAPIVideoTask(t, publicID)
	assert.Equal(t, model.TaskStatus(model.TaskStatusInProgress), task.Status)
	task = pollNewAPIVideoTask(t, publicID)
	assert.Equal(t, model.TaskStatus(model.TaskStatusSuccess), task.Status)
	assert.Equal(t, z5apiE2ETaskID, task.PrivateData.UpstreamTaskID)
	assert.Equal(t, z5apiE2EVideoURL, task.PrivateData.ResultURL)
	require.NotNil(t, task.PrivateData.BillingContext)
	assert.Positive(t, task.Quota)

	status, body := performJSONRequest(t, env.engine, http.MethodGet, "/api/v3/contents/generations/tasks/"+publicID, "Bearer e2e-1", "")
	require.Equal(t, http.StatusOK, status, string(body))
	assert.Contains(t, string(body), `"video_url":"`+z5apiE2EVideoURL+`"`)
	assertZ5APIPublicBody(t, body)
}

func TestZ5APIFailedTaskRefundsExactlyOnceE2E(t *testing.T) {
	env := setupZ5APIE2E(t, `{"task_id":"z5api-private-task","status":"failed","error":{"code":"provider_error","message":"generation failed"}}`)
	var beforeUser model.User
	require.NoError(t, model.DB.First(&beforeUser, e2eUserID).Error)
	status, submit := performJSONRequest(t, env.engine, http.MethodPost, "/api/v3/contents/generations/tasks", "Bearer e2e-1", `{"model":"sd-2-c6-imported","content":[{"type":"text","text":"refund"}],"duration":8}`)
	require.Equal(t, http.StatusOK, status, string(submit))
	var created struct {
		ID string `json:"id"`
	}
	require.NoError(t, common.Unmarshal(submit, &created))
	require.NotEmpty(t, created.ID)
	service.RunTaskPollingOnce(context.Background(), nil)
	service.RunTaskPollingOnce(context.Background(), nil)
	var task model.Task
	require.NoError(t, model.DB.Where("task_id = ?", created.ID).First(&task).Error)
	assert.Equal(t, model.TaskStatus(model.TaskStatusFailure), task.Status)
	assert.Zero(t, task.Quota)
	var afterUser model.User
	require.NoError(t, model.DB.First(&afterUser, e2eUserID).Error)
	assert.Equal(t, beforeUser.Quota, afterUser.Quota)
	var refundLogs []model.Log
	require.NoError(t, model.LOG_DB.Where("type = ?", model.LogTypeRefund).Find(&refundLogs).Error)
	assert.Len(t, refundLogs, 1)
}

func TestZ5APISubmitDoesNotRetryOnUpstreamError(t *testing.T) {
	for _, test := range []struct {
		name   string
		status int
		body   string
	}{
		{name: "rate limited", status: http.StatusTooManyRequests, body: `{"error":{"code":"rate_limited","message":"slow down"}}`},
		{name: "provider failure", status: http.StatusInternalServerError, body: `{"error":{"code":"provider_error","message":"upstream failure"}}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			env := setupZ5APIE2E(t)
			env.mock.submitStatus = test.status
			env.mock.submitResponse = test.body
			status, response := performJSONRequest(t, env.engine, http.MethodPost, "/api/v3/contents/generations/tasks", "Bearer e2e-1", `{"model":"sd-2-c6-imported","content":[{"type":"text","text":"upstream error"}],"duration":8}`)
			assert.Equal(t, test.status, status, string(response))
			assert.Equal(t, 1, env.mock.submitCount())
		})
	}
}

func TestZ5APIRejectsUnsupportedMediaBeforeUpstreamE2E(t *testing.T) {
	env := setupZ5APIE2E(t)
	status, response := performJSONRequest(t, env.engine, http.MethodPost, "/api/v3/contents/generations/tasks", "Bearer e2e-1", `{"model":"sd-2-c6-imported","content":[{"type":"text","text":"too many"},{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.8/1.png"}},{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.8/2.png"}},{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.8/3.png"}},{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.8/4.png"}},{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.8/5.png"}},{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.8/6.png"}},{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.8/7.png"}},{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.8/8.png"}},{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.8/9.png"}},{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.8/10.png"}}],"duration":8}`)
	assert.Equal(t, http.StatusBadRequest, status, string(response))
	assert.Empty(t, env.mock.snapshot())
}

func assertZ5APIPublicBody(t *testing.T, body []byte) {
	t.Helper()
	for _, privateValue := range []string{z5apiE2ETaskID, z5apiE2EUpstreamModel, "mock-z5api-key", "user_id", "channel_id", "upstream_model_name"} {
		assert.NotContains(t, string(body), privateValue)
	}
}
