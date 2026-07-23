package newapivideo

import (
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

var acceptedARKFields = map[string]struct{}{
	"model": {}, "content": {}, "ratio": {}, "resolution": {},
	"duration": {}, "watermark": {}, "generate_audio": {},
	"service_tier": {}, "draft": {}, "tools": {}, "routing": {},
}

type arkRequestError struct {
	Code    string
	Message string
}

func (e *arkRequestError) Error() string {
	return e.Code + ": " + e.Message
}

func parseARKRequest(body []byte) (arkRequest, error) {
	if common.GetJsonType(body) != "object" {
		return arkRequest{}, &arkRequestError{Code: "InvalidParameter", Message: "request body must be a JSON object"}
	}
	var fields map[string]json.RawMessage
	if err := common.Unmarshal(body, &fields); err != nil || fields == nil {
		return arkRequest{}, &arkRequestError{Code: "InvalidParameter", Message: "request body contains invalid parameters"}
	}
	for field := range fields {
		if _, accepted := acceptedARKFields[field]; !accepted {
			return arkRequest{}, &arkRequestError{Code: "InvalidParameter." + field, Message: "field is not supported by this channel"}
		}
	}

	var request arkRequest
	if err := common.Unmarshal(body, &request); err != nil {
		return arkRequest{}, &arkRequestError{Code: "InvalidParameter", Message: "request body contains invalid parameters"}
	}
	if err := validateARKSemantics(request); err != nil {
		return arkRequest{}, err
	}
	return request, nil
}

func validateARKRequest(c *gin.Context, info *relaycommon.RelayInfo, body []byte) *dto.TaskError {
	mediaType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return service.TaskErrorWrapperLocal(fmt.Errorf("ARK task requests must use application/json"), "InvalidParameter", http.StatusUnsupportedMediaType)
	}
	request, err := parseARKRequest(body)
	if err != nil {
		var requestErr *arkRequestError
		if errors.As(err, &requestErr) {
			return service.TaskErrorWrapperLocal(err, requestErr.Code, http.StatusBadRequest)
		}
		return service.TaskErrorWrapperLocal(err, "InvalidParameter", http.StatusBadRequest)
	}

	prompt := arkPrompt(request.Content)
	state := requestState{ARK: &request}
	if request.Duration != nil {
		duration := decimal.NewFromInt(int64(*request.Duration))
		state.Seconds = &duration
	}
	c.Set(requestStateContextKey, state)
	taskRequest := relaycommon.TaskSubmitReq{Model: request.Model, Prompt: prompt}
	if request.Duration != nil {
		taskRequest.Duration = *request.Duration
	}
	relaycommon.StoreTaskRequest(c, info, constant.TaskActionGenerate, taskRequest)
	return nil
}

func buildARKRequestBody(c *gin.Context, info *relaycommon.RelayInfo) ([]byte, error) {
	state, err := getRequestState(c)
	if err != nil {
		return nil, err
	}
	if state.ARK == nil {
		return nil, fmt.Errorf("ARK request state is missing")
	}
	upstreamModel := ""
	if info != nil {
		upstreamModel = info.UpstreamModelName
		if upstreamModel == "" {
			upstreamModel = info.OriginModelName
		}
	}
	request, err := arkToUpstream(
		*state.ARK,
		upstreamModel,
		common.GetContextKeyBool(c, constant.ContextKeyRoutingCapabilityMode),
	)
	if err != nil {
		return nil, err
	}
	return marshalUpstreamRequest(request)
}

func arkToUpstream(request arkRequest, upstreamModel string, resolutionPrevalidated bool) (upstreamRequest, error) {
	if err := validateARKSemantics(request); err != nil {
		return upstreamRequest{}, err
	}
	if !resolutionPrevalidated {
		if err := validateMappedResolution(request.Resolution, upstreamModel); err != nil {
			return upstreamRequest{}, &arkRequestError{Code: "InvalidParameter.resolution", Message: err.Error()}
		}
	}

	result := upstreamRequest{
		Model:         upstreamModel,
		Prompt:        arkPrompt(request.Content),
		GenerateAudio: request.GenerateAudio,
		Ratio:         request.Ratio,
		Watermark:     request.Watermark,
	}
	if request.Duration != nil {
		seconds := strconv.Itoa(*request.Duration)
		result.Seconds = &seconds
	}

	referenceMode := false
	for _, item := range request.Content {
		if item.Type == "video_url" || item.Type == "audio_url" ||
			(item.Type == "image_url" && strings.TrimSpace(item.Role) == "reference_image") {
			referenceMode = true
			break
		}
	}
	if referenceMode {
		result.Content = append([]arkContent(nil), request.Content...)
		if hasReferenceAudio(request.Content) && result.GenerateAudio == nil {
			generateAudio := true
			result.GenerateAudio = &generateAudio
		}
		return result, nil
	}

	roleImages := make([]upstreamRoleImage, 0, 2)
	for _, item := range request.Content {
		if item.Type != "image_url" {
			continue
		}
		role := strings.TrimSpace(item.Role)
		if role == "" {
			role = "first_frame"
		}
		roleImages = append(roleImages, upstreamRoleImage{URL: item.ImageURL.URL, Role: role})
	}
	if len(roleImages) == 1 {
		result.Image = roleImages[0].URL
	} else if len(roleImages) == 2 {
		result.ImageWithRoles = roleImages
	}
	return result, nil
}

func validateARKSemantics(request arkRequest) error {
	if strings.TrimSpace(request.Model) == "" {
		return &arkRequestError{Code: "MissingParameter.model", Message: "model is required"}
	}
	if len(request.Content) == 0 {
		return &arkRequestError{Code: "MissingParameter.content", Message: "content is required"}
	}
	if request.Duration != nil && (*request.Duration <= 0 || *request.Duration > relaycommon.MaxTaskDurationSeconds) {
		return &arkRequestError{Code: "InvalidParameter.duration", Message: fmt.Sprintf("duration must be between 1 and %d", relaycommon.MaxTaskDurationSeconds)}
	}
	if request.ServiceTier != nil && *request.ServiceTier != "default" {
		return &arkRequestError{Code: "InvalidParameter.service_tier", Message: "only service_tier=default is supported"}
	}
	if request.Draft != nil && *request.Draft {
		return &arkRequestError{Code: "InvalidParameter.draft", Message: "draft is not supported by this upstream"}
	}
	if request.Tools != nil && len(*request.Tools) != 0 {
		return &arkRequestError{Code: "InvalidParameter.tools", Message: "tools are not supported by this upstream"}
	}

	textCount, imageCount, videoCount, audioCount := 0, 0, 0, 0
	firstCount, lastCount, referenceImageCount := 0, 0, 0
	firstIndex, lastIndex := -1, -1
	for index, item := range request.Content {
		switch item.Type {
		case "text":
			textCount++
			if strings.TrimSpace(item.Text) == "" || item.ImageURL != nil || item.VideoURL != nil || item.AudioURL != nil || item.DraftTask != nil || strings.TrimSpace(item.Role) != "" {
				return &arkRequestError{Code: "InvalidParameter.content", Message: "text content must contain only a non-empty text field"}
			}
		case "image_url":
			imageCount++
			if item.ImageURL == nil || !validMediaURL(item.ImageURL.URL) || item.VideoURL != nil || item.AudioURL != nil || item.DraftTask != nil || strings.TrimSpace(item.Text) != "" {
				return &arkRequestError{Code: "InvalidParameter.content", Message: "image_url.url must be a valid HTTP URL"}
			}
			switch strings.TrimSpace(item.Role) {
			case "", "first_frame":
				firstCount++
				if firstIndex == -1 {
					firstIndex = index
				}
			case "last_frame":
				lastCount++
				lastIndex = index
			case "reference_image":
				referenceImageCount++
			default:
				return &arkRequestError{Code: "InvalidParameter.content", Message: "unsupported image role: " + item.Role}
			}
		case "video_url":
			videoCount++
			if item.VideoURL == nil || !validMediaURL(item.VideoURL.URL) || item.ImageURL != nil || item.AudioURL != nil || item.DraftTask != nil || strings.TrimSpace(item.Text) != "" || strings.TrimSpace(item.Role) != "reference_video" {
				return &arkRequestError{Code: "InvalidParameter.content", Message: "video content requires a valid URL and reference_video role"}
			}
		case "audio_url":
			audioCount++
			if item.AudioURL == nil || !validMediaURL(item.AudioURL.URL) || item.ImageURL != nil || item.VideoURL != nil || item.DraftTask != nil || strings.TrimSpace(item.Text) != "" || strings.TrimSpace(item.Role) != "reference_audio" {
				return &arkRequestError{Code: "InvalidParameter.content", Message: "audio content requires a valid URL and reference_audio role"}
			}
		case "draft_task":
			return &arkRequestError{Code: "InvalidParameter.content", Message: "draft_task is not supported by this upstream"}
		default:
			return &arkRequestError{Code: "InvalidParameter.content", Message: "unsupported content type: " + item.Type}
		}
	}

	if textCount != 1 || strings.TrimSpace(arkPrompt(request.Content)) == "" {
		return &arkRequestError{Code: "InvalidParameter.content", Message: "exactly one non-empty text item is required"}
	}
	if imageCount > 9 || videoCount > 3 || audioCount > 3 {
		return &arkRequestError{Code: "InvalidParameter.content", Message: "reference media count exceeds Seedance 2.0 limits"}
	}
	if audioCount > 0 && imageCount == 0 && videoCount == 0 {
		return &arkRequestError{Code: "InvalidParameter.content", Message: "audio input requires an image or video"}
	}
	if (firstCount > 0 || lastCount > 0) && (referenceImageCount > 0 || videoCount > 0 || audioCount > 0) {
		return &arkRequestError{Code: "InvalidParameter.content", Message: "first/last frame content cannot mix with reference media"}
	}
	if firstCount > 1 || lastCount > 1 || (lastCount > 0 && (firstCount != 1 || lastIndex < firstIndex)) {
		return &arkRequestError{Code: "InvalidParameter.content", Message: "first/last frames require one first frame before at most one last frame"}
	}
	if audioCount > 0 && request.GenerateAudio != nil && !*request.GenerateAudio {
		return &arkRequestError{Code: "InvalidParameter.generate_audio", Message: "reference audio conflicts with generate_audio=false"}
	}
	return nil
}

func validateMappedResolution(requested, upstreamModel string) error {
	if requested == "" {
		return nil
	}
	normalized := strings.ToLower(upstreamModel)
	for _, candidate := range []string{"480p", "720p", "1080p"} {
		if strings.Contains(normalized, candidate) {
			if !strings.EqualFold(requested, candidate) {
				return fmt.Errorf("resolution %s does not match mapped model %s", requested, upstreamModel)
			}
			return nil
		}
	}
	return fmt.Errorf("mapped model %s does not declare a resolution tier", upstreamModel)
}

func arkPrompt(content []arkContent) string {
	for _, item := range content {
		if item.Type == "text" && strings.TrimSpace(item.Text) != "" {
			return item.Text
		}
	}
	return ""
}

func hasReferenceAudio(content []arkContent) bool {
	for _, item := range content {
		if item.Type == "audio_url" && strings.TrimSpace(item.Role) == "reference_audio" {
			return true
		}
	}
	return false
}

func validMediaURL(value string) bool {
	parsed, err := url.ParseRequestURI(strings.TrimSpace(value))
	if err != nil || parsed.Host == "" {
		return false
	}
	return parsed.Scheme == "http" || parsed.Scheme == "https"
}

func marshalUpstreamRequest(request upstreamRequest) ([]byte, error) {
	return common.Marshal(request)
}
