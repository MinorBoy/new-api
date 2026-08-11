package newapivideo

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
)

type fflinkMedia struct {
	URL  string `json:"url"`
	Type string `json:"type"`
}

type fflinkImageReference struct {
	Image fflinkMedia `json:"image"`
}

type fflinkVideoReference struct {
	Video fflinkMedia `json:"video"`
}

type fflinkAudioReference struct {
	Audio fflinkMedia `json:"audio"`
}

type fflinkGuidances struct {
	ImageReference     []fflinkImageReference `json:"image_reference,omitempty"`
	VideoReferenceBase []fflinkVideoReference `json:"video_reference_base,omitempty"`
	AudioReference     []fflinkAudioReference `json:"audio_reference,omitempty"`
}

type fflinkVideoRequest struct {
	Model         string           `json:"model"`
	Prompt        string           `json:"prompt"`
	Resolution    *string          `json:"resolution,omitempty"`
	Duration      *int             `json:"duration,omitempty"`
	AspectRatio   *string          `json:"aspect_ratio,omitempty"`
	Audio         *bool            `json:"audio,omitempty"`
	StartFrameURL *string          `json:"start_frame_url,omitempty"`
	EndFrameURL   *string          `json:"end_frame_url,omitempty"`
	Guidances     *fflinkGuidances `json:"guidances,omitempty"`
}

func validateFFLinkRequest(request arkRequest, upstreamModel string) error {
	profile := fflinkProtocolProfile()
	if err := validateARKSemantics(request, profile); err != nil {
		return err
	}
	if request.Watermark != nil {
		return &arkRequestError{Code: "InvalidParameter.watermark", Message: "watermark is not supported by FYLink"}
	}
	if request.Draft != nil {
		return &arkRequestError{Code: "InvalidParameter.draft", Message: "draft is not supported by FYLink"}
	}
	if request.Duration != nil && (*request.Duration < 4 || *request.Duration > 15) {
		return &arkRequestError{Code: "InvalidParameter.duration", Message: "FYLink duration must be between 4 and 15 seconds"}
	}
	if request.Duration != nil && strings.EqualFold(optionalStringValue(request.Resolution), "1080p") && strings.Contains(strings.ToLower(upstreamModel), "seedance-2.0") && !strings.Contains(strings.ToLower(upstreamModel), "-fast") && !strings.Contains(strings.ToLower(upstreamModel), "-mini") && *request.Duration > 12 {
		return &arkRequestError{Code: "InvalidParameter.duration", Message: "FYLink seedance-2.0 1080p duration must be at most 12 seconds"}
	}
	if request.Resolution != nil && !containsFFLinkResolution(*request.Resolution) {
		return &arkRequestError{Code: "InvalidParameter.resolution", Message: "FYLink resolution must be 480p, 720p, or 1080p"}
	}
	if upstreamModel != "" && !fflinkModelSupportsResolution(upstreamModel, optionalStringValue(request.Resolution)) {
		return &arkRequestError{Code: "InvalidParameter.model", Message: "FYLink model and resolution combination is unsupported"}
	}

	imageCount, videoCount, audioCount := 0, 0, 0
	firstCount, lastCount := 0, 0
	for _, item := range request.Content {
		switch item.Type {
		case "image_url":
			imageCount++
			switch strings.TrimSpace(item.Role) {
			case "first_frame":
				firstCount++
			case "last_frame":
				lastCount++
			case "reference_image":
			default:
				return &arkRequestError{Code: "InvalidParameter.content", Message: "FYLink image role is unsupported"}
			}
		case "video_url":
			videoCount++
		case "audio_url":
			audioCount++
		}
	}
	if imageCount > 4 || videoCount > 3 || audioCount > 1 || imageCount+videoCount+audioCount > 8 {
		return &arkRequestError{Code: "InvalidParameter.content", Message: "FYLink reference media count exceeds the provider limit"}
	}
	if firstCount > 0 && lastCount > 0 && (firstCount != 1 || lastCount != 1) {
		return &arkRequestError{Code: "InvalidParameter.content", Message: "FYLink allows at most one first and one last frame"}
	}
	if firstCount+lastCount > 0 && imageCount > firstCount+lastCount {
		return &arkRequestError{Code: "InvalidParameter.content", Message: "FYLink frame images cannot mix with reference images"}
	}
	if audioCount > 0 && imageCount == 0 && videoCount == 0 {
		return &arkRequestError{Code: "InvalidParameter.content", Message: "FYLink reference audio requires a reference image or video"}
	}
	if request.GenerateAudio != nil && audioCount > 0 && !*request.GenerateAudio {
		return &arkRequestError{Code: "InvalidParameter.generate_audio", Message: "reference audio conflicts with generate_audio=false"}
	}
	return nil
}

func validateFFLinkReferenceDurations(ctx context.Context, request arkRequest) error {
	videoURLs := make([]string, 0, 3)
	for _, item := range request.Content {
		if item.Type == "video_url" {
			videoURLs = append(videoURLs, item.VideoURL.URL)
		}
	}
	if len(videoURLs) == 0 {
		return nil
	}
	durationMS, err := service.ResolveReferenceVideoDurationMS(ctx, videoURLs)
	if err != nil {
		var metadataErr *service.VideoMetadataError
		if errors.As(err, &metadataErr) && metadataErr.Kind == service.VideoMetadataInvalidMedia {
			return &arkRequestError{Code: "InvalidParameter.content", Message: "FYLink reference video is invalid"}
		}
		return fmt.Errorf("reference video metadata unavailable: %w", err)
	}
	if durationMS > 15*1000 {
		return &arkRequestError{Code: "InvalidParameter.content", Message: "FYLink reference video duration exceeds 15 seconds"}
	}
	return nil
}

func buildFFLinkRequest(request arkRequest, upstreamModel string) ([]byte, error) {
	if err := validateFFLinkRequest(request, upstreamModel); err != nil {
		return nil, err
	}
	result := fflinkVideoRequest{
		Model:       upstreamModel,
		Prompt:      arkPrompt(request.Content),
		Resolution:  request.Resolution,
		Duration:    request.Duration,
		AspectRatio: request.Ratio,
		Audio:       request.GenerateAudio,
	}
	guidances := &fflinkGuidances{}
	for _, item := range request.Content {
		switch item.Type {
		case "image_url":
			url := item.ImageURL.URL
			switch strings.TrimSpace(item.Role) {
			case "first_frame":
				result.StartFrameURL = &url
			case "last_frame":
				result.EndFrameURL = &url
			case "reference_image":
				guidances.ImageReference = append(guidances.ImageReference, fflinkImageReference{Image: fflinkMedia{URL: url, Type: "UPLOADED"}})
			}
		case "video_url":
			guidances.VideoReferenceBase = append(guidances.VideoReferenceBase, fflinkVideoReference{Video: fflinkMedia{URL: item.VideoURL.URL, Type: "UPLOADED"}})
		case "audio_url":
			guidances.AudioReference = append(guidances.AudioReference, fflinkAudioReference{Audio: fflinkMedia{URL: item.AudioURL.URL, Type: "UPLOADED"}})
		}
	}
	if len(guidances.ImageReference) > 0 || len(guidances.VideoReferenceBase) > 0 || len(guidances.AudioReference) > 0 {
		result.Guidances = guidances
	}
	return common.Marshal(result)
}

func containsFFLinkResolution(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "480p", "720p", "1080p":
		return true
	default:
		return false
	}
}

func fflinkModelSupportsResolution(model, resolution string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	resolution = strings.ToLower(strings.TrimSpace(resolution))
	if resolution == "" {
		return strings.Contains(model, "seedance-2.0")
	}
	switch {
	case strings.Contains(model, "seedance-2.0-mini"):
		return resolution == "720p"
	case strings.Contains(model, "seedance-2.0-fast"):
		return resolution == "480p" || resolution == "720p"
	case strings.Contains(model, "seedance-2.0"):
		return resolution == "480p" || resolution == "720p" || resolution == "1080p"
	default:
		return false
	}
}
