package newapivideo

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	taskdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	taskcommon "github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/types"
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
	requestDialect := a.activeProfile().requestDialect
	allowMissingSuccessURL := requestDialect == videoRequestDialectEightYes || requestDialect == videoRequestDialectFFLink
	parsed, err := parseTaskProjection(body, allowMissingSuccessURL)
	if err != nil {
		return nil, err
	}
	result := &relaycommon.TaskInfo{
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
	}
	if parsed.Nested.Duration != nil {
		result.DurationPresent = true
		if duration, numberErr := parsed.Nested.Duration.Int64(); numberErr == nil && duration > 0 && duration <= relaycommon.MaxTaskDurationSeconds {
			result.DurationSeconds = int(duration)
		}
	}
	result.ResolutionPresent = strings.TrimSpace(parsed.Nested.Resolution) != ""
	if parsed.Nested.FramesPerSecond != nil {
		result.FramesPerSecondPresent = true
		if frameRate, numberErr := parsed.Nested.FramesPerSecond.Int64(); numberErr == nil && frameRate > 0 && frameRate <= 240 {
			result.FramesPerSecond = int(frameRate)
		}
	}
	if parsed.Nested.Duration != nil {
		duration := strings.TrimSpace(parsed.Nested.Duration.String())
		value, durationErr := decimal.NewFromString(duration)
		if durationErr == nil && !value.IsNegative() &&
			value.LessThanOrEqual(decimal.NewFromInt(relaycommon.MaxTaskDurationSeconds)) {
			result.CostMeter = &types.CostMeter{
				Source:          types.CostMeterUpstreamActual,
				DurationSeconds: &duration,
			}
		}
	}
	if a.activeProfile().requestDialect == videoRequestDialectZ5APIMedia {
		result.DurationPresent = false
		result.DurationSeconds = 0
		result.CostMeter = nil
	}
	return result, nil
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
	result := relaycommon.FailTaskInfo(sanitizeUpstreamFailure(message))
	result.ErrorCode = code
	return result
}

func sanitizeUpstreamFailure(message string) string {
	return strings.TrimSpace(common.MaskSensitiveInfo(message))
}

func sanitizePublicTaskFailure(message string, upstreamTaskID string) string {
	return sanitizePublicTaskFailureWithSecrets(message, upstreamTaskID)
}

func sanitizePublicTaskFailureWithSecrets(message string, upstreamTaskID string, secrets ...string) string {
	if upstreamTaskID != "" {
		message = strings.ReplaceAll(message, upstreamTaskID, "[redacted]")
	}
	for _, secret := range secrets {
		if secret != "" {
			message = strings.ReplaceAll(message, secret, "[redacted]")
		}
	}
	return sanitizeUpstreamFailure(message)
}

func parseTaskProjection(body []byte, allowMissingSuccessURL ...bool) (*parsedTask, error) {
	var header struct {
		Code    json.RawMessage `json:"code"`
		Message string          `json:"message"`
	}
	if err := common.Unmarshal(body, &header); err != nil {
		return nil, fmt.Errorf("unmarshal new-api video task response: %w", err)
	}

	var parsed *parsedTask
	var err error
	if len(header.Code) > 0 && strings.TrimSpace(string(header.Code)) != "null" {
		var envelope detailedEnvelope
		if err := common.Unmarshal(body, &envelope); err != nil {
			return nil, fmt.Errorf("unmarshal detailed new-api video task response: %w", err)
		}
		parsed, err = parseDetailedTask(envelope)
	} else {
		parsed, err = parseDirectTask(body, header.Message)
	}
	if err != nil {
		return nil, err
	}
	if err := populateBillingUsage(parsed); err != nil {
		return nil, err
	}
	missingURLAllowed := len(allowMissingSuccessURL) > 0 && allowMissingSuccessURL[0]
	if parsed.Status == model.TaskStatusSuccess && strings.TrimSpace(parsed.URL) == "" && !missingURLAllowed {
		return nil, fmt.Errorf("successful new-api video task has no result URL")
	}
	return parsed, nil
}

func parseDetailedTask(envelope detailedEnvelope) (*parsedTask, error) {
	if envelope.Code == nil || *envelope.Code != taskdto.TaskSuccessCode || envelope.Data == nil {
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
			nested.Error = &upstreamError{Code: nested.Error.Code, Message: sanitizeUpstreamFailure(nested.Error.Message)}
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
		parsed.Reason = sanitizeUpstreamFailure(parsed.Reason)
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
	projectionBody := body
	var projection map[string]json.RawMessage
	if err := common.Unmarshal(body, &projection); err == nil {
		if rawDuration, exists := projection["duration"]; exists {
			if duration, ok := parseProviderNumber(rawDuration, "s"); ok {
				projection["duration"] = json.RawMessage(duration.String())
				normalized, err := common.Marshal(projection)
				if err != nil {
					return nil, fmt.Errorf("marshal normalized direct ARK task projection: %w", err)
				}
				projectionBody = normalized
			}
		}
	}
	var nested arkTaskData
	if err := common.Unmarshal(projectionBody, &nested); err != nil {
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
	if seconds, ok := parseProviderNumber(direct.Seconds, ""); ok {
		nested.Duration = &seconds
	}
	if nested.CreatedAt != nil {
		parsed.CreatedAt = *nested.CreatedAt
	}
	if nested.UpdatedAt != nil {
		parsed.UpdatedAt = *nested.UpdatedAt
	} else if direct.CompletedAt != 0 {
		parsed.UpdatedAt = direct.CompletedAt
	}
	parsed.URL = directTaskVideoURL(direct)
	if status == model.TaskStatusFailure {
		upstreamTaskID := direct.TaskID
		if upstreamTaskID == "" {
			upstreamTaskID = direct.ID
		}
		if nested.Error != nil {
			nested.Error = &upstreamError{
				Code:    sanitizePublicTaskFailure(nested.Error.Code, upstreamTaskID),
				Message: sanitizePublicTaskFailure(nested.Error.Message, upstreamTaskID),
			}
			parsed.ErrorCode = nested.Error.Code
			parsed.Reason = nested.Error.Message
		}
		if parsed.Reason == "" && direct.Error != nil {
			parsed.ErrorCode = sanitizePublicTaskFailure(direct.Error.Code, upstreamTaskID)
			parsed.Reason = sanitizePublicTaskFailure(direct.Error.Message, upstreamTaskID)
		}
		if parsed.Reason == "" {
			parsed.Reason = envelopeMessage
		}
		if parsed.Reason == "" {
			parsed.Reason = "task failed"
		}
		parsed.Reason = sanitizePublicTaskFailure(parsed.Reason, upstreamTaskID)
	}
	return parsed, nil
}

func parseProviderNumber(raw json.RawMessage, suffix string) (json.Number, bool) {
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "null" {
		return "", false
	}
	var number json.Number
	if err := common.Unmarshal(raw, &number); err == nil {
		if _, err := decimal.NewFromString(number.String()); err == nil {
			return number, true
		}
	}
	var text string
	if err := common.Unmarshal(raw, &text); err != nil {
		return "", false
	}
	text = strings.TrimSpace(text)
	if suffix != "" {
		text = strings.TrimSpace(strings.TrimSuffix(text, suffix))
	}
	value, err := decimal.NewFromString(text)
	if err != nil {
		return "", false
	}
	return json.Number(value.String()), true
}

func directTaskVideoURL(task directTask) string {
	if object := strings.TrimSpace(task.Object); strings.HasPrefix(strings.ToLower(object), "http://") || strings.HasPrefix(strings.ToLower(object), "https://") {
		return object
	}
	for _, value := range []string{
		task.VideoURL,
		task.URL,
		task.ResultURL,
	} {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	if len(task.Output) > 0 && strings.TrimSpace(task.Output[0].URL) != "" {
		return task.Output[0].URL
	}
	if task.Metadata != nil {
		for _, value := range []string{
			task.Metadata.URL,
			task.Metadata.ContentURL,
			task.Metadata.LocalURL,
		} {
			if strings.TrimSpace(value) != "" {
				return value
			}
		}
	}
	if task.Content != nil && strings.TrimSpace(task.Content.VideoURL) != "" {
		return task.Content.VideoURL
	}
	if len(task.Data) == 0 || strings.TrimSpace(string(task.Data)) == "null" {
		return ""
	}

	type dataURL struct {
		URL       string `json:"url,omitempty"`
		VideoURL  string `json:"video_url,omitempty"`
		ResultURL string `json:"result_url,omitempty"`
	}
	var object dataURL
	if err := common.Unmarshal(task.Data, &object); err == nil {
		for _, value := range []string{object.URL, object.VideoURL, object.ResultURL} {
			if strings.TrimSpace(value) != "" {
				return value
			}
		}
	}
	var items []dataURL
	if err := common.Unmarshal(task.Data, &items); err == nil && len(items) > 0 {
		for _, value := range []string{items[0].URL, items[0].VideoURL, items[0].ResultURL} {
			if strings.TrimSpace(value) != "" {
				return value
			}
		}
	}
	return ""
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
	case "QUEUED", "PENDING":
		return model.TaskStatusQueued, nil
	case "IN_PROGRESS", "RUNNING", "PROCESSING", "SETTLING":
		return model.TaskStatusInProgress, nil
	case "SUCCESS", "SUCCEEDED", "COMPLETED":
		return model.TaskStatusSuccess, nil
	case "FAILURE", "FAILED", "ERROR", "CANCELED", "CANCELLED", "EXPIRED":
		return model.TaskStatusFailure, nil
	default:
		return model.TaskStatusUnknown, fmt.Errorf("unknown new-api video task status: %s", status)
	}
}

func normalizedProgress(progress string, status model.TaskStatus) string {
	progress = strings.TrimSpace(progress)
	if progress != "" && progress != "%" {
		raw := strings.TrimSuffix(progress, "%")
		if value, err := decimal.NewFromString(strings.TrimSpace(raw)); err == nil {
			if value.IsNegative() {
				value = decimal.Zero
			} else if value.GreaterThan(decimal.NewFromInt(100)) {
				value = decimal.NewFromInt(100)
			}
			return value.String() + "%"
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
		video.Error = &dto.OpenAIVideoError{Code: code, Message: sanitizePublicTaskFailureWithSecrets(message, task.GetUpstreamTaskID(), task.PrivateData.Key)}
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
	if a.activeProfile().requestDialect == videoRequestDialectZ5APIMedia {
		response.Duration = z5APIRequestedDuration(task)
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
	if (response.Content == nil || response.Content.VideoURL == "") && task.Status == model.TaskStatusSuccess && a.activeProfile().requestDialect == videoRequestDialectFFLink {
		response.Content = &arkVideoContent{VideoURL: taskcommon.BuildProxyURL(task.TaskID)}
	}
	if task.Status == model.TaskStatusFailure {
		if response.Error != nil {
			response.Error = &upstreamError{
				Code:    response.Error.Code,
				Message: sanitizePublicTaskFailureWithSecrets(response.Error.Message, task.GetUpstreamTaskID(), task.PrivateData.Key),
			}
		}
		if response.Error == nil || response.Error.Message == "" {
			message := parsed.Reason
			if message == "" {
				message = task.FailReason
			}
			if message == "" {
				message = "task failed"
			}
			response.Error = &upstreamError{
				Code:    parsed.ErrorCode,
				Message: sanitizePublicTaskFailureWithSecrets(message, task.GetUpstreamTaskID(), task.PrivateData.Key),
			}
		}
	}
	return common.Marshal(response)
}

func z5APIRequestedDuration(task *model.Task) *json.Number {
	var request struct {
		Duration json.RawMessage `json:"duration"`
	}
	if err := common.Unmarshal(task.PrivateData.UserRequestData, &request); err == nil {
		if duration, ok := parseProviderNumber(request.Duration, ""); ok {
			value, err := decimal.NewFromString(duration.String())
			if err == nil && value.GreaterThan(decimal.Zero) &&
				value.LessThanOrEqual(decimal.NewFromInt(relaycommon.MaxTaskDurationSeconds)) {
				return &duration
			}
		}
	}
	if task.PrivateData.BillingContext == nil {
		return nil
	}
	durationSeconds := task.PrivateData.BillingContext.RequestedDurationSeconds
	if durationSeconds <= 0 || durationSeconds > relaycommon.MaxTaskDurationSeconds {
		return nil
	}
	duration := json.Number(strconv.Itoa(durationSeconds))
	return &duration
}

func parsePublicTaskProjection(task *model.Task) (*parsedTask, error) {
	allowMissingSuccessURL := task.Status == model.TaskStatusSuccess && (strings.TrimSpace(task.PrivateData.ResultURL) != "" || task.Platform == constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeFFLink)))
	parsed, err := parseTaskProjection(task.Data, allowMissingSuccessURL)
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
	parsed.Reason = sanitizeUpstreamFailure(parsed.Reason)
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
