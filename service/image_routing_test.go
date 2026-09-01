package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func imageRouteCandidateIDs(candidates []ImageRouteCandidate) []int {
	ids := make([]int, 0, len(candidates))
	for _, candidate := range candidates {
		ids = append(ids, candidate.ChannelID)
	}
	return ids
}
