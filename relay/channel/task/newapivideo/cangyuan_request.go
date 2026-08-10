package newapivideo

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
)

type cangyuanVideoRequest struct {
	Model              string       `json:"model"`
	Prompt             string       `json:"prompt"`
	ReferenceImageURLs []string     `json:"reference_image_urls,omitempty"`
	ReferenceVideos    []string     `json:"reference_videos,omitempty"`
	ReferenceAudios    []string     `json:"reference_audios,omitempty"`
	FirstImageURL      *string      `json:"first_image_url,omitempty"`
	LastImageURL       *string      `json:"last_image_url,omitempty"`
	AspectRatio        *string      `json:"aspect_ratio,omitempty"`
	Duration           *int         `json:"duration,omitempty"`
	Resolution         *string      `json:"resolution,omitempty"`
	Audio              *bool        `json:"audio,omitempty"`
	GenerateAudio      *bool        `json:"generate_audio,omitempty"`
	Seed               *json.Number `json:"seed,omitempty"`
}

func validateCangyuanRequest(request arkRequest, profile cangyuanRequestProfile) error {
	validationRequest := request
	// Cangyuan names the output-audio switch `audio`; reference audio is an
	// input asset and is valid even when output audio is explicitly false.
	validationRequest.GenerateAudio = nil
	if err := validateARKSemantics(validationRequest, cangyuanProtocolProfile()); err != nil {
		return err
	}
	if request.Watermark != nil {
		return &arkRequestError{Code: "InvalidParameter.watermark", Message: "watermark is not supported by Cangyuan"}
	}
	if request.ServiceTier != nil {
		return &arkRequestError{Code: "InvalidParameter.service_tier", Message: "service_tier is not supported by Cangyuan"}
	}
	if request.Draft != nil {
		return &arkRequestError{Code: "InvalidParameter.draft", Message: "draft is not supported by Cangyuan"}
	}
	if request.Seed != nil && !profile.supportsSeed && !profile.modelAgnostic {
		return &arkRequestError{Code: "InvalidParameter.seed", Message: "seed is supported only by Cangyuan sd5 Seedance models"}
	}
	prompt := arkPrompt(request.Content)
	if profile.maximumPromptLength > 0 && len([]rune(prompt)) > profile.maximumPromptLength {
		return &arkRequestError{
			Code:    "InvalidParameter.prompt",
			Message: fmt.Sprintf("Cangyuan prompt must be at most %d characters", profile.maximumPromptLength),
		}
	}
	if request.Duration != nil && (*request.Duration < profile.minimumDuration || *request.Duration > profile.maximumDuration) {
		return &arkRequestError{
			Code:    "InvalidParameter.duration",
			Message: fmt.Sprintf("Cangyuan duration must be between %d and %d", profile.minimumDuration, profile.maximumDuration),
		}
	}
	if request.Ratio != nil && !containsCangyuanValue(profile.allowedRatios, *request.Ratio) {
		return &arkRequestError{Code: "InvalidParameter.ratio", Message: "Cangyuan aspect_ratio is unsupported"}
	}
	if request.Resolution != nil && !containsCangyuanValue(profile.allowedResolutions, *request.Resolution) {
		return &arkRequestError{Code: "InvalidParameter.resolution", Message: "Cangyuan resolution must be 480p or 720p"}
	}

	imageCount, videoCount, audioCount := 0, 0, 0
	firstCount, lastCount := 0, 0
	for _, item := range request.Content {
		switch item.Type {
		case "image_url":
			if item.ImageURL == nil || !validCangyuanMediaURL(item.ImageURL.URL, "image") {
				return &arkRequestError{Code: "InvalidParameter.content", Message: "Cangyuan image URL is invalid"}
			}
			imageCount++
			switch strings.TrimSpace(item.Role) {
			case "", "reference_image":
			case "first_frame":
				firstCount++
			case "last_frame":
				lastCount++
			default:
				return &arkRequestError{Code: "InvalidParameter.content", Message: "Cangyuan image role is unsupported"}
			}
		case "video_url":
			if item.VideoURL == nil || !validCangyuanMediaURL(item.VideoURL.URL, "video") || strings.TrimSpace(item.Role) != "reference_video" {
				return &arkRequestError{Code: "InvalidParameter.content", Message: "Cangyuan videos require HTTPS reference_video URLs"}
			}
			videoCount++
		case "audio_url":
			if item.AudioURL == nil || !validCangyuanMediaURL(item.AudioURL.URL, "audio") || strings.TrimSpace(item.Role) != "reference_audio" {
				return &arkRequestError{Code: "InvalidParameter.content", Message: "Cangyuan audios require HTTPS reference_audio URLs"}
			}
			audioCount++
		}
	}
	if imageCount > profile.maximumImages || videoCount > profile.maximumVideos || audioCount > profile.maximumAudios {
		return &arkRequestError{Code: "InvalidParameter.content", Message: "Cangyuan reference media count exceeds the provider limit"}
	}
	if profile.maximumVideoAudio > 0 && videoCount+audioCount > profile.maximumVideoAudio {
		return &arkRequestError{Code: "InvalidParameter.content", Message: "Cangyuan video and audio references share a provider limit"}
	}
	if imageCount+videoCount+audioCount > profile.maximumReferenceTotal {
		return &arkRequestError{Code: "InvalidParameter.content", Message: "Cangyuan total reference media exceeds the provider limit"}
	}
	if videoCount > 0 && imageCount == 0 {
		return &arkRequestError{Code: "InvalidParameter.content", Message: "Cangyuan reference videos require at least one image"}
	}
	if audioCount > 0 && imageCount == 0 {
		return &arkRequestError{Code: "InvalidParameter.content", Message: "Cangyuan reference audios require at least one image"}
	}
	if firstCount > 1 || lastCount > 1 || firstCount != lastCount {
		return &arkRequestError{Code: "InvalidParameter.content", Message: "Cangyuan first_image_url and last_image_url must be provided as a pair"}
	}
	if firstCount > 0 && (imageCount+videoCount+audioCount) > 2 {
		return &arkRequestError{Code: "InvalidParameter.content", Message: "Cangyuan first/last frames cannot mix with reference media"}
	}
	return nil
}

func buildCangyuanRequest(request arkRequest, upstreamModel string, profile cangyuanRequestProfile) ([]byte, error) {
	if err := validateCangyuanRequest(request, profile); err != nil {
		return nil, err
	}
	result := cangyuanVideoRequest{
		Model:       upstreamModel,
		Prompt:      arkPrompt(request.Content),
		AspectRatio: request.Ratio,
		Duration:    request.Duration,
		Resolution:  request.Resolution,
	}
	if profile.sd5Dialect {
		result.GenerateAudio = request.GenerateAudio
		result.Seed = request.Seed
	} else {
		result.Audio = request.GenerateAudio
	}
	for _, item := range request.Content {
		switch item.Type {
		case "image_url":
			switch strings.TrimSpace(item.Role) {
			case "first_frame":
				result.FirstImageURL = common.GetPointer(item.ImageURL.URL)
			case "last_frame":
				result.LastImageURL = common.GetPointer(item.ImageURL.URL)
			default:
				result.ReferenceImageURLs = append(result.ReferenceImageURLs, item.ImageURL.URL)
			}
		case "video_url":
			result.ReferenceVideos = append(result.ReferenceVideos, item.VideoURL.URL)
		case "audio_url":
			result.ReferenceAudios = append(result.ReferenceAudios, item.AudioURL.URL)
		}
	}
	return common.Marshal(result)
}

func validCangyuanMediaURL(value, mediaType string) bool {
	media, err := relaycommon.ParseTaskMediaURL(value)
	if err != nil {
		return false
	}
	if media.Kind == relaycommon.TaskMediaURLData {
		return mediaType == "image" && strings.HasPrefix(strings.ToLower(media.Value), "data:image/")
	}
	if media.Kind != relaycommon.TaskMediaURLHTTP {
		return false
	}
	parsed, err := url.ParseRequestURI(media.Value)
	if err != nil || !strings.EqualFold(parsed.Scheme, "https") {
		return false
	}
	return validMediaURL(media.Value, cangyuanProtocolProfile())
}

func containsCangyuanValue(values []string, value string) bool {
	for _, candidate := range values {
		if strings.EqualFold(strings.TrimSpace(candidate), strings.TrimSpace(value)) {
			return true
		}
	}
	return false
}

func validateCangyuanReferenceDurations(ctx context.Context, request arkRequest, profile cangyuanRequestProfile) error {
	videoURLs := make([]string, 0, profile.maximumVideos)
	audioURLs := make([]string, 0, profile.maximumAudios)
	for _, item := range request.Content {
		switch item.Type {
		case "video_url":
			videoURLs = append(videoURLs, item.VideoURL.URL)
		case "audio_url":
			audioURLs = append(audioURLs, item.AudioURL.URL)
		}
	}
	if len(videoURLs) > 0 {
		durationMS, err := service.ResolveReferenceVideoDurationMS(ctx, videoURLs)
		if err != nil {
			return err
		}
		if durationMS > int64(profile.maximumVideoDuration)*1000 {
			return &arkRequestError{Code: "InvalidParameter.content", Message: fmt.Sprintf("Cangyuan reference video duration exceeds %d seconds", profile.maximumVideoDuration)}
		}
	}
	if len(audioURLs) > 0 {
		durationMS, err := service.ResolveReferenceAudioDurationMS(ctx, audioURLs)
		if err != nil {
			return err
		}
		if durationMS > int64(profile.maximumVideoDuration)*1000 {
			return &arkRequestError{Code: "InvalidParameter.content", Message: fmt.Sprintf("Cangyuan reference audio duration exceeds %d seconds", profile.maximumVideoDuration)}
		}
	}
	return nil
}
