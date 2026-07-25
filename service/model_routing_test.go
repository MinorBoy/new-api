package service_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelrouting"
	"github.com/QuantumNous/new-api/pkg/videometa"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/cost_setting"
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
	assert.Equal(t, types.ErrorCodeCompatibleChannelUnavailable, selectionErr.Code)
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
	assert.Equal(t, types.ErrorCodeInvalidRequest, selectionErr.Code)
	assert.Equal(t, http.StatusBadRequest, selectionErr.StatusCode)
	assert.Equal(t, "input video is not supported", selectionErr.Err.Error())
	assert.NotContains(t, selectionErr.Err.Error(), "assets.example")
	assert.NotContains(t, selectionErr.Err.Error(), "secret")
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
	assert.Equal(t, types.ErrorCodeCompatibleChannelUnavailable, selectionErr.Code)
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
	assert.Equal(t, types.ErrorCodeCompatibleChannelUnavailable, selectionErr.Code)
	assert.Equal(t, http.StatusServiceUnavailable, selectionErr.StatusCode)
}

type invalidVideoMetadataClient struct{}

func (invalidVideoMetadataClient) Metadata(context.Context, string) (videometa.Metadata, error) {
	return videometa.Metadata{}, &service.VideoMetadataError{Kind: service.VideoMetadataInvalidMedia}
}

// TestProfitRoutingNoOpOutsideStrictMode asserts the filter is inert when cost
// accounting is disabled: both channels remain selectable even though one carries a
// cost that would fail the margin gate under strict mode.
func TestProfitRoutingNoOpOutsideStrictMode(t *testing.T) {
	prepareCapabilitySelectionTest(t)
	require.NoError(t, model.DB.AutoMigrate(&model.ChannelModelCostRule{}))
	require.NoError(t, model.DB.Exec("DELETE FROM channel_model_cost_rules").Error)
	service.InvalidateCostCoverage(0, "")
	t.Cleanup(func() {
		service.InvalidateCostCoverage(0, "")
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
		ChannelID: channelID, BillableUpstreamModel: modelName, Version: 1,
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
		ChannelID: channelID, BillableUpstreamModel: modelName, Version: 1,
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

func prepareStrictCostRoutingServiceTest(t *testing.T) {
	t.Helper()
	prepareCapabilitySelectionTest(t)
	require.NoError(t, model.DB.AutoMigrate(&model.ChannelModelCostRule{}))
	require.NoError(t, model.DB.Exec("DELETE FROM channel_model_cost_rules").Error)
	service.InvalidateCostCoverage(0, "")

	previousLookup := service.CostCapabilityLookup
	service.CostCapabilityLookup = func(int, string, constant.TaskPlatform) types.CostCapabilities {
		return types.CostCapabilities{CanResolveBillableModel: true}
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
		service.InvalidateCostCoverage(0, "")
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

func seedActiveFreeCostRuleForRouting(t *testing.T, channelID int, modelName string) {
	t.Helper()
	configJSON, err := common.Marshal(types.CostRuleConfigV1{ZeroCostReason: "supplier contract"})
	require.NoError(t, err)
	now := common.GetTimestamp()
	require.NoError(t, model.DB.Create(&model.ChannelModelCostRule{
		ChannelID: channelID, BillableUpstreamModel: modelName, Version: 1,
		Status: string(types.CostRuleActive), CostMode: string(types.CostModeFree), SchemaVersion: 1,
		ConfigJSON: string(configJSON), Source: "manual", EffectiveFrom: &now,
		CreatedAt: now, UpdatedAt: now,
	}).Error)
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
