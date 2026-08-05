package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type routeOwnershipBackfillFixture struct {
	UniqueTargetID    int
	TimestampTargetID int
	AmbiguousTargetID int
	UnmatchedTargetID int
}

func TestPreviewConfigImportRouteOwnershipBackfillClassifiesWithoutWrites(t *testing.T) {
	prepareConfigImportRouteOwnershipTest(t)
	fixture := seedRouteOwnershipBackfillFixture(t)

	report, err := PreviewConfigImportRouteOwnershipBackfill(context.Background())
	require.NoError(t, err)
	require.Len(t, report.Matched, 2)
	assert.Equal(t, fixture.UniqueTargetID, report.Matched[0].RouteTargetID)
	assert.Equal(t, fixture.TimestampTargetID, report.Matched[1].RouteTargetID)
	require.Len(t, report.Ambiguous, 1)
	assert.Equal(t, fixture.AmbiguousTargetID, report.Ambiguous[0].RouteTargetID)
	require.Len(t, report.Unmatched, 1)
	assert.Equal(t, fixture.UnmatchedTargetID, report.Unmatched[0].RouteTargetID)

	var changed int64
	require.NoError(t, model.DB.Model(&model.RouteTarget{}).
		Where("managed_by = ?", types.RouteTargetManagedByConfigImport).Count(&changed).Error)
	assert.Zero(t, changed)
}

func TestApplyConfigImportRouteOwnershipBackfillIsIdempotent(t *testing.T) {
	prepareConfigImportRouteOwnershipTest(t)
	fixture := seedRouteOwnershipBackfillFixture(t)

	first, err := ApplyConfigImportRouteOwnershipBackfill(context.Background(), 42)
	require.NoError(t, err)
	assert.NotEmpty(t, first.OperationID)
	assert.Equal(t, 2, first.AppliedCount)

	for _, targetID := range []int{fixture.UniqueTargetID, fixture.TimestampTargetID} {
		var target model.RouteTarget
		require.NoError(t, model.DB.First(&target, targetID).Error)
		assert.Equal(t, string(types.RouteTargetManagedByConfigImport), target.ManagedBy)
		assert.NotNil(t, target.SourceBatchID)
	}
	var changeCount int64
	require.NoError(t, model.DB.Model(&model.ConfigImportRouteOwnershipChange{}).Count(&changeCount).Error)
	assert.Equal(t, int64(2), changeCount)

	second, err := ApplyConfigImportRouteOwnershipBackfill(context.Background(), 42)
	require.NoError(t, err)
	assert.Empty(t, second.OperationID)
	assert.Zero(t, second.AppliedCount)
	require.NoError(t, model.DB.Model(&model.ConfigImportRouteOwnershipChange{}).Count(&changeCount).Error)
	assert.Equal(t, int64(2), changeCount)
}

func TestRollbackConfigImportRouteOwnershipBackfillRestoresAllTargets(t *testing.T) {
	prepareConfigImportRouteOwnershipTest(t)
	fixture := seedRouteOwnershipBackfillFixture(t)
	applied, err := ApplyConfigImportRouteOwnershipBackfill(context.Background(), 42)
	require.NoError(t, err)

	reverted, err := RollbackConfigImportRouteOwnershipBackfill(context.Background(), 77, applied.OperationID)
	require.NoError(t, err)
	assert.Equal(t, applied.OperationID, reverted.OperationID)
	assert.Equal(t, 2, reverted.RevertedCount)
	for _, targetID := range []int{fixture.UniqueTargetID, fixture.TimestampTargetID} {
		var target model.RouteTarget
		require.NoError(t, model.DB.First(&target, targetID).Error)
		assert.Equal(t, string(types.RouteTargetManagedByManual), target.ManagedBy)
		assert.Nil(t, target.SourceBatchID)
	}

	_, err = RollbackConfigImportRouteOwnershipBackfill(context.Background(), 77, applied.OperationID)
	requireCode(t, err, "ROUTE_OWNERSHIP_ALREADY_REVERTED")
	_, err = RollbackConfigImportRouteOwnershipBackfill(context.Background(), 77, "missing-operation")
	requireCode(t, err, "ROUTE_OWNERSHIP_ROLLBACK_EMPTY")
}

func TestRollbackConfigImportRouteOwnershipBackfillRejectsUpdatedAtConflictWithoutPartialWrites(t *testing.T) {
	prepareConfigImportRouteOwnershipTest(t)
	fixture := seedRouteOwnershipBackfillFixture(t)
	applied, err := ApplyConfigImportRouteOwnershipBackfill(context.Background(), 42)
	require.NoError(t, err)

	var change model.ConfigImportRouteOwnershipChange
	require.NoError(t, model.DB.Where("operation_id = ? AND route_target_id = ?", applied.OperationID, fixture.TimestampTargetID).First(&change).Error)
	require.NoError(t, model.DB.Model(&model.RouteTarget{}).Where("id = ?", fixture.TimestampTargetID).
		UpdateColumn("updated_at", change.AppliedTargetUpdatedAt+1).Error)

	_, err = RollbackConfigImportRouteOwnershipBackfill(context.Background(), 77, applied.OperationID)
	requireCode(t, err, "ROUTE_OWNERSHIP_ROLLBACK_CONFLICT")
	assertOwnershipOperationNotReverted(t, applied.OperationID, fixture.UniqueTargetID, fixture.TimestampTargetID)
}

func TestRollbackConfigImportRouteOwnershipBackfillRejectsFingerprintConflictWithoutPartialWrites(t *testing.T) {
	prepareConfigImportRouteOwnershipTest(t)
	fixture := seedRouteOwnershipBackfillFixture(t)
	applied, err := ApplyConfigImportRouteOwnershipBackfill(context.Background(), 42)
	require.NoError(t, err)

	var change model.ConfigImportRouteOwnershipChange
	require.NoError(t, model.DB.Where("operation_id = ? AND route_target_id = ?", applied.OperationID, fixture.TimestampTargetID).First(&change).Error)
	require.NoError(t, model.DB.Model(&model.RouteTarget{}).Where("id = ?", fixture.TimestampTargetID).
		UpdateColumns(map[string]any{"enabled": false, "updated_at": change.AppliedTargetUpdatedAt}).Error)

	_, err = RollbackConfigImportRouteOwnershipBackfill(context.Background(), 77, applied.OperationID)
	requireCode(t, err, "ROUTE_OWNERSHIP_ROLLBACK_CONFLICT")
	assertOwnershipOperationNotReverted(t, applied.OperationID, fixture.UniqueTargetID, fixture.TimestampTargetID)
}

func prepareConfigImportRouteOwnershipTest(t *testing.T) {
	t.Helper()
	prepareConfigImportServiceDB(t)
	require.NoError(t, model.DB.AutoMigrate(
		&model.RoutingPolicy{},
		&model.RouteTarget{},
		&model.ConfigImportRouteOwnershipChange{},
	))
}

func assertOwnershipOperationNotReverted(t *testing.T, operationID string, targetIDs ...int) {
	t.Helper()
	for _, targetID := range targetIDs {
		var target model.RouteTarget
		require.NoError(t, model.DB.First(&target, targetID).Error)
		assert.Equal(t, string(types.RouteTargetManagedByConfigImport), target.ManagedBy)
	}
	var revertedCount int64
	require.NoError(t, model.DB.Model(&model.ConfigImportRouteOwnershipChange{}).
		Where("operation_id = ? AND reverted_at IS NOT NULL", operationID).Count(&revertedCount).Error)
	assert.Zero(t, revertedCount)
}

func seedRouteOwnershipBackfillFixture(t *testing.T) routeOwnershipBackfillFixture {
	t.Helper()
	const channelID = 41
	const canonicalModel = "canonical-video"
	const upstreamModel = "upstream-video"
	priority := 100
	enabled := false
	zero := 0
	referenceBounds := &types.ConfigImportReferenceBounds{Images: &zero, Videos: &zero, Audios: &zero}

	policy := model.RoutingPolicy{
		GroupName: "default", Model: canonicalModel, Enabled: true,
		DefaultResolution: "720p", DefaultDuration: 4, DefaultRatio: "adaptive",
	}
	require.NoError(t, model.DB.Create(&policy).Error)

	batches := []struct {
		name        string
		publishedAt int64
		targetRefs  []string
	}{
		{name: "batch-1", publishedAt: 100, targetRefs: []string{"unique-target", "timestamp-target", "ambiguous-target"}},
		{name: "batch-2", publishedAt: 200, targetRefs: []string{"timestamp-target", "ambiguous-target"}},
		{name: "batch-3", publishedAt: 300, targetRefs: []string{"ambiguous-target"}},
	}
	for index, input := range batches {
		publishedAt := input.publishedAt
		batch := model.ConfigImportBatch{
			SchemaVersion: 1, TemplateVersion: "1",
			SourceSHA256: fmt.Sprintf("source-%d", index), PayloadSHA256: fmt.Sprintf("payload-%d", index),
			Status: string(types.ConfigImportBatchStatusPublished), CreatedBy: 42, PublishedAt: &publishedAt,
		}
		require.NoError(t, model.DB.Create(&batch).Error)
		lineRef := fmt.Sprintf("line-%d", index)
		boundChannelID := channelID
		confirmedAt := int64(90 + index)
		require.NoError(t, model.DB.Create(&model.ConfigImportBinding{
			BatchID: batch.ID, LineRef: lineRef, Action: string(types.ConfigImportBindingActionBind), ChannelID: &boundChannelID,
			CredentialsConfirmedBy: 42, CredentialsConfirmedAt: &confirmedAt,
		}).Error)
		targets := make([]types.ConfigImportRouteTarget, 0, len(input.targetRefs))
		for _, targetRef := range input.targetRefs {
			targets = append(targets, types.ConfigImportRouteTarget{
				RouteTargetRef: targetRef, LineRef: lineRef, UpstreamModel: upstreamModel,
				SKURef: "sku-video", CostVariantKey: "default", Priority: &priority, Enabled: &enabled,
				ReferenceMinimums: referenceBounds, ReferenceLimits: referenceBounds,
			})
		}
		blueprint := types.ConfigImportRouteBlueprint{
			ConfigImportAuthoritativeEntity: types.ConfigImportAuthoritativeEntity{BusinessID: fmt.Sprintf("route-%d", index)},
			CanonicalModel:                  canonicalModel, ClientModel: canonicalModel,
			MergeMode: types.ConfigImportRouteMergeModeMerge, Targets: targets,
		}
		encoded, err := common.Marshal(blueprint)
		require.NoError(t, err)
		require.NoError(t, model.DB.Create(&model.ConfigImportItem{
			BatchID: batch.ID, EntityType: "route_blueprints", BusinessID: blueprint.BusinessID,
			EntityHash: fmt.Sprintf("hash-%d", index), CanonicalJSON: string(encoded), State: string(types.ConfigImportItemStateNew),
		}).Error)
	}

	blueprint := types.ConfigImportRouteBlueprint{
		CanonicalModel: canonicalModel, ClientModel: canonicalModel, MergeMode: types.ConfigImportRouteMergeModeMerge,
		Targets: []types.ConfigImportRouteTarget{{
			RouteTargetRef: "unique-target", LineRef: "fixture-line", UpstreamModel: upstreamModel,
			SKURef: "sku-video", CostVariantKey: "default", Priority: &priority, Enabled: &enabled,
			ReferenceMinimums: referenceBounds, ReferenceLimits: referenceBounds,
		}},
	}
	_, rows, err := configImportRouteRows(map[string]int{"fixture-line": channelID}, blueprint)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	template := rows[0]

	created := make([]model.RouteTarget, 0, 4)
	for _, input := range []struct {
		name      string
		createdAt int64
	}{
		{name: "unique-target", createdAt: 900},
		{name: "timestamp-target", createdAt: 200},
		{name: "ambiguous-target", createdAt: 900},
		{name: "unmatched-target", createdAt: 900},
	} {
		target := template
		target.ID = 0
		target.PolicyID = policy.ID
		target.Name = input.name
		target.Enabled = true
		target.ManagedBy = string(types.RouteTargetManagedByManual)
		target.CreatedAt = input.createdAt
		target.UpdatedAt = input.createdAt
		if input.name == "unique-target" {
			var reordered map[string]any
			require.NoError(t, common.UnmarshalJsonStr(target.Constraints, &reordered))
			encoded, err := common.Marshal(reordered)
			require.NoError(t, err)
			target.Constraints = string(encoded)
			assert.NotEqual(t, template.Constraints, target.Constraints)
		}
		require.NoError(t, model.DB.Create(&target).Error)
		created = append(created, target)
	}
	return routeOwnershipBackfillFixture{
		UniqueTargetID: created[0].ID, TimestampTargetID: created[1].ID,
		AmbiguousTargetID: created[2].ID, UnmatchedTargetID: created[3].ID,
	}
}
