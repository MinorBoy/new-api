package controller

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/cost_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestCostAccountingSystemTaskHandlerSchedulesRecoverableRows(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	previousDB := model.DB
	model.DB = db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		model.DB = previousDB
		require.NoError(t, sqlDB.Close())
	})
	require.NoError(t, db.AutoMigrate(&model.CostAccountingAttempt{}, &model.Option{}))
	previousMode := cost_setting.Runtime().Mode
	common.OptionMapRWMutex.Lock()
	previousOptionMap := common.OptionMap
	if common.OptionMap == nil {
		common.OptionMap = map[string]string{}
	}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptionMap
		common.OptionMapRWMutex.Unlock()
	})
	require.NoError(t, model.UpdateOption(cost_setting.ConfigName+"."+cost_setting.KeyMode, string(types.CostAccountingDisabled)))
	t.Cleanup(func() {
		require.NoError(t, model.UpdateOption(cost_setting.ConfigName+"."+cost_setting.KeyMode, string(previousMode)))
	})
	old := time.Now().Add(-time.Hour).Unix()
	require.NoError(t, db.Create(&model.CostAccountingAttempt{
		CostRequestID: 1, AttemptNo: 1, Status: string(types.CostAttemptPrepared),
		PreparedAt: old, CreatedAt: old, UpdatedAt: old,
	}).Error)

	handler := costAccountingRecoveryHandler{}
	assert.Equal(t, model.SystemTaskTypeCostAccountingRecovery, handler.Type())
	assert.Equal(t, time.Minute, handler.Interval())
	assert.True(t, handler.Enabled())
	payload, ok := handler.NewPayload().(costAccountingRecoveryTaskPayload)
	require.True(t, ok)
	assert.Positive(t, payload.Limit)
}
