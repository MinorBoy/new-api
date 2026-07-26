package service

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelrouting"
	"github.com/QuantumNous/new-api/setting/cost_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
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

// applyProfitFilter narrows filter.AllowedChannelIDs (or the legacy candidate pool when
// AllowedChannelIDs is empty) to channels whose predicted margin meets the minimum
// expected margin threshold. It is a no-op outside strict cost-accounting mode and
// preserves priority/weight/random selection: it only intersects the candidate set.
//
// The same function serves the normal, auto, affinity, specified and retry paths so
// none of them can bypass the margin gate. Unknown revenue, missing cost rule, unknown
// meter, metadata-service failure and calculation overflow all exclude the candidate
// (fail-closed). costCoverageMisses diagnostics are kept separate from profit
// exclusions — unknown cost is never treated as coverage success.
func applyProfitFilter(param *RetryParam, group string, result groupRoutingResult, filter model.ChannelSelectFilter) (model.ChannelSelectFilter, error) {
	if cost_setting.Runtime().Mode != types.CostAccountingStrict {
		return filter, nil
	}
	if param == nil || param.Ctx == nil {
		return filter, nil
	}

	candidates := profitCandidates(param, group, result, filter)
	if len(candidates) == 0 {
		return filter, nil
	}

	facts, revenueNanoUSD, hasRevenue, _ := profitFilterFacts(param, group)
	rules, err := ActiveCostRules(profitCandidateKeys(candidates), false)
	if err != nil {
		common.SysError(fmt.Sprintf("profit routing active rule batch failed: %s", err.Error()))
		// Fail-closed: drop every candidate so the caller returns 503 instead of
		// admitting an unpriced channel.
		return emptyAllowedFilter(filter), nil
	}

	filterResult := FilterProfitEligibleChannels(ProfitChannelFilterInput{
		Ctx:             param.Ctx,
		Facts:           facts,
		RevenueNanoUSD:  revenueNanoUSD,
		HasRevenue:      hasRevenue,
		GlobalMarginBPS: cost_setting.Runtime().MinimumExpectedMarginBPS,
		Candidates:      candidates,
		MetadataState:   param.ProfitRoutingState(),
	}, rules)

	if filterResult.InvalidMedia {
		return filter, &ChannelSelectionError{
			Code:       types.ErrorCodeInvalidRequest,
			StatusCode: http.StatusBadRequest,
			Err:        errors.New("input video is not supported"),
		}
	}
	recordProfitExclusions(param, result, filterResult)
	if len(filterResult.AllowedChannelIDs) == 0 {
		return emptyAllowedFilter(filter), nil
	}
	return model.ChannelSelectFilter{
		AllowedChannelIDs:  filterResult.AllowedChannelIDs,
		ExcludedChannelIDs: filter.ExcludedChannelIDs,
	}, nil
}

// profitCandidates builds the (channelID, predicted model, threshold) list the filter
// evaluates. For capability routing the predicted model is Target.UpstreamModel and
// the threshold is the target override; for legacy routing the predicted model comes
// from ResolveMappedModel per channel and the global threshold applies.
func profitCandidates(param *RetryParam, group string, result groupRoutingResult, filter model.ChannelSelectFilter) []ProfitRoutingCandidate {
	if result.Capability {
		candidates := make([]ProfitRoutingCandidate, 0, len(result.Evaluation.CompatibleByChannel))
		for channelID, target := range result.Evaluation.CompatibleByChannel {
			if !filter.Allows(channelID) {
				continue
			}
			variant := target.CostVariantKey
			if variant == "" {
				variant = string(types.DefaultCostVariantKey)
			}
			candidates = append(candidates, ProfitRoutingCandidate{
				ChannelID:              channelID,
				PredictedUpstreamModel: target.UpstreamModel,
				CostVariantKey:         variant,
				TargetThresholdBPS:     target.MinimumExpectedMarginBPS,
			})
		}
		return candidates
	}
	// Legacy path: no capability target, so resolve the mapped model for each enabled
	// channel of this group+model. A candidate with no resolvable model is still
	// included so the filter can record cost_rule_missing rather than silently dropping
	// it — but it will be excluded by FilterProfitEligibleChannels.
	channelIDs := model.GroupModelChannelIDs(group, param.ModelName, param.RequestPath, filter)
	candidates := make([]ProfitRoutingCandidate, 0, len(channelIDs))
	for _, channelID := range channelIDs {
		predictedModel := resolveLegacyPredictedModel(param, channelID)
		candidates = append(candidates, ProfitRoutingCandidate{
			ChannelID:              channelID,
			PredictedUpstreamModel: predictedModel,
			CostVariantKey:         string(types.DefaultCostVariantKey),
		})
	}
	return candidates
}

func profitCandidateKeys(candidates []ProfitRoutingCandidate) []CostRuleCandidate {
	keys := make([]CostRuleCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		keys = append(keys, CostRuleCandidate{
			ChannelID:             candidate.ChannelID,
			BillableUpstreamModel: candidate.PredictedUpstreamModel,
			CostVariantKey:        candidate.CostVariantKey,
		})
	}
	return keys
}

// profitFilterFacts resolves the request-level ProfitRoutingFacts and the predicted
// user revenue for the given group. Revenue is computed once per (group, request) so
// every candidate in the group is priced against the same revenue figure.
//
// factsErr is non-nil when the facts needed for meter/token estimation are incomplete
// (missing resolution or duration). The caller still receives the revenue figure so
// free/per-request cost candidates — which need no per-second or per-token meter — can
// be priced; per-duration/per-token candidates fail closed via meter_unknown when the
// facts are missing.
func profitFilterFacts(param *RetryParam, group string) (ProfitRoutingFacts, int64, bool, error) {
	facts := ProfitRoutingFacts{}
	resolution := ""
	duration := 0
	factsAvailable := false
	if param.RoutingInput != nil {
		if param.RoutingInput.OutputResolution != nil {
			resolution = *param.RoutingInput.OutputResolution
		}
		if param.RoutingInput.DurationSeconds != nil && *param.RoutingInput.DurationSeconds > 0 {
			duration = *param.RoutingInput.DurationSeconds
		}
	}
	if routingFacts, ok := common.GetContextKeyType[modelrouting.Facts](param.Ctx, constant.ContextKeyRoutingFacts); ok {
		if routingFacts.OutputResolution != "" {
			resolution = routingFacts.OutputResolution
		}
		if routingFacts.DurationSeconds > 0 {
			duration = routingFacts.DurationSeconds
		}
	}

	var factsErr error
	if resolution != "" && duration > 0 {
		inputDurationMS := int64(0)
		if state := param.ProfitRoutingState(); state != nil {
			metadata, err := state.Metadata(param.Ctx)
			if err == nil {
				inputDurationMS = metadata.TotalDurationMS
			}
		}
		estimated, err := EstimateProfitRoutingFacts(resolution, duration, inputDurationMS)
		if err != nil {
			factsErr = err
		} else {
			facts = estimated
			factsAvailable = true
		}
	} else {
		factsErr = fmt.Errorf("routing facts are incomplete")
	}

	durationSeconds := duration
	revenueNanoUSD, revenueErr := PreviewRoutingRevenue(param.Ctx, RoutingRevenuePreviewInput{
		OriginModelName: param.ModelName,
		Group:           group,
		RequestPath:     param.RequestPath,
		RelayMode:       common.GetContextKeyInt(param.Ctx, "relay_mode"),
		DurationSeconds: &durationSeconds,
		UserId:          common.GetContextKeyInt(param.Ctx, constant.ContextKeyUserId),
	})
	if revenueErr != nil {
		return ProfitRoutingFacts{}, 0, false, revenueErr
	}
	// factsAvailable is folded into the returned facts; when false the caller treats
	// per-duration/per-token candidates as meter_unknown while free/per-request still
	// price normally. Return factsErr so the caller can distinguish "no revenue" from
	// "facts incomplete".
	_ = factsAvailable
	return facts, revenueNanoUSD, true, factsErr
}

func resolveLegacyPredictedModel(param *RetryParam, channelID int) string {
	channel, err := model.GetChannelById(channelID, false)
	if err != nil || channel == nil {
		return ""
	}
	mappingJSON := strings.TrimSpace(channel.GetModelMapping())
	if mappingJSON == "" {
		return param.ModelName
	}
	originModel := param.ModelName
	if strings.HasSuffix(originModel, ratio_setting.CompactModelSuffix) {
		originModel = strings.TrimSuffix(originModel, ratio_setting.CompactModelSuffix)
	}
	mappedModel, _, err := ResolveMappedModel(originModel, mappingJSON)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(mappedModel)
}

func emptyAllowedFilter(filter model.ChannelSelectFilter) model.ChannelSelectFilter {
	return model.ChannelSelectFilter{
		AllowedChannelIDs:  map[int]struct{}{0: {}},
		ExcludedChannelIDs: filter.ExcludedChannelIDs,
	}
}

func recordProfitExclusions(param *RetryParam, result groupRoutingResult, filterResult ProfitChannelFilterResult) {
	if param == nil || len(filterResult.Exclusions) == 0 {
		return
	}
	if param.profitExclusions == nil {
		param.profitExclusions = make(map[int]ProfitExclusionReason, len(filterResult.Exclusions))
	}
	for _, exclusion := range filterResult.Exclusions {
		param.profitExclusions[exclusion.ChannelID] = exclusion.Reason
		if param.profitDiagnostics == nil {
			param.profitDiagnostics = make(map[int]ProfitRoutingDiagnostic, len(filterResult.Exclusions))
		}
		param.profitDiagnostics[exclusion.ChannelID] = ProfitRoutingDiagnostic{
			ChannelID:                exclusion.ChannelID,
			BillableUpstreamModel:    exclusion.UpstreamModel,
			EstimatedRevenueNanoUSD:  exclusion.EstimatedRevenueNanoUSD,
			EstimatedCostNanoUSD:     exclusion.EstimatedCostNanoUSD,
			EstimatedProfitNanoUSD:   exclusion.EstimatedProfitNanoUSD,
			GrossMarginPPM:           exclusion.GrossMarginPPM,
			MinimumExpectedMarginBPS: exclusion.MinimumExpectedMarginBPS,
			RuleID:                   exclusion.RuleID,
			RuleVersion:              exclusion.RuleVersion,
			Reason:                   exclusion.Reason,
		}
	}
	if param.Ctx != nil {
		diagnostics := make([]ProfitRoutingDiagnostic, 0, len(param.profitDiagnostics))
		for _, diagnostic := range param.profitDiagnostics {
			diagnostics = append(diagnostics, diagnostic)
		}
		common.SetContextKey(param.Ctx, constant.ContextKeyRoutingDiagnostics, diagnostics)
	}
	_ = result
}

// knownChannelPassesProfitFilter applies the same margin gate as applyProfitFilter but
// for a single caller-pinned channel. It returns false (excluding the channel) when
// strict mode is off OR the channel passes; under strict mode a failing pinned channel
// returns false so the controller surfaces a generic 503 instead of silently switching.
func knownChannelPassesProfitFilter(param *RetryParam, group string, result groupRoutingResult, channelID int) (bool, error) {
	if cost_setting.Runtime().Mode != types.CostAccountingStrict {
		return true, nil
	}
	if param == nil || param.Ctx == nil {
		return true, nil
	}
	predictedModel := ""
	threshold := (*int)(nil)
	variant := string(types.DefaultCostVariantKey)
	if result.Capability {
		if target, ok := result.Evaluation.CompatibleByChannel[channelID]; ok {
			predictedModel = target.UpstreamModel
			threshold = target.MinimumExpectedMarginBPS
			if target.CostVariantKey != "" {
				variant = target.CostVariantKey
			}
		}
	} else {
		predictedModel = resolveLegacyPredictedModel(param, channelID)
	}
	candidate := ProfitRoutingCandidate{ChannelID: channelID, PredictedUpstreamModel: predictedModel, CostVariantKey: variant, TargetThresholdBPS: threshold}

	facts, revenueNanoUSD, hasRevenue, _ := profitFilterFacts(param, group)
	rules, err := ActiveCostRules(profitCandidateKeys([]ProfitRoutingCandidate{candidate}), false)
	if err != nil {
		common.SysError(fmt.Sprintf("profit routing known-channel active rule failed: %s", err.Error()))
		return false, nil
	}
	filterResult := FilterProfitEligibleChannels(ProfitChannelFilterInput{
		Ctx:             param.Ctx,
		Facts:           facts,
		RevenueNanoUSD:  revenueNanoUSD,
		HasRevenue:      hasRevenue,
		GlobalMarginBPS: cost_setting.Runtime().MinimumExpectedMarginBPS,
		Candidates:      []ProfitRoutingCandidate{candidate},
		MetadataState:   param.ProfitRoutingState(),
	}, rules)
	if filterResult.InvalidMedia {
		return false, &ChannelSelectionError{
			Code:       types.ErrorCodeInvalidRequest,
			StatusCode: http.StatusBadRequest,
			Err:        errors.New("input video is not supported"),
		}
	}
	recordProfitExclusions(param, result, filterResult)
	_, allowed := filterResult.AllowedChannelIDs[channelID]
	return allowed, nil
}

func selectChannelForGroup(param *RetryParam, group string, priorityRetry int) (*model.Channel, groupRoutingResult, error) {
	result, err := evaluateGroupRouting(group, param.ModelName, param.RoutingInput)
	if err != nil {
		return nil, result, err
	}
	filter := model.ChannelSelectFilter{}
	filter.ExcludedChannelIDs = param.ExcludedChannelIDs
	if result.Capability {
		filter.AllowedChannelIDs = make(map[int]struct{}, len(result.Evaluation.CompatibleByChannel))
		for channelID := range result.Evaluation.CompatibleByChannel {
			filter.AllowedChannelIDs[channelID] = struct{}{}
		}
	}
	// Profit-aware candidate filter: under strict cost-accounting mode, narrow the
	// candidate set to channels whose predicted margin meets the minimum threshold.
	// The filter only intersects the set; it never reorders or changes priority/weight,
	// so GetRandomSatisfiedChannel below still applies the original selection semantics.
	filter, err = applyProfitFilter(param, group, result, filter)
	if err != nil {
		return nil, result, err
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
	} else {
		publishLegacyRoutingCostVariant(param.Ctx)
	}
	return channel, result, nil
}

func ValidateKnownChannelForRouting(param *RetryParam, group string, channelID int) (bool, error) {
	clearRoutingDecision(param.Ctx)
	result, err := evaluateGroupRouting(group, param.ModelName, param.RoutingInput)
	if err != nil {
		return false, err
	}
	if _, excluded := param.ExcludedChannelIDs[channelID]; excluded {
		if !result.Capability {
			return false, &ChannelSelectionError{
				Code: types.ErrorCodeCompatibleChannelUnavailable, StatusCode: http.StatusServiceUnavailable,
				Err: errors.New("channel is unavailable"),
			}
		}
		return false, &ChannelSelectionError{
			Code: types.ErrorCodeCompatibleChannelUnavailable, StatusCode: http.StatusServiceUnavailable,
			Err: errors.New("compatible channel is unavailable"),
			Diagnostics: []modelrouting.Audit{{
				PolicyID: result.Snapshot.ID, Facts: result.Facts, MismatchCounts: result.Evaluation.MismatchCounts,
			}},
		}
	}
	if result.Capability && !model.IsChannelEnabledForGroupModel(group, param.ModelName, channelID) {
		return false, &ChannelSelectionError{
			Code: types.ErrorCodeCompatibleChannelUnavailable, StatusCode: http.StatusServiceUnavailable,
			Err: errors.New("compatible channel is unavailable"),
			Diagnostics: []modelrouting.Audit{{
				PolicyID: result.Snapshot.ID, Facts: result.Facts, MismatchCounts: result.Evaluation.MismatchCounts,
			}},
		}
	}
	if result.Capability {
		target, compatible := result.Evaluation.CompatibleByChannel[channelID]
		if !compatible {
			return false, nil
		}
		publishRoutingDecision(param.Ctx, result, target)
	} else {
		publishLegacyRoutingCostVariant(param.Ctx)
	}

	channel, err := model.GetChannelById(channelID, false)
	if err != nil {
		return false, &ChannelSelectionError{
			Code: types.ErrorCodeCompatibleChannelUnavailable, StatusCode: http.StatusServiceUnavailable,
			Err: errors.New("channel is unavailable"),
		}
	}
	// Profit-aware gate for a caller-pinned channel. Under strict mode, a pinned channel
	// that fails the margin threshold must NOT silently switch to another channel: it is
	// excluded here and the controller returns a generic 503, matching the cost-coverage
	// path below. The same applyProfitFilter primitive as the random path is reused.
	profitEligible, profitErr := knownChannelPassesProfitFilter(param, group, result, channelID)
	if profitErr != nil {
		return false, profitErr
	}
	if !profitEligible {
		param.ExcludeChannel(channelID)
		selectionErr := &ChannelSelectionError{
			Code: types.ErrorCodeCompatibleChannelUnavailable, StatusCode: http.StatusServiceUnavailable,
			Err: errors.New("channel is unavailable"),
		}
		if result.Capability {
			selectionErr.Err = errors.New("compatible channel is unavailable")
			selectionErr.Diagnostics = []modelrouting.Audit{{
				PolicyID: result.Snapshot.ID, Facts: result.Facts, MismatchCounts: result.Evaluation.MismatchCounts,
			}}
		}
		return false, selectionErr
	}
	covered, coverageErr := CheckSelectedChannelCostCoverage(param, channel, "")
	if coverageErr != nil || !covered {
		param.ExcludeChannel(channelID)
		selectionErr := &ChannelSelectionError{
			Code: types.ErrorCodeCompatibleChannelUnavailable, StatusCode: http.StatusServiceUnavailable,
			Err: errors.New("channel is unavailable"),
		}
		if result.Capability {
			selectionErr.Err = errors.New("compatible channel is unavailable")
			selectionErr.Diagnostics = []modelrouting.Audit{{
				PolicyID: result.Snapshot.ID, Facts: result.Facts, MismatchCounts: result.Evaluation.MismatchCounts,
			}}
		}
		return false, selectionErr
	}
	return true, nil
}

func CheckSelectedChannelCostCoverage(param *RetryParam, channel *model.Channel, taskPlatform constant.TaskPlatform) (bool, error) {
	if cost_setting.Runtime().Mode != types.CostAccountingStrict {
		return true, nil
	}
	if param == nil || channel == nil {
		return false, errors.New("selected channel is required")
	}

	predictedModel := ""
	if common.GetContextKeyBool(param.Ctx, constant.ContextKeyRoutingCapabilityMode) {
		predictedModel = strings.TrimSpace(common.GetContextKeyString(param.Ctx, constant.ContextKeyRoutingUpstreamModel))
	}
	if predictedModel == "" {
		mappingJSON := strings.TrimSpace(channel.GetModelMapping())
		if mappingJSON == "" {
			mappingJSON = "{}"
		}
		originModel := param.ModelName
		if strings.HasSuffix(originModel, ratio_setting.CompactModelSuffix) {
			originModel = strings.TrimSuffix(originModel, ratio_setting.CompactModelSuffix)
		}
		mappedModel, _, err := ResolveMappedModel(originModel, mappingJSON)
		if err != nil {
			return false, nil
		}
		predictedModel = strings.TrimSpace(mappedModel)
	}
	if predictedModel == "" {
		return false, nil
	}

	input := PredictedCoverageInput{
		ChannelID:              channel.Id,
		PredictedUpstreamModel: predictedModel,
		CostVariantKey:         selectedCostVariantKey(param.Ctx),
		RequestPath:            param.RequestPath,
		TaskPlatform:           taskPlatform,
	}
	if input.TaskPlatform == "" && isCostTaskRequestPath(param.RequestPath) {
		input.TaskPlatform = constant.TaskPlatform(strconv.Itoa(channel.Type))
	}
	covered, err := CheckPredictedCostCoverage(input)
	if err != nil || covered {
		return covered, err
	}
	if param.costCoverageMisses == nil {
		param.costCoverageMisses = make(map[int]PredictedCoverageInput)
	}
	param.costCoverageMisses[channel.Id] = input
	return false, nil
}

func isCostTaskRequestPath(requestPath string) bool {
	path := strings.ToLower(strings.TrimSpace(requestPath))
	return strings.Contains(path, "/video") || strings.Contains(path, "/suno/") ||
		strings.Contains(path, "/kling/") || strings.Contains(path, "/jimeng") ||
		strings.Contains(path, "/contents/generations")
}

func RecheckCostCoverageMisses(param *RetryParam) (bool, error) {
	if cost_setting.Runtime().Mode != types.CostAccountingStrict || param == nil || len(param.costCoverageMisses) == 0 {
		return false, nil
	}

	restored := false
	for channelID, input := range param.costCoverageMisses {
		input.Authoritative = true
		covered, err := CheckPredictedCostCoverage(input)
		if err != nil {
			return false, err
		}
		if !covered {
			continue
		}
		delete(param.ExcludedChannelIDs, channelID)
		delete(param.costCoverageMisses, channelID)
		restored = true
	}
	return restored, nil
}

func publishRoutingDecision(c *gin.Context, result groupRoutingResult, target modelrouting.Target) {
	variant, err := types.NormalizeCostVariantKey(target.CostVariantKey)
	if err != nil {
		variant = string(types.DefaultCostVariantKey)
	}
	common.SetContextKey(c, constant.ContextKeyRoutingCapabilityMode, true)
	common.SetContextKey(c, constant.ContextKeyRoutingPolicyID, result.Snapshot.ID)
	common.SetContextKey(c, constant.ContextKeyRoutingTargetID, target.ID)
	common.SetContextKey(c, constant.ContextKeyRoutingTargetName, target.Name)
	common.SetContextKey(c, constant.ContextKeyRoutingUpstreamModel, target.UpstreamModel)
	common.SetContextKey(c, constant.ContextKeyRoutingCostVariant, variant)
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
	delete(c.Keys, string(constant.ContextKeyRoutingCostVariant))
	common.SetContextKey(c, constant.ContextKeyRoutingFacts, modelrouting.Facts{})
	common.SetContextKey(c, constant.ContextKeyRoutingMismatchCounts, map[modelrouting.MismatchReason]int{})
}

func publishLegacyRoutingCostVariant(c *gin.Context) {
	if c == nil {
		return
	}
	common.SetContextKey(c, constant.ContextKeyRoutingCostVariant, string(types.DefaultCostVariantKey))
}

// selectedCostVariantKey reads the cost variant identity the routing layer
// published for the current request. Capability routing publishes the variant
// bound to the matched target; legacy routing explicitly publishes default so
// existing single-contract channels keep resolving the same active cost rule.
func selectedCostVariantKey(c *gin.Context) string {
	if c == nil {
		return string(types.DefaultCostVariantKey)
	}
	variant, _ := common.GetContextKeyType[string](c, constant.ContextKeyRoutingCostVariant)
	normalized, err := types.NormalizeCostVariantKey(variant)
	if err != nil {
		return string(types.DefaultCostVariantKey)
	}
	return normalized
}

func RecordRoutingSelectionFailure(c *gin.Context, canonicalModel string, selectionErr *ChannelSelectionError) {
	if c == nil || selectionErr == nil || !constant.ErrorLogEnabled {
		return
	}
	other := map[string]interface{}{
		"error_type":  string(types.ErrorTypeNewAPIError),
		"error_code":  selectionErr.Code,
		"status_code": selectionErr.StatusCode,
	}
	if len(selectionErr.Diagnostics) > 0 {
		other["admin_info"] = map[string]interface{}{
			"routing_selection": selectionErr.Diagnostics,
		}
	}
	AppendRoutingAdminInfoFromContext(c, other)
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
