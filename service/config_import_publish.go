package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
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
			if err := model.DB.Where("key = ?", key).First(&option).Error; err != nil {
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

// PublishConfigImportBatch applies a ready batch as one database transaction.
// The current implementation publishes the staged cost drafts and records an
// auditable state transition; option, mapping, and route proposal application
// remain intentionally disabled until their concrete writers are supplied.
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
		now := common.GetTimestamp()
		if err := tx.Create(&model.ConfigImportPublishAudit{BatchID: batchID, AdminID: adminID, BeforeSHA256: baseline.Hash, AfterSHA256: current.Hash, Outcome: "published", CreatedAt: now}).Error; err != nil {
			return err
		}
		return tx.Model(&model.ConfigImportBatch{}).Where("id = ? AND status = ?", batchID, types.ConfigImportBatchStatusPublishing).Updates(map[string]any{
			"status": string(types.ConfigImportBatchStatusPublished), "published_at": now, "updated_at": now,
		}).Error
	})
	if err != nil {
		return err
	}
	if err := RefreshPublishedConfig(refresh); err != nil {
		return fmt.Errorf("published but cache refresh failed: %w", err)
	}
	return nil
}
