package objectstorage

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShouldTransfer(t *testing.T) {
	tests := []struct {
		name      string
		url       string
		whitelist []string
		blacklist []string
		want      bool
	}{
		{"whitelist exact", "https://OWN.Example.com/a.mp4", []string{"own.example.com"}, nil, true},
		{"blacklist wins", "https://own.example.com/a.mp4", []string{"own.example.com"}, []string{"own.example.com"}, false},
		{"wildcard child", "https://cdn.example.com/a.mp4", []string{"*.example.com"}, nil, true},
		{"wildcard excludes root", "https://example.com/a.mp4", []string{"*.example.com"}, nil, false},
		{"default skip", "https://other.example/a.mp4", nil, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ShouldTransfer(tt.url, tt.whitelist, tt.blacklist)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestShouldTransferNormalizesHostAndRejectsInvalidURL(t *testing.T) {
	got, err := ShouldTransfer("https://CDN.Example.com.:443/a.mp4", []string{"cdn.example.com"}, nil)
	require.NoError(t, err)
	assert.True(t, got)

	for _, rawURL := range []string{"", "/relative.mp4", "ftp://example.com/video.mp4"} {
		_, err := ShouldTransfer(rawURL, nil, nil)
		assert.Error(t, err)
	}
}
