package newapivideo

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
)

var z5apiRatios = map[string]struct{}{
	"1:1": {}, "16:9": {}, "9:16": {}, "4:3": {}, "3:4": {},
}

type z5apiRequest struct {
	Model      string           `json:"model"`
	Prompt     string           `json:"prompt"`
	Media      []z5apiMedia     `json:"media,omitempty"`
	Parameters *z5apiParameters `json:"parameters,omitempty"`
}

type z5apiMedia struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

type z5apiParameters struct {
	Resolution *string `json:"resolution,omitempty"`
	Ratio      *string `json:"ratio,omitempty"`
	Duration   *int    `json:"duration,omitempty"`
}

func validateZ5APIRequest(request arkRequest) error {
	if err := validateARKSemantics(request, z5apiProtocolProfile()); err != nil {
		return err
	}
	if request.Watermark != nil {
		return &arkRequestError{Code: "InvalidParameter.watermark", Message: "watermark is not supported by Z5API"}
	}
	if request.GenerateAudio != nil {
		return &arkRequestError{Code: "InvalidParameter.generate_audio", Message: "generate_audio is not supported by Z5API"}
	}
	if request.Draft != nil {
		return &arkRequestError{Code: "InvalidParameter.draft", Message: "draft is not supported by Z5API"}
	}
	if request.Tools != nil {
		return &arkRequestError{Code: "InvalidParameter.tools", Message: "tools are not supported by Z5API"}
	}
	if request.Ratio != nil {
		ratio := strings.TrimSpace(*request.Ratio)
		if _, ok := z5apiRatios[ratio]; !ok {
			return &arkRequestError{Code: "InvalidParameter.ratio", Message: "ratio is not supported by Z5API"}
		}
	}
	return nil
}

func buildZ5APIRequest(request arkRequest, upstreamModel string) ([]byte, error) {
	if err := validateZ5APIRequest(request); err != nil {
		return nil, err
	}
	result := z5apiRequest{
		Model:  upstreamModel,
		Prompt: arkPrompt(request.Content),
	}
	if request.Resolution != nil || request.Ratio != nil || request.Duration != nil {
		result.Parameters = &z5apiParameters{
			Resolution: request.Resolution,
			Ratio:      request.Ratio,
			Duration:   request.Duration,
		}
	}
	for _, item := range request.Content {
		switch item.Type {
		case "image_url":
			mediaType := strings.TrimSpace(item.Role)
			if mediaType == "" {
				mediaType = "first_frame"
			}
			result.Media = append(result.Media, z5apiMedia{Type: mediaType, URL: item.ImageURL.URL})
		case "video_url":
			result.Media = append(result.Media, z5apiMedia{Type: "reference_video", URL: item.VideoURL.URL})
		case "audio_url":
			result.Media = append(result.Media, z5apiMedia{Type: "reference_voice", URL: item.AudioURL.URL})
		}
	}
	return common.Marshal(result)
}
