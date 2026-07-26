package model

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
