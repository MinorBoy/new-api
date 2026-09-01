package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/image_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImageOptionsValidateAndApplyTypedSnapshots(t *testing.T) {
	originalCatalog := image_setting.Catalog2JSONString()
	originalRouting := image_setting.Routing2JSONString()
	t.Cleanup(func() {
		require.NoError(t, image_setting.UpdateCatalogByJSONString(originalCatalog))
		require.NoError(t, image_setting.UpdateRoutingByJSONString(originalRouting))
	})
	common.OptionMapRWMutex.Lock()
	common.OptionMap = make(map[string]string)
	common.OptionMapRWMutex.Unlock()

	require.NoError(t, updateOptionMap(image_setting.CatalogOptionKey, `{"version":1,"models":{}}`))
	require.NoError(t, updateOptionMap(image_setting.RoutingOptionKey, `{"version":1,"default":{"strategy":"lowest_cost"}}`))
	assert.Equal(t, 1, image_setting.Snapshot().Version)
	assert.Equal(t, image_setting.StrategyLowestCost, image_setting.PolicyFor("", "").Strategy)

	assert.Error(t, validateOptionValue(image_setting.CatalogOptionKey, `{"version":99,"models":{}}`))
	assert.Error(t, validateOptionValue(image_setting.RoutingOptionKey, `{"version":1,"default":{"strategy":"bad"}}`))
	assert.Equal(t, image_setting.StrategyLowestCost, image_setting.PolicyFor("", "").Strategy)
	assert.Error(t, updateOptionMap(image_setting.RoutingOptionKey, `{"version":1,"default":{"strategy":"bad"}}`))
	assert.Equal(t, image_setting.StrategyLowestCost, image_setting.PolicyFor("", "").Strategy)
}
