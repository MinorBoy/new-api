package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskPrivateDataValueRetainsUserResponseAuditData(t *testing.T) {
	privateData := TaskPrivateData{UserResponseData: []byte(`{"id":"task-public"}`)}

	value, err := privateData.Value()

	require.NoError(t, err)
	require.NotNil(t, value)
	storedData, ok := value.([]byte)
	require.True(t, ok)
	var stored map[string]any
	require.NoError(t, common.Unmarshal(storedData, &stored))
	assert.Equal(t, "task-public", stored["user_response_data"].(map[string]any)["id"])
}

func TestTaskPrivateDataValueRetainsSeedanceUsageSnapshot(t *testing.T) {
	privateData := TaskPrivateData{BillingContext: &TaskBillingContext{
		InputVideoDurationMS: 2500,
		UpstreamCostMode:     string(types.CostModePerDuration),
	}}

	value, err := privateData.Value()

	require.NoError(t, err)
	require.NotNil(t, value)
	storedData, ok := value.([]byte)
	require.True(t, ok)
	var decoded TaskPrivateData
	require.NoError(t, common.Unmarshal(storedData, &decoded))
	require.NotNil(t, decoded.BillingContext)
	assert.Equal(t, int64(2500), decoded.BillingContext.InputVideoDurationMS)
	assert.Equal(t, string(types.CostModePerDuration), decoded.BillingContext.UpstreamCostMode)
}
