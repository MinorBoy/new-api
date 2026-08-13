package e2e

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	apidto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelrouting"
	"github.com/QuantumNous/new-api/pkg/videometa"
	"github.com/QuantumNous/new-api/relay"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/router"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/cost_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

const importedMaterialMatrixGroup = "default"
const importedMaterialMatrixAssetBaseURL = "https://cdn.openai.com/ark-matrix"
const importedMaterialMatrixGroupRatio = 1.25

type importedMaterialMatrixTarget struct {
	CaseID                             string
	RouteTargetRef                     string
	LineRef                            string
	ChannelRef                         string
	Provider                           string
	CanonicalModel                     string
	RuntimeModel                       string
	UpstreamModel                      string
	Resolution                         string
	Duration                           int
	Durations                          modelrouting.DurationConstraint
	CostVariantKey                     string
	Minimums                           modelrouting.ReferenceLimits
	References                         modelrouting.ReferenceLimits
	RequestRefs                        modelrouting.ReferenceLimits
	ReferenceTotalMax                  *int
	ReferenceVideoAudioTotalMax        *int
	ReferenceVideoTotalDurationSeconds *int
	AspectRatio                        string
	AspectRatios                       []string
	InputModes                         []modelrouting.InputMode
	ReferenceModes                     []string
	SupportsRealPerson                 *bool
	ChannelType                        int
	ExpectedPath                       string
	CostEnabled                        bool
	CostMode                           types.CostMode
	CostConfig                         types.CostRuleConfigV1
}

func TestImportedMaterialMatrixRequestReferencesRespectAggregateLimits(t *testing.T) {
	tests := []struct {
		name          string
		minimums      modelrouting.ReferenceLimits
		limits        modelrouting.ReferenceLimits
		totalMax      *int
		videoAudioMax *int
		want          modelrouting.ReferenceLimits
	}{
		{
			name: "933 with both aggregate limits", limits: modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3},
			totalMax: common.GetPointer(12), videoAudioMax: common.GetPointer(3),
			want: modelrouting.ReferenceLimits{Images: 9, Videos: 2, Audios: 1},
		},
		{
			name: "933 with total limit", limits: modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3},
			totalMax: common.GetPointer(12),
			want:     modelrouting.ReferenceLimits{Images: 9, Videos: 2, Audios: 1},
		},
		{
			name: "minimums are preserved", minimums: modelrouting.ReferenceLimits{Images: 1, Videos: 1, Audios: 1},
			limits: modelrouting.ReferenceLimits{Images: 4, Videos: 3, Audios: 3}, totalMax: common.GetPointer(4),
			want: modelrouting.ReferenceLimits{Images: 2, Videos: 1, Audios: 1},
		},
		{
			name: "independent limits unchanged", limits: modelrouting.ReferenceLimits{Images: 4, Videos: 3, Audios: 1},
			want: modelrouting.ReferenceLimits{Images: 4, Videos: 3, Audios: 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := importedMaterialMatrixRequestReferences(tt.minimums, tt.limits, tt.totalMax, tt.videoAudioMax)
			require.Equal(t, tt.want, got)
		})
	}
}

type importedMaterialMatrixEnv struct {
	engine           http.Handler
	mock             *capabilityRecordingServer
	channelIDs       map[string]int
	policyIDs        map[string]int
	publishedTargets map[string]model.RouteTarget
	materialSeen     map[string]int
	assetBaseURL     string
}

type importedMaterialMatrixVideoMetadataClient struct{}

func (importedMaterialMatrixVideoMetadataClient) Metadata(context.Context, string) (videometa.Metadata, error) {
	return videometa.Metadata{DurationMS: 5_000}, nil
}

type importedMaterialMatrixAudioDurationResolver struct{}

func (importedMaterialMatrixAudioDurationResolver) ResolveMS(context.Context, []string) (int64, error) {
	return 5_000, nil
}

func TestSeedanceImportedMaterialMatrixFullFlowE2E(t *testing.T) {
	silenceSeedanceBillingLogs(t)
	targets := loadImportedMaterialMatrixTargets(t)
	env := setupImportedMaterialMatrixE2E(t, targets)
	require.Len(t, targets, len(loadImportedMaterialMatrixDocument(t).Entities.CostRuleDrafts))
	checked := 0
	accepted := 0
	contractBlocks := 0
	disabledPricingDrafts := 0

	for _, target := range targets {
		t.Run(target.CaseID, func(t *testing.T) {
			checked++
			env.materialSeen[fmt.Sprintf("%d%d%d", target.References.Images, target.References.Videos, target.References.Audios)]++
			service.ClearChannelAffinityCacheAll()
			publishedTarget, ok := env.publishedTargets[target.RouteTargetRef]
			require.True(t, ok, target.CaseID)
			require.Equal(t, env.channelIDs[target.LineRef], publishedTarget.ChannelID, target.CaseID)
			require.Equal(t, target.UpstreamModel, publishedTarget.UpstreamModel, target.CaseID)
			require.Equal(t, target.CostVariantKey, publishedTarget.CostVariantKey, target.CaseID)
			var publishedConstraints modelrouting.Constraints
			require.NoError(t, common.UnmarshalJsonStr(publishedTarget.Constraints, &publishedConstraints), target.CaseID)
			require.Equal(t, target.References, publishedConstraints.ReferenceLimits, target.CaseID)
			require.Equal(t, target.Minimums, publishedConstraints.ReferenceMinimums, target.CaseID)
			request := service.RoutingPolicyWriteRequest{
				GroupName: importedMaterialMatrixGroup,
				Model:     target.RuntimeModel,
				Enabled:   true,
				Defaults: modelrouting.Defaults{
					OutputResolution: target.Resolution,
					DurationSeconds:  target.Duration,
					AspectRatio:      target.AspectRatio,
				},
				Targets: []service.RouteTargetWriteRequest{{
					ChannelID:      publishedTarget.ChannelID,
					Name:           publishedTarget.Name,
					UpstreamModel:  publishedTarget.UpstreamModel,
					CostVariantKey: publishedTarget.CostVariantKey,
					TargetPriority: publishedTarget.TargetPriority,
					Enabled:        true,
					Constraints:    publishedConstraints,
				}},
			}
			before := seedanceBillingDomainSnapshotFor(t, &seedanceBillingE2EEnv{
				User: &model.User{Id: e2eUserID}, Token: &model.Token{Id: 1},
				Channel: &model.Channel{Id: env.channelIDs[target.LineRef]},
			})
			costBefore := importedMaterialMatrixCostSnapshot(t)
			requestsBefore := env.mock.snapshot()
			var channel model.Channel
			require.NoError(t, model.DB.Where("id = ?", env.channelIDs[target.LineRef]).First(&channel).Error, target.CaseID)
			require.Equal(t, target.ChannelType, channel.Type, target.CaseID)
			if !target.CostEnabled {
				disabledPricingDrafts++
				var costRuleCount int64
				require.NoError(t, model.DB.Model(&model.ChannelModelCostRule{}).Where(
					"channel_id = ? AND billable_upstream_model = ? AND cost_variant_key = ? AND status = ?",
					channel.Id, target.UpstreamModel, target.CostVariantKey, types.CostRuleActive,
				).Count(&costRuleCount).Error, target.CaseID)
				require.Zero(t, costRuleCount, target.CaseID)
				after := seedanceBillingDomainSnapshotFor(t, &seedanceBillingE2EEnv{
					User: &model.User{Id: e2eUserID}, Token: &model.Token{Id: 1},
					Channel: &model.Channel{Id: env.channelIDs[target.LineRef]},
				})
				require.Zero(t, after.delta(before), target.CaseID)
				require.Equal(t, costBefore, importedMaterialMatrixCostSnapshot(t), target.CaseID)
				require.Equal(t, requestsBefore, env.mock.snapshot(), target.CaseID)
				return
			}
			var costRule model.ChannelModelCostRule
			require.NoError(t, model.DB.Where(
				"channel_id = ? AND billable_upstream_model = ? AND cost_variant_key = ? AND status = ?",
				channel.Id, target.UpstreamModel, target.CostVariantKey, types.CostRuleActive,
			).First(&costRule).Error, target.CaseID)
			require.Equal(t, string(target.CostMode), costRule.CostMode, target.CaseID)
			require.Equal(t, "config_import", costRule.Source, target.CaseID)
			var costConfig types.CostRuleConfigV1
			require.NoError(t, common.UnmarshalJsonStr(costRule.ConfigJSON, &costConfig), target.CaseID)
			require.Equal(t, target.CostConfig.Currency, costConfig.Currency, target.CaseID)
			require.Equal(t, target.CostConfig.UnitPrice, costConfig.UnitPrice, target.CaseID)
			require.Equal(t, target.CostConfig.PricePerSecond, costConfig.PricePerSecond, target.CaseID)
			require.Equal(t, target.CostConfig.InputPerMillion, costConfig.InputPerMillion, target.CaseID)
			require.Equal(t, target.CostConfig.OutputPerMillion, costConfig.OutputPerMillion, target.CaseID)
			require.Equal(t, target.CostConfig.CompletionPerMillion, costConfig.CompletionPerMillion, target.CaseID)
			require.Equal(t, target.CostConfig.TotalPerMillion, costConfig.TotalPerMillion, target.CaseID)
			require.Equal(t, target.CostConfig.NormalizedUSDPrices, costConfig.NormalizedUSDPrices, target.CaseID)

			view, err := service.SaveRoutingPolicy(env.policyIDs[target.RuntimeModel], request)
			if err != nil {
				var policyErr *service.RoutingPolicyServiceError
				require.True(t, errors.As(err, &policyErr), "%s: %v", target.CaseID, err)
				require.Equal(t, "incompatible_channel_contract", policyErr.Code, target.CaseID)
				require.Equal(t, "targets.0.constraints", policyErr.Field, target.CaseID)
				t.Logf("contract blocked: provider=%s target=%s reason=%s", target.Provider, target.RouteTargetRef, policyErr.Error())
				contractBlocks++
				after := seedanceBillingDomainSnapshotFor(t, &seedanceBillingE2EEnv{
					User: &model.User{Id: e2eUserID}, Token: &model.Token{Id: 1},
					Channel: &model.Channel{Id: env.channelIDs[target.LineRef]},
				})
				require.Zero(t, after.delta(before), target.CaseID)
				require.Equal(t, costBefore, importedMaterialMatrixCostSnapshot(t), target.CaseID)
				require.Equal(t, requestsBefore, env.mock.snapshot(), target.CaseID)
				return
			}
			env.policyIDs[target.RuntimeModel] = view.ID
			accepted++

			body := importedMaterialMatrixRequestBody(t, target, env.assetBaseURL)
			status, submit := performJSONRequest(t, env.engine, http.MethodPost, "/api/v3/contents/generations/tasks", "Bearer e2e", body)
			require.Equal(t, http.StatusOK, status, "%s: %s", target.CaseID, submit)

			var created struct {
				ID string `json:"id"`
			}
			require.NoError(t, common.Unmarshal(submit, &created), target.CaseID)
			require.True(t, strings.HasPrefix(created.ID, "task_"), target.CaseID)
			require.NotContains(t, string(submit), target.UpstreamModel, target.CaseID)

			requests := env.mock.snapshot()
			require.Len(t, requests, len(requestsBefore)+1, target.CaseID)
			submitRequest := requests[len(requestsBefore)]
			require.Equal(t, http.MethodPost, submitRequest.Method, target.CaseID)
			require.Equal(t, target.ExpectedPath, submitRequest.Path, target.CaseID)
			require.Equal(t, fmt.Sprintf("Bearer matrix-key-%d", env.channelIDs[target.LineRef]), submitRequest.Authorization, target.CaseID)
			if strings.HasPrefix(submitRequest.ContentType, "multipart/") {
				require.Contains(t, string(submitRequest.Body), target.UpstreamModel, target.CaseID)
			} else {
				var upstreamRequest map[string]any
				require.NoError(t, common.Unmarshal(submitRequest.Body, &upstreamRequest), target.CaseID)
				require.Equal(t, target.UpstreamModel, upstreamRequest["model"], target.CaseID)
				require.NotContains(t, upstreamRequest, "routing", target.CaseID)
			}

			var task model.Task
			require.NoError(t, model.DB.Where("task_id = ?", created.ID).First(&task).Error, target.CaseID)
			require.Equal(t, env.channelIDs[target.LineRef], task.ChannelId, target.CaseID)
			require.NotNil(t, task.PrivateData.Routing, target.CaseID)
			require.Equal(t, target.UpstreamModel, task.PrivateData.Routing.UpstreamModel, target.CaseID)
			require.Equal(t, target.CostVariantKey, task.PrivateData.Routing.CostVariantKey, target.CaseID)
			require.Positive(t, task.Quota, target.CaseID)
			require.NotNil(t, task.PrivateData.BillingContext, target.CaseID)
			hasVideoInput := target.RequestRefs.Videos > 0
			require.Equal(t, hasVideoInput, task.PrivateData.BillingContext.HasVideoInput, target.CaseID)
			require.Equal(t, importedMaterialMatrixGroupRatio, task.PrivateData.BillingContext.GroupRatio, target.CaseID)
			require.Equal(t, billing_setting.BillingModeSeedanceTokens, task.PrivateData.BillingContext.BillingMode, target.CaseID)
			require.NotNil(t, task.PrivateData.BillingContext.SeedanceTokenPrice, target.CaseID)
			require.NotNil(t, task.PrivateData.BillingContext.SeedanceTokenBilling, target.CaseID)
			seedanceBilling := task.PrivateData.BillingContext.SeedanceTokenBilling
			expectedFinalCharge, err := decimal.NewFromString(seedanceBilling.FinalCharge)
			require.NoError(t, err, target.CaseID)
			expectedQuota := common.QuotaFromDecimal(expectedFinalCharge.Mul(decimal.NewFromFloat(common.QuotaPerUnit)))
			require.Equal(t, expectedQuota, task.Quota, target.CaseID)
			require.Equal(t, "official-token-v1", seedanceBilling.PricingVersion, target.CaseID)
			require.Equal(t, target.Resolution, seedanceBilling.Resolution, target.CaseID)
			require.Equal(t, target.Duration, seedanceBilling.OutputDurationSeconds, target.CaseID)
			require.Positive(t, seedanceBilling.OutputTokens, target.CaseID)
			require.Zero(t, seedanceBilling.InputTokens, target.CaseID)
			require.Equal(t, seedanceBilling.OutputTokens, seedanceBilling.TotalTokens, target.CaseID)
			if hasVideoInput {
				require.Positive(t, seedanceBilling.InputVideoDurationMS, target.CaseID)
			} else {
				require.Zero(t, seedanceBilling.InputVideoDurationMS, target.CaseID)
				require.Zero(t, seedanceBilling.InputTokens, target.CaseID)
			}
			require.Equal(t, model.TaskUsageSnapshotVersion1, task.PrivateData.BillingContext.UsageSnapshotVersion, target.CaseID)
			require.Positive(t, task.PrivateData.BillingContext.UsageCompletionTokens, target.CaseID)
			require.Equal(t, task.PrivateData.BillingContext.UsageCompletionTokens, task.PrivateData.BillingContext.UsageTotalTokens, target.CaseID)
			preConsumedQuota := task.Quota

			summary := service.RunTaskPollingOnce(context.Background(), nil)
			require.Equal(t, 1, summary.UnfinishedTasks, target.CaseID)
			require.NoError(t, model.DB.Where("task_id = ?", created.ID).First(&task).Error, target.CaseID)
			require.Equal(t, string(model.TaskStatusSuccess), string(task.Status), target.CaseID)
			require.NotNil(t, task.PrivateData.BillingContext, target.CaseID)
			require.Equal(t, model.TaskUsageProfileSeedance, task.PrivateData.BillingContext.UsageProfile, target.CaseID)
			require.Contains(t, []string{model.TaskUsageSourceUpstream, model.TaskUsageSourceLocalCalculated}, task.PrivateData.BillingContext.UsageSource, target.CaseID)
			require.Positive(t, task.PrivateData.BillingContext.BillingTokens, target.CaseID)

			status, single := performJSONRequest(t, env.engine, http.MethodGet, "/api/v3/contents/generations/tasks/"+created.ID, "Bearer e2e", "")
			require.Equal(t, http.StatusOK, status, "%s: %s", target.CaseID, single)
			require.Contains(t, string(single), target.RuntimeModel, target.CaseID)
			require.NotContains(t, string(single), target.UpstreamModel, target.CaseID)
			var publicTask struct {
				Usage struct {
					CompletionTokens int `json:"completion_tokens"`
					TotalTokens      int `json:"total_tokens"`
				} `json:"usage"`
			}
			require.NoError(t, common.Unmarshal(single, &publicTask), target.CaseID)
			require.Positive(t, publicTask.Usage.CompletionTokens, target.CaseID)
			require.Equal(t, publicTask.Usage.CompletionTokens, publicTask.Usage.TotalTokens, target.CaseID)
			require.Equal(t, task.PrivateData.BillingContext.UsageCompletionTokens, publicTask.Usage.CompletionTokens, target.CaseID)
			require.Equal(t, task.PrivateData.BillingContext.UsageTotalTokens, publicTask.Usage.TotalTokens, target.CaseID)
			require.Equal(t, task.PrivateData.BillingContext.UsageTotalTokens, task.PrivateData.BillingContext.BillingTokens, target.CaseID)
			require.NotContains(t, string(single), "usage_source", target.CaseID)

			after := seedanceBillingDomainSnapshotFor(t, &seedanceBillingE2EEnv{
				User: &model.User{Id: e2eUserID}, Token: &model.Token{Id: 1},
				Channel: &model.Channel{Id: env.channelIDs[target.LineRef]},
			}, created.ID)
			delta := after.delta(before)
			require.Equal(t, int64(1), delta.TaskCount, target.CaseID)
			require.Equal(t, 1, delta.UserRequestCount, target.CaseID)
			quotaDelta := task.Quota - preConsumedQuota
			expectedLogCount := int64(1)
			expectedConsumeLogCount := 1
			expectedRefundLogCount := 0
			if quotaDelta != 0 {
				expectedLogCount = 2
			}
			if quotaDelta > 0 {
				expectedConsumeLogCount = 2
			} else if quotaDelta < 0 {
				expectedRefundLogCount = 1
			}
			require.Equal(t, expectedLogCount, delta.LogCount, target.CaseID)
			require.Equal(t, expectedConsumeLogCount, delta.ConsumeLogCount, target.CaseID)
			require.Equal(t, expectedRefundLogCount, delta.RefundLogCount, target.CaseID)
			require.Equal(t, task.Quota, delta.UserUsedQuota, target.CaseID)
			require.Equal(t, int64(task.Quota), delta.ChannelUsedQuota, target.CaseID)
			require.Equal(t, task.Quota, delta.TokenUsedQuota, target.CaseID)
			require.Equal(t, 1, delta.QuotaDataCount, target.CaseID)
			require.Equal(t, task.Quota, delta.QuotaDataQuota, target.CaseID)
			if quotaDelta != 0 {
				require.Equal(t, task.PrivateData.BillingContext.BillingTokens, delta.QuotaDataTokenUsed, target.CaseID)
			} else {
				require.Contains(t, []int{0, task.PrivateData.BillingContext.BillingTokens}, delta.QuotaDataTokenUsed, target.CaseID)
			}
			if expectedLogCount == 2 {
				require.Equal(t, 1, delta.SettlementLogCount, target.CaseID)
				require.Equal(t, 1, delta.SettlementConsumeLogCount+delta.SettlementRefundLogCount, target.CaseID)
				require.True(t, delta.SettlementLogBillingTokensPresent, target.CaseID)
				require.Equal(t, task.PrivateData.BillingContext.BillingTokens, delta.SettlementLogBillingTokens, target.CaseID)
			}

			logs := seedanceBillingLogsAfter(t, &seedanceBillingE2EEnv{User: &model.User{Id: e2eUserID}}, before.LastLogID)
			require.Len(t, logs, int(expectedLogCount), target.CaseID)
			require.Equal(t, true, logs[0].Other["is_task"], target.CaseID)
			require.Equal(t, "/v1/video/generations", logs[0].Other["request_path"], target.CaseID)
			if expectedLogCount == 2 {
				require.Equal(t, created.ID, logs[1].Other["task_id"], target.CaseID)
				require.Equal(t, float64(task.PrivateData.BillingContext.BillingTokens), logs[1].Other["billing_tokens"], target.CaseID)
			}

			var costRequest model.CostAccountingRequest
			require.NoError(t, model.DB.Where("task_id = ?", created.ID).First(&costRequest).Error, target.CaseID)
			var costAttempt model.CostAccountingAttempt
			require.NoError(t, model.DB.Where("cost_request_id = ?", costRequest.ID).First(&costAttempt).Error, target.CaseID)
			require.Equal(t, target.ChannelType, costAttempt.ChannelType, target.CaseID)
			require.Equal(t, target.UpstreamModel, costAttempt.BillableUpstreamModel, target.CaseID)
			require.Equal(t, target.CostVariantKey, costAttempt.CostVariantKey, target.CaseID)
			require.Equal(t, string(target.CostMode), costAttempt.CostMode, target.CaseID)
			require.Equal(t, string(types.CostAttemptSettled), costAttempt.Status, target.CaseID)
			require.NotNil(t, costAttempt.CostNanoUSD, target.CaseID)
			if target.CostMode == types.CostModePerToken {
				require.Contains(t, costAttempt.ActualMeterJSON, "completion_tokens", target.CaseID)
				require.Contains(t, costAttempt.ActualMeterJSON, "total_tokens", target.CaseID)
			}
			var attemptConfig types.CostRuleConfigV1
			require.NoError(t, common.UnmarshalJsonStr(costAttempt.RuleConfigJSON, &attemptConfig), target.CaseID)
			require.Equal(t, target.CostConfig.Currency, attemptConfig.Currency, target.CaseID)
			require.Equal(t, target.CostConfig.UnitPrice, attemptConfig.UnitPrice, target.CaseID)
			require.Equal(t, target.CostConfig.PricePerSecond, attemptConfig.PricePerSecond, target.CaseID)
			require.Equal(t, target.CostConfig.NormalizedUSDPrices, attemptConfig.NormalizedUSDPrices, target.CaseID)
		})
	}

	if checked == len(targets) {
		require.Equal(t, len(targets), accepted)
		require.Zero(t, contractBlocks)
		require.Zero(t, disabledPricingDrafts)
		require.Equal(t, importedMaterialMatrixMaterialCounts(targets), env.materialSeen)
	}
}

func importedMaterialMatrixMaterialCounts(targets []importedMaterialMatrixTarget) map[string]int {
	counts := make(map[string]int)
	for _, target := range targets {
		counts[fmt.Sprintf("%d%d%d", target.References.Images, target.References.Videos, target.References.Audios)]++
	}
	return counts
}

func setupImportedMaterialMatrixE2E(t *testing.T, targets []importedMaterialMatrixTarget) *importedMaterialMatrixEnv {
	t.Helper()
	setupSeedanceE2EDB(t)
	require.NoError(t, model.DB.AutoMigrate(
		&model.RoutingPolicy{}, &model.RouteTarget{}, &model.ChannelModelCostRule{},
		&model.CostAccountingRequest{}, &model.CostAccountingAttempt{}, &model.CostAccountingAudit{},
		&model.ConfigImportBatch{}, &model.ConfigImportItem{}, &model.ConfigImportBinding{},
		&model.ConfigImportIssue{}, &model.ConfigImportResolution{}, &model.ConfigImportPublishAudit{},
		&model.ConfigImportActivationAudit{}, &model.ConfigImportRouteOwnershipChange{}, &model.Option{},
	))
	require.NoError(t, model.InitRoutingPolicyCache())
	t.Cleanup(func() {
		if model.DB != nil {
			require.NoError(t, model.InitRoutingPolicyCache())
		}
	})
	common.OptionMapRWMutex.Lock()
	previousOptionMap := common.OptionMap
	common.OptionMap = make(map[string]string)
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptionMap
		common.OptionMapRWMutex.Unlock()
	})
	billingConfig := config.GlobalConfig.Get("billing_setting")
	originalBillingConfig, err := config.ConfigToMap(billingConfig)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, config.UpdateConfigFromMap(billingConfig, originalBillingConfig))
	})
	originalCostRuntime := cost_setting.Runtime()
	costConfig := config.GlobalConfig.Get(cost_setting.ConfigName)
	require.NoError(t, config.UpdateConfigFromMap(costConfig, map[string]string{
		cost_setting.KeyMode:                     string(types.CostAccountingTracking),
		cost_setting.KeyMinimumExpectedMarginBPS: "0",
	}))
	cost_setting.UpdateAndSync()
	require.Equal(t, types.CostAccountingTracking, cost_setting.Runtime().Mode)
	t.Cleanup(func() {
		require.NoError(t, config.UpdateConfigFromMap(costConfig, map[string]string{
			cost_setting.KeyMode:                     string(originalCostRuntime.Mode),
			cost_setting.KeyMinimumExpectedMarginBPS: fmt.Sprintf("%d", originalCostRuntime.MinimumExpectedMarginBPS),
		}))
		cost_setting.UpdateAndSync()
	})

	user := &model.User{
		Id: e2eUserID, Username: "ark_sdk_material_matrix_user", Password: "e2e-password",
		Role: common.RoleRootUser, Status: common.UserStatusEnabled, Quota: 2_000_000_000,
		Group: importedMaterialMatrixGroup, AffCode: "ark-sdk-material-matrix-user",
	}
	require.NoError(t, model.DB.Create(user).Error)
	require.NoError(t, model.DB.Create(&model.Token{
		Id: 1, UserId: e2eUserID, Key: e2eToken, Status: common.TokenStatusEnabled,
		RemainQuota: 2_000_000_000, UnlimitedQuota: true, Group: importedMaterialMatrixGroup,
	}).Error)

	originalGroupRatios := ratio_setting.GroupRatio2JSONString()
	groupRatios := ratio_setting.GetGroupRatioCopy()
	groupRatios[importedMaterialMatrixGroup] = importedMaterialMatrixGroupRatio
	encodedGroupRatios, err := common.Marshal(groupRatios)
	require.NoError(t, err)
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(string(encodedGroupRatios)))
	t.Cleanup(func() { require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalGroupRatios)) })

	mock := &capabilityRecordingServer{}
	server := httptestNewServer(t, importedMaterialMatrixAssetHandler(mock))
	service.SetVideoMetadataClient(importedMaterialMatrixVideoMetadataClient{})
	t.Cleanup(func() { service.SetVideoMetadataClient(nil) })
	service.SetReferenceAudioDurationResolver(importedMaterialMatrixAudioDurationResolver{})
	t.Cleanup(func() { service.SetReferenceAudioDurationResolver(nil) })
	document := loadImportedMaterialMatrixDocument(t)
	channelIDs := seedImportedMaterialMatrixChannels(t, server.URL, document)

	previousGetTaskAdaptorFunc := service.GetTaskAdaptorFunc
	previousCostCapabilityLookup := service.CostCapabilityLookup
	previousRouteTargetContractValidator := service.RouteTargetContractValidator
	service.GetTaskAdaptorFunc = func(platform constant.TaskPlatform) service.TaskPollingAdaptor {
		return relay.GetTaskAdaptor(platform)
	}
	service.CostCapabilityLookup = relay.CostCapabilitiesForRoute
	service.RouteTargetContractValidator = relay.ValidateVideoRouteTargetContract
	t.Cleanup(func() {
		service.GetTaskAdaptorFunc = previousGetTaskAdaptorFunc
		service.CostCapabilityLookup = previousCostCapabilityLookup
		service.RouteTargetContractValidator = previousRouteTargetContractValidator
	})

	payload, err := os.ReadFile(filepath.Join("testdata", "channel-config-v1.json"))
	require.NoError(t, err)
	batch, created, err := service.CreateConfigImportBatch(context.Background(), 1, bytes.NewReader(payload))
	require.NoError(t, err)
	require.True(t, created)
	bindings := make([]apidto.ConfigImportBindingInput, 0, len(document.Entities.ChannelLines))
	for _, line := range document.Entities.ChannelLines {
		channelID := channelIDs[line.LineRef]
		require.Positive(t, channelID, line.LineRef)
		bindings = append(bindings, apidto.ConfigImportBindingInput{
			LineRef: line.LineRef, Action: types.ConfigImportBindingActionBind,
			ChannelID: &channelID, CredentialsConfirmed: true,
		})
	}
	_, err = service.UpdateConfigImportBindings(context.Background(), 1, batch.ID, bindings)
	require.NoError(t, err)
	staged, err := service.StageConfigImportBatch(context.Background(), 1, batch.ID)
	require.NoError(t, err)
	require.Equal(t, types.ConfigImportBatchStatusReady, staged.Status)
	require.NoError(t, service.PublishConfigImportBatch(context.Background(), batch.ID, 1))
	var publishedBatch model.ConfigImportBatch
	require.NoError(t, model.DB.First(&publishedBatch, batch.ID).Error)
	require.Equal(t, string(types.ConfigImportBatchStatusPublished), publishedBatch.Status)
	var activeRuleCount int64
	require.NoError(t, model.DB.Model(&model.ChannelModelCostRule{}).
		Where("source = ? AND status = ?", "config_import", types.CostRuleActive).
		Count(&activeRuleCount).Error)
	require.Zero(t, activeRuleCount)
	var draftRuleCount int64
	require.NoError(t, model.DB.Model(&model.ChannelModelCostRule{}).
		Where("source = ? AND status = ?", "config_import", types.CostRuleDraft).
		Count(&draftRuleCount).Error)
	require.EqualValues(t, len(document.Entities.CostRuleDrafts), draftRuleCount)
	preview, err := service.PreviewConfigImportBatchActivation(context.Background(), batch.ID)
	require.NoError(t, err)
	require.Truef(t, preview.Ready, "activation blockers: %+v", preview.Blockers)
	_, err = service.ActivateConfigImportBatch(context.Background(), batch.ID, 1)
	require.NoError(t, err)
	require.NoError(t, model.DB.Model(&model.ChannelModelCostRule{}).
		Where("source = ? AND status = ?", "config_import", types.CostRuleActive).
		Count(&activeRuleCount).Error)
	require.EqualValues(t, len(document.Entities.CostRuleDrafts), activeRuleCount)

	policyIDs := make(map[string]int)
	var policies []model.RoutingPolicy
	require.NoError(t, model.DB.Where("group_name = ?", importedMaterialMatrixGroup).Find(&policies).Error)
	for _, policy := range policies {
		policyIDs[policy.Model] = policy.ID
	}
	publishedTargets := make(map[string]model.RouteTarget, len(targets))
	var routeTargets []model.RouteTarget
	require.NoError(t, model.DB.Find(&routeTargets).Error)
	for _, routeTarget := range routeTargets {
		publishedTargets[routeTarget.Name] = routeTarget
	}
	require.Len(t, publishedTargets, len(targets))
	model.InitChannelCache()

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	router.SetRelayRouter(engine)
	router.SetVideoRouter(engine)
	return &importedMaterialMatrixEnv{
		engine: engine, mock: mock, channelIDs: channelIDs,
		policyIDs: policyIDs, publishedTargets: publishedTargets, materialSeen: map[string]int{}, assetBaseURL: importedMaterialMatrixAssetBaseURL,
	}
}

func seedImportedMaterialMatrixChannels(t *testing.T, upstreamURL string, document types.ConfigImportDocument) map[string]int {
	t.Helper()
	lineRefs := make([]string, 0, len(document.Entities.ChannelLines))
	for _, line := range document.Entities.ChannelLines {
		lineRefs = append(lineRefs, line.LineRef)
	}
	sort.Strings(lineRefs)

	channelIDs := make(map[string]int, len(lineRefs))
	for index, lineRef := range lineRefs {
		channelID := index + 1
		channelIDs[lineRef] = channelID
		priority := int64(100 - index)
		weight := uint(100)
		channelType := importedMaterialChannelType(t, document, lineRef)
		channel := &model.Channel{
			Id: channelID, Type: channelType, Key: fmt.Sprintf("matrix-key-%d", channelID),
			Status: common.ChannelStatusEnabled, Name: lineRef, BaseURL: common.GetPointer(upstreamURL),
			Models: strings.Join(importedMaterialChannelModels(t, document, lineRef), ","),
			Group:  importedMaterialMatrixGroup, Priority: &priority, Weight: &weight,
			CreatedTime: time.Now().Unix(), OtherSettings: "{}",
		}
		settings := dto.ChannelOtherSettings{DisableTaskPollingSleep: true}
		if channelType == constant.ChannelTypeSecure {
			switch lineRef {
			case "secure-discount":
				settings.SecureVideoGroup = dto.SecureVideoGroupDiscount
			case "secure-overseas":
				settings.SecureVideoGroup = dto.SecureVideoGroupOverseas
			case "secure-enterprise":
				settings.SecureVideoGroup = dto.SecureVideoGroupEnterprise
			}
		}
		channel.SetOtherSettings(settings)
		require.NoError(t, channel.Insert(), lineRef)
	}
	return channelIDs
}

func importedMaterialChannelType(t *testing.T, document types.ConfigImportDocument, lineRef string) int {
	t.Helper()
	channelRef := ""
	for _, line := range document.Entities.ChannelLines {
		if line.LineRef == lineRef {
			channelRef = line.ChannelRef
			break
		}
	}
	require.NotEmpty(t, channelRef, lineRef)
	for _, channel := range document.Entities.Channels {
		if channel.BusinessID != channelRef {
			continue
		}
		require.NotNil(t, channel.ChannelType, channelRef)
		channelType := *channel.ChannelType
		if channelRef == "CH-4STOKEN" && channelType == constant.ChannelTypeOpenAI {
			return constant.ChannelTypeFourSToken
		}
		if channelRef == "CH-8YES" && channelType == constant.ChannelTypeOpenAI {
			return constant.ChannelTypeEightYes
		}
		return channelType
	}
	require.FailNow(t, "imported channel is missing", channelRef)
	return 0
}

func TestImportedMaterialChannelTypePreservesMikotoType(t *testing.T) {
	document := loadImportedMaterialMatrixDocument(t)
	require.Equal(t, constant.ChannelTypeMikoto, importedMaterialChannelType(t, document, "mikoto-sd"))
}

func importedMaterialChannelModels(t *testing.T, document types.ConfigImportDocument, lineRef string) []string {
	t.Helper()
	modelSet := make(map[string]struct{})
	for _, mapping := range document.Entities.ModelMappings {
		if mapping.LineRef != lineRef || strings.TrimSpace(mapping.UpstreamModel) == "" {
			continue
		}
		modelSet[mapping.UpstreamModel] = struct{}{}
		if runtimeModel := importedMaterialMatrixRuntimeModel(mapping.CanonicalModel); runtimeModel != "" {
			modelSet[runtimeModel] = struct{}{}
		}
	}
	models := make([]string, 0, len(modelSet))
	for modelName := range modelSet {
		models = append(models, modelName)
	}
	sort.Strings(models)
	require.NotEmpty(t, models, lineRef)
	return models
}

func loadImportedMaterialMatrixTargets(t *testing.T) []importedMaterialMatrixTarget {
	t.Helper()
	document := loadImportedMaterialMatrixDocument(t)
	lines := make(map[string]types.ConfigImportChannelLine, len(document.Entities.ChannelLines))
	for _, line := range document.Entities.ChannelLines {
		lines[line.LineRef] = line
	}
	costRules := make(map[string]types.ConfigImportCostRuleDraft, len(document.Entities.CostRuleDrafts))
	for _, draft := range document.Entities.CostRuleDrafts {
		costRules[draft.RouteTargetRef] = draft
	}

	targets := make([]importedMaterialMatrixTarget, 0)
	for _, blueprint := range document.Entities.RouteBlueprints {
		runtimeModel := importedMaterialMatrixRuntimeModel(blueprint.CanonicalModel)
		require.NotEmpty(t, runtimeModel, blueprint.BusinessID)
		for _, target := range blueprint.Targets {
			require.NotNil(t, target.ReferenceLimits, target.RouteTargetRef)
			minimums := modelrouting.ReferenceLimits{}
			if target.ReferenceMinimums != nil {
				minimums = importedMaterialMatrixReferenceLimits(t, target.ReferenceMinimums, target.RouteTargetRef)
			}
			references := importedMaterialMatrixReferenceLimits(t, target.ReferenceLimits, target.RouteTargetRef)
			requestReferences := importedMaterialMatrixRequestReferences(
				minimums, references, target.ReferenceTotalMax, target.ReferenceVideoAudioTotalMax,
			)
			require.NotEmpty(t, target.OutputResolutions, target.RouteTargetRef)
			duration := 10
			durations := modelrouting.DurationConstraint{
				Values: append([]int(nil), target.DurationValues...), Min: target.DurationMin, Max: target.DurationMax,
			}
			if len(target.DurationValues) > 0 {
				duration = target.DurationValues[0]
			} else if target.DurationMin != nil {
				duration = *target.DurationMin
			}
			if len(durations.Values) == 0 && durations.Min == nil && durations.Max == nil {
				durations.Min = common.GetPointer(duration)
				durations.Max = common.GetPointer(duration)
			}
			aspectRatios := make([]string, 0, len(target.AspectRatios))
			aspectRatio := "16:9"
			for _, value := range target.AspectRatios {
				normalized := strings.ToLower(strings.TrimSpace(value))
				if normalized == "" {
					continue
				}
				aspectRatios = append(aspectRatios, normalized)
				if normalized == "16:9" || len(aspectRatios) == 1 && aspectRatio == "16:9" {
					aspectRatio = normalized
				}
			}
			if len(aspectRatios) == 0 {
				aspectRatios = []string{aspectRatio}
			}
			inputModes := make([]modelrouting.InputMode, 0, len(target.InputModes))
			for _, value := range target.InputModes {
				inputModes = append(inputModes, modelrouting.InputMode(strings.ToLower(strings.TrimSpace(value))))
			}
			requestReferences = importedMaterialMatrixApplyInputMode(requestReferences, references, inputModes)
			line, ok := lines[target.LineRef]
			require.True(t, ok, target.LineRef)
			draft, ok := costRules[target.RouteTargetRef]
			require.True(t, ok, target.RouteTargetRef)
			require.NotNil(t, draft.Enabled, target.RouteTargetRef)
			costMode, costConfig := importedMaterialCostRuleConfig(t, draft)
			channelType := importedMaterialChannelType(t, document, target.LineRef)
			targets = append(targets, importedMaterialMatrixTarget{
				CaseID:         fmt.Sprintf("%s/%s/%d%d%d", target.RouteTargetRef, target.OutputResolutions[0], references.Images, references.Videos, references.Audios),
				RouteTargetRef: target.RouteTargetRef,
				LineRef:        target.LineRef, ChannelRef: line.ChannelRef, Provider: line.ProviderTypeHint,
				CanonicalModel: blueprint.CanonicalModel, RuntimeModel: runtimeModel, UpstreamModel: target.UpstreamModel,
				Resolution: strings.ToLower(target.OutputResolutions[0]), Duration: duration, Durations: durations,
				CostVariantKey: target.CostVariantKey, Minimums: minimums, References: references, RequestRefs: requestReferences,
				ReferenceTotalMax: target.ReferenceTotalMax, ReferenceVideoAudioTotalMax: target.ReferenceVideoAudioTotalMax,
				ReferenceVideoTotalDurationSeconds: target.ReferenceVideoTotalDurationSeconds,
				AspectRatio:                        aspectRatio, AspectRatios: aspectRatios, InputModes: inputModes,
				ReferenceModes: append([]string(nil), target.ReferenceModes...), SupportsRealPerson: target.SupportsRealPerson,
				ChannelType: channelType, CostEnabled: *draft.Enabled,
				ExpectedPath: importedMaterialSubmitPath(channelType, target.LineRef), CostMode: costMode, CostConfig: costConfig,
			})
		}
	}
	sort.Slice(targets, func(left, right int) bool {
		return targets[left].CaseID < targets[right].CaseID
	})
	return targets
}

func importedMaterialCostRuleConfig(t *testing.T, draft types.ConfigImportCostRuleDraft) (types.CostMode, types.CostRuleConfigV1) {
	t.Helper()
	mode := types.CostMode(strings.TrimSpace(draft.CostMode))
	config := types.CostRuleConfigV1{
		Currency: draft.Currency, BillingMultiplier: importedMaterialStringValue(draft.BillingMultiplier),
		PurchaseDiscountRatio: importedMaterialStringValue(draft.PurchaseDiscountRatio),
		RechargeExchangeRatio: importedMaterialStringValue(draft.RechargeExchangeRatio),
		FeeRate:               importedMaterialStringValue(draft.FeeRate), CurrencyToUSDRate: importedMaterialStringValue(draft.CurrencyToUSDRate),
		UnitPrice: draft.UnitPrice, PricePerSecond: draft.PricePerSecond, InputPerMillion: draft.InputPerMillion,
		OutputPerMillion: draft.OutputPerMillion, CompletionPerMillion: draft.CompletionPerMillion,
		TotalPerMillion: draft.TotalPerMillion, ZeroCostReason: draft.ZeroCostReason,
		ChargeEvent: types.CostChargeEvent(draft.ChargeEvent), MeterSource: types.CostMeterSource(draft.MeterSource),
		TokenMode: types.CostTokenMode(draft.TokenMode),
	}
	if config.ChargeEvent == "" {
		switch mode {
		case types.CostModePerRequest:
			config.ChargeEvent = types.CostChargeSubmitAccepted
		default:
			config.ChargeEvent = types.CostChargeTaskSucceeded
		}
	}
	if config.MeterSource == "" && mode == types.CostModePerDuration {
		config.MeterSource = types.CostMeterValidatedRequest
	}
	if config.MeterSource == "" && mode == types.CostModePerToken {
		config.MeterSource = types.CostMeterUpstreamUsage
	}
	if mode == types.CostModePerToken && config.TokenMode == "" {
		switch {
		case config.InputPerMillion != nil || config.OutputPerMillion != nil:
			config.TokenMode = types.CostTokenModeInputOutput
		case config.CompletionPerMillion != nil:
			config.TokenMode = types.CostTokenModeCompletion
		default:
			config.TokenMode = types.CostTokenModeTotal
		}
	}
	normalized, err := service.NormalizeCostRuleConfig(mode, config)
	require.NoError(t, err, draft.RouteTargetRef)
	return mode, normalized
}

func importedMaterialStringValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func importedMaterialSubmitPath(channelType int, lineRef string) string {
	switch channelType {
	case constant.ChannelTypeDimensio:
		return "/v1/videos/generations"
	case constant.ChannelTypeFFLink:
		return "/v1/videos/generations"
	case constant.ChannelTypeLucen, constant.ChannelTypeNewAPIVideo:
		return "/v1/video/generations"
	case constant.ChannelTypeEightYes:
		return "/v1/videos"
	case constant.ChannelTypeOmegaAI:
		return "/v1/media/generate"
	case constant.ChannelTypeSecure:
		if lineRef == "secure-discount" || lineRef == "secure-overseas" {
			return "/api/generate-video"
		}
		return "/v1/videos"
	default:
		return "/v1/videos"
	}
}

type importedMaterialMatrixCostCounts struct {
	Requests int64
	Attempts int64
	Audits   int64
}

func importedMaterialMatrixCostSnapshot(t *testing.T) importedMaterialMatrixCostCounts {
	t.Helper()
	var snapshot importedMaterialMatrixCostCounts
	require.NoError(t, model.DB.Model(&model.CostAccountingRequest{}).Count(&snapshot.Requests).Error)
	require.NoError(t, model.DB.Model(&model.CostAccountingAttempt{}).Count(&snapshot.Attempts).Error)
	require.NoError(t, model.DB.Model(&model.CostAccountingAudit{}).Count(&snapshot.Audits).Error)
	return snapshot
}

func loadImportedMaterialMatrixDocument(t *testing.T) types.ConfigImportDocument {
	t.Helper()
	payload, err := os.ReadFile(filepath.Join("testdata", "channel-config-v1.json"))
	require.NoError(t, err)
	var document types.ConfigImportDocument
	require.NoError(t, common.Unmarshal(payload, &document))
	return document
}

func importedMaterialMatrixRuntimeModel(modelName string) string {
	switch modelName {
	case "seedance-2.0":
		return modelrouting.Seedance20
	case "seedance-2.0-fast":
		return modelrouting.Seedance20Fast
	case "seedance-2.0-mini":
		return modelrouting.Seedance20Mini
	default:
		return ""
	}
}

func importedMaterialMatrixReferenceLimits(t *testing.T, bounds *types.ConfigImportReferenceBounds, label string) modelrouting.ReferenceLimits {
	t.Helper()
	require.NotNil(t, bounds.Images, label)
	require.NotNil(t, bounds.Videos, label)
	require.NotNil(t, bounds.Audios, label)
	return modelrouting.ReferenceLimits{Images: *bounds.Images, Videos: *bounds.Videos, Audios: *bounds.Audios}
}

func importedMaterialMatrixRequestReferences(
	minimums modelrouting.ReferenceLimits,
	limits modelrouting.ReferenceLimits,
	totalMax *int,
	videoAudioMax *int,
) modelrouting.ReferenceLimits {
	result := limits
	shrinkVideoAudio := func(maximum int) {
		for result.Videos+result.Audios > maximum {
			switch {
			case result.Audios > minimums.Audios && result.Audios >= result.Videos:
				result.Audios--
			case result.Videos > minimums.Videos:
				result.Videos--
			case result.Audios > minimums.Audios:
				result.Audios--
			default:
				return
			}
		}
	}
	if videoAudioMax != nil {
		shrinkVideoAudio(*videoAudioMax)
	}
	if totalMax != nil {
		availableForVideoAudio := *totalMax - result.Images
		minimumVideoAudio := minimums.Videos + minimums.Audios
		if availableForVideoAudio < minimumVideoAudio {
			availableForVideoAudio = minimumVideoAudio
		}
		shrinkVideoAudio(availableForVideoAudio)
		for result.Images+result.Videos+result.Audios > *totalMax && result.Images > minimums.Images {
			result.Images--
		}
	}
	return result
}

func importedMaterialMatrixApplyInputMode(
	requestReferences modelrouting.ReferenceLimits,
	limits modelrouting.ReferenceLimits,
	inputModes []modelrouting.InputMode,
) modelrouting.ReferenceLimits {
	if len(inputModes) == 0 {
		return requestReferences
	}
	for _, mode := range inputModes {
		if mode == modelrouting.InputModeOmniReference {
			return requestReferences
		}
	}
	for _, mode := range inputModes {
		if mode == modelrouting.InputModeFirstLastFrames && limits.Images >= 2 {
			return modelrouting.ReferenceLimits{Images: 2}
		}
	}
	for _, mode := range inputModes {
		if mode == modelrouting.InputModeFirstFrame && limits.Images >= 1 {
			return modelrouting.ReferenceLimits{Images: 1}
		}
	}
	return modelrouting.ReferenceLimits{}
}

func importedMaterialMatrixRequestBody(t *testing.T, target importedMaterialMatrixTarget, assetBaseURL string) string {
	t.Helper()
	content := []map[string]any{{"type": "text", "text": "ARK SDK material matrix acceptance " + target.CaseID}}
	requestMode := modelrouting.InputModeOmniReference
	if len(target.InputModes) > 0 {
		requestMode = target.InputModes[0]
		for _, mode := range target.InputModes {
			if mode == modelrouting.InputModeOmniReference {
				requestMode = mode
				break
			}
		}
	}
	for index := 0; index < target.RequestRefs.Images; index++ {
		role := "reference_image"
		if requestMode == modelrouting.InputModeFirstFrame {
			role = "first_frame"
		} else if requestMode == modelrouting.InputModeFirstLastFrames {
			if index == 0 {
				role = "first_frame"
			} else {
				role = "last_frame"
			}
		}
		content = append(content, map[string]any{
			"type": "image_url", "role": role,
			"image_url": map[string]any{"url": fmt.Sprintf("https://cdn.openai.com/ark-matrix/%s/image-%02d.png", target.Provider, index+1)},
		})
	}
	for index := 0; index < target.RequestRefs.Videos; index++ {
		content = append(content, map[string]any{
			"type": "video_url", "role": "reference_video",
			"video_url": map[string]any{"url": fmt.Sprintf("%s/sample.mp4?provider=%s&index=%d", assetBaseURL, target.Provider, index+1)},
		})
	}
	for index := 0; index < target.RequestRefs.Audios; index++ {
		content = append(content, map[string]any{
			"type": "audio_url", "role": "reference_audio",
			"audio_url": map[string]any{"url": fmt.Sprintf("%s/audio.wav?provider=%s&index=%d", assetBaseURL, target.Provider, index+1)},
		})
	}
	payload := map[string]any{
		"model": target.RuntimeModel, "content": content,
		"duration": target.Duration, "ratio": target.AspectRatio,
	}
	if target.ChannelType != constant.ChannelTypeZZone {
		payload["resolution"] = target.Resolution
	}
	body, err := common.Marshal(payload)
	require.NoError(t, err)
	return string(body)
}

func importedMaterialMatrixAssetHandler(upstream http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ark-matrix/sample.mp4":
			data, err := os.ReadFile(filepath.Join("testdata", "sample.mp4"))
			if err != nil {
				http.Error(w, "video fixture unavailable", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "video/mp4")
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
			if r.Method == http.MethodGet {
				_, _ = w.Write(data)
				return
			}
			if r.Method == http.MethodHead {
				return
			}
			w.WriteHeader(http.StatusMethodNotAllowed)
		case "/ark-matrix/audio.wav":
			data := importedMaterialMatrixPCM16WAV(1_000)
			w.Header().Set("Content-Type", "audio/wav")
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
			if r.Method == http.MethodGet {
				_, _ = w.Write(data)
				return
			}
			if r.Method == http.MethodHead {
				return
			}
			w.WriteHeader(http.StatusMethodNotAllowed)
		default:
			upstream.ServeHTTP(w, r)
		}
	})
}

func importedMaterialMatrixPCM16WAV(durationMS int) []byte {
	const sampleRate = 8_000
	const channels = 1
	const bitsPerSample = 16
	sampleCount := sampleRate * durationMS / 1_000
	dataSize := sampleCount * channels * bitsPerSample / 8
	buffer := bytes.NewBuffer(make([]byte, 0, 44+dataSize))
	buffer.WriteString("RIFF")
	_ = binary.Write(buffer, binary.LittleEndian, uint32(36+dataSize))
	buffer.WriteString("WAVEfmt ")
	_ = binary.Write(buffer, binary.LittleEndian, uint32(16))
	_ = binary.Write(buffer, binary.LittleEndian, uint16(1))
	_ = binary.Write(buffer, binary.LittleEndian, uint16(channels))
	_ = binary.Write(buffer, binary.LittleEndian, uint32(sampleRate))
	_ = binary.Write(buffer, binary.LittleEndian, uint32(sampleRate*channels*bitsPerSample/8))
	_ = binary.Write(buffer, binary.LittleEndian, uint16(channels*bitsPerSample/8))
	_ = binary.Write(buffer, binary.LittleEndian, uint16(bitsPerSample))
	buffer.WriteString("data")
	_ = binary.Write(buffer, binary.LittleEndian, uint32(dataSize))
	buffer.Write(make([]byte, dataSize))
	return buffer.Bytes()
}

func httptestNewServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server
}
