package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
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
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/cost-accounting/settings", strings.NewReader(`{"mode":"strict"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	UpdateCostAccountingSettings(ctx)

	assert.Equal(t, http.StatusConflict, recorder.Code)
	var response struct {
		Code string `json:"code"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, "cost_coverage_incomplete", response.Code)
}

func costControllerStringPointer(value string) *string {
	return &value
}

func costControllerInt64Pointer(value int64) *int64 {
	return &value
}
