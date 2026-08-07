package ratio_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckGroupRoutingRequirementsRejectsInvalidShape(t *testing.T) {
	err := CheckGroupRoutingRequirements(`{"真人分组":{"require_real_person":"yes"}}`)
	require.Error(t, err)
}

func TestCheckGroupRoutingRequirementsRejectsUnknownField(t *testing.T) {
	err := CheckGroupRoutingRequirements(`{"真人分组":{"unknown":true}}`)
	require.Error(t, err)
}

func TestGroupRoutingRequirementsDefaultsMissingGroupToFalse(t *testing.T) {
	original := GroupRoutingRequirements2JSONString()
	t.Cleanup(func() { require.NoError(t, UpdateGroupRoutingRequirementsByJSONString(original)) })

	require.NoError(t, UpdateGroupRoutingRequirementsByJSONString(`{"真人分组":{"require_real_person":true}}`))
	require.NotNil(t, GetGroupRoutingRequirements("真人分组").RequireRealPerson)
	assert.True(t, *GetGroupRoutingRequirements("真人分组").RequireRealPerson)
	assert.Nil(t, GetGroupRoutingRequirements("未配置分组").RequireRealPerson)
}

func TestUpdateGroupRoutingRequirementsRejectsWithoutChangingExistingValue(t *testing.T) {
	original := GroupRoutingRequirements2JSONString()
	t.Cleanup(func() { require.NoError(t, UpdateGroupRoutingRequirementsByJSONString(original)) })

	require.NoError(t, UpdateGroupRoutingRequirementsByJSONString(`{"真人分组":{"require_real_person":true}}`))
	require.Error(t, UpdateGroupRoutingRequirementsByJSONString(`{"真人分组":{"unknown":true}}`))
	require.NotNil(t, GetGroupRoutingRequirements("真人分组").RequireRealPerson)
	assert.True(t, *GetGroupRoutingRequirements("真人分组").RequireRealPerson)
}
