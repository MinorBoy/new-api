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
}

func TestCostSettingAcceptsStrictMode(t *testing.T) {
	withCostSettingConfig(t, map[string]string{KeyMode: string(types.CostAccountingStrict)})

	assert.Equal(t, types.CostAccountingStrict, Runtime().Mode)
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
	withCostSettingConfig(t, map[string]string{KeyMode: string(types.CostAccountingStrict)})

	exported, err := config.ConfigToMap(&costSetting)
	require.NoError(t, err)
	assert.Equal(t, string(types.CostAccountingStrict), exported[KeyMode])
}
