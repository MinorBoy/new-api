package service

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelrouting"
	"github.com/QuantumNous/new-api/pkg/seedancepricing"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/cost_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

// ProfitExclusionReason is the stable reason code recorded in admin diagnostics when a
// candidate is excluded from selection. The string values are part of the admin-only
// diagnostics contract and must never change.
type ProfitExclusionReason string

const (
	ProfitReasonRevenueUnknown       ProfitExclusionReason = "revenue_unknown"
	ProfitReasonCostRuleMissing      ProfitExclusionReason = "cost_rule_missing"
	ProfitReasonMeterUnknown         ProfitExclusionReason = "meter_unknown"
	ProfitReasonMarginBelowThreshold ProfitExclusionReason = "margin_below_threshold"
	ProfitReasonCalculationError     ProfitExclusionReason = "calculation_error"
	ProfitReasonMetadataUnavailable  ProfitExclusionReason = "metadata_unavailable"
)

// ProfitRoutingFacts is the request-level snapshot of the values the profit predictor
// needs. Pixel dimensions and frame rate come from pkg/seedancepricing; the input
// duration is the aggregate from the standalone metadata service; the output duration
// is the validated request value. Tokens are populated by EstimateSeedanceTokens.
type ProfitRoutingFacts struct {
	OutputDurationSeconds int
	InputDurationMS       int64
	Width                 int
	Height                int
	FrameRateNum          int64
	FrameRateDen          int64
	InputTokens           int64
	OutputTokens          int64
	TotalTokens           int64
}

// ProfitRoutingInput is the amount pair plus threshold that the pure eligibility
// comparison consumes. Splitting it from cost-rule resolution keeps the comparison
// testable in isolation.
type ProfitRoutingInput struct {
	RevenueNanoUSD int64
	CostNanoUSD    int64
	ThresholdBPS   int
	RuleID         int64
	RuleVersion    int
}

// ProfitEligibilityResult is the outcome of a single candidate's profit evaluation.
// Amounts, the threshold and the rule identity are admin-only diagnostics and never
// reach ordinary users.
type ProfitEligibilityResult struct {
	Eligible       bool
	Reason         ProfitExclusionReason
	RevenueNanoUSD int64
	CostNanoUSD    int64
	ProfitNanoUSD  int64
	MarginPPM      *int64
	ThresholdBPS   int
	RuleID         int64
	RuleVersion    int
}

// ProfitRoutingDiagnostic is the admin-only explanation for one candidate that the
// margin gate excluded. It deliberately contains only pricing and rule identity data;
// request URLs, tokens, query parameters, and media contents never leave request
// memory through this structure.
type ProfitRoutingDiagnostic struct {
	ChannelID                int                   `json:"channel_id"`
	BillableUpstreamModel    string                `json:"billable_upstream_model"`
	EstimatedRevenueNanoUSD  int64                 `json:"estimated_revenue_nano_usd"`
	EstimatedCostNanoUSD     *int64                `json:"estimated_cost_nano_usd,omitempty"`
	EstimatedProfitNanoUSD   *int64                `json:"estimated_profit_nano_usd,omitempty"`
	GrossMarginPPM           *int64                `json:"gross_margin_ppm,omitempty"`
	MinimumExpectedMarginBPS int                   `json:"minimum_expected_margin_bps"`
	RuleID                   int64                 `json:"rule_id"`
	RuleVersion              int                   `json:"rule_version"`
	Reason                   ProfitExclusionReason `json:"reason"`
}

// tokenEstimateScale encodes the Seedance token formula:
//
//	tokens = duration_ms * width * height * frame_rate / 1024 / 1000
//
// 1024 normalizes pixels-per-frame to the provider's token unit; 1000 converts ms→s
// without a floating-point division.
var (
	tokenPixelDivisor    = decimal.NewFromInt(1024)
	tokenDurationDivisor = decimal.NewFromInt(1000)
)

// EstimateSeedanceTokens computes the predicted input, output and total token counts
// for a Seedance video request. Input uses the aggregated input-video duration;
// output uses the validated requested duration. The intermediate math uses Decimal,
// and each meter that becomes a billing multiplier is ceil-rounded independently.
//
// total_tokens is computed as ceil of the exact (input + output) Decimal sum, NOT by
// adding the already-rounded input and output values — adding two ceils can under-
// estimate the true sum by up to 2 tokens and thus under-price the channel.
//
// Any result that would exceed relaycommon.MaxTokensLimit fails closed (returning an
// error) so the caller excludes the candidate with meter_unknown rather than letting
// an overflowed or saturated count reach the billing path.
func EstimateSeedanceTokens(facts ProfitRoutingFacts) (inputTokens, outputTokens, totalTokens int64, err error) {
	if facts.Width <= 0 || facts.Height <= 0 || facts.FrameRateNum <= 0 || facts.FrameRateDen <= 0 {
		return 0, 0, 0, fmt.Errorf("profit routing facts are missing resolution or frame rate")
	}
	pixels := decimal.NewFromInt(int64(facts.Width)).Mul(decimal.NewFromInt(int64(facts.Height)))
	frameRate := decimal.NewFromInt(facts.FrameRateNum).Div(decimal.NewFromInt(facts.FrameRateDen))

	inputRaw := decimal.NewFromInt(facts.InputDurationMS).Mul(pixels).Mul(frameRate).Div(tokenPixelDivisor).Div(tokenDurationDivisor)
	outputRaw := decimal.NewFromInt(int64(facts.OutputDurationSeconds) * 1000).Mul(pixels).Mul(frameRate).Div(tokenPixelDivisor).Div(tokenDurationDivisor)

	inputCeil, inputErr := boundedCeilTokens(inputRaw)
	outputCeil, outputErr := boundedCeilTokens(outputRaw)
	if inputErr != nil || outputErr != nil {
		return 0, 0, 0, fmt.Errorf("seedance token estimate is out of range")
	}
	totalRaw := inputRaw.Add(outputRaw)
	totalCeil, totalErr := boundedCeilTokens(totalRaw)
	if totalErr != nil {
		return 0, 0, 0, fmt.Errorf("seedance token estimate is out of range")
	}
	return inputCeil, outputCeil, totalCeil, nil
}

// boundedCeilTokens ceil-rounds a Decimal token count and rejects anything beyond
// MaxTokensLimit, mirroring the relay's own token bound so predicted and actual
// settlement share the same ceiling.
func boundedCeilTokens(value decimal.Decimal) (int64, error) {
	ceiled := value.Ceil()
	if ceiled.GreaterThan(decimal.NewFromInt(relayMaxTokensLimit())) {
		return 0, fmt.Errorf("token count exceeds MaxTokensLimit")
	}
	if ceiled.LessThan(decimal.Zero) {
		return 0, fmt.Errorf("token count cannot be negative")
	}
	return ceiled.IntPart(), nil
}

// relayMaxTokensLimit exposes relaycommon.MaxTokensLimit without creating an import
// cycle: the constant is MaxInt32/2 and is stable. Keeping it as a function lets a
// future refactor thread the value explicitly without touching call sites.
func relayMaxTokensLimit() int64 {
	return int64(math.MaxInt32 / 2)
}

// BuildProfitCostMeter assembles the CostMeter the predictor hands to
// CalculateAttemptCost. Per-request carries no quantity; per-duration uses the
// validated requested seconds; per-token populates every token field from the
// Seedance estimate so all three token sub-modes price correctly without the caller
// having to know which one the active rule selected.
func BuildProfitCostMeter(mode types.CostMode, facts ProfitRoutingFacts) (types.CostMeter, error) {
	meter := types.CostMeter{Source: types.CostMeterValidatedRequest}
	switch mode {
	case types.CostModeFree, types.CostModePerRequest:
		return meter, nil
	case types.CostModePerDuration:
		if facts.OutputDurationSeconds <= 0 {
			return types.CostMeter{}, fmt.Errorf("per-duration cost requires a positive output duration")
		}
		seconds := fmt.Sprintf("%d", facts.OutputDurationSeconds)
		meter.DurationSeconds = &seconds
		return meter, nil
	case types.CostModePerToken:
		if facts.TotalTokens == 0 && facts.OutputTokens == 0 && facts.InputTokens == 0 {
			return types.CostMeter{}, fmt.Errorf("per-token cost requires a token estimate")
		}
		inputTokens := facts.InputTokens
		outputTokens := facts.OutputTokens
		completionTokens := facts.OutputTokens
		totalTokens := facts.TotalTokens
		meter.InputTokens = &inputTokens
		meter.OutputTokens = &outputTokens
		meter.CompletionTokens = &completionTokens
		meter.TotalTokens = &totalTokens
		return meter, nil
	default:
		return types.CostMeter{}, fmt.Errorf("unsupported cost mode %q", mode)
	}
}

// EvaluateProfitEligibility is the pure amount comparison. It reuses the same
// overflow-checked subtract and PPM helpers as the live cost preview, and admits a
// candidate only when revenue is known and the margin in PPM is at least the
// threshold (converted exactly via multiply-by-100: 1 BPS = 100 PPM). Equality is an
// admission (>=), matching the design's "minimum expected margin" contract.
func EvaluateProfitEligibility(input ProfitRoutingInput) ProfitEligibilityResult {
	result := ProfitEligibilityResult{
		RevenueNanoUSD: input.RevenueNanoUSD,
		CostNanoUSD:    input.CostNanoUSD,
		ThresholdBPS:   input.ThresholdBPS,
		RuleID:         input.RuleID,
		RuleVersion:    input.RuleVersion,
	}
	if input.RevenueNanoUSD <= 0 {
		result.Reason = ProfitReasonRevenueUnknown
		return result
	}
	profitNanoUSD, err := CheckedNanoSubtract(input.RevenueNanoUSD, input.CostNanoUSD)
	if err != nil {
		result.Reason = ProfitReasonCalculationError
		return result
	}
	result.ProfitNanoUSD = profitNanoUSD
	marginPPM, err := GrossMarginPPM(profitNanoUSD, input.RevenueNanoUSD)
	if err != nil || marginPPM == nil {
		result.Reason = ProfitReasonCalculationError
		return result
	}
	result.MarginPPM = marginPPM
	// 1 BPS = 1/100 of a percent = 100 PPM. Exact integer multiply, no float.
	thresholdPPM := int64(input.ThresholdBPS) * 100
	if *marginPPM < thresholdPPM {
		result.Reason = ProfitReasonMarginBelowThreshold
		return result
	}
	result.Eligible = true
	return result
}

// CalculateTaskTokenQuota is the pure core of asynchronous token settlement. Both the
// async RecalculateTaskQuotaByTokens path and the profit predictor call it so the
// predicted cost and the eventual actual charge share one formula and one saturation
// helper — they cannot drift apart.
//
// The formula is totalTokens * modelRatio * groupRatio * otherMultiplier, in quota
// units (modelRatio is already "quota per token", so QuotaPerUnit is NOT applied
// here). All inputs are validated for finiteness and non-negativity; the product is
// saturated to int32 via common.QuotaFromFloatChecked so an overflow can never wrap to
// a negative charge.
func CalculateTaskTokenQuota(totalTokens int64, modelRatio, groupRatio, otherMultiplier float64) (int, *common.QuotaClamp, error) {
	if totalTokens < 0 {
		return 0, nil, fmt.Errorf("total tokens cannot be negative")
	}
	if math.IsNaN(modelRatio) || math.IsInf(modelRatio, 0) || modelRatio < 0 {
		return 0, nil, fmt.Errorf("model ratio must be a non-negative finite number")
	}
	if math.IsNaN(groupRatio) || math.IsInf(groupRatio, 0) || groupRatio < 0 {
		return 0, nil, fmt.Errorf("group ratio must be a non-negative finite number")
	}
	if math.IsNaN(otherMultiplier) || math.IsInf(otherMultiplier, 0) || otherMultiplier < 0 {
		return 0, nil, fmt.Errorf("other multiplier must be a non-negative finite number")
	}
	quota, clamp := common.QuotaFromFloatChecked(float64(totalTokens) * modelRatio * groupRatio * otherMultiplier)
	return quota, clamp, nil
}

// RoutingRevenuePreviewInput carries the request facts the revenue callback needs to
// reproduce the official user price for the same request. The origin model, effective
// group and validated output duration are the only inputs: the callback must never be
// given the channel's mapped upstream model, the supplier cost, or any URL, because
// user revenue is defined purely by the client-facing model and group ratio.
type RoutingRevenuePreviewInput struct {
	// OriginModelName is the client-facing model name used to look up the official
	// model ratio/price — never the channel-mapped upstream model.
	OriginModelName string
	// Group is the effective user group for this routing evaluation (auto groups may
	// re-evaluate revenue per group).
	Group string
	// RequestPath identifies the relay path so the callback selects the right pricing
	// mode (per-call video/image vs token-based text).
	RequestPath string
	// RelayMode is the relay mode constant used by ModelPriceHelperPerCall to decide
	// between per-call and per-token pricing.
	RelayMode int
	// DurationSeconds is the validated requested output duration, used by per-duration
	// billing models. Zero/nil means "let the helper apply its own default".
	DurationSeconds *int
	// UserId carries the requesting user so the helper can resolve per-user settings.
	UserId int
}

// RoutingRevenuePreviewFunc previews the final user quota (and the QuotaPerUnit
// snapshot) for a request, reusing the same ModelPriceHelper + PreviewFinalUserQuota
// chain as live billing. It is injected from main.go to avoid a service → relay/helper
// import cycle. When unset, revenue is treated as unknown and every candidate is
// excluded (fail-closed).
type RoutingRevenuePreviewFunc func(ctx context.Context, input RoutingRevenuePreviewInput) (finalUserQuota int64, quotaPerUnitSnapshot string, err error)

var routingRevenuePreviewHolder = struct {
	mu   sync.RWMutex
	hook RoutingRevenuePreviewFunc
}{}

// SetRoutingRevenuePreview installs the process-wide revenue preview callback. main.go
// calls this once during startup with relay/helper.PreviewRoutingRevenue. Passing nil
// restores the fail-closed default (revenue unknown → all candidates excluded).
func SetRoutingRevenuePreview(hook RoutingRevenuePreviewFunc) {
	routingRevenuePreviewHolder.mu.Lock()
	defer routingRevenuePreviewHolder.mu.Unlock()
	routingRevenuePreviewHolder.hook = hook
}

// RevenuePreviewHookForTest returns the currently installed revenue preview callback.
// It exists so tests can save and restore the hook around strict-mode fixtures without
// reaching into package-private state.
func RevenuePreviewHookForTest() RoutingRevenuePreviewFunc {
	routingRevenuePreviewHolder.mu.RLock()
	defer routingRevenuePreviewHolder.mu.RUnlock()
	return routingRevenuePreviewHolder.hook
}

// currentRoutingRevenuePreview returns the installed callback or nil.
func currentRoutingRevenuePreview() RoutingRevenuePreviewFunc {
	routingRevenuePreviewHolder.mu.RLock()
	defer routingRevenuePreviewHolder.mu.RUnlock()
	return routingRevenuePreviewHolder.hook
}

// PreviewRoutingRevenue calls the injected callback and converts the resulting user
// quota into revenue nano-USD via RevenueEquivalentNanoUSD. A nil callback or callback
// error yields (0, error) so the caller excludes the candidate as revenue_unknown
// rather than treating the absence as free revenue.
func PreviewRoutingRevenue(ctx context.Context, input RoutingRevenuePreviewInput) (int64, error) {
	hook := currentRoutingRevenuePreview()
	if hook == nil {
		return 0, fmt.Errorf("routing revenue preview is not configured")
	}
	finalUserQuota, quotaPerUnitSnapshot, err := hook(ctx, input)
	if err != nil {
		return 0, err
	}
	return RevenueEquivalentNanoUSD(finalUserQuota, quotaPerUnitSnapshot)
}

// ProfitRoutingCandidate describes one channel the candidate stage is considering. The
// predicted billable upstream model is the model the adapter will actually send
// upstream after model mapping — either the capability-routing Target.UpstreamModel
// or the legacy ResolveMappedModel result. An empty predicted model excludes the
// candidate as cost_rule_missing (we cannot price what we cannot identify).
type ProfitRoutingCandidate struct {
	ChannelID              int
	PredictedUpstreamModel string
	// TargetThresholdBPS is the route-target override (nil = inherit the global threshold).
	TargetThresholdBPS *int
}

// ProfitChannelFilterInput carries everything the candidate filter needs: the request
// facts (already metadata-resolved), the global threshold, the per-candidate threshold
// overrides, and the revenue for this group's request. The revenue is computed once
// per (group, request) and reused across every candidate in that group.
type ProfitChannelFilterInput struct {
	Ctx             context.Context
	Facts           ProfitRoutingFacts
	RevenueNanoUSD  int64
	HasRevenue      bool
	GlobalMarginBPS int
	Candidates      []ProfitRoutingCandidate
	// MetadataState, when non-nil, is asked to resolve input reference video durations
	// before any token-priced candidate is evaluated. It is nil for requests without
	// reference videos.
	MetadataState *ProfitRoutingRequestState
}

// ProfitChannelFilterResult is the filtered candidate set plus admin-only diagnostics
// for each excluded candidate. Surviving channel IDs are written into
// ChannelSelectFilter.AllowedChannelIDs by the caller; the diagnostics feed the
// routing_diagnostics admin log and never reach ordinary users.
type ProfitChannelFilterResult struct {
	AllowedChannelIDs map[int]struct{}
	Exclusions        []ProfitChannelExclusion
}

type ProfitChannelExclusion struct {
	ChannelID                int
	UpstreamModel            string
	EstimatedRevenueNanoUSD  int64
	EstimatedCostNanoUSD     *int64
	EstimatedProfitNanoUSD   *int64
	GrossMarginPPM           *int64
	MinimumExpectedMarginBPS int
	RuleID                   int64
	RuleVersion              int
	Reason                   ProfitExclusionReason
}

// FilterProfitEligibleChannels evaluates each candidate's predicted margin and returns
// the survivors. It is the single entry point the normal, auto, affinity, specified
// and retry paths must all call, so none of them can bypass the minimum-margin gate.
//
// The filter never reorders candidates or changes priority/weight: it only intersects
// the candidate set with the margin survivors, and GetRandomSatisfiedChannel still
// performs the original weighted random pick on the result. When the cost-accounting
// mode is not strict, the filter is a no-op and returns every candidate (the caller
// decides strictness so this function stays pure and testable).
//
// Fail-closed semantics: unknown revenue, a missing active cost rule, an unknown meter
// (e.g. token cost without a resolvable duration), a metadata-service failure that
// blocks token prediction, or any calculation overflow all exclude the candidate.
func FilterProfitEligibleChannels(input ProfitChannelFilterInput, rules map[CostRuleCandidate]*model.ChannelModelCostRule) ProfitChannelFilterResult {
	result := ProfitChannelFilterResult{
		AllowedChannelIDs: make(map[int]struct{}, len(input.Candidates)),
	}
	if len(input.Candidates) == 0 {
		return result
	}

	// Resolve input reference video metadata once for the whole filter pass, but only
	// if at least one candidate needs it (per-token cost). A failure here is recorded
	// per-candidate as metadata_unavailable so token-priced candidates are excluded
	// while per-request/per-duration/free candidates keep working.
	var metadataDurationMS int64
	var metadataResolved bool
	var metadataErr error
	needsMetadata := false
	for _, candidate := range input.Candidates {
		rule := rules[CostRuleCandidate{ChannelID: candidate.ChannelID, BillableUpstreamModel: candidate.PredictedUpstreamModel}]
		if rule != nil && types.CostMode(rule.CostMode) == types.CostModePerToken && input.Facts.InputDurationMS <= 0 && input.MetadataState != nil {
			needsMetadata = true
			break
		}
	}
	if needsMetadata && input.MetadataState != nil {
		metadata, err := input.MetadataState.Metadata(input.Ctx)
		if err != nil {
			metadataErr = err
		} else {
			metadataDurationMS = metadata.TotalDurationMS
			metadataResolved = true
		}
	}

	for _, candidate := range input.Candidates {
		exclusion := evaluateCandidateProfit(input, rules, candidate, metadataDurationMS, metadataResolved, metadataErr)
		if exclusion.Reason != "" {
			result.Exclusions = append(result.Exclusions, exclusion)
			continue
		}
		result.AllowedChannelIDs[candidate.ChannelID] = struct{}{}
	}
	return result
}

func evaluateCandidateProfit(
	input ProfitChannelFilterInput,
	rules map[CostRuleCandidate]*model.ChannelModelCostRule,
	candidate ProfitRoutingCandidate,
	metadataDurationMS int64,
	metadataResolved bool,
	metadataErr error,
) ProfitChannelExclusion {
	threshold := input.GlobalMarginBPS
	if candidate.TargetThresholdBPS != nil {
		threshold = *candidate.TargetThresholdBPS
	}
	exclusion := ProfitChannelExclusion{
		ChannelID:                candidate.ChannelID,
		UpstreamModel:            candidate.PredictedUpstreamModel,
		EstimatedRevenueNanoUSD:  input.RevenueNanoUSD,
		MinimumExpectedMarginBPS: threshold,
	}
	if !input.HasRevenue || input.RevenueNanoUSD <= 0 {
		exclusion.Reason = ProfitReasonRevenueUnknown
		return exclusion
	}
	predictedModel := strings.TrimSpace(candidate.PredictedUpstreamModel)
	if predictedModel == "" {
		exclusion.Reason = ProfitReasonCostRuleMissing
		return exclusion
	}
	rule := rules[CostRuleCandidate{ChannelID: candidate.ChannelID, BillableUpstreamModel: predictedModel}]
	if rule == nil {
		exclusion.Reason = ProfitReasonCostRuleMissing
		return exclusion
	}
	exclusion.RuleID = rule.ID
	exclusion.RuleVersion = rule.Version

	facts := input.Facts
	if metadataResolved {
		facts.InputDurationMS = metadataDurationMS
		inputTokens, outputTokens, totalTokens, err := EstimateSeedanceTokens(facts)
		if err != nil {
			exclusion.Reason = ProfitReasonMeterUnknown
			return exclusion
		}
		facts.InputTokens = inputTokens
		facts.OutputTokens = outputTokens
		facts.TotalTokens = totalTokens
	}
	meter, err := BuildProfitCostMeter(types.CostMode(rule.CostMode), facts)
	if err != nil {
		exclusion.Reason = ProfitReasonMeterUnknown
		return exclusion
	}
	// A metadata state means the request carries reference videos. Their unknown
	// duration cannot safely be priced at zero tokens; no state means there is no input
	// video, so zero input tokens and the estimated output tokens are valid.
	if types.CostMode(rule.CostMode) == types.CostModePerToken && input.MetadataState != nil && facts.InputDurationMS <= 0 {
		if metadataErr != nil {
			exclusion.Reason = ProfitReasonMetadataUnavailable
			return exclusion
		}
		exclusion.Reason = ProfitReasonMeterUnknown
		return exclusion
	}

	config, err := parseCostRuleConfigForProfit(rule)
	if err != nil {
		exclusion.Reason = ProfitReasonCalculationError
		return exclusion
	}
	_, costNanoUSD, err := CalculateAttemptCost(types.CostMode(rule.CostMode), config, meter)
	if err != nil {
		exclusion.Reason = ProfitReasonCalculationError
		return exclusion
	}
	exclusion.EstimatedCostNanoUSD = &costNanoUSD

	eligibility := EvaluateProfitEligibility(ProfitRoutingInput{
		RevenueNanoUSD: input.RevenueNanoUSD,
		CostNanoUSD:    costNanoUSD,
		ThresholdBPS:   threshold,
		RuleID:         rule.ID,
		RuleVersion:    rule.Version,
	})
	if eligibility.Reason != ProfitReasonCalculationError {
		profitNanoUSD := eligibility.ProfitNanoUSD
		exclusion.EstimatedProfitNanoUSD = &profitNanoUSD
		exclusion.GrossMarginPPM = eligibility.MarginPPM
	}
	if !eligibility.Eligible {
		exclusion.Reason = eligibility.Reason
		return exclusion
	}
	return exclusion
}

// parseCostRuleConfigForProfit reuses validateCostRuleContract to parse and normalize
// a stored rule's ConfigJSON. The profit gate only needs price fields; the selected
// channel's actual capability contract is enforced by coverage and dispatch before
// an upstream request can be sent.
func parseCostRuleConfigForProfit(rule *model.ChannelModelCostRule) (types.CostRuleConfigV1, error) {
	if rule == nil {
		return types.CostRuleConfigV1{}, fmt.Errorf("cost rule is nil")
	}
	return validateCostRuleContract(rule, types.CostCapabilities{
		CanResolveBillableModel: true,
		ChargeEvents: []types.CostChargeEvent{
			types.CostChargeResponseSucceeded,
			types.CostChargeSubmitAccepted,
			types.CostChargeTaskSucceeded,
		},
		MeterSources: []types.CostMeterSource{
			types.CostMeterValidatedRequest,
			types.CostMeterUpstreamActual,
			types.CostMeterUpstreamUsage,
			types.CostMeterLocalUsage,
		},
	})
}

// EstimateProfitRoutingFacts builds the ProfitRoutingFacts the filter consumes from the
// routing-layer Facts plus the resolved metadata duration. Width/height/frame-rate
// come from the shared seedancepricing profile so the predictor and the billing
// adapter agree on dimensions.
func EstimateProfitRoutingFacts(resolution string, outputDurationSeconds int, inputDurationMS int64) (ProfitRoutingFacts, error) {
	profile, ok := seedancepricing.Profile(resolution)
	if !ok {
		return ProfitRoutingFacts{}, fmt.Errorf("unsupported output resolution %q", resolution)
	}
	facts := ProfitRoutingFacts{
		OutputDurationSeconds: outputDurationSeconds,
		InputDurationMS:       inputDurationMS,
		Width:                 profile.Width,
		Height:                profile.Height,
		FrameRateNum:          profile.FrameRateNum,
		FrameRateDen:          profile.FrameRateDen,
	}
	inputTokens, outputTokens, totalTokens, err := EstimateSeedanceTokens(facts)
	if err != nil {
		return ProfitRoutingFacts{}, err
	}
	facts.InputTokens = inputTokens
	facts.OutputTokens = outputTokens
	facts.TotalTokens = totalTokens
	return facts, nil
}

// ErrProfitEligibility is the sentinel a failed pre-dispatch recheck matches. The
// matching error also unwraps to CostCoverageError, preserving the controller's
// existing exclude-and-retry behavior without exposing diagnostics to callers.
var ErrProfitEligibility = fmt.Errorf("profit eligibility recheck failed")

// ProfitEligibilityError carries the channel that failed the authoritative pre-dispatch
// recheck. Its public text is deliberately generic: amounts, thresholds and rule
// identity remain request-level admin diagnostics, never API error data.
type ProfitEligibilityError struct {
	ChannelID int
	Reason    ProfitExclusionReason
}

func (e *ProfitEligibilityError) Error() string {
	if e == nil {
		return "profit eligibility recheck failed"
	}
	return ErrProfitEligibility.Error()
}

func (e *ProfitEligibilityError) Is(target error) bool {
	return target == ErrProfitEligibility
}

func (e *ProfitEligibilityError) Unwrap() error {
	if e == nil {
		return nil
	}
	return &CostCoverageError{ChannelID: e.ChannelID}
}

// RecheckSelectedChannelProfit is the authoritative pre-dispatch margin gate. It runs
// in the task relay path immediately after ConfirmTaskCostIdentity sets the final
// BillableUpstreamModel, and BEFORE PrepareCostAttempt/AuthorizeCostDispatch/DoRequest,
// so a failed recheck creates no cost attempt and triggers no upstream side-effect.
//
// Unlike the candidate-stage filter (which prices the predicted model from the routing
// target), the recheck prices the authoritative billable model the adapter will actually
// send upstream. It re-reads the current active rule (authoritative=true, bypassing the
// candidate cache) and the latest global threshold, so a rule activation/version change
// or threshold bump between candidate selection and dispatch cannot bypass the gate.
//
// Returns nil when the channel passes; returns *ProfitEligibilityError (wrapping
// *CostCoverageError{ChannelID}) when it fails. The caller maps the failure to a
// retryable exclude-and-retry outcome, never to a silent switch.
func RecheckSelectedChannelProfit(c *gin.Context, info *relaycommon.RelayInfo) error {
	if cost_setting.Runtime().Mode != types.CostAccountingStrict {
		return nil
	}
	if info == nil || info.ChannelMeta == nil {
		return &ProfitEligibilityError{Reason: ProfitReasonCalculationError}
	}
	info.CostProfitRecheckSnapshot = nil
	billableModel := strings.TrimSpace(info.BillableUpstreamModel)
	channelID := info.ChannelId
	if billableModel == "" || channelID <= 0 {
		return &ProfitEligibilityError{
			ChannelID: channelID,
			Reason:    ProfitReasonMeterUnknown,
		}
	}
	ctx := context.Background()
	if c != nil && c.Request != nil {
		ctx = c.Request.Context()
	}

	candidate := ProfitRoutingCandidate{ChannelID: channelID, PredictedUpstreamModel: billableModel}
	group := info.UsingGroup
	if group == "" {
		group = info.TokenGroup
	}

	facts, revenueNanoUSD, hasRevenue := recheckFacts(c, ctx, info, group)
	if strings.Contains(strings.ToLower(info.RequestURLPath), "/video") &&
		(facts.OutputDurationSeconds <= 0 || facts.Width <= 0 || facts.Height <= 0 || facts.FrameRateNum <= 0 || facts.FrameRateDen <= 0) {
		return &ProfitEligibilityError{ChannelID: channelID, Reason: ProfitReasonMeterUnknown}
	}
	ruleCandidate := CostRuleCandidate{ChannelID: channelID, BillableUpstreamModel: billableModel}
	rules, err := ActiveCostRules([]CostRuleCandidate{ruleCandidate}, true)
	if err != nil {
		common.SysError(fmt.Sprintf("profit recheck active rule lookup failed: %s", err.Error()))
		return &ProfitEligibilityError{ChannelID: channelID, Reason: ProfitReasonCalculationError}
	}
	globalThreshold := cost_setting.Runtime().MinimumExpectedMarginBPS
	threshold, targetSnapshot, err := currentRecheckMarginThreshold(info, globalThreshold)
	if err != nil {
		common.SysError(fmt.Sprintf("profit recheck routing threshold lookup failed: %s", err.Error()))
		return &ProfitEligibilityError{ChannelID: channelID, Reason: ProfitReasonCalculationError}
	}
	// Resolve any input reference video metadata so per-token candidates price with the
	// authoritative input duration. The URLs are read from the request context's
	// FactsInput (set by the routing middleware); they stay in request memory only.
	var metadataState *ProfitRoutingRequestState
	if c != nil {
		input, ok := common.GetContextKeyType[modelrouting.FactsInput](c, constant.ContextKeyRoutingFactsInput)
		if ok && len(input.ReferenceVideoURLs) > 0 {
			metadataState = NewProfitRoutingRequestState(currentVideoMetadataClient(), input.ReferenceVideoURLs)
		}
	}

	filterResult := FilterProfitEligibleChannels(ProfitChannelFilterInput{
		Ctx:             ctx,
		Facts:           facts,
		RevenueNanoUSD:  revenueNanoUSD,
		HasRevenue:      hasRevenue,
		GlobalMarginBPS: threshold,
		Candidates:      []ProfitRoutingCandidate{candidate},
		MetadataState:   metadataState,
	}, rules)
	if _, allowed := filterResult.AllowedChannelIDs[channelID]; allowed {
		rule := rules[ruleCandidate]
		if rule == nil {
			return &ProfitEligibilityError{ChannelID: channelID, Reason: ProfitReasonCalculationError}
		}
		info.CostProfitRecheckSnapshot = &types.CostProfitRecheckSnapshot{
			ChannelID:                      channelID,
			BillableUpstreamModel:          billableModel,
			RuleID:                         rule.ID,
			RuleVersion:                    rule.Version,
			GlobalMinimumExpectedMarginBPS: globalThreshold,
			RouteTarget:                    targetSnapshot,
		}
		return nil
	}
	reason := ProfitReasonMarginBelowThreshold
	if len(filterResult.Exclusions) > 0 {
		reason = filterResult.Exclusions[0].Reason
	}
	return &ProfitEligibilityError{ChannelID: channelID, Reason: reason}
}

// currentRecheckMarginThreshold reads the selected target directly from storage rather
// than reusing the candidate-stage policy snapshot. A deleted or disabled target is
// not eligible to dispatch under strict accounting, so it fails closed.
func currentRecheckMarginThreshold(info *relaycommon.RelayInfo, globalThreshold int) (int, *types.CostRoutingTargetSnapshot, error) {
	if info == nil || info.Routing == nil {
		return globalThreshold, nil, nil
	}
	routing := info.Routing
	if routing.PolicyID <= 0 || routing.TargetID <= 0 || info.ChannelId <= 0 {
		return 0, nil, fmt.Errorf("selected routing target is incomplete")
	}
	var target model.RouteTarget
	err := model.DB.Where("id = ? AND policy_id = ? AND channel_id = ? AND enabled = ?", routing.TargetID, routing.PolicyID, info.ChannelId, true).
		First(&target).Error
	if err != nil {
		return 0, nil, err
	}
	if strings.TrimSpace(target.UpstreamModel) != strings.TrimSpace(info.BillableUpstreamModel) {
		return 0, nil, fmt.Errorf("selected routing target model changed")
	}
	targetSnapshot := &types.CostRoutingTargetSnapshot{
		ID:            target.ID,
		PolicyID:      target.PolicyID,
		ChannelID:     target.ChannelID,
		UpstreamModel: target.UpstreamModel,
	}
	if target.MinimumExpectedMarginBPS != nil {
		threshold := *target.MinimumExpectedMarginBPS
		targetSnapshot.MinimumExpectedMarginBPS = &threshold
		return threshold, targetSnapshot, nil
	}
	return globalThreshold, targetSnapshot, nil
}

// recheckFacts resolves the ProfitRoutingFacts + revenue for the pre-dispatch recheck.
// It prefers the capability-routing facts already on RelayInfo.Routing.Facts; otherwise
// it falls back to the request body's resolution/duration. Revenue is computed once via
// the injected callback, mirroring the candidate-stage path.
func recheckFacts(c *gin.Context, ctx context.Context, info *relaycommon.RelayInfo, group string) (ProfitRoutingFacts, int64, bool) {
	resolution := ""
	duration := 0
	if info.Routing != nil {
		resolution = info.Routing.Facts.OutputResolution
		duration = info.Routing.Facts.DurationSeconds
	}
	if c != nil {
		if routingFacts, ok := common.GetContextKeyType[modelrouting.Facts](c, constant.ContextKeyRoutingFacts); ok {
			if resolution == "" {
				resolution = routingFacts.OutputResolution
			}
			if duration <= 0 {
				duration = routingFacts.DurationSeconds
			}
		}
		if input, ok := common.GetContextKeyType[modelrouting.FactsInput](c, constant.ContextKeyRoutingFactsInput); ok {
			if resolution == "" && input.OutputResolution != nil {
				resolution = *input.OutputResolution
			}
			if duration <= 0 && input.DurationSeconds != nil && *input.DurationSeconds > 0 {
				duration = *input.DurationSeconds
			}
		}
	}
	if (resolution == "" || duration <= 0) && info.PriceData.RequestedDurationSeconds > 0 {
		duration = info.PriceData.RequestedDurationSeconds
	}
	facts := ProfitRoutingFacts{OutputDurationSeconds: duration}
	if resolution != "" && duration > 0 {
		if estimated, err := EstimateProfitRoutingFacts(resolution, duration, 0); err == nil {
			facts = estimated
		}
	}
	durationSeconds := duration
	revenueNanoUSD, err := PreviewRoutingRevenue(ctx, RoutingRevenuePreviewInput{
		OriginModelName: info.OriginModelName,
		Group:           group,
		RequestPath:     info.RequestURLPath,
		RelayMode:       info.RelayMode,
		DurationSeconds: &durationSeconds,
		UserId:          info.UserId,
	})
	if err != nil {
		return facts, 0, false
	}
	return facts, revenueNanoUSD, true
}
