package newapivideo

import (
	"encoding/json"
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

const requestStateContextKey = "newapi_video_request_state"

type requestState struct {
	OpenAIFields map[string]json.RawMessage
	ARK          *arkRequest
	Duration     *decimal.Decimal
	Seconds      *decimal.Decimal
}

type arkRequest struct {
	Model         string       `json:"model"`
	Content       []arkContent `json:"content"`
	Ratio         string       `json:"ratio,omitempty"`
	Resolution    string       `json:"resolution,omitempty"`
	Duration      *int         `json:"duration,omitempty"`
	Watermark     *bool        `json:"watermark,omitempty"`
	GenerateAudio *bool        `json:"generate_audio,omitempty"`
	ServiceTier   *string      `json:"service_tier,omitempty"`
	Draft         *bool        `json:"draft,omitempty"`
	Tools         *[]arkTool   `json:"tools,omitempty"`
}

type arkTool struct {
	Type string `json:"type,omitempty"`
}

type arkMedia struct {
	URL string `json:"url"`
}

type arkContent struct {
	Type      string    `json:"type"`
	Text      string    `json:"text,omitempty"`
	ImageURL  *arkMedia `json:"image_url,omitempty"`
	VideoURL  *arkMedia `json:"video_url,omitempty"`
	AudioURL  *arkMedia `json:"audio_url,omitempty"`
	DraftTask any       `json:"draft_task,omitempty"`
	Role      string    `json:"role,omitempty"`
}

type upstreamRequest struct {
	Model          string              `json:"model"`
	Prompt         string              `json:"prompt"`
	Image          string              `json:"image,omitempty"`
	ImageWithRoles []upstreamRoleImage `json:"image_with_roles,omitempty"`
	Content        []arkContent        `json:"content,omitempty"`
	GenerateAudio  *bool               `json:"generateAudio,omitempty"`
	Ratio          string              `json:"ratio,omitempty"`
	Seconds        *string             `json:"seconds,omitempty"`
	Watermark      *bool               `json:"watermark,omitempty"`
}

type upstreamRoleImage struct {
	URL  string `json:"url"`
	Role string `json:"role"`
}

type tokenUsage struct {
	CompletionTokens *json.Number `json:"completion_tokens,omitempty"`
	TotalTokens      *json.Number `json:"total_tokens,omitempty"`
}

type arkVideoContent struct {
	VideoURL string `json:"video_url,omitempty"`
}

type arkTaskData struct {
	Content               *arkVideoContent `json:"content,omitempty"`
	CreatedAt             *int64           `json:"created_at,omitempty"`
	UpdatedAt             *int64           `json:"updated_at,omitempty"`
	Draft                 *bool            `json:"draft,omitempty"`
	Duration              *json.Number     `json:"duration,omitempty"`
	ExecutionExpiresAfter *json.Number     `json:"execution_expires_after,omitempty"`
	FramesPerSecond       *json.Number     `json:"framespersecond,omitempty"`
	GenerateAudio         *bool            `json:"generate_audio,omitempty"`
	Priority              *json.Number     `json:"priority,omitempty"`
	Ratio                 string           `json:"ratio,omitempty"`
	Resolution            string           `json:"resolution,omitempty"`
	Seed                  *json.Number     `json:"seed,omitempty"`
	ServiceTier           string           `json:"service_tier,omitempty"`
	Status                string           `json:"status,omitempty"`
	Usage                 *tokenUsage      `json:"usage,omitempty"`
	Error                 *upstreamError   `json:"error,omitempty"`
}

type detailedTask struct {
	TaskID     string          `json:"task_id"`
	Status     string          `json:"status"`
	FailReason string          `json:"fail_reason"`
	ResultURL  string          `json:"result_url"`
	SubmitTime int64           `json:"submit_time"`
	StartTime  int64           `json:"start_time"`
	FinishTime int64           `json:"finish_time"`
	Progress   string          `json:"progress"`
	Data       json.RawMessage `json:"data"`
}

type detailedEnvelope struct {
	Code    *string       `json:"code"`
	Message string        `json:"message"`
	Data    *detailedTask `json:"data"`
}

type directTask struct {
	ID          string `json:"id"`
	TaskID      string `json:"task_id"`
	Status      string `json:"status"`
	Progress    int    `json:"progress"`
	CreatedAt   int64  `json:"created_at"`
	CompletedAt int64  `json:"completed_at"`
	Metadata    *struct {
		URL string `json:"url,omitempty"`
	} `json:"metadata,omitempty"`
	Content *arkVideoContent `json:"content,omitempty"`
	Data    *struct {
		URL string `json:"url,omitempty"`
	} `json:"data,omitempty"`
	Usage *tokenUsage    `json:"usage,omitempty"`
	Error *upstreamError `json:"error,omitempty"`
}

type arkTaskResponse struct {
	ID                    string           `json:"id"`
	Model                 string           `json:"model"`
	Status                string           `json:"status"`
	Content               *arkVideoContent `json:"content,omitempty"`
	CreatedAt             *int64           `json:"created_at,omitempty"`
	UpdatedAt             *int64           `json:"updated_at,omitempty"`
	Draft                 *bool            `json:"draft,omitempty"`
	Duration              *json.Number     `json:"duration,omitempty"`
	ExecutionExpiresAfter *json.Number     `json:"execution_expires_after,omitempty"`
	FramesPerSecond       *json.Number     `json:"framespersecond,omitempty"`
	GenerateAudio         *bool            `json:"generate_audio,omitempty"`
	Priority              *json.Number     `json:"priority,omitempty"`
	Ratio                 string           `json:"ratio,omitempty"`
	Resolution            string           `json:"resolution,omitempty"`
	Seed                  *json.Number     `json:"seed,omitempty"`
	ServiceTier           string           `json:"service_tier,omitempty"`
	Usage                 *tokenUsage      `json:"usage,omitempty"`
	Error                 *upstreamError   `json:"error,omitempty"`
}

func getRequestState(c *gin.Context) (requestState, error) {
	value, exists := c.Get(requestStateContextKey)
	if !exists {
		return requestState{}, fmt.Errorf("new-api video request state is missing")
	}
	state, ok := value.(requestState)
	if !ok {
		return requestState{}, fmt.Errorf("invalid new-api video request state")
	}
	return state, nil
}
