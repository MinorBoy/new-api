package relay

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
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
	if cost_setting.Runtime().Mode == types.CostAccountingStrict && strings.TrimSpace(info.BillableUpstreamModel) == "" {
		return nil, ErrCostIdentityUnconfirmed
	}
	return a.Adaptor.DoRequest(c, info, requestBody)
}

func (a *costAccountingAdaptor) DoResponse(c *gin.Context, response *http.Response, info *relaycommon.RelayInfo) (any, *types.NewAPIError) {
	return a.Adaptor.DoResponse(c, response, info)
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
	return meter, nil
}

func (c *jsonCostAccountingContract) ClassifyCostOutcome(info *relaycommon.RelayInfo, response *http.Response, requestErr error) types.CostOutcome {
	if requestErr != nil || response == nil {
		return types.CostOutcome{Status: types.CostAttemptUnknown, FailureCode: "upstream_transport_ambiguous"}
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return types.CostOutcome{Status: types.CostAttemptUnknown, UpstreamAccepted: true, FailureCode: "upstream_response_rejected"}
	}
	outcome := types.CostOutcome{Status: types.CostAttemptAwaitingMeter, UpstreamAccepted: true}
	if info != nil && info.CostAttempt != nil {
		switch info.CostAttempt.CostMode {
		case types.CostModeFree:
			outcome.Status = types.CostAttemptConfirmedZero
		case types.CostModePerRequest:
			outcome.Status = types.CostAttemptSettled
		}
	}
	return outcome
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
