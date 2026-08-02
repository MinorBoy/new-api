package newapivideo

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
)

type omegaMediaRequest struct {
	Model       string   `json:"model"`
	Prompt      string   `json:"prompt"`
	Duration    *int     `json:"duration,omitempty"`
	AspectRatio *string  `json:"aspect_ratio,omitempty"`
	Images      []string `json:"images,omitempty"`
	Videos      []string `json:"videos,omitempty"`
	Audios      []string `json:"audios,omitempty"`
}

func validateOmegaAIRequest(request arkRequest, profile omegaRequestProfile, upstreamModel string) error {
	if err := validateARKSemantics(request, omegaAIProtocolProfile()); err != nil {
		return err
	}
	if request.Resolution != nil && strings.ToLower(strings.TrimSpace(*request.Resolution)) != "720p" {
		return &arkRequestError{Code: "InvalidParameter.resolution", Message: "OmegaAI supports only 720p resolution"}
	}
	if request.GenerateAudio != nil {
		return &arkRequestError{Code: "InvalidParameter.generate_audio", Message: "generate_audio is not supported by OmegaAI"}
	}
	if request.Watermark != nil {
		return &arkRequestError{Code: "InvalidParameter.watermark", Message: "watermark is not supported by OmegaAI"}
	}
	if request.Draft != nil {
		return &arkRequestError{Code: "InvalidParameter.draft", Message: "draft is not supported by OmegaAI"}
	}

	imageCount, videoCount, audioCount := 0, 0, 0
	for _, item := range request.Content {
		switch item.Type {
		case "image_url":
			if strings.TrimSpace(item.Role) != "reference_image" {
				return &arkRequestError{Code: "InvalidParameter.content", Message: "OmegaAI images require the reference_image role"}
			}
			imageCount++
		case "video_url":
			videoCount++
		case "audio_url":
			audioCount++
		}
	}
	if imageCount > profile.MaxImages || videoCount > profile.MaxVideos || audioCount > profile.MaxAudios {
		return &arkRequestError{Code: "InvalidParameter.content", Message: "reference media count exceeds OmegaAI limits"}
	}
	if upstreamModel == "" {
		return nil
	}

	knownModel := false
	for _, modelName := range omegaAIProtocolProfile().modelList {
		if upstreamModel == modelName {
			knownModel = true
			break
		}
	}
	if !knownModel {
		return &arkRequestError{Code: "InvalidParameter.model", Message: "mapped model is not supported by OmegaAI"}
	}
	if upstreamModel != "klsdpro2-720p" && (videoCount > 0 || audioCount > 0) {
		return &arkRequestError{Code: "InvalidParameter.content", Message: "mapped OmegaAI model supports only text and reference images"}
	}
	return nil
}

func buildOmegaAIRequest(request arkRequest, upstreamModel string, profile omegaRequestProfile) ([]byte, error) {
	if err := validateOmegaAIRequest(request, profile, upstreamModel); err != nil {
		return nil, err
	}
	result := omegaMediaRequest{
		Model:       upstreamModel,
		Prompt:      arkPrompt(request.Content),
		Duration:    request.Duration,
		AspectRatio: request.Ratio,
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
