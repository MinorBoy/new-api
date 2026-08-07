package e2e

import (
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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	groupRoutingFallbackGroup = "分组B"
	groupRoutingFallbackModel = "bb-seedance2.0-auto-fallback"
	groupRoutingTokenKey      = "e2e-group-routing"
	groupRoutingRetryModel    = "bb-seedance2.0-1080p-real-person-retry"
)

func setGroupRoutingRequirementsE2E(t *testing.T, value string) {
	t.Helper()
	original := ratio_setting.GroupRoutingRequirements2JSONString()
	require.NoError(t, ratio_setting.UpdateGroupRoutingRequirementsByJSONString(value))
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRoutingRequirementsByJSONString(original))
	})
}

func TestGroupRoutingRequirementsForcesRealPersonRouteE2E(t *testing.T) {
	env := setupSeedanceCapabilityRoutingE2E(t)
	setGroupRoutingRequirementsE2E(t, `{"分组A":{"require_real_person":true}}`)

	body := capabilityRequestBody(t, modelrouting.Seedance20, "720p", 10, "16:9", modelrouting.ReferenceLimits{Images: 4, Videos: 3, Audios: 1}, false)
	status, response := performJSONRequest(t, env.engine, http.MethodPost, "/api/v3/contents/generations/tasks", "Bearer e2e", body)
	require.Equal(t, http.StatusOK, status, string(response))

	var task model.Task
	require.NoError(t, model.DB.Order("id DESC").First(&task).Error)
	assert.Equal(t, capabilityChannelB, task.ChannelId)
	require.NotNil(t, task.PrivateData.Routing)
	assert.Equal(t, upstreamStandardMG, task.PrivateData.Routing.UpstreamModel)
	assert.True(t, task.PrivateData.Routing.Facts.RequireRealPerson)
	require.Len(t, env.channelB.snapshot(), 1)
	assert.Empty(t, env.channelA.snapshot())

	adminLogs, _, err := model.GetAllLogs(model.LogTypeConsume, 0, 0, modelrouting.Seedance20, "", "", 0, 20, 0, "", "", "")
	require.NoError(t, err)
	require.NotEmpty(t, adminLogs)
	var other map[string]any
	require.NoError(t, common.UnmarshalJsonStr(adminLogs[0].Other, &other))
	adminInfo, ok := other["admin_info"].(map[string]any)
	require.True(t, ok)
	routing, ok := adminInfo["routing"].(map[string]any)
	require.True(t, ok)
	facts, ok := routing["facts"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, true, facts["require_real_person"])
}

func TestGroupRoutingRequirementsRejectsUnsupportedSpecificChannelE2E(t *testing.T) {
	env := setupSeedanceCapabilityRoutingE2E(t)
	setGroupRoutingRequirementsE2E(t, `{"分组A":{"require_real_person":true}}`)

	body := capabilityRequestBody(t, modelrouting.Seedance20, "1080p", 15, "9:16", modelrouting.ReferenceLimits{}, false)
	status, response := performJSONRequest(t, env.engine, http.MethodPost, "/api/v3/contents/generations/tasks", "Bearer e2e-1", body)
	require.Equal(t, http.StatusBadRequest, status, string(response))
	assert.Contains(t, string(response), `"code":"no_compatible_route"`)
	assert.Empty(t, env.channelA.snapshot())
	assert.Empty(t, env.channelB.snapshot())
}

func TestGroupRoutingRequirementsLogsRealPersonMismatchE2E(t *testing.T) {
	env := setupSeedanceCapabilityRoutingE2E(t)
	setGroupRoutingRequirementsE2E(t, `{"分组A":{"require_real_person":true}}`)
	previousErrorLogEnabled := constant.ErrorLogEnabled
	constant.ErrorLogEnabled = true
	t.Cleanup(func() { constant.ErrorLogEnabled = previousErrorLogEnabled })

	body := capabilityRequestBody(t, modelrouting.Seedance20, "720p", 15, "9:16", modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3}, false)
	status, response := performJSONRequest(t, env.engine, http.MethodPost, "/api/v3/contents/generations/tasks", "Bearer e2e", body)
	require.Equal(t, http.StatusBadRequest, status, string(response))
	assert.Contains(t, string(response), `"code":"no_compatible_route"`)
	assert.Empty(t, env.channelA.snapshot())
	assert.Empty(t, env.channelB.snapshot())

	adminLogs, _, err := model.GetAllLogs(model.LogTypeError, 0, 0, modelrouting.Seedance20, "", "", 0, 20, 0, "", "", "")
	require.NoError(t, err)
	require.NotEmpty(t, adminLogs)
	var other map[string]any
	require.NoError(t, common.UnmarshalJsonStr(adminLogs[0].Other, &other))
	adminInfo, ok := other["admin_info"].(map[string]any)
	require.True(t, ok)
	routingSelection, ok := adminInfo["routing_selection"].([]any)
	require.True(t, ok)
	require.NotEmpty(t, routingSelection)
	firstDiagnostic, ok := routingSelection[0].(map[string]any)
	require.True(t, ok)
	mismatchCounts, ok := firstDiagnostic["mismatch_counts"].(map[string]any)
	require.True(t, ok)
	assert.Positive(t, mismatchCounts[string(modelrouting.MismatchRealPerson)])
}

func TestGroupRoutingRequirementsPersistAcrossRetryE2E(t *testing.T) {
	env := setupSeedanceCapabilityRoutingE2E(t)
	setGroupRoutingRequirementsE2E(t, `{"分组A":{"require_real_person":true}}`)

	retryTarget := capabilityTarget(
		capabilityChannelA,
		groupRoutingRetryModel,
		120,
		[]string{"1080p"},
		discreteDuration(15),
		[]string{"9:16"},
		modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3},
		true,
	)
	env.standardRequest.Targets = append(env.standardRequest.Targets, retryTarget)
	_, err := service.SaveRoutingPolicy(env.standardPolicy, env.standardRequest)
	require.NoError(t, err)
	env.channelA.submitStatus = http.StatusInternalServerError
	env.channelA.submitResponse = `{"error":{"code":"upstream_failure","message":"retry first group target"}}`

	previousRetryTimes := common.RetryTimes
	common.RetryTimes = 1
	t.Cleanup(func() { common.RetryTimes = previousRetryTimes })
	body := capabilityRequestBody(t, modelrouting.Seedance20, "1080p", 15, "9:16", modelrouting.ReferenceLimits{}, false)
	status, response := performJSONRequest(t, env.engine, http.MethodPost, "/api/v3/contents/generations/tasks", "Bearer e2e", body)
	require.Equal(t, http.StatusOK, status, string(response))
	require.Len(t, env.channelA.snapshot(), 1)
	require.Len(t, env.channelB.snapshot(), 1)

	var task model.Task
	require.NoError(t, model.DB.Order("id DESC").First(&task).Error)
	assert.Equal(t, capabilityChannelB, task.ChannelId)
	require.NotNil(t, task.PrivateData.Routing)
	assert.True(t, task.PrivateData.Routing.Facts.RequireRealPerson)
	assert.Equal(t, upstreamUpscaled1080, task.PrivateData.Routing.UpstreamModel)
}

func TestGroupRoutingRequirementsAutoSkipsIncompatibleGroupE2E(t *testing.T) {
	env := setupSeedanceCapabilityRoutingE2E(t)
	setGroupRoutingRequirementsE2E(t, `{"分组A":{"require_real_person":true}}`)

	originalAutoGroups := setting.AutoGroups2JsonString()
	originalUsableGroups := setting.UserUsableGroups2JSONString()
	originalGroupRatios := ratio_setting.GroupRatio2JSONString()
	originalSpecialGroups := ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup.ReadAll()
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateAutoGroupsByJsonString(originalAutoGroups))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalUsableGroups))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalGroupRatios))
		specialGroups := ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup
		specialGroups.Clear()
		specialGroups.AddAll(originalSpecialGroups)
	})
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["分组A","分组B"]`))
	usableGroups := setting.GetUserUsableGroupsCopy()
	usableGroups["auto"] = "auto"
	encodedUsableGroups, err := common.Marshal(usableGroups)
	require.NoError(t, err)
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(string(encodedUsableGroups)))
	groups := ratio_setting.GetGroupRatioCopy()
	groups[groupRoutingFallbackGroup] = 1
	encodedGroups, err := common.Marshal(groups)
	require.NoError(t, err)
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(string(encodedGroups)))
	specialGroups := ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup
	specialGroups.Set(capabilityGroup, map[string]string{groupRoutingFallbackGroup: "fallback"})

	// Make the first auto group have only a non-real-person target, so its group
	// requirement rejects it and the selector must continue with group B.
	groupARequest := env.standardRequest
	groupARequest.Defaults = modelrouting.Defaults{OutputResolution: "1080p", DurationSeconds: 15, AspectRatio: "9:16"}
	groupARequest.Targets = []service.RouteTargetWriteRequest{
		capabilityTarget(capabilityChannelA, upstreamStandard1080, 100, []string{"1080p"}, discreteDuration(15), []string{"9:16"}, modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3}, false),
	}
	_, err = service.SaveRoutingPolicy(env.standardPolicy, groupARequest)
	require.NoError(t, err)

	upstream := &capabilityRecordingServer{}
	upstreamServer := httptest.NewServer(upstream)
	t.Cleanup(upstreamServer.Close)
	secondChannel := &model.Channel{
		Id: 3, Type: constant.ChannelTypeNewAPIVideo, Key: groupRoutingTokenKey,
		Status: common.ChannelStatusEnabled, Name: "group-routing-fallback", BaseURL: common.GetPointer(upstreamServer.URL),
		Models: strings.Join(modelrouting.CanonicalModels, ","), Group: groupRoutingFallbackGroup,
		Weight: common.GetPointer[uint](1), Priority: common.GetPointer[int64](100), CreatedTime: time.Now().Unix(), OtherSettings: "{}",
	}
	secondChannel.SetOtherSettings(dto.ChannelOtherSettings{DisableTaskPollingSleep: true})
	require.NoError(t, secondChannel.Insert())
	_, err = service.SaveRoutingPolicy(0, service.RoutingPolicyWriteRequest{
		GroupName: groupRoutingFallbackGroup, Model: modelrouting.Seedance20, Enabled: true,
		Defaults: modelrouting.Defaults{OutputResolution: "1080p", DurationSeconds: 15, AspectRatio: "9:16"},
		Targets: []service.RouteTargetWriteRequest{
			capabilityTarget(3, groupRoutingFallbackModel, 100, []string{"1080p"}, discreteDuration(15), []string{"9:16"}, modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3}, false),
		},
	})
	require.NoError(t, err)
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", 1).Update("group", "auto").Error)

	body := capabilityRequestBody(t, modelrouting.Seedance20, "1080p", 15, "9:16", modelrouting.ReferenceLimits{}, false)
	status, response := performJSONRequest(t, env.engine, http.MethodPost, "/api/v3/contents/generations/tasks", "Bearer e2e", body)
	require.Equal(t, http.StatusOK, status, string(response))
	assert.Empty(t, env.channelA.snapshot())
	assert.Empty(t, env.channelB.snapshot())
	require.Len(t, upstream.snapshot(), 1)

	var task model.Task
	require.NoError(t, model.DB.Order("id DESC").First(&task).Error)
	assert.Equal(t, 3, task.ChannelId)
	require.NotNil(t, task.PrivateData.Routing)
	assert.Equal(t, groupRoutingFallbackGroup, task.PrivateData.Routing.Facts.GroupName)
	assert.Equal(t, groupRoutingFallbackModel, task.PrivateData.Routing.UpstreamModel)

	var submitted map[string]any
	require.NoError(t, common.Unmarshal(upstream.snapshot()[0].Body, &submitted))
	assert.Equal(t, groupRoutingFallbackModel, submitted["model"])
}
