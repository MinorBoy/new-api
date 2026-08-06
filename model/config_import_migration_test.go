package model

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

func TestConfigImportMigrationUsesTextForCanonicalJSON(t *testing.T) {
	prepareConfigImportDB(t)
	require.NoError(t, DB.AutoMigrate(&ConfigImportActivationAudit{}, &ConfigImportRouteOwnershipChange{}))

	type schemaRow struct {
		SQL string
	}
	var rows []schemaRow
	require.NoError(t, DB.Raw(`SELECT sql FROM sqlite_master WHERE type = 'table' AND name LIKE 'config_import_%'`).Scan(&rows).Error)
	require.Len(t, rows, 8)
	for _, row := range rows {
		sql := strings.ToUpper(row.SQL)
		assert.NotContains(t, sql, " JSON")
		assert.NotContains(t, sql, "ON DELETE CASCADE")
		assert.NotContains(t, sql, " WHERE ")
	}

	var itemDDL schemaRow
	require.NoError(t, DB.Raw(`SELECT sql FROM sqlite_master WHERE type = 'table' AND name = 'config_import_items'`).Scan(&itemDDL).Error)
	assert.Contains(t, strings.ToUpper(itemDDL.SQL), "`CANONICAL_JSON` TEXT")
}

func TestRouteTargetOwnershipMigrationBackfillsManualWithoutChangingRuntimeState(t *testing.T) {
	prepareConfigImportDB(t)
	require.NoError(t, DB.Migrator().DropTable(&RouteTarget{}))
	require.NoError(t, DB.Exec(`CREATE TABLE route_targets (
		id integer primary key,
		policy_id integer not null,
		channel_id integer not null,
		name varchar(128) not null,
		upstream_model varchar(255) not null,
		cost_variant_key varchar(64) not null,
		target_priority integer not null,
		minimum_expected_margin_bps integer,
		"constraints" text not null,
		enabled numeric not null,
		created_at bigint,
		updated_at bigint
	)`).Error)
	require.NoError(t, DB.Exec(`INSERT INTO route_targets
		(id, policy_id, channel_id, name, upstream_model, cost_variant_key, target_priority, constraints, enabled, created_at, updated_at)
		VALUES (1, 2, 3, 'legacy', 'upstream', 'default', 10, '{}', 1, 11, 12)`).Error)

	require.NoError(t, migrateRouteTargetOwnershipColumns())
	require.NoError(t, DB.AutoMigrate(&RouteTarget{}, &ConfigImportActivationAudit{}, &ConfigImportRouteOwnershipChange{}))

	var target RouteTarget
	require.NoError(t, DB.First(&target, 1).Error)
	assert.Equal(t, string(types.RouteTargetManagedByManual), target.ManagedBy)
	assert.Nil(t, target.SourceBatchID)
	assert.Nil(t, target.RetiredAt)
	assert.True(t, target.Enabled)

	require.NoError(t, migrateRouteTargetOwnershipColumns())
}

func TestConfigImportMigrationAllowsOneChannelAcrossMultipleLines(t *testing.T) {
	prepareConfigImportDB(t)

	type indexRow struct {
		Name   string `gorm:"column:name"`
		Unique int    `gorm:"column:unique"`
	}
	var indexes []indexRow
	require.NoError(t, DB.Raw(`PRAGMA index_list('config_import_bindings')`).Scan(&indexes).Error)
	for _, index := range indexes {
		if index.Name == "idx_config_import_binding_channel" {
			t.Fatalf("legacy unique index still exists: %+v", index)
		}
	}
}

func TestConfigImportMigrationDropsLegacyBatchChannelUniqueIndex(t *testing.T) {
	prepareConfigImportDB(t)
	require.NoError(t, DB.Exec("CREATE UNIQUE INDEX idx_config_import_binding_channel ON config_import_bindings(batch_id, channel_id)").Error)
	require.True(t, DB.Migrator().HasIndex(&ConfigImportBinding{}, "idx_config_import_binding_channel"))

	require.NoError(t, migrateConfigImportBindingChannelIndex())
	assert.False(t, DB.Migrator().HasIndex(&ConfigImportBinding{}, "idx_config_import_binding_channel"))
	require.NoError(t, migrateConfigImportBindingChannelIndex())
}

func TestConfigImportBatchIdentityMigrationPreservesPayloadHashAndAllowsCopies(t *testing.T) {
	prepareConfigImportDB(t)
	require.NoError(t, DB.Migrator().DropIndex(&ConfigImportBatch{}, "idx_config_import_batches_payload_sha256"))
	require.NoError(t, DB.Exec(`CREATE UNIQUE INDEX idx_config_import_batches_payload_sha256 ON config_import_batches(payload_sha256)`).Error)
	source := ConfigImportBatch{SchemaVersion: 1, TemplateVersion: "v1", SourceSHA256: "source", PayloadSHA256: "payload", Status: "published", CreatedBy: 42, SummaryJSON: "{}", BaselineJSON: "{}"}
	require.NoError(t, DB.Create(&source).Error)

	require.NoError(t, migrateConfigImportBatchIdentity())
	require.NoError(t, DB.AutoMigrate(&ConfigImportBatch{}))

	var loadedSource ConfigImportBatch
	require.NoError(t, DB.First(&loadedSource, source.ID).Error)
	require.NotNil(t, loadedSource.DeduplicationKey)
	assert.Equal(t, "upload:payload", *loadedSource.DeduplicationKey)
	assert.Nil(t, loadedSource.CopiedFromBatchID)

	copyKey := "copy:operation-1"
	copySourceID := loadedSource.ID
	copyBatch := ConfigImportBatch{
		SchemaVersion: loadedSource.SchemaVersion, TemplateVersion: loadedSource.TemplateVersion,
		SourceSHA256: loadedSource.SourceSHA256, PayloadSHA256: loadedSource.PayloadSHA256,
		DeduplicationKey: &copyKey, CopiedFromBatchID: &copySourceID,
		Status: "binding", CreatedBy: 99, SummaryJSON: "{}", BaselineJSON: "{}",
	}
	require.NoError(t, DB.Create(&copyBatch).Error)
	assert.NotEqual(t, loadedSource.ID, copyBatch.ID)
	assert.Equal(t, loadedSource.PayloadSHA256, copyBatch.PayloadSHA256)

	require.NoError(t, migrateConfigImportBatchIdentity())
}

func TestConfigImportMigrationConfiguredDatabases(t *testing.T) {
	tests := []struct {
		name      string
		env       string
		dialect   common.DatabaseType
		dialector func(string) gorm.Dialector
	}{
		{
			name: "mysql", env: "TEST_MYSQL_DSN", dialect: common.DatabaseTypeMySQL,
			dialector: func(dsn string) gorm.Dialector { return mysql.Open(dsn) },
		},
		{
			name: "postgres", env: "TEST_POSTGRES_DSN", dialect: common.DatabaseTypePostgreSQL,
			dialector: func(dsn string) gorm.Dialector {
				return postgres.New(postgres.Config{DSN: dsn, PreferSimpleProtocol: true})
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dsn := strings.TrimSpace(os.Getenv(test.env))
			if dsn == "" {
				t.Skip(test.env + " is not configured")
			}
			db, err := gorm.Open(test.dialector(dsn), configImportMigrationGORMConfig())
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
			testConfigImportMigrationContracts(t, db, test.dialect)
		})
	}
}

func configImportMigrationGORMConfig() *gorm.Config {
	prefix := fmt.Sprintf("cim_%x_", time.Now().UnixNano())
	return &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
		NamingStrategy: schema.NamingStrategy{
			TablePrefix: prefix,
		},
	}
}

func testConfigImportMigrationContracts(t *testing.T, db *gorm.DB, _ common.DatabaseType) {
	t.Helper()
	t.Cleanup(func() {
		require.NoError(t, db.Migrator().DropTable(
			&ConfigImportRouteOwnershipChange{},
			&ConfigImportActivationAudit{},
			&ConfigImportPublishAudit{},
			&ConfigImportResolution{},
			&ConfigImportIssue{},
			&ConfigImportBinding{},
			&ConfigImportItem{},
			&ConfigImportBatch{},
		))
	})
	require.NoError(t, db.AutoMigrate(
		&ConfigImportBatch{},
		&ConfigImportItem{},
		&ConfigImportBinding{},
		&ConfigImportIssue{},
		&ConfigImportResolution{},
		&ConfigImportPublishAudit{},
		&ConfigImportActivationAudit{},
		&ConfigImportRouteOwnershipChange{},
	))
	require.NoError(t, db.AutoMigrate(
		&ConfigImportBatch{},
		&ConfigImportItem{},
		&ConfigImportBinding{},
		&ConfigImportIssue{},
		&ConfigImportResolution{},
		&ConfigImportPublishAudit{},
		&ConfigImportActivationAudit{},
		&ConfigImportRouteOwnershipChange{},
	))

	batch := ConfigImportBatch{SchemaVersion: 1, TemplateVersion: "1", SourceSHA256: "source", PayloadSHA256: "payload", Status: "binding", CreatedBy: 1}
	require.NoError(t, db.Create(&batch).Error)
	item := ConfigImportItem{BatchID: batch.ID, EntityType: "sources", BusinessID: "source-a", EntityHash: "hash-a", CanonicalJSON: `{"url":"https://example.com"}`, State: "new"}
	require.NoError(t, db.Create(&item).Error)
	var stored ConfigImportItem
	require.NoError(t, db.Where("id = ?", item.ID).First(&stored).Error)
	assert.Equal(t, item.CanonicalJSON, stored.CanonicalJSON)

	channelID := 99
	require.NoError(t, db.Create(&ConfigImportBinding{BatchID: batch.ID, LineRef: "line-a", Action: "bind", ChannelID: &channelID}).Error)
	require.NoError(t, db.Create(&ConfigImportBinding{BatchID: batch.ID, LineRef: "line-b", Action: "bind", ChannelID: &channelID}).Error)

	activation := ConfigImportActivationAudit{
		BatchID:      batch.ID,
		AdminID:      1,
		Outcome:      "activated",
		BeforeSHA256: "before",
		AfterSHA256:  "after",
		SummaryJSON:  `{"targets":1}`,
	}
	require.NoError(t, db.Create(&activation).Error)
	var storedActivation ConfigImportActivationAudit
	require.NoError(t, db.First(&storedActivation, activation.ID).Error)
	assert.Equal(t, activation.SummaryJSON, storedActivation.SummaryJSON)
	assert.NotEmpty(t, storedActivation.BeforeSHA256)
	assert.NotEmpty(t, storedActivation.AfterSHA256)

	change := ConfigImportRouteOwnershipChange{
		OperationID:            "operation-1",
		RouteTargetID:          101,
		PreviousManagedBy:      string(types.RouteTargetManagedByManual),
		AssignedBatchID:        batch.ID,
		AppliedTargetUpdatedAt: 123,
		AppliedTargetSHA256:    "target-sha256",
		AppliedBy:              1,
	}
	require.NoError(t, db.Create(&change).Error)
	duplicate := change
	duplicate.ID = 0
	assert.Error(t, db.Create(&duplicate).Error)
}
