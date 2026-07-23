package service_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelrouting"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSelectCapabilityChannelPublishesTargetDecision(t *testing.T) {
	prepareCapabilitySelectionTest(t)
	seedRoutingCandidate(t, 11, "A1", "分组A", modelrouting.Seedance20, true)
	policy := capabilityPolicyRequest("分组A", modelrouting.Seedance20, 11, "provider-1080p", "1080p")
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
	facts, ok := common.GetContextKeyType[modelrouting.Facts](c, constant.ContextKeyRoutingFacts)
	require.True(t, ok)
	assert.Equal(t, "分组A", facts.GroupName)
	assert.Equal(t, "1080p", facts.OutputResolution)
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
}

func TestSelectCapabilityChannelClassifiesNoMatchAndUnavailable(t *testing.T) {
	tests := []struct {
		name           string
		resolution     string
		duration       int
		disableAbility bool
		excludeChannel bool
		wantCode       types.ErrorCode
		wantStatus     int
	}{
		{name: "unsupported resolution", resolution: "4k", duration: 10, wantCode: types.ErrorCodeNoCompatibleRoute, wantStatus: http.StatusBadRequest},
		{name: "smart duration", resolution: "1080p", duration: -1, wantCode: types.ErrorCodeNoCompatibleRoute, wantStatus: http.StatusBadRequest},
		{name: "compatible channel disabled", resolution: "1080p", duration: 10, disableAbility: true, wantCode: types.ErrorCodeCompatibleChannelUnavailable, wantStatus: http.StatusServiceUnavailable},
		{name: "compatible channel excluded", resolution: "1080p", duration: 10, excludeChannel: true, wantCode: types.ErrorCodeCompatibleChannelUnavailable, wantStatus: http.StatusServiceUnavailable},
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
				param.ExcludeCapabilityChannel(11)
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
		wantCode           types.ErrorCode
		wantStatus         int
	}{
		{name: "all policies have no match", wantCode: types.ErrorCodeNoCompatibleRoute, wantStatus: http.StatusBadRequest},
		{name: "a compatible route is unavailable", disableSecondRoute: true, wantCode: types.ErrorCodeCompatibleChannelUnavailable, wantStatus: http.StatusServiceUnavailable},
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
	assert.Equal(t, types.ErrorCodeCompatibleChannelUnavailable, selectionErr.Code)
}

func prepareCapabilitySelectionTest(t *testing.T) {
	t.Helper()
	prepareRoutingPolicyServiceTest(t)
	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false
	t.Cleanup(func() { common.MemoryCacheEnabled = previousMemoryCacheEnabled })
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
