package ratio_setting

import (
	"testing"

	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseGroupRoutingRequirementsNormalizesDynamicProfile(t *testing.T) {
	profiles, err := ParseGroupRoutingRequirementsJSONString(`{
		"客户A": {
			"status": "active",
			"routing_source": "default",
			"real_person_mode": "required",
			"allowed_cost_modes": ["per_duration", "per_request", "per_duration"],
			"excluded_target_keys": ["grt_a", "grt_a", "grt_b"]
		}
	}`)
	require.NoError(t, err)
	profile := profiles["客户A"]
	assert.Equal(t, GroupRoutingProfileActive, profile.Status)
	assert.Equal(t, GroupRealPersonRequired, profile.EffectiveRealPersonMode())
	assert.Equal(t, []types.CostMode{types.CostModePerDuration, types.CostModePerRequest}, profile.AllowedCostModes)
	assert.Equal(t, []string{"grt_a", "grt_b"}, profile.ExcludedTargetKeys)
}

func TestParseGroupRoutingRequirementsKeepsLegacyRealPersonSemantics(t *testing.T) {
	profiles, err := ParseGroupRoutingRequirementsJSONString(`{"真人分组":{"require_real_person":true}}`)
	require.NoError(t, err)
	profile := profiles["真人分组"]
	assert.False(t, profile.IsDynamic())
	assert.Equal(t, GroupRealPersonRequired, profile.EffectiveRealPersonMode())
}

func TestCheckGroupRoutingRequirementsRejectsUnsafeDynamicProfiles(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{name: "default self inheritance", raw: `{"default":{"status":"active","routing_source":"default"}}`},
		{name: "auto pseudo group", raw: `{"auto":{"status":"active","routing_source":"default"}}`},
		{name: "invalid real person mode", raw: `{"客户A":{"status":"draft","routing_source":"default","real_person_mode":"sometimes"}}`},
		{name: "invalid cost mode", raw: `{"客户A":{"status":"draft","routing_source":"default","allowed_cost_modes":["per_minute"]}}`},
		{name: "legacy conflict", raw: `{"客户A":{"require_real_person":true,"real_person_mode":"forbidden"}}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Error(t, CheckGroupRoutingRequirements(test.raw))
		})
	}
}

func TestUpdateGroupRoutingRequirementsStoresNormalizedProfile(t *testing.T) {
	original := GroupRoutingRequirements2JSONString()
	t.Cleanup(func() { require.NoError(t, UpdateGroupRoutingRequirementsByJSONString(original)) })

	require.NoError(t, UpdateGroupRoutingRequirementsByJSONString(`{
		"客户A": {
			"status": "active",
			"routing_source": "default",
			"allowed_cost_modes": ["per_request", "per_duration", "per_request"],
			"excluded_target_keys": ["grt_b", "grt_a", "grt_b"]
		}
	}`))
	profile := GetGroupRoutingRequirements("客户A")
	assert.Equal(t, []types.CostMode{types.CostModePerDuration, types.CostModePerRequest}, profile.AllowedCostModes)
	assert.Equal(t, []string{"grt_a", "grt_b"}, profile.ExcludedTargetKeys)
}

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
