package videometa

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetcherUsesHeadMetadataAndCachesParsedAsset(t *testing.T) {
	payload, err := os.ReadFile("testdata/sample.mp4")
	require.NoError(t, err)
	var heads atomic.Int32
	var gets atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("ETag", `"sample-v1"`)
		writer.Header().Set("Last-Modified", "Sat, 25 Jul 2026 10:00:00 GMT")
		writer.Header().Set("Content-Length", strconv.Itoa(len(payload)))
		switch request.Method {
		case http.MethodHead:
			heads.Add(1)
		case http.MethodGet:
			gets.Add(1)
			_, _ = writer.Write(payload)
		default:
			writer.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	t.Cleanup(server.Close)

	tempDir := t.TempDir()
	fetcher := NewFetcher(FetcherOptions{
		Client:   server.Client(),
		Cache:    NewCache(4),
		MaxBytes: MaxVideoBytes,
		TempDir:  tempDir,
	})
	request := Request{URL: server.URL + "/sample.mp4", MediaType: "video", MaxBytes: MaxVideoBytes, DeadlineMS: 1_000}

	first, err := fetcher.Metadata(context.Background(), request)
	require.NoError(t, err)
	second, err := fetcher.Metadata(context.Background(), request)
	require.NoError(t, err)

	assert.Equal(t, first.DurationMS, second.DurationMS)
	assert.Equal(t, first.ContentLength, second.ContentLength)
	assert.False(t, first.CacheHit)
	assert.True(t, second.CacheHit)
	assert.Equal(t, `"sample-v1"`, first.ETag)
	assert.Equal(t, int32(2), heads.Load())
	assert.Equal(t, int32(1), gets.Load())
	assertTempDirectoryEmpty(t, tempDir)
}

func TestFetcherUsesShortTTLForSignedURL(t *testing.T) {
	payload, err := os.ReadFile("testdata/sample.mp4")
	require.NoError(t, err)
	var gets atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("ETag", `"sample-v1"`)
		writer.Header().Set("Content-Length", strconv.Itoa(len(payload)))
		if request.Method == http.MethodGet {
			gets.Add(1)
			_, _ = writer.Write(payload)
		}
	}))
	t.Cleanup(server.Close)

	now := time.Date(2026, time.July, 25, 10, 0, 0, 0, time.UTC)
	cache := newCache(4, func() time.Time { return now })
	fetcher := NewFetcher(FetcherOptions{
		Client:            server.Client(),
		Cache:             cache,
		MaxBytes:          MaxVideoBytes,
		TempDir:           t.TempDir(),
		CacheTTL:          10 * time.Minute,
		SignedURLCacheTTL: time.Minute,
	})
	signed := Request{URL: server.URL + "/sample.mp4?signature=secret", MediaType: "video", MaxBytes: MaxVideoBytes, DeadlineMS: 1_000}
	unsigned := Request{URL: server.URL + "/sample.mp4", MediaType: "video", MaxBytes: MaxVideoBytes, DeadlineMS: 1_000}

	_, err = fetcher.Metadata(context.Background(), signed)
	require.NoError(t, err)
	now = now.Add(61 * time.Second)
	_, err = fetcher.Metadata(context.Background(), signed)
	require.NoError(t, err)
	assert.Equal(t, int32(2), gets.Load())

	_, err = fetcher.Metadata(context.Background(), unsigned)
	require.NoError(t, err)
	now = now.Add(61 * time.Second)
	_, err = fetcher.Metadata(context.Background(), unsigned)
	require.NoError(t, err)
	assert.Equal(t, int32(3), gets.Load())
}

func TestFetcherRejectsDeclaredOversizedAssetWithoutGet(t *testing.T) {
	var gets atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodGet {
			gets.Add(1)
		}
		writer.Header().Set("Content-Length", "1025")
	}))
	t.Cleanup(server.Close)

	fetcher := NewFetcher(FetcherOptions{Client: server.Client(), MaxBytes: 1_024, TempDir: t.TempDir()})
	request := Request{URL: server.URL + "/video.mp4", MediaType: "video", MaxBytes: 1_024, DeadlineMS: 1_000}

	_, err := fetcher.Metadata(context.Background(), request)

	assertServiceErrorCode(t, err, ErrorMediaTooLarge)
	assert.Zero(t, gets.Load())
}

func TestFetcherStopsAtActualByteLimitAndCleansTempFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodHead {
			writer.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		_, _ = writer.Write([]byte(strings.Repeat("x", 1_025)))
	}))
	t.Cleanup(server.Close)

	tempDir := t.TempDir()
	fetcher := NewFetcher(FetcherOptions{Client: server.Client(), MaxBytes: 1_024, TempDir: tempDir})
	request := Request{URL: server.URL + "/video.mp4", MediaType: "video", MaxBytes: 1_024, DeadlineMS: 1_000}

	_, err := fetcher.Metadata(context.Background(), request)

	assertServiceErrorCode(t, err, ErrorMediaTooLarge)
	assertTempDirectoryEmpty(t, tempDir)
}

func TestFetcherCleansTempFileWhenBodyReadIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method == http.MethodHead {
			return &http.Response{StatusCode: http.StatusMethodNotAllowed, Body: http.NoBody, Header: make(http.Header), Request: request}, nil
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       &cancelingBody{cancel: cancel},
			Header:     make(http.Header),
			Request:    request,
		}, nil
	})}
	tempDir := t.TempDir()
	fetcher := NewFetcher(FetcherOptions{Client: client, MaxBytes: MaxVideoBytes, TempDir: tempDir})
	request := Request{URL: "https://assets.example/video.mp4", MediaType: "video", MaxBytes: MaxVideoBytes, DeadlineMS: 1_000}

	_, err := fetcher.Metadata(ctx, request)

	require.Error(t, err)
	assertTempDirectoryEmpty(t, tempDir)
}

func TestFetcherErrorDoesNotExposeRawURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(server.Close)
	rawURL := server.URL + "/video.mp4?signature=secret-value"
	fetcher := NewFetcher(FetcherOptions{Client: server.Client(), MaxBytes: MaxVideoBytes, TempDir: t.TempDir()})
	request := Request{URL: rawURL, MediaType: "video", MaxBytes: MaxVideoBytes, DeadlineMS: 1_000}

	_, err := fetcher.Metadata(context.Background(), request)

	require.Error(t, err)
	assert.NotContains(t, err.Error(), rawURL)
	assert.NotContains(t, err.Error(), "secret-value")
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

type cancelingBody struct {
	cancel context.CancelFunc
	read   bool
}

func (body *cancelingBody) Read(buffer []byte) (int, error) {
	if body.read {
		return 0, context.Canceled
	}
	body.read = true
	count := copy(buffer, "partial video")
	body.cancel()
	return count, nil
}

func (body *cancelingBody) Close() error {
	return nil
}

func assertServiceErrorCode(t *testing.T, err error, expected ErrorCode) {
	t.Helper()
	var serviceError *ServiceError
	require.ErrorAs(t, err, &serviceError)
	assert.Equal(t, expected, serviceError.Code)
}

func assertTempDirectoryEmpty(t *testing.T, directory string) {
	t.Helper()
	entries, err := os.ReadDir(directory)
	require.NoError(t, err)
	assert.Empty(t, entries, fmt.Sprintf("temporary directory %s should be empty", directory))
}

var _ io.ReadCloser = (*cancelingBody)(nil)
