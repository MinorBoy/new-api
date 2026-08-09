package ratio_setting

import (
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
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
		{name: "missing dynamic status", raw: `{"客户A":{"routing_source":"default"}}`},
		{name: "unsupported routing source", raw: `{"客户A":{"status":"active","routing_source":"backup"}}`},
		{name: "status without routing source", raw: `{"客户A":{"status":"draft"}}`},
		{name: "real person mode without routing source", raw: `{"客户A":{"real_person_mode":"required"}}`},
		{name: "cost modes without routing source", raw: `{"客户A":{"allowed_cost_modes":["per_request"]}}`},
		{name: "excluded targets without routing source", raw: `{"客户A":{"excluded_target_keys":["grt_a"]}}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Error(t, CheckGroupRoutingRequirements(test.raw))
		})
	}
}

func TestParseGroupRoutingRequirementsEnforcesLimits(t *testing.T) {
	t.Run("JSON bytes", func(t *testing.T) {
		const maxJSONBytes = 1 << 20
		raw := `{"legacy":{"require_real_person":true}}`
		exact := raw + strings.Repeat(" ", maxJSONBytes-len(raw))

		_, err := ParseGroupRoutingRequirementsJSONString(exact)
		require.NoError(t, err)
		_, err = ParseGroupRoutingRequirementsJSONString(exact + " ")
		require.Error(t, err)
	})

	t.Run("profile name runes", func(t *testing.T) {
		exactName := strings.Repeat("客", 64)
		encoded, err := common.Marshal(map[string]GroupRoutingRequirements{exactName: {}})
		require.NoError(t, err)
		_, err = ParseGroupRoutingRequirementsJSONString(string(encoded))
		require.NoError(t, err)

		overName := strings.Repeat("客", 65)
		encoded, err = common.Marshal(map[string]GroupRoutingRequirements{overName: {}})
		require.NoError(t, err)
		_, err = ParseGroupRoutingRequirementsJSONString(string(encoded))
		require.Error(t, err)
	})

	t.Run("profile count", func(t *testing.T) {
		profiles := make(map[string]GroupRoutingRequirements, 201)
		for index := 0; index < 200; index++ {
			profiles[fmt.Sprintf("group_%03d", index)] = GroupRoutingRequirements{}
		}
		encoded, err := common.Marshal(profiles)
		require.NoError(t, err)
		_, err = ParseGroupRoutingRequirementsJSONString(string(encoded))
		require.NoError(t, err)

		profiles["one_over"] = GroupRoutingRequirements{}
		encoded, err = common.Marshal(profiles)
		require.NoError(t, err)
		_, err = ParseGroupRoutingRequirementsJSONString(string(encoded))
		require.Error(t, err)
	})

	t.Run("excluded target count", func(t *testing.T) {
		targetKeys := make([]string, 500)
		for index := range targetKeys {
			targetKeys[index] = fmt.Sprintf("grt_%03d", index)
		}
		profile := GroupRoutingRequirements{
			Status:             GroupRoutingProfileDraft,
			RoutingSource:      GroupRoutingSourceDefault,
			ExcludedTargetKeys: targetKeys,
		}
		encoded, err := common.Marshal(map[string]GroupRoutingRequirements{"客户A": profile})
		require.NoError(t, err)
		_, err = ParseGroupRoutingRequirementsJSONString(string(encoded))
		require.NoError(t, err)

		profile.ExcludedTargetKeys = append(profile.ExcludedTargetKeys, "one_over")
		encoded, err = common.Marshal(map[string]GroupRoutingRequirements{"客户A": profile})
		require.NoError(t, err)
		_, err = ParseGroupRoutingRequirementsJSONString(string(encoded))
		require.Error(t, err)
	})

	t.Run("excluded target key bytes", func(t *testing.T) {
		profile := GroupRoutingRequirements{
			Status:             GroupRoutingProfileDraft,
			RoutingSource:      GroupRoutingSourceDefault,
			ExcludedTargetKeys: []string{strings.Repeat("a", 128)},
		}
		encoded, err := common.Marshal(map[string]GroupRoutingRequirements{"客户A": profile})
		require.NoError(t, err)
		_, err = ParseGroupRoutingRequirementsJSONString(string(encoded))
		require.NoError(t, err)

		profile.ExcludedTargetKeys = []string{strings.Repeat("a", 129)}
		encoded, err = common.Marshal(map[string]GroupRoutingRequirements{"客户A": profile})
		require.NoError(t, err)
		_, err = ParseGroupRoutingRequirementsJSONString(string(encoded))
		require.Error(t, err)
	})
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
