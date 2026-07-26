package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTaskMediaURL(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		kind      TaskMediaURLKind
		fetchable bool
	}{
		{name: "https", value: "https://assets.example/video.mp4?sig=secret", kind: TaskMediaURLHTTP, fetchable: true},
		{name: "data base64", value: "data:image/png;base64,QUJDRA==", kind: TaskMediaURLData},
		{name: "asset", value: "asset://video-reference-1", kind: TaskMediaURLAsset},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := ParseTaskMediaURL(tt.value)
			require.NoError(t, err)
			assert.Equal(t, tt.value, parsed.Value)
			assert.Equal(t, tt.kind, parsed.Kind)
			assert.Equal(t, tt.fetchable, parsed.FetchableHTTP())
		})
	}
}

func TestParseTaskMediaURLRejectsMalformedValues(t *testing.T) {
	for _, value := range []string{
		"", "ftp://assets.example/video.mp4", "https:///video.mp4",
		"data:image/png,not-base64", "data:image/png;base64,", "asset://",
	} {
		_, err := ParseTaskMediaURL(value)
		require.Error(t, err, value)
	}
}
