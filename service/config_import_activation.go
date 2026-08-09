package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelrouting"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type configImportActivationPolicyPlan struct {
	Policy     model.RoutingPolicy
	Defaults   modelrouting.Defaults
	MergeMode  types.ConfigImportRouteMergeMode
	EnableIDs  []int
	RetireIDs  []int
	ChannelIDs []int
	Snapshot   modelrouting.PolicySnapshot
}

type configImportActivationPlan struct {
	Batch         model.ConfigImportBatch
	Policies      []configImportActivationPolicyPlan
	ChannelIDs    []int
	Blockers      []dto.ConfigImportActivationBlocker
	BeforeSHA256  string
	CurrentSHA256 string
}

type configImportActivationTargetMetadata struct {
	LineRef string
	Target  types.ConfigImportRouteTarget
}

type configImportActivationAuditInput struct {
	Preview        dto.ConfigImportActivationPreview
	BeforeSHA256   string
	AfterSHA256    string
	FailureCode    string
	FailureMessage string
}

var refreshConfigImportActivation = func(keys ConfigImportRefreshKeys) error {
	if err := RefreshPublishedConfig(keys); err != nil {
		return err
	}
	ResetProxyClientCache()
	return nil
}

func PreviewConfigImportBatchActivation(ctx context.Context, batchID int64) (*dto.ConfigImportActivationPreview, error) {
	if batchID <= 0 {
		return nil, configImportError("SCHEMA_BATCH_ID", "batch ID is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	plan, err := buildConfigImportActivationPlan(model.DB.WithContext(ctx), batchID, false)
	if err != nil {
		return nil, err
	}
	preview := configImportActivationPreview(plan)
	return &preview, nil
}

func ActivateConfigImportBatch(ctx context.Context, batchID int64, adminID int) (*dto.ConfigImportBatchDetail, error) {
	if adminID <= 0 {
		return nil, configImportError("SCHEMA_ADMIN", "admin ID is required")
	}
	if batchID <= 0 {
		return nil, configImportError("SCHEMA_BATCH_ID", "batch ID is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	refresh := ConfigImportRefreshKeys{}
	alreadyActivated := false
	var rejectedAudit *configImportActivationAuditInput
	err := model.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var batch model.ConfigImportBatch
		if err := model.LockModelForUpdate(tx, &model.ConfigImportBatch{}).Where("id = ?", batchID).First(&batch).Error; err != nil {
			return err
		}
		if batch.ActivatedAt != nil {
			alreadyActivated = true
			return nil
		}
		plan, err := buildConfigImportActivationPlan(tx, batchID, true)
		if err != nil {
			return err
		}
		preview := configImportActivationPreview(plan)
		if len(plan.Blockers) > 0 {
			rejectedAudit = &configImportActivationAuditInput{
				Preview: preview, BeforeSHA256: plan.BeforeSHA256, AfterSHA256: plan.CurrentSHA256,
				FailureCode: "ACTIVATION_BLOCKED", FailureMessage: fmt.Sprintf("activation blocked by %d checks", len(plan.Blockers)),
			}
			return configImportErrorWithData("ACTIVATION_BLOCKED", preview, "batch %d activation is blocked", batchID)
		}

		now := common.GetTimestamp()
		var items []model.ConfigImportItem
		if err := tx.Where("batch_id = ?", batchID).Order("entity_type ASC, business_id ASC, id ASC").Find(&items).Error; err != nil {
			return err
		}
		if err := publishConfigImportAuthoritativeCostRules(tx, items, &refresh); err != nil {
			return err
		}
		for _, item := range items {
			if item.EntityType != "cost_rule_drafts" || item.MaterializedID == nil ||
				item.State == string(types.ConfigImportItemStateExcluded) || item.State == string(types.ConfigImportItemStateUnchanged) {
				continue
			}
			if *item.MaterializedID <= 0 {
				return configImportError("ACTIVATION_COST_DRAFT_ID", "cost draft item %d has an invalid materialized ID", item.ID)
			}
			activated, err := model.ActivateChannelModelCostRuleWithTx(tx, int64(*item.MaterializedID), adminID, now, nil)
			if err != nil {
				return err
			}
			refresh.CostModelKeys = appendConfigImportRefreshString(
				refresh.CostModelKeys,
				fmt.Sprintf("%d|%s|%s", activated.ChannelID, activated.BillableUpstreamModel, activated.CostVariantKey),
			)
		}
		if err := publishConfigImportSaleOptions(tx, items, &refresh); err != nil {
			return err
		}
		if err := publishConfigImportModelMappings(tx, items, &refresh); err != nil {
			return err
		}
		for _, policyPlan := range plan.Policies {
			if len(policyPlan.RetireIDs) > 0 {
				result := tx.Model(&model.RouteTarget{}).
					Where("id IN ? AND managed_by = ?", policyPlan.RetireIDs, string(types.RouteTargetManagedByConfigImport)).
					Updates(map[string]any{"enabled": false, "retired_at": now, "updated_at": now})
				if result.Error != nil {
					return result.Error
				}
				if result.RowsAffected != int64(len(policyPlan.RetireIDs)) {
					return configImportError("ACTIVATION_CONCURRENT", "route retirement set changed concurrently")
				}
			}
			if len(policyPlan.EnableIDs) > 0 {
				result := tx.Model(&model.RouteTarget{}).
					Where("id IN ? AND source_batch_id = ?", policyPlan.EnableIDs, batchID).
					Updates(map[string]any{"enabled": true, "retired_at": nil, "updated_at": now})
				if result.Error != nil {
					return result.Error
				}
				if result.RowsAffected != int64(len(policyPlan.EnableIDs)) {
					return configImportError("ACTIVATION_CONCURRENT", "route activation set changed concurrently")
				}
			}
			result := tx.Model(&model.RoutingPolicy{}).Where("id = ?", policyPlan.Policy.ID).Updates(map[string]any{
				"enabled": true, "default_resolution": policyPlan.Defaults.OutputResolution,
				"default_duration": policyPlan.Defaults.DurationSeconds, "default_ratio": policyPlan.Defaults.AspectRatio,
				"updated_at": now,
			})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return configImportError("ACTIVATION_CONCURRENT", "routing policy %d changed concurrently", policyPlan.Policy.ID)
			}
			refresh.RoutingPolicyKeys = appendConfigImportRefreshRoutingKey(
				refresh.RoutingPolicyKeys,
				model.RoutingPolicyKey{GroupName: policyPlan.Policy.GroupName, Model: policyPlan.Policy.Model},
			)
		}
		if len(plan.ChannelIDs) > 0 {
			if err := tx.Model(&model.Channel{}).
				Where("id IN ? AND status = ?", plan.ChannelIDs, common.ChannelStatusManuallyDisabled).
				Update("status", common.ChannelStatusEnabled).Error; err != nil {
				return err
			}
			var channels []model.Channel
			if err := model.LockChannelsForUpdate(tx).Where("id IN ?", plan.ChannelIDs).Order("id ASC").Find(&channels).Error; err != nil {
				return err
			}
			if len(channels) != len(plan.ChannelIDs) {
				return configImportError("ACTIVATION_CONCURRENT", "activation channel set changed concurrently")
			}
			for _, channel := range channels {
				if channel.Status != common.ChannelStatusEnabled {
					return configImportError("ACTIVATION_CONCURRENT", "channel %d is not enabled", channel.Id)
				}
			}
			if err := tx.Model(&model.Ability{}).Where("channel_id IN ?", plan.ChannelIDs).Update("enabled", true).Error; err != nil {
				return err
			}
		}
		for _, channelID := range plan.ChannelIDs {
			refresh.ChannelIDs = appendConfigImportRefreshInt(refresh.ChannelIDs, channelID)
		}
		if err := publishConfigImportGroupRoutingRequirements(tx, items, &refresh); err != nil {
			return err
		}

		after, err := CaptureConfigImportBaseline(tx, batchID)
		if err != nil {
			return err
		}
		if err := appendConfigImportActivationAudit(
			tx, batchID, adminID, "activated", plan.BeforeSHA256, after.Hash, preview, "", "",
		); err != nil {
			return err
		}
		result := tx.Model(&model.ConfigImportBatch{}).Where("id = ? AND activated_at IS NULL", batchID).
			Updates(map[string]any{"activated_at": now, "updated_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return configImportError("ACTIVATION_CONCURRENT", "batch %d activation changed concurrently", batchID)
		}
		return nil
	})
	if err != nil {
		if rejectedAudit != nil {
			auditErr := model.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
				return appendConfigImportActivationAudit(
					tx, batchID, adminID, "rejected", rejectedAudit.BeforeSHA256, rejectedAudit.AfterSHA256,
					rejectedAudit.Preview, rejectedAudit.FailureCode, rejectedAudit.FailureMessage,
				)
			})
			if auditErr != nil {
				common.SysError(fmt.Sprintf("failed to append rejected activation audit for batch %d: %v", batchID, auditErr))
			}
		}
		return nil, err
	}
	if alreadyActivated {
		return GetConfigImportBatch(ctx, batchID)
	}
	if err := refreshConfigImportActivation(refresh); err != nil {
		if markErr := markConfigImportActivationCacheRefreshPending(ctx, batchID); markErr != nil {
			common.SysError(fmt.Sprintf("failed to record pending activation cache refresh for batch %d: %v", batchID, markErr))
		}
		if auditErr := appendConfigImportActivationCacheAudit(ctx, batchID, adminID, "cache_refresh_pending"); auditErr != nil {
			common.SysError(fmt.Sprintf("failed to append pending activation cache audit for batch %d: %v", batchID, auditErr))
		}
		return nil, configImportErrorWithData(
			"ACTIVATION_CACHE_REFRESH_PENDING",
			map[string]any{"batch_id": batchID, "activated": true},
			"batch %d activated but cache refresh is pending",
			batchID,
		)
	}
	if err := recordPostActivationCostCoverage(ctx, batchID, refresh); err != nil {
		common.SysError(fmt.Sprintf("failed to record post-activation cost coverage for batch %d: %v", batchID, err))
	}
	return GetConfigImportBatch(ctx, batchID)
}

func publishConfigImportGroupRoutingRequirements(tx *gorm.DB, items []model.ConfigImportItem, refresh *ConfigImportRefreshKeys) error {
	if tx == nil || !tx.Migrator().HasTable(&model.Option{}) {
		return nil
	}
	requirementsByGroup := make(map[string]ratio_setting.GroupRoutingRequirements)
	var option model.Option
	err := tx.Where(clause.Eq{Column: clause.Column{Name: "key"}, Value: "GroupRoutingRequirements"}).First(&option).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if err == nil && strings.TrimSpace(option.Value) != "" {
		var decodeErr error
		requirementsByGroup, decodeErr = ratio_setting.ParseGroupRoutingRequirementsJSONString(option.Value)
		if decodeErr != nil {
			return configImportError("ACTIVATION_GROUP_ROUTING_REQUIREMENT_OPTION", "GroupRoutingRequirements is not a JSON object: %v", decodeErr)
		}
	}
	knownGroups := ratio_setting.GetGroupRatioCopy()
	for groupName := range setting.GetUserUsableGroupsCopy() {
		knownGroups[groupName] = 1
	}
	changed := false
	for _, item := range items {
		if item.EntityType != "group_routing_requirements" || item.State == string(types.ConfigImportItemStateExcluded) || item.State == string(types.ConfigImportItemStateUnchanged) {
			continue
		}
		var imported types.ConfigImportGroupRoutingRequirement
		if decodeErr := common.UnmarshalJsonStr(item.CanonicalJSON, &imported); decodeErr != nil {
			return decodeErr
		}
		groupName := strings.TrimSpace(imported.GroupName)
		if _, known := knownGroups[groupName]; !known {
			return configImportError(configImportIssueGroupRoutingRequirementUnknown, "group routing requirement references unknown group %q", groupName)
		}
		importedValue, normalizeErr := groupRoutingRequirementsFromImport(groupName, imported.Requirements)
		if normalizeErr != nil {
			return configImportError("ACTIVATION_GROUP_ROUTING_REQUIREMENT", "group routing requirement %q is invalid: %v", groupName, normalizeErr)
		}
		importedValue.ExcludedTargetKeys = append([]string(nil), requirementsByGroup[groupName].ExcludedTargetKeys...)
		requirementsByGroup[groupName] = importedValue
		changed = true
	}
	if !changed {
		return nil
	}
	encoded, err := common.Marshal(requirementsByGroup)
	if err != nil {
		return err
	}
	if err := ValidateActiveGroupRoutingProfilesWithDB(tx, string(encoded)); err != nil {
		return err
	}
	if err := model.UpdateOptionsWithTx(tx, map[string]string{"GroupRoutingRequirements": string(encoded)}); err != nil {
		return err
	}
	refresh.OptionKeys = appendConfigImportRefreshString(refresh.OptionKeys, "GroupRoutingRequirements")
	return nil
}

func configImportActivationPreview(plan *configImportActivationPlan) dto.ConfigImportActivationPreview {
	targetCount := 0
	retireTargetCount := 0
	for _, policy := range plan.Policies {
		targetCount += len(policy.EnableIDs)
		retireTargetCount += len(policy.RetireIDs)
	}
	return dto.ConfigImportActivationPreview{
		Ready:             len(plan.Blockers) == 0,
		ChannelCount:      len(plan.ChannelIDs),
		PolicyCount:       len(plan.Policies),
		TargetCount:       targetCount,
		RetireTargetCount: retireTargetCount,
		Blockers:          plan.Blockers,
	}
}

func buildConfigImportActivationPlan(tx *gorm.DB, batchID int64, lock bool) (*configImportActivationPlan, error) {
	if tx == nil {
		return nil, errors.New("config import activation database is required")
	}
	plan := &configImportActivationPlan{Blockers: []dto.ConfigImportActivationBlocker{}}

	batchQuery := tx.Model(&model.ConfigImportBatch{})
	if lock {
		batchQuery = model.LockModelForUpdate(tx, &model.ConfigImportBatch{})
	}
	if err := batchQuery.Where("id = ?", batchID).First(&plan.Batch).Error; err != nil {
		return nil, err
	}

	var items []model.ConfigImportItem
	itemsQuery := tx.Model(&model.ConfigImportItem{})
	if lock {
		itemsQuery = model.LockModelForUpdate(tx, &model.ConfigImportItem{})
	}
	if err := itemsQuery.Where("batch_id = ?", batchID).Order("id ASC").Find(&items).Error; err != nil {
		return nil, err
	}
	var bindings []model.ConfigImportBinding
	bindingsQuery := tx.Model(&model.ConfigImportBinding{})
	if lock {
		bindingsQuery = model.LockModelForUpdate(tx, &model.ConfigImportBinding{})
	}
	if err := bindingsQuery.Where("batch_id = ?", batchID).Order("id ASC").Find(&bindings).Error; err != nil {
		return nil, err
	}
	var issues []model.ConfigImportIssue
	issuesQuery := tx.Model(&model.ConfigImportIssue{})
	if lock {
		issuesQuery = model.LockModelForUpdate(tx, &model.ConfigImportIssue{})
	}
	if err := issuesQuery.Where("batch_id = ?", batchID).Order("id ASC").Find(&issues).Error; err != nil {
		return nil, err
	}

	if types.ConfigImportBatchStatus(plan.Batch.Status) != types.ConfigImportBatchStatusPublished || plan.Batch.ActivatedAt != nil {
		plan.Blockers = append(plan.Blockers, dto.ConfigImportActivationBlocker{
			Code: "ACTIVATION_BATCH_STATUS", Message: "config import batch must be published and not already activated",
		})
	}
	for _, issue := range issues {
		if issue.ResolutionStatus != "open" {
			continue
		}
		if issue.Severity != string(types.ConfigImportIssueSeverityError) && issue.Severity != string(types.ConfigImportIssueSeverityWarning) {
			continue
		}
		plan.Blockers = append(plan.Blockers, dto.ConfigImportActivationBlocker{
			Code: "ACTIVATION_OPEN_ISSUES", Message: "config import batch has unresolved warning or error issues",
		})
		break
	}

	var baseline ConfigImportBaseline
	if err := common.UnmarshalJsonStr(string(plan.Batch.BaselineJSON), &baseline); err != nil {
		return nil, fmt.Errorf("decode activation baseline for batch %d: %w", batchID, err)
	}
	plan.BeforeSHA256 = baseline.Hash

	bindingsByLine := make(map[string]model.ConfigImportBinding, len(bindings))
	for _, binding := range bindings {
		bindingsByLine[binding.LineRef] = binding
	}

	routeMetadata := make(map[string]configImportActivationTargetMetadata)
	routeLineRefs := make(map[string]struct{})
	for _, item := range items {
		if item.EntityType != "route_blueprints" || item.State == string(types.ConfigImportItemStateExcluded) {
			continue
		}
		var blueprint types.ConfigImportRouteBlueprint
		if err := common.UnmarshalJsonStr(item.CanonicalJSON, &blueprint); err != nil {
			return nil, err
		}
		mergeMode := blueprint.MergeMode
		if mergeMode == "" {
			mergeMode = types.ConfigImportRouteMergeModeMerge
		}
		if mergeMode == types.ConfigImportRouteMergeModeSkip {
			continue
		}
		for _, target := range blueprint.Targets {
			routeMetadata[target.RouteTargetRef] = configImportActivationTargetMetadata{LineRef: target.LineRef, Target: target}
			routeLineRefs[target.LineRef] = struct{}{}
		}
	}
	lineRefs := make([]string, 0, len(routeLineRefs))
	for lineRef := range routeLineRefs {
		lineRefs = append(lineRefs, lineRef)
	}
	sort.Strings(lineRefs)
	hasBindingBlocker := false
	for _, lineRef := range lineRefs {
		binding, found := bindingsByLine[lineRef]
		if found && binding.Action != string(types.ConfigImportBindingActionSkip) && binding.ChannelID != nil &&
			binding.CredentialsConfirmedAt != nil && binding.CredentialsConfirmedBy > 0 {
			continue
		}
		plan.Blockers = append(plan.Blockers, dto.ConfigImportActivationBlocker{
			Code: "ACTIVATION_CREDENTIALS_UNCONFIRMED", Message: fmt.Sprintf("line %q requires a confirmed channel binding", lineRef), LineRef: lineRef,
		})
		hasBindingBlocker = true
	}
	if hasBindingBlocker {
		plan.CurrentSHA256 = plan.BeforeSHA256
		plan.Blockers = normalizeConfigImportActivationBlockers(plan.Blockers)
		return plan, nil
	}
	current, err := CaptureConfigImportBaseline(tx, batchID)
	if err != nil {
		return nil, err
	}
	plan.CurrentSHA256 = current.Hash
	if baseline.Hash == "" || baseline.Hash != current.Hash {
		plan.Blockers = append(plan.Blockers, dto.ConfigImportActivationBlocker{
			Code: "ACTIVATION_STALE_BASE_VERSION", Message: "active configuration changed after the batch was published",
		})
	}

	publishedPlans, err := buildConfigImportPublishedRoutePlans(tx, items)
	if err != nil {
		return nil, err
	}
	policyModels := make([]string, 0, len(publishedPlans))
	seenPolicyModels := make(map[string]struct{}, len(publishedPlans))
	for _, publishedPlan := range publishedPlans {
		if _, exists := seenPolicyModels[publishedPlan.PolicyKey.Model]; exists {
			continue
		}
		seenPolicyModels[publishedPlan.PolicyKey.Model] = struct{}{}
		policyModels = append(policyModels, publishedPlan.PolicyKey.Model)
	}
	sort.Strings(policyModels)
	var policies []model.RoutingPolicy
	if len(policyModels) > 0 {
		policiesQuery := tx.Model(&model.RoutingPolicy{})
		if lock {
			policiesQuery = model.LockModelForUpdate(tx, &model.RoutingPolicy{})
		}
		if err := policiesQuery.Where("model IN ?", policyModels).Order("group_name ASC, model ASC, id ASC").Find(&policies).Error; err != nil {
			return nil, err
		}
	}
	requestedPolicyKeys := make(map[model.RoutingPolicyKey]struct{}, len(publishedPlans))
	for _, publishedPlan := range publishedPlans {
		requestedPolicyKeys[publishedPlan.PolicyKey] = struct{}{}
	}
	policiesByKey := make(map[model.RoutingPolicyKey]model.RoutingPolicy, len(policies))
	policyIDs := make([]int, 0, len(policies))
	for _, policy := range policies {
		key := model.RoutingPolicyKey{GroupName: policy.GroupName, Model: policy.Model}
		if _, requested := requestedPolicyKeys[key]; !requested {
			continue
		}
		policiesByKey[key] = policy
		policyIDs = append(policyIDs, policy.ID)
	}
	sort.Ints(policyIDs)
	var targets []model.RouteTarget
	if len(policyIDs) > 0 {
		targetsQuery := tx.Model(&model.RouteTarget{})
		if lock {
			targetsQuery = model.LockModelForUpdate(tx, &model.RouteTarget{})
		}
		if err := targetsQuery.Where("policy_id IN ?", policyIDs).Order("id ASC").Find(&targets).Error; err != nil {
			return nil, err
		}
	}
	targetsByPolicy := make(map[int][]model.RouteTarget, len(policies))
	for _, target := range targets {
		targetsByPolicy[target.PolicyID] = append(targetsByPolicy[target.PolicyID], target)
	}
	channelIDs := make([]int, 0)
	for _, publishedPlan := range publishedPlans {
		for _, target := range publishedPlan.Targets {
			channelIDs = appendConfigImportRefreshInt(channelIDs, target.ChannelID)
		}
	}
	sort.Ints(channelIDs)

	mappings := make(map[string]struct{})
	costItemsByTarget := make(map[string]model.ConfigImportItem)
	costDraftIDs := make([]int64, 0)
	for _, item := range items {
		if item.State == string(types.ConfigImportItemStateExcluded) {
			continue
		}
		switch item.EntityType {
		case "model_mappings":
			var mapping types.ConfigImportModelMapping
			if err := common.UnmarshalJsonStr(item.CanonicalJSON, &mapping); err != nil {
				return nil, err
			}
			key := fmt.Sprintf("%s|%s|%s", mapping.LineRef, configImportRuntimeCanonicalModel(mapping.CanonicalModel), strings.TrimSpace(mapping.UpstreamModel))
			mappings[key] = struct{}{}
		case "cost_rule_drafts":
			var draft types.ConfigImportCostRuleDraft
			if err := common.UnmarshalJsonStr(item.CanonicalJSON, &draft); err != nil {
				return nil, err
			}
			costItemsByTarget[draft.RouteTargetRef] = item
			if item.MaterializedID != nil && *item.MaterializedID > 0 {
				id := int64(*item.MaterializedID)
				costDraftIDs = append(costDraftIDs, id)
			}
		}
	}
	sort.Slice(costDraftIDs, func(i, j int) bool { return costDraftIDs[i] < costDraftIDs[j] })
	var costDrafts []model.ChannelModelCostRule
	if len(costDraftIDs) > 0 || lock && len(channelIDs) > 0 {
		costQuery := tx.Model(&model.ChannelModelCostRule{})
		if lock {
			costQuery = model.LockModelForUpdate(tx, &model.ChannelModelCostRule{})
		}
		switch {
		case len(costDraftIDs) > 0 && lock && len(channelIDs) > 0:
			costQuery = costQuery.Where("id IN ? OR (channel_id IN ? AND status = ?)", costDraftIDs, channelIDs, types.CostRuleActive)
		case len(costDraftIDs) > 0:
			costQuery = costQuery.Where("id IN ?", costDraftIDs)
		default:
			costQuery = costQuery.Where("channel_id IN ? AND status = ?", channelIDs, types.CostRuleActive)
		}
		if err := costQuery.Order("id ASC").Find(&costDrafts).Error; err != nil {
			return nil, err
		}
	}
	costDraftsByID := make(map[int64]model.ChannelModelCostRule, len(costDrafts))
	for _, draft := range costDrafts {
		costDraftsByID[draft.ID] = draft
	}

	var channels []model.Channel
	if len(channelIDs) > 0 {
		channelsQuery := tx.Model(&model.Channel{})
		if lock {
			channelsQuery = model.LockChannelsForUpdate(tx)
		}
		if err := channelsQuery.Where("id IN ?", channelIDs).Order("id ASC").Find(&channels).Error; err != nil {
			return nil, err
		}
	}
	channelsByID := make(map[int]model.Channel, len(channels))
	for _, channel := range channels {
		channelsByID[channel.Id] = channel
	}
	if lock && len(channelIDs) > 0 {
		var abilities []model.Ability
		if err := model.LockModelForUpdate(tx, &model.Ability{}).Where("channel_id IN ?", channelIDs).
			Order(clause.OrderByColumn{Column: clause.Column{Name: "channel_id"}}).
			Order(clause.OrderByColumn{Column: clause.Column{Name: "group"}}).
			Order(clause.OrderByColumn{Column: clause.Column{Name: "model"}}).
			Find(&abilities).Error; err != nil {
			return nil, err
		}
	}
	if lock {
		scope, err := configImportBaselineScopeForBatch(tx, batchID)
		if err != nil {
			return nil, err
		}
		optionKeys := make([]string, 0, len(scope.optionFields))
		for key := range scope.optionFields {
			optionKeys = append(optionKeys, key)
		}
		sort.Strings(optionKeys)
		if len(optionKeys) > 0 {
			var options []model.Option
			if err := configImportActivationOptionsQuery(tx, optionKeys).Find(&options).Error; err != nil {
				return nil, err
			}
		}
	}

	plan.ChannelIDs = channelIDs
	for _, channelID := range channelIDs {
		channel, found := channelsByID[channelID]
		if !found {
			plan.Blockers = append(plan.Blockers, dto.ConfigImportActivationBlocker{
				Code: "ACTIVATION_TARGET_MISSING", Message: fmt.Sprintf("channel %d referenced by an activation target does not exist", channelID), ChannelID: activationIntPointer(channelID),
			})
			continue
		}
		if strings.TrimSpace(channel.Key) == "" {
			plan.Blockers = append(plan.Blockers, dto.ConfigImportActivationBlocker{
				Code: "ACTIVATION_CHANNEL_KEY_MISSING", Message: fmt.Sprintf("channel %d does not have an API key", channelID), ChannelID: activationIntPointer(channelID),
			})
		}
		if channel.Status != common.ChannelStatusEnabled && channel.Status != common.ChannelStatusManuallyDisabled {
			plan.Blockers = append(plan.Blockers, dto.ConfigImportActivationBlocker{
				Code: "ACTIVATION_CHANNEL_AUTO_DISABLED", Message: fmt.Sprintf("channel %d is auto-disabled and must be reviewed", channelID), ChannelID: activationIntPointer(channelID),
			})
		}
	}

	for _, publishedPlan := range publishedPlans {
		policy, found := policiesByKey[publishedPlan.PolicyKey]
		if !found {
			plan.Blockers = append(plan.Blockers, dto.ConfigImportActivationBlocker{
				Code: "ACTIVATION_TARGET_MISSING", Message: fmt.Sprintf("routing policy %s|%s does not exist", publishedPlan.PolicyKey.GroupName, publishedPlan.PolicyKey.Model),
			})
			continue
		}
		mergeMode := publishedPlan.MergeMode
		if mergeMode == "" {
			mergeMode = types.ConfigImportRouteMergeModeMerge
		}
		policyPlan := configImportActivationPolicyPlan{
			Policy: policy, Defaults: publishedPlan.Defaults, MergeMode: mergeMode,
			Snapshot: modelrouting.PolicySnapshot{
				ID: policy.ID, GroupName: policy.GroupName, CanonicalModel: policy.Model, Enabled: true,
				Defaults: publishedPlan.Defaults, TargetsByChannel: make(map[int][]modelrouting.Target),
			},
		}
		currentNames := make(map[string]struct{}, len(publishedPlan.Targets))
		currentTargetIDs := make(map[int]struct{}, len(publishedPlan.Targets))
		policyHasMissingTarget := false
		for _, expected := range publishedPlan.Targets {
			currentNames[expected.Name] = struct{}{}
			matches := make([]model.RouteTarget, 0, 1)
			for _, actual := range targetsByPolicy[policy.ID] {
				if actual.SourceBatchID == nil || *actual.SourceBatchID != batchID || actual.Name != expected.Name {
					continue
				}
				matches = append(matches, actual)
			}
			if len(matches) != 1 || !configImportActivationTargetMatches(matches[0], expected) {
				metadata := routeMetadata[expected.Name]
				plan.Blockers = append(plan.Blockers, dto.ConfigImportActivationBlocker{
					Code: "ACTIVATION_TARGET_MISSING", Message: fmt.Sprintf("route target %q is missing or differs from its published candidate", expected.Name),
					LineRef: metadata.LineRef, RouteTargetRef: expected.Name, ChannelID: activationIntPointer(expected.ChannelID),
				})
				policyHasMissingTarget = true
				continue
			}
			actual := matches[0]
			currentTargetIDs[actual.ID] = struct{}{}
			policyPlan.EnableIDs = append(policyPlan.EnableIDs, actual.ID)
			policyPlan.ChannelIDs = appendConfigImportRefreshInt(policyPlan.ChannelIDs, actual.ChannelID)

			metadata := routeMetadata[actual.Name]
			mappingKey := fmt.Sprintf("%s|%s|%s", metadata.LineRef, policy.Model, strings.TrimSpace(actual.UpstreamModel))
			if _, mapped := mappings[mappingKey]; !mapped {
				plan.Blockers = append(plan.Blockers, dto.ConfigImportActivationBlocker{
					Code: "ACTIVATION_MODEL_MAPPING_MISSING", Message: fmt.Sprintf("route target %q does not have a matching model mapping", actual.Name),
					LineRef: metadata.LineRef, RouteTargetRef: actual.Name, ChannelID: activationIntPointer(actual.ChannelID),
				})
			}
			costItem, hasCostItem := costItemsByTarget[actual.Name]
			var costDraft model.ChannelModelCostRule
			costDraftFound := false
			if hasCostItem && costItem.MaterializedID != nil && *costItem.MaterializedID > 0 {
				costDraft, costDraftFound = costDraftsByID[int64(*costItem.MaterializedID)]
			}
			if !costDraftFound || costDraft.Status != string(types.CostRuleDraft) || costDraft.ChannelID != actual.ChannelID ||
				costDraft.BillableUpstreamModel != actual.UpstreamModel || costDraft.CostVariantKey != actual.CostVariantKey {
				plan.Blockers = append(plan.Blockers, dto.ConfigImportActivationBlocker{
					Code: "ACTIVATION_COST_DRAFT_MISSING", Message: fmt.Sprintf("route target %q does not have its staged cost draft", actual.Name),
					LineRef: metadata.LineRef, RouteTargetRef: actual.Name, ChannelID: activationIntPointer(actual.ChannelID),
				})
			} else if channel, channelFound := channelsByID[actual.ChannelID]; channelFound {
				capabilities, lookupErr := lookupChannelCostCapabilities(channel.Type, "", "")
				if lookupErr != nil {
					plan.Blockers = append(plan.Blockers, dto.ConfigImportActivationBlocker{
						Code: "ACTIVATION_COST_DRAFT_INVALID", Message: lookupErr.Error(), LineRef: metadata.LineRef,
						RouteTargetRef: actual.Name, ChannelID: activationIntPointer(actual.ChannelID),
					})
				} else if _, validateErr := validateCostRuleContract(&costDraft, capabilities); validateErr != nil {
					plan.Blockers = append(plan.Blockers, dto.ConfigImportActivationBlocker{
						Code: "ACTIVATION_COST_DRAFT_INVALID", Message: validateErr.Error(), LineRef: metadata.LineRef,
						RouteTargetRef: actual.Name, ChannelID: activationIntPointer(actual.ChannelID),
					})
				}
			}
			targetSnapshot, targetErr := configImportActivationTargetSnapshot(actual, true)
			if targetErr != nil {
				return nil, targetErr
			}
			if channel, channelFound := channelsByID[actual.ChannelID]; channelFound && RouteTargetContractValidator != nil {
				if contractErr := RouteTargetContractValidator(&channel, targetSnapshot); contractErr != nil {
					plan.Blockers = append(plan.Blockers, dto.ConfigImportActivationBlocker{
						Code: "ACTIVATION_CHANNEL_CONTRACT", Message: contractErr.Error(), LineRef: metadata.LineRef,
						RouteTargetRef: actual.Name, ChannelID: activationIntPointer(actual.ChannelID),
					})
				}
			}
		}
		for _, target := range targetsByPolicy[policy.ID] {
			if target.SourceBatchID == nil || *target.SourceBatchID != batchID {
				continue
			}
			if _, expected := currentTargetIDs[target.ID]; expected {
				continue
			}
			metadata := routeMetadata[target.Name]
			plan.Blockers = append(plan.Blockers, dto.ConfigImportActivationBlocker{
				Code: "ACTIVATION_TARGET_MISSING", Message: fmt.Sprintf("route target %q is not part of the published activation plan", target.Name),
				LineRef: metadata.LineRef, RouteTargetRef: target.Name, ChannelID: activationIntPointer(target.ChannelID),
			})
			policyHasMissingTarget = true
		}
		for _, target := range targetsByPolicy[policy.ID] {
			if target.ManagedBy != string(types.RouteTargetManagedByConfigImport) || target.SourceBatchID == nil || *target.SourceBatchID == batchID || !target.Enabled || target.RetiredAt != nil {
				continue
			}
			_, sameName := currentNames[target.Name]
			if mergeMode == types.ConfigImportRouteMergeModeReplace || sameName {
				policyPlan.RetireIDs = append(policyPlan.RetireIDs, target.ID)
			}
		}
		retireIDs := make(map[int]struct{}, len(policyPlan.RetireIDs))
		for _, targetID := range policyPlan.RetireIDs {
			retireIDs[targetID] = struct{}{}
		}
		for _, target := range targetsByPolicy[policy.ID] {
			enabled := target.Enabled
			if _, currentTarget := currentTargetIDs[target.ID]; currentTarget {
				enabled = true
			}
			if _, retireTarget := retireIDs[target.ID]; retireTarget {
				enabled = false
			}
			targetSnapshot, targetErr := configImportActivationTargetSnapshot(target, enabled)
			if targetErr != nil {
				return nil, targetErr
			}
			policyPlan.Snapshot.TargetsByChannel[target.ChannelID] = append(policyPlan.Snapshot.TargetsByChannel[target.ChannelID], targetSnapshot)
		}
		sort.Ints(policyPlan.EnableIDs)
		sort.Ints(policyPlan.RetireIDs)
		sort.Ints(policyPlan.ChannelIDs)
		if !policyHasMissingTarget {
			if validationErr := modelrouting.ValidatePolicy(policyPlan.Snapshot, relaycommon.MaxTaskDurationSeconds); validationErr != nil {
				plan.Blockers = append(plan.Blockers, dto.ConfigImportActivationBlocker{
					Code: "ACTIVATION_ROUTING_CONFLICT", Message: validationErr.Error(),
				})
			}
		}
		plan.Policies = append(plan.Policies, policyPlan)
	}
	plan.Blockers = normalizeConfigImportActivationBlockers(plan.Blockers)
	return plan, nil
}

func configImportActivationOptionsQuery(tx *gorm.DB, optionKeys []string) *gorm.DB {
	optionValues := make([]any, 0, len(optionKeys))
	for _, optionKey := range optionKeys {
		optionValues = append(optionValues, optionKey)
	}
	return model.LockModelForUpdate(tx, &model.Option{}).
		Where(clause.IN{Column: clause.Column{Name: "key"}, Values: optionValues}).
		Order(clause.OrderByColumn{Column: clause.Column{Name: "key"}})
}

func configImportActivationTargetMatches(actual, expected model.RouteTarget) bool {
	return actual.ChannelID == expected.ChannelID && actual.Name == expected.Name &&
		actual.UpstreamModel == expected.UpstreamModel && actual.CostVariantKey == expected.CostVariantKey &&
		actual.TargetPriority == expected.TargetPriority && configImportActivationOptionalIntEqual(actual.MinimumExpectedMarginBPS, expected.MinimumExpectedMarginBPS) &&
		actual.Constraints == expected.Constraints && !actual.Enabled && actual.RetiredAt == nil &&
		actual.ManagedBy == string(types.RouteTargetManagedByConfigImport)
}

func configImportActivationOptionalIntEqual(left, right *int) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func configImportActivationTargetSnapshot(target model.RouteTarget, enabled bool) (modelrouting.Target, error) {
	var constraints modelrouting.Constraints
	if err := common.UnmarshalJsonStr(target.Constraints, &constraints); err != nil {
		return modelrouting.Target{}, fmt.Errorf("decode route target %q constraints: %w", target.Name, err)
	}
	variant, err := types.NormalizeCostVariantKey(target.CostVariantKey)
	if err != nil {
		return modelrouting.Target{}, fmt.Errorf("normalize route target %q cost variant: %w", target.Name, err)
	}
	return modelrouting.Target{
		ID: target.ID, PolicyID: target.PolicyID, ChannelID: target.ChannelID, Name: target.Name,
		UpstreamModel: target.UpstreamModel, CostVariantKey: variant, Priority: target.TargetPriority,
		MinimumExpectedMarginBPS: target.MinimumExpectedMarginBPS, Enabled: enabled, Constraints: constraints,
	}, nil
}

func normalizeConfigImportActivationBlockers(blockers []dto.ConfigImportActivationBlocker) []dto.ConfigImportActivationBlocker {
	byKey := make(map[string]dto.ConfigImportActivationBlocker, len(blockers))
	keys := make([]string, 0, len(blockers))
	for _, blocker := range blockers {
		channelID := 0
		if blocker.ChannelID != nil {
			channelID = *blocker.ChannelID
		}
		key := fmt.Sprintf("%s|%010d|%s|%s", blocker.Code, channelID, blocker.RouteTargetRef, blocker.LineRef)
		if _, exists := byKey[key]; exists {
			continue
		}
		byKey[key] = blocker
		keys = append(keys, key)
	}
	sort.Strings(keys)
	normalized := make([]dto.ConfigImportActivationBlocker, 0, len(keys))
	for _, key := range keys {
		normalized = append(normalized, byKey[key])
	}
	return normalized
}

func activationIntPointer(value int) *int {
	return &value
}

func appendConfigImportActivationAudit(
	tx *gorm.DB,
	batchID int64,
	adminID int,
	outcome string,
	beforeSHA256 string,
	afterSHA256 string,
	preview dto.ConfigImportActivationPreview,
	failureCode string,
	failureMessage string,
) error {
	if beforeSHA256 == "" || afterSHA256 == "" {
		return configImportError("ACTIVATION_AUDIT_HASH", "activation audit requires before and after hashes")
	}
	type auditBlocker struct {
		Code           string `json:"code"`
		LineRef        string `json:"line_ref,omitempty"`
		RouteTargetRef string `json:"route_target_ref,omitempty"`
		ChannelID      *int   `json:"channel_id,omitempty"`
	}
	blockers := make([]auditBlocker, 0, len(preview.Blockers))
	for _, blocker := range preview.Blockers {
		blockers = append(blockers, auditBlocker{
			Code: blocker.Code, LineRef: blocker.LineRef,
			RouteTargetRef: blocker.RouteTargetRef, ChannelID: blocker.ChannelID,
		})
	}
	summary, err := common.Marshal(struct {
		Ready             bool           `json:"ready"`
		ChannelCount      int            `json:"channel_count"`
		PolicyCount       int            `json:"policy_count"`
		TargetCount       int            `json:"target_count"`
		RetireTargetCount int            `json:"retire_target_count"`
		Blockers          []auditBlocker `json:"blockers"`
	}{
		Ready: preview.Ready, ChannelCount: preview.ChannelCount,
		PolicyCount: preview.PolicyCount, TargetCount: preview.TargetCount,
		RetireTargetCount: preview.RetireTargetCount, Blockers: blockers,
	})
	if err != nil {
		return err
	}
	return tx.Create(&model.ConfigImportActivationAudit{
		BatchID: batchID, AdminID: adminID, Outcome: outcome,
		ChannelCount: preview.ChannelCount, PolicyCount: preview.PolicyCount,
		TargetCount: preview.TargetCount, RetiredTargetCount: preview.RetireTargetCount,
		BeforeSHA256: beforeSHA256, AfterSHA256: afterSHA256,
		FailureCode: failureCode, FailureMessage: failureMessage,
		SummaryJSON: model.ConfigImportSummaryJSON(summary), CreatedAt: common.GetTimestamp(),
	}).Error
}

func markConfigImportActivationCacheRefreshPending(ctx context.Context, batchID int64) error {
	if !model.DB.Migrator().HasTable(&model.ConfigImportIssue{}) {
		return nil
	}
	now := common.GetTimestamp()
	var issue model.ConfigImportIssue
	err := model.DB.WithContext(ctx).Where("batch_id = ? AND code = ?", batchID, "ACTIVATION_CACHE_REFRESH_PENDING").First(&issue).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.DB.WithContext(ctx).Create(&model.ConfigImportIssue{
			BatchID: batchID, Severity: string(types.ConfigImportIssueSeverityWarning), Code: "ACTIVATION_CACHE_REFRESH_PENDING",
			Message: "Activated configuration cache refresh is pending.", ResolutionStatus: "open", CreatedAt: now, UpdatedAt: now,
		}).Error
	}
	if err != nil {
		return err
	}
	return model.DB.WithContext(ctx).Model(&model.ConfigImportIssue{}).Where("id = ?", issue.ID).Updates(map[string]any{
		"severity": string(types.ConfigImportIssueSeverityWarning), "message": "Activated configuration cache refresh is pending.",
		"resolution_status": "open", "updated_at": now,
	}).Error
}

func appendConfigImportActivationCacheAudit(ctx context.Context, batchID int64, adminID int, outcome string) error {
	var activated model.ConfigImportActivationAudit
	if err := model.DB.WithContext(ctx).Where("batch_id = ? AND outcome = ?", batchID, "activated").Order("id DESC").First(&activated).Error; err != nil {
		return err
	}
	preview := dto.ConfigImportActivationPreview{
		Ready: true, ChannelCount: activated.ChannelCount, PolicyCount: activated.PolicyCount,
		TargetCount: activated.TargetCount, RetireTargetCount: activated.RetiredTargetCount,
		Blockers: []dto.ConfigImportActivationBlocker{},
	}
	return model.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return appendConfigImportActivationAudit(
			tx, batchID, adminID, outcome, activated.BeforeSHA256, activated.AfterSHA256, preview, "", "",
		)
	})
}
