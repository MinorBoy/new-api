package newapivideo

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/pkg/modelrouting"
	"github.com/QuantumNous/new-api/relay/channel"
	taskcommon "github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

const ChannelName = "NewAPIVideo"

const megaByAIMediaValidationContextKey = "newapi_video_megabyai_media_validation"

type megaByAIMediaValidation struct {
	taskErr *dto.TaskError
}

type TaskAdaptor struct {
	taskcommon.BaseBilling
	apiKey  string
	baseURL string
	profile protocolProfile
}

func (a *TaskAdaptor) CostCapabilities(_ *relaycommon.RelayInfo) types.CostCapabilities {
	return taskcommon.TaskCostCapabilities(
		types.CostMeterValidatedRequest,
		types.CostMeterUpstreamActual,
		types.CostMeterUpstreamUsage,
	)
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
		profile := a.activeProfile()
		if taskErr := validateARKRequest(c, info, body, profile); taskErr != nil {
			return taskErr
		}
		if profile.requestDialect == videoRequestDialectTextJSON {
			state, err := getRequestState(c)
			if err != nil || state.ARK == nil {
				return service.TaskErrorWrapperLocal(fmt.Errorf("ARK request state is missing"), "InvalidParameter", http.StatusBadRequest)
			}
			if profile.textRequest == nil {
				return service.TaskErrorWrapperLocal(fmt.Errorf("text request profile is missing"), "internal_error", http.StatusInternalServerError)
			}
			if err := validateTextVideoRequest(*state.ARK, *profile.textRequest); err != nil {
				var requestErr *arkRequestError
				if errors.As(err, &requestErr) {
					return service.TaskErrorWrapperLocal(err, requestErr.Code, http.StatusBadRequest)
				}
				return service.TaskErrorWrapperLocal(err, "InvalidParameter", http.StatusBadRequest)
			}
			return nil
		}
		if profile.requestDialect != videoRequestDialectMegaReferenceArrays {
			return nil
		}

		state, err := getRequestState(c)
		if err != nil || state.ARK == nil {
			return service.TaskErrorWrapperLocal(fmt.Errorf("ARK request state is missing"), "InvalidParameter", http.StatusBadRequest)
		}
		if err := validateMegaByAIRequest(*state.ARK); err != nil {
			var requestErr *arkRequestError
			if errors.As(err, &requestErr) {
				return service.TaskErrorWrapperLocal(err, requestErr.Code, http.StatusBadRequest)
			}
			return service.TaskErrorWrapperLocal(err, "InvalidParameter", http.StatusBadRequest)
		}

		var validation megaByAIMediaValidation
		if cached, exists := c.Get(megaByAIMediaValidationContextKey); exists {
			var ok bool
			validation, ok = cached.(megaByAIMediaValidation)
			if !ok {
				return service.TaskErrorWrapperLocal(fmt.Errorf("MegaByAI media validation state is invalid"), "internal_error", http.StatusInternalServerError)
			}
		} else {
			validation.taskErr = validateMegaByAIMedia(c.Request.Context(), *state.ARK)
			c.Set(megaByAIMediaValidationContextKey, validation)
		}
		if validation.taskErr != nil {
			return validation.taskErr
		}

		state.ProviderValidationComplete = true
		c.Set(requestStateContextKey, state)
		return nil
	}
	return validateOpenAIRequest(c, info, body)
}

func validateMegaByAIMedia(ctx context.Context, request arkRequest) *dto.TaskError {
	videoURLs := make([]string, 0, 3)
	audioURLs := make([]string, 0, 3)
	for _, item := range request.Content {
		switch item.Type {
		case "video_url":
			videoURLs = append(videoURLs, item.VideoURL.URL)
		case "audio_url":
			audioURLs = append(audioURLs, item.AudioURL.URL)
		}
	}
	if len(videoURLs) > 0 {
		durationMS, err := service.ResolveReferenceVideoDurationMS(ctx, videoURLs)
		if err != nil {
			var metadataErr *service.VideoMetadataError
			if errors.As(err, &metadataErr) && metadataErr.Kind == service.VideoMetadataInvalidMedia {
				return service.TaskErrorWrapperLocal(fmt.Errorf("reference video is invalid"), "InvalidParameter.content", http.StatusBadRequest)
			}
			return service.TaskErrorWrapperLocal(fmt.Errorf("reference video metadata is unavailable"), "reference_media_metadata_unavailable", http.StatusServiceUnavailable)
		}
		if durationMS > maxMegaByAIReferenceDurationMS {
			return service.TaskErrorWrapperLocal(fmt.Errorf("reference video duration exceeds 15 seconds"), "InvalidParameter.content", http.StatusBadRequest)
		}
	}
	if len(audioURLs) > 0 {
		durationMS, err := service.ResolveReferenceAudioDurationMS(ctx, audioURLs)
		if err != nil {
			var durationErr *service.ReferenceAudioDurationError
			if errors.As(err, &durationErr) && durationErr.Kind == service.ReferenceAudioInvalidMedia {
				return service.TaskErrorWrapperLocal(fmt.Errorf("reference audio is invalid"), "InvalidParameter.content", http.StatusBadRequest)
			}
			return service.TaskErrorWrapperLocal(fmt.Errorf("reference audio metadata is unavailable"), "reference_media_metadata_unavailable", http.StatusServiceUnavailable)
		}
		if durationMS > maxMegaByAIReferenceDurationMS {
			return service.TaskErrorWrapperLocal(fmt.Errorf("reference audio duration exceeds 15 seconds"), "InvalidParameter.content", http.StatusBadRequest)
		}
	}
	return nil
}

// ValidateBillingRequest runs after model mapping and before pricing/pre-consume.
// Lucen uses it for mapped-resolution validation and for resolving an omitted Ark
// duration from capability-routing facts, so provider constraints cannot be bypassed
// by a client alias or discovered only while building the upstream body.
func (a *TaskAdaptor) ValidateBillingRequest(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	profile := a.activeProfile()
	if profile.channelName != ChannelNameLucen && (profile.textRequest == nil || !profile.textRequest.enforceModelResolutionSuffix) {
		return nil
	}
	state, err := getRequestState(c)
	if err != nil {
		return service.TaskErrorWrapperLocal(fmt.Errorf("ARK request state is missing"), "InvalidParameter", http.StatusBadRequest)
	}
	if state.ARK == nil {
		return nil
	}
	if !common.GetContextKeyBool(c, constant.ContextKeyRoutingCapabilityMode) {
		upstreamModel := ""
		if info != nil {
			upstreamModel = info.UpstreamModelName
		}
		var validationErr error
		if profile.textRequest != nil && profile.textRequest.enforceModelResolutionSuffix {
			validationErr = validateTextVideoRequest(*state.ARK, *profile.textRequest, upstreamModel)
		} else {
			validationErr = validateMappedResolution(state.ARK.Resolution, upstreamModel)
		}
		if validationErr != nil {
			code := "InvalidParameter.resolution"
			var requestErr *arkRequestError
			if errors.As(validationErr, &requestErr) {
				code = requestErr.Code
			}
			return service.TaskErrorWrapperLocal(validationErr, code, http.StatusBadRequest)
		}
	}
	if state.Seconds == nil && profile.useRoutingDurationDefault {
		if duration := routingDurationSeconds(c); duration > 0 {
			value := decimal.NewFromInt(int64(duration))
			state.Seconds = &value
			state.ARK.Duration = common.GetPointer(duration)
			c.Set(requestStateContextKey, state)
			taskRequest, requestErr := relaycommon.GetTaskRequest(c)
			if requestErr == nil {
				taskRequest.Duration = duration
				relaycommon.StoreTaskRequest(c, info, constant.TaskActionGenerate, taskRequest)
			}
		}
	}
	return nil
}

func routingDurationSeconds(c *gin.Context) int {
	if facts, ok := common.GetContextKeyType[modelrouting.Facts](c, constant.ContextKeyRoutingFacts); ok && facts.DurationSeconds > 0 {
		return facts.DurationSeconds
	}
	if input, ok := common.GetContextKeyType[modelrouting.FactsInput](c, constant.ContextKeyRoutingFactsInput); ok && input.DurationSeconds != nil && *input.DurationSeconds > 0 {
		return *input.DurationSeconds
	}
	return 0
}

func (a *TaskAdaptor) BuildRequestURL(_ *relaycommon.RelayInfo) (string, error) {
	return a.baseURL + a.activeProfile().submitPath, nil
}

func (a *TaskAdaptor) BuildRequestHeader(_ *gin.Context, req *http.Request, _ *relaycommon.RelayInfo) error {
	req.Header.Set("Content-Type", a.activeProfile().contentType)
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
		profile := a.activeProfile()
		switch profile.requestDialect {
		case videoRequestDialectTextJSON:
			state, stateErr := getRequestState(c)
			if stateErr != nil {
				return nil, stateErr
			}
			if state.ARK == nil {
				return nil, fmt.Errorf("ARK request state is missing")
			}
			if profile.textRequest == nil {
				return nil, fmt.Errorf("text request profile is missing")
			}
			body, err = buildTextVideoRequest(*state.ARK, modelName, *profile.textRequest)
		case videoRequestDialectMegaReferenceArrays:
			state, stateErr := getRequestState(c)
			if stateErr != nil {
				return nil, stateErr
			}
			if state.ARK == nil {
				return nil, fmt.Errorf("ARK request state is missing")
			}
			if !state.ProviderValidationComplete {
				return nil, fmt.Errorf("MegaByAI provider validation is incomplete")
			}
			body, err = buildMegaByAIRequest(*state.ARK, modelName)
		default:
			body, err = buildARKRequestBody(c, info, profile)
		}
	} else {
		body, err = buildOpenAIRequestBody(c, modelName)
	}
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(body), nil
}

func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, body io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, body)
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
	pollPath := a.activeProfile().pollPath
	if strings.Count(pollPath, "{task_id}") != 1 {
		return nil, fmt.Errorf("task polling path must contain {task_id} exactly once")
	}
	pollPath = strings.Replace(pollPath, "{task_id}", url.PathEscape(taskID), 1)
	requestURL := strings.TrimRight(baseURL, "/") + pollPath
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
