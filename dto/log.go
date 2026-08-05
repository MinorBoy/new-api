package dto

type PublicLog struct {
	ID               int            `json:"id"`
	CreatedAt        int64          `json:"created_at"`
	Type             int            `json:"type"`
	Content          string         `json:"content"`
	TokenName        string         `json:"token_name"`
	ModelName        string         `json:"model_name"`
	Quota            int            `json:"quota"`
	PromptTokens     int            `json:"prompt_tokens"`
	CompletionTokens int            `json:"completion_tokens"`
	UseTime          int            `json:"use_time"`
	IsStream         bool           `json:"is_stream"`
	RequestID        string         `json:"request_id,omitempty"`
	Other            PublicLogOther `json:"other"`
}

type PublicLogOther struct {
	LoginMethod           string `json:"login_method,omitempty"`
	UserAgent             string `json:"user_agent,omitempty"`
	WebSocket             *bool  `json:"ws,omitempty"`
	Audio                 *bool  `json:"audio,omitempty"`
	AudioInput            *int   `json:"audio_input,omitempty"`
	AudioOutput           *int   `json:"audio_output,omitempty"`
	TextInput             *int   `json:"text_input,omitempty"`
	TextOutput            *int   `json:"text_output,omitempty"`
	CacheTokens           *int   `json:"cache_tokens,omitempty"`
	CacheCreationTokens   *int   `json:"cache_creation_tokens,omitempty"`
	CacheCreationTokens5m *int   `json:"cache_creation_tokens_5m,omitempty"`
	CacheCreationTokens1h *int   `json:"cache_creation_tokens_1h,omitempty"`
	FirstResponseTime     *int   `json:"frt,omitempty"`
	Image                 *bool  `json:"image,omitempty"`
	ImageOutput           *int   `json:"image_output,omitempty"`
	WebSearchCount        *int   `json:"web_search_call_count,omitempty"`
	FileSearchCount       *int   `json:"file_search_call_count,omitempty"`
	BillingSource         string `json:"billing_source,omitempty"`
	SubscriptionConsumed  *int   `json:"subscription_consumed,omitempty"`
	SubscriptionRemain    *int   `json:"subscription_remain,omitempty"`
	SubscriptionTotal     *int   `json:"subscription_total,omitempty"`
	IsTask                *bool  `json:"is_task,omitempty"`
	TaskID                string `json:"task_id,omitempty"`
}
