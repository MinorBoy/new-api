package videometa

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequestValidateAcceptsBoundedHTTPSVideo(t *testing.T) {
	request := Request{
		URL:        "https://assets.example/video.mp4",
		MediaType:  "video",
		MaxBytes:   MaxVideoBytes,
		DeadlineMS: MaxDeadlineMS,
	}

	require.NoError(t, request.Validate())
}

func TestRequestValidateRejectsUnsafeLimits(t *testing.T) {
	tests := []struct {
		name    string
		request Request
	}{
		{
			name: "unsupported scheme",
			request: Request{
				URL: "file:///etc/passwd", MediaType: "video", MaxBytes: MaxVideoBytes, DeadlineMS: MaxDeadlineMS,
			},
		},
		{
			name: "missing host",
			request: Request{
				URL: "https:///video.mp4", MediaType: "video", MaxBytes: MaxVideoBytes, DeadlineMS: MaxDeadlineMS,
			},
		},
		{
			name: "wrong media type",
			request: Request{
				URL: "https://assets.example/video.mp4", MediaType: "audio", MaxBytes: MaxVideoBytes, DeadlineMS: MaxDeadlineMS,
			},
		},
		{
			name: "zero byte limit",
			request: Request{
				URL: "https://assets.example/video.mp4", MediaType: "video", MaxBytes: 0, DeadlineMS: MaxDeadlineMS,
			},
		},
		{
			name: "oversized byte limit",
			request: Request{
				URL: "https://assets.example/video.mp4", MediaType: "video", MaxBytes: MaxVideoBytes + 1, DeadlineMS: MaxDeadlineMS,
			},
		},
		{
			name: "zero deadline",
			request: Request{
				URL: "https://assets.example/video.mp4", MediaType: "video", MaxBytes: MaxVideoBytes, DeadlineMS: 0,
			},
		},
		{
			name: "oversized deadline",
			request: Request{
				URL: "https://assets.example/video.mp4", MediaType: "video", MaxBytes: MaxVideoBytes, DeadlineMS: MaxDeadlineMS + 1,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Error(t, test.request.Validate())
		})
	}
}

func TestRequestValidateDoesNotExposeRawURL(t *testing.T) {
	request := Request{
		URL:        "https://assets.example/video.mp4?signature=secret-value",
		MediaType:  "audio",
		MaxBytes:   MaxVideoBytes,
		DeadlineMS: MaxDeadlineMS,
	}

	err := request.Validate()
	require.Error(t, err)
	assert.NotContains(t, err.Error(), request.URL)
	assert.False(t, strings.Contains(err.Error(), "secret-value"))
}

func TestMetadataValidateAcceptsBoundedMP4(t *testing.T) {
	metadata := Metadata{
		DurationMS:    5_200,
		Width:         1_280,
		Height:        720,
		FrameRateNum:  24,
		FrameRateDen:  1,
		Container:     "mp4",
		ContentLength: 1_834_210,
	}

	require.NoError(t, metadata.Validate())
}

func TestMetadataValidateRejectsInvalidFields(t *testing.T) {
	valid := Metadata{
		DurationMS:    5_200,
		Width:         1_280,
		Height:        720,
		FrameRateNum:  24,
		FrameRateDen:  1,
		Container:     "mov",
		ContentLength: 1_834_210,
	}
	tests := []struct {
		name   string
		mutate func(*Metadata)
	}{
		{name: "zero duration", mutate: func(value *Metadata) { value.DurationMS = 0 }},
		{name: "zero width", mutate: func(value *Metadata) { value.Width = 0 }},
		{name: "oversized width", mutate: func(value *Metadata) { value.Width = MaxDimension + 1 }},
		{name: "zero height", mutate: func(value *Metadata) { value.Height = 0 }},
		{name: "oversized height", mutate: func(value *Metadata) { value.Height = MaxDimension + 1 }},
		{name: "zero frame numerator", mutate: func(value *Metadata) { value.FrameRateNum = 0 }},
		{name: "zero frame denominator", mutate: func(value *Metadata) { value.FrameRateDen = 0 }},
		{name: "frame rate too high", mutate: func(value *Metadata) { value.FrameRateNum = MaxFrameRate * 1_001; value.FrameRateDen = 1_000 }},
		{name: "unsupported container", mutate: func(value *Metadata) { value.Container = "webm" }},
		{name: "negative content length", mutate: func(value *Metadata) { value.ContentLength = -1 }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			metadata := valid
			test.mutate(&metadata)
			assert.Error(t, metadata.Validate())
		})
	}
}
