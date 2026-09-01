package service

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/imageprofile"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/cost_setting"
	"github.com/QuantumNous/new-api/setting/image_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

var (
	ErrNoCompatibleImageChannel = errors.New("no compatible image channel is available")
	ErrNoEligibleImageChannel   = errors.New("no eligible image channel is available")
)

// ImageRouteCandidate is the admin-safe routing snapshot for one image
// channel. Estimated amounts are expressed in nano-USD and are never exposed
// to ordinary users by the relay error path.
type ImageRouteCandidate struct {
	ChannelID               int    `json:"channel_id"`
	ChannelName             string `json:"channel_name,omitempty"`
	Priority                int    `json:"priority"`
	Weight                  int    `json:"weight"`
	UpstreamModel           string `json:"upstream_model,omitempty"`
	SKUKey                  string `json:"sku_key,omitempty"`
	CostKnown               bool   `json:"cost_known"`
	EstimatedCostNanoUSD    int64  `json:"estimated_cost_nano_usd,omitempty"`
	EstimatedRevenueNanoUSD int64  `json:"estimated_revenue_nano_usd,omitempty"`
	EstimatedCostUSD        string `json:"estimated_cost_usd,omitempty"`
	EstimatedRevenueUSD     string `json:"estimated_revenue_usd,omitempty"`
	RuleID                  int64  `json:"rule_id,omitempty"`
	RuleVersion             int    `json:"rule_version,omitempty"`
	ExclusionReason         string `json:"exclusion_reason,omitempty"`
	EffectiveWeight         int    `json:"-"`
}

// ImageRouteDecision contains the complete candidate audit snapshot and the
// selected candidate. Candidates include excluded entries so an administrator
// can see why a channel was not selected; Selected is always nil when no
// eligible candidate remains.
type ImageRouteDecision struct {
	Strategy   image_setting.Strategy
	Selected   *ImageRouteCandidate
	Candidates []ImageRouteCandidate
}

// PreviewImageRoute builds the same cost-aware decision used by dispatch, but
// does not mutate channel state or send an upstream request. It is intended for
// administrator previews and diagnostics.
func PreviewImageRoute(ctx *gin.Context, group, modelName, requestPath string, request ImageRequestContext) (ImageRouteDecision, error) {
	param := &RetryParam{Ctx: ctx, TokenGroup: group, ModelName: modelName, RequestPath: requestPath, ImageRequest: &request}
	candidates, err := model.ListSatisfiedChannels(group, modelName, requestPath, model.ChannelSelectFilter{})
	if err != nil {
		return ImageRouteDecision{}, err
	}
	return BuildImageRouteDecision(param, group, candidates)
}

// imageRouteWeightSelector is kept injectable for deterministic routing tests.
// Production uses the same baseline-weight semantics as the legacy manual
// selector (+10 per channel), while cost and priority remain deterministic
// before this function is called.
var imageRouteWeightSelector = selectWeightedImageRouteCandidate

// SortImageRouteCandidates returns a stable copy ordered by the lowest-cost
// policy. Known costs always precede unknown costs; known costs sort ascending,
// then priority descends. Unknown costs retain priority ordering so a fallback
// still respects the operator's reliability preference.
func SortImageRouteCandidates(candidates []ImageRouteCandidate) []ImageRouteCandidate {
	sorted := append([]ImageRouteCandidate(nil), candidates...)
	sort.SliceStable(sorted, func(i, j int) bool {
		left, right := sorted[i], sorted[j]
		if left.CostKnown != right.CostKnown {
			return left.CostKnown
		}
		if left.CostKnown && left.EstimatedCostNanoUSD != right.EstimatedCostNanoUSD {
			return left.EstimatedCostNanoUSD < right.EstimatedCostNanoUSD
		}
		if left.Priority != right.Priority {
			return left.Priority > right.Priority
		}
		return left.ChannelID < right.ChannelID
	})
	return sorted
}

// SelectImageRouteCandidate selects from the first cost/priority equivalence
// class. Weight is intentionally consulted only after those two keys match.
func SelectImageRouteCandidate(candidates []ImageRouteCandidate) *ImageRouteCandidate {
	sorted := SortImageRouteCandidates(candidates)
	if len(sorted) == 0 {
		return nil
	}
	first := sorted[0]
	tie := make([]ImageRouteCandidate, 0, len(sorted))
	for _, candidate := range sorted {
		if candidate.CostKnown != first.CostKnown || candidate.Priority != first.Priority {
			break
		}
		if candidate.CostKnown && candidate.EstimatedCostNanoUSD != first.EstimatedCostNanoUSD {
			break
		}
		tie = append(tie, candidate)
	}
	selected := imageRouteWeightSelector(tie)
	if selected == nil {
		return nil
	}
	return selected
}

const defaultImageCostToleranceBPS = 1000

// BuildCostWeightedImageRoutePool retains known-cost candidates within the
// configured tolerance of the cheapest candidate and assigns each an
// integer weight that combines the existing channel weight with inverse cost.
// The returned slice is a copy and the input candidates are never modified.
func BuildCostWeightedImageRoutePool(candidates []ImageRouteCandidate, toleranceBPS *int) []ImageRouteCandidate {
	known := make([]ImageRouteCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.CostKnown && candidate.EstimatedCostNanoUSD >= 0 {
			known = append(known, candidate)
		}
	}
	if len(known) == 0 {
		return []ImageRouteCandidate{}
	}
	tolerance := defaultImageCostToleranceBPS
	if toleranceBPS != nil {
		tolerance = *toleranceBPS
		if tolerance < 0 {
			tolerance = 0
		}
		if tolerance > 10000 {
			tolerance = 10000
		}
	}
	minCost := known[0].EstimatedCostNanoUSD
	for _, candidate := range known[1:] {
		if candidate.EstimatedCostNanoUSD < minCost {
			minCost = candidate.EstimatedCostNanoUSD
		}
	}
	upperBound := decimal.NewFromInt(minCost).
		Mul(decimal.NewFromInt(int64(10000 + tolerance))).
		Div(decimal.NewFromInt(10000))

	pool := make([]ImageRouteCandidate, 0, len(known))
	for _, candidate := range known {
		cost := decimal.NewFromInt(candidate.EstimatedCostNanoUSD)
		if cost.GreaterThan(upperBound) {
			continue
		}
		candidate.EffectiveWeight = imageCostWeightedEffectiveWeight(candidate.Weight, minCost, candidate.EstimatedCostNanoUSD)
		pool = append(pool, candidate)
	}
	sort.SliceStable(pool, func(i, j int) bool {
		if pool[i].EstimatedCostNanoUSD != pool[j].EstimatedCostNanoUSD {
			return pool[i].EstimatedCostNanoUSD < pool[j].EstimatedCostNanoUSD
		}
		return pool[i].ChannelID < pool[j].ChannelID
	})
	return pool
}

func imageCostWeightedEffectiveWeight(baseWeight int, minimumCost, candidateCost int64) int {
	base := int64(baseWeight)
	if base < 0 {
		base = 0
	}
	if base > int64(maxInt()-10) {
		base = int64(maxInt() - 10)
	}
	base += 10
	if base < 1 {
		base = 1
	}
	if minimumCost == 0 {
		if candidateCost == 0 {
			return int(base)
		}
		return 1
	}
	if candidateCost <= 0 {
		return int(base)
	}
	// Keep six decimal places of inverse-cost precision before rounding down
	// to an integer selector weight. Decimal avoids int64 multiplication
	// overflow when provider costs approach the storage limit.
	scale := decimal.NewFromInt(1_000_000)
	factor := decimal.NewFromInt(minimumCost).Mul(scale).Div(decimal.NewFromInt(candidateCost))
	weight := decimal.NewFromInt(base).Mul(factor).IntPart() / 1_000_000
	if weight < 1 {
		weight = 1
	}
	if weight > int64(maxInt()) {
		return maxInt()
	}
	return int(weight)
}

func maxInt() int {
	return int(^uint(0) >> 1)
}

func selectCostWeightedImageRouteCandidate(candidates []ImageRouteCandidate, toleranceBPS *int) *ImageRouteCandidate {
	pool := BuildCostWeightedImageRoutePool(candidates, toleranceBPS)
	if len(pool) == 0 {
		return nil
	}
	weighted := make([]ImageRouteCandidate, len(pool))
	copy(weighted, pool)
	for index := range weighted {
		weighted[index].Weight = weighted[index].EffectiveWeight
	}
	return imageRouteWeightSelector(weighted)
}

// BuildImageRouteDecision evaluates and selects the complete image candidate
// set. It is intentionally independent from the relay dispatch loop: callers
// can remove failed channel IDs and invoke it again, causing a full cost sort
// instead of advancing only the old priority layer.
func BuildImageRouteDecision(param *RetryParam, group string, candidates []model.SatisfiedChannel) (ImageRouteDecision, error) {
	if param == nil || param.ImageRequest == nil {
		return ImageRouteDecision{}, errors.New("image request context is required")
	}
	group = strings.TrimSpace(group)
	resolved := param.ImageRequest.Resolved
	policy := image_setting.PolicyFor(group, resolved.Model)
	decision := ImageRouteDecision{Strategy: policy.Strategy, Candidates: make([]ImageRouteCandidate, 0, len(candidates))}

	eligible := make([]ImageRouteCandidate, 0, len(candidates))
	for _, satisfied := range candidates {
		candidate := imageRouteCandidateFromSatisfied(satisfied)
		if satisfied.Channel == nil {
			candidate.ExclusionReason = "channel_missing"
			decision.Candidates = append(decision.Candidates, candidate)
			continue
		}
		eligibility, err := EvaluateImageChannel(satisfied.Channel, resolved.Model, *param.ImageRequest)
		if err != nil {
			candidate.ExclusionReason = "image_incompatible"
			decision.Candidates = append(decision.Candidates, candidate)
			continue
		}
		candidate.UpstreamModel = strings.TrimSpace(eligibility.UpstreamModel)
		candidate.SKUKey = strings.TrimSpace(eligibility.CostVariantKey)
		if candidate.SKUKey == "" {
			candidate.SKUKey = string(types.DefaultCostVariantKey)
		}
		if reason := imageCompatibilityExclusion(satisfied.Channel, resolved.Model, *param.ImageRequest, policy.RequireCompatibilityTest); reason != "" {
			candidate.ExclusionReason = reason
			decision.Candidates = append(decision.Candidates, candidate)
			continue
		}
		eligible = append(eligible, candidate)
	}

	if len(eligible) == 0 {
		decision.Candidates = SortImageRouteCandidates(decision.Candidates)
		return decision, ErrNoCompatibleImageChannel
	}

	strict := cost_setting.Runtime().Mode == types.CostAccountingStrict
	if policy.Strategy == image_setting.StrategyManual && !strict {
		decision.Candidates = append(decision.Candidates, eligible...)
		decision.Candidates = SortImageRouteCandidates(decision.Candidates)
		selected := SelectManualImageRouteCandidate(eligible, 0)
		if selected == nil {
			return decision, ErrNoCompatibleImageChannel
		}
		decision.Selected = findImageRouteCandidate(decision.Candidates, selected.ChannelID)
		return decision, nil
	}
	if policy.Strategy != image_setting.StrategyLowestCost && policy.Strategy != image_setting.StrategyManual {
		return ImageRouteDecision{}, fmt.Errorf("unsupported image routing strategy %q", policy.Strategy)
	}

	revenueNanoUSD, revenueErr := imageRoutingRevenueNanoUSD(param, group)
	if revenueErr != nil {
		for index := range eligible {
			eligible[index].ExclusionReason = "revenue_unknown"
		}
		decision.Candidates = append(decision.Candidates, eligible...)
		decision.Candidates = SortImageRouteCandidates(decision.Candidates)
		return decision, revenueErr
	}

	ruleCandidates := make([]CostRuleCandidate, 0, len(eligible))
	for _, candidate := range eligible {
		ruleCandidates = append(ruleCandidates, CostRuleCandidate{
			ChannelID:             candidate.ChannelID,
			BillableUpstreamModel: candidate.UpstreamModel,
			CostVariantKey:        candidate.SKUKey,
		})
	}
	rules, err := ActiveCostRules(ruleCandidates, false)
	if err != nil {
		return decision, err
	}
	minimumMarginBPS := cost_setting.Runtime().MinimumExpectedMarginBPS
	if policy.MinimumExpectedMarginBPS != nil {
		minimumMarginBPS = *policy.MinimumExpectedMarginBPS
	}
	priced := make([]ImageRouteCandidate, 0, len(eligible))
	for index := range eligible {
		candidate := &eligible[index]
		candidate.EstimatedRevenueNanoUSD = revenueNanoUSD
		candidate.EstimatedRevenueUSD = formatImageRouteUSD(revenueNanoUSD)
		ruleKey := CostRuleCandidate{
			ChannelID:             candidate.ChannelID,
			BillableUpstreamModel: candidate.UpstreamModel,
			CostVariantKey:        candidate.SKUKey,
		}
		rule := rules[ruleKey]
		if rule == nil {
			candidate.ExclusionReason = "cost_rule_missing"
			if strict {
				decision.Candidates = append(decision.Candidates, *candidate)
				continue
			}
			priced = append(priced, *candidate)
			decision.Candidates = append(decision.Candidates, *candidate)
			continue
		}
		candidate.RuleID = rule.ID
		candidate.RuleVersion = rule.Version
		costNanoUSD, known, costReason := estimateImageRouteCost(rule, resolved.N)
		candidate.CostKnown = known
		candidate.EstimatedCostNanoUSD = costNanoUSD
		candidate.EstimatedCostUSD = formatImageRouteUSD(costNanoUSD)
		if !known {
			candidate.ExclusionReason = costReason
			if strict {
				decision.Candidates = append(decision.Candidates, *candidate)
				continue
			}
			priced = append(priced, *candidate)
			decision.Candidates = append(decision.Candidates, *candidate)
			continue
		}
		if strict {
			profitEligibility := EvaluateProfitEligibility(ProfitRoutingInput{
				RevenueNanoUSD: revenueNanoUSD,
				CostNanoUSD:    costNanoUSD,
				ThresholdBPS:   minimumMarginBPS,
				RuleID:         rule.ID,
				RuleVersion:    rule.Version,
			})
			if !profitEligibility.Eligible {
				candidate.ExclusionReason = string(profitEligibility.Reason)
				decision.Candidates = append(decision.Candidates, *candidate)
				continue
			}
		}
		priced = append(priced, *candidate)
		decision.Candidates = append(decision.Candidates, *candidate)
	}

	decision.Candidates = SortImageRouteCandidates(decision.Candidates)
	if len(priced) == 0 {
		return decision, ErrNoEligibleImageChannel
	}
	if policy.Strategy == image_setting.StrategyManual {
		selected := SelectManualImageRouteCandidate(priced, 0)
		if selected == nil {
			return decision, ErrNoEligibleImageChannel
		}
		decision.Selected = findImageRouteCandidate(decision.Candidates, selected.ChannelID)
		return decision, nil
	}
	selected := SelectImageRouteCandidate(priced)
	if selected == nil {
		return decision, ErrNoEligibleImageChannel
	}
	decision.Selected = findImageRouteCandidate(decision.Candidates, selected.ChannelID)
	return decision, nil
}

// SelectManualImageRouteCandidate applies the legacy priority-layer semantics
// to the already eligible image candidates. Image retries pass the remaining
// candidate set through this function, so they restart at the highest priority
// instead of advancing a stale retry index to another layer.
func SelectManualImageRouteCandidate(candidates []ImageRouteCandidate, priorityRetry int) *ImageRouteCandidate {
	_ = priorityRetry
	if len(candidates) == 0 {
		return nil
	}
	priorities := make([]int, 0, len(candidates))
	seen := make(map[int]struct{}, len(candidates))
	for _, candidate := range candidates {
		if _, exists := seen[candidate.Priority]; exists {
			continue
		}
		seen[candidate.Priority] = struct{}{}
		priorities = append(priorities, candidate.Priority)
	}
	sort.Slice(priorities, func(i, j int) bool { return priorities[i] > priorities[j] })
	targetPriority := priorities[0]
	tie := make([]ImageRouteCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.Priority == targetPriority {
			tie = append(tie, candidate)
		}
	}
	return imageRouteWeightSelector(tie)
}

func imageRouteCandidateFromSatisfied(satisfied model.SatisfiedChannel) ImageRouteCandidate {
	candidate := ImageRouteCandidate{Priority: int(satisfied.Priority), Weight: satisfied.Weight}
	if satisfied.Channel != nil {
		candidate.ChannelID = satisfied.Channel.Id
		candidate.ChannelName = strings.TrimSpace(satisfied.Channel.Name)
	}
	return candidate
}

func formatImageRouteUSD(value int64) string {
	return decimal.NewFromInt(value).Div(decimal.NewFromInt(1_000_000_000)).String()
}

func imageCompatibilityExclusion(channel *model.Channel, publicModel string, request ImageRequestContext, requireTest bool) string {
	if channel == nil {
		return "channel_missing"
	}
	binding := channel.GetOtherSettings().ImageProfile
	if binding == nil {
		return "image_profile_missing"
	}
	key := fmt.Sprintf("%s:%s", strings.TrimSpace(publicModel), request.Resolved.Endpoint)
	compatibility, exists := binding.Compatibility[key]
	if !exists {
		if requireTest {
			return "compatibility_untested"
		}
		return ""
	}
	if compatibility.Status == imageprofile.CompatibilityFailed {
		return "compatibility_failed"
	}
	if compatibility.ProfileVersion != 0 && compatibility.ProfileVersion != binding.ProfileVersion {
		return "compatibility_stale"
	}
	if compatibility.Status == imageprofile.CompatibilityPassed && compatibility.ContractHash != "" {
		hash, err := ResolveImageContractHash(channel, publicModel, request)
		if err != nil || hash != compatibility.ContractHash {
			return "compatibility_stale"
		}
	}
	if compatibility.Status == imageprofile.CompatibilityUntested && requireTest {
		return "compatibility_untested"
	}
	return ""
}

func estimateImageRouteCost(rule *model.ChannelModelCostRule, imageCount uint) (int64, bool, string) {
	if rule == nil {
		return 0, false, "cost_rule_missing"
	}
	mode := types.CostMode(rule.CostMode)
	if mode != types.CostModeFree && mode != types.CostModePerRequest && mode != types.CostModePerImage {
		return 0, false, "cost_meter_unknown"
	}
	config, err := parseCostRuleConfigForProfit(rule)
	if err != nil {
		return 0, false, "cost_rule_invalid"
	}
	if mode == types.CostModeFree {
		return 0, true, ""
	}
	if imageCount == 0 || imageCount > dto.MaxImageN {
		return 0, false, "image_count_invalid"
	}
	meter := types.CostMeter{}
	if mode == types.CostModePerImage {
		count := int64(imageCount)
		meter.ImageCount = &count
	}
	_, costNanoUSD, err := CalculateAttemptCost(mode, config, meter)
	if err != nil || costNanoUSD < 0 {
		return 0, false, "cost_calculation_error"
	}
	return costNanoUSD, true, ""
}

func imageRoutingRevenueNanoUSD(param *RetryParam, group string) (int64, error) {
	if param == nil || param.ImageRequest == nil {
		return 0, errors.New("image request context is required")
	}
	resolved := param.ImageRequest.Resolved
	if resolved.N == 0 || resolved.N > dto.MaxImageN {
		return 0, fmt.Errorf("image n must be between 1 and %d", dto.MaxImageN)
	}
	price, err := decimal.NewFromString(strings.TrimSpace(resolved.SalePriceUSD))
	if err != nil || price.IsNegative() {
		return 0, errors.New("image sale price is invalid")
	}
	ratio := imageRoutingGroupRatio(param.Ctx, group)
	if math.IsNaN(ratio) || math.IsInf(ratio, 0) || ratio < 0 {
		return 0, errors.New("image group ratio is invalid")
	}
	revenue := price.Mul(decimal.NewFromInt(int64(resolved.N))).Mul(decimal.NewFromFloat(ratio))
	return DecimalToNanoUSD(revenue)
}

func imageRoutingGroupRatio(ctx *gin.Context, group string) float64 {
	userGroup := ""
	if ctx != nil {
		userGroup = common.GetContextKeyString(ctx, constant.ContextKeyUserGroup)
	}
	if special, ok := ratio_setting.GetGroupGroupRatio(userGroup, group); ok {
		return special
	}
	return ratio_setting.GetGroupRatio(group)
}

func findImageRouteCandidate(candidates []ImageRouteCandidate, channelID int) *ImageRouteCandidate {
	for index := range candidates {
		if candidates[index].ChannelID == channelID {
			candidate := candidates[index]
			return &candidate
		}
	}
	return nil
}

func selectWeightedImageRouteCandidate(candidates []ImageRouteCandidate) *ImageRouteCandidate {
	if len(candidates) == 0 {
		return nil
	}
	total := int64(0)
	for _, candidate := range candidates {
		weight := candidate.Weight
		if weight < 0 {
			weight = 0
		}
		if total > int64(^uint(0)>>1)-int64(weight)-10 {
			selected := candidates[0]
			return &selected
		}
		total += int64(weight) + 10
	}
	if total <= 0 {
		selected := candidates[0]
		return &selected
	}
	value := common.GetRandomInt(int(total))
	for _, candidate := range candidates {
		weight := candidate.Weight
		if weight < 0 {
			weight = 0
		}
		value -= weight + 10
		if value < 0 {
			selected := candidate
			return &selected
		}
	}
	selected := candidates[len(candidates)-1]
	return &selected
}
