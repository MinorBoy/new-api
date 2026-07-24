package relay

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
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
	if cost_setting.Runtime().Mode != types.CostAccountingStrict {
		return a.Adaptor.DoRequest(c, info, requestBody)
	}
	if info == nil || strings.TrimSpace(info.BillableUpstreamModel) == "" {
		return nil, ErrCostIdentityUnconfirmed
	}

	billingSource := strings.TrimSpace(info.BillingSource)
	if billingSource == "" {
		billingSource = service.BillingSourceWallet
	}
	requestCtx := context.Background()
	channelName := ""
	if c != nil && c.Request != nil {
		requestCtx = c.Request.Context()
	}
	if c != nil {
		channelName = c.GetString(string(constant.ContextKeyChannelName))
	}
	handle, err := service.PrepareCostAttempt(requestCtx, service.PrepareCostAttemptInput{
		RequestID:              info.RequestId,
		UserID:                 info.UserId,
		TokenID:                info.TokenId,
		UserGroup:              info.UserGroup,
		UsingGroup:             info.UsingGroup,
		OriginModelName:        info.OriginModelName,
		BillingSource:          billingSource,
		SubscriptionID:         info.SubscriptionId,
		SubscriptionPlanID:     info.SubscriptionPlanId,
		QuotaPerUnitSnapshot:   strconv.FormatFloat(common.QuotaPerUnit, 'f', -1, 64),
		ChannelID:              info.ChannelId,
		ChannelName:            channelName,
		ChannelType:            info.ChannelType,
		PredictedUpstreamModel: info.PredictedUpstreamModel,
		BillableUpstreamModel:  info.BillableUpstreamModel,
		RequestPath:            info.RequestURLPath,
	})
	if err != nil {
		var coverageErr *service.CostCoverageError
		if errors.As(err, &coverageErr) {
			return nil, types.NewError(err, types.ErrorCodeDoRequestFailed)
		}
		return nil, err
	}
	info.CostRequestID = handle.CostRequestID
	info.CostAttempt = handle
	if err := service.AuthorizeCostDispatch(requestCtx, handle); err != nil {
		return nil, err
	}

	response, requestErr := a.Adaptor.DoRequest(c, info, requestBody)
	var httpResponse *http.Response
	if response != nil {
		httpResponse, _ = response.(*http.Response)
	}
	persistenceCtx, cancel := costPersistenceContext()
	outcomeErr := service.RecordCostDispatchOutcome(persistenceCtx, handle, a.ClassifyCostOutcome(info, httpResponse, requestErr))
	cancel()
	if outcomeErr != nil {
		logger.LogWarn(c, fmt.Sprintf("persist cost dispatch outcome failed: %s", outcomeErr.Error()))
	}
	return response, requestErr
}

func (a *costAccountingAdaptor) DoResponse(c *gin.Context, response *http.Response, info *relaycommon.RelayInfo) (any, *types.NewAPIError) {
	result, apiErr := a.Adaptor.DoResponse(c, response, info)
	if info == nil || info.CostAttempt == nil {
		return result, apiErr
	}

	if info.CostAttempt.CostMode == types.CostModePerRequest && apiErr != nil {
		persistenceCtx, cancel := costPersistenceContext()
		outcomeErr := service.RecordCostDispatchOutcome(persistenceCtx, info.CostAttempt, types.CostOutcome{
			Status:           types.CostAttemptUnknown,
			UpstreamAccepted: true,
			FailureCode:      "upstream_response_invalid",
		})
		cancel()
		if outcomeErr != nil {
			logger.LogWarn(c, fmt.Sprintf("persist cost response outcome failed: %s", outcomeErr.Error()))
		}
	} else if info.CostAttempt.CostMode == types.CostModePerRequest ||
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
	if a.contract == nil {
		return types.CostCapabilities{}
	}
	return a.contract.CostCapabilities(info)
}

func (a *costAccountingAdaptor) ConfirmCostIdentity(info *relaycommon.RelayInfo, finalRequestBody []byte) error {
	if a.contract == nil {
		return ErrCostIdentityUnconfirmed
	}
	return a.contract.ConfirmCostIdentity(info, finalRequestBody)
}

func (a *costAccountingAdaptor) NormalizeCostMeter(info *relaycommon.RelayInfo, usage any) (types.CostMeter, error) {
	if a.contract == nil {
		return types.CostMeter{}, ErrAuthoritativeCostMeter
	}
	return a.contract.NormalizeCostMeter(info, usage)
}

func (a *costAccountingAdaptor) ClassifyCostOutcome(info *relaycommon.RelayInfo, response *http.Response, requestErr error) types.CostOutcome {
	if a.contract == nil {
		return types.CostOutcome{Status: types.CostAttemptUnknown, FailureCode: "cost_contract_unavailable"}
	}
	return a.contract.ClassifyCostOutcome(info, response, requestErr)
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
		return types.CostOutcome{Status: types.CostAttemptUnknown, UpstreamAccepted: true, FailureCode: "upstream_response_rejected"}
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
	requestPath = strings.ToLower(strings.TrimSpace(requestPath))
	if strings.Contains(requestPath, "/realtime") || strings.Contains(requestPath, "/mj") {
		return types.CostCapabilities{}
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
