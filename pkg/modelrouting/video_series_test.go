package modelrouting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMiniMaxH3ContractIsIndependentFromSeedance(t *testing.T) {
	contract, ok := MiniMaxH3Contract(MiniMaxH3VIP)
	require.True(t, ok)
	assert.Equal(t, MiniMaxH3, contract.Series)
	assert.Equal(t, []string{"720p", "768p", "2k"}, contract.OutputResolutions)
	// The verified provider dialect supports 21:9 for text/reference modes
	// (Auto only in first/last-frame mode); it appears across channel rows.
	assert.Equal(t,
		[]string{"auto", "1:1", "16:9", "9:16", "21:9", "3:4", "4:3"},
		contract.AspectRatios)
	assert.Equal(t, 4, contract.MinDurationSeconds)
	assert.Equal(t, 15, contract.MaxDurationSeconds)
	assert.False(t, IsPublicSeedanceModel(MiniMaxH3))
	assert.NotEqual(t, MiniMaxH3, SeedanceSeriesContractForModel(MiniMaxH3).Series)
}

func TestMiniMaxH3CanonicalIdentityIsCaseInsensitive(t *testing.T) {
	// Downstream clients address MiniMax-H3; routing policies, caches and task
	// facts key on the lowercase canonical identity.
	for _, modelName := range []string{"MiniMax-H3", "MINIMAX-H3", "  minimax-h3  "} {
		assert.Equal(t, MiniMaxH3, NormalizeCanonicalModel(modelName), modelName)
		assert.True(t, IsMiniMaxH3Canonical(modelName), modelName)
		assert.True(t, IsCanonicalVideoModel(modelName), modelName)
	}
	// Provider upstream IDs share the H3 contract but are never canonical, or a
	// client could address a provider model directly and bypass routing.
	for _, modelName := range []string{MiniMaxH3VIP, "MiniMax-H3-VIP", "minimax-h3-768p", ""} {
		assert.False(t, IsMiniMaxH3Canonical(modelName), modelName)
		assert.False(t, IsCanonicalVideoModel(modelName), modelName)
	}
	assert.True(t, IsMiniMaxH3Model(MiniMaxH3VIP))
}

func TestVideoSeriesContractRejectsUnknownFutureModel(t *testing.T) {
	_, ok := VideoSeriesContractForModel("wan-video-future")
	assert.False(t, ok)
}
