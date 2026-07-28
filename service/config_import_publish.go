package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelrouting"
	"github.com/QuantumNous/new-api/types"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrConfigImportStale       = configImportError("STALE_BASE_VERSION", "active configuration changed since staging")
	ErrConfigImportNotReady    = configImportError("PUBLISH_BATCH_STATUS", "config import batch is not ready")
	ErrConfigImportAlreadyDone = configImportError("PUBLISH_ALREADY_COMPLETE", "config import batch has already been published")
)

// ConfigImportRefreshKeys identifies cache domains affected by a publication.
type ConfigImportRefreshKeys struct {
	OptionKeys        []string
	CostChannelIDs    []int
	CostModelKeys     []string
	RoutingPolicyKeys []model.RoutingPolicyKey
	ChannelIDs        []int
}

// RefreshPublishedConfig refreshes committed in-memory state in dependency
// order. It never opens a transaction or mutates database rows.
func RefreshPublishedConfig(keys ConfigImportRefreshKeys) error {
	if len(keys.OptionKeys) > 0 {
		values := make(map[string]string, len(keys.OptionKeys))
		for _, key := range keys.OptionKeys {
			var option model.Option
			if err := model.DB.Where(clause.Eq{
				Column: clause.Column{Name: "key"},
				Value:  key,
			}).First(&option).Error; err != nil {
				return err
			}
			values[key] = option.Value
		}
		if err := model.RefreshOptions(values); err != nil {
			return err
		}
	}
	for _, key := range keys.CostModelKeys {
		parts := strings.SplitN(key, "|", 3)
		if len(parts) != 3 {
			continue
		}
		channelID := 0
		if _, err := fmt.Sscanf(parts[0], "%d", &channelID); err != nil {
			continue
		}
		InvalidateCostCoverage(channelID, parts[1], parts[2])
	}
	if err := model.RefreshRoutingPolicyCacheKeys(keys.RoutingPolicyKeys); err != nil {
		return err
	}
	if len(keys.ChannelIDs) > 0 {
		model.InitChannelCache()
	}
	return nil
}

// PublishConfigImportBatch applies all reviewed proposals as one database
// transaction. Cache and in-memory setting refreshes happen only after commit.
func PublishConfigImportBatch(ctx context.Context, batchID int64, adminID int) error {
	if adminID <= 0 {
		return configImportError("SCHEMA_ADMIN", "admin ID is required")
	}
	if batchID <= 0 {
		return configImportError("SCHEMA_BATCH_ID", "batch ID is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var refresh ConfigImportRefreshKeys
	publishStarted := false
	err := model.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var batch model.ConfigImportBatch
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", batchID).First(&batch).Error; err != nil {
			return err
		}
		switch types.ConfigImportBatchStatus(batch.Status) {
		case types.ConfigImportBatchStatusPublished:
			return ErrConfigImportAlreadyDone
		case types.ConfigImportBatchStatusReady:
		default:
			return ErrConfigImportNotReady
		}
		var baseline ConfigImportBaseline
		if err := common.UnmarshalJsonStr(batch.BaselineJSON, &baseline); err != nil {
			return err
		}
		current, err := CaptureConfigImportBaseline(tx, batchID)
		if err != nil {
			return err
		}
		if baseline.Hash == "" || current.Hash != baseline.Hash {
			_ = tx.Model(&model.ConfigImportBatch{}).Where("id = ?", batchID).Updates(map[string]any{
				"status": string(types.ConfigImportBatchStatusStaged), "failure_code": "STALE_BASE_VERSION", "failure_message": "active configuration changed since staging", "updated_at": common.GetTimestamp(),
			})
			return ErrConfigImportStale
		}
		updated, err := model.UpdateConfigImportBatchStatus(tx, batchID, types.ConfigImportBatchStatusReady, types.ConfigImportBatchStatusPublishing)
		if err != nil {
			return err
		}
		if !updated {
			return ErrConfigImportNotReady
		}
		publishStarted = true
		var items []model.ConfigImportItem
		if err := tx.Where("batch_id = ?", batchID).Order("entity_type ASC, business_id ASC, id ASC").Find(&items).Error; err != nil {
			return err
		}
		for _, item := range items {
			if item.EntityType == "cost_rule_drafts" && item.MaterializedID != nil && item.State != string(types.ConfigImportItemStateExcluded) && item.State != string(types.ConfigImportItemStateUnchanged) {
				var rule model.ChannelModelCostRule
				if err := tx.Where("id = ? AND status = ?", *item.MaterializedID, types.CostRuleDraft).First(&rule).Error; err != nil {
					return err
				}
				activated, err := model.ActivateChannelModelCostRuleWithTx(tx, rule.ID, adminID, common.GetTimestamp(), nil)
				if err != nil {
					return err
				}
				refresh.CostModelKeys = append(refresh.CostModelKeys, fmt.Sprintf("%d|%s|%s", activated.ChannelID, activated.BillableUpstreamModel, activated.CostVariantKey))
			}
		}
		if err := publishConfigImportSaleOptions(tx, items, &refresh); err != nil {
			return err
		}
		if err := publishConfigImportModelMappings(tx, items, &refresh); err != nil {
			return err
		}
		if err := publishConfigImportRoutes(tx, items, &refresh); err != nil {
			return err
		}
		after, err := CaptureConfigImportBaseline(tx, batchID)
		if err != nil {
			return err
		}
		now := common.GetTimestamp()
		if err := tx.Create(&model.ConfigImportPublishAudit{BatchID: batchID, AdminID: adminID, BeforeSHA256: current.Hash, AfterSHA256: after.Hash, Outcome: "published", CreatedAt: now}).Error; err != nil {
			return err
		}
		return tx.Model(&model.ConfigImportBatch{}).Where("id = ? AND status = ?", batchID, types.ConfigImportBatchStatusPublishing).Updates(map[string]any{
			"status": string(types.ConfigImportBatchStatusPublished), "published_at": now, "updated_at": now,
		}).Error
	})
	if err != nil {
		if publishStarted {
			if markErr := markConfigImportPublishFailed(ctx, batchID, err); markErr != nil {
				common.SysError(fmt.Sprintf("failed to mark config import batch %d as publish_failed: %v", batchID, markErr))
			}
		}
		return err
	}
	if err := RefreshPublishedConfig(refresh); err != nil {
		if markErr := markConfigImportCacheRefreshPending(ctx, batchID); markErr != nil {
			common.SysError(fmt.Sprintf("failed to record pending config import cache refresh for batch %d: %v", batchID, markErr))
		}
		return fmt.Errorf("published but cache refresh failed: %w", err)
	}
	return nil
}

func markConfigImportPublishFailed(ctx context.Context, batchID int64, publishErr error) error {
	code := "PUBLISH_FAILED"
	var schemaErr *ConfigImportSchemaError
	if errors.As(publishErr, &schemaErr) && strings.TrimSpace(schemaErr.Code) != "" {
		code = schemaErr.Code
	}
	now := common.GetTimestamp()
	return model.DB.WithContext(ctx).Model(&model.ConfigImportBatch{}).
		Where("id = ? AND status = ?", batchID, types.ConfigImportBatchStatusReady).
		Updates(map[string]any{
			"status":          string(types.ConfigImportBatchStatusPublishFailed),
			"failure_code":    code,
			"failure_message": "configuration publish transaction failed",
			"failed_at":       now,
			"updated_at":      now,
		}).Error
}

func markConfigImportCacheRefreshPending(ctx context.Context, batchID int64) error {
	if !model.DB.Migrator().HasTable(&model.ConfigImportIssue{}) {
		return nil
	}
	now := common.GetTimestamp()
	var issue model.ConfigImportIssue
	err := model.DB.WithContext(ctx).Where("batch_id = ? AND code = ?", batchID, "CACHE_REFRESH_PENDING").First(&issue).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.DB.WithContext(ctx).Create(&model.ConfigImportIssue{
			BatchID: batchID, Severity: string(types.ConfigImportIssueSeverityWarning), Code: "CACHE_REFRESH_PENDING",
			Message: "Published configuration cache refresh is pending.", ResolutionStatus: "open", CreatedAt: now, UpdatedAt: now,
		}).Error
	}
	if err != nil {
		return err
	}
	return model.DB.WithContext(ctx).Model(&model.ConfigImportIssue{}).Where("id = ?", issue.ID).Updates(map[string]any{
		"severity": string(types.ConfigImportIssueSeverityWarning), "message": "Published configuration cache refresh is pending.",
		"resolution_status": "open", "updated_at": now,
	}).Error
}

// RetryConfigImportBatchCache rebuilds only in-memory configuration from an
// already-published batch. It never replays the publication transaction.
func RetryConfigImportBatchCache(ctx context.Context, batchID int64, adminID int) error {
	if adminID <= 0 {
		return configImportError("SCHEMA_ADMIN", "admin ID is required")
	}
	if batchID <= 0 {
		return configImportError("SCHEMA_BATCH_ID", "batch ID is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var batch model.ConfigImportBatch
	if err := model.DB.WithContext(ctx).Where("id = ? AND status = ?", batchID, types.ConfigImportBatchStatusPublished).First(&batch).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return configImportError("CACHE_REFRESH_BATCH_STATUS", "config import batch is not published")
		}
		return err
	}
	keys, err := configImportRefreshKeysForBatch(model.DB.WithContext(ctx), batchID)
	if err != nil {
		return err
	}
	if err := RefreshPublishedConfig(keys); err != nil {
		if markErr := markConfigImportCacheRefreshPending(ctx, batchID); markErr != nil {
			common.SysError(fmt.Sprintf("failed to record pending config import cache refresh for batch %d: %v", batchID, markErr))
		}
		return fmt.Errorf("config import cache refresh failed: %w", err)
	}
	if !model.DB.Migrator().HasTable(&model.ConfigImportIssue{}) {
		return nil
	}
	return model.DB.WithContext(ctx).Model(&model.ConfigImportIssue{}).
		Where("batch_id = ? AND code = ? AND resolution_status = ?", batchID, "CACHE_REFRESH_PENDING", "open").
		Updates(map[string]any{"resolution_status": "resolved", "updated_at": common.GetTimestamp()}).Error
}

func configImportRefreshKeysForBatch(db *gorm.DB, batchID int64) (ConfigImportRefreshKeys, error) {
	keys := ConfigImportRefreshKeys{}
	var items []model.ConfigImportItem
	if err := db.Where("batch_id = ?", batchID).Order("entity_type ASC, business_id ASC, id ASC").Find(&items).Error; err != nil {
		return keys, err
	}
	lineChannels, err := configImportPublishedLineChannels(db, items)
	if err != nil {
		return keys, err
	}
	for _, item := range items {
		if item.State == string(types.ConfigImportItemStateExcluded) || item.State == string(types.ConfigImportItemStateUnchanged) {
			continue
		}
		switch item.EntityType {
		case "cost_rule_drafts":
			if item.MaterializedID == nil || !db.Migrator().HasTable(&model.ChannelModelCostRule{}) {
				continue
			}
			var rule model.ChannelModelCostRule
			if err := db.Where("id = ? AND status = ?", *item.MaterializedID, types.CostRuleActive).First(&rule).Error; err != nil {
				return keys, err
			}
			keys.CostModelKeys = appendConfigImportRefreshString(keys.CostModelKeys, fmt.Sprintf("%d|%s|%s", rule.ChannelID, rule.BillableUpstreamModel, rule.CostVariantKey))
		case "sale_proposals":
			if !db.Migrator().HasTable(&model.Option{}) {
				continue
			}
			var document map[string]any
			if err := common.UnmarshalJsonStr(item.CanonicalJSON, &document); err != nil {
				return keys, err
			}
			staged, ok := document["staged_proposal"].(map[string]any)
			if !ok {
				return keys, configImportError("PUBLISH_PRICING_PATCH", "sale proposal %q has no staged option patch", item.BusinessID)
			}
			optionPatches, ok := staged["option_patches"].(map[string]any)
			if !ok {
				return keys, configImportError("PUBLISH_PRICING_PATCH", "sale proposal %q has invalid option patches", item.BusinessID)
			}
			for key := range optionPatches {
				keys.OptionKeys = appendConfigImportRefreshString(keys.OptionKeys, key)
			}
		case "model_mappings":
			var mapping types.ConfigImportModelMapping
			if err := common.UnmarshalJsonStr(item.CanonicalJSON, &mapping); err != nil {
				return keys, err
			}
			if channelID := lineChannels[mapping.LineRef]; channelID > 0 {
				keys.ChannelIDs = appendConfigImportRefreshInt(keys.ChannelIDs, channelID)
			}
		case "route_blueprints":
			var blueprint types.ConfigImportRouteBlueprint
			if err := common.UnmarshalJsonStr(item.CanonicalJSON, &blueprint); err != nil {
				return keys, err
			}
			if blueprint.MergeMode != types.ConfigImportRouteMergeModeSkip {
				keys.RoutingPolicyKeys = appendConfigImportRefreshRoutingKey(keys.RoutingPolicyKeys, model.RoutingPolicyKey{
					GroupName: "default", Model: configImportRuntimeCanonicalModel(blueprint.CanonicalModel),
				})
			}
		}
	}
	return keys, nil
}

func appendConfigImportRefreshString(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func appendConfigImportRefreshInt(values []int, value int) []int {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func appendConfigImportRefreshRoutingKey(values []model.RoutingPolicyKey, value model.RoutingPolicyKey) []model.RoutingPolicyKey {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func publishConfigImportSaleOptions(tx *gorm.DB, items []model.ConfigImportItem, refresh *ConfigImportRefreshKeys) error {
	if !tx.Migrator().HasTable(&model.Option{}) {
		return nil
	}
	patches := make(map[string]map[string]any)
	for _, item := range items {
		if item.EntityType != "sale_proposals" || item.State == string(types.ConfigImportItemStateExcluded) || item.State == string(types.ConfigImportItemStateUnchanged) {
			continue
		}
		var document map[string]any
		if err := common.UnmarshalJsonStr(item.CanonicalJSON, &document); err != nil {
			return err
		}
		staged, ok := document["staged_proposal"].(map[string]any)
		if !ok {
			continue
		}
		optionPatches, ok := staged["option_patches"].(map[string]any)
		if !ok {
			continue
		}
		for key, raw := range optionPatches {
			values, ok := raw.(map[string]any)
			if !ok {
				return configImportError("PUBLISH_PRICING_PATCH", "sale proposal %q has invalid option patch %q", item.BusinessID, key)
			}
			if patches[key] == nil {
				patches[key] = make(map[string]any)
			}
			for modelName, value := range values {
				patches[key][modelName] = value
			}
		}
	}
	keys := make([]string, 0, len(patches))
	for key := range patches {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		var option model.Option
		err := tx.Where(clause.Eq{
			Column: clause.Column{Name: "key"},
			Value:  key,
		}).First(&option).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		current := make(map[string]any)
		if err == nil && strings.TrimSpace(option.Value) != "" {
			if decodeErr := common.UnmarshalJsonStr(option.Value, &current); decodeErr != nil {
				return configImportError("PUBLISH_PRICING_OPTION", "option %q is not a JSON object", key)
			}
		}
		for modelName, value := range patches[key] {
			current[modelName] = value
		}
		encoded, marshalErr := common.Marshal(current)
		if marshalErr != nil {
			return marshalErr
		}
		if err := model.UpdateOptionsWithTx(tx, map[string]string{key: string(encoded)}); err != nil {
			return err
		}
		refresh.OptionKeys = append(refresh.OptionKeys, key)
	}
	return nil
}

func publishConfigImportModelMappings(tx *gorm.DB, items []model.ConfigImportItem, refresh *ConfigImportRefreshKeys) error {
	if !tx.Migrator().HasTable(&model.Channel{}) {
		return nil
	}
	lineChannels, err := configImportPublishedLineChannels(tx, items)
	if err != nil {
		return err
	}
	mappingsByChannel := make(map[int]map[string]string)
	for _, item := range items {
		if item.EntityType != "model_mappings" || item.State == string(types.ConfigImportItemStateExcluded) || item.State == string(types.ConfigImportItemStateUnchanged) {
			continue
		}
		var mapping types.ConfigImportModelMapping
		if err := common.UnmarshalJsonStr(item.CanonicalJSON, &mapping); err != nil {
			return err
		}
		channelID := lineChannels[mapping.LineRef]
		if channelID <= 0 {
			return configImportError("PUBLISH_LINE_UNBOUND", "model mapping %q references unbound line %q", item.BusinessID, mapping.LineRef)
		}
		if mappingsByChannel[channelID] == nil {
			mappingsByChannel[channelID] = make(map[string]string)
		}
		mappingsByChannel[channelID][configImportRuntimeCanonicalModel(mapping.CanonicalModel)] = mapping.UpstreamModel
	}
	channelIDs := make([]int, 0, len(mappingsByChannel))
	for channelID := range mappingsByChannel {
		channelIDs = append(channelIDs, channelID)
	}
	sort.Ints(channelIDs)
	for _, channelID := range channelIDs {
		var channel model.Channel
		if err := tx.Where("id = ?", channelID).First(&channel).Error; err != nil {
			return err
		}
		mapping := make(map[string]string)
		if strings.TrimSpace(channel.GetModelMapping()) != "" {
			if err := common.UnmarshalJsonStr(channel.GetModelMapping(), &mapping); err != nil {
				return configImportError("PUBLISH_MODEL_MAPPING", "channel %d has invalid existing model mapping", channelID)
			}
		}
		for origin, upstream := range mappingsByChannel[channelID] {
			mapping[origin] = upstream
		}
		encoded, err := common.Marshal(mapping)
		if err != nil {
			return err
		}
		if err := tx.Model(&model.Channel{}).Where("id = ?", channelID).Updates(map[string]any{"model_mapping": string(encoded)}).Error; err != nil {
			return err
		}
		refresh.ChannelIDs = append(refresh.ChannelIDs, channelID)
	}
	return nil
}

func configImportPublishedLineChannels(tx *gorm.DB, items []model.ConfigImportItem) (map[string]int, error) {
	lineChannels := make(map[string]int)
	if len(items) == 0 {
		return lineChannels, nil
	}
	var bindings []model.ConfigImportBinding
	if err := tx.Where("batch_id = ?", items[0].BatchID).Find(&bindings).Error; err != nil {
		return nil, err
	}
	for _, binding := range bindings {
		if binding.Action == string(types.ConfigImportBindingActionSkip) || binding.ChannelID == nil || binding.CredentialsConfirmedAt == nil || binding.CredentialsConfirmedBy <= 0 {
			continue
		}
		lineChannels[binding.LineRef] = *binding.ChannelID
	}
	return lineChannels, nil
}

func publishConfigImportRoutes(tx *gorm.DB, items []model.ConfigImportItem, refresh *ConfigImportRefreshKeys) error {
	if !tx.Migrator().HasTable(&model.RoutingPolicy{}) || !tx.Migrator().HasTable(&model.RouteTarget{}) {
		return nil
	}
	lineChannels, err := configImportPublishedLineChannels(tx, items)
	if err != nil {
		return err
	}
	for _, item := range items {
		if item.EntityType != "route_blueprints" || item.State == string(types.ConfigImportItemStateExcluded) || item.State == string(types.ConfigImportItemStateUnchanged) {
			continue
		}
		var blueprint types.ConfigImportRouteBlueprint
		if err := common.UnmarshalJsonStr(item.CanonicalJSON, &blueprint); err != nil {
			return err
		}
		if blueprint.MergeMode == types.ConfigImportRouteMergeModeSkip {
			continue
		}
		if blueprint.MergeMode == "" {
			blueprint.MergeMode = types.ConfigImportRouteMergeModeMerge
		}
		policy, targets, err := configImportRouteRows(lineChannels, blueprint)
		if err != nil {
			return err
		}
		var existing model.RoutingPolicy
		findErr := tx.Where("group_name = ? AND model = ?", policy.GroupName, policy.Model).First(&existing).Error
		if findErr != nil && !errors.Is(findErr, gorm.ErrRecordNotFound) {
			return findErr
		}
		policyID := 0
		if findErr == nil {
			policyID = existing.ID
			if blueprint.MergeMode == types.ConfigImportRouteMergeModeMerge {
				var current []model.RouteTarget
				if err := tx.Where("policy_id = ?", existing.ID).
					Order("channel_id ASC").
					Order("target_priority DESC").
					Order("name ASC").
					Order("id ASC").
					Find(&current).Error; err != nil {
					return err
				}
				policy = existing
				policy.Targets = nil
				targets = configImportMergeRouteTargets(current, targets)
			}
		}
		policy.ID = policyID
		if _, err := model.ReplaceRoutingPolicyWithTx(tx, policyID, policy, targets); err != nil {
			return configImportError("PUBLISH_ROUTE", "route blueprint %q failed validation: %v", item.BusinessID, err)
		}
		refresh.RoutingPolicyKeys = append(refresh.RoutingPolicyKeys, model.RoutingPolicyKey{GroupName: policy.GroupName, Model: policy.Model})
	}
	return nil
}

// configImportMergeRouteTargets leaves unrelated targets in place and replaces
// only targets whose stable route_target_ref (stored as Name) is owned by this
// import. ReplaceRoutingPolicyWithTx rewrites the complete target collection,
// so the merged set must be assembled before that call.
func configImportMergeRouteTargets(existing, incoming []model.RouteTarget) []model.RouteTarget {
	merged := make([]model.RouteTarget, len(existing))
	copy(merged, existing)
	byName := make(map[string]int, len(existing))
	for index := range merged {
		byName[merged[index].Name] = index
	}
	for _, target := range incoming {
		if index, found := byName[target.Name]; found {
			merged[index] = target
			continue
		}
		byName[target.Name] = len(merged)
		merged = append(merged, target)
	}
	return merged
}

func configImportRouteRows(lineChannels map[string]int, blueprint types.ConfigImportRouteBlueprint) (model.RoutingPolicy, []model.RouteTarget, error) {
	policy := model.RoutingPolicy{
		GroupName:         "default",
		Model:             configImportRuntimeCanonicalModel(blueprint.CanonicalModel),
		Enabled:           false,
		DefaultResolution: "720p",
		DefaultDuration:   4,
		DefaultRatio:      "adaptive",
	}
	if len(blueprint.Targets) > 0 {
		first := blueprint.Targets[0]
		if len(first.OutputResolutions) > 0 {
			policy.DefaultResolution = first.OutputResolutions[0]
		}
		for _, duration := range first.DurationValues {
			if duration > 0 {
				policy.DefaultDuration = duration
				break
			}
		}
		if first.DurationMin != nil && *first.DurationMin > 0 {
			policy.DefaultDuration = *first.DurationMin
		} else if len(first.DurationValues) == 0 && first.DurationMax != nil && *first.DurationMax > 0 {
			policy.DefaultDuration = *first.DurationMax
		}
		if len(first.AspectRatios) > 0 {
			policy.DefaultRatio = first.AspectRatios[0]
		}
	}
	targets := make([]model.RouteTarget, 0, len(blueprint.Targets))
	for _, target := range blueprint.Targets {
		channelID := lineChannels[target.LineRef]
		if channelID <= 0 {
			return policy, nil, configImportError("PUBLISH_LINE_UNBOUND", "route target %q references unbound line %q", target.RouteTargetRef, target.LineRef)
		}
		constraints := modelrouting.Constraints{
			OutputResolutions:  target.OutputResolutions,
			Durations:          modelrouting.DurationConstraint{Values: target.DurationValues, Min: target.DurationMin, Max: target.DurationMax},
			AspectRatios:       target.AspectRatios,
			ReferenceMinimums:  configImportReferenceLimits(target.ReferenceMinimums),
			ReferenceLimits:    configImportReferenceLimits(target.ReferenceLimits),
			SupportsRealPerson: target.SupportsRealPerson,
		}
		for _, mode := range target.InputModes {
			constraints.InputModes = append(constraints.InputModes, modelrouting.InputMode(mode))
		}
		encoded, err := common.Marshal(constraints)
		if err != nil {
			return policy, nil, err
		}
		priority := 0
		if target.Priority != nil {
			priority = *target.Priority
		}
		enabled := false
		targets = append(targets, model.RouteTarget{ChannelID: channelID, Name: target.RouteTargetRef, UpstreamModel: target.UpstreamModel, CostVariantKey: target.CostVariantKey, TargetPriority: priority, Enabled: enabled, Constraints: string(encoded)})
	}
	return policy, targets, nil
}

// Config documents use the stable, human-readable model families in legacy
// workbooks. Routing policies use the current public model identifiers.
func configImportRuntimeCanonicalModel(modelName string) string {
	switch strings.TrimSpace(modelName) {
	case "seedance-2.0":
		return modelrouting.Seedance20
	case "seedance-2.0-fast":
		return modelrouting.Seedance20Fast
	case "seedance-2.0-mini":
		return modelrouting.Seedance20Mini
	default:
		return strings.TrimSpace(modelName)
	}
}

func configImportReferenceLimits(bounds *types.ConfigImportReferenceBounds) modelrouting.ReferenceLimits {
	limits := modelrouting.ReferenceLimits{}
	if bounds == nil {
		return limits
	}
	if bounds.Images != nil {
		limits.Images = *bounds.Images
	}
	if bounds.Videos != nil {
		limits.Videos = *bounds.Videos
	}
	if bounds.Audios != nil {
		limits.Audios = *bounds.Audios
	}
	return limits
}
