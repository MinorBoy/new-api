package relay

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	relaytypes "github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/cost_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

var (
	ErrCostIdentityUnconfirmed = errors.New("final billable upstream model is not confirmed")
	ErrAuthoritativeCostMeter  = errors.New("authoritative upstream cost meter is unavailable")
)

type costAccountingAdaptor struct {
	channel.Adaptor
	apiType  int
	contract channel.CostAccountingAdaptor
}

func newCostAccountingAdaptor(adaptor channel.Adaptor, apiType int) *costAccountingAdaptor {
	return &costAccountingAdaptor{
		Adaptor:  adaptor,
		apiType:  apiType,
		contract: costContractForAPIType(apiType),
	}
}

func (a *costAccountingAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	mode := cost_setting.Runtime().Mode
	if mode == types.CostAccountingDisabled {
		return a.Adaptor.DoRequest(c, info, requestBody)
	}
	if info == nil || strings.TrimSpace(info.BillableUpstreamModel) == "" {
		if mode == types.CostAccountingStrict {
			return nil, ErrCostIdentityUnconfirmed
		}
		logger.LogWarn(c, "skip cost tracking because the final billable upstream model is not confirmed")
		return a.Adaptor.DoRequest(c, info, requestBody)
	}
	if mode == types.CostAccountingStrict {
		if err := service.RecheckSelectedChannelProfit(c, info); err != nil {
			return nil, relaytypes.NewError(err, relaytypes.ErrorCodeDoRequestFailed)
		}
	}

	billingSource := strings.TrimSpace(info.BillingSource)
	if billingSource == "" {
		billingSource = service.BillingSourceWallet
	}
	requestCtx := context.Background()
	channelName := ""
	costVariantKey := string(types.DefaultCostVariantKey)
	if c != nil && c.Request != nil {
		requestCtx = c.Request.Context()
	}
	if c != nil {
		channelName = c.GetString(string(constant.ContextKeyChannelName))
	}
	if info.ChannelMeta != nil && info.ChannelMeta.Routing != nil {
		costVariantKey = info.ChannelMeta.Routing.CostVariantKey
	} else if c != nil {
		if selected, ok := common.GetContextKeyType[string](c, constant.ContextKeyRoutingCostVariant); ok && strings.TrimSpace(selected) != "" {
			costVariantKey = selected
		}
	}
	var requestMeter *types.CostMeter
	if isOpenAIImagesPath(info.RequestURLPath) {
		if count, countErr := validatedImageCount(info); countErr == nil {
			requestMeter = &types.CostMeter{Source: types.CostMeterValidatedRequest, ImageCount: &count}
		}
	}
	handle, err := service.PrepareCostAttempt(requestCtx, service.PrepareCostAttemptInput{
		RequestID:                 info.RequestId,
		UserID:                    info.UserId,
		TokenID:                   info.TokenId,
		UserGroup:                 info.UserGroup,
		UsingGroup:                info.UsingGroup,
		OriginModelName:           info.OriginModelName,
		BillingSource:             billingSource,
		SubscriptionID:            info.SubscriptionId,
		SubscriptionPlanID:        info.SubscriptionPlanId,
		QuotaPerUnitSnapshot:      strconv.FormatFloat(common.QuotaPerUnit, 'f', -1, 64),
		ChannelID:                 info.ChannelId,
		ChannelName:               channelName,
		ChannelType:               info.ChannelType,
		PredictedUpstreamModel:    info.PredictedUpstreamModel,
		BillableUpstreamModel:     info.BillableUpstreamModel,
		RequestPath:               relaycommon.SafeRequestPath(info.RequestURLPath),
		CostVariantKey:            costVariantKey,
		RequestMeter:              requestMeter,
		CostProfitRecheckSnapshot: info.CostProfitRecheckSnapshot,
	})
	if err != nil {
		if mode == types.CostAccountingStrict {
			var coverageErr *service.CostCoverageError
			if errors.As(err, &coverageErr) {
				return nil, relaytypes.NewError(err, relaytypes.ErrorCodeDoRequestFailed)
			}
			return nil, err
		}
		logger.LogWarn(c, fmt.Sprintf("skip cost tracking: request_id=%s channel_id=%d error=%v", info.RequestId, info.ChannelId, err))
		return a.Adaptor.DoRequest(c, info, requestBody)
	}
	if err := service.AuthorizeCostDispatch(requestCtx, handle); err != nil {
		if mode == types.CostAccountingStrict {
			return nil, err
		}
		logger.LogWarn(c, fmt.Sprintf("skip cost tracking after attempt preparation: request_id=%s channel_id=%d error=%v", info.RequestId, info.ChannelId, err))
		return a.Adaptor.DoRequest(c, info, requestBody)
	}
	info.CostRequestID = handle.CostRequestID
	info.CostAttempt = handle

	response, requestErr := a.Adaptor.DoRequest(c, info, requestBody)
	var httpResponse *http.Response
	if response != nil {
		httpResponse, _ = response.(*http.Response)
	}
	persistenceCtx, cancel := costPersistenceContext()
	outcome := a.ClassifyCostOutcome(info, httpResponse, requestErr)
	info.CostOutcome = &outcome
	outcomeErr := service.RecordCostDispatchOutcome(persistenceCtx, handle, outcome)
	cancel()
	if outcomeErr != nil {
		logger.LogWarn(c, fmt.Sprintf("persist cost dispatch outcome failed: %s", outcomeErr.Error()))
	}
	return response, requestErr
}

func (a *costAccountingAdaptor) DoResponse(c *gin.Context, response *http.Response, info *relaycommon.RelayInfo) (any, *relaytypes.NewAPIError) {
	result, apiErr := a.Adaptor.DoResponse(c, response, info)
	if info == nil || info.CostAttempt == nil {
		return result, apiErr
	}

	if (info.CostAttempt.CostMode == types.CostModePerRequest ||
		info.CostAttempt.CostMode == types.CostModePerImage) && apiErr != nil {
		outcome := a.ClassifyCostOutcome(info, response, nil)
		if outcome.Status != types.CostAttemptNotDispatched {
			// A successful HTTP response with an invalid image payload means the
			// upstream accepted the request, but the result cannot be settled.
			outcome = types.CostOutcome{
				Status:           types.CostAttemptUnknown,
				UpstreamAccepted: response != nil,
				FailureCode:      "upstream_response_invalid",
			}
		}
		persistenceCtx, cancel := costPersistenceContext()
		outcomeErr := service.RecordCostDispatchOutcome(persistenceCtx, info.CostAttempt, outcome)
		info.CostOutcome = &outcome
		cancel()
		if outcomeErr != nil {
			logger.LogWarn(c, fmt.Sprintf("persist cost response outcome failed: %s", outcomeErr.Error()))
		}
	} else if info.CostAttempt.CostMode == types.CostModePerRequest ||
		info.CostAttempt.CostMode == types.CostModePerImage ||
		info.CostAttempt.CostMode == types.CostModePerDuration || info.CostAttempt.CostMode == types.CostModePerToken {
		meter := types.CostMeter{}
		var meterErr error
		if apiErr == nil && info.CostAttempt.CostMode != types.CostModePerRequest {
			meter, meterErr = a.NormalizeCostMeter(info, result)
		} else {
			meterErr = apiErr
		}
		persistenceCtx, cancel := costPersistenceContext()
		settleErr := service.SettleSyncCostAttempt(persistenceCtx, info.CostAttempt, meter)
		cancel()
		if meterErr != nil || settleErr != nil {
			logger.LogWarn(c, fmt.Sprintf("persist cost settlement failed: meter=%v settlement=%v", meterErr, settleErr))
		}
		if meterErr == nil && settleErr == nil {
			outcome := types.CostOutcome{Status: types.CostAttemptSettled, UpstreamAccepted: true}
			info.CostOutcome = &outcome
		}
	}
	if apiErr == nil {
		persistenceCtx, cancel := costPersistenceContext()
		winnerErr := service.MarkWinningCostAttempt(persistenceCtx, info.CostAttempt)
		cancel()
		if winnerErr != nil {
			logger.LogWarn(c, fmt.Sprintf("persist winning cost attempt failed: %s", winnerErr.Error()))
		}
	}
	return result, apiErr
}

func (a *costAccountingAdaptor) CostCapabilities(info *relaycommon.RelayInfo) types.CostCapabilities {
	contract := a.contractForInfo(info)
	if contract == nil {
		return types.CostCapabilities{}
	}
	return contract.CostCapabilities(info)
}

func (a *costAccountingAdaptor) ConfirmCostIdentity(info *relaycommon.RelayInfo, finalRequestBody []byte) error {
	if a.contract == nil {
		return ErrCostIdentityUnconfirmed
	}
	return a.contract.ConfirmCostIdentity(info, finalRequestBody)
}

func (a *costAccountingAdaptor) NormalizeCostMeter(info *relaycommon.RelayInfo, usage any) (types.CostMeter, error) {
	contract := a.contractForInfo(info)
	if contract == nil {
		return types.CostMeter{}, ErrAuthoritativeCostMeter
	}
	return contract.NormalizeCostMeter(info, usage)
}

func (a *costAccountingAdaptor) contractForInfo(info *relaycommon.RelayInfo) channel.CostAccountingAdaptor {
	if info != nil && isOpenAIImagesPath(info.RequestURLPath) && isOpenAIImagesChannelType(info.ChannelType) {
		return openAIImagesCostContract()
	}
	return a.contract
}

func (a *costAccountingAdaptor) ClassifyCostOutcome(info *relaycommon.RelayInfo, response *http.Response, requestErr error) types.CostOutcome {
	contract := a.contractForInfo(info)
	if contract == nil {
		return types.CostOutcome{Status: types.CostAttemptUnknown, FailureCode: "cost_contract_unavailable"}
	}
	return contract.ClassifyCostOutcome(info, response, requestErr)
}

type jsonCostAccountingContract struct {
	capabilities types.CostCapabilities
}

func jsonModelCostContract() *jsonCostAccountingContract {
	return &jsonCostAccountingContract{capabilities: types.CostCapabilities{
		CanResolveBillableModel: true,
		ChargeEvents:            []types.CostChargeEvent{types.CostChargeResponseSucceeded},
		MeterSources:            []types.CostMeterSource{types.CostMeterUpstreamUsage},
	}}
}

func perRequestCostContract() *jsonCostAccountingContract {
	return &jsonCostAccountingContract{capabilities: types.CostCapabilities{
		CanResolveBillableModel: true,
		ChargeEvents:            []types.CostChargeEvent{types.CostChargeResponseSucceeded},
	}}
}

// openAIImagesCostContract describes the shared /v1/images contract used by
// OpenAI-compatible channels. Image rules are charged per successfully
// generated image and may use either the validated request count or an
// authoritative upstream count.
func openAIImagesCostContract() *openAIImagesCostAccountingContract {
	return &openAIImagesCostAccountingContract{jsonCostAccountingContract: &jsonCostAccountingContract{capabilities: types.CostCapabilities{
		CanResolveBillableModel: true,
		ChargeEvents:            []types.CostChargeEvent{types.CostChargeResponseSucceeded},
		MeterSources:            []types.CostMeterSource{types.CostMeterValidatedRequest, types.CostMeterUpstreamActual},
	}}}
}

type openAIImagesCostAccountingContract struct {
	*jsonCostAccountingContract
}

func (c *openAIImagesCostAccountingContract) NormalizeCostMeter(info *relaycommon.RelayInfo, usage any) (types.CostMeter, error) {
	if info != nil && info.ImageBillingSnapshot != nil && info.ImageBillingSnapshot.SettledImages != nil {
		count := *info.ImageBillingSnapshot.SettledImages
		if count < 1 || count > int64(dto.MaxImageN) {
			return types.CostMeter{}, fmt.Errorf("image count must be between 1 and %d", dto.MaxImageN)
		}
		return imageCostMeter(types.CostMeterUpstreamActual, count), nil
	}
	if count, present, err := imageCountFromUsage(usage); present {
		if err != nil {
			return types.CostMeter{}, err
		}
		return imageCostMeter(types.CostMeterUpstreamActual, count), nil
	}
	count, err := validatedImageCount(info)
	if err != nil {
		return types.CostMeter{}, err
	}
	return imageCostMeter(types.CostMeterValidatedRequest, count), nil
}

func imageCostMeter(source types.CostMeterSource, count int64) types.CostMeter {
	return types.CostMeter{Source: source, ImageCount: &count}
}

func imageCountFromUsage(usage any) (int64, bool, error) {
	var count int
	switch value := usage.(type) {
	case *dto.Usage:
		if value == nil {
			return 0, false, nil
		}
		count = value.GeneratedImages
	case dto.Usage:
		count = value.GeneratedImages
	case *dto.BillingUsage:
		if value == nil || value.OpenAIUsage == nil {
			return 0, false, nil
		}
		count = value.OpenAIUsage.GeneratedImages
	default:
		return 0, false, nil
	}
	if count == 0 {
		return 0, false, nil
	}
	if count < 0 || count > dto.MaxImageN {
		return 0, true, fmt.Errorf("image count must be between 1 and %d", dto.MaxImageN)
	}
	return int64(count), true, nil
}

func validatedImageCount(info *relaycommon.RelayInfo) (int64, error) {
	if info == nil {
		return 0, errors.New("image request info is required")
	}
	if info.ImageBillingSnapshot != nil && info.ImageBillingSnapshot.RequestedImages > 0 {
		count := info.ImageBillingSnapshot.RequestedImages
		if count > int64(dto.MaxImageN) {
			return 0, fmt.Errorf("image count must be between 1 and %d", dto.MaxImageN)
		}
		return count, nil
	}
	if count, ok := info.PriceData.OtherRatios()["n"]; ok {
		if count < 1 || count > float64(dto.MaxImageN) || math.Trunc(count) != count {
			return 0, fmt.Errorf("image count must be between 1 and %d", dto.MaxImageN)
		}
		return int64(count), nil
	}
	// OpenAI Images defaults n to one when the client omits it.
	return 1, nil
}

func (c *jsonCostAccountingContract) CostCapabilities(_ *relaycommon.RelayInfo) types.CostCapabilities {
	return types.CostCapabilities{
		CanResolveBillableModel: c.capabilities.CanResolveBillableModel,
		ChargeEvents:            append([]types.CostChargeEvent(nil), c.capabilities.ChargeEvents...),
		MeterSources:            append([]types.CostMeterSource(nil), c.capabilities.MeterSources...),
	}
}

func (c *jsonCostAccountingContract) ConfirmCostIdentity(info *relaycommon.RelayInfo, finalRequestBody []byte) error {
	if info == nil {
		return ErrCostIdentityUnconfirmed
	}
	var identity struct {
		Model *string `json:"model"`
	}
	if len(finalRequestBody) > 0 {
		if err := common.Unmarshal(finalRequestBody, &identity); err != nil {
			return fmt.Errorf("decode final upstream request identity: %w", err)
		}
	}
	modelName := ""
	if identity.Model != nil {
		modelName = strings.TrimSpace(*identity.Model)
	}
	if modelName == "" && info.ChannelMeta != nil {
		modelName = strings.TrimSpace(info.UpstreamModelName)
	}
	if modelName == "" {
		modelName = strings.TrimSpace(info.PredictedUpstreamModel)
	}
	if modelName == "" {
		return ErrCostIdentityUnconfirmed
	}
	info.BillableUpstreamModel = modelName
	return nil
}

func (c *jsonCostAccountingContract) NormalizeCostMeter(_ *relaycommon.RelayInfo, usage any) (types.CostMeter, error) {
	billingUsage := authoritativeBillingUsage(usage)
	if billingUsage == nil || billingUsage.Estimated {
		return types.CostMeter{}, ErrAuthoritativeCostMeter
	}

	meter := types.CostMeter{Source: types.CostMeterUpstreamUsage}
	switch {
	case billingUsage.OpenAIUsage != nil:
		openAIUsage := billingUsage.OpenAIUsage
		input := int64(openAIUsage.PromptTokens)
		if openAIUsage.InputTokens != 0 {
			input = int64(openAIUsage.InputTokens)
		}
		output := int64(openAIUsage.CompletionTokens)
		if openAIUsage.OutputTokens != 0 {
			output = int64(openAIUsage.OutputTokens)
		}
		total := int64(openAIUsage.TotalTokens)
		if total == 0 {
			total = input + output
		}
		meter.InputTokens = &input
		meter.OutputTokens = &output
		meter.CompletionTokens = &output
		meter.TotalTokens = &total
	case billingUsage.ClaudeUsage != nil:
		input := int64(billingUsage.ClaudeUsage.InputTokens)
		output := int64(billingUsage.ClaudeUsage.OutputTokens)
		total := input + output
		meter.InputTokens = &input
		meter.OutputTokens = &output
		meter.CompletionTokens = &output
		meter.TotalTokens = &total
	case billingUsage.GeminiUsageMetadata != nil:
		metadata := billingUsage.GeminiUsageMetadata
		output := int64(metadata.CandidatesTokenCount)
		total := int64(metadata.TotalTokenCount)
		input := int64(metadata.PromptTokenCount + metadata.ToolUsePromptTokenCount)
		if total > 0 && total >= output {
			input = total - output
		}
		if total == 0 {
			total = input + output
		}
		meter.InputTokens = &input
		meter.OutputTokens = &output
		meter.CompletionTokens = &output
		meter.TotalTokens = &total
	default:
		return types.CostMeter{}, ErrAuthoritativeCostMeter
	}
	for name, value := range map[string]*int64{
		"input tokens":      meter.InputTokens,
		"output tokens":     meter.OutputTokens,
		"completion tokens": meter.CompletionTokens,
		"total tokens":      meter.TotalTokens,
	} {
		if value != nil && (*value < 0 || *value > int64(relaycommon.MaxTokensLimit)) {
			return types.CostMeter{}, fmt.Errorf("%s exceeds the supported cost meter range", name)
		}
	}
	return meter, nil
}

func (c *jsonCostAccountingContract) ClassifyCostOutcome(info *relaycommon.RelayInfo, response *http.Response, requestErr error) types.CostOutcome {
	if info != nil && info.CostAttempt != nil && info.CostAttempt.CostMode == types.CostModeFree {
		return types.CostOutcome{Status: types.CostAttemptConfirmedZero, UpstreamAccepted: response != nil}
	}
	if requestErr != nil || response == nil {
		return types.CostOutcome{Status: types.CostAttemptUnknown, FailureCode: "upstream_transport_ambiguous"}
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return types.CostOutcome{Status: types.CostAttemptNotDispatched, FailureCode: "upstream_response_rejected"}
	}
	outcome := types.CostOutcome{Status: types.CostAttemptAwaitingMeter, UpstreamAccepted: true}
	return outcome
}

func costPersistenceContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 10*time.Second)
}

func authoritativeBillingUsage(usage any) *dto.BillingUsage {
	switch value := usage.(type) {
	case *dto.BillingUsage:
		return dto.CloneBillingUsage(value)
	case *dto.Usage:
		if value == nil {
			return nil
		}
		if value.BillingUsage != nil {
			return dto.CloneBillingUsage(value.BillingUsage)
		}
		return dto.NewOpenAIChatBillingUsage(value)
	case *dto.ClaudeUsage:
		if value == nil {
			return nil
		}
		if value.BillingUsage != nil {
			return dto.CloneBillingUsage(value.BillingUsage)
		}
		return dto.NewClaudeMessagesBillingUsage(value)
	case *dto.GeminiUsageMetadata:
		if value == nil {
			return nil
		}
		if value.BillingUsage != nil {
			return dto.CloneBillingUsage(value.BillingUsage)
		}
		return dto.NewGeminiChatBillingUsage(value)
	default:
		return nil
	}
}

func ConfirmCostIdentity(adaptor channel.Adaptor, info *relaycommon.RelayInfo, finalRequestBody []byte) error {
	costAdaptor, ok := adaptor.(channel.CostAccountingAdaptor)
	if !ok {
		if cost_setting.Runtime().Mode == types.CostAccountingStrict {
			return ErrCostIdentityUnconfirmed
		}
		return nil
	}
	if err := costAdaptor.ConfirmCostIdentity(info, finalRequestBody); err != nil {
		if cost_setting.Runtime().Mode == types.CostAccountingStrict {
			return err
		}
	}
	return nil
}

func CostCapabilitiesForRoute(channelType int, requestPath string, taskPlatform constant.TaskPlatform) types.CostCapabilities {
	requestPath = strings.ToLower(strings.TrimSpace(relaycommon.SafeRequestPath(requestPath)))
	if strings.Contains(requestPath, "/realtime") || strings.Contains(requestPath, "/mj") {
		return types.CostCapabilities{}
	}
	if isOpenAIImagesPath(requestPath) {
		if !isOpenAIImagesChannelType(channelType) {
			return types.CostCapabilities{}
		}
		return openAIImagesCostContract().CostCapabilities(nil)
	}
	if taskPlatform != "" {
		return taskCostCapabilities(taskPlatform)
	}
	apiType, ok := common.ChannelType2APIType(channelType)
	if !ok {
		return types.CostCapabilities{}
	}
	contract := costContractForAPIType(apiType)
	if contract == nil {
		return types.CostCapabilities{}
	}
	return contract.CostCapabilities(&relaycommon.RelayInfo{RequestURLPath: requestPath})
}

func isOpenAIImagesPath(requestPath string) bool {
	switch strings.ToLower(strings.TrimSpace(relaycommon.SafeRequestPath(requestPath))) {
	case "/v1/images/generations", "/v1/images/edits":
		return true
	default:
		return false
	}
}

func isOpenAIImagesChannelType(channelType int) bool {
	switch channelType {
	case constant.ChannelTypeOpenAI,
		constant.ChannelTypeAzure,
		constant.ChannelTypeOpenAIMax,
		constant.ChannelTypeCustom,
		constant.ChannelTypeOpenRouter,
		constant.ChannelTypeXinference:
		return true
	default:
		return false
	}
}

func costContractForAPIType(apiType int) channel.CostAccountingAdaptor {
	switch apiType {
	case constant.APITypeOpenAI, constant.APITypeAnthropic, constant.APITypeGemini, constant.APITypeOpenRouter:
		return jsonModelCostContract()
	case constant.APITypePaLM, constant.APITypeBaidu, constant.APITypeZhipu, constant.APITypeAli,
		constant.APITypeXunfei, constant.APITypeTencent, constant.APITypeZhipuV4, constant.APITypeOllama,
		constant.APITypePerplexity, constant.APITypeAws, constant.APITypeCohere, constant.APITypeDify,
		constant.APITypeJina, constant.APITypeCloudflare, constant.APITypeSiliconFlow, constant.APITypeVertexAi,
		constant.APITypeMistral, constant.APITypeDeepSeek, constant.APITypeMokaAI, constant.APITypeVolcEngine,
		constant.APITypeBaiduV2, constant.APITypeXinference, constant.APITypeXai, constant.APITypeCoze,
		constant.APITypeJimeng, constant.APITypeMoonshot, constant.APITypeSubmodel, constant.APITypeMiniMax,
		constant.APITypeReplicate, constant.APITypeCodex, constant.APITypeAdvancedCustom:
		return perRequestCostContract()
	default:
		return nil
	}
}

func taskCostCapabilities(platform constant.TaskPlatform) types.CostCapabilities {
	adaptor := GetTaskAdaptor(platform)
	costAdaptor, ok := adaptor.(channel.TaskCostAccountingAdaptor)
	if !ok {
		return types.CostCapabilities{}
	}
	return costAdaptor.CostCapabilities(nil)
}

var _ channel.CostAccountingAdaptor = (*costAccountingAdaptor)(nil)
var _ channel.CostAccountingAdaptor = (*jsonCostAccountingContract)(nil)
