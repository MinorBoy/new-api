package service

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
)

func ProjectPublicTask(task *model.Task) *dto.PublicTaskDto {
	if task == nil {
		return nil
	}

	requestModel := publicTaskModel(task)
	public := &dto.PublicTaskDto{
		CreatedAt:    task.CreatedAt,
		UpdatedAt:    task.UpdatedAt,
		TaskID:       task.TaskID,
		Quota:        task.Quota,
		Action:       task.Action,
		Status:       string(task.Status),
		SubmitTime:   task.SubmitTime,
		StartTime:    task.StartTime,
		FinishTime:   task.FinishTime,
		Progress:     task.Progress,
		RequestModel: requestModel,
	}
	if task.Status == model.TaskStatusFailure {
		public.FailReason = "task failed"
	}
	public.UserResponseData = projectPublicTaskResult(task, requestModel)
	return public
}

func publicTaskModel(task *model.Task) string {
	if task == nil {
		return ""
	}
	if modelName := strings.TrimSpace(task.Properties.OriginModelName); modelName != "" {
		return modelName
	}
	if task.PrivateData.BillingContext != nil {
		if modelName := strings.TrimSpace(task.PrivateData.BillingContext.OriginModelName); modelName != "" {
			return modelName
		}
	}
	if len(task.PrivateData.UserRequestData) > 0 {
		var request struct {
			Model string `json:"model"`
		}
		if common.Unmarshal(task.PrivateData.UserRequestData, &request) == nil {
			return strings.TrimSpace(request.Model)
		}
	}
	return ""
}

func projectPublicTaskResult(task *model.Task, requestModel string) *dto.PublicTaskResult {
	if task == nil || !IsSeedanceTask(task) {
		return nil
	}
	if task.Status != model.TaskStatusSuccess && task.Status != model.TaskStatusFailure {
		return nil
	}

	response := make(map[string]interface{})
	if len(task.PrivateData.UserResponseData) > 0 {
		if err := common.Unmarshal(task.PrivateData.UserResponseData, &response); err != nil {
			response = make(map[string]interface{})
		}
	}
	if task.Status == model.TaskStatusSuccess {
		response["content"] = map[string]interface{}{
			"video_url": taskcommon.BuildProxyURL(task.TaskID),
		}
	}
	if err := NormalizeSeedanceTaskResponse(task, response); err != nil {
		common.SysError("failed to normalize public task result: task_id=" + task.TaskID)
		return nil
	}

	payload, err := common.Marshal(response)
	if err != nil {
		common.SysError("failed to serialize public task result: task_id=" + task.TaskID)
		return nil
	}
	result := &dto.PublicTaskResult{}
	if err := common.Unmarshal(payload, result); err != nil {
		common.SysError("failed to project public task result: task_id=" + task.TaskID)
		return nil
	}

	result.ID = task.TaskID
	result.Model = requestModel
	result.Status = SeedanceTaskStatus(task.Status)
	if task.Status == model.TaskStatusSuccess {
		result.Content = &dto.PublicTaskContent{
			VideoURL: taskcommon.BuildProxyURL(task.TaskID),
		}
		result.Error = nil
	} else {
		result.Content = nil
		result.Error = &dto.PublicTaskError{
			Code:    "task_failed",
			Message: "task failed",
		}
	}
	return result
}
