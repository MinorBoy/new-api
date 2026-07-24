package model

import (
	"errors"
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

func TestCostAccountingMigrationSQLite(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), costAccountingMigrationGORMConfig())
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })

	testCostAccountingMigrationContracts(t, db, common.DatabaseTypeSQLite)
}

func TestCostAccountingMigrationConfiguredDatabases(t *testing.T) {
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
			db, err := gorm.Open(test.dialector(dsn), costAccountingMigrationGORMConfig())
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })

			testCostAccountingMigrationContracts(t, db, test.dialect)
		})
	}
}

func costAccountingMigrationGORMConfig() *gorm.Config {
	prefix := fmt.Sprintf("cam_%x_", time.Now().UnixNano())
	return &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
		NamingStrategy: schema.NamingStrategy{
			TablePrefix: prefix,
		},
	}
}

func testCostAccountingMigrationContracts(t *testing.T, db *gorm.DB, dialect common.DatabaseType) {
	t.Helper()
	previousDB := DB
	previousMainType := common.MainDatabaseType()
	previousLogType := common.LogDatabaseType()
	DB = db
	common.SetDatabaseTypes(dialect, previousLogType)
	defer func() {
		DB = previousDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
	}()

	t.Cleanup(func() {
		require.NoError(t, db.Migrator().DropTable(
			&CostAccountingAudit{},
			&CostAccountingAttempt{},
			&CostAccountingRequest{},
			&ChannelModelCostRule{},
		))
	})
	require.NoError(t, db.AutoMigrate(
		&ChannelModelCostRule{},
		&CostAccountingRequest{},
		&CostAccountingAttempt{},
		&CostAccountingAudit{},
	))

	requests := []CostAccountingRequest{
		{RequestID: "nullable-task-1"},
		{RequestID: "nullable-task-2"},
		{RequestID: "nullable-task-3"},
	}
	for i := range requests {
		require.NoError(t, db.Create(&requests[i]).Error)
	}
	taskID := "shared-public-task"
	require.NoError(t, db.Create(&CostAccountingRequest{RequestID: "task-owner", TaskID: &taskID}).Error)
	require.Error(t, db.Create(&CostAccountingRequest{RequestID: "task-duplicate", TaskID: &taskID}).Error)

	firstAttempt := CostAccountingAttempt{CostRequestID: requests[0].ID, AttemptNo: 1}
	require.NoError(t, db.Create(&firstAttempt).Error)
	require.Error(t, db.Create(&CostAccountingAttempt{CostRequestID: requests[0].ID, AttemptNo: 1}).Error)
	require.NoError(t, db.Create(&CostAccountingAttempt{CostRequestID: requests[0].ID, AttemptNo: 2}).Error)
	require.NoError(t, db.Create(&CostAccountingAttempt{CostRequestID: requests[1].ID, AttemptNo: 1}).Error)

	uniqueRule := migrationCostRule(20, "unique-model", 1, types.CostRuleDraft)
	require.NoError(t, CreateCostRuleDraft(&uniqueRule))
	duplicateRule := migrationCostRule(20, "unique-model", 1, types.CostRuleDraft)
	require.Error(t, CreateCostRuleDraft(&duplicateRule))
	nextVersion := migrationCostRule(20, "unique-model", 2, types.CostRuleDraft)
	require.NoError(t, CreateCostRuleDraft(&nextVersion))

	active := migrationCostRule(30, "activation-model", 1, types.CostRuleActive)
	activeFrom := int64(100)
	active.EffectiveFrom = &activeFrom
	require.NoError(t, db.Create(&active).Error)
	draft := migrationCostRule(30, "activation-model", 2, types.CostRuleDraft)
	require.NoError(t, CreateCostRuleDraft(&draft))

	activationErr := errors.New("migration activation rejected")
	activationUpdates := 0
	callbackName := fmt.Sprintf("test:fail_cost_rule_activation_%d", time.Now().UnixNano())
	callbackRegistered := true
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		if strings.HasSuffix(tx.Statement.Table, "channel_model_cost_rules") {
			activationUpdates++
			if activationUpdates == 2 {
				tx.AddError(activationErr)
			}
		}
	}))
	t.Cleanup(func() {
		if callbackRegistered {
			_ = db.Callback().Update().Remove(callbackName)
		}
	})
	_, err := ActivateChannelModelCostRule(draft.ID, 42, 200, nil)
	require.ErrorIs(t, err, activationErr)
	require.Equal(t, 2, activationUpdates)
	require.NoError(t, db.Callback().Update().Remove(callbackName))
	callbackRegistered = false
	var rolledBackActive, rolledBackDraft ChannelModelCostRule
	require.NoError(t, db.First(&rolledBackActive, active.ID).Error)
	require.NoError(t, db.First(&rolledBackDraft, draft.ID).Error)
	assert.Equal(t, string(types.CostRuleActive), rolledBackActive.Status)
	assert.Nil(t, rolledBackActive.EffectiveTo)
	assert.Equal(t, string(types.CostRuleDraft), rolledBackDraft.Status)
	assert.Nil(t, rolledBackDraft.EffectiveFrom)

	activated, err := ActivateChannelModelCostRule(draft.ID, 43, 300, nil)
	require.NoError(t, err)
	assert.Equal(t, string(types.CostRuleActive), activated.Status)
	assert.Equal(t, 43, activated.ActivatedBy)
	require.NotNil(t, activated.EffectiveFrom)
	assert.Equal(t, int64(300), *activated.EffectiveFrom)
	require.NoError(t, db.First(&active, active.ID).Error)
	assert.Equal(t, string(types.CostRuleRetired), active.Status)
	require.NotNil(t, active.EffectiveTo)
	assert.Equal(t, int64(300), *active.EffectiveTo)

	request := CostAccountingRequest{RequestID: "settlement-cas"}
	attempt := migrationCostAttempt()
	require.NoError(t, PrepareCostAttempt(&request, &attempt))
	require.NoError(t, TransitionCostAttempt(attempt.ID, types.CostAttemptPrepared, types.CostAttemptDispatching, nil))
	cost := int64(250_000_000)
	settlement := SettleCostAttemptInput{
		AttemptID: attempt.ID, From: types.CostAttemptDispatching, To: types.CostAttemptSettled,
		OriginalCost: "0.25", CostNanoUSD: &cost, UpstreamAccepted: true, SettledAt: 400,
	}
	require.NoError(t, SettleCostAttempt(settlement))
	require.ErrorIs(t, SettleCostAttempt(settlement), ErrCostStateConflict)
	require.NoError(t, db.First(&request, request.ID).Error)
	assert.Equal(t, cost, request.ConfirmedCostNanoUSD)
	assert.Equal(t, 1, request.AttemptCount)
}

func migrationCostRule(channelID int, modelName string, version int, status types.CostRuleStatus) ChannelModelCostRule {
	return ChannelModelCostRule{
		ChannelID: channelID, BillableUpstreamModel: modelName, Version: version,
		Status: string(status), CostMode: string(types.CostModePerRequest), SchemaVersion: 1,
		ConfigJSON: `{"unit_price":"0.25"}`, Source: "manual", CreatedBy: 11,
	}
}

func migrationCostAttempt() CostAccountingAttempt {
	return CostAccountingAttempt{
		AttemptNo: 1, ChannelID: 40, ChannelName: "migration-supplier", ChannelType: 1,
		PredictedUpstreamModel: "migration-model", BillableUpstreamModel: "migration-model",
		RuleID: 1, RuleVersion: 1, CostMode: string(types.CostModePerRequest), SchemaVersion: 1,
		RuleConfigJSON: `{"unit_price":"0.25"}`, ChargeEvent: string(types.CostChargeResponseSucceeded),
		MeterSource: string(types.CostMeterValidatedRequest), BillableRequestCount: 1, RequestMeterJSON: `{}`,
	}
}
