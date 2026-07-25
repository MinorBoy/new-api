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

func TestVideoInputRatioMatchesOfficialSeedanceMatrix(t *testing.T) {
	tests := []struct {
		name       string
		model      string
		resolution string
		hasVideo   bool
		want       float64
		wantOK     bool
	}{
		{"2.0 480p text", "doubao-seedance-2-0-260128", "480p", false, 1, true},
		{"2.0 480p video", "doubao-seedance-2-0-260128", "480p", true, 28.0 / 46.0, true},
		{"2.0 720p text", "doubao-seedance-2-0-260128", "720p", false, 1, true},
		{"2.0 720p video", "doubao-seedance-2-0-260128", "720p", true, 28.0 / 46.0, true},
		{"2.0 1080p text", "doubao-seedance-2-0-260128", "1080p", false, 51.0 / 46.0, true},
		{"2.0 1080p video", "doubao-seedance-2-0-260128", "1080p", true, 31.0 / 46.0, true},
		{"2.0 4k text", "doubao-seedance-2-0-260128", "4k", false, 26.0 / 46.0, true},
		{"2.0 4k video", "doubao-seedance-2-0-260128", "4k", true, 16.0 / 46.0, true},
		{"fast 480p text", "doubao-seedance-2-0-fast-260128", "480p", false, 1, true},
		{"fast 480p video", "doubao-seedance-2-0-fast-260128", "480p", true, 22.0 / 37.0, true},
		{"fast 720p video", "doubao-seedance-2-0-fast-260128", "720p", true, 22.0 / 37.0, true},
		{"mini 480p text", "doubao-seedance-2-0-mini-260615", "480p", false, 1, true},
		{"mini 720p video", "doubao-seedance-2-0-mini-260615", "720p", true, 14.0 / 23.0, true},
		{"mini future suffix", "doubao-seedance-2-0-mini-270101", "480p", false, 1, true},
		{"2.0 future suffix video", "doubao-seedance-2-0-271231", "1080p", true, 31.0 / 46.0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := VideoInputRatio(tt.model, tt.resolution, tt.hasVideo)
			assert.Equal(t, tt.wantOK, ok)
			if tt.wantOK {
				assert.InDelta(t, tt.want, got, 1e-9)
			}
		})
	}
}

func TestVideoInputRatioRejectsUnsupportedCombinations(t *testing.T) {
	tests := []struct {
		name       string
		model      string
		resolution string
		hasVideo   bool
	}{
		{"fast rejects 1080p", "doubao-seedance-2-0-fast-260128", "1080p", false},
		{"fast rejects 4k", "doubao-seedance-2-0-fast-260128", "4k", true},
		{"mini rejects 1080p", "doubao-seedance-2-0-mini-260615", "1080p", false},
		{"mini rejects 4k", "doubao-seedance-2-0-mini-260615", "4k", true},
		{"unknown resolution", "doubao-seedance-2-0-260128", "2k", false},
		{"unknown model", "other-model", "720p", false},
		{"1.5 pro has no video ratio", "doubao-seedance-1-5-pro-251215", "720p", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ok := VideoInputRatio(tt.model, tt.resolution, tt.hasVideo)
			assert.False(t, ok)
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
		{"other-model", ""},
	}
	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			assert.Equal(t, tt.want, Family(tt.model))
		})
	}
}

func TestVideoInputRatioNormalizesResolution(t *testing.T) {
	got, ok := VideoInputRatio("doubao-seedance-2-0-260128", " 4K ", true)
	require.True(t, ok)
	assert.InDelta(t, 16.0/46.0, got, 1e-9)

	got, ok = VideoInputRatio("doubao-seedance-2-0-260128", "", false)
	require.True(t, ok)
	assert.InDelta(t, 1.0, got, 1e-9)
}
