package dimensio

import (
	"bytes"
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
	"github.com/pkg/errors"
)

type TaskAdaptor struct {
	taskcommon.BaseBilling
	ChannelType int
	apiKey      string
	baseURL     string
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.ChannelType = info.ChannelType
	a.baseURL = strings.TrimRight(info.ChannelBaseUrl, "/")
	a.apiKey = info.ApiKey
}

func (a *TaskAdaptor) GetModelList() []string { return ModelList }
func (a *TaskAdaptor) GetChannelName() string { return ChannelName }

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	if !c.GetBool(common.KeySeedanceOfficialAPI) {
		return service.TaskErrorWrapperLocal(fmt.Errorf("dimensio requires the ARK /api/v3 task API"), "invalid_request", http.StatusBadRequest)
	}
	var req ArkRequest
	if err := common.UnmarshalBodyReusable(c, &req); err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	rawBody, err := storage.Bytes()
	if err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	var rawFields map[string]interface{}
	if err := common.Unmarshal(rawBody, &rawFields); err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	allowedFields := map[string]bool{
		"model": true, "content": true, "resolution": true, "ratio": true, "duration": true,
		"seed": true, "camera_fixed": true, "watermark": true, "generate_audio": true, "frames": true,
		"draft": true, "priority": true, "execution_expires_after": true, "return_last_frame": true,
		"safety_identifier": true, "tools": true, "intelligent_ratio": true, "face_grid": true,
	}
	for field := range rawFields {
		if !allowedFields[field] {
			return service.TaskErrorWrapperLocal(fmt.Errorf("unsupported ARK field: %s", field), "invalid_request", http.StatusBadRequest)
		}
	}
	if strings.TrimSpace(req.Model) == "" {
		return service.TaskErrorWrapperLocal(fmt.Errorf("field model is required"), "missing_model", http.StatusBadRequest)
	}
	if len(req.Content) == 0 {
		return service.TaskErrorWrapperLocal(fmt.Errorf("field content is required"), "missing_content", http.StatusBadRequest)
	}
	if req.Duration != nil && (*req.Duration < 4 || *req.Duration > 15 || *req.Duration > relaycommon.MaxTaskDurationSeconds) {
		return service.TaskErrorWrapperLocal(fmt.Errorf("duration must be between 4 and 15 seconds"), "invalid_duration", http.StatusBadRequest)
	}
	if req.Duration == nil {
		req.Duration = common.GetPointer(5)
	}
	req.Resolution = strings.ToLower(strings.TrimSpace(req.Resolution))
	if req.Resolution == "" {
		req.Resolution = "720p"
	}
	if req.Resolution != "720p" && req.Resolution != "1080p" {
		return service.TaskErrorWrapperLocal(fmt.Errorf("resolution %s is not supported by dimensio", req.Resolution), "invalid_resolution", http.StatusBadRequest)
	}
	req.Ratio = strings.ToLower(strings.TrimSpace(req.Ratio))
	if req.Ratio == "" {
		req.Ratio = "16:9"
	}
	if req.Ratio == "adaptive" {
		return service.TaskErrorWrapperLocal(fmt.Errorf("ratio adaptive is not supported by dimensio"), "invalid_ratio", http.StatusBadRequest)
	}
	validRatios := map[string]bool{"16:9": true, "4:3": true, "1:1": true, "3:4": true, "9:16": true, "21:9": true}
	if !validRatios[req.Ratio] {
		return service.TaskErrorWrapperLocal(fmt.Errorf("ratio %s is not supported by dimensio", req.Ratio), "invalid_ratio", http.StatusBadRequest)
	}
	if _, err := ArkToDimensio(req); err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	prompt := ""
	for _, item := range req.Content {
		if item.Type == "text" && strings.TrimSpace(item.Text) != "" {
			prompt = item.Text
			break
		}
	}
	relaycommon.StoreTaskRequest(c, info, constant.TaskActionGenerate, relaycommon.TaskSubmitReq{
		Model: req.Model, Prompt: prompt, Duration: *req.Duration,
		Metadata: map[string]interface{}{"resolution": req.Resolution, "ratio": req.Ratio},
	})
	c.Set("dimensio_ark_request", req)
	c.Set("task_resolution", req.Resolution)
	return nil
}

func (a *TaskAdaptor) EstimateBilling(c *gin.Context, _ *relaycommon.RelayInfo) map[string]float64 {
	v, ok := c.Get("dimensio_ark_request")
	if !ok {
		return nil
	}
	req, ok := v.(ArkRequest)
	if !ok || req.Duration == nil || *req.Duration < 4 || *req.Duration > 15 || *req.Duration > relaycommon.MaxTaskDurationSeconds {
		return nil
	}
	ratios := map[string]float64{"seconds": float64(*req.Duration), "resolution": 1}
	if req.Resolution == "1080p" {
		ratios["resolution"] = 2.5
	}
	return ratios
}

func (a *TaskAdaptor) ValidateBillingRequest(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	if info == nil {
		return service.TaskErrorWrapperLocal(fmt.Errorf("relay info is nil"), "invalid_request", http.StatusBadRequest)
	}
	requestModel := info.UpstreamModelName
	if requestModel == "" {
		requestModel = info.OriginModelName
	}
	requestModel = strings.ToLower(strings.TrimSpace(requestModel))
	if !common.StringsContains(ModelList, requestModel) {
		return service.TaskErrorWrapperLocal(fmt.Errorf("model %s is not supported by dimensio", requestModel), "invalid_model", http.StatusBadRequest)
	}
	if strings.EqualFold(strings.TrimSpace(c.GetString("task_resolution")), "1080p") && requestModel != "jimeng-video-seedance-2.0-vip" {
		return service.TaskErrorWrapperLocal(fmt.Errorf("1080p is supported only by jimeng-video-seedance-2.0-vip"), "invalid_resolution", http.StatusBadRequest)
	}
	return nil
}

func (a *TaskAdaptor) BuildRequestURL(_ *relaycommon.RelayInfo) (string, error) {
	return a.baseURL + "/v1/videos/generations", nil
}

func (a *TaskAdaptor) BuildRequestHeader(_ *gin.Context, req *http.Request, _ *relaycommon.RelayInfo) error {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	v, ok := c.Get("dimensio_ark_request")
	if !ok {
		return nil, fmt.Errorf("dimensio_ark_request not found in context")
	}
	req, ok := v.(ArkRequest)
	if !ok {
		return nil, fmt.Errorf("invalid ark request type")
	}
	dim, err := ArkToDimensio(req)
	if err != nil {
		return nil, errors.Wrap(err, "translate ARK to dimensio failed")
	}
	if info.UpstreamModelName != "" {
		dim.Model = info.UpstreamModelName
	}
	body, err := MarshalDimensioRequest(dim)
	if err != nil {
		return nil, errors.Wrap(err, "marshal dimensio request failed")
	}
	return bytes.NewReader(body), nil
}

func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, body io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, body)
}

func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (string, []byte, *dto.TaskError) {
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return "", nil, service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
	}
	var errorResponse struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if common.Unmarshal(body, &errorResponse) == nil && errorResponse.Code != 0 && errorResponse.Message != "" {
		statusCode := resp.StatusCode
		if statusCode >= http.StatusOK && statusCode < http.StatusMultipleChoices {
			statusCode = http.StatusBadGateway
		}
		return "", body, a.ParseTaskError(body, statusCode)
	}
	var response DimensioSubmitResponse
	if err := common.Unmarshal(body, &response); err != nil || response.TaskID == "" {
		return "", body, service.TaskErrorWrapper(fmt.Errorf("invalid dimensio task response"), "invalid_response", http.StatusInternalServerError)
	}
	if strings.TrimSpace(info.PublicTaskID) == "" {
		return "", body, service.TaskErrorWrapper(fmt.Errorf("public task_id is empty"), "invalid_response", http.StatusInternalServerError)
	}
	c.JSON(http.StatusOK, gin.H{"id": info.PublicTaskID})
	return response.TaskID, body, nil
}

func (a *TaskAdaptor) ParseTaskError(body []byte, statusCode int) *dto.TaskError {
	var response struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := common.Unmarshal(body, &response); err != nil || response.Message == "" {
		return service.TaskErrorWrapper(fmt.Errorf("%s", string(body)), "fail_to_fetch_task", statusCode)
	}
	if response.Code == -2000 {
		statusCode = http.StatusBadRequest
	} else if response.Code < 0 {
		statusCode = http.StatusBadGateway
	}
	return service.TaskErrorWrapperLocal(fmt.Errorf("%s", response.Message), strconv.Itoa(response.Code), statusCode)
}

func (a *TaskAdaptor) FetchTask(baseURL, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok || strings.TrimSpace(taskID) == "" {
		return nil, fmt.Errorf("invalid task_id")
	}
	req, err := http.NewRequest(http.MethodGet, strings.TrimRight(baseURL, "/")+"/v1/videos/tasks/"+url.PathEscape(taskID), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Accept", "application/json")
	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, err
	}
	return client.Do(req)
}

func (a *TaskAdaptor) ParseTaskResult(body []byte) (*relaycommon.TaskInfo, error) {
	var response DimensioTaskResponse
	if err := common.Unmarshal(body, &response); err != nil {
		return nil, errors.Wrap(err, "unmarshal dimensio task result failed")
	}
	result := &relaycommon.TaskInfo{}
	switch response.Status {
	case "pending":
		result.Status, result.Progress = model.TaskStatusQueued, "10%"
	case "processing":
		result.Status, result.Progress = model.TaskStatusInProgress, "50%"
		if response.Progress > 0 && response.Progress < 100 {
			result.Progress = strconv.Itoa(response.Progress) + "%"
		}
	case "completed":
		result.Status, result.Progress, result.Url = model.TaskStatusSuccess, "100%", response.Result.URL
	case "failed":
		result.Status, result.Progress, result.Reason = model.TaskStatusFailure, "100%", response.Error
		result.ErrorCode = response.ErrorCode
		if result.Reason == "" {
			result.Reason = "task failed"
		}
	case "not_found":
		result.Status, result.Progress, result.Reason = model.TaskStatusFailure, "100%", "task not found or expired"
		result.ErrorCode = response.ErrorCode
	case "":
		var apiError struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}
		if err := common.Unmarshal(body, &apiError); err != nil || apiError.Message == "" {
			return nil, fmt.Errorf("unknown dimensio task status: %s", response.Status)
		}
		if apiError.Code == 1057 || apiError.Code == 121101 {
			return nil, fmt.Errorf("retryable dimensio error %d: %s", apiError.Code, apiError.Message)
		}
		result.Status, result.Progress, result.Reason = model.TaskStatusFailure, "100%", apiError.Message
		result.ErrorCode = strconv.Itoa(apiError.Code)
	default:
		return nil, fmt.Errorf("unknown dimensio task status: %s", response.Status)
	}
	return result, nil
}

func (a *TaskAdaptor) ConvertToArkVideoTask(task *model.Task) ([]byte, error) {
	var response DimensioTaskResponse
	if err := common.Unmarshal(task.Data, &response); err != nil {
		return nil, errors.Wrap(err, "unmarshal dimensio task data failed")
	}
	converted, err := DimensioToArkTask(response, task.TaskID, task.Properties.OriginModelName, task.SubmitTime, task.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return common.Marshal(converted)
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(task *model.Task) ([]byte, error) {
	var response DimensioTaskResponse
	if err := common.Unmarshal(task.Data, &response); err != nil {
		return nil, errors.Wrap(err, "unmarshal dimensio task data failed")
	}
	video := dto.NewOpenAIVideo()
	video.ID, video.TaskID, video.Model = task.TaskID, task.TaskID, task.Properties.OriginModelName
	video.Status, video.CreatedAt, video.CompletedAt = task.Status.ToVideoStatus(), task.CreatedAt, task.UpdatedAt
	if response.CreatedAt != 0 {
		video.CreatedAt = response.CreatedAt
	}
	if response.UpdatedAt != 0 {
		video.CompletedAt = response.UpdatedAt
	}
	video.SetProgressStr(task.Progress)
	if response.Result.URL != "" {
		video.SetMetadata("url", response.Result.URL)
	}
	if response.Status == "failed" || response.Status == "not_found" || (response.Status == "" && response.Code != 0 && response.Message != "") {
		message := response.Error
		code := response.ErrorCode
		if response.Status == "" {
			message = response.Message
			code = strconv.Itoa(response.Code)
		}
		if message == "" {
			message = "task not found or expired"
		}
		video.Error = &dto.OpenAIVideoError{Code: code, Message: message}
	}
	return common.Marshal(video)
}
