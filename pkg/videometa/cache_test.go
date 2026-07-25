package videometa

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCacheKeyHashesSensitiveURL(t *testing.T) {
	input := CacheKeyInput{
		URL:           "https://assets.example/video.mp4?signature=secret-value",
		ETag:          `"version-1"`,
		LastModified:  "Sat, 25 Jul 2026 10:00:00 GMT",
		ContentLength: 8_278,
	}

	key := CacheKey(input)

	assert.Len(t, key, 64)
	assert.NotContains(t, key, "assets.example")
	assert.NotContains(t, key, "secret-value")
	assert.Equal(t, key, CacheKey(input))
	input.ETag = `"version-2"`
	assert.NotEqual(t, key, CacheKey(input))
}

func TestCacheExpiresEntriesWithoutSleeping(t *testing.T) {
	now := time.Date(2026, time.July, 25, 10, 0, 0, 0, time.UTC)
	cache := newCache(2, func() time.Time { return now })
	metadata := Metadata{DurationMS: 1_000, Width: 320, Height: 180, FrameRateNum: 10, FrameRateDen: 1, Container: "mp4", ContentLength: 8_278}
	cache.Set("asset", metadata, time.Minute)

	stored, ok := cache.Get("asset")
	require.True(t, ok)
	assert.Equal(t, metadata, stored)

	now = now.Add(time.Minute)
	_, ok = cache.Get("asset")
	assert.False(t, ok)
}

func TestCacheEvictsLeastRecentlyUsedEntry(t *testing.T) {
	cache := NewCache(2)
	metadata := Metadata{DurationMS: 1_000, Width: 320, Height: 180, FrameRateNum: 10, FrameRateDen: 1, Container: "mp4", ContentLength: 8_278}
	cache.Set("first", metadata, time.Minute)
	cache.Set("second", metadata, time.Minute)
	_, ok := cache.Get("first")
	require.True(t, ok)

	cache.Set("third", metadata, time.Minute)

	_, firstOK := cache.Get("first")
	_, secondOK := cache.Get("second")
	_, thirdOK := cache.Get("third")
	assert.True(t, firstOK)
	assert.False(t, secondOK)
	assert.True(t, thirdOK)
}

func TestCacheWithZeroCapacityStoresNothing(t *testing.T) {
	cache := NewCache(0)
	cache.Set("asset", Metadata{}, time.Minute)

	_, ok := cache.Get("asset")
	assert.False(t, ok)
}
