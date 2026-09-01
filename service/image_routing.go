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

// ImageRouteCandidate is the admin-safe routing snapshot for one image
// channel. Estimated amounts are expressed in nano-USD and are never exposed
// to ordinary users by the relay error path.
type ImageRouteCandidate struct {
	ChannelID               int
	Priority                int
	Weight                  int
	UpstreamModel           string
	SKUKey                  string
	CostKnown               bool
	EstimatedCostNanoUSD    int64
	EstimatedRevenueNanoUSD int64
	RuleID                  int64
	RuleVersion             int
	ExclusionReason         string
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
	eligibleSatisfied := make([]model.SatisfiedChannel, 0, len(candidates))
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
		eligibleSatisfied = append(eligibleSatisfied, satisfied)
	}

	if len(eligible) == 0 {
		decision.Candidates = SortImageRouteCandidates(decision.Candidates)
		return decision, errors.New("no compatible image channel is available")
	}

	if policy.Strategy == image_setting.StrategyManual {
		decision.Candidates = append(decision.Candidates, eligible...)
		decision.Candidates = SortImageRouteCandidates(decision.Candidates)
		selected := model.SelectManualChannel(eligibleSatisfied, param.GetRetry())
		if selected == nil {
			return decision, errors.New("no compatible image channel is available")
		}
		decision.Selected = findImageRouteCandidate(decision.Candidates, selected.Id)
		return decision, nil
	}
	if policy.Strategy != image_setting.StrategyLowestCost {
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
	strict := cost_setting.Runtime().Mode == types.CostAccountingStrict
	priced := make([]ImageRouteCandidate, 0, len(eligible))
	for index := range eligible {
		candidate := &eligible[index]
		candidate.EstimatedRevenueNanoUSD = revenueNanoUSD
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
		return decision, errors.New("no eligible image channel is available")
	}
	selected := SelectImageRouteCandidate(priced)
	if selected == nil {
		return decision, errors.New("no eligible image channel is available")
	}
	decision.Selected = findImageRouteCandidate(decision.Candidates, selected.ChannelID)
	return decision, nil
}

func imageRouteCandidateFromSatisfied(satisfied model.SatisfiedChannel) ImageRouteCandidate {
	candidate := ImageRouteCandidate{Priority: int(satisfied.Priority), Weight: satisfied.Weight}
	if satisfied.Channel != nil {
		candidate.ChannelID = satisfied.Channel.Id
	}
	return candidate
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
