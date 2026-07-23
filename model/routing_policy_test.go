package model_test

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelrouting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestReplaceRoutingPolicyPersistsTypedConstraints(t *testing.T) {
	db := openRoutingTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.RoutingPolicy{}, &model.RouteTarget{}))
	policy := validRoutingPolicyRow()
	targets := []model.RouteTarget{{
		ChannelID:      11,
		Name:           "A1 720 fast",
		UpstreamModel:  "bb-seedance2.0-720p-fast-gz-15s",
		TargetPriority: 100,
		Enabled:        true,
		Constraints:    validConstraintsJSON(t, modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3}),
	}}

	created, err := model.ReplaceRoutingPolicy(0, policy, targets)
	require.NoError(t, err)
	require.NotZero(t, created.ID)
	require.Len(t, created.Targets, 1)

	loaded, err := model.GetRoutingPolicy(created.ID)
	require.NoError(t, err)
	assert.Equal(t, "bb-seedance2.0-720p-fast-gz-15s", loaded.Targets[0].UpstreamModel)
	assert.JSONEq(t, targets[0].Constraints, loaded.Targets[0].Constraints)
}

func TestRoutingPolicyUniqueGroupAndModel(t *testing.T) {
	db := openRoutingTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.RoutingPolicy{}, &model.RouteTarget{}))
	policy := validRoutingPolicyRow()
	policy.Enabled = false

	_, err := model.ReplaceRoutingPolicy(0, policy, nil)
	require.NoError(t, err)
	_, err = model.ReplaceRoutingPolicy(0, policy, nil)
	require.Error(t, err)
}

func TestReplaceRoutingPolicyRollsBackBeforeInvalidTargetReplacement(t *testing.T) {
	db := openRoutingTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.RoutingPolicy{}, &model.RouteTarget{}))
	created, err := model.ReplaceRoutingPolicy(0, validRoutingPolicyRow(), []model.RouteTarget{{
		ChannelID:      11,
		Name:           "original",
		UpstreamModel:  "provider-original",
		TargetPriority: 100,
		Enabled:        true,
		Constraints:    validConstraintsJSON(t, modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3}),
	}})
	require.NoError(t, err)

	_, err = model.ReplaceRoutingPolicy(created.ID, validRoutingPolicyRow(), []model.RouteTarget{
		{
			ChannelID:      11,
			Name:           "replacement",
			UpstreamModel:  "provider-replacement",
			TargetPriority: 100,
			Enabled:        true,
			Constraints:    validConstraintsJSON(t, modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3}),
		},
		{
			ChannelID:      12,
			Name:           "broken",
			UpstreamModel:  "provider-broken",
			TargetPriority: 100,
			Enabled:        true,
			Constraints:    `{broken`,
		},
	})
	require.Error(t, err)

	loaded, err := model.GetRoutingPolicy(created.ID)
	require.NoError(t, err)
	require.Len(t, loaded.Targets, 1)
	assert.Equal(t, "original", loaded.Targets[0].Name)
	assert.Equal(t, "provider-original", loaded.Targets[0].UpstreamModel)
}

func TestDeleteRoutingPolicyExplicitlyRemovesTargets(t *testing.T) {
	db := openRoutingTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.RoutingPolicy{}, &model.RouteTarget{}))
	created, err := model.ReplaceRoutingPolicy(0, validRoutingPolicyRow(), []model.RouteTarget{{
		ChannelID:      11,
		Name:           "target",
		UpstreamModel:  "provider-model",
		TargetPriority: 100,
		Enabled:        true,
		Constraints:    validConstraintsJSON(t, modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3}),
	}})
	require.NoError(t, err)

	require.NoError(t, model.DeleteRoutingPolicy(created.ID))
	var policyCount int64
	var targetCount int64
	require.NoError(t, db.Model(&model.RoutingPolicy{}).Where("id = ?", created.ID).Count(&policyCount).Error)
	require.NoError(t, db.Model(&model.RouteTarget{}).Where("policy_id = ?", created.ID).Count(&targetCount).Error)
	assert.Zero(t, policyCount)
	assert.Zero(t, targetCount)
}

func TestListRoutingCandidatesUsesExactAbilityAndOmitsSecrets(t *testing.T) {
	db := openRoutingTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.RoutingPolicy{}, &model.RouteTarget{}))
	priorityHigh := int64(100)
	priorityLow := int64(50)
	weightHigh := uint(20)
	weightLow := uint(10)
	channels := []model.Channel{
		{Id: 11, Name: "A1", Key: "secret-a1", Status: common.ChannelStatusEnabled, Priority: &priorityHigh, Weight: &weightHigh},
		{Id: 12, Name: "A1_copy", Key: "secret-a1-copy", Status: common.ChannelStatusManuallyDisabled, Priority: &priorityLow, Weight: &weightLow},
		{Id: 13, Name: "other", Key: "secret-other", Status: common.ChannelStatusEnabled, Priority: &priorityHigh, Weight: &weightHigh},
	}
	require.NoError(t, db.Create(&channels).Error)
	abilities := []model.Ability{
		{Group: "分组A", Model: modelrouting.Seedance20, ChannelId: 11, Enabled: true, Priority: &priorityHigh, Weight: weightHigh},
		{Group: "分组A", Model: modelrouting.Seedance20, ChannelId: 12, Enabled: false, Priority: &priorityLow, Weight: weightLow},
		{Group: "分组B", Model: modelrouting.Seedance20, ChannelId: 13, Enabled: true, Priority: &priorityHigh, Weight: weightHigh},
		{Group: "分组A", Model: modelrouting.Seedance20Fast, ChannelId: 13, Enabled: true, Priority: &priorityHigh, Weight: weightHigh},
	}
	require.NoError(t, db.Create(&abilities).Error)

	candidates, err := model.ListRoutingCandidates("分组A", modelrouting.Seedance20)
	require.NoError(t, err)
	require.Len(t, candidates, 2)
	assert.ElementsMatch(t, []int{11, 12}, []int{candidates[0].ID, candidates[1].ID})
	encoded, err := common.Marshal(candidates)
	require.NoError(t, err)
	assert.NotContains(t, strings.ToLower(string(encoded)), "secret")
	assert.NotContains(t, strings.ToLower(string(encoded)), `"key"`)
}

func openRoutingTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	previousDB := model.DB
	model.DB = db
	t.Cleanup(func() {
		model.DB = previousDB
		require.NoError(t, sqlDB.Close())
	})
	return db
}

func validRoutingPolicyRow() model.RoutingPolicy {
	return model.RoutingPolicy{
		GroupName:         "分组A",
		Model:             modelrouting.Seedance20,
		Enabled:           true,
		DefaultResolution: "720p",
		DefaultDuration:   10,
		DefaultRatio:      "16:9",
	}
}

func validConstraintsJSON(t *testing.T, limits modelrouting.ReferenceLimits) string {
	t.Helper()
	supportsRealPerson := true
	encoded, err := common.Marshal(modelrouting.Constraints{
		OutputResolutions:  []string{"720p"},
		Durations:          modelrouting.DurationConstraint{Min: routingIntPtr(4), Max: routingIntPtr(15)},
		AspectRatios:       []string{"16:9", "9:16"},
		ReferenceLimits:    limits,
		SupportsRealPerson: &supportsRealPerson,
	})
	require.NoError(t, err)
	return string(encoded)
}

func routingIntPtr(value int) *int {
	return &value
}
