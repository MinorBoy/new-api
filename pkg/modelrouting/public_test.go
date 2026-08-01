package modelrouting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsPublicModelRequiresExactCanonicalID(t *testing.T) {
	for _, modelName := range CanonicalModels {
		assert.True(t, IsPublicModel(modelName), modelName)
	}
	for _, modelName := range []string{
		"doubao-seedance-2-0-mini-260128",
		"mg-seedance2.0-480p-fast-gz-15s",
		" " + Seedance20,
		Seedance20 + " ",
		"",
	} {
		assert.False(t, IsPublicModel(modelName), modelName)
	}
}

func TestFilterPublicModelsUsesCatalogOrderAndDeduplicates(t *testing.T) {
	actual := FilterPublicModels([]string{
		"provider-hidden", Seedance20Mini, Seedance20, Seedance20Mini, Seedance20Fast,
	})
	require.Equal(t, []string{Seedance20, Seedance20Fast, Seedance20Mini}, actual)
}
