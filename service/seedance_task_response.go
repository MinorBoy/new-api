package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/seedancepricing"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/shopspring/decimal"
)

const (
	defaultSeedanceResolution            = "720p"
	defaultSeedanceRatio                 = "16:9"
	defaultSeedanceDurationSeconds       = int64(5)
	defaultSeedanceFramesPerSecond       = int64(24)
	defaultSeedanceServiceTier           = "default"
	defaultSeedanceExecutionExpiresAfter = int64(172800)
	maxSeedanceResponseInteger           = int64(1<<31 - 1)
	maxSeedanceTimestamp                 = int64(1<<63 - 1)
)

type seedanceTaskRequestSnapshot struct {
	Seed                  *json.Number `json:"seed"`
	Resolution            string       `json:"resolution"`
	Ratio                 string       `json:"ratio"`
	Duration              *json.Number `json:"duration"`
	FramesPerSecond       *json.Number `json:"framespersecond"`
	ServiceTier           string       `json:"service_tier"`
	ExecutionExpiresAfter *json.Number `json:"execution_expires_after"`
	GenerateAudio         *bool        `json:"generate_audio"`
	Draft                 *bool        `json:"draft"`
	Priority              *json.Number `json:"priority"`
}

func NormalizeSeedanceTaskResponse(task *model.Task, response map[string]interface{}) error {
	if task == nil || response == nil {
		return nil
	}

	response["id"] = task.TaskID
	publicStatus := SeedanceTaskStatus(task.Status)
	if upstreamStatus, ok := response["status"].(string); ok && task.Status == model.TaskStatusFailure {
		switch strings.ToLower(upstreamStatus) {
		case "expired", "cancelled":
			publicStatus = strings.ToLower(upstreamStatus)
		}
	}
	response["status"] = publicStatus

	billingContext := task.PrivateData.BillingContext
	modelName := strings.TrimSpace(task.Properties.OriginModelName)
	if modelName == "" && billingContext != nil {
		modelName = strings.TrimSpace(billingContext.OriginModelName)
	}
	if modelName == "" {
		modelName, _ = response["model"].(string)
		modelName = strings.TrimSpace(modelName)
	}
	if modelName == "" {
		modelName = strings.TrimSpace(task.Properties.UpstreamModelName)
	}
	if modelName == "" && billingContext != nil {
		modelName = strings.TrimSpace(billingContext.UpstreamModelName)
	}
	response["model"] = modelName
	setSeedanceTimestamp(response, "created_at", task.SubmitTime, task.CreatedAt)
	setSeedanceTimestamp(response, "updated_at", task.FinishTime, task.UpdatedAt)
	if task.Status == model.TaskStatusSuccess {
		delete(response, "error")
		content, ok := response["content"].(map[string]interface{})
		if !ok {
			content = make(map[string]interface{})
		}
		videoURL, _ := content["video_url"].(string)
		if strings.TrimSpace(videoURL) == "" && strings.TrimSpace(task.PrivateData.ResultURL) != "" {
			videoURL = task.PrivateData.ResultURL
		}
		if strings.TrimSpace(videoURL) == "" && strings.TrimSpace(task.PrivateData.ResultObjectKey) != "" {
			resolved, err := ResolveTaskResultURL(context.Background(), task)
			if err != nil {
				return err
			}
			videoURL = resolved
		}
		if strings.TrimSpace(videoURL) == "" {
			return errors.New("successful Seedance task response is missing content.video_url")
		}
		content["video_url"] = videoURL
		response["content"] = content
	}
	if err := populateSeedanceTaskUsage(task, response); err != nil {
		return err
	}
	completeSeedanceTerminalResponse(task, response)
	return nil
}

func IsSeedanceTaskPlatform(platform constant.TaskPlatform) bool {
	switch platform {
	case constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeVolcEngine)),
		constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeDoubaoVideo)),
		constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeDimensio)),
		constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeNewAPIVideo)),
		constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeLucen)),
		constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeMegaByAI)),
		constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeCangyuan)),
		constant.TaskPlatform(strconv.Itoa(constant.ChannelTypePaipu)),
		constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeSecure)),
		constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeOmegaAI)),
		constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeFourSToken)),
		constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeEightYes)),
		constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeZ5API)),
		constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeMikoto)),
		constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeClmmMall)):
		return true
	default:
		return false
	}
}

func IsSeedanceTask(task *model.Task) bool {
	if task == nil || !IsSeedanceTaskPlatform(task.Platform) {
		return false
	}
	if billingContext := task.PrivateData.BillingContext; billingContext != nil {
		if billingContext.UsageProfile == model.TaskUsageProfileSeedance {
			return true
		}
		for _, modelName := range []string{billingContext.OriginModelName, billingContext.UpstreamModelName} {
			if seedancepricing.Family(modelName) != "" {
				return true
			}
		}
	}
	requestPath := strings.ToLower(strings.TrimSpace(task.Properties.RequestPath))
	if strings.HasPrefix(requestPath, "/api/v3/contents/generations/tasks") {
		return true
	}
	for _, modelName := range []string{task.Properties.OriginModelName, task.Properties.UpstreamModelName} {
		if seedancepricing.Family(modelName) != "" {
			return true
		}
	}
	return false
}

func SeedanceTaskPlatformValues() []string {
	candidates := []constant.TaskPlatform{
		constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeVolcEngine)),
		constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeDoubaoVideo)),
		constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeDimensio)),
		constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeNewAPIVideo)),
		constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeLucen)),
		constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeMegaByAI)),
		constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeCangyuan)),
		constant.TaskPlatform(strconv.Itoa(constant.ChannelTypePaipu)),
		constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeSecure)),
		constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeOmegaAI)),
		constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeFourSToken)),
		constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeEightYes)),
		constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeZ5API)),
		constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeMikoto)),
		constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeClmmMall)),
	}
	values := make([]string, 0, len(candidates))
	for _, platform := range candidates {
		if IsSeedanceTaskPlatform(platform) {
			values = append(values, string(platform))
		}
	}
	return values
}

func SeedanceTaskStatus(status model.TaskStatus) string {
	switch status {
	case model.TaskStatusSuccess:
		return "succeeded"
	case model.TaskStatusFailure:
		return "failed"
	case model.TaskStatusInProgress:
		return "running"
	case model.TaskStatusQueued, model.TaskStatusSubmitted, model.TaskStatusNotStart:
		return "queued"
	default:
		return "queued"
	}
}

func populateSeedanceTaskUsage(task *model.Task, response map[string]interface{}) error {
	if task == nil || task.Status != model.TaskStatusSuccess || response == nil {
		return nil
	}
	billingContext := task.PrivateData.BillingContext
	if billingContext != nil && billingContext.UsageSnapshotVersion == model.TaskUsageSnapshotVersion1 {
		if billingContext.UsageSource == model.TaskUsageSourceUpstream ||
			billingContext.UsageSource == model.TaskUsageSourceLocalCalculated {
			if usage, ok := PersistedSeedanceTaskUsage(billingContext); ok {
				response["usage"] = map[string]interface{}{
					"completion_tokens": usage.CompletionTokens,
					"total_tokens":      usage.TotalTokens,
				}
			}
		}
		return nil
	}
	preferLocalUsage := billingContext != nil && billingContext.UsageSource == model.TaskUsageSourceLocalCalculated
	if rawUsage, exists := response["usage"]; exists && !preferLocalUsage {
		usageData, err := common.Marshal(rawUsage)
		if err == nil {
			var usage struct {
				CompletionTokens *int64 `json:"completion_tokens"`
				TotalTokens      *int64 `json:"total_tokens"`
			}
			if common.Unmarshal(usageData, &usage) == nil &&
				usage.CompletionTokens != nil && usage.TotalTokens != nil &&
				IsValidSeedanceUpstreamUsage(*usage.CompletionTokens, *usage.TotalTokens) {
				return nil
			}
		}
	}

	if billingContext == nil {
		return nil
	}
	if billingContext.UsageProfile != model.TaskUsageProfileSeedance {
		costMode := types.CostMode(billingContext.UpstreamCostMode)
		if costMode != types.CostModePerRequest && costMode != types.CostModePerDuration {
			return nil
		}
	}

	responseData, err := common.Marshal(response)
	if err != nil {
		return nil
	}
	var output struct {
		Duration        *json.Number `json:"duration"`
		Resolution      string       `json:"resolution"`
		FramesPerSecond *json.Number `json:"framespersecond"`
	}
	if common.Unmarshal(responseData, &output) != nil {
		return nil
	}

	terminalFacts := SeedanceTerminalFacts{
		Resolution:        output.Resolution,
		ResolutionPresent: strings.TrimSpace(output.Resolution) != "",
	}
	if output.Duration != nil {
		terminalFacts.DurationPresent = true
		value, ok := boundedSeedanceResponseInteger(output.Duration, relaycommon.MaxTaskDurationSeconds)
		if ok {
			terminalFacts.DurationSeconds = int(value)
		}
	}
	if output.FramesPerSecond != nil {
		terminalFacts.FramesPerSecondPresent = true
		value, ok := boundedSeedanceResponseInteger(output.FramesPerSecond, 240)
		if ok {
			terminalFacts.FramesPerSecond = int(value)
		}
	}
	usage, err := CalculateSeedanceTaskUsage(billingContext, terminalFacts)
	if err != nil {
		return nil
	}
	response["usage"] = map[string]interface{}{
		"completion_tokens": usage.CompletionTokens,
		"total_tokens":      usage.TotalTokens,
	}
	return nil
}

func boundedSeedanceResponseInteger(number *json.Number, maximum int64) (int64, bool) {
	if number == nil {
		return 0, false
	}
	value, err := decimal.NewFromString(number.String())
	if err != nil || value.LessThanOrEqual(decimal.Zero) || value.GreaterThan(decimal.NewFromInt(maximum)) || !value.Equal(value.Truncate(0)) {
		return 0, false
	}
	return value.IntPart(), true
}

func completeSeedanceTerminalResponse(task *model.Task, response map[string]interface{}) {
	if task == nil || response == nil {
		return
	}
	if task.Status != model.TaskStatusSuccess && task.Status != model.TaskStatusFailure {
		return
	}

	request := seedanceTaskRequestSnapshot{}
	if len(task.PrivateData.UserRequestData) > 0 {
		_ = common.Unmarshal(task.PrivateData.UserRequestData, &request)
	}
	billingContext := task.PrivateData.BillingContext

	setSeedanceInteger(response, "seed", 0, maxSeedanceResponseInteger, request.Seed, int64(0))
	setSeedanceString(response, "resolution", request.Resolution, seedanceBillingResolution(billingContext), defaultSeedanceResolution)
	setSeedanceString(response, "ratio", request.Ratio, defaultSeedanceRatio)
	setSeedanceInteger(
		response,
		"duration",
		1,
		int64(relaycommon.MaxTaskDurationSeconds),
		request.Duration,
		seedanceRequestedDuration(billingContext),
		defaultSeedanceDurationSeconds,
	)
	setSeedanceInteger(
		response,
		"framespersecond",
		1,
		240,
		request.FramesPerSecond,
		seedanceBillingFrameRate(billingContext),
		defaultSeedanceFramesPerSecond,
	)
	setSeedanceString(response, "service_tier", request.ServiceTier, seedanceBillingServiceTier(billingContext), defaultSeedanceServiceTier)
	setSeedanceInteger(
		response,
		"execution_expires_after",
		1,
		maxSeedanceResponseInteger,
		request.ExecutionExpiresAfter,
		defaultSeedanceExecutionExpiresAfter,
	)
	setSeedanceBool(response, "generate_audio", request.GenerateAudio, seedanceBillingGenerateAudio(billingContext), true)
	setSeedanceBool(response, "draft", request.Draft, seedanceBillingDraft(billingContext), false)
	setSeedanceInteger(response, "priority", 0, maxSeedanceResponseInteger, request.Priority, int64(0))
	completeSeedanceUsage(response)

	if task.Status == model.TaskStatusFailure {
		completeSeedanceFailure(task, response)
	}
}

func setSeedanceString(response map[string]interface{}, key string, candidates ...string) {
	if value, ok := response[key].(string); ok && strings.TrimSpace(value) != "" {
		return
	}
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		response[key] = candidate
		return
	}
}

func setSeedanceTimestamp(response map[string]interface{}, key string, candidates ...int64) {
	if value, ok := boundedSeedanceInteger(response[key], 1, maxSeedanceTimestamp); ok {
		response[key] = value
		return
	}
	for _, candidate := range candidates {
		if value, ok := boundedSeedanceInteger(candidate, 1, maxSeedanceTimestamp); ok {
			response[key] = value
			return
		}
	}
	response[key] = int64(0)
}

func setSeedanceInteger(response map[string]interface{}, key string, minimum, maximum int64, candidates ...interface{}) {
	if value, ok := boundedSeedanceInteger(response[key], minimum, maximum); ok {
		response[key] = value
		return
	}
	for _, candidate := range candidates {
		if value, ok := boundedSeedanceInteger(candidate, minimum, maximum); ok {
			response[key] = value
			return
		}
	}
}

func boundedSeedanceInteger(value interface{}, minimum, maximum int64) (int64, bool) {
	if value == nil {
		return 0, false
	}
	number, err := decimal.NewFromString(strings.TrimSpace(fmt.Sprint(value)))
	if err != nil || !number.Equal(number.Truncate(0)) ||
		number.LessThan(decimal.NewFromInt(minimum)) || number.GreaterThan(decimal.NewFromInt(maximum)) {
		return 0, false
	}
	return number.IntPart(), true
}

func setSeedanceBool(response map[string]interface{}, key string, candidates ...interface{}) {
	if _, ok := response[key].(bool); ok {
		return
	}
	for _, candidate := range candidates {
		switch value := candidate.(type) {
		case bool:
			response[key] = value
			return
		case *bool:
			if value != nil {
				response[key] = *value
				return
			}
		}
	}
}

func seedanceBillingResolution(billingContext *model.TaskBillingContext) string {
	if billingContext == nil {
		return ""
	}
	if value := strings.TrimSpace(billingContext.Resolution); value != "" {
		return value
	}
	return strings.TrimSpace(billingContext.DurationResolution)
}

func seedanceRequestedDuration(billingContext *model.TaskBillingContext) interface{} {
	if billingContext == nil {
		return nil
	}
	return billingContext.RequestedDurationSeconds
}

func seedanceBillingFrameRate(billingContext *model.TaskBillingContext) interface{} {
	if billingContext == nil || billingContext.SeedanceTokenBilling == nil {
		return nil
	}
	return billingContext.SeedanceTokenBilling.FrameRate
}

func seedanceBillingServiceTier(billingContext *model.TaskBillingContext) string {
	if billingContext == nil {
		return ""
	}
	return billingContext.ServiceTier
}

func seedanceBillingGenerateAudio(billingContext *model.TaskBillingContext) interface{} {
	if billingContext == nil {
		return nil
	}
	return billingContext.GenerateAudio
}

func seedanceBillingDraft(billingContext *model.TaskBillingContext) interface{} {
	if billingContext == nil {
		return nil
	}
	return billingContext.Draft
}

func completeSeedanceUsage(response map[string]interface{}) {
	usage, ok := response["usage"].(map[string]interface{})
	if !ok {
		usage = make(map[string]interface{})
	}
	completionTokens, ok := boundedSeedanceInteger(usage["completion_tokens"], 0, int64(relaycommon.MaxTokensLimit))
	if !ok {
		completionTokens = 0
	}
	usage["completion_tokens"] = completionTokens
	usage["total_tokens"] = completionTokens
	response["usage"] = usage
}

func completeSeedanceFailure(task *model.Task, response map[string]interface{}) {
	errorData, ok := response["error"].(map[string]interface{})
	if !ok {
		errorData = make(map[string]interface{})
	}
	upstreamTaskID := ""
	if task != nil {
		upstreamTaskID = task.GetUpstreamTaskID()
	}
	code, _ := errorData["code"].(string)
	code = strings.TrimSpace(code)
	if upstreamTaskID != "" {
		code = strings.ReplaceAll(code, upstreamTaskID, "[redacted]")
	}
	if code == "" {
		errorData["code"] = "task_failed"
	} else {
		errorData["code"] = code
	}
	message, _ := errorData["message"].(string)
	message = sanitizeSeedanceFailureText(message, upstreamTaskID)
	if message == "" && task != nil {
		message = sanitizeSeedanceFailureText(task.FailReason, upstreamTaskID)
	}
	if message == "" {
		message = "task failed"
	}
	errorData["message"] = message
	response["error"] = errorData
	delete(response, "content")
}

func sanitizeSeedanceFailureText(value string, upstreamTaskID string) string {
	value = strings.TrimSpace(value)
	if upstreamTaskID != "" {
		value = strings.ReplaceAll(value, upstreamTaskID, "[redacted]")
	}
	return strings.TrimSpace(common.MaskSensitiveInfo(value))
}
