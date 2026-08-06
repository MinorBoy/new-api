package service

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/types"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func prepareConfigImportServiceDB(t *testing.T) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	previousDB := model.DB
	previousMainType := common.MainDatabaseType()
	previousLogType := common.LogDatabaseType()
	model.DB = db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, previousLogType)
	t.Cleanup(func() {
		model.DB = previousDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
		require.NoError(t, sqlDB.Close())
	})

	require.NoError(t, db.AutoMigrate(
		&model.ConfigImportBatch{},
		&model.ConfigImportItem{},
		&model.ConfigImportBinding{},
		&model.ConfigImportIssue{},
		&model.ConfigImportResolution{},
		&model.ConfigImportPublishAudit{},
		&model.ConfigImportActivationAudit{},
	))
}

func TestConfigImportUploadCreatesBindingBatchAndDiscardsPreview(t *testing.T) {
	prepareConfigImportServiceDB(t)
	payload := configImportDocumentJSON(t, map[string]any{})
	payload = strings.Replace(payload, `"derived_preview":{}`, `"derived_preview":{"must_not_persist":"preview-marker"}`, 1)

	batch, created, err := CreateConfigImportBatch(context.Background(), 42, bytes.NewReader([]byte(payload)))

	require.NoError(t, err)
	require.True(t, created)
	require.Equal(t, types.ConfigImportBatchStatusBinding, batch.Status)
	assert.Equal(t, []string{"bind", "resolve", "stage"}, batch.AllowedActions)
	require.Len(t, batch.Items, 1)
	assert.Equal(t, "sources", batch.Items[0].EntityType)
	assert.Equal(t, "source-workbook", batch.Items[0].BusinessID)
	require.Empty(t, batch.Issues)

	var persisted model.ConfigImportBatch
	require.NoError(t, model.DB.First(&persisted, batch.ID).Error)
	assert.NotContains(t, persisted.SummaryJSON, "preview-marker")
	assert.NotContains(t, persisted.BaselineJSON, "preview-marker")
	var persistedItems []model.ConfigImportItem
	require.NoError(t, model.DB.Where("batch_id = ?", batch.ID).Find(&persistedItems).Error)
	require.Len(t, persistedItems, 1)
	assert.NotContains(t, persistedItems[0].CanonicalJSON, "preview-marker")
}

func TestConfigImportUploadRequestRoundTripPreservesFixtureIntegrity(t *testing.T) {
	payload, err := os.ReadFile(filepath.Join("..", "e2e", "testdata", "channel-config-v1.json"))
	require.NoError(t, err)

	var request dto.ConfigImportUploadRequest
	require.NoError(t, common.Unmarshal([]byte(`{"document":`+string(payload)+`}`), &request))
	encoded, err := common.Marshal(request.Document)
	require.NoError(t, err)
	_, err = ParseConfigImportDocument(bytes.NewReader(encoded))
	require.NoError(t, err)
}

func TestConfigImportUploadBlocksSourceFailure(t *testing.T) {
	prepareConfigImportServiceDB(t)
	payload := configImportDocumentJSONWithIssues(t, map[string]any{}, []any{map[string]any{
		"code":       "CONVERTER_FAILURE",
		"severity":   "error",
		"message":    "source row could not be normalized",
		"entity_ref": "source-workbook",
		"sheet":      "Channels",
		"row":        4,
		"note":       "correct the source row",
	}})

	batch, created, err := CreateConfigImportBatch(context.Background(), 42, bytes.NewReader([]byte(payload)))

	require.NoError(t, err)
	require.True(t, created)
	assert.Equal(t, types.ConfigImportBatchStatusBlocked, batch.Status)
	assert.Empty(t, batch.AllowedActions)
	require.Len(t, batch.Issues, 1)
	assert.Equal(t, "sources", batch.Issues[0].EntityType)
	assert.Equal(t, "source-workbook", batch.Issues[0].BusinessID)
	assert.Equal(t, "open", batch.Issues[0].ResolutionStatus)
}

func TestConfigImportUploadRejectsGoogleCredentialInDisplayNameBeforePersistence(t *testing.T) {
	prepareConfigImportServiceDB(t)
	googleCredential := "AIza" + strings.Repeat("A", 35)
	payload := configImportDocumentJSON(t, map[string]any{
		"channels": []any{map[string]any{
			"business_id":  "channel-one",
			"entity_hash":  strings.Repeat("c", 64),
			"source_ref":   "source-workbook",
			"display_name": googleCredential,
		}},
	})

	_, created, err := CreateConfigImportBatch(context.Background(), 42, bytes.NewReader([]byte(payload)))

	requireCode(t, err, "SECURITY_CREDENTIAL_VALUE")
	assert.False(t, created)
	assertConfigImportPersistenceEmpty(t)
}

func TestConfigImportUploadRejectsGoogleCredentialInAuditNoteBeforePersistence(t *testing.T) {
	prepareConfigImportServiceDB(t)
	googleCredential := "AIza" + strings.Repeat("A", 35)
	payload := configImportDocumentJSON(t, map[string]any{
		"sources": []any{map[string]any{
			"business_id":     "source-workbook",
			"entity_hash":     strings.Repeat("b", 64),
			"source_ref":      "source-workbook",
			"sheet":           "Channels",
			"row":             4,
			"raw_business_id": "source-workbook",
			"audit_note":      googleCredential,
			"url":             "https://example.test/template.xlsx",
		}},
	})

	_, created, err := CreateConfigImportBatch(context.Background(), 42, bytes.NewReader([]byte(payload)))

	requireCode(t, err, "SECURITY_CREDENTIAL_VALUE")
	assert.False(t, created)
	assertConfigImportPersistenceEmpty(t)
}

func TestConfigImportUploadIsIdempotentByPayloadHash(t *testing.T) {
	prepareConfigImportServiceDB(t)
	payload := configImportDocumentJSON(t, map[string]any{})

	first, created, err := CreateConfigImportBatch(context.Background(), 42, bytes.NewReader([]byte(payload)))
	require.NoError(t, err)
	require.True(t, created)
	again, created, err := CreateConfigImportBatch(context.Background(), 99, bytes.NewReader([]byte(payload)))
	require.NoError(t, err)
	require.False(t, created)
	assert.Equal(t, first.ID, again.ID)
	assert.Equal(t, first.CreatedBy, again.CreatedBy)

	var batchCount int64
	require.NoError(t, model.DB.Model(&model.ConfigImportBatch{}).Count(&batchCount).Error)
	assert.Equal(t, int64(1), batchCount)
	var itemCount int64
	require.NoError(t, model.DB.Model(&model.ConfigImportItem{}).Count(&itemCount).Error)
	assert.Equal(t, int64(1), itemCount)
}

func TestConfigImportPublishedBatchCopiesToFreshBindingBatch(t *testing.T) {
	prepareConfigImportServiceDB(t)
	payload := configImportDocumentJSON(t, map[string]any{})
	source, created, err := CreateConfigImportBatch(context.Background(), 42, bytes.NewReader([]byte(payload)))
	require.NoError(t, err)
	require.True(t, created)

	publishedAt := common.GetTimestamp()
	require.NoError(t, model.DB.Model(&model.ConfigImportBatch{}).Where("id = ?", source.ID).Updates(map[string]any{
		"status": string(types.ConfigImportBatchStatusPublished), "published_at": publishedAt,
	}).Error)
	materializedID := 17
	require.NoError(t, model.DB.Model(&model.ConfigImportItem{}).Where("batch_id = ?", source.ID).Updates(map[string]any{
		"state": string(types.ConfigImportItemStateExcluded), "materialized_type": "source",
		"materialized_id": materializedID, "conflict_reason": "reviewed", "exclusion_reason": "excluded",
	}).Error)
	channelID := 9
	require.NoError(t, model.DB.Create(&model.ConfigImportBinding{
		BatchID: source.ID, LineRef: "line-a", Action: string(types.ConfigImportBindingActionBind), ChannelID: &channelID,
	}).Error)
	require.NoError(t, model.DB.Create(&model.ConfigImportIssue{
		BatchID: source.ID, Severity: string(types.ConfigImportIssueSeverityWarning), Code: "REVIEWED",
		Message: "reviewed issue", ResolutionStatus: "resolved",
	}).Error)
	require.NoError(t, model.DB.Create(&model.ConfigImportResolution{
		BatchID: source.ID, ItemBusinessID: "source-workbook", Action: string(types.ConfigImportResolutionActionExclude),
		DecisionJSON: `{}`, CreatedBy: 42,
	}).Error)

	firstCopy, err := CopyConfigImportBatchForBinding(context.Background(), 99, source.ID)
	require.NoError(t, err)
	secondCopy, err := CopyConfigImportBatchForBinding(context.Background(), 100, source.ID)
	require.NoError(t, err)

	assert.NotEqual(t, source.ID, firstCopy.ID)
	assert.NotEqual(t, firstCopy.ID, secondCopy.ID)
	assert.Equal(t, source.PayloadSHA256, firstCopy.PayloadSHA256)
	assert.Equal(t, types.ConfigImportBatchStatusBinding, firstCopy.Status)
	require.NotNil(t, firstCopy.CopiedFromBatchID)
	assert.Equal(t, source.ID, *firstCopy.CopiedFromBatchID)
	assert.Equal(t, 99, firstCopy.CreatedBy)
	assert.Equal(t, []string{"bind", "resolve", "stage"}, firstCopy.AllowedActions)
	assert.Nil(t, firstCopy.ActivatedAt)
	assert.Empty(t, firstCopy.Bindings)
	assert.Empty(t, firstCopy.Issues)
	require.Len(t, firstCopy.Items, 1)
	assert.Equal(t, types.ConfigImportItemStateNew, firstCopy.Items[0].State)
	assert.Empty(t, firstCopy.Items[0].MaterializedType)
	assert.Nil(t, firstCopy.Items[0].MaterializedID)
	assert.Empty(t, firstCopy.Items[0].ConflictReason)
	assert.Empty(t, firstCopy.Items[0].ExclusionReason)

	var persistedSource, persistedCopy model.ConfigImportBatch
	require.NoError(t, model.DB.First(&persistedSource, source.ID).Error)
	require.NoError(t, model.DB.First(&persistedCopy, firstCopy.ID).Error)
	require.NotNil(t, persistedSource.DeduplicationKey)
	require.NotNil(t, persistedCopy.DeduplicationKey)
	assert.Equal(t, "upload:"+source.PayloadSHA256, *persistedSource.DeduplicationKey)
	assert.NotEqual(t, *persistedSource.DeduplicationKey, *persistedCopy.DeduplicationKey)
	assert.True(t, strings.HasPrefix(*persistedCopy.DeduplicationKey, "copy:"))

	for _, table := range []any{&model.ConfigImportBinding{}, &model.ConfigImportIssue{}, &model.ConfigImportResolution{}} {
		var count int64
		require.NoError(t, model.DB.Model(table).Where("batch_id = ?", firstCopy.ID).Count(&count).Error)
		assert.Zero(t, count)
	}
}

func TestConfigImportCopyForBindingRejectsUnpublishedSource(t *testing.T) {
	prepareConfigImportServiceDB(t)
	payload := configImportDocumentJSON(t, map[string]any{})
	source, created, err := CreateConfigImportBatch(context.Background(), 42, bytes.NewReader([]byte(payload)))
	require.NoError(t, err)
	require.True(t, created)

	_, err = CopyConfigImportBatchForBinding(context.Background(), 99, source.ID)

	requireCode(t, err, "COPY_FOR_BINDING_SOURCE_STATUS")
	var count int64
	require.NoError(t, model.DB.Model(&model.ConfigImportBatch{}).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

func TestConfigImportUploadCreatesNewBatchForChangedPayload(t *testing.T) {
	prepareConfigImportServiceDB(t)
	firstPayload := configImportDocumentJSON(t, map[string]any{})
	secondPayload := configImportDocumentJSON(t, map[string]any{
		"sources": []any{map[string]any{
			"business_id":     "source-workbook",
			"entity_hash":     strings.Repeat("b", 64),
			"source_ref":      "source-workbook",
			"sheet":           "Channels",
			"row":             4,
			"raw_business_id": "source-workbook",
			"audit_note":      "changed source note",
			"url":             "https://example.test/changed-template.xlsx",
		}},
	})

	first, created, err := CreateConfigImportBatch(context.Background(), 42, bytes.NewReader([]byte(firstPayload)))
	require.NoError(t, err)
	require.True(t, created)
	second, created, err := CreateConfigImportBatch(context.Background(), 42, bytes.NewReader([]byte(secondPayload)))
	require.NoError(t, err)
	require.True(t, created)
	assert.NotEqual(t, first.ID, second.ID)
	assert.NotEqual(t, first.PayloadSHA256, second.PayloadSHA256)
}

func TestConfigImportListPaginatesNewestFirst(t *testing.T) {
	prepareConfigImportServiceDB(t)
	for _, sourceURL := range []string{
		"https://example.test/one.xlsx",
		"https://example.test/two.xlsx",
		"https://example.test/three.xlsx",
	} {
		payload := configImportDocumentJSON(t, map[string]any{
			"sources": []any{map[string]any{
				"business_id":     "source-workbook",
				"entity_hash":     strings.Repeat("b", 64),
				"source_ref":      "source-workbook",
				"sheet":           "Channels",
				"row":             4,
				"raw_business_id": "source-workbook",
				"audit_note":      sourceURL,
				"url":             sourceURL,
			}},
		})
		_, created, err := CreateConfigImportBatch(context.Background(), 42, bytes.NewReader([]byte(payload)))
		require.NoError(t, err)
		require.True(t, created)
	}

	firstPage, err := ListConfigImportBatches(context.Background(), 1, 2)
	require.NoError(t, err)
	assert.Equal(t, int64(3), firstPage.Total)
	require.Len(t, firstPage.Items, 2)
	assert.Greater(t, firstPage.Items[0].ID, firstPage.Items[1].ID)
	secondPage, err := ListConfigImportBatches(context.Background(), 2, 2)
	require.NoError(t, err)
	require.Len(t, secondPage.Items, 1)
	assert.Less(t, secondPage.Items[0].ID, firstPage.Items[1].ID)
}

func TestConfigImportListAppliesPersistedIssueGate(t *testing.T) {
	prepareConfigImportServiceDB(t)
	payload := configImportDocumentJSON(t, map[string]any{})
	batch, created, err := CreateConfigImportBatch(context.Background(), 42, bytes.NewReader([]byte(payload)))
	require.NoError(t, err)
	require.True(t, created)
	require.NoError(t, model.DB.Model(&model.ConfigImportBatch{}).Where("id = ?", batch.ID).
		Update("status", string(types.ConfigImportBatchStatusReady)).Error)
	require.NoError(t, model.DB.Create(&model.ConfigImportIssue{
		BatchID:          batch.ID,
		Severity:         string(types.ConfigImportIssueSeverityError),
		Code:             "SERVER_FAILURE",
		Message:          "server validation failed",
		ResolutionStatus: "open",
	}).Error)

	page, err := ListConfigImportBatches(context.Background(), 1, 10)

	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	assert.Empty(t, page.Items[0].AllowedActions)
}

func TestConfigImportDetailRecoversNormalizedItemsAndIssues(t *testing.T) {
	prepareConfigImportServiceDB(t)
	payload := configImportDocumentJSONWithIssues(t, map[string]any{}, []any{map[string]any{
		"code":       "CONVERTER_WARNING",
		"severity":   "warning",
		"message":    "margin requires review",
		"entity_ref": "source-workbook",
		"sheet":      "Channels",
		"row":        4,
	}})
	created, inserted, err := CreateConfigImportBatch(context.Background(), 42, bytes.NewReader([]byte(payload)))
	require.NoError(t, err)
	require.True(t, inserted)

	detail, err := GetConfigImportBatch(context.Background(), created.ID)

	require.NoError(t, err)
	assert.Equal(t, created.ID, detail.ID)
	require.Len(t, detail.Items, 1)
	require.Len(t, detail.Issues, 1)
	assert.Equal(t, "CONVERTER_WARNING", detail.Issues[0].Code)
	assert.NotContains(t, detail.Items[0].CanonicalJSON, "derived_preview")
}

func TestConfigImportAllowedActionsAreStatusBounded(t *testing.T) {
	activatedAt := int64(1)
	cacheIssue := []model.ConfigImportIssue{{Code: "CACHE_REFRESH_PENDING", ResolutionStatus: "open"}}
	activationCacheIssue := []model.ConfigImportIssue{{Code: "ACTIVATION_CACHE_REFRESH_PENDING", ResolutionStatus: "open"}}
	assert.Empty(t, configImportAllowedActions(types.ConfigImportBatchStatusValidating, nil, nil))
	assert.Empty(t, configImportAllowedActions(types.ConfigImportBatchStatusBlocked, nil, nil))
	assert.Equal(t, []string{"bind", "resolve", "stage"}, configImportAllowedActions(types.ConfigImportBatchStatusBinding, nil, nil))
	assert.Equal(t, []string{"resolve", "validate"}, configImportAllowedActions(types.ConfigImportBatchStatusStaged, nil, nil))
	assert.Equal(t, []string{"publish"}, configImportAllowedActions(types.ConfigImportBatchStatusReady, nil, nil))
	assert.Empty(t, configImportAllowedActions(types.ConfigImportBatchStatusPublishing, nil, nil))
	assert.Equal(t, []string{"activate", "copy_for_binding"}, configImportAllowedActions(types.ConfigImportBatchStatusPublished, nil, nil))
	assert.Equal(t, []string{"refresh_cache", "copy_for_binding"}, configImportAllowedActions(types.ConfigImportBatchStatusPublished, nil, cacheIssue))
	assert.Equal(t, []string{"copy_for_binding"}, configImportAllowedActions(types.ConfigImportBatchStatusPublished, &activatedAt, nil))
	assert.Equal(t, []string{"refresh_cache", "copy_for_binding"}, configImportAllowedActions(types.ConfigImportBatchStatusPublished, &activatedAt, activationCacheIssue))
	assert.Equal(t, []string{"validate"}, configImportAllowedActions(types.ConfigImportBatchStatusPublishFailed, nil, nil))
}

func TestGetConfigImportBatchIncludesActivationPreviewUntilActivated(t *testing.T) {
	fixture := createActivationFixture(t)

	detail, err := GetConfigImportBatch(context.Background(), fixture.BatchID)

	require.NoError(t, err)
	require.NotNil(t, detail.ActivationPreview)
	assert.True(t, detail.ActivationPreview.Ready)
	assert.Equal(t, []string{"activate", "copy_for_binding"}, detail.AllowedActions)
	assert.Nil(t, detail.ActivatedAt)

	activatedAt := common.GetTimestamp()
	require.NoError(t, model.DB.Model(&model.ConfigImportBatch{}).Where("id = ?", fixture.BatchID).Update("activated_at", activatedAt).Error)
	detail, err = GetConfigImportBatch(context.Background(), fixture.BatchID)

	require.NoError(t, err)
	assert.Nil(t, detail.ActivationPreview)
	require.NotNil(t, detail.ActivatedAt)
	assert.Equal(t, activatedAt, *detail.ActivatedAt)
	assert.Equal(t, []string{"copy_for_binding"}, detail.AllowedActions)
}

func assertConfigImportPersistenceEmpty(t *testing.T) {
	t.Helper()
	for _, table := range []any{
		&model.ConfigImportBatch{},
		&model.ConfigImportItem{},
		&model.ConfigImportIssue{},
	} {
		var count int64
		require.NoError(t, model.DB.Model(table).Count(&count).Error)
		assert.Zero(t, count)
	}
}
