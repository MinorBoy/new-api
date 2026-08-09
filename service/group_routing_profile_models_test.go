package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelrouting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type profileModelFixture struct {
	Model         string
	UpstreamModel string
	CostMode      types.CostMode
}

func TestGetGroupsEnabledModelsUsesDynamicProfileMatches(t *testing.T) {
	seedProfileModelCatalog(t, []profileModelFixture{
		{Model: modelrouting.Seedance20, UpstreamModel: "duration-upstream", CostMode: types.CostModePerDuration},
		{Model: modelrouting.Seedance20Fast, UpstreamModel: "request-upstream", CostMode: types.CostModePerRequest},
	})
	setProfileModelRequirementsForTest(t, `{
		"按秒客户":{"status":"active","routing_source":"default","allowed_cost_modes":["per_duration"]}
	}`)

	assert.Equal(t, []string{modelrouting.Seedance20}, GetGroupsEnabledModels([]string{"按秒客户"}))
}

func TestGetGroupsEnabledModelsHidesDraftProfile(t *testing.T) {
	seedProfileModelCatalog(t, []profileModelFixture{
		{Model: modelrouting.Seedance20, UpstreamModel: "duration-upstream", CostMode: types.CostModePerDuration},
	})
	priority := int64(100)
	require.NoError(t, model.DB.Create(&model.Ability{
		Group: "草稿客户", Model: modelrouting.Seedance20, ChannelId: 7, Enabled: true,
		Priority: &priority, Weight: 10,
	}).Error)
	setProfileModelRequirementsForTest(t, `{
		"草稿客户":{"status":"draft","routing_source":"default"}
	}`)

	assert.Empty(t, GetGroupsEnabledModels([]string{"草稿客户"}))
}

func seedProfileModelCatalog(t *testing.T, fixtures []profileModelFixture) {
	t.Helper()
	prepareCostRuleServiceDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Ability{}, &model.RoutingPolicy{}, &model.RouteTarget{}))
	for _, table := range []string{"route_targets", "routing_policies", "abilities"} {
		require.NoError(t, model.DB.Exec("DELETE FROM "+table).Error)
	}
	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		common.MemoryCacheEnabled = previousMemoryCacheEnabled
		for _, table := range []string{"route_targets", "routing_policies", "abilities"} {
			require.NoError(t, model.DB.Exec("DELETE FROM "+table).Error)
		}
	})
	priority := int64(100)
	weight := uint(10)
	now := common.GetTimestamp()
	for index, fixture := range fixtures {
		policyID := index + 1
		require.NoError(t, model.DB.Create(&model.Ability{
			Group: "default", Model: fixture.Model, ChannelId: 7, Enabled: true,
			Priority: &priority, Weight: weight,
		}).Error)
		require.NoError(t, model.DB.Create(&model.RoutingPolicy{
			ID: policyID, GroupName: "default", Model: fixture.Model, Enabled: true,
			DefaultResolution: "720p", DefaultDuration: 10, DefaultRatio: "16:9",
		}).Error)
		supportsRealPerson := true
		constraints, err := common.Marshal(modelrouting.Constraints{
			OutputResolutions:  []string{"720p"},
			Durations:          modelrouting.DurationConstraint{Min: common.GetPointer(4), Max: common.GetPointer(15)},
			AspectRatios:       []string{"16:9"},
			ReferenceLimits:    modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3},
			SupportsRealPerson: &supportsRealPerson,
		})
		require.NoError(t, err)
		require.NoError(t, model.DB.Create(&model.RouteTarget{
			PolicyID: policyID, ChannelID: 7, Name: fixture.UpstreamModel,
			UpstreamModel: fixture.UpstreamModel, CostVariantKey: string(types.DefaultCostVariantKey),
			TargetPriority: 100, Enabled: true, Constraints: string(constraints),
		}).Error)
		seedActiveCostRuleRow(t, 7, fixture.UpstreamModel, fixture.CostMode, 1, &now)
	}
	InvalidateCostCoverage(0, "", "")
}

func setProfileModelRequirementsForTest(t *testing.T, value string) {
	t.Helper()
	original := ratio_setting.GroupRoutingRequirements2JSONString()
	require.NoError(t, ratio_setting.UpdateGroupRoutingRequirementsByJSONString(value))
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRoutingRequirementsByJSONString(original))
	})
}
