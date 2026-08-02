package newapivideo

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
)

type eightYesRequest struct {
	Model           string   `json:"model"`
	Prompt          string   `json:"prompt"`
	Duration        *int     `json:"duration,omitempty"`
	Ratio           *string  `json:"ratio,omitempty"`
	Seed            *int64   `json:"seed,omitempty"`
	ReferenceImages []string `json:"referenceImages,omitempty"`
	ReferenceVideos []string `json:"referenceVideos,omitempty"`
	ReferenceAudios []string `json:"referenceAudios,omitempty"`
}

func validateEightYesRequest(request arkRequest, upstreamModel string) error {
	if err := validateARKSemantics(request, eightYesProtocolProfile()); err != nil {
		return err
	}
	if request.Watermark != nil {
		return &arkRequestError{Code: "InvalidParameter.watermark", Message: "watermark is not supported by 8yes"}
	}
	if request.GenerateAudio != nil && !*request.GenerateAudio {
		return &arkRequestError{Code: "InvalidParameter.generate_audio", Message: "generate_audio=false is not supported by 8yes"}
	}
	for _, item := range request.Content {
		if item.Type == "image_url" && strings.TrimSpace(item.Role) == "last_frame" {
			return &arkRequestError{Code: "InvalidParameter.content", Message: "8yes reference arrays do not preserve last_frame semantics"}
		}
	}
	if upstreamModel == "" || request.Resolution == nil {
		return nil
	}
	if err := validateMappedResolution(*request.Resolution, upstreamModel); err != nil {
		return &arkRequestError{Code: "InvalidParameter.resolution", Message: err.Error()}
	}
	return nil
}

func buildEightYesRequest(request arkRequest, upstreamModel string) ([]byte, error) {
	if err := validateEightYesRequest(request, upstreamModel); err != nil {
		return nil, err
	}
	result := eightYesRequest{
		Model:    upstreamModel,
		Prompt:   arkPrompt(request.Content),
		Duration: request.Duration,
		Ratio:    request.Ratio,
	}
	if request.Seed != nil {
		seed, err := request.Seed.Int64()
		if err != nil {
			return nil, &arkRequestError{Code: "InvalidParameter.seed", Message: "seed must be an integer between -1 and 4294967295"}
		}
		result.Seed = &seed
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
