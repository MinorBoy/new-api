package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func prepareConfigImportDB(t *testing.T) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	previousDB := DB
	previousMainType := common.MainDatabaseType()
	previousLogType := common.LogDatabaseType()
	DB = db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, previousLogType)
	t.Cleanup(func() {
		DB = previousDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
		require.NoError(t, sqlDB.Close())
	})

	require.NoError(t, db.AutoMigrate(
		&ConfigImportBatch{},
		&ConfigImportItem{},
		&ConfigImportBinding{},
		&ConfigImportIssue{},
		&ConfigImportResolution{},
		&ConfigImportPublishAudit{},
	))
}

func createConfigImportBatch(t *testing.T, payloadSHA256 string) ConfigImportBatch {
	t.Helper()
	batch := ConfigImportBatch{
		SchemaVersion:   1,
		TemplateVersion: "1",
		SourceSHA256:    "source-sha256",
		PayloadSHA256:   payloadSHA256,
		Status:          string(types.ConfigImportBatchStatusBinding),
		CreatedBy:       42,
		SummaryJSON:     `{"items":{"channels":1}}`,
		BaselineJSON:    `{"channel_versions":{}}`,
	}
	require.NoError(t, DB.Create(&batch).Error)
	return batch
}

func TestConfigImportBatchPersistsRelatedRecords(t *testing.T) {
	prepareConfigImportDB(t)
	batch := createConfigImportBatch(t, "payload-sha256-1")
	channelID := 7
	confirmedAt := int64(1_725_000_000)
	row := 21

	item := ConfigImportItem{
		BatchID:          batch.ID,
		EntityType:       "channel_line",
		BusinessID:       "CH-SECURE/video-2.0-pro",
		EntityHash:       "entity-sha256",
		CanonicalJSON:    `{"line_ref":"CH-SECURE/video-2.0-pro"}`,
		State:            string(types.ConfigImportItemStateNew),
		SourceRef:        "渠道成本!A21",
		SourceSheet:      "渠道成本",
		SourceRow:        &row,
		MaterializedType: "channel",
		MaterializedID:   &channelID,
	}
	binding := ConfigImportBinding{
		BatchID:                batch.ID,
		LineRef:                "CH-SECURE/video-2.0-pro",
		Action:                 string(types.ConfigImportBindingActionBind),
		ChannelID:              &channelID,
		CredentialsConfirmedBy: 42,
		CredentialsConfirmedAt: &confirmedAt,
	}
	issue := ConfigImportIssue{
		BatchID:          batch.ID,
		Severity:         string(types.ConfigImportIssueSeverityWarning),
		Code:             "REFERENCE_MISSING",
		EntityType:       item.EntityType,
		BusinessID:       item.BusinessID,
		Sheet:            item.SourceSheet,
		Row:              &row,
		Field:            "channel_ref",
		Message:          "channel reference is unresolved",
		Suggestion:       "bind an existing channel",
		ResolutionStatus: "open",
	}
	resolution := ConfigImportResolution{
		BatchID:        batch.ID,
		ItemBusinessID: item.BusinessID,
		Action:         string(types.ConfigImportResolutionActionUseImport),
		DecisionJSON:   `{"channel_ref":"CH-SECURE"}`,
		CreatedBy:      42,
	}
	audit := ConfigImportPublishAudit{
		BatchID:      batch.ID,
		AdminID:      42,
		BeforeSHA256: "before-sha256",
		AfterSHA256:  "after-sha256",
		Outcome:      "published",
		CreatedAt:    confirmedAt,
	}

	require.NoError(t, DB.Create(&item).Error)
	require.NoError(t, DB.Create(&binding).Error)
	require.NoError(t, DB.Create(&issue).Error)
	require.NoError(t, DB.Create(&resolution).Error)
	require.NoError(t, DB.Create(&audit).Error)

	var loadedItem ConfigImportItem
	var loadedBinding ConfigImportBinding
	var loadedIssue ConfigImportIssue
	var loadedResolution ConfigImportResolution
	var loadedAudit ConfigImportPublishAudit
	require.NoError(t, DB.Where("batch_id = ?", batch.ID).First(&loadedItem).Error)
	require.NoError(t, DB.Where("batch_id = ?", batch.ID).First(&loadedBinding).Error)
	require.NoError(t, DB.Where("batch_id = ?", batch.ID).First(&loadedIssue).Error)
	require.NoError(t, DB.Where("batch_id = ?", batch.ID).First(&loadedResolution).Error)
	require.NoError(t, DB.Where("batch_id = ?", batch.ID).First(&loadedAudit).Error)
	assert.Equal(t, item.CanonicalJSON, loadedItem.CanonicalJSON)
	assert.Equal(t, binding.LineRef, loadedBinding.LineRef)
	assert.Equal(t, issue.ResolutionStatus, loadedIssue.ResolutionStatus)
	assert.Equal(t, resolution.DecisionJSON, loadedResolution.DecisionJSON)
	assert.Equal(t, audit.AfterSHA256, loadedAudit.AfterSHA256)
}

func TestConfigImportItemBusinessIDIsUniqueWithinBatchAndEntityType(t *testing.T) {
	prepareConfigImportDB(t)
	batch := createConfigImportBatch(t, "payload-sha256-2")
	item := ConfigImportItem{
		BatchID: batch.ID, EntityType: "channel", BusinessID: "CH-SECURE",
	}
	require.NoError(t, DB.Create(&item).Error)
	require.Error(t, DB.Create(&ConfigImportItem{
		BatchID: batch.ID, EntityType: "channel", BusinessID: "CH-SECURE",
	}).Error)
	require.NoError(t, DB.Create(&ConfigImportItem{
		BatchID: batch.ID, EntityType: "channel_line", BusinessID: "CH-SECURE",
	}).Error)
}

func TestConfigImportBindingLineRefIsUniqueWithinBatch(t *testing.T) {
	prepareConfigImportDB(t)
	batch := createConfigImportBatch(t, "payload-sha256-3")
	binding := ConfigImportBinding{BatchID: batch.ID, LineRef: "CH-SECURE/video-2.0-pro"}
	require.NoError(t, DB.Create(&binding).Error)
	require.Error(t, DB.Create(&ConfigImportBinding{
		BatchID: batch.ID, LineRef: binding.LineRef,
	}).Error)
}

func TestConfigImportPayloadSHA256IsUnique(t *testing.T) {
	prepareConfigImportDB(t)
	createConfigImportBatch(t, "payload-sha256-4")
	duplicate := ConfigImportBatch{
		SchemaVersion: 1, TemplateVersion: "1", PayloadSHA256: "payload-sha256-4",
	}
	require.Error(t, DB.Create(&duplicate).Error)
}

func TestUpdateConfigImportBatchStatusUsesCompareAndSwap(t *testing.T) {
	prepareConfigImportDB(t)
	batch := createConfigImportBatch(t, "payload-sha256-5")

	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		updated, err := UpdateConfigImportBatchStatus(
			tx,
			batch.ID,
			types.ConfigImportBatchStatusBinding,
			types.ConfigImportBatchStatusStaged,
		)
		require.NoError(t, err)
		assert.True(t, updated)
		return nil
	}))

	updated, err := UpdateConfigImportBatchStatus(
		DB,
		batch.ID,
		types.ConfigImportBatchStatusBinding,
		types.ConfigImportBatchStatusStaged,
	)
	require.NoError(t, err)
	assert.False(t, updated)

	var loaded ConfigImportBatch
	require.NoError(t, DB.First(&loaded, batch.ID).Error)
	assert.Equal(t, string(types.ConfigImportBatchStatusStaged), loaded.Status)
}
