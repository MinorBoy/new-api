package relay

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func SeedanceTaskCancel(c *gin.Context) ([]byte, *dto.TaskError) {
	if c == nil {
		return nil, service.TaskErrorWrapperLocal(errors.New("request context is required"), "invalid_request", http.StatusBadRequest)
	}
	taskID := strings.TrimSpace(c.Param("task_id"))
	task, exists, err := model.GetByTaskId(c.GetInt("id"), taskID)
	if err != nil {
		return nil, service.TaskErrorWrapper(err, "get_task_failed", http.StatusInternalServerError)
	}
	if !exists || !service.IsSeedanceTask(task) {
		return nil, service.TaskErrorWrapperLocal(errors.New("task_not_exist"), "task_not_exist", http.StatusNotFound)
	}
	if !seedanceTaskCancellable(task.Status) {
		return nil, service.TaskErrorWrapperLocal(errors.New("task_not_cancellable"), "task_not_cancellable", http.StatusConflict)
	}

	adaptor := GetTaskAdaptor(task.Platform)
	cancellationAdaptor, ok := adaptor.(channel.TaskCancellationAdaptor)
	if !ok {
		return nil, service.TaskErrorWrapperLocal(errors.New("task_cancellation_not_supported"), "task_cancellation_not_supported", http.StatusMethodNotAllowed)
	}
	channelModel, err := model.GetChannelById(task.ChannelId, true)
	if err != nil {
		return nil, service.TaskErrorWrapper(err, "channel_not_found", http.StatusInternalServerError)
	}
	key := strings.TrimSpace(task.PrivateData.Key)
	if key == "" {
		key = channelModel.Key
	}
	response, err := cancellationAdaptor.CancelTask(c.Request.Context(), channelModel.GetBaseURL(), key, task.GetUpstreamTaskID(), channelModel.GetSetting().Proxy)
	if err != nil {
		return nil, service.TaskErrorWrapper(err, "task_cancellation_failed", http.StatusBadGateway)
	}
	if response == nil {
		return nil, service.TaskErrorWrapperLocal(errors.New("upstream cancellation response is empty"), "task_cancellation_failed", http.StatusBadGateway)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, service.TaskErrorWrapperLocal(fmt.Errorf("upstream cancellation returned HTTP %d", response.StatusCode), "task_cancellation_failed", http.StatusBadGateway)
	}

	task.Status = model.TaskStatusFailure
	task.Progress = "100%"
	task.FailReason = "task canceled"
	task.FinishTime = time.Now().Unix()
	task.PrivateData.UpstreamTaskID = ""
	task.PrivateData.Key = ""
	won, err := task.UpdateWithStatuses([]model.TaskStatus{
		model.TaskStatusNotStart,
		model.TaskStatusSubmitted,
		model.TaskStatusQueued,
		model.TaskStatusInProgress,
	})
	if err != nil {
		return nil, service.TaskErrorWrapper(err, "update_task_failed", http.StatusInternalServerError)
	}
	if !won {
		return nil, service.TaskErrorWrapperLocal(errors.New("task_state_changed"), "task_state_changed", http.StatusConflict)
	}
	service.RefundTaskQuota(c.Request.Context(), task, task.FailReason)

	responseBody, err := seedanceTaskResponse(task)
	if err != nil {
		return nil, service.TaskErrorWrapper(err, "marshal_response_failed", http.StatusInternalServerError)
	}
	body, err := common.Marshal(responseBody)
	if err != nil {
		return nil, service.TaskErrorWrapper(err, "marshal_response_failed", http.StatusInternalServerError)
	}
	return body, nil
}

func seedanceTaskCancellable(status model.TaskStatus) bool {
	switch status {
	case model.TaskStatusNotStart, model.TaskStatusSubmitted, model.TaskStatusQueued, model.TaskStatusInProgress:
		return true
	default:
		return false
	}
}
