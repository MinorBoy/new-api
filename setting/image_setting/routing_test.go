package image_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRoutingDefaultsAndOverrides(t *testing.T) {
	t.Cleanup(func() { require.NoError(t, UpdateRoutingByJSONString(`{"version":1,"default":{"strategy":"manual"}}`)) })
	assert.Equal(t, StrategyManual, PolicyFor("image", "gpt-image-1").Strategy)
	raw := `{"version":1,"default":{"strategy":"manual"},"groups":{"image":{"gpt-image-1":{"strategy":"lowest_cost","minimum_expected_margin_bps":1250,"require_compatibility_test":true}}}}`
	require.NoError(t, UpdateRoutingByJSONString(raw))
	policy := PolicyFor("image", "gpt-image-1")
	assert.Equal(t, StrategyLowestCost, policy.Strategy)
	assert.Equal(t, 1250, *policy.MinimumExpectedMarginBPS)
	assert.True(t, policy.RequireCompatibilityTest)
	assert.Equal(t, StrategyManual, PolicyFor("other", "model").Strategy)
}

func TestRoutingRejectsInvalidStrategyAndMarginWithoutReplacingSnapshot(t *testing.T) {
	t.Cleanup(func() { require.NoError(t, UpdateRoutingByJSONString(`{"version":1,"default":{"strategy":"manual"}}`)) })
	require.NoError(t, UpdateRoutingByJSONString(`{"version":1,"default":{"strategy":"lowest_cost"}}`))
	for _, raw := range []string{
		`{"version":1,"default":{"strategy":"unknown"}}`,
		`{"version":1,"default":{"strategy":"manual","minimum_expected_margin_bps":10001}}`,
		`{"version":1,"default":{"strategy":"manual","minimum_expected_margin_bps":-1}}`,
	} {
		require.Error(t, UpdateRoutingByJSONString(raw))
		assert.Equal(t, StrategyLowestCost, PolicyFor("", "").Strategy)
	}
}

func TestRoutingAcceptsCostWeightedTolerance(t *testing.T) {
	t.Cleanup(func() { require.NoError(t, UpdateRoutingByJSONString(`{"version":1,"default":{"strategy":"manual"}}`)) })
	require.NoError(t, UpdateRoutingByJSONString(`{"version":1,"default":{"strategy":"cost_weighted","cost_tolerance_bps":1250}}`))
	policy := PolicyFor("", "")
	assert.Equal(t, StrategyCostWeighted, policy.Strategy)
	require.NotNil(t, policy.CostToleranceBPS)
	assert.Equal(t, 1250, *policy.CostToleranceBPS)
}

func TestRoutingRejectsInvalidCostWeightedTolerance(t *testing.T) {
	t.Cleanup(func() { require.NoError(t, UpdateRoutingByJSONString(`{"version":1,"default":{"strategy":"manual"}}`)) })
	require.NoError(t, UpdateRoutingByJSONString(`{"version":1,"default":{"strategy":"lowest_cost"}}`))
	for _, raw := range []string{
		`{"version":1,"default":{"strategy":"cost_weighted","cost_tolerance_bps":-1}}`,
		`{"version":1,"default":{"strategy":"cost_weighted","cost_tolerance_bps":10001}}`,
		`{"version":1,"default":{"strategy":"manual","cost_tolerance_bps":100}}`,
	} {
		require.Error(t, UpdateRoutingByJSONString(raw))
		assert.Equal(t, StrategyLowestCost, PolicyFor("", "").Strategy)
	}
}
