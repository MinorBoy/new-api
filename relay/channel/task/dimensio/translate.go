package dimensio

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

// ArkRequest is the ARK v3 video task request accepted by the native task API.
// Fields without a Dimensio equivalent are retained so the adaptor can reject
// them explicitly instead of silently changing the request's meaning.
type ArkRequest struct {
	Model      string       `json:"model"`
	Content    []ArkContent `json:"content"`
	Resolution string       `json:"resolution,omitempty"`
	Ratio      string       `json:"ratio,omitempty"`
	Duration   *int         `json:"duration,omitempty"`

	Seed                  *int    `json:"seed,omitempty"`
	CameraFixed           *bool   `json:"camera_fixed,omitempty"`
	Watermark             *bool   `json:"watermark,omitempty"`
	GenerateAudio         *bool   `json:"generate_audio,omitempty"`
	Frames                *int    `json:"frames,omitempty"`
	Draft                 *bool   `json:"draft,omitempty"`
	Priority              *int    `json:"priority,omitempty"`
	ExecutionExpiresAfter *int    `json:"execution_expires_after,omitempty"`
	ReturnLastFrame       *bool   `json:"return_last_frame,omitempty"`
	SafetyIdentifier      *string `json:"safety_identifier,omitempty"`
	Tools                 *[]struct {
		Type string `json:"type,omitempty"`
	} `json:"tools,omitempty"`

	IntelligentRatio *bool `json:"intelligent_ratio,omitempty"`
	FaceGrid         *bool `json:"face_grid,omitempty"`
}

type ArkContent struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *ArkMedia `json:"image_url,omitempty"`
	VideoURL *ArkMedia `json:"video_url,omitempty"`
	AudioURL *ArkMedia `json:"audio_url,omitempty"`
	Role     string    `json:"role,omitempty"`
}

type ArkMedia struct {
	URL string `json:"url"`
}

// DimensioRequest is the JSON request sent to Dimensio. Media maps are
// expanded into top-level image_file_N/video_file_N/audio_file_N fields by
// MarshalDimensioRequest.
type DimensioRequest struct {
	Model            string            `json:"model"`
	Prompt           string            `json:"prompt"`
	FunctionMode     string            `json:"functionMode"`
	Ratio            string            `json:"ratio,omitempty"`
	Resolution       string            `json:"resolution,omitempty"`
	Duration         *int              `json:"duration,omitempty"`
	IntelligentRatio *bool             `json:"intelligent_ratio,omitempty"`
	FaceGrid         *bool             `json:"face_grid,omitempty"`
	FilePaths        []string          `json:"-"`
	ImageFiles       map[string]string `json:"-"`
	VideoFiles       map[string]string `json:"-"`
	AudioFiles       map[string]string `json:"-"`
}

// ArkToDimensio translates an ARK request into Dimensio's top-level fields.
// Model mapping is intentionally left to the adaptor, after channel model
// mapping has selected the provider model.
func ArkToDimensio(ark ArkRequest) (DimensioRequest, error) {
	if err := validateUnsupportedFields(ark); err != nil {
		return DimensioRequest{}, err
	}
	if err := validateArkContentRoles(ark.Content); err != nil {
		return DimensioRequest{}, err
	}

	dim := DimensioRequest{
		Model:            ark.Model,
		Ratio:            ark.Ratio,
		Resolution:       ark.Resolution,
		Duration:         ark.Duration,
		IntelligentRatio: ark.IntelligentRatio,
		FaceGrid:         ark.FaceGrid,
		FilePaths:        []string{},
		ImageFiles:       map[string]string{},
		VideoFiles:       map[string]string{},
		AudioFiles:       map[string]string{},
	}

	imageIndex, videoIndex, audioIndex := 0, 0, 0
	for _, item := range ark.Content {
		switch item.Type {
		case "text":
			if dim.Prompt == "" && strings.TrimSpace(item.Text) != "" {
				dim.Prompt = item.Text
			}
		case "image_url":
			if item.ImageURL == nil || strings.TrimSpace(item.ImageURL.URL) == "" {
				return DimensioRequest{}, fmt.Errorf("image_url.url is required")
			}
			imageIndex++
			dim.ImageFiles[fmt.Sprintf("image_file_%d", imageIndex)] = item.ImageURL.URL
			dim.FilePaths = append(dim.FilePaths, item.ImageURL.URL)
		case "video_url":
			if item.VideoURL == nil || strings.TrimSpace(item.VideoURL.URL) == "" {
				return DimensioRequest{}, fmt.Errorf("video_url.url is required")
			}
			videoIndex++
			dim.VideoFiles[fmt.Sprintf("video_file_%d", videoIndex)] = item.VideoURL.URL
		case "audio_url":
			if item.AudioURL == nil || strings.TrimSpace(item.AudioURL.URL) == "" {
				return DimensioRequest{}, fmt.Errorf("audio_url.url is required")
			}
			audioIndex++
			dim.AudioFiles[fmt.Sprintf("audio_file_%d", audioIndex)] = item.AudioURL.URL
		default:
			return DimensioRequest{}, fmt.Errorf("unsupported content type: %s", item.Type)
		}
	}
	if strings.TrimSpace(dim.Prompt) == "" {
		return DimensioRequest{}, fmt.Errorf("text prompt is required")
	}
	dim.FunctionMode = deriveFunctionMode(ark.Content)
	return dim, nil
}

func validateUnsupportedFields(ark ArkRequest) error {
	if ark.Seed != nil || ark.CameraFixed != nil || ark.Watermark != nil || ark.GenerateAudio != nil ||
		ark.Frames != nil || ark.Draft != nil || ark.Priority != nil || ark.ExecutionExpiresAfter != nil ||
		ark.ReturnLastFrame != nil || ark.SafetyIdentifier != nil || ark.Tools != nil {
		return fmt.Errorf("ARK field is not supported by dimensio adaptor: unsupported field")
	}
	return nil
}

func deriveFunctionMode(content []ArkContent) string {
	for _, item := range content {
		if item.Type == "video_url" || item.Type == "audio_url" ||
			(item.Type == "image_url" && strings.EqualFold(strings.TrimSpace(item.Role), "reference_image")) {
			return "omni_reference"
		}
	}
	return "first_last_frames"
}

func validateArkContentRoles(content []ArkContent) error {
	images, first, last, references, videos, audios := 0, 0, 0, 0, 0, 0
	for _, item := range content {
		switch item.Type {
		case "text":
		case "image_url":
			images++
			switch strings.TrimSpace(item.Role) {
			case "", "first_frame":
				first++
			case "last_frame":
				last++
			case "reference_image":
				references++
			default:
				return fmt.Errorf("unsupported image role: %s", item.Role)
			}
		case "video_url":
			videos++
			if strings.TrimSpace(item.Role) != "reference_video" {
				return fmt.Errorf("video role must be reference_video")
			}
		case "audio_url":
			audios++
			if strings.TrimSpace(item.Role) != "reference_audio" {
				return fmt.Errorf("audio role must be reference_audio")
			}
		default:
			return fmt.Errorf("unsupported content type: %s", item.Type)
		}
	}

	if images > 9 {
		return fmt.Errorf("too many images: dimensio allows at most 9 (image_file_1..9)")
	}
	if videos > 3 {
		return fmt.Errorf("too many videos: dimensio allows at most 3 (video_file_1..3)")
	}
	if audios > 3 {
		return fmt.Errorf("too many audios: dimensio allows at most 3 (audio_file_1..3)")
	}
	if images+videos+audios > 12 {
		return fmt.Errorf("too many media items: dimensio allows at most 12 total")
	}
	if audios > 0 && images == 0 && videos == 0 {
		return fmt.Errorf("audio input requires an image or video")
	}
	// First/last-frame mode is mutually exclusive with reference media. A
	// reference image may be combined with reference video/audio in omni mode.
	if (first > 0 || last > 0) && (references > 0 || videos > 0 || audios > 0) {
		return fmt.Errorf("reference media cannot mix with first/last frames")
	}
	if first > 1 || last > 1 || (last > 0 && first != 1) {
		return fmt.Errorf("first/last frames require one first frame and at most one last frame")
	}
	return nil
}

// MarshalDimensioRequest expands the media maps into the top-level JSON
// object expected by Dimensio. It deliberately never emits file_paths.
func MarshalDimensioRequest(dim DimensioRequest) ([]byte, error) {
	base, err := common.Marshal(dim)
	if err != nil {
		return nil, err
	}
	var merged map[string]interface{}
	if err := common.Unmarshal(base, &merged); err != nil {
		return nil, err
	}
	for key, value := range dim.ImageFiles {
		merged[key] = value
	}
	for key, value := range dim.VideoFiles {
		merged[key] = value
	}
	for key, value := range dim.AudioFiles {
		merged[key] = value
	}
	return common.Marshal(merged)
}

type DimensioTaskResponse struct {
	TaskID    string         `json:"task_id"`
	Status    string         `json:"status"`
	Progress  int            `json:"progress"`
	Code      int            `json:"code,omitempty"`
	Message   string         `json:"message,omitempty"`
	CreatedAt int64          `json:"created_at,omitempty"`
	UpdatedAt int64          `json:"updated_at,omitempty"`
	Result    DimensioResult `json:"result"`
	Error     string         `json:"error,omitempty"`
	ErrorCode string         `json:"error_code,omitempty"`
}

type DimensioResult struct {
	URL string `json:"url"`
}

type DimensioSubmitResponse struct {
	Created int64  `json:"created"`
	TaskID  string `json:"task_id"`
	Status  string `json:"status"`
}

type ArkTaskResponse struct {
	ID        string         `json:"id"`
	Model     string         `json:"model,omitempty"`
	Status    string         `json:"status"`
	Content   ArkContentResp `json:"content"`
	Usage     ArkUsage       `json:"usage,omitempty"`
	Error     *ArkError      `json:"error,omitempty"`
	CreatedAt int64          `json:"created_at,omitempty"`
	UpdatedAt int64          `json:"updated_at,omitempty"`
}

type ArkContentResp struct {
	VideoURL string `json:"video_url,omitempty"`
}

type ArkUsage struct {
	CompletionTokens int `json:"completion_tokens,omitempty"`
	TotalTokens      int `json:"total_tokens,omitempty"`
}

type ArkError struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

func DimensioToArkTask(dim DimensioTaskResponse, publicTaskID, modelName string, createdAt, updatedAt int64) (ArkTaskResponse, error) {
	if dim.CreatedAt != 0 {
		createdAt = dim.CreatedAt
	}
	if dim.UpdatedAt != 0 {
		updatedAt = dim.UpdatedAt
	}
	ark := ArkTaskResponse{
		ID: publicTaskID, Model: modelName, CreatedAt: createdAt, UpdatedAt: updatedAt,
	}
	if dim.Status == "" && dim.Code != 0 && dim.Message != "" {
		ark.Status = "failed"
		ark.Error = &ArkError{Code: strconv.Itoa(dim.Code), Message: dim.Message}
		return ark, nil
	}
	switch dim.Status {
	case "pending":
		ark.Status = "queued"
	case "processing":
		ark.Status = "running"
	case "completed":
		ark.Status = "succeeded"
		if dim.Result.URL != "" {
			ark.Content.VideoURL = dim.Result.URL
		}
	case "failed":
		ark.Status = "failed"
		message := dim.Error
		if message == "" {
			message = "task failed"
		}
		ark.Error = &ArkError{Code: dim.ErrorCode, Message: message}
	case "not_found":
		ark.Status = "failed"
		ark.Error = &ArkError{Code: dim.ErrorCode, Message: "task not found or expired"}
	default:
		return ArkTaskResponse{}, fmt.Errorf("unknown dimensio task status: %s", dim.Status)
	}
	return ark, nil
}
