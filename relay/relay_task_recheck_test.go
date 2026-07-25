package relay

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelrouting"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestProfitRecheckBlocksDispatchAfterRuleChange asserts the pre-dispatch recheck
// blocks the upstream call when the active cost rule (resolved against the final
// billable model after ConfirmTaskCostIdentity) predicts a margin below the global
// threshold. The candidate stage would have priced the predicted model; the recheck
// prices the authoritative billable model and must fail closed.
//
// Invariants proven: no CostAccountingAttempt row is created, no upstream DoRequest
// happens, and the returned error wraps *service.CostCoverageError so the controller
// excludes the channel and retries with another candidate.
func TestProfitRecheckBlocksDispatchAfterRuleChange(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service.InitHttpClient()
	withCostAccountingMode(t, types.CostAccountingStrict)
	configureNewAPIVideoFixedPricing(t, "client-video")
	setupTaskCostSubmitDB(t)

	const (
		channelID = 710001
		requestID = "profit-recheck-block"
		modelName = "seedance-720p-token"
	)
	require.NoError(t, model.DB.Create(&model.Channel{
		Id: channelID, Type: constant.ChannelTypeNewAPIVideo, Name: "supplier", Key: "secret",
	}).Error)
	// A per-request cost of $100 far exceeds the preview revenue (configureNewAPIVideoFixedPricing
	// sets a tiny fixed price), so the predicted margin is deeply negative.
	unitPrice := "100"
	ruleConfig, err := service.NormalizeCostRuleConfig(types.CostModePerRequest, types.CostRuleConfigV1{
		Currency: "USD", BillingMultiplier: "1", PurchaseDiscountRatio: "1",
		RechargeExchangeRatio: "1", FeeRate: "0", CurrencyToUSDRate: "1",
		UnitPrice: &unitPrice, ChargeEvent: types.CostChargeSubmitAccepted,
	})
	require.NoError(t, err)
	configJSON, err := common.Marshal(ruleConfig)
	require.NoError(t, err)
	now := common.GetTimestamp()
	require.NoError(t, model.DB.Create(&model.ChannelModelCostRule{
		ChannelID: channelID, BillableUpstreamModel: modelName, Version: 1,
		Status: string(types.CostRuleActive), CostMode: string(types.CostModePerRequest), SchemaVersion: 1,
		ConfigJSON: string(configJSON), Source: "manual", EffectiveFrom: &now,
		CreatedAt: now, UpdatedAt: now,
	}).Error)
	previousLookup := service.CostCapabilityLookup
	service.CostCapabilityLookup = CostCapabilitiesForRoute
	service.InvalidateCostCoverage(channelID, modelName)
	// Inject a small positive revenue so the predictor has a real figure to compare
	// against the $100 cost (otherwise revenue_unknown would also exclude, but for the
	// wrong reason and the recheck would not exercise the margin comparison).
	previousRevenueHook := service.RevenuePreviewHookForTest()
	service.SetRoutingRevenuePreview(func(_ context.Context, _ service.RoutingRevenuePreviewInput) (int64, string, error) {
		return 1_000, "500000", nil
	})
	t.Cleanup(func() {
		service.CostCapabilityLookup = previousLookup
		service.InvalidateCostCoverage(channelID, modelName)
		service.SetRoutingRevenuePreview(previousRevenueHook)
	})

	var upstreamCalls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&upstreamCalls, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"upstream-task","status":"queued"}`))
	}))
	t.Cleanup(server.Close)

	c, info := newNewAPIVideoRelayContext(`{"model":"client-video","prompt":"text","seconds":"5"}`, server.URL)
	c.Set(string(constant.ContextKeyChannelId), channelID)
	c.Set(string(constant.ContextKeyChannelName), "supplier")
	info.RequestId = requestID
	info.RequestURLPath = "/v1/video/generations"
	info.UserId = 11
	info.TokenId = 22

	result, taskErr := RelayTaskSubmit(c, info)

	require.Nil(t, result)
	require.NotNil(t, taskErr)
	assert.Equal(t, http.StatusServiceUnavailable, taskErr.StatusCode)
	// The error must unwrap to *service.CostCoverageError so handleTaskCostCoverageFailure
	// excludes the channel and the controller retries with another candidate.
	var coverageErr *service.CostCoverageError
	require.ErrorAs(t, taskErr.Error, &coverageErr)
	assert.Equal(t, channelID, coverageErr.ChannelID)
	// No upstream side-effect and no cost attempt must be left behind.
	assert.Zero(t, atomic.LoadInt32(&upstreamCalls), "upstream DoRequest must not run when the recheck fails")
	var attemptCount int64
	require.NoError(t, model.DB.Model(&model.CostAccountingAttempt{}).Where("cost_request_id IN (SELECT id FROM cost_accounting_requests WHERE request_id = ?)", requestID).Count(&attemptCount).Error)
	assert.Zero(t, attemptCount, "no cost attempt must be created when the recheck fails")
}

// TestProfitRecheckPassesEligibleChannel asserts the recheck is a no-op for a channel
// whose predicted margin meets the threshold, so the normal cost-attempt + dispatch
// flow still runs and the upstream is called exactly once.
func TestProfitRecheckPassesEligibleChannel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service.InitHttpClient()
	withCostAccountingMode(t, types.CostAccountingStrict)
	configureNewAPIVideoFixedPricing(t, "client-video")
	setupTaskCostSubmitDB(t)

	const (
		channelID = 710002
		requestID = "profit-recheck-pass"
		modelName = "seedance-720p-token"
	)
	// A free cost rule yields 100% margin against any positive revenue. seedTaskCostSubmitRule
	// creates the channel too, so do not create it separately.
	freeConfig := types.CostRuleConfigV1{ZeroCostReason: "supplier contract"}
	seedTaskCostSubmitRule(t, channelID, modelName, types.CostModeFree, freeConfig)
	previousRevenueHook := service.RevenuePreviewHookForTest()
	service.SetRoutingRevenuePreview(func(_ context.Context, _ service.RoutingRevenuePreviewInput) (int64, string, error) {
		return 1_000_000, "500000", nil
	})
	t.Cleanup(func() { service.SetRoutingRevenuePreview(previousRevenueHook) })

	var upstreamCalls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&upstreamCalls, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"upstream-task","status":"queued"}`))
	}))
	t.Cleanup(server.Close)

	c, info := newNewAPIVideoRelayContext(`{"model":"client-video","prompt":"text","seconds":"5"}`, server.URL)
	c.Set(string(constant.ContextKeyChannelId), channelID)
	c.Set(string(constant.ContextKeyChannelName), "supplier")
	info.RequestId = requestID
	info.RequestURLPath = "/v1/video/generations"
	info.UserId = 11
	info.TokenId = 22

	result, taskErr := RelayTaskSubmit(c, info)

	require.Nil(t, taskErr)
	require.NotNil(t, result)
	assert.Equal(t, int32(1), atomic.LoadInt32(&upstreamCalls), "upstream must be called when the recheck passes")
}

func TestProfitEligibilityErrorSupportsCoverageRetryAndSentinelMatching(t *testing.T) {
	err := &service.ProfitEligibilityError{
		ChannelID: 710003,
		Reason:    service.ProfitReasonMarginBelowThreshold,
	}

	assert.ErrorIs(t, err, service.ErrProfitEligibility)
	var coverageErr *service.CostCoverageError
	require.ErrorAs(t, err, &coverageErr)
	assert.Equal(t, 710003, coverageErr.ChannelID)
	assert.Equal(t, "profit eligibility recheck failed", err.Error())
}

func TestProfitRecheckFailsClosedWhenCurrentTargetModelChanges(t *testing.T) {
	gin.SetMode(gin.TestMode)
	withCostAccountingMode(t, types.CostAccountingStrict)
	setupTaskCostSubmitDB(t)

	const (
		channelID  = 710004
		policyID   = 91
		targetID   = 92
		finalModel = "final-provider-model"
	)
	seedTaskCostSubmitRule(t, channelID, finalModel, types.CostModeFree, types.CostRuleConfigV1{ZeroCostReason: "contract"})
	require.NoError(t, model.DB.AutoMigrate(&model.RouteTarget{}))
	require.NoError(t, model.DB.Create(&model.RouteTarget{
		ID: targetID, PolicyID: policyID, ChannelID: channelID, Name: "old target", UpstreamModel: "old-provider-model",
		TargetPriority: 100, Constraints: `{}`, Enabled: true,
	}).Error)

	c, info := newNewAPIVideoRelayContext(`{"model":"client-video","prompt":"text","seconds":"5"}`, "https://provider.example")
	info.ChannelMeta = &relaycommon.ChannelMeta{
		ChannelId: channelID,
		Routing: &modelrouting.Audit{
			PolicyID: policyID,
			TargetID: targetID,
			Facts:    modelrouting.Facts{OutputResolution: "720p", DurationSeconds: 5},
		},
	}
	info.BillableUpstreamModel = finalModel

	err := service.RecheckSelectedChannelProfit(c, info)

	require.ErrorIs(t, err, service.ErrProfitEligibility)
	var coverageErr *service.CostCoverageError
	require.ErrorAs(t, err, &coverageErr)
	assert.Equal(t, channelID, coverageErr.ChannelID)
}

func TestProfitRecheckFailsClosedWhenOutputFactsAreMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	withCostAccountingMode(t, types.CostAccountingStrict)
	setupTaskCostSubmitDB(t)

	const (
		channelID = 710005
		modelName = "provider-model"
	)
	seedTaskCostSubmitRule(t, channelID, modelName, types.CostModeFree, types.CostRuleConfigV1{ZeroCostReason: "contract"})
	c, info := newNewAPIVideoRelayContext(`{"model":"client-video","prompt":"text"}`, "https://provider.example")
	common.SetContextKey(c, constant.ContextKeyRoutingFacts, modelrouting.Facts{})
	info.ChannelMeta = &relaycommon.ChannelMeta{ChannelId: channelID}
	info.BillableUpstreamModel = modelName
	info.RequestURLPath = "/v1/video/generations"

	err := service.RecheckSelectedChannelProfit(c, info)

	require.ErrorIs(t, err, service.ErrProfitEligibility)
	var coverageErr *service.CostCoverageError
	require.ErrorAs(t, err, &coverageErr)
	assert.Equal(t, channelID, coverageErr.ChannelID)
}
