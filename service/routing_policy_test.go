package service_test

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelrouting"
	"github.com/QuantumNous/new-api/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSaveRoutingPolicyNormalizesAndPublishesCompleteReplacement(t *testing.T) {
	prepareRoutingPolicyServiceTest(t)
	seedRoutingCandidate(t, 11, "A1", "分组A", modelrouting.Seedance20, true)
	request := validRoutingPolicyWriteRequest()
	request.Defaults.OutputResolution = " 720P "
	request.Targets[0].Name = " A1 fast "
	request.Targets[0].UpstreamModel = " bb-seedance2.0-720p-fast-gz-15s "
	request.Targets[0].Constraints.OutputResolutions = []string{"720P", "720p"}
	request.Targets[0].Constraints.Durations = modelrouting.DurationConstraint{Values: []int{15, 10, 10}}
	request.Targets[0].Constraints.AspectRatios = []string{"9:16", "16:9", "16:9"}

	saved, err := service.SaveRoutingPolicy(0, request)
	require.NoError(t, err)
	require.NotZero(t, saved.ID)
	require.Len(t, saved.Targets, 1)
	assert.Equal(t, "720p", saved.Defaults.OutputResolution)
	assert.Equal(t, "A1 fast", saved.Targets[0].Name)
	assert.Equal(t, "bb-seedance2.0-720p-fast-gz-15s", saved.Targets[0].UpstreamModel)
	assert.Equal(t, []string{"720p"}, saved.Targets[0].Constraints.OutputResolutions)
	assert.Equal(t, []int{10, 15}, saved.Targets[0].Constraints.Durations.Values)
	assert.Equal(t, []string{"16:9", "9:16"}, saved.Targets[0].Constraints.AspectRatios)

	snapshot, ok := model.GetRoutingPolicySnapshot("分组A", modelrouting.Seedance20)
	require.True(t, ok)
	assert.Equal(t, saved.ID, snapshot.ID)
	assert.Equal(t, saved.Targets[0].UpstreamModel, snapshot.TargetsByChannel[11][0].UpstreamModel)
}

func TestSaveRoutingPolicyRejectsAutoGroup(t *testing.T) {
	prepareRoutingPolicyServiceTest(t)
	request := validRoutingPolicyWriteRequest()
	request.GroupName = "auto"

	_, err := service.SaveRoutingPolicy(0, request)
	assertRoutingPolicyServiceError(t, err, "invalid_group", nil)
}

func TestSaveRoutingPolicyRejectsNonCandidateChannel(t *testing.T) {
	prepareRoutingPolicyServiceTest(t)
	seedRoutingCandidate(t, 11, "A1", "分组A", modelrouting.Seedance20Fast, true)
	request := validRoutingPolicyWriteRequest()

	_, err := service.SaveRoutingPolicy(0, request)
	assertRoutingPolicyServiceError(t, err, "invalid_channel", nil)
}

func TestSaveRoutingPolicyReturnsOverlappingTargetIndexes(t *testing.T) {
	prepareRoutingPolicyServiceTest(t)
	seedRoutingCandidate(t, 11, "A1", "分组A", modelrouting.Seedance20, true)
	request := validRoutingPolicyWriteRequest()
	second := request.Targets[0]
	second.Name = "second"
	second.UpstreamModel = "provider-second"
	request.Targets = append(request.Targets, second)

	_, err := service.SaveRoutingPolicy(0, request)
	assertRoutingPolicyServiceError(t, err, "routing_target_overlap", []int{0, 1})
	var serviceErr *service.RoutingPolicyServiceError
	require.ErrorAs(t, err, &serviceErr)
	assert.Equal(t, "targets overlap at the same channel priority", serviceErr.Error())
}

func TestSaveRoutingPolicyRequiresEnabledTarget(t *testing.T) {
	prepareRoutingPolicyServiceTest(t)
	request := validRoutingPolicyWriteRequest()
	request.Targets = nil

	_, err := service.SaveRoutingPolicy(0, request)
	assertRoutingPolicyServiceError(t, err, "default_route_unavailable", nil)
}

func TestSetRoutingPolicyStatusAllowsExistingChannelAfterAbilityRemoval(t *testing.T) {
	prepareRoutingPolicyServiceTest(t)
	seedRoutingCandidate(t, 11, "A1", "分组A", modelrouting.Seedance20, true)
	created, err := service.SaveRoutingPolicy(0, validRoutingPolicyWriteRequest())
	require.NoError(t, err)
	require.NoError(t, model.DB.Where("channel_id = ?", 11).Delete(&model.Ability{}).Error)

	updated, err := service.SetRoutingPolicyStatus(created.ID, false)
	require.NoError(t, err)
	assert.False(t, updated.Enabled)
	require.Len(t, updated.Targets, 1)
	assert.Equal(t, 11, updated.Targets[0].ChannelID)
}

func prepareRoutingPolicyServiceTest(t *testing.T) {
	t.Helper()
	require.NoError(t, model.DB.AutoMigrate(&model.Ability{}, &model.RoutingPolicy{}, &model.RouteTarget{}))
	for _, table := range []string{"route_targets", "routing_policies", "abilities", "channels"} {
		require.NoError(t, model.DB.Exec("DELETE FROM "+table).Error)
	}
	require.NoError(t, model.InitRoutingPolicyCache())
	t.Cleanup(func() {
		for _, table := range []string{"route_targets", "routing_policies", "abilities", "channels"} {
			require.NoError(t, model.DB.Exec("DELETE FROM "+table).Error)
		}
		require.NoError(t, model.InitRoutingPolicyCache())
	})
}

func seedRoutingCandidate(t *testing.T, channelID int, name, groupName, canonicalModel string, enabled bool) {
	t.Helper()
	priority := int64(100)
	weight := uint(10)
	status := common.ChannelStatusManuallyDisabled
	if enabled {
		status = common.ChannelStatusEnabled
	}
	require.NoError(t, model.DB.Create(&model.Channel{
		Id: channelID, Name: name, Key: "secret", Status: status, Priority: &priority, Weight: &weight,
	}).Error)
	require.NoError(t, model.DB.Create(&model.Ability{
		Group: groupName, Model: canonicalModel, ChannelId: channelID, Enabled: enabled, Priority: &priority, Weight: weight,
	}).Error)
}

func validRoutingPolicyWriteRequest() service.RoutingPolicyWriteRequest {
	supportsRealPerson := true
	return service.RoutingPolicyWriteRequest{
		GroupName: "分组A",
		Model:     modelrouting.Seedance20,
		Enabled:   true,
		Defaults: modelrouting.Defaults{
			OutputResolution: "720p",
			DurationSeconds:  10,
			AspectRatio:      "16:9",
		},
		Targets: []service.RouteTargetWriteRequest{{
			ChannelID:      11,
			Name:           "A1 standard",
			UpstreamModel:  "provider-standard",
			TargetPriority: 100,
			Enabled:        true,
			Constraints: modelrouting.Constraints{
				OutputResolutions:  []string{"720p"},
				Durations:          modelrouting.DurationConstraint{Min: serviceIntPtr(4), Max: serviceIntPtr(15)},
				AspectRatios:       []string{"16:9", "9:16"},
				ReferenceLimits:    modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3},
				SupportsRealPerson: &supportsRealPerson,
			},
		}},
	}
}

func assertRoutingPolicyServiceError(t *testing.T, err error, code string, targetIndexes []int) {
	t.Helper()
	var serviceErr *service.RoutingPolicyServiceError
	require.ErrorAs(t, err, &serviceErr)
	assert.Equal(t, code, serviceErr.Code)
	assert.Equal(t, targetIndexes, serviceErr.TargetIndexes)
}

func serviceIntPtr(value int) *int {
	return &value
}
