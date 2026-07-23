package doubao

import (
	"bytes"
	stderrors "errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
	"github.com/samber/lo"
	"github.com/tidwall/gjson"
)

// ============================
// Request / Response structures
// ============================

type ContentItem struct {
	Type      string    `json:"type,omitempty"`
	Text      string    `json:"text,omitempty"`
	ImageURL  *MediaURL `json:"image_url,omitempty"`
	VideoURL  *MediaURL `json:"video_url,omitempty"`
	AudioURL  *MediaURL `json:"audio_url,omitempty"`
	DraftTask *struct {
		ID string `json:"id,omitempty"`
	} `json:"draft_task,omitempty"`
	Role string `json:"role,omitempty"`
}

type MediaURL struct {
	URL string `json:"url,omitempty"`
}

type requestPayload struct {
	Model                 string         `json:"model"`
	Content               []ContentItem  `json:"content,omitempty"`
	CallbackURL           string         `json:"callback_url,omitempty"`
	ReturnLastFrame       *dto.BoolValue `json:"return_last_frame,omitempty"`
	ServiceTier           string         `json:"service_tier,omitempty"`
	ExecutionExpiresAfter *dto.IntValue  `json:"execution_expires_after,omitempty"`
	GenerateAudio         *dto.BoolValue `json:"generate_audio,omitempty"`
	Draft                 *dto.BoolValue `json:"draft,omitempty"`
	Tools                 []struct {
		Type string `json:"type,omitempty"`
	} `json:"tools,omitempty"`
	SafetyIdentifier string         `json:"safety_identifier,omitempty"`
	Priority         *dto.IntValue  `json:"priority,omitempty"`
	Resolution       string         `json:"resolution,omitempty"`
	Ratio            string         `json:"ratio,omitempty"`
	Duration         *dto.IntValue  `json:"duration,omitempty"`
	Frames           *dto.IntValue  `json:"frames,omitempty"`
	Seed             *dto.IntValue  `json:"seed,omitempty"`
	CameraFixed      *dto.BoolValue `json:"camera_fixed,omitempty"`
	Watermark        *dto.BoolValue `json:"watermark,omitempty"`
}

type responsePayload struct {
	ID string `json:"id"` // task_id
}

type responseTask struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Status  string `json:"status"`
	Content struct {
		VideoURL string `json:"video_url"`
	} `json:"content"`
	Seed            int    `json:"seed"`
	Resolution      string `json:"resolution"`
	Duration        int    `json:"duration"`
	Ratio           string `json:"ratio"`
	FramesPerSecond int    `json:"framespersecond"`
	ServiceTier     string `json:"service_tier"`
	Tools           []struct {
		Type string `json:"type"`
	} `json:"tools"`
	Usage struct {
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
		ToolUsage        struct {
			WebSearch int `json:"web_search"`
		} `json:"tool_usage"`
	} `json:"usage"`
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
	CreatedAt int64 `json:"created_at"`
	UpdatedAt int64 `json:"updated_at"`
}

// ============================
// Adaptor implementation
// ============================

type TaskAdaptor struct {
	taskcommon.BaseBilling
	ChannelType int
	apiKey      string
	baseURL     string
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.ChannelType = info.ChannelType
	a.baseURL = info.ChannelBaseUrl
	a.apiKey = info.ApiKey
}

// ValidateRequestAndSetAction parses body, validates fields and sets default action.
func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) (taskErr *dto.TaskError) {
	if c.GetBool(common.KeySeedanceOfficialAPI) {
		return a.validateNativeRequest(c, info)
	}
	// Accept only POST /v1/video/generations as "generate" action.
	return relaycommon.ValidateBasicTaskRequest(c, info, constant.TaskActionGenerate)
}

// BuildRequestURL constructs the upstream URL.
func (a *TaskAdaptor) BuildRequestURL(_ *relaycommon.RelayInfo) (string, error) {
	return fmt.Sprintf("%s/api/v3/contents/generations/tasks", a.baseURL), nil
}

// BuildRequestHeader sets required headers.
func (a *TaskAdaptor) BuildRequestHeader(_ *gin.Context, req *http.Request, _ *relaycommon.RelayInfo) error {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	return nil
}

// EstimateBilling 根据请求 metadata 中的输出分辨率与是否包含视频输入，返回相对基准价的计费 OtherRatio。
func (a *TaskAdaptor) EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64 {
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil
	}
	hasVideo := hasVideoInMetadata(req.Metadata)
	resolution := c.GetString("task_resolution")
	modelName := info.UpstreamModelName
	if modelName == "" {
		modelName = info.OriginModelName
	}
	family := seedancePricingFamily(modelName)
	if family == "" {
		return nil
	}
	if resolution == "" {
		resolution = metadataStringDefault(req.Metadata, "resolution", "720p")
	}
	resolution = strings.ToLower(strings.TrimSpace(resolution))
	serviceTier := c.GetString(string(constant.ContextKeyTaskServiceTier))
	if serviceTier == "" {
		serviceTier = metadataStringDefault(req.Metadata, "service_tier", "default")
	}
	serviceTier = strings.ToLower(strings.TrimSpace(serviceTier))
	draft := c.GetBool(string(constant.ContextKeyTaskDraft))
	if _, exists := c.Get(string(constant.ContextKeyTaskDraft)); !exists {
		draft = metadataBoolDefault(req.Metadata, "draft", false)
	}
	generateAudio := family == seedance15ProFamily || family == seedance20Family || family == seedance20FastFamily || family == seedance20MiniFamily
	if value, exists := c.Get(string(constant.ContextKeyTaskGenerateAudio)); exists {
		generateAudio, _ = value.(bool)
	}
	generateAudio = metadataBoolDefault(req.Metadata, "generate_audio", generateAudio)
	c.Set(string(constant.ContextKeyTaskVideoHasInput), hasVideo)
	c.Set(string(constant.ContextKeyTaskGenerateAudio), generateAudio)
	c.Set(string(constant.ContextKeyTaskDraft), draft)
	c.Set(string(constant.ContextKeyTaskServiceTier), serviceTier)
	c.Set("task_resolution", resolution)
	if family == seedance15ProFamily {
		ratios, _ := GetSeedance15ProRatios(generateAudio, draft, serviceTier)
		return ratios
	}
	ratio, ok := GetVideoInputRatio(modelName, resolution, hasVideo)
	if !ok || ratio == 1.0 {
		return nil
	}
	return map[string]float64{"video_input": ratio}
}

// ValidateBillingRequest runs after model mapping, so aliases cannot bypass
// unsupported pricing combinations or receive a cheaper default tier.
func (a *TaskAdaptor) ValidateBillingRequest(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	modelName := info.UpstreamModelName
	if modelName == "" {
		modelName = info.OriginModelName
	}
	family := seedancePricingFamily(modelName)
	if family == "" {
		if c.GetBool(common.KeySeedanceOfficialAPI) {
			return service.TaskErrorWrapperLocal(fmt.Errorf("unsupported Seedance model: %s", modelName), "invalid_request", http.StatusBadRequest)
		}
		return nil
	}
	if c.GetBool(common.KeySeedanceOfficialAPI) {
		storage, storageErr := common.GetBodyStorage(c)
		if storageErr != nil {
			return service.TaskErrorWrapperLocal(storageErr, "invalid_request", http.StatusBadRequest)
		}
		rawBody, bodyErr := storage.Bytes()
		if bodyErr != nil {
			return service.TaskErrorWrapperLocal(bodyErr, "invalid_request", http.StatusBadRequest)
		}
		var nativeRequest seedanceNativeRequest
		if err := common.Unmarshal(rawBody, &nativeRequest); err != nil {
			return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
		}
		nativeRequest.Model = modelName
		facts, factsErr := validateSeedanceContent(modelName, nativeRequest.Content)
		if factsErr != nil {
			return service.TaskErrorWrapperLocal(factsErr, "invalid_request", http.StatusBadRequest)
		}
		if fieldsErr := validateSeedanceNativeFields(
			nativeRequest,
			facts,
			common.GetContextKeyBool(c, constant.ContextKeyRoutingCapabilityMode),
		); fieldsErr != nil {
			return service.TaskErrorWrapperLocal(fieldsErr, "invalid_request", http.StatusBadRequest)
		}
		generateAudio := family == seedance15ProFamily || family == seedance20Family || family == seedance20FastFamily || family == seedance20MiniFamily
		if nativeRequest.GenerateAudio != nil {
			generateAudio = bool(*nativeRequest.GenerateAudio)
		}
		serviceTier := strings.ToLower(strings.TrimSpace(nativeRequest.ServiceTier))
		if serviceTier == "" {
			serviceTier = "default"
		}
		resolution := strings.ToLower(strings.TrimSpace(nativeRequest.Resolution))
		if resolution == "" {
			resolution = "720p"
			if nativeRequest.Draft != nil && bool(*nativeRequest.Draft) {
				resolution = "480p"
			}
			if family == "1.0" || family == "1.0-fast" {
				resolution = "1080p"
			}
		}
		c.Set(string(constant.ContextKeyTaskVideoHasInput), facts.videoCount > 0)
		c.Set(string(constant.ContextKeyTaskGenerateAudio), generateAudio)
		c.Set(string(constant.ContextKeyTaskDraft), nativeRequest.Draft != nil && bool(*nativeRequest.Draft))
		c.Set(string(constant.ContextKeyTaskServiceTier), serviceTier)
		c.Set("task_resolution", resolution)
	}
	resolution := strings.ToLower(strings.TrimSpace(c.GetString("task_resolution")))
	if resolution == "" {
		resolution = strings.ToLower(strings.TrimSpace(metadataStringDefault(req.Metadata, "resolution", "720p")))
	}
	if _, ok := GetVideoInputRatio(modelName, resolution, metadataContentHasVideo(req.Metadata)); family != seedance15ProFamily && !ok {
		return service.TaskErrorWrapperLocal(fmt.Errorf("resolution %s is not supported by %s", resolution, modelName), "invalid_request", http.StatusBadRequest)
	}
	serviceTier := strings.ToLower(strings.TrimSpace(c.GetString(string(constant.ContextKeyTaskServiceTier))))
	if serviceTier == "" {
		serviceTier = strings.ToLower(strings.TrimSpace(metadataStringDefault(req.Metadata, "service_tier", "default")))
	}
	if family == seedance15ProFamily {
		if serviceTier != "default" && serviceTier != "flex" {
			return service.TaskErrorWrapperLocal(fmt.Errorf("service_tier=%s is not supported by %s", serviceTier, modelName), "invalid_request", http.StatusBadRequest)
		}
		generateAudio := metadataBoolDefault(req.Metadata, "generate_audio", true)
		draft := metadataBoolDefault(req.Metadata, "draft", false)
		if draft && resolution != "480p" {
			return service.TaskErrorWrapperLocal(stderrors.New("draft requires 480p"), "invalid_request", http.StatusBadRequest)
		}
		if draft && serviceTier == "flex" {
			return service.TaskErrorWrapperLocal(stderrors.New("draft does not support flex service tier"), "invalid_request", http.StatusBadRequest)
		}
		_ = generateAudio
		return nil
	}
	if serviceTier != "default" {
		return service.TaskErrorWrapperLocal(fmt.Errorf("service_tier=%s is not supported by %s", serviceTier, modelName), "invalid_request", http.StatusBadRequest)
	}
	if metadataBoolDefault(req.Metadata, "draft", false) {
		return service.TaskErrorWrapperLocal(fmt.Errorf("draft is not supported by %s", modelName), "invalid_request", http.StatusBadRequest)
	}
	return nil
}

func metadataStringDefault(metadata map[string]interface{}, key, fallback string) string {
	if value, ok := metadata[key].(string); ok && strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

func metadataBoolDefault(metadata map[string]interface{}, key string, fallback bool) bool {
	value, ok := metadata[key]
	if !ok {
		return fallback
	}
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		if parsed, err := strconv.ParseBool(strings.TrimSpace(typed)); err == nil {
			return parsed
		}
	}
	return fallback
}

func metadataContentHasVideo(metadata map[string]interface{}) bool {
	return hasVideoInMetadata(metadata)
}

func (a *TaskAdaptor) AdjustBillingOnComplete(task *model.Task, taskResult *relaycommon.TaskInfo) int {
	bc := task.PrivateData.BillingContext
	if bc == nil || taskResult == nil {
		return 0
	}
	modelName := bc.UpstreamModelName
	if modelName == "" {
		modelName = bc.OriginModelName
	}
	family := seedancePricingFamily(modelName)
	priceData := &types.PriceData{}
	priceData.ReplaceOtherRatios(bc.OtherRatios)
	if (family == seedance20Family || family == seedance20FastFamily || family == seedance20MiniFamily) && taskResult.Resolution != "" {
		if ratio, ok := GetVideoInputRatio(modelName, taskResult.Resolution, bc.HasVideoInput); ok {
			if ratio == 1 {
				rations := priceData.OtherRatios()
				delete(rations, "video_input")
				priceData.ReplaceOtherRatios(rations)
			} else {
				priceData.AddOtherRatio("video_input", ratio)
			}
		}
	}
	if taskResult.CompletionTokensPresent || taskResult.CompletionTokens != 0 || taskResult.TotalTokens != 0 {
		ratios := priceData.OtherRatios()
		delete(ratios, "draft_estimate")
		priceData.ReplaceOtherRatios(ratios)
	}
	bc.OtherRatios = priceData.OtherRatios()
	if taskResult.Resolution != "" {
		resolution := strings.ToLower(strings.TrimSpace(taskResult.Resolution))
		if family == seedance15ProFamily {
			if resolution == "480p" || resolution == "720p" || resolution == "1080p" {
				bc.Resolution = resolution
			}
		} else if _, ok := GetVideoInputRatio(modelName, resolution, bc.HasVideoInput); ok {
			bc.Resolution = resolution
		}
	}
	return 0
}

// hasVideoInMetadata 直接检查 metadata 的 content 数组是否包含 video_url 条目，
// 避免构建完整的上游 requestPayload。
func hasVideoInMetadata(metadata map[string]interface{}) bool {
	if metadata == nil {
		return false
	}
	contentRaw, ok := metadata["content"]
	if !ok {
		return false
	}
	contentSlice, ok := contentRaw.([]interface{})
	if !ok {
		return false
	}
	for _, item := range contentSlice {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if itemMap["type"] == "video_url" {
			return true
		}
		if _, has := itemMap["video_url"]; has {
			return true
		}
	}
	return false
}

// BuildRequestBody converts request into Doubao specific format.
func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	if c.GetBool(common.KeySeedanceOfficialAPI) {
		return a.buildNativeRequestBody(c, info)
	}
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil, err
	}

	body, err := a.convertToRequestPayload(&req)
	if err != nil {
		return nil, errors.Wrap(err, "convert request payload failed")
	}
	if info.IsModelMapped {
		body.Model = info.UpstreamModelName
	} else {
		info.UpstreamModelName = body.Model
	}
	data, err := common.Marshal(body)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}

// DoRequest delegates to common helper.
func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

// DoResponse handles upstream response, returns taskID etc.
func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *dto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
		return
	}
	_ = resp.Body.Close()

	// Parse Doubao response
	var dResp responsePayload
	if err := common.Unmarshal(responseBody, &dResp); err != nil {
		taskErr = service.TaskErrorWrapper(errors.Wrapf(err, "body: %s", responseBody), "unmarshal_response_body_failed", http.StatusInternalServerError)
		return
	}

	if dResp.ID == "" {
		taskErr = service.TaskErrorWrapper(fmt.Errorf("task_id is empty"), "invalid_response", http.StatusInternalServerError)
		return
	}

	var publicResponse map[string]interface{}
	if err := common.Unmarshal(responseBody, &publicResponse); err != nil {
		taskErr = service.TaskErrorWrapper(errors.Wrapf(err, "body: %s", responseBody), "unmarshal_response_body_failed", http.StatusInternalServerError)
		return
	}
	publicResponse["id"] = info.PublicTaskID
	c.JSON(http.StatusOK, publicResponse)
	return dResp.ID, responseBody, nil
}

// FetchTask fetch task status
func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid task_id")
	}

	uri := fmt.Sprintf("%s/api/v3/contents/generations/tasks/%s", baseUrl, taskID)

	req, err := http.NewRequest(http.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)

	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	return client.Do(req)
}

func (a *TaskAdaptor) GetModelList() []string {
	return ModelList
}

func (a *TaskAdaptor) GetChannelName() string {
	return ChannelName
}

func (a *TaskAdaptor) convertToRequestPayload(req *relaycommon.TaskSubmitReq) (*requestPayload, error) {
	r := requestPayload{
		Model:   req.Model,
		Content: []ContentItem{},
	}

	// Add images if present
	if req.HasImage() {
		for _, imgURL := range req.Images {
			r.Content = append(r.Content, ContentItem{
				Type: "image_url",
				ImageURL: &MediaURL{
					URL: imgURL,
				},
			})
		}
	}

	metadata := req.Metadata
	if err := taskcommon.UnmarshalMetadata(metadata, &r); err != nil {
		return nil, errors.Wrap(err, "unmarshal metadata failed")
	}

	if sec, _ := strconv.Atoi(req.Seconds); sec > 0 {
		r.Duration = lo.ToPtr(dto.IntValue(sec))
	}

	r.Content = lo.Reject(r.Content, func(c ContentItem, _ int) bool { return c.Type == "text" })
	r.Content = append(r.Content, ContentItem{
		Type: "text",
		Text: req.Prompt,
	})

	return &r, nil
}

func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	resTask := responseTask{}
	if err := common.Unmarshal(respBody, &resTask); err != nil {
		return nil, errors.Wrap(err, "unmarshal task result failed")
	}

	taskResult := relaycommon.TaskInfo{
		Code: 0,
	}

	// Map Doubao status to internal status
	switch resTask.Status {
	case "pending", "queued":
		taskResult.Status = model.TaskStatusQueued
		taskResult.Progress = "10%"
	case "processing", "running":
		taskResult.Status = model.TaskStatusInProgress
		taskResult.Progress = "50%"
	case "succeeded":
		taskResult.Status = model.TaskStatusSuccess
		taskResult.Progress = "100%"
		taskResult.Url = resTask.Content.VideoURL
		taskResult.Resolution = resTask.Resolution
		// 解析 usage 信息用于按倍率计费
		taskResult.CompletionTokens = resTask.Usage.CompletionTokens
		taskResult.TotalTokens = resTask.Usage.TotalTokens
		completionTokens := gjson.GetBytes(respBody, "usage.completion_tokens")
		taskResult.CompletionTokensPresent = completionTokens.Exists() && completionTokens.Type == gjson.Number
	case "failed", "expired", "cancelled":
		taskResult.Status = model.TaskStatusFailure
		taskResult.Progress = "100%"
		taskResult.Reason = resTask.Error.Message
		if taskResult.Reason == "" {
			taskResult.Reason = resTask.Status
		}
	default:
		// Unknown status, treat as processing
		taskResult.Status = model.TaskStatusInProgress
		taskResult.Progress = "30%"
	}

	return &taskResult, nil
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(originTask *model.Task) ([]byte, error) {
	var dResp responseTask
	if err := common.Unmarshal(originTask.Data, &dResp); err != nil {
		return nil, errors.Wrap(err, "unmarshal doubao task data failed")
	}

	openAIVideo := dto.NewOpenAIVideo()
	openAIVideo.ID = originTask.TaskID
	openAIVideo.TaskID = originTask.TaskID
	openAIVideo.Status = originTask.Status.ToVideoStatus()
	openAIVideo.SetProgressStr(originTask.Progress)
	openAIVideo.SetMetadata("url", dResp.Content.VideoURL)
	openAIVideo.CreatedAt = originTask.CreatedAt
	openAIVideo.CompletedAt = originTask.UpdatedAt
	openAIVideo.Model = originTask.Properties.OriginModelName

	if dResp.Status == "failed" {
		openAIVideo.Error = &dto.OpenAIVideoError{
			Message: dResp.Error.Message,
			Code:    dResp.Error.Code,
		}
	}

	return common.Marshal(openAIVideo)
}
