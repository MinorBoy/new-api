package model

import (
	"math"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func prepareCostAccountingDB(t *testing.T) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	previousDB := DB
	DB = db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		DB = previousDB
		require.NoError(t, sqlDB.Close())
	})

	require.NoError(t, db.AutoMigrate(
		&ChannelModelCostRule{},
		&CostAccountingRequest{},
		&CostAccountingAttempt{},
		&CostAccountingAudit{},
	))
}

func TestCostAccountingRequestAndAttemptUniqueness(t *testing.T) {
	prepareCostAccountingDB(t)

	requests := []CostAccountingRequest{
		{RequestID: "request-1"},
		{RequestID: "request-2"},
		{RequestID: "request-3"},
	}
	for i := range requests {
		require.NoError(t, DB.Create(&requests[i]).Error)
	}

	require.Error(t, DB.Create(&CostAccountingRequest{RequestID: "request-1"}).Error)

	taskID := "public-task-1"
	require.NoError(t, DB.Create(&CostAccountingRequest{RequestID: "request-4", TaskID: &taskID}).Error)
	require.Error(t, DB.Create(&CostAccountingRequest{RequestID: "request-5", TaskID: &taskID}).Error)

	firstAttempt := CostAccountingAttempt{CostRequestID: requests[0].ID, AttemptNo: 1}
	require.NoError(t, DB.Create(&firstAttempt).Error)
	require.Error(t, DB.Create(&CostAccountingAttempt{CostRequestID: requests[0].ID, AttemptNo: 1}).Error)
	require.NoError(t, DB.Create(&CostAccountingAttempt{CostRequestID: requests[0].ID, AttemptNo: 2}).Error)
}

func TestTransitionCostAttemptRequiresExpectedState(t *testing.T) {
	prepareCostAccountingDB(t)
	attempt := seedPreparedAttempt(t)

	require.NoError(t, TransitionCostAttempt(attempt.ID, types.CostAttemptPrepared, types.CostAttemptDispatching, nil))
	err := TransitionCostAttempt(attempt.ID, types.CostAttemptPrepared, types.CostAttemptDispatching, nil)
	assert.ErrorIs(t, err, ErrCostStateConflict)
}

func TestSettleCostAttemptRecomputesRequestAtomically(t *testing.T) {
	prepareCostAccountingDB(t)
	revenue := int64(1_000_000_000)
	request := CostAccountingRequest{
		RequestID:                      "request-settle",
		RevenueStatus:                  string(types.CostRevenueSettled),
		ProfitStatus:                   string(types.CostProfitIncompleteCost),
		BilledRevenueEquivalentNanoUSD: &revenue,
	}
	attempt := preparedCostAttempt(1)
	require.NoError(t, PrepareCostAttempt(&request, &attempt))
	require.NoError(t, TransitionCostAttempt(attempt.ID, types.CostAttemptPrepared, types.CostAttemptDispatching, nil))

	settledAt := int64(200)
	cost := int64(200_000_000)
	require.NoError(t, SettleCostAttempt(SettleCostAttemptInput{
		AttemptID:    attempt.ID,
		From:         types.CostAttemptDispatching,
		To:           types.CostAttemptSettled,
		OriginalCost: "0.2",
		CostNanoUSD:  &cost,
		SettledAt:    settledAt,
	}))

	require.NoError(t, DB.First(&attempt, attempt.ID).Error)
	assert.Equal(t, string(types.CostAttemptSettled), attempt.Status)
	assert.Equal(t, cost, *attempt.CostNanoUSD)

	require.NoError(t, DB.First(&request, request.ID).Error)
	assert.Equal(t, cost, request.ConfirmedCostNanoUSD)
	assert.Equal(t, 1, request.AttemptCount)
	assert.Equal(t, string(types.CostProfitComplete), request.ProfitStatus)
	assert.Equal(t, int64(800_000_000), *request.BilledGrossProfitNanoUSD)
	assert.Equal(t, int64(800_000), *request.GrossMarginPPM)
	require.NotNil(t, request.ProfitRecognizedAt)
	assert.Equal(t, settledAt, *request.ProfitRecognizedAt)
}

func TestCostAttemptSettlementRollsBackWhenRequestCostOverflows(t *testing.T) {
	prepareCostAccountingDB(t)
	zeroRevenue := int64(0)
	request := CostAccountingRequest{
		RequestID:                      "request-overflow",
		RevenueStatus:                  string(types.CostRevenueConfirmedZero),
		ProfitStatus:                   string(types.CostProfitIncompleteCost),
		BilledRevenueEquivalentNanoUSD: &zeroRevenue,
	}
	first := preparedCostAttempt(1)
	require.NoError(t, PrepareCostAttempt(&request, &first))
	second := preparedCostAttempt(2)
	require.NoError(t, PrepareCostAttempt(&CostAccountingRequest{RequestID: request.RequestID}, &second))
	require.NoError(t, TransitionCostAttempt(first.ID, types.CostAttemptPrepared, types.CostAttemptDispatching, nil))
	require.NoError(t, TransitionCostAttempt(second.ID, types.CostAttemptPrepared, types.CostAttemptDispatching, nil))

	maxCost := int64(math.MaxInt64)
	require.NoError(t, SettleCostAttempt(SettleCostAttemptInput{
		AttemptID: first.ID, From: types.CostAttemptDispatching, To: types.CostAttemptSettled,
		OriginalCost: "9223372036.854775807", CostNanoUSD: &maxCost, SettledAt: 200,
	}))
	one := int64(1)
	err := SettleCostAttempt(SettleCostAttemptInput{
		AttemptID: second.ID, From: types.CostAttemptDispatching, To: types.CostAttemptSettled,
		OriginalCost: "0.000000001", CostNanoUSD: &one, SettledAt: 201,
	})
	require.Error(t, err)

	require.NoError(t, DB.First(&second, second.ID).Error)
	assert.Equal(t, string(types.CostAttemptDispatching), second.Status)
	assert.Nil(t, second.CostNanoUSD)
	require.NoError(t, DB.First(&request, request.ID).Error)
	assert.Equal(t, maxCost, request.ConfirmedCostNanoUSD)
}

func TestProfitRecognizedAtIsSetOnlyOnce(t *testing.T) {
	prepareCostAccountingDB(t)
	revenue := int64(1_000)
	request := CostAccountingRequest{
		RequestID:                      "request-recognized-at",
		RevenueStatus:                  string(types.CostRevenueSettled),
		ProfitStatus:                   string(types.CostProfitIncompleteCost),
		BilledRevenueEquivalentNanoUSD: &revenue,
	}
	first := preparedCostAttempt(1)
	require.NoError(t, PrepareCostAttempt(&request, &first))
	require.NoError(t, TransitionCostAttempt(first.ID, types.CostAttemptPrepared, types.CostAttemptDispatching, nil))
	firstCost := int64(100)
	require.NoError(t, SettleCostAttempt(SettleCostAttemptInput{
		AttemptID: first.ID, From: types.CostAttemptDispatching, To: types.CostAttemptSettled,
		OriginalCost: "0.0000001", CostNanoUSD: &firstCost, SettledAt: 200,
	}))

	second := preparedCostAttempt(2)
	require.NoError(t, PrepareCostAttempt(&CostAccountingRequest{RequestID: request.RequestID}, &second))
	require.NoError(t, TransitionCostAttempt(second.ID, types.CostAttemptPrepared, types.CostAttemptDispatching, nil))
	secondCost := int64(100)
	require.NoError(t, SettleCostAttempt(SettleCostAttemptInput{
		AttemptID: second.ID, From: types.CostAttemptDispatching, To: types.CostAttemptSettled,
		OriginalCost: "0.0000001", CostNanoUSD: &secondCost, SettledAt: 300,
	}))

	require.NoError(t, DB.First(&request, request.ID).Error)
	require.NotNil(t, request.ProfitRecognizedAt)
	assert.Equal(t, int64(200), *request.ProfitRecognizedAt)
	assert.Equal(t, int64(800), *request.BilledGrossProfitNanoUSD)
}

func TestRecognizeCostRevenueCompletesSettledRequest(t *testing.T) {
	prepareCostAccountingDB(t)
	request := CostAccountingRequest{RequestID: "request-revenue"}
	attempt := preparedCostAttempt(1)
	require.NoError(t, PrepareCostAttempt(&request, &attempt))
	require.NoError(t, TransitionCostAttempt(attempt.ID, types.CostAttemptPrepared, types.CostAttemptDispatching, nil))
	zeroCost := int64(0)
	require.NoError(t, SettleCostAttempt(SettleCostAttemptInput{
		AttemptID: attempt.ID, From: types.CostAttemptDispatching, To: types.CostAttemptSettled,
		OriginalCost: "0", CostNanoUSD: &zeroCost, SettledAt: 200,
	}))

	quota := int64(500_000)
	revenue := int64(1_000_000_000)
	require.NoError(t, RecognizeCostRevenue(RecognizeCostRevenueInput{
		CostRequestID:        request.ID,
		From:                 types.CostRevenuePending,
		To:                   types.CostRevenueSettled,
		FinalUserQuota:       &quota,
		QuotaPerUnitSnapshot: "500000",
		RevenueNanoUSD:       &revenue,
		SettledAt:            300,
	}))

	require.NoError(t, DB.First(&request, request.ID).Error)
	assert.Equal(t, string(types.CostRevenueSettled), request.RevenueStatus)
	assert.Equal(t, string(types.CostProfitComplete), request.ProfitStatus)
	assert.Equal(t, revenue, *request.BilledGrossProfitNanoUSD)
	assert.Equal(t, int64(300), *request.ProfitRecognizedAt)
}

func TestRecognizeCostRevenueFailureKeepsValidUpdateTimestamp(t *testing.T) {
	prepareCostAccountingDB(t)
	request := CostAccountingRequest{RequestID: "request-revenue-failed"}
	attempt := preparedCostAttempt(1)
	require.NoError(t, PrepareCostAttempt(&request, &attempt))

	require.NoError(t, RecognizeCostRevenue(RecognizeCostRevenueInput{
		CostRequestID: request.ID,
		From:          types.CostRevenuePending,
		To:            types.CostRevenueFailed,
		FailureCode:   "billing_persistence_unknown",
	}))

	require.NoError(t, DB.First(&request, request.ID).Error)
	assert.Equal(t, string(types.CostRevenueFailed), request.RevenueStatus)
	assert.Positive(t, request.UpdatedAt)
}

func TestReconcileCostAttemptAppendsAuditAndRecomputesRequest(t *testing.T) {
	prepareCostAccountingDB(t)
	revenue := int64(1_000)
	request := CostAccountingRequest{
		RequestID:                      "request-reconcile",
		RevenueStatus:                  string(types.CostRevenueSettled),
		BilledRevenueEquivalentNanoUSD: &revenue,
	}
	attempt := preparedCostAttempt(1)
	require.NoError(t, PrepareCostAttempt(&request, &attempt))
	require.NoError(t, TransitionCostAttempt(attempt.ID, types.CostAttemptPrepared, types.CostAttemptDispatching, nil))
	require.NoError(t, TransitionCostAttempt(attempt.ID, types.CostAttemptDispatching, types.CostAttemptUnknown, map[string]any{
		"failure_code": "upstream_result_ambiguous",
	}))

	cost := int64(250)
	require.NoError(t, ReconcileCostAttempt(ReconcileCostAttemptInput{
		AttemptID:    attempt.ID,
		AdminID:      42,
		To:           types.CostAttemptSettled,
		MeterJSON:    `{"source":"upstream_actual","duration_seconds":"1"}`,
		OriginalCost: "0.00000025",
		CostNanoUSD:  &cost,
		Reason:       "matched supplier invoice",
		CreatedAt:    400,
	}))

	audits, err := ListCostAccountingAudits(request.ID)
	require.NoError(t, err)
	require.Len(t, audits, 1)
	assert.Equal(t, string(types.CostAttemptUnknown), audits[0].OldState)
	assert.Equal(t, string(types.CostAttemptSettled), audits[0].NewState)
	assert.Equal(t, "matched supplier invoice", audits[0].Reason)
	assert.Equal(t, 42, audits[0].AdminID)

	require.NoError(t, DB.First(&request, request.ID).Error)
	assert.Equal(t, string(types.CostProfitComplete), request.ProfitStatus)
	assert.Equal(t, int64(750), *request.BilledGrossProfitNanoUSD)
}

func TestCostAccountingMigrationUsesPortableSchema(t *testing.T) {
	prepareCostAccountingDB(t)

	type schemaRow struct {
		SQL string
	}
	var rows []schemaRow
	require.NoError(t, DB.Raw(`SELECT sql FROM sqlite_master WHERE name LIKE 'cost_accounting_%' OR name = 'channel_model_cost_rules'`).Scan(&rows).Error)
	require.NotEmpty(t, rows)
	for _, row := range rows {
		sql := strings.ToUpper(row.SQL)
		assert.NotContains(t, sql, " JSON")
		assert.NotContains(t, sql, "ON DELETE CASCADE")
		assert.NotContains(t, sql, " WHERE ")
	}
}

func seedPreparedAttempt(t *testing.T) CostAccountingAttempt {
	t.Helper()
	request := CostAccountingRequest{RequestID: "request-seed"}
	attempt := preparedCostAttempt(1)
	require.NoError(t, PrepareCostAttempt(&request, &attempt))
	return attempt
}

func preparedCostAttempt(attemptNo int) CostAccountingAttempt {
	return CostAccountingAttempt{
		AttemptNo:              attemptNo,
		ChannelID:              7,
		ChannelName:            "supplier",
		ChannelType:            1,
		PredictedUpstreamModel: "vendor-model",
		BillableUpstreamModel:  "vendor-model",
		RuleID:                 10,
		RuleVersion:            1,
		CostMode:               string(types.CostModePerRequest),
		SchemaVersion:          1,
		RuleConfigJSON:         `{"unit_price":"0.2"}`,
		ChargeEvent:            string(types.CostChargeResponseSucceeded),
		MeterSource:            string(types.CostMeterValidatedRequest),
		BillableRequestCount:   1,
		RequestMeterJSON:       `{}`,
	}
}

func costInt64Pointer(value int64) *int64 {
	return &value
}
