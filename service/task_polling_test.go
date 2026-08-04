package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	taskdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/types"
	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type taskPollingFetchAdaptor struct {
	taskcommon.BaseBilling
	mu           sync.Mutex
	initInfo     *relaycommon.RelayInfo
	taskIDs      []string
	fetched      chan string
	blockTaskID  string
	blockStarted chan struct{}
	releaseBlock chan struct{}
	blockOnce    sync.Once
	statusCode   int
	responseBody []byte
	fetchErr     error
	parseResult  *relaycommon.TaskInfo
	parseErr     error
}

type costMeterCapturePollingAdaptor struct {
	*taskPollingFetchAdaptor
	normalizedResult *relaycommon.TaskInfo
	normalizeCalls   int
}

func (a *costMeterCapturePollingAdaptor) NormalizeTaskCostMeter(task *model.Task, result *relaycommon.TaskInfo) (types.CostMeter, error) {
	a.normalizeCalls++
	if result != nil {
		copy := *result
		if result.CostMeter != nil {
			meter := *result.CostMeter
			copy.CostMeter = &meter
		}
		a.normalizedResult = &copy
	}
	return a.BaseBilling.NormalizeTaskCostMeter(task, result)
}

type sunoFailurePollingAdaptor struct {
	failReason string
}

func (a *sunoFailurePollingAdaptor) Init(_ *relaycommon.RelayInfo) {}

func (a *sunoFailurePollingAdaptor) FetchTask(_ string, _ string, body map[string]any, _ string) (*http.Response, error) {
	taskIDs, _ := body["ids"].([]string)
	items := make([]taskdto.SunoDataResponse, 0, len(taskIDs))
	for _, taskID := range taskIDs {
		items = append(items, taskdto.SunoDataResponse{
			TaskID:     taskID,
			Status:     string(model.TaskStatusFailure),
			FailReason: a.failReason,
			FinishTime: time.Now().Unix(),
		})
	}

	responseBody, err := common.Marshal(taskdto.TaskResponse[[]taskdto.SunoDataResponse]{
		Code: taskdto.TaskSuccessCode,
		Data: items,
	})
	if err != nil {
		return nil, err
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(responseBody)),
	}, nil
}

func (a *sunoFailurePollingAdaptor) ParseTaskResult([]byte) (*relaycommon.TaskInfo, error) {
	return nil, nil
}

func (a *sunoFailurePollingAdaptor) AdjustBillingOnComplete(_ *model.Task, _ *relaycommon.TaskInfo) int {
	return 0
}

func (a *taskPollingFetchAdaptor) Init(info *relaycommon.RelayInfo) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if info == nil {
		a.initInfo = nil
		return
	}
	copy := *info
	if info.ChannelMeta != nil {
		channelMeta := *info.ChannelMeta
		copy.ChannelMeta = &channelMeta
	}
	a.initInfo = &copy
}

func (a *taskPollingFetchAdaptor) FetchTask(_ string, _ string, body map[string]any, _ string) (*http.Response, error) {
	taskID, _ := body["task_id"].(string)
	if taskID == a.blockTaskID && a.releaseBlock != nil {
		a.blockOnce.Do(func() {
			if a.blockStarted != nil {
				close(a.blockStarted)
			}
		})
		<-a.releaseBlock
	}

	a.mu.Lock()
	a.taskIDs = append(a.taskIDs, taskID)
	a.mu.Unlock()
	if a.fetched != nil {
		select {
		case a.fetched <- taskID:
		default:
		}
	}
	if a.fetchErr != nil {
		return nil, a.fetchErr
	}
	if a.responseBody != nil {
		statusCode := a.statusCode
		if statusCode == 0 {
			statusCode = http.StatusOK
		}
		return &http.Response{StatusCode: statusCode, Body: io.NopCloser(bytes.NewReader(a.responseBody))}, nil
	}

	response := taskdto.TaskResponse[model.Task]{
		Code: taskdto.TaskSuccessCode,
		Data: model.Task{
			TaskID:   taskID,
			Status:   model.TaskStatusInProgress,
			Progress: "30%",
		},
	}
	responseBody, err := common.Marshal(response)
	if err != nil {
		return nil, err
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(responseBody)),
	}, nil
}

func (a *taskPollingFetchAdaptor) ParseTaskResult([]byte) (*relaycommon.TaskInfo, error) {
	if a.parseErr != nil {
		return nil, a.parseErr
	}
	if a.parseResult != nil {
		copy := *a.parseResult
		return &copy, nil
	}
	return &relaycommon.TaskInfo{Status: model.TaskStatusInProgress}, nil
}

type taskPollingHTTPFetchAdaptor struct {
	*taskPollingFetchAdaptor
}

type taskResponseAuditPollingAdaptor struct {
	*taskPollingFetchAdaptor
}

func (a *taskResponseAuditPollingAdaptor) ConvertToOpenAIVideo(task *model.Task) ([]byte, error) {
	return common.Marshal(map[string]any{
		"id":     task.TaskID,
		"status": task.Status.ToVideoStatus(),
	})
}

func (a *taskResponseAuditPollingAdaptor) ConvertToArkVideoTask(task *model.Task) ([]byte, error) {
	response := map[string]any{
		"id":     task.TaskID,
		"status": strings.ToLower(string(task.Status)),
	}
	if task.Status == model.TaskStatusFailure {
		response["error"] = map[string]any{
			"code":    "mock_failed",
			"message": task.FailReason,
		}
	}
	return common.Marshal(response)
}

func (a *taskPollingHTTPFetchAdaptor) ParseTaskPollingHTTPError(body []byte, statusCode int) *relaycommon.TaskInfo {
	if statusCode == http.StatusNotFound || statusCode == http.StatusGone {
		result := relaycommon.FailTaskInfo("task not found or expired")
		result.ErrorCode = fmt.Sprintf("%d", statusCode)
		return result
	}
	var response struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := common.Unmarshal(body, &response); err != nil {
		return nil
	}
	result := relaycommon.FailTaskInfo(response.Message)
	result.ErrorCode = response.Code
	return result
}

func (a *taskPollingFetchAdaptor) AdjustBillingOnComplete(_ *model.Task, _ *relaycommon.TaskInfo) int {
	return 0
}

func (a *taskPollingFetchAdaptor) fetchCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.taskIDs)
}

func (a *taskPollingFetchAdaptor) fetchedTaskIDs() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.taskIDs...)
}

func (a *taskPollingFetchAdaptor) initializedInfo() *relaycommon.RelayInfo {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.initInfo == nil {
		return nil
	}
	copy := *a.initInfo
	if a.initInfo.ChannelMeta != nil {
		channelMeta := *a.initInfo.ChannelMeta
		copy.ChannelMeta = &channelMeta
	}
	return &copy
}

func seedTaskPollingChannel(t *testing.T, id int, disableSleep bool) {
	t.Helper()
	ch := &model.Channel{
		Id:     id,
		Type:   constant.ChannelTypeKling,
		Name:   "polling_channel",
		Key:    "sk-test",
		Status: common.ChannelStatusEnabled,
	}
	if disableSleep {
		ch.SetOtherSettings(dto.ChannelOtherSettings{DisableTaskPollingSleep: true})
	}
	require.NoError(t, model.DB.Create(ch).Error)
}

func seedPollingTask(t *testing.T, channelID int, publicID string, upstreamID string) *model.Task {
	t.Helper()
	task := &model.Task{
		TaskID:    publicID,
		Platform:  constant.TaskPlatform("kling"),
		UserId:    1,
		ChannelId: channelID,
		Action:    constant.TaskActionGenerate,
		Status:    model.TaskStatusInProgress,
		Progress:  "30%",
		CreatedAt: time.Now().Unix(),
		UpdatedAt: time.Now().Unix(),
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID: upstreamID,
		},
	}
	require.NoError(t, model.DB.Create(task).Error)
	return task
}

func TestTaskPollingCarriesSecureChannelProfileSettings(t *testing.T) {
	truncate(t)

	const channelID = 303
	baseURL := "https://secure.example"
	secureChannel := &model.Channel{
		Id:      channelID,
		Type:    constant.ChannelTypeSecure,
		Name:    "secure_enterprise",
		Key:     "secure-enterprise-key",
		Status:  common.ChannelStatusEnabled,
		BaseURL: &baseURL,
	}
	secureChannel.SetOtherSettings(dto.ChannelOtherSettings{
		SecureVideoGroup: dto.SecureVideoGroupEnterprise,
	})
	require.NoError(t, model.DB.Create(secureChannel).Error)

	task := seedPollingTask(t, channelID, "task_public_secure", "task_private_secure")
	task.Platform = constant.TaskPlatform("66")
	require.NoError(t, model.DB.Model(task).Update("platform", task.Platform).Error)

	adaptor := &taskPollingFetchAdaptor{}
	previousFactory := GetTaskAdaptorFunc
	GetTaskAdaptorFunc = func(constant.TaskPlatform) TaskPollingAdaptor { return adaptor }
	t.Cleanup(func() { GetTaskAdaptorFunc = previousFactory })

	require.NoError(t, updateVideoTasks(
		context.Background(),
		constant.TaskPlatform("66"),
		channelID,
		[]string{task.GetUpstreamTaskID()},
		map[string]*model.Task{task.GetUpstreamTaskID(): task},
	))

	info := adaptor.initializedInfo()
	require.NotNil(t, info)
	require.NotNil(t, info.ChannelMeta)
	assert.Equal(t, constant.ChannelTypeSecure, info.ChannelMeta.ChannelType)
	assert.Equal(t, baseURL, info.ChannelMeta.ChannelBaseUrl)
	assert.Equal(t, dto.SecureVideoGroupEnterprise, info.ChannelMeta.ChannelOtherSettings.SecureVideoGroup)
	assert.Equal(t, "secure-enterprise-key", info.ApiKey)
}

func runSinglePollingUpdate(t *testing.T, adaptor TaskPollingAdaptor, task *model.Task) error {
	t.Helper()
	upstreamID := task.GetUpstreamTaskID()
	return updateVideoSingleTask(context.Background(), adaptor, &model.Channel{
		Type: constant.ChannelTypeKling,
		Key:  "sk-test",
	}, upstreamID, map[string]*model.Task{upstreamID: task})
}

func TestSeedanceVersionedTaskDoesNotFinalizeWithoutUsage(t *testing.T) {
	truncate(t)
	task := seedPollingTask(t, 0, "task_public_broken_usage", "upstream_broken_usage")
	task.Quota = 4000
	task.PrivateData.BillingContext = &model.TaskBillingContext{
		UsageProfile:         model.TaskUsageProfileSeedance,
		UsageSnapshotVersion: model.TaskUsageSnapshotVersion1,
		HasVideoInput:        true,
	}
	require.NoError(t, task.Update())
	adaptor := &taskPollingFetchAdaptor{parseResult: &relaycommon.TaskInfo{Status: string(model.TaskStatusSuccess), Url: "https://x/video.mp4"}}

	err := runSinglePollingUpdate(t, adaptor, task)

	require.ErrorContains(t, err, "versioned Seedance usage snapshot is unavailable")
	var stored model.Task
	require.NoError(t, model.DB.First(&stored, task.ID).Error)
	assert.Equal(t, model.TaskStatus(model.TaskStatusInProgress), stored.Status)
	assert.Equal(t, 4000, stored.Quota)
}

func TestUpdateVideoSingleTaskHTTPRetryableLeavesTaskUnchanged(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		fetchErr error
		response []byte
	}{
		{name: "network", fetchErr: fmt.Errorf("network unavailable")},
		{name: "429", status: http.StatusTooManyRequests, response: []byte(`{"code":"rate_limit","message":"slow down"}`)},
		{name: "500", status: http.StatusInternalServerError, response: []byte(`{"code":"internal","message":"retry"}`)},
		{name: "503", status: http.StatusServiceUnavailable, response: []byte(`{"code":"unavailable","message":"retry"}`)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			truncate(t)
			task := seedPollingTask(t, 0, "task_public_retry", "upstream_retry")
			task.Quota = 77
			task.Data = []byte(`{"keep":true}`)
			task.PrivateData.ResultURL = "https://x/original.mp4"
			require.NoError(t, task.Update())
			adaptor := &taskPollingFetchAdaptor{statusCode: tt.status, responseBody: tt.response, fetchErr: tt.fetchErr}

			err := runSinglePollingUpdate(t, adaptor, task)

			require.Error(t, err)
			assert.Equal(t, model.TaskStatus(model.TaskStatusInProgress), task.Status)
			assert.Equal(t, "30%", task.Progress)
			assert.Equal(t, 77, task.Quota)
			assert.JSONEq(t, `{"keep":true}`, string(task.Data))
			assert.Equal(t, "https://x/original.mp4", task.PrivateData.ResultURL)
		})
	}
}

func TestUpdateVideoSingleTaskHTTPPermanentFailures(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusGone} {
		t.Run(fmt.Sprintf("status %d", status), func(t *testing.T) {
			truncate(t)
			task := seedPollingTask(t, 0, "task_public_gone", "upstream_gone")
			adaptor := &taskPollingHTTPFetchAdaptor{&taskPollingFetchAdaptor{
				statusCode: status, responseBody: []byte(`{"code":"gone","message":"provider detail"}`),
			}}
			require.NoError(t, runSinglePollingUpdate(t, adaptor, task))
			assert.Equal(t, model.TaskStatus(model.TaskStatusFailure), task.Status)
			assert.Equal(t, "task not found or expired", task.FailReason)
			assert.Contains(t, string(task.Data), `"code":"gone"`)
		})
	}

	truncate(t)
	task := seedPollingTask(t, 0, "task_public_bad", "upstream_bad")
	adaptor := &taskPollingHTTPFetchAdaptor{&taskPollingFetchAdaptor{
		statusCode: http.StatusBadRequest, responseBody: []byte(`{"code":"bad_duration","message":"duration invalid"}`),
	}}
	require.NoError(t, runSinglePollingUpdate(t, adaptor, task))
	assert.Equal(t, model.TaskStatus(model.TaskStatusFailure), task.Status)
	assert.Equal(t, "duration invalid", task.FailReason)
}

func TestUpdateVideoSingleTaskSanitizesFailurePersistenceAndLogs(t *testing.T) {
	truncate(t)
	const upstreamID = "upstream-private-id"
	responseBody := []byte(`{"code":"bad_request","message":"Authorization: Bearer secret-token; asset https://assets.example/private.mp4?signature=signed-secret; task upstream-private-id"}`)
	task := seedPollingTask(t, 0, "task_public_failure", upstreamID)
	adaptor := &taskPollingHTTPFetchAdaptor{&taskPollingFetchAdaptor{
		statusCode: http.StatusBadRequest, responseBody: responseBody,
	}}

	var logs bytes.Buffer
	previousDebug := common.DebugEnabled
	common.DebugEnabled = true
	common.LogWriterMu.Lock()
	previousWriter := gin.DefaultWriter
	previousErrorWriter := gin.DefaultErrorWriter
	gin.DefaultWriter = &logs
	gin.DefaultErrorWriter = &logs
	common.LogWriterMu.Unlock()
	t.Cleanup(func() {
		common.DebugEnabled = previousDebug
		common.LogWriterMu.Lock()
		gin.DefaultWriter = previousWriter
		gin.DefaultErrorWriter = previousErrorWriter
		common.LogWriterMu.Unlock()
	})

	require.NoError(t, runSinglePollingUpdate(t, adaptor, task))

	const expectedReason = "Authorization: Bearer ***; asset https://***.example/***?*** task [redacted]"
	assert.Equal(t, expectedReason, task.FailReason)
	var stored model.Task
	require.NoError(t, model.DB.First(&stored, task.ID).Error)
	assert.Equal(t, expectedReason, stored.FailReason)

	logOutput := logs.String()
	assert.Contains(t, logOutput, fmt.Sprintf("updateVideoSingleTask response: status=%d bytes=%d", http.StatusBadRequest, len(responseBody)))
	assert.Contains(t, logOutput, task.TaskID)
	for _, sensitive := range []string{"secret-token", "signed-secret", upstreamID} {
		assert.NotContains(t, logOutput, sensitive)
	}
}

func TestUpdateVideoSingleTaskMalformedSuccessLeavesTaskUnchanged(t *testing.T) {
	for _, tt := range []struct {
		name string
		body []byte
		err  error
	}{
		{name: "malformed JSON", body: []byte(`{bad`), err: fmt.Errorf("malformed JSON")},
		{name: "unknown status", body: []byte(`{"status":"mystery"}`), err: fmt.Errorf("unknown status")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			truncate(t)
			task := seedPollingTask(t, 0, "task_public_invalid", "upstream_invalid")
			task.Data = []byte(`{"keep":true}`)
			require.NoError(t, task.Update())
			adaptor := &taskPollingFetchAdaptor{statusCode: http.StatusOK, responseBody: tt.body, parseErr: tt.err}

			err := runSinglePollingUpdate(t, adaptor, task)

			require.Error(t, err)
			assert.Equal(t, model.TaskStatus(model.TaskStatusInProgress), task.Status)
			assert.JSONEq(t, `{"keep":true}`, string(task.Data))
		})
	}
}

func TestUpdateVideoSingleTaskDetailedWrapperPreservesBodyAndResultURL(t *testing.T) {
	truncate(t)
	task := seedPollingTask(t, 0, "task_public_detailed", "upstream_detailed")
	body := []byte(`{"code":"success","data":{"task_id":"upstream_detailed","status":"SUCCESS","result_url":"https://x/outer.mp4","progress":"100%","data":{"content":{"video_url":"https://x/nested.mp4"},"draft":false,"seed":47347,"usage":{"completion_tokens":108900,"total_tokens":108900},"future_field":{"keep":true}}}}`)
	adaptor := &taskPollingFetchAdaptor{
		statusCode:   http.StatusOK,
		responseBody: body,
		parseResult: &relaycommon.TaskInfo{
			Status: string(model.TaskStatusSuccess), Progress: "100%", Url: "https://x/outer.mp4",
			CompletionTokens: 108900, CompletionTokensPresent: true,
			TotalTokens: 108900, TotalTokensPresent: true,
		},
	}

	require.NoError(t, runSinglePollingUpdate(t, adaptor, task))
	assert.Equal(t, model.TaskStatus(model.TaskStatusSuccess), task.Status)
	assert.Equal(t, "https://x/outer.mp4", task.PrivateData.ResultURL)
	assert.NotContains(t, task.PrivateData.ResultURL, "/v1/videos/")
	assert.Contains(t, string(task.Data), `"draft":false`)
	assert.Contains(t, string(task.Data), `"seed":47347`)
	assert.Contains(t, string(task.Data), `"usage"`)
	assert.Contains(t, string(task.Data), `"future_field":{"keep":true}`)
	assert.Equal(t, []string{"upstream_detailed"}, adaptor.fetchedTaskIDs())
}

func TestVideoPollingPersistsTerminalUserResponseForOriginalProtocol(t *testing.T) {
	tests := []struct {
		name        string
		requestPath string
		result      *relaycommon.TaskInfo
		wantJSON    string
	}{
		{
			name:        "OpenAI success",
			requestPath: "/v1/video/generations",
			result:      &relaycommon.TaskInfo{Status: string(model.TaskStatusSuccess), Url: "https://x/video.mp4"},
			wantJSON:    `{"id":"task_public_audit","status":"completed"}`,
		},
		{
			name:        "ARK failure",
			requestPath: "/api/v3/contents/generations/tasks",
			result:      &relaycommon.TaskInfo{Status: string(model.TaskStatusFailure), Reason: "mock moderation rejection"},
			wantJSON:    `{"error":{"code":"mock_failed","message":"mock moderation rejection"},"id":"task_public_audit","status":"failure"}`,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			truncate(t)
			task := seedPollingTask(t, 0, "task_public_audit", "upstream_private_audit")
			task.Properties.RequestPath = testCase.requestPath
			require.NoError(t, task.Update())
			adaptor := &taskResponseAuditPollingAdaptor{&taskPollingFetchAdaptor{
				parseResult: testCase.result,
			}}

			require.NoError(t, runSinglePollingUpdate(t, adaptor, task))

			var stored model.Task
			require.NoError(t, model.DB.First(&stored, task.ID).Error)
			assert.JSONEq(t, testCase.wantJSON, string(stored.PrivateData.UserResponseData))
			assert.NotContains(t, string(stored.PrivateData.UserResponseData), "upstream_private_audit")
		})
	}
}

func TestSeedanceVideoPollingPersistsCompleteArkTerminalResponse(t *testing.T) {
	truncate(t)
	task := seedPollingTask(t, 0, "task_public_seedance_audit", "upstream_private_seedance_audit")
	task.Platform = constant.TaskPlatform(fmt.Sprint(constant.ChannelTypeFourSToken))
	task.SubmitTime = 111
	task.UpdatedAt = 222
	task.Properties.RequestPath = "/v1/video/generations"
	task.Properties.OriginModelName = "doubao-seedance-2-0-mini-260615"
	task.PrivateData.BillingContext = &model.TaskBillingContext{
		UsageProfile:             model.TaskUsageProfileSeedance,
		RequestedDurationSeconds: 5,
		Resolution:               "720p",
		SeedanceTokenBilling:     seedanceTokenBilling720p(),
	}
	require.NoError(t, task.Update())
	adaptor := &taskResponseAuditPollingAdaptor{&taskPollingFetchAdaptor{
		parseResult: &relaycommon.TaskInfo{
			Status: string(model.TaskStatusSuccess),
			Url:    "https://x/video.mp4",
		},
	}}

	require.NoError(t, runSinglePollingUpdate(t, adaptor, task))

	var stored model.Task
	require.NoError(t, model.DB.First(&stored, task.ID).Error)
	var response map[string]any
	require.NoError(t, common.Unmarshal(stored.PrivateData.UserResponseData, &response))
	assert.Equal(t, "task_public_seedance_audit", response["id"])
	assert.Equal(t, "doubao-seedance-2-0-mini-260615", response["model"])
	assert.Equal(t, "succeeded", response["status"])
	content, ok := response["content"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "https://x/video.mp4", content["video_url"])
	assert.EqualValues(t, 0, response["seed"])
	assert.Equal(t, "720p", response["resolution"])
	assert.Equal(t, "16:9", response["ratio"])
	assert.EqualValues(t, 5, response["duration"])
	assert.EqualValues(t, 24, response["framespersecond"])
	assert.Equal(t, "default", response["service_tier"])
	assert.EqualValues(t, 172800, response["execution_expires_after"])
	assert.Equal(t, true, response["generate_audio"])
	assert.Equal(t, false, response["draft"])
	assert.EqualValues(t, 0, response["priority"])
	usage, ok := response["usage"].(map[string]any)
	require.True(t, ok)
	assert.EqualValues(t, 108000, usage["completion_tokens"])
	assert.EqualValues(t, 108000, usage["total_tokens"])
	assert.NotContains(t, string(stored.PrivateData.UserResponseData), "upstream_private_seedance_audit")
}

func TestNewAPIVideoNonSeedancePollingKeepsOpenAIAuditFormat(t *testing.T) {
	truncate(t)
	task := seedPollingTask(t, 0, "task_public_generic_video", "upstream_generic_video")
	task.Platform = constant.TaskPlatform(fmt.Sprint(constant.ChannelTypeNewAPIVideo))
	task.Properties.RequestPath = "/v1/video/generations"
	task.Properties.OriginModelName = "generic-video-model"
	task.PrivateData.BillingContext = &model.TaskBillingContext{
		UpstreamCostMode: string(types.CostModePerDuration),
	}
	require.NoError(t, task.Update())
	adaptor := &taskResponseAuditPollingAdaptor{&taskPollingFetchAdaptor{
		parseResult: &relaycommon.TaskInfo{
			Status: string(model.TaskStatusSuccess),
			Url:    "https://x/video.mp4",
		},
	}}

	require.NoError(t, runSinglePollingUpdate(t, adaptor, task))

	var stored model.Task
	require.NoError(t, model.DB.First(&stored, task.ID).Error)
	assert.JSONEq(t, `{"id":"task_public_generic_video","status":"completed"}`, string(stored.PrivateData.UserResponseData))
	assert.NotContains(t, string(stored.PrivateData.UserResponseData), "resolution")
}

func TestSeedanceTaskPollingPersistsLocallyCalculatedUsage(t *testing.T) {
	truncate(t)
	task := seedPollingTask(t, 0, "task_public_seedance_usage", "upstream_seedance_usage")
	task.PrivateData.BillingContext = &model.TaskBillingContext{
		UsageProfile:             model.TaskUsageProfileSeedance,
		RequestedDurationSeconds: 5,
		Resolution:               "720p",
		SeedanceTokenBilling:     seedanceTokenBilling720p(),
	}
	require.NoError(t, task.Update())
	adaptor := &taskPollingFetchAdaptor{parseResult: &relaycommon.TaskInfo{
		Status: string(model.TaskStatusSuccess),
		Url:    "https://x/video.mp4",
	}}

	require.NoError(t, runSinglePollingUpdate(t, adaptor, task))

	var stored model.Task
	require.NoError(t, model.DB.First(&stored, task.ID).Error)
	require.NotNil(t, stored.PrivateData.BillingContext)
	assert.Equal(t, model.TaskUsageSourceLocalCalculated, stored.PrivateData.BillingContext.UsageSource)
	assert.Equal(t, 108000, stored.PrivateData.BillingContext.BillingTokens)
}

func TestTaskPollingCostSettlesOnlyOnTerminalCASWinner(t *testing.T) {
	config := validTokenCostConfig(types.CostTokenModeTotal, types.CostMeterUpstreamUsage)
	config.ChargeEvent = types.CostChargeTaskSucceeded
	task, handle := prepareTaskPollingCostAttempt(t, types.CostModePerToken, config)
	task.Platform = constant.TaskPlatform("kling")
	task.Status = model.TaskStatusInProgress
	task.Progress = "30%"
	task.PrivateData.UpstreamTaskID = "upstream-cost-cas"
	require.NoError(t, model.DB.Create(task).Error)

	firstPoll := *task
	stalePoll := *task
	adaptor := &taskPollingFetchAdaptor{parseResult: &relaycommon.TaskInfo{
		Status:             string(model.TaskStatusSuccess),
		TotalTokens:        250_000,
		TotalTokensPresent: true,
	}}

	require.NoError(t, runSinglePollingUpdate(t, adaptor, &firstPoll))
	require.NoError(t, runSinglePollingUpdate(t, adaptor, &stalePoll))

	attempt := loadCostAttempt(t, handle.AttemptID)
	assert.Equal(t, string(types.CostAttemptSettled), attempt.Status)
	require.NotNil(t, attempt.CostNanoUSD)
	assert.Equal(t, int64(250_000_000), *attempt.CostNanoUSD)
}

func TestTaskPollingCostMissingAuthoritativeMeterFailsSettlement(t *testing.T) {
	config := validTokenCostConfig(types.CostTokenModeTotal, types.CostMeterUpstreamUsage)
	config.ChargeEvent = types.CostChargeTaskSucceeded
	task, handle := prepareTaskPollingCostAttempt(t, types.CostModePerToken, config)
	task.Platform = constant.TaskPlatform("kling")
	task.Status = model.TaskStatusInProgress
	task.Progress = "30%"
	task.PrivateData.UpstreamTaskID = "upstream-cost-missing-meter"
	require.NoError(t, model.DB.Create(task).Error)
	adaptor := &taskPollingFetchAdaptor{parseResult: &relaycommon.TaskInfo{
		Status:      string(model.TaskStatusSuccess),
		TotalTokens: 250_000,
	}}

	require.NoError(t, runSinglePollingUpdate(t, adaptor, task))

	attempt := loadCostAttempt(t, handle.AttemptID)
	assert.Equal(t, string(types.CostAttemptSettlementFailed), attempt.Status)
	assert.Nil(t, attempt.CostNanoUSD)
}

func TestSeedanceLocalUsageDoesNotBecomeSupplierTokenMeter(t *testing.T) {
	config := validTokenCostConfig(types.CostTokenModeTotal, types.CostMeterUpstreamUsage)
	config.ChargeEvent = types.CostChargeTaskSucceeded
	task, handle := prepareTaskPollingCostAttempt(t, types.CostModePerToken, config)
	task.Platform = constant.TaskPlatform("kling")
	task.Status = model.TaskStatusInProgress
	task.Progress = "30%"
	task.PrivateData.UpstreamTaskID = "upstream-cost-local-usage"
	task.PrivateData.BillingContext = &model.TaskBillingContext{
		UsageProfile:             model.TaskUsageProfileSeedance,
		RequestedDurationSeconds: 5,
		Resolution:               "720p",
		SeedanceTokenBilling:     seedanceTokenBilling720p(),
	}
	require.NoError(t, model.DB.Create(task).Error)
	adaptor := &taskPollingFetchAdaptor{parseResult: &relaycommon.TaskInfo{
		Status: string(model.TaskStatusSuccess),
		Url:    "https://x/video.mp4",
	}}

	require.NoError(t, runSinglePollingUpdate(t, adaptor, task))

	attempt := loadCostAttempt(t, handle.AttemptID)
	assert.Equal(t, string(types.CostAttemptSettlementFailed), attempt.Status)
	assert.Nil(t, attempt.CostNanoUSD)
	assert.Equal(t, model.TaskUsageSourceLocalCalculated, task.PrivateData.BillingContext.UsageSource)
	assert.Equal(t, 108000, task.PrivateData.BillingContext.BillingTokens)
}

func TestSeedanceLocalSaleUsagePreservesAuthoritativeSupplierTokenMeter(t *testing.T) {
	config := validTokenCostConfig(types.CostTokenModeTotal, types.CostMeterUpstreamUsage)
	config.ChargeEvent = types.CostChargeTaskSucceeded
	task, handle := prepareTaskPollingCostAttempt(t, types.CostModePerToken, config)
	task.Platform = constant.TaskPlatform("kling")
	task.Status = model.TaskStatusInProgress
	task.Progress = "30%"
	task.PrivateData.UpstreamTaskID = "upstream-cost-independent-usage"
	task.PrivateData.BillingContext = &model.TaskBillingContext{
		UsageProfile:             model.TaskUsageProfileSeedance,
		RequestedDurationSeconds: 5,
		Resolution:               "720p",
		SeedanceTokenBilling:     seedanceTokenBilling720p(),
	}
	require.NoError(t, model.DB.Create(task).Error)
	duration := "10"
	adaptor := &taskPollingFetchAdaptor{parseResult: &relaycommon.TaskInfo{
		Status:                  string(model.TaskStatusSuccess),
		Url:                     "https://x/video.mp4",
		CompletionTokens:        200_000,
		TotalTokens:             250_000,
		CompletionTokensPresent: true,
		TotalTokensPresent:      true,
		CostMeter: &types.CostMeter{
			Source: types.CostMeterUpstreamActual, DurationSeconds: &duration,
		},
	}}

	require.NoError(t, runSinglePollingUpdate(t, adaptor, task))

	attempt := loadCostAttempt(t, handle.AttemptID)
	assert.Equal(t, string(types.CostAttemptSettled), attempt.Status)
	require.NotNil(t, attempt.CostNanoUSD)
	assert.Equal(t, int64(250_000_000), *attempt.CostNanoUSD)
	assert.Equal(t, model.TaskUsageSourceLocalCalculated, task.PrivateData.BillingContext.UsageSource)
	assert.Equal(t, 108000, task.PrivateData.BillingContext.BillingTokens)
}

func TestPreparePolledTaskCostUsesSupplierMeterWhenSeedanceSaleUsageIsLocal(t *testing.T) {
	config := validTokenCostConfig(types.CostTokenModeTotal, types.CostMeterUpstreamUsage)
	config.ChargeEvent = types.CostChargeTaskSucceeded
	task, handle := prepareTaskPollingCostAttempt(t, types.CostModePerToken, config)
	totalTokens := int64(250_000)
	supplierMeter := types.CostMeter{Source: types.CostMeterUpstreamUsage, TotalTokens: &totalTokens}
	result := &relaycommon.TaskInfo{
		Status:                  string(model.TaskStatusSuccess),
		UsageSource:             model.TaskUsageSourceLocalCalculated,
		CompletionTokens:        108_000,
		TotalTokens:             108_000,
		CompletionTokensPresent: true,
		TotalTokensPresent:      true,
		UpstreamUsageCostMeter:  &supplierMeter,
	}
	adaptor := &costMeterCapturePollingAdaptor{taskPollingFetchAdaptor: &taskPollingFetchAdaptor{}}

	require.NoError(t, preparePolledTaskCostSettlement(context.Background(), adaptor, task, result))

	require.NotNil(t, adaptor.normalizedResult)
	assert.False(t, adaptor.normalizedResult.CompletionTokensPresent)
	assert.False(t, adaptor.normalizedResult.TotalTokensPresent)
	require.NotNil(t, adaptor.normalizedResult.CostMeter)
	assert.Equal(t, supplierMeter, *adaptor.normalizedResult.CostMeter)
	attempt := loadCostAttempt(t, handle.AttemptID)
	assert.Contains(t, attempt.ActualMeterJSON, `"total_tokens":250000`)
}

func TestPreparePolledTaskCostSkipsNormalizerForValidatedRequestMeter(t *testing.T) {
	prepareCostAttemptServiceDB(t)
	config := validDurationCostConfig(types.CostMeterValidatedRequest)
	config.ChargeEvent = types.CostChargeTaskSucceeded
	seedActiveAttemptRule(t, types.CostModePerDuration, config)
	taskID := "task-validated-request-meter"
	duration := "4"
	input := preparedAttemptInput()
	input.RequestID = "request-" + taskID
	input.TaskID = &taskID
	input.TaskPlatform = constant.TaskPlatform("task-test")
	input.RequestPath = "/v1/video/generations"
	input.FinalUserQuota = nil
	input.RequestMeter = &types.CostMeter{Source: types.CostMeterValidatedRequest, DurationSeconds: &duration}
	handle, err := PrepareCostAttempt(context.Background(), input)
	require.NoError(t, err)
	require.NoError(t, AuthorizeCostDispatch(context.Background(), handle))
	require.NoError(t, RecordCostDispatchOutcome(context.Background(), handle, types.CostOutcome{
		Status: types.CostAttemptAwaitingMeter, UpstreamAccepted: true,
	}))
	require.NoError(t, MarkWinningCostAttempt(context.Background(), handle))
	task := &model.Task{TaskID: taskID, PrivateData: model.TaskPrivateData{CostRequestID: handle.CostRequestID}}
	adaptor := &costMeterCapturePollingAdaptor{taskPollingFetchAdaptor: &taskPollingFetchAdaptor{}}

	require.NoError(t, preparePolledTaskCostSettlement(context.Background(), adaptor, task, &relaycommon.TaskInfo{
		Status: string(model.TaskStatusSuccess),
	}))

	assert.Zero(t, adaptor.normalizeCalls)
}

func TestUpdateVideoSingleTaskDirectInProgressIgnoresCompletedAt(t *testing.T) {
	truncate(t)
	task := seedPollingTask(t, 0, "task_public_progress", "upstream_progress")
	body := []byte(`{"id":"upstream_progress","status":"in_progress","progress":50,"completed_at":1784716351,"metadata":{"url":""}}`)
	adaptor := &taskPollingFetchAdaptor{
		statusCode: http.StatusOK, responseBody: body,
		parseResult: &relaycommon.TaskInfo{Status: string(model.TaskStatusInProgress), Progress: "50%"},
	}

	require.NoError(t, runSinglePollingUpdate(t, adaptor, task))
	assert.Equal(t, model.TaskStatus(model.TaskStatusInProgress), task.Status)
	assert.Equal(t, "50%", task.Progress)
	assert.Zero(t, task.FinishTime)
	assert.Empty(t, task.PrivateData.ResultURL)
	assert.JSONEq(t, string(body), string(task.Data))
}

func TestUpdateVideoTasksDefaultSleepWaitsBetweenTasks(t *testing.T) {
	truncate(t)

	const channelID = 101
	seedTaskPollingChannel(t, channelID, false)
	first := seedPollingTask(t, channelID, "task_public_1", "upstream_1")
	second := seedPollingTask(t, channelID, "task_public_2", "upstream_2")

	adaptor := &taskPollingFetchAdaptor{}
	previousFactory := GetTaskAdaptorFunc
	GetTaskAdaptorFunc = func(constant.TaskPlatform) TaskPollingAdaptor { return adaptor }
	t.Cleanup(func() { GetTaskAdaptorFunc = previousFactory })

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := UpdateVideoTasks(ctx, constant.TaskPlatform("kling"), map[int][]string{
		channelID: {
			first.GetUpstreamTaskID(),
			second.GetUpstreamTaskID(),
		},
	}, map[string]*model.Task{
		first.GetUpstreamTaskID():  first,
		second.GetUpstreamTaskID(): second,
	})

	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Equal(t, 1, adaptor.fetchCount())
}

func TestUpdateVideoTasksCanSkipPollingSleepPerChannel(t *testing.T) {
	truncate(t)

	const channelID = 102
	seedTaskPollingChannel(t, channelID, true)
	first := seedPollingTask(t, channelID, "task_public_3", "upstream_3")
	second := seedPollingTask(t, channelID, "task_public_4", "upstream_4")

	adaptor := &taskPollingFetchAdaptor{}
	previousFactory := GetTaskAdaptorFunc
	GetTaskAdaptorFunc = func(constant.TaskPlatform) TaskPollingAdaptor { return adaptor }
	t.Cleanup(func() { GetTaskAdaptorFunc = previousFactory })

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	err := UpdateVideoTasks(ctx, constant.TaskPlatform("kling"), map[int][]string{
		channelID: {
			first.GetUpstreamTaskID(),
			second.GetUpstreamTaskID(),
		},
	}, map[string]*model.Task{
		first.GetUpstreamTaskID():  first,
		second.GetUpstreamTaskID(): second,
	})

	require.NoError(t, err)
	assert.Equal(t, 2, adaptor.fetchCount())
}

func TestUpdateVideoTasksDefaultSleepDoesNotBlockOtherChannels(t *testing.T) {
	truncate(t)

	const firstChannelID = 201
	const secondChannelID = 202
	seedTaskPollingChannel(t, firstChannelID, false)
	seedTaskPollingChannel(t, secondChannelID, false)
	firstChannelFirst := seedPollingTask(t, firstChannelID, "task_public_5", "upstream_a_1")
	firstChannelSecond := seedPollingTask(t, firstChannelID, "task_public_6", "upstream_a_2")
	secondChannelFirst := seedPollingTask(t, secondChannelID, "task_public_7", "upstream_b_1")
	secondChannelSecond := seedPollingTask(t, secondChannelID, "task_public_8", "upstream_b_2")

	adaptor := &taskPollingFetchAdaptor{}
	previousFactory := GetTaskAdaptorFunc
	GetTaskAdaptorFunc = func(constant.TaskPlatform) TaskPollingAdaptor { return adaptor }
	t.Cleanup(func() { GetTaskAdaptorFunc = previousFactory })

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := UpdateVideoTasks(ctx, constant.TaskPlatform("kling"), map[int][]string{
		firstChannelID: {
			firstChannelFirst.GetUpstreamTaskID(),
			firstChannelSecond.GetUpstreamTaskID(),
		},
		secondChannelID: {
			secondChannelFirst.GetUpstreamTaskID(),
			secondChannelSecond.GetUpstreamTaskID(),
		},
	}, map[string]*model.Task{
		firstChannelFirst.GetUpstreamTaskID():   firstChannelFirst,
		firstChannelSecond.GetUpstreamTaskID():  firstChannelSecond,
		secondChannelFirst.GetUpstreamTaskID():  secondChannelFirst,
		secondChannelSecond.GetUpstreamTaskID(): secondChannelSecond,
	})

	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.ElementsMatch(t, []string{"upstream_a_1", "upstream_b_1"}, adaptor.fetchedTaskIDs())
}

func TestUpdateVideoTasksSlowChannelDoesNotBlockOtherChannels(t *testing.T) {
	truncate(t)

	const slowChannelID = 251
	const fastChannelID = 252
	seedTaskPollingChannel(t, slowChannelID, false)
	seedTaskPollingChannel(t, fastChannelID, true)
	slowTask := seedPollingTask(t, slowChannelID, "task_public_slow", "upstream_slow_1")
	fastFirst := seedPollingTask(t, fastChannelID, "task_public_fast_1", "upstream_fast_parallel_1")
	fastSecond := seedPollingTask(t, fastChannelID, "task_public_fast_2", "upstream_fast_parallel_2")

	adaptor := &taskPollingFetchAdaptor{
		fetched:      make(chan string, 4),
		blockTaskID:  slowTask.GetUpstreamTaskID(),
		blockStarted: make(chan struct{}),
		releaseBlock: make(chan struct{}),
	}
	var releaseOnce sync.Once
	releaseBlockedTask := func() {
		releaseOnce.Do(func() {
			close(adaptor.releaseBlock)
		})
	}
	t.Cleanup(releaseBlockedTask)
	previousFactory := GetTaskAdaptorFunc
	GetTaskAdaptorFunc = func(constant.TaskPlatform) TaskPollingAdaptor { return adaptor }
	t.Cleanup(func() { GetTaskAdaptorFunc = previousFactory })

	errCh := make(chan error, 1)
	gopool.Go(func() {
		errCh <- UpdateVideoTasks(context.Background(), constant.TaskPlatform("kling"), map[int][]string{
			slowChannelID: {
				slowTask.GetUpstreamTaskID(),
			},
			fastChannelID: {
				fastFirst.GetUpstreamTaskID(),
				fastSecond.GetUpstreamTaskID(),
			},
		}, map[string]*model.Task{
			slowTask.GetUpstreamTaskID():   slowTask,
			fastFirst.GetUpstreamTaskID():  fastFirst,
			fastSecond.GetUpstreamTaskID(): fastSecond,
		})
	})

	select {
	case <-adaptor.blockStarted:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("slow channel did not start blocking")
	}

	require.Eventually(t, func() bool {
		fetchedTaskIDs := adaptor.fetchedTaskIDs()
		return len(fetchedTaskIDs) == 2 &&
			fetchedTaskIDs[0] == fastFirst.GetUpstreamTaskID() &&
			fetchedTaskIDs[1] == fastSecond.GetUpstreamTaskID()
	}, 500*time.Millisecond, 10*time.Millisecond)

	releaseBlockedTask()
	require.NoError(t, <-errCh)
	assert.ElementsMatch(t, []string{
		slowTask.GetUpstreamTaskID(),
		fastFirst.GetUpstreamTaskID(),
		fastSecond.GetUpstreamTaskID(),
	}, adaptor.fetchedTaskIDs())
}

func TestUpdateVideoTasksMixedChannelSleepSettings(t *testing.T) {
	truncate(t)

	const sleepyChannelID = 301
	const fastChannelID = 302
	seedTaskPollingChannel(t, sleepyChannelID, false)
	seedTaskPollingChannel(t, fastChannelID, true)
	sleepyFirst := seedPollingTask(t, sleepyChannelID, "task_public_9", "upstream_sleepy_1")
	sleepySecond := seedPollingTask(t, sleepyChannelID, "task_public_10", "upstream_sleepy_2")
	fastFirst := seedPollingTask(t, fastChannelID, "task_public_11", "upstream_fast_1")
	fastSecond := seedPollingTask(t, fastChannelID, "task_public_12", "upstream_fast_2")

	adaptor := &taskPollingFetchAdaptor{}
	previousFactory := GetTaskAdaptorFunc
	GetTaskAdaptorFunc = func(constant.TaskPlatform) TaskPollingAdaptor { return adaptor }
	t.Cleanup(func() { GetTaskAdaptorFunc = previousFactory })

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := UpdateVideoTasks(ctx, constant.TaskPlatform("kling"), map[int][]string{
		sleepyChannelID: {
			sleepyFirst.GetUpstreamTaskID(),
			sleepySecond.GetUpstreamTaskID(),
		},
		fastChannelID: {
			fastFirst.GetUpstreamTaskID(),
			fastSecond.GetUpstreamTaskID(),
		},
	}, map[string]*model.Task{
		sleepyFirst.GetUpstreamTaskID():  sleepyFirst,
		sleepySecond.GetUpstreamTaskID(): sleepySecond,
		fastFirst.GetUpstreamTaskID():    fastFirst,
		fastSecond.GetUpstreamTaskID():   fastSecond,
	})

	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.ElementsMatch(t, []string{"upstream_sleepy_1", "upstream_fast_1", "upstream_fast_2"}, adaptor.fetchedTaskIDs())
}

func TestUpdateSunoTasksStalePollsRefundExactlyOnce(t *testing.T) {
	truncate(t)

	const userID, tokenID, channelID = 401, 401, 401
	const initialUserQuota, initialTokenQuota, taskQuota = 10_000, 6_000, 2_500
	const publicTaskID, upstreamTaskID = "suno_public_refund_once", "suno_upstream_refund_once"

	seedUser(t, userID, initialUserQuota)
	seedToken(t, tokenID, userID, "sk-suno-refund-once", initialTokenQuota)
	baseURL := "https://suno.invalid"
	require.NoError(t, model.DB.Create(&model.Channel{
		Id:      channelID,
		Type:    constant.ChannelTypeSunoAPI,
		Name:    "suno_refund_once",
		Key:     "sk-suno-channel",
		Status:  common.ChannelStatusEnabled,
		BaseURL: &baseURL,
	}).Error)

	task := makeTask(userID, channelID, taskQuota, tokenID, BillingSourceWallet, 0)
	task.TaskID = publicTaskID
	task.Platform = constant.TaskPlatformSuno
	task.Status = model.TaskStatusInProgress
	task.Progress = "50%"
	task.SubmitTime = time.Now().Unix()
	task.PrivateData.UpstreamTaskID = upstreamTaskID
	require.NoError(t, model.DB.Create(task).Error)

	var firstPollTask model.Task
	var staleSecondPollTask model.Task
	require.NoError(t, model.DB.First(&firstPollTask, task.ID).Error)
	require.NoError(t, model.DB.First(&staleSecondPollTask, task.ID).Error)

	adaptor := &sunoFailurePollingAdaptor{failReason: "upstream failed"}
	previousFactory := GetTaskAdaptorFunc
	GetTaskAdaptorFunc = func(constant.TaskPlatform) TaskPollingAdaptor { return adaptor }
	t.Cleanup(func() { GetTaskAdaptorFunc = previousFactory })

	require.NoError(t, updateSunoTasks(context.Background(), channelID, []string{upstreamTaskID}, map[string]*model.Task{
		upstreamTaskID: &firstPollTask,
	}))
	require.NoError(t, updateSunoTasks(context.Background(), channelID, []string{upstreamTaskID}, map[string]*model.Task{
		upstreamTaskID: &staleSecondPollTask,
	}))

	var reloaded model.Task
	require.NoError(t, model.DB.First(&reloaded, task.ID).Error)
	assert.EqualValues(t, model.TaskStatusFailure, reloaded.Status)
	assert.Zero(t, reloaded.Quota)
	assert.Equal(t, initialUserQuota+taskQuota, getUserQuota(t, userID))
	assert.Equal(t, initialTokenQuota+taskQuota, getTokenRemainQuota(t, tokenID))
	assert.Equal(t, int64(1), countLogs(t))
}

func TestRunTaskPollingOnceDoesNotRefundHistoricalFailedTask(t *testing.T) {
	truncate(t)

	const userID, initialQuota, taskQuota = 402, 10_000, 1_200
	seedUser(t, userID, initialQuota)

	task := makeTask(userID, 0, taskQuota, 0, BillingSourceWallet, 0)
	task.TaskID = "historical_failed_already_refunded"
	task.Status = model.TaskStatusFailure
	task.Progress = "100%"
	task.SubmitTime = model.TaskRefundLegacyCutoff - 1
	task.UpdatedAt = time.Now().Add(-time.Minute).Unix()
	require.NoError(t, model.DB.Create(task).Error)

	previousFactory := GetTaskAdaptorFunc
	GetTaskAdaptorFunc = func(constant.TaskPlatform) TaskPollingAdaptor {
		return &taskPollingFetchAdaptor{}
	}
	t.Cleanup(func() { GetTaskAdaptorFunc = previousFactory })

	summary := RunTaskPollingOnce(context.Background(), nil)

	assert.Zero(t, summary.UnfinishedTasks)
	assert.Equal(t, initialQuota, getUserQuota(t, userID))
	assert.Equal(t, taskQuota, getTaskQuota(t, task.ID))
	assert.Equal(t, int64(0), countLogs(t))
}

func TestSweepTimedOutTasksHonorsRefundRolloutBoundary(t *testing.T) {
	truncate(t)

	const (
		userID          = 403
		initialQuota    = 10_000
		legacyTaskQuota = 1_800
		modernTaskQuota = 1_200
	)
	seedUser(t, userID, initialQuota)

	legacyTask := makeTask(userID, 0, legacyTaskQuota, 0, BillingSourceWallet, 0)
	legacyTask.TaskID = "legacy_timeout_without_refund"
	legacyTask.Progress = "50%"
	legacyTask.SubmitTime = 1771718399 // 2026-02-21 23:59:59 UTC
	require.NoError(t, model.DB.Create(legacyTask).Error)

	modernTask := makeTask(userID, 0, modernTaskQuota, 0, BillingSourceWallet, 0)
	modernTask.TaskID = "modern_timeout_with_refund"
	modernTask.Progress = "50%"
	modernTask.SubmitTime = 1771718400 // 2026-02-22 00:00:00 UTC
	require.NoError(t, model.DB.Create(modernTask).Error)

	previousTimeout := constant.TaskTimeoutMinutes
	constant.TaskTimeoutMinutes = 1
	t.Cleanup(func() { constant.TaskTimeoutMinutes = previousTimeout })

	sweepTimedOutTasks(context.Background())

	var reloadedLegacy model.Task
	var reloadedModern model.Task
	require.NoError(t, model.DB.First(&reloadedLegacy, legacyTask.ID).Error)
	require.NoError(t, model.DB.First(&reloadedModern, modernTask.ID).Error)
	assert.EqualValues(t, model.TaskStatusFailure, reloadedLegacy.Status)
	assert.EqualValues(t, model.TaskStatusFailure, reloadedModern.Status)
	assert.Zero(t, reloadedLegacy.Quota)
	assert.Zero(t, reloadedModern.Quota)
	assert.Contains(t, reloadedLegacy.FailReason, "旧系统遗留任务")
	assert.Contains(t, reloadedModern.FailReason, "任务超时")
	assert.Equal(t, initialQuota+modernTaskQuota, getUserQuota(t, userID))
	assert.Equal(t, int64(1), countLogs(t))
}

func TestSweepUnrefundedFailedTasksAdjustsRecognizedCostRevenue(t *testing.T) {
	truncate(t)

	config := validPerRequestCostConfig()
	config.ChargeEvent = types.CostChargeTaskSucceeded
	task, handle := prepareTaskPollingCostAttempt(t, types.CostModePerRequest, config)
	const userID, taskQuota = 11, 1_200
	seedUser(t, userID, 10_000)
	task.UserId = userID
	task.Quota = taskQuota
	task.Status = model.TaskStatusFailure
	task.Progress = "100%"
	task.SubmitTime = model.TaskRefundLegacyCutoff
	task.UpdatedAt = time.Now().Add(-time.Minute).Unix()
	require.NoError(t, model.DB.Create(task).Error)
	require.NoError(t, recordPendingAsyncCostSettlement(context.Background(), handle.CostRequestID, task, &relaycommon.TaskInfo{
		Status: string(model.TaskStatusFailure),
	}))

	callbackName := "test:fail_refund_revenue_recognition"
	callbackRegistered := true
	require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Name == "CostAccountingRequest" {
			tx.AddError(fmt.Errorf("injected refund revenue recognition failure"))
		}
	}))
	t.Cleanup(func() {
		if callbackRegistered {
			_ = model.DB.Callback().Update().Remove(callbackName)
		}
	})
	sweepUnrefundedFailedTasks(context.Background())
	require.NoError(t, model.DB.Callback().Update().Remove(callbackName))
	callbackRegistered = false

	var refunded model.Task
	require.NoError(t, model.DB.First(&refunded, task.ID).Error)
	assert.Zero(t, refunded.Quota)
	request := loadCostRevenueRequest(t, handle.CostRequestID)
	assert.Equal(t, string(types.CostRevenuePending), request.RevenueStatus)
	attempt := loadCostAttempt(t, handle.AttemptID)
	assert.Equal(t, string(types.CostAttemptAwaitingMeter), attempt.Status)

	summary, err := RecoverStaleCostAccounting(context.Background(), time.Now(), 10)
	require.NoError(t, err)
	assert.Equal(t, 1, summary.AwaitingSettled)

	request = loadCostRevenueRequest(t, handle.CostRequestID)
	assert.Equal(t, string(types.CostRevenueConfirmedZero), request.RevenueStatus)
	require.NotNil(t, request.FinalUserQuota)
	assert.Zero(t, *request.FinalUserQuota)
	require.NotNil(t, request.BilledRevenueEquivalentNanoUSD)
	assert.Zero(t, *request.BilledRevenueEquivalentNanoUSD)
	assert.Equal(t, string(types.CostProfitComplete), request.ProfitStatus)
	require.NotNil(t, request.BilledGrossProfitNanoUSD)
	assert.Zero(t, *request.BilledGrossProfitNanoUSD)
}

func TestTaskPollingKeepsTaskRetryableWhenPendingCostMeterPersistenceFails(t *testing.T) {
	truncate(t)

	config := validTokenCostConfig(types.CostTokenModeTotal, types.CostMeterUpstreamUsage)
	config.ChargeEvent = types.CostChargeTaskSucceeded
	task, handle := prepareTaskPollingCostAttempt(t, types.CostModePerToken, config)
	task.Platform = constant.TaskPlatform("kling")
	task.Status = model.TaskStatusInProgress
	task.Progress = "30%"
	task.PrivateData.UpstreamTaskID = "upstream-cost-persist-retry"
	require.NoError(t, model.DB.Create(task).Error)

	callbackName := "test:fail_pending_cost_meter"
	callbackRegistered := true
	require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Name == "CostAccountingAttempt" {
			tx.AddError(fmt.Errorf("injected pending cost meter failure"))
		}
	}))
	t.Cleanup(func() {
		if callbackRegistered {
			_ = model.DB.Callback().Update().Remove(callbackName)
		}
	})
	adaptor := &taskPollingFetchAdaptor{parseResult: &relaycommon.TaskInfo{
		Status:             string(model.TaskStatusSuccess),
		TotalTokens:        250_000,
		TotalTokensPresent: true,
	}}

	require.Error(t, runSinglePollingUpdate(t, adaptor, task))
	var retryable model.Task
	require.NoError(t, model.DB.First(&retryable, task.ID).Error)
	assert.Equal(t, model.TaskStatus(model.TaskStatusInProgress), retryable.Status)
	assert.Empty(t, loadCostAttempt(t, handle.AttemptID).ActualMeterJSON)

	require.NoError(t, model.DB.Callback().Update().Remove(callbackName))
	callbackRegistered = false
	require.NoError(t, runSinglePollingUpdate(t, adaptor, &retryable))
	require.NoError(t, model.DB.First(&retryable, task.ID).Error)
	assert.Equal(t, model.TaskStatus(model.TaskStatusSuccess), retryable.Status)
	assert.Equal(t, string(types.CostAttemptSettled), loadCostAttempt(t, handle.AttemptID).Status)
}

func TestSweepTimedOutTasksAdjustsRecognizedCostRevenue(t *testing.T) {
	truncate(t)

	config := validPerRequestCostConfig()
	config.ChargeEvent = types.CostChargeTaskSucceeded
	task, handle := prepareTaskPollingCostAttempt(t, types.CostModePerRequest, config)
	const userID, taskQuota = 11, 1_800
	seedUser(t, userID, 10_000)
	task.UserId = userID
	task.Quota = taskQuota
	task.Status = model.TaskStatusInProgress
	task.Progress = "50%"
	task.SubmitTime = time.Now().Add(-2 * time.Minute).Unix()
	task.UpdatedAt = task.SubmitTime
	require.NoError(t, model.DB.Create(task).Error)

	previousTimeout := constant.TaskTimeoutMinutes
	constant.TaskTimeoutMinutes = 1
	t.Cleanup(func() { constant.TaskTimeoutMinutes = previousTimeout })
	sweepTimedOutTasks(context.Background())

	request := loadCostRevenueRequest(t, handle.CostRequestID)
	assert.Equal(t, string(types.CostRevenueConfirmedZero), request.RevenueStatus)
	require.NotNil(t, request.FinalUserQuota)
	assert.Zero(t, *request.FinalUserQuota)
	require.NotNil(t, request.BilledRevenueEquivalentNanoUSD)
	assert.Zero(t, *request.BilledRevenueEquivalentNanoUSD)
	attempt := loadCostAttempt(t, handle.AttemptID)
	assert.Equal(t, string(types.CostAttemptUnknown), attempt.Status)
	assert.Equal(t, "task_polling_timed_out", attempt.FailureCode)
	assert.Equal(t, string(types.CostProfitIncompleteCost), request.ProfitStatus)
}

func TestTaskPollingCostDefersSettlementUntilRevenueAdjustmentRecovers(t *testing.T) {
	truncate(t)

	config := validTokenCostConfig(types.CostTokenModeTotal, types.CostMeterUpstreamUsage)
	config.ChargeEvent = types.CostChargeTaskSucceeded
	task, handle := prepareTaskPollingCostAttempt(t, types.CostModePerToken, config)
	task.UserId = 11
	task.Quota = 250_000
	task.Status = model.TaskStatusSuccess
	task.Progress = "100%"
	require.NoError(t, model.DB.Create(task).Error)

	callbackName := "test:fail_async_revenue_adjustment"
	callbackRegistered := true
	require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Name == "CostAccountingRequest" {
			tx.AddError(fmt.Errorf("injected async revenue adjustment failure"))
		}
	}))
	t.Cleanup(func() {
		if callbackRegistered {
			_ = model.DB.Callback().Update().Remove(callbackName)
		}
	})

	result := &relaycommon.TaskInfo{
		Status:             string(model.TaskStatusSuccess),
		TotalTokens:        250_000,
		TotalTokensPresent: true,
	}
	settlePolledTaskCost(context.Background(), &taskPollingFetchAdaptor{}, task, result)

	require.NoError(t, model.DB.Callback().Update().Remove(callbackName))
	callbackRegistered = false
	attempt := loadCostAttempt(t, handle.AttemptID)
	assert.Equal(t, string(types.CostAttemptAwaitingMeter), attempt.Status)
	assert.Contains(t, attempt.ActualMeterJSON, `"total_tokens":250000`)
	request := loadCostRevenueRequest(t, handle.CostRequestID)
	assert.Equal(t, string(types.CostProfitIncompleteRevenue), request.ProfitStatus)
	assert.Equal(t, string(types.CostRevenuePending), request.RevenueStatus)
	assert.Nil(t, request.FinalUserQuota)

	summary, err := RecoverStaleCostAccounting(context.Background(), time.Now(), 10)
	require.NoError(t, err)
	assert.Equal(t, 1, summary.AwaitingSettled)
	attempt = loadCostAttempt(t, handle.AttemptID)
	assert.Equal(t, string(types.CostAttemptSettled), attempt.Status)
	request = loadCostRevenueRequest(t, handle.CostRequestID)
	require.NotNil(t, request.FinalUserQuota)
	assert.Equal(t, int64(task.Quota), *request.FinalUserQuota)
	assert.Equal(t, string(types.CostProfitComplete), request.ProfitStatus)
}

func TestFailedTaskCostDefersZeroConfirmationUntilRevenueAdjustmentRecovers(t *testing.T) {
	truncate(t)

	config := validPerRequestCostConfig()
	config.ChargeEvent = types.CostChargeTaskSucceeded
	task, handle := prepareTaskPollingCostAttempt(t, types.CostModePerRequest, config)
	task.Quota = 0
	task.Status = model.TaskStatusFailure
	task.Progress = "100%"
	require.NoError(t, model.DB.Create(task).Error)

	callbackName := "test:fail_failed_task_revenue_adjustment"
	callbackRegistered := true
	require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Name == "CostAccountingRequest" {
			tx.AddError(fmt.Errorf("injected failed task revenue adjustment failure"))
		}
	}))
	t.Cleanup(func() {
		if callbackRegistered {
			_ = model.DB.Callback().Update().Remove(callbackName)
		}
	})

	settlePolledTaskCost(context.Background(), nil, task, &relaycommon.TaskInfo{
		Status: string(model.TaskStatusFailure),
	})

	require.NoError(t, model.DB.Callback().Update().Remove(callbackName))
	callbackRegistered = false
	attempt := loadCostAttempt(t, handle.AttemptID)
	assert.Equal(t, string(types.CostAttemptAwaitingMeter), attempt.Status)
	assert.JSONEq(t, `{}`, attempt.ActualMeterJSON)
	request := loadCostRevenueRequest(t, handle.CostRequestID)
	assert.Equal(t, string(types.CostProfitIncompleteRevenue), request.ProfitStatus)
	assert.Equal(t, string(types.CostRevenuePending), request.RevenueStatus)
	assert.Nil(t, request.FinalUserQuota)

	summary, err := RecoverStaleCostAccounting(context.Background(), time.Now(), 10)
	require.NoError(t, err)
	assert.Equal(t, 1, summary.AwaitingSettled)
	attempt = loadCostAttempt(t, handle.AttemptID)
	assert.Equal(t, string(types.CostAttemptConfirmedZero), attempt.Status)
	request = loadCostRevenueRequest(t, handle.CostRequestID)
	assert.Equal(t, string(types.CostRevenueConfirmedZero), request.RevenueStatus)
	require.NotNil(t, request.FinalUserQuota)
	assert.Zero(t, *request.FinalUserQuota)
	assert.Equal(t, string(types.CostProfitComplete), request.ProfitStatus)
}
