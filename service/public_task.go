package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

type publicTaskOutputSource struct {
	Title         string `json:"title"`
	Text          string `json:"text"`
	AudioURL      string `json:"audio_url"`
	VideoURL      string `json:"video_url"`
	ImageURL      string `json:"image_url"`
	ImageLargeURL string `json:"image_large_url"`
}

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
	if task.Status == model.TaskStatusSuccess && !IsSeedanceTask(task) {
		hasVideoResult := strings.TrimSpace(task.GetResultURL()) != ""
		switch strings.ToLower(strings.TrimSpace(task.Action)) {
		case "generate", "textgenerate", "firsttailgenerate", "referencegenerate", "remixgenerate":
			hasVideoResult = true
		}
		if hasVideoResult {
			if strings.TrimSpace(task.PrivateData.ResultObjectKey) != "" {
				if resolved, err := ResolveTaskResultURL(context.Background(), task); err == nil {
					public.ResultURL = resolved
				}
			} else {
				public.ResultURL = taskcommon.BuildProxyURL(task.TaskID)
			}
		}
		if task.Platform == constant.TaskPlatformSuno {
			sources := publicTaskOutputSources(task)
			public.Data = make([]dto.PublicTaskOutput, 0, len(sources))
			for index, source := range sources {
				output := dto.PublicTaskOutput{
					Title: strings.TrimSpace(source.Title),
					Text:  strings.TrimSpace(source.Text),
				}
				for kind, rawURL := range map[string]string{
					"audio": source.AudioURL,
					"video": source.VideoURL,
					"image": publicTaskImageURL(source),
				} {
					mediaURL, err := relaycommon.ParseTaskMediaURL(rawURL)
					if err != nil || (mediaURL.Kind != relaycommon.TaskMediaURLHTTP && mediaURL.Kind != relaycommon.TaskMediaURLData) {
						continue
					}
					proxyURL := taskcommon.BuildTaskMediaProxyURL(task.TaskID, index, kind)
					switch kind {
					case "audio":
						output.AudioURL = proxyURL
					case "video":
						output.VideoURL = proxyURL
					case "image":
						output.ImageURL = proxyURL
					}
				}
				if output.Title != "" || output.Text != "" || output.AudioURL != "" || output.VideoURL != "" || output.ImageURL != "" {
					public.Data = append(public.Data, output)
				}
			}
		}
	}
	public.UserResponseData = projectPublicTaskResult(task, requestModel)
	return public
}

func ResolvePublicTaskMediaURL(task *model.Task, index int, kind string) (string, error) {
	if task == nil || task.Platform != constant.TaskPlatformSuno {
		return "", fmt.Errorf("task media is unavailable")
	}
	sources := publicTaskOutputSources(task)
	if index < 0 || index >= len(sources) {
		return "", fmt.Errorf("task media index is out of range")
	}
	source := sources[index]
	var rawURL string
	switch kind {
	case "audio":
		rawURL = source.AudioURL
	case "video":
		rawURL = source.VideoURL
	case "image":
		rawURL = publicTaskImageURL(source)
	default:
		return "", fmt.Errorf("task media kind is invalid")
	}
	mediaURL, err := relaycommon.ParseTaskMediaURL(rawURL)
	if err != nil || (mediaURL.Kind != relaycommon.TaskMediaURLHTTP && mediaURL.Kind != relaycommon.TaskMediaURLData) {
		return "", fmt.Errorf("task media is unavailable")
	}
	return mediaURL.Value, nil
}

func publicTaskOutputSources(task *model.Task) []publicTaskOutputSource {
	if task == nil || task.Platform != constant.TaskPlatformSuno || len(task.Data) == 0 {
		return nil
	}
	var sources []publicTaskOutputSource
	if common.Unmarshal(task.Data, &sources) == nil {
		return sources
	}
	var source publicTaskOutputSource
	if common.Unmarshal(task.Data, &source) == nil {
		return []publicTaskOutputSource{source}
	}
	return nil
}

func publicTaskImageURL(source publicTaskOutputSource) string {
	if value := strings.TrimSpace(source.ImageLargeURL); value != "" {
		return value
	}
	return source.ImageURL
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
		var upstreamUsage struct {
			Usage *struct {
				CompletionTokens *int64 `json:"completion_tokens"`
				TotalTokens      *int64 `json:"total_tokens"`
			} `json:"usage"`
		}
		if common.Unmarshal(task.PrivateData.UserResponseData, &upstreamUsage) == nil &&
			upstreamUsage.Usage != nil &&
			upstreamUsage.Usage.CompletionTokens != nil &&
			upstreamUsage.Usage.TotalTokens != nil &&
			IsValidSeedanceUpstreamUsage(
				*upstreamUsage.Usage.CompletionTokens,
				*upstreamUsage.Usage.TotalTokens,
			) {
			response["usage"] = map[string]interface{}{
				"completion_tokens": *upstreamUsage.Usage.CompletionTokens,
				"total_tokens":      *upstreamUsage.Usage.TotalTokens,
			}
		}
	}
	if task.Status == model.TaskStatusSuccess {
		videoURL := taskcommon.BuildProxyURL(task.TaskID)
		if strings.TrimSpace(task.PrivateData.ResultObjectKey) != "" {
			if resolved, err := ResolveTaskResultURL(context.Background(), task); err == nil {
				videoURL = resolved
			}
		}
		response["content"] = map[string]interface{}{
			"video_url": videoURL,
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
	result.Resolution = strings.ToLower(strings.TrimSpace(result.Resolution))
	switch result.Resolution {
	case "480p", "720p", "1080p", "4k":
	default:
		result.Resolution = defaultSeedanceResolution
	}
	result.Ratio = strings.ToLower(strings.TrimSpace(result.Ratio))
	switch result.Ratio {
	case "16:9", "4:3", "1:1", "3:4", "9:16", "21:9", "adaptive":
	default:
		result.Ratio = defaultSeedanceRatio
	}
	result.ServiceTier = strings.ToLower(strings.TrimSpace(result.ServiceTier))
	if result.ServiceTier != "default" && result.ServiceTier != "flex" {
		result.ServiceTier = defaultSeedanceServiceTier
	}

	result.ID = task.TaskID
	result.Model = requestModel
	result.Status = SeedanceTaskStatus(task.Status)
	if task.Status == model.TaskStatusSuccess {
		videoURL := taskcommon.BuildProxyURL(task.TaskID)
		if strings.TrimSpace(task.PrivateData.ResultObjectKey) != "" {
			if resolved, err := ResolveTaskResultURL(context.Background(), task); err == nil {
				videoURL = resolved
			}
		}
		result.Content = &dto.PublicTaskContent{VideoURL: videoURL}
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
