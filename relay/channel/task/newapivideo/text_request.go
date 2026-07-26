package newapivideo

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

type textVideoRequest struct {
	Model       string `json:"model"`
	Prompt      string `json:"prompt"`
	Duration    *int   `json:"duration,omitempty"`
	Ratio       string `json:"ratio,omitempty"`
	AspectRatio string `json:"aspect_ratio,omitempty"`
	Resolution  string `json:"resolution,omitempty"`
}

func validateTextVideoRequest(request arkRequest, profile textRequestProfile) error {
	if strings.TrimSpace(request.Model) == "" {
		return &arkRequestError{Code: "MissingParameter.model", Message: "model is required"}
	}
	if len(request.Content) != 1 {
		return &arkRequestError{Code: "InvalidParameter.content", Message: "exactly one non-empty text item is required"}
	}
	content := request.Content[0]
	if content.Type != "text" || strings.TrimSpace(content.Text) == "" || content.ImageURL != nil || content.VideoURL != nil || content.AudioURL != nil || content.DraftTask != nil || strings.TrimSpace(content.Role) != "" {
		return &arkRequestError{Code: "InvalidParameter.content", Message: "exactly one non-empty text item is required"}
	}
	if request.GenerateAudio != nil {
		return &arkRequestError{Code: "InvalidParameter.generate_audio", Message: "generate_audio is not supported by this channel"}
	}
	if request.Draft != nil {
		return &arkRequestError{Code: "InvalidParameter.draft", Message: "draft is not supported by this channel"}
	}
	if request.Tools != nil && len(*request.Tools) != 0 {
		return &arkRequestError{Code: "InvalidParameter.tools", Message: "tools are not supported by this channel"}
	}
	if profile.rejectExplicitServiceTier && request.ServiceTier != nil {
		return &arkRequestError{Code: "InvalidParameter.service_tier", Message: "service_tier is not supported by this channel"}
	}
	if request.Duration != nil {
		if profile.minimumDuration > 0 && *request.Duration < profile.minimumDuration {
			return &arkRequestError{Code: "InvalidParameter.duration", Message: fmt.Sprintf("duration must be between %d and %d", profile.minimumDuration, profile.maximumDuration)}
		}
		if profile.maximumDuration > 0 && *request.Duration > profile.maximumDuration {
			return &arkRequestError{Code: "InvalidParameter.duration", Message: fmt.Sprintf("duration must be between %d and %d", profile.minimumDuration, profile.maximumDuration)}
		}
	}
	if len(profile.allowedRatios) > 0 && request.Ratio != "" {
		allowed := false
		for _, ratio := range profile.allowedRatios {
			if request.Ratio == ratio {
				allowed = true
				break
			}
		}
		if !allowed {
			return &arkRequestError{Code: "InvalidParameter.ratio", Message: "ratio is not supported by this channel"}
		}
	}
	if len(profile.allowedResolutions) > 0 && request.Resolution != "" {
		allowed := false
		for _, resolution := range profile.allowedResolutions {
			if request.Resolution == resolution {
				allowed = true
				break
			}
		}
		if !allowed {
			return &arkRequestError{Code: "InvalidParameter.resolution", Message: "resolution is not supported by this channel"}
		}
	}
	return nil
}

func buildTextVideoRequest(request arkRequest, upstreamModel string, profile textRequestProfile) ([]byte, error) {
	if err := validateTextVideoRequest(request, profile); err != nil {
		return nil, err
	}

	result := textVideoRequest{
		Model:      upstreamModel,
		Prompt:     request.Content[0].Text,
		Duration:   request.Duration,
		Resolution: request.Resolution,
	}
	switch profile.ratioField {
	case "ratio":
		result.Ratio = request.Ratio
	case "aspect_ratio":
		result.AspectRatio = request.Ratio
	default:
		return nil, fmt.Errorf("unsupported text video ratio field %q", profile.ratioField)
	}
	return common.Marshal(result)
}
