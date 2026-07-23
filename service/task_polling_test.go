package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/bytedance/gopkg/util/gopool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type taskPollingFetchAdaptor struct {
	mu           sync.Mutex
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

func (a *taskPollingFetchAdaptor) Init(_ *relaycommon.RelayInfo) {}

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

	response := dto.TaskResponse[model.Task]{
		Code: dto.TaskSuccessCode,
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

func runSinglePollingUpdate(t *testing.T, adaptor TaskPollingAdaptor, task *model.Task) error {
	t.Helper()
	upstreamID := task.GetUpstreamTaskID()
	return updateVideoSingleTask(context.Background(), adaptor, &model.Channel{
		Type: constant.ChannelTypeKling,
		Key:  "sk-test",
	}, upstreamID, map[string]*model.Task{upstreamID: task})
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
