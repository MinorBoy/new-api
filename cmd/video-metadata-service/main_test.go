package main

import (
	"net"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/pkg/videometa"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var metadataEnvironmentKeys = []string{
	"VIDEO_METADATA_LISTEN_ADDR",
	"VIDEO_METADATA_SERVICE_TOKEN",
	"VIDEO_METADATA_MAX_BYTES",
	"VIDEO_METADATA_TIMEOUT_SECONDS",
	"VIDEO_METADATA_MAX_CONCURRENCY",
	"VIDEO_METADATA_CACHE_ENTRIES",
	"VIDEO_METADATA_CACHE_TTL_SECONDS",
	"VIDEO_METADATA_SIGNED_URL_CACHE_TTL_SECONDS",
}

func TestLoadConfigDefaults(t *testing.T) {
	clearMetadataEnvironment(t)
	t.Setenv("VIDEO_METADATA_SERVICE_TOKEN", "secret")

	config, err := loadConfig()

	require.NoError(t, err)
	assert.Equal(t, ":8090", config.ListenAddr)
	assert.Equal(t, videometa.MaxVideoBytes, config.MaxBytes)
	assert.Equal(t, 30*time.Second, config.Timeout)
	assert.Equal(t, 16, config.MaxConcurrency)
	assert.Equal(t, 10_000, config.CacheEntries)
	assert.Equal(t, 10*time.Minute, config.CacheTTL)
	assert.Equal(t, time.Minute, config.SignedURLCacheTTL)
}

func TestLoadConfigRejectsInvalidEnvironment(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T)
	}{
		{name: "empty token", mutate: func(t *testing.T) { t.Setenv("VIDEO_METADATA_SERVICE_TOKEN", "   ") }},
		{name: "invalid max bytes", mutate: func(t *testing.T) { t.Setenv("VIDEO_METADATA_MAX_BYTES", "many") }},
		{name: "zero max bytes", mutate: func(t *testing.T) { t.Setenv("VIDEO_METADATA_MAX_BYTES", "0") }},
		{name: "oversized max bytes", mutate: func(t *testing.T) { t.Setenv("VIDEO_METADATA_MAX_BYTES", "134217729") }},
		{name: "oversized timeout", mutate: func(t *testing.T) { t.Setenv("VIDEO_METADATA_TIMEOUT_SECONDS", "31") }},
		{name: "zero concurrency", mutate: func(t *testing.T) { t.Setenv("VIDEO_METADATA_MAX_CONCURRENCY", "0") }},
		{name: "negative cache entries", mutate: func(t *testing.T) { t.Setenv("VIDEO_METADATA_CACHE_ENTRIES", "-1") }},
		{name: "zero cache ttl", mutate: func(t *testing.T) { t.Setenv("VIDEO_METADATA_CACHE_TTL_SECONDS", "0") }},
		{name: "signed ttl exceeds cache ttl", mutate: func(t *testing.T) {
			t.Setenv("VIDEO_METADATA_CACHE_TTL_SECONDS", "60")
			t.Setenv("VIDEO_METADATA_SIGNED_URL_CACHE_TTL_SECONDS", "61")
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			clearMetadataEnvironment(t)
			t.Setenv("VIDEO_METADATA_SERVICE_TOKEN", "secret")
			test.mutate(t)

			_, err := loadConfig()

			assert.Error(t, err)
		})
	}
}

func TestMetadataProtectionBlocksPrivateNetworksAndUnexpectedPorts(t *testing.T) {
	protection, err := newMetadataProtection()
	require.NoError(t, err)

	assert.NoError(t, protection.ValidateNetworkTarget("assets.example", 443))
	assert.Error(t, protection.ValidateNetworkTarget("assets.example", 8080))
	assert.Error(t, protection.ValidateNetworkTarget("127.0.0.1", 80))
	assert.Error(t, protection.ValidateResolvedIP("assets.example", net.ParseIP("10.0.0.1")))
	assert.True(t, protection.ApplyIPFilterForDomain)
}

func clearMetadataEnvironment(t *testing.T) {
	t.Helper()
	for _, key := range metadataEnvironmentKeys {
		t.Setenv(key, "")
	}
}
