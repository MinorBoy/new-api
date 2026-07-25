package controller

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/cost_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestCostAccountingReconcileBindingPreservesMissingAndExplicitZero(t *testing.T) {
	tests := []struct {
		name          string
		body          string
		meterMissing  bool
		durationValue *string
		totalTokens   *int64
	}{
		{
			name:         "meter missing",
			body:         `{"action":"settle","reason":"supplier invoice"}`,
			meterMissing: true,
		},
		{
			name:          "explicit zero values",
			body:          `{"action":"settle","meter":{"source":"upstream_actual","duration_seconds":"0","total_tokens":0},"reason":"supplier invoice"}`,
			durationValue: costControllerStringPointer("0"),
			totalTokens:   costControllerInt64Pointer(0),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest("POST", "/", strings.NewReader(tt.body))
			ctx.Request.Header.Set("Content-Type", "application/json")

			request, err := bindReconcileCostAttemptRequest(ctx)
			require.NoError(t, err)
			if tt.meterMissing {
				assert.Nil(t, request.Meter)
				return
			}
			require.NotNil(t, request.Meter)
			assert.Equal(t, tt.durationValue, request.Meter.DurationSeconds)
			assert.Equal(t, tt.totalTokens, request.Meter.TotalTokens)
		})
	}
}

func TestCostAccountingRevenueReconcileBindingPreservesExplicitZero(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"action":"settle","final_user_quota":0,"reason":"billing receipt"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	request, err := bindReconcileCostRevenueRequest(ctx)
	require.NoError(t, err)
	require.NotNil(t, request.FinalUserQuota)
	assert.Zero(t, *request.FinalUserQuota)
}

func TestCostAccountingRevenueReconcileEndpointRepairsFailedRevenue(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	previousDB := model.DB
	previousLogDB := model.LOG_DB
	previousRedisEnabled := common.RedisEnabled
	previousMainDatabaseType := common.MainDatabaseType()
	previousLogDatabaseType := common.LogDatabaseType()
	model.DB = db
	model.LOG_DB = db
	common.RedisEnabled = false
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		model.DB = previousDB
		model.LOG_DB = previousLogDB
		common.RedisEnabled = previousRedisEnabled
		common.SetDatabaseTypes(previousMainDatabaseType, previousLogDatabaseType)
		require.NoError(t, sqlDB.Close())
	})
	require.NoError(t, db.AutoMigrate(
		&model.CostAccountingRequest{}, &model.CostAccountingAttempt{}, &model.CostAccountingAudit{},
		&model.User{}, &model.Log{},
	))
	now := common.GetTimestamp()
	request := model.CostAccountingRequest{
		RequestID: "controller-revenue-reconcile", QuotaPerUnitSnapshot: "500000",
		RevenueStatus: string(types.CostRevenueFailed), ProfitStatus: string(types.CostProfitIncompleteRevenue),
		FailureCode: "revenue_settlement_failed", RequestedAt: now, CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, db.Create(&request).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: strconv.FormatInt(request.ID, 10)}}
	ctx.Set("id", 92)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/cost-accounting/requests/1/reconcile-revenue", strings.NewReader(`{"action":"settle","final_user_quota":200,"reason":"billing receipt"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ReconcileCostRevenue(ctx)

	assert.Equal(t, http.StatusOK, recorder.Code)
	require.NoError(t, db.First(&request, request.ID).Error)
	assert.Equal(t, string(types.CostRevenueSettled), request.RevenueStatus)
	require.NotNil(t, request.BilledRevenueEquivalentNanoUSD)
	assert.Equal(t, int64(400_000), *request.BilledRevenueEquivalentNanoUSD)
	var auditCount int64
	require.NoError(t, db.Model(&model.CostAccountingAudit{}).Where("cost_request_id = ?", request.ID).Count(&auditCount).Error)
	assert.Equal(t, int64(1), auditCount)
}

func TestCostPreviewResponseSerializesNanoUSDAsStrings(t *testing.T) {
	margin := int64(-500_000)
	response := costPreviewResponse(service.CostPreview{
		Estimated:      true,
		OriginalCost:   "0.3",
		RevenueNanoUSD: 200_000_000,
		CostNanoUSD:    300_000_000,
		ProfitNanoUSD:  -100_000_000,
		MarginPPM:      &margin,
	})

	assert.Equal(t, "200000000", response.RevenueNanoUSD)
	assert.Equal(t, "300000000", response.CostNanoUSD)
	assert.Equal(t, "-100000000", response.ProfitNanoUSD)
	require.NotNil(t, response.MarginPPM)
	assert.Equal(t, "-500000", *response.MarginPPM)
}

func TestCostReportResponsesSerializeNanoUSDAsStrings(t *testing.T) {
	margin := int64(-500_000)
	summary := costProfitSummaryResponse(service.CostProfitSummary{
		RealizedRevenueNanoUSD: 200_000_000, RealizedCostNanoUSD: 300_000_000,
		RealizedProfitNanoUSD: -100_000_000, GrossMarginPPM: &margin,
		KnownIncompleteCostNanoUSD: 50_000_000,
	})
	assert.Equal(t, "200000000", summary.RealizedRevenueNanoUSD)
	assert.Equal(t, "300000000", summary.RealizedCostNanoUSD)
	assert.Equal(t, "-100000000", summary.RealizedProfitNanoUSD)
	assert.Equal(t, "50000000", summary.KnownIncompleteCostNanoUSD)
	require.NotNil(t, summary.GrossMarginPPM)
	assert.Equal(t, "-500000", *summary.GrossMarginPPM)

	rows := costProfitBreakdownResponses([]service.CostProfitBreakdownRow{{
		ChannelID: 7, BillableUpstreamModel: "vendor-model",
		RealizedRevenueNanoUSD: 200_000_000, RealizedCostNanoUSD: 300_000_000,
		RealizedProfitNanoUSD: -100_000_000, KnownIncompleteCostNanoUSD: 50_000_000,
	}})
	require.Len(t, rows, 1)
	assert.Equal(t, "200000000", rows[0].RealizedRevenueNanoUSD)
	assert.Equal(t, "300000000", rows[0].RealizedCostNanoUSD)
	assert.Equal(t, "-100000000", rows[0].RealizedProfitNanoUSD)
}

func TestCostRequestDetailResponseSerializesLedgerNanoUSDAsStrings(t *testing.T) {
	revenue := int64(1_000_000_000)
	profit := int64(700_000_000)
	cost := int64(300_000_000)
	oldAmount := int64(100_000_000)
	response := costRequestDetailResponse(service.CostRequestDetail{
		Request: model.CostAccountingRequest{
			ID: 1, BilledRevenueEquivalentNanoUSD: &revenue,
			ConfirmedCostNanoUSD: cost, BilledGrossProfitNanoUSD: &profit,
		},
		Attempts: []service.CostRequestAttemptDetail{{
			Attempt: model.CostAccountingAttempt{ID: 2, CostNanoUSD: &cost}, Winning: true,
		}},
		Audits: []model.CostAccountingAudit{{
			ID: 3, OldAmountNanoUSD: &oldAmount, NewAmountNanoUSD: &cost,
		}},
	})
	raw, err := common.Marshal(response)
	require.NoError(t, err)
	var decoded struct {
		Request struct {
			Revenue *string `json:"billed_revenue_equivalent_nano_usd"`
			Cost    string  `json:"confirmed_cost_nano_usd"`
			Profit  *string `json:"billed_gross_profit_nano_usd"`
		} `json:"request"`
		Attempts []struct {
			Attempt struct {
				Cost *string `json:"cost_nano_usd"`
			} `json:"attempt"`
		} `json:"attempts"`
		Audits []struct {
			Old *string `json:"old_amount_nano_usd"`
			New *string `json:"new_amount_nano_usd"`
		} `json:"audits"`
	}
	require.NoError(t, common.Unmarshal(raw, &decoded))
	assert.Equal(t, costControllerStringPointer("1000000000"), decoded.Request.Revenue)
	assert.Equal(t, "300000000", decoded.Request.Cost)
	assert.Equal(t, costControllerStringPointer("700000000"), decoded.Request.Profit)
	require.Len(t, decoded.Attempts, 1)
	assert.Equal(t, costControllerStringPointer("300000000"), decoded.Attempts[0].Attempt.Cost)
	require.Len(t, decoded.Audits, 1)
	assert.Equal(t, costControllerStringPointer("100000000"), decoded.Audits[0].Old)
	assert.Equal(t, costControllerStringPointer("300000000"), decoded.Audits[0].New)
}

func TestCostCoverageResponseKeepsPredictedUpstreamModelContract(t *testing.T) {
	response := costCoverageResponse(service.CostCoverageRow{
		ChannelID: 7, OriginModel: "client-model", BillableUpstreamModel: "vendor-model", Covered: true,
	})
	assert.Equal(t, 7, response.ChannelID)
	assert.Equal(t, "client-model", response.OriginModel)
	assert.Equal(t, "vendor-model", response.PredictedUpstreamModel)
	assert.True(t, response.Covered)
}

func TestCostAccountingStrictModeRejectsIncompleteCoverage(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	previousDB := model.DB
	previousLookup := service.CostCapabilityLookup
	model.DB = db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	service.CostCapabilityLookup = func(int, string, constant.TaskPlatform) types.CostCapabilities {
		return types.CostCapabilities{CanResolveBillableModel: true}
	}
	t.Cleanup(func() {
		model.DB = previousDB
		service.CostCapabilityLookup = previousLookup
		service.InvalidateCostCoverage(0, "")
		require.NoError(t, sqlDB.Close())
	})
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.Option{}, &model.ChannelModelCostRule{}))
	require.NoError(t, db.Create(&model.Channel{Id: 7, Type: 1, Name: "supplier", Key: "secret", Status: common.ChannelStatusEnabled}).Error)
	require.NoError(t, db.Create(&model.Ability{Group: "default", Model: "client-model", ChannelId: 7, Enabled: true}).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/cost-accounting/settings", strings.NewReader(`{"mode":"strict","minimum_expected_margin_bps":0}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	UpdateCostAccountingSettings(ctx)

	assert.Equal(t, http.StatusConflict, recorder.Code)
	var response struct {
		Code string `json:"code"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, "cost_coverage_incomplete", response.Code)
}

func TestCostAccountingSettingsUpdatePreservesExplicitZero(t *testing.T) {
	prepareCostAccountingSettingsControllerTest(t)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/cost-accounting/settings", strings.NewReader(`{"mode":"strict","minimum_expected_margin_bps":0}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	UpdateCostAccountingSettings(ctx)

	assert.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Data struct {
			Mode                     types.CostAccountingMode `json:"mode"`
			MinimumExpectedMarginBPS int                      `json:"minimum_expected_margin_bps"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, types.CostAccountingStrict, response.Data.Mode)
	assert.Zero(t, response.Data.MinimumExpectedMarginBPS)
	assert.Equal(t, response.Data.Mode, cost_setting.Runtime().Mode)
	assert.Equal(t, response.Data.MinimumExpectedMarginBPS, cost_setting.Runtime().MinimumExpectedMarginBPS)
}

func TestCostAccountingSettingsInvalidMarginDoesNotUpdateEitherOption(t *testing.T) {
	prepareCostAccountingSettingsControllerTest(t)
	require.NoError(t, model.UpdateOptionsBulk(map[string]string{
		cost_setting.ConfigName + "." + cost_setting.KeyMode:                     string(types.CostAccountingDisabled),
		cost_setting.ConfigName + "." + cost_setting.KeyMinimumExpectedMarginBPS: "375",
	}))

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/cost-accounting/settings", strings.NewReader(`{"mode":"strict","minimum_expected_margin_bps":10001}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	UpdateCostAccountingSettings(ctx)

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	var options []model.Option
	require.NoError(t, model.DB.Where("key IN ?", []string{
		cost_setting.ConfigName + "." + cost_setting.KeyMode,
		cost_setting.ConfigName + "." + cost_setting.KeyMinimumExpectedMarginBPS,
	}).Find(&options).Error)
	require.Len(t, options, 2)
	values := make(map[string]string, len(options))
	for _, option := range options {
		values[option.Key] = option.Value
	}
	assert.Equal(t, string(types.CostAccountingDisabled), values[cost_setting.ConfigName+"."+cost_setting.KeyMode])
	assert.Equal(t, "375", values[cost_setting.ConfigName+"."+cost_setting.KeyMinimumExpectedMarginBPS])
}

func prepareCostAccountingSettingsControllerTest(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	previousDB := model.DB
	previousLogDB := model.LOG_DB
	previousRuntime := cost_setting.Runtime()
	previousMainType := common.MainDatabaseType()
	previousLogType := common.LogDatabaseType()
	previousRedisEnabled := common.RedisEnabled
	common.OptionMapRWMutex.Lock()
	previousOptionMap := common.OptionMap
	common.OptionMap = map[string]string{}
	common.OptionMapRWMutex.Unlock()
	model.DB = db
	model.LOG_DB = db
	common.RedisEnabled = false
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	require.NoError(t, db.AutoMigrate(
		&model.Channel{}, &model.Ability{}, &model.Option{}, &model.ChannelModelCostRule{},
		&model.User{}, &model.Log{},
	))
	t.Cleanup(func() {
		require.NoError(t, model.UpdateOptionsBulk(map[string]string{
			cost_setting.ConfigName + "." + cost_setting.KeyMode:                     string(previousRuntime.Mode),
			cost_setting.ConfigName + "." + cost_setting.KeyMinimumExpectedMarginBPS: strconv.Itoa(previousRuntime.MinimumExpectedMarginBPS),
		}))
		model.DB = previousDB
		model.LOG_DB = previousLogDB
		common.RedisEnabled = previousRedisEnabled
		common.SetDatabaseTypes(previousMainType, previousLogType)
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptionMap
		common.OptionMapRWMutex.Unlock()
		service.InvalidateCostCoverage(0, "")
		require.NoError(t, sqlDB.Close())
	})
}

func costControllerStringPointer(value string) *string {
	return &value
}

func costControllerInt64Pointer(value int64) *int64 {
	return &value
}
