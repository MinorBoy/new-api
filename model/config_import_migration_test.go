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
