package e2e

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelrouting"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	groupCapabilityRealDurationGroup      = "真人按秒"
	groupCapabilityBlockedPerRequestGroup = "卡真人按次"
	groupCapabilityZeroCandidateGroup     = "无候选分组"

	groupCapabilityRealPerRequestModel    = "e2e-real-person-per-request"
	groupCapabilityRealPerDurationModel   = "e2e-real-person-per-duration"
	groupCapabilityBlockedPerRequestModel = "e2e-no-real-person-per-request"
	groupCapabilityUnknownModel           = "e2e-unknown-capability-and-cost"
)

type groupCapabilityRoutingProfilesE2EEnv struct {
	engine     http.Handler
	upstreams  map[int]*capabilityRecordingServer
	policy     *service.RoutingPolicyView
	rules      map[string]*model.ChannelModelCostRule
	targetIDs  map[string]int
	targetKeys map[string]string
}

func TestGroupCapabilityRoutingProfilesE2E(t *testing.T) {
	t.Run("real-person per-duration group selects only compatible target", func(t *testing.T) {
		env := setupGroupCapabilityRoutingProfilesE2E(t)
		assignGroupCapabilityE2E(t, groupCapabilityRealDurationGroup)

		status, response := performJSONRequest(t, env.engine, http.MethodPost,
			"/api/v3/contents/generations/tasks", "Bearer e2e",
			capabilityRequestBody(t, modelrouting.Seedance20, "720p", 10, "16:9", modelrouting.ReferenceLimits{}, false))

		require.Equal(t, http.StatusOK, status, string(response))
		assertGroupCapabilityUpstreamCalls(t, env, capabilityChannelB)
		assertGroupCapabilityRequestModel(t, env.upstreams[capabilityChannelB], groupCapabilityRealPerDurationModel)
		assertGroupCapabilityTask(t, groupCapabilityRealDurationGroup, capabilityChannelB,
			groupCapabilityRealPerDurationModel, env.policy.ID, env.targetIDs[groupCapabilityRealPerDurationModel])
	})

	t.Run("non-real-person per-request group selects only compatible target", func(t *testing.T) {
		env := setupGroupCapabilityRoutingProfilesE2E(t)
		assignGroupCapabilityE2E(t, groupCapabilityBlockedPerRequestGroup)

		status, response := performJSONRequest(t, env.engine, http.MethodPost,
			"/api/v3/contents/generations/tasks", "Bearer e2e",
			capabilityRequestBody(t, modelrouting.Seedance20, "720p", 10, "16:9", modelrouting.ReferenceLimits{}, false))

		require.Equal(t, http.StatusOK, status, string(response))
		assertGroupCapabilityUpstreamCalls(t, env, 3)
		assertGroupCapabilityRequestModel(t, env.upstreams[3], groupCapabilityBlockedPerRequestModel)
		assertGroupCapabilityTask(t, groupCapabilityBlockedPerRequestGroup, 3,
			groupCapabilityBlockedPerRequestModel, env.policy.ID, env.targetIDs[groupCapabilityBlockedPerRequestModel])
	})

	t.Run("request real-person requirement conflicts before upstream dispatch", func(t *testing.T) {
		env := setupGroupCapabilityRoutingProfilesE2E(t)
		assignGroupCapabilityE2E(t, groupCapabilityBlockedPerRequestGroup)

		status, response := performJSONRequest(t, env.engine, http.MethodPost,
			"/api/v3/contents/generations/tasks", "Bearer e2e",
			capabilityRequestBody(t, modelrouting.Seedance20, "720p", 10, "16:9", modelrouting.ReferenceLimits{}, true))

		require.Equal(t, http.StatusBadRequest, status, string(response))
		assert.Contains(t, string(response), `"code":"no_compatible_route"`)
		assertGroupCapabilityUpstreamCalls(t, env, 0)
	})

	t.Run("excluding the only compatible target rejects strictly", func(t *testing.T) {
		env := setupGroupCapabilityRoutingProfilesE2E(t)
		profiles := groupCapabilityProfilesE2E()
		blocked := profiles[groupCapabilityBlockedPerRequestGroup]
		blocked.ExcludedTargetKeys = []string{env.targetKeys[groupCapabilityBlockedPerRequestModel]}
		profiles[groupCapabilityBlockedPerRequestGroup] = blocked
		setGroupCapabilityProfilesE2E(t, profiles)
		assignGroupCapabilityE2E(t, groupCapabilityBlockedPerRequestGroup)

		status, response := performJSONRequest(t, env.engine, http.MethodPost,
			"/api/v3/contents/generations/tasks", "Bearer e2e",
			capabilityRequestBody(t, modelrouting.Seedance20, "720p", 10, "16:9", modelrouting.ReferenceLimits{}, false))

		require.Equal(t, http.StatusBadRequest, status, string(response))
		assert.Contains(t, string(response), `"code":"no_compatible_route"`)
		assertGroupCapabilityUpstreamCalls(t, env, 0)
	})

	t.Run("auto skips zero-candidate group and uses a later group", func(t *testing.T) {
		env := setupGroupCapabilityRoutingProfilesE2E(t)
		configureGroupCapabilityAutoE2E(t)

		status, response := performJSONRequest(t, env.engine, http.MethodPost,
			"/api/v3/contents/generations/tasks", "Bearer e2e",
			capabilityRequestBody(t, modelrouting.Seedance20, "720p", 10, "16:9", modelrouting.ReferenceLimits{}, false))

		require.Equal(t, http.StatusOK, status, string(response))
		assertGroupCapabilityUpstreamCalls(t, env, capabilityChannelB)
		assertGroupCapabilityTask(t, groupCapabilityRealDurationGroup, capabilityChannelB,
			groupCapabilityRealPerDurationModel, env.policy.ID, env.targetIDs[groupCapabilityRealPerDurationModel])
	})

	t.Run("task usage and cost records preserve group route target and cost mode", func(t *testing.T) {
		env := setupGroupCapabilityRoutingProfilesE2E(t)
		assignGroupCapabilityE2E(t, groupCapabilityRealDurationGroup)

		status, response := performJSONRequest(t, env.engine, http.MethodPost,
			"/api/v3/contents/generations/tasks", "Bearer e2e",
			capabilityRequestBody(t, modelrouting.Seedance20, "720p", 10, "16:9", modelrouting.ReferenceLimits{}, false))
		require.Equal(t, http.StatusOK, status, string(response))
		var submitted struct {
			ID string `json:"id"`
		}
		require.NoError(t, common.Unmarshal(response, &submitted))
		require.NotEmpty(t, submitted.ID)

		task := pollNewAPIVideoTask(t, submitted.ID)
		assert.Equal(t, groupCapabilityRealDurationGroup, task.Group)
		require.NotNil(t, task.PrivateData.Routing)
		assert.Equal(t, env.policy.ID, task.PrivateData.Routing.PolicyID)
		assert.Equal(t, env.targetIDs[groupCapabilityRealPerDurationModel], task.PrivateData.Routing.TargetID)
		assert.Equal(t, "default", task.PrivateData.Routing.SourceGroup)
		assert.Equal(t, groupCapabilityRealDurationGroup, task.PrivateData.Routing.Facts.GroupName)
		require.NotNil(t, task.PrivateData.BillingContext)
		assert.Equal(t, string(types.CostModePerDuration), task.PrivateData.BillingContext.UpstreamCostMode)

		var costRequest model.CostAccountingRequest
		require.NoError(t, model.DB.Where("task_id = ?", submitted.ID).First(&costRequest).Error)
		assert.Equal(t, groupCapabilityRealDurationGroup, costRequest.UserGroup)
		assert.Equal(t, groupCapabilityRealDurationGroup, costRequest.UsingGroup)
		var attempt model.CostAccountingAttempt
		require.NoError(t, model.DB.Where("cost_request_id = ?", costRequest.ID).First(&attempt).Error)
		assert.Equal(t, capabilityChannelB, attempt.ChannelID)
		assert.Equal(t, groupCapabilityRealPerDurationModel, attempt.PredictedUpstreamModel)
		assert.Equal(t, string(types.CostModePerDuration), attempt.CostMode)
		assert.Equal(t, env.rules[groupCapabilityRealPerDurationModel].ID, attempt.RuleID)

		logs, _, err := model.GetAllLogs(model.LogTypeConsume, 0, 0, modelrouting.Seedance20,
			"", "", 0, 20, 0, "", "", "")
		require.NoError(t, err)
		require.NotEmpty(t, logs)
		assert.Equal(t, groupCapabilityRealDurationGroup, logs[0].Group)
		assert.Equal(t, capabilityChannelB, logs[0].ChannelId)
		var other map[string]any
		require.NoError(t, common.UnmarshalJsonStr(logs[0].Other, &other))
		adminInfo, ok := other["admin_info"].(map[string]any)
		require.True(t, ok)
		routing, ok := adminInfo["routing"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, float64(env.policy.ID), routing["policy_id"])
		assert.Equal(t, float64(env.targetIDs[groupCapabilityRealPerDurationModel]), routing["target_id"])
		assert.Equal(t, groupCapabilityRealPerDurationModel, routing["upstream_model"])
		assert.Equal(t, ratio_setting.GroupRoutingSourceDefault, routing["source_group"])
	})
}

func setupGroupCapabilityRoutingProfilesE2E(t *testing.T) *groupCapabilityRoutingProfilesE2EEnv {
	t.Helper()
	base := setupStrictProfitRoutingE2E(t, func(context.Context, service.RoutingRevenuePreviewInput) (int64, string, error) {
		return 2_000_000_000, "1000", nil
	})

	upstreams := map[int]*capabilityRecordingServer{
		capabilityChannelA: base.channelA,
		capabilityChannelB: base.channelB,
	}
	for _, channelID := range []int{capabilityChannelA, capabilityChannelB} {
		channel, err := model.GetChannelById(channelID, true)
		require.NoError(t, err)
		channel.Group = ratio_setting.GroupRoutingSourceDefault
		channel.Models = modelrouting.Seedance20
		require.NoError(t, channel.Update())
	}

	for channelID := 3; channelID <= 4; channelID++ {
		upstream := &capabilityRecordingServer{}
		server := httptest.NewServer(upstream)
		t.Cleanup(server.Close)
		channel := &model.Channel{
			Id: channelID, Type: constant.ChannelTypeNewAPIVideo, Key: "group-capability-e2e-key",
			Status: common.ChannelStatusEnabled, Name: "group-capability-e2e-" + string(rune('a'+channelID-1)),
			BaseURL: common.GetPointer(server.URL), Models: modelrouting.Seedance20,
			Group: ratio_setting.GroupRoutingSourceDefault, Weight: common.GetPointer[uint](1),
			Priority: common.GetPointer[int64](int64(100 - channelID)), CreatedTime: time.Now().Unix(), OtherSettings: "{}",
		}
		channel.SetOtherSettings(dto.ChannelOtherSettings{DisableTaskPollingSleep: true})
		require.NoError(t, channel.Insert())
		upstreams[channelID] = upstream
	}

	knownConstraints := modelrouting.Constraints{
		OutputResolutions: []string{"720p"}, Durations: rangeDuration(4, 15),
		ReferenceLimits: modelrouting.ReferenceLimits{Images: 4, Videos: 3, Audios: 1},
	}
	realConstraints := knownConstraints
	realConstraints.SupportsRealPerson = common.GetPointer(true)
	blockedConstraints := knownConstraints
	blockedConstraints.SupportsRealPerson = common.GetPointer(false)
	policy, err := service.SaveRoutingPolicy(0, service.RoutingPolicyWriteRequest{
		GroupName: ratio_setting.GroupRoutingSourceDefault, Model: modelrouting.Seedance20, Enabled: true,
		Defaults: modelrouting.Defaults{OutputResolution: "720p", DurationSeconds: 10, AspectRatio: "16:9"},
		Targets: []service.RouteTargetWriteRequest{
			{ChannelID: capabilityChannelA, Name: groupCapabilityRealPerRequestModel, UpstreamModel: groupCapabilityRealPerRequestModel, TargetPriority: 400, Enabled: true, Constraints: realConstraints},
			{ChannelID: capabilityChannelB, Name: groupCapabilityRealPerDurationModel, UpstreamModel: groupCapabilityRealPerDurationModel, TargetPriority: 300, Enabled: true, Constraints: realConstraints},
			{ChannelID: 3, Name: groupCapabilityBlockedPerRequestModel, UpstreamModel: groupCapabilityBlockedPerRequestModel, TargetPriority: 200, Enabled: true, Constraints: blockedConstraints},
			{ChannelID: 4, Name: groupCapabilityUnknownModel, UpstreamModel: groupCapabilityUnknownModel, TargetPriority: 100, Enabled: true, Constraints: knownConstraints},
		},
	})
	require.NoError(t, err)

	unitPrice := "0.10"
	perRequestConfig := types.CostRuleConfigV1{
		Currency: "USD", BillingMultiplier: "1", PurchaseDiscountRatio: "1",
		RechargeExchangeRatio: "1", FeeRate: "0", CurrencyToUSDRate: "1",
		UnitPrice: &unitPrice, ChargeEvent: types.CostChargeSubmitAccepted,
	}
	pricePerSecond := "0.01"
	perDurationConfig := types.CostRuleConfigV1{
		Currency: "USD", BillingMultiplier: "1", PurchaseDiscountRatio: "1",
		RechargeExchangeRatio: "1", FeeRate: "0", CurrencyToUSDRate: "1",
		PricePerSecond: &pricePerSecond, MeterSource: types.CostMeterValidatedRequest,
		ChargeEvent: types.CostChargeSubmitAccepted,
	}
	rules := map[string]*model.ChannelModelCostRule{
		groupCapabilityRealPerRequestModel:    seedProfitRoutingRuleWithVariantE2E(t, capabilityChannelA, groupCapabilityRealPerRequestModel, string(types.DefaultCostVariantKey), types.CostModePerRequest, perRequestConfig),
		groupCapabilityRealPerDurationModel:   seedProfitRoutingRuleWithVariantE2E(t, capabilityChannelB, groupCapabilityRealPerDurationModel, string(types.DefaultCostVariantKey), types.CostModePerDuration, perDurationConfig),
		groupCapabilityBlockedPerRequestModel: seedProfitRoutingRuleWithVariantE2E(t, 3, groupCapabilityBlockedPerRequestModel, string(types.DefaultCostVariantKey), types.CostModePerRequest, perRequestConfig),
	}

	originalRatios := ratio_setting.GroupRatio2JSONString()
	ratios := ratio_setting.GetGroupRatioCopy()
	for _, group := range []string{
		groupCapabilityRealDurationGroup,
		groupCapabilityBlockedPerRequestGroup,
		groupCapabilityZeroCandidateGroup,
	} {
		ratios[group] = 1
	}
	encodedRatios, err := common.Marshal(ratios)
	require.NoError(t, err)
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(string(encodedRatios)))
	t.Cleanup(func() { require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalRatios)) })
	setGroupCapabilityProfilesE2E(t, groupCapabilityProfilesE2E())

	targetIDs := make(map[string]int, len(policy.Targets))
	targetKeys := make(map[string]string, len(policy.Targets))
	for _, target := range policy.Targets {
		targetIDs[target.UpstreamModel] = target.ID
		targetKeys[target.UpstreamModel] = service.GroupRoutingTargetKey(
			ratio_setting.GroupRoutingSourceDefault,
			modelrouting.Seedance20,
			modelrouting.Target{
				ChannelID: target.ChannelID, Name: target.Name, UpstreamModel: target.UpstreamModel,
				CostVariantKey: target.CostVariantKey,
			},
		)
	}
	return &groupCapabilityRoutingProfilesE2EEnv{
		engine: base.engine, upstreams: upstreams, policy: policy, rules: rules, targetIDs: targetIDs, targetKeys: targetKeys,
	}
}

func groupCapabilityProfilesE2E() map[string]ratio_setting.GroupRoutingRequirements {
	return map[string]ratio_setting.GroupRoutingRequirements{
		groupCapabilityRealDurationGroup: {
			Status: ratio_setting.GroupRoutingProfileActive, RoutingSource: ratio_setting.GroupRoutingSourceDefault,
			RealPersonMode: ratio_setting.GroupRealPersonRequired, AllowedCostModes: []types.CostMode{types.CostModePerDuration},
		},
		groupCapabilityBlockedPerRequestGroup: {
			Status: ratio_setting.GroupRoutingProfileActive, RoutingSource: ratio_setting.GroupRoutingSourceDefault,
			RealPersonMode: ratio_setting.GroupRealPersonForbidden, AllowedCostModes: []types.CostMode{types.CostModePerRequest},
		},
		groupCapabilityZeroCandidateGroup: {
			Status: ratio_setting.GroupRoutingProfileActive, RoutingSource: ratio_setting.GroupRoutingSourceDefault,
			RealPersonMode: ratio_setting.GroupRealPersonAny, AllowedCostModes: []types.CostMode{types.CostModePerToken},
		},
	}
}

func setGroupCapabilityProfilesE2E(t *testing.T, profiles map[string]ratio_setting.GroupRoutingRequirements) {
	t.Helper()
	encoded, err := common.Marshal(profiles)
	require.NoError(t, err)
	setGroupRoutingRequirementsE2E(t, string(encoded))
}

func assignGroupCapabilityE2E(t *testing.T, group string) {
	t.Helper()
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", e2eUserID).Update("group", group).Error)
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", 1).Update("group", group).Error)
}

func configureGroupCapabilityAutoE2E(t *testing.T) {
	t.Helper()
	originalAutoGroups := setting.AutoGroups2JsonString()
	originalUsableGroups := setting.UserUsableGroups2JSONString()
	originalSpecialGroups := ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup.ReadAll()
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateAutoGroupsByJsonString(originalAutoGroups))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalUsableGroups))
		specialGroups := ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup
		specialGroups.Clear()
		specialGroups.AddAll(originalSpecialGroups)
	})
	autoGroups, err := common.Marshal([]string{groupCapabilityZeroCandidateGroup, groupCapabilityRealDurationGroup})
	require.NoError(t, err)
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(string(autoGroups)))
	usableGroups := setting.GetUserUsableGroupsCopy()
	usableGroups["auto"] = "auto"
	encodedUsableGroups, err := common.Marshal(usableGroups)
	require.NoError(t, err)
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(string(encodedUsableGroups)))
	ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup.Set(capabilityGroup, map[string]string{
		groupCapabilityZeroCandidateGroup: "zero candidate",
		groupCapabilityRealDurationGroup:  "fallback",
	})
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", 1).Update("group", "auto").Error)
}

func assertGroupCapabilityUpstreamCalls(t *testing.T, env *groupCapabilityRoutingProfilesE2EEnv, selectedChannelID int) {
	t.Helper()
	for channelID, upstream := range env.upstreams {
		submitCount := 0
		for _, request := range upstream.snapshot() {
			if request.Method == http.MethodPost && isVideoSubmitPath(request.Path) {
				submitCount++
			}
		}
		if channelID == selectedChannelID {
			assert.Equal(t, 1, submitCount, "selected channel %d submit count", channelID)
			continue
		}
		assert.Zero(t, submitCount, "unexpected submit to channel %d", channelID)
	}
}

func assertGroupCapabilityTask(t *testing.T, group string, channelID int, upstreamModel string, policyID, targetID int) {
	t.Helper()
	var task model.Task
	require.NoError(t, model.DB.Order("id DESC").First(&task).Error)
	assert.Equal(t, group, task.Group)
	assert.Equal(t, channelID, task.ChannelId)
	require.NotNil(t, task.PrivateData.Routing)
	assert.Equal(t, policyID, task.PrivateData.Routing.PolicyID)
	assert.Equal(t, targetID, task.PrivateData.Routing.TargetID)
	assert.Equal(t, upstreamModel, task.PrivateData.Routing.UpstreamModel)
	assert.Equal(t, ratio_setting.GroupRoutingSourceDefault, task.PrivateData.Routing.SourceGroup)
	assert.Equal(t, group, task.PrivateData.Routing.Facts.GroupName)
	var policy model.RoutingPolicy
	require.NoError(t, model.DB.First(&policy, policyID).Error)
	assert.Equal(t, ratio_setting.GroupRoutingSourceDefault, policy.GroupName)
}

func assertGroupCapabilityRequestModel(t *testing.T, upstream *capabilityRecordingServer, expected string) {
	t.Helper()
	for _, request := range upstream.snapshot() {
		if request.Method != http.MethodPost || !isVideoSubmitPath(request.Path) {
			continue
		}
		var body map[string]any
		require.NoError(t, common.Unmarshal(request.Body, &body))
		modelName, ok := body["model"].(string)
		require.True(t, ok)
		assert.Equal(t, expected, strings.TrimSpace(modelName))
		return
	}
	t.Fatalf("no upstream submit request recorded")
}
