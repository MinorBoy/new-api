package service

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// costRuleWithVariant mirrors costRuleWithConfig but lets the caller pick the
// variant identity, so the test mirrors the post-migration state instead of the
// legacy blank-variant rows.
func costRuleWithVariant(t *testing.T, variant string, version int, status types.CostRuleStatus, unitPrice string) model.ChannelModelCostRule {
	t.Helper()
	rule := costRuleWithConfig(t, types.CostModePerRequest, normalizedPerRequestConfig(t, unitPrice))
	rule.CostVariantKey = variant
	rule.Version = version
	rule.Status = string(status)
	return rule
}

func TestActiveCostRuleResolvesVariantScopedRules(t *testing.T) {
	prepareCostRuleServiceDB(t)

	rule480 := costRuleWithVariant(t, "480p", 1, types.CostRuleActive, "0.2")
	require.NoError(t, model.DB.Create(&rule480).Error)
	rule720 := costRuleWithVariant(t, "720p", 1, types.CostRuleActive, "0.4")
	require.NoError(t, model.DB.Create(&rule720).Error)

	found480, err := ActiveCostRule(7, "vendor-model", "480p", true)
	require.NoError(t, err)
	assert.Equal(t, "480p", found480.CostVariantKey)

	found720, err := ActiveCostRule(7, "vendor-model", "720p", true)
	require.NoError(t, err)
	assert.Equal(t, "720p", found720.CostVariantKey)
	assert.NotEqual(t, found480.ID, found720.ID)
}

func TestActiveCostRuleBlankVariantResolvesDefault(t *testing.T) {
	prepareCostRuleServiceDB(t)

	active := costRuleWithVariant(t, string(types.DefaultCostVariantKey), 1, types.CostRuleActive, "0.2")
	require.NoError(t, model.DB.Create(&active).Error)

	// A blank variant must resolve to default so legacy callers keep working.
	rule, err := ActiveCostRule(7, "vendor-model", "", true)
	require.NoError(t, err)
	assert.Equal(t, string(types.DefaultCostVariantKey), rule.CostVariantKey)

	// An explicit default lookup returns the same rule.
	same, err := ActiveCostRule(7, "vendor-model", string(types.DefaultCostVariantKey), true)
	require.NoError(t, err)
	assert.Equal(t, rule.ID, same.ID)
}

func TestInvalidateCostCoverageIsVariantScoped(t *testing.T) {
	prepareCostRuleServiceDB(t)

	rule480 := costRuleWithVariant(t, "480p", 1, types.CostRuleActive, "0.2")
	require.NoError(t, model.DB.Create(&rule480).Error)
	rule720 := costRuleWithVariant(t, "720p", 1, types.CostRuleActive, "0.4")
	require.NoError(t, model.DB.Create(&rule720).Error)

	// Prime both caches.
	_, err := ActiveCostRule(7, "vendor-model", "480p", false)
	require.NoError(t, err)
	_, err = ActiveCostRule(7, "vendor-model", "720p", false)
	require.NoError(t, err)

	// Invalidating only the 480p variant leaves 720p cached.
	require.NoError(t, model.DB.Delete(&rule720).Error)
	InvalidateCostCoverage(7, "vendor-model", "480p")
	found, err := ActiveCostRule(7, "vendor-model", "720p", false)
	require.NoError(t, err)
	assert.Equal(t, rule720.ID, found.ID)
}

func TestActiveCostRulesResolvesEachVariantSeparately(t *testing.T) {
	prepareCostRuleServiceDB(t)

	rule480 := costRuleWithVariant(t, "480p", 1, types.CostRuleActive, "0.2")
	require.NoError(t, model.DB.Create(&rule480).Error)
	rule720 := costRuleWithVariant(t, "720p", 1, types.CostRuleActive, "0.4")
	require.NoError(t, model.DB.Create(&rule720).Error)

	rules, err := ActiveCostRules([]CostRuleCandidate{
		{ChannelID: 7, BillableUpstreamModel: "vendor-model", CostVariantKey: "480p"},
		{ChannelID: 7, BillableUpstreamModel: "vendor-model", CostVariantKey: "720p"},
	}, true)
	require.NoError(t, err)
	found480, ok := rules[CostRuleCandidate{ChannelID: 7, BillableUpstreamModel: "vendor-model", CostVariantKey: "480p"}]
	require.True(t, ok)
	assert.Equal(t, rule480.ID, found480.ID)
	found720, ok := rules[CostRuleCandidate{ChannelID: 7, BillableUpstreamModel: "vendor-model", CostVariantKey: "720p"}]
	require.True(t, ok)
	assert.Equal(t, rule720.ID, found720.ID)
}

func TestCheckAuthoritativeCostCoverageEvaluatesActiveVariants(t *testing.T) {
	prepareCostRuleServiceDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Ability{}))
	require.NoError(t, model.DB.Exec("DELETE FROM abilities").Error)
	t.Cleanup(func() {
		require.NoError(t, model.DB.Exec("DELETE FROM abilities").Error)
	})
	require.NoError(t, model.DB.Create(&model.Ability{
		Group: "default", Model: "vendor-model", ChannelId: 7, Enabled: true,
	}).Error)

	rule480 := costRuleWithVariant(t, "480p", 1, types.CostRuleActive, "0.2")
	require.NoError(t, model.DB.Create(&rule480).Error)

	results, err := CheckAuthoritativeCostCoverage()

	require.NoError(t, err)
	require.Len(t, results, 1)
	coveredByVariant := make(map[string]bool, len(results))
	for _, result := range results {
		coveredByVariant[result.CostVariantKey] = result.Covered
	}
	assert.NotContains(t, coveredByVariant, string(types.DefaultCostVariantKey))
	assert.True(t, coveredByVariant["480p"])
}

func TestCheckAuthoritativeCostCoverageRequiresDefaultWithoutVariantRules(t *testing.T) {
	prepareCostRuleServiceDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Ability{}))
	require.NoError(t, model.DB.Exec("DELETE FROM abilities").Error)
	t.Cleanup(func() {
		require.NoError(t, model.DB.Exec("DELETE FROM abilities").Error)
	})
	require.NoError(t, model.DB.Create(&model.Ability{
		Group: "default", Model: "vendor-model", ChannelId: 7, Enabled: true,
	}).Error)

	results, err := CheckAuthoritativeCostCoverage()

	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, string(types.DefaultCostVariantKey), results[0].CostVariantKey)
	assert.False(t, results[0].Covered)
}

func TestCheckAuthoritativeCostCoverageIncludesEnabledRoutingTargetVariant(t *testing.T) {
	prepareCostRuleServiceDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Ability{}, &model.RoutingPolicy{}, &model.RouteTarget{}))
	require.NoError(t, model.DB.Exec("DELETE FROM abilities").Error)
	require.NoError(t, model.DB.Exec("DELETE FROM route_targets").Error)
	require.NoError(t, model.DB.Exec("DELETE FROM routing_policies").Error)
	t.Cleanup(func() {
		require.NoError(t, model.DB.Exec("DELETE FROM abilities").Error)
		require.NoError(t, model.DB.Exec("DELETE FROM route_targets").Error)
		require.NoError(t, model.DB.Exec("DELETE FROM routing_policies").Error)
	})
	require.NoError(t, model.DB.Create(&model.Ability{
		Group: "default", Model: "vendor-model", ChannelId: 7, Enabled: true,
	}).Error)
	defaultRule := costRuleWithVariant(t, string(types.DefaultCostVariantKey), 1, types.CostRuleActive, "0.2")
	require.NoError(t, model.DB.Create(&defaultRule).Error)
	policy := model.RoutingPolicy{
		GroupName: "default", Model: "vendor-model", Enabled: true,
		DefaultResolution: "720p", DefaultDuration: 10, DefaultRatio: "16:9",
	}
	require.NoError(t, model.DB.Create(&policy).Error)
	require.NoError(t, model.DB.Create(&model.RouteTarget{
		PolicyID: policy.ID, ChannelID: 7, Name: "480p route", UpstreamModel: "vendor-model",
		CostVariantKey: "480p", TargetPriority: 1, Constraints: "{}", Enabled: true,
	}).Error)

	results, err := CheckAuthoritativeCostCoverage()

	require.NoError(t, err)
	coveredByVariant := make(map[string]bool, len(results))
	for _, result := range results {
		coveredByVariant[result.CostVariantKey] = result.Covered
	}
	assert.True(t, coveredByVariant[string(types.DefaultCostVariantKey)])
	assert.Contains(t, coveredByVariant, "480p")
	assert.False(t, coveredByVariant["480p"])
}

func TestListCostRulesFiltersByVariant(t *testing.T) {
	prepareCostRuleServiceDB(t)

	rule480 := costRuleWithVariant(t, "480p", 1, types.CostRuleActive, "0.2")
	require.NoError(t, model.DB.Create(&rule480).Error)
	rule720 := costRuleWithVariant(t, "720p", 1, types.CostRuleActive, "0.4")
	require.NoError(t, model.DB.Create(&rule720).Error)

	rules, err := ListCostRules(7, "vendor-model", "480p")
	require.NoError(t, err)
	require.Len(t, rules, 1)
	assert.Equal(t, "480p", rules[0].CostVariantKey)
}
