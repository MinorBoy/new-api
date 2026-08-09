package service

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelrouting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
)

type GroupRoutingTargetStatus string

const (
	GroupRoutingTargetMatched            GroupRoutingTargetStatus = "matched"
	GroupRoutingTargetRealPersonMismatch GroupRoutingTargetStatus = "real_person_mismatch"
	GroupRoutingTargetRealPersonUnknown  GroupRoutingTargetStatus = "real_person_unknown"
	GroupRoutingTargetCostModeMismatch   GroupRoutingTargetStatus = "cost_mode_mismatch"
	GroupRoutingTargetCostRuleMissing    GroupRoutingTargetStatus = "cost_rule_missing"
	GroupRoutingTargetExcluded           GroupRoutingTargetStatus = "excluded"
	GroupRoutingTargetDisabled           GroupRoutingTargetStatus = "target_disabled"
	GroupRoutingTargetChannelUnavailable GroupRoutingTargetStatus = "channel_unavailable"
)

type GroupRoutingTargetEvaluation struct {
	Target      modelrouting.Target
	TargetKey   string
	Eligible    bool
	Status      GroupRoutingTargetStatus
	Issues      []GroupRoutingTargetStatus
	CostMode    types.CostMode
	CostRuleID  int64
	CostVersion int
}

type GroupRoutingProfileEvaluation struct {
	Snapshot       modelrouting.PolicySnapshot
	Targets        []GroupRoutingTargetEvaluation
	MismatchCounts map[GroupRoutingTargetStatus]int
	CostRules      map[CostRuleCandidate]*model.ChannelModelCostRule
}

type GroupRoutingAvailabilityKey = model.RoutingAvailabilityKey

func GroupRoutingTargetKey(sourceGroup, canonicalModel string, target modelrouting.Target) string {
	identity := strings.Join([]string{
		strings.TrimSpace(sourceGroup),
		modelrouting.NormalizeCanonicalModel(canonicalModel),
		strconv.Itoa(target.ChannelID),
		strings.TrimSpace(target.UpstreamModel),
		strings.TrimSpace(target.CostVariantKey),
		strings.TrimSpace(target.Name),
	}, "\x1f")
	return "grt_" + fmt.Sprintf("%x", common.Sha256Raw([]byte(identity)))
}

func EvaluateGroupRoutingProfile(
	profile ratio_setting.GroupRoutingRequirements,
	snapshot modelrouting.PolicySnapshot,
	rules map[CostRuleCandidate]*model.ChannelModelCostRule,
	available map[GroupRoutingAvailabilityKey]struct{},
) GroupRoutingProfileEvaluation {
	filtered := snapshot
	filtered.TargetsByChannel = make(map[int][]modelrouting.Target)
	result := GroupRoutingProfileEvaluation{
		Snapshot:       filtered,
		Targets:        make([]GroupRoutingTargetEvaluation, 0),
		MismatchCounts: make(map[GroupRoutingTargetStatus]int),
		CostRules:      rules,
	}
	excluded := make(map[string]struct{}, len(profile.ExcludedTargetKeys))
	for _, targetKey := range profile.ExcludedTargetKeys {
		excluded[targetKey] = struct{}{}
	}

	channelIDs := make([]int, 0, len(snapshot.TargetsByChannel))
	for channelID := range snapshot.TargetsByChannel {
		channelIDs = append(channelIDs, channelID)
	}
	sort.Ints(channelIDs)
	for _, channelID := range channelIDs {
		for _, target := range snapshot.TargetsByChannel[channelID] {
			evaluation := evaluateGroupRoutingTarget(profile, snapshot, target, rules, available, excluded)
			result.Targets = append(result.Targets, evaluation)
			if !evaluation.Eligible {
				result.MismatchCounts[evaluation.Status]++
				continue
			}
			result.Snapshot.TargetsByChannel[channelID] = append(result.Snapshot.TargetsByChannel[channelID], target)
		}
	}
	return result
}

func evaluateGroupRoutingTarget(
	profile ratio_setting.GroupRoutingRequirements,
	snapshot modelrouting.PolicySnapshot,
	target modelrouting.Target,
	rules map[CostRuleCandidate]*model.ChannelModelCostRule,
	available map[GroupRoutingAvailabilityKey]struct{},
	excluded map[string]struct{},
) GroupRoutingTargetEvaluation {
	evaluation := GroupRoutingTargetEvaluation{
		Target:    target,
		TargetKey: GroupRoutingTargetKey(profile.RoutingSource, snapshot.CanonicalModel, target),
		Eligible:  true,
		Status:    GroupRoutingTargetMatched,
	}
	hardIssues := make([]GroupRoutingTargetStatus, 0, 5)
	addHardIssue := func(status GroupRoutingTargetStatus) {
		hardIssues = append(hardIssues, status)
		evaluation.Issues = append(evaluation.Issues, status)
	}

	if !target.Enabled {
		addHardIssue(GroupRoutingTargetDisabled)
	}
	availabilityKey := GroupRoutingAvailabilityKey{
		CanonicalModel: modelrouting.NormalizeCanonicalModel(snapshot.CanonicalModel),
		ChannelID:      target.ChannelID,
	}
	if _, ok := available[availabilityKey]; !ok {
		addHardIssue(GroupRoutingTargetChannelUnavailable)
	}

	switch profile.EffectiveRealPersonMode() {
	case ratio_setting.GroupRealPersonRequired:
		if target.Constraints.SupportsRealPerson == nil {
			addHardIssue(GroupRoutingTargetRealPersonUnknown)
		} else if !*target.Constraints.SupportsRealPerson {
			addHardIssue(GroupRoutingTargetRealPersonMismatch)
		}
	case ratio_setting.GroupRealPersonForbidden:
		if target.Constraints.SupportsRealPerson == nil {
			addHardIssue(GroupRoutingTargetRealPersonUnknown)
		} else if *target.Constraints.SupportsRealPerson {
			addHardIssue(GroupRoutingTargetRealPersonMismatch)
		}
	}

	if _, ok := excluded[evaluation.TargetKey]; ok {
		addHardIssue(GroupRoutingTargetExcluded)
	}

	variant, variantErr := types.NormalizeCostVariantKey(target.CostVariantKey)
	candidate := CostRuleCandidate{
		ChannelID:             target.ChannelID,
		BillableUpstreamModel: strings.TrimSpace(target.UpstreamModel),
		CostVariantKey:        variant,
	}
	rule := rules[candidate]
	if variantErr != nil || rule == nil {
		evaluation.Issues = append(evaluation.Issues, GroupRoutingTargetCostRuleMissing)
		if len(profile.AllowedCostModes) > 0 {
			hardIssues = append(hardIssues, GroupRoutingTargetCostRuleMissing)
		}
	} else {
		evaluation.CostMode = types.CostMode(rule.CostMode)
		evaluation.CostRuleID = rule.ID
		evaluation.CostVersion = rule.Version
		if len(profile.AllowedCostModes) > 0 {
			allowed := false
			for _, costMode := range profile.AllowedCostModes {
				if costMode == evaluation.CostMode {
					allowed = true
					break
				}
			}
			if !allowed {
				addHardIssue(GroupRoutingTargetCostModeMismatch)
			}
		}
	}

	if len(hardIssues) > 0 {
		evaluation.Eligible = false
		evaluation.Status = hardIssues[0]
	}
	return evaluation
}

func ResolveGroupRoutingProfilePolicies(
	profile ratio_setting.GroupRoutingRequirements,
	policies []model.RoutingPolicy,
) ([]GroupRoutingProfileEvaluation, error) {
	keys := make([]CostRuleCandidate, 0)
	models := make([]string, 0, len(policies))
	seenModels := make(map[string]struct{}, len(policies))
	for _, policy := range policies {
		canonicalModel := modelrouting.NormalizeCanonicalModel(policy.Model)
		if _, exists := seenModels[canonicalModel]; !exists {
			seenModels[canonicalModel] = struct{}{}
			models = append(models, canonicalModel)
		}
		for _, target := range policy.Targets {
			keys = append(keys, CostRuleCandidate{
				ChannelID:             target.ChannelID,
				BillableUpstreamModel: target.UpstreamModel,
				CostVariantKey:        target.CostVariantKey,
			})
		}
	}
	rules, err := ActiveCostRules(keys, false)
	if err != nil {
		return nil, err
	}
	available, err := model.ListRoutingAvailability(profile.RoutingSource, models)
	if err != nil {
		return nil, err
	}
	results := make([]GroupRoutingProfileEvaluation, 0, len(policies))
	for index := range policies {
		snapshot, err := model.RoutingPolicySnapshotFromRows(policies[index], policies[index].Targets)
		if err != nil {
			return nil, err
		}
		results = append(results, EvaluateGroupRoutingProfile(profile, snapshot, rules, available))
	}
	return results, nil
}
