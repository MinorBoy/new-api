package e2e

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelrouting"
	"github.com/QuantumNous/new-api/pkg/videometa"
	"github.com/QuantumNous/new-api/relay"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/cost_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupStrictProfitRoutingE2E(t *testing.T, preview service.RoutingRevenuePreviewFunc) *seedanceCapabilityE2EEnv {
	t.Helper()
	env := setupSeedanceCapabilityRoutingE2E(t)
	require.NoError(t, model.DB.AutoMigrate(
		&model.ChannelModelCostRule{},
		&model.CostAccountingRequest{},
		&model.CostAccountingAttempt{},
		&model.CostAccountingAudit{},
	))

	previousLookup := service.CostCapabilityLookup
	previousPreview := service.RevenuePreviewHookForTest()
	service.CostCapabilityLookup = relay.CostCapabilitiesForRoute
	service.SetRoutingRevenuePreview(preview)
	service.InvalidateCostCoverage(0, "")

	costConfig := config.GlobalConfig.Get(cost_setting.ConfigName)
	previousConfig, err := config.ConfigToMap(costConfig)
	require.NoError(t, err)
	require.NoError(t, config.UpdateConfigFromMap(costConfig, map[string]string{
		cost_setting.KeyMode:                     string(types.CostAccountingStrict),
		cost_setting.KeyMinimumExpectedMarginBPS: "0",
	}))
	cost_setting.UpdateAndSync()
	t.Cleanup(func() {
		require.NoError(t, config.UpdateConfigFromMap(costConfig, previousConfig))
		cost_setting.UpdateAndSync()
		service.CostCapabilityLookup = previousLookup
		service.SetRoutingRevenuePreview(previousPreview)
		service.InvalidateCostCoverage(0, "")
	})
	return env
}

func setProfitRoutingMarginE2E(t *testing.T, minimumExpectedMarginBPS int) {
	t.Helper()
	costConfig := config.GlobalConfig.Get(cost_setting.ConfigName)
	require.NoError(t, config.UpdateConfigFromMap(costConfig, map[string]string{
		cost_setting.KeyMinimumExpectedMarginBPS: strconv.Itoa(minimumExpectedMarginBPS),
	}))
	cost_setting.UpdateAndSync()
}

func TestProfitRoutingPerRequestMarginBoundariesE2E(t *testing.T) {
	env := setupStrictProfitRoutingE2E(t, func(_ context.Context, input service.RoutingRevenuePreviewInput) (int64, string, error) {
		switch *input.DurationSeconds {
		case 5:
			return 2_475, "1000", nil
		case 11:
			return 5_445, "1000", nil
		case 12:
			return 5_940, "1000", nil
		default:
			return 0, "1000", nil
		}
	})
	seedCostAccountingRule(t, capabilityChannelB, upstreamStandardMG, types.CostChargeSubmitAccepted, "5")

	tests := []struct {
		name       string
		threshold  int
		duration   int
		wantStatus int
	}{
		{name: "zero percent five seconds rejects", threshold: 0, duration: 5, wantStatus: http.StatusServiceUnavailable},
		{name: "zero percent eleven seconds admits", threshold: 0, duration: 11, wantStatus: http.StatusOK},
		{name: "ten percent eleven seconds rejects", threshold: 1_000, duration: 11, wantStatus: http.StatusServiceUnavailable},
		{name: "ten percent twelve seconds admits", threshold: 1_000, duration: 12, wantStatus: http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setProfitRoutingMarginE2E(t, tt.threshold)
			before := len(env.channelB.snapshot())
			body := capabilityRequestBody(t, modelrouting.Seedance20, "720p", tt.duration, "16:9", modelrouting.ReferenceLimits{}, false)
			status, response := performJSONRequest(t, env.engine, http.MethodPost, "/api/v3/contents/generations/tasks", "Bearer e2e", body)

			require.Equal(t, tt.wantStatus, status, string(response))
			if tt.wantStatus == http.StatusOK {
				assert.Len(t, env.channelB.snapshot(), before+1)
				return
			}
			assert.Len(t, env.channelB.snapshot(), before)
			assert.NotContains(t, string(response), "5")
			assert.NotContains(t, string(response), "margin")
		})
	}
}

type profitRoutingE2EMetadataClient struct {
	mu      sync.Mutex
	results map[string]videometa.Metadata
	err     error
	calls   []string
}

func (c *profitRoutingE2EMetadataClient) Metadata(_ context.Context, assetURL string) (videometa.Metadata, error) {
	c.mu.Lock()
	c.calls = append(c.calls, assetURL)
	c.mu.Unlock()
	if c.err != nil {
		return videometa.Metadata{}, c.err
	}
	if metadata, ok := c.results[assetURL]; ok {
		return metadata, nil
	}
	return videometa.Metadata{DurationMS: 3_000, Width: 1280, Height: 720, FrameRateNum: 24, FrameRateDen: 1, Container: "mp4", ContentLength: 100}, nil
}

func (c *profitRoutingE2EMetadataClient) snapshot() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.calls...)
}

func performProfitRoutingRequest(t *testing.T, engine http.Handler, authorization, affinity, body string) (int, []byte) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", strings.NewReader(body))
	req.Header.Set("Authorization", authorization)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if affinity != "" {
		req.Header.Set("X-Profit-Affinity", affinity)
	}
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)
	return recorder.Code, recorder.Body.Bytes()
}

func seedProfitRoutingRuleE2E(t *testing.T, channelID int, modelName string, mode types.CostMode, config types.CostRuleConfigV1) {
	t.Helper()
	configValue, err := service.NormalizeCostRuleConfig(mode, config)
	require.NoError(t, err)
	configJSON, err := common.Marshal(configValue)
	require.NoError(t, err)
	now := common.GetTimestamp()
	require.NoError(t, model.DB.Create(&model.ChannelModelCostRule{
		ChannelID: channelID, BillableUpstreamModel: modelName, Version: 1,
		Status: string(types.CostRuleActive), CostMode: string(mode), SchemaVersion: 1,
		ConfigJSON: string(configJSON), Source: "manual", EffectiveFrom: &now,
		CreatedAt: now, UpdatedAt: now,
	}).Error)
	service.InvalidateCostCoverage(channelID, modelName)
}

func seedProfitRoutingTokenRuleE2E(t *testing.T, channelID int, modelName, pricePerMillion string) {
	t.Helper()
	seedProfitRoutingRuleE2E(t, channelID, modelName, types.CostModePerToken, types.CostRuleConfigV1{
		Currency: "USD", BillingMultiplier: "1", PurchaseDiscountRatio: "1",
		RechargeExchangeRatio: "1", FeeRate: "0", CurrencyToUSDRate: "1",
		TotalPerMillion: &pricePerMillion, TokenMode: types.CostTokenModeTotal,
		MeterSource: types.CostMeterUpstreamUsage, ChargeEvent: types.CostChargeResponseSucceeded,
	})
}

func TestLucenProfitRoutingChoosesEligibleFixedOrTokenChannelE2E(t *testing.T) {
	tests := []struct {
		name            string
		fixedCost       string
		tokenPerMillion string
		wantChannelID   int
		wantModel       string
	}{
		{name: "token eligible", fixedCost: "2", tokenPerMillion: "1", wantChannelID: capabilityChannelB, wantModel: "seedance-720p-token"},
		{name: "fixed eligible", fixedCost: "0.1", tokenPerMillion: "10", wantChannelID: capabilityChannelA, wantModel: "seedance-720p-10s"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := setupStrictProfitRoutingE2E(t, func(context.Context, service.RoutingRevenuePreviewInput) (int64, string, error) {
				return 1_000_000_000, "1000000000", nil
			})
			require.NoError(t, model.DB.Model(&model.Channel{}).
				Where("id IN ?", []int{capabilityChannelA, capabilityChannelB}).
				Update("type", constant.ChannelTypeLucen).Error)

			request := capabilityPolicyRequest(modelrouting.Seedance20, []service.RouteTargetWriteRequest{
				capabilityTarget(capabilityChannelA, "seedance-720p-10s", 100, []string{"720p"}, discreteDuration(10), []string{"16:9"}, modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3}, false),
				capabilityTarget(capabilityChannelB, "seedance-720p-token", 90, []string{"720p"}, discreteDuration(10), []string{"16:9"}, modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3}, false),
			})
			_, err := service.SaveRoutingPolicy(env.standardPolicy, request)
			require.NoError(t, err)

			fixedPrice := tt.fixedCost
			seedProfitRoutingRuleE2E(t, capabilityChannelA, "seedance-720p-10s", types.CostModePerRequest, types.CostRuleConfigV1{
				Currency: "USD", BillingMultiplier: "1", PurchaseDiscountRatio: "1",
				RechargeExchangeRatio: "1", FeeRate: "0", CurrencyToUSDRate: "1",
				UnitPrice: &fixedPrice, ChargeEvent: types.CostChargeSubmitAccepted,
			})
			tokenPrice := tt.tokenPerMillion
			seedProfitRoutingRuleE2E(t, capabilityChannelB, "seedance-720p-token", types.CostModePerToken, types.CostRuleConfigV1{
				Currency: "USD", BillingMultiplier: "1", PurchaseDiscountRatio: "1",
				RechargeExchangeRatio: "1", FeeRate: "0", CurrencyToUSDRate: "1",
				TotalPerMillion: &tokenPrice, TokenMode: types.CostTokenModeTotal,
				MeterSource: types.CostMeterUpstreamUsage, ChargeEvent: types.CostChargeTaskSucceeded,
			})

			body := capabilityRequestBody(t, modelrouting.Seedance20, "720p", 10, "16:9", modelrouting.ReferenceLimits{}, false)
			status, response := performProfitRoutingRequest(t, env.engine, "Bearer e2e", "", body)
			require.Equal(t, http.StatusOK, status, string(response))
			assertCapabilityPublicBody(t, response)
			assert.NotContains(t, string(response), tt.wantModel)
			assert.NotContains(t, string(response), "channel_id")

			selectedA := env.channelA.snapshot()
			selectedB := env.channelB.snapshot()
			var selected []mockArkRequest
			if tt.wantChannelID == capabilityChannelA {
				require.Len(t, selectedA, 1)
				assert.Empty(t, selectedB)
				selected = selectedA
			} else {
				require.Len(t, selectedB, 1)
				assert.Empty(t, selectedA)
				selected = selectedB
			}
			assert.Equal(t, http.MethodPost, selected[0].Method)
			assert.Equal(t, "/v1/video/generations", selected[0].Path)
			var upstream map[string]any
			require.NoError(t, common.Unmarshal(selected[0].Body, &upstream))
			assert.Equal(t, tt.wantModel, upstream["model"])

			var task model.Task
			require.NoError(t, model.DB.Order("id DESC").First(&task).Error)
			assert.Equal(t, tt.wantChannelID, task.ChannelId)
			require.NotNil(t, task.PrivateData.Routing)
			assert.Equal(t, tt.wantModel, task.PrivateData.Routing.UpstreamModel)

			beforeA, beforeB := len(selectedA), len(selectedB)
			mismatchBody := capabilityRequestBody(t, modelrouting.Seedance20, "720p", 5, "16:9", modelrouting.ReferenceLimits{}, false)
			status, response = performProfitRoutingRequest(t, env.engine, "Bearer e2e", "", mismatchBody)
			require.Equal(t, http.StatusBadRequest, status, string(response))
			assert.Len(t, env.channelA.snapshot(), beforeA)
			assert.Len(t, env.channelB.snapshot(), beforeB)
		})
	}
}

func replaceProfitRoutingRequestPriceE2E(t *testing.T, channelID int, modelName, price string) {
	t.Helper()
	configValue, err := service.NormalizeCostRuleConfig(types.CostModePerRequest, types.CostRuleConfigV1{
		Currency: "USD", BillingMultiplier: "1", PurchaseDiscountRatio: "1",
		RechargeExchangeRatio: "1", FeeRate: "0", CurrencyToUSDRate: "1",
		UnitPrice: &price, ChargeEvent: types.CostChargeSubmitAccepted,
	})
	require.NoError(t, err)
	configJSON, err := common.Marshal(configValue)
	require.NoError(t, err)
	require.NoError(t, model.DB.Model(&model.ChannelModelCostRule{}).
		Where("channel_id = ? AND billable_upstream_model = ?", channelID, modelName).
		Update("config_json", string(configJSON)).Error)
	service.InvalidateCostCoverage(channelID, modelName)
}

func TestProfitRoutingDispatchesFreeAndPerDurationRulesE2E(t *testing.T) {
	tests := []struct {
		name string
		seed func(t *testing.T)
	}{
		{
			name: "free",
			seed: func(t *testing.T) {
				seedProfitRoutingRuleE2E(t, capabilityChannelB, upstreamStandardMG, types.CostModeFree, types.CostRuleConfigV1{
					ZeroCostReason: "supplier contract",
				})
			},
		},
		{
			name: "per duration",
			seed: func(t *testing.T) {
				pricePerSecond := "0.1"
				seedProfitRoutingRuleE2E(t, capabilityChannelB, upstreamStandardMG, types.CostModePerDuration, types.CostRuleConfigV1{
					Currency: "USD", BillingMultiplier: "1", PurchaseDiscountRatio: "1",
					RechargeExchangeRatio: "1", FeeRate: "0", CurrencyToUSDRate: "1",
					PricePerSecond: &pricePerSecond, MeterSource: types.CostMeterValidatedRequest,
					ChargeEvent: types.CostChargeSubmitAccepted,
				})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := setupStrictProfitRoutingE2E(t, func(context.Context, service.RoutingRevenuePreviewInput) (int64, string, error) {
				return 2_000_000_000, "1000", nil
			})
			tt.seed(t)

			body := capabilityRequestBody(t, modelrouting.Seedance20, "720p", 10, "16:9", modelrouting.ReferenceLimits{}, false)
			status, response := performProfitRoutingRequest(t, env.engine, "Bearer e2e", "", body)

			require.Equal(t, http.StatusOK, status, string(response))
			assert.Empty(t, env.channelA.snapshot())
			selected := env.channelB.snapshot()
			require.Len(t, selected, 1)
			assert.Equal(t, http.MethodPost, selected[0].Method)
			assert.Equal(t, "/v1/video/generations", selected[0].Path)
		})
	}
}

func TestProfitRoutingMetadataUnavailableExcludesTokenAndDispatchesNonTokenE2E(t *testing.T) {
	env := setupStrictProfitRoutingE2E(t, func(context.Context, service.RoutingRevenuePreviewInput) (int64, string, error) {
		return 2_000_000_000, "1000", nil
	})
	seedProfitRoutingTokenRuleE2E(t, capabilityChannelA, upstreamStandard1080, "1")
	seedCostAccountingRule(t, capabilityChannelB, upstreamUpscaled1080, types.CostChargeSubmitAccepted, "1")
	metadataClient := &profitRoutingE2EMetadataClient{err: &service.VideoMetadataError{Kind: service.VideoMetadataUnavailable}}
	service.SetVideoMetadataClient(metadataClient)
	t.Cleanup(func() { service.SetVideoMetadataClient(nil) })

	body := capabilityRequestBody(t, modelrouting.Seedance20, "1080p", 15, "9:16", modelrouting.ReferenceLimits{Videos: 1}, false)
	body = strings.Replace(body, "video-a.mp4", "video-a.mp4?signature=secret-token", 1)
	status, response := performProfitRoutingRequest(t, env.engine, "Bearer e2e", "", body)

	require.Equal(t, http.StatusOK, status, string(response))
	assert.Empty(t, env.channelA.snapshot())
	selected := env.channelB.snapshot()
	require.Len(t, selected, 1)
	assert.Equal(t, http.MethodPost, selected[0].Method)
	assert.Equal(t, "/v1/video/generations", selected[0].Path)
	metadataCalls := metadataClient.snapshot()
	require.NotEmpty(t, metadataCalls)
	for _, assetURL := range metadataCalls {
		assert.Equal(t, "https://mock.example/video-a.mp4?signature=secret-token", assetURL)
	}
	assert.NotContains(t, string(response), "metadata_unavailable")
	assert.NotContains(t, string(response), "routing_diagnostics")
	assert.NotContains(t, string(response), "assets.example")
	assert.NotContains(t, string(response), "secret-token")

	adminLogs, _, err := model.GetAllLogs(model.LogTypeUnknown, 0, 0, modelrouting.Seedance20, "", "", 0, 20, 0, "", "", "")
	require.NoError(t, err)
	var diagnosticLog *model.Log
	for _, log := range adminLogs {
		if strings.Contains(log.Other, "routing_diagnostics") {
			diagnosticLog = log
			break
		}
	}
	require.NotNil(t, diagnosticLog)
	assert.NotContains(t, diagnosticLog.Other, "assets.example")
	assert.NotContains(t, diagnosticLog.Other, "secret-token")
	var other map[string]any
	require.NoError(t, common.UnmarshalJsonStr(diagnosticLog.Other, &other))
	adminInfo, ok := other["admin_info"].(map[string]any)
	require.True(t, ok)
	diagnostics, ok := adminInfo["routing_diagnostics"].([]any)
	require.True(t, ok)
	require.Len(t, diagnostics, 1)
	diagnostic, ok := diagnostics[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, float64(capabilityChannelA), diagnostic["channel_id"])
	assert.Equal(t, string(service.ProfitReasonMetadataUnavailable), diagnostic["reason"])

	userLogs, _, err := model.GetUserLogs(e2eUserID, model.LogTypeUnknown, 0, 0, "", "", 0, 20, "", "", "")
	require.NoError(t, err)
	require.NotEmpty(t, userLogs)
	for _, log := range userLogs {
		assert.NotContains(t, log.Other, "admin_info")
		assert.NotContains(t, log.Other, "routing_diagnostics")
		assert.NotContains(t, log.Other, "assets.example")
		assert.NotContains(t, log.Other, "secret-token")
	}
}

func TestProfitRoutingRejectsInvalidReferenceVideoBeforeUpstreamE2E(t *testing.T) {
	env := setupStrictProfitRoutingE2E(t, func(context.Context, service.RoutingRevenuePreviewInput) (int64, string, error) {
		return 2_000, "1000", nil
	})
	seedCostAccountingRule(t, capabilityChannelB, upstreamStandardMG, types.CostChargeSubmitAccepted, "1")
	metadataClient := &profitRoutingE2EMetadataClient{err: &service.VideoMetadataError{Kind: service.VideoMetadataInvalidMedia}}
	service.SetVideoMetadataClient(metadataClient)
	t.Cleanup(func() { service.SetVideoMetadataClient(nil) })

	body := capabilityRequestBody(t, modelrouting.Seedance20, "720p", 10, "16:9", modelrouting.ReferenceLimits{Videos: 1}, false)
	body = strings.Replace(body, "video-a.mp4", "video-a.mp4?signature=secret-token", 1)
	status, response := performProfitRoutingRequest(t, env.engine, "Bearer e2e", "", body)

	require.Equal(t, http.StatusBadRequest, status, string(response))
	assert.Contains(t, string(response), `"code":"invalid_request"`)
	assert.Contains(t, string(response), "input video is not supported")
	assert.NotContains(t, string(response), "assets.example")
	assert.NotContains(t, string(response), "secret-token")
	assert.Empty(t, env.channelA.snapshot())
	assert.Empty(t, env.channelB.snapshot())
	require.NotEmpty(t, metadataClient.snapshot())
}

func TestProfitRoutingUsesTotalReferenceVideoDurationToExcludeTokenCandidateE2E(t *testing.T) {
	env := setupStrictProfitRoutingE2E(t, func(context.Context, service.RoutingRevenuePreviewInput) (int64, string, error) {
		return 2_000, "1000", nil
	})
	// At $2/M tokens, the 15s output alone stays under the $2 revenue preview, while
	// the two reference videos' combined 11s input pushes the total above it.
	seedProfitRoutingTokenRuleE2E(t, capabilityChannelA, upstreamStandard1080, "2")
	seedCostAccountingRule(t, capabilityChannelB, upstreamUpscaled1080, types.CostChargeSubmitAccepted, "1")
	firstURL := "https://mock.example/video-a.mp4?signature=first-secret"
	secondURL := "https://mock.example/video-b.mp4?signature=second-secret"
	metadataClient := &profitRoutingE2EMetadataClient{results: map[string]videometa.Metadata{
		firstURL:  {DurationMS: 3_000, Width: 1280, Height: 720, FrameRateNum: 24, FrameRateDen: 1, Container: "mp4", ContentLength: 100},
		secondURL: {DurationMS: 8_000, Width: 1280, Height: 720, FrameRateNum: 24, FrameRateDen: 1, Container: "mp4", ContentLength: 100},
	}}
	service.SetVideoMetadataClient(metadataClient)
	t.Cleanup(func() { service.SetVideoMetadataClient(nil) })

	body := capabilityRequestBody(t, modelrouting.Seedance20, "1080p", 15, "9:16", modelrouting.ReferenceLimits{Videos: 2}, false)
	body = strings.Replace(body, "https://mock.example/video-a.mp4", firstURL, 1)
	body = strings.Replace(body, "https://mock.example/video-b.mp4", secondURL, 1)
	status, response := performProfitRoutingRequest(t, env.engine, "Bearer e2e", "", body)

	require.Equal(t, http.StatusOK, status, string(response))
	assert.Empty(t, env.channelA.snapshot(), "the token-priced candidate must be excluded before dispatch")
	require.Len(t, env.channelB.snapshot(), 1)
	calls := metadataClient.snapshot()
	assert.Contains(t, calls, firstURL)
	assert.Contains(t, calls, secondURL)
}

func TestProfitRoutingAutoAndSpecificSelectionRejectBeforeUpstreamE2E(t *testing.T) {
	t.Run("auto", func(t *testing.T) {
		env := setupStrictProfitRoutingE2E(t, func(context.Context, service.RoutingRevenuePreviewInput) (int64, string, error) {
			return 2_000, "1000", nil
		})
		previousAutoGroups := setting.AutoGroups2JsonString()
		previousUsableGroups := setting.UserUsableGroups2JSONString()
		require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["分组A"]`))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"分组A":"A","auto":"自动分组"}`))
		t.Cleanup(func() {
			require.NoError(t, setting.UpdateAutoGroupsByJsonString(previousAutoGroups))
			require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(previousUsableGroups))
		})
		require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", 1).Update("group", "auto").Error)
		seedCostAccountingRule(t, capabilityChannelB, upstreamStandardMG, types.CostChargeSubmitAccepted, "100")

		body := capabilityRequestBody(t, modelrouting.Seedance20, "720p", 10, "16:9", modelrouting.ReferenceLimits{}, false)
		status, response := performProfitRoutingRequest(t, env.engine, "Bearer e2e", "", body)

		require.Equal(t, http.StatusServiceUnavailable, status, string(response))
		assert.NotContains(t, string(response), "100")
		assert.Empty(t, env.channelA.snapshot())
		assert.Empty(t, env.channelB.snapshot())
	})

	t.Run("specific", func(t *testing.T) {
		env := setupStrictProfitRoutingE2E(t, func(context.Context, service.RoutingRevenuePreviewInput) (int64, string, error) {
			return 2_000, "1000", nil
		})
		seedCostAccountingRule(t, capabilityChannelB, upstreamStandardMG, types.CostChargeSubmitAccepted, "100")

		body := capabilityRequestBody(t, modelrouting.Seedance20, "720p", 10, "16:9", modelrouting.ReferenceLimits{}, false)
		status, response := performProfitRoutingRequest(t, env.engine, "Bearer e2e-2", "", body)

		require.Equal(t, http.StatusServiceUnavailable, status, string(response))
		assert.NotContains(t, string(response), "100")
		assert.Empty(t, env.channelA.snapshot())
		assert.Empty(t, env.channelB.snapshot())
	})
}

func TestProfitRoutingAffinityDoesNotBypassMarginGateE2E(t *testing.T) {
	env := setupStrictProfitRoutingE2E(t, func(context.Context, service.RoutingRevenuePreviewInput) (int64, string, error) {
		return 2_000, "1000", nil
	})
	seedCostAccountingRule(t, capabilityChannelB, upstreamStandardMG, types.CostChargeSubmitAccepted, "1")
	affinitySetting := operation_setting.GetChannelAffinitySetting()
	previousAffinitySetting := *affinitySetting
	*affinitySetting = operation_setting.ChannelAffinitySetting{
		Enabled: true, SwitchOnSuccess: true, MaxEntries: 100, DefaultTTLSeconds: 60,
		Rules: []operation_setting.ChannelAffinityRule{{
			Name: "profit-routing-e2e", ModelRegex: []string{"^doubao-seedance-2-0-260128$"},
			PathRegex:         []string{"/v1/video/generations"},
			KeySources:        []operation_setting.ChannelAffinityKeySource{{Type: "request_header", Key: "X-Profit-Affinity"}},
			IncludeUsingGroup: true, IncludeModelName: true, IncludeRuleName: true,
		}},
	}
	service.ClearChannelAffinityCacheAll()
	t.Cleanup(func() {
		service.ClearChannelAffinityCacheAll()
		*affinitySetting = previousAffinitySetting
	})

	body := capabilityRequestBody(t, modelrouting.Seedance20, "720p", 10, "16:9", modelrouting.ReferenceLimits{}, false)
	status, response := performProfitRoutingRequest(t, env.engine, "Bearer e2e", "route-lock", body)
	require.Equal(t, http.StatusOK, status, string(response))
	require.Len(t, env.channelB.snapshot(), 1)

	replaceProfitRoutingRequestPriceE2E(t, capabilityChannelB, upstreamStandardMG, "100")
	status, response = performProfitRoutingRequest(t, env.engine, "Bearer e2e", "route-lock", body)
	require.Equal(t, http.StatusServiceUnavailable, status, string(response))
	assert.NotContains(t, string(response), "100")
	assert.Len(t, env.channelB.snapshot(), 1)
}

func TestProfitRoutingRetryRejectsBeforeSecondUpstreamAndKeepsDiagnosticsPrivateE2E(t *testing.T) {
	env := setupStrictProfitRoutingE2E(t, func(context.Context, service.RoutingRevenuePreviewInput) (int64, string, error) {
		return 2_000, "1000", nil
	})
	seedCostAccountingRule(t, capabilityChannelA, upstreamStandard1080, types.CostChargeSubmitAccepted, "1")
	seedCostAccountingRule(t, capabilityChannelB, upstreamUpscaled1080, types.CostChargeSubmitAccepted, "100")
	metadataClient := &profitRoutingE2EMetadataClient{}
	service.SetVideoMetadataClient(metadataClient)
	t.Cleanup(func() { service.SetVideoMetadataClient(nil) })
	previousErrorLogEnabled := constant.ErrorLogEnabled
	constant.ErrorLogEnabled = true
	t.Cleanup(func() { constant.ErrorLogEnabled = previousErrorLogEnabled })
	env.channelA.submitStatus = http.StatusInternalServerError
	env.channelA.submitResponse = `{"error":{"code":"upstream_failure","message":"first channel failed"}}`
	previousRetryTimes := common.RetryTimes
	common.RetryTimes = 1
	t.Cleanup(func() { common.RetryTimes = previousRetryTimes })

	body := capabilityRequestBody(t, modelrouting.Seedance20, "1080p", 15, "9:16", modelrouting.ReferenceLimits{Videos: 1}, false)
	body = strings.Replace(body, "video-a.mp4", "video-a.mp4?signature=secret-token", 1)
	status, response := performProfitRoutingRequest(t, env.engine, "Bearer e2e", "", body)

	require.Equal(t, http.StatusServiceUnavailable, status, string(response))
	assert.Len(t, env.channelA.snapshot(), 1)
	assert.Empty(t, env.channelB.snapshot())
	assert.NotContains(t, string(response), "100")
	assert.NotContains(t, string(response), "assets.example")
	assert.NotContains(t, string(response), "secret-token")

	var adminLogs []*model.Log
	adminLogs, _, err := model.GetAllLogs(model.LogTypeError, 0, 0, modelrouting.Seedance20, "", "", 0, 20, 0, "", "", "")
	require.NoError(t, err)
	require.NotEmpty(t, adminLogs)
	var diagnosticLog *model.Log
	for _, log := range adminLogs {
		if strings.Contains(log.Other, "routing_diagnostics") {
			diagnosticLog = log
			break
		}
	}
	require.NotNil(t, diagnosticLog)
	assert.NotContains(t, diagnosticLog.Other, "assets.example")
	assert.NotContains(t, diagnosticLog.Other, "secret-token")
	var other map[string]any
	require.NoError(t, common.UnmarshalJsonStr(diagnosticLog.Other, &other))
	adminInfo, ok := other["admin_info"].(map[string]any)
	require.True(t, ok)
	diagnostics, ok := adminInfo["routing_diagnostics"].([]any)
	require.True(t, ok)
	require.NotEmpty(t, diagnostics)
	diagnostic, ok := diagnostics[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, string(service.ProfitReasonMarginBelowThreshold), diagnostic["reason"])

	userLogs, _, err := model.GetUserLogs(e2eUserID, model.LogTypeError, 0, 0, "", "", 0, 20, "", "", "")
	require.NoError(t, err)
	require.NotEmpty(t, userLogs)
	for _, log := range userLogs {
		assert.NotContains(t, log.Other, "admin_info")
		assert.NotContains(t, log.Other, "secret-token")
	}
}
