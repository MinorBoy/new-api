package relay

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	relaytypes "github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/cost_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestConfirmCostIdentityUsesFinalOverriddenModel(t *testing.T) {
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "mapped-model"}}
	contract := jsonModelCostContract()
	err := contract.ConfirmCostIdentity(info, []byte(`{"model":"final-override-model"}`))
	require.NoError(t, err)
	assert.Equal(t, "final-override-model", info.BillableUpstreamModel)
}

func TestConfirmCostIdentityFallsBackToMappedModel(t *testing.T) {
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "mapped-model"}}

	err := jsonModelCostContract().ConfirmCostIdentity(info, []byte(`{"messages":[]}`))
	require.NoError(t, err)
	assert.Equal(t, "mapped-model", info.BillableUpstreamModel)
}

func TestCostCapabilitiesExcludeUnsupportedRealtimePath(t *testing.T) {
	covered := CostCapabilitiesForRoute(1, "/v1/chat/completions", "")
	assert.True(t, covered.CanResolveBillableModel)
	assert.Contains(t, covered.ChargeEvents, types.CostChargeResponseSucceeded)
	assert.Contains(t, covered.MeterSources, types.CostMeterUpstreamUsage)

	uncovered := CostCapabilitiesForRoute(1, "/v1/realtime", "")
	assert.False(t, uncovered.CanResolveBillableModel)
	assert.Empty(t, uncovered.ChargeEvents)
	assert.Empty(t, uncovered.MeterSources)
}

func TestTaskCostCapabilitiesAreRegisteredPerPlatform(t *testing.T) {
	tests := []struct {
		name     string
		channel  int
		expected []types.CostMeterSource
	}{
		{name: "new api video", channel: constant.ChannelTypeNewAPIVideo, expected: []types.CostMeterSource{
			types.CostMeterValidatedRequest, types.CostMeterUpstreamActual, types.CostMeterUpstreamUsage,
		}},
		{name: "lucen video", channel: constant.ChannelTypeLucen, expected: []types.CostMeterSource{
			types.CostMeterValidatedRequest, types.CostMeterUpstreamActual, types.CostMeterUpstreamUsage,
		}},
		{name: "megabyai video", channel: constant.ChannelTypeMegaByAI, expected: []types.CostMeterSource{
			types.CostMeterValidatedRequest, types.CostMeterUpstreamActual, types.CostMeterUpstreamUsage,
		}},
		{name: "cangyuan video", channel: constant.ChannelTypeCangyuan, expected: []types.CostMeterSource{
			types.CostMeterValidatedRequest, types.CostMeterUpstreamActual, types.CostMeterUpstreamUsage,
		}},
		{name: "paipu video", channel: constant.ChannelTypePaipu, expected: []types.CostMeterSource{
			types.CostMeterValidatedRequest, types.CostMeterUpstreamActual, types.CostMeterUpstreamUsage,
		}},
		{name: "secure video", channel: constant.ChannelTypeSecure, expected: []types.CostMeterSource{
			types.CostMeterValidatedRequest, types.CostMeterUpstreamActual, types.CostMeterUpstreamUsage,
		}},
		{name: "omegaai video", channel: constant.ChannelTypeOmegaAI, expected: []types.CostMeterSource{
			types.CostMeterValidatedRequest, types.CostMeterUpstreamActual, types.CostMeterUpstreamUsage,
		}},
		{name: "4stoken video", channel: constant.ChannelTypeFourSToken, expected: []types.CostMeterSource{
			types.CostMeterValidatedRequest, types.CostMeterUpstreamActual, types.CostMeterUpstreamUsage,
		}},
		{name: "8yes video", channel: constant.ChannelTypeEightYes, expected: []types.CostMeterSource{
			types.CostMeterValidatedRequest, types.CostMeterUpstreamActual, types.CostMeterUpstreamUsage,
		}},
		{name: "z5api video", channel: constant.ChannelTypeZ5API, expected: []types.CostMeterSource{
			types.CostMeterValidatedRequest, types.CostMeterUpstreamActual, types.CostMeterUpstreamUsage,
		}},
		{name: "zzone video", channel: constant.ChannelTypeZZone, expected: []types.CostMeterSource{
			types.CostMeterValidatedRequest, types.CostMeterUpstreamActual, types.CostMeterUpstreamUsage,
		}},
		{name: "mikoto video", channel: constant.ChannelTypeMikoto, expected: []types.CostMeterSource{
			types.CostMeterValidatedRequest, types.CostMeterUpstreamActual, types.CostMeterUpstreamUsage,
		}},
		{name: "doubao usage", channel: constant.ChannelTypeDoubaoVideo, expected: []types.CostMeterSource{
			types.CostMeterUpstreamUsage,
		}},
		{name: "dimensio validated duration", channel: constant.ChannelTypeDimensio, expected: []types.CostMeterSource{
			types.CostMeterValidatedRequest,
		}},
		{name: "kling per request only", channel: constant.ChannelTypeKling},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			capabilities := CostCapabilitiesForRoute(
				test.channel,
				"/v1/video/generations",
				constant.TaskPlatform(strconv.Itoa(test.channel)),
			)

			assert.True(t, capabilities.CanResolveBillableModel)
			assert.ElementsMatch(t, test.expected, capabilities.MeterSources)
		})
	}
}

func TestNormalizeCostMeterRequiresAuthoritativeBillingUsage(t *testing.T) {
	contract := jsonModelCostContract()
	info := &relaycommon.RelayInfo{}

	_, err := contract.NormalizeCostMeter(info, &dto.Usage{
		BillingUsage: &dto.BillingUsage{
			Estimated:   true,
			OpenAIUsage: &dto.Usage{PromptTokens: 10, CompletionTokens: 2, TotalTokens: 12},
		},
	})
	require.Error(t, err)

	meter, err := contract.NormalizeCostMeter(info, &dto.Usage{
		BillingUsage: dto.NewOpenAIChatBillingUsage(&dto.Usage{PromptTokens: 10, CompletionTokens: 2, TotalTokens: 12}),
	})
	require.NoError(t, err)
	require.NotNil(t, meter.InputTokens)
	require.NotNil(t, meter.OutputTokens)
	require.NotNil(t, meter.TotalTokens)
	assert.Equal(t, int64(10), *meter.InputTokens)
	assert.Equal(t, int64(2), *meter.OutputTokens)
	assert.Equal(t, int64(12), *meter.TotalTokens)
}

func TestNormalizeCostMeterPreservesZeroAndRejectsOutOfRangeTokens(t *testing.T) {
	contract := jsonModelCostContract()

	meter, err := contract.NormalizeCostMeter(&relaycommon.RelayInfo{}, &dto.Usage{
		BillingUsage: &dto.BillingUsage{OpenAIUsage: &dto.Usage{}},
	})
	require.NoError(t, err)
	require.NotNil(t, meter.TotalTokens)
	assert.Zero(t, *meter.TotalTokens)

	_, err = contract.NormalizeCostMeter(&relaycommon.RelayInfo{}, &dto.Usage{
		BillingUsage: dto.NewOpenAIChatBillingUsage(&dto.Usage{
			PromptTokens: relaycommon.MaxTokensLimit + 1,
			TotalTokens:  relaycommon.MaxTokensLimit + 1,
		}),
	})
	require.Error(t, err)
}

func TestPerRequestCostWaitsForProtocolSuccess(t *testing.T) {
	outcome := jsonModelCostContract().ClassifyCostOutcome(&relaycommon.RelayInfo{
		CostAttempt: &types.CostAttemptHandle{CostMode: types.CostModePerRequest},
	}, &http.Response{StatusCode: http.StatusOK}, nil)

	assert.Equal(t, types.CostAttemptAwaitingMeter, outcome.Status)
}

func TestProviderContractCanExplicitlyConfirmNoCharge(t *testing.T) {
	wrapped := &costAccountingAdaptor{contract: &knownNoChargeCostContract{jsonModelCostContract()}}
	outcome := wrapped.ClassifyCostOutcome(&relaycommon.RelayInfo{}, &http.Response{StatusCode: http.StatusBadRequest}, nil)

	assert.Equal(t, types.CostAttemptConfirmedZero, outcome.Status)
}

func TestStrictCostAdaptorRejectsEmptyIdentityBeforeTransport(t *testing.T) {
	withCostAccountingMode(t, types.CostAccountingStrict)
	fake := &costTransportAdaptor{}
	wrapped := newCostAccountingAdaptor(fake, 1)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())

	_, err := wrapped.DoRequest(ctx, &relaycommon.RelayInfo{}, bytes.NewReader([]byte(`{}`)))
	require.Error(t, err)
	assert.False(t, fake.called)
}

func TestDisabledCostAdaptorPreservesExistingTransport(t *testing.T) {
	withCostAccountingMode(t, types.CostAccountingDisabled)
	fake := &costTransportAdaptor{}
	wrapped := newCostAccountingAdaptor(fake, 1)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())

	_, err := wrapped.DoRequest(ctx, &relaycommon.RelayInfo{}, bytes.NewReader([]byte(`{}`)))
	require.NoError(t, err)
	assert.True(t, fake.called)
}

func TestTrackingCostAdaptorRecordsCoveredRequest(t *testing.T) {
	fake := &costTransportAdaptor{}
	wrapped, ctx, info := prepareStrictPerRequestCostRelay(t, "relay-tracking-covered", fake)
	withCostAccountingMode(t, types.CostAccountingTracking)

	response, err := wrapped.DoRequest(ctx, info, bytes.NewReader([]byte(`{}`)))

	require.NoError(t, err)
	assert.True(t, fake.called)
	require.NotNil(t, info.CostAttempt)
	_, apiErr := wrapped.DoResponse(ctx, response.(*http.Response), info)
	require.Nil(t, apiErr)
	assert.Equal(t, string(types.CostAttemptSettled), loadRelayCostAttempt(t, info.CostAttempt.AttemptID).Status)
}

func TestTrackingCostAdaptorPreservesUncoveredTransport(t *testing.T) {
	fake := &costTransportAdaptor{}
	wrapped, ctx, info := prepareStrictPerRequestCostRelay(t, "relay-tracking-uncovered", fake)
	withCostAccountingMode(t, types.CostAccountingTracking)
	require.NoError(t, model.DB.Where("channel_id = ?", info.ChannelId).Delete(&model.ChannelModelCostRule{}).Error)
	service.InvalidateCostCoverage(info.ChannelId, info.BillableUpstreamModel, "")

	response, err := wrapped.DoRequest(ctx, info, bytes.NewReader([]byte(`{}`)))

	require.NoError(t, err)
	require.NotNil(t, response)
	assert.True(t, fake.called)
	assert.Nil(t, info.CostAttempt)
	var requestCount int64
	require.NoError(t, model.DB.Model(&model.CostAccountingRequest{}).Where("request_id = ?", info.RequestId).Count(&requestCount).Error)
	assert.Zero(t, requestCount)
}

func TestStrictCostAdaptorPersistsDispatchAuthorizationBeforeTransport(t *testing.T) {
	statusDuringSend := ""
	fake := &costTransportAdaptor{onRequest: func(info *relaycommon.RelayInfo) {
		var attempt model.CostAccountingAttempt
		require.NoError(t, model.DB.First(&attempt, info.CostAttempt.AttemptID).Error)
		statusDuringSend = attempt.Status
	}}
	wrapped, ctx, info := prepareStrictPerRequestCostRelay(t, "relay-cost-dispatch", fake)

	response, err := wrapped.DoRequest(ctx, info, bytes.NewReader([]byte(`{}`)))
	require.NoError(t, err)
	assert.True(t, fake.called)
	assert.Equal(t, string(types.CostAttemptDispatching), statusDuringSend)
	assert.Equal(t, string(types.CostAttemptAwaitingMeter), loadRelayCostAttempt(t, info.CostAttempt.AttemptID).Status)
	_, apiErr := wrapped.DoResponse(ctx, response.(*http.Response), info)
	require.Nil(t, apiErr)
	assert.Equal(t, string(types.CostAttemptSettled), loadRelayCostAttempt(t, info.CostAttempt.AttemptID).Status)
}

func TestStrictCostAdaptorRejectsSnapshotChangedBeforePrepare(t *testing.T) {
	fake := &costTransportAdaptor{}
	wrapped, ctx, info := prepareStrictPerRequestCostRelay(t, "relay-cost-snapshot-changed", fake)

	var oldRule model.ChannelModelCostRule
	require.NoError(t, model.DB.Where("channel_id = ? AND billable_upstream_model = ? AND version = ?", info.ChannelId, info.BillableUpstreamModel, 1).First(&oldRule).Error)
	unitPrice := "100"
	newConfig, err := service.NormalizeCostRuleConfig(types.CostModePerRequest, types.CostRuleConfigV1{
		Currency: "USD", BillingMultiplier: "1", PurchaseDiscountRatio: "1",
		RechargeExchangeRatio: "1", FeeRate: "0", CurrencyToUSDRate: "1",
		UnitPrice: &unitPrice, ChargeEvent: types.CostChargeResponseSucceeded,
	})
	require.NoError(t, err)
	newConfigJSON, err := common.Marshal(newConfig)
	require.NoError(t, err)
	now := common.GetTimestamp()
	newRule := model.ChannelModelCostRule{
		ChannelID: info.ChannelId, BillableUpstreamModel: info.BillableUpstreamModel, CostVariantKey: string(types.DefaultCostVariantKey), Version: 2,
		Status: string(types.CostRuleDraft), CostMode: string(types.CostModePerRequest), SchemaVersion: 1,
		ConfigJSON: string(newConfigJSON), Source: "manual", CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, model.DB.Create(&newRule).Error)
	ruleQueries := switchActiveCostRuleBeforePrepare(t, oldRule, newRule, now)

	_, err = wrapped.DoRequest(ctx, info, bytes.NewReader([]byte(`{}`)))

	assert.Equal(t, int32(2), ruleQueries.Load(), "PrepareCostAttempt must lock the rule switched after recheck")
	require.Error(t, err)
	require.ErrorIs(t, err, service.ErrProfitEligibility)
	var coverageErr *service.CostCoverageError
	require.ErrorAs(t, err, &coverageErr)
	assert.Equal(t, info.ChannelId, coverageErr.ChannelID)
	assert.Nil(t, info.CostAttempt, "snapshot rejection must occur before authorization")
	assert.False(t, fake.called, "snapshot rejection must not call upstream")
	var requestCount int64
	require.NoError(t, model.DB.Model(&model.CostAccountingRequest{}).Where("request_id = ?", info.RequestId).Count(&requestCount).Error)
	assert.Zero(t, requestCount, "snapshot rejection must not create a cost request")
	var attemptCount int64
	require.NoError(t, model.DB.Model(&model.CostAccountingAttempt{}).Where("cost_request_id IN (SELECT id FROM cost_accounting_requests WHERE request_id = ?)", info.RequestId).Count(&attemptCount).Error)
	assert.Zero(t, attemptCount, "snapshot rejection must not create a cost attempt")
}

func TestPerRequestProtocolErrorDoesNotConfirmCost(t *testing.T) {
	fake := &costTransportAdaptor{responseErr: relaytypes.NewError(errors.New("invalid upstream response"), relaytypes.ErrorCodeBadResponse)}
	wrapped, ctx, info := prepareStrictPerRequestCostRelay(t, "relay-cost-protocol-error", fake)

	response, err := wrapped.DoRequest(ctx, info, bytes.NewReader([]byte(`{}`)))
	require.NoError(t, err)
	_, apiErr := wrapped.DoResponse(ctx, response.(*http.Response), info)
	require.NotNil(t, apiErr)
	attempt := loadRelayCostAttempt(t, info.CostAttempt.AttemptID)
	assert.Equal(t, string(types.CostAttemptUnknown), attempt.Status)
	assert.Nil(t, attempt.CostNanoUSD)
}

func TestInFlightCostAttemptSettlesAfterModeDisabled(t *testing.T) {
	fake := &costTransportAdaptor{}
	wrapped, ctx, info := prepareStrictPerRequestCostRelay(t, "relay-cost-mode-toggle", fake)
	response, err := wrapped.DoRequest(ctx, info, bytes.NewReader([]byte(`{}`)))
	require.NoError(t, err)

	cfg := config.GlobalConfig.Get(cost_setting.ConfigName)
	require.NoError(t, config.UpdateConfigFromMap(cfg, map[string]string{cost_setting.KeyMode: string(types.CostAccountingDisabled)}))
	cost_setting.UpdateAndSync()
	_, apiErr := wrapped.DoResponse(ctx, response.(*http.Response), info)
	require.Nil(t, apiErr)
	assert.Equal(t, string(types.CostAttemptSettled), loadRelayCostAttempt(t, info.CostAttempt.AttemptID).Status)
	var request model.CostAccountingRequest
	require.NoError(t, model.DB.First(&request, info.CostRequestID).Error)
	require.NotNil(t, request.WinningAttemptID)
	assert.Equal(t, info.CostAttempt.AttemptID, *request.WinningAttemptID)
}

func prepareStrictPerRequestCostRelay(t *testing.T, requestID string, fake *costTransportAdaptor) (*costAccountingAdaptor, *gin.Context, *relaycommon.RelayInfo) {
	t.Helper()
	withCostAccountingMode(t, types.CostAccountingStrict)
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
		&model.CostAccountingAttempt{}, &model.CostAccountingAudit{},
	))
	const channelID = 700007
	require.NoError(t, model.DB.Create(&model.Channel{Id: channelID, Type: 1, Name: "supplier", Key: "secret"}).Error)

	unitPrice := "0.2"
	ruleConfig, err := service.NormalizeCostRuleConfig(types.CostModePerRequest, types.CostRuleConfigV1{
		Currency: "USD", BillingMultiplier: "1", PurchaseDiscountRatio: "1",
		RechargeExchangeRatio: "1", FeeRate: "0", CurrencyToUSDRate: "1",
		UnitPrice: &unitPrice, ChargeEvent: types.CostChargeResponseSucceeded,
	})
	require.NoError(t, err)
	configJSON, err := common.Marshal(ruleConfig)
	require.NoError(t, err)
	now := common.GetTimestamp()
	require.NoError(t, model.DB.Create(&model.ChannelModelCostRule{
		ChannelID: channelID, BillableUpstreamModel: "vendor-model", CostVariantKey: string(types.DefaultCostVariantKey), Version: 1,
		Status: string(types.CostRuleActive), CostMode: string(types.CostModePerRequest), SchemaVersion: 1,
		ConfigJSON: string(configJSON), Source: "manual", EffectiveFrom: &now,
		CreatedAt: now, UpdatedAt: now,
	}).Error)
	previousLookup := service.CostCapabilityLookup
	service.CostCapabilityLookup = CostCapabilitiesForRoute
	service.InvalidateCostCoverage(channelID, "vendor-model", "")
	t.Cleanup(func() {
		service.CostCapabilityLookup = previousLookup
		service.InvalidateCostCoverage(channelID, "vendor-model", "")
	})

	wrapped := newCostAccountingAdaptor(fake, 1)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Set("channel_name", "supplier")
	info := &relaycommon.RelayInfo{
		RequestId: requestID, OriginModelName: "client-model",
		PredictedUpstreamModel: "vendor-model", BillableUpstreamModel: "vendor-model",
		RequestURLPath: "/v1/chat/completions",
		ChannelMeta:    &relaycommon.ChannelMeta{ChannelId: channelID, ChannelType: 1},
	}
	return wrapped, ctx, info
}

func withCostAccountingMode(t *testing.T, mode types.CostAccountingMode) {
	t.Helper()
	cfg := config.GlobalConfig.Get(cost_setting.ConfigName)
	require.NotNil(t, cfg)
	require.NoError(t, config.UpdateConfigFromMap(cfg, map[string]string{cost_setting.KeyMode: string(mode)}))
	cost_setting.UpdateAndSync()
	previousRevenueHook := service.RevenuePreviewHookForTest()
	if mode == types.CostAccountingStrict {
		service.SetRoutingRevenuePreview(func(_ context.Context, _ service.RoutingRevenuePreviewInput) (int64, string, error) {
			return 1_000_000, "500000", nil
		})
	}
	t.Cleanup(func() {
		require.NoError(t, config.UpdateConfigFromMap(cfg, map[string]string{cost_setting.KeyMode: string(types.CostAccountingDisabled)}))
		cost_setting.UpdateAndSync()
		service.SetRoutingRevenuePreview(previousRevenueHook)
	})
}

type costTransportAdaptor struct {
	channel.Adaptor
	called      bool
	onRequest   func(info *relaycommon.RelayInfo)
	responseErr *relaytypes.NewAPIError
}

func (a *costTransportAdaptor) DoResponse(_ *gin.Context, _ *http.Response, _ *relaycommon.RelayInfo) (any, *relaytypes.NewAPIError) {
	return nil, a.responseErr
}

func (a *costTransportAdaptor) DoRequest(_ *gin.Context, info *relaycommon.RelayInfo, _ io.Reader) (any, error) {
	a.called = true
	if a.onRequest != nil {
		a.onRequest(info)
	}
	return &http.Response{StatusCode: http.StatusOK}, nil
}

func loadRelayCostAttempt(t *testing.T, id int64) model.CostAccountingAttempt {
	t.Helper()
	var attempt model.CostAccountingAttempt
	require.NoError(t, model.DB.First(&attempt, id).Error)
	return attempt
}

var _ channel.Adaptor = (*costTransportAdaptor)(nil)

type knownNoChargeCostContract struct {
	*jsonCostAccountingContract
}

func (c *knownNoChargeCostContract) ClassifyCostOutcome(_ *relaycommon.RelayInfo, _ *http.Response, _ error) types.CostOutcome {
	return types.CostOutcome{Status: types.CostAttemptConfirmedZero}
}

var _ channel.CostAccountingAdaptor = (*knownNoChargeCostContract)(nil)
