package newapivideo

import (
	"bytes"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	taskcommon "github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

var ModelList = []string{}

const ChannelName = "NewAPIVideo"

type TaskAdaptor struct {
	taskcommon.BaseBilling
	apiKey  string
	baseURL string
}

type upstreamSubmitResponse struct {
	ID        string `json:"id"`
	TaskID    string `json:"task_id"`
	Object    string `json:"object"`
	Model     string `json:"model"`
	Status    string `json:"status"`
	Progress  int    `json:"progress"`
	CreatedAt int64  `json:"created_at"`
}

type upstreamErrorEnvelope struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Error   *upstreamError `json:"error,omitempty"`
}

type upstreamError struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	if info == nil {
		return
	}
	a.apiKey = info.ApiKey
	a.baseURL = strings.TrimRight(info.ChannelBaseUrl, "/")
}

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	mediaType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return service.TaskErrorWrapperLocal(fmt.Errorf("new-api video requests must use application/json"), "unsupported_media_type", http.StatusUnsupportedMediaType)
	}
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_json", http.StatusBadRequest)
	}
	body, err := storage.Bytes()
	if err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_json", http.StatusBadRequest)
	}
	if c.GetBool(common.KeySeedanceOfficialAPI) {
		return validateARKRequest(c, info, body)
	}
	return validateOpenAIRequest(c, info, body)
}

func (a *TaskAdaptor) BuildRequestURL(_ *relaycommon.RelayInfo) (string, error) {
	return a.baseURL + "/v1/video/generations", nil
}

func (a *TaskAdaptor) BuildRequestHeader(_ *gin.Context, req *http.Request, _ *relaycommon.RelayInfo) error {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	modelName := ""
	if info != nil {
		modelName = info.UpstreamModelName
		if modelName == "" {
			modelName = info.OriginModelName
		}
	}
	var body []byte
	var err error
	if c.GetBool(common.KeySeedanceOfficialAPI) {
		body, err = buildARKRequestBody(c, modelName)
	} else {
		body, err = buildOpenAIRequestBody(c, modelName)
	}
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(body), nil
}

func (a *TaskAdaptor) EstimateDurationSeconds(c *gin.Context, _ *relaycommon.RelayInfo) (int, *dto.TaskError) {
	state, err := getRequestState(c)
	if err != nil {
		return 0, service.TaskErrorWrapperLocal(err, "invalid_duration", http.StatusBadRequest)
	}
	value := state.Seconds
	if value == nil || !value.Equal(value.Truncate(0)) {
		return 0, service.TaskErrorWrapperLocal(fmt.Errorf("per_duration billing requires an integer seconds value"), "invalid_seconds", http.StatusBadRequest)
	}
	return int(value.IntPart()), nil
}

func (a *TaskAdaptor) GetModelList() []string { return ModelList }

func (a *TaskAdaptor) GetChannelName() string { return ChannelName }

func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *dto.TaskError) {
	if resp == nil || resp.Body == nil {
		return "", nil, service.TaskErrorWrapperLocal(fmt.Errorf("upstream response is empty"), "invalid_response", http.StatusBadGateway)
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return "", body, service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
	}
	var response upstreamSubmitResponse
	if err := common.Unmarshal(body, &response); err != nil {
		return "", body, service.TaskErrorWrapperLocal(fmt.Errorf("invalid upstream submit response: %w", err), "invalid_response", http.StatusBadGateway)
	}
	if response.ID != "" && response.TaskID != "" && response.ID != response.TaskID {
		return "", body, service.TaskErrorWrapperLocal(fmt.Errorf("upstream id and task_id do not match"), "invalid_response", http.StatusBadGateway)
	}
	taskID = response.TaskID
	if taskID == "" {
		taskID = response.ID
	}
	if taskID == "" {
		return "", body, service.TaskErrorWrapperLocal(fmt.Errorf("upstream task id is empty"), "invalid_response", http.StatusBadGateway)
	}
	if info == nil || info.TaskRelayInfo == nil || strings.TrimSpace(info.PublicTaskID) == "" {
		return "", body, service.TaskErrorWrapperLocal(fmt.Errorf("public task id is empty"), "invalid_response", http.StatusInternalServerError)
	}

	if c.GetBool(common.KeySeedanceOfficialAPI) {
		c.JSON(http.StatusOK, gin.H{"id": info.PublicTaskID})
		return taskID, body, nil
	}
	video := dto.NewOpenAIVideo()
	video.ID = info.PublicTaskID
	video.TaskID = info.PublicTaskID
	video.Model = info.OriginModelName
	if response.Status != "" {
		video.Status = response.Status
	}
	video.Progress = response.Progress
	video.CreatedAt = response.CreatedAt
	c.JSON(http.StatusOK, video)
	return taskID, body, nil
}

func (a *TaskAdaptor) FetchTask(baseURL, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok || strings.TrimSpace(taskID) == "" {
		return nil, fmt.Errorf("invalid task_id")
	}
	requestURL := strings.TrimRight(baseURL, "/") + "/v1/video/generations/" + url.PathEscape(taskID)
	req, err := http.NewRequest(http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Accept", "application/json")
	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	if client == nil {
		client = http.DefaultClient
	}
	return client.Do(req)
}

func (a *TaskAdaptor) ParseTaskError(body []byte, statusCode int) *dto.TaskError {
	var response upstreamErrorEnvelope
	if err := common.Unmarshal(body, &response); err != nil {
		return service.TaskErrorWrapper(fmt.Errorf("%s", string(body)), "fail_to_fetch_task", statusCode)
	}
	code, message := response.Code, response.Message
	if response.Error != nil {
		if response.Error.Code != "" {
			code = response.Error.Code
		}
		if response.Error.Message != "" {
			message = response.Error.Message
		}
	}
	if code == "" && message == "" {
		return service.TaskErrorWrapper(fmt.Errorf("%s", string(body)), "fail_to_fetch_task", statusCode)
	}
	if message == "" {
		message = code
	}
	if code == "" {
		code = "upstream_error"
	}
	return service.TaskErrorWrapper(fmt.Errorf("%s", message), code, statusCode)
}
