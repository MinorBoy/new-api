package service

import (
	"testing"

	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDisabledGroupIsExcludedFromUsableGroupsAndModels(t *testing.T) {
	originalGroups := setting.UserUsableGroups2JSONString()
	originalStatus := ratio_setting.GroupStatus2JSONString()
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalGroups))
		require.NoError(t, ratio_setting.UpdateGroupStatusByJSONString(originalStatus))
	})

	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default","paused":"Paused"}`))
	require.NoError(t, ratio_setting.UpdateGroupStatusByJSONString(`{"paused":false}`))

	groups := GetUserUsableGroups("default")
	assert.Contains(t, groups, "default")
	assert.NotContains(t, groups, "paused")
	assert.Empty(t, GetGroupEnabledModelsForRouting("paused"))
}

func TestDisabledUserGroupIsNotReintroducedAsUsable(t *testing.T) {
	originalGroups := setting.UserUsableGroups2JSONString()
	originalStatus := ratio_setting.GroupStatus2JSONString()
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalGroups))
		require.NoError(t, ratio_setting.UpdateGroupStatusByJSONString(originalStatus))
	})

	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default"}`))
	require.NoError(t, ratio_setting.UpdateGroupStatusByJSONString(`{"paused":false}`))

	assert.NotContains(t, GetUserUsableGroups("paused"), "paused")
}
