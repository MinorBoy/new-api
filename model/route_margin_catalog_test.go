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

func TestListActiveImportedRouteMarginTargets(t *testing.T) {
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
	require.NoError(t, db.AutoMigrate(&Channel{}, &RoutingPolicy{}, &RouteTarget{}))

	channel := Channel{Id: 11, Name: "channel-a", Type: 47, Key: "test-key"}
	require.NoError(t, db.Create(&channel).Error)
	activePolicy := RoutingPolicy{
		GroupName: "default", Model: "doubao-seedance-2-0-260128", Enabled: true,
		DefaultResolution: "720p", DefaultDuration: 4, DefaultRatio: "16:9",
	}
	disabledPolicy := RoutingPolicy{
		GroupName: "default", Model: "doubao-seedance-2-0-fast-260128", Enabled: false,
		DefaultResolution: "720p", DefaultDuration: 4, DefaultRatio: "16:9",
	}
	require.NoError(t, db.Create(&activePolicy).Error)
	require.NoError(t, db.Create(&disabledPolicy).Error)

	minimumMarginBPS := 2500
	constraints := `{"output_resolutions":["720p"],"durations":{"values":[4]},"reference_limits":{"images":9,"videos":3,"audios":3}}`
	activeTarget := RouteTarget{
		PolicyID: activePolicy.ID, ChannelID: channel.Id, Name: "route-target/active",
		UpstreamModel: "vendor-model", CostVariantKey: "720p", TargetPriority: 100,
		MinimumExpectedMarginBPS: &minimumMarginBPS, Constraints: constraints, Enabled: true,
		ManagedBy: string(types.RouteTargetManagedByConfigImport),
	}
	require.NoError(t, db.Create(&activeTarget).Error)

	retiredAt := int64(1700000000)
	excluded := []RouteTarget{
		{
			PolicyID: activePolicy.ID, ChannelID: channel.Id, Name: "route-target/manual",
			UpstreamModel: "manual-model", CostVariantKey: "default", Constraints: constraints,
			Enabled: true, ManagedBy: string(types.RouteTargetManagedByManual),
		},
		{
			PolicyID: activePolicy.ID, ChannelID: channel.Id, Name: "route-target/retired",
			UpstreamModel: "retired-model", CostVariantKey: "default", Constraints: constraints,
			Enabled: false, ManagedBy: string(types.RouteTargetManagedByConfigImport), RetiredAt: &retiredAt,
		},
		{
			PolicyID: disabledPolicy.ID, ChannelID: channel.Id, Name: "route-target/policy-disabled",
			UpstreamModel: "disabled-model", CostVariantKey: "default", Constraints: constraints,
			Enabled: true, ManagedBy: string(types.RouteTargetManagedByConfigImport),
		},
	}
	require.NoError(t, db.Create(&excluded).Error)

	rows, err := ListActiveImportedRouteMarginTargets(RouteMarginTargetQuery{})

	require.NoError(t, err)
	require.Len(t, rows, 1)
	row := rows[0]
	assert.Equal(t, activeTarget.ID, row.TargetID)
	assert.Equal(t, activePolicy.ID, row.PolicyID)
	assert.Equal(t, "default", row.GroupName)
	assert.Equal(t, "doubao-seedance-2-0-260128", row.CanonicalModel)
	assert.Equal(t, "720p", row.DefaultResolution)
	assert.Equal(t, 4, row.DefaultDuration)
	assert.Equal(t, "16:9", row.DefaultRatio)
	assert.Equal(t, channel.Id, row.ChannelID)
	assert.Equal(t, "channel-a", row.ChannelName)
	assert.Equal(t, 47, row.ChannelType)
	assert.Equal(t, "vendor-model", row.UpstreamModel)
	assert.Equal(t, "720p", row.CostVariantKey)
	require.NotNil(t, row.MinimumExpectedMarginBPS)
	assert.Equal(t, 2500, *row.MinimumExpectedMarginBPS)
	assert.JSONEq(t, constraints, row.Constraints)

	filters := []RouteMarginTargetQuery{
		{ChannelID: 999},
		{CanonicalModel: "doubao-seedance-2-0-mini-260615"},
		{UpstreamModel: "other-vendor-model"},
		{TargetName: "route-target/other"},
	}
	for _, filter := range filters {
		filtered, filterErr := ListActiveImportedRouteMarginTargets(filter)
		require.NoError(t, filterErr)
		assert.Empty(t, filtered)
	}
}
