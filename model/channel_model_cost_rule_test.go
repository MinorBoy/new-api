package model

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelModelCostRuleVersionIsUnique(t *testing.T) {
	prepareCostAccountingDB(t)

	first := validCostRule(1, types.CostRuleDraft)
	require.NoError(t, CreateCostRuleDraft(&first))

	duplicate := validCostRule(1, types.CostRuleDraft)
	require.Error(t, CreateCostRuleDraft(&duplicate))

	nextVersion := validCostRule(2, types.CostRuleDraft)
	require.NoError(t, CreateCostRuleDraft(&nextVersion))
}

func TestActivateChannelModelCostRuleRetiresPreviousVersionAtomically(t *testing.T) {
	prepareCostAccountingDB(t)

	active := validCostRule(1, types.CostRuleActive)
	active.EffectiveFrom = costInt64Pointer(100)
	require.NoError(t, DB.Create(&active).Error)
	draft := validCostRule(2, types.CostRuleDraft)
	require.NoError(t, CreateCostRuleDraft(&draft))

	activated, err := ActivateChannelModelCostRule(draft.ID, 42, 200, func(rule *ChannelModelCostRule) error {
		assert.Equal(t, draft.ID, rule.ID)
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, string(types.CostRuleActive), activated.Status)
	assert.Equal(t, 42, activated.ActivatedBy)
	assert.Equal(t, int64(200), *activated.EffectiveFrom)

	require.NoError(t, DB.First(&active, active.ID).Error)
	assert.Equal(t, string(types.CostRuleRetired), active.Status)
	assert.Equal(t, int64(200), *active.EffectiveTo)
}

func TestActivateChannelModelCostRuleRollsBackWhenValidationFails(t *testing.T) {
	prepareCostAccountingDB(t)

	draft := validCostRule(1, types.CostRuleDraft)
	require.NoError(t, CreateCostRuleDraft(&draft))

	validationErr := errors.New("invalid rule")
	_, err := ActivateChannelModelCostRule(draft.ID, 42, 200, func(*ChannelModelCostRule) error {
		return validationErr
	})
	assert.ErrorIs(t, err, validationErr)

	require.NoError(t, DB.First(&draft, draft.ID).Error)
	assert.Equal(t, string(types.CostRuleDraft), draft.Status)
	assert.Nil(t, draft.EffectiveFrom)
}

func validCostRule(version int, status types.CostRuleStatus) ChannelModelCostRule {
	return ChannelModelCostRule{
		ChannelID:             7,
		BillableUpstreamModel: "vendor-model",
		Version:               version,
		Status:                string(status),
		CostMode:              string(types.CostModePerRequest),
		SchemaVersion:         1,
		ConfigJSON:            `{"unit_price":"0.2"}`,
		Source:                "manual",
		CreatedBy:             11,
	}
}
