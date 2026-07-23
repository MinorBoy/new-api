package model_test

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelrouting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestInitRoutingPolicyCacheLoadsOnlyEnabledPoliciesAndTargets(t *testing.T) {
	db := openRoutingTestDB(t)
	prepareRoutingCacheTest(t, db)
	standard := createCachedPolicy(t, modelrouting.Seedance20, true, 11, "provider-standard", true)
	createCachedPolicy(t, modelrouting.Seedance20Fast, false, 12, "provider-fast", true)
	require.NoError(t, db.Create(&model.RouteTarget{
		PolicyID:       standard.ID,
		ChannelID:      13,
		Name:           "disabled target",
		UpstreamModel:  "provider-disabled",
		TargetPriority: 90,
		Enabled:        false,
		Constraints:    validConstraintsJSON(t, modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3}),
	}).Error)

	require.NoError(t, model.InitRoutingPolicyCache())
	snapshot, ok := model.GetRoutingPolicySnapshot("分组A", modelrouting.Seedance20)
	require.True(t, ok)
	require.Len(t, snapshot.TargetsByChannel, 1)
	require.Len(t, snapshot.TargetsByChannel[11], 1)
	assert.Equal(t, "provider-standard", snapshot.TargetsByChannel[11][0].UpstreamModel)
	_, ok = model.GetRoutingPolicySnapshot("分组A", modelrouting.Seedance20Fast)
	assert.False(t, ok)
}

func TestRefreshRoutingPolicyCacheReplacesOnlyOneKey(t *testing.T) {
	db := openRoutingTestDB(t)
	prepareRoutingCacheTest(t, db)
	standard := createCachedPolicy(t, modelrouting.Seedance20, true, 11, "provider-standard", true)
	createCachedPolicy(t, modelrouting.Seedance20Fast, true, 12, "provider-fast", true)
	require.NoError(t, model.InitRoutingPolicyCache())
	beforeStandard, ok := model.GetRoutingPolicySnapshot("分组A", modelrouting.Seedance20)
	require.True(t, ok)
	beforeFast, ok := model.GetRoutingPolicySnapshot("分组A", modelrouting.Seedance20Fast)
	require.True(t, ok)

	require.NoError(t, db.Model(&model.RouteTarget{}).
		Where("policy_id = ?", standard.ID).
		Update("upstream_model", "provider-standard-v2").Error)
	require.NoError(t, model.RefreshRoutingPolicyCache("分组A", modelrouting.Seedance20))

	afterStandard, ok := model.GetRoutingPolicySnapshot("分组A", modelrouting.Seedance20)
	require.True(t, ok)
	afterFast, ok := model.GetRoutingPolicySnapshot("分组A", modelrouting.Seedance20Fast)
	require.True(t, ok)
	assert.Equal(t, "provider-standard", beforeStandard.TargetsByChannel[11][0].UpstreamModel)
	assert.Equal(t, "provider-standard-v2", afterStandard.TargetsByChannel[11][0].UpstreamModel)
	assert.Equal(t, beforeFast, afterFast)
}

func TestRefreshRoutingPolicyCacheKeepsPreviousSnapshotOnDecodeFailure(t *testing.T) {
	db := openRoutingTestDB(t)
	prepareRoutingCacheTest(t, db)
	policy := createCachedPolicy(t, modelrouting.Seedance20, true, 11, "provider-standard", true)
	require.NoError(t, model.InitRoutingPolicyCache())
	before, ok := model.GetRoutingPolicySnapshot("分组A", modelrouting.Seedance20)
	require.True(t, ok)

	require.NoError(t, db.Model(&model.RouteTarget{}).
		Where("policy_id = ?", policy.ID).
		Update("constraints", `{broken`).Error)
	err := model.RefreshRoutingPolicyCache("分组A", modelrouting.Seedance20)
	require.Error(t, err)
	after, ok := model.GetRoutingPolicySnapshot("分组A", modelrouting.Seedance20)
	require.True(t, ok)
	assert.Equal(t, before, after)
}

func TestRoutingPolicyCacheLoadsLegacyUpscaleProperties(t *testing.T) {
	db := openRoutingTestDB(t)
	prepareRoutingCacheTest(t, db)
	policy := createCachedPolicy(t, modelrouting.Seedance20, true, 11, "provider-1080p", true)
	legacy := `{
		"output_resolutions":["1080p"],
		"generation_resolution":"720p",
		"upscaled":true,
		"durations":{"min":4,"max":15},
		"aspect_ratios":["16:9"],
		"reference_limits":{"images":4,"videos":3,"audios":1},
		"supports_real_person":true
	}`
	require.NoError(t, db.Model(&model.RouteTarget{}).
		Where("policy_id = ?", policy.ID).
		Update("constraints", legacy).Error)
	require.NoError(t, db.Model(&model.RoutingPolicy{}).
		Where("id = ?", policy.ID).
		Update("default_resolution", "1080p").Error)

	require.NoError(t, model.InitRoutingPolicyCache())
	snapshot, ok := model.GetRoutingPolicySnapshot("分组A", modelrouting.Seedance20)
	require.True(t, ok)

	matched := modelrouting.Evaluate(snapshot, matchingFactsForResolution("1080p"))
	assert.Contains(t, matched.CompatibleByChannel, 11)

	notMatched := modelrouting.Evaluate(snapshot, matchingFactsForResolution("720p"))
	assert.NotContains(t, notMatched.CompatibleByChannel, 11)
}

func matchingFactsForResolution(resolution string) modelrouting.Facts {
	return modelrouting.Facts{
		OutputResolution: resolution,
		DurationSeconds:  10,
		AspectRatio:      "16:9",
	}
}

func TestRefreshRoutingPolicyCacheRemovesDisabledPolicy(t *testing.T) {
	db := openRoutingTestDB(t)
	prepareRoutingCacheTest(t, db)
	policy := createCachedPolicy(t, modelrouting.Seedance20, true, 11, "provider-standard", true)
	require.NoError(t, model.InitRoutingPolicyCache())
	_, ok := model.GetRoutingPolicySnapshot("分组A", modelrouting.Seedance20)
	require.True(t, ok)

	require.NoError(t, db.Model(&model.RoutingPolicy{}).Where("id = ?", policy.ID).Update("enabled", false).Error)
	require.NoError(t, model.RefreshRoutingPolicyCache("分组A", modelrouting.Seedance20))
	_, ok = model.GetRoutingPolicySnapshot("分组A", modelrouting.Seedance20)
	assert.False(t, ok)
}

func prepareRoutingCacheTest(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.AutoMigrate(&model.RoutingPolicy{}, &model.RouteTarget{}))
	t.Cleanup(func() {
		require.NoError(t, db.Exec("DELETE FROM route_targets").Error)
		require.NoError(t, db.Exec("DELETE FROM routing_policies").Error)
		require.NoError(t, model.InitRoutingPolicyCache())
	})
}

func createCachedPolicy(t *testing.T, canonicalModel string, policyEnabled bool, channelID int, upstreamModel string, targetEnabled bool) *model.RoutingPolicy {
	t.Helper()
	policy := validRoutingPolicyRow()
	policy.Model = canonicalModel
	policy.Enabled = policyEnabled
	targets := []model.RouteTarget{{
		ChannelID:      channelID,
		Name:           upstreamModel,
		UpstreamModel:  upstreamModel,
		TargetPriority: 100,
		Enabled:        targetEnabled,
		Constraints:    validConstraintsJSON(t, modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3}),
	}}
	created, err := model.ReplaceRoutingPolicy(0, policy, targets)
	require.NoError(t, err)
	return created
}
