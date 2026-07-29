package cost_setting

import (
	"testing"

	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func withCostSettingConfig(t *testing.T, values map[string]string) {
	t.Helper()

	original := costSetting
	t.Cleanup(func() {
		costSetting = original
		UpdateAndSync()
	})

	require.NoError(t, config.UpdateConfigFromMap(&costSetting, values))
	UpdateAndSync()
}

func TestCostSettingDefaultsDisabled(t *testing.T) {
	withCostSettingConfig(t, map[string]string{})

	assert.Equal(t, types.CostAccountingDisabled, Runtime().Mode)
	assert.Zero(t, Runtime().MinimumExpectedMarginBPS)
}

func TestCostSettingAcceptsStrictMode(t *testing.T) {
	withCostSettingConfig(t, map[string]string{KeyMode: string(types.CostAccountingStrict)})

	assert.Equal(t, types.CostAccountingStrict, Runtime().Mode)
}

func TestCostSettingAcceptsTrackingMode(t *testing.T) {
	withCostSettingConfig(t, map[string]string{KeyMode: string(types.CostAccountingTracking)})

	assert.Equal(t, types.CostAccountingTracking, Runtime().Mode)
}

func TestCostSettingRejectsUnsupportedModes(t *testing.T) {
	for _, mode := range []types.CostAccountingMode{"", "permissive", "enabled"} {
		t.Run(string(mode), func(t *testing.T) {
			require.Error(t, ValidateMode(mode))
		})
	}
}

func TestCostSettingInvalidRuntimeValueFallsBackToDisabled(t *testing.T) {
	withCostSettingConfig(t, map[string]string{KeyMode: "permissive"})

	assert.Equal(t, types.CostAccountingDisabled, Runtime().Mode)
}

func TestCostSettingExportsFlatModeKey(t *testing.T) {
	withCostSettingConfig(t, map[string]string{
		KeyMode:                     string(types.CostAccountingStrict),
		KeyMinimumExpectedMarginBPS: "1250",
	})

	exported, err := config.ConfigToMap(&costSetting)
	require.NoError(t, err)
	assert.Equal(t, string(types.CostAccountingStrict), exported[KeyMode])
	assert.Equal(t, "1250", exported[KeyMinimumExpectedMarginBPS])
}

func TestValidateMinimumExpectedMarginBPS(t *testing.T) {
	assert.NoError(t, ValidateMinimumExpectedMarginBPS(0))
	assert.NoError(t, ValidateMinimumExpectedMarginBPS(10_000))
	assert.Error(t, ValidateMinimumExpectedMarginBPS(-1))
	assert.Error(t, ValidateMinimumExpectedMarginBPS(10_001))
}

func TestRuntimePreservesExplicitZeroMargin(t *testing.T) {
	original := costSetting
	t.Cleanup(func() {
		costSetting = original
		UpdateAndSync()
	})

	costSetting = CostSetting{
		Mode:                     types.CostAccountingStrict,
		MinimumExpectedMarginBPS: 0,
	}
	UpdateAndSync()

	assert.Equal(t, types.CostAccountingStrict, Runtime().Mode)
	assert.Zero(t, Runtime().MinimumExpectedMarginBPS)
}

func TestWithRuntimeReadLockBlocksRuntimeWriter(t *testing.T) {
	withCostSettingConfig(t, map[string]string{
		KeyMode:                     string(types.CostAccountingStrict),
		KeyMinimumExpectedMarginBPS: "1250",
	})

	require.NoError(t, WithRuntimeReadLock(func(snapshot RuntimeSnapshot) error {
		assert.Equal(t, 1250, snapshot.MinimumExpectedMarginBPS)
		assert.False(t, runtimeUpdateGuard.TryLock(), "runtime updates must wait for a profit snapshot guard")
		return nil
	}))
	require.True(t, runtimeUpdateGuard.TryLock())
	runtimeUpdateGuard.Unlock()
}

func TestInvalidRuntimeMarginFallsBackToZero(t *testing.T) {
	withCostSettingConfig(t, map[string]string{
		KeyMode:                     string(types.CostAccountingStrict),
		KeyMinimumExpectedMarginBPS: "10001",
	})

	assert.Equal(t, types.CostAccountingStrict, Runtime().Mode)
	assert.Zero(t, Runtime().MinimumExpectedMarginBPS)
}
