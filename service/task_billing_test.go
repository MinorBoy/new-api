package service

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestMain(m *testing.M) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		panic("failed to open test db: " + err.Error())
	}
	sqlDB, err := db.DB()
	if err != nil {
		panic("failed to get sql.DB: " + err.Error())
	}
	sqlDB.SetMaxOpenConns(1)

	model.DB = db
	model.LOG_DB = db

	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	common.BatchUpdateEnabled = false
	common.LogConsumeEnabled = true

	if err := db.AutoMigrate(
		&model.Task{},
		&model.User{},
		&model.Token{},
		&model.Log{},
		&model.Channel{},
		&model.TopUp{},
		&model.UserSubscription{},
		&model.SystemTask{},
		&model.SystemTaskLock{},
		&model.QuotaData{},
	); err != nil {
		panic("failed to migrate: " + err.Error())
	}

	os.Exit(m.Run())
}

// ---------------------------------------------------------------------------
// Seed helpers
// ---------------------------------------------------------------------------

func truncate(t *testing.T) {
	t.Helper()
	resetQuotaDataCache()
	t.Cleanup(func() {
		model.DB.Exec("DELETE FROM tasks")
		model.DB.Exec("DELETE FROM users")
		model.DB.Exec("DELETE FROM tokens")
		model.DB.Exec("DELETE FROM logs")
		model.DB.Exec("DELETE FROM channels")
		model.DB.Exec("DELETE FROM top_ups")
		model.DB.Exec("DELETE FROM user_subscriptions")
		model.DB.Exec("DELETE FROM system_task_locks")
		model.DB.Exec("DELETE FROM system_tasks")
		model.DB.Exec("DELETE FROM quota_data")
		resetQuotaDataCache()
	})
}

func seedUser(t *testing.T, id int, quota int) {
	seedUserWithUsage(t, id, quota, 0, 0)
}

func seedUserWithUsage(t *testing.T, id, quota, usedQuota, requestCount int) {
	t.Helper()
	user := &model.User{
		Id:           id,
		Username:     "test_user",
		Quota:        quota,
		UsedQuota:    usedQuota,
		RequestCount: requestCount,
		Status:       common.UserStatusEnabled,
	}
	require.NoError(t, model.DB.Create(user).Error)
}

func seedToken(t *testing.T, id int, userId int, key string, remainQuota int) {
	t.Helper()
	token := &model.Token{
		Id:          id,
		UserId:      userId,
		Key:         key,
		Name:        "test_token",
		Status:      common.TokenStatusEnabled,
		RemainQuota: remainQuota,
		UsedQuota:   0,
	}
	require.NoError(t, model.DB.Create(token).Error)
}

func seedSubscription(t *testing.T, id int, userId int, amountTotal int64, amountUsed int64) {
	t.Helper()
	sub := &model.UserSubscription{
		Id:          id,
		UserId:      userId,
		AmountTotal: amountTotal,
		AmountUsed:  amountUsed,
		Status:      "active",
		StartTime:   time.Now().Unix(),
		EndTime:     time.Now().Add(30 * 24 * time.Hour).Unix(),
	}
	require.NoError(t, model.DB.Create(sub).Error)
}

func seedChannel(t *testing.T, id int) {
	seedChannelWithUsage(t, id, 0)
}

func seedChannelWithUsage(t *testing.T, id int, usedQuota int64) {
	t.Helper()
	ch := &model.Channel{
		Id:        id,
		Name:      "test_channel",
		Key:       "sk-test",
		Status:    common.ChannelStatusEnabled,
		UsedQuota: usedQuota,
	}
	require.NoError(t, model.DB.Create(ch).Error)
}

func makeTask(userId, channelId, quota, tokenId int, billingSource string, subscriptionId int) *model.Task {
	return &model.Task{
		TaskID:     "task_" + time.Now().Format("150405.000"),
		UserId:     userId,
		ChannelId:  channelId,
		Quota:      quota,
		Status:     model.TaskStatus(model.TaskStatusInProgress),
		Group:      "default",
		Data:       json.RawMessage(`{}`),
		SubmitTime: 1_700_001_234,
		CreatedAt:  time.Now().Unix(),
		UpdatedAt:  time.Now().Unix(),
		Properties: model.Properties{
			OriginModelName: "test-model",
		},
		PrivateData: model.TaskPrivateData{
			BillingSource:  billingSource,
			SubscriptionId: subscriptionId,
			TokenId:        tokenId,
			NodeName:       "test-node",
			BillingContext: &model.TaskBillingContext{
				ModelPrice:      0.02,
				GroupRatio:      1.0,
				OriginModelName: "test-model",
			},
		},
	}
}

func setDurationBillingContext(task *model.Task) {
	task.PrivateData.BillingContext = &model.TaskBillingContext{
		BillingMode: billing_setting.BillingModePerDuration,
		DurationPrice: &types.DurationPrice{
			Price: 0.1, Unit: types.DurationUnitSecond,
			RoundingStepSeconds: 1, MinimumDurationSeconds: 4,
		},
		DurationSource:           types.DurationSourceRequest,
		RequestedDurationSeconds: 6,
		BillableDurationSeconds:  6,
		GroupRatio:               1,
		OtherRatios:              map[string]float64{"resolution": 2.5},
		OriginModelName:          "test-model",
		UpstreamModelName:        "jimeng-video-seedance-2.0-vip",
		Resolution:               "1080p",
		PerCallBilling:           true,
	}
}

func resetQuotaDataCache() {
	model.CacheQuotaDataLock.Lock()
	defer model.CacheQuotaDataLock.Unlock()
	model.CacheQuotaData = make(map[string]*model.QuotaData)
}

func enableTaskQuotaData(t *testing.T) {
	t.Helper()
	previous := common.DataExportEnabled
	common.DataExportEnabled = true
	t.Cleanup(func() { common.DataExportEnabled = previous })
}

func TestPriceDataOtherRatiosFilterAndSnapshot(t *testing.T) {
	priceData := types.PriceData{}

	priceData.AddOtherRatio("zero", 0)
	priceData.AddOtherRatio("negative", -0.5)
	priceData.AddOtherRatio("nan", math.NaN())
	priceData.AddOtherRatio("inf", math.Inf(1))
	priceData.AddOtherRatio("one", 1)
	priceData.AddOtherRatio("positive", 2.5)

	ratios := priceData.OtherRatios()
	require.Len(t, ratios, 2)
	assert.Equal(t, 1.0, ratios["one"])
	assert.Equal(t, 2.5, ratios["positive"])
	assert.True(t, priceData.HasOtherRatio("one"))
	assert.False(t, priceData.HasOtherRatio("zero"))

	ratios["positive"] = 99
	ratios["new"] = 3
	nextSnapshot := priceData.OtherRatios()
	assert.Equal(t, 2.5, nextSnapshot["positive"])
	assert.NotContains(t, nextSnapshot, "new")
}

func TestPriceDataReplaceAndApplyOtherRatios(t *testing.T) {
	priceData := types.PriceData{}

	replaced := priceData.ReplaceOtherRatios(map[string]float64{
		"zero":     0,
		"negative": -3,
		"nan":      math.NaN(),
		"inf":      math.Inf(1),
		"one":      1,
		"duration": 2,
		"size":     1.5,
	})

	require.True(t, replaced)
	assert.Equal(t, 3.0, priceData.OtherRatioMultiplier())
	assert.Equal(t, 30.0, priceData.ApplyOtherRatiosToFloat(10))
	assert.Equal(t, 10.0, priceData.RemoveOtherRatiosFromFloat(30))
	assert.True(t, decimal.NewFromInt(30).Equal(priceData.ApplyOtherRatiosToDecimal(decimal.NewFromInt(10))))

	replaced = priceData.ReplaceOtherRatios(map[string]float64{
		"zero": 0,
		"nan":  math.NaN(),
	})

	require.False(t, replaced)
	assert.Nil(t, priceData.OtherRatios())
	assert.Equal(t, 1.0, priceData.OtherRatioMultiplier())
}

func TestTaskBillingOtherFiltersHistoricalOtherRatios(t *testing.T) {
	task := makeTask(1, 1, 100, 0, BillingSourceWallet, 0)
	task.PrivateData.BillingContext.OtherRatios = map[string]float64{
		"seconds":  2,
		"identity": 1,
		"zero":     0,
		"negative": -1,
		"nan":      math.NaN(),
		"inf":      math.Inf(1),
	}

	other := taskBillingOther(task)

	assert.Equal(t, 2.0, other["seconds"])
	assert.Equal(t, 1.0, other["identity"])
	assert.NotContains(t, other, "zero")
	assert.NotContains(t, other, "negative")
	assert.NotContains(t, other, "nan")
	assert.NotContains(t, other, "inf")
}

func TestTaskBillingOtherIncludesDurationSnapshot(t *testing.T) {
	task := makeTask(1, 1, 100, 0, BillingSourceWallet, 0)
	setDurationBillingContext(task)

	other := taskBillingOther(task)

	assert.Equal(t, billing_setting.BillingModePerDuration, other["billing_mode"])
	assert.Equal(t, 0.1, other["duration_price"])
	assert.Equal(t, types.DurationUnitSecond, other["duration_unit"])
	assert.Equal(t, 1, other["rounding_step_seconds"])
	assert.Equal(t, 4, other["minimum_duration_seconds"])
	assert.Equal(t, types.DurationSourceRequest, other["duration_source"])
	assert.Equal(t, 6, other["requested_duration_seconds"])
	assert.Equal(t, 6, other["billable_duration_seconds"])
	assert.Equal(t, 2.5, other["resolution_ratio"])
	assert.Equal(t, "1080p", other["resolution"])
	assert.NotContains(t, other, "model_price")
}

func TestLogTaskConsumptionIncludesDurationSnapshot(t *testing.T) {
	truncate(t)
	const userID, channelID, quota = 41, 41, 750_000
	seedUser(t, userID, 10_000_000)
	seedChannel(t, channelID)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", nil)
	c.Set("token_name", "duration-token")
	rule := types.DurationPrice{
		Price: 0.1, Unit: types.DurationUnitSecond,
		RoundingStepSeconds: 1, MinimumDurationSeconds: 4,
	}
	priceData := types.PriceData{
		BillingMode:              billing_setting.BillingModePerDuration,
		DurationPrice:            &rule,
		DurationSource:           types.DurationSourceRequest,
		RequestedDurationSeconds: 6,
		BillableDurationSeconds:  6,
		Quota:                    quota,
		GroupRatioInfo:           types.GroupRatioInfo{GroupRatio: 1},
	}
	priceData.AddOtherRatio("resolution", 2.5)
	info := &relaycommon.RelayInfo{
		UserId:          userID,
		OriginModelName: "test-model",
		UsingGroup:      "default",
		ChannelMeta:     &relaycommon.ChannelMeta{ChannelId: channelID},
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{Action: "generate"},
		PriceData:       priceData,
	}

	LogTaskConsumption(c, info)

	log := getLastLog(t)
	require.NotNil(t, log)
	var other map[string]interface{}
	require.NoError(t, common.UnmarshalJsonStr(log.Other, &other))
	assert.Equal(t, billing_setting.BillingModePerDuration, other["billing_mode"])
	assert.Equal(t, 0.1, other["duration_price"])
	assert.Equal(t, types.DurationUnitSecond, other["duration_unit"])
	assert.Equal(t, float64(1), other["rounding_step_seconds"])
	assert.Equal(t, float64(4), other["minimum_duration_seconds"])
	assert.Equal(t, types.DurationSourceRequest, other["duration_source"])
	assert.Equal(t, float64(6), other["requested_duration_seconds"])
	assert.Equal(t, float64(6), other["billable_duration_seconds"])
	assert.Equal(t, 2.5, other["resolution_ratio"])
	assert.NotContains(t, other, "model_price")
	assert.Equal(t, quota, getUserUsedQuota(t, userID))
	assert.Equal(t, int64(quota), getChannelUsedQuota(t, channelID))
}

func TestTaskBillingOtherPreservesServiceTierRatio(t *testing.T) {
	task := makeTask(1, 1, 100, 0, BillingSourceWallet, 0)
	task.PrivateData.BillingContext.OtherRatios = map[string]float64{"service_tier": 0.5}
	task.PrivateData.BillingContext.ServiceTier = "flex"

	other := taskBillingOther(task)

	assert.Equal(t, 0.5, other["service_tier"])
	assert.Equal(t, "flex", other["service_tier_value"])
}

func TestTaskBillingOtherStoresDefaultServiceTierWithoutRatio(t *testing.T) {
	task := makeTask(1, 1, 100, 0, BillingSourceWallet, 0)
	task.PrivateData.BillingContext.ServiceTier = "default"

	other := taskBillingOther(task)

	assert.NotContains(t, other, "service_tier")
	assert.Equal(t, "default", other["service_tier_value"])
}

func TestTaskBillingContextPriceDataFiltersMultiplier(t *testing.T) {
	priceData := taskBillingContextPriceData(&model.TaskBillingContext{
		OtherRatios: map[string]float64{
			"seconds":  2,
			"size":     3,
			"identity": 1,
			"zero":     0,
			"negative": -1,
			"nan":      math.NaN(),
			"inf":      math.Inf(1),
		},
	})

	require.NotNil(t, priceData)
	assert.Equal(t, 6.0, priceData.OtherRatioMultiplier())
	assert.Equal(t, map[string]float64{
		"seconds":  2,
		"size":     3,
		"identity": 1,
	}, priceData.OtherRatios())
}

// ---------------------------------------------------------------------------
// Read-back helpers
// ---------------------------------------------------------------------------

func getUserQuota(t *testing.T, id int) int {
	t.Helper()
	var user model.User
	require.NoError(t, model.DB.Select("quota").Where("id = ?", id).First(&user).Error)
	return user.Quota
}

func getUserUsedQuota(t *testing.T, id int) int {
	t.Helper()
	var user model.User
	require.NoError(t, model.DB.Select("used_quota").Where("id = ?", id).First(&user).Error)
	return user.UsedQuota
}

func getUserRequestCount(t *testing.T, id int) int {
	t.Helper()
	var user model.User
	require.NoError(t, model.DB.Select("request_count").Where("id = ?", id).First(&user).Error)
	return user.RequestCount
}

func getChannelUsedQuota(t *testing.T, id int) int64 {
	t.Helper()
	var channel model.Channel
	require.NoError(t, model.DB.Select("used_quota").Where("id = ?", id).First(&channel).Error)
	return channel.UsedQuota
}

func getTokenRemainQuota(t *testing.T, id int) int {
	t.Helper()
	var token model.Token
	require.NoError(t, model.DB.Select("remain_quota").Where("id = ?", id).First(&token).Error)
	return token.RemainQuota
}

func getTokenUsedQuota(t *testing.T, id int) int {
	t.Helper()
	var token model.Token
	require.NoError(t, model.DB.Select("used_quota").Where("id = ?", id).First(&token).Error)
	return token.UsedQuota
}

func getSubscriptionUsed(t *testing.T, id int) int64 {
	t.Helper()
	var sub model.UserSubscription
	require.NoError(t, model.DB.Select("amount_used").Where("id = ?", id).First(&sub).Error)
	return sub.AmountUsed
}

func getLastLog(t *testing.T) *model.Log {
	t.Helper()
	var log model.Log
	err := model.LOG_DB.Order("id desc").First(&log).Error
	if err != nil {
		return nil
	}
	return &log
}

func countLogs(t *testing.T) int64 {
	t.Helper()
	var count int64
	model.LOG_DB.Model(&model.Log{}).Count(&count)
	return count
}

func seedTaskQuotaData(t *testing.T, task *model.Task) {
	t.Helper()
	username, err := model.GetUsernameById(task.UserId, false)
	require.NoError(t, err)
	model.LogQuotaData(model.QuotaDataLogParams{
		UserID:    task.UserId,
		Username:  username,
		ModelName: taskModelName(task),
		Quota:     task.Quota,
		CreatedAt: task.SubmitTime,
		UseGroup:  task.Group,
		TokenID:   task.PrivateData.TokenId,
		ChannelID: task.ChannelId,
		NodeName:  task.PrivateData.NodeName,
	})
}

func getTaskQuotaData(t *testing.T, task *model.Task) model.QuotaData {
	t.Helper()
	username, err := model.GetUsernameById(task.UserId, false)
	require.NoError(t, err)
	wantHour := task.SubmitTime - task.SubmitTime%3600

	model.CacheQuotaDataLock.Lock()
	defer model.CacheQuotaDataLock.Unlock()
	for _, quotaData := range model.CacheQuotaData {
		if quotaData.UserID == task.UserId &&
			quotaData.Username == username &&
			quotaData.ModelName == taskModelName(task) &&
			quotaData.CreatedAt == wantHour &&
			quotaData.UseGroup == task.Group &&
			quotaData.TokenID == task.PrivateData.TokenId &&
			quotaData.ChannelID == task.ChannelId &&
			quotaData.NodeName == task.PrivateData.NodeName {
			return *quotaData
		}
	}
	require.FailNow(t, "task quota_data entry not found")
	return model.QuotaData{}
}

// ===========================================================================
// RefundTaskQuota tests
// ===========================================================================

func TestRefundTaskQuota_Wallet(t *testing.T) {
	truncate(t)
	enableTaskQuotaData(t)
	ctx := context.Background()

	const userID, tokenID, channelID = 1, 1, 1
	const initQuota, preConsumed = 10000, 3000
	const tokenRemain = 5000

	seedUserWithUsage(t, userID, initQuota, preConsumed, 7)
	seedToken(t, tokenID, userID, "sk-test-key", tokenRemain)
	seedChannelWithUsage(t, channelID, preConsumed)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)
	setDurationBillingContext(task)
	seedTaskQuotaData(t, task)

	RefundTaskQuota(ctx, task, "task failed: upstream error")

	// User quota should increase by preConsumed
	assert.Equal(t, initQuota+preConsumed, getUserQuota(t, userID))
	assert.Equal(t, 0, getUserUsedQuota(t, userID))
	assert.Equal(t, 7, getUserRequestCount(t, userID))
	assert.Equal(t, int64(0), getChannelUsedQuota(t, channelID))
	quotaData := getTaskQuotaData(t, task)
	assert.Equal(t, 1, quotaData.Count)
	assert.Equal(t, 0, quotaData.Quota)

	// Token remain_quota should increase, used_quota should decrease
	assert.Equal(t, tokenRemain+preConsumed, getTokenRemainQuota(t, tokenID))
	assert.Equal(t, -preConsumed, getTokenUsedQuota(t, tokenID))

	// A refund log should be created
	log := getLastLog(t)
	require.NotNil(t, log)
	assert.Equal(t, model.LogTypeRefund, log.Type)
	assert.Equal(t, preConsumed, log.Quota)
	assert.Equal(t, "test-model", log.ModelName)
	var other map[string]interface{}
	require.NoError(t, common.UnmarshalJsonStr(log.Other, &other))
	assert.Equal(t, billing_setting.BillingModePerDuration, other["billing_mode"])
	assert.Equal(t, 0.1, other["duration_price"])
	assert.Equal(t, types.DurationUnitSecond, other["duration_unit"])
	assert.Equal(t, float64(6), other["requested_duration_seconds"])
	assert.Equal(t, float64(6), other["billable_duration_seconds"])
	assert.Equal(t, 2.5, other["resolution_ratio"])
	assert.Equal(t, "1080p", other["resolution"])
	assert.NotContains(t, other, "model_price")
}

func TestRefundTaskQuota_Subscription(t *testing.T) {
	truncate(t)
	enableTaskQuotaData(t)
	ctx := context.Background()

	const userID, tokenID, channelID, subID = 2, 2, 2, 1
	const preConsumed = 2000
	const subTotal, subUsed int64 = 100000, 50000
	const tokenRemain = 8000

	seedUserWithUsage(t, userID, 0, preConsumed, 3)
	seedToken(t, tokenID, userID, "sk-sub-key", tokenRemain)
	seedChannelWithUsage(t, channelID, preConsumed)
	seedSubscription(t, subID, userID, subTotal, subUsed)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceSubscription, subID)
	seedTaskQuotaData(t, task)

	RefundTaskQuota(ctx, task, "subscription task failed")

	// Subscription used should decrease by preConsumed
	assert.Equal(t, subUsed-int64(preConsumed), getSubscriptionUsed(t, subID))
	assert.Equal(t, 0, getUserUsedQuota(t, userID))
	assert.Equal(t, 3, getUserRequestCount(t, userID))
	assert.Equal(t, int64(0), getChannelUsedQuota(t, channelID))
	quotaData := getTaskQuotaData(t, task)
	assert.Equal(t, 1, quotaData.Count)
	assert.Equal(t, 0, quotaData.Quota)

	// Token should also be refunded
	assert.Equal(t, tokenRemain+preConsumed, getTokenRemainQuota(t, tokenID))

	log := getLastLog(t)
	require.NotNil(t, log)
	assert.Equal(t, model.LogTypeRefund, log.Type)
}

func TestRefundTaskQuota_ZeroQuota(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID = 3
	seedUser(t, userID, 5000)

	task := makeTask(userID, 0, 0, 0, BillingSourceWallet, 0)

	RefundTaskQuota(ctx, task, "zero quota task")

	// No change to user quota
	assert.Equal(t, 5000, getUserQuota(t, userID))

	// No log created
	assert.Equal(t, int64(0), countLogs(t))
}

func TestRefundTaskQuota_NoToken(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, channelID = 4, 4
	const initQuota, preConsumed = 10000, 1500

	seedUser(t, userID, initQuota)
	seedChannel(t, channelID)

	task := makeTask(userID, channelID, preConsumed, 0, BillingSourceWallet, 0) // TokenId=0

	RefundTaskQuota(ctx, task, "no token task failed")

	// User quota refunded
	assert.Equal(t, initQuota+preConsumed, getUserQuota(t, userID))

	// Log created
	log := getLastLog(t)
	require.NotNil(t, log)
	assert.Equal(t, model.LogTypeRefund, log.Type)
}

// ===========================================================================
// RecalculateTaskQuota tests
// ===========================================================================

func TestRecalculate_PositiveDelta(t *testing.T) {
	truncate(t)
	enableTaskQuotaData(t)
	ctx := context.Background()

	const userID, tokenID, channelID = 10, 10, 10
	const initQuota, preConsumed = 10000, 2000
	const actualQuota = 3000 // under-charged by 1000
	const tokenRemain = 5000

	seedUserWithUsage(t, userID, initQuota, preConsumed, 4)
	seedToken(t, tokenID, userID, "sk-recalc-pos", tokenRemain)
	seedChannelWithUsage(t, channelID, preConsumed)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)
	require.NoError(t, model.DB.Create(task).Error)
	seedTaskQuotaData(t, task)

	RecalculateTaskQuotaWithTokens(ctx, task, actualQuota, 1234, "adaptor adjustment")

	// User quota should decrease by the delta (1000 additional charge)
	assert.Equal(t, initQuota-(actualQuota-preConsumed), getUserQuota(t, userID))
	assert.Equal(t, actualQuota, getUserUsedQuota(t, userID))
	assert.Equal(t, 4, getUserRequestCount(t, userID))
	assert.Equal(t, int64(actualQuota), getChannelUsedQuota(t, channelID))
	quotaData := getTaskQuotaData(t, task)
	assert.Equal(t, 1, quotaData.Count)
	assert.Equal(t, actualQuota, quotaData.Quota)
	assert.Equal(t, 1234, quotaData.TokenUsed)

	// Token should also be charged the delta
	assert.Equal(t, tokenRemain-(actualQuota-preConsumed), getTokenRemainQuota(t, tokenID))

	// task.Quota should be updated to actualQuota
	assert.Equal(t, actualQuota, task.Quota)
	var persistedTask model.Task
	require.NoError(t, model.DB.Where("task_id = ?", task.TaskID).First(&persistedTask).Error)
	assert.Equal(t, actualQuota, persistedTask.Quota)

	// Log type should be Consume (additional charge)
	log := getLastLog(t)
	require.NotNil(t, log)
	assert.Equal(t, model.LogTypeConsume, log.Type)
	assert.Equal(t, actualQuota-preConsumed, log.Quota)
}

func TestRecalculate_NegativeDelta(t *testing.T) {
	truncate(t)
	enableTaskQuotaData(t)
	ctx := context.Background()

	const userID, tokenID, channelID = 11, 11, 11
	const initQuota, preConsumed = 10000, 5000
	const actualQuota = 3000 // over-charged by 2000
	const tokenRemain = 5000

	seedUserWithUsage(t, userID, initQuota, preConsumed, 5)
	seedToken(t, tokenID, userID, "sk-recalc-neg", tokenRemain)
	seedChannelWithUsage(t, channelID, preConsumed)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)
	require.NoError(t, model.DB.Create(task).Error)
	seedTaskQuotaData(t, task)

	RecalculateTaskQuotaWithTokens(ctx, task, actualQuota, 987, "adaptor adjustment")

	// User quota should increase by abs(delta) = 2000 (refund overpayment)
	assert.Equal(t, initQuota+(preConsumed-actualQuota), getUserQuota(t, userID))
	assert.Equal(t, actualQuota, getUserUsedQuota(t, userID))
	assert.Equal(t, 5, getUserRequestCount(t, userID))
	assert.Equal(t, int64(actualQuota), getChannelUsedQuota(t, channelID))
	quotaData := getTaskQuotaData(t, task)
	assert.Equal(t, 1, quotaData.Count)
	assert.Equal(t, actualQuota, quotaData.Quota)
	assert.Equal(t, 987, quotaData.TokenUsed)

	// Token should be refunded the difference
	assert.Equal(t, tokenRemain+(preConsumed-actualQuota), getTokenRemainQuota(t, tokenID))

	// task.Quota updated
	assert.Equal(t, actualQuota, task.Quota)
	var persistedTask model.Task
	require.NoError(t, model.DB.Where("task_id = ?", task.TaskID).First(&persistedTask).Error)
	assert.Equal(t, actualQuota, persistedTask.Quota)

	// Log type should be Refund
	log := getLastLog(t)
	require.NotNil(t, log)
	assert.Equal(t, model.LogTypeRefund, log.Type)
	assert.Equal(t, preConsumed-actualQuota, log.Quota)
}

func TestRecalculate_ZeroDelta(t *testing.T) {
	truncate(t)
	enableTaskQuotaData(t)
	ctx := context.Background()

	const userID, tokenID = 12, 12
	const initQuota, preConsumed = 10000, 3000
	const tokenRemain = 5000

	seedUserWithUsage(t, userID, initQuota, preConsumed, 6)
	seedToken(t, tokenID, userID, "sk-recalc-zero", tokenRemain)

	seedChannelWithUsage(t, userID, preConsumed)
	task := makeTask(userID, userID, preConsumed, tokenID, BillingSourceWallet, 0)
	require.NoError(t, model.DB.Create(task).Error)
	seedTaskQuotaData(t, task)

	RecalculateTaskQuotaWithTokens(ctx, task, preConsumed, 777, "exact match")

	// No change to user quota
	assert.Equal(t, initQuota, getUserQuota(t, userID))
	assert.Equal(t, preConsumed, getUserUsedQuota(t, userID))
	assert.Equal(t, 6, getUserRequestCount(t, userID))
	assert.Equal(t, tokenRemain, getTokenRemainQuota(t, task.PrivateData.TokenId))
	assert.Equal(t, 0, getTokenUsedQuota(t, task.PrivateData.TokenId))
	assert.Equal(t, int64(preConsumed), getChannelUsedQuota(t, userID))
	quotaData := getTaskQuotaData(t, task)
	assert.Equal(t, 1, quotaData.Count)
	assert.Equal(t, preConsumed, quotaData.Quota)
	assert.Equal(t, 777, quotaData.TokenUsed)
	var persistedTask model.Task
	require.NoError(t, model.DB.Where("task_id = ?", task.TaskID).First(&persistedTask).Error)
	assert.Equal(t, preConsumed, persistedTask.Quota)

	// No log created (delta is zero)
	assert.Equal(t, int64(0), countLogs(t))
}

func TestRecalculate_ActualQuotaZero(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID = 13
	const initQuota = 10000

	seedUser(t, userID, initQuota)

	task := makeTask(userID, 0, 5000, 0, BillingSourceWallet, 0)

	RecalculateTaskQuota(ctx, task, 0, "zero actual")

	// No change (early return)
	assert.Equal(t, initQuota, getUserQuota(t, userID))
	assert.Equal(t, int64(0), countLogs(t))
}

func TestRecalculate_Subscription_NegativeDelta(t *testing.T) {
	truncate(t)
	enableTaskQuotaData(t)
	ctx := context.Background()

	const userID, tokenID, channelID, subID = 14, 14, 14, 2
	const preConsumed = 5000
	const actualQuota = 2000 // over-charged by 3000
	const subTotal, subUsed int64 = 100000, 50000
	const tokenRemain = 8000

	seedUserWithUsage(t, userID, 0, preConsumed, 2)
	seedToken(t, tokenID, userID, "sk-sub-recalc", tokenRemain)
	seedChannelWithUsage(t, channelID, preConsumed)
	seedSubscription(t, subID, userID, subTotal, subUsed)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceSubscription, subID)
	seedTaskQuotaData(t, task)

	RecalculateTaskQuota(ctx, task, actualQuota, "subscription over-charge")

	// Subscription used should decrease by delta (refund 3000)
	assert.Equal(t, subUsed-int64(preConsumed-actualQuota), getSubscriptionUsed(t, subID))
	assert.Equal(t, actualQuota, getUserUsedQuota(t, userID))
	assert.Equal(t, 2, getUserRequestCount(t, userID))
	assert.Equal(t, int64(actualQuota), getChannelUsedQuota(t, channelID))
	quotaData := getTaskQuotaData(t, task)
	assert.Equal(t, 1, quotaData.Count)
	assert.Equal(t, actualQuota, quotaData.Quota)

	// Token refunded
	assert.Equal(t, tokenRemain+(preConsumed-actualQuota), getTokenRemainQuota(t, tokenID))

	assert.Equal(t, actualQuota, task.Quota)

	log := getLastLog(t)
	require.NotNil(t, log)
	assert.Equal(t, model.LogTypeRefund, log.Type)
}

func TestRecalculate_Subscription_PositiveDelta(t *testing.T) {
	truncate(t)
	enableTaskQuotaData(t)
	ctx := context.Background()

	const userID, tokenID, channelID, subID = 15, 15, 15, 3
	const preConsumed = 2000
	const actualQuota = 3500
	const subTotal, subUsed int64 = 100000, 50000
	const tokenRemain = 8000

	seedUserWithUsage(t, userID, 0, preConsumed, 8)
	seedToken(t, tokenID, userID, "sk-sub-recalc-positive", tokenRemain)
	seedChannelWithUsage(t, channelID, preConsumed)
	seedSubscription(t, subID, userID, subTotal, subUsed)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceSubscription, subID)
	seedTaskQuotaData(t, task)

	RecalculateTaskQuotaWithTokens(ctx, task, actualQuota, 4321, "subscription under-charge")

	assert.Equal(t, subUsed+int64(actualQuota-preConsumed), getSubscriptionUsed(t, subID))
	assert.Equal(t, actualQuota, getUserUsedQuota(t, userID))
	assert.Equal(t, 8, getUserRequestCount(t, userID))
	assert.Equal(t, int64(actualQuota), getChannelUsedQuota(t, channelID))
	quotaData := getTaskQuotaData(t, task)
	assert.Equal(t, 1, quotaData.Count)
	assert.Equal(t, actualQuota, quotaData.Quota)
	assert.Equal(t, 4321, quotaData.TokenUsed)
}

// ===========================================================================
// CAS + Billing integration tests
// Simulates the flow in updateVideoSingleTask (service/task_polling.go)
// ===========================================================================

// simulatePollBilling reproduces the CAS + billing logic from updateVideoSingleTask.
// It takes a persisted task (already in DB), applies the new status, and performs
// the conditional update + billing exactly as the polling loop does.
func simulatePollBilling(ctx context.Context, task *model.Task, newStatus model.TaskStatus, actualQuota int) {
	snap := task.Snapshot()

	shouldRefund := false
	shouldSettle := false
	quota := task.Quota

	task.Status = newStatus
	switch string(newStatus) {
	case model.TaskStatusSuccess:
		task.Progress = "100%"
		task.FinishTime = 9999
		shouldSettle = true
	case model.TaskStatusFailure:
		task.Progress = "100%"
		task.FinishTime = 9999
		task.FailReason = "upstream error"
		if quota != 0 {
			shouldRefund = true
		}
	default:
		task.Progress = "50%"
	}

	isDone := task.Status == model.TaskStatus(model.TaskStatusSuccess) || task.Status == model.TaskStatus(model.TaskStatusFailure)
	if isDone && snap.Status != task.Status {
		won, err := task.UpdateWithStatus(snap.Status)
		if err != nil {
			shouldRefund = false
			shouldSettle = false
		} else if !won {
			shouldRefund = false
			shouldSettle = false
		}
	} else if !snap.Equal(task.Snapshot()) {
		_, _ = task.UpdateWithStatus(snap.Status)
	}

	if shouldSettle && actualQuota > 0 {
		RecalculateTaskQuota(ctx, task, actualQuota, "test settle")
	}
	if shouldRefund {
		RefundTaskQuota(ctx, task, task.FailReason)
	}
}

func TestCASGuardedRefund_Win(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID = 20, 20, 20
	const initQuota, preConsumed = 10000, 4000
	const tokenRemain = 6000

	seedUser(t, userID, initQuota)
	seedToken(t, tokenID, userID, "sk-cas-refund-win", tokenRemain)
	seedChannel(t, channelID)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)
	task.Status = model.TaskStatus(model.TaskStatusInProgress)
	require.NoError(t, model.DB.Create(task).Error)

	simulatePollBilling(ctx, task, model.TaskStatus(model.TaskStatusFailure), 0)

	// CAS wins: task in DB should now be FAILURE
	var reloaded model.Task
	require.NoError(t, model.DB.First(&reloaded, task.ID).Error)
	assert.EqualValues(t, model.TaskStatusFailure, reloaded.Status)

	// Refund should have happened
	assert.Equal(t, initQuota+preConsumed, getUserQuota(t, userID))
	assert.Equal(t, tokenRemain+preConsumed, getTokenRemainQuota(t, tokenID))

	log := getLastLog(t)
	require.NotNil(t, log)
	assert.Equal(t, model.LogTypeRefund, log.Type)
}

func TestCASGuardedRefund_Lose(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID = 21, 21, 21
	const initQuota, preConsumed = 10000, 4000
	const tokenRemain = 6000

	seedUser(t, userID, initQuota)
	seedToken(t, tokenID, userID, "sk-cas-refund-lose", tokenRemain)
	seedChannel(t, channelID)

	// Create task with IN_PROGRESS in DB
	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)
	task.Status = model.TaskStatus(model.TaskStatusInProgress)
	require.NoError(t, model.DB.Create(task).Error)

	// Simulate another process already transitioning to FAILURE
	model.DB.Model(&model.Task{}).Where("id = ?", task.ID).Update("status", model.TaskStatusFailure)

	// Our process still has the old in-memory state (IN_PROGRESS) and tries to transition
	// task.Status is still IN_PROGRESS in the snapshot
	simulatePollBilling(ctx, task, model.TaskStatus(model.TaskStatusFailure), 0)

	// CAS lost: user quota should NOT change (no double refund)
	assert.Equal(t, initQuota, getUserQuota(t, userID))
	assert.Equal(t, tokenRemain, getTokenRemainQuota(t, tokenID))

	// No billing log should be created
	assert.Equal(t, int64(0), countLogs(t))
}

func TestCASGuardedSettle_Win(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID = 22, 22, 22
	const initQuota, preConsumed = 10000, 5000
	const actualQuota = 3000 // over-charged, should get partial refund
	const tokenRemain = 8000

	seedUser(t, userID, initQuota)
	seedToken(t, tokenID, userID, "sk-cas-settle-win", tokenRemain)
	seedChannel(t, channelID)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)
	task.Status = model.TaskStatus(model.TaskStatusInProgress)
	require.NoError(t, model.DB.Create(task).Error)

	simulatePollBilling(ctx, task, model.TaskStatus(model.TaskStatusSuccess), actualQuota)

	// CAS wins: task should be SUCCESS
	var reloaded model.Task
	require.NoError(t, model.DB.First(&reloaded, task.ID).Error)
	assert.EqualValues(t, model.TaskStatusSuccess, reloaded.Status)

	// Settlement should refund the over-charge (5000 - 3000 = 2000 back to user)
	assert.Equal(t, initQuota+(preConsumed-actualQuota), getUserQuota(t, userID))
	assert.Equal(t, tokenRemain+(preConsumed-actualQuota), getTokenRemainQuota(t, tokenID))

	// task.Quota should be updated to actualQuota
	assert.Equal(t, actualQuota, task.Quota)
}

func TestNonTerminalUpdate_NoBilling(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, channelID = 23, 23
	const initQuota, preConsumed = 10000, 3000

	seedUser(t, userID, initQuota)
	seedChannel(t, channelID)

	task := makeTask(userID, channelID, preConsumed, 0, BillingSourceWallet, 0)
	task.Status = model.TaskStatus(model.TaskStatusInProgress)
	task.Progress = "20%"
	require.NoError(t, model.DB.Create(task).Error)

	// Simulate a non-terminal poll update (still IN_PROGRESS, progress changed)
	simulatePollBilling(ctx, task, model.TaskStatus(model.TaskStatusInProgress), 0)

	// User quota should NOT change
	assert.Equal(t, initQuota, getUserQuota(t, userID))

	// No billing log
	assert.Equal(t, int64(0), countLogs(t))

	// Task progress should be updated in DB
	var reloaded model.Task
	require.NoError(t, model.DB.First(&reloaded, task.ID).Error)
	assert.Equal(t, "50%", reloaded.Progress)
}

// ===========================================================================
// Mock adaptor for settleTaskBillingOnComplete tests
// ===========================================================================

type mockAdaptor struct {
	adjustReturn int
}

func (m *mockAdaptor) Init(_ *relaycommon.RelayInfo) {}
func (m *mockAdaptor) FetchTask(string, string, map[string]any, string) (*http.Response, error) {
	return nil, nil
}
func (m *mockAdaptor) ParseTaskResult([]byte) (*relaycommon.TaskInfo, error) { return nil, nil }
func (m *mockAdaptor) AdjustBillingOnComplete(_ *model.Task, _ *relaycommon.TaskInfo) int {
	return m.adjustReturn
}

// ===========================================================================
// PerCallBilling tests — settleTaskBillingOnComplete
// ===========================================================================

func TestSettle_PerCallBilling_SkipsAdaptorAdjust(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID = 30, 30, 30
	const initQuota, preConsumed = 10000, 5000
	const tokenRemain = 8000

	seedUser(t, userID, initQuota)
	seedToken(t, tokenID, userID, "sk-percall-adaptor", tokenRemain)
	seedChannel(t, channelID)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)
	setDurationBillingContext(task)

	adaptor := &mockAdaptor{adjustReturn: 2000}
	taskResult := &relaycommon.TaskInfo{Status: model.TaskStatusSuccess}

	settleTaskBillingOnComplete(ctx, adaptor, task, taskResult)

	// Per-call: no adjustment despite adaptor returning 2000
	assert.Equal(t, initQuota, getUserQuota(t, userID))
	assert.Equal(t, tokenRemain, getTokenRemainQuota(t, tokenID))
	assert.Equal(t, preConsumed, task.Quota)
	assert.Equal(t, 6, task.PrivateData.BillingContext.BillableDurationSeconds)
	assert.Equal(t, int64(0), countLogs(t))
}

func TestSettle_PerCallBilling_SkipsTotalTokens(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID = 31, 31, 31
	const initQuota, preConsumed = 10000, 4000
	const tokenRemain = 7000

	seedUser(t, userID, initQuota)
	seedToken(t, tokenID, userID, "sk-percall-tokens", tokenRemain)
	seedChannel(t, channelID)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)
	task.PrivateData.BillingContext.PerCallBilling = true

	adaptor := &mockAdaptor{adjustReturn: 0}
	taskResult := &relaycommon.TaskInfo{Status: model.TaskStatusSuccess, TotalTokens: 9999}

	settleTaskBillingOnComplete(ctx, adaptor, task, taskResult)

	// Per-call: no recalculation by tokens
	assert.Equal(t, initQuota, getUserQuota(t, userID))
	assert.Equal(t, tokenRemain, getTokenRemainQuota(t, tokenID))
	assert.Equal(t, preConsumed, task.Quota)
	assert.Equal(t, int64(0), countLogs(t))
}

func TestSettle_NonPerCallBilling_AppliesAdaptorAdjustment(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID = 32, 32, 32
	const initQuota, preConsumed = 10000, 5000
	const adaptorQuota = 3000
	const tokenRemain = 8000

	seedUser(t, userID, initQuota)
	seedToken(t, tokenID, userID, "sk-nonpercall-adj", tokenRemain)
	seedChannel(t, channelID)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)
	// PerCallBilling defaults to false

	adaptor := &mockAdaptor{adjustReturn: adaptorQuota}
	taskResult := &relaycommon.TaskInfo{Status: model.TaskStatusSuccess}

	settleTaskBillingOnComplete(ctx, adaptor, task, taskResult)

	// Non-per-call: adaptor adjustment applies (refund 2000)
	assert.Equal(t, initQuota+(preConsumed-adaptorQuota), getUserQuota(t, userID))
	assert.Equal(t, tokenRemain+(preConsumed-adaptorQuota), getTokenRemainQuota(t, tokenID))
	assert.Equal(t, adaptorQuota, task.Quota)

	log := getLastLog(t)
	require.NotNil(t, log)
	assert.Equal(t, model.LogTypeRefund, log.Type)
}
