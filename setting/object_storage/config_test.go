package object_storage

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testEnabledConfig() ObjectStorageConfig {
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.Endpoint = "https://s3.example.com"
	cfg.PublicEndpoint = "https://cdn.example.com"
	cfg.Bucket = "videos"
	cfg.AccessKeyID = "access"
	cfg.SecretAccessKey = "secret"
	return cfg
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	assert.False(t, cfg.Enabled)
	assert.Equal(t, "us-east-1", cfg.Region)
	assert.Equal(t, 512, cfg.MaxVideoSizeMB)
	assert.Equal(t, 86400, cfg.ExpiresSeconds)
}

func TestValidateConfigRequiresCredentialsWhenEnabled(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*ObjectStorageConfig)
		wantErr string
	}{
		{"disabled allows draft", func(c *ObjectStorageConfig) { c.Enabled = false }, ""},
		{"missing endpoint", func(c *ObjectStorageConfig) { c.Endpoint = "" }, "endpoint"},
		{"missing public endpoint", func(c *ObjectStorageConfig) { c.PublicEndpoint = "" }, "public_endpoint"},
		{"missing bucket", func(c *ObjectStorageConfig) { c.Bucket = "" }, "bucket"},
		{"missing access key", func(c *ObjectStorageConfig) { c.AccessKeyID = "" }, "access_key_id"},
		{"missing secret", func(c *ObjectStorageConfig) { c.SecretAccessKey = "" }, "secret_access_key"},
		{"expires too short", func(c *ObjectStorageConfig) { c.ExpiresSeconds = 59 }, "expires_seconds"},
		{"expires too long", func(c *ObjectStorageConfig) { c.ExpiresSeconds = 604801 }, "expires_seconds"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := testEnabledConfig()
			tt.mutate(&cfg)
			err := ValidateConfig(cfg)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestValidateConfigAcceptsBoundaries(t *testing.T) {
	for _, size := range []int{1, 2048} {
		cfg := testEnabledConfig()
		cfg.MaxVideoSizeMB = size
		require.NoError(t, ValidateConfig(cfg))
	}
	for _, expires := range []int{60, 604800} {
		cfg := testEnabledConfig()
		cfg.ExpiresSeconds = expires
		require.NoError(t, ValidateConfig(cfg))
	}
}

func TestNormalizeConfigTrimsAndDefaults(t *testing.T) {
	cfg := ObjectStorageConfig{
		Enabled:                   false,
		Region:                    " ",
		MaxVideoSizeMB:            0,
		ExpiresSeconds:            0,
		TransferDomainWhitelist:   []string{" Own.Example.com. ", ""},
		NoTransferDomainBlacklist: []string{" CDN.Example.com:443 ", ""},
	}
	normalized := NormalizeConfig(cfg)
	assert.Equal(t, "us-east-1", normalized.Region)
	assert.Equal(t, 512, normalized.MaxVideoSizeMB)
	assert.Equal(t, 86400, normalized.ExpiresSeconds)
	assert.Equal(t, []string{"own.example.com"}, normalized.TransferDomainWhitelist)
	assert.Equal(t, []string{"cdn.example.com"}, normalized.NoTransferDomainBlacklist)
}

func TestNormalizeConfigMigratesLegacyDomainLists(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TransferMode = ""
	cfg.TransferDomainWhitelist = []string{"Own.Example.com"}
	cfg.NoTransferDomainBlacklist = []string{"CDN.Example.com"}

	got := NormalizeConfig(cfg)
	require.Equal(t, TransferModeRules, got.TransferMode)
	require.True(t, got.WhitelistEnabled)
	require.True(t, got.BlacklistEnabled)
}

func TestNormalizeConfigDefaultsToNoTransfer(t *testing.T) {
	got := NormalizeConfig(DefaultConfig())
	require.Equal(t, TransferModeDefault, got.TransferMode)
	require.False(t, got.WhitelistEnabled)
	require.False(t, got.BlacklistEnabled)
}

func TestNormalizeConfigPreservesExplicitModeAndRules(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TransferMode = TransferModeAll
	cfg.WhitelistEnabled = true
	cfg.TransferDomainWhitelist = []string{"provider.example.com"}

	got := NormalizeConfig(cfg)
	require.Equal(t, TransferModeAll, got.TransferMode)
	require.True(t, got.WhitelistEnabled)
	require.Equal(t, []string{"provider.example.com"}, got.TransferDomainWhitelist)
}

func TestNormalizeConfigRejectsUnknownModeByDefaulting(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TransferMode = "unsupported"

	require.Equal(t, TransferModeDefault, NormalizeConfig(cfg).TransferMode)
}
