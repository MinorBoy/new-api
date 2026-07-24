package service

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCostRecoveryAdvancesOnlyProvableStaleStatesAndIsIdempotent(t *testing.T) {
	prepareCostAttemptServiceDB(t)
	config := validDurationCostConfig(types.CostMeterUpstreamActual)
	config.ChargeEvent = types.CostChargeTaskSucceeded
	seedActiveAttemptRule(t, types.CostModePerDuration, config)

	now := time.Now()
	staleAt := now.Add(-time.Hour).Unix()
	prepared := prepareRecoveryCostAttempt(t, "prepared")
	require.NoError(t, model.DB.Model(&model.CostAccountingAttempt{}).Where("id = ?", prepared.AttemptID).
		Updates(map[string]any{"prepared_at": staleAt, "updated_at": staleAt}).Error)
	fresh := prepareRecoveryCostAttempt(t, "fresh-prepared")

	dispatching := prepareRecoveryCostAttempt(t, "dispatching")
	require.NoError(t, AuthorizeCostDispatch(context.Background(), dispatching))
	require.NoError(t, model.DB.Model(&model.CostAccountingAttempt{}).Where("id = ?", dispatching.AttemptID).
		Updates(map[string]any{"dispatching_at": staleAt, "updated_at": staleAt}).Error)

	awaiting := prepareRecoveryCostAttempt(t, "awaiting")
	require.NoError(t, AuthorizeCostDispatch(context.Background(), awaiting))
	require.NoError(t, RecordCostDispatchOutcome(context.Background(), awaiting, types.CostOutcome{
		Status: types.CostAttemptAwaitingMeter, UpstreamAccepted: true,
	}))
	duration := "6"
	meterJSON, err := common.Marshal(types.CostMeter{
		Source: types.CostMeterUpstreamActual, DurationSeconds: &duration,
	})
	require.NoError(t, err)
	require.NoError(t, model.DB.Model(&model.CostAccountingAttempt{}).Where("id = ?", awaiting.AttemptID).
		Updates(map[string]any{"actual_meter_json": string(meterJSON), "updated_at": staleAt}).Error)

	unknown := prepareRecoveryCostAttempt(t, "unknown")
	require.NoError(t, AuthorizeCostDispatch(context.Background(), unknown))
	require.NoError(t, RecordCostDispatchOutcome(context.Background(), unknown, types.CostOutcome{
		Status: types.CostAttemptUnknown, FailureCode: "transport_outcome_unknown",
	}))

	failed := prepareRecoveryCostAttempt(t, "failed")
	require.NoError(t, AuthorizeCostDispatch(context.Background(), failed))
	require.NoError(t, RecordCostDispatchOutcome(context.Background(), failed, types.CostOutcome{
		Status: types.CostAttemptAwaitingMeter, UpstreamAccepted: true,
	}))
	require.NoError(t, model.TransitionCostAttempt(failed.AttemptID, types.CostAttemptAwaitingMeter, types.CostAttemptSettlementFailed, map[string]any{
		"failure_code": "cost_meter_invalid", "actual_meter_json": "{}", "terminal_at": staleAt,
	}))

	summary, err := RecoverStaleCostAccounting(context.Background(), now, 20)
	require.NoError(t, err)
	assert.Equal(t, CostRecoverySummary{PreparedClosed: 1, DispatchingUnknown: 1, AwaitingSettled: 1}, summary)
	assert.Equal(t, string(types.CostAttemptNotDispatched), loadCostAttempt(t, prepared.AttemptID).Status)
	assert.Equal(t, string(types.CostAttemptUnknown), loadCostAttempt(t, dispatching.AttemptID).Status)
	settled := loadCostAttempt(t, awaiting.AttemptID)
	assert.Equal(t, string(types.CostAttemptSettled), settled.Status)
	require.NotNil(t, settled.CostNanoUSD)
	assert.Equal(t, int64(600_000_000), *settled.CostNanoUSD)
	assert.Equal(t, string(types.CostAttemptUnknown), loadCostAttempt(t, unknown.AttemptID).Status)
	assert.Equal(t, string(types.CostAttemptSettlementFailed), loadCostAttempt(t, failed.AttemptID).Status)
	assert.Equal(t, string(types.CostAttemptPrepared), loadCostAttempt(t, fresh.AttemptID).Status)

	repeated, err := RecoverStaleCostAccounting(context.Background(), now.Add(time.Minute), 20)
	require.NoError(t, err)
	assert.Equal(t, CostRecoverySummary{}, repeated)
}

func TestCostRecoveryRespectsLimitAndOldestFirst(t *testing.T) {
	prepareCostAttemptServiceDB(t)
	seedActiveAttemptRule(t, types.CostModePerRequest, normalizedPerRequestConfig(t, "0.2"))
	now := time.Now()
	older := prepareRecoveryCostAttempt(t, "older")
	newer := prepareRecoveryCostAttempt(t, "newer")
	require.NoError(t, model.DB.Model(&model.CostAccountingAttempt{}).Where("id = ?", older.AttemptID).
		Updates(map[string]any{"prepared_at": now.Add(-2 * time.Hour).Unix(), "updated_at": now.Add(-2 * time.Hour).Unix()}).Error)
	require.NoError(t, model.DB.Model(&model.CostAccountingAttempt{}).Where("id = ?", newer.AttemptID).
		Updates(map[string]any{"prepared_at": now.Add(-time.Hour).Unix(), "updated_at": now.Add(-time.Hour).Unix()}).Error)

	summary, err := RecoverStaleCostAccounting(context.Background(), now, 1)
	require.NoError(t, err)
	assert.Equal(t, 1, summary.PreparedClosed)
	assert.Equal(t, string(types.CostAttemptNotDispatched), loadCostAttempt(t, older.AttemptID).Status)
	assert.Equal(t, string(types.CostAttemptPrepared), loadCostAttempt(t, newer.AttemptID).Status)
}

func TestCostReconcileAttemptUsesFrozenSnapshotAndAppendsOneAudit(t *testing.T) {
	prepareCostAttemptServiceDB(t)
	config := validDurationCostConfig(types.CostMeterUpstreamActual)
	config.ChargeEvent = types.CostChargeTaskSucceeded
	seedActiveAttemptRule(t, types.CostModePerDuration, config)
	handle := prepareRecoveryCostAttempt(t, "manual-attempt")
	require.NoError(t, AuthorizeCostDispatch(context.Background(), handle))
	require.NoError(t, RecordCostDispatchOutcome(context.Background(), handle, types.CostOutcome{
		Status: types.CostAttemptAwaitingMeter, UpstreamAccepted: true,
	}))
	attempt := loadCostAttempt(t, handle.AttemptID)
	require.NoError(t, markCostSettlementFailed(context.Background(), &attempt, types.CostMeter{}))

	changed := validDurationCostConfig(types.CostMeterUpstreamActual)
	changed.ChargeEvent = types.CostChargeTaskSucceeded
	changed.PricePerSecond = costStringPointer("9")
	normalized, err := NormalizeCostRuleConfig(types.CostModePerDuration, changed)
	require.NoError(t, err)
	changedJSON, err := common.Marshal(normalized)
	require.NoError(t, err)
	require.NoError(t, model.DB.Model(&model.ChannelModelCostRule{}).Where("id = ?", attempt.RuleID).
		Update("config_json", string(changedJSON)).Error)

	duration := "6"
	meter := &types.CostMeter{Source: types.CostMeterUpstreamActual, DurationSeconds: &duration}
	incompatible := &types.CostMeter{Source: types.CostMeterUpstreamUsage}
	require.Error(t, ReconcileCostAttempt(context.Background(), handle.AttemptID, 91, "settle", meter, "   "))
	require.Error(t, ReconcileCostAttempt(context.Background(), handle.AttemptID, 91, "confirm_zero", incompatible, "provider waiver"))
	require.NoError(t, ReconcileCostAttempt(context.Background(), handle.AttemptID, 91, "settle", meter, "supplier invoice"))
	require.ErrorIs(t, ReconcileCostAttempt(context.Background(), handle.AttemptID, 91, "settle", meter, "duplicate"), model.ErrCostStateConflict)

	settled := loadCostAttempt(t, handle.AttemptID)
	assert.Equal(t, string(types.CostAttemptSettled), settled.Status)
	require.NotNil(t, settled.CostNanoUSD)
	assert.Equal(t, int64(600_000_000), *settled.CostNanoUSD)
	var request model.CostAccountingRequest
	require.NoError(t, model.DB.First(&request, handle.CostRequestID).Error)
	assert.Equal(t, int64(600_000_000), request.ConfirmedCostNanoUSD)
	var audits []model.CostAccountingAudit
	require.NoError(t, model.DB.Where("cost_attempt_id = ?", handle.AttemptID).Find(&audits).Error)
	require.Len(t, audits, 1)
	assert.Equal(t, 91, audits[0].AdminID)
	assert.Equal(t, attempt.RuleID, audits[0].RuleID)
	assert.Equal(t, attempt.RuleVersion, audits[0].RuleVersion)
	assert.Equal(t, "supplier invoice", audits[0].Reason)
}

func TestCostReconcileAttemptRollsBackWhenAuditInsertFails(t *testing.T) {
	prepareCostAttemptServiceDB(t)
	seedActiveAttemptRule(t, types.CostModePerRequest, normalizedPerRequestConfig(t, "0.2"))
	handle := prepareRecoveryCostAttempt(t, "manual-rollback")
	require.NoError(t, AuthorizeCostDispatch(context.Background(), handle))
	require.NoError(t, RecordCostDispatchOutcome(context.Background(), handle, types.CostOutcome{
		Status: types.CostAttemptUnknown, FailureCode: "transport_outcome_unknown",
	}))
	require.NoError(t, model.DB.Migrator().DropTable(&model.CostAccountingAudit{}))
	t.Cleanup(func() {
		require.NoError(t, model.DB.AutoMigrate(&model.CostAccountingAudit{}))
	})

	require.Error(t, ReconcileCostAttempt(context.Background(), handle.AttemptID, 91, "confirm_zero", nil, "provider waiver"))
	assert.Equal(t, string(types.CostAttemptUnknown), loadCostAttempt(t, handle.AttemptID).Status)
}

func TestCostReconcileRevenueUsesFrozenQuotaSnapshotAndAppendsOneAudit(t *testing.T) {
	prepareCostAttemptServiceDB(t)
	seedActiveAttemptRule(t, types.CostModePerRequest, normalizedPerRequestConfig(t, "0.2"))
	handle := prepareRecoveryCostAttempt(t, "manual-revenue")
	require.NoError(t, model.TransitionCostAttempt(handle.AttemptID, types.CostAttemptPrepared, types.CostAttemptNotDispatched, map[string]any{
		"failure_code": "not_dispatched", "terminal_at": common.GetTimestamp(),
	}))
	require.NoError(t, model.RecognizeCostRevenue(model.RecognizeCostRevenueInput{
		CostRequestID: handle.CostRequestID, From: types.CostRevenuePending, To: types.CostRevenueFailed,
		FailureCode: "revenue_settlement_failed",
	}))

	require.Error(t, ReconcileCostRevenue(context.Background(), handle.CostRequestID, 92, 200, " "))
	require.NoError(t, ReconcileCostRevenue(context.Background(), handle.CostRequestID, 92, 200, "billing receipt"))
	require.ErrorIs(t, ReconcileCostRevenue(context.Background(), handle.CostRequestID, 92, 200, "duplicate"), model.ErrCostStateConflict)

	var request model.CostAccountingRequest
	require.NoError(t, model.DB.First(&request, handle.CostRequestID).Error)
	assert.Equal(t, string(types.CostRevenueSettled), request.RevenueStatus)
	require.NotNil(t, request.FinalUserQuota)
	assert.Equal(t, int64(200), *request.FinalUserQuota)
	assert.Equal(t, "500000", request.QuotaPerUnitSnapshot)
	require.NotNil(t, request.BilledRevenueEquivalentNanoUSD)
	assert.Equal(t, int64(400_000), *request.BilledRevenueEquivalentNanoUSD)
	assert.Equal(t, string(types.CostProfitComplete), request.ProfitStatus)
	require.NotNil(t, request.BilledGrossProfitNanoUSD)
	assert.Equal(t, int64(400_000), *request.BilledGrossProfitNanoUSD)

	var audits []model.CostAccountingAudit
	require.NoError(t, model.DB.Where("cost_request_id = ? AND cost_attempt_id IS NULL", handle.CostRequestID).Find(&audits).Error)
	require.Len(t, audits, 1)
	assert.Equal(t, 92, audits[0].AdminID)
	assert.Equal(t, string(types.CostRevenueFailed), audits[0].OldState)
	assert.Equal(t, string(types.CostRevenueSettled), audits[0].NewState)
	assert.Equal(t, "billing receipt", audits[0].Reason)
}

func TestCostReconcileRevenuePreservesExplicitZero(t *testing.T) {
	prepareCostAttemptServiceDB(t)
	seedActiveAttemptRule(t, types.CostModePerRequest, normalizedPerRequestConfig(t, "0.2"))
	handle := prepareRecoveryCostAttempt(t, "manual-zero-revenue")
	require.NoError(t, model.TransitionCostAttempt(handle.AttemptID, types.CostAttemptPrepared, types.CostAttemptNotDispatched, map[string]any{
		"failure_code": "not_dispatched", "terminal_at": common.GetTimestamp(),
	}))
	require.NoError(t, model.RecognizeCostRevenue(model.RecognizeCostRevenueInput{
		CostRequestID: handle.CostRequestID, From: types.CostRevenuePending, To: types.CostRevenueFailed,
		FailureCode: "revenue_settlement_failed",
	}))

	require.NoError(t, ReconcileCostRevenue(context.Background(), handle.CostRequestID, 92, 0, "zero-charge receipt"))
	var request model.CostAccountingRequest
	require.NoError(t, model.DB.First(&request, handle.CostRequestID).Error)
	assert.Equal(t, string(types.CostRevenueConfirmedZero), request.RevenueStatus)
	require.NotNil(t, request.FinalUserQuota)
	assert.Zero(t, *request.FinalUserQuota)
	require.NotNil(t, request.BilledRevenueEquivalentNanoUSD)
	assert.Zero(t, *request.BilledRevenueEquivalentNanoUSD)
	assert.Equal(t, string(types.CostProfitComplete), request.ProfitStatus)
	require.NotNil(t, request.BilledGrossProfitNanoUSD)
	assert.Zero(t, *request.BilledGrossProfitNanoUSD)
	assert.Nil(t, request.GrossMarginPPM)
}

func prepareRecoveryCostAttempt(t *testing.T, suffix string) *types.CostAttemptHandle {
	t.Helper()
	input := preparedAttemptInput()
	input.RequestID = "cost-recovery-" + suffix
	handle, err := PrepareCostAttempt(context.Background(), input)
	require.NoError(t, err)
	return handle
}
