package service

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelrouting"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"gorm.io/gorm"
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

const (
	GroupRoutingProfileErrorInvalid     = "invalid_group_profile"
	GroupRoutingProfileErrorUnavailable = "group_profile_unavailable"
	GroupRoutingProfileErrorPreview     = "group_profile_preview_failed"
)

type GroupRoutingProfileError struct {
	Code string
	Err  error
}

func (e *GroupRoutingProfileError) Error() string {
	if e.Err == nil {
		return e.Code
	}
	return e.Err.Error()
}

func (e *GroupRoutingProfileError) Unwrap() error {
	return e.Err
}

type GroupRoutingProfilePreviewInput struct {
	GroupName string                                 `json:"group_name"`
	Profile   ratio_setting.GroupRoutingRequirements `json:"profile"`
	Model     string                                 `json:"model,omitempty"`
	ChannelID int                                    `json:"channel_id,omitempty"`
	CostMode  types.CostMode                         `json:"cost_mode,omitempty"`
	Status    GroupRoutingTargetStatus               `json:"status,omitempty"`
	Page      int                                    `json:"page"`
	PageSize  int                                    `json:"page_size"`
}

type GroupRoutingProfileSummary struct {
	Models          int `json:"models"`
	MatchedModels   int `json:"matched_models"`
	Targets         int `json:"targets"`
	MatchedTargets  int `json:"matched_targets"`
	StaleExclusions int `json:"stale_exclusions"`
}

type GroupRoutingProfileTargetView struct {
	Model              string                     `json:"model"`
	ChannelID          int                        `json:"channel_id"`
	ChannelName        string                     `json:"channel_name"`
	TargetName         string                     `json:"target_name"`
	UpstreamModel      string                     `json:"upstream_model"`
	CostVariantKey     string                     `json:"cost_variant_key"`
	TargetPriority     int                        `json:"target_priority"`
	SupportsRealPerson *bool                      `json:"supports_real_person"`
	CostMode           types.CostMode             `json:"cost_mode,omitempty"`
	CostRuleID         int64                      `json:"cost_rule_id,omitempty"`
	CostRuleVersion    int                        `json:"cost_rule_version,omitempty"`
	TargetKey          string                     `json:"target_key"`
	Status             GroupRoutingTargetStatus   `json:"status"`
	Issues             []GroupRoutingTargetStatus `json:"issues"`
}

type GroupRoutingProfileChannelFacet struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type GroupRoutingProfileFacets struct {
	Models    []string                          `json:"models"`
	Channels  []GroupRoutingProfileChannelFacet `json:"channels"`
	CostModes []types.CostMode                  `json:"cost_modes"`
	Statuses  []GroupRoutingTargetStatus        `json:"statuses"`
}

type GroupRoutingProfileTargetPage struct {
	Items    []GroupRoutingProfileTargetView `json:"items"`
	Summary  GroupRoutingProfileSummary      `json:"summary"`
	Facets   GroupRoutingProfileFacets       `json:"facets"`
	Page     int                             `json:"page"`
	PageSize int                             `json:"page_size"`
	Total    int                             `json:"total"`
}

type groupRoutingProfileCatalog struct {
	snapshots []modelrouting.PolicySnapshot
	rules     map[CostRuleCandidate]*model.ChannelModelCostRule
	available map[GroupRoutingAvailabilityKey]struct{}
	channels  map[int]string
}

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

func PreviewGroupRoutingProfile(input GroupRoutingProfilePreviewInput) (GroupRoutingProfileTargetPage, error) {
	input, err := normalizeGroupRoutingProfilePreviewInput(input)
	if err != nil {
		return GroupRoutingProfileTargetPage{}, err
	}
	catalog, err := loadGroupRoutingProfileCatalog(input.Profile.RoutingSource, true)
	if err != nil {
		return GroupRoutingProfileTargetPage{}, err
	}
	evaluations := evaluateGroupRoutingProfileCatalog(input.Profile, catalog)
	summary := summarizeGroupRoutingProfile(input.Profile, evaluations)
	views := groupRoutingProfileTargetViews(evaluations, catalog.channels)
	facets := groupRoutingProfileFacets(views)

	filtered := make([]GroupRoutingProfileTargetView, 0, len(views))
	for _, view := range views {
		if input.Model != "" && modelrouting.NormalizeCanonicalModel(view.Model) != input.Model {
			continue
		}
		if input.ChannelID > 0 && view.ChannelID != input.ChannelID {
			continue
		}
		if input.CostMode != "" && view.CostMode != input.CostMode {
			continue
		}
		if input.Status != "" && view.Status != input.Status {
			matchedIssue := false
			for _, issue := range view.Issues {
				if issue == input.Status {
					matchedIssue = true
					break
				}
			}
			if !matchedIssue {
				continue
			}
		}
		filtered = append(filtered, view)
	}

	start := len(filtered)
	pageOffset := input.Page - 1
	if pageOffset <= len(filtered)/input.PageSize {
		start = pageOffset * input.PageSize
	}
	end := start + input.PageSize
	if end > len(filtered) {
		end = len(filtered)
	}
	return GroupRoutingProfileTargetPage{
		Items:    filtered[start:end],
		Summary:  summary,
		Facets:   facets,
		Page:     input.Page,
		PageSize: input.PageSize,
		Total:    len(filtered),
	}, nil
}

func PreviewGroupRoutingProfileSummaries(profiles map[string]ratio_setting.GroupRoutingRequirements) (map[string]GroupRoutingProfileSummary, error) {
	return previewGroupRoutingProfileSummariesWithDB(model.DB, profiles)
}

func previewGroupRoutingProfileSummariesWithDB(db *gorm.DB, profiles map[string]ratio_setting.GroupRoutingRequirements) (map[string]GroupRoutingProfileSummary, error) {
	normalized, err := normalizeGroupRoutingProfiles(profiles)
	if err != nil {
		return nil, err
	}
	dynamicGroups := make([]string, 0, len(normalized))
	for groupName, profile := range normalized {
		if profile.IsDynamic() {
			dynamicGroups = append(dynamicGroups, groupName)
		}
	}
	sort.Strings(dynamicGroups)
	result := make(map[string]GroupRoutingProfileSummary, len(dynamicGroups))
	if len(dynamicGroups) == 0 {
		return result, nil
	}

	catalog, err := loadGroupRoutingProfileCatalogWithDB(db, ratio_setting.GroupRoutingSourceDefault, false)
	if err != nil {
		return nil, err
	}
	for _, groupName := range dynamicGroups {
		profile := normalized[groupName]
		result[groupName] = summarizeGroupRoutingProfile(profile, evaluateGroupRoutingProfileCatalog(profile, catalog))
	}
	return result, nil
}

func ValidateActiveGroupRoutingProfiles(raw string) error {
	return ValidateActiveGroupRoutingProfilesWithDB(model.DB, raw)
}

func ValidateActiveGroupRoutingProfilesWithDB(db *gorm.DB, raw string) error {
	if db == nil {
		return newGroupRoutingProfileError(GroupRoutingProfileErrorPreview, errors.New("group routing profile database is required"))
	}
	profiles, err := ratio_setting.ParseGroupRoutingRequirementsJSONString(raw)
	if err != nil {
		return newGroupRoutingProfileError(GroupRoutingProfileErrorInvalid, err)
	}
	knownGroups := ratio_setting.GetGroupRatioCopy()
	for groupName := range setting.GetUserUsableGroupsCopy() {
		if _, exists := knownGroups[groupName]; !exists {
			knownGroups[groupName] = 1
		}
	}
	activeProfiles := make(map[string]ratio_setting.GroupRoutingRequirements)
	for groupName, profile := range profiles {
		if !profile.IsDynamic() || profile.Status != ratio_setting.GroupRoutingProfileActive {
			continue
		}
		if _, exists := knownGroups[groupName]; !exists {
			return newGroupRoutingProfileError(GroupRoutingProfileErrorInvalid, fmt.Errorf("group routing profile %q references an unknown group", groupName))
		}
		activeProfiles[groupName] = profile
	}
	if len(activeProfiles) == 0 {
		return nil
	}
	summaries, err := previewGroupRoutingProfileSummariesWithDB(db, activeProfiles)
	if err != nil {
		return err
	}
	groupNames := make([]string, 0, len(summaries))
	for groupName := range summaries {
		groupNames = append(groupNames, groupName)
	}
	sort.Strings(groupNames)
	for _, groupName := range groupNames {
		if summaries[groupName].MatchedTargets == 0 {
			return newGroupRoutingProfileError(GroupRoutingProfileErrorUnavailable, fmt.Errorf("group routing profile %q has no compatible targets", groupName))
		}
	}
	return nil
}

func normalizeGroupRoutingProfilePreviewInput(input GroupRoutingProfilePreviewInput) (GroupRoutingProfilePreviewInput, error) {
	if input.Page == 0 {
		input.Page = 1
	}
	if input.PageSize == 0 {
		input.PageSize = 25
	}
	if input.Page < 1 {
		return input, newGroupRoutingProfileError(GroupRoutingProfileErrorInvalid, errors.New("page must be greater than zero"))
	}
	if input.PageSize != 25 && input.PageSize != 50 && input.PageSize != 100 {
		return input, newGroupRoutingProfileError(GroupRoutingProfileErrorInvalid, errors.New("page size must be one of 25, 50, or 100"))
	}
	if input.ChannelID < 0 {
		return input, newGroupRoutingProfileError(GroupRoutingProfileErrorInvalid, errors.New("channel id must not be negative"))
	}
	if input.CostMode != "" && !validGroupRoutingCostMode(input.CostMode) {
		return input, newGroupRoutingProfileError(GroupRoutingProfileErrorInvalid, fmt.Errorf("invalid cost mode %q", input.CostMode))
	}
	if input.Status != "" && !validGroupRoutingTargetStatus(input.Status) {
		return input, newGroupRoutingProfileError(GroupRoutingProfileErrorInvalid, fmt.Errorf("invalid target status %q", input.Status))
	}
	normalized, err := normalizeGroupRoutingProfiles(map[string]ratio_setting.GroupRoutingRequirements{input.GroupName: input.Profile})
	if err != nil {
		return input, err
	}
	input.Profile = normalized[input.GroupName]
	if !input.Profile.IsDynamic() {
		return input, newGroupRoutingProfileError(GroupRoutingProfileErrorInvalid, fmt.Errorf("group routing profile %q is not dynamic", input.GroupName))
	}
	input.Model = modelrouting.NormalizeCanonicalModel(input.Model)
	return input, nil
}

func normalizeGroupRoutingProfiles(profiles map[string]ratio_setting.GroupRoutingRequirements) (map[string]ratio_setting.GroupRoutingRequirements, error) {
	encoded, err := common.Marshal(profiles)
	if err != nil {
		return nil, newGroupRoutingProfileError(GroupRoutingProfileErrorInvalid, err)
	}
	normalized, err := ratio_setting.ParseGroupRoutingRequirementsJSONString(string(encoded))
	if err != nil {
		return nil, newGroupRoutingProfileError(GroupRoutingProfileErrorInvalid, err)
	}
	return normalized, nil
}

func loadGroupRoutingProfileCatalog(sourceGroup string, includeChannelNames bool) (groupRoutingProfileCatalog, error) {
	return loadGroupRoutingProfileCatalogWithDB(model.DB, sourceGroup, includeChannelNames)
}

func loadGroupRoutingProfileCatalogWithDB(db *gorm.DB, sourceGroup string, includeChannelNames bool) (groupRoutingProfileCatalog, error) {
	policies, err := model.ListEnabledRoutingPoliciesByGroupWithDB(db, sourceGroup)
	if err != nil {
		return groupRoutingProfileCatalog{}, newGroupRoutingProfileError(GroupRoutingProfileErrorPreview, err)
	}
	candidates := make([]CostRuleCandidate, 0)
	models := make([]string, 0, len(policies))
	snapshots := make([]modelrouting.PolicySnapshot, 0, len(policies))
	allTargets := make([]model.RouteTarget, 0)
	for index := range policies {
		models = append(models, policies[index].Model)
		allTargets = append(allTargets, policies[index].Targets...)
		for _, target := range policies[index].Targets {
			candidates = append(candidates, CostRuleCandidate{
				ChannelID:             target.ChannelID,
				BillableUpstreamModel: target.UpstreamModel,
				CostVariantKey:        target.CostVariantKey,
			})
		}
		snapshot, err := model.RoutingPolicySnapshotFromRows(policies[index], policies[index].Targets)
		if err != nil {
			return groupRoutingProfileCatalog{}, newGroupRoutingProfileError(GroupRoutingProfileErrorPreview, err)
		}
		snapshots = append(snapshots, snapshot)
	}
	rules, err := ActiveCostRulesWithDB(db, candidates)
	if err != nil {
		return groupRoutingProfileCatalog{}, newGroupRoutingProfileError(GroupRoutingProfileErrorPreview, err)
	}
	available, err := model.ListRoutingAvailabilityWithDB(db, sourceGroup, models)
	if err != nil {
		return groupRoutingProfileCatalog{}, newGroupRoutingProfileError(GroupRoutingProfileErrorPreview, err)
	}
	channelNames := make(map[int]string)
	if includeChannelNames {
		channelIDs := make([]int, 0)
		seenChannelIDs := make(map[int]struct{})
		for _, target := range allTargets {
			if _, exists := seenChannelIDs[target.ChannelID]; exists {
				continue
			}
			seenChannelIDs[target.ChannelID] = struct{}{}
			channelIDs = append(channelIDs, target.ChannelID)
		}
		if len(channelIDs) > 0 {
			var channels []model.Channel
			if err := db.Select("id", "name").Where("id IN ?", channelIDs).Find(&channels).Error; err != nil {
				return groupRoutingProfileCatalog{}, newGroupRoutingProfileError(GroupRoutingProfileErrorPreview, err)
			}
			for _, channel := range channels {
				channelNames[channel.Id] = channel.Name
			}
		}
	}
	return groupRoutingProfileCatalog{snapshots: snapshots, rules: rules, available: available, channels: channelNames}, nil
}

func evaluateGroupRoutingProfileCatalog(profile ratio_setting.GroupRoutingRequirements, catalog groupRoutingProfileCatalog) []GroupRoutingProfileEvaluation {
	evaluations := make([]GroupRoutingProfileEvaluation, 0, len(catalog.snapshots))
	for _, snapshot := range catalog.snapshots {
		evaluations = append(evaluations, EvaluateGroupRoutingProfile(profile, snapshot, catalog.rules, catalog.available))
	}
	return evaluations
}

func summarizeGroupRoutingProfile(profile ratio_setting.GroupRoutingRequirements, evaluations []GroupRoutingProfileEvaluation) GroupRoutingProfileSummary {
	summary := GroupRoutingProfileSummary{Models: len(evaluations)}
	liveTargetKeys := make(map[string]struct{})
	for _, evaluation := range evaluations {
		matchedModel := false
		for _, target := range evaluation.Targets {
			summary.Targets++
			liveTargetKeys[target.TargetKey] = struct{}{}
			if target.Eligible {
				summary.MatchedTargets++
				matchedModel = true
			}
		}
		if matchedModel {
			summary.MatchedModels++
		}
	}
	for _, targetKey := range profile.ExcludedTargetKeys {
		if _, exists := liveTargetKeys[targetKey]; !exists {
			summary.StaleExclusions++
		}
	}
	return summary
}

func groupRoutingProfileTargetViews(evaluations []GroupRoutingProfileEvaluation, channelNames map[int]string) []GroupRoutingProfileTargetView {
	views := make([]GroupRoutingProfileTargetView, 0)
	for _, evaluation := range evaluations {
		for _, target := range evaluation.Targets {
			views = append(views, GroupRoutingProfileTargetView{
				Model:              evaluation.Snapshot.CanonicalModel,
				ChannelID:          target.Target.ChannelID,
				ChannelName:        channelNames[target.Target.ChannelID],
				TargetName:         target.Target.Name,
				UpstreamModel:      target.Target.UpstreamModel,
				CostVariantKey:     target.Target.CostVariantKey,
				TargetPriority:     target.Target.Priority,
				SupportsRealPerson: target.Target.Constraints.SupportsRealPerson,
				CostMode:           target.CostMode,
				CostRuleID:         target.CostRuleID,
				CostRuleVersion:    target.CostVersion,
				TargetKey:          target.TargetKey,
				Status:             target.Status,
				Issues:             append([]GroupRoutingTargetStatus(nil), target.Issues...),
			})
		}
	}
	sort.Slice(views, func(i, j int) bool {
		if views[i].Model != views[j].Model {
			return views[i].Model < views[j].Model
		}
		if views[i].ChannelID != views[j].ChannelID {
			return views[i].ChannelID < views[j].ChannelID
		}
		if views[i].TargetPriority != views[j].TargetPriority {
			return views[i].TargetPriority > views[j].TargetPriority
		}
		if views[i].TargetName != views[j].TargetName {
			return views[i].TargetName < views[j].TargetName
		}
		return views[i].TargetKey < views[j].TargetKey
	})
	return views
}

func groupRoutingProfileFacets(views []GroupRoutingProfileTargetView) GroupRoutingProfileFacets {
	modelSet := make(map[string]struct{})
	channelSet := make(map[int]string)
	costModeSet := make(map[types.CostMode]struct{})
	statusSet := make(map[GroupRoutingTargetStatus]struct{})
	for _, view := range views {
		modelSet[view.Model] = struct{}{}
		channelSet[view.ChannelID] = view.ChannelName
		if view.CostMode != "" {
			costModeSet[view.CostMode] = struct{}{}
		}
		statusSet[view.Status] = struct{}{}
		for _, issue := range view.Issues {
			statusSet[issue] = struct{}{}
		}
	}
	facets := GroupRoutingProfileFacets{
		Models:    make([]string, 0, len(modelSet)),
		Channels:  make([]GroupRoutingProfileChannelFacet, 0, len(channelSet)),
		CostModes: make([]types.CostMode, 0, len(costModeSet)),
		Statuses:  make([]GroupRoutingTargetStatus, 0, len(statusSet)),
	}
	for modelName := range modelSet {
		facets.Models = append(facets.Models, modelName)
	}
	for channelID, channelName := range channelSet {
		facets.Channels = append(facets.Channels, GroupRoutingProfileChannelFacet{ID: channelID, Name: channelName})
	}
	for costMode := range costModeSet {
		facets.CostModes = append(facets.CostModes, costMode)
	}
	for status := range statusSet {
		facets.Statuses = append(facets.Statuses, status)
	}
	sort.Strings(facets.Models)
	sort.Slice(facets.Channels, func(i, j int) bool { return facets.Channels[i].ID < facets.Channels[j].ID })
	sort.Slice(facets.CostModes, func(i, j int) bool { return facets.CostModes[i] < facets.CostModes[j] })
	sort.Slice(facets.Statuses, func(i, j int) bool { return facets.Statuses[i] < facets.Statuses[j] })
	return facets
}

func validGroupRoutingCostMode(mode types.CostMode) bool {
	switch mode {
	case types.CostModeFree, types.CostModePerRequest, types.CostModePerDuration, types.CostModePerToken:
		return true
	default:
		return false
	}
}

func validGroupRoutingTargetStatus(status GroupRoutingTargetStatus) bool {
	switch status {
	case GroupRoutingTargetMatched,
		GroupRoutingTargetRealPersonMismatch,
		GroupRoutingTargetRealPersonUnknown,
		GroupRoutingTargetCostModeMismatch,
		GroupRoutingTargetCostRuleMissing,
		GroupRoutingTargetExcluded,
		GroupRoutingTargetDisabled,
		GroupRoutingTargetChannelUnavailable:
		return true
	default:
		return false
	}
}

func newGroupRoutingProfileError(code string, err error) *GroupRoutingProfileError {
	return &GroupRoutingProfileError{Code: code, Err: err}
}
