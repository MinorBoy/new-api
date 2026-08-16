package objectstorage

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShouldTransfer(t *testing.T) {
	tests := []struct {
		name             string
		url              string
		mode             string
		whitelistEnabled bool
		blacklistEnabled bool
		defaultTransfer  bool
		whitelist        []string
		blacklist        []string
		want             bool
	}{
		{"default skips", "https://other.example/a.mp4", "default", false, false, false, nil, nil, false},
		{"all transfers without rules", "https://other.example/a.mp4", "all", false, false, false, nil, nil, true},
		{"whitelist exact", "https://OWN.Example.com/a.mp4", "rules", true, false, false, []string{"own.example.com"}, nil, true},
		{"whitelist unmatched", "https://other.example/a.mp4", "rules", true, false, false, []string{"own.example.com"}, nil, false},
		{"whitelist unmatched defaults to transfer", "https://other.example/a.mp4", "rules", true, false, true, []string{"own.example.com"}, nil, true},
		{"blacklist listed", "https://own.example.com/a.mp4", "rules", false, true, true, nil, []string{"own.example.com"}, false},
		{"blacklist unmatched defaults to skip", "https://other.example/a.mp4", "rules", false, true, false, nil, []string{"own.example.com"}, false},
		{"blacklist unmatched defaults to transfer", "https://other.example/a.mp4", "rules", false, true, true, nil, []string{"own.example.com"}, true},
		{"blacklist wins over default transfer", "https://own.example.com/a.mp4", "rules", false, true, true, nil, []string{"own.example.com"}, false},
		{"blacklist wins", "https://own.example.com/a.mp4", "rules", true, true, true, []string{"own.example.com"}, []string{"own.example.com"}, false},
		{"both rules disabled defaults to skip", "https://other.example.com/a.mp4", "rules", false, false, false, []string{"own.example.com"}, []string{"blocked.example.com"}, false},
		{"both rules disabled defaults to transfer", "https://other.example.com/a.mp4", "rules", false, false, true, []string{"own.example.com"}, []string{"blocked.example.com"}, true},
		{"wildcard child", "https://cdn.example.com/a.mp4", "rules", true, false, false, []string{"*.example.com"}, nil, true},
		{"wildcard excludes root", "https://example.com/a.mp4", "rules", true, false, false, []string{"*.example.com"}, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ShouldTransfer(
				tt.url,
				tt.mode,
				tt.whitelistEnabled,
				tt.blacklistEnabled,
				tt.defaultTransfer,
				tt.whitelist,
				tt.blacklist,
			)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestShouldTransferNormalizesHostAndRejectsInvalidURL(t *testing.T) {
	got, err := ShouldTransfer(
		"https://CDN.Example.com.:443/a.mp4",
		"rules",
		true,
		false,
		false,
		[]string{"cdn.example.com"},
		nil,
	)
	require.NoError(t, err)
	assert.True(t, got)

	for _, rawURL := range []string{"", "/relative.mp4", "ftp://example.com/video.mp4"} {
		_, err := ShouldTransfer(rawURL, "all", false, false, false, nil, nil)
		assert.Error(t, err)
	}
}
