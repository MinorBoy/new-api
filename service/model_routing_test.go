package service_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelrouting"
	"github.com/QuantumNous/new-api/pkg/videometa"
	relaytypes "github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/cost_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSelectCapabilityChannelPublishesTargetDecision(t *testing.T) {
	prepareCapabilitySelectionTest(t)
	seedRoutingCandidate(t, 11, "A1", "分组A", modelrouting.Seedance20, true)
	policy := capabilityPolicyRequest("分组A", modelrouting.Seedance20, 11, "provider-1080p", "1080p")
	policy.Targets[0].CostVariantKey = "720p"
	saved, err := service.SaveRoutingPolicy(0, policy)
	require.NoError(t, err)

	c := capabilitySelectionContext()
	input := seedanceFactsInput(modelrouting.Seedance20, "1080p", 10, "16:9")
	param := &service.RetryParam{
		Ctx: c, TokenGroup: "分组A", ModelName: modelrouting.Seedance20,
		RequestPath: "/v1/video/generations", Retry: common.GetPointer(0), RoutingInput: &input,
	}
	channel, group, err := service.CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, "分组A", group)
	assert.Equal(t, 11, channel.Id)
	assert.True(t, common.GetContextKeyBool(c, constant.ContextKeyRoutingCapabilityMode))
	assert.Equal(t, saved.ID, common.GetContextKeyInt(c, constant.ContextKeyRoutingPolicyID))
	assert.Equal(t, saved.Targets[0].ID, common.GetContextKeyInt(c, constant.ContextKeyRoutingTargetID))
	assert.Equal(t, "provider-1080p", common.GetContextKeyString(c, constant.ContextKeyRoutingUpstreamModel))
	assert.Equal(t, "720p", common.GetContextKeyString(c, constant.ContextKeyRoutingCostVariant))
	facts, ok := common.GetContextKeyType[modelrouting.Facts](c, constant.ContextKeyRoutingFacts)
	require.True(t, ok)
	assert.Equal(t, "分组A", facts.GroupName)
	assert.Equal(t, "1080p", facts.OutputResolution)
}

func TestSelectCapabilityChannelNormalizesMiniClientAlias(t *testing.T) {
	prepareCapabilitySelectionTest(t)
	seedRoutingCandidate(t, 11, "mini", "分组A", modelrouting.Seedance20Mini, true)
	policy := capabilityPolicyRequest("分组A", modelrouting.Seedance20Mini, 11, "provider-mini", "720p")
	saved, err := service.SaveRoutingPolicy(0, policy)
	require.NoError(t, err)

	c := capabilitySelectionContext()
	const legacyMiniAlias = "doubao-seedance-2-0-mini-260128"
	input := seedanceFactsInput(legacyMiniAlias, "720p", 5, "16:9")
	channel, group, err := service.CacheGetRandomSatisfiedChannel(&service.RetryParam{
		Ctx: c, TokenGroup: "分组A", ModelName: legacyMiniAlias,
		RequestPath: "/v1/video/generations", Retry: common.GetPointer(0), RoutingInput: &input,
	})

	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, "分组A", group)
	assert.Equal(t, 11, channel.Id)
	assert.Equal(t, saved.ID, common.GetContextKeyInt(c, constant.ContextKeyRoutingPolicyID))
	facts, ok := common.GetContextKeyType[modelrouting.Facts](c, constant.ContextKeyRoutingFacts)
	require.True(t, ok)
	assert.Equal(t, modelrouting.Seedance20Mini, facts.CanonicalModel)
}

func TestRoutingPolicyNormalizesInputModesAndReferenceMinimums(t *testing.T) {
	prepareCapabilitySelectionTest(t)
	seedRoutingCandidate(t, 11, "A1", "分组A", modelrouting.Seedance20, true)
	request := capabilityPolicyRequest("分组A", modelrouting.Seedance20, 11, "provider-720p", "720p")
	request.Defaults.DurationSeconds = 8
	request.Targets[0].Constraints.InputModes = []modelrouting.InputMode{
		" omni_reference ", "FIRST_FRAME", "omni_reference",
	}
	request.Targets[0].Constraints.ReferenceMinimums.Images = 1

	saved, err := service.SaveRoutingPolicy(0, request)

	require.NoError(t, err)
	require.Len(t, saved.Targets, 1)
	assert.Equal(t, []modelrouting.InputMode{
		modelrouting.InputModeFirstFrame,
		modelrouting.InputModeOmniReference,
	}, saved.Targets[0].Constraints.InputModes)
	assert.Equal(t, modelrouting.ReferenceLimits{Images: 1}, saved.Targets[0].Constraints.ReferenceMinimums)
}

func TestSelectCapabilityChannelPreservesLegacyWithoutPolicy(t *testing.T) {
	prepareCapabilitySelectionTest(t)
	seedRoutingCandidate(t, 11, "A1", "分组A", modelrouting.Seedance20, true)
	c := capabilitySelectionContext()
	input := seedanceFactsInput(modelrouting.Seedance20, "1080p", 10, "16:9")

	channel, group, err := service.CacheGetRandomSatisfiedChannel(&service.RetryParam{
		Ctx: c, TokenGroup: "分组A", ModelName: modelrouting.Seedance20,
		RequestPath: "/v1/video/generations", Retry: common.GetPointer(0), RoutingInput: &input,
	})
	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, "分组A", group)
	assert.Equal(t, 11, channel.Id)
	assert.False(t, common.GetContextKeyBool(c, constant.ContextKeyRoutingCapabilityMode))
	assert.Equal(t, string(types.DefaultCostVariantKey), common.GetContextKeyString(c, constant.ContextKeyRoutingCostVariant))
}

func TestSelectCapabilityChannelAppliesGroupRealPersonRequirement(t *testing.T) {
	prepareCapabilitySelectionTest(t)
	setGroupRoutingRequirementsForTest(t, `{"真人分组":{"require_real_person":true}}`)
	seedRoutingCandidate(t, 11, "supports", "真人分组", modelrouting.Seedance20, true)
	seedRoutingCandidate(t, 12, "generic", "真人分组", modelrouting.Seedance20, true)
	require.NoError(t, model.DB.Model(&model.Ability{}).Where("channel_id = ?", 12).Update("priority", 200).Error)

	policy := capabilityPolicyRequest("真人分组", modelrouting.Seedance20, 11, "provider-face", "720p")
	genericTarget := policy.Targets[0]
	genericTarget.ChannelID = 12
	genericTarget.Name = "provider-generic"
	genericTarget.UpstreamModel = "provider-generic"
	genericTarget.Constraints.SupportsRealPerson = common.GetPointer(false)
	policy.Targets = append(policy.Targets, genericTarget)
	_, err := service.SaveRoutingPolicy(0, policy)
	require.NoError(t, err)

	input := seedanceFactsInput(modelrouting.Seedance20, "720p", 8, "16:9")
	c := capabilitySelectionContext()
	channel, _, err := service.CacheGetRandomSatisfiedChannel(&service.RetryParam{
		Ctx: c, TokenGroup: "真人分组", ModelName: modelrouting.Seedance20,
		RequestPath: "/v1/video/generations", Retry: common.GetPointer(0), RoutingInput: &input,
	})
	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 11, channel.Id)
	facts, ok := common.GetContextKeyType[modelrouting.Facts](c, constant.ContextKeyRoutingFacts)
	require.True(t, ok)
	assert.True(t, facts.RequireRealPerson)
}

func TestRequiredGroupRealPersonFailsClosedWithoutCapabilityPolicy(t *testing.T) {
	prepareCapabilitySelectionTest(t)
	setGroupRoutingRequirementsForTest(t, `{"真人分组":{"require_real_person":true}}`)
	seedRoutingCandidate(t, 11, "legacy", "真人分组", modelrouting.Seedance20, true)

	channel, _, err := service.CacheGetRandomSatisfiedChannel(&service.RetryParam{
		Ctx: capabilitySelectionContext(), TokenGroup: "真人分组", ModelName: modelrouting.Seedance20,
		RequestPath: "/v1/video/generations", Retry: common.GetPointer(0),
	})
	assert.Nil(t, channel)
	var selectionErr *service.ChannelSelectionError
	require.ErrorAs(t, err, &selectionErr)
	assert.Equal(t, relaytypes.ErrorCodeNoCompatibleRoute, selectionErr.Code)
	assert.Equal(t, http.StatusBadRequest, selectionErr.StatusCode)
}

func TestSelectCapabilityChannelUsesDefaultPoolForDynamicGroup(t *testing.T) {
	prepareDynamicGroupRoutingSelectionTest(t)
	seedRoutingCandidate(t, 11, "per-request", "default", modelrouting.Seedance20, true)
	seedRoutingCandidate(t, 12, "per-duration", "default", modelrouting.Seedance20, true)
	require.NoError(t, model.DB.Model(&model.Ability{}).Where("channel_id = ?", 11).Update("priority", 200).Error)
	request := capabilityPolicyRequest("default", modelrouting.Seedance20, 11, "request-model", "720p")
	durationTarget := capabilityPolicyRequest("default", modelrouting.Seedance20, 12, "duration-model", "720p").Targets[0]
	request.Targets = append(request.Targets, durationTarget)
	_, err := service.SaveRoutingPolicy(0, request)
	require.NoError(t, err)
	seedActiveCostModeRuleForRouting(t, 11, "request-model", types.CostModePerRequest)
	seedActiveCostModeRuleForRouting(t, 12, "duration-model", types.CostModePerDuration)
	setGroupRoutingRequirementsForTest(t, `{
		"客户A":{"status":"active","routing_source":"default","real_person_mode":"required","allowed_cost_modes":["per_duration"]}
	}`)

	input := seedanceFactsInput(modelrouting.Seedance20, "720p", 10, "16:9")
	channel, group, err := service.CacheGetRandomSatisfiedChannel(&service.RetryParam{
		Ctx: capabilitySelectionContext(), TokenGroup: "客户A", ModelName: modelrouting.Seedance20,
		RequestPath: "/v1/video/generations", Retry: common.GetPointer(0), RoutingInput: &input,
	})

	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 12, channel.Id)
	assert.Equal(t, "客户A", group)
}

func TestDynamicGroupDraftFailsClosed(t *testing.T) {
	prepareDynamicGroupRoutingSelectionTest(t)
	seedRoutingCandidate(t, 11, "default", "default", modelrouting.Seedance20, true)
	_, err := service.SaveRoutingPolicy(0, capabilityPolicyRequest("default", modelrouting.Seedance20, 11, "provider-default", "720p"))
	require.NoError(t, err)
	setGroupRoutingRequirementsForTest(t, `{"客户A":{"status":"draft","routing_source":"default"}}`)
	input := seedanceFactsInput(modelrouting.Seedance20, "720p", 10, "16:9")

	channel, _, err := service.CacheGetRandomSatisfiedChannel(&service.RetryParam{
		Ctx: capabilitySelectionContext(), TokenGroup: "客户A", ModelName: modelrouting.Seedance20,
		RequestPath: "/v1/video/generations", Retry: common.GetPointer(0), RoutingInput: &input,
	})

	assert.Nil(t, channel)
	var selectionErr *service.ChannelSelectionError
	require.ErrorAs(t, err, &selectionErr)
	assert.Equal(t, relaytypes.ErrorCodeNoCompatibleRoute, selectionErr.Code)
}

func TestForbiddenRealPersonGroupRejectsRequiredRequest(t *testing.T) {
	prepareDynamicGroupRoutingSelectionTest(t)
	seedRoutingCandidate(t, 11, "default", "default", modelrouting.Seedance20, true)
	request := capabilityPolicyRequest("default", modelrouting.Seedance20, 11, "provider-default", "720p")
	request.Targets[0].Constraints.SupportsRealPerson = common.GetPointer(false)
	_, err := service.SaveRoutingPolicy(0, request)
	require.NoError(t, err)
	setGroupRoutingRequirementsForTest(t, `{
		"卡真人":{"status":"active","routing_source":"default","real_person_mode":"forbidden"}
	}`)
	input := seedanceFactsInput(modelrouting.Seedance20, "720p", 10, "16:9")
	input.RequireRealPerson = true

	channel, _, err := service.CacheGetRandomSatisfiedChannel(&service.RetryParam{
		Ctx: capabilitySelectionContext(), TokenGroup: "卡真人", ModelName: modelrouting.Seedance20,
		RequestPath: "/v1/video/generations", Retry: common.GetPointer(0), RoutingInput: &input,
	})

	assert.Nil(t, channel)
	var selectionErr *service.ChannelSelectionError
	require.ErrorAs(t, err, &selectionErr)
	assert.Equal(t, relaytypes.ErrorCodeNoCompatibleRoute, selectionErr.Code)
}

func TestDynamicGroupKnownChannelCannotBypassManualExclusion(t *testing.T) {
	prepareDynamicGroupRoutingSelectionTest(t)
	seedRoutingCandidate(t, 11, "default", "default", modelrouting.Seedance20, true)
	saved, err := service.SaveRoutingPolicy(0, capabilityPolicyRequest("default", modelrouting.Seedance20, 11, "provider-default", "720p"))
	require.NoError(t, err)
	require.Len(t, saved.Targets, 1)
	targetKey := service.GroupRoutingTargetKey("default", modelrouting.Seedance20, modelrouting.Target{
		ID: saved.Targets[0].ID, PolicyID: saved.ID, ChannelID: saved.Targets[0].ChannelID,
		Name: saved.Targets[0].Name, UpstreamModel: saved.Targets[0].UpstreamModel,
		CostVariantKey: saved.Targets[0].CostVariantKey,
	})
	setGroupRoutingRequirementsForTest(t, fmt.Sprintf(`{
		"客户A":{"status":"active","routing_source":"default","excluded_target_keys":[%q]}
	}`, targetKey))
	input := seedanceFactsInput(modelrouting.Seedance20, "720p", 10, "16:9")

	compatible, err := service.ValidateKnownChannelForRouting(&service.RetryParam{
		Ctx: capabilitySelectionContext(), TokenGroup: "客户A", ModelName: modelrouting.Seedance20,
		RequestPath: "/v1/video/generations", Retry: common.GetPointer(0), RoutingInput: &input,
	}, "客户A", 11)

	assert.False(t, compatible)
	var selectionErr *service.ChannelSelectionError
	require.ErrorAs(t, err, &selectionErr)
	assert.Equal(t, relaytypes.ErrorCodeNoCompatibleRoute, selectionErr.Code)
	require.Len(t, selectionErr.Diagnostics, 1)
	assert.Equal(t, "default", selectionErr.Diagnostics[0].SourceGroup)
	assert.Equal(t, 1, selectionErr.Diagnostics[0].ProfileMismatchCounts["excluded"])
}

func TestAutoGroupSkipsDynamicProfileWithoutCompatibleTargets(t *testing.T) {
	prepareDynamicGroupRoutingSelectionTest(t)
	prepareAutoGroupSelectionTest(t)
	seedRoutingCandidate(t, 11, "default", "default", modelrouting.Seedance20, true)
	request := capabilityPolicyRequest("default", modelrouting.Seedance20, 11, "provider-default", "720p")
	request.Targets[0].Constraints.SupportsRealPerson = common.GetPointer(false)
	_, err := service.SaveRoutingPolicy(0, request)
	require.NoError(t, err)
	setGroupRoutingRequirementsForTest(t, `{
		"分组A":{"status":"active","routing_source":"default","real_person_mode":"required"},
		"分组B":{"status":"active","routing_source":"default","real_person_mode":"forbidden"}
	}`)
	input := seedanceFactsInput(modelrouting.Seedance20, "720p", 10, "16:9")
	c := capabilitySelectionContext()
	common.SetContextKey(c, constant.ContextKeyUserGroup, "分组A")

	channel, group, err := service.CacheGetRandomSatisfiedChannel(&service.RetryParam{
		Ctx: c, TokenGroup: "auto", ModelName: modelrouting.Seedance20,
		RequestPath: "/v1/video/generations", Retry: common.GetPointer(0), RoutingInput: &input,
	})

	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 11, channel.Id)
	assert.Equal(t, "分组B", group)
}

func TestDynamicGroupRetryKeepsUsingSourceGroupCapabilities(t *testing.T) {
	prepareDynamicGroupRoutingSelectionTest(t)
	seedRoutingCandidate(t, 11, "first", "default", modelrouting.Seedance20, true)
	seedRoutingCandidate(t, 12, "retry", "default", modelrouting.Seedance20, true)
	require.NoError(t, model.DB.Model(&model.Ability{}).Where("channel_id = ?", 11).Update("priority", 200).Error)
	request := capabilityPolicyRequest("default", modelrouting.Seedance20, 11, "provider-first", "720p")
	request.Targets = append(request.Targets, capabilityPolicyRequest(
		"default", modelrouting.Seedance20, 12, "provider-retry", "720p",
	).Targets[0])
	_, err := service.SaveRoutingPolicy(0, request)
	require.NoError(t, err)
	setGroupRoutingRequirementsForTest(t, `{"客户A":{"status":"active","routing_source":"default"}}`)
	input := seedanceFactsInput(modelrouting.Seedance20, "720p", 10, "16:9")
	param := &service.RetryParam{
		Ctx: capabilitySelectionContext(), TokenGroup: "客户A", ModelName: modelrouting.Seedance20,
		RequestPath: "/v1/video/generations", Retry: common.GetPointer(0), RoutingInput: &input,
	}

	channel, _, err := service.CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 11, channel.Id)

	param.ExcludeChannel(11)
	channel, _, err = service.CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 12, channel.Id)
}

func TestStrictDynamicGroupReusesProfileCostRulesForProfitFilter(t *testing.T) {
	prepareStrictCostRoutingServiceTest(t)
	seedRoutingCandidate(t, 11, "default", "default", modelrouting.Seedance20, true)
	_, err := service.SaveRoutingPolicy(0, capabilityPolicyRequest("default", modelrouting.Seedance20, 11, "provider-default", "720p"))
	require.NoError(t, err)
	seedActiveFreeCostRuleForRouting(t, 11, "provider-default")
	setGroupRoutingRequirementsForTest(t, `{
		"客户A":{"status":"active","routing_source":"default","allowed_cost_modes":["free"]}
	}`)

	costRuleQueryCount := 0
	const callbackName = "test:dynamic-profile-cost-rule-query-count"
	callbackRegistered := true
	require.NoError(t, model.DB.Callback().Query().Before("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table == "channel_model_cost_rules" {
			costRuleQueryCount++
		}
	}))
	t.Cleanup(func() {
		if callbackRegistered {
			_ = model.DB.Callback().Query().Remove(callbackName)
		}
	})
	input := seedanceFactsInput(modelrouting.Seedance20, "720p", 10, "16:9")

	channel, _, err := service.CacheGetRandomSatisfiedChannel(&service.RetryParam{
		Ctx: capabilitySelectionContext(), TokenGroup: "客户A", ModelName: modelrouting.Seedance20,
		RequestPath: "/v1/video/generations", Retry: common.GetPointer(0), RoutingInput: &input,
	})

	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 1, costRuleQueryCount)
}

func TestAutoGroupRoutingAppliesRequirementPerActualGroup(t *testing.T) {
	prepareCapabilitySelectionTest(t)
	prepareAutoGroupSelectionTest(t)
	setGroupRoutingRequirementsForTest(t, `{"分组A":{"require_real_person":true}}`)
	seedRoutingCandidate(t, 11, "generic", "分组A", modelrouting.Seedance20, true)
	seedRoutingCandidate(t, 12, "supports", "分组B", modelrouting.Seedance20, true)

	firstPolicy := capabilityPolicyRequest("分组A", modelrouting.Seedance20, 11, "provider-generic", "720p")
	firstPolicy.Targets[0].Constraints.SupportsRealPerson = common.GetPointer(false)
	_, err := service.SaveRoutingPolicy(0, firstPolicy)
	require.NoError(t, err)
	_, err = service.SaveRoutingPolicy(0, capabilityPolicyRequest("分组B", modelrouting.Seedance20, 12, "provider-face", "720p"))
	require.NoError(t, err)

	c := capabilitySelectionContext()
	common.SetContextKey(c, constant.ContextKeyUserGroup, "分组A")
	input := seedanceFactsInput(modelrouting.Seedance20, "720p", 8, "16:9")
	channel, group, err := service.CacheGetRandomSatisfiedChannel(&service.RetryParam{
		Ctx: c, TokenGroup: "auto", ModelName: modelrouting.Seedance20,
		RequestPath: "/v1/video/generations", Retry: common.GetPointer(0), RoutingInput: &input,
	})
	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 12, channel.Id)
	assert.Equal(t, "分组B", group)
	facts, ok := common.GetContextKeyType[modelrouting.Facts](c, constant.ContextKeyRoutingFacts)
	require.True(t, ok)
	assert.Equal(t, "分组B", facts.GroupName)
	assert.False(t, facts.RequireRealPerson)
}

func TestCostRoutingExcludesChannelWithoutCapabilityPolicy(t *testing.T) {
	prepareCapabilitySelectionTest(t)
	seedRoutingCandidate(t, 11, "higher", "分组A", modelrouting.Seedance20, true)
	seedRoutingCandidate(t, 12, "covered", "分组A", modelrouting.Seedance20, true)
	require.NoError(t, model.DB.Model(&model.Ability{}).Where("channel_id = ?", 11).Update("priority", 200).Error)
	require.NoError(t, model.DB.Model(&model.Ability{}).Where("channel_id = ?", 12).Update("priority", 100).Error)

	param := &service.RetryParam{
		Ctx: capabilitySelectionContext(), TokenGroup: "分组A", ModelName: modelrouting.Seedance20,
		RequestPath: "/v1/video/generations", Retry: common.GetPointer(0),
	}
	param.ExcludeChannel(11)

	channel, _, err := service.CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 12, channel.Id)
}

func TestCostRoutingExcludedKnownChannelWithoutCapabilityPolicy(t *testing.T) {
	prepareCapabilitySelectionTest(t)
	seedRoutingCandidate(t, 11, "known", "分组A", modelrouting.Seedance20, true)
	param := &service.RetryParam{
		Ctx: capabilitySelectionContext(), TokenGroup: "分组A", ModelName: modelrouting.Seedance20,
		RequestPath: "/v1/video/generations", Retry: common.GetPointer(0),
	}
	param.ExcludeChannel(11)

	compatible, err := service.ValidateKnownChannelForRouting(param, "分组A", 11)
	assert.False(t, compatible)
	require.Error(t, err)
}

func TestCostRoutingKnownChannelRequiresStrictCoverage(t *testing.T) {
	prepareStrictCostRoutingServiceTest(t)
	seedRoutingCandidate(t, 11, "known", "分组A", modelrouting.Seedance20, true)
	param := &service.RetryParam{
		Ctx: capabilitySelectionContext(), TokenGroup: "分组A", ModelName: modelrouting.Seedance20,
		RequestPath: "/v1/video/generations", Retry: common.GetPointer(0),
	}

	compatible, err := service.ValidateKnownChannelForRouting(param, "分组A", 11)
	assert.False(t, compatible)
	var selectionErr *service.ChannelSelectionError
	require.ErrorAs(t, err, &selectionErr)
	assert.Equal(t, relaytypes.ErrorCodeCompatibleChannelUnavailable, selectionErr.Code)
}

func TestCostRoutingKnownCapabilityUsesTargetModelForCoverage(t *testing.T) {
	prepareStrictCostRoutingServiceTest(t)
	seedRoutingCandidate(t, 11, "known", "分组A", modelrouting.Seedance20, true)
	mapping := `{"` + modelrouting.Seedance20 + `":"wrong-mapped-model"}`
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", 11).Update("model_mapping", mapping).Error)
	_, err := service.SaveRoutingPolicy(0, capabilityPolicyRequest("分组A", modelrouting.Seedance20, 11, "provider-target", "1080p"))
	require.NoError(t, err)
	seedActiveFreeCostRuleForRouting(t, 11, "provider-target")

	input := seedanceFactsInput(modelrouting.Seedance20, "1080p", 10, "16:9")
	param := &service.RetryParam{
		Ctx: capabilitySelectionContext(), TokenGroup: "分组A", ModelName: modelrouting.Seedance20,
		RequestPath: "/v1/video/generations", Retry: common.GetPointer(0), RoutingInput: &input,
	}
	compatible, err := service.ValidateKnownChannelForRouting(param, "分组A", 11)
	require.NoError(t, err)
	assert.True(t, compatible)
	assert.Equal(t, "provider-target", common.GetContextKeyString(param.Ctx, constant.ContextKeyRoutingUpstreamModel))
}

func TestCostRoutingKnownCapabilityDoesNotFallbackToDefaultCostVariant(t *testing.T) {
	prepareStrictCostRoutingServiceTest(t)
	seedRoutingCandidate(t, 11, "known", "分组A", modelrouting.Seedance20, true)
	policy := capabilityPolicyRequest("分组A", modelrouting.Seedance20, 11, "provider-target", "1080p")
	policy.Targets[0].CostVariantKey = "720p"
	_, err := service.SaveRoutingPolicy(0, policy)
	require.NoError(t, err)
	seedActiveFreeCostRuleForRouting(t, 11, "provider-target")

	input := seedanceFactsInput(modelrouting.Seedance20, "1080p", 10, "16:9")
	param := &service.RetryParam{
		Ctx: capabilitySelectionContext(), TokenGroup: "分组A", ModelName: modelrouting.Seedance20,
		RequestPath: "/v1/video/generations", Retry: common.GetPointer(0), RoutingInput: &input,
	}
	compatible, err := service.ValidateKnownChannelForRouting(param, "分组A", 11)

	assert.False(t, compatible)
	var selectionErr *service.ChannelSelectionError
	require.ErrorAs(t, err, &selectionErr)
	assert.Equal(t, relaytypes.ErrorCodeCompatibleChannelUnavailable, selectionErr.Code)
	assert.Equal(t, "720p", common.GetContextKeyString(param.Ctx, constant.ContextKeyRoutingCostVariant))
}

func TestCostRoutingRechecksExcludedMissesAuthoritatively(t *testing.T) {
	prepareStrictCostRoutingServiceTest(t)
	seedRoutingCandidate(t, 11, "candidate", "分组A", modelrouting.Seedance20, true)
	mapping := `{"` + modelrouting.Seedance20 + `":"provider-late"}`
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", 11).Update("model_mapping", mapping).Error)
	channel, err := model.GetChannelById(11, false)
	require.NoError(t, err)
	param := &service.RetryParam{
		Ctx: capabilitySelectionContext(), TokenGroup: "分组A", ModelName: modelrouting.Seedance20,
		RequestPath: "/v1/video/generations", Retry: common.GetPointer(0),
	}

	covered, err := service.CheckSelectedChannelCostCoverage(param, channel, "")
	require.NoError(t, err)
	assert.False(t, covered)
	param.ExcludeChannel(channel.Id)
	seedActiveFreeCostRuleForRouting(t, 11, "provider-late")

	restored, err := service.RecheckCostCoverageMisses(param)
	require.NoError(t, err)
	assert.True(t, restored)
	selected, _, err := service.CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, selected)
	assert.Equal(t, 11, selected.Id)
}

// TestProfitRoutingExcludesBelowMarginCapabilityCandidate asserts the candidate-stage
// profit filter narrows the capability candidate set by predicted margin. Two channels
// both satisfy the capability policy; the one seeded with a per-request cost above the
// (fixed) preview revenue is excluded as margin_below_threshold, so only the free-cost
// channel can be selected.
func TestProfitRoutingExcludesBelowMarginCapabilityCandidate(t *testing.T) {
	prepareStrictCostRoutingServiceTest(t)
	seedRoutingCandidate(t, 11, "cheap", "分组A", modelrouting.Seedance20, true)
	seedRoutingCandidate(t, 12, "expensive", "分组A", modelrouting.Seedance20, true)
	_, err := service.SaveRoutingPolicy(0, multiTargetPolicyRequest("分组A", modelrouting.Seedance20, "1080p",
		[]policyTarget{{ChannelID: 11, UpstreamModel: "cheap-model"}, {ChannelID: 12, UpstreamModel: "expensive-model"}}))
	require.NoError(t, err)
	seedActiveFreeCostRuleForRouting(t, 11, "cheap-model")
	seedPerRequestCostRuleForRouting(t, 12, "expensive-model", "100") // $100/req >> ~$2 preview revenue

	input := seedanceFactsInput(modelrouting.Seedance20, "1080p", 10, "16:9")
	param := &service.RetryParam{
		Ctx: capabilitySelectionContext(), TokenGroup: "分组A", ModelName: modelrouting.Seedance20,
		RequestPath: "/v1/video/generations", Retry: common.GetPointer(0), RoutingInput: &input,
	}
	// The expensive channel must never be selected across several attempts because the
	// profit filter excludes it before the weighted random pick runs.
	for attempt := 0; attempt < 10; attempt++ {
		param.SetRetry(0)
		selected, _, selectErr := service.CacheGetRandomSatisfiedChannel(param)
		require.NoError(t, selectErr)
		require.NotNil(t, selected, "free-cost candidate must remain selectable")
		assert.Equal(t, 11, selected.Id, "expensive candidate must be excluded by the margin gate")
	}
}

func TestProfitRoutingRejectsInvalidInputVideo(t *testing.T) {
	prepareStrictCostRoutingServiceTest(t)
	seedRoutingCandidate(t, 11, "token-priced", "分组A", modelrouting.Seedance20, true)
	_, err := service.SaveRoutingPolicy(0, capabilityPolicyRequest("分组A", modelrouting.Seedance20, 11, "token-model", "1080p"))
	require.NoError(t, err)
	seedTokenCostRuleForRouting(t, 11, "token-model")

	service.SetVideoMetadataClient(invalidVideoMetadataClient{})
	t.Cleanup(func() { service.SetVideoMetadataClient(nil) })
	input := seedanceFactsInput(modelrouting.Seedance20, "1080p", 10, "16:9")
	input.ReferenceVideoURLs = []string{"https://assets.example/invalid.mp4?signature=secret"}

	channel, _, err := service.CacheGetRandomSatisfiedChannel(&service.RetryParam{
		Ctx: capabilitySelectionContext(), TokenGroup: "分组A", ModelName: modelrouting.Seedance20,
		RequestPath: "/v1/video/generations", Retry: common.GetPointer(0), RoutingInput: &input,
	})

	assert.Nil(t, channel)
	var selectionErr *service.ChannelSelectionError
	require.ErrorAs(t, err, &selectionErr)
	assert.Equal(t, relaytypes.ErrorCodeInvalidRequest, selectionErr.Code)
	assert.Equal(t, http.StatusBadRequest, selectionErr.StatusCode)
	assert.Equal(t, "input video is not supported", selectionErr.Err.Error())
	assert.NotContains(t, selectionErr.Err.Error(), "assets.example")
	assert.NotContains(t, selectionErr.Err.Error(), "secret")
}

func TestProfitRoutingRejectsInvalidInputVideoForEveryCostMode(t *testing.T) {
	tests := []struct {
		name     string
		seedRule func(t *testing.T, channelID int, modelName string)
	}{
		{
			name:     "free",
			seedRule: seedActiveFreeCostRuleForRouting,
		},
		{
			name: "per request",
			seedRule: func(t *testing.T, channelID int, modelName string) {
				seedPerRequestCostRuleForRouting(t, channelID, modelName, "1")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prepareStrictCostRoutingServiceTest(t)
			seedRoutingCandidate(t, 11, "non-token", "分组A", modelrouting.Seedance20, true)
			_, err := service.SaveRoutingPolicy(0, capabilityPolicyRequest("分组A", modelrouting.Seedance20, 11, "non-token-model", "1080p"))
			require.NoError(t, err)
			tt.seedRule(t, 11, "non-token-model")

			service.SetVideoMetadataClient(invalidVideoMetadataClient{})
			t.Cleanup(func() { service.SetVideoMetadataClient(nil) })
			input := seedanceFactsInput(modelrouting.Seedance20, "1080p", 10, "16:9")
			input.ReferenceVideoURLs = []string{"https://assets.example/invalid.mp4?signature=secret"}

			channel, _, err := service.CacheGetRandomSatisfiedChannel(&service.RetryParam{
				Ctx: capabilitySelectionContext(), TokenGroup: "分组A", ModelName: modelrouting.Seedance20,
				RequestPath: "/v1/video/generations", Retry: common.GetPointer(0), RoutingInput: &input,
			})

			assert.Nil(t, channel)
			var selectionErr *service.ChannelSelectionError
			require.ErrorAs(t, err, &selectionErr)
			assert.Equal(t, relaytypes.ErrorCodeInvalidRequest, selectionErr.Code)
			assert.Equal(t, http.StatusBadRequest, selectionErr.StatusCode)
			assert.Equal(t, "input video is not supported", selectionErr.Err.Error())
			assert.NotContains(t, selectionErr.Err.Error(), "assets.example")
			assert.NotContains(t, selectionErr.Err.Error(), "secret")
		})
	}
}

func TestProfitRoutingKeepsNonTokenCandidateWhenMetadataIsUnavailable(t *testing.T) {
	prepareStrictCostRoutingServiceTest(t)
	seedRoutingCandidate(t, 11, "token", "分组A", modelrouting.Seedance20, true)
	seedRoutingCandidate(t, 12, "request", "分组A", modelrouting.Seedance20, true)
	_, err := service.SaveRoutingPolicy(0, multiTargetPolicyRequest("分组A", modelrouting.Seedance20, "1080p",
		[]policyTarget{{ChannelID: 11, UpstreamModel: "token-model"}, {ChannelID: 12, UpstreamModel: "request-model"}}))
	require.NoError(t, err)
	seedTokenCostRuleForRouting(t, 11, "token-model")
	seedPerRequestCostRuleForRouting(t, 12, "request-model", "1")

	service.SetVideoMetadataClient(unavailableVideoMetadataClient{})
	t.Cleanup(func() { service.SetVideoMetadataClient(nil) })
	input := seedanceFactsInput(modelrouting.Seedance20, "1080p", 10, "16:9")
	input.ReferenceVideoURLs = []string{"https://assets.example/unavailable.mp4?signature=secret"}

	c := capabilitySelectionContext()
	channel, _, err := service.CacheGetRandomSatisfiedChannel(&service.RetryParam{
		Ctx: c, TokenGroup: "分组A", ModelName: modelrouting.Seedance20,
		RequestPath: "/v1/video/generations", Retry: common.GetPointer(0), RoutingInput: &input,
	})

	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 12, channel.Id)
	diagnostics, ok := common.GetContextKeyType[[]service.ProfitRoutingDiagnostic](c, constant.ContextKeyRoutingDiagnostics)
	require.True(t, ok)
	require.Len(t, diagnostics, 1)
	assert.Equal(t, 11, diagnostics[0].ChannelID)
	assert.Equal(t, service.ProfitReasonMetadataUnavailable, diagnostics[0].Reason)
}

func TestProfitRoutingKeepsOnlyNonTokenCandidateWhenMetadataIsUnavailable(t *testing.T) {
	prepareStrictCostRoutingServiceTest(t)
	seedRoutingCandidate(t, 11, "request", "分组A", modelrouting.Seedance20, true)
	_, err := service.SaveRoutingPolicy(0, capabilityPolicyRequest("分组A", modelrouting.Seedance20, 11, "request-model", "1080p"))
	require.NoError(t, err)
	seedPerRequestCostRuleForRouting(t, 11, "request-model", "1")

	service.SetVideoMetadataClient(unavailableVideoMetadataClient{})
	t.Cleanup(func() { service.SetVideoMetadataClient(nil) })
	input := seedanceFactsInput(modelrouting.Seedance20, "1080p", 10, "16:9")
	input.ReferenceVideoURLs = []string{"https://assets.example/unavailable.mp4?signature=secret"}

	channel, _, err := service.CacheGetRandomSatisfiedChannel(&service.RetryParam{
		Ctx: capabilitySelectionContext(), TokenGroup: "分组A", ModelName: modelrouting.Seedance20,
		RequestPath: "/v1/video/generations", Retry: common.GetPointer(0), RoutingInput: &input,
	})

	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 11, channel.Id)
}

func TestProfitRoutingReturns503WhenEveryCandidateIsBelowMargin(t *testing.T) {
	prepareStrictCostRoutingServiceTest(t)
	seedRoutingCandidate(t, 11, "expensive", "分组A", modelrouting.Seedance20, true)
	_, err := service.SaveRoutingPolicy(0, capabilityPolicyRequest("分组A", modelrouting.Seedance20, 11, "expensive-model", "1080p"))
	require.NoError(t, err)
	seedPerRequestCostRuleForRouting(t, 11, "expensive-model", "100")

	c := capabilitySelectionContext()
	input := seedanceFactsInput(modelrouting.Seedance20, "1080p", 10, "16:9")
	channel, _, err := service.CacheGetRandomSatisfiedChannel(&service.RetryParam{
		Ctx: c, TokenGroup: "分组A", ModelName: modelrouting.Seedance20,
		RequestPath: "/v1/video/generations", Retry: common.GetPointer(0), RoutingInput: &input,
	})

	assert.Nil(t, channel)
	var selectionErr *service.ChannelSelectionError
	require.ErrorAs(t, err, &selectionErr)
	assert.Equal(t, relaytypes.ErrorCodeCompatibleChannelUnavailable, selectionErr.Code)
	assert.Equal(t, http.StatusServiceUnavailable, selectionErr.StatusCode)
	assert.Equal(t, "compatible channels are unavailable", selectionErr.Err.Error())
	assert.NotContains(t, selectionErr.Err.Error(), "100")

	diagnostics, ok := common.GetContextKeyType[[]service.ProfitRoutingDiagnostic](c, constant.ContextKeyRoutingDiagnostics)
	require.True(t, ok)
	require.Len(t, diagnostics, 1)
	assert.Equal(t, service.ProfitReasonMarginBelowThreshold, diagnostics[0].Reason)
}

func TestProfitRoutingAutoGroupSkipsBelowMarginCandidates(t *testing.T) {
	prepareStrictCostRoutingServiceTest(t)
	prepareAutoGroupSelectionTest(t)
	seedRoutingCandidate(t, 11, "expensive", "分组A", modelrouting.Seedance20, true)
	seedRoutingCandidate(t, 12, "free", "分组B", modelrouting.Seedance20, true)
	_, err := service.SaveRoutingPolicy(0, capabilityPolicyRequest("分组A", modelrouting.Seedance20, 11, "expensive-model", "1080p"))
	require.NoError(t, err)
	_, err = service.SaveRoutingPolicy(0, capabilityPolicyRequest("分组B", modelrouting.Seedance20, 12, "free-model", "1080p"))
	require.NoError(t, err)
	seedPerRequestCostRuleForRouting(t, 11, "expensive-model", "100")
	seedActiveFreeCostRuleForRouting(t, 12, "free-model")

	c := capabilitySelectionContext()
	common.SetContextKey(c, constant.ContextKeyUserGroup, "分组A")
	input := seedanceFactsInput(modelrouting.Seedance20, "1080p", 10, "16:9")
	channel, group, err := service.CacheGetRandomSatisfiedChannel(&service.RetryParam{
		Ctx: c, TokenGroup: "auto", ModelName: modelrouting.Seedance20,
		RequestPath: "/v1/video/generations", Retry: common.GetPointer(0), RoutingInput: &input,
	})

	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 12, channel.Id)
	assert.Equal(t, "分组B", group)
}

func TestProfitRoutingKnownChannelRejectsBelowMarginForSpecificAndAffinitySelection(t *testing.T) {
	prepareStrictCostRoutingServiceTest(t)
	seedRoutingCandidate(t, 11, "expensive", "分组A", modelrouting.Seedance20, true)
	_, err := service.SaveRoutingPolicy(0, capabilityPolicyRequest("分组A", modelrouting.Seedance20, 11, "expensive-model", "1080p"))
	require.NoError(t, err)
	seedPerRequestCostRuleForRouting(t, 11, "expensive-model", "100")

	input := seedanceFactsInput(modelrouting.Seedance20, "1080p", 10, "16:9")
	compatible, err := service.ValidateKnownChannelForRouting(&service.RetryParam{
		Ctx: capabilitySelectionContext(), TokenGroup: "分组A", ModelName: modelrouting.Seedance20,
		RequestPath: "/v1/video/generations", Retry: common.GetPointer(0), RoutingInput: &input,
	}, "分组A", 11)

	assert.False(t, compatible)
	var selectionErr *service.ChannelSelectionError
	require.ErrorAs(t, err, &selectionErr)
	assert.Equal(t, relaytypes.ErrorCodeCompatibleChannelUnavailable, selectionErr.Code)
	assert.Equal(t, http.StatusServiceUnavailable, selectionErr.StatusCode)
}

type invalidVideoMetadataClient struct{}

func (invalidVideoMetadataClient) Metadata(context.Context, string) (videometa.Metadata, error) {
	return videometa.Metadata{}, &service.VideoMetadataError{Kind: service.VideoMetadataInvalidMedia}
}

type unavailableVideoMetadataClient struct{}

func (unavailableVideoMetadataClient) Metadata(context.Context, string) (videometa.Metadata, error) {
	return videometa.Metadata{}, &service.VideoMetadataError{Kind: service.VideoMetadataUnavailable}
}

type routingDurationMetadataClient struct {
	calls      int
	durationMS int64
	err        error
}

func (c *routingDurationMetadataClient) Metadata(context.Context, string) (videometa.Metadata, error) {
	c.calls++
	if c.err != nil {
		return videometa.Metadata{}, c.err
	}
	return videometa.Metadata{DurationMS: c.durationMS, Width: 1280, Height: 720, FrameRateNum: 24, FrameRateDen: 1, Container: "mp4"}, nil
}

func TestCapabilityRoutingEnforcesReferenceVideoTotalDuration(t *testing.T) {
	tests := []struct {
		name       string
		durationMS int64
		wantCode   relaytypes.ErrorCode
		wantStatus int
	}{
		{name: "exact limit", durationMS: 15_000},
		{name: "one millisecond over limit", durationMS: 15_001, wantCode: relaytypes.ErrorCodeNoCompatibleRoute, wantStatus: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prepareCapabilitySelectionTest(t)
			seedRoutingCandidate(t, 11, "duration", "分组A", modelrouting.Seedance20, true)
			request := capabilityPolicyRequest("分组A", modelrouting.Seedance20, 11, "duration-model", "1080p")
			request.Targets[0].Constraints.ReferenceVideoTotalDurationSeconds = serviceIntPtr(15)
			_, err := service.SaveRoutingPolicy(0, request)
			require.NoError(t, err)

			client := &routingDurationMetadataClient{durationMS: tt.durationMS}
			service.SetVideoMetadataClient(client)
			t.Cleanup(func() { service.SetVideoMetadataClient(nil) })
			input := seedanceFactsInput(modelrouting.Seedance20, "1080p", 10, "16:9")
			input.ReferenceVideos = 1
			input.ReferenceVideoURLs = []string{"https://assets.example/input.mp4?signature=secret"}

			channel, _, err := service.CacheGetRandomSatisfiedChannel(&service.RetryParam{
				Ctx: capabilitySelectionContext(), TokenGroup: "分组A", ModelName: modelrouting.Seedance20,
				RequestPath: "/v1/video/generations", Retry: common.GetPointer(0), RoutingInput: &input,
			})

			assert.Equal(t, 1, client.calls)
			if tt.wantCode == "" {
				require.NoError(t, err)
				require.NotNil(t, channel)
				assert.Equal(t, 11, channel.Id)
				return
			}
			assert.Nil(t, channel)
			var selectionErr *service.ChannelSelectionError
			require.ErrorAs(t, err, &selectionErr)
			assert.Equal(t, tt.wantCode, selectionErr.Code)
			assert.Equal(t, tt.wantStatus, selectionErr.StatusCode)
			require.Len(t, selectionErr.Diagnostics, 1)
			assert.Equal(t, 1, selectionErr.Diagnostics[0].MismatchCounts[modelrouting.MismatchReferenceVideoDuration])
			assert.NotContains(t, selectionErr.Error(), "assets.example")
			assert.NotContains(t, selectionErr.Error(), "secret")
		})
	}
}

func TestCapabilityRoutingLoadsMetadataOnlyForDurationConstrainedTargets(t *testing.T) {
	prepareCapabilitySelectionTest(t)
	seedRoutingCandidate(t, 11, "unconstrained", "分组A", modelrouting.Seedance20, true)
	_, err := service.SaveRoutingPolicy(0, capabilityPolicyRequest("分组A", modelrouting.Seedance20, 11, "request-model", "1080p"))
	require.NoError(t, err)

	client := &routingDurationMetadataClient{err: &service.VideoMetadataError{Kind: service.VideoMetadataUnavailable}}
	service.SetVideoMetadataClient(client)
	t.Cleanup(func() { service.SetVideoMetadataClient(nil) })
	input := seedanceFactsInput(modelrouting.Seedance20, "1080p", 10, "16:9")
	input.ReferenceVideos = 1
	input.ReferenceVideoURLs = []string{"https://assets.example/unavailable.mp4?signature=secret"}

	channel, _, err := service.CacheGetRandomSatisfiedChannel(&service.RetryParam{
		Ctx: capabilitySelectionContext(), TokenGroup: "分组A", ModelName: modelrouting.Seedance20,
		RequestPath: "/v1/video/generations", Retry: common.GetPointer(0), RoutingInput: &input,
	})

	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 11, channel.Id)
	assert.Zero(t, client.calls)
}

func TestCapabilityRoutingKeepsUnconstrainedTargetsWhenVideoMetadataIsUnavailable(t *testing.T) {
	prepareCapabilitySelectionTest(t)
	seedRoutingCandidate(t, 11, "duration", "分组A", modelrouting.Seedance20, true)
	seedRoutingCandidate(t, 12, "unconstrained", "分组A", modelrouting.Seedance20, true)
	request := multiTargetPolicyRequest("分组A", modelrouting.Seedance20, "1080p", []policyTarget{
		{ChannelID: 11, UpstreamModel: "duration-model"},
		{ChannelID: 12, UpstreamModel: "unconstrained-model"},
	})
	request.Targets[0].Constraints.ReferenceVideoTotalDurationSeconds = serviceIntPtr(15)
	_, err := service.SaveRoutingPolicy(0, request)
	require.NoError(t, err)

	client := &routingDurationMetadataClient{err: &service.VideoMetadataError{Kind: service.VideoMetadataUnavailable}}
	service.SetVideoMetadataClient(client)
	t.Cleanup(func() { service.SetVideoMetadataClient(nil) })
	input := seedanceFactsInput(modelrouting.Seedance20, "1080p", 10, "16:9")
	input.ReferenceVideos = 1
	input.ReferenceVideoURLs = []string{"https://assets.example/unavailable.mp4?signature=secret"}

	channel, _, err := service.CacheGetRandomSatisfiedChannel(&service.RetryParam{
		Ctx: capabilitySelectionContext(), TokenGroup: "分组A", ModelName: modelrouting.Seedance20,
		RequestPath: "/v1/video/generations", Retry: common.GetPointer(0), RoutingInput: &input,
	})

	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 12, channel.Id)
	assert.Equal(t, 1, client.calls)
}

func TestCapabilityRoutingFailsClosedWhenRequiredVideoMetadataIsUnavailable(t *testing.T) {
	tests := []struct {
		name       string
		kind       service.VideoMetadataErrorKind
		wantCode   relaytypes.ErrorCode
		wantStatus int
	}{
		{name: "invalid media", kind: service.VideoMetadataInvalidMedia, wantCode: relaytypes.ErrorCodeInvalidRequest, wantStatus: http.StatusBadRequest},
		{name: "metadata unavailable", kind: service.VideoMetadataUnavailable, wantCode: relaytypes.ErrorCodeCompatibleChannelUnavailable, wantStatus: http.StatusServiceUnavailable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prepareCapabilitySelectionTest(t)
			seedRoutingCandidate(t, 11, "duration", "分组A", modelrouting.Seedance20, true)
			request := capabilityPolicyRequest("分组A", modelrouting.Seedance20, 11, "duration-model", "1080p")
			request.Targets[0].Constraints.ReferenceVideoTotalDurationSeconds = serviceIntPtr(15)
			_, err := service.SaveRoutingPolicy(0, request)
			require.NoError(t, err)

			client := &routingDurationMetadataClient{err: &service.VideoMetadataError{Kind: tt.kind}}
			service.SetVideoMetadataClient(client)
			t.Cleanup(func() { service.SetVideoMetadataClient(nil) })
			input := seedanceFactsInput(modelrouting.Seedance20, "1080p", 10, "16:9")
			input.ReferenceVideos = 1
			input.ReferenceVideoURLs = []string{"https://assets.example/failure.mp4?signature=secret"}

			channel, _, err := service.CacheGetRandomSatisfiedChannel(&service.RetryParam{
				Ctx: capabilitySelectionContext(), TokenGroup: "分组A", ModelName: modelrouting.Seedance20,
				RequestPath: "/v1/video/generations", Retry: common.GetPointer(0), RoutingInput: &input,
			})

			assert.Nil(t, channel)
			assert.Equal(t, 1, client.calls)
			var selectionErr *service.ChannelSelectionError
			require.ErrorAs(t, err, &selectionErr)
			assert.Equal(t, tt.wantCode, selectionErr.Code)
			assert.Equal(t, tt.wantStatus, selectionErr.StatusCode)
			assert.NotContains(t, selectionErr.Error(), "assets.example")
			assert.NotContains(t, selectionErr.Error(), "secret")
		})
	}
}

// TestProfitRoutingNoOpOutsideStrictMode asserts the filter is inert when cost
// accounting is disabled: both channels remain selectable even though one carries a
// cost that would fail the margin gate under strict mode.
func TestProfitRoutingNoOpOutsideStrictMode(t *testing.T) {
	prepareCapabilitySelectionTest(t)
	require.NoError(t, model.DB.AutoMigrate(&model.ChannelModelCostRule{}))
	require.NoError(t, model.DB.Exec("DELETE FROM channel_model_cost_rules").Error)
	service.InvalidateCostCoverage(0, "", "")
	t.Cleanup(func() {
		service.InvalidateCostCoverage(0, "", "")
		model.DB.Exec("DELETE FROM channel_model_cost_rules")
	})
	seedRoutingCandidate(t, 11, "cheap", "分组A", modelrouting.Seedance20, true)
	seedRoutingCandidate(t, 12, "expensive", "分组A", modelrouting.Seedance20, true)
	_, err := service.SaveRoutingPolicy(0, multiTargetPolicyRequest("分组A", modelrouting.Seedance20, "1080p",
		[]policyTarget{{ChannelID: 11, UpstreamModel: "cheap-model"}, {ChannelID: 12, UpstreamModel: "expensive-model"}}))
	require.NoError(t, err)
	seedPerRequestCostRuleForRouting(t, 12, "expensive-model", "100")

	input := seedanceFactsInput(modelrouting.Seedance20, "1080p", 10, "16:9")
	param := &service.RetryParam{
		Ctx: capabilitySelectionContext(), TokenGroup: "分组A", ModelName: modelrouting.Seedance20,
		RequestPath: "/v1/video/generations", Retry: common.GetPointer(0), RoutingInput: &input,
	}
	seen := map[int]bool{}
	for attempt := 0; attempt < 20; attempt++ {
		param.SetRetry(0)
		selected, _, selectErr := service.CacheGetRandomSatisfiedChannel(param)
		require.NoError(t, selectErr)
		require.NotNil(t, selected)
		seen[selected.Id] = true
	}
	// Both candidates must be reachable because the margin filter is a no-op outside
	// strict mode; the weighted random pick keeps its full candidate set.
	assert.True(t, seen[11] && seen[12], "both candidates must remain selectable outside strict mode")
}

type policyTarget struct {
	ChannelID     int
	UpstreamModel string
}

func multiTargetPolicyRequest(group, modelName, resolution string, targets []policyTarget) service.RoutingPolicyWriteRequest {
	supportsRealPerson := true
	request := service.RoutingPolicyWriteRequest{
		GroupName: group,
		Model:     modelName,
		Enabled:   true,
		Defaults: modelrouting.Defaults{
			OutputResolution: resolution,
			DurationSeconds:  10,
			AspectRatio:      "16:9",
		},
	}
	for _, target := range targets {
		request.Targets = append(request.Targets, service.RouteTargetWriteRequest{
			ChannelID:      target.ChannelID,
			Name:           target.UpstreamModel,
			UpstreamModel:  target.UpstreamModel,
			TargetPriority: 100,
			Enabled:        true,
			Constraints: modelrouting.Constraints{
				OutputResolutions:  []string{resolution},
				Durations:          modelrouting.DurationConstraint{Min: serviceIntPtr(4), Max: serviceIntPtr(15)},
				AspectRatios:       []string{"16:9", "9:16"},
				ReferenceLimits:    modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3},
				SupportsRealPerson: &supportsRealPerson,
			},
		})
	}
	return request
}

func seedPerRequestCostRuleForRouting(t *testing.T, channelID int, modelName, unitPriceUSD string) {
	t.Helper()
	config := types.CostRuleConfigV1{
		Currency:              "USD",
		BillingMultiplier:     "1",
		PurchaseDiscountRatio: "1",
		RechargeExchangeRatio: "1",
		FeeRate:               "0",
		CurrencyToUSDRate:     "1",
		UnitPrice:             &unitPriceUSD,
		ChargeEvent:           types.CostChargeResponseSucceeded,
	}
	normalized, err := service.NormalizeCostRuleConfig(types.CostModePerRequest, config)
	require.NoError(t, err)
	configJSON, err := common.Marshal(normalized)
	require.NoError(t, err)
	now := common.GetTimestamp()
	require.NoError(t, model.DB.Create(&model.ChannelModelCostRule{
		ChannelID: channelID, BillableUpstreamModel: modelName, CostVariantKey: string(types.DefaultCostVariantKey), Version: 1,
		Status: string(types.CostRuleActive), CostMode: string(types.CostModePerRequest), SchemaVersion: 1,
		ConfigJSON: string(configJSON), Source: "manual", EffectiveFrom: &now,
		CreatedAt: now, UpdatedAt: now,
	}).Error)
}

func seedTokenCostRuleForRouting(t *testing.T, channelID int, modelName string) {
	t.Helper()
	price := "1"
	config := types.CostRuleConfigV1{
		Currency:              "USD",
		BillingMultiplier:     "1",
		PurchaseDiscountRatio: "1",
		RechargeExchangeRatio: "1",
		FeeRate:               "0",
		CurrencyToUSDRate:     "1",
		TotalPerMillion:       &price,
		TokenMode:             types.CostTokenModeTotal,
		MeterSource:           types.CostMeterLocalUsage,
		ChargeEvent:           types.CostChargeResponseSucceeded,
	}
	normalized, err := service.NormalizeCostRuleConfig(types.CostModePerToken, config)
	require.NoError(t, err)
	configJSON, err := common.Marshal(normalized)
	require.NoError(t, err)
	now := common.GetTimestamp()
	require.NoError(t, model.DB.Create(&model.ChannelModelCostRule{
		ChannelID: channelID, BillableUpstreamModel: modelName, CostVariantKey: string(types.DefaultCostVariantKey), Version: 1,
		Status: string(types.CostRuleActive), CostMode: string(types.CostModePerToken), SchemaVersion: 1,
		ConfigJSON: string(configJSON), Source: "manual", EffectiveFrom: &now,
		CreatedAt: now, UpdatedAt: now,
	}).Error)
}

func TestSelectCapabilityChannelClassifiesNoMatchAndUnavailable(t *testing.T) {
	tests := []struct {
		name           string
		resolution     string
		duration       int
		disableAbility bool
		excludeChannel bool
		wantCode       relaytypes.ErrorCode
		wantStatus     int
	}{
		{name: "unsupported resolution", resolution: "4k", duration: 10, wantCode: relaytypes.ErrorCodeNoCompatibleRoute, wantStatus: http.StatusBadRequest},
		{name: "smart duration", resolution: "1080p", duration: -1, wantCode: relaytypes.ErrorCodeNoCompatibleRoute, wantStatus: http.StatusBadRequest},
		{name: "compatible channel disabled", resolution: "1080p", duration: 10, disableAbility: true, wantCode: relaytypes.ErrorCodeCompatibleChannelUnavailable, wantStatus: http.StatusServiceUnavailable},
		{name: "compatible channel excluded", resolution: "1080p", duration: 10, excludeChannel: true, wantCode: relaytypes.ErrorCodeCompatibleChannelUnavailable, wantStatus: http.StatusServiceUnavailable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prepareCapabilitySelectionTest(t)
			seedRoutingCandidate(t, 11, "A1", "分组A", modelrouting.Seedance20, true)
			_, err := service.SaveRoutingPolicy(0, capabilityPolicyRequest("分组A", modelrouting.Seedance20, 11, "provider-1080p", "1080p"))
			require.NoError(t, err)
			if tt.disableAbility {
				require.NoError(t, model.DB.Model(&model.Ability{}).Where("channel_id = ?", 11).Update("enabled", false).Error)
			}

			c := capabilitySelectionContext()
			input := seedanceFactsInput(modelrouting.Seedance20, tt.resolution, tt.duration, "16:9")
			param := &service.RetryParam{
				Ctx: c, TokenGroup: "分组A", ModelName: modelrouting.Seedance20,
				RequestPath: "/v1/video/generations", Retry: common.GetPointer(0), RoutingInput: &input,
			}
			if tt.excludeChannel {
				param.ExcludeChannel(11)
			}

			channel, _, err := service.CacheGetRandomSatisfiedChannel(param)
			assert.Nil(t, channel)
			var selectionErr *service.ChannelSelectionError
			require.ErrorAs(t, err, &selectionErr)
			assert.Equal(t, tt.wantCode, selectionErr.Code)
			assert.Equal(t, tt.wantStatus, selectionErr.StatusCode)
		})
	}
}

func TestAutoGroupCapabilitySelectsLaterMatchingPolicy(t *testing.T) {
	prepareCapabilitySelectionTest(t)
	prepareAutoGroupSelectionTest(t)
	seedRoutingCandidate(t, 11, "A1", "分组A", modelrouting.Seedance20, true)
	seedRoutingCandidate(t, 12, "A2", "分组B", modelrouting.Seedance20, true)
	_, err := service.SaveRoutingPolicy(0, capabilityPolicyRequest("分组A", modelrouting.Seedance20, 11, "provider-720p", "720p"))
	require.NoError(t, err)
	_, err = service.SaveRoutingPolicy(0, capabilityPolicyRequest("分组B", modelrouting.Seedance20, 12, "provider-1080p", "1080p"))
	require.NoError(t, err)

	c := capabilitySelectionContext()
	common.SetContextKey(c, constant.ContextKeyUserGroup, "分组A")
	input := seedanceFactsInput(modelrouting.Seedance20, "1080p", 10, "16:9")
	channel, group, err := service.CacheGetRandomSatisfiedChannel(&service.RetryParam{
		Ctx: c, TokenGroup: "auto", ModelName: modelrouting.Seedance20,
		RequestPath: "/v1/video/generations", Retry: common.GetPointer(0), RoutingInput: &input,
	})
	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 12, channel.Id)
	assert.Equal(t, "分组B", group)
	assert.Equal(t, "分组B", common.GetContextKeyString(c, constant.ContextKeyAutoGroup))
	facts, ok := common.GetContextKeyType[modelrouting.Facts](c, constant.ContextKeyRoutingFacts)
	require.True(t, ok)
	assert.Equal(t, "分组B", facts.GroupName)
}

func TestAutoGroupCapabilityAggregatesRoutingErrors(t *testing.T) {
	tests := []struct {
		name               string
		disableSecondRoute bool
		wantCode           relaytypes.ErrorCode
		wantStatus         int
	}{
		{name: "all policies have no match", wantCode: relaytypes.ErrorCodeNoCompatibleRoute, wantStatus: http.StatusBadRequest},
		{name: "a compatible route is unavailable", disableSecondRoute: true, wantCode: relaytypes.ErrorCodeCompatibleChannelUnavailable, wantStatus: http.StatusServiceUnavailable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prepareCapabilitySelectionTest(t)
			prepareAutoGroupSelectionTest(t)
			seedRoutingCandidate(t, 11, "A1", "分组A", modelrouting.Seedance20, true)
			seedRoutingCandidate(t, 12, "A2", "分组B", modelrouting.Seedance20, true)
			_, err := service.SaveRoutingPolicy(0, capabilityPolicyRequest("分组A", modelrouting.Seedance20, 11, "provider-720p", "720p"))
			require.NoError(t, err)
			secondResolution := "720p"
			if tt.disableSecondRoute {
				secondResolution = "1080p"
			}
			_, err = service.SaveRoutingPolicy(0, capabilityPolicyRequest("分组B", modelrouting.Seedance20, 12, "provider-second", secondResolution))
			require.NoError(t, err)
			if tt.disableSecondRoute {
				require.NoError(t, model.DB.Model(&model.Ability{}).Where("channel_id = ?", 12).Update("enabled", false).Error)
			}

			c := capabilitySelectionContext()
			common.SetContextKey(c, constant.ContextKeyUserGroup, "分组A")
			input := seedanceFactsInput(modelrouting.Seedance20, "1080p", 10, "16:9")
			channel, _, err := service.CacheGetRandomSatisfiedChannel(&service.RetryParam{
				Ctx: c, TokenGroup: "auto", ModelName: modelrouting.Seedance20,
				RequestPath: "/v1/video/generations", Retry: common.GetPointer(0), RoutingInput: &input,
			})
			assert.Nil(t, channel)
			var selectionErr *service.ChannelSelectionError
			require.ErrorAs(t, err, &selectionErr)
			assert.Equal(t, tt.wantCode, selectionErr.Code)
			assert.Equal(t, tt.wantStatus, selectionErr.StatusCode)
			require.Len(t, selectionErr.Diagnostics, 2)
		})
	}
}

func TestAutoGroupCapabilityKeepsLegacyGroupEligible(t *testing.T) {
	prepareCapabilitySelectionTest(t)
	prepareAutoGroupSelectionTest(t)
	seedRoutingCandidate(t, 11, "A1", "分组A", modelrouting.Seedance20, true)
	c := capabilitySelectionContext()
	common.SetContextKey(c, constant.ContextKeyUserGroup, "分组A")
	input := seedanceFactsInput(modelrouting.Seedance20, "1080p", 10, "16:9")

	channel, group, err := service.CacheGetRandomSatisfiedChannel(&service.RetryParam{
		Ctx: c, TokenGroup: "auto", ModelName: modelrouting.Seedance20,
		RequestPath: "/v1/video/generations", Retry: common.GetPointer(0), RoutingInput: &input,
	})
	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 11, channel.Id)
	assert.Equal(t, "分组A", group)
	assert.False(t, common.GetContextKeyBool(c, constant.ContextKeyRoutingCapabilityMode))
}

func TestValidateKnownChannelForRoutingRechecksCompatibilityAndAvailability(t *testing.T) {
	prepareCapabilitySelectionTest(t)
	seedRoutingCandidate(t, 11, "A1", "分组A", modelrouting.Seedance20, true)
	seedRoutingCandidate(t, 12, "A2", "分组A", modelrouting.Seedance20, true)
	request := capabilityPolicyRequest("分组A", modelrouting.Seedance20, 11, "provider-720p", "720p")
	second := capabilityPolicyRequest("分组A", modelrouting.Seedance20, 12, "provider-1080p", "1080p").Targets[0]
	request.Targets = append(request.Targets, second)
	_, err := service.SaveRoutingPolicy(0, request)
	require.NoError(t, err)

	c := capabilitySelectionContext()
	input := seedanceFactsInput(modelrouting.Seedance20, "1080p", 10, "16:9")
	param := &service.RetryParam{
		Ctx: c, TokenGroup: "分组A", ModelName: modelrouting.Seedance20,
		RequestPath: "/v1/video/generations", Retry: common.GetPointer(0), RoutingInput: &input,
	}

	compatible, err := service.ValidateKnownChannelForRouting(param, "分组A", 11)
	require.NoError(t, err)
	assert.False(t, compatible)
	assert.False(t, common.GetContextKeyBool(c, constant.ContextKeyRoutingCapabilityMode))

	compatible, err = service.ValidateKnownChannelForRouting(param, "分组A", 12)
	require.NoError(t, err)
	assert.True(t, compatible)
	assert.True(t, common.GetContextKeyBool(c, constant.ContextKeyRoutingCapabilityMode))
	assert.Equal(t, "provider-1080p", common.GetContextKeyString(c, constant.ContextKeyRoutingUpstreamModel))

	require.NoError(t, model.DB.Model(&model.Ability{}).Where("channel_id = ?", 12).Update("enabled", false).Error)
	compatible, err = service.ValidateKnownChannelForRouting(param, "分组A", 12)
	assert.False(t, compatible)
	var selectionErr *service.ChannelSelectionError
	require.ErrorAs(t, err, &selectionErr)
	assert.Equal(t, relaytypes.ErrorCodeCompatibleChannelUnavailable, selectionErr.Code)
}

func prepareCapabilitySelectionTest(t *testing.T) {
	t.Helper()
	prepareRoutingPolicyServiceTest(t)
	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false
	t.Cleanup(func() { common.MemoryCacheEnabled = previousMemoryCacheEnabled })
}

func prepareDynamicGroupRoutingSelectionTest(t *testing.T) {
	t.Helper()
	prepareCapabilitySelectionTest(t)
	require.NoError(t, model.DB.AutoMigrate(&model.ChannelModelCostRule{}))
	require.NoError(t, model.DB.Exec("DELETE FROM channel_model_cost_rules").Error)
	service.InvalidateCostCoverage(0, "", "")
	t.Cleanup(func() {
		service.InvalidateCostCoverage(0, "", "")
		require.NoError(t, model.DB.Exec("DELETE FROM channel_model_cost_rules").Error)
	})
}

func prepareStrictCostRoutingServiceTest(t *testing.T) {
	t.Helper()
	prepareCapabilitySelectionTest(t)
	require.NoError(t, model.DB.AutoMigrate(&model.ChannelModelCostRule{}))
	require.NoError(t, model.DB.Exec("DELETE FROM channel_model_cost_rules").Error)
	service.InvalidateCostCoverage(0, "", "")

	previousLookup := service.CostCapabilityLookup
	service.CostCapabilityLookup = func(_ int, requestPath string, _ constant.TaskPlatform) types.CostCapabilities {
		capabilities := types.CostCapabilities{CanResolveBillableModel: true}
		if requestPath == "/v1/images/generations" || requestPath == "/v1/images/edits" {
			capabilities.ChargeEvents = []types.CostChargeEvent{types.CostChargeResponseSucceeded}
			capabilities.MeterSources = []types.CostMeterSource{
				types.CostMeterValidatedRequest,
				types.CostMeterUpstreamActual,
			}
		}
		return capabilities
	}
	costConfig := config.GlobalConfig.Get(cost_setting.ConfigName)
	require.NotNil(t, costConfig)
	previousMode := cost_setting.Runtime().Mode
	require.NoError(t, config.UpdateConfigFromMap(costConfig, map[string]string{
		cost_setting.KeyMode: string(types.CostAccountingStrict),
	}))
	cost_setting.UpdateAndSync()

	// Inject a revenue preview callback so strict-mode profit routing has a positive
	// revenue to compare against the (typically free) cost rule these tests seed. The
	// callback mirrors the production wiring (main.go) but returns a fixed positive
	// quota so free-cost candidates pass the margin gate; pure profit-margin behavior
	// is asserted in dedicated profit_routing tests instead.
	previousRevenueHook := service.RevenuePreviewHookForTest()
	service.SetRoutingRevenuePreview(func(_ context.Context, _ service.RoutingRevenuePreviewInput) (int64, string, error) {
		return 1_000_000, "500000", nil
	})

	t.Cleanup(func() {
		require.NoError(t, config.UpdateConfigFromMap(costConfig, map[string]string{
			cost_setting.KeyMode: string(previousMode),
		}))
		cost_setting.UpdateAndSync()
		service.CostCapabilityLookup = previousLookup
		service.SetRoutingRevenuePreview(previousRevenueHook)
		service.InvalidateCostCoverage(0, "", "")
		require.NoError(t, model.DB.Exec("DELETE FROM channel_model_cost_rules").Error)
	})
}

func prepareAutoGroupSelectionTest(t *testing.T) {
	t.Helper()
	previousAutoGroups := setting.AutoGroups2JsonString()
	previousUsableGroups := setting.UserUsableGroups2JSONString()
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["分组A","分组B"]`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"分组A":"A","分组B":"B"}`))
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateAutoGroupsByJsonString(previousAutoGroups))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(previousUsableGroups))
	})
}

func setGroupRoutingRequirementsForTest(t *testing.T, value string) {
	t.Helper()
	original := ratio_setting.GroupRoutingRequirements2JSONString()
	require.NoError(t, ratio_setting.UpdateGroupRoutingRequirementsByJSONString(value))
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRoutingRequirementsByJSONString(original))
	})
}

func seedActiveFreeCostRuleForRouting(t *testing.T, channelID int, modelName string) {
	t.Helper()
	configJSON, err := common.Marshal(types.CostRuleConfigV1{ZeroCostReason: "supplier contract"})
	require.NoError(t, err)
	now := common.GetTimestamp()
	require.NoError(t, model.DB.Create(&model.ChannelModelCostRule{
		ChannelID: channelID, BillableUpstreamModel: modelName, CostVariantKey: string(types.DefaultCostVariantKey), Version: 1,
		Status: string(types.CostRuleActive), CostMode: string(types.CostModeFree), SchemaVersion: 1,
		ConfigJSON: string(configJSON), Source: "manual", EffectiveFrom: &now,
		CreatedAt: now, UpdatedAt: now,
	}).Error)
}

func seedActiveCostModeRuleForRouting(t *testing.T, channelID int, modelName string, mode types.CostMode) {
	t.Helper()
	now := common.GetTimestamp()
	require.NoError(t, model.DB.Create(&model.ChannelModelCostRule{
		ChannelID: channelID, BillableUpstreamModel: modelName, CostVariantKey: string(types.DefaultCostVariantKey), Version: 1,
		Status: string(types.CostRuleActive), CostMode: string(mode), SchemaVersion: 1,
		ConfigJSON: `{}`, Source: "manual", EffectiveFrom: &now, CreatedAt: now, UpdatedAt: now,
	}).Error)
	service.InvalidateCostCoverage(channelID, modelName, string(types.DefaultCostVariantKey))
}

func capabilitySelectionContext() *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/video/generations", nil)
	return c
}

func seedanceFactsInput(modelName, resolution string, duration int, ratio string) modelrouting.FactsInput {
	return modelrouting.FactsInput{
		CanonicalModel:   modelName,
		OutputResolution: &resolution,
		DurationSeconds:  &duration,
		AspectRatio:      &ratio,
	}
}

func capabilityPolicyRequest(group, modelName string, channelID int, upstreamModel, resolution string) service.RoutingPolicyWriteRequest {
	supportsRealPerson := true
	return service.RoutingPolicyWriteRequest{
		GroupName: group,
		Model:     modelName,
		Enabled:   true,
		Defaults: modelrouting.Defaults{
			OutputResolution: resolution,
			DurationSeconds:  10,
			AspectRatio:      "16:9",
		},
		Targets: []service.RouteTargetWriteRequest{{
			ChannelID:      channelID,
			Name:           upstreamModel,
			UpstreamModel:  upstreamModel,
			TargetPriority: 100,
			Enabled:        true,
			Constraints: modelrouting.Constraints{
				OutputResolutions:  []string{resolution},
				Durations:          modelrouting.DurationConstraint{Min: serviceIntPtr(4), Max: serviceIntPtr(15)},
				AspectRatios:       []string{"16:9", "9:16"},
				ReferenceLimits:    modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3},
				SupportsRealPerson: &supportsRealPerson,
			},
		}},
	}
}
