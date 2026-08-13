package newapivideo

import (
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

var zzoneRatios = map[string]struct{}{
	"16:9": {},
	"9:16": {},
	"1:1":  {},
}

type zzoneRequest struct {
	Model       string   `json:"model"`
	Prompt      string   `json:"prompt"`
	Seconds     *string  `json:"seconds,omitempty"`
	AspectRatio *string  `json:"aspect_ratio,omitempty"`
	Images      []string `json:"images,omitempty"`
	Videos      []string `json:"videos,omitempty"`
	Audios      []string `json:"audios,omitempty"`
}

func validateZZoneRequest(request arkRequest) error {
	if err := validateARKSemantics(request, zzoneProtocolProfile()); err != nil {
		return err
	}
	if request.Resolution != nil && strings.TrimSpace(*request.Resolution) != "720p" {
		return &arkRequestError{Code: "InvalidParameter.resolution", Message: "ZZone only supports 720p resolution"}
	}
	if request.Seed != nil {
		return &arkRequestError{Code: "InvalidParameter.seed", Message: "seed is not supported by ZZone"}
	}
	if request.Watermark != nil {
		return &arkRequestError{Code: "InvalidParameter.watermark", Message: "watermark is not supported by ZZone"}
	}
	if request.GenerateAudio != nil {
		return &arkRequestError{Code: "InvalidParameter.generate_audio", Message: "generate_audio is not supported by ZZone"}
	}
	if request.ServiceTier != nil {
		return &arkRequestError{Code: "InvalidParameter.service_tier", Message: "service_tier is not supported by ZZone"}
	}
	if request.Draft != nil {
		return &arkRequestError{Code: "InvalidParameter.draft", Message: "draft is not supported by ZZone"}
	}
	if request.Tools != nil {
		return &arkRequestError{Code: "InvalidParameter.tools", Message: "tools are not supported by ZZone"}
	}
	if request.CallbackURL != nil {
		return &arkRequestError{Code: "InvalidParameter.callback_url", Message: "callback_url is not supported by ZZone"}
	}
	if request.Ratio != nil {
		ratio := strings.TrimSpace(*request.Ratio)
		if _, ok := zzoneRatios[ratio]; !ok {
			return &arkRequestError{Code: "InvalidParameter.ratio", Message: "ratio is not supported by ZZone"}
		}
	}

	images, videos, audios := 0, 0, 0
	for _, item := range request.Content {
		switch item.Type {
		case "image_url":
			images++
		case "video_url":
			videos++
		case "audio_url":
			audios++
		}
	}
	if images > 4 || videos > 3 || audios > 1 {
		return &arkRequestError{Code: "InvalidParameter.content", Message: "reference media count exceeds ZZone limits"}
	}
	return nil
}

func buildZZoneRequest(request arkRequest, upstreamModel string) ([]byte, error) {
	if err := validateZZoneRequest(request); err != nil {
		return nil, err
	}
	result := zzoneRequest{
		Model:  upstreamModel,
		Prompt: arkPrompt(request.Content),
	}
	if request.Duration != nil {
		seconds := strconv.Itoa(*request.Duration)
		result.Seconds = &seconds
	}
	if request.Ratio != nil {
		ratio := strings.TrimSpace(*request.Ratio)
		result.AspectRatio = &ratio
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
