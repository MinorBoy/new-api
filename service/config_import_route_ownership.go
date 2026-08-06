package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelrouting"
	"github.com/QuantumNous/new-api/types"
	"gorm.io/gorm"
)

type configImportRouteOwnershipKey struct {
	GroupName      string
	CanonicalModel string
	ChannelID      int
	RouteTargetRef string
	UpstreamModel  string
	CostVariantKey string
	Constraints    string
}

type configImportRouteOwnershipBatchCandidate struct {
	BatchID     int64
	PublishedAt *int64
}

func PreviewConfigImportRouteOwnershipBackfill(ctx context.Context) (*dto.ConfigImportRouteOwnershipReport, error) {
	return previewConfigImportRouteOwnershipBackfill(model.DB.WithContext(ctx))
}

func ApplyConfigImportRouteOwnershipBackfill(ctx context.Context, adminID int) (*dto.ConfigImportRouteOwnershipReport, error) {
	if adminID <= 0 {
		return nil, configImportError("SCHEMA_ADMIN", "admin ID is required")
	}

	report, err := previewConfigImportRouteOwnershipBackfill(model.DB.WithContext(ctx))
	if err != nil {
		return nil, err
	}
	if len(report.Matched) == 0 {
		return report, nil
	}

	operationID := common.GetUUID()
	appliedCount := 0
	err = model.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, candidate := range report.Matched {
			if candidate.BatchID == nil {
				continue
			}
			var current model.RouteTarget
			if err := model.LockModelForUpdate(tx, &model.RouteTarget{}).
				Where("id = ?", candidate.RouteTargetID).First(&current).Error; err != nil {
				return err
			}
			managedBy, err := types.NormalizeRouteTargetManagedBy(current.ManagedBy)
			if err != nil {
				return err
			}
			if managedBy != types.RouteTargetManagedByManual {
				continue
			}

			previousManagedBy := string(managedBy)
			previousSourceBatchID := current.SourceBatchID
			now := common.GetTimestamp()
			if err := tx.Model(&model.RouteTarget{}).Where("id = ?", current.ID).Updates(map[string]any{
				"managed_by":      string(types.RouteTargetManagedByConfigImport),
				"source_batch_id": *candidate.BatchID,
				"updated_at":      now,
			}).Error; err != nil {
				return err
			}
			if err := tx.First(&current, current.ID).Error; err != nil {
				return err
			}
			fingerprint, err := configImportRouteTargetOwnershipFingerprint(current)
			if err != nil {
				return err
			}
			change := model.ConfigImportRouteOwnershipChange{
				OperationID:            operationID,
				RouteTargetID:          current.ID,
				PreviousManagedBy:      previousManagedBy,
				PreviousSourceBatchID:  previousSourceBatchID,
				AssignedBatchID:        *candidate.BatchID,
				AppliedTargetUpdatedAt: current.UpdatedAt,
				AppliedTargetSHA256:    fingerprint,
				AppliedBy:              adminID,
			}
			if err := tx.Create(&change).Error; err != nil {
				return err
			}
			appliedCount++
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if appliedCount > 0 {
		report.OperationID = operationID
	}
	report.AppliedCount = appliedCount
	return report, nil
}

func RollbackConfigImportRouteOwnershipBackfill(ctx context.Context, adminID int, operationID string) (*dto.ConfigImportRouteOwnershipReport, error) {
	if adminID <= 0 {
		return nil, configImportError("SCHEMA_ADMIN", "admin ID is required")
	}
	operationID = strings.TrimSpace(operationID)
	if operationID == "" {
		return nil, configImportError("ROUTE_OWNERSHIP_OPERATION_ID", "operation ID is required")
	}

	report := newConfigImportRouteOwnershipReport()
	report.OperationID = operationID
	err := model.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var changes []model.ConfigImportRouteOwnershipChange
		if err := model.LockModelForUpdate(tx, &model.ConfigImportRouteOwnershipChange{}).
			Where("operation_id = ?", operationID).
			Order("route_target_id ASC").
			Find(&changes).Error; err != nil {
			return err
		}
		if len(changes) == 0 {
			return configImportError("ROUTE_OWNERSHIP_ROLLBACK_EMPTY", "operation %s has no ownership changes", operationID)
		}
		for _, change := range changes {
			if change.RevertedAt != nil {
				return configImportError("ROUTE_OWNERSHIP_ALREADY_REVERTED", "operation %s was already reverted", operationID)
			}
		}

		type rollbackTarget struct {
			change  model.ConfigImportRouteOwnershipChange
			current model.RouteTarget
		}
		locked := make([]rollbackTarget, 0, len(changes))
		for _, change := range changes {
			var current model.RouteTarget
			if err := model.LockModelForUpdate(tx, &model.RouteTarget{}).
				Where("id = ?", change.RouteTargetID).First(&current).Error; err != nil {
				return err
			}
			fingerprint, err := configImportRouteTargetOwnershipFingerprint(current)
			if err != nil {
				return err
			}
			if current.ManagedBy != string(types.RouteTargetManagedByConfigImport) ||
				current.SourceBatchID == nil || *current.SourceBatchID != change.AssignedBatchID ||
				current.UpdatedAt != change.AppliedTargetUpdatedAt ||
				fingerprint != change.AppliedTargetSHA256 {
				return configImportError("ROUTE_OWNERSHIP_ROLLBACK_CONFLICT", "route target %d changed after operation %s", change.RouteTargetID, operationID)
			}
			locked = append(locked, rollbackTarget{change: change, current: current})
		}

		now := common.GetTimestamp()
		for _, item := range locked {
			var previousSourceBatchID any
			if item.change.PreviousSourceBatchID != nil {
				previousSourceBatchID = *item.change.PreviousSourceBatchID
			}
			if err := tx.Model(&model.RouteTarget{}).Where("id = ?", item.current.ID).Updates(map[string]any{
				"managed_by":      item.change.PreviousManagedBy,
				"source_batch_id": previousSourceBatchID,
				"updated_at":      now,
			}).Error; err != nil {
				return err
			}
			assignedBatchID := item.change.AssignedBatchID
			report.Matched = append(report.Matched, dto.ConfigImportRouteOwnershipCandidate{
				RouteTargetID:  item.current.ID,
				PolicyID:       item.current.PolicyID,
				RouteTargetRef: item.current.Name,
				ChannelID:      item.current.ChannelID,
				BatchID:        &assignedBatchID,
				Reason:         "reverted",
			})
		}
		result := tx.Model(&model.ConfigImportRouteOwnershipChange{}).
			Where("operation_id = ? AND reverted_at IS NULL", operationID).
			Updates(map[string]any{"reverted_by": adminID, "reverted_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != int64(len(changes)) {
			return configImportError("ROUTE_OWNERSHIP_ROLLBACK_CONFLICT", "operation %s changed while it was being reverted", operationID)
		}
		report.RevertedCount = len(changes)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return report, nil
}

func previewConfigImportRouteOwnershipBackfill(db *gorm.DB) (*dto.ConfigImportRouteOwnershipReport, error) {
	report := newConfigImportRouteOwnershipReport()
	candidatesByKey, err := configImportRouteOwnershipCandidates(db)
	if err != nil {
		return nil, err
	}

	var targets []model.RouteTarget
	if err := db.Where("managed_by IN ?", []string{
		string(types.RouteTargetManagedByManual), "",
	}).Order("id ASC").Find(&targets).Error; err != nil {
		return nil, err
	}
	if len(targets) == 0 {
		return report, nil
	}

	policyIDs := make([]int, 0, len(targets))
	seenPolicyIDs := make(map[int]struct{}, len(targets))
	for _, target := range targets {
		if _, exists := seenPolicyIDs[target.PolicyID]; exists {
			continue
		}
		seenPolicyIDs[target.PolicyID] = struct{}{}
		policyIDs = append(policyIDs, target.PolicyID)
	}
	var policies []model.RoutingPolicy
	if err := db.Where("id IN ?", policyIDs).Find(&policies).Error; err != nil {
		return nil, err
	}
	policiesByID := make(map[int]model.RoutingPolicy, len(policies))
	for _, policy := range policies {
		policiesByID[policy.ID] = policy
	}

	for _, target := range targets {
		policy, exists := policiesByID[target.PolicyID]
		if !exists {
			continue
		}
		variant, err := types.NormalizeCostVariantKey(target.CostVariantKey)
		if err != nil {
			return nil, fmt.Errorf("route target %d cost variant: %w", target.ID, err)
		}
		constraints, err := normalizeConfigImportRouteOwnershipConstraints(target.Constraints)
		if err != nil {
			return nil, fmt.Errorf("route target %d constraints: %w", target.ID, err)
		}
		key := configImportRouteOwnershipKey{
			GroupName:      policy.GroupName,
			CanonicalModel: policy.Model,
			ChannelID:      target.ChannelID,
			RouteTargetRef: target.Name,
			UpstreamModel:  target.UpstreamModel,
			CostVariantKey: variant,
			Constraints:    constraints,
		}
		candidates := candidatesByKey[key]
		row := dto.ConfigImportRouteOwnershipCandidate{
			RouteTargetID:  target.ID,
			PolicyID:       target.PolicyID,
			RouteTargetRef: target.Name,
			ChannelID:      target.ChannelID,
		}
		for _, candidate := range candidates {
			row.CandidateBatchIDs = append(row.CandidateBatchIDs, candidate.BatchID)
		}
		switch len(candidates) {
		case 0:
			row.Reason = "no_candidate_batch"
			report.Unmatched = append(report.Unmatched, row)
		case 1:
			batchID := candidates[0].BatchID
			row.BatchID = &batchID
			row.Reason = "unique_semantic_match"
			report.Matched = append(report.Matched, row)
		default:
			timestampMatches := make([]configImportRouteOwnershipBatchCandidate, 0, len(candidates))
			for _, candidate := range candidates {
				if candidate.PublishedAt != nil && *candidate.PublishedAt == target.CreatedAt {
					timestampMatches = append(timestampMatches, candidate)
				}
			}
			if len(timestampMatches) == 1 {
				batchID := timestampMatches[0].BatchID
				row.BatchID = &batchID
				row.Reason = "published_at_matches_created_at"
				report.Matched = append(report.Matched, row)
				continue
			}
			row.Reason = "multiple_candidate_batches"
			report.Ambiguous = append(report.Ambiguous, row)
		}
	}
	configImportSortRouteOwnershipReport(report)
	return report, nil
}

func configImportRouteOwnershipCandidates(db *gorm.DB) (map[configImportRouteOwnershipKey][]configImportRouteOwnershipBatchCandidate, error) {
	var batches []model.ConfigImportBatch
	if err := db.Where("status = ?", types.ConfigImportBatchStatusPublished).Order("id ASC").Find(&batches).Error; err != nil {
		return nil, err
	}
	result := make(map[configImportRouteOwnershipKey][]configImportRouteOwnershipBatchCandidate)
	if len(batches) == 0 {
		return result, nil
	}

	batchIDs := make([]int64, 0, len(batches))
	batchesByID := make(map[int64]model.ConfigImportBatch, len(batches))
	for _, batch := range batches {
		batchIDs = append(batchIDs, batch.ID)
		batchesByID[batch.ID] = batch
	}
	var bindings []model.ConfigImportBinding
	if err := db.Where("batch_id IN ? AND action IN ? AND channel_id IS NOT NULL", batchIDs, []string{
		string(types.ConfigImportBindingActionBind),
		string(types.ConfigImportBindingActionCreate),
	}).Find(&bindings).Error; err != nil {
		return nil, err
	}
	lineChannelsByBatch := make(map[int64]map[string]int, len(batches))
	for _, binding := range bindings {
		if binding.ChannelID == nil {
			continue
		}
		if lineChannelsByBatch[binding.BatchID] == nil {
			lineChannelsByBatch[binding.BatchID] = make(map[string]int)
		}
		lineChannelsByBatch[binding.BatchID][binding.LineRef] = *binding.ChannelID
	}

	var items []model.ConfigImportItem
	if err := db.Where("batch_id IN ? AND entity_type = ? AND state <> ?", batchIDs, "route_blueprints", types.ConfigImportItemStateExcluded).
		Order("batch_id ASC, id ASC").Find(&items).Error; err != nil {
		return nil, err
	}
	seen := make(map[configImportRouteOwnershipKey]map[int64]struct{})
	for _, item := range items {
		var blueprint types.ConfigImportRouteBlueprint
		if err := common.UnmarshalJsonStr(item.CanonicalJSON, &blueprint); err != nil {
			return nil, fmt.Errorf("decode route blueprint %s: %w", item.BusinessID, err)
		}
		if blueprint.MergeMode == types.ConfigImportRouteMergeModeSkip {
			continue
		}
		policy, targets, err := configImportRouteRows(lineChannelsByBatch[item.BatchID], blueprint)
		if err != nil {
			// Older published imports predate the complete reference-bound
			// contract. They cannot be matched deterministically, so leave
			// their existing targets manual instead of blocking the full
			// backfill preview. New publishes remain strict.
			var schemaErr *ConfigImportSchemaError
			if errors.As(err, &schemaErr) && schemaErr.Code == "PUBLISH_ROUTE_REFERENCE" {
				continue
			}
			return nil, err
		}
		batch := batchesByID[item.BatchID]
		for _, target := range targets {
			variant, err := types.NormalizeCostVariantKey(target.CostVariantKey)
			if err != nil {
				return nil, err
			}
			constraints, err := normalizeConfigImportRouteOwnershipConstraints(target.Constraints)
			if err != nil {
				return nil, err
			}
			key := configImportRouteOwnershipKey{
				GroupName:      policy.GroupName,
				CanonicalModel: policy.Model,
				ChannelID:      target.ChannelID,
				RouteTargetRef: target.Name,
				UpstreamModel:  target.UpstreamModel,
				CostVariantKey: variant,
				Constraints:    constraints,
			}
			if seen[key] == nil {
				seen[key] = make(map[int64]struct{})
			}
			if _, exists := seen[key][batch.ID]; exists {
				continue
			}
			seen[key][batch.ID] = struct{}{}
			result[key] = append(result[key], configImportRouteOwnershipBatchCandidate{
				BatchID: batch.ID, PublishedAt: batch.PublishedAt,
			})
		}
	}
	for key := range result {
		sort.Slice(result[key], func(i, j int) bool {
			return result[key][i].BatchID < result[key][j].BatchID
		})
	}
	return result, nil
}

func normalizeConfigImportRouteOwnershipConstraints(encoded string) (string, error) {
	var constraints modelrouting.Constraints
	if err := common.UnmarshalJsonStr(encoded, &constraints); err != nil {
		return "", err
	}
	normalized, err := common.Marshal(constraints)
	if err != nil {
		return "", err
	}
	return string(normalized), nil
}

func configImportRouteTargetOwnershipFingerprint(target model.RouteTarget) (string, error) {
	payload := struct {
		ID                       int    `json:"id"`
		PolicyID                 int    `json:"policy_id"`
		ChannelID                int    `json:"channel_id"`
		Name                     string `json:"name"`
		UpstreamModel            string `json:"upstream_model"`
		CostVariantKey           string `json:"cost_variant_key"`
		TargetPriority           int    `json:"target_priority"`
		MinimumExpectedMarginBPS *int   `json:"minimum_expected_margin_bps"`
		Constraints              string `json:"constraints"`
		Enabled                  bool   `json:"enabled"`
		ManagedBy                string `json:"managed_by"`
		SourceBatchID            *int64 `json:"source_batch_id"`
		RetiredAt                *int64 `json:"retired_at"`
	}{
		target.ID, target.PolicyID, target.ChannelID, target.Name, target.UpstreamModel,
		target.CostVariantKey, target.TargetPriority, target.MinimumExpectedMarginBPS,
		target.Constraints, target.Enabled, target.ManagedBy, target.SourceBatchID, target.RetiredAt,
	}
	encoded, err := common.Marshal(payload)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func newConfigImportRouteOwnershipReport() *dto.ConfigImportRouteOwnershipReport {
	return &dto.ConfigImportRouteOwnershipReport{
		Matched:   []dto.ConfigImportRouteOwnershipCandidate{},
		Ambiguous: []dto.ConfigImportRouteOwnershipCandidate{},
		Unmatched: []dto.ConfigImportRouteOwnershipCandidate{},
	}
}

func configImportSortRouteOwnershipReport(report *dto.ConfigImportRouteOwnershipReport) {
	for _, rows := range [][]dto.ConfigImportRouteOwnershipCandidate{report.Matched, report.Ambiguous, report.Unmatched} {
		sort.Slice(rows, func(i, j int) bool {
			return rows[i].RouteTargetID < rows[j].RouteTargetID
		})
	}
}
