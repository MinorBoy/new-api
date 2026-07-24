package taskcommon

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBaseBillingConfirmsMappedTaskCostIdentity(t *testing.T) {
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: " provider-model "}}

	require.NoError(t, (BaseBilling{}).ConfirmTaskCostIdentity(info))
	assert.Equal(t, "provider-model", info.BillableUpstreamModel)
	require.Error(t, (BaseBilling{}).ConfirmTaskCostIdentity(&relaycommon.RelayInfo{}))
}

func TestBaseBillingTaskCostCapabilities(t *testing.T) {
	capabilities := (BaseBilling{}).CostCapabilities(nil)

	assert.True(t, capabilities.CanResolveBillableModel)
	assert.ElementsMatch(t, []types.CostChargeEvent{
		types.CostChargeSubmitAccepted,
		types.CostChargeTaskSucceeded,
	}, capabilities.ChargeEvents)
	assert.Empty(t, capabilities.MeterSources)
}

func TestBaseBillingNormalizesExplicitTaskCostMeter(t *testing.T) {
	duration := "6"
	want := types.CostMeter{Source: types.CostMeterUpstreamActual, DurationSeconds: &duration}

	got, err := (BaseBilling{}).NormalizeTaskCostMeter(&model.Task{}, &relaycommon.TaskInfo{CostMeter: &want})

	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestBaseBillingNormalizesAuthoritativeTaskUsageAndPreservesZero(t *testing.T) {
	meter, err := (BaseBilling{}).NormalizeTaskCostMeter(&model.Task{}, &relaycommon.TaskInfo{
		CompletionTokensPresent: true,
		TotalTokensPresent:      true,
	})

	require.NoError(t, err)
	assert.Equal(t, types.CostMeterUpstreamUsage, meter.Source)
	require.NotNil(t, meter.CompletionTokens)
	assert.Zero(t, *meter.CompletionTokens)
	require.NotNil(t, meter.TotalTokens)
	assert.Zero(t, *meter.TotalTokens)
}

func TestBaseBillingRejectsMissingOrUnboundedTaskUsage(t *testing.T) {
	_, err := (BaseBilling{}).NormalizeTaskCostMeter(&model.Task{}, &relaycommon.TaskInfo{TotalTokens: 42})
	require.Error(t, err)

	_, err = (BaseBilling{}).NormalizeTaskCostMeter(&model.Task{}, &relaycommon.TaskInfo{
		TotalTokens:        common.MaxQuota + 1,
		TotalTokensPresent: true,
	})
	require.Error(t, err)
}
