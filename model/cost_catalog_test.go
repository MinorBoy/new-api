package model

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

func TestListCostCatalogRowsFiltersSortsAndKeepsMissingChannels(t *testing.T) {
	prepareCostCatalogQueryDB(t)
	seedCostCatalogQueryRows(t)
	assertCostCatalogQueryContract(t)
}

func assertCostCatalogQueryContract(t *testing.T) {
	t.Helper()
	rows, total, err := ListCostCatalogRows(CostCatalogQuery{
		BillableUpstreamModel: "100%_literal",
		Status:                string(types.CostRuleActive),
		SortBy:                "channel_name",
		SortOrder:             "asc",
		Offset:                0,
		Limit:                 25,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	require.Len(t, rows, 2)
	assert.Equal(t, "Alpha", rows[0].ChannelName)
	assert.False(t, rows[0].ChannelMissing)
	assert.Equal(t, "", rows[1].ChannelName)
	assert.True(t, rows[1].ChannelMissing)

	page, pageTotal, err := ListCostCatalogRows(CostCatalogQuery{
		Status: "all", SortBy: "version", SortOrder: "desc", Offset: 1, Limit: 1,
	})
	require.NoError(t, err)
	assert.Greater(t, pageTotal, int64(1))
	require.Len(t, page, 1)
}

func TestCostCatalogQueryConfiguredDatabases(t *testing.T) {
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
			db, err := gorm.Open(test.dialector(dsn), costCatalogQueryGORMConfig())
			require.NoError(t, err)
			prepareCostCatalogContractDB(t, db, test.dialect)
			seedCostCatalogQueryRows(t)
			assertCostCatalogQueryContract(t)
		})
	}
}

func TestListCostCatalogRowsFiltersChannelModeAndSource(t *testing.T) {
	prepareCostCatalogQueryDB(t)
	seedCostCatalogQueryRows(t)

	rows, total, err := ListCostCatalogRows(CostCatalogQuery{
		ChannelID:             20,
		BillableUpstreamModel: "CASE-MODEL",
		CostMode:              string(types.CostModePerRequest),
		Status:                string(types.CostRuleActive),
		Source:                "config_import",
		SortBy:                "billable_upstream_model",
		SortOrder:             "asc",
		Limit:                 25,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, rows, 1)
	assert.Equal(t, "vendor-case-model", rows[0].BillableUpstreamModel)
	assert.Equal(t, "Beta", rows[0].ChannelName)
}

func TestWalkCostCatalogRowsVisitsStableBatches(t *testing.T) {
	prepareCostCatalogQueryDB(t)
	seedCostCatalogQueryRows(t)

	query := CostCatalogQuery{Status: "all", SortBy: "version", SortOrder: "desc"}
	want, total, err := ListCostCatalogRows(CostCatalogQuery{
		Status: query.Status, SortBy: query.SortBy, SortOrder: query.SortOrder, Limit: 100,
	})
	require.NoError(t, err)
	require.Equal(t, int64(len(want)), total)

	visited := make([]int64, 0, len(want))
	batchSizes := make([]int, 0)
	err = WalkCostCatalogRows(query, 2, func(rows []CostCatalogRow) error {
		batchSizes = append(batchSizes, len(rows))
		for _, row := range rows {
			visited = append(visited, row.ID)
		}
		return nil
	})
	require.NoError(t, err)

	wantIDs := make([]int64, 0, len(want))
	for _, row := range want {
		wantIDs = append(wantIDs, row.ID)
	}
	assert.Equal(t, wantIDs, visited)
	assert.Equal(t, []int{2, 2, 2}, batchSizes)
	uniqueIDs := make(map[int64]struct{}, len(visited))
	for _, id := range visited {
		uniqueIDs[id] = struct{}{}
	}
	assert.Len(t, uniqueIDs, len(visited))
}

func prepareCostCatalogQueryDB(t *testing.T) {
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

	require.NoError(t, db.AutoMigrate(&Channel{}, &ChannelModelCostRule{}))
}

func prepareCostCatalogContractDB(t *testing.T, db *gorm.DB, dialect common.DatabaseType) {
	t.Helper()

	sqlDB, err := db.DB()
	require.NoError(t, err)
	previousDB := DB
	previousMainType := common.MainDatabaseType()
	previousLogType := common.LogDatabaseType()
	DB = db
	common.SetDatabaseTypes(dialect, previousLogType)
	t.Cleanup(func() {
		require.NoError(t, db.Migrator().DropTable(&ChannelModelCostRule{}, &Channel{}))
		DB = previousDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
		require.NoError(t, sqlDB.Close())
	})
	require.NoError(t, db.AutoMigrate(&Channel{}, &ChannelModelCostRule{}))
}

func costCatalogQueryGORMConfig() *gorm.Config {
	return &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
		NamingStrategy: schema.NamingStrategy{
			TablePrefix: fmt.Sprintf("ccq_%x_", time.Now().UnixNano()),
		},
	}
}

func seedCostCatalogQueryRows(t *testing.T) {
	t.Helper()

	channels := []Channel{
		{Id: 10, Name: "Alpha", Type: 1, Key: "alpha-key"},
		{Id: 20, Name: "Beta", Type: 2, Key: "beta-key"},
	}
	require.NoError(t, DB.Create(&channels).Error)

	rules := []ChannelModelCostRule{
		costCatalogQueryRule(10, "vendor-100%_literal", 1, types.CostRuleActive, types.CostModePerRequest, "config_import"),
		costCatalogQueryRule(999, "vendor-100%_literal", 1, types.CostRuleActive, types.CostModePerRequest, "config_import"),
		costCatalogQueryRule(20, "vendor-100x-literal", 1, types.CostRuleActive, types.CostModePerDuration, "manual"),
		costCatalogQueryRule(10, "vendor-100%_literal", 2, types.CostRuleDraft, types.CostModePerRequest, "manual"),
		costCatalogQueryRule(10, "vendor-100%_literal", 0, types.CostRuleRetired, types.CostModePerRequest, "manual"),
		costCatalogQueryRule(20, "vendor-case-model", 3, types.CostRuleActive, types.CostModePerRequest, "config_import"),
	}
	require.NoError(t, DB.Create(&rules).Error)
}

func costCatalogQueryRule(channelID int, modelName string, version int, status types.CostRuleStatus, mode types.CostMode, source string) ChannelModelCostRule {
	return ChannelModelCostRule{
		ChannelID: channelID, BillableUpstreamModel: modelName, CostVariantKey: string(types.DefaultCostVariantKey),
		Version: version, Status: string(status), CostMode: string(mode), SchemaVersion: 1,
		ConfigJSON: `{"currency":"USD","unit_price":"0.25"}`, Source: source,
	}
}
