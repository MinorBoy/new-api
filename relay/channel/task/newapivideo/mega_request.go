package newapivideo

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
)

const maxMegaByAIReferenceDurationMS int64 = 15000

type megaByAIRequest struct {
	Model           string   `json:"model"`
	Prompt          string   `json:"prompt"`
	Duration        *int     `json:"duration,omitempty"`
	Ratio           *string  `json:"ratio,omitempty"`
	Resolution      *string  `json:"resolution,omitempty"`
	ReferenceImages []string `json:"referenceImages,omitempty"`
	ReferenceVideos []string `json:"referenceVideos,omitempty"`
	ReferenceAudios []string `json:"referenceAudios,omitempty"`
}

func validateMegaByAIRequest(request arkRequest) error {
	if request.GenerateAudio != nil && !*request.GenerateAudio && hasReferenceAudio(request.Content) {
		return &arkRequestError{Code: "InvalidParameter.generate_audio", Message: "reference audio conflicts with generate_audio=false"}
	}
	if request.Duration != nil && (*request.Duration < 4 || *request.Duration > 15) {
		return &arkRequestError{Code: "InvalidParameter.duration", Message: "MegaByAI duration must be between 4 and 15"}
	}
	if request.Ratio != nil {
		switch *request.Ratio {
		case "16:9", "9:16", "1:1":
		default:
			return &arkRequestError{Code: "InvalidParameter.ratio", Message: "MegaByAI ratio is unsupported"}
		}
	}
	if request.Resolution != nil {
		switch *request.Resolution {
		case "480p", "720p":
		default:
			return &arkRequestError{Code: "InvalidParameter.resolution", Message: "MegaByAI resolution is unsupported"}
		}
	}
	if request.Watermark != nil {
		return &arkRequestError{Code: "InvalidParameter.watermark", Message: "watermark is not supported by MegaByAI"}
	}
	if request.ServiceTier != nil {
		return &arkRequestError{Code: "InvalidParameter.service_tier", Message: "service_tier is not supported by MegaByAI"}
	}
	if request.Draft != nil {
		return &arkRequestError{Code: "InvalidParameter.draft", Message: "draft is not supported by MegaByAI"}
	}
	if request.Tools != nil {
		return &arkRequestError{Code: "InvalidParameter.tools", Message: "tools are not supported by MegaByAI"}
	}
	for _, item := range request.Content {
		if item.Type == "image_url" && strings.TrimSpace(item.Role) == "last_frame" {
			return &arkRequestError{Code: "InvalidParameter.content", Message: "MegaByAI does not support last_frame"}
		}
	}
	return nil
}

func buildMegaByAIRequest(request arkRequest, upstreamModel string) ([]byte, error) {
	if err := validateARKSemantics(request, megaByAIProtocolProfile()); err != nil {
		return nil, err
	}
	if err := validateMegaByAIRequest(request); err != nil {
		return nil, err
	}

	result := megaByAIRequest{
		Model:      upstreamModel,
		Prompt:     arkPrompt(request.Content),
		Duration:   request.Duration,
		Ratio:      request.Ratio,
		Resolution: request.Resolution,
	}
	for _, item := range request.Content {
		switch item.Type {
		case "image_url":
			result.ReferenceImages = append(result.ReferenceImages, item.ImageURL.URL)
		case "video_url":
			result.ReferenceVideos = append(result.ReferenceVideos, item.VideoURL.URL)
		case "audio_url":
			result.ReferenceAudios = append(result.ReferenceAudios, item.AudioURL.URL)
		}
	}
	return common.Marshal(result)
}
