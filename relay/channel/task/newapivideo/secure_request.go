package newapivideo

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	taskdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

const maxSecureOverseasReferenceVideoDurationMS int64 = 15000

type secureRequestProfile struct {
	group dto.SecureVideoGroup
}

type secureEnterpriseRequest struct {
	Model              string   `json:"model"`
	Prompt             string   `json:"prompt"`
	Duration           *int     `json:"duration"`
	AspectRatio        *string  `json:"aspect_ratio,omitempty"`
	ImageURL           string   `json:"image_url,omitempty"`
	ExtraImages        []string `json:"extra_images,omitempty"`
	UsePersonCharacter bool     `json:"use_person_character,omitempty"`
	ExtraVideos        []string `json:"extra_videos,omitempty"`
	ExtraAudios        []string `json:"extra_audios,omitempty"`
}

type secureMediaInputs struct {
	images []arkContent
	videos []string
	audios []string
}

func collectSecureMedia(request arkRequest) secureMediaInputs {
	media := secureMediaInputs{}
	for _, item := range request.Content {
		switch item.Type {
		case "image_url":
			media.images = append(media.images, item)
		case "video_url":
			media.videos = append(media.videos, item.VideoURL.URL)
		case "audio_url":
			media.audios = append(media.audios, item.AudioURL.URL)
		}
	}
	return media
}

func validateSecureRequest(request arkRequest, profile secureRequestProfile, upstreamModel string) error {
	if err := validateARKSemantics(request, protocolProfile{
		requirePublicHTTPMedia: true,
	}); err != nil {
		return err
	}
	if request.Watermark != nil {
		return &arkRequestError{Code: "InvalidParameter.watermark", Message: "watermark is not supported by Secure"}
	}
	if request.GenerateAudio != nil {
		return &arkRequestError{Code: "InvalidParameter.generate_audio", Message: "generate_audio is not supported by Secure"}
	}
	if request.ServiceTier != nil && *request.ServiceTier != "default" {
		return &arkRequestError{Code: "InvalidParameter.service_tier", Message: "only service_tier=default is supported by Secure"}
	}
	if request.Draft != nil {
		return &arkRequestError{Code: "InvalidParameter.draft", Message: "draft is not supported by Secure"}
	}
	if request.Tools != nil && len(*request.Tools) != 0 {
		return &arkRequestError{Code: "InvalidParameter.tools", Message: "tools are not supported by Secure"}
	}

	media := collectSecureMedia(request)
	hasLastFrame := false
	for _, image := range media.images {
		if strings.TrimSpace(image.Role) == "last_frame" {
			hasLastFrame = true
			break
		}
	}

	switch profile.group {
	case dto.SecureVideoGroupDiscount:
		if len(media.images) == 0 {
			return &arkRequestError{Code: "InvalidParameter.content", Message: "Secure discount video requires at least one image"}
		}
		if hasLastFrame {
			return &arkRequestError{Code: "InvalidParameter.content", Message: "Secure discount video does not support last_frame"}
		}
		if len(media.videos)+len(media.audios) > 3 {
			return &arkRequestError{Code: "InvalidParameter.content", Message: "Secure discount video supports at most three video and audio inputs combined"}
		}
		if request.Duration != nil && (*request.Duration < 4 || *request.Duration > 15) {
			return &arkRequestError{Code: "InvalidParameter.duration", Message: "Secure discount duration must be between 4 and 15"}
		}
		if request.Ratio != nil {
			switch *request.Ratio {
			case "16:9", "9:16":
			default:
				return &arkRequestError{Code: "InvalidParameter.ratio", Message: "Secure discount ratio is unsupported"}
			}
		}
		if request.Resolution != nil {
			switch *request.Resolution {
			case "720p", "1080p", "4k":
			default:
				return &arkRequestError{Code: "InvalidParameter.resolution", Message: "Secure discount resolution is unsupported"}
			}
		}
	case dto.SecureVideoGroupOverseas:
		if len(media.images)+len(media.videos)+len(media.audios) > 12 {
			return &arkRequestError{Code: "InvalidParameter.content", Message: "Secure overseas video supports at most twelve media inputs"}
		}
		if hasLastFrame && (len(media.videos) > 0 || len(media.audios) > 0) {
			return &arkRequestError{Code: "InvalidParameter.content", Message: "Secure overseas first/last frames cannot mix with video or audio inputs"}
		}
		if request.Duration != nil && (*request.Duration < 4 || *request.Duration > 15) {
			return &arkRequestError{Code: "InvalidParameter.duration", Message: "Secure overseas duration must be between 4 and 15"}
		}
		if request.Ratio != nil {
			switch *request.Ratio {
			case "1:1", "4:3", "3:4", "16:9", "9:16", "21:9":
			default:
				return &arkRequestError{Code: "InvalidParameter.ratio", Message: "Secure overseas ratio is unsupported"}
			}
		}
		if request.Resolution != nil {
			switch *request.Resolution {
			case "720p", "1080p":
			default:
				return &arkRequestError{Code: "InvalidParameter.resolution", Message: "Secure overseas resolution is unsupported"}
			}
		}
	case dto.SecureVideoGroupEnterprise:
		if len(media.videos) > 0 {
			return &arkRequestError{Code: "InvalidParameter.content", Message: "Secure enterprise video does not support reference videos"}
		}
		if len(media.images) > 9 || len(media.audios) > 3 || len(media.images)+len(media.audios) > 12 {
			return &arkRequestError{Code: "InvalidParameter.content", Message: "Secure enterprise media limits are exceeded"}
		}
		if hasLastFrame {
			return &arkRequestError{Code: "InvalidParameter.content", Message: "Secure enterprise video does not support last_frame"}
		}
		if request.Duration == nil {
			return &arkRequestError{Code: "MissingParameter.duration", Message: "duration is required by Secure enterprise video"}
		}
		if *request.Duration < 5 || *request.Duration > 15 {
			return &arkRequestError{Code: "InvalidParameter.duration", Message: "Secure enterprise duration must be between 5 and 15"}
		}
		if request.Ratio != nil {
			switch *request.Ratio {
			case "16:9", "9:16", "1:1":
			default:
				return &arkRequestError{Code: "InvalidParameter.ratio", Message: "Secure enterprise ratio is unsupported"}
			}
		}
		if request.Resolution != nil && *request.Resolution != "720p" {
			return &arkRequestError{Code: "InvalidParameter.resolution", Message: "Secure enterprise resolution must be 720p"}
		}
	default:
		return fmt.Errorf("invalid Secure request profile: %s", profile.group)
	}

	if upstreamModel == "" {
		return nil
	}
	switch upstreamModel {
	case "video-2.0-fast", "video-2.0-mini":
		if profile.group == dto.SecureVideoGroupEnterprise {
			return &arkRequestError{Code: "InvalidParameter.model", Message: "Secure enterprise video requires video-2.0-pro"}
		}
		if request.Resolution != nil && *request.Resolution != "720p" {
			return &arkRequestError{Code: "InvalidParameter.resolution", Message: upstreamModel + " only supports 720p"}
		}
	case "video-2.0-pro":
		return nil
	default:
		return &arkRequestError{Code: "InvalidParameter.model", Message: "model is not supported by this Secure video group"}
	}
	return nil
}

func validateSecureOverseasReferenceVideos(ctx context.Context, request arkRequest) *taskdto.TaskError {
	media := collectSecureMedia(request)
	if len(media.videos) == 0 {
		return nil
	}
	durationMS, err := service.ResolveReferenceVideoDurationMS(ctx, media.videos)
	if err != nil {
		var metadataErr *service.VideoMetadataError
		if errors.As(err, &metadataErr) && metadataErr.Kind == service.VideoMetadataInvalidMedia {
			return service.TaskErrorWrapperLocal(fmt.Errorf("reference video is invalid"), "InvalidParameter.content", http.StatusBadRequest)
		}
		return service.TaskErrorWrapperLocal(fmt.Errorf("reference video metadata is unavailable"), "reference_video_metadata_unavailable", http.StatusServiceUnavailable)
	}
	if durationMS > maxSecureOverseasReferenceVideoDurationMS {
		return service.TaskErrorWrapperLocal(fmt.Errorf("reference video duration exceeds 15 seconds"), "InvalidParameter.content", http.StatusBadRequest)
	}
	return nil
}

func buildSecureDiscountRequest(request arkRequest, upstreamModel string, profile secureRequestProfile) ([]byte, string, error) {
	if err := validateSecureRequest(request, profile, upstreamModel); err != nil {
		return nil, "", err
	}
	media := collectSecureMedia(request)
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	if err := writer.WriteField("model", upstreamModel); err != nil {
		return nil, "", err
	}
	if err := writer.WriteField("prompt", arkPrompt(request.Content)); err != nil {
		return nil, "", err
	}
	if request.Duration != nil {
		if err := writer.WriteField("duration", strconv.Itoa(*request.Duration)); err != nil {
			return nil, "", err
		}
	}
	if request.Ratio != nil {
		if err := writer.WriteField("ratio", *request.Ratio); err != nil {
			return nil, "", err
		}
	}
	if request.Resolution != nil {
		if err := writer.WriteField("resolution", *request.Resolution); err != nil {
			return nil, "", err
		}
	}
	for _, image := range media.images {
		if err := writer.WriteField("files", image.ImageURL.URL); err != nil {
			return nil, "", err
		}
	}
	for _, video := range media.videos {
		if err := writer.WriteField("video_urls", video); err != nil {
			return nil, "", err
		}
	}
	for _, audio := range media.audios {
		if err := writer.WriteField("audio_urls", audio); err != nil {
			return nil, "", err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, "", err
	}
	return buffer.Bytes(), writer.FormDataContentType(), nil
}

func buildSecureOverseasRequest(request arkRequest, upstreamModel string, profile secureRequestProfile) ([]byte, string, error) {
	if err := validateSecureRequest(request, profile, upstreamModel); err != nil {
		return nil, "", err
	}
	media := collectSecureMedia(request)
	prompt := arkPrompt(request.Content)
	omniReference := len(media.videos) > 0 || len(media.audios) > 0
	for _, image := range media.images {
		if strings.TrimSpace(image.Role) == "reference_image" {
			omniReference = true
			break
		}
	}
	functionMode := ""
	if omniReference {
		functionMode = "omni_reference"
		for index := range media.images {
			marker := fmt.Sprintf("@image_file_%d", index+1)
			if !strings.Contains(prompt, marker) {
				prompt += " " + marker
			}
		}
		for index := range media.videos {
			marker := fmt.Sprintf("@video_file_%d", index+1)
			if !strings.Contains(prompt, marker) {
				prompt += " " + marker
			}
		}
		for index := range media.audios {
			marker := fmt.Sprintf("@audio_file_%d", index+1)
			if !strings.Contains(prompt, marker) {
				prompt += " " + marker
			}
		}
	} else if len(media.images) == 1 {
		functionMode = "first_frame"
	} else if len(media.images) == 2 {
		functionMode = "first_last_frames"
	}

	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	if err := writer.WriteField("model", upstreamModel); err != nil {
		return nil, "", err
	}
	if err := writer.WriteField("prompt", strings.TrimSpace(prompt)); err != nil {
		return nil, "", err
	}
	if request.Duration != nil {
		if err := writer.WriteField("duration", strconv.Itoa(*request.Duration)); err != nil {
			return nil, "", err
		}
	}
	if request.Ratio != nil {
		if err := writer.WriteField("ratio", *request.Ratio); err != nil {
			return nil, "", err
		}
	}
	if request.Resolution != nil {
		if err := writer.WriteField("resolution", *request.Resolution); err != nil {
			return nil, "", err
		}
	}
	if functionMode != "" {
		if err := writer.WriteField("functionMode", functionMode); err != nil {
			return nil, "", err
		}
	}
	for index, image := range media.images {
		if err := writer.WriteField(fmt.Sprintf("image_file_%d", index+1), image.ImageURL.URL); err != nil {
			return nil, "", err
		}
	}
	for index, video := range media.videos {
		if err := writer.WriteField(fmt.Sprintf("video_file_%d", index+1), video); err != nil {
			return nil, "", err
		}
	}
	for index, audio := range media.audios {
		if err := writer.WriteField(fmt.Sprintf("audio_file_%d", index+1), audio); err != nil {
			return nil, "", err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, "", err
	}
	return buffer.Bytes(), writer.FormDataContentType(), nil
}

func buildSecureEnterpriseRequest(request arkRequest, upstreamModel string, profile secureRequestProfile) ([]byte, error) {
	return buildSecureEnterpriseRequestWithAssets(request, upstreamModel, profile, nil)
}

func buildSecureEnterpriseRequestWithContext(c *gin.Context, request arkRequest, upstreamModel string, profile secureRequestProfile) ([]byte, error) {
	mappings, _ := common.GetContextKeyType[map[string]string](c, constant.ContextKeyVideoAssetMappings)
	return buildSecureEnterpriseRequestWithAssets(request, upstreamModel, profile, mappings)
}

func buildSecureEnterpriseRequestWithAssets(request arkRequest, upstreamModel string, profile secureRequestProfile, mappings map[string]string) ([]byte, error) {
	if profile.group != dto.SecureVideoGroupEnterprise {
		for _, item := range request.Content {
			if item.Type == "image_url" && item.ImageURL != nil && strings.HasPrefix(strings.TrimSpace(item.ImageURL.URL), "asset://") {
				return nil, &arkRequestError{Code: "InvalidParameter.content", Message: "role assets are only supported by Secure enterprise video"}
			}
		}
	}
	validatedRequest, upstreamImages, err := prepareSecureEnterpriseRoleAssets(request, mappings)
	if err != nil {
		return nil, err
	}
	if err := validateSecureRequest(validatedRequest, profile, upstreamModel); err != nil {
		return nil, err
	}
	if len(upstreamImages) > 0 {
		media := collectSecureMedia(request)
		result := secureEnterpriseRequest{
			Model: upstreamModel, Prompt: arkPrompt(request.Content), Duration: request.Duration,
			AspectRatio: request.Ratio, ExtraVideos: media.videos, ExtraAudios: media.audios,
			UsePersonCharacter: true, ExtraImages: upstreamImages,
		}
		return common.Marshal(result)
	}

	media := collectSecureMedia(request)
	result := secureEnterpriseRequest{
		Model:       upstreamModel,
		Prompt:      arkPrompt(request.Content),
		Duration:    request.Duration,
		AspectRatio: request.Ratio,
		ExtraVideos: media.videos,
		ExtraAudios: media.audios,
	}
	if len(media.images) > 0 {
		result.ImageURL = media.images[0].ImageURL.URL
	}
	if len(media.images) > 1 {
		result.ExtraImages = make([]string, 0, len(media.images)-1)
		for _, image := range media.images[1:] {
			result.ExtraImages = append(result.ExtraImages, image.ImageURL.URL)
		}
	}
	return common.Marshal(result)
}

func prepareSecureEnterpriseRoleAssets(request arkRequest, mappings map[string]string) (arkRequest, []string, error) {
	roleAssetURLs := make([]string, 0)
	publicImage := false
	for _, item := range request.Content {
		if item.Type != "image_url" || strings.TrimSpace(item.Role) != "reference_image" || item.ImageURL == nil {
			continue
		}
		value := strings.TrimSpace(item.ImageURL.URL)
		if strings.HasPrefix(value, "asset://") {
			roleAssetURLs = append(roleAssetURLs, value)
		} else if value != "" {
			publicImage = true
		}
	}
	if len(roleAssetURLs) > 0 {
		if publicImage || len(roleAssetURLs) > 9 || len(mappings) == 0 {
			return arkRequest{}, nil, &arkRequestError{Code: "InvalidParameter.content", Message: "role assets cannot be mixed with public image URLs"}
		}
		normalized := request
		normalized.Content = append([]arkContent(nil), request.Content...)
		upstreamImages := make([]string, 0, len(roleAssetURLs))
		for index := range normalized.Content {
			item := &normalized.Content[index]
			if item.Type != "image_url" || strings.TrimSpace(item.Role) != "reference_image" || item.ImageURL == nil {
				continue
			}
			projectID := strings.TrimPrefix(strings.TrimSpace(item.ImageURL.URL), "asset://")
			upstreamID, ok := mappings[projectID]
			if !ok || !strings.HasPrefix(upstreamID, "asset-local-") {
				return arkRequest{}, nil, &arkRequestError{Code: "InvalidParameter.content", Message: "role asset reference is unavailable"}
			}
			upstreamImages = append(upstreamImages, "asset://"+upstreamID)
			item.ImageURL.URL = "https://example.com/asset-reference/" + projectID + ".png"
		}
		return normalized, upstreamImages, nil
	}
	return request, nil, nil
}

func validateSecureRequestWithAssets(request arkRequest, profile secureRequestProfile, upstreamModel string, mappings map[string]string) error {
	if profile.group != dto.SecureVideoGroupEnterprise {
		for _, item := range request.Content {
			if item.Type == "image_url" && item.ImageURL != nil && strings.HasPrefix(strings.TrimSpace(item.ImageURL.URL), "asset://") {
				return &arkRequestError{Code: "InvalidParameter.content", Message: "role assets are only supported by Secure enterprise video"}
			}
		}
	}
	validated, _, err := prepareSecureEnterpriseRoleAssets(request, mappings)
	if err != nil {
		return err
	}
	return validateSecureRequest(validated, profile, upstreamModel)
}
