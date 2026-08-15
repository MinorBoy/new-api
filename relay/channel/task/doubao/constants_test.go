package doubao

import (
	"testing"

	"github.com/QuantumNous/new-api/pkg/seedancepricing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSeedanceResolutionCapabilityMatrix(t *testing.T) {
	tests := []struct {
		name       string
		model      string
		resolution string
		want       bool
	}{
		{"2.0 720p", "doubao-seedance-2-0-260128", "720p", true},
		{"2.0 1080p", "doubao-seedance-2-0-260128", "1080p", true},
		{"2.0 4k", "doubao-seedance-2-0-260128", "4K", true},
		{"fast 480p", "doubao-seedance-2-0-fast-260128", "480p", true},
		{"mini exact", "doubao-seedance-2-0-mini-260615", "720p", true},
		{"mini future suffix", "doubao-seedance-2-0-mini-270101", "480p", true},
		{"mini rejects 1080p", "doubao-seedance-2-0-mini-260615", "1080p", false},
		{"fast rejects 4k", "doubao-seedance-2-0-fast-260128", "4k", false},
		{"unknown resolution", "doubao-seedance-2-0-260128", "2k", false},
		{"unknown model", "other-model", "720p", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, seedancepricing.SupportsResolution(tt.model, tt.resolution))
		})
	}
	require.Contains(t, ModelList, "doubao-seedance-2-0-mini-260615")
	require.Contains(t, ModelList, "doubao-seedance-2-5-260628")
}

func TestSeedancePricing15ProRatios(t *testing.T) {
	ratios, ok := GetSeedance15ProRatios(true, true, "default")
	require.True(t, ok)
	assert.InDelta(t, 2, ratios["audio"], 1e-9)
	assert.InDelta(t, 0.6, ratios["draft_estimate"], 1e-9)

	ratios, ok = GetSeedance15ProRatios(false, false, "flex")
	require.True(t, ok)
	assert.InDelta(t, 0.5, ratios["service_tier"], 1e-9)
	assert.NotContains(t, ratios, "audio")
	assert.NotContains(t, ratios, "draft_estimate")

	_, ok = GetSeedance15ProRatios(true, false, "unsupported")
	assert.False(t, ok)
}
