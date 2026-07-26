package service

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReferenceAudioDurationSumsExactlyFifteenSeconds(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/wav")
		if r.URL.Path == "/a.wav" {
			_, _ = w.Write(testPCM16WAV(t, 7000))
			return
		}
		_, _ = w.Write(testPCM16WAV(t, 8000))
	}))
	t.Cleanup(server.Close)
	tempDir := t.TempDir()
	resolver := &httpReferenceAudioDurationResolver{client: server.Client(), maxBytes: referenceAudioMaxBytes, tempDir: tempDir}

	duration, err := resolver.ResolveMS(context.Background(), []string{server.URL + "/a.wav", server.URL + "/b.wav"})
	require.NoError(t, err)
	assert.Equal(t, int64(15000), duration)
	entries, err := os.ReadDir(tempDir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestReferenceAudioDurationRejectsFifteenSecondsAndOneMillisecond(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "audio/wav")
		_, _ = w.Write(testPCM16WAV(t, 15001))
	}))
	t.Cleanup(server.Close)
	resolver := &httpReferenceAudioDurationResolver{client: server.Client(), maxBytes: referenceAudioMaxBytes, tempDir: t.TempDir()}

	duration, err := resolver.ResolveMS(context.Background(), []string{server.URL + "/long.wav"})
	require.NoError(t, err)
	assert.Equal(t, int64(15001), duration)
}

func TestReferenceAudioDurationRejectsAggregateResponseLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "audio/wav")
		_, _ = w.Write(testPCM16WAV(t, 100))
	}))
	t.Cleanup(server.Close)
	resolver := &httpReferenceAudioDurationResolver{client: server.Client(), maxBytes: 64, tempDir: t.TempDir()}

	_, err := resolver.ResolveMS(context.Background(), []string{server.URL + "/audio.wav"})
	var durationErr *ReferenceAudioDurationError
	require.ErrorAs(t, err, &durationErr)
	assert.Equal(t, ReferenceAudioInvalidMedia, durationErr.Kind)
}

func TestReferenceAudioDurationClassifiesUnavailableAndInvalidMedia(t *testing.T) {
	t.Run("unknown format", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = io.WriteString(w, "not audio")
		}))
		t.Cleanup(server.Close)
		resolver := &httpReferenceAudioDurationResolver{client: server.Client(), maxBytes: referenceAudioMaxBytes, tempDir: t.TempDir()}
		_, err := resolver.ResolveMS(context.Background(), []string{server.URL + "/secret.bin?signature=do-not-leak"})
		var durationErr *ReferenceAudioDurationError
		require.ErrorAs(t, err, &durationErr)
		assert.Equal(t, ReferenceAudioInvalidMedia, durationErr.Kind)
		assert.NotContains(t, err.Error(), "do-not-leak")
		assert.NotContains(t, err.Error(), "secret.bin")
	})

	t.Run("timeout", func(t *testing.T) {
		client := &http.Client{Transport: referenceAudioRoundTripperFunc(func(req *http.Request) (*http.Response, error) {
			<-req.Context().Done()
			return nil, req.Context().Err()
		})}
		resolver := &httpReferenceAudioDurationResolver{client: client, maxBytes: referenceAudioMaxBytes, tempDir: t.TempDir()}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()
		_, err := resolver.ResolveMS(ctx, []string{"https://assets.example/secret.wav?signature=do-not-leak"})
		var durationErr *ReferenceAudioDurationError
		require.ErrorAs(t, err, &durationErr)
		assert.Equal(t, ReferenceAudioMetadataUnavailable, durationErr.Kind)
		assert.NotContains(t, err.Error(), "do-not-leak")
	})

	t.Run("blocked redirect", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Location", "http://127.0.0.1/private.wav?secret=value")
			w.WriteHeader(http.StatusFound)
		}))
		t.Cleanup(server.Close)
		client := server.Client()
		client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return errors.New("SSRF redirect blocked") }
		resolver := &httpReferenceAudioDurationResolver{client: client, maxBytes: referenceAudioMaxBytes, tempDir: t.TempDir()}
		_, err := resolver.ResolveMS(context.Background(), []string{server.URL + "/redirect"})
		var durationErr *ReferenceAudioDurationError
		require.ErrorAs(t, err, &durationErr)
		assert.Equal(t, ReferenceAudioMetadataUnavailable, durationErr.Kind)
		assert.NotContains(t, err.Error(), "private.wav")
	})
}

func TestReferenceAudioDurationWaitsForCleanupOnConcurrentFailure(t *testing.T) {
	tempDir := t.TempDir()
	slowReadStarted := make(chan struct{})
	releaseSlowRead := make(chan struct{})
	invalidReturned := make(chan struct{})
	wavBody := &blockingReferenceAudioBody{
		reader:  bytes.NewReader(testPCM16WAV(t, 1000)),
		started: slowReadStarted,
		release: releaseSlowRead,
	}
	client := &http.Client{Transport: referenceAudioRoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/slow.wav" {
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"audio/wav"}}, Body: wavBody, Request: req}, nil
		}
		<-slowReadStarted
		close(invalidReturned)
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/octet-stream"}}, Body: io.NopCloser(strings.NewReader("not audio")), Request: req}, nil
	})}
	resolver := &httpReferenceAudioDurationResolver{client: client, maxBytes: referenceAudioMaxBytes, tempDir: tempDir}
	resolved := make(chan error, 1)
	go func() {
		_, err := resolver.ResolveMS(context.Background(), []string{"https://assets.example/invalid.bin", "https://assets.example/slow.wav"})
		resolved <- err
	}()

	<-invalidReturned
	select {
	case <-resolved:
		t.Fatal("ResolveMS returned while another temporary audio file was still in use")
	default:
	}
	close(releaseSlowRead)
	require.Error(t, <-resolved)
	entries, readErr := os.ReadDir(tempDir)
	require.NoError(t, readErr)
	assert.Empty(t, entries, "all in-flight temporary files must be removed before ResolveMS returns")
}

type referenceAudioRoundTripperFunc func(*http.Request) (*http.Response, error)

func (f referenceAudioRoundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type blockingReferenceAudioBody struct {
	reader  *bytes.Reader
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (b *blockingReferenceAudioBody) Read(buffer []byte) (int, error) {
	b.once.Do(func() { close(b.started) })
	<-b.release
	return b.reader.Read(buffer)
}

func (b *blockingReferenceAudioBody) Close() error {
	return nil
}

func TestReferenceAudioDurationGlobalResolver(t *testing.T) {
	original := currentReferenceAudioDurationResolver()
	t.Cleanup(func() { SetReferenceAudioDurationResolver(original) })
	SetReferenceAudioDurationResolver(referenceAudioDurationResolverFunc(func(_ context.Context, urls []string) (int64, error) {
		assert.Equal(t, []string{"https://x/audio.wav"}, urls)
		return 1234, nil
	}))
	duration, err := ResolveReferenceAudioDurationMS(context.Background(), []string{"https://x/audio.wav"})
	require.NoError(t, err)
	assert.Equal(t, int64(1234), duration)

	SetReferenceAudioDurationResolver(nil)
	assert.NotNil(t, currentReferenceAudioDurationResolver())
}

type referenceAudioDurationResolverFunc func(context.Context, []string) (int64, error)

func (f referenceAudioDurationResolverFunc) ResolveMS(ctx context.Context, urls []string) (int64, error) {
	return f(ctx, urls)
}

func testPCM16WAV(t *testing.T, durationMS int) []byte {
	t.Helper()
	const sampleRate = 1000
	const channels = 1
	const bitsPerSample = 16
	samples := sampleRate * durationMS / 1000
	dataSize := samples * channels * bitsPerSample / 8
	var buffer bytes.Buffer
	buffer.WriteString("RIFF")
	require.NoError(t, binary.Write(&buffer, binary.LittleEndian, uint32(36+dataSize)))
	buffer.WriteString("WAVEfmt ")
	require.NoError(t, binary.Write(&buffer, binary.LittleEndian, uint32(16)))
	require.NoError(t, binary.Write(&buffer, binary.LittleEndian, uint16(1)))
	require.NoError(t, binary.Write(&buffer, binary.LittleEndian, uint16(channels)))
	require.NoError(t, binary.Write(&buffer, binary.LittleEndian, uint32(sampleRate)))
	require.NoError(t, binary.Write(&buffer, binary.LittleEndian, uint32(sampleRate*channels*bitsPerSample/8)))
	require.NoError(t, binary.Write(&buffer, binary.LittleEndian, uint16(channels*bitsPerSample/8)))
	require.NoError(t, binary.Write(&buffer, binary.LittleEndian, uint16(bitsPerSample)))
	buffer.WriteString("data")
	require.NoError(t, binary.Write(&buffer, binary.LittleEndian, uint32(dataSize)))
	_, err := buffer.Write(make([]byte, dataSize))
	require.NoError(t, err)
	return buffer.Bytes()
}

func TestReferenceAudioDurationUsesSafeExtensionSources(t *testing.T) {
	tests := []struct {
		name               string
		urlPath            string
		contentType        string
		contentDisposition string
	}{
		{name: "content type", urlPath: "/audio", contentType: "audio/wav"},
		{name: "content disposition", urlPath: "/audio", contentType: "application/octet-stream", contentDisposition: `attachment; filename="clip.wav"`},
		{name: "URL path", urlPath: "/clip.wav", contentType: "application/octet-stream"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", tt.contentType)
				if tt.contentDisposition != "" {
					w.Header().Set("Content-Disposition", tt.contentDisposition)
				}
				_, _ = w.Write(testPCM16WAV(t, 1000))
			}))
			t.Cleanup(server.Close)
			tempDir := t.TempDir()
			resolver := &httpReferenceAudioDurationResolver{client: server.Client(), maxBytes: referenceAudioMaxBytes, tempDir: tempDir}
			duration, err := resolver.ResolveMS(context.Background(), []string{server.URL + tt.urlPath})
			require.NoError(t, err)
			assert.Equal(t, int64(1000), duration)
			matches, err := filepath.Glob(filepath.Join(tempDir, "*"))
			require.NoError(t, err)
			assert.Empty(t, matches)
		})
	}
}
