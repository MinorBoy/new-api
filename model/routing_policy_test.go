package model_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelrouting"
	"github.com/QuantumNous/new-api/types"
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

func TestReplaceRoutingPolicyPreservesMinimumExpectedMarginNullAndZero(t *testing.T) {
	db := openRoutingTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.RoutingPolicy{}, &model.RouteTarget{}))
	zero := 0
	targets := []model.RouteTarget{
		{
			ChannelID: 11, Name: "inherit", UpstreamModel: "provider-inherit", TargetPriority: 100,
			Enabled: true, Constraints: validConstraintsJSON(t, modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3}),
		},
		{
			ChannelID: 12, Name: "zero", UpstreamModel: "provider-zero", TargetPriority: 100,
			Enabled: true, MinimumExpectedMarginBPS: &zero,
			Constraints: validConstraintsJSON(t, modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3}),
		},
	}

	created, err := model.ReplaceRoutingPolicy(0, validRoutingPolicyRow(), targets)
	require.NoError(t, err)
	require.Len(t, created.Targets, 2)
	assert.Nil(t, created.Targets[0].MinimumExpectedMarginBPS)
	require.NotNil(t, created.Targets[1].MinimumExpectedMarginBPS)
	assert.Zero(t, *created.Targets[1].MinimumExpectedMarginBPS)
}

func TestReplaceRoutingPolicyPreservesImportedTargetOwnershipByID(t *testing.T) {
	db := openRoutingTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.RoutingPolicy{}, &model.RouteTarget{}))
	created, err := model.ReplaceRoutingPolicy(0, validRoutingPolicyRow(), []model.RouteTarget{{
		ChannelID:      11,
		Name:           "imported",
		UpstreamModel:  "provider-original",
		TargetPriority: 100,
		Enabled:        true,
		Constraints:    validConstraintsJSON(t, modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3}),
	}})
	require.NoError(t, err)
	require.Len(t, created.Targets, 1)
	batchID := int64(20)
	require.NoError(t, db.Model(&model.RouteTarget{}).Where("id = ?", created.Targets[0].ID).Updates(map[string]any{
		"managed_by":      string(types.RouteTargetManagedByConfigImport),
		"source_batch_id": batchID,
	}).Error)

	target := created.Targets[0]
	target.Name = "updated"
	updated, err := model.ReplaceRoutingPolicy(created.ID, validRoutingPolicyRow(), []model.RouteTarget{target})
	require.NoError(t, err)
	require.Len(t, updated.Targets, 1)
	assert.Equal(t, created.Targets[0].ID, updated.Targets[0].ID)
	assert.Equal(t, string(types.RouteTargetManagedByConfigImport), updated.Targets[0].ManagedBy)
	require.NotNil(t, updated.Targets[0].SourceBatchID)
	assert.Equal(t, batchID, *updated.Targets[0].SourceBatchID)
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

func TestListEnabledRoutingPoliciesByGroupLoadsAllTargetsInStableOrder(t *testing.T) {
	db := openRoutingTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.RoutingPolicy{}, &model.RouteTarget{}))
	policies := []model.RoutingPolicy{
		{ID: 30, GroupName: "default", Model: modelrouting.Seedance20Fast, Enabled: true, DefaultResolution: "720p", DefaultDuration: 10, DefaultRatio: "16:9"},
		{ID: 10, GroupName: "default", Model: modelrouting.Seedance20, Enabled: true, DefaultResolution: "720p", DefaultDuration: 10, DefaultRatio: "16:9"},
		{ID: 20, GroupName: "default", Model: modelrouting.Seedance20Mini, Enabled: false, DefaultResolution: "720p", DefaultDuration: 10, DefaultRatio: "16:9"},
		{ID: 40, GroupName: "other", Model: modelrouting.Seedance20, Enabled: true, DefaultResolution: "720p", DefaultDuration: 10, DefaultRatio: "16:9"},
	}
	require.NoError(t, db.Create(&policies).Error)
	targets := []model.RouteTarget{
		{ID: 4, PolicyID: 10, ChannelID: 12, Name: "later-channel", UpstreamModel: "model-d", CostVariantKey: "default", TargetPriority: 100, Enabled: true, Constraints: "{}"},
		{ID: 3, PolicyID: 10, ChannelID: 11, Name: "lower-priority", UpstreamModel: "model-c", CostVariantKey: "default", TargetPriority: 50, Enabled: false, Constraints: "{}"},
		{ID: 2, PolicyID: 10, ChannelID: 11, Name: "id-tie", UpstreamModel: "model-b", CostVariantKey: "default", TargetPriority: 100, Enabled: true, Constraints: "{}"},
		{ID: 1, PolicyID: 10, ChannelID: 11, Name: "first", UpstreamModel: "model-a", CostVariantKey: "default", TargetPriority: 100, Enabled: true, Constraints: "{}"},
		{ID: 5, PolicyID: 30, ChannelID: 13, Name: "fast", UpstreamModel: "model-fast", CostVariantKey: "default", TargetPriority: 100, Enabled: true, Constraints: "{}"},
		{ID: 6, PolicyID: 20, ChannelID: 14, Name: "disabled-policy", UpstreamModel: "model-mini", CostVariantKey: "default", TargetPriority: 100, Enabled: true, Constraints: "{}"},
	}
	require.NoError(t, db.Create(&targets).Error)
	queryCount := 0
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register("count_routing_policy_queries", func(*gorm.DB) {
		queryCount++
	}))

	loaded, err := model.ListEnabledRoutingPoliciesByGroup("default")
	require.NoError(t, err)
	assert.Equal(t, 2, queryCount)
	require.Len(t, loaded, 2)
	assert.Equal(t, []string{modelrouting.Seedance20, modelrouting.Seedance20Fast}, []string{loaded[0].Model, loaded[1].Model})
	require.Len(t, loaded[0].Targets, 4)
	assert.Equal(t, []int{1, 2, 3, 4}, []int{loaded[0].Targets[0].ID, loaded[0].Targets[1].ID, loaded[0].Targets[2].ID, loaded[0].Targets[3].ID})
	assert.False(t, loaded[0].Targets[2].Enabled)
}

func TestGroupRoutingAvailabilityUsesEnabledSourceAbilitiesAndChannels(t *testing.T) {
	for _, memoryCacheEnabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("memory_cache_%t", memoryCacheEnabled), func(t *testing.T) {
			db := openRoutingTestDB(t)
			require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}))
			previousMemoryCacheEnabled := common.MemoryCacheEnabled
			common.MemoryCacheEnabled = memoryCacheEnabled
			t.Cleanup(func() { common.MemoryCacheEnabled = previousMemoryCacheEnabled })
			priority := int64(100)
			weight := uint(10)
			channels := []model.Channel{
				{Id: 11, Name: "enabled", Key: "secret-enabled", Status: common.ChannelStatusEnabled, Group: "default", Models: modelrouting.Seedance20, Priority: &priority, Weight: &weight},
				{Id: 12, Name: "disabled", Key: "secret-disabled", Status: common.ChannelStatusManuallyDisabled, Group: "default", Models: modelrouting.Seedance20, Priority: &priority, Weight: &weight},
				{Id: 14, Name: "other-group", Key: "secret-other", Status: common.ChannelStatusEnabled, Group: "other", Models: modelrouting.Seedance20, Priority: &priority, Weight: &weight},
				{Id: 15, Name: "disabled-ability", Key: "secret-disabled-ability", Status: common.ChannelStatusEnabled, Group: "default", Models: modelrouting.Seedance20, Priority: &priority, Weight: &weight},
				{Id: 16, Name: "missing-ability", Key: "secret-missing-ability", Status: common.ChannelStatusEnabled, Group: "default", Models: modelrouting.Seedance20, Priority: &priority, Weight: &weight},
			}
			require.NoError(t, db.Create(&channels).Error)
			abilities := []model.Ability{
				{Group: "default", Model: modelrouting.Seedance20, ChannelId: 11, Enabled: true, Priority: &priority, Weight: weight},
				{Group: "default", Model: modelrouting.Seedance20, ChannelId: 12, Enabled: false, Priority: &priority, Weight: weight},
				{Group: "default", Model: modelrouting.Seedance20, ChannelId: 13, Enabled: true, Priority: &priority, Weight: weight},
				{Group: "other", Model: modelrouting.Seedance20, ChannelId: 14, Enabled: true, Priority: &priority, Weight: weight},
				{Group: "default", Model: modelrouting.Seedance20, ChannelId: 15, Enabled: false, Priority: &priority, Weight: weight},
			}
			require.NoError(t, db.Create(&abilities).Error)
			if memoryCacheEnabled {
				model.InitChannelCache()
			}
			queryCount := 0
			require.NoError(t, db.Callback().Query().Before("gorm:query").Register("count_routing_availability_queries", func(*gorm.DB) {
				queryCount++
			}))

			available, err := model.ListRoutingAvailability("default", []string{" " + modelrouting.Seedance20 + " ", modelrouting.Seedance20})
			require.NoError(t, err)
			if memoryCacheEnabled {
				assert.Zero(t, queryCount)
			} else {
				assert.Equal(t, 2, queryCount)
			}
			assert.Equal(t, map[model.RoutingAvailabilityKey]struct{}{
				{CanonicalModel: modelrouting.Seedance20, ChannelID: 11}: {},
			}, available)
		})
	}
}

func TestGroupRoutingAvailabilityRecoversImmediatelyAfterChannelReenable(t *testing.T) {
	db := openRoutingTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}))
	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = true
	t.Cleanup(func() { common.MemoryCacheEnabled = previousMemoryCacheEnabled })
	priority := int64(100)
	weight := uint(10)
	require.NoError(t, db.Create(&model.Channel{
		Id: 11, Name: "recoverable", Key: "secret", Status: common.ChannelStatusEnabled,
		Group: "channel-only-group", Models: "channel-only-model", Priority: &priority, Weight: &weight,
	}).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group: "default", Model: modelrouting.Seedance20, ChannelId: 11,
		Enabled: true, Priority: &priority, Weight: weight,
	}).Error)
	model.InitChannelCache()
	expected := map[model.RoutingAvailabilityKey]struct{}{
		{CanonicalModel: modelrouting.Seedance20, ChannelID: 11}: {},
	}

	available, err := model.ListRoutingAvailability("default", []string{modelrouting.Seedance20})
	require.NoError(t, err)
	assert.Equal(t, expected, available)

	require.True(t, model.UpdateChannelStatus(11, "", common.ChannelStatusAutoDisabled, "test disable"))
	available, err = model.ListRoutingAvailability("default", []string{modelrouting.Seedance20})
	require.NoError(t, err)
	assert.Empty(t, available)

	require.True(t, model.UpdateChannelStatus(11, "", common.ChannelStatusEnabled, ""))
	available, err = model.ListRoutingAvailability("default", []string{modelrouting.Seedance20})
	require.NoError(t, err)
	assert.Equal(t, expected, available)
	var ability model.Ability
	require.NoError(t, db.First(&ability, "channel_id = ?", 11).Error)
	assert.True(t, ability.Enabled)
}

func TestGroupRoutingAvailabilityDoesNotRecoverWhenAbilityEnableFails(t *testing.T) {
	db := openRoutingTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}))
	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = true
	t.Cleanup(func() { common.MemoryCacheEnabled = previousMemoryCacheEnabled })
	priority := int64(100)
	weight := uint(10)
	require.NoError(t, db.Create(&model.Channel{
		Id: 11, Name: "ability-failure", Key: "secret", Status: common.ChannelStatusEnabled,
		Priority: &priority, Weight: &weight,
	}).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group: "default", Model: modelrouting.Seedance20, ChannelId: 11,
		Enabled: true, Priority: &priority, Weight: weight,
	}).Error)
	model.InitChannelCache()
	require.True(t, model.UpdateChannelStatus(11, "", common.ChannelStatusAutoDisabled, "test disable"))

	forcedErr := errors.New("forced ability enable failure")
	const callbackName = "test:group-routing-ability-enable-failure"
	callbackRegistered := true
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table == "abilities" {
			tx.AddError(forcedErr)
		}
	}))
	t.Cleanup(func() {
		if callbackRegistered {
			_ = db.Callback().Update().Remove(callbackName)
		}
	})

	require.True(t, model.UpdateChannelStatus(11, "", common.ChannelStatusEnabled, ""))
	require.NoError(t, db.Callback().Update().Remove(callbackName))
	callbackRegistered = false
	available, err := model.ListRoutingAvailability("default", []string{modelrouting.Seedance20})
	require.NoError(t, err)
	assert.Empty(t, available)
	var ability model.Ability
	require.NoError(t, db.First(&ability, "channel_id = ?", 11).Error)
	assert.False(t, ability.Enabled)
}

func TestGroupRoutingAvailabilityRetriesAbilityEnableAfterTransientFailure(t *testing.T) {
	db := openRoutingTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}))
	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = true
	t.Cleanup(func() { common.MemoryCacheEnabled = previousMemoryCacheEnabled })
	priority := int64(100)
	weight := uint(10)
	require.NoError(t, db.Create(&model.Channel{
		Id: 11, Name: "ability-retry", Key: "secret", Status: common.ChannelStatusEnabled,
		Priority: &priority, Weight: &weight,
	}).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group: "default", Model: modelrouting.Seedance20, ChannelId: 11,
		Enabled: true, Priority: &priority, Weight: weight,
	}).Error)
	model.InitChannelCache()
	require.True(t, model.UpdateChannelStatus(11, "", common.ChannelStatusAutoDisabled, "test disable"))

	forcedErr := errors.New("forced transient ability enable failure")
	const callbackName = "test:group-routing-ability-enable-retry"
	callbackRegistered := true
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table == "abilities" {
			tx.AddError(forcedErr)
		}
	}))
	t.Cleanup(func() {
		if callbackRegistered {
			_ = db.Callback().Update().Remove(callbackName)
		}
	})

	require.True(t, model.UpdateChannelStatus(11, "", common.ChannelStatusEnabled, ""))
	require.NoError(t, db.Callback().Update().Remove(callbackName))
	callbackRegistered = false

	available, err := model.ListRoutingAvailability("default", []string{modelrouting.Seedance20})
	require.NoError(t, err)
	assert.Empty(t, available)

	require.True(t, model.UpdateChannelStatus(11, "", common.ChannelStatusEnabled, ""))
	available, err = model.ListRoutingAvailability("default", []string{modelrouting.Seedance20})
	require.NoError(t, err)
	assert.Equal(t, map[model.RoutingAvailabilityKey]struct{}{
		{CanonicalModel: modelrouting.Seedance20, ChannelID: 11}: {},
	}, available)
}

func TestChannelReenableDoesNotRunFullChannelCacheRefreshPerChannel(t *testing.T) {
	db := openRoutingTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}))
	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = true
	t.Cleanup(func() { common.MemoryCacheEnabled = previousMemoryCacheEnabled })
	priority := int64(100)
	weight := uint(10)
	for channelID := 11; channelID <= 12; channelID++ {
		require.NoError(t, db.Create(&model.Channel{
			Id: channelID, Name: fmt.Sprintf("cache-refresh-%d", channelID), Key: "secret",
			Status: common.ChannelStatusEnabled, Priority: &priority, Weight: &weight,
		}).Error)
		require.NoError(t, db.Create(&model.Ability{
			Group: "default", Model: modelrouting.Seedance20, ChannelId: channelID,
			Enabled: true, Priority: &priority, Weight: weight,
		}).Error)
	}
	model.InitChannelCache()
	require.True(t, model.UpdateChannelStatus(11, "", common.ChannelStatusAutoDisabled, "test disable"))
	require.True(t, model.UpdateChannelStatus(12, "", common.ChannelStatusAutoDisabled, "test disable"))

	channelQueryCount := 0
	const callbackName = "test:group-routing-channel-refresh-count"
	callbackRegistered := true
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table == "channels" {
			channelQueryCount++
		}
	}))
	t.Cleanup(func() {
		if callbackRegistered {
			_ = db.Callback().Query().Remove(callbackName)
		}
	})

	require.True(t, model.UpdateChannelStatus(11, "", common.ChannelStatusEnabled, ""))
	require.True(t, model.UpdateChannelStatus(12, "", common.ChannelStatusEnabled, ""))

	assert.Equal(t, 2, channelQueryCount)
}

func TestInitChannelCachePreservesLastValidSnapshotOnLoadFailure(t *testing.T) {
	for _, failedTable := range []string{"channels", "abilities"} {
		t.Run(failedTable, func(t *testing.T) {
			db := openRoutingTestDB(t)
			require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}))
			previousMemoryCacheEnabled := common.MemoryCacheEnabled
			common.MemoryCacheEnabled = true
			t.Cleanup(func() { common.MemoryCacheEnabled = previousMemoryCacheEnabled })
			priority := int64(100)
			weight := uint(10)
			require.NoError(t, db.Create(&model.Channel{
				Id: 11, Name: "cached", Key: "secret", Status: common.ChannelStatusEnabled,
				Priority: &priority, Weight: &weight,
			}).Error)
			require.NoError(t, db.Create(&model.Ability{
				Group: "default", Model: modelrouting.Seedance20, ChannelId: 11,
				Enabled: true, Priority: &priority, Weight: weight,
			}).Error)
			model.InitChannelCache()

			forcedErr := errors.New("forced " + failedTable + " query failure")
			callbackName := "test:group-routing-cache-refresh-" + failedTable
			callbackRegistered := true
			require.NoError(t, db.Callback().Query().Before("gorm:query").Register(callbackName, func(tx *gorm.DB) {
				if tx.Statement.Table == failedTable {
					tx.AddError(forcedErr)
				}
			}))
			t.Cleanup(func() {
				if callbackRegistered {
					_ = db.Callback().Query().Remove(callbackName)
				}
			})

			model.InitChannelCache()
			require.NoError(t, db.Callback().Query().Remove(callbackName))
			callbackRegistered = false
			available, err := model.ListRoutingAvailability("default", []string{modelrouting.Seedance20})
			require.NoError(t, err)
			assert.Equal(t, map[model.RoutingAvailabilityKey]struct{}{
				{CanonicalModel: modelrouting.Seedance20, ChannelID: 11}: {},
			}, available)
			cached, err := model.CacheGetChannel(11)
			require.NoError(t, err)
			assert.Equal(t, "cached", cached.Name)
		})
	}
}

func TestChannelDeletePathsCleanRoutingTargetsAndAbilities(t *testing.T) {
	tests := []struct {
		name   string
		status int
		remove func(t *testing.T, channel *model.Channel)
	}{
		{
			name:   "single delete",
			status: common.ChannelStatusEnabled,
			remove: func(t *testing.T, channel *model.Channel) {
				require.NoError(t, channel.Delete())
			},
		},
		{
			name:   "batch delete",
			status: common.ChannelStatusEnabled,
			remove: func(t *testing.T, channel *model.Channel) {
				rows, err := model.BatchDeleteChannels([]int{channel.Id})
				require.NoError(t, err)
				assert.Equal(t, int64(1), rows)
			},
		},
		{
			name:   "delete disabled",
			status: common.ChannelStatusManuallyDisabled,
			remove: func(t *testing.T, _ *model.Channel) {
				rows, err := model.DeleteDisabledChannel()
				require.NoError(t, err)
				assert.Equal(t, int64(1), rows)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := openRoutingTestDB(t)
			require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.RoutingPolicy{}, &model.RouteTarget{}))
			priority := int64(100)
			weight := uint(10)
			channels := []model.Channel{
				{Id: 11, Name: "delete", Key: "secret-delete", Status: tt.status, Priority: &priority, Weight: &weight},
				{Id: 12, Name: "retain", Key: "secret-retain", Status: common.ChannelStatusEnabled, Priority: &priority, Weight: &weight},
			}
			require.NoError(t, db.Create(&channels).Error)
			abilities := []model.Ability{
				{Group: "分组A", Model: modelrouting.Seedance20, ChannelId: 11, Enabled: tt.status == common.ChannelStatusEnabled, Priority: &priority, Weight: weight},
				{Group: "分组A", Model: modelrouting.Seedance20, ChannelId: 12, Enabled: true, Priority: &priority, Weight: weight},
			}
			require.NoError(t, db.Create(&abilities).Error)
			created, err := model.ReplaceRoutingPolicy(0, validRoutingPolicyRow(), []model.RouteTarget{
				{ChannelID: 11, Name: "delete", UpstreamModel: "provider-delete", TargetPriority: 100, Enabled: true, Constraints: validConstraintsJSON(t, modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3})},
				{ChannelID: 12, Name: "retain", UpstreamModel: "provider-retain", TargetPriority: 100, Enabled: true, Constraints: validConstraintsJSON(t, modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3})},
			})
			require.NoError(t, err)
			require.NoError(t, model.InitRoutingPolicyCache())

			tt.remove(t, &channels[0])

			var deletedTargetCount int64
			var retainedTargetCount int64
			var deletedAbilityCount int64
			require.NoError(t, db.Model(&model.RouteTarget{}).Where("policy_id = ? AND channel_id = ?", created.ID, 11).Count(&deletedTargetCount).Error)
			require.NoError(t, db.Model(&model.RouteTarget{}).Where("policy_id = ? AND channel_id = ?", created.ID, 12).Count(&retainedTargetCount).Error)
			require.NoError(t, db.Model(&model.Ability{}).Where("channel_id = ?", 11).Count(&deletedAbilityCount).Error)
			assert.Zero(t, deletedTargetCount)
			assert.Equal(t, int64(1), retainedTargetCount)
			assert.Zero(t, deletedAbilityCount)

			snapshot, ok := model.GetRoutingPolicySnapshot("分组A", modelrouting.Seedance20)
			require.True(t, ok)
			assert.NotContains(t, snapshot.TargetsByChannel, 11)
			assert.Contains(t, snapshot.TargetsByChannel, 12)
		})
	}
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
