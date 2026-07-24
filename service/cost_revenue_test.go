package service

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecognizeBilledRevenueUsesFrozenQuotaSnapshotForWalletAndSubscription(t *testing.T) {
	for _, billingSource := range []string{BillingSourceWallet, BillingSourceSubscription} {
		t.Run(billingSource, func(t *testing.T) {
			prepareCostRevenueDB(t)
			request := seedPendingCostRevenue(t, "revenue-"+billingSource, billingSource, "250000")
			info := &relaycommon.RelayInfo{CostRequestID: request.ID}

			require.NoError(t, RecognizeBilledRevenue(context.Background(), info, 500_000))
			request = loadCostRevenueRequest(t, request.ID)
			assert.Equal(t, string(types.CostRevenueSettled), request.RevenueStatus)
			require.NotNil(t, request.BilledRevenueEquivalentNanoUSD)
			assert.Equal(t, int64(2_000_000_000), *request.BilledRevenueEquivalentNanoUSD)
			assert.Equal(t, billingSource, request.BillingSource)
		})
	}
}

func TestRecognizeBilledRevenueConfirmsZeroWithNullMargin(t *testing.T) {
	prepareCostRevenueDB(t)
	request := seedPendingCostRevenue(t, "revenue-zero", BillingSourceWallet, "500000")

	require.NoError(t, RecognizeBilledRevenue(context.Background(), &relaycommon.RelayInfo{CostRequestID: request.ID}, 0))
	request = loadCostRevenueRequest(t, request.ID)
	assert.Equal(t, string(types.CostRevenueConfirmedZero), request.RevenueStatus)
	require.NotNil(t, request.BilledRevenueEquivalentNanoUSD)
	assert.Zero(t, *request.BilledRevenueEquivalentNanoUSD)
	assert.Nil(t, request.GrossMarginPPM)
}

func TestRecognizeBilledRevenueIsIdempotentForSameFrozenValues(t *testing.T) {
	prepareCostRevenueDB(t)
	request := seedPendingCostRevenue(t, "revenue-idempotent", BillingSourceWallet, "500000")
	info := &relaycommon.RelayInfo{CostRequestID: request.ID}

	require.NoError(t, RecognizeBilledRevenue(context.Background(), info, 500_000))
	require.NoError(t, RecognizeBilledRevenue(context.Background(), info, 500_000))

	request = loadCostRevenueRequest(t, request.ID)
	assert.Equal(t, string(types.CostRevenueSettled), request.RevenueStatus)
	require.NotNil(t, request.BilledRevenueEquivalentNanoUSD)
	assert.Equal(t, int64(1_000_000_000), *request.BilledRevenueEquivalentNanoUSD)
}

func TestRecognizeBilledRevenueWaitsForCostAndAllowsNegativeProfit(t *testing.T) {
	prepareCostRevenueDB(t)
	request := seedPendingCostRevenue(t, "revenue-negative-profit", BillingSourceWallet, "500000")
	attempt := model.CostAccountingAttempt{
		CostRequestID: request.ID, AttemptNo: 1, Status: string(types.CostAttemptAwaitingMeter),
		BillableRequestCount: 1, PreparedAt: 100, CreatedAt: 100, UpdatedAt: 100,
	}
	require.NoError(t, model.DB.Create(&attempt).Error)

	require.NoError(t, RecognizeBilledRevenue(context.Background(), &relaycommon.RelayInfo{CostRequestID: request.ID}, 100_000))
	request = loadCostRevenueRequest(t, request.ID)
	assert.Equal(t, string(types.CostProfitIncompleteCost), request.ProfitStatus)
	assert.Nil(t, request.BilledGrossProfitNanoUSD)

	cost := int64(300_000_000)
	require.NoError(t, model.SettleCostAttempt(model.SettleCostAttemptInput{
		AttemptID: attempt.ID, From: types.CostAttemptAwaitingMeter, To: types.CostAttemptSettled,
		OriginalCost: "0.3", CostNanoUSD: &cost, SettledAt: 200,
	}))
	request = loadCostRevenueRequest(t, request.ID)
	assert.Equal(t, string(types.CostProfitComplete), request.ProfitStatus)
	require.NotNil(t, request.BilledGrossProfitNanoUSD)
	assert.Equal(t, int64(-100_000_000), *request.BilledGrossProfitNanoUSD)
	require.NotNil(t, request.GrossMarginPPM)
	assert.Equal(t, int64(-500_000), *request.GrossMarginPPM)
}

func TestCostRevenueSettleBillingMarksRevenueFailedAndReturnsOriginalBillingError(t *testing.T) {
	prepareCostRevenueDB(t)
	request := seedPendingCostRevenue(t, "revenue-billing-failed", BillingSourceWallet, "500000")
	billingErr := errors.New("wallet settlement failed")
	billing := &costRevenueBillingStub{preConsumed: 100, settleErr: billingErr}
	info := &relaycommon.RelayInfo{CostRequestID: request.ID, Billing: billing}
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())

	err := SettleBilling(ctx, info, 200)
	require.ErrorIs(t, err, billingErr)
	request = loadCostRevenueRequest(t, request.ID)
	assert.Equal(t, string(types.CostRevenueFailed), request.RevenueStatus)
	assert.Equal(t, "user_billing_settlement_failed", request.FailureCode)
}

func TestCostRevenueSettleBillingFallbackMarksRevenueFailedAndReturnsOriginalBillingError(t *testing.T) {
	prepareCostRevenueDB(t)
	request := seedPendingCostRevenue(t, "revenue-fallback-failed", BillingSourceSubscription, "500000")
	info := &relaycommon.RelayInfo{
		CostRequestID: request.ID, BillingSource: BillingSourceSubscription,
		FinalPreConsumedQuota: 100,
	}
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())

	err := SettleBilling(ctx, info, 200)
	require.EqualError(t, err, "subscription id is missing")
	request = loadCostRevenueRequest(t, request.ID)
	assert.Equal(t, string(types.CostRevenueFailed), request.RevenueStatus)
	assert.Equal(t, "user_billing_settlement_failed", request.FailureCode)
}

func TestCostRevenueSettleBillingFallbackRecognizesRevenueWhenNoQuotaAdjustmentIsNeeded(t *testing.T) {
	prepareCostRevenueDB(t)
	request := seedPendingCostRevenue(t, "revenue-fallback-zero-delta", BillingSourceWallet, "500000")
	info := &relaycommon.RelayInfo{CostRequestID: request.ID, FinalPreConsumedQuota: 200}
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())

	require.NoError(t, SettleBilling(ctx, info, 200))
	request = loadCostRevenueRequest(t, request.ID)
	assert.Equal(t, string(types.CostRevenueSettled), request.RevenueStatus)
	require.NotNil(t, request.BilledRevenueEquivalentNanoUSD)
	assert.Equal(t, int64(400_000), *request.BilledRevenueEquivalentNanoUSD)
}

func TestCostRevenueSettleBillingKeepsSuccessWhenRevenuePersistenceFails(t *testing.T) {
	prepareCostRevenueDB(t)
	billing := &costRevenueBillingStub{preConsumed: 100}
	info := &relaycommon.RelayInfo{CostRequestID: 999999, Billing: billing}
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())

	require.NoError(t, SettleBilling(ctx, info, 200))
	assert.Equal(t, 200, billing.settledQuota)
}

func TestCostRevenueSettleBillingMarksRecognitionFailureWithoutChangingBillingSuccess(t *testing.T) {
	prepareCostRevenueDB(t)
	request := seedPendingCostRevenue(t, "revenue-recognition-failed", BillingSourceWallet, "invalid-snapshot")
	billing := &costRevenueBillingStub{preConsumed: 100}
	info := &relaycommon.RelayInfo{CostRequestID: request.ID, Billing: billing}
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())

	require.NoError(t, SettleBilling(ctx, info, 200))
	assert.Equal(t, 200, billing.settledQuota)
	request = loadCostRevenueRequest(t, request.ID)
	assert.Equal(t, string(types.CostRevenueFailed), request.RevenueStatus)
	assert.Equal(t, "revenue_recognition_failed", request.FailureCode)
}

func TestAttachCostAccountingAdminInfoAddsOnlyLedgerReference(t *testing.T) {
	other := map[string]interface{}{
		"model_price": 0.2,
		"admin_info":  map[string]interface{}{"use_channel": []string{"7"}},
	}
	attachCostAccountingAdminInfo(&relaycommon.RelayInfo{CostRequestID: 42}, other)

	adminInfo, ok := other["admin_info"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, int64(42), adminInfo["cost_accounting_request_id"])
	assert.Equal(t, []string{"7"}, adminInfo["use_channel"])
	assert.Len(t, adminInfo, 2)
	assert.Equal(t, 0.2, other["model_price"])
}

func prepareCostRevenueDB(t *testing.T) {
	t.Helper()
	require.NoError(t, model.DB.AutoMigrate(&model.CostAccountingRequest{}, &model.CostAccountingAttempt{}))
	require.NoError(t, model.DB.Exec("DELETE FROM cost_accounting_attempts").Error)
	require.NoError(t, model.DB.Exec("DELETE FROM cost_accounting_requests").Error)
}

func seedPendingCostRevenue(t *testing.T, requestID, billingSource, quotaSnapshot string) model.CostAccountingRequest {
	t.Helper()
	now := int64(100)
	request := model.CostAccountingRequest{
		RequestID: requestID, BillingSource: billingSource, QuotaPerUnitSnapshot: quotaSnapshot,
		RevenueStatus: string(types.CostRevenuePending), ProfitStatus: string(types.CostProfitIncompleteRevenue),
		RequestedAt: now, CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, model.DB.Create(&request).Error)
	return request
}

func loadCostRevenueRequest(t *testing.T, id int64) model.CostAccountingRequest {
	t.Helper()
	var request model.CostAccountingRequest
	require.NoError(t, model.DB.First(&request, id).Error)
	return request
}

type costRevenueBillingStub struct {
	preConsumed  int
	settledQuota int
	settleErr    error
}

func (s *costRevenueBillingStub) Settle(actualQuota int) error {
	s.settledQuota = actualQuota
	return s.settleErr
}

func (s *costRevenueBillingStub) Refund(*gin.Context) {}

func (s *costRevenueBillingStub) NeedsRefund() bool { return false }

func (s *costRevenueBillingStub) GetPreConsumedQuota() int { return s.preConsumed }

func (s *costRevenueBillingStub) Reserve(int) error { return nil }

var _ relaycommon.BillingSettler = (*costRevenueBillingStub)(nil)
