package clmmmall

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	taskcommon "github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

const (
	clmmArkRequestContextKey = "clmm_mall_ark_request"
	clmmRequestContextKey    = "clmm_mall_request"
)

type TaskAdaptor struct {
	taskcommon.BaseBilling
	channelType int
	apiKey      string
	baseURL     string
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	if info == nil || info.ChannelMeta == nil {
		return
	}
	a.channelType = info.ChannelType
	a.baseURL = strings.TrimRight(info.ChannelBaseUrl, "/")
	a.apiKey = info.ApiKey
}

func (a *TaskAdaptor) GetModelList() []string { return ModelList }

func (a *TaskAdaptor) GetChannelName() string { return ChannelName }

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	if !c.GetBool(common.KeySeedanceOfficialAPI) {
		return service.TaskErrorWrapperLocal(errors.New("CLMM Mall requires the Ark /api/v3 task API"), "invalid_request", http.StatusBadRequest)
	}
	if info == nil || info.TaskRelayInfo == nil {
		return service.TaskErrorWrapperLocal(errors.New("task relay info is missing"), "invalid_request", http.StatusBadRequest)
	}

	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	rawBody, err := storage.Bytes()
	if err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	var rawFields map[string]any
	if err := common.Unmarshal(rawBody, &rawFields); err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	allowedFields := map[string]bool{
		"model": true, "content": true, "ratio": true, "resolution": true, "duration": true,
		"service_tier": true, "watermark": true, "generate_audio": true, "draft": true,
		"tools": true, "seed": true, "camera_fixed": true, "frames": true, "priority": true,
		"execution_expires_after": true, "return_last_frame": true, "safety_identifier": true,
	}
	for field := range rawFields {
		if !allowedFields[field] {
			return service.TaskErrorWrapperLocal(fmt.Errorf("unsupported Ark field: %s", field), "invalid_request", http.StatusBadRequest)
		}
	}

	var request arkRequest
	if err := common.Unmarshal(rawBody, &request); err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	normalized, err := normalizeArkRequest(request)
	if err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}

	duration := 5
	if request.Duration != nil {
		duration = *request.Duration
	}
	relaycommon.StoreTaskRequest(c, info, constant.TaskActionGenerate, relaycommon.TaskSubmitReq{
		Model:    request.Model,
		Prompt:   normalized.prompt,
		Duration: duration,
		Metadata: map[string]any{"ratio": normalized.ratio, "resolution": normalized.resolution},
	})
	c.Set(clmmArkRequestContextKey, request)
	return nil
}

func (a *TaskAdaptor) ValidateBillingRequest(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	request, err := clmmArkRequest(c)
	if err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	if info == nil || info.ChannelMeta == nil || strings.TrimSpace(info.UpstreamModelName) == "" {
		return service.TaskErrorWrapperLocal(errors.New("mapped CLMM Mall model is required"), "invalid_model", http.StatusBadRequest)
	}
	converted, billingSeconds, err := arkToClmm(request, info.UpstreamModelName)
	if err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_model", http.StatusBadRequest)
	}
	relaycommon.StoreTaskRequest(c, info, constant.TaskActionGenerate, relaycommon.TaskSubmitReq{
		Model:    request.Model,
		Prompt:   converted.Prompt,
		Duration: billingSeconds,
		Metadata: map[string]any{"ratio": converted.AspectRatio, "resolution": converted.Resolution},
	})
	c.Set(clmmRequestContextKey, converted)
	return nil
}

func (a *TaskAdaptor) EstimateDurationSeconds(c *gin.Context, info *relaycommon.RelayInfo) (int, *dto.TaskError) {
	request, err := clmmArkRequest(c)
	if err != nil {
		return 0, service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	if info == nil || info.ChannelMeta == nil || strings.TrimSpace(info.UpstreamModelName) == "" {
		return 0, service.TaskErrorWrapperLocal(errors.New("mapped CLMM Mall model is required"), "invalid_model", http.StatusBadRequest)
	}
	_, billingSeconds, err := arkToClmm(request, info.UpstreamModelName)
	if err != nil {
		return 0, service.TaskErrorWrapperLocal(err, "invalid_model", http.StatusBadRequest)
	}
	return billingSeconds, nil
}

func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	baseURL := a.baseURL
	if baseURL == "" && info != nil && info.ChannelMeta != nil {
		baseURL = strings.TrimRight(info.ChannelBaseUrl, "/")
	}
	if baseURL == "" {
		return "", errors.New("CLMM Mall base URL is empty")
	}
	return baseURL + "/v1/videos", nil
}

func (a *TaskAdaptor) BuildRequestHeader(_ *gin.Context, request *http.Request, info *relaycommon.RelayInfo) error {
	apiKey := a.apiKey
	if apiKey == "" && info != nil && info.ChannelMeta != nil {
		apiKey = info.ApiKey
	}
	request.Header.Set("Authorization", "Bearer "+apiKey)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	var converted clmmRequest
	if value, ok := c.Get(clmmRequestContextKey); ok {
		var valid bool
		converted, valid = value.(clmmRequest)
		if !valid {
			return nil, errors.New("invalid CLMM Mall request in context")
		}
	} else {
		request, err := clmmArkRequest(c)
		if err != nil {
			return nil, err
		}
		if info == nil || info.ChannelMeta == nil {
			return nil, errors.New("CLMM Mall channel metadata is missing")
		}
		converted, _, err = arkToClmm(request, info.UpstreamModelName)
		if err != nil {
			return nil, err
		}
	}
	body, err := marshalClmmRequest(converted)
	if err != nil {
		return nil, fmt.Errorf("marshal CLMM Mall request: %w", err)
	}
	return bytes.NewReader(body), nil
}

func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

func (a *TaskAdaptor) DoResponse(c *gin.Context, response *http.Response, info *relaycommon.RelayInfo) (string, []byte, *dto.TaskError) {
	if response == nil || response.Body == nil {
		return "", nil, service.TaskErrorWrapper(errors.New("CLMM Mall response is empty"), "invalid_response", http.StatusBadGateway)
	}
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		return "", nil, service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusBadGateway)
	}
	var submitResponse clmmSubmitResponse
	if err := common.Unmarshal(body, &submitResponse); err != nil {
		return "", body, service.TaskErrorWrapper(errors.New("invalid CLMM Mall task response"), "invalid_response", http.StatusBadGateway)
	}
	upstreamTaskID := strings.TrimSpace(submitResponse.TaskID)
	if upstreamTaskID == "" {
		upstreamTaskID = strings.TrimSpace(submitResponse.ID)
	}
	if upstreamTaskID == "" {
		return "", body, service.TaskErrorWrapper(errors.New("CLMM Mall task response is missing an ID"), "invalid_response", http.StatusBadGateway)
	}
	if info == nil || info.TaskRelayInfo == nil || strings.TrimSpace(info.PublicTaskID) == "" {
		return "", body, service.TaskErrorWrapper(errors.New("public task ID is missing"), "invalid_response", http.StatusInternalServerError)
	}
	c.JSON(http.StatusOK, gin.H{"id": info.PublicTaskID})
	return upstreamTaskID, body, nil
}

func (a *TaskAdaptor) ParseTaskError(_ []byte, statusCode int) *dto.TaskError {
	switch statusCode {
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		message := "CLMM Mall rejected the request"
		return &dto.TaskError{Code: "invalid_request", Message: message, StatusCode: http.StatusBadRequest, Error: errors.New(message)}
	case http.StatusTooManyRequests:
		message := "CLMM Mall rate limit exceeded"
		return &dto.TaskError{Code: "rate_limit_exceeded", Message: message, StatusCode: http.StatusTooManyRequests, Error: errors.New(message)}
	default:
		message := "CLMM Mall upstream request failed"
		return &dto.TaskError{Code: "upstream_error", Message: message, StatusCode: http.StatusBadGateway, Error: errors.New(message)}
	}
}

func (a *TaskAdaptor) FetchTask(baseURL, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok || strings.TrimSpace(taskID) == "" {
		return nil, errors.New("invalid task_id")
	}
	requestURL := strings.TrimRight(baseURL, "/") + "/v1/videos/" + url.PathEscape(taskID)
	request, err := http.NewRequest(http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+key)
	request.Header.Set("Accept", "application/json")
	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("create proxy HTTP client: %w", err)
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		_ = response.Body.Close()
		return nil, fmt.Errorf("CLMM Mall task polling failed with HTTP status %d", response.StatusCode)
	}
	return response, nil
}

func (a *TaskAdaptor) ParseTaskResult(body []byte) (*relaycommon.TaskInfo, error) {
	var response clmmTaskResponse
	if err := common.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("unmarshal CLMM Mall task result: %w", err)
	}
	result := &relaycommon.TaskInfo{}
	defaultProgress := 0
	switch strings.ToLower(strings.TrimSpace(response.Status)) {
	case "queued", "pending":
		result.Status = string(model.TaskStatusQueued)
	case "processing", "running", "in_progress":
		result.Status = string(model.TaskStatusInProgress)
		defaultProgress = 50
	case "completed", "succeeded", "success":
		result.Status = string(model.TaskStatusSuccess)
		defaultProgress = 100
		result.Url = clmmResultURL(response)
		if result.Url == "" {
			return nil, errors.New("CLMM Mall task succeeded without a result URL")
		}
	case "failed", "error", "cancelled", "canceled":
		result.Status = string(model.TaskStatusFailure)
		defaultProgress = 100
		result.ErrorCode, result.Reason = clmmErrorValue(response.Error)
		if result.Reason == "" {
			result.Reason = clmmMessageValue(response.Detail)
		}
		if result.Reason == "" {
			result.Reason = strings.TrimSpace(response.Message)
		}
		if result.ErrorCode == "" {
			result.ErrorCode = "task_failed"
		}
		if result.Reason == "" {
			result.Reason = "task failed"
		}
	default:
		return nil, fmt.Errorf("unknown CLMM Mall task status: %s", strings.TrimSpace(response.Status))
	}
	progress := defaultProgress
	if response.Progress != nil {
		progress = *response.Progress
		if progress < 0 {
			progress = 0
		} else if progress > 100 {
			progress = 100
		}
	}
	result.Progress = strconv.Itoa(progress) + "%"
	return result, nil
}

func (a *TaskAdaptor) ConvertToArkVideoTask(task *model.Task) ([]byte, error) {
	if task == nil {
		return nil, errors.New("task is nil")
	}
	var upstream clmmTaskResponse
	if len(task.Data) > 0 {
		if err := common.Unmarshal(task.Data, &upstream); err != nil {
			return nil, fmt.Errorf("unmarshal CLMM Mall task data: %w", err)
		}
	}
	createdAt := task.SubmitTime
	if createdAt == 0 {
		createdAt = task.CreatedAt
	}
	response := arkTaskResponse{
		ID:        task.TaskID,
		Model:     task.Properties.OriginModelName,
		Status:    arkTaskStatus(task.Status),
		CreatedAt: createdAt,
		UpdatedAt: task.UpdatedAt,
	}
	if response.Status == "succeeded" {
		response.Content.VideoURL = clmmResultURL(upstream)
		if response.Content.VideoURL == "" {
			response.Content.VideoURL = task.PrivateData.ResultURL
		}
	}
	if response.Status == "failed" {
		response.Error = &arkTaskError{Code: "task_failed", Message: "task failed"}
	}
	return common.Marshal(response)
}

func clmmArkRequest(c *gin.Context) (arkRequest, error) {
	value, ok := c.Get(clmmArkRequestContextKey)
	if !ok {
		return arkRequest{}, errors.New("CLMM Mall Ark request is missing")
	}
	request, ok := value.(arkRequest)
	if !ok {
		return arkRequest{}, errors.New("invalid CLMM Mall Ark request")
	}
	return request, nil
}

func clmmErrorValue(value any) (string, string) {
	switch typed := value.(type) {
	case string:
		return "", strings.TrimSpace(typed)
	case map[string]any:
		return clmmMessageValue(typed["code"]), clmmMessageValue(typed["message"])
	default:
		return "", ""
	}
}

func clmmMessageValue(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case int:
		return strconv.Itoa(typed)
	default:
		return ""
	}
}

func clmmResultURL(response clmmTaskResponse) string {
	for _, candidate := range []string{
		response.VideoURL,
		response.URL,
		response.ResultURL,
		clmmMessageValue(response.Metadata["url"]),
	} {
		if candidate = strings.TrimSpace(candidate); candidate != "" {
			return candidate
		}
	}
	return ""
}

func arkTaskStatus(status model.TaskStatus) string {
	switch status {
	case model.TaskStatusInProgress:
		return "running"
	case model.TaskStatusSuccess:
		return "succeeded"
	case model.TaskStatusFailure:
		return "failed"
	default:
		return "queued"
	}
}
