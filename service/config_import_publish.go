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
	"github.com/QuantumNous/new-api/pkg/seedancepricing"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/types"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrConfigImportStale       = configImportError("STALE_BASE_VERSION", "active configuration changed since staging")
	ErrConfigImportNotReady    = configImportError("PUBLISH_BATCH_STATUS", "config import batch is not ready")
	ErrConfigImportAlreadyDone = configImportError("PUBLISH_ALREADY_COMPLETE", "config import batch has already been published")
)

var configImportSeedanceSaleOptionKeys = []string{
	"ModelPrice",
	"ModelRatio",
	"CompletionRatio",
	"billing_setting.billing_mode",
	"billing_setting.billing_expr",
	"billing_setting.duration_price",
	"billing_setting.seedance_token_price",
}

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

// PublishConfigImportBatch freezes reviewed proposals and creates disabled,
// batch-owned route candidates. Active configuration changes happen only in
// the activation transaction.
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
	publishStarted := false
	err := model.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var batch model.ConfigImportBatch
		if err := model.LockModelForUpdate(tx, &model.ConfigImportBatch{}).Where("id = ?", batchID).First(&batch).Error; err != nil {
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
		if err := common.UnmarshalJsonStr(string(batch.BaselineJSON), &baseline); err != nil {
			return err
		}
		current, err := CaptureConfigImportBaseline(tx, batchID)
		if err != nil {
			return err
		}
		if baseline.Hash == "" || current.Hash != baseline.Hash {
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
		if err := publishConfigImportRoutes(tx, items, nil); err != nil {
			return err
		}
		after, err := CaptureConfigImportBaseline(tx, batchID)
		if err != nil {
			return err
		}
		afterJSON, err := common.Marshal(after)
		if err != nil {
			return err
		}
		now := common.GetTimestamp()
		if err := tx.Create(&model.ConfigImportPublishAudit{BatchID: batchID, AdminID: adminID, BeforeSHA256: current.Hash, AfterSHA256: after.Hash, Outcome: "published", CreatedAt: now}).Error; err != nil {
			return err
		}
		return tx.Model(&model.ConfigImportBatch{}).Where("id = ? AND status = ?", batchID, types.ConfigImportBatchStatusPublishing).Updates(map[string]any{
			"status": string(types.ConfigImportBatchStatusPublished), "baseline_json": string(afterJSON), "published_at": now, "updated_at": now,
		}).Error
	})
	if err != nil {
		if errors.Is(err, ErrConfigImportStale) {
			if markErr := markConfigImportStale(ctx, batchID); markErr != nil {
				return fmt.Errorf("%w: failed to persist stale batch status: %v", err, markErr)
			}
			return err
		}
		if publishStarted {
			if markErr := markConfigImportPublishFailed(ctx, batchID, err); markErr != nil {
				common.SysError(fmt.Sprintf("failed to mark config import batch %d as publish_failed: %v", batchID, markErr))
			}
		}
		return err
	}
	return nil
}

func markConfigImportStale(ctx context.Context, batchID int64) error {
	now := common.GetTimestamp()
	return model.DB.WithContext(ctx).Model(&model.ConfigImportBatch{}).
		Where("id = ? AND status = ?", batchID, types.ConfigImportBatchStatusReady).
		Updates(map[string]any{
			"status":          string(types.ConfigImportBatchStatusStaged),
			"failure_code":    "STALE_BASE_VERSION",
			"failure_message": "active configuration changed since staging",
			"updated_at":      now,
		}).Error
}

func publishConfigImportAuthoritativeCostRules(tx *gorm.DB, items []model.ConfigImportItem, refresh *ConfigImportRefreshKeys) error {
	if !tx.Migrator().HasTable(&model.ChannelModelCostRule{}) {
		return nil
	}
	lineChannels, err := configImportPublishedLineChannels(tx, items)
	if err != nil {
		return err
	}
	affectedChannels := make(map[int]struct{})
	for _, channelID := range lineChannels {
		if channelID > 0 {
			affectedChannels[channelID] = struct{}{}
		}
	}
	if len(affectedChannels) == 0 {
		return nil
	}
	authoritativeKeys := make(map[string]struct{})
	for _, item := range items {
		if item.EntityType != "cost_rule_drafts" || item.State == string(types.ConfigImportItemStateExcluded) {
			continue
		}
		var draft types.ConfigImportCostRuleDraft
		if err := common.UnmarshalJsonStr(item.CanonicalJSON, &draft); err != nil {
			return err
		}
		channelID := lineChannels[draft.LineRef]
		if channelID <= 0 {
			continue
		}
		if draft.Enabled != nil && !*draft.Enabled {
			continue
		}
		authoritativeKeys[fmt.Sprintf("%d|%s|%s", channelID, strings.TrimSpace(draft.UpstreamModel), draft.CostVariantKey)] = struct{}{}
	}
	channelIDs := make([]int, 0, len(affectedChannels))
	for channelID := range affectedChannels {
		channelIDs = append(channelIDs, channelID)
	}
	sort.Ints(channelIDs)
	var activeRules []model.ChannelModelCostRule
	if err := tx.Where("channel_id IN ? AND status = ?", channelIDs, types.CostRuleActive).
		Order("channel_id ASC, billable_upstream_model ASC, cost_variant_key ASC, id ASC").Find(&activeRules).Error; err != nil {
		return err
	}
	now := common.GetTimestamp()
	for _, rule := range activeRules {
		key := fmt.Sprintf("%d|%s|%s", rule.ChannelID, rule.BillableUpstreamModel, rule.CostVariantKey)
		if _, retained := authoritativeKeys[key]; retained {
			continue
		}
		result := tx.Model(&model.ChannelModelCostRule{}).
			Where("id = ? AND status = ?", rule.ID, types.CostRuleActive).
			Updates(map[string]any{
				"status": string(types.CostRuleRetired), "effective_to": now, "updated_at": now,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return model.ErrCostRuleStateConflict
		}
		refresh.CostModelKeys = appendConfigImportRefreshString(
			refresh.CostModelKeys,
			key,
		)
	}
	return nil
}

// recordPostActivationCostCoverage makes the cost-accounting consequence of an
// import visible on the import batch itself. It is deliberately best-effort:
// activation has already committed, so a coverage-check persistence failure
// must not report the committed import as failed.
func recordPostActivationCostCoverage(ctx context.Context, batchID int64, refresh ConfigImportRefreshKeys) error {
	if !model.DB.Migrator().HasTable(&model.ConfigImportIssue{}) {
		return nil
	}
	coverage, err := CheckAuthoritativeCostCoverage()
	if err != nil {
		return err
	}
	affectedChannels := make(map[int]struct{}, len(refresh.ChannelIDs)+len(refresh.CostModelKeys))
	for _, channelID := range refresh.ChannelIDs {
		if channelID > 0 {
			affectedChannels[channelID] = struct{}{}
		}
	}
	for _, key := range refresh.CostModelKeys {
		parts := strings.SplitN(key, "|", 2)
		if len(parts) != 2 {
			continue
		}
		channelID := 0
		if _, err := fmt.Sscanf(parts[0], "%d", &channelID); err == nil && channelID > 0 {
			affectedChannels[channelID] = struct{}{}
		}
	}
	uncovered := 0
	for _, item := range coverage {
		if _, affected := affectedChannels[item.ChannelID]; affected && !item.Covered {
			uncovered++
		}
	}
	now := common.GetTimestamp()
	var issue model.ConfigImportIssue
	err = model.DB.WithContext(ctx).
		Where("batch_id = ? AND code = ?", batchID, "COST_COVERAGE_INCOMPLETE").
		First(&issue).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if uncovered == 0 {
			return nil
		}
		return model.DB.WithContext(ctx).Create(&model.ConfigImportIssue{
			BatchID: batchID, Severity: string(types.ConfigImportIssueSeverityWarning),
			Code: "COST_COVERAGE_INCOMPLETE", EntityType: "cost_accounting",
			Message:          fmt.Sprintf("Published configuration has %d uncovered enabled channel model mappings.", uncovered),
			Suggestion:       "Add an active compatible cost rule or disable the corresponding channel/model mapping.",
			ResolutionStatus: "open", CreatedAt: now, UpdatedAt: now,
		}).Error
	}
	if err != nil {
		return err
	}
	message := "Authoritative cost coverage is complete after publish."
	status := "resolved"
	if uncovered > 0 {
		message = fmt.Sprintf("Published configuration has %d uncovered enabled channel model mappings.", uncovered)
		status = "open"
	}
	return model.DB.WithContext(ctx).Model(&issue).Updates(map[string]any{
		"severity": string(types.ConfigImportIssueSeverityWarning),
		"message":  message, "resolution_status": status, "updated_at": now,
	}).Error
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
	hadActivationPending := false
	if model.DB.Migrator().HasTable(&model.ConfigImportIssue{}) {
		var pendingCount int64
		if err := model.DB.WithContext(ctx).Model(&model.ConfigImportIssue{}).
			Where("batch_id = ? AND code = ? AND resolution_status = ?", batchID, "ACTIVATION_CACHE_REFRESH_PENDING", "open").
			Count(&pendingCount).Error; err != nil {
			return err
		}
		hadActivationPending = pendingCount > 0
	}
	if err := refreshConfigImportActivation(keys); err != nil {
		var markErr error
		if batch.ActivatedAt != nil {
			markErr = markConfigImportActivationCacheRefreshPending(ctx, batchID)
		} else {
			markErr = markConfigImportCacheRefreshPending(ctx, batchID)
		}
		if markErr != nil {
			common.SysError(fmt.Sprintf("failed to record pending config import cache refresh for batch %d: %v", batchID, markErr))
		}
		return fmt.Errorf("config import cache refresh failed: %w", err)
	}
	if !model.DB.Migrator().HasTable(&model.ConfigImportIssue{}) {
		return nil
	}
	if err := model.DB.WithContext(ctx).Model(&model.ConfigImportIssue{}).
		Where("batch_id = ? AND code IN ? AND resolution_status = ?", batchID, []string{"CACHE_REFRESH_PENDING", "ACTIVATION_CACHE_REFRESH_PENDING"}, "open").
		Updates(map[string]any{"resolution_status": "resolved", "updated_at": common.GetTimestamp()}).Error; err != nil {
		return err
	}
	if hadActivationPending {
		if err := appendConfigImportActivationCacheAudit(ctx, batchID, adminID, "cache_refreshed"); err != nil {
			return err
		}
	}
	return nil
}

func configImportRefreshKeysForBatch(db *gorm.DB, batchID int64) (ConfigImportRefreshKeys, error) {
	keys := ConfigImportRefreshKeys{}
	var batch model.ConfigImportBatch
	if err := db.Select("summary_json").Where("id = ?", batchID).First(&batch).Error; err != nil {
		return keys, err
	}
	var storedSummary configImportBatchSummaryStorage
	if strings.TrimSpace(string(batch.SummaryJSON)) != "" {
		if err := common.UnmarshalJsonStr(string(batch.SummaryJSON), &storedSummary); err != nil {
			return keys, err
		}
	}
	for _, snapshot := range storedSummary.ChannelModelSnapshots {
		keys.ChannelIDs = appendConfigImportRefreshInt(keys.ChannelIDs, snapshot.ChannelID)
		if db.Migrator().HasTable(&model.ChannelModelCostRule{}) {
			var rules []model.ChannelModelCostRule
			if err := db.Where("channel_id = ? AND status = ?", snapshot.ChannelID, types.CostRuleRetired).
				Order("billable_upstream_model ASC, cost_variant_key ASC, id ASC").Find(&rules).Error; err != nil {
				return keys, err
			}
			for _, rule := range rules {
				keys.CostModelKeys = appendConfigImportRefreshString(keys.CostModelKeys, fmt.Sprintf("%d|%s|%s", rule.ChannelID, rule.BillableUpstreamModel, rule.CostVariantKey))
			}
		}
		if len(snapshot.RemovedModels) == 0 {
			continue
		}
		if db.Migrator().HasTable(&model.RoutingPolicy{}) && db.Migrator().HasTable(&model.RouteTarget{}) {
			var policyIDs []int
			if err := db.Model(&model.RouteTarget{}).
				Where("channel_id = ? AND upstream_model IN ?", snapshot.ChannelID, snapshot.RemovedModels).
				Distinct("policy_id").Pluck("policy_id", &policyIDs).Error; err != nil {
				return keys, err
			}
			if len(policyIDs) > 0 {
				var policies []model.RoutingPolicy
				if err := db.Where("id IN ?", policyIDs).Order("id ASC").Find(&policies).Error; err != nil {
					return keys, err
				}
				for _, policy := range policies {
					keys.RoutingPolicyKeys = appendConfigImportRefreshRoutingKey(keys.RoutingPolicyKeys, model.RoutingPolicyKey{GroupName: policy.GroupName, Model: policy.Model})
				}
			}
		}
	}
	var items []model.ConfigImportItem
	if err := db.Where("batch_id = ?", batchID).Order("entity_type ASC, business_id ASC, id ASC").Find(&items).Error; err != nil {
		return keys, err
	}
	lineChannels, err := configImportPublishedLineChannels(db, items)
	if err != nil {
		return keys, err
	}
	for _, item := range items {
		disabledCost, err := configImportDisabledCostRuleRetiresActive(item)
		if err != nil {
			return keys, err
		}
		if item.State == string(types.ConfigImportItemStateExcluded) && !disabledCost ||
			item.State == string(types.ConfigImportItemStateUnchanged) && item.EntityType != "model_mappings" {
			continue
		}
		switch item.EntityType {
		case "cost_rule_drafts":
			if disabledCost {
				var draft types.ConfigImportCostRuleDraft
				if err := common.UnmarshalJsonStr(item.CanonicalJSON, &draft); err != nil {
					return keys, err
				}
				if channelID := lineChannels[draft.LineRef]; channelID > 0 {
					keys.CostModelKeys = appendConfigImportRefreshString(keys.CostModelKeys, fmt.Sprintf("%d|%s|%s", channelID, draft.UpstreamModel, draft.CostVariantKey))
				}
				continue
			}
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
	seedanceOfficialSale, seedanceSaleModels, err := configImportSeedanceSaleCleanupScope(tx, items)
	if err != nil {
		return err
	}
	for _, item := range items {
		if item.EntityType != "sale_proposals" || item.State == string(types.ConfigImportItemStateExcluded) {
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
				if (key == "billing_setting.duration_price" && configImportDurationPriceHasScenarios(value)) ||
					(key == "billing_setting.seedance_token_price" && configImportSeedanceTokenPriceHasScenarios(value)) {
					if previous, exists := patches[key][modelName]; exists {
						merged, mergeErr := mergeConfigImportDurationPriceValues(previous, value, modelName)
						if mergeErr != nil {
							return configImportError("PUBLISH_PRICING_CONFLICT", "%v", mergeErr)
						}
						value = merged
					}
				}
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
		if seedanceOfficialSale {
			if common.StringsContains(configImportSeedanceSaleOptionKeys, key) {
				for modelName := range current {
					if _, updatedByImport := patches[key][modelName]; updatedByImport {
						continue
					}
					_, mappedSeedanceModel := seedanceSaleModels[modelName]
					if mappedSeedanceModel || seedancepricing.Family(modelName) != "" {
						delete(current, modelName)
					}
				}
			}
		}
		for modelName, value := range patches[key] {
			if value == nil {
				delete(current, modelName)
				continue
			}
			if key == "billing_setting."+billing_setting.BillingExprField {
				if expression, ok := value.(string); ok && strings.TrimSpace(expression) == "" {
					delete(current, modelName)
					continue
				}
			}
			if key == "billing_setting.duration_price" && configImportDurationPriceHasScenarios(value) {
				if existing, exists := current[modelName]; exists {
					merged, mergeErr := mergeConfigImportPublishedDurationPriceValue(existing, value, modelName)
					if mergeErr != nil {
						return configImportError("PUBLISH_PRICING_OPTION", "%v", mergeErr)
					}
					value = merged
				}
				encodedPrice, encodeErr := common.Marshal(value)
				if encodeErr != nil {
					return encodeErr
				}
				var durationPrice types.DurationPrice
				if decodeErr := common.Unmarshal(encodedPrice, &durationPrice); decodeErr != nil {
					return configImportError("PUBLISH_PRICING_OPTION", "duration price for model %q is invalid: %v", modelName, decodeErr)
				}
				if validateErr := durationPrice.Validate(relaycommon.MaxTaskDurationSeconds); validateErr != nil {
					return configImportError("PUBLISH_PRICING_OPTION", "duration price for model %q is invalid: %v", modelName, validateErr)
				}
			}
			if key == "billing_setting.seedance_token_price" && configImportSeedanceTokenPriceHasScenarios(value) {
				encodedPrice, encodeErr := common.Marshal(value)
				if encodeErr != nil {
					return encodeErr
				}
				var tokenPrice types.SeedanceTokenPrice
				if decodeErr := common.Unmarshal(encodedPrice, &tokenPrice); decodeErr != nil {
					return configImportError("PUBLISH_PRICING_OPTION", "Seedance token price for model %q is invalid: %v", modelName, decodeErr)
				}
				if validateErr := tokenPrice.Validate(relaycommon.MaxTokensLimit); validateErr != nil {
					return configImportError("PUBLISH_PRICING_OPTION", "Seedance token price for model %q is invalid: %v", modelName, validateErr)
				}
			}
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

func configImportSeedanceSaleCleanupScope(db *gorm.DB, items []model.ConfigImportItem) (bool, map[string]struct{}, error) {
	models := make(map[string]struct{})
	officialSale := false
	for _, item := range items {
		if item.State == string(types.ConfigImportItemStateExcluded) {
			continue
		}
		switch item.EntityType {
		case "model_mappings", "route_blueprints":
			if err := configImportCollectSeedanceSaleModels(item, models); err != nil {
				return false, nil, err
			}
		case "sale_proposals":
			var document map[string]any
			if err := common.UnmarshalJsonStr(item.CanonicalJSON, &document); err != nil {
				return false, nil, err
			}
			staged, ok := document["staged_proposal"].(map[string]any)
			if !ok {
				continue
			}
			optionPatches, ok := staged["option_patches"].(map[string]any)
			if !ok {
				continue
			}
			rawTokenPrices, ok := optionPatches["billing_setting.seedance_token_price"].(map[string]any)
			if !ok {
				continue
			}
			for modelName, value := range rawTokenPrices {
				if configImportSeedanceTokenPriceHasScenarios(value) && seedancepricing.Family(modelName) != "" {
					officialSale = true
					models[strings.TrimSpace(modelName)] = struct{}{}
				}
			}
		}
	}
	if !officialSale || db == nil || !db.Migrator().HasTable(&model.ConfigImportBatch{}) || !db.Migrator().HasTable(&model.ConfigImportItem{}) {
		return officialSale, models, nil
	}

	var publishedBatchIDs []int64
	if err := db.Model(&model.ConfigImportBatch{}).
		Where("status = ?", types.ConfigImportBatchStatusPublished).
		Order("id ASC").
		Pluck("id", &publishedBatchIDs).Error; err != nil {
		return false, nil, err
	}
	if len(publishedBatchIDs) == 0 {
		return officialSale, models, nil
	}
	var historicalItems []model.ConfigImportItem
	if err := db.Where("batch_id IN ? AND entity_type IN ?", publishedBatchIDs, []string{"model_mappings", "route_blueprints"}).
		Order("batch_id ASC, entity_type ASC, business_id ASC, id ASC").
		Find(&historicalItems).Error; err != nil {
		return false, nil, err
	}
	for _, item := range historicalItems {
		if item.State == string(types.ConfigImportItemStateExcluded) {
			continue
		}
		if err := configImportCollectSeedanceSaleModels(item, models); err != nil {
			return false, nil, err
		}
	}
	return officialSale, models, nil
}

func configImportCollectSeedanceSaleModels(item model.ConfigImportItem, models map[string]struct{}) error {
	switch item.EntityType {
	case "model_mappings":
		var mapping types.ConfigImportModelMapping
		if err := common.UnmarshalJsonStr(item.CanonicalJSON, &mapping); err != nil {
			return err
		}
		canonicalModel := configImportRuntimeCanonicalModel(mapping.CanonicalModel)
		if seedancepricing.Family(canonicalModel) == "" && seedancepricing.Family(mapping.ClientModel) == "" {
			return nil
		}
		for _, modelName := range []string{canonicalModel, mapping.CanonicalModel, mapping.ClientModel, mapping.UpstreamModel} {
			if modelName = strings.TrimSpace(modelName); modelName != "" {
				models[modelName] = struct{}{}
			}
		}
	case "route_blueprints":
		var blueprint types.ConfigImportRouteBlueprint
		if err := common.UnmarshalJsonStr(item.CanonicalJSON, &blueprint); err != nil {
			return err
		}
		canonicalModel := configImportRuntimeCanonicalModel(blueprint.CanonicalModel)
		if seedancepricing.Family(canonicalModel) == "" && seedancepricing.Family(blueprint.ClientModel) == "" {
			return nil
		}
		for _, modelName := range []string{canonicalModel, blueprint.CanonicalModel, blueprint.ClientModel} {
			if modelName = strings.TrimSpace(modelName); modelName != "" {
				models[modelName] = struct{}{}
			}
		}
		for _, target := range blueprint.Targets {
			if modelName := strings.TrimSpace(target.UpstreamModel); modelName != "" {
				models[modelName] = struct{}{}
			}
		}
	}
	return nil
}

func configImportDurationPriceHasScenarios(value any) bool {
	durationPrice, ok := value.(map[string]any)
	if !ok {
		return false
	}
	scenarios, ok := durationPrice["scenarios"].(map[string]any)
	return ok && len(scenarios) > 0
}

func configImportSeedanceTokenPriceHasScenarios(value any) bool {
	tokenPrice, ok := value.(map[string]any)
	if !ok {
		return false
	}
	scenarios, ok := tokenPrice["scenarios"].(map[string]any)
	return ok && len(scenarios) > 0
}

func mergeConfigImportPublishedDurationPriceValue(existing, incoming any, modelName string) (map[string]any, error) {
	left, leftOK := existing.(map[string]any)
	right, rightOK := incoming.(map[string]any)
	if !leftOK || !rightOK {
		return nil, fmt.Errorf("duration price for model %q is not an explicit scenario object", modelName)
	}
	rightScenarios, ok := right["scenarios"].(map[string]any)
	if !ok || len(rightScenarios) == 0 {
		return nil, fmt.Errorf("duration price for model %q has no incoming explicit scenarios", modelName)
	}

	merged := make(map[string]any, len(right))
	for key, value := range right {
		if key != "scenarios" {
			merged[key] = value
		}
	}
	mergedScenarios := make(map[string]any)
	if raw, exists := left["scenarios"]; exists {
		leftScenarios, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("duration price for model %q has invalid published scenarios", modelName)
		}
		for key, value := range leftScenarios {
			mergedScenarios[key] = value
		}
	}
	for key, value := range rightScenarios {
		mergedScenarios[key] = value
	}
	merged["scenarios"] = mergedScenarios
	return merged, nil
}

func mergeConfigImportDurationPriceValues(existing, incoming any, modelName string) (map[string]any, error) {
	left, leftOK := existing.(map[string]any)
	right, rightOK := incoming.(map[string]any)
	if !leftOK || !rightOK {
		return nil, fmt.Errorf("duration price for model %q is not an explicit scenario object", modelName)
	}
	merged := make(map[string]any, len(left)+1)
	for key, value := range left {
		merged[key] = value
	}
	for key, value := range right {
		if key == "scenarios" {
			continue
		}
		if previous, exists := merged[key]; exists {
			previousJSON, _ := common.Marshal(previous)
			valueJSON, _ := common.Marshal(value)
			if string(previousJSON) != string(valueJSON) {
				return nil, fmt.Errorf("duration price for model %q has conflicting field %q", modelName, key)
			}
			continue
		}
		merged[key] = value
	}
	leftScenarios := map[string]any{}
	if raw, exists := left["scenarios"]; exists {
		var ok bool
		leftScenarios, ok = raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("duration price for model %q has invalid scenarios", modelName)
		}
	}
	rightScenarios := map[string]any{}
	if raw, exists := right["scenarios"]; exists {
		var ok bool
		rightScenarios, ok = raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("duration price for model %q has invalid scenarios", modelName)
		}
	}
	mergedScenarios := make(map[string]any, len(leftScenarios)+len(rightScenarios))
	for key, value := range leftScenarios {
		mergedScenarios[key] = value
	}
	for key, value := range rightScenarios {
		if previous, exists := mergedScenarios[key]; exists {
			previousJSON, _ := common.Marshal(previous)
			valueJSON, _ := common.Marshal(value)
			if string(previousJSON) != string(valueJSON) {
				return nil, fmt.Errorf("duration price for model %q has conflicting scenario %q", modelName, key)
			}
			continue
		}
		mergedScenarios[key] = value
	}
	if len(mergedScenarios) == 0 {
		return nil, fmt.Errorf("duration price for model %q has no explicit scenarios", modelName)
	}
	merged["scenarios"] = mergedScenarios
	return merged, nil
}

func publishConfigImportModelMappings(tx *gorm.DB, items []model.ConfigImportItem, refresh *ConfigImportRefreshKeys) error {
	if !tx.Migrator().HasTable(&model.Channel{}) {
		return nil
	}
	targetsByChannel, err := configImportChannelModelSnapshotTargets(tx, items)
	if err != nil {
		return err
	}
	channelIDs := make([]int, 0, len(targetsByChannel))
	for channelID := range targetsByChannel {
		channelIDs = append(channelIDs, channelID)
	}
	sort.Ints(channelIDs)
	hasAbilityTable := tx.Migrator().HasTable(&model.Ability{})
	hasCostRuleTable := tx.Migrator().HasTable(&model.ChannelModelCostRule{})
	for _, channelID := range channelIDs {
		var channel model.Channel
		if err := tx.Where("id = ?", channelID).First(&channel).Error; err != nil {
			return err
		}
		currentModels, err := configImportCurrentChannelModels(&channel)
		if err != nil {
			return err
		}
		target := targetsByChannel[channelID]
		modelNames := make([]string, 0, len(target.Models))
		removedModels := make([]string, 0)
		for modelName := range target.Models {
			modelNames = append(modelNames, modelName)
		}
		for modelName := range currentModels {
			if _, retained := target.Models[modelName]; !retained {
				removedModels = append(removedModels, modelName)
			}
		}
		sort.Strings(modelNames)
		sort.Strings(removedModels)
		encoded, err := common.Marshal(target.Mapping)
		if err != nil {
			return err
		}
		channel.Models = strings.Join(modelNames, ",")
		mappingJSON := string(encoded)
		channel.ModelMapping = &mappingJSON
		if err := tx.Model(&model.Channel{}).Where("id = ?", channelID).Updates(map[string]any{
			"model_mapping": mappingJSON,
			"models":        channel.Models,
		}).Error; err != nil {
			return err
		}
		if hasAbilityTable {
			if err := channel.UpdateAbilities(tx); err != nil {
				return err
			}
		}
		if len(removedModels) > 0 && hasCostRuleTable {
			var activeRules []model.ChannelModelCostRule
			if err := tx.Where("channel_id = ? AND billable_upstream_model IN ? AND status = ?", channelID, removedModels, types.CostRuleActive).
				Order("billable_upstream_model ASC, cost_variant_key ASC, id ASC").Find(&activeRules).Error; err != nil {
				return err
			}
			if len(activeRules) > 0 {
				now := common.GetTimestamp()
				ruleIDs := make([]int64, 0, len(activeRules))
				for _, rule := range activeRules {
					ruleIDs = append(ruleIDs, rule.ID)
					refresh.CostModelKeys = appendConfigImportRefreshString(refresh.CostModelKeys, fmt.Sprintf("%d|%s|%s", rule.ChannelID, rule.BillableUpstreamModel, rule.CostVariantKey))
				}
				if err := tx.Model(&model.ChannelModelCostRule{}).Where("id IN ? AND status = ?", ruleIDs, types.CostRuleActive).Updates(map[string]any{
					"status": string(types.CostRuleRetired), "effective_to": now, "updated_at": now,
				}).Error; err != nil {
					return err
				}
			}
		}
		refresh.ChannelIDs = appendConfigImportRefreshInt(refresh.ChannelIDs, channelID)
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

type configImportPublishRouteBlueprint struct {
	item      model.ConfigImportItem
	blueprint types.ConfigImportRouteBlueprint
}

type configImportPublishedRoutePlan struct {
	PolicyKey model.RoutingPolicyKey
	MergeMode types.ConfigImportRouteMergeMode
	Defaults  modelrouting.Defaults
	Targets   []model.RouteTarget
	ItemIDs   []int64
}

func publishConfigImportRoutes(tx *gorm.DB, items []model.ConfigImportItem, _ *ConfigImportRefreshKeys) error {
	if !tx.Migrator().HasTable(&model.RoutingPolicy{}) || !tx.Migrator().HasTable(&model.RouteTarget{}) {
		return nil
	}
	if len(items) == 0 {
		return nil
	}
	batchID := items[0].BatchID
	if err := tx.Where("source_batch_id = ?", batchID).Delete(&model.RouteTarget{}).Error; err != nil {
		return err
	}
	plans, err := buildConfigImportPublishedRoutePlans(tx, items)
	if err != nil {
		return err
	}
	for _, plan := range plans {
		var policy model.RoutingPolicy
		findErr := tx.Where("group_name = ? AND model = ?", plan.PolicyKey.GroupName, plan.PolicyKey.Model).First(&policy).Error
		if errors.Is(findErr, gorm.ErrRecordNotFound) {
			policy = model.RoutingPolicy{
				GroupName:         plan.PolicyKey.GroupName,
				Model:             plan.PolicyKey.Model,
				Enabled:           false,
				DefaultResolution: plan.Defaults.OutputResolution,
				DefaultDuration:   plan.Defaults.DurationSeconds,
				DefaultRatio:      plan.Defaults.AspectRatio,
			}
			if err := tx.Create(&policy).Error; err != nil {
				return err
			}
		} else if findErr != nil {
			return findErr
		}
		for index := range plan.Targets {
			plan.Targets[index].ID = 0
			plan.Targets[index].PolicyID = policy.ID
			plan.Targets[index].ManagedBy = string(types.RouteTargetManagedByConfigImport)
			plan.Targets[index].SourceBatchID = &batchID
			plan.Targets[index].RetiredAt = nil
			plan.Targets[index].Enabled = false
		}
		if len(plan.Targets) > 0 {
			if err := tx.Create(&plan.Targets).Error; err != nil {
				return err
			}
		}
		if len(plan.ItemIDs) > 0 {
			if err := tx.Model(&model.ConfigImportItem{}).Where("id IN ?", plan.ItemIDs).Updates(map[string]any{
				"materialized_type": "routing_policy", "materialized_id": policy.ID,
			}).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func buildConfigImportPublishedRoutePlans(tx *gorm.DB, items []model.ConfigImportItem) ([]configImportPublishedRoutePlan, error) {
	lineChannels, err := configImportPublishedLineChannels(tx, items)
	if err != nil {
		return nil, err
	}
	routeBlueprints := make([]configImportPublishRouteBlueprint, 0)
	for _, item := range items {
		if item.EntityType != "route_blueprints" || item.State == string(types.ConfigImportItemStateExcluded) {
			continue
		}
		var blueprint types.ConfigImportRouteBlueprint
		if err := common.UnmarshalJsonStr(item.CanonicalJSON, &blueprint); err != nil {
			return nil, err
		}
		if blueprint.MergeMode == types.ConfigImportRouteMergeModeSkip {
			continue
		}
		if blueprint.MergeMode == "" {
			blueprint.MergeMode = types.ConfigImportRouteMergeModeMerge
		}
		routeBlueprints = append(routeBlueprints, configImportPublishRouteBlueprint{item: item, blueprint: blueprint})
	}
	sort.SliceStable(routeBlueprints, func(i, j int) bool {
		left, right := routeBlueprints[i].item, routeBlueprints[j].item
		if left.BusinessID != right.BusinessID {
			return left.BusinessID < right.BusinessID
		}
		return left.ID < right.ID
	})
	priorityOverrides := configImportRouteTargetPriorityOverrides(lineChannels, routeBlueprints)
	plansByKey := make(map[model.RoutingPolicyKey]*configImportPublishedRoutePlan)
	for _, routeBlueprint := range routeBlueprints {
		item := routeBlueprint.item
		blueprint := routeBlueprint.blueprint
		policy, targets, err := configImportRouteRowsWithPriorityOverrides(lineChannels, blueprint, priorityOverrides)
		if err != nil {
			return nil, err
		}
		key := model.RoutingPolicyKey{GroupName: policy.GroupName, Model: policy.Model}
		defaults := modelrouting.Defaults{OutputResolution: policy.DefaultResolution, DurationSeconds: policy.DefaultDuration, AspectRatio: policy.DefaultRatio}
		plan := plansByKey[key]
		if plan == nil {
			plan = &configImportPublishedRoutePlan{PolicyKey: key, MergeMode: blueprint.MergeMode, Defaults: defaults}
			plansByKey[key] = plan
		} else if plan.MergeMode != blueprint.MergeMode {
			return nil, configImportError("PUBLISH_ROUTE_PLAN", "route blueprint %q conflicts with another blueprint for %s|%s", item.BusinessID, key.GroupName, key.Model)
		}
		plan.Targets = append(plan.Targets, targets...)
		plan.ItemIDs = append(plan.ItemIDs, item.ID)
	}
	keys := make([]model.RoutingPolicyKey, 0, len(plansByKey))
	for key := range plansByKey {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].GroupName != keys[j].GroupName {
			return keys[i].GroupName < keys[j].GroupName
		}
		return keys[i].Model < keys[j].Model
	})
	plans := make([]configImportPublishedRoutePlan, 0, len(keys))
	for _, key := range keys {
		plans = append(plans, *plansByKey[key])
	}
	return plans, nil
}

func configImportRouteRows(lineChannels map[string]int, blueprint types.ConfigImportRouteBlueprint) (model.RoutingPolicy, []model.RouteTarget, error) {
	return configImportRouteRowsWithPriorityOverrides(lineChannels, blueprint, nil)
}

func configImportRoutePolicy(blueprint types.ConfigImportRouteBlueprint) model.RoutingPolicy {
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
	return policy
}

func configImportRouteRowsWithPriorityOverrides(lineChannels map[string]int, blueprint types.ConfigImportRouteBlueprint, priorityOverrides map[string]int) (model.RoutingPolicy, []model.RouteTarget, error) {
	policy := configImportRoutePolicy(blueprint)
	priorities := configImportRouteTargetPriorities(lineChannels, blueprint.Targets)
	for index, target := range blueprint.Targets {
		if priority, found := priorityOverrides[target.RouteTargetRef]; found {
			priorities[index] = priority
		}
	}
	targets := make([]model.RouteTarget, 0, len(blueprint.Targets))
	for index, target := range blueprint.Targets {
		channelID := lineChannels[target.LineRef]
		if channelID <= 0 {
			return policy, nil, configImportError("PUBLISH_LINE_UNBOUND", "route target %q references unbound line %q", target.RouteTargetRef, target.LineRef)
		}
		referenceMinimums, err := configImportReferenceLimits(target.ReferenceMinimums)
		if err != nil {
			return policy, nil, configImportError("PUBLISH_ROUTE_REFERENCE", "route target %q reference minimums are invalid: %v", target.RouteTargetRef, err)
		}
		referenceLimits, err := configImportReferenceLimits(target.ReferenceLimits)
		if err != nil {
			return policy, nil, configImportError("PUBLISH_ROUTE_REFERENCE", "route target %q reference limits are invalid: %v", target.RouteTargetRef, err)
		}
		constraints := modelrouting.Constraints{
			OutputResolutions:                  target.OutputResolutions,
			Durations:                          modelrouting.DurationConstraint{Values: target.DurationValues, Min: target.DurationMin, Max: target.DurationMax},
			AspectRatios:                       target.AspectRatios,
			ReferenceMinimums:                  referenceMinimums,
			ReferenceLimits:                    referenceLimits,
			ReferenceTotalMax:                  target.ReferenceTotalMax,
			ReferenceVideoAudioTotalMax:        target.ReferenceVideoAudioTotalMax,
			ReferenceVideoTotalDurationSeconds: target.ReferenceVideoTotalDurationSeconds,
			ReferenceModes:                     target.ReferenceModes,
			SupportsRealPerson:                 target.SupportsRealPerson,
		}
		for _, mode := range target.InputModes {
			constraints.InputModes = append(constraints.InputModes, modelrouting.InputMode(mode))
		}
		encoded, err := common.Marshal(constraints)
		if err != nil {
			return policy, nil, err
		}
		enabled := false
		targets = append(targets, model.RouteTarget{ChannelID: channelID, Name: target.RouteTargetRef, UpstreamModel: target.UpstreamModel, CostVariantKey: target.CostVariantKey, TargetPriority: priorities[index], Enabled: enabled, Constraints: string(encoded)})
	}
	return policy, targets, nil
}

func configImportRouteTargetPriorityOverrides(lineChannels map[string]int, blueprints []configImportPublishRouteBlueprint) map[string]int {
	// Priorities are allocated across every blueprint in one publish batch so
	// separately stored route rows for the same channel cannot collide.
	type priorityKey struct {
		canonicalModel string
		channelID      int
	}
	targetCountByKey := make(map[priorityKey]int)
	usedByKey := make(map[priorityKey]map[int]struct{})
	for _, routeBlueprint := range blueprints {
		canonicalModel := configImportRuntimeCanonicalModel(routeBlueprint.blueprint.CanonicalModel)
		for _, target := range routeBlueprint.blueprint.Targets {
			key := priorityKey{canonicalModel: canonicalModel, channelID: lineChannels[target.LineRef]}
			targetCountByKey[key]++
			if target.Priority == nil {
				continue
			}
			if usedByKey[key] == nil {
				usedByKey[key] = make(map[int]struct{})
			}
			usedByKey[key][*target.Priority] = struct{}{}
		}
	}

	overrides := make(map[string]int)
	nextByKey := make(map[priorityKey]int)
	for _, routeBlueprint := range blueprints {
		canonicalModel := configImportRuntimeCanonicalModel(routeBlueprint.blueprint.CanonicalModel)
		for _, target := range routeBlueprint.blueprint.Targets {
			if target.Priority != nil {
				continue
			}
			key := priorityKey{canonicalModel: canonicalModel, channelID: lineChannels[target.LineRef]}
			candidate, initialized := nextByKey[key]
			if !initialized {
				candidate = targetCountByKey[key] - 1
			}
			if usedByKey[key] == nil {
				usedByKey[key] = make(map[int]struct{})
			}
			for {
				if _, used := usedByKey[key][candidate]; !used {
					break
				}
				candidate--
			}
			overrides[target.RouteTargetRef] = candidate
			usedByKey[key][candidate] = struct{}{}
			nextByKey[key] = candidate - 1
		}
	}
	return overrides
}

// configImportRouteTargetPriorities preserves explicit priorities and assigns
// descending, collision-free defaults to targets on the same channel. The
// blueprint order determines precedence for targets without an explicit value.
func configImportRouteTargetPriorities(lineChannels map[string]int, targets []types.ConfigImportRouteTarget) []int {
	overrides := configImportRouteTargetPriorityOverrides(lineChannels, []configImportPublishRouteBlueprint{{
		blueprint: types.ConfigImportRouteBlueprint{Targets: targets},
	}})
	priorities := make([]int, len(targets))
	for index, target := range targets {
		if target.Priority != nil {
			priorities[index] = *target.Priority
			continue
		}
		priorities[index] = overrides[target.RouteTargetRef]
	}
	return priorities
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

func configImportReferenceLimits(bounds *types.ConfigImportReferenceBounds) (modelrouting.ReferenceLimits, error) {
	if bounds == nil || bounds.Images == nil || bounds.Videos == nil || bounds.Audios == nil {
		return modelrouting.ReferenceLimits{}, errors.New("reference limits must include images, videos, and audios")
	}
	return modelrouting.ReferenceLimits{
		Images: *bounds.Images,
		Videos: *bounds.Videos,
		Audios: *bounds.Audios,
	}, nil
}
