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
	assert.Equal(t, []string{"720p", "2k"}, contract.OutputResolutions)
	assert.Equal(t, 4, contract.MinDurationSeconds)
	assert.Equal(t, 15, contract.MaxDurationSeconds)
	assert.False(t, IsPublicSeedanceModel(MiniMaxH3))
	assert.NotEqual(t, MiniMaxH3, SeedanceSeriesContractForModel(MiniMaxH3).Series)
}

func TestVideoSeriesContractRejectsUnknownFutureModel(t *testing.T) {
	_, ok := VideoSeriesContractForModel("wan-video-future")
	assert.False(t, ok)
}
