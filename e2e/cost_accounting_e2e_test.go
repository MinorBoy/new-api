package e2e

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	appI18n "github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay"
	"github.com/QuantumNous/new-api/router"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/cost_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const (
	costE2EChatModel  = "cost-e2e-chat"
	costE2EChatUserID = 2001
	costE2EChatToken  = "cost"
)

type costE2EOpenAIProvider struct {
	mu              sync.Mutex
	status          int
	stream          bool
	requestCount    int
	requestQueries  []string
	clientCancelled chan struct{}
	cancelOnce      sync.Once
}

func (p *costE2EOpenAIProvider) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	p.requestCount++
	p.requestQueries = append(p.requestQueries, r.URL.RawQuery)
	p.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")

	status := p.status
	if status == 0 {
		status = http.StatusOK
	}
	if status != http.StatusOK {
		w.WriteHeader(status)
		_, _ = io.WriteString(w, `{"error":{"message":"provider unavailable","type":"server_error","code":"server_error"}}`)
		return
	}
	if !p.stream {
		_, _ = io.WriteString(w, `{"id":"chatcmpl-cost-e2e","object":"chat.completion","created":1784800000,"model":"cost-e2e-chat","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	_, _ = io.WriteString(w, "data: {\"id\":\"chatcmpl-cost-stream\",\"object\":\"chat.completion.chunk\",\"created\":1784800001,\"model\":\"cost-e2e-chat\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"first\"},\"finish_reason\":null}]}\n\n")
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
	_, _ = io.WriteString(w, "data: {\"id\":\"chatcmpl-cost-stream\",\"object\":\"chat.completion.chunk\",\"created\":1784800001,\"model\":\"cost-e2e-chat\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"second\"},\"finish_reason\":null}]}\n\n")
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
	select {
	case <-r.Context().Done():
		p.cancelOnce.Do(func() {
			if p.clientCancelled != nil {
				close(p.clientCancelled)
			}
		})
	case <-time.After(2 * time.Second):
	}
}

func (p *costE2EOpenAIProvider) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.requestCount
}

func (p *costE2EOpenAIProvider) queries() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.requestQueries...)
}

type cancelOnFirstWriteRecorder struct {
	*httptest.ResponseRecorder
	cancel context.CancelFunc
	once   sync.Once
}

func (w *cancelOnFirstWriteRecorder) Write(data []byte) (int, error) {
	n, err := w.ResponseRecorder.Write(data)
	w.once.Do(w.cancel)
	return n, err
}

func setupCostAccountingE2E(t *testing.T) {
	t.Helper()
	setupSeedanceE2EDB(t)
	require.NoError(t, appI18n.Init())
	require.NoError(t, model.DB.AutoMigrate(
		&model.ChannelModelCostRule{},
		&model.CostAccountingRequest{},
		&model.CostAccountingAttempt{},
		&model.CostAccountingAudit{},
	))

	previousMemoryCache := common.MemoryCacheEnabled
	previousLookup := service.CostCapabilityLookup
	previousPreview := service.RevenuePreviewHookForTest()
	previousStreamingTimeout := constant.StreamingTimeout
	common.MemoryCacheEnabled = false
	constant.StreamingTimeout = 300
	service.CostCapabilityLookup = relay.CostCapabilitiesForRoute
	service.SetRoutingRevenuePreview(func(context.Context, service.RoutingRevenuePreviewInput) (int64, string, error) {
		return 1_000_000, "500000", nil
	})
	service.InvalidateCostCoverage(0, "", "")

	cfg := config.GlobalConfig.Get(cost_setting.ConfigName)
	previousConfig, err := config.ConfigToMap(cfg)
	require.NoError(t, err)
	require.NoError(t, config.UpdateConfigFromMap(cfg, map[string]string{
		cost_setting.KeyMode: string(types.CostAccountingStrict),
	}))
	cost_setting.UpdateAndSync()
	t.Cleanup(func() {
		require.NoError(t, config.UpdateConfigFromMap(cfg, previousConfig))
		cost_setting.UpdateAndSync()
		service.InvalidateCostCoverage(0, "", "")
		service.CostCapabilityLookup = previousLookup
		service.SetRoutingRevenuePreview(previousPreview)
		common.MemoryCacheEnabled = previousMemoryCache
		constant.StreamingTimeout = previousStreamingTimeout
	})
}

func costAccountingSeedanceRequestBody(t *testing.T) string {
	t.Helper()
	var request map[string]any
	require.NoError(t, common.UnmarshalJsonStr(seedance20MultimodalRequestBody, &request))
	request["resolution"] = "720p"
	body, err := common.Marshal(request)
	require.NoError(t, err)
	return string(body)
}

func costAccountingE2ERouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	router.SetRelayRouter(engine)
	router.SetVideoRouter(engine)
	return engine
}

func seedCostAccountingChatData(t *testing.T, upstreamURLs ...string) {
	t.Helper()
	require.NotEmpty(t, upstreamURLs)
	require.NoError(t, model.DB.Create(&model.User{
		Id: costE2EChatUserID, Username: "cost_e2e_user", Password: "e2e-password",
		Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Quota: 2_000_000_000,
		Group: "default", AffCode: "cost-e2e-user",
	}).Error)
	require.NoError(t, model.DB.Create(&model.Token{
		Id: 101, UserId: costE2EChatUserID, Key: costE2EChatToken,
		Status: common.TokenStatusEnabled, RemainQuota: 2_000_000_000,
		UnlimitedQuota: true, Group: "default",
	}).Error)

	ratios := ratio_setting.GetModelRatioCopy()
	ratios[costE2EChatModel] = 1
	ratioJSON, err := common.Marshal(ratios)
	require.NoError(t, err)
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(string(ratioJSON)))

	for index, upstreamURL := range upstreamURLs {
		priority := int64(len(upstreamURLs) - index)
		channel := &model.Channel{
			Id: 101 + index, Type: constant.ChannelTypeOpenAI, Key: "cost-e2e-key",
			Status: common.ChannelStatusEnabled, Name: "cost-e2e-provider-" + string(rune('a'+index)),
			Weight: common.GetPointer[uint](1), Priority: &priority, BaseURL: common.GetPointer(upstreamURL),
			Models: costE2EChatModel, Group: "default", CreatedTime: time.Now().Unix(), OtherSettings: "{}",
		}
		require.NoError(t, channel.Insert())
	}
}

func seedCostAccountingRule(t *testing.T, channelID int, modelName string, chargeEvent types.CostChargeEvent, price string) {
	t.Helper()
	configValue, err := service.NormalizeCostRuleConfig(types.CostModePerRequest, types.CostRuleConfigV1{
		Currency: "USD", BillingMultiplier: "1", PurchaseDiscountRatio: "1",
		RechargeExchangeRatio: "1", FeeRate: "0", CurrencyToUSDRate: "1",
		UnitPrice: &price, ChargeEvent: chargeEvent,
	})
	require.NoError(t, err)
	configJSON, err := common.Marshal(configValue)
	require.NoError(t, err)
	now := common.GetTimestamp()
	require.NoError(t, model.DB.Create(&model.ChannelModelCostRule{
		ChannelID: channelID, BillableUpstreamModel: modelName, CostVariantKey: string(types.DefaultCostVariantKey), Version: 1,
		Status: string(types.CostRuleActive), CostMode: string(types.CostModePerRequest), SchemaVersion: 1,
		ConfigJSON: string(configJSON), Source: "manual", CreatedBy: 1, ActivatedBy: 1,
		EffectiveFrom: &now, CreatedAt: now, UpdatedAt: now,
	}).Error)
	service.InvalidateCostCoverage(channelID, modelName, "")
}

func TestCostAccountingPreservesRequestQueryE2E(t *testing.T) {
	setupCostAccountingE2E(t)
	provider := &costE2EOpenAIProvider{}
	server := httptest.NewServer(provider)
	t.Cleanup(server.Close)
	seedCostAccountingChatData(t, server.URL)
	seedCostAccountingRule(t, 101, costE2EChatModel, types.CostChargeResponseSucceeded, "0.10")

	status, response := performJSONRequest(t, costAccountingE2ERouter(), http.MethodPost,
		"/v1/chat/completions?foo=bar&signature=secret", "Bearer "+costE2EChatToken,
		`{"model":"cost-e2e-chat","messages":[{"role":"user","content":"hello"}]}`)

	require.Equal(t, http.StatusOK, status, string(response))
	assert.Equal(t, []string{"foo=bar&signature=secret"}, provider.queries())
	assert.NotContains(t, string(response), "signature=secret")

	adminLogs, _, err := model.GetAllLogs(model.LogTypeUnknown, 0, 0, costE2EChatModel, "", "", 0, 20, 0, "", "", "")
	require.NoError(t, err)
	require.NotEmpty(t, adminLogs)
	for _, log := range adminLogs {
		assert.NotContains(t, log.Other, "foo=bar")
		assert.NotContains(t, log.Other, "signature=secret")
		var other map[string]any
		require.NoError(t, common.UnmarshalJsonStr(log.Other, &other))
		assert.Equal(t, "/v1/chat/completions", other["request_path"])
	}
}

func TestCostAccountingSyncRetryAndLogPrivacyE2E(t *testing.T) {
	setupCostAccountingE2E(t)
	failedProvider := &costE2EOpenAIProvider{status: http.StatusInternalServerError}
	failedServer := httptest.NewServer(failedProvider)
	t.Cleanup(failedServer.Close)
	successProvider := &costE2EOpenAIProvider{}
	successServer := httptest.NewServer(successProvider)
	t.Cleanup(successServer.Close)
	seedCostAccountingChatData(t, failedServer.URL, successServer.URL)
	seedCostAccountingRule(t, 101, costE2EChatModel, types.CostChargeResponseSucceeded, "0.10")
	seedCostAccountingRule(t, 102, costE2EChatModel, types.CostChargeResponseSucceeded, "0.20")

	previousRetryTimes := common.RetryTimes
	common.RetryTimes = 1
	t.Cleanup(func() { common.RetryTimes = previousRetryTimes })
	status, body := performJSONRequest(t, costAccountingE2ERouter(), http.MethodPost, "/v1/chat/completions", "Bearer "+costE2EChatToken,
		`{"model":"cost-e2e-chat","messages":[{"role":"user","content":"hello"}]}`)
	require.Equal(t, http.StatusOK, status, string(body))
	assert.Equal(t, 1, failedProvider.count())
	assert.Equal(t, 1, successProvider.count())

	var requests []model.CostAccountingRequest
	require.NoError(t, model.DB.Order("id ASC").Find(&requests).Error)
	require.Len(t, requests, 1)
	detail, err := service.GetCostRequestDetail(requests[0].ID)
	require.NoError(t, err)
	require.Len(t, detail.Attempts, 2)
	assert.Equal(t, 1, detail.Attempts[0].Attempt.AttemptNo)
	assert.Equal(t, 101, detail.Attempts[0].Attempt.ChannelID)
	assert.Equal(t, string(types.CostAttemptUnknown), detail.Attempts[0].Attempt.Status)
	assert.False(t, detail.Attempts[0].Winning)
	assert.Equal(t, 2, detail.Attempts[1].Attempt.AttemptNo)
	assert.Equal(t, 102, detail.Attempts[1].Attempt.ChannelID)
	assert.Equal(t, string(types.CostAttemptSettled), detail.Attempts[1].Attempt.Status)
	require.NotNil(t, detail.Attempts[1].Attempt.CostNanoUSD)
	assert.Equal(t, int64(200_000_000), *detail.Attempts[1].Attempt.CostNanoUSD)
	assert.True(t, detail.Attempts[1].Winning)
	assert.Equal(t, 2, detail.Request.AttemptCount)
	assert.Equal(t, int64(200_000_000), detail.Request.ConfirmedCostNanoUSD)
	assert.Equal(t, string(types.CostRevenueSettled), detail.Request.RevenueStatus)
	require.NotNil(t, detail.Request.FinalUserQuota)
	assert.Equal(t, int64(15), *detail.Request.FinalUserQuota)
	require.NotNil(t, detail.Request.BilledRevenueEquivalentNanoUSD)
	assert.Equal(t, int64(30_000), *detail.Request.BilledRevenueEquivalentNanoUSD)
	assert.Equal(t, string(types.CostProfitIncompleteCost), detail.Request.ProfitStatus)
	assert.Nil(t, detail.Request.BilledGrossProfitNanoUSD)

	summary, err := service.SummarizeCostProfit(service.CostReportFilter{TimeBasis: service.CostReportTimeRequested})
	require.NoError(t, err)
	assert.Zero(t, summary.CompleteRequestCount)
	assert.Zero(t, summary.RealizedRevenueNanoUSD)
	assert.Equal(t, int64(200_000_000), summary.KnownIncompleteCostNanoUSD)
	assert.Equal(t, int64(1), summary.RetryAttemptCount)
	assert.Equal(t, int64(1), summary.UnknownCostCount)

	userLogs, _, err := model.GetUserLogs(costE2EChatUserID, model.LogTypeConsume, 0, 0, "", "", 0, 20, "", "", "")
	require.NoError(t, err)
	require.Len(t, userLogs, 1)
	assert.NotContains(t, userLogs[0].Other, "admin_info")
	assert.NotContains(t, userLogs[0].Other, "cost_accounting_request_id")
	adminLogs, _, err := model.GetAllLogs(model.LogTypeConsume, 0, 0, "", "", "", 0, 20, 0, "", "", "")
	require.NoError(t, err)
	require.Len(t, adminLogs, 1)
	var adminOther map[string]any
	require.NoError(t, common.UnmarshalJsonStr(adminLogs[0].Other, &adminOther))
	adminInfo, ok := adminOther["admin_info"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, float64(detail.Request.ID), adminInfo["cost_accounting_request_id"])
}

func TestCostAccountingStreamCancellationPersistsAttemptE2E(t *testing.T) {
	setupCostAccountingE2E(t)
	provider := &costE2EOpenAIProvider{stream: true, clientCancelled: make(chan struct{})}
	server := httptest.NewServer(provider)
	t.Cleanup(server.Close)
	seedCostAccountingChatData(t, server.URL)
	seedCostAccountingRule(t, 101, costE2EChatModel, types.CostChargeResponseSucceeded, "0.05")

	previousRetryTimes := common.RetryTimes
	common.RetryTimes = 0
	t.Cleanup(func() { common.RetryTimes = previousRetryTimes })
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	recorder := &cancelOnFirstWriteRecorder{ResponseRecorder: httptest.NewRecorder(), cancel: cancel}
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(
		`{"model":"cost-e2e-chat","messages":[{"role":"user","content":"disconnect"}],"stream":true}`,
	)).WithContext(ctx)
	request.Header.Set("Authorization", "Bearer "+costE2EChatToken)
	request.Header.Set("Content-Type", "application/json")
	costAccountingE2ERouter().ServeHTTP(recorder, request)

	select {
	case <-provider.clientCancelled:
	case <-time.After(3 * time.Second):
		t.Fatal("upstream request was not cancelled after the client disconnected")
	}
	assert.Equal(t, 1, provider.count())
	var requestLedger model.CostAccountingRequest
	require.NoError(t, model.DB.First(&requestLedger).Error)
	var attempt model.CostAccountingAttempt
	require.NoError(t, model.DB.Where("cost_request_id = ?", requestLedger.ID).First(&attempt).Error)
	assert.Equal(t, string(types.CostAttemptSettled), attempt.Status)
	require.NotNil(t, attempt.CostNanoUSD)
	assert.Equal(t, int64(50_000_000), *attempt.CostNanoUSD)
	require.NotNil(t, requestLedger.WinningAttemptID)
	assert.Equal(t, attempt.ID, *requestLedger.WinningAttemptID)
	assert.NotEqual(t, string(types.CostRevenuePending), requestLedger.RevenueStatus)
}

func TestCostAccountingAsyncChargeEventsE2E(t *testing.T) {
	tests := []struct {
		name        string
		chargeEvent types.CostChargeEvent
	}{
		{name: "submit_accepted", chargeEvent: types.CostChargeSubmitAccepted},
		{name: "task_succeeded", chargeEvent: types.CostChargeTaskSucceeded},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setupCostAccountingE2E(t)
			provider := &mockArkServer{}
			server := httptest.NewServer(provider)
			t.Cleanup(server.Close)
			seedSeedanceE2EData(t, server.URL)
			seedCostAccountingRule(t, e2eChannelID, "doubao-seedance-2-0-260128", test.chargeEvent, "0.20")
			engine := costAccountingE2ERouter()

			status, body := performJSONRequest(t, engine, http.MethodPost, "/api/v3/contents/generations/tasks", "Bearer e2e", costAccountingSeedanceRequestBody(t))
			require.Equal(t, http.StatusOK, status, string(body))
			var response map[string]any
			require.NoError(t, common.Unmarshal(body, &response))
			publicID, ok := response["id"].(string)
			require.True(t, ok)

			var requestLedger model.CostAccountingRequest
			require.NoError(t, model.DB.Where("task_id = ?", publicID).First(&requestLedger).Error)
			var attempt model.CostAccountingAttempt
			require.NoError(t, model.DB.Where("cost_request_id = ?", requestLedger.ID).First(&attempt).Error)
			require.NotNil(t, requestLedger.WinningAttemptID)
			assert.Equal(t, attempt.ID, *requestLedger.WinningAttemptID)
			var task model.Task
			require.NoError(t, model.DB.Where("task_id = ?", publicID).First(&task).Error)
			assert.Equal(t, requestLedger.ID, task.PrivateData.CostRequestID)

			if test.chargeEvent == types.CostChargeTaskSucceeded {
				assert.Equal(t, string(types.CostAttemptAwaitingMeter), attempt.Status)
			} else {
				assert.Equal(t, string(types.CostAttemptSettled), attempt.Status)
			}
			assert.Equal(t, string(types.CostRevenuePending), requestLedger.RevenueStatus)
			assert.Nil(t, requestLedger.FinalUserQuota)
			assert.Equal(t, string(types.CostProfitIncompleteRevenue), requestLedger.ProfitStatus)

			previousFactory := service.GetTaskAdaptorFunc
			service.GetTaskAdaptorFunc = func(platform constant.TaskPlatform) service.TaskPollingAdaptor {
				return relay.GetTaskAdaptor(platform)
			}
			t.Cleanup(func() { service.GetTaskAdaptorFunc = previousFactory })
			summary := service.RunTaskPollingOnce(context.Background(), nil)
			assert.Equal(t, 1, summary.UnfinishedTasks)
			require.NoError(t, model.DB.Where("task_id = ?", publicID).First(&task).Error)
			assert.Equal(t, model.TaskStatus(model.TaskStatusSuccess), task.Status)
			require.NoError(t, model.DB.First(&attempt, attempt.ID).Error)

			assert.Equal(t, string(types.CostAttemptSettled), attempt.Status)
			require.NotNil(t, attempt.CostNanoUSD)
			assert.Equal(t, int64(200_000_000), *attempt.CostNanoUSD)
			require.NoError(t, model.DB.First(&requestLedger, requestLedger.ID).Error)
			require.NotNil(t, requestLedger.FinalUserQuota)
			assert.Equal(t, int64(task.Quota), *requestLedger.FinalUserQuota)
			expectedRevenue, err := service.RevenueEquivalentNanoUSD(int64(task.Quota), requestLedger.QuotaPerUnitSnapshot)
			require.NoError(t, err)
			require.NotNil(t, requestLedger.BilledRevenueEquivalentNanoUSD)
			assert.Equal(t, expectedRevenue, *requestLedger.BilledRevenueEquivalentNanoUSD)
			assert.Equal(t, string(types.CostProfitComplete), requestLedger.ProfitStatus)
			require.NotNil(t, requestLedger.BilledGrossProfitNanoUSD)
			assert.Equal(t, expectedRevenue-int64(200_000_000), *requestLedger.BilledGrossProfitNanoUSD)

			report, err := service.SummarizeCostProfit(service.CostReportFilter{})
			require.NoError(t, err)
			assert.Equal(t, int64(1), report.CompleteRequestCount)
			assert.Equal(t, expectedRevenue, report.RealizedRevenueNanoUSD)
			assert.Equal(t, int64(200_000_000), report.RealizedCostNanoUSD)
			assert.Equal(t, expectedRevenue-int64(200_000_000), report.RealizedProfitNanoUSD)
			assert.Equal(t, int64(1), report.NegativeProfitRequestCount)
		})
	}
}

func TestCostAccountingOrphanTaskInsertionE2E(t *testing.T) {
	setupCostAccountingE2E(t)
	provider := &mockArkServer{taskID: "cost-e2e-orphan-upstream"}
	server := httptest.NewServer(provider)
	t.Cleanup(server.Close)
	seedSeedanceE2EData(t, server.URL)
	seedCostAccountingRule(t, e2eChannelID, "doubao-seedance-2-0-260128", types.CostChargeSubmitAccepted, "0.20")

	forcedInsertErr := errors.New("forced cost E2E task insert failure")
	callbackName := "test:cost_e2e_fail_task_insert"
	require.NoError(t, model.DB.Callback().Create().Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Table == "tasks" {
			tx.AddError(forcedInsertErr)
		}
	}))
	t.Cleanup(func() { require.NoError(t, model.DB.Callback().Create().Remove(callbackName)) })

	status, body := performJSONRequest(t, costAccountingE2ERouter(), http.MethodPost, "/api/v3/contents/generations/tasks", "Bearer e2e", costAccountingSeedanceRequestBody(t))
	require.Equal(t, http.StatusOK, status, string(body))
	var response map[string]any
	require.NoError(t, common.Unmarshal(body, &response))
	publicID, ok := response["id"].(string)
	require.True(t, ok)

	var requestLedger model.CostAccountingRequest
	require.NoError(t, model.DB.Where("task_id = ?", publicID).First(&requestLedger).Error)
	require.NotNil(t, requestLedger.UpstreamTaskID)
	assert.Equal(t, "cost-e2e-orphan-upstream", *requestLedger.UpstreamTaskID)
	assert.Equal(t, "orphaned_task_insert_failed", requestLedger.FailureCode)
	var attempt model.CostAccountingAttempt
	require.NoError(t, model.DB.Where("cost_request_id = ?", requestLedger.ID).First(&attempt).Error)
	assert.Equal(t, string(types.CostAttemptSettled), attempt.Status)
	require.NotNil(t, attempt.CostNanoUSD)
	assert.Equal(t, int64(200_000_000), *attempt.CostNanoUSD)
	var taskCount int64
	require.NoError(t, model.DB.Model(&model.Task{}).Count(&taskCount).Error)
	assert.Zero(t, taskCount)
	anomalies, total, err := service.ListCostAnomalies(service.CostAnomalyFilter{Kind: "orphaned_task", Page: 1, PageSize: 20})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, anomalies, 1)
	assert.Equal(t, "orphaned_task", anomalies[0].Kind)
	assert.Equal(t, requestLedger.ID, anomalies[0].Request.ID)
}

func TestCostAccountingStrictAllUncoveredRejectsBeforeSendE2E(t *testing.T) {
	setupCostAccountingE2E(t)
	firstProvider := &mockArkServer{}
	firstServer := httptest.NewServer(firstProvider)
	t.Cleanup(firstServer.Close)
	secondProvider := &mockArkServer{}
	secondServer := httptest.NewServer(secondProvider)
	t.Cleanup(secondServer.Close)
	seedSeedanceE2EData(t, firstServer.URL)
	seedSecondSeedanceE2EChannel(t, secondServer.URL)
	previousRetryTimes := common.RetryTimes
	common.RetryTimes = 1
	t.Cleanup(func() { common.RetryTimes = previousRetryTimes })

	status, body := performJSONRequest(t, costAccountingE2ERouter(), http.MethodPost, "/api/v3/contents/generations/tasks", "Bearer e2e", seedance20MultimodalRequestBody)
	assert.Equal(t, http.StatusServiceUnavailable, status, string(body))
	assert.Empty(t, firstProvider.snapshot())
	assert.Empty(t, secondProvider.snapshot())
	response := strings.ToLower(string(body))
	assert.NotContains(t, response, "cost")
	assert.NotContains(t, response, "price")
	assert.NotContains(t, response, "rule")
	assert.NotContains(t, response, firstServer.URL)
	assert.NotContains(t, response, secondServer.URL)
	var requestCount int64
	require.NoError(t, model.DB.Model(&model.CostAccountingRequest{}).Count(&requestCount).Error)
	assert.Zero(t, requestCount)
}
