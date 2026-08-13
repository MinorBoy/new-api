package e2e

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelrouting"
	"github.com/QuantumNous/new-api/relay"
	relayhelper "github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/cost_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	zzoneE2EClientModel   = "zzone-imported-model"
	zzoneE2EUpstreamModel = "zzone-private-model"
	zzoneE2ETaskID        = "zzone-private-task"
)

type zzoneE2EMock struct {
	mu             sync.Mutex
	requests       []mockArkRequest
	pollResponses  []string
	pollIndex      int
	submitStatus   int
	submitResponse string
}

func (m *zzoneE2EMock) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	body, _ := io.ReadAll(request.Body)
	m.mu.Lock()
	m.requests = append(m.requests, mockArkRequest{
		Method: request.Method, Path: request.URL.Path,
		Authorization: request.Header.Get("Authorization"), Body: append([]byte(nil), body...),
	})
	response := ""
	switch {
	case request.Method == http.MethodPost && request.URL.Path == "/v1/videos":
		if m.submitStatus != 0 {
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(m.submitStatus)
			_, _ = writer.Write([]byte(m.submitResponse))
			m.mu.Unlock()
			return
		}
		response = `{"id":"` + zzoneE2ETaskID + `","status":"queued","seconds":"15"}`
	case request.Method == http.MethodGet && request.URL.Path == "/v1/videos/"+zzoneE2ETaskID:
		if len(m.pollResponses) > 0 {
			index := min(m.pollIndex, len(m.pollResponses)-1)
			response = m.pollResponses[index]
			m.pollIndex++
		}
	case request.Method == http.MethodGet && request.URL.Path == "/v1/videos/"+zzoneE2ETaskID+"/content":
		m.mu.Unlock()
		writer.Header().Set("Content-Type", "video/mp4")
		writer.Header().Set("Accept-Ranges", "bytes")
		_, _ = writer.Write([]byte("zzone-mp4"))
		return
	}
	m.mu.Unlock()

	writer.Header().Set("Content-Type", "application/json")
	if response == "" {
		http.NotFound(writer, request)
		return
	}
	_, _ = writer.Write([]byte(response))
}

func (m *zzoneE2EMock) snapshot() []mockArkRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	requests := make([]mockArkRequest, len(m.requests))
	copy(requests, m.requests)
	return requests
}

func (m *zzoneE2EMock) submitCount() int {
	count := 0
	for _, request := range m.snapshot() {
		if request.Method == http.MethodPost && request.Path == "/v1/videos" {
			count++
		}
	}
	return count
}

type zzoneE2EEnvironment struct {
	engine *gin.Engine
	mock   *zzoneE2EMock
}

func setupZZoneE2E(t *testing.T, pollResponses ...string) *zzoneE2EEnvironment {
	t.Helper()
	setupSeedanceE2EDB(t)
	setupSeedanceE2EVideoMetadata(t)
	mock := &zzoneE2EMock{pollResponses: pollResponses}
	server := httptest.NewServer(mock)
	t.Cleanup(server.Close)
	seedSeedanceE2EData(t, server.URL)

	fetchSetting := system_setting.GetFetchSetting()
	originalFetchSetting := *fetchSetting
	parsedURL, err := url.Parse(server.URL)
	require.NoError(t, err)
	fetchSetting.EnableSSRFProtection = true
	fetchSetting.AllowPrivateIp = true
	fetchSetting.AllowedPorts = []string{parsedURL.Port()}
	service.InitHttpClient()
	t.Cleanup(func() {
		*fetchSetting = originalFetchSetting
		service.InitHttpClient()
	})

	channel, err := model.GetChannelById(e2eChannelID, true)
	require.NoError(t, err)
	mapping := `{"` + zzoneE2EClientModel + `":"` + zzoneE2EUpstreamModel + `"}`
	channel.Type = constant.ChannelTypeZZone
	channel.Key = "mock-zzone-key"
	channel.Models = zzoneE2EClientModel
	channel.ModelMapping = &mapping
	require.NoError(t, channel.Update())

	ratios := ratio_setting.GetModelRatioCopy()
	ratios[zzoneE2EClientModel] = 0.1
	encoded, err := common.Marshal(ratios)
	require.NoError(t, err)
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(string(encoded)))

	service.GetTaskAdaptorFunc = func(platform constant.TaskPlatform) service.TaskPollingAdaptor {
		return relay.GetTaskAdaptor(platform)
	}
	t.Cleanup(func() { service.GetTaskAdaptorFunc = nil })
	return &zzoneE2EEnvironment{engine: seedanceE2ERouter(), mock: mock}
}

func TestZZoneArkLifecycleE2E(t *testing.T) {
	environment := setupZZoneE2E(t,
		`{"id":"zzone-private-task","status":"processing","progress":42,"seconds":"15"}`,
		`{"id":"zzone-private-task","status":"completed","progress":100,"seconds":"15"}`,
	)
	requestBody := `{
		"model":"zzone-imported-model",
		"content":[
			{"type":"text","text":"zzone multimodal acceptance"},
			{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.8/ref.png"}},
			{"type":"video_url","role":"reference_video","video_url":{"url":"https://8.8.4.4/ref.mp4"}},
			{"type":"audio_url","role":"reference_audio","audio_url":{"url":"https://1.1.1.1/ref.mp3"}}
		],
		"duration":15,
		"ratio":"9:16"
	}`

	status, submit := performJSONRequest(t, environment.engine, http.MethodPost, "/api/v3/contents/generations/tasks", "Bearer e2e-1", requestBody)
	require.Equal(t, http.StatusOK, status, string(submit))
	var created struct {
		ID string `json:"id"`
	}
	require.NoError(t, common.Unmarshal(submit, &created))
	require.True(t, strings.HasPrefix(created.ID, "task_"))
	assertZZonePublicBody(t, submit)

	requests := environment.mock.snapshot()
	require.Len(t, requests, 1)
	assert.Equal(t, http.MethodPost, requests[0].Method)
	assert.Equal(t, "/v1/videos", requests[0].Path)
	assert.Equal(t, "Bearer mock-zzone-key", requests[0].Authorization)
	assert.JSONEq(t, `{
		"model":"zzone-private-model",
		"prompt":"zzone multimodal acceptance",
		"seconds":"15",
		"aspect_ratio":"9:16",
		"images":["https://8.8.8.8/ref.png"],
		"videos":["https://8.8.4.4/ref.mp4"],
		"audios":["https://1.1.1.1/ref.mp3"]
	}`, string(requests[0].Body))

	task := pollNewAPIVideoTask(t, created.ID)
	assert.Equal(t, model.TaskStatus(model.TaskStatusInProgress), task.Status)
	task = pollNewAPIVideoTask(t, created.ID)
	assert.Equal(t, model.TaskStatus(model.TaskStatusSuccess), task.Status)
	assert.Equal(t, zzoneE2ETaskID, task.PrivateData.UpstreamTaskID)
	assert.Contains(t, task.PrivateData.ResultURL, "/v1/videos/"+created.ID+"/content")
	require.NotNil(t, task.PrivateData.BillingContext)
	assert.Positive(t, task.Quota)

	status, single := performJSONRequest(t, environment.engine, http.MethodGet, "/api/v3/contents/generations/tasks/"+created.ID, "Bearer e2e-1", "")
	require.Equal(t, http.StatusOK, status, string(single))
	assert.Contains(t, string(single), `"video_url":"`+task.PrivateData.ResultURL+`"`)
	assertZZonePublicBody(t, single)

	status, list := performJSONRequest(t, environment.engine, http.MethodGet, "/api/v3/contents/generations/tasks?filter.task_ids="+created.ID+"&page_size=20", "Bearer e2e-1", "")
	require.Equal(t, http.StatusOK, status, string(list))
	assert.Contains(t, string(list), created.ID)
	assertZZonePublicBody(t, list)

	status, content := performJSONRequest(t, environment.engine, http.MethodGet, "/v1/videos/"+created.ID+"/content", "Bearer e2e-1", "")
	require.Equal(t, http.StatusOK, status, string(content))
	assert.Equal(t, "zzone-mp4", string(content))
	requests = environment.mock.snapshot()
	require.Len(t, requests, 4)
	assert.Equal(t, "/v1/videos/"+zzoneE2ETaskID+"/content", requests[3].Path)
	assert.Equal(t, "Bearer mock-zzone-key", requests[3].Authorization)
}

func TestZZoneArkLifecycleUses720pRoutingContractWithoutUpstreamResolutionE2E(t *testing.T) {
	environment := setupZZoneE2E(t,
		`{"id":"zzone-private-task","status":"completed","progress":100,"seconds":"5"}`,
	)

	status, submit := performJSONRequest(t, environment.engine, http.MethodPost, "/api/v3/contents/generations/tasks", "Bearer e2e-1", `{
		"model":"zzone-imported-model",
		"content":[{"type":"text","text":"720p contract"}],
		"duration":5,
		"ratio":"16:9",
		"resolution":"720p"
	}`)
	require.Equal(t, http.StatusOK, status, string(submit))

	requests := environment.mock.snapshot()
	require.Len(t, requests, 1)
	assert.JSONEq(t, `{
		"model":"zzone-private-model",
		"prompt":"720p contract",
		"seconds":"5",
		"aspect_ratio":"16:9"
	}`, string(requests[0].Body))
	assert.NotContains(t, string(requests[0].Body), `"resolution"`)
}

func TestZZoneArkCapabilityRouteMaintainsStrict60PercentMarginE2E(t *testing.T) {
	environment := setupZZoneE2E(t, `{"id":"zzone-private-task","status":"completed","progress":100,"seconds":"5"}`)
	require.NoError(t, model.DB.AutoMigrate(&model.ChannelModelCostRule{}, &model.CostAccountingRequest{}, &model.CostAccountingAttempt{}, &model.CostAccountingAudit{}, &model.RoutingPolicy{}, &model.RouteTarget{}))

	const (
		expectedCostNanoUSD      = int64(100_000_000)
		expectedRevenueNanoUSD   = int64(540_000_000)
		expectedProfitNanoUSD    = int64(440_000_000)
		expectedGrossMarginPPM   = int64(814_815)
		minimumExpectedMarginBPS = 6000
	)
	previousCostCapabilityLookup := service.CostCapabilityLookup
	previousRevenuePreview := service.RevenuePreviewHookForTest()
	service.CostCapabilityLookup = relay.CostCapabilitiesForRoute
	service.SetRoutingRevenuePreview(func(_ context.Context, input service.RoutingRevenuePreviewInput) (int64, string, error) {
		return relayhelper.PreviewRoutingRevenueWithSeedanceInput(input.OriginModelName, input.Group, input.RequestPath, input.RelayMode, input.DurationSeconds, input.UserId, input.OutputResolution, input.HasReferenceVideo, input.InputVideoDurationMS)
	})
	service.InvalidateCostCoverage(0, "", "")
	t.Cleanup(func() {
		service.CostCapabilityLookup = previousCostCapabilityLookup
		service.SetRoutingRevenuePreview(previousRevenuePreview)
		service.InvalidateCostCoverage(0, "", "")
	})

	costConfig := config.GlobalConfig.Get(cost_setting.ConfigName)
	previousCostConfig, err := config.ConfigToMap(costConfig)
	require.NoError(t, err)
	require.NoError(t, config.UpdateConfigFromMap(costConfig, map[string]string{
		cost_setting.KeyMode:                     string(types.CostAccountingStrict),
		cost_setting.KeyMinimumExpectedMarginBPS: fmt.Sprint(minimumExpectedMarginBPS),
	}))
	cost_setting.UpdateAndSync()
	t.Cleanup(func() {
		require.NoError(t, config.UpdateConfigFromMap(costConfig, previousCostConfig))
		cost_setting.UpdateAndSync()
	})
	previousBillingConfig, err := config.ConfigToMap(config.GlobalConfig.Get("billing_setting"))
	require.NoError(t, err)
	seedanceTokenPriceJSON, err := common.Marshal(map[string]types.SeedanceTokenPrice{
		modelrouting.Seedance20Fast: {Scenarios: map[string]types.SeedanceTokenPriceScenario{
			types.SeedanceTokenScenarioKey("720p", types.SeedanceTokenScenarioNoVideo): {
				PricePerMillion: "5", Width: 1280, Height: 720, FrameRate: 24,
				PricingVersion: "e2e", Source: "zzone strict contract",
			},
		}},
	})
	require.NoError(t, err)
	billingModeJSON, err := common.Marshal(map[string]string{
		modelrouting.Seedance20Fast: billing_setting.BillingModeSeedanceTokens,
	})
	require.NoError(t, err)
	require.NoError(t, config.UpdateConfigFromMap(config.GlobalConfig.Get("billing_setting"), map[string]string{
		billing_setting.BillingModeField:        string(billingModeJSON),
		billing_setting.DurationPriceField:      "{}",
		billing_setting.SeedanceTokenPriceField: string(seedanceTokenPriceJSON),
	}))
	t.Cleanup(func() {
		require.NoError(t, config.UpdateConfigFromMap(config.GlobalConfig.Get("billing_setting"), previousBillingConfig))
	})

	costConfigValue, err := service.NormalizeCostRuleConfig(types.CostModePerRequest, types.CostRuleConfigV1{
		Currency: "USD", BillingMultiplier: "1", PurchaseDiscountRatio: "1",
		RechargeExchangeRatio: "1", FeeRate: "0", CurrencyToUSDRate: "1",
		UnitPrice: common.GetPointer("0.10"), ChargeEvent: types.CostChargeTaskSucceeded,
	})
	require.NoError(t, err)
	costConfigJSON, err := common.Marshal(costConfigValue)
	require.NoError(t, err)
	now := common.GetTimestamp()
	require.NoError(t, model.DB.Create(&model.ChannelModelCostRule{
		ChannelID: e2eChannelID, BillableUpstreamModel: zzoneE2EUpstreamModel, CostVariantKey: "zzone-720p", Version: 1,
		Status: string(types.CostRuleActive), CostMode: string(types.CostModePerRequest), SchemaVersion: 1,
		ConfigJSON: string(costConfigJSON), Source: "e2e", CreatedBy: 1, ActivatedBy: 1,
		EffectiveFrom: &now, CreatedAt: now, UpdatedAt: now,
	}).Error)
	service.InvalidateCostCoverage(e2eChannelID, zzoneE2EUpstreamModel, "zzone-720p")
	channel, err := model.GetChannelById(e2eChannelID, true)
	require.NoError(t, err)
	channel.Models = strings.Join([]string{zzoneE2EClientModel, modelrouting.Seedance20Fast}, ",")
	require.NoError(t, channel.Update())

	margin := minimumExpectedMarginBPS
	policy, err := service.SaveRoutingPolicy(0, service.RoutingPolicyWriteRequest{
		GroupName: "default", Model: modelrouting.Seedance20Fast, Enabled: true,
		Defaults: modelrouting.Defaults{OutputResolution: "720p", DurationSeconds: 5, AspectRatio: "16:9"},
		Targets: []service.RouteTargetWriteRequest{{
			ChannelID: e2eChannelID, Name: "zzone 720p", UpstreamModel: zzoneE2EUpstreamModel, CostVariantKey: "zzone-720p",
			TargetPriority: 100, MinimumExpectedMarginBPS: &margin, Enabled: true,
			Constraints: modelrouting.Constraints{
				OutputResolutions: []string{"720p"}, Durations: modelrouting.DurationConstraint{Values: []int{5}}, AspectRatios: []string{"16:9"},
				ReferenceLimits: modelrouting.ReferenceLimits{Images: 4, Videos: 3, Audios: 1},
			},
		}},
	})
	require.NoError(t, err)

	status, submit := performJSONRequest(t, environment.engine, http.MethodPost, "/api/v3/contents/generations/tasks", "Bearer e2e-1-"+fmt.Sprint(e2eChannelID), `{
		"model":"doubao-seedance-2-0-fast-260128",
		"content":[{"type":"text","text":"strict 720p contract"}],
		"duration":5,
		"ratio":"16:9",
		"resolution":"720p"
	}`)
	require.Equal(t, http.StatusOK, status, string(submit))

	requests := environment.mock.snapshot()
	require.Len(t, requests, 1)
	assert.JSONEq(t, `{"model":"zzone-private-model","prompt":"strict 720p contract","seconds":"5","aspect_ratio":"16:9"}`, string(requests[0].Body))
	assert.NotContains(t, string(requests[0].Body), `"resolution"`)

	var task model.Task
	require.NoError(t, model.DB.Order("id DESC").First(&task).Error)
	require.NotNil(t, task.PrivateData.Routing)
	assert.Equal(t, policy.ID, task.PrivateData.Routing.PolicyID)
	assert.Equal(t, "720p", task.PrivateData.Routing.Facts.OutputResolution)
	assert.Equal(t, zzoneE2EUpstreamModel, task.PrivateData.Routing.UpstreamModel)
	assert.Equal(t, "zzone-720p", task.PrivateData.Routing.CostVariantKey)
	var costRequest model.CostAccountingRequest
	require.NoError(t, model.DB.Where("task_id = ?", task.TaskID).First(&costRequest).Error)
	var costAttempt model.CostAccountingAttempt
	require.NoError(t, model.DB.Where("cost_request_id = ?", costRequest.ID).First(&costAttempt).Error)
	assert.Equal(t, string(types.CostChargeTaskSucceeded), costAttempt.ChargeEvent)
	assert.Equal(t, "zzone-720p", costAttempt.CostVariantKey)
	assert.Equal(t, string(types.CostAttemptAwaitingMeter), costAttempt.Status)
	assert.Nil(t, costAttempt.CostNanoUSD)

	task = pollNewAPIVideoTask(t, task.TaskID)
	assert.Equal(t, model.TaskStatus(model.TaskStatusSuccess), task.Status)
	require.NoError(t, model.DB.First(&costAttempt, costAttempt.ID).Error)
	assert.Equal(t, string(types.CostAttemptSettled), costAttempt.Status)
	require.NotNil(t, costAttempt.CostNanoUSD)
	assert.Equal(t, expectedCostNanoUSD, *costAttempt.CostNanoUSD)
	require.NoError(t, model.DB.First(&costRequest, costRequest.ID).Error)
	require.NotNil(t, costRequest.BilledRevenueEquivalentNanoUSD)
	require.NotNil(t, costRequest.BilledGrossProfitNanoUSD)
	require.NotNil(t, costRequest.GrossMarginPPM)
	assert.Equal(t, string(types.CostRevenueSettled), costRequest.RevenueStatus)
	assert.Equal(t, string(types.CostProfitComplete), costRequest.ProfitStatus)
	assert.Equal(t, expectedCostNanoUSD, costRequest.ConfirmedCostNanoUSD)
	assert.Equal(t, expectedRevenueNanoUSD, *costRequest.BilledRevenueEquivalentNanoUSD)
	assert.Equal(t, expectedProfitNanoUSD, *costRequest.BilledGrossProfitNanoUSD)
	assert.Equal(t, expectedGrossMarginPPM, *costRequest.GrossMarginPPM)
	assert.GreaterOrEqual(t, *costRequest.GrossMarginPPM, int64(minimumExpectedMarginBPS*100))
	assert.Equal(t, types.CostAccountingStrict, cost_setting.Runtime().Mode)
	assert.Equal(t, minimumExpectedMarginBPS, cost_setting.Runtime().MinimumExpectedMarginBPS)
}

func TestZZoneFailedTaskRefundsExactlyOnceE2E(t *testing.T) {
	environment := setupZZoneE2E(t, `{"id":"zzone-private-task","status":"failed","error":{"code":"provider_error","message":"generation failed"}}`)
	var beforeUser model.User
	require.NoError(t, model.DB.First(&beforeUser, e2eUserID).Error)
	status, submit := performJSONRequest(t, environment.engine, http.MethodPost, "/api/v3/contents/generations/tasks", "Bearer e2e-1", `{"model":"zzone-imported-model","content":[{"type":"text","text":"refund"}],"duration":15}`)
	require.Equal(t, http.StatusOK, status, string(submit))
	var created struct {
		ID string `json:"id"`
	}
	require.NoError(t, common.Unmarshal(submit, &created))
	require.NotEmpty(t, created.ID)

	service.RunTaskPollingOnce(context.Background(), nil)
	service.RunTaskPollingOnce(context.Background(), nil)
	var task model.Task
	require.NoError(t, model.DB.Where("task_id = ?", created.ID).First(&task).Error)
	assert.Equal(t, model.TaskStatus(model.TaskStatusFailure), task.Status)
	assert.Zero(t, task.Quota)
	var afterUser model.User
	require.NoError(t, model.DB.First(&afterUser, e2eUserID).Error)
	assert.Equal(t, beforeUser.Quota, afterUser.Quota)
	var refundLogs []model.Log
	require.NoError(t, model.LOG_DB.Where("type = ?", model.LogTypeRefund).Find(&refundLogs).Error)
	assert.Len(t, refundLogs, 1)
}

func TestZZoneSubmitErrorsAreNotRetriedE2E(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		status int
		body   string
	}{
		{name: "validation", status: http.StatusBadRequest, body: `{"error":{"type":"validation_error","message":"prompt is required","param":"prompt","code":"missing_required_parameter"}}`},
		{name: "unauthorized", status: http.StatusUnauthorized, body: `{"error":{"type":"authentication_error","message":"invalid key","code":"invalid_api_key"}}`},
		{name: "rate limited", status: http.StatusTooManyRequests, body: `{"error":{"type":"rate_limit_error","message":"slow down","code":"rate_limited"}}`},
		{name: "provider failure", status: http.StatusInternalServerError, body: `{"error":{"type":"server_error","message":"upstream failure","code":"provider_error"}}`},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			environment := setupZZoneE2E(t)
			environment.mock.submitStatus = testCase.status
			environment.mock.submitResponse = testCase.body
			status, response := performJSONRequest(t, environment.engine, http.MethodPost, "/api/v3/contents/generations/tasks", "Bearer e2e-1", `{"model":"zzone-imported-model","content":[{"type":"text","text":"upstream error"}],"duration":15}`)
			assert.Equal(t, testCase.status, status, string(response))
			assert.Equal(t, 1, environment.mock.submitCount())
		})
	}
}

func TestZZoneInvalidRequestsHaveNoSideEffectsE2E(t *testing.T) {
	environment := setupZZoneE2E(t)
	requests := []string{
		zzoneE2EImageRequest(5),
		`{"model":"zzone-imported-model","content":[{"type":"text","text":"unsupported"}],"generate_audio":false}`,
	}
	for _, requestBody := range requests {
		var beforeTasks int64
		require.NoError(t, model.DB.Model(&model.Task{}).Count(&beforeTasks).Error)
		status, response := performJSONRequest(t, environment.engine, http.MethodPost, "/api/v3/contents/generations/tasks", "Bearer e2e-1", requestBody)
		assert.Equal(t, http.StatusBadRequest, status, string(response))
		var afterTasks int64
		require.NoError(t, model.DB.Model(&model.Task{}).Count(&afterTasks).Error)
		assert.Equal(t, beforeTasks, afterTasks)
	}
	assert.Empty(t, environment.mock.snapshot())
}

func TestZZoneUnknownStatusDoesNotCompleteTaskE2E(t *testing.T) {
	environment := setupZZoneE2E(t, `{"id":"zzone-private-task","status":"waiting_for_magic"}`)
	status, submit := performJSONRequest(t, environment.engine, http.MethodPost, "/api/v3/contents/generations/tasks", "Bearer e2e-1", `{"model":"zzone-imported-model","content":[{"type":"text","text":"unknown"}],"duration":15}`)
	require.Equal(t, http.StatusOK, status, string(submit))
	var created struct {
		ID string `json:"id"`
	}
	require.NoError(t, common.Unmarshal(submit, &created))

	var task model.Task
	require.NoError(t, model.DB.Where("task_id = ?", created.ID).First(&task).Error)
	upstreamID := task.GetUpstreamTaskID()
	initialStatus := task.Status
	initialQuota := task.Quota
	require.NoError(t, service.UpdateVideoTasks(context.Background(), task.Platform,
		map[int][]string{task.ChannelId: {upstreamID}},
		map[string]*model.Task{upstreamID: &task},
	))
	require.NoError(t, model.DB.Where("task_id = ?", created.ID).First(&task).Error)
	assert.Equal(t, initialStatus, task.Status)
	assert.Equal(t, initialQuota, task.Quota)
}

func zzoneE2EImageRequest(count int) string {
	items := `[{"type":"text","text":"too many"}`
	for index := 0; index < count; index++ {
		items += `,{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.` + string(rune('1'+index)) + `/ref.png"}}`
	}
	return `{"model":"zzone-imported-model","content":` + items + `],"duration":15}`
}

func assertZZonePublicBody(t *testing.T, body []byte) {
	t.Helper()
	for _, privateValue := range []string{
		zzoneE2ETaskID, zzoneE2EUpstreamModel, "mock-zzone-key",
		"user_id", "channel_id", "upstream_model_name",
	} {
		assert.NotContains(t, string(body), privateValue)
	}
}
