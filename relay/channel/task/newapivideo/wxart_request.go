package newapivideo

import (
	"context"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
)

type wxArtRequest struct {
	Model           string   `json:"model"`
	Prompt          string   `json:"prompt"`
	Ratio           *string  `json:"ratio,omitempty"`
	Duration        *int     `json:"duration,omitempty"`
	Resolution      *string  `json:"resolution,omitempty"`
	FirstImage      string   `json:"first_image,omitempty"`
	LastImage       string   `json:"last_image,omitempty"`
	ReferenceImages []string `json:"referenceImages,omitempty"`
	ReferenceVideos []string `json:"referenceVideos,omitempty"`
	ReferenceAudios []string `json:"referenceAudios,omitempty"`
}

func wxArtModel(model string) (string, bool) {
	compact := strings.NewReplacer("-", "", "_", "", ".", "", " ", "").Replace(strings.ToLower(strings.TrimSpace(model)))
	switch compact {
	case "seedance20", "doubaoseedance20", "doubaoseedance20260128":
		return "seedance2.0", true
	case "seedance25", "doubaoseedance25", "doubaoseedance25260628":
		return "seedance2.5", true
	default:
		return "", false
	}
}

// AnalyzeWxArtModel normalizes a configured upstream model to the two WxArt
// Seedance families accepted by the provider.
func AnalyzeWxArtModel(model string) (string, bool) {
	return wxArtModel(model)
}

func validateWxArtRequest(request arkRequest) error {
	model, ok := wxArtModel(request.Model)
	if !ok {
		return &arkRequestError{Code: "InvalidParameter.model", Message: "model must be seedance2.0 or seedance2.5"}
	}
	if request.Watermark != nil {
		return &arkRequestError{Code: "InvalidParameter.watermark", Message: "watermark is not supported by WxArt"}
	}
	if request.GenerateAudio != nil {
		return &arkRequestError{Code: "InvalidParameter.generate_audio", Message: "generate_audio is not supported by WxArt"}
	}
	if request.Draft != nil {
		return &arkRequestError{Code: "InvalidParameter.draft", Message: "draft is not supported by WxArt"}
	}
	if request.Tools != nil {
		return &arkRequestError{Code: "InvalidParameter.tools", Message: "tools are not supported by WxArt"}
	}
	if request.Seed != nil {
		return &arkRequestError{Code: "InvalidParameter.seed", Message: "seed is not supported by WxArt"}
	}
	if request.CallbackURL != nil {
		return &arkRequestError{Code: "InvalidParameter.callback_url", Message: "callback_url is not supported by WxArt"}
	}
	if request.ServiceTier != nil && strings.ToLower(strings.TrimSpace(*request.ServiceTier)) != "default" {
		return &arkRequestError{Code: "InvalidParameter.service_tier", Message: "only service_tier=default is supported by WxArt"}
	}
	if request.Duration != nil {
		maximum := 15
		if model == "seedance2.5" {
			maximum = 30
		}
		if *request.Duration < 4 || *request.Duration > maximum {
			return &arkRequestError{Code: "InvalidParameter.duration", Message: fmt.Sprintf("duration must be between 4 and %d for %s", maximum, model)}
		}
	}
	if request.Resolution != nil {
		resolution := strings.ToLower(strings.TrimSpace(*request.Resolution))
		allowed := []string{"480p", "720p"}
		if model == "seedance2.0" {
			allowed = []string{"480p", "720p", "1080p", "4k"}
		}
		if !common.StringsContains(allowed, resolution) {
			return &arkRequestError{Code: "InvalidParameter.resolution", Message: "resolution is not supported by " + model}
		}
	}
	if request.Ratio != nil {
		ratio := strings.ToLower(strings.TrimSpace(*request.Ratio))
		if !common.StringsContains([]string{"21:9", "16:9", "4:3", "1:1", "3:4", "9:16", "auto"}, ratio) {
			return &arkRequestError{Code: "InvalidParameter.ratio", Message: "ratio is not supported by WxArt"}
		}
	}

	textCount, images, videos, audios := 0, 0, 0, 0
	first, last := "", ""
	referenceImages := 0
	for _, item := range request.Content {
		switch item.Type {
		case "text":
			if strings.TrimSpace(item.Text) == "" || item.ImageURL != nil || item.VideoURL != nil || item.AudioURL != nil || item.Role != "" {
				return &arkRequestError{Code: "InvalidParameter.content", Message: "text content must contain only a non-empty text field"}
			}
			textCount++
		case "image_url":
			if item.ImageURL == nil || !validMediaURL(item.ImageURL.URL, wxartProtocolProfile()) {
				return &arkRequestError{Code: "InvalidParameter.content", Message: "image_url.url must be a public HTTP(S) URL"}
			}
			images++
			switch strings.TrimSpace(item.Role) {
			case "", "reference_image":
				referenceImages++
			case "first_frame":
				if first != "" {
					return &arkRequestError{Code: "InvalidParameter.content", Message: "first_frame accepts at most one image"}
				}
				first = item.ImageURL.URL
			case "last_frame":
				if last != "" {
					return &arkRequestError{Code: "InvalidParameter.content", Message: "last_frame accepts at most one image"}
				}
				last = item.ImageURL.URL
			default:
				return &arkRequestError{Code: "InvalidParameter.content", Message: "unsupported image role: " + item.Role}
			}
		case "video_url":
			if item.VideoURL == nil || !validMediaURL(item.VideoURL.URL, wxartProtocolProfile()) || item.Role != "reference_video" {
				return &arkRequestError{Code: "InvalidParameter.content", Message: "video content requires a public URL and reference_video role"}
			}
			videos++
		case "audio_url":
			if item.AudioURL == nil || !validMediaURL(item.AudioURL.URL, wxartProtocolProfile()) || item.Role != "reference_audio" {
				return &arkRequestError{Code: "InvalidParameter.content", Message: "audio content requires a public URL and reference_audio role"}
			}
			audios++
		default:
			return &arkRequestError{Code: "InvalidParameter.content", Message: "unsupported content type: " + item.Type}
		}
	}
	if textCount != 1 {
		return &arkRequestError{Code: "InvalidParameter.content", Message: "exactly one non-empty text item is required"}
	}
	if first != "" && (referenceImages > 0 || videos > 0 || audios > 0) {
		return &arkRequestError{Code: "InvalidParameter.content", Message: "first/last frame content cannot mix with reference media"}
	}
	if last != "" && first == "" {
		return &arkRequestError{Code: "InvalidParameter.content", Message: "last_frame requires first_frame"}
	}
	if first != "" && last != "" && request.Ratio != nil && !strings.EqualFold(strings.TrimSpace(*request.Ratio), "auto") {
		return &arkRequestError{Code: "InvalidParameter.ratio", Message: "first/last frame mode only supports ratio=Auto"}
	}
	maxImages, maxVideos, maxAudios, maxTotal := 9, 3, 3, 12
	if model == "seedance2.5" {
		maxImages, maxVideos, maxAudios, maxTotal = 30, 10, 10, 50
	}
	if images > maxImages || videos > maxVideos || audios > maxAudios || images+videos+audios > maxTotal {
		return &arkRequestError{Code: "InvalidParameter.content", Message: "reference media count exceeds " + model + " limits"}
	}
	return nil
}

func buildWxArtRequest(request arkRequest, upstreamModel string) ([]byte, error) {
	model, ok := wxArtModel(upstreamModel)
	if !ok {
		model = strings.TrimSpace(upstreamModel)
	}
	if err := validateWxArtRequest(request); err != nil {
		return nil, err
	}
	result := wxArtRequest{Model: model, Prompt: arkPrompt(request.Content)}
	result.Ratio = request.Ratio
	result.Duration = request.Duration
	result.Resolution = request.Resolution
	for _, item := range request.Content {
		switch item.Type {
		case "image_url":
			switch strings.TrimSpace(item.Role) {
			case "first_frame":
				result.FirstImage = item.ImageURL.URL
			case "last_frame":
				result.LastImage = item.ImageURL.URL
			default:
				result.ReferenceImages = append(result.ReferenceImages, item.ImageURL.URL)
			}
		case "video_url":
			result.ReferenceVideos = append(result.ReferenceVideos, item.VideoURL.URL)
		case "audio_url":
			result.ReferenceAudios = append(result.ReferenceAudios, item.AudioURL.URL)
		}
	}
	if result.Ratio != nil && strings.EqualFold(strings.TrimSpace(*result.Ratio), "auto") {
		value := "Auto"
		result.Ratio = &value
	}
	if result.Resolution != nil {
		value := strings.ToLower(strings.TrimSpace(*result.Resolution))
		result.Resolution = &value
	}
	return common.Marshal(result)
}

func validateWxArtReferenceDurations(ctx context.Context, request arkRequest, model string) error {
	videoURLs := make([]string, 0)
	audioURLs := make([]string, 0)
	for _, item := range request.Content {
		switch item.Type {
		case "video_url":
			videoURLs = append(videoURLs, item.VideoURL.URL)
		case "audio_url":
			audioURLs = append(audioURLs, item.AudioURL.URL)
		}
	}

	maximumVideoMS := int64(15_000)
	minimumVideoMS := int64(1_800)
	if model == "seedance2.5" {
		maximumVideoMS = 30_000
		minimumVideoMS = 0
	}
	var totalVideoMS int64
	for _, rawURL := range videoURLs {
		durationMS, err := service.ResolveReferenceVideoDurationMS(ctx, []string{rawURL})
		if err != nil {
			return err
		}
		if durationMS < minimumVideoMS {
			return &arkRequestError{Code: "InvalidParameter.content", Message: "reference video duration must be between 1.8 and 15 seconds"}
		}
		if durationMS > maximumVideoMS || totalVideoMS > maximumVideoMS-durationMS {
			return &arkRequestError{Code: "InvalidParameter.content", Message: fmt.Sprintf("reference video duration exceeds %d seconds", maximumVideoMS/1000)}
		}
		totalVideoMS += durationMS
	}

	if model == "seedance2.5" {
		for _, rawURL := range audioURLs {
			durationMS, err := service.ResolveReferenceAudioDurationMS(ctx, []string{rawURL})
			if err != nil {
				return err
			}
			if durationMS < 2_000 || durationMS > 30_000 {
				return &arkRequestError{Code: "InvalidParameter.content", Message: "reference audio duration must be between 2 and 30 seconds"}
			}
		}
	}
	return nil
}
