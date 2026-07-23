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
