package relay

import (
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestTaskDurationQuota(t *testing.T) {
	tests := []struct {
		name      string
		rule      types.DurationPrice
		requested int
		group     float64
		ratios    map[string]float64
		wantQuota int
		wantSecs  int
	}{
		{
			name:      "seconds with resolution ratio",
			rule:      types.DurationPrice{Price: 0.1, Unit: types.DurationUnitSecond, RoundingStepSeconds: 1, MinimumDurationSeconds: 4},
			requested: 6,
			group:     1,
			ratios:    map[string]float64{"resolution": 2.5},
			wantQuota: 750_000,
			wantSecs:  6,
		},
		{
			name:      "minutes round to five second step",
			rule:      types.DurationPrice{Price: 6, Unit: types.DurationUnitMinute, RoundingStepSeconds: 5, MinimumDurationSeconds: 4},
			requested: 6,
			group:     0.8,
			wantQuota: 400_000,
			wantSecs:  10,
		},
		{
			name:      "minimum duration",
			rule:      types.DurationPrice{Price: 0.2, Unit: types.DurationUnitSecond, RoundingStepSeconds: 1, MinimumDurationSeconds: 4},
			requested: 1,
			group:     1,
			wantQuota: 400_000,
			wantSecs:  4,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			priceData := types.PriceData{
				BillingMode:    "per_duration",
				DurationPrice:  &test.rule,
				GroupRatioInfo: types.GroupRatioInfo{GroupRatio: test.group},
			}
			priceData.ReplaceOtherRatios(test.ratios)

			quota, billable, clamp, err := taskDurationQuota(priceData, test.requested)

			require.NoError(t, err)
			assert.Equal(t, test.wantQuota, quota)
			assert.Equal(t, test.wantSecs, billable)
			assert.Nil(t, clamp)
		})
	}
}

func TestTaskDurationQuotaRejectsReservedDurationRatios(t *testing.T) {
	rule := types.DurationPrice{Price: 0.1, Unit: types.DurationUnitSecond, RoundingStepSeconds: 1, MinimumDurationSeconds: 4}
	for _, key := range []string{"seconds", "duration"} {
		t.Run(key, func(t *testing.T) {
			priceData := types.PriceData{DurationPrice: &rule, GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1}}
			priceData.ReplaceOtherRatios(map[string]float64{key: 1})

			_, _, _, err := taskDurationQuota(priceData, 6)

			require.Error(t, err)
			assert.Contains(t, err.Error(), "reserved duration ratio")
		})
	}
}

func TestTaskDurationQuotaSaturatesFinitePrice(t *testing.T) {
	rule := types.DurationPrice{Price: math.MaxFloat64, Unit: types.DurationUnitSecond, RoundingStepSeconds: 1, MinimumDurationSeconds: 4}
	priceData := types.PriceData{DurationPrice: &rule, GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1}}

	quota, billable, clamp, err := taskDurationQuota(priceData, 6)

	require.NoError(t, err)
	assert.Equal(t, common.MaxQuota, quota)
	assert.Equal(t, 6, billable)
	require.NotNil(t, clamp)
	assert.Equal(t, common.QuotaClampOverflow, clamp.Kind)

	info := &relaycommon.RelayInfo{}
	noteTaskQuotaClamp(info, clamp)
	assert.Same(t, clamp, info.QuotaClamp)
}

func TestTaskDurationQuotaPreservesFractionalQuotaPerUnit(t *testing.T) {
	originalQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 1.5
	t.Cleanup(func() {
		common.QuotaPerUnit = originalQuotaPerUnit
	})

	rule := types.DurationPrice{
		Price:               2,
		Unit:                types.DurationUnitSecond,
		RoundingStepSeconds: 1,
	}
	priceData := types.PriceData{
		DurationPrice:  &rule,
		GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1},
	}

	quota, _, clamp, err := taskDurationQuota(priceData, 1)

	require.NoError(t, err)
	assert.Equal(t, 3, quota)
	assert.Nil(t, clamp)
}

func TestTaskQuotaWithOtherRatiosUsesRawBase(t *testing.T) {
	tests := []struct {
		name      string
		priceData types.PriceData
		ratios    map[string]float64
		want      int
	}{
		{
			name: "ratio mode converts once after all multipliers",
			priceData: types.PriceData{
				ModelRatio:     46.0 / 14,
				GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1},
			},
			ratios: map[string]float64{
				"duration":     2,
				"service_tier": 0.5,
				"audio":        28.0 / 46,
			},
			want: 500_000,
		},
		{
			name: "fixed price mode",
			priceData: types.PriceData{
				UsePrice:       true,
				ModelPrice:     0.75,
				GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1.2},
			},
			ratios: map[string]float64{"duration": 2},
			want:   900_000,
		},
		{
			name: "no other ratios",
			priceData: types.PriceData{
				ModelRatio:     2,
				GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 0.5},
			},
			want: 250_000,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for name, ratio := range test.ratios {
				test.priceData.AddOtherRatio(name, ratio)
			}

			quota, clamp := taskQuotaWithOtherRatios(test.priceData)

			assert.Equal(t, test.want, quota)
			assert.Nil(t, clamp)
		})
	}
}

func TestTaskRecalcQuotaFromRatiosUsesRawBase(t *testing.T) {
	tests := []struct {
		name      string
		priceData types.PriceData
		ratios    map[string]float64
		want      int
	}{
		{
			name: "ratio mode converts once after adjusted multipliers",
			priceData: types.PriceData{
				Quota:          499_999,
				ModelRatio:     46.0 / 14,
				GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1},
			},
			ratios: map[string]float64{
				"duration":     2,
				"service_tier": 0.5,
				"audio":        28.0 / 46,
			},
			want: 500_000,
		},
		{
			name: "fixed price mode",
			priceData: types.PriceData{
				UsePrice:       true,
				ModelPrice:     0.75,
				Quota:          900_000,
				GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1.2},
			},
			ratios: map[string]float64{"duration": 2},
			want:   900_000,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			info := &relaycommon.RelayInfo{PriceData: test.priceData}
			info.PriceData.AddOtherRatio("estimated", 1.5)

			quota, ok := recalcQuotaFromRatios(info, test.ratios)

			require.True(t, ok)
			assert.Equal(t, test.want, quota)
		})
	}
}

func TestTaskRecalcQuotaFromRatiosPreservesPerDurationPricing(t *testing.T) {
	rule := types.DurationPrice{
		Price:                  0.1,
		Unit:                   types.DurationUnitSecond,
		RoundingStepSeconds:    1,
		MinimumDurationSeconds: 4,
	}
	info := &relaycommon.RelayInfo{PriceData: types.PriceData{
		BillingMode:              "per_duration",
		DurationPrice:            &rule,
		RequestedDurationSeconds: 6,
		BillableDurationSeconds:  6,
		Quota:                    300_000,
		GroupRatioInfo:           types.GroupRatioInfo{GroupRatio: 1},
	}}
	info.PriceData.AddOtherRatio("estimated_resolution", 1)

	quota, ok := recalcQuotaFromRatios(info, map[string]float64{"resolution": 2.5})

	require.True(t, ok)
	assert.Equal(t, 750_000, quota)
}

func TestCostTaskSubmitPersistsDispatchAuthorizationBeforeTransport(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service.InitHttpClient()
	withCostAccountingMode(t, types.CostAccountingStrict)
	configureNewAPIVideoFixedPricing(t, "client-video")
	setupTaskCostSubmitDB(t)

	const (
		channelID = 700009
		requestID = "task-cost-submit"
		modelName = "seedance-720p-token"
	)
	require.NoError(t, model.DB.Create(&model.Channel{
		Id: channelID, Type: constant.ChannelTypeNewAPIVideo, Name: "task supplier", Key: "secret",
	}).Error)
	unitPrice := "0.2"
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
	t.Cleanup(func() {
		service.CostCapabilityLookup = previousLookup
		service.InvalidateCostCoverage(channelID, modelName)
	})

	type transportObservation struct {
		request model.CostAccountingRequest
		attempt model.CostAccountingAttempt
		tasks   int64
		err     error
	}
	observed := make(chan transportObservation, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		observation := transportObservation{}
		observation.err = model.DB.Where("request_id = ?", requestID).First(&observation.request).Error
		if observation.err == nil {
			observation.err = model.DB.Where("cost_request_id = ?", observation.request.ID).First(&observation.attempt).Error
		}
		if observation.err == nil && observation.request.TaskID != nil {
			observation.err = model.DB.Model(&model.Task{}).Where("task_id = ?", *observation.request.TaskID).Count(&observation.tasks).Error
		}
		observed <- observation
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"upstream-task","status":"queued"}`))
	}))
	t.Cleanup(server.Close)

	c, info := newNewAPIVideoRelayContext(`{"model":"client-video","prompt":"text","seconds":"5"}`, server.URL)
	c.Set(string(constant.ContextKeyChannelId), channelID)
	c.Set(string(constant.ContextKeyChannelName), "task supplier")
	info.RequestId = requestID
	info.RequestURLPath = "/v1/video/generations"
	info.UserId = 11
	info.TokenId = 22

	result, taskErr := RelayTaskSubmit(c, info)

	require.Nil(t, taskErr)
	require.NotNil(t, result)
	observation := <-observed
	require.NoError(t, observation.err)
	require.NotNil(t, observation.request.TaskID)
	assert.Equal(t, info.PublicTaskID, *observation.request.TaskID)
	assert.Zero(t, observation.tasks)
	assert.Equal(t, string(types.CostAttemptDispatching), observation.attempt.Status)
	require.NotNil(t, info.CostAttempt)
	assert.Equal(t, observation.request.ID, info.CostRequestID)
	assert.Equal(t, observation.attempt.ID, info.CostAttempt.AttemptID)

	var settled model.CostAccountingAttempt
	require.NoError(t, model.DB.First(&settled, observation.attempt.ID).Error)
	assert.Equal(t, string(types.CostAttemptSettled), settled.Status)
	require.NotNil(t, settled.CostNanoUSD)
	assert.Equal(t, int64(200_000_000), *settled.CostNanoUSD)
	var request model.CostAccountingRequest
	require.NoError(t, model.DB.First(&request, observation.request.ID).Error)
	require.NotNil(t, request.WinningAttemptID)
	assert.Equal(t, settled.ID, *request.WinningAttemptID)
}

func TestCostTaskSubmitUsesValidatedDurationOutsideUserDurationBilling(t *testing.T) {
	tests := []struct {
		name             string
		channelType      int
		body             string
		upstreamModel    string
		durationSeconds  int
		arkOfficialRoute bool
	}{
		{name: "NewAPIVideo", channelType: constant.ChannelTypeNewAPIVideo, body: `{"model":"client-video","prompt":"text","seconds":"5"}`, upstreamModel: "seedance-720p-token", durationSeconds: 5},
		{name: "Lucen", channelType: constant.ChannelTypeLucen, body: `{"model":"client-video","prompt":"text","seconds":"5"}`, upstreamModel: "seedance-720p-token", durationSeconds: 5},
		{name: "MegaByAI", channelType: constant.ChannelTypeMegaByAI, body: `{"model":"client-video","content":[{"type":"text","text":"text"}],"duration":8}`, upstreamModel: "videos-mini", durationSeconds: 8, arkOfficialRoute: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			service.InitHttpClient()
			withCostAccountingMode(t, types.CostAccountingStrict)
			configureNewAPIVideoFixedPricing(t, "client-video")
			setupTaskCostSubmitDB(t)

			const channelID = 700010
			pricePerSecond := "0.1"
			seedTaskCostSubmitRuleForChannel(t, channelID, tt.channelType, tt.upstreamModel, types.CostModePerDuration, types.CostRuleConfigV1{
				Currency: "USD", BillingMultiplier: "1", PurchaseDiscountRatio: "1",
				RechargeExchangeRatio: "1", FeeRate: "0", CurrencyToUSDRate: "1",
				PricePerSecond: &pricePerSecond, ChargeEvent: types.CostChargeSubmitAccepted,
				MeterSource: types.CostMeterValidatedRequest,
			})

			upstreamCalled := make(chan struct{}, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				upstreamCalled <- struct{}{}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"id":"upstream-duration-task","status":"queued"}`))
			}))
			t.Cleanup(server.Close)
			c, info := newNewAPIVideoRelayContext(tt.body, server.URL)
			c.Set(string(constant.ContextKeyChannelType), tt.channelType)
			c.Set(string(constant.ContextKeyChannelId), channelID)
			c.Set(string(constant.ContextKeyChannelName), "task supplier")
			if tt.arkOfficialRoute {
				c.Set(common.KeySeedanceOfficialAPI, true)
				c.Set("model_mapping", `{"client-video":"`+tt.upstreamModel+`"}`)
			}
			info.RequestId = "task-cost-validated-duration"
			info.RequestURLPath = "/v1/video/generations"

			result, taskErr := RelayTaskSubmit(c, info)

			require.Nil(t, taskErr)
			require.NotNil(t, result)
			assert.NotContains(t, info.PriceData.OtherRatios(), "seconds")
			select {
			case <-upstreamCalled:
			default:
				t.Fatal("upstream was not called")
			}
			require.NotNil(t, info.CostAttempt)
			var attempt model.CostAccountingAttempt
			require.NoError(t, model.DB.First(&attempt, info.CostAttempt.AttemptID).Error)
			assert.Equal(t, string(types.CostAttemptSettled), attempt.Status)
			assert.JSONEq(t, fmt.Sprintf(`{"source":"validated_request","duration_seconds":"%d"}`, tt.durationSeconds), attempt.RequestMeterJSON)
			require.NotNil(t, attempt.CostNanoUSD)
			assert.Equal(t, int64(tt.durationSeconds)*100_000_000, *attempt.CostNanoUSD)
		})
	}
}

func seedTaskCostSubmitRule(t *testing.T, channelID int, modelName string, mode types.CostMode, config types.CostRuleConfigV1) {
	t.Helper()
	seedTaskCostSubmitRuleForChannel(t, channelID, constant.ChannelTypeNewAPIVideo, modelName, mode, config)
}

func seedTaskCostSubmitRuleForChannel(t *testing.T, channelID, channelType int, modelName string, mode types.CostMode, config types.CostRuleConfigV1) {
	t.Helper()
	require.NoError(t, model.DB.Create(&model.Channel{
		Id: channelID, Type: channelType, Name: "task supplier", Key: "secret",
	}).Error)
	normalized, err := service.NormalizeCostRuleConfig(mode, config)
	require.NoError(t, err)
	configJSON, err := common.Marshal(normalized)
	require.NoError(t, err)
	now := common.GetTimestamp()
	require.NoError(t, model.DB.Create(&model.ChannelModelCostRule{
		ChannelID: channelID, BillableUpstreamModel: modelName, Version: 1,
		Status: string(types.CostRuleActive), CostMode: string(mode), SchemaVersion: 1,
		ConfigJSON: string(configJSON), Source: "manual", EffectiveFrom: &now,
		CreatedAt: now, UpdatedAt: now,
	}).Error)
	previousLookup := service.CostCapabilityLookup
	service.CostCapabilityLookup = CostCapabilitiesForRoute
	service.InvalidateCostCoverage(channelID, modelName)
	t.Cleanup(func() {
		service.CostCapabilityLookup = previousLookup
		service.InvalidateCostCoverage(channelID, modelName)
	})
}

func setupTaskCostSubmitDB(t *testing.T) {
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
