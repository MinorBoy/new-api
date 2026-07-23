package clmmmall

type arkRequest struct {
	Model                 string       `json:"model"`
	Content               []arkContent `json:"content"`
	Ratio                 *string      `json:"ratio,omitempty"`
	Resolution            *string      `json:"resolution,omitempty"`
	Duration              *int         `json:"duration,omitempty"`
	ServiceTier           *string      `json:"service_tier,omitempty"`
	Watermark             *bool        `json:"watermark,omitempty"`
	GenerateAudio         *bool        `json:"generate_audio,omitempty"`
	Draft                 *bool        `json:"draft,omitempty"`
	Tools                 *[]arkTool   `json:"tools,omitempty"`
	Seed                  *int         `json:"seed,omitempty"`
	CameraFixed           *bool        `json:"camera_fixed,omitempty"`
	Frames                *int         `json:"frames,omitempty"`
	Priority              *int         `json:"priority,omitempty"`
	ExecutionExpiresAfter *int         `json:"execution_expires_after,omitempty"`
	ReturnLastFrame       *bool        `json:"return_last_frame,omitempty"`
	SafetyIdentifier      *string      `json:"safety_identifier,omitempty"`
}

type arkContent struct {
	Type      string    `json:"type"`
	Text      string    `json:"text,omitempty"`
	ImageURL  *arkMedia `json:"image_url,omitempty"`
	VideoURL  *arkMedia `json:"video_url,omitempty"`
	AudioURL  *arkMedia `json:"audio_url,omitempty"`
	Role      string    `json:"role,omitempty"`
	DraftTask any       `json:"draft_task,omitempty"`
}

type arkMedia struct {
	URL string `json:"url"`
}

type arkTool struct {
	Type string `json:"type,omitempty"`
}

type clmmRequest struct {
	Model              string   `json:"model"`
	Prompt             string   `json:"prompt"`
	AspectRatio        string   `json:"aspect_ratio"`
	Resolution         string   `json:"resolution"`
	Size               string   `json:"size"`
	Seconds            string   `json:"seconds"`
	MySeconds          string   `json:"mySeconds,omitempty"`
	ReferenceImageURLs []string `json:"reference_image_urls,omitempty"`
	ReferenceVideos    []string `json:"reference_videos,omitempty"`
}

type clmmSubmitResponse struct {
	ID     string `json:"id,omitempty"`
	TaskID string `json:"task_id,omitempty"`
	Status string `json:"status,omitempty"`
}

type clmmTaskResponse struct {
	ID          string         `json:"id,omitempty"`
	TaskID      string         `json:"task_id,omitempty"`
	Model       string         `json:"model,omitempty"`
	Status      string         `json:"status,omitempty"`
	Progress    *int           `json:"progress,omitempty"`
	VideoURL    string         `json:"video_url,omitempty"`
	ResultURL   string         `json:"result_url,omitempty"`
	URL         string         `json:"url,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	Error       any            `json:"error,omitempty"`
	Message     string         `json:"message,omitempty"`
	Detail      any            `json:"detail,omitempty"`
	CreatedAt   int64          `json:"created_at,omitempty"`
	UpdatedAt   int64          `json:"updated_at,omitempty"`
	CompletedAt int64          `json:"completed_at,omitempty"`
}

type arkTaskResponse struct {
	ID        string         `json:"id"`
	Model     string         `json:"model,omitempty"`
	Status    string         `json:"status"`
	Content   arkTaskContent `json:"content"`
	Error     *arkTaskError  `json:"error,omitempty"`
	CreatedAt int64          `json:"created_at,omitempty"`
	UpdatedAt int64          `json:"updated_at,omitempty"`
}

type arkTaskContent struct {
	VideoURL string `json:"video_url,omitempty"`
}

type arkTaskError struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}
