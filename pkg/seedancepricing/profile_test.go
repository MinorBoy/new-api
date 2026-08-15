package seedancepricing

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProfileReturnsCanonicalDimensionsAndFrameRate(t *testing.T) {
	tests := []struct {
		name       string
		resolution string
		width      int
		height     int
	}{
		{"480p landscape", "480p", 864, 496},
		{"720p landscape", "720p", 1280, 720},
		{"1080p landscape", "1080p", 1920, 1080},
		{"4k landscape", "4k", 3840, 2160},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile, ok := Profile(tt.resolution)
			require.True(t, ok)
			assert.Equal(t, tt.resolution, profile.Name)
			assert.Equal(t, tt.width, profile.Width)
			assert.Equal(t, tt.height, profile.Height)
			assert.Equal(t, int64(24), profile.FrameRateNum)
			assert.Equal(t, int64(1), profile.FrameRateDen)
		})
	}
}

func TestProfileNormalizesAndRejectsUnknown(t *testing.T) {
	upper, ok := Profile(" 1080P ")
	require.True(t, ok)
	assert.Equal(t, 1920, upper.Width)

	// An empty resolution defaults to Seedance's documented 720p default.
	empty, ok := Profile("")
	require.True(t, ok)
	assert.Equal(t, "720p", empty.Name)

	for _, resolution := range []string{"2k", "1440p", "8k"} {
		_, ok := Profile(resolution)
		assert.False(t, ok, "resolution %q should be unsupported", resolution)
	}
}

func TestSupportsResolutionUsesCapabilityContractWithoutPricing(t *testing.T) {
	tests := []struct {
		model      string
		resolution string
		want       bool
	}{
		{"doubao-seedance-2-0-260128", "480p", true},
		{"doubao-seedance-2-0-260128", "1080p", true},
		{"doubao-seedance-2-0-260128", "4K", true},
		{"doubao-seedance-2-0-fast-260128", "720p", true},
		{"doubao-seedance-2-0-fast-260128", "1080p", false},
		{"doubao-seedance-2-0-mini-260615", "480p", true},
		{"doubao-seedance-2-0-mini-260615", "4k", false},
		{"doubao-seedance-2-5-260628", "480p", true},
		{"doubao-seedance-2-5-260628", "720p", true},
		{"doubao-seedance-2-5-260628", "1080p", false},
		{"doubao-seedance-1-5-pro-251215", "1080p", true},
		{"other-model", "720p", false},
	}
	for _, tt := range tests {
		t.Run(tt.model+"/"+tt.resolution, func(t *testing.T) {
			assert.Equal(t, tt.want, SupportsResolution(tt.model, tt.resolution))
		})
	}
}

func TestFamilyResolvesFromCanonicalModelNames(t *testing.T) {
	tests := []struct {
		model string
		want  string
	}{
		{"doubao-seedance-2-0-fast-260128", Family20Fast},
		{"doubao-seedance-2-0-mini-260615", Family20Mini},
		{"doubao-seedance-2-0-260128", Family20},
		{"doubao-seedance-1-5-pro-251215", Family15Pro},
		{"doubao-seedance-2-0-fast-270101", Family20Fast},
		{"doubao-seedance-2-5-260628", Family25},
		{"other-model", ""},
	}
	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			assert.Equal(t, tt.want, Family(tt.model))
		})
	}
}

func TestFamilyResolvesProviderSeedanceAliases(t *testing.T) {
	tests := []struct {
		model string
		want  string
	}{
		{"jmg-video-seedance-2.0-fast-vip", Family20Fast},
		{"jmg-video-seedance-2.0-mini", Family20Mini},
		{"jmg-video-seedance-2.0-vip", Family20},
		{"pxv-seedance-2.0-standard", Family20},
		{"jimeng-video-seedance-2.0-vip", Family20},
		{"unrelated-video-model", ""},
	}
	for _, test := range tests {
		t.Run(test.model, func(t *testing.T) {
			assert.Equal(t, test.want, Family(test.model))
		})
	}
}
