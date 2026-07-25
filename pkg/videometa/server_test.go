package videometa

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServerHealthCheckDoesNotRequireAuthentication(t *testing.T) {
	handler := NewServer(ServerOptions{
		Token:          "service-secret",
		MaxConcurrency: 1,
		Metadata: func(context.Context, Request) (Metadata, error) {
			require.FailNow(t, "health check must not invoke metadata")
			return Metadata{}, nil
		},
	})
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.JSONEq(t, `{"status":"ok"}`, recorder.Body.String())
}

func TestServerRequiresExactBearerToken(t *testing.T) {
	handler := NewServer(ServerOptions{
		Token:          "service-secret",
		MaxConcurrency: 1,
		Metadata: func(context.Context, Request) (Metadata, error) {
			require.FailNow(t, "unauthorized request must not invoke metadata")
			return Metadata{}, nil
		},
	})
	tests := []string{"", "Bearer", "Bearer wrong", "Basic service-secret", "bearer service-secret"}

	for _, authorization := range tests {
		t.Run(authorization, func(t *testing.T) {
			request := validMetadataRequest(t)
			if authorization != "" {
				request.Header.Set("Authorization", authorization)
			}
			recorder := httptest.NewRecorder()

			handler.ServeHTTP(recorder, request)

			assert.Equal(t, http.StatusUnauthorized, recorder.Code)
			assert.Contains(t, recorder.Body.String(), string(ErrorUnauthorized))
		})
	}
}

func TestServerWithEmptyTokenRejectsAllRequests(t *testing.T) {
	handler := NewServer(ServerOptions{
		MaxConcurrency: 1,
		Metadata: func(context.Context, Request) (Metadata, error) {
			require.FailNow(t, "empty token configuration must not authenticate")
			return Metadata{}, nil
		},
	})
	request := validMetadataRequest(t)
	request.Header["Authorization"] = []string{"Bearer "}
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusUnauthorized, recorder.Code)
}

func TestServerRejectsOversizedAndTrailingJSON(t *testing.T) {
	handler := NewServer(ServerOptions{
		Token:          "service-secret",
		MaxConcurrency: 1,
		Metadata: func(context.Context, Request) (Metadata, error) {
			require.FailNow(t, "invalid request must not invoke metadata")
			return Metadata{}, nil
		},
	})
	tests := []struct {
		name       string
		body       string
		statusCode int
	}{
		{name: "oversized", body: `{"url":"https://assets.example/` + strings.Repeat("x", int(MaxRequestBodyBytes)) + `","media_type":"video","max_bytes":1,"deadline_ms":1}`, statusCode: http.StatusRequestEntityTooLarge},
		{name: "trailing JSON", body: `{"url":"https://assets.example/video.mp4","media_type":"video","max_bytes":1,"deadline_ms":1}{}`, statusCode: http.StatusBadRequest},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/v1/metadata/video", strings.NewReader(test.body))
			request.Header.Set("Authorization", "Bearer service-secret")
			request.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()

			handler.ServeHTTP(recorder, request)

			assert.Equal(t, test.statusCode, recorder.Code)
		})
	}
}

func TestServerLimitsConcurrentMetadataRequests(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	handler := NewServer(ServerOptions{
		Token:          "service-secret",
		MaxConcurrency: 1,
		Metadata: func(context.Context, Request) (Metadata, error) {
			close(entered)
			<-release
			return validMetadata(), nil
		},
	})
	firstRequest := validMetadataRequest(t)
	firstRequest.Header.Set("Authorization", "Bearer service-secret")
	firstRecorder := httptest.NewRecorder()
	var wait sync.WaitGroup
	wait.Add(1)
	go func() {
		defer wait.Done()
		handler.ServeHTTP(firstRecorder, firstRequest)
	}()
	<-entered

	secondRequest := validMetadataRequest(t)
	secondRequest.Header.Set("Authorization", "Bearer service-secret")
	secondRecorder := httptest.NewRecorder()
	handler.ServeHTTP(secondRecorder, secondRequest)
	close(release)
	wait.Wait()

	assert.Equal(t, http.StatusServiceUnavailable, secondRecorder.Code)
	assert.Contains(t, secondRecorder.Body.String(), string(ErrorConcurrencyLimited))
	assert.Equal(t, http.StatusOK, firstRecorder.Code)
}

func TestServerMapsStableServiceErrors(t *testing.T) {
	tests := []struct {
		name       string
		metadata   func(context.Context, Request) (Metadata, error)
		statusCode int
		code       ErrorCode
	}{
		{
			name: "invalid media",
			metadata: func(context.Context, Request) (Metadata, error) {
				return Metadata{}, newServiceError(ErrorUnsupportedFormat, http.StatusBadRequest, fmt.Errorf("private parser detail"))
			},
			statusCode: http.StatusBadRequest,
			code:       ErrorUnsupportedFormat,
		},
		{
			name: "fetch unavailable",
			metadata: func(context.Context, Request) (Metadata, error) {
				return Metadata{}, newServiceError(ErrorFetchUnavailable, http.StatusServiceUnavailable, fmt.Errorf("private network detail"))
			},
			statusCode: http.StatusServiceUnavailable,
			code:       ErrorFetchUnavailable,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler := NewServer(ServerOptions{Token: "service-secret", MaxConcurrency: 1, Metadata: test.metadata})
			request := validMetadataRequest(t)
			request.Header.Set("Authorization", "Bearer service-secret")
			recorder := httptest.NewRecorder()

			handler.ServeHTTP(recorder, request)

			assert.Equal(t, test.statusCode, recorder.Code)
			assert.Contains(t, recorder.Body.String(), string(test.code))
			assert.NotContains(t, recorder.Body.String(), "private")
		})
	}
}

func TestServerReturnsStableMetadataEnvelope(t *testing.T) {
	handler := NewServer(ServerOptions{
		Token:          "service-secret",
		MaxConcurrency: 2,
		Metadata: func(context.Context, Request) (Metadata, error) {
			return validMetadata(), nil
		},
	})
	request := validMetadataRequest(t)
	request.Header.Set("Authorization", "Bearer service-secret")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.JSONEq(t, `{"duration_ms":5200,"width":1280,"height":720,"frame_rate_num":24,"frame_rate_den":1,"container":"mp4","content_length":1834210}`, recorder.Body.String())
}

func TestServerLogsOnlyAllowlistedFields(t *testing.T) {
	var message string
	var fields map[string]any
	handler := NewServer(ServerOptions{
		Token:          "service-secret",
		MaxConcurrency: 1,
		Metadata: func(context.Context, Request) (Metadata, error) {
			return validMetadata(), nil
		},
		Log: func(logMessage string, logFields map[string]any) {
			message = logMessage
			fields = logFields
		},
	})
	request := validMetadataRequest(t)
	request.Header.Set("Authorization", "Bearer service-secret")
	request.Header.Set("X-Request-ID", "https://assets.example/input.mp4?signature=request-id-secret")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	require.NotEmpty(t, message)
	require.NotNil(t, fields)
	for key := range fields {
		assert.Contains(t, []string{"request_id", "result_code", "elapsed_ms", "bytes", "cache_hit"}, key)
	}
	combined := fmt.Sprintf("%s %v", message, fields)
	assert.NotContains(t, combined, "signature=private")
	assert.NotContains(t, combined, "request-id-secret")
	assert.NotContains(t, combined, "service-secret")
}

func TestMetadataServiceEndToEndCachesValidatedAsset(t *testing.T) {
	payload, err := os.ReadFile("testdata/sample.mp4")
	require.NoError(t, err)
	var gets atomic.Int32
	asset := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("ETag", `"sample-v1"`)
		writer.Header().Set("Content-Length", strconv.Itoa(len(payload)))
		if request.Method == http.MethodGet {
			gets.Add(1)
			_, _ = writer.Write(payload)
		}
	}))
	t.Cleanup(asset.Close)
	var cacheHits []bool
	handler := NewServer(ServerOptions{
		Token:          "service-secret",
		MaxConcurrency: 2,
		Metadata: NewFetcher(FetcherOptions{
			Client: asset.Client(), Cache: NewCache(4), MaxBytes: MaxVideoBytes, TempDir: t.TempDir(),
		}).Metadata,
		Log: func(_ string, fields map[string]any) {
			cacheHit, ok := fields["cache_hit"].(bool)
			require.True(t, ok)
			cacheHits = append(cacheHits, cacheHit)
		},
	})
	body := fmt.Sprintf(`{"url":%q,"media_type":"video","max_bytes":134217728,"deadline_ms":30000}`, asset.URL+"/sample.mp4")

	first := postMetadata(t, handler, body)
	second := postMetadata(t, handler, body)

	assert.Equal(t, http.StatusOK, first.Code)
	assert.Equal(t, http.StatusOK, second.Code)
	assert.Equal(t, int32(1), gets.Load())
	assert.Equal(t, []bool{false, true}, cacheHits)
	assert.NotContains(t, second.Body.String(), "cache_hit")
}

func postMetadata(t *testing.T, handler http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/v1/metadata/video", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer service-secret")
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func validMetadataRequest(t *testing.T) *http.Request {
	t.Helper()
	body := `{"url":"https://assets.example/input.mp4?signature=private","media_type":"video","max_bytes":134217728,"deadline_ms":30000}`
	request := httptest.NewRequest(http.MethodPost, "/v1/metadata/video", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	return request
}

func validMetadata() Metadata {
	return Metadata{
		DurationMS: 5_200, Width: 1_280, Height: 720, FrameRateNum: 24, FrameRateDen: 1,
		Container: "mp4", ContentLength: 1_834_210,
	}
}
