package e2e

import (
	"context"
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
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelrouting"
	"github.com/QuantumNous/new-api/pkg/videometa"
	"github.com/QuantumNous/new-api/relay"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/router"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/cost_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const importedMaterialMatrixGroup = "ark-sdk-material-matrix"

type importedMaterialMatrixTarget struct {
	CaseID         string
	RouteTargetRef string
	LineRef        string
	ChannelRef     string
	Provider       string
	CanonicalModel string
	RuntimeModel   string
	UpstreamModel  string
	Resolution     string
	Duration       int
	CostVariantKey string
	Minimums       modelrouting.ReferenceLimits
	References     modelrouting.ReferenceLimits
	ChannelType    int
	ExpectedPath   string
	CostMode       types.CostMode
	CostConfig     types.CostRuleConfigV1
}

type importedMaterialMatrixEnv struct {
	engine       http.Handler
	mock         *capabilityRecordingServer
	channelIDs   map[string]int
	policyIDs    map[string]int
	materialSeen map[string]int
}

func TestSeedanceImportedMaterialMatrixFullFlowE2E(t *testing.T) {
	silenceSeedanceBillingLogs(t)
	targets := loadImportedMaterialMatrixTargets(t)
	env := setupImportedMaterialMatrixE2E(t, targets)
	require.Len(t, targets, 98)
	checked := 0
	accepted := 0
	contractBlocks := 0

	for _, target := range targets {
		t.Run(target.CaseID, func(t *testing.T) {
			checked++
			env.materialSeen[fmt.Sprintf("%d%d%d", target.References.Images, target.References.Videos, target.References.Audios)]++
			service.ClearChannelAffinityCacheAll()
			request := service.RoutingPolicyWriteRequest{
				GroupName: importedMaterialMatrixGroup,
				Model:     target.RuntimeModel,
				Enabled:   true,
				Defaults: modelrouting.Defaults{
					OutputResolution: target.Resolution,
					DurationSeconds:  target.Duration,
					AspectRatio:      "16:9",
				},
				Targets: []service.RouteTargetWriteRequest{{
					ChannelID:      env.channelIDs[target.LineRef],
					Name:           target.UpstreamModel,
					UpstreamModel:  target.UpstreamModel,
					CostVariantKey: target.CostVariantKey,
					TargetPriority: 100,
					Enabled:        true,
					Constraints: modelrouting.Constraints{
						OutputResolutions: []string{target.Resolution},
						Durations: modelrouting.DurationConstraint{
							Min: common.GetPointer(target.Duration),
							Max: common.GetPointer(target.Duration),
						},
						AspectRatios:      []string{"16:9"},
						ReferenceMinimums: target.Minimums,
						ReferenceLimits:   target.References,
					},
				}},
			}
			var channel model.Channel
			require.NoError(t, model.DB.Where("id = ?", env.channelIDs[target.LineRef]).First(&channel).Error, target.CaseID)
			require.Equal(t, target.ChannelType, channel.Type, target.CaseID)
			var costRule model.ChannelModelCostRule
			require.NoError(t, model.DB.Where(
				"channel_id = ? AND billable_upstream_model = ? AND cost_variant_key = ? AND status = ?",
				channel.Id, target.UpstreamModel, target.CostVariantKey, types.CostRuleActive,
			).First(&costRule).Error, target.CaseID)
			require.Equal(t, string(target.CostMode), costRule.CostMode, target.CaseID)
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

			before := seedanceBillingDomainSnapshotFor(t, &seedanceBillingE2EEnv{
				User: &model.User{Id: e2eUserID}, Token: &model.Token{Id: 1},
				Channel: &model.Channel{Id: env.channelIDs[target.LineRef]},
			})
			costBefore := importedMaterialMatrixCostSnapshot(t)
			requestsBefore := env.mock.snapshot()

			view, err := service.SaveRoutingPolicy(env.policyIDs[target.RuntimeModel], request)
			if err != nil {
				var policyErr *service.RoutingPolicyServiceError
				require.True(t, errors.As(err, &policyErr), "%s: %v", target.CaseID, err)
				require.Equal(t, "incompatible_channel_contract", policyErr.Code, target.CaseID)
				require.Equal(t, "targets.0.constraints", policyErr.Field, target.CaseID)
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

			body := importedMaterialMatrixRequestBody(t, target)
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

			summary := service.RunTaskPollingOnce(context.Background(), nil)
			require.Equal(t, 1, summary.UnfinishedTasks, target.CaseID)
			require.NoError(t, model.DB.Where("task_id = ?", created.ID).First(&task).Error, target.CaseID)
			require.Equal(t, string(model.TaskStatusSuccess), string(task.Status), target.CaseID)
			require.NotNil(t, task.PrivateData.BillingContext, target.CaseID)
			require.GreaterOrEqual(t, task.PrivateData.BillingContext.BillingTokens, 0, target.CaseID)

			status, single := performJSONRequest(t, env.engine, http.MethodGet, "/api/v3/contents/generations/tasks/"+created.ID, "Bearer e2e", "")
			require.Equal(t, http.StatusOK, status, "%s: %s", target.CaseID, single)
			require.Contains(t, string(single), target.RuntimeModel, target.CaseID)
			require.NotContains(t, string(single), target.UpstreamModel, target.CaseID)

			after := seedanceBillingDomainSnapshotFor(t, &seedanceBillingE2EEnv{
				User: &model.User{Id: e2eUserID}, Token: &model.Token{Id: 1},
				Channel: &model.Channel{Id: env.channelIDs[target.LineRef]},
			}, created.ID)
			delta := after.delta(before)
			require.Equal(t, int64(1), delta.TaskCount, target.CaseID)
			require.Equal(t, 1, delta.UserRequestCount, target.CaseID)
			expectedLogCount := int64(2)
			expectedRefundLogCount := 1
			if target.CostMode == types.CostModePerRequest ||
				((target.ChannelType == constant.ChannelTypeFourSToken || target.ChannelType == constant.ChannelTypePaipu) && target.CostMode == types.CostModePerDuration) {
				expectedLogCount = 1
				expectedRefundLogCount = 0
			}
			require.Equal(t, expectedLogCount, delta.LogCount, target.CaseID)
			require.Equal(t, 1, delta.ConsumeLogCount, target.CaseID)
			require.Equal(t, expectedRefundLogCount, delta.RefundLogCount, target.CaseID)
			require.Equal(t, task.Quota, delta.UserUsedQuota, target.CaseID)
			require.Equal(t, int64(task.Quota), delta.ChannelUsedQuota, target.CaseID)
			require.Equal(t, task.Quota, delta.TokenUsedQuota, target.CaseID)
			require.Equal(t, 1, delta.QuotaDataCount, target.CaseID)
			require.Equal(t, task.Quota, delta.QuotaDataQuota, target.CaseID)
			require.Equal(t, task.PrivateData.BillingContext.BillingTokens, delta.QuotaDataTokenUsed, target.CaseID)
			if expectedLogCount == 2 {
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
			var attemptConfig types.CostRuleConfigV1
			require.NoError(t, common.UnmarshalJsonStr(costAttempt.RuleConfigJSON, &attemptConfig), target.CaseID)
			require.Equal(t, target.CostConfig.Currency, attemptConfig.Currency, target.CaseID)
			require.Equal(t, target.CostConfig.UnitPrice, attemptConfig.UnitPrice, target.CaseID)
			require.Equal(t, target.CostConfig.PricePerSecond, attemptConfig.PricePerSecond, target.CaseID)
			require.Equal(t, target.CostConfig.NormalizedUSDPrices, attemptConfig.NormalizedUSDPrices, target.CaseID)
		})
	}

	if checked == len(targets) {
		require.Equal(t, 58, accepted)
		require.Equal(t, 40, contractBlocks)
		require.Equal(t, map[string]int{"431": 22, "900": 4, "903": 1, "933": 71}, env.materialSeen)
	}
}

func setupImportedMaterialMatrixE2E(t *testing.T, targets []importedMaterialMatrixTarget) *importedMaterialMatrixEnv {
	t.Helper()
	setupSeedanceE2EDB(t)
	service.SetVideoMetadataClient(materialMatrixVideoMetadataClient{})
	t.Cleanup(func() { service.SetVideoMetadataClient(nil) })
	service.SetReferenceAudioDurationResolver(materialMatrixReferenceAudioResolver{})
	t.Cleanup(func() { service.SetReferenceAudioDurationResolver(nil) })
	require.NoError(t, model.DB.AutoMigrate(
		&model.RoutingPolicy{}, &model.RouteTarget{}, &model.ChannelModelCostRule{},
		&model.CostAccountingRequest{}, &model.CostAccountingAttempt{}, &model.CostAccountingAudit{},
	))
	require.NoError(t, model.InitRoutingPolicyCache())
	t.Cleanup(func() {
		if model.DB != nil {
			require.NoError(t, model.InitRoutingPolicyCache())
		}
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

	originalUsableGroups := setting.UserUsableGroups2JSONString()
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"`+importedMaterialMatrixGroup+`":"ARK SDK material matrix"}`))
	t.Cleanup(func() { require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalUsableGroups)) })

	originalGroupRatios := ratio_setting.GroupRatio2JSONString()
	groupRatios := ratio_setting.GetGroupRatioCopy()
	groupRatios[importedMaterialMatrixGroup] = 1
	encodedGroupRatios, err := common.Marshal(groupRatios)
	require.NoError(t, err)
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(string(encodedGroupRatios)))
	t.Cleanup(func() { require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalGroupRatios)) })

	originalRatios := ratio_setting.GetModelRatioCopy()
	ratios := ratio_setting.GetModelRatioCopy()
	for _, modelName := range modelrouting.CanonicalModels {
		ratios[modelName] = 0.1
	}
	encodedRatios, err := common.Marshal(ratios)
	require.NoError(t, err)
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(string(encodedRatios)))
	t.Cleanup(func() {
		encoded, encodeErr := common.Marshal(originalRatios)
		require.NoError(t, encodeErr)
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(string(encoded)))
	})

	mock := &capabilityRecordingServer{}
	server := httptestNewServer(t, mock)
	document := loadImportedMaterialMatrixDocument(t)
	channelIDs := seedImportedMaterialMatrixChannels(t, server.URL, document)
	seedImportedMaterialMatrixCostRules(t, targets, channelIDs)
	model.InitChannelCache()

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

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	router.SetRelayRouter(engine)
	router.SetVideoRouter(engine)
	return &importedMaterialMatrixEnv{
		engine: engine, mock: mock, channelIDs: channelIDs,
		policyIDs: map[string]int{}, materialSeen: map[string]int{},
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

func TestImportedMaterialChannelTypeNormalizesLegacyEightYesType(t *testing.T) {
	document := loadImportedMaterialMatrixDocument(t)
	require.Equal(t, constant.ChannelTypeEightYes, importedMaterialChannelType(t, document, "channel-8yes"))
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

func seedImportedMaterialMatrixCostRules(t *testing.T, targets []importedMaterialMatrixTarget, channelIDs map[string]int) {
	t.Helper()
	now := time.Now().Unix()
	seen := make(map[string]struct{})
	for _, target := range targets {
		key := fmt.Sprintf("%d\x00%s\x00%s", channelIDs[target.LineRef], target.UpstreamModel, target.CostVariantKey)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		configJSON, err := common.Marshal(target.CostConfig)
		require.NoError(t, err, target.CaseID)
		rule := &model.ChannelModelCostRule{
			ChannelID: channelIDs[target.LineRef], BillableUpstreamModel: target.UpstreamModel,
			CostVariantKey: target.CostVariantKey, Version: 1, Status: string(types.CostRuleActive),
			CostMode: string(target.CostMode), SchemaVersion: 1, ConfigJSON: string(configJSON),
			Source: "e2e_import", CreatedBy: 1, ActivatedBy: 1, EffectiveFrom: &now,
			CreatedAt: now, UpdatedAt: now,
		}
		require.NoError(t, model.DB.Create(rule).Error, target.CaseID)
	}
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
			require.NotEmpty(t, target.OutputResolutions, target.RouteTargetRef)
			duration := 10
			if target.DurationMin != nil {
				duration = *target.DurationMin
			}
			line, ok := lines[target.LineRef]
			require.True(t, ok, target.LineRef)
			draft, ok := costRules[target.RouteTargetRef]
			require.True(t, ok, target.RouteTargetRef)
			costMode, costConfig := importedMaterialCostRuleConfig(t, draft)
			channelType := importedMaterialChannelType(t, document, target.LineRef)
			targets = append(targets, importedMaterialMatrixTarget{
				CaseID:         fmt.Sprintf("%s/%s/%d%d%d", target.RouteTargetRef, target.OutputResolutions[0], references.Images, references.Videos, references.Audios),
				RouteTargetRef: target.RouteTargetRef,
				LineRef:        target.LineRef, ChannelRef: line.ChannelRef, Provider: line.ProviderTypeHint,
				CanonicalModel: blueprint.CanonicalModel, RuntimeModel: runtimeModel, UpstreamModel: target.UpstreamModel,
				Resolution: strings.ToLower(target.OutputResolutions[0]), Duration: duration,
				CostVariantKey: target.CostVariantKey, Minimums: minimums, References: references, ChannelType: channelType,
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
	case constant.ChannelTypeLucen, constant.ChannelTypeNewAPIVideo:
		return "/v1/video/generations"
	case constant.ChannelTypeEightYes:
		return "/v1/videos"
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

type materialMatrixVideoMetadataClient struct{}

func (materialMatrixVideoMetadataClient) Metadata(context.Context, string) (videometa.Metadata, error) {
	return videometa.Metadata{DurationMS: 5000}, nil
}

type materialMatrixReferenceAudioResolver struct{}

func (materialMatrixReferenceAudioResolver) ResolveMS(context.Context, []string) (int64, error) {
	return 5000, nil
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

func importedMaterialMatrixRequestBody(t *testing.T, target importedMaterialMatrixTarget) string {
	t.Helper()
	content := []map[string]any{{"type": "text", "text": "ARK SDK material matrix acceptance " + target.CaseID}}
	for index := 0; index < target.References.Images; index++ {
		content = append(content, map[string]any{
			"type": "image_url", "role": "reference_image",
			"image_url": map[string]any{"url": fmt.Sprintf("https://cdn.openai.com/ark-matrix/%s/image-%02d.png", target.Provider, index+1)},
		})
	}
	for index := 0; index < target.References.Videos; index++ {
		content = append(content, map[string]any{
			"type": "video_url", "role": "reference_video",
			"video_url": map[string]any{"url": fmt.Sprintf("https://cdn.openai.com/ark-matrix/%s/video-%02d.mp4", target.Provider, index+1)},
		})
	}
	for index := 0; index < target.References.Audios; index++ {
		content = append(content, map[string]any{
			"type": "audio_url", "role": "reference_audio",
			"audio_url": map[string]any{"url": fmt.Sprintf("https://cdn.openai.com/ark-matrix/%s/audio-%02d.mp3", target.Provider, index+1)},
		})
	}
	body, err := common.Marshal(map[string]any{
		"model": target.RuntimeModel, "content": content, "resolution": target.Resolution,
		"duration": target.Duration, "ratio": "16:9",
	})
	require.NoError(t, err)
	return string(body)
}

func httptestNewServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server
}
