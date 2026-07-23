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

const newAPIVideoPollingResponse = `{"code":"success","message":"","data":{"task_id":"upstream-task","status":"SUCCESS","result_url":"https://example.com/video.mp4","submit_time":1784728184,"start_time":1784728190,"finish_time":1784728390,"progress":"100%","user_id":59,"channel_id":14,"group":"secret","quota":2000000,"platform":"54","properties":{"origin_model_name":"client-video","upstream_model_name":"seedance-720p-token"},"data":{"content":{"video_url":"https://example.com/video.mp4"},"created_at":1784728184,"updated_at":1784728390,"draft":false,"duration":10,"execution_expires_after":172800,"framespersecond":24,"generate_audio":true,"id":"provider-secret","model":"doubao-seedance-2.0","priority":0,"ratio":"16:9","resolution":"720p","seed":92859,"service_tier":"default","status":"succeeded","usage":{"completion_tokens":216900,"total_tokens":216900},"future_field":{"keep":true}}}}`

type mockNewAPIVideoServer struct {
	mu       sync.Mutex
	requests []mockArkRequest
}

func (m *mockNewAPIVideoServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	m.mu.Lock()
	m.requests = append(m.requests, mockArkRequest{
		Method: r.Method, Path: r.URL.Path, Authorization: r.Header.Get("Authorization"), Body: append([]byte(nil), body...),
	})
	m.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/v1/video/generations":
		_, _ = w.Write([]byte(`{"id":"upstream-task","task_id":"upstream-task","object":"video","model":"seedance-720p-token","status":"queued","progress":0,"created_at":1784728184}`))
	case r.Method == http.MethodGet && r.URL.Path == "/v1/video/generations/upstream-task":
		_, _ = w.Write([]byte(newAPIVideoPollingResponse))
	default:
		http.NotFound(w, r)
	}
}

func (m *mockNewAPIVideoServer) snapshot() []mockArkRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	requests := make([]mockArkRequest, len(m.requests))
	copy(requests, m.requests)
	return requests
}

func setupNewAPIVideoLifecycle(t *testing.T) (*gin.Engine, *mockNewAPIVideoServer) {
	t.Helper()
	setupSeedanceE2EDB(t)
	mock := &mockNewAPIVideoServer{}
	server := httptest.NewServer(mock)
	t.Cleanup(server.Close)
	seedSeedanceE2EData(t, server.URL)

	channel, err := model.GetChannelById(e2eChannelID, true)
	require.NoError(t, err)
	mapping := `{"client-video":"seedance-720p-token"}`
	channel.Type = constant.ChannelTypeNewAPIVideo
	channel.Key = "mock-newapi-video-key"
	channel.Models = "client-video"
	channel.ModelMapping = &mapping
	require.NoError(t, channel.Update())

	ratios := ratio_setting.GetModelRatioCopy()
	ratios["client-video"] = 0.1
	encoded, err := common.Marshal(ratios)
	require.NoError(t, err)
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(string(encoded)))

	service.GetTaskAdaptorFunc = func(platform constant.TaskPlatform) service.TaskPollingAdaptor {
		return relay.GetTaskAdaptor(platform)
	}
	t.Cleanup(func() { service.GetTaskAdaptorFunc = nil })
	return seedanceE2ERouter(), mock
}

func pollNewAPIVideoTask(t *testing.T, publicID string) model.Task {
	t.Helper()
	var task model.Task
	require.NoError(t, model.DB.Where("task_id = ?", publicID).First(&task).Error)
	upstreamID := task.GetUpstreamTaskID()
	require.NoError(t, service.UpdateVideoTasks(context.Background(), task.Platform,
		map[int][]string{task.ChannelId: {upstreamID}},
		map[string]*model.Task{upstreamID: &task},
	))
	require.NoError(t, model.DB.Where("task_id = ?", publicID).First(&task).Error)
	return task
}

func assertNewAPIVideoLifecycleQueries(t *testing.T, engine http.Handler, publicID string) {
	t.Helper()
	status, openAI := performJSONRequest(t, engine, http.MethodGet, "/v1/video/generations/"+publicID, "Bearer e2e-1", "")
	require.Equal(t, http.StatusOK, status, string(openAI))
	assertNewAPIVideoE2EPublicBody(t, openAI)
	var openAIResponse map[string]interface{}
	require.NoError(t, common.Unmarshal(openAI, &openAIResponse))
	assert.Equal(t, publicID, openAIResponse["id"])
	assert.Equal(t, publicID, openAIResponse["task_id"])
	assert.Equal(t, "client-video", openAIResponse["model"])
	assert.Equal(t, "completed", openAIResponse["status"])
	assert.Equal(t, "https://example.com/video.mp4", openAIResponse["metadata"].(map[string]interface{})["url"])

	status, ark := performJSONRequest(t, engine, http.MethodGet, "/api/v3/contents/generations/tasks/"+publicID, "Bearer e2e-1", "")
	require.Equal(t, http.StatusOK, status, string(ark))
	assertNewAPIVideoE2EPublicBody(t, ark)
	var arkResponse map[string]interface{}
	require.NoError(t, common.Unmarshal(ark, &arkResponse))
	assert.Equal(t, publicID, arkResponse["id"])
	assert.Equal(t, "client-video", arkResponse["model"])
	assert.Equal(t, "succeeded", arkResponse["status"])
	assert.Equal(t, map[string]interface{}{"completion_tokens": float64(216900), "total_tokens": float64(216900)}, arkResponse["usage"])
}

func assertNewAPIVideoE2EPublicBody(t *testing.T, body []byte) {
	t.Helper()
	for _, privateValue := range []string{
		"upstream-task", "provider-secret", "doubao-seedance-2.0", "seedance-720p-token",
		"user_id", "channel_id", `"group"`, `"quota"`, `"platform"`, `"properties"`,
	} {
		assert.NotContains(t, string(body), privateValue)
	}
}

func TestNewAPIVideoOpenAILifecycleE2E(t *testing.T) {
	engine, mock := setupNewAPIVideoLifecycle(t)
	requestBody := `{"model":"client-video","prompt":"A cinematic rainy city","seconds":"10","watermark":false,"seed":0,"unknown":{"zero":0,"flag":false}}`
	status, submit := performJSONRequest(t, engine, http.MethodPost, "/v1/video/generations", "Bearer e2e-1", requestBody)
	require.Equal(t, http.StatusOK, status, string(submit))
	assert.NotContains(t, string(submit), "upstream-task")
	var submitResponse map[string]interface{}
	require.NoError(t, common.Unmarshal(submit, &submitResponse))
	publicID, ok := submitResponse["id"].(string)
	require.True(t, ok)
	assert.True(t, strings.HasPrefix(publicID, "task_"))
	assert.Equal(t, publicID, submitResponse["task_id"])
	assert.Equal(t, "client-video", submitResponse["model"])

	requests := mock.snapshot()
	require.Len(t, requests, 1)
	assert.Equal(t, http.MethodPost, requests[0].Method)
	assert.Equal(t, "/v1/video/generations", requests[0].Path)
	assert.Equal(t, "Bearer mock-newapi-video-key", requests[0].Authorization)
	assert.JSONEq(t, `{"model":"seedance-720p-token","prompt":"A cinematic rainy city","seconds":"10","watermark":false,"seed":0,"unknown":{"zero":0,"flag":false}}`, string(requests[0].Body))

	task := pollNewAPIVideoTask(t, publicID)
	assert.Equal(t, model.TaskStatus(model.TaskStatusSuccess), task.Status)
	assert.Equal(t, "upstream-task", task.PrivateData.UpstreamTaskID)
	assert.Equal(t, "https://example.com/video.mp4", task.PrivateData.ResultURL)
	assert.Contains(t, string(task.Data), `"future_field":{"keep":true}`)
	assert.Contains(t, string(task.Data), `"start_time":1784728190`)
	assert.Contains(t, string(task.Data), `"origin_model_name":"client-video"`)
	assert.Contains(t, string(task.Data), `"upstream_model_name":"seedance-720p-token"`)
	require.NotNil(t, task.PrivateData.BillingContext)
	assert.Equal(t, 216900, task.PrivateData.BillingContext.BillingTokens)

	requests = mock.snapshot()
	require.Len(t, requests, 2)
	assert.Equal(t, http.MethodGet, requests[1].Method)
	assert.Equal(t, "/v1/video/generations/upstream-task", requests[1].Path)
	assertNewAPIVideoLifecycleQueries(t, engine, publicID)
}

func TestNewAPIVideoARKLifecycleE2E(t *testing.T) {
	engine, mock := setupNewAPIVideoLifecycle(t)
	requestBody := `{"model":"client-video","content":[{"type":"text","text":"text"},{"type":"image_url","image_url":{"url":"https://x/a.png"},"role":"reference_image"},{"type":"image_url","image_url":{"url":"https://x/b.png"},"role":"reference_image"},{"type":"video_url","video_url":{"url":"https://x/a.mp4"},"role":"reference_video"},{"type":"audio_url","audio_url":{"url":"https://x/a.mp3"},"role":"reference_audio"}],"generate_audio":true,"duration":10}`
	status, submit := performJSONRequest(t, engine, http.MethodPost, "/api/v3/contents/generations/tasks", "Bearer e2e-1", requestBody)
	require.Equal(t, http.StatusOK, status, string(submit))
	assert.NotContains(t, string(submit), "upstream-task")
	var submitResponse map[string]interface{}
	require.NoError(t, common.Unmarshal(submit, &submitResponse))
	require.Len(t, submitResponse, 1)
	publicID, ok := submitResponse["id"].(string)
	require.True(t, ok)

	requests := mock.snapshot()
	require.Len(t, requests, 1)
	var upstream map[string]interface{}
	require.NoError(t, common.Unmarshal(requests[0].Body, &upstream))
	assert.Equal(t, "seedance-720p-token", upstream["model"])
	assert.Equal(t, "text", upstream["prompt"])
	assert.Equal(t, "10", upstream["seconds"])
	assert.Equal(t, true, upstream["generateAudio"])
	assert.NotContains(t, upstream, "duration")
	content, ok := upstream["content"].([]interface{})
	require.True(t, ok)
	require.Len(t, content, 5)
	assert.Equal(t, "text", content[0].(map[string]interface{})["type"])
	assert.Equal(t, "reference_image", content[1].(map[string]interface{})["role"])
	assert.Equal(t, "reference_image", content[2].(map[string]interface{})["role"])
	assert.Equal(t, "reference_video", content[3].(map[string]interface{})["role"])
	assert.Equal(t, "reference_audio", content[4].(map[string]interface{})["role"])

	task := pollNewAPIVideoTask(t, publicID)
	assert.Equal(t, model.TaskStatus(model.TaskStatusSuccess), task.Status)
	require.NotNil(t, task.PrivateData.BillingContext)
	assert.Equal(t, 216900, task.PrivateData.BillingContext.BillingTokens)
	assertNewAPIVideoLifecycleQueries(t, engine, publicID)

	invalid := `{"model":"client-video","content":[{"type":"image_url","image_url":{"url":"https://x/a.png"},"role":"first_frame"}]}`
	status, invalidResponse := performJSONRequest(t, engine, http.MethodPost, "/api/v3/contents/generations/tasks", "Bearer e2e-1", invalid)
	assert.Equal(t, http.StatusBadRequest, status, string(invalidResponse))
	assert.Contains(t, string(invalidResponse), "text")
	assert.Len(t, mock.snapshot(), 2)
}
