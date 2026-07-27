package model_test

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelrouting"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigPublishTransactionWritersRollBackWithoutRefresh(t *testing.T) {
	db := openRoutingTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Option{}, &model.RoutingPolicy{}, &model.RouteTarget{}, &model.ChannelModelCostRule{}))
	draft := &model.ChannelModelCostRule{
		ChannelID: 7, BillableUpstreamModel: "provider-model", CostVariantKey: "transaction-test", Version: 1,
		Status: string(types.CostRuleDraft), CostMode: "per_request", SchemaVersion: 1, ConfigJSON: `{}`,
	}
	require.NoError(t, db.Create(draft).Error)

	tx := db.Begin()
	require.NoError(t, tx.Error)
	_, err := model.ActivateChannelModelCostRuleWithTx(tx, draft.ID, 42, 100, nil)
	require.NoError(t, err)
	_, err = model.ReplaceRoutingPolicyWithTx(tx, 0, validRoutingPolicyRow(), []model.RouteTarget{{
		ChannelID: 11, Name: "target", UpstreamModel: "provider-model", CostVariantKey: "default", TargetPriority: 100, Enabled: true,
		Constraints: validConstraintsJSON(t, modelrouting.ReferenceLimits{}),
	}})
	require.NoError(t, err)
	require.NoError(t, model.UpdateOptionsWithTx(tx, map[string]string{"config-publish-test": "draft"}))
	require.NoError(t, tx.Rollback().Error)

	var stored model.ChannelModelCostRule
	require.NoError(t, db.First(&stored, draft.ID).Error)
	assert.Equal(t, string(types.CostRuleDraft), stored.Status)
	var policyCount, optionCount int64
	require.NoError(t, db.Model(&model.RoutingPolicy{}).Count(&policyCount).Error)
	require.NoError(t, db.Model(&model.Option{}).Where("key = ?", "config-publish-test").Count(&optionCount).Error)
	assert.Zero(t, policyCount)
	assert.Zero(t, optionCount)

	common.OptionMapRWMutex.RLock()
	_, refreshed := common.OptionMap["config-publish-test"]
	common.OptionMapRWMutex.RUnlock()
	assert.False(t, refreshed)
}

func TestConfigPublishTransactionRefreshesCommittedOptionsAndPolicies(t *testing.T) {
	db := openRoutingTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Option{}, &model.RoutingPolicy{}, &model.RouteTarget{}))
	common.OptionMapRWMutex.Lock()
	previousOptions := common.OptionMap
	common.OptionMap = map[string]string{}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptions
		common.OptionMapRWMutex.Unlock()
	})
	tx := db.Begin()
	require.NoError(t, tx.Error)
	policy, err := model.ReplaceRoutingPolicyWithTx(tx, 0, validRoutingPolicyRow(), []model.RouteTarget{{
		ChannelID: 11, Name: "target", UpstreamModel: "provider-model", CostVariantKey: "default", TargetPriority: 100, Enabled: true,
		Constraints: validConstraintsJSON(t, modelrouting.ReferenceLimits{}),
	}})
	require.NoError(t, err)
	require.NoError(t, model.UpdateOptionsWithTx(tx, map[string]string{"config-publish-test": "committed"}))
	require.NoError(t, tx.Commit().Error)

	require.NoError(t, model.RefreshOptions(map[string]string{"config-publish-test": "committed"}))
	require.NoError(t, model.RefreshRoutingPolicyCache(policy.GroupName, policy.Model))
	common.OptionMapRWMutex.RLock()
	value := common.OptionMap["config-publish-test"]
	common.OptionMapRWMutex.RUnlock()
	assert.Equal(t, "committed", value)
	_, cached := model.GetRoutingPolicySnapshot(policy.GroupName, policy.Model)
	assert.True(t, cached)
}
