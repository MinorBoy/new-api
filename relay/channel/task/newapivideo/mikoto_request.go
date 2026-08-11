package newapivideo

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
)

type mikotoDialect string

const (
	mikotoDialectUnknown  mikotoDialect = ""
	mikotoDialectSora     mikotoDialect = "sora"
	mikotoDialectSeedance mikotoDialect = "seedance"
)

var mikotoSeedanceModels = map[string]struct{}{
	"seedance-2.0-1080p": {},
	"seedance-2.0-720p":  {},
	"seedance-fast-480p": {},
	"seedance-fast-720p": {},
}

type MikotoModelContract struct {
	Dialect             string
	OutputResolution    string
	ReferenceTotalLimit int
}

func AnalyzeMikotoModel(modelName string) (MikotoModelContract, bool) {
	modelName = strings.ToLower(strings.TrimSpace(modelName))
	if modelName == "sora-v3-pro" {
		return MikotoModelContract{Dialect: string(mikotoDialectSora), OutputResolution: "720p", ReferenceTotalLimit: 12}, true
	}
	if _, ok := mikotoSeedanceModels[modelName]; !ok {
		return MikotoModelContract{}, false
	}
	resolution := "720p"
	if strings.HasSuffix(modelName, "-1080p") {
		resolution = "1080p"
	} else if strings.HasSuffix(modelName, "-480p") {
		resolution = "480p"
	}
	return MikotoModelContract{Dialect: string(mikotoDialectSeedance), OutputResolution: resolution}, true
}

func mikotoRequestDialect(modelName string) mikotoDialect {
	contract, ok := AnalyzeMikotoModel(modelName)
	if !ok {
		return mikotoDialectUnknown
	}
	return mikotoDialect(contract.Dialect)
}

type mikotoSoraRequest struct {
	Model              string                 `json:"model"`
	Prompt             string                 `json:"prompt"`
	Seconds            *string                `json:"seconds,omitempty"`
	AspectRatio        *string                `json:"aspect_ratio,omitempty"`
	Resolution         *string                `json:"resolution,omitempty"`
	ImageURL           *string                `json:"image_url,omitempty"`
	ReferenceImageURLs []string               `json:"reference_image_urls,omitempty"`
	ReferenceVideos    []string               `json:"reference_videos,omitempty"`
	AudioURL           any                    `json:"audio_url,omitempty"`
	VideoConfig        *mikotoSoraVideoConfig `json:"video_config,omitempty"`
}

type mikotoSoraVideoConfig struct {
	ReferenceMode string `json:"reference_mode"`
}

type mikotoSeedanceRequest struct {
	Model           string   `json:"model"`
	Prompt          string   `json:"prompt"`
	Duration        *int     `json:"duration,omitempty"`
	AspectRatio     *string  `json:"aspect_ratio,omitempty"`
	Images          []string `json:"images,omitempty"`
	ReferenceMode   *string  `json:"reference_mode,omitempty"`
	ReferenceVideos []string `json:"referenceVideos,omitempty"`
	ReferenceAudios []string `json:"referenceAudios,omitempty"`
	GenerateAudio   *bool    `json:"generate_audio,omitempty"`
}

var mikotoSoraRatios = map[string]struct{}{
	"16:9": {}, "9:16": {}, "4:3": {}, "3:4": {}, "1:1": {}, "21:9": {},
}

var mikotoSeedanceRatios = map[string]struct{}{
	"16:9": {}, "9:16": {}, "4:3": {}, "3:4": {}, "1:1": {},
}

func validateMikotoRequest(request arkRequest, upstreamModel string) error {
	switch mikotoRequestDialect(upstreamModel) {
	case mikotoDialectSora:
		return validateMikotoSoraRequest(request)
	case mikotoDialectSeedance:
		return validateMikotoSeedanceRequest(request, upstreamModel)
	default:
		return &arkRequestError{Code: "InvalidParameter.model", Message: "Mikoto upstream model is not verified"}
	}
}

func validateMikotoSeedanceRequest(request arkRequest, upstreamModel string) error {
	if err := validateARKSemantics(request, mikotoProtocolProfile()); err != nil {
		return err
	}
	if request.Duration == nil || *request.Duration < 4 || *request.Duration > 15 {
		return &arkRequestError{Code: "InvalidParameter.duration", Message: "Mikoto Seedance duration must be between 4 and 15"}
	}
	if request.Ratio != nil {
		if _, ok := mikotoSeedanceRatios[strings.TrimSpace(*request.Ratio)]; !ok {
			return &arkRequestError{Code: "InvalidParameter.ratio", Message: "Mikoto Seedance aspect ratio is unsupported"}
		}
	}
	if request.Resolution != nil {
		if err := validateMappedResolution(*request.Resolution, upstreamModel); err != nil {
			return &arkRequestError{Code: "InvalidParameter.resolution", Message: err.Error()}
		}
	}
	if request.Watermark != nil {
		return &arkRequestError{Code: "InvalidParameter.watermark", Message: "Mikoto Seedance does not support watermark"}
	}
	if request.Draft != nil {
		return &arkRequestError{Code: "InvalidParameter.draft", Message: "Mikoto Seedance does not support draft"}
	}
	if request.ServiceTier != nil {
		return &arkRequestError{Code: "InvalidParameter.service_tier", Message: "Mikoto Seedance does not support service_tier"}
	}
	if request.Tools != nil {
		return &arkRequestError{Code: "InvalidParameter.tools", Message: "Mikoto Seedance does not support tools"}
	}
	for _, item := range request.Content {
		switch item.Type {
		case "image_url":
			if !validMikotoMediaURL(item.ImageURL.URL, "image", true) {
				return &arkRequestError{Code: "InvalidParameter.content", Message: "Mikoto Seedance image URL is invalid"}
			}
		case "video_url":
			if !validMikotoMediaURL(item.VideoURL.URL, "video", true) {
				return &arkRequestError{Code: "InvalidParameter.content", Message: "Mikoto Seedance video URL is invalid"}
			}
		case "audio_url":
			if !validMikotoMediaURL(item.AudioURL.URL, "audio", true) {
				return &arkRequestError{Code: "InvalidParameter.content", Message: "Mikoto Seedance audio URL is invalid"}
			}
		}
	}
	return nil
}

func validateMikotoSoraRequest(request arkRequest) error {
	profile := mikotoProtocolProfile()
	profile.allowEmbeddedMedia = false
	profile.requirePublicHTTPMedia = true
	if err := validateARKSemantics(request, profile); err != nil {
		return err
	}
	if request.Duration == nil || *request.Duration < 4 || *request.Duration > 15 {
		return &arkRequestError{Code: "InvalidParameter.duration", Message: "Mikoto Sora duration must be between 4 and 15"}
	}
	if request.Ratio == nil {
		return &arkRequestError{Code: "MissingParameter.ratio", Message: "Mikoto Sora ratio is required"}
	}
	if _, ok := mikotoSoraRatios[strings.TrimSpace(*request.Ratio)]; !ok {
		return &arkRequestError{Code: "InvalidParameter.ratio", Message: "Mikoto Sora aspect ratio is unsupported"}
	}
	if request.Resolution == nil {
		return &arkRequestError{Code: "MissingParameter.resolution", Message: "Mikoto Sora resolution is required"}
	}
	if !strings.EqualFold(strings.TrimSpace(*request.Resolution), "720p") {
		return &arkRequestError{Code: "InvalidParameter.resolution", Message: "Mikoto Sora resolution must be 720p"}
	}
	if request.GenerateAudio != nil {
		return &arkRequestError{Code: "InvalidParameter.generate_audio", Message: "Mikoto Sora does not support generate_audio"}
	}
	if request.Watermark != nil {
		return &arkRequestError{Code: "InvalidParameter.watermark", Message: "Mikoto Sora does not support watermark"}
	}
	if request.Draft != nil {
		return &arkRequestError{Code: "InvalidParameter.draft", Message: "Mikoto Sora does not support draft"}
	}
	if request.ServiceTier != nil {
		return &arkRequestError{Code: "InvalidParameter.service_tier", Message: "Mikoto Sora does not support service_tier"}
	}
	if request.Tools != nil {
		return &arkRequestError{Code: "InvalidParameter.tools", Message: "Mikoto Sora does not support tools"}
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
	if images+videos+audios > 12 {
		return &arkRequestError{Code: "InvalidParameter.content", Message: "Mikoto Sora accepts at most 12 reference assets"}
	}
	if audios > 0 && images == 0 {
		return &arkRequestError{Code: "InvalidParameter.content", Message: "Mikoto Sora reference audio requires an image"}
	}
	return nil
}

func buildMikotoRequest(request arkRequest, upstreamModel string) ([]byte, error) {
	if err := validateMikotoRequest(request, upstreamModel); err != nil {
		return nil, err
	}
	switch mikotoRequestDialect(upstreamModel) {
	case mikotoDialectSora:
		return buildMikotoSoraRequest(request, upstreamModel)
	case mikotoDialectSeedance:
		return buildMikotoSeedanceRequest(request, upstreamModel)
	default:
		return nil, fmt.Errorf("unsupported Mikoto request dialect")
	}
}

func buildMikotoSeedanceRequest(request arkRequest, upstreamModel string) ([]byte, error) {
	result := mikotoSeedanceRequest{
		Model:         upstreamModel,
		Prompt:        arkPrompt(request.Content),
		Duration:      request.Duration,
		AspectRatio:   request.Ratio,
		GenerateAudio: request.GenerateAudio,
	}
	referenceImages := false
	for _, item := range request.Content {
		switch item.Type {
		case "image_url":
			result.Images = append(result.Images, item.ImageURL.URL)
			referenceImages = referenceImages || item.Role == "reference_image"
		case "video_url":
			result.ReferenceVideos = append(result.ReferenceVideos, item.VideoURL.URL)
		case "audio_url":
			result.ReferenceAudios = append(result.ReferenceAudios, item.AudioURL.URL)
		}
	}
	if len(result.Images) > 0 {
		mode := "frame"
		if referenceImages || len(result.Images) > 2 {
			mode = "media"
		}
		result.ReferenceMode = &mode
	}
	return common.Marshal(result)
}

func validMikotoMediaURL(value, mediaType string, allowData bool) bool {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(value)), "data:") {
		return allowData && validMikotoDataURI(value, mediaType, 50<<20)
	}
	media, err := relaycommon.ParseTaskMediaURL(value)
	if err != nil {
		return false
	}
	if media.Kind != relaycommon.TaskMediaURLHTTP || !strings.HasPrefix(strings.ToLower(media.Value), "https://") {
		return false
	}
	profile := mikotoProtocolProfile()
	profile.allowEmbeddedMedia = false
	profile.requirePublicHTTPMedia = true
	return validMediaURL(media.Value, profile)
}

func validMikotoDataURI(value, mediaType string, maximumBytes int) bool {
	media, err := relaycommon.ParseTaskMediaURL(value)
	if err != nil || media.Kind != relaycommon.TaskMediaURLData || maximumBytes < 0 ||
		!strings.HasPrefix(strings.ToLower(media.Value), "data:"+mediaType+"/") {
		return false
	}
	comma := strings.IndexByte(media.Value, ',')
	payload := media.Value[comma+1:]
	size := base64.StdEncoding.DecodedLen(len(payload))
	if strings.HasSuffix(payload, "==") {
		size -= 2
	} else if strings.HasSuffix(payload, "=") {
		size--
	}
	return size <= maximumBytes
}

func validateMikotoReferenceDurations(ctx context.Context, request arkRequest, upstreamModel string) error {
	if mikotoRequestDialect(upstreamModel) != mikotoDialectSora {
		return nil
	}
	videoTotalMS, audioTotalMS := int64(0), int64(0)
	for _, item := range request.Content {
		switch item.Type {
		case "video_url":
			durationMS, err := service.ResolveReferenceVideoDurationMS(ctx, []string{item.VideoURL.URL})
			if err != nil {
				return err
			}
			if durationMS < 3_000 || durationMS > 10_000 {
				return &arkRequestError{Code: "InvalidParameter.content", Message: "Mikoto Sora reference videos must be between 3 and 10 seconds"}
			}
			videoTotalMS += durationMS
		case "audio_url":
			durationMS, err := service.ResolveReferenceAudioDurationMS(ctx, []string{item.AudioURL.URL})
			if err != nil {
				return err
			}
			if durationMS <= 2_000 || durationMS >= 15_000 {
				return &arkRequestError{Code: "InvalidParameter.content", Message: "Mikoto Sora reference audio must be longer than 2 and shorter than 15 seconds"}
			}
			audioTotalMS += durationMS
		}
	}
	if videoTotalMS > 15_000 || audioTotalMS > 15_000 {
		return &arkRequestError{Code: "InvalidParameter.content", Message: "Mikoto Sora reference media exceeds the 15 second total limit"}
	}
	return nil
}

func buildMikotoSoraRequest(request arkRequest, upstreamModel string) ([]byte, error) {
	seconds := strconv.Itoa(*request.Duration)
	result := mikotoSoraRequest{
		Model:       upstreamModel,
		Prompt:      arkPrompt(request.Content),
		Seconds:     &seconds,
		AspectRatio: request.Ratio,
		Resolution:  request.Resolution,
	}
	images := make([]string, 0, 9)
	audios := make([]string, 0, 3)
	firstFrame, lastFrame := false, false
	for _, item := range request.Content {
		switch item.Type {
		case "image_url":
			images = append(images, item.ImageURL.URL)
			firstFrame = firstFrame || item.Role == "first_frame" || item.Role == ""
			lastFrame = lastFrame || item.Role == "last_frame"
		case "video_url":
			result.ReferenceVideos = append(result.ReferenceVideos, item.VideoURL.URL)
		case "audio_url":
			audios = append(audios, item.AudioURL.URL)
		}
	}
	if len(images) > 0 {
		result.ImageURL = &images[0]
		result.ReferenceImageURLs = append(result.ReferenceImageURLs, images[1:]...)
	}
	if firstFrame {
		mode := "start_frame"
		if lastFrame {
			mode = "start_end"
		}
		result.VideoConfig = &mikotoSoraVideoConfig{ReferenceMode: mode}
	}
	switch len(audios) {
	case 1:
		result.AudioURL = audios[0]
	case 2, 3:
		result.AudioURL = audios
	}
	return common.Marshal(result)
}
