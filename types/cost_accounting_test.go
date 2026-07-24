package types_test

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCostModesExposeCompleteContract(t *testing.T) {
	modes := []types.CostMode{
		types.CostModeFree,
		types.CostModePerRequest,
		types.CostModePerDuration,
		types.CostModePerToken,
	}

	assert.Equal(t, []types.CostMode{"free", "per_request", "per_duration", "per_token"}, modes)
}

func TestCostMeterPreservesExplicitZeroAndMissingValues(t *testing.T) {
	zero := int64(0)
	meter := types.CostMeter{
		Source:       types.CostMeterUpstreamUsage,
		InputTokens:  &zero,
		OutputTokens: nil,
	}

	data, err := common.Marshal(meter)
	require.NoError(t, err)
	assert.JSONEq(t, `{"source":"upstream_usage","input_tokens":0}`, string(data))
}
