package service

import (
	"errors"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelrouting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

var routingPolicySnapshotLookup = model.GetRoutingPolicySnapshot

type ChannelSelectionError struct {
	Code        types.ErrorCode
	StatusCode  int
	Err         error
	Diagnostics []modelrouting.Audit
}

func (e *ChannelSelectionError) Error() string {
	return e.Err.Error()
}

type groupRoutingResult struct {
	Capability bool
	Snapshot   modelrouting.PolicySnapshot
	Facts      modelrouting.Facts
	Evaluation modelrouting.Evaluation
}

func evaluateGroupRouting(group, modelName string, input *modelrouting.FactsInput) (groupRoutingResult, error) {
	if input == nil {
		return groupRoutingResult{}, nil
	}
	snapshot, ok := routingPolicySnapshotLookup(group, modelName)
	if !ok {
		return groupRoutingResult{}, nil
	}
	if !snapshot.Enabled || snapshot.ID <= 0 || snapshot.GroupName != group || snapshot.CanonicalModel != modelName || snapshot.TargetsByChannel == nil {
		return groupRoutingResult{}, &ChannelSelectionError{
			Code: types.ErrorCodeRoutingPolicyError, StatusCode: http.StatusInternalServerError,
			Err: errors.New("routing policy cache is invalid"),
		}
	}
	facts, err := modelrouting.ResolveFacts(group, *input, snapshot.Defaults)
	if err != nil {
		return groupRoutingResult{}, &ChannelSelectionError{
			Code: types.ErrorCodeRoutingPolicyError, StatusCode: http.StatusInternalServerError, Err: err,
		}
	}
	evaluation := modelrouting.Evaluate(snapshot, facts)
	result := groupRoutingResult{Capability: true, Snapshot: snapshot, Facts: facts, Evaluation: evaluation}
	if len(evaluation.CompatibleByChannel) == 0 {
		return result, &ChannelSelectionError{
			Code: types.ErrorCodeNoCompatibleRoute, StatusCode: http.StatusBadRequest,
			Err: errors.New("no compatible route supports this request"),
			Diagnostics: []modelrouting.Audit{{
				PolicyID: snapshot.ID, Facts: facts, MismatchCounts: evaluation.MismatchCounts,
			}},
		}
	}
	return result, nil
}

func selectChannelForGroup(param *RetryParam, group string, priorityRetry int) (*model.Channel, groupRoutingResult, error) {
	result, err := evaluateGroupRouting(group, param.ModelName, param.RoutingInput)
	if err != nil {
		return nil, result, err
	}
	filter := model.ChannelSelectFilter{}
	if result.Capability {
		filter.AllowedChannelIDs = make(map[int]struct{}, len(result.Evaluation.CompatibleByChannel))
		for channelID := range result.Evaluation.CompatibleByChannel {
			filter.AllowedChannelIDs[channelID] = struct{}{}
		}
		filter.ExcludedChannelIDs = param.ExcludedChannelIDs
	}
	channel, err := model.GetRandomSatisfiedChannel(group, param.ModelName, priorityRetry, param.RequestPath, filter)
	if err != nil {
		return nil, result, &ChannelSelectionError{
			Code: types.ErrorCodeRoutingPolicyError, StatusCode: http.StatusInternalServerError, Err: err,
		}
	}
	if channel == nil {
		if !result.Capability {
			return nil, result, nil
		}
		return nil, result, &ChannelSelectionError{
			Code: types.ErrorCodeCompatibleChannelUnavailable, StatusCode: http.StatusServiceUnavailable,
			Err: errors.New("compatible channels are unavailable"),
			Diagnostics: []modelrouting.Audit{{
				PolicyID: result.Snapshot.ID, Facts: result.Facts, MismatchCounts: result.Evaluation.MismatchCounts,
			}},
		}
	}
	if result.Capability {
		target, ok := result.Evaluation.CompatibleByChannel[channel.Id]
		if !ok {
			return nil, result, &ChannelSelectionError{
				Code: types.ErrorCodeRoutingPolicyError, StatusCode: http.StatusInternalServerError,
				Err: errors.New("selected channel has no routing target"),
			}
		}
		publishRoutingDecision(param.Ctx, result, target)
	}
	return channel, result, nil
}

func ValidateKnownChannelForRouting(param *RetryParam, group string, channelID int) (bool, error) {
	clearRoutingDecision(param.Ctx)
	result, err := evaluateGroupRouting(group, param.ModelName, param.RoutingInput)
	if err != nil {
		return false, err
	}
	if !result.Capability {
		return true, nil
	}
	target, compatible := result.Evaluation.CompatibleByChannel[channelID]
	if !compatible {
		return false, nil
	}
	if _, excluded := param.ExcludedChannelIDs[channelID]; excluded ||
		!model.IsChannelEnabledForGroupModel(group, param.ModelName, channelID) {
		return false, &ChannelSelectionError{
			Code: types.ErrorCodeCompatibleChannelUnavailable, StatusCode: http.StatusServiceUnavailable,
			Err: errors.New("compatible channel is unavailable"),
			Diagnostics: []modelrouting.Audit{{
				PolicyID: result.Snapshot.ID, Facts: result.Facts, MismatchCounts: result.Evaluation.MismatchCounts,
			}},
		}
	}
	publishRoutingDecision(param.Ctx, result, target)
	return true, nil
}

func publishRoutingDecision(c *gin.Context, result groupRoutingResult, target modelrouting.Target) {
	common.SetContextKey(c, constant.ContextKeyRoutingCapabilityMode, true)
	common.SetContextKey(c, constant.ContextKeyRoutingPolicyID, result.Snapshot.ID)
	common.SetContextKey(c, constant.ContextKeyRoutingTargetID, target.ID)
	common.SetContextKey(c, constant.ContextKeyRoutingTargetName, target.Name)
	common.SetContextKey(c, constant.ContextKeyRoutingUpstreamModel, target.UpstreamModel)
	common.SetContextKey(c, constant.ContextKeyRoutingFacts, result.Facts)
	common.SetContextKey(c, constant.ContextKeyRoutingMismatchCounts, result.Evaluation.MismatchCounts)
}

func clearRoutingDecision(c *gin.Context) {
	if c == nil {
		return
	}
	common.SetContextKey(c, constant.ContextKeyRoutingCapabilityMode, false)
	common.SetContextKey(c, constant.ContextKeyRoutingPolicyID, 0)
	common.SetContextKey(c, constant.ContextKeyRoutingTargetID, 0)
	common.SetContextKey(c, constant.ContextKeyRoutingTargetName, "")
	common.SetContextKey(c, constant.ContextKeyRoutingUpstreamModel, "")
	common.SetContextKey(c, constant.ContextKeyRoutingFacts, modelrouting.Facts{})
	common.SetContextKey(c, constant.ContextKeyRoutingMismatchCounts, map[modelrouting.MismatchReason]int{})
}

func RecordRoutingSelectionFailure(c *gin.Context, canonicalModel string, selectionErr *ChannelSelectionError) {
	if c == nil || selectionErr == nil || !constant.ErrorLogEnabled {
		return
	}
	other := map[string]interface{}{
		"error_type":  string(types.ErrorTypeNewAPIError),
		"error_code":  selectionErr.Code,
		"status_code": selectionErr.StatusCode,
		"admin_info": map[string]interface{}{
			"routing_diagnostics": selectionErr.Diagnostics,
		},
	}
	if c.Request != nil && c.Request.URL != nil {
		other["request_path"] = c.Request.URL.Path
	}
	startTime := common.GetContextKeyTime(c, constant.ContextKeyRequestStartTime)
	if startTime.IsZero() {
		startTime = time.Now()
	}
	model.RecordErrorLog(
		c,
		common.GetContextKeyInt(c, constant.ContextKeyUserId),
		0,
		canonicalModel,
		c.GetString("token_name"),
		selectionErr.Error(),
		common.GetContextKeyInt(c, constant.ContextKeyTokenId),
		int(time.Since(startTime).Seconds()),
		common.GetContextKeyBool(c, constant.ContextKeyIsStream),
		common.GetContextKeyString(c, constant.ContextKeyUserGroup),
		other,
	)
}
