package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelrouting"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/cost_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
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

func TestPrepareCostAttemptUsesSelectedRoutingCostVariant(t *testing.T) {
	prepareCostAttemptServiceDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.RouteTarget{}))
	require.NoError(t, model.DB.Exec("DELETE FROM route_targets").Error)

	rule480 := seedActiveAttemptRuleWithVariant(t, "480p", types.CostModePerRequest, normalizedPerRequestConfig(t, "0.1"))
	rule720 := seedActiveAttemptRuleWithVariant(t, "720p", types.CostModePerRequest, normalizedPerRequestConfig(t, "0.2"))
	configureProfitRecheckSettings(t, 0)

	target := model.RouteTarget{
		ID: 41, PolicyID: 42, ChannelID: 7, Name: "supplier", UpstreamModel: "vendor-model",
		CostVariantKey: "720p", TargetPriority: 100, Constraints: `{}`, Enabled: true,
	}
	require.NoError(t, model.DB.Create(&target).Error)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	common.SetContextKey(c, constant.ContextKeyRoutingCostVariant, "720p")
	info := profitRecheckRelayInfo(target.ID, target.PolicyID)
	setProfitRecheckRevenue(t)
	require.NoError(t, RecheckSelectedChannelProfit(c, info))
	require.NotNil(t, info.CostProfitRecheckSnapshot)
	assert.Equal(t, rule720.ID, info.CostProfitRecheckSnapshot.RuleID)
	assert.Equal(t, "720p", info.CostProfitRecheckSnapshot.CostVariantKey)
	require.NotNil(t, info.CostProfitRecheckSnapshot.RouteTarget)
	assert.Equal(t, target.ID, info.CostProfitRecheckSnapshot.RouteTarget.TargetID)
	assert.Equal(t, "720p", info.CostProfitRecheckSnapshot.RouteTarget.CostVariantKey)
	assert.Equal(t, target.TargetPriority, info.CostProfitRecheckSnapshot.RouteTarget.Priority)

	input := preparedAttemptInput()
	input.CostProfitRecheckSnapshot = info.CostProfitRecheckSnapshot
	handle, err := PrepareCostAttempt(context.Background(), input)
	require.NoError(t, err)
	attempt := loadCostAttempt(t, handle.AttemptID)
	assert.Equal(t, rule720.ID, attempt.RuleID)
	assert.NotEqual(t, rule480.ID, attempt.RuleID)
	assert.Equal(t, "720p", attempt.CostVariantKey)
}

func TestRecheckSelectedChannelProfitDoesNotFallbackAcrossCostVariants(t *testing.T) {
	prepareCostAttemptServiceDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.RouteTarget{}))
	require.NoError(t, model.DB.Exec("DELETE FROM route_targets").Error)
	seedActiveAttemptRule(t, types.CostModePerRequest, normalizedPerRequestConfig(t, "0.1"))
	configureProfitRecheckSettings(t, 0)

	target := model.RouteTarget{
		ID: 41, PolicyID: 42, ChannelID: 7, Name: "supplier", UpstreamModel: "vendor-model",
		CostVariantKey: "720p", TargetPriority: 100, Constraints: `{}`, Enabled: true,
	}
	require.NoError(t, model.DB.Create(&target).Error)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	common.SetContextKey(c, constant.ContextKeyRoutingCostVariant, "720p")
	info := profitRecheckRelayInfo(target.ID, target.PolicyID)
	setProfitRecheckRevenue(t)
	err := RecheckSelectedChannelProfit(c, info)

	require.ErrorIs(t, err, ErrProfitEligibility)
	var eligibilityErr *ProfitEligibilityError
	require.ErrorAs(t, err, &eligibilityErr)
	assert.Equal(t, ProfitReasonCostRuleMissing, eligibilityErr.Reason)
	assert.Nil(t, info.CostProfitRecheckSnapshot)
}

func TestPrepareCostAttemptRejectsChangedProfitRecheckTarget(t *testing.T) {
	prepareCostAttemptServiceDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.RouteTarget{}))
	require.NoError(t, model.DB.Exec("DELETE FROM route_targets").Error)
	seedActiveAttemptRule(t, types.CostModeFree, types.CostRuleConfigV1{ZeroCostReason: "contract"})
	configureProfitRecheckSettings(t, 0)

	threshold := 100
	target := model.RouteTarget{
		ID: 41, PolicyID: 42, ChannelID: 7, Name: "supplier", UpstreamModel: "vendor-model",
		CostVariantKey: string(types.DefaultCostVariantKey), TargetPriority: 100,
		MinimumExpectedMarginBPS: &threshold, Constraints: `{}`, Enabled: true,
	}
	require.NoError(t, model.DB.Create(&target).Error)
	info := profitRecheckRelayInfo(target.ID, target.PolicyID)
	setProfitRecheckRevenue(t)
	require.NoError(t, RecheckSelectedChannelProfit(nil, info))
	require.NotNil(t, info.CostProfitRecheckSnapshot)

	changedThreshold := 200
	require.NoError(t, model.DB.Model(&model.RouteTarget{}).Where("id = ?", target.ID).
		Update("minimum_expected_margin_bps", &changedThreshold).Error)
	input := preparedAttemptInput()
	input.CostProfitRecheckSnapshot = info.CostProfitRecheckSnapshot

	_, err := PrepareCostAttempt(context.Background(), input)

	require.ErrorIs(t, err, ErrProfitEligibility)
	var coverageErr *CostCoverageError
	require.ErrorAs(t, err, &coverageErr)
	assert.Equal(t, input.ChannelID, coverageErr.ChannelID)
	assertNoPreparedCostRequest(t, input.RequestID)
}

func TestPrepareCostAttemptRejectsChangedProfitRecheckGlobalThreshold(t *testing.T) {
	prepareCostAttemptServiceDB(t)
	seedActiveAttemptRule(t, types.CostModeFree, types.CostRuleConfigV1{ZeroCostReason: "contract"})
	configureProfitRecheckSettings(t, 0)

	info := profitRecheckRelayInfo(0, 0)
	setProfitRecheckRevenue(t)
	require.NoError(t, RecheckSelectedChannelProfit(nil, info))
	require.NotNil(t, info.CostProfitRecheckSnapshot)

	costConfig := config.GlobalConfig.Get(cost_setting.ConfigName)
	require.NotNil(t, costConfig)
	require.NoError(t, config.UpdateConfigFromMap(costConfig, map[string]string{
		cost_setting.KeyMode:                     string(types.CostAccountingStrict),
		cost_setting.KeyMinimumExpectedMarginBPS: "200",
	}))
	cost_setting.UpdateAndSync()
	input := preparedAttemptInput()
	input.CostProfitRecheckSnapshot = info.CostProfitRecheckSnapshot

	_, err := PrepareCostAttempt(context.Background(), input)

	require.ErrorIs(t, err, ErrProfitEligibility)
	var coverageErr *CostCoverageError
	require.ErrorAs(t, err, &coverageErr)
	assert.Equal(t, input.ChannelID, coverageErr.ChannelID)
	assertNoPreparedCostRequest(t, input.RequestID)
}

func TestPrepareCostAttemptMapsSnapshotLockConflictToProfitEligibility(t *testing.T) {
	prepareCostAttemptServiceDB(t)
	seedActiveAttemptRule(t, types.CostModeFree, types.CostRuleConfigV1{ZeroCostReason: "contract"})
	configureProfitRecheckSettings(t, 0)

	info := profitRecheckRelayInfo(0, 0)
	setProfitRecheckRevenue(t)
	require.NoError(t, RecheckSelectedChannelProfit(nil, info))
	require.NotNil(t, info.CostProfitRecheckSnapshot)
	callbackName := "profit_recheck_snapshot_sqlite_locked"
	require.NoError(t, model.DB.Callback().Query().Before("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table == "channel_model_cost_rules" {
			tx.AddError(errors.New("database is locked"))
		}
	}))
	t.Cleanup(func() {
		require.NoError(t, model.DB.Callback().Query().Remove(callbackName))
	})
	input := preparedAttemptInput()
	input.CostProfitRecheckSnapshot = info.CostProfitRecheckSnapshot

	_, err := PrepareCostAttempt(context.Background(), input)

	require.ErrorIs(t, err, ErrProfitEligibility)
	var coverageErr *CostCoverageError
	require.ErrorAs(t, err, &coverageErr)
	assert.Equal(t, input.ChannelID, coverageErr.ChannelID)
	assertNoPreparedCostRequest(t, input.RequestID)
}

func TestPrepareCostAttemptIgnoresValidatedDurationCandidateForUpstreamActualRule(t *testing.T) {
	prepareCostAttemptServiceDB(t)
	config := validDurationCostConfig(types.CostMeterUpstreamActual)
	config.ChargeEvent = types.CostChargeTaskSucceeded
	seedActiveAttemptRule(t, types.CostModePerDuration, config)
	duration := "6"
	input := preparedAttemptInput()
	input.TaskPlatform = constant.TaskPlatform("task-test")
	input.RequestPath = "/v1/video/generations"
	input.RequestMeter = &types.CostMeter{
		Source: types.CostMeterValidatedRequest, DurationSeconds: &duration,
	}

	handle, err := PrepareCostAttempt(context.Background(), input)

	require.NoError(t, err)
	attempt := loadCostAttempt(t, handle.AttemptID)
	assert.JSONEq(t, `{}`, attempt.RequestMeterJSON)
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
	InvalidateCostCoverage(0, "", "")
	t.Cleanup(func() {
		CostCapabilityLookup = previousLookup
		InvalidateCostCoverage(0, "", "")
	})
}

func seedActiveAttemptRule(t *testing.T, mode types.CostMode, config types.CostRuleConfigV1) {
	seedActiveAttemptRuleWithVariant(t, string(types.DefaultCostVariantKey), mode, config)
}

func seedActiveAttemptRuleWithVariant(t *testing.T, costVariantKey string, mode types.CostMode, config types.CostRuleConfigV1) *model.ChannelModelCostRule {
	t.Helper()
	normalized, err := NormalizeCostRuleConfig(mode, config)
	require.NoError(t, err)
	configJSON, err := common.Marshal(normalized)
	require.NoError(t, err)
	now := common.GetTimestamp()
	rule := &model.ChannelModelCostRule{
		ChannelID: 7, BillableUpstreamModel: "vendor-model", CostVariantKey: costVariantKey, Version: 1,
		Status: string(types.CostRuleActive), CostMode: string(mode), SchemaVersion: 1,
		ConfigJSON: string(configJSON), Source: "manual", EffectiveFrom: &now,
		CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, model.DB.Create(rule).Error)
	return rule
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

func configureProfitRecheckSettings(t *testing.T, minimumExpectedMarginBPS int) {
	t.Helper()
	costConfig := config.GlobalConfig.Get(cost_setting.ConfigName)
	require.NotNil(t, costConfig)
	previous := cost_setting.Runtime()
	t.Cleanup(func() {
		require.NoError(t, config.UpdateConfigFromMap(costConfig, map[string]string{
			cost_setting.KeyMode:                     string(previous.Mode),
			cost_setting.KeyMinimumExpectedMarginBPS: strconv.Itoa(previous.MinimumExpectedMarginBPS),
		}))
		cost_setting.UpdateAndSync()
	})
	require.NoError(t, config.UpdateConfigFromMap(costConfig, map[string]string{
		cost_setting.KeyMode:                     string(types.CostAccountingStrict),
		cost_setting.KeyMinimumExpectedMarginBPS: strconv.Itoa(minimumExpectedMarginBPS),
	}))
	cost_setting.UpdateAndSync()
}

func profitRecheckRelayInfo(targetID, policyID int) *relaycommon.RelayInfo {
	info := &relaycommon.RelayInfo{
		OriginModelName:        "client-model",
		RequestURLPath:         "/v1/chat/completions",
		BillableUpstreamModel:  "vendor-model",
		PredictedUpstreamModel: "vendor-model",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId: 7, ChannelType: constant.ChannelTypeOpenAI,
		},
	}
	if targetID > 0 {
		info.Routing = &modelrouting.Audit{PolicyID: policyID, TargetID: targetID}
	}
	return info
}

func setProfitRecheckRevenue(t *testing.T) {
	t.Helper()
	previous := RevenuePreviewHookForTest()
	SetRoutingRevenuePreview(func(context.Context, RoutingRevenuePreviewInput) (int64, string, error) {
		return 1_000_000, "500000", nil
	})
	t.Cleanup(func() { SetRoutingRevenuePreview(previous) })
}

func assertNoPreparedCostRequest(t *testing.T, requestID string) {
	t.Helper()
	var requestCount int64
	require.NoError(t, model.DB.Model(&model.CostAccountingRequest{}).Where("request_id = ?", requestID).Count(&requestCount).Error)
	assert.Zero(t, requestCount)
	var attemptCount int64
	require.NoError(t, model.DB.Model(&model.CostAccountingAttempt{}).Where("cost_request_id IN (SELECT id FROM cost_accounting_requests WHERE request_id = ?)", requestID).Count(&attemptCount).Error)
	assert.Zero(t, attemptCount)
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
