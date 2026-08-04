package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	taskdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/samber/lo"
)

// TaskPollingAdaptor 定义轮询所需的最小适配器接口，避免 service -> relay 的循环依赖
type TaskPollingAdaptor interface {
	Init(info *relaycommon.RelayInfo)
	FetchTask(baseURL string, key string, body map[string]any, proxy string) (*http.Response, error)
	ParseTaskResult(body []byte) (*relaycommon.TaskInfo, error)
	// AdjustBillingOnComplete 在任务到达终态（成功/失败）时由轮询循环调用。
	// 返回正数触发差额结算（补扣/退还），返回 0 保持预扣费金额不变。
	AdjustBillingOnComplete(task *model.Task, taskResult *relaycommon.TaskInfo) int
}

type TaskPollingHTTPErrorParser interface {
	ParseTaskPollingHTTPError(body []byte, statusCode int) *relaycommon.TaskInfo
}

type taskCostMeterNormalizer interface {
	NormalizeTaskCostMeter(task *model.Task, result *relaycommon.TaskInfo) (types.CostMeter, error)
}

type taskPollingResponseData struct {
	TaskID     string           `json:"task_id"`
	Status     model.TaskStatus `json:"status"`
	FailReason string           `json:"fail_reason"`
	ResultURL  string           `json:"result_url"`
	Progress   string           `json:"progress"`
	Data       json.RawMessage  `json:"data"`
}

// GetTaskAdaptorFunc 由 main 包注入，用于获取指定平台的任务适配器。
// 打破 service -> relay -> relay/channel -> service 的循环依赖。
var GetTaskAdaptorFunc func(platform constant.TaskPlatform) TaskPollingAdaptor

const (
	refundReconciliationLimit       = 100
	refundReconciliationGracePeriod = 30 * time.Second
)

// sweepTimedOutTasks 在主轮询之前独立清理超时任务。
// 每次最多处理 100 条，剩余的下个周期继续处理。
// 使用 per-task CAS (UpdateWithStatus) 防止覆盖被正常轮询已推进的任务。
func sweepTimedOutTasks(ctx context.Context) {
	if constant.TaskTimeoutMinutes <= 0 {
		return
	}
	cutoff := time.Now().Unix() - int64(constant.TaskTimeoutMinutes)*60
	tasks := model.GetTimedOutUnfinishedTasks(cutoff, 100)
	if len(tasks) == 0 {
		return
	}

	reason := fmt.Sprintf("任务超时（%d分钟）", constant.TaskTimeoutMinutes)
	legacyReason := "任务超时（旧系统遗留任务，不进行退款，请联系管理员）"
	now := time.Now().Unix()
	timedOutCount := 0

	for _, task := range tasks {
		isLegacy := task.SubmitTime > 0 && task.SubmitTime < model.TaskRefundLegacyCutoff

		oldStatus := task.Status
		task.Status = model.TaskStatusFailure
		task.Progress = "100%"
		task.FinishTime = now
		if isLegacy {
			task.FailReason = legacyReason
			// 旧系统任务明确不退款，随终态 CAS 一并清掉 quota，
			// 避免留下可再次退款的计费状态。
			task.Quota = 0
		} else {
			task.FailReason = reason
		}
		if err := markTimedOutAsyncCostAttempt(ctx, task.PrivateData.CostRequestID, task); err != nil {
			logger.LogError(ctx, fmt.Sprintf("sweepTimedOutTasks cost preparation error for task %s: %v", task.TaskID, err))
			continue
		}

		won, err := task.UpdateWithStatus(oldStatus)
		if err != nil {
			logger.LogError(ctx, fmt.Sprintf("sweepTimedOutTasks CAS update error for task %s: %v", task.TaskID, err))
			continue
		}
		if !won {
			logger.LogInfo(ctx, fmt.Sprintf("sweepTimedOutTasks: task %s already transitioned, skip", task.TaskID))
			continue
		}
		timedOutCount++
		if !isLegacy && task.Quota != 0 {
			RefundTaskQuota(ctx, task, reason)
		}
	}

	if timedOutCount > 0 {
		logger.LogInfo(ctx, fmt.Sprintf("sweepTimedOutTasks: timed out %d tasks", timedOutCount))
	}
}

// sweepUnrefundedFailedTasks retries failed tasks that still carry a quota
// marker after their terminal transition. A short grace period avoids racing
// the normal polling path that performs the immediate refund.
func sweepUnrefundedFailedTasks(ctx context.Context) {
	updatedBefore := time.Now().Add(-refundReconciliationGracePeriod).Unix()
	tasks := model.GetUnrefundedFailedTasks(updatedBefore, refundReconciliationLimit)
	for _, task := range tasks {
		if ctx.Err() != nil {
			return
		}

		quota := task.Quota
		claimed, err := model.ClaimQuotaForRefund(task.ID, quota)
		if err != nil {
			logger.LogError(ctx, fmt.Sprintf("sweepUnrefundedFailedTasks claim error for task %s: %v", task.TaskID, err))
			continue
		}
		if !claimed {
			logger.LogDebug(ctx, "sweepUnrefundedFailedTasks: task %s claim lost, skip refund", task.TaskID)
			continue
		}

		if RefundTaskQuota(ctx, task, task.FailReason) {
			continue
		}

		restored, restoreErr := model.RestoreQuotaAfterFailedRefund(task.ID, quota)
		if restoreErr != nil {
			logger.LogError(ctx, fmt.Sprintf("sweepUnrefundedFailedTasks restore quota error for task %s: %v", task.TaskID, restoreErr))
		} else if !restored {
			logger.LogError(ctx, fmt.Sprintf("sweepUnrefundedFailedTasks could not restore quota marker for task %s", task.TaskID))
		}
	}
}

// TaskPollSummary is the result recorded on an async_task_poll system task row,
// summarizing one polling pass.
type TaskPollSummary struct {
	UnfinishedTasks  int `json:"unfinished_tasks"`
	PlatformsScanned int `json:"platforms_scanned"`
	NullTasksFailed  int `json:"null_tasks_failed"`
}

// RunTaskPollingOnce performs one async-task (Suno/video) polling pass
// synchronously. It honors ctx cancellation (the system-task runner cancels it
// when the lease is lost) and, when report is non-nil, reports progress as
// (processedPlatforms, totalPlatforms). It returns immediately if the task
// adaptor factory has not been wired yet, to avoid a nil call during startup.
func RunTaskPollingOnce(ctx context.Context, report func(processed, total int)) TaskPollSummary {
	summary := TaskPollSummary{}
	if GetTaskAdaptorFunc == nil {
		return summary
	}
	if ctx == nil {
		ctx = context.Background()
	}

	common.SysLog("任务进度轮询开始")
	sweepTimedOutTasks(ctx)
	sweepUnrefundedFailedTasks(ctx)
	allTasks := model.GetAllUnFinishSyncTasks(constant.TaskQueryLimit)
	summary.UnfinishedTasks = len(allTasks)
	platformTask := make(map[constant.TaskPlatform][]*model.Task)
	for _, t := range allTasks {
		platformTask[t.Platform] = append(platformTask[t.Platform], t)
	}

	totalPlatforms := len(platformTask)
	processedPlatforms := 0
	for platform, tasks := range platformTask {
		if ctx.Err() != nil {
			break
		}
		if report != nil {
			report(processedPlatforms, totalPlatforms)
		}
		processedPlatforms++
		if len(tasks) == 0 {
			continue
		}
		summary.PlatformsScanned++
		taskChannelM := make(map[int][]string)
		taskM := make(map[string]*model.Task)
		nullTaskIds := make([]int64, 0)
		for _, task := range tasks {
			upstreamID := task.GetUpstreamTaskID()
			if upstreamID == "" {
				// 统计失败的未完成任务
				nullTaskIds = append(nullTaskIds, task.ID)
				continue
			}
			taskM[upstreamID] = task
			taskChannelM[task.ChannelId] = append(taskChannelM[task.ChannelId], upstreamID)
		}
		if len(nullTaskIds) > 0 {
			summary.NullTasksFailed += len(nullTaskIds)
			err := model.TaskBulkUpdateByID(nullTaskIds, map[string]any{
				"status":   "FAILURE",
				"progress": "100%",
			})
			if err != nil {
				logger.LogError(ctx, fmt.Sprintf("Fix null task_id task error: %v", err))
			} else {
				logger.LogInfo(ctx, fmt.Sprintf("Fix null task_id task success: %v", nullTaskIds))
			}
		}
		if len(taskChannelM) == 0 {
			continue
		}

		DispatchPlatformUpdate(ctx, platform, taskChannelM, taskM)
	}
	if report != nil && ctx.Err() == nil {
		report(totalPlatforms, totalPlatforms)
	}
	common.SysLog("任务进度轮询完成")
	return summary
}

// DispatchPlatformUpdate 按平台分发轮询更新
func DispatchPlatformUpdate(ctx context.Context, platform constant.TaskPlatform, taskChannelM map[int][]string, taskM map[string]*model.Task) {
	if ctx == nil {
		ctx = context.Background()
	}
	switch platform {
	case constant.TaskPlatformMidjourney:
		// MJ 轮询由其自身处理，这里预留入口
	case constant.TaskPlatformSuno:
		_ = UpdateSunoTasks(ctx, taskChannelM, taskM)
	default:
		if err := UpdateVideoTasks(ctx, platform, taskChannelM, taskM); err != nil {
			common.SysLog(fmt.Sprintf("UpdateVideoTasks fail: %s", err))
		}
	}
}

// UpdateSunoTasks 按渠道更新所有 Suno 任务
func UpdateSunoTasks(ctx context.Context, taskChannelM map[int][]string, taskM map[string]*model.Task) error {
	for channelId, taskIds := range taskChannelM {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		err := updateSunoTasks(ctx, channelId, taskIds, taskM)
		if err != nil {
			logger.LogError(ctx, fmt.Sprintf("渠道 #%d 更新异步任务失败: %s", channelId, err.Error()))
		}
	}
	return nil
}

func updateSunoTasks(ctx context.Context, channelId int, taskIds []string, taskM map[string]*model.Task) error {
	logger.LogInfo(ctx, fmt.Sprintf("渠道 #%d 未完成的任务有: %d", channelId, len(taskIds)))
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if len(taskIds) == 0 {
		return nil
	}
	ch, err := model.CacheGetChannel(channelId)
	if err != nil {
		common.SysLog(fmt.Sprintf("CacheGetChannel: %v", err))
		// Collect DB primary key IDs for bulk update (taskIds are upstream IDs, not task_id column values)
		var failedIDs []int64
		for _, upstreamID := range taskIds {
			if t, ok := taskM[upstreamID]; ok {
				failedIDs = append(failedIDs, t.ID)
			}
		}
		err = model.TaskBulkUpdateByID(failedIDs, map[string]any{
			"fail_reason": fmt.Sprintf("获取渠道信息失败，请联系管理员，渠道ID：%d", channelId),
			"status":      "FAILURE",
			"progress":    "100%",
		})
		if err != nil {
			common.SysLog(fmt.Sprintf("UpdateSunoTask error: %v", err))
		}
		return err
	}
	adaptor := GetTaskAdaptorFunc(constant.TaskPlatformSuno)
	if adaptor == nil {
		return errors.New("adaptor not found")
	}
	proxy := ch.GetSetting().Proxy
	resp, err := adaptor.FetchTask(*ch.BaseURL, ch.Key, map[string]any{
		"ids": taskIds,
	}, proxy)
	if err != nil {
		common.SysLog(fmt.Sprintf("Get Task Do req error: %v", err))
		return err
	}
	if resp.StatusCode != http.StatusOK {
		logger.LogError(ctx, fmt.Sprintf("Get Task status code: %d", resp.StatusCode))
		return fmt.Errorf("Get Task status code: %d", resp.StatusCode)
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		common.SysLog(fmt.Sprintf("Get Suno Task parse body error: %v", err))
		return err
	}
	var responseItems taskdto.TaskResponse[[]taskdto.SunoDataResponse]
	err = common.Unmarshal(responseBody, &responseItems)
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("Get Suno Task parse body error2: %v, body: %s", err, string(responseBody)))
		return err
	}
	if !responseItems.IsSuccess() {
		common.SysLog(fmt.Sprintf("渠道 #%d 未完成的任务有: %d, 成功获取到任务数: %s", channelId, len(taskIds), string(responseBody)))
		return err
	}

	for _, responseItem := range responseItems.Data {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		task := taskM[responseItem.TaskID]
		if task == nil {
			logger.LogWarn(ctx, fmt.Sprintf("Suno task response ignored: unknown task_id=%s", responseItem.TaskID))
			continue
		}
		if !taskNeedsUpdate(task, responseItem) {
			continue
		}

		prevStatus := task.Status
		task.Status = lo.If(model.TaskStatus(responseItem.Status) != "", model.TaskStatus(responseItem.Status)).Else(task.Status)
		task.FailReason = lo.If(responseItem.FailReason != "", responseItem.FailReason).Else(task.FailReason)
		task.SubmitTime = lo.If(responseItem.SubmitTime != 0, responseItem.SubmitTime).Else(task.SubmitTime)
		task.StartTime = lo.If(responseItem.StartTime != 0, responseItem.StartTime).Else(task.StartTime)
		task.FinishTime = lo.If(responseItem.FinishTime != 0, responseItem.FinishTime).Else(task.FinishTime)
		isFailure := responseItem.FailReason != "" || task.Status == model.TaskStatusFailure
		if isFailure {
			logger.LogInfo(ctx, task.TaskID+" 构建失败，"+task.FailReason)
			task.Status = model.TaskStatusFailure
			task.Progress = "100%"
		}
		if responseItem.Status == model.TaskStatusSuccess {
			task.Progress = "100%"
		}
		task.Data = responseItem.Data
		terminal := task.Status == model.TaskStatusSuccess || task.Status == model.TaskStatusFailure
		result := &relaycommon.TaskInfo{Status: string(task.Status), Reason: task.FailReason}
		if terminal {
			if err := preparePolledTaskCostSettlement(ctx, adaptor, task, result); err != nil {
				logger.LogError(ctx, fmt.Sprintf("prepare Suno task cost settlement failed for task %s: %v", task.TaskID, err))
				continue
			}
		}

		// 持久化走 CAS，防止重叠轮询/sweep/多实例/持久化失败重试导致重复退款或覆盖终态。
		won, err := task.UpdateWithStatus(prevStatus)
		if err != nil {
			logger.LogError(ctx, fmt.Sprintf("UpdateSunoTask task %s error: %v", task.TaskID, err))
		} else if !won {
			logger.LogWarn(ctx, fmt.Sprintf("Task %s CAS lost or no-op update, skip billing", task.TaskID))
		} else {
			if isFailure && prevStatus != model.TaskStatusFailure && task.Quota != 0 {
				RefundTaskQuota(ctx, task, task.FailReason)
			}
			if terminal {
				settlePolledTaskCost(ctx, adaptor, task, result)
			}
		}
	}
	return nil
}

// taskNeedsUpdate 检查 Suno 任务是否需要更新
func taskNeedsUpdate(oldTask *model.Task, newTask taskdto.SunoDataResponse) bool {
	if oldTask.SubmitTime != newTask.SubmitTime {
		return true
	}
	if oldTask.StartTime != newTask.StartTime {
		return true
	}
	if oldTask.FinishTime != newTask.FinishTime {
		return true
	}
	if string(oldTask.Status) != newTask.Status {
		return true
	}
	if oldTask.FailReason != newTask.FailReason {
		return true
	}

	if (oldTask.Status == model.TaskStatusFailure || oldTask.Status == model.TaskStatusSuccess) && oldTask.Progress != "100%" {
		return true
	}

	oldData, _ := common.Marshal(oldTask.Data)
	newData, _ := common.Marshal(newTask.Data)

	sort.Slice(oldData, func(i, j int) bool {
		return oldData[i] < oldData[j]
	})
	sort.Slice(newData, func(i, j int) bool {
		return newData[i] < newData[j]
	})

	if string(oldData) != string(newData) {
		return true
	}
	return false
}

// UpdateVideoTasks 按渠道更新所有视频任务
func UpdateVideoTasks(ctx context.Context, platform constant.TaskPlatform, taskChannelM map[int][]string, taskM map[string]*model.Task) error {
	channelIDs := make([]int, 0, len(taskChannelM))
	for channelID := range taskChannelM {
		channelIDs = append(channelIDs, channelID)
	}
	sort.Ints(channelIDs)

	var wg sync.WaitGroup
	for _, channelId := range channelIDs {
		taskIds := taskChannelM[channelId]
		if len(taskIds) == 0 {
			continue
		}
		taskIds = append([]string(nil), taskIds...)

		wg.Add(1)
		gopool.Go(func() {
			defer wg.Done()
			if err := updateVideoTasks(ctx, platform, channelId, taskIds, taskM); err != nil {
				logger.LogError(ctx, fmt.Sprintf("Channel #%d failed to update video async tasks: %s", channelId, err.Error()))
			}
		})
	}
	wg.Wait()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return nil
}

func updateVideoTasks(ctx context.Context, platform constant.TaskPlatform, channelId int, taskIds []string, taskM map[string]*model.Task) error {
	logger.LogInfo(ctx, fmt.Sprintf("Channel #%d pending video tasks: %d", channelId, len(taskIds)))
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if len(taskIds) == 0 {
		return nil
	}
	cacheGetChannel, err := model.CacheGetChannel(channelId)
	if err != nil {
		// Collect DB primary key IDs for bulk update (taskIds are upstream IDs, not task_id column values)
		var failedIDs []int64
		for _, upstreamID := range taskIds {
			if t, ok := taskM[upstreamID]; ok {
				failedIDs = append(failedIDs, t.ID)
			}
		}
		errUpdate := model.TaskBulkUpdateByID(failedIDs, map[string]any{
			"fail_reason": fmt.Sprintf("Failed to get channel info, channel ID: %d", channelId),
			"status":      "FAILURE",
			"progress":    "100%",
		})
		if errUpdate != nil {
			common.SysLog(fmt.Sprintf("UpdateVideoTask error: %v", errUpdate))
		}
		return fmt.Errorf("CacheGetChannel failed: %w", err)
	}
	adaptor := GetTaskAdaptorFunc(platform)
	if adaptor == nil {
		return fmt.Errorf("video adaptor not found")
	}
	info := &relaycommon.RelayInfo{}
	info.ChannelMeta = &relaycommon.ChannelMeta{
		ChannelType:          cacheGetChannel.Type,
		ChannelBaseUrl:       cacheGetChannel.GetBaseURL(),
		ChannelOtherSettings: cacheGetChannel.GetOtherSettings(),
	}
	info.ApiKey = cacheGetChannel.Key
	adaptor.Init(info)
	disablePollingSleep := cacheGetChannel.GetOtherSettings().DisableTaskPollingSleep
	for i, taskId := range taskIds {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := updateVideoSingleTask(ctx, adaptor, cacheGetChannel, taskId, taskM); err != nil {
			publicTaskID := "[unknown]"
			if task := taskM[taskId]; task != nil {
				publicTaskID = task.TaskID
			}
			logger.LogError(ctx, fmt.Sprintf("Failed to update video task %s: %s", publicTaskID, sanitizeTaskPollingText(err.Error(), taskId)))
		}
		if disablePollingSleep || i == len(taskIds)-1 {
			continue
		}

		// sleep 1 second between tasks for this channel only.
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(1 * time.Second):
		}
	}
	return nil
}

func sanitizeTaskPollingText(value string, privateTaskID string) string {
	value = common.MaskSensitiveInfo(value)
	if privateTaskID != "" {
		value = strings.ReplaceAll(value, privateTaskID, "[redacted]")
	}
	return value
}

func updateVideoSingleTask(ctx context.Context, adaptor TaskPollingAdaptor, ch *model.Channel, taskId string, taskM map[string]*model.Task) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	baseURL := constant.ChannelBaseURLs[ch.Type]
	if ch.GetBaseURL() != "" {
		baseURL = ch.GetBaseURL()
	}
	proxy := ch.GetSetting().Proxy

	task := taskM[taskId]
	if task == nil {
		logger.LogError(ctx, "Task not found in task map")
		return errors.New("task not found in task map")
	}
	privateTaskID := task.GetUpstreamTaskID()
	key := ch.Key

	privateData := task.PrivateData
	if privateData.Key != "" {
		key = privateData.Key
	}
	resp, err := adaptor.FetchTask(baseURL, key, map[string]any{
		"task_id": task.GetUpstreamTaskID(),
		"action":  task.Action,
	}, proxy)
	if err != nil {
		return fmt.Errorf("fetchTask failed for task %s: %s", task.TaskID, sanitizeTaskPollingText(err.Error(), privateTaskID))
	}
	if resp == nil || resp.Body == nil {
		return fmt.Errorf("fetchTask returned an empty response for task %s", task.TaskID)
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("readAll failed for task %s: %s", task.TaskID, sanitizeTaskPollingText(err.Error(), privateTaskID))
	}

	logger.LogDebug(ctx, "updateVideoSingleTask response: status=%d bytes=%d", resp.StatusCode, len(responseBody))

	snap := task.Snapshot()
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= http.StatusInternalServerError {
		return fmt.Errorf("retryable polling HTTP status %d for task %s", resp.StatusCode, task.TaskID)
	}

	var taskResult *relaycommon.TaskInfo
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		if parser, ok := adaptor.(TaskPollingHTTPErrorParser); ok {
			taskResult = parser.ParseTaskPollingHTTPError(responseBody, resp.StatusCode)
		}
		if taskResult == nil {
			taskResult = relaycommon.FailTaskInfo(fmt.Sprintf("upstream returned HTTP %d", resp.StatusCode))
			taskResult.ErrorCode = strconv.Itoa(resp.StatusCode)
		}
	} else {
		taskResult, err = adaptor.ParseTaskResult(responseBody)
		if err != nil || taskResult == nil || taskResult.Status == "" {
			parseErr := err
			if parseErr == nil {
				parseErr = fmt.Errorf("upstream returned empty task status")
			}
			var wrapper taskdto.TaskResponse[taskPollingResponseData]
			if wrapperErr := common.Unmarshal(responseBody, &wrapper); wrapperErr != nil || !wrapper.IsSuccess() {
				return fmt.Errorf("parseTaskResult failed for task %s: %s", task.TaskID, sanitizeTaskPollingText(parseErr.Error(), privateTaskID))
			}
			taskResult = &relaycommon.TaskInfo{
				TaskID:   wrapper.Data.TaskID,
				Status:   string(wrapper.Data.Status),
				Url:      wrapper.Data.ResultURL,
				Progress: wrapper.Data.Progress,
				Reason:   wrapper.Data.FailReason,
			}
		}
	}
	taskResult.Reason = sanitizeTaskPollingText(taskResult.Reason, privateTaskID)
	taskResult.ErrorCode = sanitizeTaskPollingText(taskResult.ErrorCode, privateTaskID)
	if model.TaskStatus(taskResult.Status) == model.TaskStatusSuccess {
		if err := NormalizeSeedanceTaskUsage(task, taskResult); err != nil {
			logger.LogError(ctx, fmt.Sprintf(
				"Seedance task usage normalization failed: task_id=%s channel_id=%d cost_request_id=%d error=%s",
				task.TaskID, task.ChannelId, task.PrivateData.CostRequestID,
				sanitizeTaskPollingText(err.Error(), privateTaskID),
			))
			return fmt.Errorf("normalize Seedance task usage for task %s: %s", task.TaskID, sanitizeTaskPollingText(err.Error(), privateTaskID))
		}
	}

	task.Data = redactVideoResponseBody(responseBody)

	logger.LogDebug(ctx, "updateVideoSingleTask result: task_id=%s status=%s progress=%s", task.TaskID, sanitizeTaskPollingText(taskResult.Status, privateTaskID), sanitizeTaskPollingText(taskResult.Progress, privateTaskID))

	now := time.Now().Unix()

	shouldRefund := false
	shouldSettle := false
	terminalCASWinner := false
	quota := task.Quota

	task.Status = model.TaskStatus(taskResult.Status)
	switch taskResult.Status {
	case model.TaskStatusSubmitted:
		task.Progress = taskcommon.ProgressSubmitted
	case model.TaskStatusQueued:
		task.Progress = taskcommon.ProgressQueued
	case model.TaskStatusInProgress:
		task.Progress = taskcommon.ProgressInProgress
		if task.StartTime == 0 {
			task.StartTime = now
		}
	case model.TaskStatusSuccess:
		task.Progress = taskcommon.ProgressComplete
		if task.FinishTime == 0 {
			task.FinishTime = now
		}
		if strings.HasPrefix(taskResult.Url, "data:") {
			// data: URI (e.g. Vertex base64 encoded video) — keep in Data, not in ResultURL
			task.PrivateData.ResultURL = taskcommon.BuildProxyURL(task.TaskID)
		} else if taskResult.Url != "" {
			// Direct upstream URL (e.g. Kling, Ali, Doubao, etc.)
			task.PrivateData.ResultURL = taskResult.Url
		} else {
			// No URL from adaptor — construct proxy URL using public task ID
			task.PrivateData.ResultURL = taskcommon.BuildProxyURL(task.TaskID)
		}
		shouldSettle = true
	case model.TaskStatusFailure:
		task.Status = model.TaskStatusFailure
		task.Progress = taskcommon.ProgressComplete
		if task.FinishTime == 0 {
			task.FinishTime = now
		}
		task.FailReason = taskResult.Reason
		logger.LogInfo(ctx, fmt.Sprintf("Task %s failed: %s", task.TaskID, task.FailReason))
		taskResult.Progress = taskcommon.ProgressComplete
		if quota != 0 {
			shouldRefund = true
		}
	default:
		return fmt.Errorf("unknown task status %s for task %s", sanitizeTaskPollingText(taskResult.Status, privateTaskID), task.TaskID)
	}
	if taskResult.Progress != "" {
		task.Progress = taskResult.Progress
	}
	isDone := task.Status == model.TaskStatusSuccess || task.Status == model.TaskStatusFailure
	if isDone && snap.Status != task.Status {
		if err := preparePolledTaskCostSettlement(ctx, adaptor, task, taskResult); err != nil {
			return fmt.Errorf("prepare task cost settlement for task %s: %s", task.TaskID, sanitizeTaskPollingText(err.Error(), privateTaskID))
		}
		won, err := task.UpdateWithStatus(snap.Status)
		if err != nil {
			logger.LogError(ctx, fmt.Sprintf("UpdateWithStatus failed for task %s: %s", task.TaskID, err.Error()))
			shouldRefund = false
			shouldSettle = false
		} else if !won {
			logger.LogWarn(ctx, fmt.Sprintf("Task %s CAS lost or no-op update, skip billing", task.TaskID))
			shouldRefund = false
			shouldSettle = false
		} else {
			terminalCASWinner = true
		}
	} else if !snap.Equal(task.Snapshot()) {
		if _, err := task.UpdateWithStatus(snap.Status); err != nil {
			logger.LogError(ctx, fmt.Sprintf("Failed to update task %s: %s", task.TaskID, err.Error()))
		}
	} else {
		// No changes, skip update
		logger.LogDebug(ctx, "No update needed for task %s", task.TaskID)
	}

	if shouldSettle {
		settleTaskBillingOnComplete(ctx, adaptor, task, taskResult)
	}
	if shouldRefund {
		RefundTaskQuota(ctx, task, task.FailReason)
	}
	if terminalCASWinner {
		settlePolledTaskCost(ctx, adaptor, task, taskResult)
		persistPolledTerminalTaskUserResponse(ctx, adaptor, task)
	}

	return nil
}

func settlePolledTaskCost(ctx context.Context, adaptor TaskPollingAdaptor, task *model.Task, result *relaycommon.TaskInfo) {
	if task == nil || task.PrivateData.CostRequestID == 0 || result == nil {
		return
	}
	if err := preparePolledTaskCostSettlement(ctx, adaptor, task, result); err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("record pending task cost settlement failed: task_id=%s cost_request_id=%d error=%v", task.TaskID, task.PrivateData.CostRequestID, err))
		return
	}
	if err := recognizeAsyncBilledRevenue(ctx, task.PrivateData.CostRequestID, task.Quota); err != nil &&
		!errors.Is(err, errAsyncRevenueManualReconciliation) {
		logger.LogWarn(ctx, fmt.Sprintf("recognize task cost revenue failed: task_id=%s cost_request_id=%d error=%v", task.TaskID, task.PrivateData.CostRequestID, err))
		return
	}
	if err := SettleAsyncCostAttempt(ctx, task.PrivateData.CostRequestID, task, result); err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("settle task cost failed: task_id=%s cost_request_id=%d error=%v", task.TaskID, task.PrivateData.CostRequestID, err))
	}
}

func preparePolledTaskCostSettlement(ctx context.Context, adaptor TaskPollingAdaptor, task *model.Task, result *relaycommon.TaskInfo) error {
	if task == nil || task.PrivateData.CostRequestID == 0 || result == nil {
		return nil
	}
	if model.TaskStatus(result.Status) == model.TaskStatusSuccess {
		normalizer, ok := adaptor.(taskCostMeterNormalizer)
		if ok {
			normalizerResult := result
			if result.UsageSource == model.TaskUsageSourceLocalCalculated {
				resultCopy := *result
				resultCopy.CompletionTokens = 0
				resultCopy.TotalTokens = 0
				resultCopy.CompletionTokensPresent = false
				resultCopy.TotalTokensPresent = false
				normalizerResult = &resultCopy
			}
			meter, err := normalizer.NormalizeTaskCostMeter(task, normalizerResult)
			if err != nil {
				logger.LogWarn(ctx, fmt.Sprintf("normalize task cost meter failed: task_id=%s cost_request_id=%d error=%v", task.TaskID, task.PrivateData.CostRequestID, err))
			} else {
				result.CostMeter = &meter
			}
		}
	}
	return recordPendingAsyncCostSettlement(ctx, task.PrivateData.CostRequestID, task, result)
}

func redactVideoResponseBody(body []byte) []byte {
	var m map[string]any
	if err := common.Unmarshal(body, &m); err != nil {
		return body
	}
	resp, _ := m["response"].(map[string]any)
	if resp != nil {
		delete(resp, "bytesBase64Encoded")
		if v, ok := resp["video"].(string); ok {
			resp["video"] = truncateBase64(v)
		}
		if vs, ok := resp["videos"].([]any); ok {
			for i := range vs {
				if vm, ok := vs[i].(map[string]any); ok {
					delete(vm, "bytesBase64Encoded")
				}
			}
		}
	}
	b, err := common.Marshal(m)
	if err != nil {
		return body
	}
	return b
}

func truncateBase64(s string) string {
	const maxKeep = 256
	if len(s) <= maxKeep {
		return s
	}
	return s[:maxKeep] + "..."
}

// settleTaskBillingOnComplete 任务完成时的统一计费调整。
// 优先级：1. Seedance 使用冻结官方 Token 合同与 total_tokens 结算
//  2. adaptor.AdjustBillingOnComplete 返回正数 → 使用 adaptor 计算的额度
//  3. 通用任务使用权威 completion_tokens（或兼容回退 total_tokens）重算
//  4. 都不满足 → 保持预扣额度不变
func settleTaskBillingOnComplete(ctx context.Context, adaptor TaskPollingAdaptor, task *model.Task, taskResult *relaycommon.TaskInfo) {
	if bc := task.PrivateData.BillingContext; bc != nil {
		if bc.BillingMode == billing_setting.BillingModePerDuration {
			logger.LogInfo(ctx, fmt.Sprintf("任务 %s 按时长计费，跳过 token 差额结算", task.TaskID))
			return
		}
		if bc.PerCallBilling && bc.BillingMode != billing_setting.BillingModeSeedanceTokens {
			logger.LogInfo(ctx, fmt.Sprintf("任务 %s 按次计费，跳过差额结算", task.TaskID))
			return
		}
	}
	// 1. 优先让 adaptor 决定最终额度
	actualQuota := adaptor.AdjustBillingOnComplete(task, taskResult)
	if bc := task.PrivateData.BillingContext; bc != nil && bc.BillingMode == billing_setting.BillingModeSeedanceTokens {
		if err := recalculateSeedanceTaskQuota(ctx, task, taskResult.BillingClamp); err != nil {
			logger.LogError(ctx, fmt.Sprintf("任务 %s Seedance 官方 Token 结算失败: %s", task.TaskID, err.Error()))
		}
		return
	}
	tokens, hasTokens, clamp := taskBillingTokensChecked(taskResult)
	if hasTokens && task.PrivateData.BillingContext != nil {
		task.PrivateData.BillingContext.BillingTokens = tokens
	}
	if actualQuota > 0 {
		if task.ID > 0 && task.PrivateData.BillingContext != nil {
			if err := task.Update(); err != nil {
				logger.LogWarn(ctx, fmt.Sprintf("任务 %s billing token 回写失败: %s", task.TaskID, err.Error()))
			}
		}
		if hasTokens {
			RecalculateTaskQuotaWithTokens(ctx, task, actualQuota, tokens, "adaptor计费调整", clamp)
		} else {
			RecalculateTaskQuota(ctx, task, actualQuota, "adaptor计费调整", clamp)
		}
		return
	}
	// Persist any terminal billing-context normalization (resolution or draft
	// ratio removal) before the token settlement reads it.
	if task.ID > 0 && task.PrivateData.BillingContext != nil {
		if err := task.Update(); err != nil {
			logger.LogWarn(ctx, fmt.Sprintf("任务 %s 计费上下文回写失败: %s", task.TaskID, err.Error()))
		}
	}
	// 2. Use completion_tokens when the provider included that field, even when
	// its explicit value is zero. Otherwise fall back to total_tokens.
	if hasTokens {
		RecalculateTaskQuotaByTokens(ctx, task, tokens, clamp)
		return
	}
	// 3. 无调整，保持预扣额度
}

// taskBillingTokens returns the bounded token count used by task settlement.
// It preserves the compact helper contract used by existing callers; the
// checked variant additionally reports field presence and saturation.
func taskBillingTokens(taskResult *relaycommon.TaskInfo) int {
	tokens, _, _ := taskBillingTokensChecked(taskResult)
	return tokens
}

func taskBillingTokensChecked(taskResult *relaycommon.TaskInfo) (int, bool, *common.QuotaClamp) {
	if taskResult == nil {
		return 0, false, nil
	}
	tokens := taskResult.TotalTokens
	switch {
	case taskResult.CompletionTokensPresent:
		tokens = taskResult.CompletionTokens
	case taskResult.TotalTokensPresent:
	case taskResult.TotalTokens != 0:
	case taskResult.CompletionTokens > 0:
		tokens = taskResult.CompletionTokens
	default:
		return 0, false, nil
	}
	if tokens < 0 {
		common.SysError("negative async task billing tokens; clamped to zero")
		clamp := &common.QuotaClamp{Op: "TaskBillingTokens", Kind: common.QuotaClampUnderflow, Original: float64(tokens), Clamped: 0}
		taskResult.BillingClamp = clamp
		return 0, true, clamp
	}
	if tokens > common.MaxQuota {
		common.SysError("async task billing tokens exceed quota bound; clamped")
		clamp := &common.QuotaClamp{Op: "TaskBillingTokens", Kind: common.QuotaClampOverflow, Original: float64(tokens), Clamped: common.MaxQuota}
		taskResult.BillingClamp = clamp
		return common.MaxQuota, true, clamp
	}
	return tokens, true, taskResult.BillingClamp
}
