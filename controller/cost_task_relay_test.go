package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestPersistSubmittedTaskMarksOrphanWithoutChangingAcceptedCost(t *testing.T) {
	setupControllerTaskCostDB(t)
	const (
		channelID = 701001
		taskID    = "task-orphan-public"
		modelName = "provider-video-model"
	)
	require.NoError(t, model.DB.Create(&model.Channel{
		Id: channelID, Type: constant.ChannelTypeNewAPIVideo, Name: "task supplier", Key: "secret",
	}).Error)
	unitPrice := "0.2"
	config, err := service.NormalizeCostRuleConfig(types.CostModePerRequest, types.CostRuleConfigV1{
		Currency: "USD", BillingMultiplier: "1", PurchaseDiscountRatio: "1",
		RechargeExchangeRatio: "1", FeeRate: "0", CurrencyToUSDRate: "1",
		UnitPrice: &unitPrice, ChargeEvent: types.CostChargeSubmitAccepted,
	})
	require.NoError(t, err)
	configJSON, err := common.Marshal(config)
	require.NoError(t, err)
	now := common.GetTimestamp()
	require.NoError(t, model.DB.Create(&model.ChannelModelCostRule{
		ChannelID: channelID, BillableUpstreamModel: modelName, Version: 1,
		Status: string(types.CostRuleActive), CostMode: string(types.CostModePerRequest), SchemaVersion: 1,
		ConfigJSON: string(configJSON), Source: "manual", EffectiveFrom: &now,
		CreatedAt: now, UpdatedAt: now,
	}).Error)

	previousLookup := service.CostCapabilityLookup
	service.CostCapabilityLookup = func(int, string, constant.TaskPlatform) types.CostCapabilities {
		return types.CostCapabilities{
			CanResolveBillableModel: true,
			ChargeEvents:            []types.CostChargeEvent{types.CostChargeSubmitAccepted, types.CostChargeTaskSucceeded},
			MeterSources:            []types.CostMeterSource{types.CostMeterValidatedRequest, types.CostMeterUpstreamActual, types.CostMeterUpstreamUsage},
		}
	}
	service.InvalidateCostCoverage(channelID, modelName)
	t.Cleanup(func() {
		service.CostCapabilityLookup = previousLookup
		service.InvalidateCostCoverage(channelID, modelName)
	})
	handle, err := service.PrepareCostAttempt(t.Context(), service.PrepareCostAttemptInput{
		RequestID: "task-orphan-request", TaskID: common.GetPointer(taskID),
		UserID: 11, TokenID: 22, UserGroup: "default", UsingGroup: "default",
		OriginModelName: "client-video", BillingSource: service.BillingSourceWallet,
		QuotaPerUnitSnapshot: "500000", ChannelID: channelID, ChannelName: "task supplier",
		ChannelType: constant.ChannelTypeNewAPIVideo, PredictedUpstreamModel: modelName,
		BillableUpstreamModel: modelName, RequestPath: "/v1/video/generations",
		TaskPlatform: constant.TaskPlatform("task-test"),
	})
	require.NoError(t, err)
	require.NoError(t, service.AuthorizeCostDispatch(t.Context(), handle))
	require.NoError(t, service.SettleSyncCostAttempt(t.Context(), handle, types.CostMeter{}))
	require.NoError(t, service.MarkWinningCostAttempt(t.Context(), handle))

	forcedInsertErr := errors.New("forced task insert failure")
	callbackName := "test:fail_orphan_task_insert"
	require.NoError(t, model.DB.Callback().Create().Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Table == "tasks" {
			tx.AddError(forcedInsertErr)
		}
	}))
	t.Cleanup(func() { require.NoError(t, model.DB.Callback().Create().Remove(callbackName)) })

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/video/generations", nil)
	info := &relaycommon.RelayInfo{
		RequestId: "task-orphan-request", OriginModelName: "client-video",
		CostRequestID: handle.CostRequestID, CostAttempt: handle,
		PriceData: types.PriceData{Quota: 100},
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId: channelID, ChannelType: constant.ChannelTypeNewAPIVideo,
			UpstreamModelName: modelName,
		},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{PublicTaskID: taskID},
	}
	result := &relay.TaskSubmitResult{
		UpstreamTaskID: "upstream-orphan-id", TaskData: []byte(`{"id":"upstream-orphan-id"}`),
		Platform: constant.TaskPlatform("task-test"), Quota: 100,
	}

	err = persistSubmittedTask(c, info, result)

	require.ErrorIs(t, err, forcedInsertErr)
	var request model.CostAccountingRequest
	require.NoError(t, model.DB.First(&request, handle.CostRequestID).Error)
	require.NotNil(t, request.TaskID)
	assert.Equal(t, taskID, *request.TaskID)
	require.NotNil(t, request.UpstreamTaskID)
	assert.Equal(t, "upstream-orphan-id", *request.UpstreamTaskID)
	assert.Equal(t, "orphaned_task_insert_failed", request.FailureCode)
	var attempt model.CostAccountingAttempt
	require.NoError(t, model.DB.First(&attempt, handle.AttemptID).Error)
	assert.Equal(t, string(types.CostAttemptSettled), attempt.Status)
	require.NotNil(t, attempt.CostNanoUSD)
	assert.Equal(t, int64(200_000_000), *attempt.CostNanoUSD)
	var taskCount int64
	require.NoError(t, model.DB.Model(&model.Task{}).Count(&taskCount).Error)
	assert.Zero(t, taskCount)
}

func TestHandleTaskCostCoverageFailureExcludesChannelAndRetries(t *testing.T) {
	previousRetryTimes := common.RetryTimes
	common.RetryTimes = 1
	t.Cleanup(func() { common.RetryTimes = previousRetryTimes })
	retryParam := &service.RetryParam{Retry: common.GetPointer(0)}
	info := &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
	taskErr := service.TaskErrorWrapperLocal(
		&service.CostCoverageError{ChannelID: 73},
		"cost_coverage_unavailable",
		http.StatusServiceUnavailable,
	)

	safeErr, retry, handled := handleTaskCostCoverageFailure(retryParam, info, taskErr)

	assert.True(t, handled)
	assert.True(t, retry)
	require.NotNil(t, safeErr)
	assert.Equal(t, http.StatusServiceUnavailable, safeErr.StatusCode)
	assert.Equal(t, "get_channel_failed", safeErr.Code)
	_, excluded := retryParam.ExcludedChannelIDs[73]
	assert.True(t, excluded)
}

func TestHandleTaskCostCoverageFailureDoesNotBreakChannelAffinity(t *testing.T) {
	retryParam := &service.RetryParam{Retry: common.GetPointer(0)}
	info := &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{LockedChannel: &model.Channel{Id: 73}}}
	taskErr := service.TaskErrorWrapperLocal(
		&service.CostCoverageError{ChannelID: 73},
		"cost_coverage_unavailable",
		http.StatusServiceUnavailable,
	)

	_, retry, handled := handleTaskCostCoverageFailure(retryParam, info, taskErr)

	assert.True(t, handled)
	assert.False(t, retry)
}

func TestHandleTaskProfitEligibilityFailureDoesNotRetrySpecificChannel(t *testing.T) {
	previousRetryTimes := common.RetryTimes
	common.RetryTimes = 1
	t.Cleanup(func() { common.RetryTimes = previousRetryTimes })

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("specific_channel_id", "73")
	retryParam := &service.RetryParam{Ctx: c, Retry: common.GetPointer(0)}
	info := &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
	taskErr := service.TaskErrorWrapperLocal(
		&service.ProfitEligibilityError{ChannelID: 73, Reason: service.ProfitReasonMarginBelowThreshold},
		"cost_coverage_unavailable",
		http.StatusServiceUnavailable,
	)

	safeErr, retry, handled := handleTaskCostCoverageFailure(retryParam, info, taskErr)

	assert.True(t, handled)
	assert.False(t, retry)
	require.NotNil(t, safeErr)
	assert.Equal(t, http.StatusServiceUnavailable, safeErr.StatusCode)
	assert.Equal(t, "available channel is unavailable", safeErr.Message)
	_, excluded := retryParam.ExcludedChannelIDs[73]
	assert.True(t, excluded)
}

func TestCostCoverageRetryIsBlockedForPinnedChannels(t *testing.T) {
	specific, _ := gin.CreateTestContext(httptest.NewRecorder())
	specific.Set("specific_channel_id", "73")

	assert.False(t, canRetryCostCoverageFailure(specific, &relaycommon.RelayInfo{}))
	assert.False(t, canRetryCostCoverageFailure(nil, &relaycommon.RelayInfo{
		TaskRelayInfo: &relaycommon.TaskRelayInfo{LockedChannel: &model.Channel{Id: 73}},
	}))
	assert.True(t, canRetryCostCoverageFailure(nil, &relaycommon.RelayInfo{}))
}

func setupControllerTaskCostDB(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	previousDB := model.DB
	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	model.DB = db
	common.MemoryCacheEnabled = false
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		model.DB = previousDB
		common.MemoryCacheEnabled = previousMemoryCacheEnabled
		require.NoError(t, sqlDB.Close())
	})
	require.NoError(t, model.DB.AutoMigrate(
		&model.Channel{}, &model.ChannelModelCostRule{}, &model.CostAccountingRequest{},
		&model.CostAccountingAttempt{}, &model.CostAccountingAudit{}, &model.Task{},
	))
}
