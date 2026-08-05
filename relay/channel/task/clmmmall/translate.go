package clmmmall

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

var (
	durationControlPattern = regexp.MustCompile(`(?i)^(\d+)s$`)
	imageControlPattern    = regexp.MustCompile(`(?i)^(\d+)img$`)
)

type modelControls struct {
	resolution    string
	durationLimit int
	hasDuration   bool
	fixedDuration bool
	minimumImages int
	dropVideos    bool
}

type normalizedArkRequest struct {
	model      string
	prompt     string
	ratio      string
	resolution string
	duration   *int
	images     []string
	videos     []string
	audios     []string
}

func arkToClmm(request arkRequest, upstreamModel string) (clmmRequest, int, error) {
	upstreamModel = strings.TrimSpace(upstreamModel)
	normalized, err := normalizeArkRequest(request)
	if err != nil {
		return clmmRequest{}, 0, err
	}
	controls, err := parseModelControls(upstreamModel)
	if err != nil {
		return clmmRequest{}, 0, err
	}
	resolution := normalized.resolution
	if controls.resolution != "" {
		resolution = controls.resolution
	}
	if len(normalized.images) < controls.minimumImages {
		return clmmRequest{}, 0, fmt.Errorf("model requires at least %d reference images", controls.minimumImages)
	}
	videos := normalized.videos
	if controls.dropVideos {
		videos = nil
	}

	seconds := ""
	mySeconds := ""
	billingSeconds := 0
	if controls.hasDuration {
		billingSeconds = controls.durationLimit
		if normalized.duration != nil && !controls.fixedDuration {
			if *normalized.duration > controls.durationLimit {
				return clmmRequest{}, 0, fmt.Errorf("duration must be between 1 and %d seconds", controls.durationLimit)
			}
			billingSeconds = *normalized.duration
		}
		seconds = "1"
		mySeconds = strconv.Itoa(billingSeconds)
	} else {
		billingSeconds = 5
		if normalized.duration != nil {
			billingSeconds = *normalized.duration
		}
		if billingSeconds < 4 || billingSeconds > 15 || billingSeconds > relaycommon.MaxTaskDurationSeconds {
			return clmmRequest{}, 0, fmt.Errorf("duration must be between 4 and 15 seconds")
		}
		seconds = strconv.Itoa(billingSeconds)
	}

	size := "1280x720"
	if normalized.ratio == "9:16" {
		size = "720x1280"
	}
	return clmmRequest{
		Model:              upstreamModel,
		Prompt:             normalized.prompt,
		AspectRatio:        normalized.ratio,
		Resolution:         resolution,
		Size:               size,
		Seconds:            seconds,
		MySeconds:          mySeconds,
		ReferenceImageURLs: normalized.images,
		ReferenceVideos:    videos,
		ReferenceAudios:    normalized.audios,
	}, billingSeconds, nil
}

func normalizeArkRequest(request arkRequest) (normalizedArkRequest, error) {
	if strings.TrimSpace(request.Model) == "" {
		return normalizedArkRequest{}, fmt.Errorf("model is required")
	}
	if len(request.Content) == 0 {
		return normalizedArkRequest{}, fmt.Errorf("content is required")
	}
	if err := validateUnsupportedArkFields(request); err != nil {
		return normalizedArkRequest{}, err
	}
	if request.Duration != nil && (*request.Duration <= 0 || *request.Duration > relaycommon.MaxTaskDurationSeconds) {
		return normalizedArkRequest{}, fmt.Errorf("duration must be between 1 and %d seconds", relaycommon.MaxTaskDurationSeconds)
	}

	ratio := "16:9"
	if request.Ratio != nil {
		ratio = strings.ToLower(strings.TrimSpace(*request.Ratio))
		if ratio == "" {
			return normalizedArkRequest{}, fmt.Errorf("ratio must not be empty")
		}
	}
	if ratio != "16:9" && ratio != "9:16" {
		return normalizedArkRequest{}, fmt.Errorf("ratio %s is not supported", ratio)
	}
	resolution := "480p"
	if request.Resolution != nil {
		resolution = strings.ToLower(strings.TrimSpace(*request.Resolution))
		if resolution == "" {
			return normalizedArkRequest{}, fmt.Errorf("resolution must not be empty")
		}
	}
	if resolution != "480p" && resolution != "720p" {
		return normalizedArkRequest{}, fmt.Errorf("resolution %s is not supported", resolution)
	}

	texts := make([]string, 0, len(request.Content))
	images := make([]string, 0)
	videos := make([]string, 0)
	audios := make([]string, 0)
	for _, item := range request.Content {
		switch strings.TrimSpace(item.Type) {
		case "text":
			if text := strings.TrimSpace(item.Text); text != "" {
				texts = append(texts, text)
			}
		case "image_url":
			if item.ImageURL == nil || strings.TrimSpace(item.ImageURL.URL) == "" {
				return normalizedArkRequest{}, fmt.Errorf("image_url.url is required")
			}
			switch strings.TrimSpace(item.Role) {
			case "", "first_frame", "last_frame", "reference_image":
			default:
				return normalizedArkRequest{}, fmt.Errorf("unsupported image role: %s", item.Role)
			}
			images = append(images, item.ImageURL.URL)
		case "video_url":
			if item.VideoURL == nil || strings.TrimSpace(item.VideoURL.URL) == "" {
				return normalizedArkRequest{}, fmt.Errorf("video_url.url is required")
			}
			if strings.TrimSpace(item.Role) != "reference_video" {
				return normalizedArkRequest{}, fmt.Errorf("video role must be reference_video")
			}
			videos = append(videos, item.VideoURL.URL)
		case "audio_url":
			if item.AudioURL == nil || strings.TrimSpace(item.AudioURL.URL) == "" {
				return normalizedArkRequest{}, fmt.Errorf("audio_url.url is required")
			}
			role := strings.TrimSpace(item.Role)
			if role != "" && role != "reference_audio" {
				return normalizedArkRequest{}, fmt.Errorf("audio role must be reference_audio")
			}
			audios = append(audios, item.AudioURL.URL)
		case "draft_task":
			return normalizedArkRequest{}, fmt.Errorf("draft_task is not supported by CLMM Mall")
		default:
			return normalizedArkRequest{}, fmt.Errorf("unsupported content type: %s", item.Type)
		}
	}
	if len(texts) == 0 {
		return normalizedArkRequest{}, fmt.Errorf("text prompt is required")
	}
	if len(images) > 9 {
		return normalizedArkRequest{}, fmt.Errorf("too many reference images: maximum is 9")
	}
	if len(videos) > 3 {
		return normalizedArkRequest{}, fmt.Errorf("too many reference videos: maximum is 3")
	}
	if len(audios) > 3 {
		return normalizedArkRequest{}, fmt.Errorf("too many reference audios: maximum is 3")
	}
	if len(images)+len(videos)+len(audios) > 15 {
		return normalizedArkRequest{}, fmt.Errorf("too many media items: maximum is 15")
	}
	return normalizedArkRequest{
		model:      request.Model,
		prompt:     strings.Join(texts, "\n"),
		ratio:      ratio,
		resolution: resolution,
		duration:   request.Duration,
		images:     images,
		videos:     videos,
		audios:     audios,
	}, nil
}

func parseModelControls(modelName string) (modelControls, error) {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return modelControls{}, fmt.Errorf("model is required")
	}

	controls := modelControls{}
	hasBaseModel := false
	for _, rawSegment := range strings.FieldsFunc(strings.ToLower(modelName), func(r rune) bool {
		return r == '-' || unicode.IsSpace(r)
	}) {
		segment := strings.TrimSpace(rawSegment)
		switch segment {
		case "480p", "720p":
			controls.resolution = segment
		case "gz":
			controls.fixedDuration = true
		case "nv":
			controls.dropVideos = true
		default:
			if matches := durationControlPattern.FindStringSubmatch(segment); len(matches) == 2 {
				value, err := strconv.Atoi(matches[1])
				if err != nil || value <= 0 || value > relaycommon.MaxTaskDurationSeconds {
					return modelControls{}, fmt.Errorf("model duration suffix is invalid")
				}
				if controls.hasDuration && controls.durationLimit != value {
					return modelControls{}, fmt.Errorf("model has conflicting duration suffixes")
				}
				controls.durationLimit = value
				controls.hasDuration = true
				continue
			}
			if matches := imageControlPattern.FindStringSubmatch(segment); len(matches) == 2 {
				value, err := strconv.Atoi(matches[1])
				if err != nil || value <= 0 || value > 9 {
					return modelControls{}, fmt.Errorf("model image suffix is invalid")
				}
				if value > controls.minimumImages {
					controls.minimumImages = value
				}
				continue
			}
			hasBaseModel = true
		}
	}
	if !hasBaseModel {
		return modelControls{}, fmt.Errorf("model %s is missing a base model", modelName)
	}
	if controls.fixedDuration && !controls.hasDuration {
		return modelControls{}, fmt.Errorf("model suffix gz requires a duration suffix")
	}
	return controls, nil
}

// RouteModelContract exposes the parsed CLMM behavior needed by routing-policy validation.
type RouteModelContract struct {
	ControlsDuration bool
}

// AnalyzeRouteModel is the single model grammar entry point shared by submission and routing validation.
func AnalyzeRouteModel(modelName string) (RouteModelContract, error) {
	controls, err := parseModelControls(strings.TrimSpace(modelName))
	if err != nil {
		return RouteModelContract{}, err
	}
	return RouteModelContract{ControlsDuration: controls.hasDuration}, nil
}

// ValidateRouteModel verifies the mapped model grammar without constructing an
// upstream request. Routing-policy validation uses the same parser as submit.
func ValidateRouteModel(modelName string) error {
	_, err := parseModelControls(strings.TrimSpace(modelName))
	return err
}

func validateUnsupportedArkFields(request arkRequest) error {
	if request.Watermark != nil || request.GenerateAudio != nil || request.Draft != nil || request.Tools != nil ||
		request.Seed != nil || request.CameraFixed != nil || request.Frames != nil || request.Priority != nil ||
		request.ExecutionExpiresAfter != nil || request.ReturnLastFrame != nil || request.SafetyIdentifier != nil {
		return fmt.Errorf("request contains an unsupported Ark field")
	}
	if request.ServiceTier != nil && !strings.EqualFold(strings.TrimSpace(*request.ServiceTier), "default") {
		return fmt.Errorf("service_tier %s is not supported", *request.ServiceTier)
	}
	return nil
}

func marshalClmmRequest(request clmmRequest) ([]byte, error) {
	return common.Marshal(request)
}
