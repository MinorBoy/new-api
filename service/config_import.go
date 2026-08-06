package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/types"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	configImportDefaultPageSize = 20
	configImportMaxPageSize     = 100
)

type configImportBatchSummaryStorage struct {
	ItemCounts            types.ConfigImportEntityCounts               `json:"item_counts"`
	IssueCount            int                                          `json:"issue_count"`
	ChannelModelSnapshots []types.ConfigImportChannelModelSnapshotDiff `json:"channel_model_snapshots,omitempty"`
}

// CreateConfigImportBatch persists a credential-free, normalized import
// document. An upload with an existing authoritative payload hash returns the
// already materialized batch without writing duplicate rows.
func CreateConfigImportBatch(ctx context.Context, adminID int, reader io.Reader) (*dto.ConfigImportBatchDetail, bool, error) {
	if adminID <= 0 {
		return nil, false, configImportError("SCHEMA_ADMIN", "admin ID is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	document, err := ParseConfigImportDocument(reader)
	if err != nil {
		return nil, false, err
	}
	normalizeConfigImportDocument(document)

	batchID, created, err := createConfigImportBatch(ctx, adminID, document)
	if err != nil {
		// A concurrent transaction may win the unique payload-hash insertion
		// after the initial lookup. Read the durable winner after rollback.
		var existing model.ConfigImportBatch
		lookupErr := model.DB.WithContext(ctx).
			Where("deduplication_key = ?", model.ConfigImportUploadDeduplicationKey(document.Manifest.PayloadSHA256)).
			First(&existing).Error
		if lookupErr != nil {
			lookupErr = model.DB.WithContext(ctx).
				Where("payload_sha256 = ? AND (deduplication_key IS NULL OR deduplication_key = '')", document.Manifest.PayloadSHA256).
				First(&existing).Error
		}
		if lookupErr != nil {
			return nil, false, err
		}
		batchID = existing.ID
		created = false
	}

	batch, err := GetConfigImportBatch(ctx, batchID)
	if err != nil {
		return nil, false, err
	}
	return batch, created, nil
}

func createConfigImportBatch(ctx context.Context, adminID int, document *types.ConfigImportDocument) (int64, bool, error) {
	var batchID int64
	created := false
	deduplicationKey := model.ConfigImportUploadDeduplicationKey(document.Manifest.PayloadSHA256)
	err := model.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing model.ConfigImportBatch
		payloadQuery := tx.Where("deduplication_key = ?", deduplicationKey)
		if !common.UsingMainDatabase(common.DatabaseTypeSQLite) {
			payloadQuery = payloadQuery.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		err := payloadQuery.First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = tx.Where("payload_sha256 = ? AND (deduplication_key IS NULL OR deduplication_key = '')", document.Manifest.PayloadSHA256).First(&existing).Error
		}
		if err == nil {
			batchID = existing.ID
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		items, err := normalizedConfigImportItems(document)
		if err != nil {
			return err
		}
		issues := configImportPersistedIssues(document)
		summaryJSON, err := common.Marshal(configImportBatchSummaryStorage{
			ItemCounts: configImportEntityCounts(document.Entities),
			IssueCount: len(issues),
		})
		if err != nil {
			return fmt.Errorf("marshal config import summary: %w", err)
		}
		baselineJSON, err := common.Marshal(map[string]any{})
		if err != nil {
			return fmt.Errorf("marshal config import baseline: %w", err)
		}
		now := common.GetTimestamp()
		batch := model.ConfigImportBatch{
			SchemaVersion:    document.SchemaVersion,
			TemplateVersion:  document.TemplateVersion,
			SourceSHA256:     document.Manifest.SourceSHA256,
			PayloadSHA256:    document.Manifest.PayloadSHA256,
			DeduplicationKey: &deduplicationKey,
			Status:           string(types.ConfigImportBatchStatusValidating),
			CreatedBy:        adminID,
			SummaryJSON:      model.ConfigImportSummaryJSON(summaryJSON),
			BaselineJSON:     model.ConfigImportBaselineJSON(baselineJSON),
		}
		if err := tx.Create(&batch).Error; err != nil {
			return err
		}
		for index := range items {
			items[index].BatchID = batch.ID
		}
		if len(items) > 0 {
			if err := tx.CreateInBatches(items, 100).Error; err != nil {
				return err
			}
		}
		for index := range issues {
			issues[index].BatchID = batch.ID
		}
		if len(issues) > 0 {
			if err := tx.CreateInBatches(issues, 100).Error; err != nil {
				return err
			}
		}

		nextStatus := types.ConfigImportBatchStatusBinding
		if configImportIssuesHaveFailure(issues) {
			nextStatus = types.ConfigImportBatchStatusBlocked
		}
		updated, err := model.UpdateConfigImportBatchStatus(
			tx,
			batch.ID,
			types.ConfigImportBatchStatusValidating,
			nextStatus,
		)
		if err != nil {
			return err
		}
		if !updated {
			return errors.New("config import batch status transition was not applied")
		}
		if err := tx.Model(&model.ConfigImportBatch{}).Where("id = ?", batch.ID).
			Update("validated_at", now).Error; err != nil {
			return err
		}
		batchID = batch.ID
		created = true
		return nil
	})
	return batchID, created, err
}

// CopyConfigImportBatchForBinding creates an explicit fresh binding attempt
// from a published batch. It preserves authoritative entities but never
// carries over credentials, review state, issues, or materialized IDs.
func CopyConfigImportBatchForBinding(ctx context.Context, adminID int, sourceBatchID int64) (*dto.ConfigImportBatchDetail, error) {
	if adminID <= 0 {
		return nil, configImportError("SCHEMA_ADMIN", "admin ID is required")
	}
	if sourceBatchID <= 0 {
		return nil, configImportError("SCHEMA_BATCH_ID", "batch ID is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var copiedBatchID int64
	err := model.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var source model.ConfigImportBatch
		if err := model.LockModelForUpdate(tx, &model.ConfigImportBatch{}).Where("id = ?", sourceBatchID).First(&source).Error; err != nil {
			return err
		}
		if types.ConfigImportBatchStatus(source.Status) != types.ConfigImportBatchStatusPublished {
			return configImportError("COPY_FOR_BINDING_SOURCE_STATUS", "batch %d is not published", sourceBatchID)
		}

		var items []model.ConfigImportItem
		if err := tx.Where("batch_id = ?", source.ID).Order("entity_type ASC, business_id ASC, id ASC").Find(&items).Error; err != nil {
			return err
		}
		var storedSummary configImportBatchSummaryStorage
		if err := common.UnmarshalJsonStr(string(source.SummaryJSON), &storedSummary); err != nil {
			return fmt.Errorf("decode config import batch %d summary: %w", source.ID, err)
		}
		storedSummary.IssueCount = 0
		storedSummary.ChannelModelSnapshots = nil
		summaryJSON, err := common.Marshal(storedSummary)
		if err != nil {
			return fmt.Errorf("marshal copied config import summary: %w", err)
		}
		baselineJSON, err := common.Marshal(map[string]any{})
		if err != nil {
			return fmt.Errorf("marshal copied config import baseline: %w", err)
		}
		deduplicationKey := "copy:" + common.GetUUID()
		copiedFromBatchID := source.ID
		batch := model.ConfigImportBatch{
			SchemaVersion:     source.SchemaVersion,
			TemplateVersion:   source.TemplateVersion,
			SourceSHA256:      source.SourceSHA256,
			PayloadSHA256:     source.PayloadSHA256,
			DeduplicationKey:  &deduplicationKey,
			CopiedFromBatchID: &copiedFromBatchID,
			Status:            string(types.ConfigImportBatchStatusBinding),
			CreatedBy:         adminID,
			SummaryJSON:       model.ConfigImportSummaryJSON(summaryJSON),
			BaselineJSON:      model.ConfigImportBaselineJSON(baselineJSON),
		}
		if err := tx.Create(&batch).Error; err != nil {
			return err
		}
		for _, item := range items {
			copiedItem := model.ConfigImportItem{
				BatchID:       batch.ID,
				EntityType:    item.EntityType,
				BusinessID:    item.BusinessID,
				EntityHash:    item.EntityHash,
				CanonicalJSON: item.CanonicalJSON,
				State:         string(types.ConfigImportItemStateNew),
				SourceRef:     item.SourceRef,
				SourceSheet:   item.SourceSheet,
				SourceRow:     item.SourceRow,
			}
			if err := tx.Create(&copiedItem).Error; err != nil {
				return err
			}
		}
		copiedBatchID = batch.ID
		return nil
	})
	if err != nil {
		return nil, err
	}
	return GetConfigImportBatch(ctx, copiedBatchID)
}

// ListConfigImportBatches returns newest batches first. Pagination is bounded
// so list views cannot request unbounded database work.
func ListConfigImportBatches(ctx context.Context, page, pageSize int) (*dto.ConfigImportBatchPage, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = configImportDefaultPageSize
	}
	if pageSize > configImportMaxPageSize {
		pageSize = configImportMaxPageSize
	}

	pageResult := &dto.ConfigImportBatchPage{Page: page, PageSize: pageSize, Items: []dto.ConfigImportBatchSummary{}}
	if err := model.DB.WithContext(ctx).Model(&model.ConfigImportBatch{}).Count(&pageResult.Total).Error; err != nil {
		return nil, err
	}
	if int64(page-1) > math.MaxInt64/int64(pageSize) {
		return pageResult, nil
	}

	var batches []model.ConfigImportBatch
	offset := (page - 1) * pageSize
	if err := model.DB.WithContext(ctx).
		Order("id DESC").Offset(offset).Limit(pageSize).Find(&batches).Error; err != nil {
		return nil, err
	}
	issuesByBatchID := make(map[int64][]model.ConfigImportIssue, len(batches))
	if len(batches) > 0 {
		batchIDs := make([]int64, 0, len(batches))
		for index := range batches {
			batchIDs = append(batchIDs, batches[index].ID)
		}
		var issues []model.ConfigImportIssue
		if err := model.DB.WithContext(ctx).Where("batch_id IN ?", batchIDs).Find(&issues).Error; err != nil {
			return nil, err
		}
		for index := range issues {
			issuesByBatchID[issues[index].BatchID] = append(issuesByBatchID[issues[index].BatchID], issues[index])
		}
	}
	for index := range batches {
		summary, err := configImportBatchSummary(&batches[index], issuesByBatchID[batches[index].ID])
		if err != nil {
			return nil, err
		}
		pageResult.Items = append(pageResult.Items, summary)
	}
	return pageResult, nil
}

// GetConfigImportBatch rebuilds a batch detail solely from normalized rows;
// neither the original upload body nor derived preview is retained.
func GetConfigImportBatch(ctx context.Context, batchID int64) (*dto.ConfigImportBatchDetail, error) {
	if batchID <= 0 {
		return nil, errors.New("config import batch ID is invalid")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var batch model.ConfigImportBatch
	if err := model.DB.WithContext(ctx).Where("id = ?", batchID).First(&batch).Error; err != nil {
		return nil, err
	}
	summary, err := configImportBatchSummary(&batch, nil)
	if err != nil {
		return nil, err
	}

	var items []model.ConfigImportItem
	if err := model.DB.WithContext(ctx).Where("batch_id = ?", batch.ID).
		Order("entity_type ASC, business_id ASC, id ASC").Find(&items).Error; err != nil {
		return nil, err
	}
	var issues []model.ConfigImportIssue
	if err := model.DB.WithContext(ctx).Where("batch_id = ?", batch.ID).
		Order("id ASC").Find(&issues).Error; err != nil {
		return nil, err
	}
	var bindings []model.ConfigImportBinding
	if err := model.DB.WithContext(ctx).Where("batch_id = ?", batch.ID).
		Order("line_ref ASC, id ASC").Find(&bindings).Error; err != nil {
		return nil, err
	}

	detail := &dto.ConfigImportBatchDetail{
		ConfigImportBatchSummary: summary,
		Items:                    make([]dto.ConfigImportItemDetail, 0, len(items)),
		Bindings:                 make([]dto.ConfigImportBindingDetail, 0, len(bindings)),
		Issues:                   make([]dto.ConfigImportIssueDetail, 0, len(issues)),
		ChannelModelSnapshots:    []types.ConfigImportChannelModelSnapshotDiff{},
	}
	var storedSummary configImportBatchSummaryStorage
	if err := common.UnmarshalJsonStr(string(batch.SummaryJSON), &storedSummary); err != nil {
		return nil, fmt.Errorf("decode config import batch %d summary: %w", batch.ID, err)
	}
	if storedSummary.ChannelModelSnapshots != nil {
		detail.ChannelModelSnapshots = storedSummary.ChannelModelSnapshots
	}
	for index := range items {
		detail.Items = append(detail.Items, dto.ConfigImportItemDetail{
			ID:               items[index].ID,
			EntityType:       items[index].EntityType,
			BusinessID:       items[index].BusinessID,
			EntityHash:       items[index].EntityHash,
			CanonicalJSON:    items[index].CanonicalJSON,
			State:            types.ConfigImportItemState(items[index].State),
			SourceRef:        items[index].SourceRef,
			SourceSheet:      items[index].SourceSheet,
			SourceRow:        items[index].SourceRow,
			MaterializedType: items[index].MaterializedType,
			MaterializedID:   items[index].MaterializedID,
			ConflictReason:   items[index].ConflictReason,
			ExclusionReason:  items[index].ExclusionReason,
		})
	}
	for index := range bindings {
		binding := bindings[index]
		detail.Bindings = append(detail.Bindings, dto.ConfigImportBindingDetail{
			LineRef:              binding.LineRef,
			Action:               binding.Action,
			ChannelID:            binding.ChannelID,
			CredentialsConfirmed: binding.CredentialsConfirmedAt != nil,
		})
	}
	for index := range issues {
		detail.Issues = append(detail.Issues, dto.ConfigImportIssueDetail{
			ID:               issues[index].ID,
			Severity:         types.ConfigImportIssueSeverity(issues[index].Severity),
			Code:             issues[index].Code,
			EntityType:       issues[index].EntityType,
			BusinessID:       issues[index].BusinessID,
			Sheet:            issues[index].Sheet,
			Row:              issues[index].Row,
			Field:            issues[index].Field,
			Message:          issues[index].Message,
			Suggestion:       issues[index].Suggestion,
			ResolutionStatus: issues[index].ResolutionStatus,
		})
	}
	detail.AllowedActions = configImportAllowedActions(types.ConfigImportBatchStatus(batch.Status), batch.ActivatedAt, issues)
	if types.ConfigImportBatchStatus(batch.Status) == types.ConfigImportBatchStatusPublished && batch.ActivatedAt == nil {
		preview, err := PreviewConfigImportBatchActivation(ctx, batch.ID)
		if err != nil {
			return nil, err
		}
		detail.ActivationPreview = preview
	}
	return detail, nil
}

func normalizedConfigImportItems(document *types.ConfigImportDocument) ([]model.ConfigImportItem, error) {
	items := make([]model.ConfigImportItem, 0,
		len(document.Entities.Channels)+len(document.Entities.ChannelLines)+len(document.Entities.ModelSKUs)+
			len(document.Entities.SaleProposals)+len(document.Entities.CostRuleDrafts)+len(document.Entities.ModelMappings)+
			len(document.Entities.RouteBlueprints)+len(document.Entities.Sources)+len(document.Entities.UnresolvedVariants),
	)
	for index := range document.Entities.Channels {
		item, err := normalizedConfigImportItem("channels", document.Entities.Channels[index].ConfigImportAuthoritativeEntity, document.Entities.Channels[index])
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	for index := range document.Entities.ChannelLines {
		item, err := normalizedConfigImportItem("channel_lines", document.Entities.ChannelLines[index].ConfigImportAuthoritativeEntity, document.Entities.ChannelLines[index])
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	for index := range document.Entities.ModelSKUs {
		item, err := normalizedConfigImportItem("model_skus", document.Entities.ModelSKUs[index].ConfigImportAuthoritativeEntity, document.Entities.ModelSKUs[index])
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	for index := range document.Entities.SaleProposals {
		item, err := normalizedConfigImportItem("sale_proposals", document.Entities.SaleProposals[index].ConfigImportAuthoritativeEntity, document.Entities.SaleProposals[index])
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	for index := range document.Entities.CostRuleDrafts {
		item, err := normalizedConfigImportItem("cost_rule_drafts", document.Entities.CostRuleDrafts[index].ConfigImportAuthoritativeEntity, document.Entities.CostRuleDrafts[index])
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	for index := range document.Entities.ModelMappings {
		item, err := normalizedConfigImportItem("model_mappings", document.Entities.ModelMappings[index].ConfigImportAuthoritativeEntity, document.Entities.ModelMappings[index])
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	for index := range document.Entities.RouteBlueprints {
		item, err := normalizedConfigImportItem("route_blueprints", document.Entities.RouteBlueprints[index].ConfigImportAuthoritativeEntity, document.Entities.RouteBlueprints[index])
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	for index := range document.Entities.Sources {
		item, err := normalizedConfigImportItem("sources", document.Entities.Sources[index].ConfigImportAuthoritativeEntity, document.Entities.Sources[index])
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	for index := range document.Entities.UnresolvedVariants {
		item, err := normalizedConfigImportItem("unresolved_variants", document.Entities.UnresolvedVariants[index].ConfigImportAuthoritativeEntity, document.Entities.UnresolvedVariants[index])
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func normalizedConfigImportItem(entityType string, entity types.ConfigImportAuthoritativeEntity, value any) (model.ConfigImportItem, error) {
	canonicalJSON, err := common.Marshal(value)
	if err != nil {
		return model.ConfigImportItem{}, fmt.Errorf("marshal %s %q: %w", entityType, entity.BusinessID, err)
	}
	return model.ConfigImportItem{
		EntityType:    entityType,
		BusinessID:    entity.BusinessID,
		EntityHash:    entity.EntityHash,
		CanonicalJSON: string(canonicalJSON),
		State:         string(types.ConfigImportItemStateNew),
		SourceRef:     entity.SourceRef,
		SourceSheet:   entity.Sheet,
		SourceRow:     entity.Row,
	}, nil
}

func configImportPersistedIssues(document *types.ConfigImportDocument) []model.ConfigImportIssue {
	entityTypes := configImportBusinessIDs(document.Entities)
	issues := make([]model.ConfigImportIssue, 0, len(document.Issues))
	for index := range document.Issues {
		sourceIssue := document.Issues[index]
		issues = append(issues, model.ConfigImportIssue{
			Severity:         string(sourceIssue.Severity),
			Code:             sourceIssue.Code,
			EntityType:       entityTypes[sourceIssue.EntityRef],
			BusinessID:       sourceIssue.EntityRef,
			Sheet:            sourceIssue.Sheet,
			Row:              sourceIssue.Row,
			Message:          sourceIssue.Message,
			Suggestion:       sourceIssue.Note,
			ResolutionStatus: "open",
		})
	}
	return issues
}

func configImportEntityCounts(entities types.ConfigImportEntities) types.ConfigImportEntityCounts {
	return types.ConfigImportEntityCounts{
		Channels:           len(entities.Channels),
		ChannelLines:       len(entities.ChannelLines),
		ModelSKUs:          len(entities.ModelSKUs),
		SaleProposals:      len(entities.SaleProposals),
		CostRuleDrafts:     len(entities.CostRuleDrafts),
		ModelMappings:      len(entities.ModelMappings),
		RouteBlueprints:    len(entities.RouteBlueprints),
		Sources:            len(entities.Sources),
		UnresolvedVariants: len(entities.UnresolvedVariants),
	}
}

func configImportBatchSummary(batch *model.ConfigImportBatch, issues []model.ConfigImportIssue) (dto.ConfigImportBatchSummary, error) {
	var stored configImportBatchSummaryStorage
	if err := common.UnmarshalJsonStr(string(batch.SummaryJSON), &stored); err != nil {
		return dto.ConfigImportBatchSummary{}, fmt.Errorf("decode config import batch %d summary: %w", batch.ID, err)
	}
	return dto.ConfigImportBatchSummary{
		ID:                batch.ID,
		SchemaVersion:     batch.SchemaVersion,
		TemplateVersion:   batch.TemplateVersion,
		SourceSHA256:      batch.SourceSHA256,
		PayloadSHA256:     batch.PayloadSHA256,
		Status:            types.ConfigImportBatchStatus(batch.Status),
		CreatedBy:         batch.CreatedBy,
		ItemCounts:        stored.ItemCounts,
		IssueCount:        stored.IssueCount,
		AllowedActions:    configImportAllowedActions(types.ConfigImportBatchStatus(batch.Status), batch.ActivatedAt, issues),
		CopiedFromBatchID: batch.CopiedFromBatchID,
		ActivatedAt:       batch.ActivatedAt,
		CreatedAt:         batch.CreatedAt,
		UpdatedAt:         batch.UpdatedAt,
	}, nil
}

func configImportIssuesHaveFailure(issues []model.ConfigImportIssue) bool {
	for index := range issues {
		if issues[index].Severity == string(types.ConfigImportIssueSeverityError) {
			return true
		}
	}
	return false
}

// configImportAllowedActions is the only place that turns lifecycle state and
// persisted issue gates into administrative commands.
func configImportAllowedActions(status types.ConfigImportBatchStatus, activatedAt *int64, issues []model.ConfigImportIssue) []string {
	if status == types.ConfigImportBatchStatusPublished {
		for _, issue := range issues {
			if (issue.Code == "CACHE_REFRESH_PENDING" || issue.Code == "ACTIVATION_CACHE_REFRESH_PENDING") && issue.ResolutionStatus == "open" {
				return []string{"refresh_cache", "copy_for_binding"}
			}
		}
		if activatedAt == nil {
			return []string{"activate", "copy_for_binding"}
		}
		return []string{"copy_for_binding"}
	}
	if status == types.ConfigImportBatchStatusBlocked ||
		status == types.ConfigImportBatchStatusPublishing || status == types.ConfigImportBatchStatusValidating {
		return []string{}
	}
	if status == types.ConfigImportBatchStatusPublishFailed {
		return []string{"validate"}
	}
	if configImportIssuesHaveFailure(issues) {
		return []string{}
	}
	switch status {
	case types.ConfigImportBatchStatusBinding:
		return []string{"bind", "resolve", "stage"}
	case types.ConfigImportBatchStatusStaged:
		return []string{"resolve", "validate"}
	case types.ConfigImportBatchStatusReady:
		return []string{"publish"}
	default:
		return []string{}
	}
}
