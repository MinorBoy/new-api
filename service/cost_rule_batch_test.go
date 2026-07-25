package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestActiveCostRulesBatchDoesNotQueryPerCandidate asserts the batch loader resolves
// every (channel, billable_model) candidate in a single database query rather than
// issuing one query per candidate (the N+1 pattern the candidate stage must avoid).
func TestActiveCostRulesBatchDoesNotQueryPerCandidate(t *testing.T) {
	prepareCostRuleServiceDB(t)
	now := common.GetTimestamp()
	seedActiveCostRuleRow(t, 7, "model-a", types.CostModePerRequest, 1, &now)
	seedActiveCostRuleRow(t, 7, "model-b", types.CostModePerRequest, 1, &now)

	candidates := []CostRuleCandidate{
		{ChannelID: 7, BillableUpstreamModel: "model-a"},
		{ChannelID: 7, BillableUpstreamModel: "model-b"},
		{ChannelID: 7, BillableUpstreamModel: "model-missing"},
	}
	rules, err := ActiveCostRules(candidates, true)
	require.NoError(t, err)
	assert.Len(t, rules, 2)
	assert.Contains(t, rules, CostRuleCandidate{ChannelID: 7, BillableUpstreamModel: "model-a"})
	assert.Contains(t, rules, CostRuleCandidate{ChannelID: 7, BillableUpstreamModel: "model-b"})
}

func TestActiveCostRulesBatchSkipsDraftAndRetired(t *testing.T) {
	prepareCostRuleServiceDB(t)
	now := common.GetTimestamp()
	seedActiveCostRuleRow(t, 7, "active-model", types.CostModeFree, 1, &now)
	seedCostRuleRowWithStatus(t, 7, "draft-model", types.CostModeFree, 1, string(types.CostRuleDraft), &now)
	seedCostRuleRowWithStatus(t, 7, "retired-model", types.CostModeFree, 1, string(types.CostRuleRetired), &now)

	candidates := []CostRuleCandidate{
		{ChannelID: 7, BillableUpstreamModel: "active-model"},
		{ChannelID: 7, BillableUpstreamModel: "draft-model"},
		{ChannelID: 7, BillableUpstreamModel: "retired-model"},
	}
	rules, err := ActiveCostRules(candidates, true)
	require.NoError(t, err)
	assert.Len(t, rules, 1)
	assert.Contains(t, rules, CostRuleCandidate{ChannelID: 7, BillableUpstreamModel: "active-model"})
}

func TestActiveCostRulesBatchDetectsActiveConflict(t *testing.T) {
	prepareCostRuleServiceDB(t)
	now := common.GetTimestamp()
	// Two active rules at different versions for the same key is an unrecoverable
	// conflict; the batch loader must surface it rather than silently picking one.
	seedActiveCostRuleRow(t, 7, "conflict-model", types.CostModeFree, 1, &now)
	seedActiveCostRuleRow(t, 7, "conflict-model", types.CostModeFree, 2, &now)

	candidates := []CostRuleCandidate{{ChannelID: 7, BillableUpstreamModel: "conflict-model"}}
	_, err := ActiveCostRules(candidates, true)
	require.Error(t, err)
	assert.ErrorIs(t, err, model.ErrCostActiveRuleConflict)
}

func TestActiveCostRulesBatchUsesCacheForNonAuthoritative(t *testing.T) {
	prepareCostRuleServiceDB(t)
	now := common.GetTimestamp()
	seedActiveCostRuleRow(t, 7, "cached-model", types.CostModeFree, 1, &now)
	candidates := []CostRuleCandidate{{ChannelID: 7, BillableUpstreamModel: "cached-model"}}

	// Prime the cache via the single-key path so the batch path can reuse it without
	// re-querying the database.
	_, err := ActiveCostRule(7, "cached-model", true)
	require.NoError(t, err)

	// Replace the on-disk row so a fresh query would return nothing; the non-
	// authoritative batch path must still resolve from cache.
	require.NoError(t, model.DB.Exec("DELETE FROM channel_model_cost_rules").Error)
	rules, err := ActiveCostRules(candidates, false)
	require.NoError(t, err)
	assert.Contains(t, rules, CostRuleCandidate{ChannelID: 7, BillableUpstreamModel: "cached-model"})
}

func TestActiveCostRulesBatchEmptyCandidatesReturnsEmpty(t *testing.T) {
	prepareCostRuleServiceDB(t)
	rules, err := ActiveCostRules(nil, true)
	require.NoError(t, err)
	assert.Empty(t, rules)
}

func seedActiveCostRuleRow(t *testing.T, channelID int, model string, mode types.CostMode, version int, now *int64) {
	t.Helper()
	seedCostRuleRowWithStatus(t, channelID, model, mode, version, string(types.CostRuleActive), now)
}

func seedCostRuleRowWithStatus(t *testing.T, channelID int, billableModel string, mode types.CostMode, version int, status string, now *int64) {
	t.Helper()
	configJSON, err := common.Marshal(types.CostRuleConfigV1{ZeroCostReason: "fixture"})
	require.NoError(t, err)
	require.NoError(t, model.DB.Create(&model.ChannelModelCostRule{
		ChannelID: channelID, BillableUpstreamModel: billableModel, Version: version,
		Status: status, CostMode: string(mode), SchemaVersion: 1,
		ConfigJSON: string(configJSON), Source: "manual", EffectiveFrom: now,
		CreatedAt: *now, UpdatedAt: *now,
	}).Error)
}
