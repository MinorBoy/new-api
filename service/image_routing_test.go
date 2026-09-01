package service

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImageRouteUnavailableErrorsAreClassifiable(t *testing.T) {
	assert.True(t, errors.Is(ErrNoCompatibleImageChannel, ErrNoCompatibleImageChannel))
	assert.True(t, errors.Is(ErrNoEligibleImageChannel, ErrNoEligibleImageChannel))
}

func TestImageRouteCandidateJSONUsesAdminPreviewContract(t *testing.T) {
	candidate := ImageRouteCandidate{
		ChannelID: 7, ChannelName: "cheap-images", UpstreamModel: "vendor-image",
		SKUKey: "gen-1024x1024-medium", CostKnown: true,
		EstimatedCostNanoUSD: 125_000_000, EstimatedRevenueNanoUSD: 250_000_000,
		EstimatedCostUSD: "0.125", EstimatedRevenueUSD: "0.25",
		RuleID: 9, RuleVersion: 2, ExclusionReason: "",
	}
	raw, err := common.Marshal(candidate)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"channel_id":7`)
	assert.Contains(t, string(raw), `"channel_name":"cheap-images"`)
	assert.Contains(t, string(raw), `"estimated_cost_usd":"0.125"`)
	assert.Contains(t, string(raw), `"rule_version":2`)
	assert.NotContains(t, string(raw), `"ChannelID"`)
}

func TestSortImageRouteCandidatesOrdersKnownCostBeforeUnknown(t *testing.T) {
	candidates := []ImageRouteCandidate{
		{ChannelID: 30, Priority: 100, CostKnown: false, ExclusionReason: "cost_rule_missing"},
		{ChannelID: 20, Priority: 1, CostKnown: true, EstimatedCostNanoUSD: 20},
		{ChannelID: 10, Priority: 50, CostKnown: true, EstimatedCostNanoUSD: 10},
	}

	sorted := SortImageRouteCandidates(candidates)
	require.Len(t, sorted, 3)
	assert.Equal(t, []int{10, 20, 30}, imageRouteCandidateIDs(sorted))
	assert.Equal(t, []int{30, 20, 10}, imageRouteCandidateIDs(candidates), "sorting must not mutate the input slice")
}

func TestSelectImageRouteCandidateUsesPriorityAfterEqualCost(t *testing.T) {
	candidates := []ImageRouteCandidate{
		{ChannelID: 10, Priority: 10, CostKnown: true, EstimatedCostNanoUSD: 100},
		{ChannelID: 20, Priority: 20, CostKnown: true, EstimatedCostNanoUSD: 100},
		{ChannelID: 30, Priority: 100, CostKnown: true, EstimatedCostNanoUSD: 200},
	}

	var selectorCalls int
	previous := imageRouteWeightSelector
	imageRouteWeightSelector = func(tie []ImageRouteCandidate) *ImageRouteCandidate {
		selectorCalls++
		require.Len(t, tie, 1)
		selected := tie[0]
		return &selected
	}
	t.Cleanup(func() { imageRouteWeightSelector = previous })

	selected := SelectImageRouteCandidate(candidates)
	require.NotNil(t, selected)
	assert.Equal(t, 20, selected.ChannelID)
	assert.Equal(t, 1, selectorCalls)
}

func TestSelectImageRouteCandidateUsesWeightOnlyForFullTie(t *testing.T) {
	candidates := []ImageRouteCandidate{
		{ChannelID: 10, Priority: 20, Weight: 1, CostKnown: true, EstimatedCostNanoUSD: 100},
		{ChannelID: 20, Priority: 20, Weight: 9, CostKnown: true, EstimatedCostNanoUSD: 100},
		{ChannelID: 30, Priority: 10, Weight: 100, CostKnown: true, EstimatedCostNanoUSD: 100},
	}

	previous := imageRouteWeightSelector
	imageRouteWeightSelector = func(tie []ImageRouteCandidate) *ImageRouteCandidate {
		require.Equal(t, []int{10, 20}, imageRouteCandidateIDs(tie))
		selected := tie[1]
		return &selected
	}
	t.Cleanup(func() { imageRouteWeightSelector = previous })

	selected := SelectImageRouteCandidate(candidates)
	require.NotNil(t, selected)
	assert.Equal(t, 20, selected.ChannelID)
}

func TestSelectImageRouteCandidateKeepsUnknownCostAsLastFallback(t *testing.T) {
	known := ImageRouteCandidate{ChannelID: 10, Priority: 1, CostKnown: true, EstimatedCostNanoUSD: 100}
	unknown := ImageRouteCandidate{ChannelID: 20, Priority: 100, CostKnown: false, ExclusionReason: "cost_meter_unknown"}

	selected := SelectImageRouteCandidate([]ImageRouteCandidate{unknown, known})
	require.NotNil(t, selected)
	assert.Equal(t, known.ChannelID, selected.ChannelID)

	selected = SelectImageRouteCandidate([]ImageRouteCandidate{unknown})
	require.NotNil(t, selected)
	assert.Equal(t, unknown.ChannelID, selected.ChannelID)
	matched := findImageRouteCandidate([]ImageRouteCandidate{unknown}, selected.ChannelID)
	require.NotNil(t, matched)
	assert.Equal(t, unknown.ExclusionReason, matched.ExclusionReason)
}

func TestSelectImageRouteCandidateReordersAfterFailedChannelIsRemoved(t *testing.T) {
	candidates := []ImageRouteCandidate{
		{ChannelID: 10, Priority: 1, CostKnown: true, EstimatedCostNanoUSD: 100},
		{ChannelID: 20, Priority: 100, CostKnown: true, EstimatedCostNanoUSD: 200},
		{ChannelID: 30, Priority: 1, CostKnown: true, EstimatedCostNanoUSD: 150},
	}

	selected := SelectImageRouteCandidate(candidates)
	require.NotNil(t, selected)
	assert.Equal(t, 10, selected.ChannelID)

	remaining := []ImageRouteCandidate{candidates[1], candidates[2]}
	selected = SelectImageRouteCandidate(remaining)
	require.NotNil(t, selected)
	assert.Equal(t, 30, selected.ChannelID, "retry must sort the remaining candidates by cost again")
}

func TestSelectManualImageRouteCandidateStartsAtHighestRemainingPriority(t *testing.T) {
	candidates := []ImageRouteCandidate{
		{ChannelID: 20, Priority: 10, Weight: 100},
		{ChannelID: 30, Priority: 30, Weight: 1},
	}

	selected := SelectManualImageRouteCandidate(candidates, 4)
	require.NotNil(t, selected)
	assert.Equal(t, 30, selected.ChannelID)
}

func TestCostWeightedImageRoutePoolUsesDefaultToleranceAndInverseCost(t *testing.T) {
	candidates := []ImageRouteCandidate{
		{ChannelID: 1, Weight: 0, CostKnown: true, EstimatedCostNanoUSD: 100},
		{ChannelID: 2, Weight: 0, CostKnown: true, EstimatedCostNanoUSD: 105},
		{ChannelID: 3, Weight: 0, CostKnown: true, EstimatedCostNanoUSD: 120},
	}
	pool := BuildCostWeightedImageRoutePool(candidates, nil)
	require.Equal(t, []int{1, 2}, imageRouteCandidateIDs(pool))
	assert.Greater(t, pool[0].EffectiveWeight, pool[1].EffectiveWeight)
}

func TestCostWeightedImageRoutePoolKeepsEqualCostWeightSemantics(t *testing.T) {
	candidates := []ImageRouteCandidate{
		{ChannelID: 1, Weight: 1, CostKnown: true, EstimatedCostNanoUSD: 100},
		{ChannelID: 2, Weight: 9, CostKnown: true, EstimatedCostNanoUSD: 100},
	}
	pool := BuildCostWeightedImageRoutePool(candidates, intPtr(0))
	require.Len(t, pool, 2)
	assert.Equal(t, 11, pool[0].EffectiveWeight)
	assert.Equal(t, 19, pool[1].EffectiveWeight)
}

func TestCostWeightedImageRoutePoolRecomputesMinimumAfterRetry(t *testing.T) {
	remaining := []ImageRouteCandidate{
		{ChannelID: 2, CostKnown: true, EstimatedCostNanoUSD: 200},
		{ChannelID: 3, CostKnown: true, EstimatedCostNanoUSD: 220},
	}
	pool := BuildCostWeightedImageRoutePool(remaining, intPtr(1000))
	require.Equal(t, []int{2, 3}, imageRouteCandidateIDs(pool))
	assert.Equal(t, int64(200), pool[0].EstimatedCostNanoUSD)
}

func imageRouteCandidateIDs(candidates []ImageRouteCandidate) []int {
	ids := make([]int, 0, len(candidates))
	for _, candidate := range candidates {
		ids = append(ids, candidate.ChannelID)
	}
	return ids
}

func intPtr(value int) *int {
	return &value
}
