package service

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthorizeDispatchBeforeTransport(t *testing.T) {
	prepareCostAttemptServiceDB(t)
	seedActiveAttemptRule(t, types.CostModePerRequest, normalizedPerRequestConfig(t, "0.2"))

	handle, err := PrepareCostAttempt(context.Background(), preparedAttemptInput())
	require.NoError(t, err)
	require.Equal(t, string(types.CostAttemptPrepared), loadCostAttempt(t, handle.AttemptID).Status)
	require.NoError(t, AuthorizeCostDispatch(context.Background(), handle))
	assert.Equal(t, string(types.CostAttemptDispatching), loadCostAttempt(t, handle.AttemptID).Status)

	err = AuthorizeCostDispatch(context.Background(), handle)
	assert.ErrorIs(t, err, model.ErrCostStateConflict)
}

func TestPrepareCostAttemptAllocatesMonotonicAttemptNumbers(t *testing.T) {
	prepareCostAttemptServiceDB(t)
	seedActiveAttemptRule(t, types.CostModePerRequest, normalizedPerRequestConfig(t, "0.2"))
	input := preparedAttemptInput()

	first, err := PrepareCostAttempt(context.Background(), input)
	require.NoError(t, err)
	second, err := PrepareCostAttempt(context.Background(), input)
	require.NoError(t, err)
	assert.Equal(t, 1, first.AttemptNo)
	assert.Equal(t, 2, second.AttemptNo)
}

func TestPrepareCostAttemptRejectsMissingCoverage(t *testing.T) {
	prepareCostAttemptServiceDB(t)
	_, err := PrepareCostAttempt(context.Background(), preparedAttemptInput())
	var coverageErr *CostCoverageError
	require.ErrorAs(t, err, &coverageErr)
	assert.Equal(t, 7, coverageErr.ChannelID)
	var requestCount int64
	require.NoError(t, model.DB.Model(&model.CostAccountingRequest{}).
		Where("request_id = ?", preparedAttemptInput().RequestID).Count(&requestCount).Error)
	assert.Zero(t, requestCount)
}

func TestRecordCostDispatchOutcomeClassifiesZeroAndAmbiguousFailures(t *testing.T) {
	prepareCostAttemptServiceDB(t)
	seedActiveAttemptRule(t, types.CostModeFree, types.CostRuleConfigV1{ZeroCostReason: "contract"})

	freeHandle, err := PrepareCostAttempt(context.Background(), preparedAttemptInput())
	require.NoError(t, err)
	require.NoError(t, AuthorizeCostDispatch(context.Background(), freeHandle))
	require.NoError(t, RecordCostDispatchOutcome(context.Background(), freeHandle, types.CostOutcome{
		Status: types.CostAttemptConfirmedZero,
	}))
	assert.Equal(t, string(types.CostAttemptConfirmedZero), loadCostAttempt(t, freeHandle.AttemptID).Status)

	unknownHandle, err := PrepareCostAttempt(context.Background(), preparedAttemptInput())
	require.NoError(t, err)
	require.NoError(t, AuthorizeCostDispatch(context.Background(), unknownHandle))
	require.NoError(t, RecordCostDispatchOutcome(context.Background(), unknownHandle, types.CostOutcome{
		Status: types.CostAttemptUnknown, FailureCode: "upstream_transport_ambiguous",
	}))
	assert.Equal(t, string(types.CostAttemptUnknown), loadCostAttempt(t, unknownHandle.AttemptID).Status)
}

func TestSettleSyncMissingMeterMarksSettlementFailed(t *testing.T) {
	prepareCostAttemptServiceDB(t)
	seedActiveAttemptRule(t, types.CostModePerToken, normalizedTokenCostConfig(t))
	handle, err := PrepareCostAttempt(context.Background(), preparedAttemptInput())
	require.NoError(t, err)
	require.NoError(t, AuthorizeCostDispatch(context.Background(), handle))
	require.NoError(t, RecordCostDispatchOutcome(context.Background(), handle, types.CostOutcome{
		Status: types.CostAttemptAwaitingMeter, UpstreamAccepted: true,
	}))

	err = SettleSyncCostAttempt(context.Background(), handle, types.CostMeter{Source: types.CostMeterUpstreamUsage})
	require.Error(t, err)
	assert.Equal(t, string(types.CostAttemptSettlementFailed), loadCostAttempt(t, handle.AttemptID).Status)
}

func TestCostPersistenceSurvivesCanceledClientContext(t *testing.T) {
	prepareCostAttemptServiceDB(t)
	seedActiveAttemptRule(t, types.CostModePerRequest, normalizedPerRequestConfig(t, "0.2"))
	handle, err := PrepareCostAttempt(context.Background(), preparedAttemptInput())
	require.NoError(t, err)
	require.NoError(t, AuthorizeCostDispatch(context.Background(), handle))
	require.NoError(t, RecordCostDispatchOutcome(context.Background(), handle, types.CostOutcome{
		Status: types.CostAttemptAwaitingMeter, UpstreamAccepted: true,
	}))

	clientCtx, cancel := context.WithCancel(context.Background())
	cancel()
	err = SettleSyncCostAttempt(clientCtx, handle, types.CostMeter{Source: types.CostMeterValidatedRequest})
	require.NoError(t, err)
	assert.Equal(t, string(types.CostAttemptSettled), loadCostAttempt(t, handle.AttemptID).Status)
}

func TestCostAccountingPersistenceContextIsDetachedAndBounded(t *testing.T) {
	clientCtx, cancelClient := context.WithCancel(context.Background())
	cancelClient()

	persistenceCtx, cancelPersistence := costAccountingPersistenceContext(clientCtx)
	defer cancelPersistence()
	deadline, ok := persistenceCtx.Deadline()
	require.True(t, ok)
	assert.NoError(t, persistenceCtx.Err())
	assert.WithinDuration(t, time.Now().Add(10*time.Second), deadline, time.Second)
}

func TestSettleCostAttemptPreservesExplicitZeroAndRejectsOversizedMeter(t *testing.T) {
	prepareCostAttemptServiceDB(t)
	seedActiveAttemptRule(t, types.CostModePerToken, normalizedTokenCostConfig(t))

	zeroHandle, err := PrepareCostAttempt(context.Background(), preparedAttemptInput())
	require.NoError(t, err)
	require.NoError(t, AuthorizeCostDispatch(context.Background(), zeroHandle))
	require.NoError(t, RecordCostDispatchOutcome(context.Background(), zeroHandle, types.CostOutcome{
		Status: types.CostAttemptAwaitingMeter, UpstreamAccepted: true,
	}))
	zero := int64(0)
	require.NoError(t, SettleSyncCostAttempt(context.Background(), zeroHandle, types.CostMeter{
		Source: types.CostMeterUpstreamUsage, TotalTokens: &zero,
	}))
	zeroAttempt := loadCostAttempt(t, zeroHandle.AttemptID)
	require.NotNil(t, zeroAttempt.CostNanoUSD)
	assert.Zero(t, *zeroAttempt.CostNanoUSD)
	assert.Contains(t, zeroAttempt.ActualMeterJSON, `"total_tokens":0`)

	oversizedHandle, err := PrepareCostAttempt(context.Background(), preparedAttemptInput())
	require.NoError(t, err)
	require.NoError(t, AuthorizeCostDispatch(context.Background(), oversizedHandle))
	require.NoError(t, RecordCostDispatchOutcome(context.Background(), oversizedHandle, types.CostOutcome{
		Status: types.CostAttemptAwaitingMeter, UpstreamAccepted: true,
	}))
	oversized := int64(relaycommon.MaxTokensLimit) + 1
	err = SettleSyncCostAttempt(context.Background(), oversizedHandle, types.CostMeter{
		Source: types.CostMeterUpstreamUsage, TotalTokens: &oversized,
	})
	require.Error(t, err)
	assert.Equal(t, string(types.CostAttemptSettlementFailed), loadCostAttempt(t, oversizedHandle.AttemptID).Status)
}

func TestMarkWinningCostAttemptRequiresPostDispatchStateAndIsIdempotent(t *testing.T) {
	prepareCostAttemptServiceDB(t)
	seedActiveAttemptRule(t, types.CostModePerRequest, normalizedPerRequestConfig(t, "0.2"))
	handle, err := PrepareCostAttempt(context.Background(), preparedAttemptInput())
	require.NoError(t, err)
	assert.ErrorIs(t, MarkWinningCostAttempt(context.Background(), handle), model.ErrCostStateConflict)

	require.NoError(t, AuthorizeCostDispatch(context.Background(), handle))
	require.NoError(t, RecordCostDispatchOutcome(context.Background(), handle, types.CostOutcome{
		Status: types.CostAttemptSettled, UpstreamAccepted: true,
	}))
	require.NoError(t, MarkWinningCostAttempt(context.Background(), handle))
	require.NoError(t, MarkWinningCostAttempt(context.Background(), handle))

	var request model.CostAccountingRequest
	require.NoError(t, model.DB.First(&request, handle.CostRequestID).Error)
	require.NotNil(t, request.WinningAttemptID)
	assert.Equal(t, handle.AttemptID, *request.WinningAttemptID)
}

func prepareCostAttemptServiceDB(t *testing.T) {
	t.Helper()
	require.NoError(t, model.DB.AutoMigrate(
		&model.Channel{}, &model.ChannelModelCostRule{}, &model.CostAccountingRequest{},
		&model.CostAccountingAttempt{}, &model.CostAccountingAudit{},
	))
	require.NoError(t, model.DB.Exec("DELETE FROM cost_accounting_audits").Error)
	require.NoError(t, model.DB.Exec("DELETE FROM cost_accounting_attempts").Error)
	require.NoError(t, model.DB.Exec("DELETE FROM cost_accounting_requests").Error)
	require.NoError(t, model.DB.Exec("DELETE FROM channel_model_cost_rules").Error)
	require.NoError(t, model.DB.Exec("DELETE FROM channels").Error)
	require.NoError(t, model.DB.Create(&model.Channel{Id: 7, Type: constant.ChannelTypeOpenAI, Name: "supplier", Key: "secret"}).Error)

	previousLookup := CostCapabilityLookup
	CostCapabilityLookup = func(int, string, constant.TaskPlatform) types.CostCapabilities {
		return completeCostCapabilities()
	}
	InvalidateCostCoverage(0, "")
	t.Cleanup(func() {
		CostCapabilityLookup = previousLookup
		InvalidateCostCoverage(0, "")
	})
}

func seedActiveAttemptRule(t *testing.T, mode types.CostMode, config types.CostRuleConfigV1) {
	t.Helper()
	normalized, err := NormalizeCostRuleConfig(mode, config)
	require.NoError(t, err)
	configJSON, err := common.Marshal(normalized)
	require.NoError(t, err)
	now := common.GetTimestamp()
	require.NoError(t, model.DB.Create(&model.ChannelModelCostRule{
		ChannelID: 7, BillableUpstreamModel: "vendor-model", Version: 1,
		Status: string(types.CostRuleActive), CostMode: string(mode), SchemaVersion: 1,
		ConfigJSON: string(configJSON), Source: "manual", EffectiveFrom: &now,
		CreatedAt: now, UpdatedAt: now,
	}).Error)
}

func normalizedTokenCostConfig(t *testing.T) types.CostRuleConfigV1 {
	t.Helper()
	config := validTokenCostConfig(types.CostTokenModeTotal, types.CostMeterUpstreamUsage)
	return normalizedCostAttemptConfig(t, types.CostModePerToken, config)
}

func normalizedCostAttemptConfig(t *testing.T, mode types.CostMode, config types.CostRuleConfigV1) types.CostRuleConfigV1 {
	t.Helper()
	normalized, err := NormalizeCostRuleConfig(mode, config)
	require.NoError(t, err)
	return normalized
}

func preparedAttemptInput() PrepareCostAttemptInput {
	quota := int64(500_000)
	return PrepareCostAttemptInput{
		RequestID: "request-cost-attempt", UserID: 11, TokenID: 22,
		UserGroup: "default", UsingGroup: "default", OriginModelName: "client-model",
		BillingSource: "wallet", FinalUserQuota: &quota, QuotaPerUnitSnapshot: "500000",
		ChannelID: 7, ChannelName: "supplier", ChannelType: constant.ChannelTypeOpenAI,
		PredictedUpstreamModel: "vendor-model", BillableUpstreamModel: "vendor-model",
		RequestPath: "/v1/chat/completions",
	}
}

func loadCostAttempt(t *testing.T, id int64) model.CostAccountingAttempt {
	t.Helper()
	var attempt model.CostAccountingAttempt
	require.NoError(t, model.DB.First(&attempt, id).Error)
	return attempt
}
