package doubao

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

type seedanceNativeRequest struct {
	Model                 string         `json:"model"`
	Content               []ContentItem  `json:"content"`
	ServiceTier           string         `json:"service_tier,omitempty"`
	ExecutionExpiresAfter *dto.IntValue  `json:"execution_expires_after,omitempty"`
	GenerateAudio         *dto.BoolValue `json:"generate_audio,omitempty"`
	Draft                 *dto.BoolValue `json:"draft,omitempty"`
	Priority              *dto.IntValue  `json:"priority,omitempty"`
	Resolution            string         `json:"resolution,omitempty"`
	Ratio                 string         `json:"ratio,omitempty"`
	Duration              *dto.IntValue  `json:"duration,omitempty"`
	Frames                *dto.IntValue  `json:"frames,omitempty"`
	Seed                  *dto.IntValue  `json:"seed,omitempty"`
	CameraFixed           *dto.BoolValue `json:"camera_fixed,omitempty"`
	ReturnLastFrame       *dto.BoolValue `json:"return_last_frame,omitempty"`
	Watermark             *dto.BoolValue `json:"watermark,omitempty"`
	Tools                 []struct {
		Type string `json:"type,omitempty"`
	} `json:"tools,omitempty"`
	SafetyIdentifier string `json:"safety_identifier,omitempty"`
}

type seedanceContentFacts struct {
	imageCount          int
	videoCount          int
	audioCount          int
	textCount           int
	firstFrameCount     int
	lastFrameCount      int
	referenceImageCount int
	hasDraftTask        bool
}

func (a *TaskAdaptor) validateNativeRequest(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	if !strings.HasPrefix(strings.ToLower(c.GetHeader("Content-Type")), "application/json") {
		return nativeTaskError("native ARK task requests must use application/json", "InvalidParameter")
	}

	var request seedanceNativeRequest
	if err := common.UnmarshalBodyReusable(c, &request); err != nil {
		return nativeTaskError("request body contains invalid parameters", "InvalidParameter")
	}
	if strings.TrimSpace(request.Model) == "" {
		return nativeTaskError("model is required", "MissingParameter.model")
	}
	if len(request.Content) == 0 {
		return nativeTaskError("content is required", "MissingParameter.content")
	}

	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return nativeTaskError(err.Error(), "InvalidParameter")
	}
	rawBody, err := storage.Bytes()
	if err != nil {
		return nativeTaskError(err.Error(), "InvalidParameter")
	}
	var metadata map[string]interface{}
	if err := common.Unmarshal(rawBody, &metadata); err != nil {
		return nativeTaskError(err.Error(), "InvalidParameter")
	}

	facts, err := validateSeedanceContent(request.Model, request.Content)
	if err != nil {
		return nativeTaskError(err.Error(), "InvalidParameter.content")
	}
	if seedanceModelFamily(request.Model) != "" {
		if err := validateSeedanceNativeFields(request, facts); err != nil {
			return nativeTaskError(err.Error(), "InvalidParameter")
		}
	}

	prompt := firstSeedanceText(request.Content)
	taskRequest := relaycommon.TaskSubmitReq{
		Model:    request.Model,
		Prompt:   prompt,
		Metadata: metadata,
	}
	if request.Duration != nil {
		taskRequest.Duration = int(*request.Duration)
	}
	relaycommon.StoreTaskRequest(c, info, constant.TaskActionGenerate, taskRequest)

	modelFamily := seedanceModelFamily(request.Model)
	generateAudio := modelFamily == "2.0" || modelFamily == "2.0-fast" || modelFamily == "2.0-mini" || modelFamily == "1.5"
	if request.GenerateAudio != nil {
		generateAudio = bool(*request.GenerateAudio)
	}
	serviceTier := strings.ToLower(strings.TrimSpace(request.ServiceTier))
	if serviceTier == "" {
		serviceTier = "default"
	}
	draft := request.Draft != nil && bool(*request.Draft)
	resolution := strings.ToLower(strings.TrimSpace(request.Resolution))
	if resolution == "" {
		if draft {
			resolution = "480p"
		} else if modelFamily == "1.0" || modelFamily == "1.0-fast" {
			resolution = "1080p"
		} else {
			resolution = "720p"
		}
	}
	c.Set(string(constant.ContextKeyTaskVideoHasInput), facts.videoCount > 0)
	c.Set(string(constant.ContextKeyTaskGenerateAudio), generateAudio)
	c.Set(string(constant.ContextKeyTaskDraft), draft)
	c.Set(string(constant.ContextKeyTaskServiceTier), serviceTier)
	c.Set("task_resolution", resolution)
	return nil
}

func (a *TaskAdaptor) buildNativeRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return nil, err
	}
	rawBody, err := storage.Bytes()
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err := common.Unmarshal(rawBody, &fields); err != nil {
		return nil, fmt.Errorf("invalid ARK request body: %w", err)
	}
	if info.UpstreamModelName == "" {
		var modelName string
		if err := common.Unmarshal(fields["model"], &modelName); err != nil || strings.TrimSpace(modelName) == "" {
			return nil, fmt.Errorf("invalid ARK model")
		}
		info.UpstreamModelName = modelName
	}
	mappedModel, err := common.Marshal(info.UpstreamModelName)
	if err != nil {
		return nil, err
	}
	fields["model"] = mappedModel
	data, err := common.Marshal(fields)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}

func validateSeedanceContent(modelName string, content []ContentItem) (seedanceContentFacts, error) {
	facts := seedanceContentFacts{}
	for _, item := range content {
		switch item.Type {
		case "text":
			if strings.TrimSpace(item.Text) != "" {
				facts.textCount++
			}
		case "image_url":
			if item.ImageURL == nil || strings.TrimSpace(item.ImageURL.URL) == "" {
				return facts, fmt.Errorf("image_url.url is required")
			}
			facts.imageCount++
			role := item.Role
			if role == "" {
				role = "first_frame"
			}
			switch role {
			case "first_frame":
				facts.firstFrameCount++
			case "last_frame":
				facts.lastFrameCount++
			case "reference_image":
				facts.referenceImageCount++
			default:
				return facts, fmt.Errorf("unsupported image role: %s", role)
			}
		case "video_url":
			if item.VideoURL == nil || strings.TrimSpace(item.VideoURL.URL) == "" {
				return facts, fmt.Errorf("video_url.url is required")
			}
			if item.Role != "reference_video" {
				return facts, fmt.Errorf("video role must be reference_video")
			}
			facts.videoCount++
		case "audio_url":
			if item.AudioURL == nil || strings.TrimSpace(item.AudioURL.URL) == "" {
				return facts, fmt.Errorf("audio_url.url is required")
			}
			if item.Role != "reference_audio" {
				return facts, fmt.Errorf("audio role must be reference_audio")
			}
			facts.audioCount++
		case "draft_task":
			if item.DraftTask == nil || strings.TrimSpace(item.DraftTask.ID) == "" {
				return facts, fmt.Errorf("draft_task.id is required")
			}
			facts.hasDraftTask = true
		default:
			return facts, fmt.Errorf("unsupported content type: %s", item.Type)
		}
	}
	if facts.audioCount > 0 && facts.imageCount == 0 && facts.videoCount == 0 {
		return facts, fmt.Errorf("audio input requires an image or video")
	}
	if facts.firstFrameCount > 0 || facts.lastFrameCount > 0 {
		if facts.referenceImageCount > 0 || facts.videoCount > 0 || facts.audioCount > 0 {
			return facts, fmt.Errorf("first/last frame content cannot mix with reference media")
		}
		if facts.firstFrameCount > 1 || facts.lastFrameCount > 1 {
			return facts, fmt.Errorf("first_frame and last_frame each accept at most one image")
		}
		if facts.lastFrameCount > 0 && facts.firstFrameCount != 1 {
			return facts, fmt.Errorf("last_frame requires exactly one first_frame")
		}
	}
	if facts.hasDraftTask && len(content) != 1 {
		return facts, fmt.Errorf("draft_task cannot be combined with other content")
	}
	return facts, nil
}

func validateSeedanceNativeFields(request seedanceNativeRequest, facts seedanceContentFacts) error {
	family := seedanceModelFamily(request.Model)
	if family == "" {
		return fmt.Errorf("unsupported Seedance model: %s", request.Model)
	}
	if facts.imageCount > 9 || facts.videoCount > 3 || facts.audioCount > 3 {
		return fmt.Errorf("reference media count exceeds Seedance 2.0 limits")
	}
	if family != "2.0" && family != "2.0-fast" && family != "2.0-mini" && (facts.videoCount > 0 || facts.audioCount > 0 || facts.referenceImageCount > 0) {
		return fmt.Errorf("reference video/audio and reference_image require Seedance 2.0")
	}
	if family == "2.0-fast" || family == "2.0-mini" {
		if facts.firstFrameCount > 0 && facts.lastFrameCount > 0 {
			return fmt.Errorf("model does not support first and last frame mode")
		}
	}
	if family == "1.0-fast" && facts.lastFrameCount > 0 {
		return fmt.Errorf("model does not support last_frame")
	}
	if facts.hasDraftTask && family != "1.5" {
		return fmt.Errorf("draft_task is supported only by Seedance 1.5 Pro")
	}

	resolution := strings.ToLower(strings.TrimSpace(request.Resolution))
	if resolution != "" {
		allowed := map[string]bool{"480p": true, "720p": true, "1080p": true}
		if family == "2.0" {
			allowed["4k"] = true
		}
		if family == "2.0-fast" || family == "2.0-mini" {
			delete(allowed, "1080p")
		}
		if !allowed[resolution] {
			return fmt.Errorf("resolution %s is not supported by %s", resolution, family)
		}
	}

	if request.Duration != nil {
		duration := int(*request.Duration)
		if duration == -1 {
			if family != "1.5" && family != "2.0" && family != "2.0-fast" && family != "2.0-mini" {
				return fmt.Errorf("duration=-1 is not supported by %s", family)
			}
		} else {
			if duration <= 0 || duration > relaycommon.MaxTaskDurationSeconds {
				return fmt.Errorf("duration must be positive and bounded")
			}
			minDuration, maxDuration := 2, 12
			if family == "1.5" || family == "2.0" || family == "2.0-fast" || family == "2.0-mini" {
				minDuration = 4
			}
			if family == "2.0" || family == "2.0-fast" || family == "2.0-mini" {
				maxDuration = 15
			}
			if duration < minDuration || duration > maxDuration {
				return fmt.Errorf("duration is outside the %s model range", family)
			}
		}
	}
	if request.Frames != nil {
		frames := int(*request.Frames)
		if family != "1.0" && family != "1.0-fast" {
			return fmt.Errorf("frames are not supported by %s", family)
		}
		if frames < 29 || frames > 289 || (frames-25)%4 != 0 {
			return fmt.Errorf("frames must match the ARK frame range")
		}
	}

	serviceTier := strings.ToLower(strings.TrimSpace(request.ServiceTier))
	if serviceTier == "" {
		serviceTier = "default"
	}
	if family == "1.5" {
		if serviceTier != "default" && serviceTier != "flex" {
			return fmt.Errorf("service_tier is invalid")
		}
	} else if serviceTier != "default" {
		return fmt.Errorf("service_tier=%s is not supported by %s", serviceTier, family)
	}

	if request.GenerateAudio != nil && family != "1.5" && family != "2.0" && family != "2.0-fast" && family != "2.0-mini" {
		return fmt.Errorf("generate_audio is not supported by %s", family)
	}
	draft := request.Draft != nil && bool(*request.Draft)
	if draft {
		if family != "1.5" {
			return fmt.Errorf("draft is supported only by Seedance 1.5 Pro")
		}
		if resolution != "" && resolution != "480p" {
			return fmt.Errorf("draft requires 480p")
		}
		if request.ReturnLastFrame != nil && bool(*request.ReturnLastFrame) {
			return fmt.Errorf("draft does not support return_last_frame")
		}
		if serviceTier == "flex" {
			return fmt.Errorf("draft does not support flex service tier")
		}
	}
	if request.Priority != nil {
		if family != "2.0" && family != "2.0-fast" && family != "2.0-mini" {
			return fmt.Errorf("priority is supported only by Seedance 2.0")
		}
		if int(*request.Priority) < 0 || int(*request.Priority) > 9 {
			return fmt.Errorf("priority must be between 0 and 9")
		}
	}
	if request.ExecutionExpiresAfter != nil {
		expires := int(*request.ExecutionExpiresAfter)
		if expires < 3600 || expires > 259200 {
			return fmt.Errorf("execution_expires_after must be between 3600 and 259200")
		}
	}
	if request.Seed != nil {
		if family == "2.0" || family == "2.0-fast" || family == "2.0-mini" {
			return fmt.Errorf("seed is not supported by Seedance 2.0")
		}
		seed := int64(*request.Seed)
		if seed < -1 || seed > int64(1<<32-1) {
			return fmt.Errorf("seed is outside the ARK range")
		}
	}
	if request.CameraFixed != nil && bool(*request.CameraFixed) {
		if family == "2.0" || family == "2.0-fast" || family == "2.0-mini" || facts.firstFrameCount > 0 || facts.lastFrameCount > 0 {
			return fmt.Errorf("camera_fixed is not supported for this request")
		}
	}
	if ratio := strings.ToLower(strings.TrimSpace(request.Ratio)); ratio != "" {
		allowed := map[string]bool{"16:9": true, "4:3": true, "1:1": true, "3:4": true, "9:16": true, "21:9": true, "adaptive": true}
		if !allowed[ratio] || (ratio == "adaptive" && family != "1.5" && family != "2.0" && family != "2.0-fast" && family != "2.0-mini") {
			return fmt.Errorf("ratio %s is not supported by %s", ratio, family)
		}
		if ratio == "adaptive" && (facts.videoCount > 0 || facts.audioCount > 0) {
			return fmt.Errorf("ratio adaptive is supported only for text or image generation")
		}
	}
	for _, tool := range request.Tools {
		if family != "2.0" && family != "2.0-fast" && family != "2.0-mini" || tool.Type != "web_search" {
			return fmt.Errorf("unsupported Seedance tool")
		}
	}
	if request.SafetyIdentifier != "" {
		if len(request.SafetyIdentifier) > 64 {
			return fmt.Errorf("safety_identifier must be at most 64 characters")
		}
		for _, r := range request.SafetyIdentifier {
			if r > unicode.MaxASCII || unicode.IsControl(r) {
				return fmt.Errorf("safety_identifier must contain ASCII characters")
			}
		}
	}
	return nil
}

func firstSeedanceText(content []ContentItem) string {
	for _, item := range content {
		if item.Type == "text" && strings.TrimSpace(item.Text) != "" {
			return item.Text
		}
	}
	return ""
}

func seedanceModelFamily(modelName string) string {
	modelName = strings.ToLower(strings.TrimSpace(modelName))
	switch {
	case strings.HasPrefix(modelName, "doubao-seedance-2-0-fast"):
		return "2.0-fast"
	case strings.HasPrefix(modelName, "doubao-seedance-2-0-mini"):
		return "2.0-mini"
	case strings.HasPrefix(modelName, "doubao-seedance-2-0"):
		return "2.0"
	case strings.HasPrefix(modelName, "doubao-seedance-1-5-pro"):
		return "1.5"
	case strings.HasPrefix(modelName, "doubao-seedance-1-0-pro-fast"):
		return "1.0-fast"
	case strings.HasPrefix(modelName, "doubao-seedance-1-0-pro"):
		return "1.0"
	default:
		return ""
	}
}

func nativeTaskError(message, code string) *dto.TaskError {
	return service.TaskErrorWrapperLocal(fmt.Errorf("%s", message), code, 400)
}
