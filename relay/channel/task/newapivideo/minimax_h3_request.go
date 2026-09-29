package newapivideo

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/modelrouting"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

// requestUsesMiniMaxH3Protocol reports whether the request is an H3 request that
// must use the MiniMax H3 dialect. The client model is the canonical minimax-h3
// identity; provider upstream IDs are channel-level data and are resolved later,
// so capability routing facts count as evidence too.
func requestUsesMiniMaxH3Protocol(c *gin.Context, info *relaycommon.RelayInfo) bool {
	if info != nil {
		if modelrouting.IsMiniMaxH3Canonical(info.OriginModelName) {
			return true
		}
		// UpstreamModelName is promoted from the embedded ChannelMeta pointer, so
		// it must be read only when that pointer is set.
		if info.ChannelMeta != nil && modelrouting.IsMiniMaxH3Canonical(info.ChannelMeta.UpstreamModelName) {
			return true
		}
	}
	if c == nil {
		return false
	}
	if input, ok := common.GetContextKeyType[modelrouting.FactsInput](c, constant.ContextKeyRoutingFactsInput); ok && modelrouting.IsMiniMaxH3Canonical(input.CanonicalModel) {
		return true
	}
	if facts, ok := common.GetContextKeyType[modelrouting.Facts](c, constant.ContextKeyRoutingFacts); ok && modelrouting.IsMiniMaxH3Canonical(facts.CanonicalModel) {
		return true
	}
	return false
}

type minimaxH3Request struct {
	Model       string   `json:"model"`
	Prompt      string   `json:"prompt"`
	Mode        string   `json:"mode"`
	Duration    *int     `json:"duration,omitempty"`
	AspectRatio *string  `json:"aspect_ratio,omitempty"`
	Resolution  *string  `json:"resolution,omitempty"`
	Images      []string `json:"images,omitempty"`
	Videos      []string `json:"videos,omitempty"`
	Audios      []string `json:"audios,omitempty"`
}

func validateMiniMaxH3Request(request arkRequest, upstreamModel string) error {
	contract, ok := modelrouting.MiniMaxH3Contract(modelrouting.MiniMaxH3)
	if !ok {
		return &arkRequestError{Code: "internal_error", Message: "MiniMax H3 contract is unavailable"}
	}
	// The upstream model is a channel-level provider ID. Any non-empty ID is
	// accepted; the route contract already rejects Seedance canonical models.
	if strings.TrimSpace(upstreamModel) == "" {
		return &arkRequestError{Code: "InvalidParameter.model", Message: "MiniMax H3 requires an explicit upstream model"}
	}
	if strings.TrimSpace(request.Model) == "" {
		return &arkRequestError{Code: "MissingParameter.model", Message: "model is required"}
	}
	if len(request.Content) == 0 {
		return &arkRequestError{Code: "MissingParameter.content", Message: "content is required"}
	}
	if request.GenerateAudio != nil {
		return &arkRequestError{Code: "InvalidParameter.generate_audio", Message: "generate_audio is not supported by MiniMax H3"}
	}
	if request.Watermark != nil {
		return &arkRequestError{Code: "InvalidParameter.watermark", Message: "watermark is not supported by MiniMax H3"}
	}
	if request.Seed != nil {
		return &arkRequestError{Code: "InvalidParameter.seed", Message: "seed is not supported by the UniArt MiniMax H3 endpoint"}
	}
	if request.Duration != nil && (*request.Duration < contract.MinDurationSeconds || *request.Duration > contract.MaxDurationSeconds) {
		return &arkRequestError{Code: "InvalidParameter.duration", Message: fmt.Sprintf("duration must be between %d and %d", contract.MinDurationSeconds, contract.MaxDurationSeconds)}
	}
	if request.Resolution != nil && !containsString(contract.OutputResolutions, strings.ToLower(strings.TrimSpace(*request.Resolution))) {
		return &arkRequestError{Code: "InvalidParameter.resolution", Message: "resolution is not supported by MiniMax H3"}
	}
	if request.Ratio != nil && !containsString(contract.AspectRatios, strings.ToLower(strings.TrimSpace(*request.Ratio))) {
		return &arkRequestError{Code: "InvalidParameter.ratio", Message: "ratio is not supported by MiniMax H3"}
	}

	textCount, imageCount, videoCount, audioCount := 0, 0, 0, 0
	for _, item := range request.Content {
		switch item.Type {
		case "text":
			if strings.TrimSpace(item.Text) == "" || strings.TrimSpace(item.Role) != "" || item.ImageURL != nil || item.VideoURL != nil || item.AudioURL != nil || item.DraftTask != nil {
				return &arkRequestError{Code: "InvalidParameter.content", Message: "text content must contain only a non-empty text field"}
			}
			textCount++
		case "image_url":
			if item.ImageURL == nil || !validMediaURL(item.ImageURL.URL, minimaxH3ProtocolProfile()) || item.VideoURL != nil || item.AudioURL != nil || item.DraftTask != nil {
				return &arkRequestError{Code: "InvalidParameter.content", Message: "MiniMax H3 images must be public HTTP(S) reference images"}
			}
			// The H3 contract allows reference images and first/last frame roles;
			// an untyped image defaults to a reference image.
			switch strings.TrimSpace(item.Role) {
			case "", "reference_image", "first_frame", "last_frame":
			default:
				return &arkRequestError{Code: "InvalidParameter.content", Message: "unsupported MiniMax H3 image role: " + item.Role}
			}
			imageCount++
		case "video_url":
			if item.VideoURL == nil || !validMediaURL(item.VideoURL.URL, minimaxH3ProtocolProfile()) || item.ImageURL != nil || item.AudioURL != nil || item.DraftTask != nil || strings.TrimSpace(item.Role) != "reference_video" {
				return &arkRequestError{Code: "InvalidParameter.content", Message: "MiniMax H3 videos require the reference_video role"}
			}
			videoCount++
		case "audio_url":
			if item.AudioURL == nil || !validMediaURL(item.AudioURL.URL, minimaxH3ProtocolProfile()) || item.ImageURL != nil || item.VideoURL != nil || item.DraftTask != nil || strings.TrimSpace(item.Role) != "reference_audio" {
				return &arkRequestError{Code: "InvalidParameter.content", Message: "MiniMax H3 audios require the reference_audio role"}
			}
			audioCount++
		default:
			return &arkRequestError{Code: "InvalidParameter.content", Message: "unsupported MiniMax H3 content type"}
		}
	}
	if textCount != 1 {
		return &arkRequestError{Code: "InvalidParameter.content", Message: "exactly one non-empty text item is required"}
	}
	if imageCount > contract.ReferenceLimits.Images || videoCount > contract.ReferenceLimits.Videos || audioCount > contract.ReferenceLimits.Audios || imageCount+videoCount+audioCount > contract.ReferenceTotalMax {
		return &arkRequestError{Code: "InvalidParameter.content", Message: "MiniMax H3 reference media count exceeds the verified limits"}
	}
	if audioCount > 0 && imageCount == 0 && videoCount == 0 {
		return &arkRequestError{Code: "InvalidParameter.content", Message: "audio input requires an image or video"}
	}
	return nil
}

func buildMiniMaxH3Request(request arkRequest, upstreamModel string) ([]byte, error) {
	if err := validateMiniMaxH3Request(request, upstreamModel); err != nil {
		return nil, err
	}
	contract, _ := modelrouting.MiniMaxH3Contract(modelrouting.MiniMaxH3)
	// The provider gateway requires an explicit generation mode with its own
	// enum (text2video / image2video / reference2video / frames2video); derive
	// it from the content roles instead of asking clients to pass a
	// provider-specific field through the unified new-api video entry.
	mode := "text2video"
	hasFrameRole := false
	hasReference := false
	for _, item := range request.Content {
		switch item.Type {
		case "image_url":
			switch strings.TrimSpace(item.Role) {
			case "first_frame", "last_frame":
				hasFrameRole = true
			default:
				hasReference = true
			}
		case "video_url", "audio_url":
			hasReference = true
		}
	}
	if hasFrameRole {
		mode = "frames2video"
	} else if hasReference {
		mode = "reference2video"
	}
	result := minimaxH3Request{
		Model:       upstreamModel,
		Prompt:      arkPrompt(request.Content),
		Mode:        mode,
		Duration:    request.Duration,
		AspectRatio: request.Ratio,
		Resolution:  request.Resolution,
	}
	if result.Duration == nil {
		result.Duration = common.GetPointer(contract.DefaultDuration)
	}
	if result.AspectRatio == nil {
		result.AspectRatio = common.GetPointer(contract.DefaultAspectRatio)
	}
	if result.Resolution == nil {
		result.Resolution = common.GetPointer(contract.DefaultResolution)
	}
	for _, item := range request.Content {
		switch item.Type {
		case "image_url":
			result.Images = append(result.Images, item.ImageURL.URL)
		case "video_url":
			result.Videos = append(result.Videos, item.VideoURL.URL)
		case "audio_url":
			result.Audios = append(result.Audios, item.AudioURL.URL)
		}
	}
	return common.Marshal(result)
}

func containsString(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func validMiniMaxH3Resolution(value string) bool {
	contract, _ := modelrouting.MiniMaxH3Contract(modelrouting.MiniMaxH3)
	return containsString(contract.OutputResolutions, strings.ToLower(strings.TrimSpace(value)))
}

func validMiniMaxH3Ratio(value string) bool {
	contract, _ := modelrouting.MiniMaxH3Contract(modelrouting.MiniMaxH3)
	return containsString(contract.AspectRatios, strings.ToLower(strings.TrimSpace(value)))
}

func validMiniMaxH3Media(value string) bool {
	_, err := relaycommon.ParseTaskMediaURL(value)
	return err == nil
}
