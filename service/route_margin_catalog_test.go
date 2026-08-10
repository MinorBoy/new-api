package service

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListRouteMarginCatalogFiltersAtInclusiveThreshold(t *testing.T) {
	prepareCostRuleServiceDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.RoutingPolicy{}, &model.RouteTarget{}))
	require.NoError(t, model.DB.Exec("DELETE FROM route_targets").Error)
	require.NoError(t, model.DB.Exec("DELETE FROM routing_policies").Error)

	policy := model.RoutingPolicy{
		GroupName: "default", Model: "doubao-seedance-2-0-260128", Enabled: true,
		DefaultResolution: "720p", DefaultDuration: 4, DefaultRatio: "16:9",
	}
	require.NoError(t, model.DB.Create(&policy).Error)
	constraints := `{"output_resolutions":["720p"],"durations":{"values":[4]},"reference_limits":{"images":9,"videos":3,"audios":3}}`
	require.NoError(t, model.DB.Create(&model.RouteTarget{
		PolicyID: policy.ID, ChannelID: 7, Name: "route-target/a", UpstreamModel: "vendor-model",
		CostVariantKey: "default", TargetPriority: 100, Constraints: constraints, Enabled: true,
		ManagedBy: string(types.RouteTargetManagedByConfigImport),
	}).Error)

	now := common.GetTimestamp()
	configJSON, err := common.Marshal(normalizedUSDConfig(types.CostRuleConfigV1{
		UnitPrice: stringPointer("0.7"), ChargeEvent: types.CostChargeResponseSucceeded,
	}))
	require.NoError(t, err)
	require.NoError(t, model.DB.Create(&model.ChannelModelCostRule{
		ChannelID: 7, BillableUpstreamModel: "vendor-model", CostVariantKey: "default", Version: 1,
		Status: string(types.CostRuleActive), CostMode: string(types.CostModePerRequest), SchemaVersion: 1,
		ConfigJSON: string(configJSON), Source: "config_import", EffectiveFrom: &now, CreatedAt: now, UpdatedAt: now,
	}).Error)

	previousRevenueHook := RevenuePreviewHookForTest()
	SetRoutingRevenuePreview(func(context.Context, RoutingRevenuePreviewInput) (int64, string, error) {
		return 500_000, "500000", nil
	})
	t.Cleanup(func() { SetRoutingRevenuePreview(previousRevenueHook) })

	page, err := ListRouteMarginCatalog(context.Background(), RouteMarginCatalogFilter{
		MinimumMarginPPM: 300000, DurationSeconds: 4, GroupRatio: 1,
		Scenario: "no_video", Page: 1, PageSize: 50,
		SortBy: "gross_margin_ppm", SortOrder: "desc",
	})

	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	assert.True(t, page.Items[0].Eligible)
	assert.Equal(t, int64(300000), *page.Items[0].GrossMarginPPM)
	assert.Equal(t, int64(1_000_000_000), *page.Items[0].EstimatedRevenueNanoUSD)
	assert.Equal(t, int64(700_000_000), *page.Items[0].EstimatedCostNanoUSD)
}

func TestListRouteMarginCatalogExpandsBothScenariosAndPerDurationCost(t *testing.T) {
	prepareRouteMarginCatalogServiceDB(t)
	policy := seedRouteMarginPolicyTarget(t, "route-target/duration", "duration-model", "720p")
	seedRouteMarginRule(t, 7, "duration-model", types.CostModePerDuration, types.CostRuleConfigV1{
		PricePerSecond: stringPointer("0.175"), MeterSource: types.CostMeterValidatedRequest,
		ChargeEvent: types.CostChargeTaskSucceeded,
	})
	installRouteMarginRevenueHook(t, 500_000, "500000")

	page, err := ListRouteMarginCatalog(context.Background(), RouteMarginCatalogFilter{
		MinimumMarginPPM: 300000, DurationSeconds: 4, GroupRatio: 1,
		Scenario: RouteMarginScenarioAll, Page: 1, PageSize: 50,
	})

	require.NoError(t, err)
	assert.Equal(t, policy.ID, page.Items[0].PolicyID)
	require.Len(t, page.Items, 2)
	assert.Equal(t, RouteMarginScenarioNoVideo, page.Items[0].Scenario)
	assert.Equal(t, RouteMarginScenarioWithVideo, page.Items[1].Scenario)
	assert.Equal(t, 2, page.Summary.EligibleScenarioCount)
	assert.Equal(t, 1, page.Summary.FullyEligibleTargetCount)
}

func TestListRouteMarginCatalogRejectsMarginJustBelowThreshold(t *testing.T) {
	prepareRouteMarginCatalogServiceDB(t)
	seedRouteMarginPolicyTarget(t, "route-target/below", "below-model", "720p")
	seedRouteMarginRule(t, 7, "below-model", types.CostModePerRequest, types.CostRuleConfigV1{
		UnitPrice: stringPointer("0.700001"), ChargeEvent: types.CostChargeResponseSucceeded,
	})
	installRouteMarginRevenueHook(t, 500_000, "500000")

	page, err := ListRouteMarginCatalog(context.Background(), RouteMarginCatalogFilter{
		MinimumMarginPPM: 300000, DurationSeconds: 4, GroupRatio: 1,
		Scenario: RouteMarginScenarioNoVideo, Page: 1, PageSize: 50,
	})

	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	assert.False(t, page.Items[0].Eligible)
	require.NotNil(t, page.Items[0].GrossMarginPPM)
	assert.Equal(t, int64(299999), *page.Items[0].GrossMarginPPM)
	assert.Equal(t, string(ProfitReasonMarginBelowThreshold), page.Items[0].FailureReason)
}

func TestListRouteMarginCatalogKeepsMissingCostRuleAsIneligible(t *testing.T) {
	prepareRouteMarginCatalogServiceDB(t)
	seedRouteMarginPolicyTarget(t, "route-target/missing", "missing-model", "720p")
	installRouteMarginRevenueHook(t, 500_000, "500000")

	page, err := ListRouteMarginCatalog(context.Background(), RouteMarginCatalogFilter{
		MinimumMarginPPM: 300000, DurationSeconds: 4, GroupRatio: 1,
		Scenario: RouteMarginScenarioNoVideo, Page: 1, PageSize: 50,
	})

	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	assert.False(t, page.Items[0].Eligible)
	assert.Equal(t, string(ProfitReasonCostRuleMissing), page.Items[0].FailureReason)
	assert.Nil(t, page.Items[0].EstimatedCostNanoUSD)
}

func TestListRouteMarginCatalogCalculatesTokenCostFromResolutionFacts(t *testing.T) {
	prepareRouteMarginCatalogServiceDB(t)
	seedRouteMarginPolicyTarget(t, "route-target/token", "token-model", "720p")
	seedRouteMarginRule(t, 7, "token-model", types.CostModePerToken, types.CostRuleConfigV1{
		TokenMode: types.CostTokenModeTotal, TotalPerMillion: stringPointer("1"),
		MeterSource: types.CostMeterUpstreamUsage, ChargeEvent: types.CostChargeTaskSucceeded,
	})
	installRouteMarginRevenueHook(t, 500_000, "500000")

	page, err := ListRouteMarginCatalog(context.Background(), RouteMarginCatalogFilter{
		MinimumMarginPPM: 300000, DurationSeconds: 4, GroupRatio: 1,
		Scenario: RouteMarginScenarioNoVideo, Page: 1, PageSize: 50,
	})

	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	require.NotNil(t, page.Items[0].EstimatedCostNanoUSD)
	require.NotNil(t, page.Items[0].GrossMarginPPM)
	assert.True(t, page.Items[0].Eligible)
}

func TestNormalizeRouteMarginCatalogFilterRejectsUnsafeValues(t *testing.T) {
	tests := []RouteMarginCatalogFilter{
		{MinimumMarginPPM: 1_000_001, DurationSeconds: 4, GroupRatio: 1},
		{MinimumMarginPPM: 300000, DurationSeconds: 0, GroupRatio: 1},
		{MinimumMarginPPM: 300000, DurationSeconds: 4, GroupRatio: 0},
		{MinimumMarginPPM: 300000, DurationSeconds: 4, GroupRatio: 1, Scenario: "unknown"},
	}
	for _, filter := range tests {
		_, err := normalizeRouteMarginCatalogFilter(filter)
		require.Error(t, err)
	}
}

func prepareRouteMarginCatalogServiceDB(t *testing.T) {
	t.Helper()
	prepareCostRuleServiceDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.RoutingPolicy{}, &model.RouteTarget{}))
	require.NoError(t, model.DB.Exec("DELETE FROM route_targets").Error)
	require.NoError(t, model.DB.Exec("DELETE FROM routing_policies").Error)
	t.Cleanup(func() {
		model.DB.Exec("DELETE FROM route_targets")
		model.DB.Exec("DELETE FROM routing_policies")
	})
}

func seedRouteMarginPolicyTarget(t *testing.T, name, upstreamModel, resolution string) model.RoutingPolicy {
	t.Helper()
	policy := model.RoutingPolicy{
		GroupName: "default", Model: "doubao-seedance-2-0-260128", Enabled: true,
		DefaultResolution: resolution, DefaultDuration: 4, DefaultRatio: "16:9",
	}
	require.NoError(t, model.DB.Create(&policy).Error)
	constraints := `{"output_resolutions":["` + resolution + `"],"durations":{"values":[4]},"reference_limits":{"images":9,"videos":3,"audios":3}}`
	require.NoError(t, model.DB.Create(&model.RouteTarget{
		PolicyID: policy.ID, ChannelID: 7, Name: name, UpstreamModel: upstreamModel,
		CostVariantKey: "default", TargetPriority: 100, Constraints: constraints, Enabled: true,
		ManagedBy: string(types.RouteTargetManagedByConfigImport),
	}).Error)
	return policy
}

func seedRouteMarginRule(t *testing.T, channelID int, upstreamModel string, mode types.CostMode, config types.CostRuleConfigV1) {
	t.Helper()
	now := common.GetTimestamp()
	configJSON, err := common.Marshal(normalizedUSDConfig(config))
	require.NoError(t, err)
	require.NoError(t, model.DB.Create(&model.ChannelModelCostRule{
		ChannelID: channelID, BillableUpstreamModel: upstreamModel, CostVariantKey: "default", Version: 1,
		Status: string(types.CostRuleActive), CostMode: string(mode), SchemaVersion: 1,
		ConfigJSON: string(configJSON), Source: "config_import", EffectiveFrom: &now, CreatedAt: now, UpdatedAt: now,
	}).Error)
}

func installRouteMarginRevenueHook(t *testing.T, finalQuota int64, snapshot string) {
	t.Helper()
	previous := RevenuePreviewHookForTest()
	SetRoutingRevenuePreview(func(context.Context, RoutingRevenuePreviewInput) (int64, string, error) {
		return finalQuota, snapshot, nil
	})
	t.Cleanup(func() { SetRoutingRevenuePreview(previous) })
}
