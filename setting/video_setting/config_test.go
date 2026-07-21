package video_setting

import (
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/setting/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func withVideoSettingConfig(t *testing.T, values map[string]string) {
	t.Helper()

	original := videoSetting
	t.Cleanup(func() {
		videoSetting = original
		UpdateAndSync()
	})

	require.NoError(t, config.UpdateConfigFromMap(&videoSetting, values))
	UpdateAndSync()
}

func TestRuntimeDefaultsDisableBase64AndLimitJSONBody(t *testing.T) {
	withVideoSettingConfig(t, map[string]string{})

	snapshot := Runtime()

	assert.False(t, snapshot.Base64InputEnabled)
	assert.Equal(t, DefaultJSONRequestBodyMaxMB, snapshot.JSONRequestBodyMaxMB)
	assert.Equal(t, int64(DefaultJSONRequestBodyMaxMB)<<20, snapshot.JSONRequestBodyMaxBytes)
}

func TestRuntimeAcceptsConfiguredValues(t *testing.T) {
	withVideoSettingConfig(t, map[string]string{
		KeyBase64InputEnabled:   "true",
		KeyJSONRequestBodyMaxMB: "32",
	})

	snapshot := Runtime()

	assert.True(t, snapshot.Base64InputEnabled)
	assert.Equal(t, 32, snapshot.JSONRequestBodyMaxMB)
	assert.Equal(t, int64(32)<<20, snapshot.JSONRequestBodyMaxBytes)
}

func TestRuntimeFallsBackForInvalidJSONBodyLimit(t *testing.T) {
	for _, value := range []string{"0", "-1", "129", "999"} {
		t.Run(value, func(t *testing.T) {
			withVideoSettingConfig(t, map[string]string{
				KeyJSONRequestBodyMaxMB: value,
			})

			snapshot := Runtime()

			assert.Equal(t, DefaultJSONRequestBodyMaxMB, snapshot.JSONRequestBodyMaxMB)
			assert.Equal(t, int64(DefaultJSONRequestBodyMaxMB)<<20, snapshot.JSONRequestBodyMaxBytes)
		})
	}
}

func TestValidateJSONRequestBodyMaxMB(t *testing.T) {
	for _, value := range []int{MinJSONRequestBodyMaxMB, 16, MaxJSONRequestBodyMaxMB} {
		require.NoError(t, ValidateJSONRequestBodyMaxMB(value))
	}

	for _, value := range []int{0, -1, MaxJSONRequestBodyMaxMB + 1} {
		require.Error(t, ValidateJSONRequestBodyMaxMB(value))
	}
}

func TestConfigExportsFlatVideoSettingKeys(t *testing.T) {
	withVideoSettingConfig(t, map[string]string{
		KeyBase64InputEnabled:   "true",
		KeyJSONRequestBodyMaxMB: "24",
	})

	exported, err := config.ConfigToMap(&videoSetting)
	require.NoError(t, err)

	assert.Equal(t, "true", exported[KeyBase64InputEnabled])
	assert.Equal(t, strconv.Itoa(24), exported[KeyJSONRequestBodyMaxMB])
}
