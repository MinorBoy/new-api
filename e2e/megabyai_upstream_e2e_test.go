package e2e

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/videometa"
	"github.com/QuantumNous/new-api/relay"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	megaByAIE2EUpstreamTaskID = "videos-mini_private"
	megaByAIE2EVideoURL       = "https://assets.example/megabyai.mp4"
)

type megaByAIE2EMock struct {
	mu            sync.Mutex
	requests      []mockArkRequest
	pollResponses []string
	pollIndex     int
}

func (m *megaByAIE2EMock) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	body, _ := io.ReadAll(request.Body)
	m.mu.Lock()
	m.requests = append(m.requests, mockArkRequest{
		Method: request.Method, Path: request.URL.Path,
		Authorization: request.Header.Get("Authorization"), Body: append([]byte(nil), body...),
	})
	response := ""
	switch {
	case request.Method == http.MethodPost && request.URL.Path == "/v1/videos":
		response = `{"task_id":"` + megaByAIE2EUpstreamTaskID + `","status":"queued"}`
	case request.Method == http.MethodGet && request.URL.Path == "/v1/videos/"+megaByAIE2EUpstreamTaskID:
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

func (m *megaByAIE2EMock) snapshot() []mockArkRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	requests := make([]mockArkRequest, len(m.requests))
	copy(requests, m.requests)
	return requests
}

type megaByAIE2EVideoMetadataClient struct {
	durationMS  atomic.Int64
	unavailable atomic.Bool
}

func (c *megaByAIE2EVideoMetadataClient) Metadata(_ context.Context, _ string) (videometa.Metadata, error) {
	if c.unavailable.Load() {
		return videometa.Metadata{}, &service.VideoMetadataError{Kind: service.VideoMetadataUnavailable}
	}
	return videometa.Metadata{
		DurationMS: c.durationMS.Load(), Width: 1280, Height: 720,
		FrameRateNum: 24, FrameRateDen: 1, Container: "mp4", ContentLength: 1,
	}, nil
}

type megaByAIE2EAudioDurationResolver struct {
	durationMS  atomic.Int64
	unavailable atomic.Bool
}

func (r *megaByAIE2EAudioDurationResolver) ResolveMS(_ context.Context, _ []string) (int64, error) {
	if r.unavailable.Load() {
		return 0, &service.ReferenceAudioDurationError{Kind: service.ReferenceAudioMetadataUnavailable}
	}
	return r.durationMS.Load(), nil
}

type megaByAIE2EEnvironment struct {
	engine        *gin.Engine
	mock          *megaByAIE2EMock
	videoMetadata *megaByAIE2EVideoMetadataClient
	audioMetadata *megaByAIE2EAudioDurationResolver
}

func setupMegaByAIE2E(t *testing.T, pollResponses ...string) *megaByAIE2EEnvironment {
	t.Helper()
	setupSeedanceE2EDB(t)
	mock := &megaByAIE2EMock{pollResponses: pollResponses}
	server := httptest.NewServer(mock)
	t.Cleanup(server.Close)
	seedSeedanceE2EData(t, server.URL)

	channel, err := model.GetChannelById(e2eChannelID, true)
	require.NoError(t, err)
	mapping := `{"doubao-seedance-2-0-260128":"videos-mini"}`
	channel.Type = constant.ChannelTypeMegaByAI
	channel.Key = "mock-megabyai-key"
	channel.Models = "doubao-seedance-2-0-260128"
	channel.ModelMapping = &mapping
	require.NoError(t, channel.Update())

	ratios := ratio_setting.GetModelRatioCopy()
	ratios["doubao-seedance-2-0-260128"] = 0.1
	encoded, err := common.Marshal(ratios)
	require.NoError(t, err)
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(string(encoded)))

	videoMetadata := &megaByAIE2EVideoMetadataClient{}
	videoMetadata.durationMS.Store(15_000)
	audioMetadata := &megaByAIE2EAudioDurationResolver{}
	audioMetadata.durationMS.Store(15_000)
	service.SetVideoMetadataClient(videoMetadata)
	service.SetReferenceAudioDurationResolver(audioMetadata)
	service.GetTaskAdaptorFunc = func(platform constant.TaskPlatform) service.TaskPollingAdaptor {
		return relay.GetTaskAdaptor(platform)
	}
	t.Cleanup(func() {
		service.SetVideoMetadataClient(nil)
		service.SetReferenceAudioDurationResolver(nil)
		service.GetTaskAdaptorFunc = nil
	})
	return &megaByAIE2EEnvironment{
		engine: seedanceE2ERouter(), mock: mock,
		videoMetadata: videoMetadata, audioMetadata: audioMetadata,
	}
}

func TestMegaByAIARKLifecycleAndPreConsumeValidationE2E(t *testing.T) {
	env := setupMegaByAIE2E(t,
		`{"task_id":"videos-mini_private","status":"in_progress","progress":50}`,
		`{"task_id":"videos-mini_private","status":"completed","progress":100,"metadata":{"content_url":"https://assets.example/megabyai.mp4"}}`,
	)
	requestBody := `{
		"model":"doubao-seedance-2-0-260128","content":[
			{"type":"text","text":"multimodal MegaByAI acceptance"},
			{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.8/ref.jpg"}},
			{"type":"video_url","role":"reference_video","video_url":{"url":"https://8.8.8.8/ref.mp4"}},
			{"type":"audio_url","role":"reference_audio","audio_url":{"url":"https://8.8.8.8/ref.wav"}}
		],"duration":8,"ratio":"16:9","resolution":"720p","generate_audio":true
	}`

	status, submit := performJSONRequest(t, env.engine, http.MethodPost, "/api/v3/contents/generations/tasks", "Bearer e2e-1", requestBody)
	require.Equal(t, http.StatusOK, status, string(submit))
	var submitResponse map[string]any
	require.NoError(t, common.Unmarshal(submit, &submitResponse))
	require.Len(t, submitResponse, 1)
	publicID, ok := submitResponse["id"].(string)
	require.True(t, ok)
	assert.True(t, strings.HasPrefix(publicID, "task_"))
	assertMegaByAIE2EPublicBody(t, submit)

	requests := env.mock.snapshot()
	require.Len(t, requests, 1)
	assert.Equal(t, http.MethodPost, requests[0].Method)
	assert.Equal(t, "/v1/videos", requests[0].Path)
	assert.Equal(t, "Bearer mock-megabyai-key", requests[0].Authorization)
	assert.JSONEq(t, `{
		"model":"videos-mini","prompt":"multimodal MegaByAI acceptance","duration":8,
		"ratio":"16:9","resolution":"720p",
		"referenceImages":["https://8.8.8.8/ref.jpg"],
		"referenceVideos":["https://8.8.8.8/ref.mp4"],
		"referenceAudios":["https://8.8.8.8/ref.wav"]
	}`, string(requests[0].Body))
	assert.NotContains(t, string(requests[0].Body), "generate_audio")

	task := pollNewAPIVideoTask(t, publicID)
	assert.Equal(t, model.TaskStatus(model.TaskStatusInProgress), task.Status)
	task = pollNewAPIVideoTask(t, publicID)
	assert.Equal(t, model.TaskStatus(model.TaskStatusSuccess), task.Status)
	assert.Equal(t, megaByAIE2EUpstreamTaskID, task.PrivateData.UpstreamTaskID)
	assert.Equal(t, megaByAIE2EVideoURL, task.PrivateData.ResultURL)

	status, single := performJSONRequest(t, env.engine, http.MethodGet, "/api/v3/contents/generations/tasks/"+publicID, "Bearer e2e-1", "")
	require.Equal(t, http.StatusOK, status, string(single))
	assertMegaByAIE2ESucceededTask(t, single, publicID)

	status, list := performJSONRequest(t, env.engine, http.MethodGet, "/api/v3/contents/generations/tasks?filter.task_ids="+publicID+"&page_size=20", "Bearer e2e-1", "")
	require.Equal(t, http.StatusOK, status, string(list))
	assertMegaByAIE2EPublicBody(t, list)
	var listResponse struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	require.NoError(t, common.Unmarshal(list, &listResponse))
	require.Equal(t, 1, listResponse.Total)
	require.Len(t, listResponse.Items, 1)
	listItem, err := common.Marshal(listResponse.Items[0])
	require.NoError(t, err)
	assertMegaByAIE2ESucceededTask(t, listItem, publicID)

	assertMegaByAIInvalidRequestDoesNotConsume(t, env, requestBody, func() {
		env.videoMetadata.durationMS.Store(15_001)
	}, http.StatusBadRequest, "InvalidParameter.content")
	env.videoMetadata.durationMS.Store(15_000)
	assertMegaByAIInvalidRequestDoesNotConsume(t, env, requestBody, func() {
		env.audioMetadata.durationMS.Store(15_001)
	}, http.StatusBadRequest, "InvalidParameter.content")
	env.audioMetadata.durationMS.Store(15_000)
	assertMegaByAIInvalidRequestDoesNotConsume(t, env, requestBody, func() {
		env.audioMetadata.unavailable.Store(true)
	}, http.StatusServiceUnavailable, "reference_media_metadata_unavailable")
}

func TestMegaByAIFailedTaskRefundsAndPreservesErrorE2E(t *testing.T) {
	env := setupMegaByAIE2E(t,
		`{"task_id":"videos-mini_private","status":"failed","progress":100,"error":{"code":"unsupported_material","message":"\u7d20\u6750\u683c\u5f0f\u4e0d\u652f\u6301"}}`,
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

	summary := service.RunTaskPollingOnce(context.Background(), nil)
	assert.Equal(t, 1, summary.UnfinishedTasks)
	status, failed := performJSONRequest(t, env.engine, http.MethodGet, "/api/v3/contents/generations/tasks/"+publicID, "Bearer e2e-1", "")
	require.Equal(t, http.StatusOK, status, string(failed))
	assertMegaByAIE2EPublicBody(t, failed)
	var response struct {
		ID     string `json:"id"`
		Model  string `json:"model"`
		Status string `json:"status"`
		Error  struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	require.NoError(t, common.Unmarshal(failed, &response))
	assert.Equal(t, publicID, response.ID)
	assert.Equal(t, "doubao-seedance-2-0-260128", response.Model)
	assert.Equal(t, "failed", response.Status)
	assert.Equal(t, "unsupported_material", response.Error.Code)
	assert.Equal(t, "\u7d20\u6750\u683c\u5f0f\u4e0d\u652f\u6301", response.Error.Message)

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

func assertMegaByAIInvalidRequestDoesNotConsume(
	t *testing.T,
	env *megaByAIE2EEnvironment,
	requestBody string,
	mutate func(),
	wantStatus int,
	wantCode string,
) {
	t.Helper()
	var beforeTasks int64
	var beforeUser model.User
	require.NoError(t, model.DB.Model(&model.Task{}).Count(&beforeTasks).Error)
	require.NoError(t, model.DB.First(&beforeUser, e2eUserID).Error)
	beforeRequests := len(env.mock.snapshot())
	mutate()

	status, body := performJSONRequest(t, env.engine, http.MethodPost, "/api/v3/contents/generations/tasks", "Bearer e2e-1", requestBody)
	assert.Equal(t, wantStatus, status, string(body))
	assert.Contains(t, string(body), `"code":"`+wantCode+`"`)
	assert.Len(t, env.mock.snapshot(), beforeRequests)
	var afterTasks int64
	var afterUser model.User
	require.NoError(t, model.DB.Model(&model.Task{}).Count(&afterTasks).Error)
	require.NoError(t, model.DB.First(&afterUser, e2eUserID).Error)
	assert.Equal(t, beforeTasks, afterTasks)
	assert.Equal(t, beforeUser.Quota, afterUser.Quota)
	assert.Equal(t, beforeUser.UsedQuota, afterUser.UsedQuota)
}

func assertMegaByAIE2ESucceededTask(t *testing.T, body []byte, publicID string) {
	t.Helper()
	assertMegaByAIE2EPublicBody(t, body)
	var response map[string]any
	require.NoError(t, common.Unmarshal(body, &response))
	assert.Equal(t, publicID, response["id"])
	assert.Equal(t, "doubao-seedance-2-0-260128", response["model"])
	assert.Equal(t, "succeeded", response["status"])
	assert.Equal(t, map[string]any{"video_url": megaByAIE2EVideoURL}, response["content"])
}

func assertMegaByAIE2EPublicBody(t *testing.T, body []byte) {
	t.Helper()
	for _, privateValue := range []string{
		megaByAIE2EUpstreamTaskID, "videos-mini", "mock-megabyai-key",
		"user_id", "channel_id", `"group"`, `"quota"`, `"platform"`, `"properties"`, "upstream_model_name",
	} {
		assert.NotContains(t, string(body), privateValue)
	}
}
