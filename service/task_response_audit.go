package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
)

const maxTaskUserResponseAuditBytes = 1 << 20

type taskOpenAIVideoConverter interface {
	ConvertToOpenAIVideo(task *model.Task) ([]byte, error)
}

type taskArkVideoConverter interface {
	ConvertToArkVideoTask(task *model.Task) ([]byte, error)
}

func PersistTerminalTaskUserResponse(ctx context.Context, task *model.Task, responseBody []byte) {
	if task == nil || len(responseBody) == 0 {
		return
	}
	if task.Status != model.TaskStatusSuccess && task.Status != model.TaskStatusFailure {
		return
	}
	if len(responseBody) > maxTaskUserResponseAuditBytes {
		logger.LogWarn(ctx, fmt.Sprintf("skip terminal task user response audit payload because it exceeds %d bytes: task_id=%s", maxTaskUserResponseAuditBytes, task.TaskID))
		return
	}
	if err := model.UpdateTaskUserResponseData(task.ID, json.RawMessage(bytes.Clone(responseBody))); err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("persist terminal task user response audit payload failed: task_id=%s error=%v", task.TaskID, err))
	}
}

func persistPolledTerminalTaskUserResponse(ctx context.Context, adaptor TaskPollingAdaptor, task *model.Task) {
	if task == nil || adaptor == nil {
		return
	}

	requestPath := strings.TrimSpace(task.Properties.RequestPath)
	var (
		responseBody []byte
		err          error
	)
	if IsSeedanceTask(task) {
		converter, ok := adaptor.(taskArkVideoConverter)
		if !ok {
			return
		}
		responseBody, err = converter.ConvertToArkVideoTask(task)
		if err == nil {
			var response map[string]interface{}
			if err = common.Unmarshal(responseBody, &response); err == nil {
				err = NormalizeSeedanceTaskResponse(task, response)
			}
			if err == nil {
				responseBody, err = common.Marshal(response)
			}
		}
		if err != nil {
			logger.LogWarn(ctx, fmt.Sprintf("build terminal task user response audit payload failed: task_id=%s request_path=%s error=%v", task.TaskID, requestPath, err))
			return
		}
		PersistTerminalTaskUserResponse(ctx, task, responseBody)
		return
	}
	switch {
	case strings.HasPrefix(requestPath, "/v1/video/generations"), strings.HasPrefix(requestPath, "/v1/videos"):
		converter, ok := adaptor.(taskOpenAIVideoConverter)
		if !ok {
			return
		}
		responseBody, err = converter.ConvertToOpenAIVideo(task)
	case strings.HasPrefix(requestPath, "/api/v3/contents/generations/tasks"):
		converter, ok := adaptor.(taskArkVideoConverter)
		if !ok {
			return
		}
		responseBody, err = converter.ConvertToArkVideoTask(task)
	default:
		return
	}
	if err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("build terminal task user response audit payload failed: task_id=%s request_path=%s error=%v", task.TaskID, requestPath, err))
		return
	}
	PersistTerminalTaskUserResponse(ctx, task, responseBody)
}
