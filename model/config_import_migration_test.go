package model

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
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

	type schemaRow struct {
		SQL string
	}
	var rows []schemaRow
	require.NoError(t, DB.Raw(`SELECT sql FROM sqlite_master WHERE type = 'table' AND name LIKE 'config_import_%'`).Scan(&rows).Error)
	require.Len(t, rows, 6)
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

func TestConfigImportMigrationCreatesBatchChannelUniqueIndex(t *testing.T) {
	prepareConfigImportDB(t)

	type indexRow struct {
		Name   string `gorm:"column:name"`
		Unique int    `gorm:"column:unique"`
	}
	var indexes []indexRow
	require.NoError(t, DB.Raw(`PRAGMA index_list('config_import_bindings')`).Scan(&indexes).Error)
	for _, index := range indexes {
		if index.Name == "idx_config_import_binding_channel" {
			assert.Equal(t, 1, index.Unique)
			return
		}
	}
	t.Fatal("missing idx_config_import_binding_channel")
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
	require.Error(t, db.Create(&ConfigImportBinding{BatchID: batch.ID, LineRef: "line-b", Action: "bind", ChannelID: &channelID}).Error)
}
