package doubao

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSeedancePricingMatrix(t *testing.T) {
	tests := []struct {
		name       string
		model      string
		resolution string
		hasVideo   bool
		want       float64
		wantOK     bool
	}{
		{"2.0 720p text", "doubao-seedance-2-0-260128", "720p", false, 1, true},
		{"2.0 1080p video", "doubao-seedance-2-0-260128", "1080p", true, 31.0 / 46.0, true},
		{"2.0 4k text", "doubao-seedance-2-0-260128", "4K", false, 26.0 / 46.0, true},
		{"fast video", "doubao-seedance-2-0-fast-260128", "480p", true, 22.0 / 37.0, true},
		{"mini exact", "doubao-seedance-2-0-mini-260615", "720p", true, 14.0 / 23.0, true},
		{"mini future suffix", "doubao-seedance-2-0-mini-270101", "480p", false, 1, true},
		{"mini rejects 1080p", "doubao-seedance-2-0-mini-260615", "1080p", false, 0, false},
		{"fast rejects 4k", "doubao-seedance-2-0-fast-260128", "4k", false, 0, false},
		{"unknown resolution", "doubao-seedance-2-0-260128", "2k", false, 0, false},
		{"unknown model", "other-model", "720p", false, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := GetVideoBillingRatio(tt.model, tt.resolution, tt.hasVideo)
			assert.Equal(t, tt.wantOK, ok)
			if tt.wantOK {
				assert.InDelta(t, tt.want, got, 1e-9)
			}
		})
	}
	require.Contains(t, ModelList, "doubao-seedance-2-0-mini-260615")
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
