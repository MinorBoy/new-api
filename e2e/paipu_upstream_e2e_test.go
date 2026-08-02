package e2e

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/videometa"
	"github.com/QuantumNous/new-api/relay"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	paipuE2EUpstreamTaskID = "paipu-private"
	paipuE2EVideoURL       = "https://assets.example/paipu.mp4"
)

type paipuE2EMock struct {
	mu             sync.Mutex
	requests       []mockArkRequest
	pollResponses  []string
	pollIndex      int
	submitStatus   int
	submitResponse string
}

func (m *paipuE2EMock) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
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
		response = `{"task_id":"` + paipuE2EUpstreamTaskID + `","status":"queued"}`
	case request.Method == http.MethodGet && request.URL.Path == "/v1/videos/"+paipuE2EUpstreamTaskID:
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

func (m *paipuE2EMock) submitCount() int {
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

func (m *paipuE2EMock) pollRequests() []mockArkRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	var polls []mockArkRequest
	for _, request := range m.requests {
		if request.Method == http.MethodGet {
			polls = append(polls, request)
		}
	}
	return polls
}

func (m *paipuE2EMock) snapshot() []mockArkRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	requests := make([]mockArkRequest, len(m.requests))
	copy(requests, m.requests)
	return requests
}

type paipuE2EEnvironment struct {
	engine *gin.Engine
	mock   *paipuE2EMock
}

type paipuE2EVideoMetadataClient struct {
	durationMS int64
}

func (c paipuE2EVideoMetadataClient) Metadata(context.Context, string) (videometa.Metadata, error) {
	return videometa.Metadata{DurationMS: c.durationMS}, nil
}

func setupPaipuE2E(t *testing.T, pollResponses ...string) *paipuE2EEnvironment {
	t.Helper()
	setupSeedanceE2EDB(t)
	setupSeedanceE2EVideoMetadata(t)
	mock := &paipuE2EMock{pollResponses: pollResponses}
	server := httptest.NewServer(mock)
	t.Cleanup(server.Close)
	seedSeedanceE2EData(t, server.URL)

	channel, err := model.GetChannelById(e2eChannelID, true)
	require.NoError(t, err)
	mapping := `{"doubao-seedance-2-0-260128":"imported-paipu-model"}`
	channel.Type = constant.ChannelTypePaipu
	channel.Key = "mock-paipu-key"
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
	return &paipuE2EEnvironment{engine: seedanceE2ERouter(), mock: mock}
}

func setupPaipuE2ENoRetry(t *testing.T, submitStatus int, submitResponse string) *paipuE2EEnvironment {
	t.Helper()
	env := setupPaipuE2E(t)
	env.mock.submitStatus = submitStatus
	env.mock.submitResponse = submitResponse
	previousRetryTimes := common.RetryTimes
	common.RetryTimes = 3
	t.Cleanup(func() { common.RetryTimes = previousRetryTimes })
	return env
}

func TestPaipuARKMultimodalLifecycleE2E(t *testing.T) {
	env := setupPaipuE2E(t,
		`{"task_id":"paipu-private","status":"in_progress","progress":50}`,
		`{"task_id":"paipu-private","status":"completed","data":[{"url":"https://assets.example/paipu.mp4"}]}`,
	)
	requestBody := `{"model":"doubao-seedance-2-0-260128","content":[{"type":"text","text":"paipu multimodal acceptance"},{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.8/ref.png"}},{"type":"video_url","role":"reference_video","video_url":{"url":"https://8.8.4.4/ref.mp4"}},{"type":"audio_url","role":"reference_audio","audio_url":{"url":"data:audio/wav;base64,UklGRg=="}}],"duration":8,"ratio":"16:9","resolution":"720p"}`

	status, submit := performJSONRequest(t, env.engine, http.MethodPost, "/api/v3/contents/generations/tasks", "Bearer e2e-1", requestBody)
	require.Equal(t, http.StatusOK, status, string(submit))
	var submitResponse map[string]any
	require.NoError(t, common.Unmarshal(submit, &submitResponse))
	publicID, ok := submitResponse["id"].(string)
	require.True(t, ok)
	assert.True(t, strings.HasPrefix(publicID, "task_"))
	assertPaipuE2EPublicBody(t, submit)

	requests := env.mock.snapshot()
	require.Len(t, requests, 1)
	assert.Equal(t, http.MethodPost, requests[0].Method)
	assert.Equal(t, "/v1/videos", requests[0].Path)
	assert.Equal(t, "Bearer mock-paipu-key", requests[0].Authorization)
	assert.JSONEq(t, `{
		"model":"imported-paipu-model","prompt":"paipu multimodal acceptance",
		"duration":8,"aspect_ratio":"16:9","resolution":"720p",
		"images":["https://8.8.8.8/ref.png"],
		"videos":["https://8.8.4.4/ref.mp4"],
		"audios":["data:audio/wav;base64,UklGRg=="]
	}`, string(requests[0].Body))
	assert.NotContains(t, string(requests[0].Body), `"ratio"`)
	assert.NotContains(t, string(requests[0].Body), `"content"`)

	task := pollNewAPIVideoTask(t, publicID)
	assert.Equal(t, model.TaskStatus(model.TaskStatusInProgress), task.Status)
	task = pollNewAPIVideoTask(t, publicID)
	assert.Equal(t, model.TaskStatus(model.TaskStatusSuccess), task.Status)
	assert.Equal(t, paipuE2EUpstreamTaskID, task.PrivateData.UpstreamTaskID)
	assert.Equal(t, paipuE2EVideoURL, task.PrivateData.ResultURL)

	status, single := performJSONRequest(t, env.engine, http.MethodGet, "/api/v3/contents/generations/tasks/"+publicID, "Bearer e2e-1", "")
	require.Equal(t, http.StatusOK, status, string(single))
	assertPaipuE2ESucceededTask(t, single, publicID)

	status, list := performJSONRequest(t, env.engine, http.MethodGet, "/api/v3/contents/generations/tasks?filter.task_ids="+publicID+"&page_size=20", "Bearer e2e-1", "")
	require.Equal(t, http.StatusOK, status, string(list))
	assertPaipuE2EPublicBody(t, list)
	assert.Contains(t, string(list), publicID)
}

func TestPaipuMissingUsageUsesValidatedResolutionE2E(t *testing.T) {
	tests := []struct {
		resolution string
		wantTokens int
	}{
		{resolution: "1080p", wantTokens: 243000},
		{resolution: "4k", wantTokens: 972000},
	}
	for _, tt := range tests {
		t.Run(tt.resolution, func(t *testing.T) {
			env := setupPaipuE2E(t, `{"task_id":"paipu-private","status":"completed","data":[{"url":"https://assets.example/paipu.mp4"}]}`)
			requestBody := fmt.Sprintf(`{"model":"doubao-seedance-2-0-260128","content":[{"type":"text","text":"validated resolution fallback"}],"duration":5,"resolution":%q}`, tt.resolution)

			status, submit := performJSONRequest(t, env.engine, http.MethodPost, "/api/v3/contents/generations/tasks", "Bearer e2e-1", requestBody)
			require.Equal(t, http.StatusOK, status, string(submit))
			var submitted struct {
				ID string `json:"id"`
			}
			require.NoError(t, common.Unmarshal(submit, &submitted))
			require.NotEmpty(t, submitted.ID)

			var task model.Task
			require.NoError(t, model.DB.Where("task_id = ?", submitted.ID).First(&task).Error)
			require.NotNil(t, task.PrivateData.BillingContext)
			assert.Equal(t, model.TaskUsageSnapshotVersion1, task.PrivateData.BillingContext.UsageSnapshotVersion)
			assert.Equal(t, tt.wantTokens, task.PrivateData.BillingContext.UsageCompletionTokens)
			assert.Equal(t, tt.wantTokens, task.PrivateData.BillingContext.UsageTotalTokens)

			task = pollNewAPIVideoTask(t, submitted.ID)
			require.Equal(t, model.TaskStatus(model.TaskStatusSuccess), task.Status)
			require.NotNil(t, task.PrivateData.BillingContext)
			assert.Equal(t, tt.resolution, task.PrivateData.BillingContext.Resolution)
			assert.Equal(t, tt.wantTokens, task.PrivateData.BillingContext.BillingTokens)
			assert.Equal(t, tt.wantTokens, task.PrivateData.BillingContext.UsageCompletionTokens)
			assert.Equal(t, tt.wantTokens, task.PrivateData.BillingContext.UsageTotalTokens)

			status, body := performJSONRequest(t, env.engine, http.MethodGet, "/api/v3/contents/generations/tasks/"+submitted.ID, "Bearer e2e-1", "")
			require.Equal(t, http.StatusOK, status, string(body))
			var response struct {
				Usage struct {
					CompletionTokens int `json:"completion_tokens"`
					TotalTokens      int `json:"total_tokens"`
				} `json:"usage"`
			}
			require.NoError(t, common.Unmarshal(body, &response))
			assert.Equal(t, task.PrivateData.BillingContext.UsageCompletionTokens, response.Usage.CompletionTokens)
			assert.Equal(t, task.PrivateData.BillingContext.UsageTotalTokens, response.Usage.TotalTokens)
		})
	}
}

func TestPaipuRejectsOverlongReferenceVideoBeforeUpstreamE2E(t *testing.T) {
	env := setupPaipuE2E(t)
	service.SetVideoMetadataClient(paipuE2EVideoMetadataClient{
		durationMS: int64(relaycommon.MaxTaskDurationSeconds)*1000 + 1,
	})
	requestBody := `{"model":"doubao-seedance-2-0-260128","content":[{"type":"text","text":"overlong reference"},{"type":"video_url","role":"reference_video","video_url":{"url":"https://8.8.8.8/overlong.mp4"}}],"duration":5,"resolution":"720p"}`

	status, body := performJSONRequest(t, env.engine, http.MethodPost, "/api/v3/contents/generations/tasks", "Bearer e2e-1", requestBody)

	assert.Equal(t, http.StatusBadRequest, status, string(body))
	assert.Contains(t, string(body), `"code":"invalid_reference_video"`)
	assert.Empty(t, env.mock.snapshot())
}

func TestPaipuRejectsProtocolViolationsBeforeUpstreamAndPreConsumeE2E(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "ten images", body: paipuTenImageE2EBody()},
		{name: "first frame", body: `{"model":"doubao-seedance-2-0-260128","content":[{"type":"text","text":"t"},{"type":"image_url","role":"first_frame","image_url":{"url":"https://8.8.8.8/f.png"}}],"duration":8}`},
		{name: "private url", body: `{"model":"doubao-seedance-2-0-260128","content":[{"type":"text","text":"t"},{"type":"image_url","role":"reference_image","image_url":{"url":"http://192.168.1.1/ref.png"}}],"duration":8}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := setupPaipuE2E(t)
			var beforeTasks int64
			var beforeUser model.User
			require.NoError(t, model.DB.Model(&model.Task{}).Count(&beforeTasks).Error)
			require.NoError(t, model.DB.First(&beforeUser, e2eUserID).Error)
			status, body := performJSONRequest(t, env.engine, http.MethodPost, "/api/v3/contents/generations/tasks", "Bearer e2e-1", tt.body)
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
		})
	}
}

func paipuTenImageE2EBody() string {
	items := `[{"type":"text","text":"too many images"}`
	for i := 0; i < 10; i++ {
		items += fmt.Sprintf(`,{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.%d/ref-%d.png"}}`, i%250+1, i)
	}
	items += "]"
	return fmt.Sprintf(`{"model":"doubao-seedance-2-0-260128","content":%s,"duration":8}`, items)
}

func TestPaipuFailedTaskRefundsE2E(t *testing.T) {
	env := setupPaipuE2E(t, `{"task_id":"paipu-private","status":"failed","error":{"code":"provider_error","message":"generation failed"}}`)
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
	assertPaipuE2EPublicBody(t, failed)
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

func TestPaipuSubmitDoesNotRetryOnUpstreamError(t *testing.T) {
	tests := []struct {
		name           string
		submitStatus   int
		submitResponse string
	}{
		{name: "429 too many requests", submitStatus: http.StatusTooManyRequests, submitResponse: `{"error":{"code":"rate_limited","message":"slow down"}}`},
		{name: "500 server error", submitStatus: http.StatusInternalServerError, submitResponse: `{"error":{"code":"provider_error","message":"upstream failure"}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := setupPaipuE2ENoRetry(t, tt.submitStatus, tt.submitResponse)
			requestBody := `{"model":"doubao-seedance-2-0-260128","content":[{"type":"text","text":"no retry submit"}],"duration":8}`
			status, body := performJSONRequest(t, env.engine, http.MethodPost, "/api/v3/contents/generations/tasks", "Bearer e2e-1", requestBody)
			assert.Equal(t, tt.submitStatus, status, string(body))
			assert.Equal(t, 1, env.mock.submitCount())
			assert.Empty(t, env.mock.pollRequests())
		})
	}
}

func assertPaipuE2ESucceededTask(t *testing.T, body []byte, publicID string) {
	t.Helper()
	assertPaipuE2EPublicBody(t, body)
	var response map[string]any
	require.NoError(t, common.Unmarshal(body, &response))
	assert.Equal(t, publicID, response["id"])
	assert.Equal(t, "doubao-seedance-2-0-260128", response["model"])
	assert.Equal(t, "succeeded", response["status"])
	assert.Equal(t, map[string]any{"video_url": paipuE2EVideoURL}, response["content"])
}

func assertPaipuE2EPublicBody(t *testing.T, body []byte) {
	t.Helper()
	for _, privateValue := range []string{
		paipuE2EUpstreamTaskID, "imported-paipu-model", "mock-paipu-key",
		"user_id", "channel_id", `"group"`, `"quota"`, `"platform"`, `"properties"`, "upstream_model_name",
	} {
		assert.NotContains(t, string(body), privateValue)
	}
}
