package dto

import (
	"encoding/json"
)

type TaskError struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	Data       any    `json:"data"`
	StatusCode int    `json:"-"`
	LocalError bool   `json:"-"`
	Error      error  `json:"-"`
}

type TaskData interface {
	SunoDataResponse | []SunoDataResponse | string | any
}

const TaskSuccessCode = "success"

type TaskResponse[T TaskData] struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Data    T      `json:"data"`
}

func (t *TaskResponse[T]) IsSuccess() bool {
	return t.Code == TaskSuccessCode
}

type TaskDto struct {
	ID         int64  `json:"id"`
	CreatedAt  int64  `json:"created_at"`
	UpdatedAt  int64  `json:"updated_at"`
	TaskID     string `json:"task_id"`
	Platform   string `json:"platform"`
	UserId     int    `json:"user_id"`
	Group      string `json:"group"`
	ChannelId  int    `json:"channel_id"`
	Quota      int    `json:"quota"`
	Action     string `json:"action"`
	Status     string `json:"status"`
	FailReason string `json:"fail_reason"`
	ResultURL  string `json:"result_url,omitempty"` // 任务结果 URL（视频地址等）
	SubmitTime int64  `json:"submit_time"`
	StartTime  int64  `json:"start_time"`
	FinishTime int64  `json:"finish_time"`
	Progress   string `json:"progress"`
	Properties any    `json:"properties"`
	// RequestModel is the canonical model name received from the client.
	RequestModel string          `json:"request_model,omitempty"`
	Username     string          `json:"username,omitempty"`
	Data         json.RawMessage `json:"data"`
	RequestPath  string          `json:"request_path,omitempty"`
	// UserRequestData is available to the task owner. The upstream and returned
	// response payloads remain administrator-only.
	UserRequestData      json.RawMessage `json:"user_request_data,omitempty"`
	UpstreamResponseData json.RawMessage `json:"upstream_response_data,omitempty"`
	UserResponseData     json.RawMessage `json:"user_response_data,omitempty"`
}

type PublicTaskDto struct {
	CreatedAt        int64              `json:"created_at"`
	UpdatedAt        int64              `json:"updated_at"`
	TaskID           string             `json:"task_id"`
	Quota            int                `json:"quota"`
	Action           string             `json:"action"`
	Status           string             `json:"status"`
	FailReason       string             `json:"fail_reason,omitempty"`
	SubmitTime       int64              `json:"submit_time"`
	StartTime        int64              `json:"start_time"`
	FinishTime       int64              `json:"finish_time"`
	Progress         string             `json:"progress"`
	RequestModel     string             `json:"request_model,omitempty"`
	ResultURL        string             `json:"result_url,omitempty"`
	Data             []PublicTaskOutput `json:"data,omitempty"`
	UserResponseData *PublicTaskResult  `json:"user_response_data,omitempty"`
}

type PublicTaskOutput struct {
	Title    string `json:"title,omitempty"`
	Text     string `json:"text,omitempty"`
	AudioURL string `json:"audio_url,omitempty"`
	VideoURL string `json:"video_url,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
}

type PublicTaskResult struct {
	ID                    string             `json:"id"`
	Model                 string             `json:"model"`
	Status                string             `json:"status"`
	Content               *PublicTaskContent `json:"content,omitempty"`
	Usage                 PublicTaskUsage    `json:"usage"`
	CreatedAt             int64              `json:"created_at"`
	UpdatedAt             int64              `json:"updated_at"`
	Seed                  int64              `json:"seed"`
	Resolution            string             `json:"resolution"`
	Ratio                 string             `json:"ratio"`
	Duration              int64              `json:"duration"`
	FramesPerSecond       int64              `json:"framespersecond"`
	ServiceTier           string             `json:"service_tier"`
	ExecutionExpiresAfter int64              `json:"execution_expires_after"`
	GenerateAudio         bool               `json:"generate_audio"`
	Draft                 bool               `json:"draft"`
	Priority              int64              `json:"priority"`
	Error                 *PublicTaskError   `json:"error,omitempty"`
}

type PublicTaskContent struct {
	VideoURL string `json:"video_url"`
}

type PublicTaskUsage struct {
	CompletionTokens int64 `json:"completion_tokens"`
	TotalTokens      int64 `json:"total_tokens"`
}

type PublicTaskError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type FetchReq struct {
	IDs []string `json:"ids"`
}

type TaskFilterUserOption struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
}

type TaskFilterChannelOption struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type TaskFilterOptions struct {
	Channels      []TaskFilterChannelOption `json:"channels,omitempty"`
	Statuses      []string                  `json:"statuses"`
	RequestModels []string                  `json:"request_models"`
	Users         []TaskFilterUserOption    `json:"users,omitempty"`
}
