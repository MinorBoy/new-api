package videometa

import (
	"bytes"
	"errors"
	"math"
	"os"
	"testing"

	"github.com/abema/go-mp4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The media fixtures are copied from github.com/abema/go-mp4 v1.4.1/testdata
// and remain covered by that module's MIT license.
func TestParseVideoMetadataMP4(t *testing.T) {
	file, err := os.Open("testdata/sample.mp4")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, file.Close()) })

	metadata, err := Parse(file, 8_278)
	require.NoError(t, err)
	assert.Equal(t, int64(1_000), metadata.DurationMS)
	assert.Equal(t, 320, metadata.Width)
	assert.Equal(t, 180, metadata.Height)
	assert.Equal(t, int64(10), metadata.FrameRateNum)
	assert.Equal(t, int64(1), metadata.FrameRateDen)
	assert.Equal(t, "mp4", metadata.Container)
	assert.Equal(t, int64(8_278), metadata.ContentLength)
}

func TestParseVideoMetadataQuickTime(t *testing.T) {
	file, err := os.Open("testdata/sample_qt.mp4")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, file.Close()) })

	metadata, err := Parse(file, 340_481)
	require.NoError(t, err)
	assert.Equal(t, int64(596_459), metadata.DurationMS)
	assert.Equal(t, 424, metadata.Width)
	assert.Equal(t, 240, metadata.Height)
	assert.Equal(t, int64(24), metadata.FrameRateNum)
	assert.Equal(t, int64(1), metadata.FrameRateDen)
	assert.Equal(t, "mov", metadata.Container)
	assert.Equal(t, int64(340_481), metadata.ContentLength)
}

func TestParseVideoMetadataRejectsBrokenContainer(t *testing.T) {
	reader := bytes.NewReader([]byte("not-an-iso-bmff-file"))

	_, err := Parse(reader, int64(reader.Len()))

	assert.ErrorIs(t, err, ErrUnsupportedContainer)
}

func TestFrameRateReducesTimingRatio(t *testing.T) {
	value := &mp4.Stts{Entries: []mp4.SttsEntry{{SampleCount: 10, SampleDelta: 1_024}}}

	numerator, denominator, err := frameRate(value, 10_240)

	require.NoError(t, err)
	assert.Equal(t, int64(10), numerator)
	assert.Equal(t, int64(1), denominator)
}

func TestFrameRatePreservesFractionalRate(t *testing.T) {
	value := &mp4.Stts{Entries: []mp4.SttsEntry{{SampleCount: 1, SampleDelta: 1_001}}}

	numerator, denominator, err := frameRate(value, 30_000)

	require.NoError(t, err)
	assert.Equal(t, int64(30_000), numerator)
	assert.Equal(t, int64(1_001), denominator)
}

func TestFrameRateRejectsInvalidTiming(t *testing.T) {
	tests := []struct {
		name      string
		value     *mp4.Stts
		timescale uint32
	}{
		{name: "missing table", value: nil, timescale: 1},
		{name: "empty entries", value: &mp4.Stts{}, timescale: 1},
		{name: "zero timescale", value: &mp4.Stts{Entries: []mp4.SttsEntry{{SampleCount: 1, SampleDelta: 1}}}, timescale: 0},
		{name: "zero sample count", value: &mp4.Stts{Entries: []mp4.SttsEntry{{SampleCount: 0, SampleDelta: 1}}}, timescale: 1},
		{name: "zero sample delta", value: &mp4.Stts{Entries: []mp4.SttsEntry{{SampleCount: 1, SampleDelta: 0}}}, timescale: 1},
		{
			name: "sample sum overflow",
			value: &mp4.Stts{Entries: []mp4.SttsEntry{
				{SampleCount: math.MaxUint32, SampleDelta: 1},
				{SampleCount: math.MaxUint32, SampleDelta: 1},
				{SampleCount: math.MaxUint32, SampleDelta: 1},
			}},
			timescale: math.MaxUint32,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := frameRate(test.value, test.timescale)
			assert.True(t, errors.Is(err, ErrInvalidVideoTrack))
		})
	}
}
