package service

import (
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelrouting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestAppendRoutingAdminInfoFromContextIncludesProfitDiagnostics(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	diagnostic := ProfitRoutingDiagnostic{
		ChannelID:                17,
		BillableUpstreamModel:    "vendor-video-model",
		EstimatedRevenueNanoUSD:  5_445_000_000,
		EstimatedCostNanoUSD:     common.GetPointer(int64(5_000_000_000)),
		EstimatedProfitNanoUSD:   common.GetPointer(int64(445_000_000)),
		GrossMarginPPM:           common.GetPointer(int64(81_726)),
		MinimumExpectedMarginBPS: 1_000,
		RuleID:                   88,
		RuleVersion:              3,
		Reason:                   ProfitReasonMarginBelowThreshold,
	}
	common.SetContextKey(c, constant.ContextKeyRoutingDiagnostics, []ProfitRoutingDiagnostic{diagnostic})
	common.SetContextKey(c, constant.ContextKeyRoutingFactsInput, modelrouting.FactsInput{
		ReferenceVideoURLs: []string{"https://assets.example/input.mp4?token=secret"},
	})

	other := map[string]interface{}{}
	AppendRoutingAdminInfoFromContext(c, other)

	adminInfo, ok := other["admin_info"].(map[string]interface{})
	require.True(t, ok)
	diagnostics, ok := adminInfo["routing_diagnostics"].([]ProfitRoutingDiagnostic)
	require.True(t, ok)
	require.Equal(t, []ProfitRoutingDiagnostic{diagnostic}, diagnostics)

	body, err := common.Marshal(other)
	require.NoError(t, err)
	assert.NotContains(t, string(body), "assets.example")
	assert.NotContains(t, string(body), "secret")
}

func TestRecordProfitExclusionsKeepsCompleteAdminDiagnostic(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	unitPrice := "9.1"
	config, err := NormalizeCostRuleConfig(types.CostModePerRequest, types.CostRuleConfigV1{
		Currency:              "USD",
		BillingMultiplier:     "1",
		PurchaseDiscountRatio: "1",
		RechargeExchangeRatio: "1",
		CurrencyToUSDRate:     "1",
		UnitPrice:             &unitPrice,
		ChargeEvent:           types.CostChargeResponseSucceeded,
	})
	require.NoError(t, err)
	configJSON, err := common.Marshal(config)
	require.NoError(t, err)

	candidate := ProfitRoutingCandidate{ChannelID: 17, PredictedUpstreamModel: "vendor-video-model"}
	rule := &model.ChannelModelCostRule{
		ID: 88, ChannelID: candidate.ChannelID, BillableUpstreamModel: candidate.PredictedUpstreamModel,
		Version: 3, CostMode: string(types.CostModePerRequest), SchemaVersion: 1, ConfigJSON: string(configJSON),
	}
	filterResult := FilterProfitEligibleChannels(ProfitChannelFilterInput{
		RevenueNanoUSD: nano("10"), HasRevenue: true, GlobalMarginBPS: 1_000,
		Candidates: []ProfitRoutingCandidate{candidate},
	}, map[CostRuleCandidate]*model.ChannelModelCostRule{
		{ChannelID: candidate.ChannelID, BillableUpstreamModel: candidate.PredictedUpstreamModel}: rule,
	})
	require.Len(t, filterResult.Exclusions, 1)

	param := &RetryParam{Ctx: c}
	recordProfitExclusions(param, groupRoutingResult{}, filterResult)

	diagnostics, ok := common.GetContextKeyType[[]ProfitRoutingDiagnostic](c, constant.ContextKeyRoutingDiagnostics)
	require.True(t, ok)
	require.Len(t, diagnostics, 1)
	diagnostic := diagnostics[0]
	assert.Equal(t, 17, diagnostic.ChannelID)
	assert.Equal(t, "vendor-video-model", diagnostic.BillableUpstreamModel)
	assert.Equal(t, nano("10"), diagnostic.EstimatedRevenueNanoUSD)
	require.NotNil(t, diagnostic.EstimatedCostNanoUSD)
	assert.Equal(t, nano("9.1"), *diagnostic.EstimatedCostNanoUSD)
	require.NotNil(t, diagnostic.EstimatedProfitNanoUSD)
	assert.Equal(t, nano("0.9"), *diagnostic.EstimatedProfitNanoUSD)
	require.NotNil(t, diagnostic.GrossMarginPPM)
	assert.Equal(t, int64(90_000), *diagnostic.GrossMarginPPM)
	assert.Equal(t, 1_000, diagnostic.MinimumExpectedMarginBPS)
	assert.Equal(t, int64(88), diagnostic.RuleID)
	assert.Equal(t, 3, diagnostic.RuleVersion)
	assert.Equal(t, ProfitReasonMarginBelowThreshold, diagnostic.Reason)
}

func TestRecordRoutingSelectionFailureUsesProfitAdminDiagnostics(t *testing.T) {
	previousLogDB := model.LOG_DB
	previousErrorLogEnabled := constant.ErrorLogEnabled
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	model.LOG_DB = db
	constant.ErrorLogEnabled = true
	t.Cleanup(func() {
		model.LOG_DB = previousLogDB
		constant.ErrorLogEnabled = previousErrorLogEnabled
	})

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/video/generations", nil)
	common.SetContextKey(c, constant.ContextKeyRoutingDiagnostics, []ProfitRoutingDiagnostic{{
		ChannelID:                17,
		BillableUpstreamModel:    "vendor-video-model",
		EstimatedRevenueNanoUSD:  nano("10"),
		EstimatedCostNanoUSD:     common.GetPointer(nano("9.1")),
		EstimatedProfitNanoUSD:   common.GetPointer(nano("0.9")),
		GrossMarginPPM:           common.GetPointer(int64(90_000)),
		MinimumExpectedMarginBPS: 1_000,
		RuleID:                   88,
		RuleVersion:              3,
		Reason:                   ProfitReasonMarginBelowThreshold,
	}})
	common.SetContextKey(c, constant.ContextKeyRoutingFactsInput, modelrouting.FactsInput{
		ReferenceVideoURLs: []string{"https://assets.example/input.mp4?token=secret"},
	})

	RecordRoutingSelectionFailure(c, modelrouting.Seedance20, &ChannelSelectionError{
		Code:       types.ErrorCodeCompatibleChannelUnavailable,
		StatusCode: 503,
		Err:        errors.New("compatible channel is unavailable"),
		Diagnostics: []modelrouting.Audit{{
			PolicyID: 7,
		}},
	})

	var log model.Log
	require.NoError(t, db.Last(&log).Error)
	assert.Equal(t, "compatible channel is unavailable", log.Content)
	assert.NotContains(t, log.Content, "vendor-video-model")
	assert.NotContains(t, log.Content, "90_000")
	assert.NotContains(t, log.Content, "secret")

	other, err := common.StrToMap(log.Other)
	require.NoError(t, err)
	adminInfo, ok := other["admin_info"].(map[string]interface{})
	require.True(t, ok)
	diagnostics, ok := adminInfo["routing_diagnostics"].([]interface{})
	require.True(t, ok)
	require.Len(t, diagnostics, 1)
	diagnostic, ok := diagnostics[0].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, float64(17), diagnostic["channel_id"])
	assert.Equal(t, "vendor-video-model", diagnostic["billable_upstream_model"])
	assert.Equal(t, string(ProfitReasonMarginBelowThreshold), diagnostic["reason"])
	assert.NotContains(t, log.Other, "assets.example")
	assert.NotContains(t, log.Other, "secret")
}
