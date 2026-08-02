package newapivideo

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

type paipuRequest struct {
	Model       string   `json:"model"`
	Prompt      string   `json:"prompt"`
	Duration    *int     `json:"duration,omitempty"`
	AspectRatio *string  `json:"aspect_ratio,omitempty"`
	Resolution  *string  `json:"resolution,omitempty"`
	Images      []string `json:"images,omitempty"`
	Videos      []string `json:"videos,omitempty"`
	Audios      []string `json:"audios,omitempty"`
}

const (
	paipuMaxImages = 9
	paipuMaxVideos = 3
	paipuMaxAudios = 3
)

func validatePaipuRequest(request arkRequest, upstreamModel string) error {
	if err := validateARKSemantics(request, paipuProtocolProfile()); err != nil {
		return err
	}
	if request.GenerateAudio != nil {
		return &arkRequestError{Code: "InvalidParameter.generate_audio", Message: "generate_audio is not supported by Paipu"}
	}
	if request.Watermark != nil {
		return &arkRequestError{Code: "InvalidParameter.watermark", Message: "watermark is not supported by Paipu"}
	}
	if strings.TrimSpace(upstreamModel) == "" {
		return nil
	}

	imageCount, videoCount, audioCount := 0, 0, 0
	for _, item := range request.Content {
		switch item.Type {
		case "image_url":
			role := strings.TrimSpace(item.Role)
			if role != "" && role != "reference_image" {
				return &arkRequestError{Code: "InvalidParameter.content", Message: "Paipu images only support the reference_image role"}
			}
			if !validPaipuMediaURL(item.ImageURL.URL, "image") {
				return &arkRequestError{Code: "InvalidParameter.content", Message: "image_url.url must be a public HTTP(S) URL or a matching data URI"}
			}
			imageCount++
		case "video_url":
			role := strings.TrimSpace(item.Role)
			if role != "" && role != "reference_video" {
				return &arkRequestError{Code: "InvalidParameter.content", Message: "Paipu videos only support the reference_video role"}
			}
			if !validPaipuMediaURL(item.VideoURL.URL, "video") {
				return &arkRequestError{Code: "InvalidParameter.content", Message: "video_url.url must be a public HTTP(S) URL or a matching data URI"}
			}
			videoCount++
		case "audio_url":
			role := strings.TrimSpace(item.Role)
			if role != "" && role != "reference_audio" {
				return &arkRequestError{Code: "InvalidParameter.content", Message: "Paipu audios only support the reference_audio role"}
			}
			if !validPaipuMediaURL(item.AudioURL.URL, "audio") {
				return &arkRequestError{Code: "InvalidParameter.content", Message: "audio_url.url must be a public HTTP(S) URL or a matching data URI"}
			}
			audioCount++
		}
	}
	if imageCount > paipuMaxImages {
		return &arkRequestError{Code: "InvalidParameter.content", Message: fmt.Sprintf("Paipu images exceed the %d item limit", paipuMaxImages)}
	}
	if videoCount > paipuMaxVideos {
		return &arkRequestError{Code: "InvalidParameter.content", Message: fmt.Sprintf("Paipu videos exceed the %d item limit", paipuMaxVideos)}
	}
	if audioCount > paipuMaxAudios {
		return &arkRequestError{Code: "InvalidParameter.content", Message: fmt.Sprintf("Paipu audios exceed the %d item limit", paipuMaxAudios)}
	}
	return nil
}

func validPaipuMediaURL(value, mediaType string) bool {
	media, err := relaycommon.ParseTaskMediaURL(value)
	if err != nil {
		return false
	}
	if media.Kind == relaycommon.TaskMediaURLData {
		return strings.HasPrefix(strings.ToLower(media.Value), "data:"+mediaType+"/")
	}
	if media.Kind != relaycommon.TaskMediaURLHTTP {
		return false
	}
	return validMediaURL(media.Value, paipuProtocolProfile())
}

func buildPaipuRequest(request arkRequest, upstreamModel string) ([]byte, error) {
	if err := validatePaipuRequest(request, upstreamModel); err != nil {
		return nil, err
	}
	result := paipuRequest{
		Model:       upstreamModel,
		Prompt:      arkPrompt(request.Content),
		Duration:    request.Duration,
		AspectRatio: request.Ratio,
		Resolution:  request.Resolution,
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
