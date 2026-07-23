package newapivideo

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/shopspring/decimal"
)

type parsedTask struct {
	Status                  model.TaskStatus
	Progress                string
	URL                     string
	Reason                  string
	ErrorCode               string
	CreatedAt               int64
	UpdatedAt               int64
	Nested                  *arkTaskData
	Usage                   *tokenUsage
	CompletionTokens        int
	CompletionTokensPresent bool
	TotalTokens             int
	TotalTokensPresent      bool
	BillingClamp            *common.QuotaClamp
}

func (a *TaskAdaptor) ParseTaskResult(body []byte) (*relaycommon.TaskInfo, error) {
	parsed, err := parseTaskProjection(body)
	if err != nil {
		return nil, err
	}
	return &relaycommon.TaskInfo{
		Code:                    0,
		Status:                  string(parsed.Status),
		Reason:                  parsed.Reason,
		ErrorCode:               parsed.ErrorCode,
		Url:                     parsed.URL,
		Progress:                parsed.Progress,
		CompletionTokens:        parsed.CompletionTokens,
		TotalTokens:             parsed.TotalTokens,
		Resolution:              parsed.Nested.Resolution,
		CompletionTokensPresent: parsed.CompletionTokensPresent,
		TotalTokensPresent:      parsed.TotalTokensPresent,
		BillingClamp:            parsed.BillingClamp,
	}, nil
}

func (a *TaskAdaptor) ParseTaskPollingHTTPError(body []byte, statusCode int) *relaycommon.TaskInfo {
	if statusCode == http.StatusNotFound || statusCode == http.StatusGone {
		result := relaycommon.FailTaskInfo("task not found or expired")
		result.ErrorCode = strconv.Itoa(statusCode)
		return result
	}
	if statusCode < http.StatusBadRequest || statusCode >= http.StatusInternalServerError {
		return nil
	}

	var response upstreamErrorEnvelope
	code, message := strconv.Itoa(statusCode), fmt.Sprintf("upstream returned HTTP %d", statusCode)
	if err := common.Unmarshal(body, &response); err == nil {
		if response.Code != "" {
			code = response.Code
		}
		if response.Message != "" {
			message = response.Message
		}
		if response.Error != nil {
			if response.Error.Code != "" {
				code = response.Error.Code
			}
			if response.Error.Message != "" {
				message = response.Error.Message
			}
		}
	}
	result := relaycommon.FailTaskInfo(message)
	result.ErrorCode = code
	return result
}

func parseTaskProjection(body []byte) (*parsedTask, error) {
	var envelope detailedEnvelope
	if err := common.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("unmarshal new-api video task response: %w", err)
	}

	var parsed *parsedTask
	var err error
	if envelope.Code != nil {
		parsed, err = parseDetailedTask(envelope)
	} else {
		parsed, err = parseDirectTask(body, envelope.Message)
	}
	if err != nil {
		return nil, err
	}
	if err := populateBillingUsage(parsed); err != nil {
		return nil, err
	}
	if parsed.Status == model.TaskStatusSuccess && strings.TrimSpace(parsed.URL) == "" {
		return nil, fmt.Errorf("successful new-api video task has no result URL")
	}
	return parsed, nil
}

func parseDetailedTask(envelope detailedEnvelope) (*parsedTask, error) {
	if envelope.Code == nil || *envelope.Code != dto.TaskSuccessCode || envelope.Data == nil {
		code := ""
		if envelope.Code != nil {
			code = *envelope.Code
		}
		return nil, fmt.Errorf("new-api video task wrapper failed: code=%s message=%s", code, envelope.Message)
	}
	status, err := mapUpstreamTaskStatus(envelope.Data.Status)
	if err != nil {
		return nil, err
	}
	nested := &arkTaskData{}
	if len(envelope.Data.Data) > 0 && string(envelope.Data.Data) != "null" {
		if err := common.Unmarshal(envelope.Data.Data, nested); err != nil {
			return nil, fmt.Errorf("unmarshal detailed new-api video task data: %w", err)
		}
	}

	parsed := &parsedTask{
		Status:    status,
		Progress:  normalizedProgress(envelope.Data.Progress, status),
		URL:       envelope.Data.ResultURL,
		CreatedAt: envelope.Data.SubmitTime,
		UpdatedAt: envelope.Data.FinishTime,
		Nested:    nested,
		Usage:     nested.Usage,
	}
	if nested.CreatedAt != nil {
		parsed.CreatedAt = *nested.CreatedAt
	}
	if nested.UpdatedAt != nil {
		parsed.UpdatedAt = *nested.UpdatedAt
	}
	if parsed.URL == "" && nested.Content != nil {
		parsed.URL = nested.Content.VideoURL
	}
	if status == model.TaskStatusFailure {
		parsed.Reason = envelope.Data.FailReason
		if nested.Error != nil {
			parsed.ErrorCode = nested.Error.Code
			if parsed.Reason == "" {
				parsed.Reason = nested.Error.Message
			}
		}
		if parsed.Reason == "" {
			parsed.Reason = envelope.Message
		}
		if parsed.Reason == "" {
			parsed.Reason = "task failed"
		}
	}
	return parsed, nil
}

func parseDirectTask(body []byte, envelopeMessage string) (*parsedTask, error) {
	var direct directTask
	if err := common.Unmarshal(body, &direct); err != nil {
		return nil, fmt.Errorf("unmarshal direct new-api video task: %w", err)
	}
	status, err := mapUpstreamTaskStatus(direct.Status)
	if err != nil {
		return nil, err
	}
	var nested arkTaskData
	if err := common.Unmarshal(body, &nested); err != nil {
		return nil, fmt.Errorf("unmarshal direct ARK task projection: %w", err)
	}
	progress := ""
	if direct.Progress != 0 || (status != model.TaskStatusSuccess && status != model.TaskStatusFailure) {
		progress = strconv.Itoa(direct.Progress) + "%"
	}
	parsed := &parsedTask{
		Status:    status,
		Progress:  normalizedProgress(progress, status),
		CreatedAt: direct.CreatedAt,
		Nested:    &nested,
		Usage:     direct.Usage,
	}
	if nested.CreatedAt != nil {
		parsed.CreatedAt = *nested.CreatedAt
	}
	if nested.UpdatedAt != nil {
		parsed.UpdatedAt = *nested.UpdatedAt
	} else if direct.CompletedAt != 0 {
		parsed.UpdatedAt = direct.CompletedAt
	}
	if direct.Metadata != nil {
		parsed.URL = direct.Metadata.URL
	}
	if parsed.URL == "" && direct.Content != nil {
		parsed.URL = direct.Content.VideoURL
	}
	if parsed.URL == "" && direct.Data != nil {
		parsed.URL = direct.Data.URL
	}
	if status == model.TaskStatusFailure {
		if nested.Error != nil {
			parsed.ErrorCode = nested.Error.Code
			parsed.Reason = nested.Error.Message
		}
		if parsed.Reason == "" && direct.Error != nil {
			parsed.ErrorCode = direct.Error.Code
			parsed.Reason = direct.Error.Message
		}
		if parsed.Reason == "" {
			parsed.Reason = envelopeMessage
		}
		if parsed.Reason == "" {
			parsed.Reason = "task failed"
		}
	}
	return parsed, nil
}

func populateBillingUsage(parsed *parsedTask) error {
	if parsed == nil || parsed.Usage == nil {
		return nil
	}
	if parsed.Usage.CompletionTokens != nil {
		value, clamp, err := tokenNumberToBillingInt(*parsed.Usage.CompletionTokens)
		if err != nil {
			return fmt.Errorf("invalid completion_tokens: %w", err)
		}
		parsed.CompletionTokens = value
		parsed.CompletionTokensPresent = true
		parsed.BillingClamp = clamp
	}
	if parsed.Usage.TotalTokens != nil {
		value, clamp, err := tokenNumberToBillingInt(*parsed.Usage.TotalTokens)
		if err != nil {
			return fmt.Errorf("invalid total_tokens: %w", err)
		}
		parsed.TotalTokens = value
		parsed.TotalTokensPresent = true
		if parsed.BillingClamp == nil {
			parsed.BillingClamp = clamp
		}
	}
	return nil
}

func tokenNumberToBillingInt(number json.Number) (int, *common.QuotaClamp, error) {
	value, err := decimal.NewFromString(number.String())
	if err != nil {
		return 0, nil, err
	}
	if value.IsNegative() || !value.Equal(value.Truncate(0)) {
		return 0, nil, fmt.Errorf("token usage must be a non-negative integer")
	}
	quota, clamp := common.QuotaFromDecimalChecked(value)
	return quota, clamp, nil
}

func mapUpstreamTaskStatus(status string) (model.TaskStatus, error) {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "NOT_START":
		return model.TaskStatusNotStart, nil
	case "SUBMITTED":
		return model.TaskStatusSubmitted, nil
	case "QUEUED":
		return model.TaskStatusQueued, nil
	case "IN_PROGRESS", "RUNNING", "PROCESSING":
		return model.TaskStatusInProgress, nil
	case "SUCCESS", "SUCCEEDED", "COMPLETED":
		return model.TaskStatusSuccess, nil
	case "FAILURE", "FAILED", "CANCELLED", "EXPIRED":
		return model.TaskStatusFailure, nil
	default:
		return model.TaskStatusUnknown, fmt.Errorf("unknown new-api video task status: %s", status)
	}
}

func normalizedProgress(progress string, status model.TaskStatus) string {
	progress = strings.TrimSpace(progress)
	if progress != "" && progress != "%" {
		if !strings.HasSuffix(progress, "%") {
			progress += "%"
		}
		return progress
	}
	switch status {
	case model.TaskStatusNotStart:
		return "0%"
	case model.TaskStatusSubmitted:
		return "10%"
	case model.TaskStatusQueued:
		return "20%"
	case model.TaskStatusInProgress:
		return "50%"
	case model.TaskStatusSuccess, model.TaskStatusFailure:
		return "100%"
	default:
		return "0%"
	}
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(task *model.Task) ([]byte, error) {
	if task == nil {
		return nil, fmt.Errorf("task is nil")
	}
	parsed, err := parsePublicTaskProjection(task)
	if err != nil {
		return nil, err
	}
	video := dto.NewOpenAIVideo()
	video.ID = task.TaskID
	video.TaskID = task.TaskID
	video.Model = task.Properties.OriginModelName
	video.Status = task.Status.ToVideoStatus()
	video.CreatedAt = task.SubmitTime
	if video.CreatedAt == 0 {
		video.CreatedAt = task.CreatedAt
	}
	video.SetProgressStr(task.Progress)
	if video.Progress < 0 {
		video.Progress = 0
	}
	if video.Progress > 100 {
		video.Progress = 100
	}
	video.SetMetadata("url", "")
	if task.Status == model.TaskStatusSuccess {
		resultURL := parsed.URL
		if resultURL == "" {
			resultURL = task.PrivateData.ResultURL
		}
		video.SetMetadata("url", resultURL)
	}
	if task.Status == model.TaskStatusSuccess || task.Status == model.TaskStatusFailure {
		video.CompletedAt = task.FinishTime
		if video.CompletedAt == 0 {
			video.CompletedAt = task.UpdatedAt
		}
	}
	if task.Status == model.TaskStatusFailure {
		message := parsed.Reason
		if message == "" {
			message = task.FailReason
		}
		if message == "" {
			message = "task failed"
		}
		code := parsed.ErrorCode
		if code == "" {
			code = "task_failed"
		}
		video.Error = &dto.OpenAIVideoError{Code: code, Message: message}
	}
	return common.Marshal(video)
}

func (a *TaskAdaptor) ConvertToArkVideoTask(task *model.Task) ([]byte, error) {
	if task == nil {
		return nil, fmt.Errorf("task is nil")
	}
	parsed, err := parsePublicTaskProjection(task)
	if err != nil {
		return nil, err
	}
	nested := parsed.Nested
	response := arkTaskResponse{
		ID:                    task.TaskID,
		Model:                 task.Properties.OriginModelName,
		Status:                arkStatus(task.Status),
		Content:               nested.Content,
		CreatedAt:             nested.CreatedAt,
		UpdatedAt:             nested.UpdatedAt,
		Draft:                 nested.Draft,
		Duration:              nested.Duration,
		ExecutionExpiresAfter: nested.ExecutionExpiresAfter,
		FramesPerSecond:       nested.FramesPerSecond,
		GenerateAudio:         nested.GenerateAudio,
		Priority:              nested.Priority,
		Ratio:                 nested.Ratio,
		Resolution:            nested.Resolution,
		Seed:                  nested.Seed,
		ServiceTier:           nested.ServiceTier,
		Usage:                 nested.Usage,
		Error:                 nested.Error,
	}
	if response.CreatedAt == nil {
		createdAt := parsed.CreatedAt
		if createdAt == 0 {
			createdAt = task.SubmitTime
		}
		if createdAt != 0 {
			response.CreatedAt = &createdAt
		}
	}
	if response.UpdatedAt == nil {
		updatedAt := parsed.UpdatedAt
		if updatedAt == 0 && (task.Status == model.TaskStatusSuccess || task.Status == model.TaskStatusFailure) {
			updatedAt = task.FinishTime
			if updatedAt == 0 {
				updatedAt = task.UpdatedAt
			}
		}
		if updatedAt != 0 {
			response.UpdatedAt = &updatedAt
		}
	}
	if (response.Content == nil || response.Content.VideoURL == "") && task.PrivateData.ResultURL != "" {
		response.Content = &arkVideoContent{VideoURL: task.PrivateData.ResultURL}
	}
	if task.Status == model.TaskStatusFailure && (response.Error == nil || response.Error.Message == "") {
		message := parsed.Reason
		if message == "" {
			message = task.FailReason
		}
		if message == "" {
			message = "task failed"
		}
		response.Error = &upstreamError{Code: parsed.ErrorCode, Message: message}
	}
	return common.Marshal(response)
}

func parsePublicTaskProjection(task *model.Task) (*parsedTask, error) {
	parsed, err := parseTaskProjection(task.Data)
	if err == nil || task.Status != model.TaskStatusFailure {
		return parsed, err
	}

	parsed = &parsedTask{
		Status: model.TaskStatusFailure,
		Reason: strings.TrimSpace(task.FailReason),
		Nested: &arkTaskData{},
	}
	var response upstreamErrorEnvelope
	if unmarshalErr := common.Unmarshal(task.Data, &response); unmarshalErr == nil {
		parsed.ErrorCode = response.Code
		if parsed.Reason == "" {
			parsed.Reason = response.Message
		}
		if response.Error != nil {
			if response.Error.Code != "" {
				parsed.ErrorCode = response.Error.Code
			}
			if parsed.Reason == "" {
				parsed.Reason = response.Error.Message
			}
		}
	}
	if parsed.ErrorCode == "" {
		parsed.ErrorCode = "task_failed"
	}
	if parsed.Reason == "" {
		parsed.Reason = "task failed"
	}
	return parsed, nil
}

func arkStatus(status model.TaskStatus) string {
	switch status {
	case model.TaskStatusNotStart, model.TaskStatusSubmitted, model.TaskStatusQueued:
		return "queued"
	case model.TaskStatusInProgress:
		return "running"
	case model.TaskStatusSuccess:
		return "succeeded"
	case model.TaskStatusFailure:
		return "failed"
	default:
		return "unknown"
	}
}
