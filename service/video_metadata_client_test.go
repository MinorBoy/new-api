package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/modelrouting"
	"github.com/QuantumNous/new-api/pkg/videometa"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newVideoMetadataClientForTest(t *testing.T, baseURL string) *httpVideoMetadataClient {
	t.Helper()
	return &httpVideoMetadataClient{
		baseURL:    baseURL,
		token:      "service-secret",
		httpClient: &http.Client{Timeout: 2 * time.Second},
		maxBytes:   videometa.MaxVideoBytes,
	}
}

func metadataSuccessBody() string {
	return `{"duration_ms":5200,"width":1280,"height":720,"frame_rate_num":24,"frame_rate_den":1,"container":"mp4","content_length":1834210}`
}

func TestVideoMetadataClientReturnsValidatedMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/metadata/video", r.URL.Path)
		require.Equal(t, "Bearer service-secret", r.Header.Get("Authorization"))
		var request videometa.Request
		require.NoError(t, common.DecodeJson(r.Body, &request))
		assert.Equal(t, "https://assets.example/input.mp4?sig=secret", request.URL)
		assert.Equal(t, "video", request.MediaType)
		assert.Equal(t, videometa.MaxVideoBytes, request.MaxBytes)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, metadataSuccessBody())
	}))
	t.Cleanup(server.Close)

	client := newVideoMetadataClientForTest(t, server.URL)
	metadata, err := client.Metadata(context.Background(), "https://assets.example/input.mp4?sig=secret")
	require.NoError(t, err)
	assert.Equal(t, int64(5200), metadata.DurationMS)
	assert.Equal(t, 1280, metadata.Width)
	assert.Equal(t, 720, metadata.Height)
	assert.Equal(t, int64(24), metadata.FrameRateNum)
	assert.Equal(t, int64(1), metadata.FrameRateDen)
	assert.Equal(t, "mp4", metadata.Container)
}

func TestVideoMetadataClientRejectsMissingOrInvalidServiceToken(t *testing.T) {
	tests := []struct {
		name   string
		client VideoMetadataClient
	}{
		{"missing token", &httpVideoMetadataClient{baseURL: "https://example.invalid", token: "", httpClient: &http.Client{}, maxBytes: videometa.MaxVideoBytes}},
		{"whitespace token", &httpVideoMetadataClient{baseURL: "https://example.invalid", token: "   ", httpClient: &http.Client{}, maxBytes: videometa.MaxVideoBytes}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.client.Metadata(context.Background(), "https://assets.example/input.mp4")
			require.Error(t, err)
			var metadataErr *VideoMetadataError
			require.ErrorAs(t, err, &metadataErr)
			assert.Equal(t, VideoMetadataUnavailable, metadataErr.Kind)
		})
	}
}

func TestVideoMetadataClientClassifiesHTTPErrors(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		body     string
		wantKind VideoMetadataErrorKind
	}{
		{"unauthorized", http.StatusUnauthorized, `{"error":{"code":"unauthorized","message":"bad token"}}`, VideoMetadataUnavailable},
		{"server error", http.StatusInternalServerError, `{"error":{"code":"internal_error","message":"boom"}}`, VideoMetadataUnavailable},
		{"bad gateway", http.StatusBadGateway, `{"error":{"code":"fetch_unavailable","message":"upstream down"}}`, VideoMetadataUnavailable},
		{"service unavailable", http.StatusServiceUnavailable, `{"error":{"code":"concurrency_limited","message":"busy"}}`, VideoMetadataUnavailable},
		{"invalid media", http.StatusBadRequest, `{"error":{"code":"metadata_invalid","message":"bad media"}}`, VideoMetadataInvalidMedia},
		{"unsupported format", http.StatusBadRequest, `{"error":{"code":"unsupported_format","message":"webm"}}`, VideoMetadataInvalidMedia},
		{"too large", http.StatusBadRequest, `{"error":{"code":"media_too_large","message":"too big"}}`, VideoMetadataInvalidMedia},
		{"invalid request from client side", http.StatusBadRequest, `{"error":{"code":"invalid_request","message":"bad"}}`, VideoMetadataInvalidMedia},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, tt.body)
			}))
			t.Cleanup(server.Close)

			client := newVideoMetadataClientForTest(t, server.URL)
			_, err := client.Metadata(context.Background(), "https://assets.example/input.mp4?sig=secret")
			require.Error(t, err)
			var metadataErr *VideoMetadataError
			require.ErrorAs(t, err, &metadataErr)
			assert.Equal(t, tt.wantKind, metadataErr.Kind)
		})
	}
}

func TestVideoMetadataClientUnavailableAndInvalidMediaAreDistinct(t *testing.T) {
	require.NotEqual(t, VideoMetadataUnavailable, VideoMetadataInvalidMedia)
	require.NotEqual(t, VideoMetadataUnavailable, VideoMetadataInvalidResponse)
	require.NotEqual(t, VideoMetadataInvalidMedia, VideoMetadataInvalidResponse)
}

func TestVideoMetadataClientTimeoutIsUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, metadataSuccessBody())
	}))
	t.Cleanup(server.Close)

	client := &httpVideoMetadataClient{
		baseURL:    server.URL,
		token:      "service-secret",
		httpClient: &http.Client{Timeout: 50 * time.Millisecond},
		maxBytes:   videometa.MaxVideoBytes,
	}
	_, err := client.Metadata(context.Background(), "https://assets.example/input.mp4")
	require.Error(t, err)
	var metadataErr *VideoMetadataError
	require.ErrorAs(t, err, &metadataErr)
	assert.Equal(t, VideoMetadataUnavailable, metadataErr.Kind)
}

func TestVideoMetadataClientMaliciousJSONIsInvalidResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{not valid json`)
	}))
	t.Cleanup(server.Close)

	client := newVideoMetadataClientForTest(t, server.URL)
	_, err := client.Metadata(context.Background(), "https://assets.example/input.mp4")
	require.Error(t, err)
	var metadataErr *VideoMetadataError
	require.ErrorAs(t, err, &metadataErr)
	assert.Equal(t, VideoMetadataInvalidResponse, metadataErr.Kind)
}

func TestVideoMetadataClientRejectsOutOfBoundsFields(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"zero duration", `{"duration_ms":0,"width":1280,"height":720,"frame_rate_num":24,"frame_rate_den":1,"container":"mp4","content_length":100}`},
		{"negative duration", `{"duration_ms":-100,"width":1280,"height":720,"frame_rate_num":24,"frame_rate_den":1,"container":"mp4","content_length":100}`},
		{"zero dimensions", `{"duration_ms":5200,"width":0,"height":0,"frame_rate_num":24,"frame_rate_den":1,"container":"mp4","content_length":100}`},
		{"oversized dimensions", `{"duration_ms":5200,"width":99999,"height":99999,"frame_rate_num":24,"frame_rate_den":1,"container":"mp4","content_length":100}`},
		{"zero frame rate", `{"duration_ms":5200,"width":1280,"height":720,"frame_rate_num":0,"frame_rate_den":1,"container":"mp4","content_length":100}`},
		{"unsupported container", `{"duration_ms":5200,"width":1280,"height":720,"frame_rate_num":24,"frame_rate_den":1,"container":"webm","content_length":100}`},
		{"negative content length", `{"duration_ms":5200,"width":1280,"height":720,"frame_rate_num":24,"frame_rate_den":1,"container":"mp4","content_length":-1}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = io.WriteString(w, tt.body)
			}))
			t.Cleanup(server.Close)

			client := newVideoMetadataClientForTest(t, server.URL)
			_, err := client.Metadata(context.Background(), "https://assets.example/input.mp4")
			require.Error(t, err)
			var metadataErr *VideoMetadataError
			require.ErrorAs(t, err, &metadataErr)
			assert.Equal(t, VideoMetadataInvalidResponse, metadataErr.Kind)
		})
	}
}

func TestVideoMetadataClientIgnoresUnknownResponseFields(t *testing.T) {
	body := `{"duration_ms":5200,"width":1280,"height":720,"frame_rate_num":24,"frame_rate_den":1,"container":"mp4","content_length":1834210,"malicious_field":"should-be-ignored","nested":{"injection":"attempt"}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)

	client := newVideoMetadataClientForTest(t, server.URL)
	metadata, err := client.Metadata(context.Background(), "https://assets.example/input.mp4")
	require.NoError(t, err)
	assert.Equal(t, int64(5200), metadata.DurationMS)
}

func TestVideoMetadataClientErrorsNeverContainSecrets(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T) (*httptest.Server, *httpVideoMetadataClient)
		url   string
	}{
		{
			name: "unauthorized echoes no token",
			setup: func(t *testing.T) (*httptest.Server, *httpVideoMetadataClient) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusUnauthorized)
					_, _ = io.WriteString(w, `{"error":{"code":"unauthorized","message":"no"}}`)
				}))
				return server, &httpVideoMetadataClient{baseURL: server.URL, token: "service-secret", httpClient: server.Client(), maxBytes: videometa.MaxVideoBytes}
			},
			url: "https://assets.example/input.mp4?sig=secret-token",
		},
		{
			name: "malformed response echoes no url",
			setup: func(t *testing.T) (*httptest.Server, *httpVideoMetadataClient) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusOK)
					_, _ = io.WriteString(w, `{bad`)
				}))
				return server, &httpVideoMetadataClient{baseURL: server.URL, token: "service-secret", httpClient: server.Client(), maxBytes: videometa.MaxVideoBytes}
			},
			url: "https://assets.example/input.mp4?sig=secret-token",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, client := tt.setup(t)
			t.Cleanup(server.Close)
			_, err := client.Metadata(context.Background(), tt.url)
			require.Error(t, err)
			msg := err.Error()
			assert.NotContains(t, msg, "service-secret")
			assert.NotContains(t, msg, "assets.example")
			assert.NotContains(t, msg, "sig=secret-token")
			assert.NotContains(t, msg, "secret-token")
		})
	}
}

type fakeMetadataClient struct {
	results map[string]videometa.Metadata
	errs    map[string]error
	Calls   *atomic.Int32
	mu      sync.Mutex
	callLog []string
}

func (f *fakeMetadataClient) Metadata(ctx context.Context, url string) (videometa.Metadata, error) {
	if f.Calls != nil {
		f.Calls.Add(1)
	}
	f.mu.Lock()
	f.callLog = append(f.callLog, url)
	f.mu.Unlock()
	if f.errs != nil {
		if err, ok := f.errs[url]; ok {
			return videometa.Metadata{}, err
		}
	}
	if f.results != nil {
		if result, ok := f.results[url]; ok {
			return result, nil
		}
	}
	return videometa.Metadata{DurationMS: 3000, Width: 1280, Height: 720, FrameRateNum: 24, FrameRateDen: 1, Container: "mp4", ContentLength: 100}, nil
}

func TestVideoMetadataLoaderUsesSyncOnce(t *testing.T) {
	calls := atomic.Int32{}
	client := &fakeMetadataClient{Calls: &calls}
	state := NewProfitRoutingRequestState(client, []string{"https://assets.example/a.mp4"}, 1)
	first, firstErr := state.Metadata(context.Background())
	second, secondErr := state.Metadata(context.Background())
	require.NoError(t, firstErr)
	require.NoError(t, secondErr)
	assert.Equal(t, int32(1), calls.Load())
	assert.Equal(t, first.TotalDurationMS, second.TotalDurationMS)
}

func TestProfitRoutingRequestStateNoURLsReturnsZeroWithoutCalling(t *testing.T) {
	calls := atomic.Int32{}
	client := &fakeMetadataClient{Calls: &calls}
	state := NewProfitRoutingRequestState(client, nil, 0)
	result, err := state.Metadata(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(0), result.TotalDurationMS)
	assert.Equal(t, int32(0), calls.Load())
	assert.False(t, result.HasReferenceVideos)
}

func TestProfitRoutingRequestStateRejectsUnfetchableReferenceVideo(t *testing.T) {
	calls := atomic.Int32{}
	state := NewProfitRoutingRequestState(&fakeMetadataClient{Calls: &calls}, nil, 1)

	assert.True(t, state.HasReferenceVideos())
	result, err := state.Metadata(context.Background())
	require.Error(t, err)
	assert.True(t, result.HasReferenceVideos)
	assert.Zero(t, result.TotalDurationMS)
	assert.Zero(t, calls.Load())
	var metadataErr *VideoMetadataError
	require.ErrorAs(t, err, &metadataErr)
	assert.Equal(t, VideoMetadataUnavailable, metadataErr.Kind)
}

func TestProfitRoutingRequestStateSumsMultipleDurations(t *testing.T) {
	calls := atomic.Int32{}
	client := &fakeMetadataClient{
		Calls: &calls,
		results: map[string]videometa.Metadata{
			"https://assets.example/a.mp4": {DurationMS: 5200, Width: 1280, Height: 720, FrameRateNum: 24, FrameRateDen: 1, Container: "mp4", ContentLength: 1},
			"https://assets.example/b.mp4": {DurationMS: 4100, Width: 1280, Height: 720, FrameRateNum: 24, FrameRateDen: 1, Container: "mp4", ContentLength: 1},
			"https://assets.example/c.mp4": {DurationMS: 1700, Width: 1280, Height: 720, FrameRateNum: 24, FrameRateDen: 1, Container: "mp4", ContentLength: 1},
		},
	}
	state := NewProfitRoutingRequestState(client, []string{
		"https://assets.example/a.mp4",
		"https://assets.example/b.mp4",
		"https://assets.example/c.mp4",
	}, 3)
	result, err := state.Metadata(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(11000), result.TotalDurationMS)
	assert.True(t, result.HasReferenceVideos)
	assert.Equal(t, int32(3), calls.Load())
}

func TestProfitRoutingRequestStateConcurrencyCappedAtThree(t *testing.T) {
	var inFlight atomic.Int32
	var maxInFlight atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		current := inFlight.Add(1)
		for {
			old := maxInFlight.Load()
			if current <= old || maxInFlight.CompareAndSwap(old, current) {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
		inFlight.Add(-1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, metadataSuccessBody())
	}))
	t.Cleanup(server.Close)

	client := newVideoMetadataClientForTest(t, server.URL)
	urls := []string{
		server.URL + "/a.mp4",
		server.URL + "/b.mp4",
		server.URL + "/c.mp4",
		server.URL + "/d.mp4",
		server.URL + "/e.mp4",
	}
	state := NewProfitRoutingRequestState(client, urls, len(urls))
	_, err := state.Metadata(context.Background())
	require.NoError(t, err)
	assert.LessOrEqual(t, maxInFlight.Load(), int32(3), "concurrent metadata fetches must not exceed 3")
}

func TestProfitRoutingRequestStateUnavailableExcludesTokenDependentCandidates(t *testing.T) {
	client := &fakeMetadataClient{
		errs: map[string]error{"https://assets.example/a.mp4": &VideoMetadataError{Kind: VideoMetadataUnavailable}},
	}
	state := NewProfitRoutingRequestState(client, []string{"https://assets.example/a.mp4"}, 1)
	_, err := state.Metadata(context.Background())
	require.Error(t, err)
	var metadataErr *VideoMetadataError
	require.ErrorAs(t, err, &metadataErr)
	assert.Equal(t, VideoMetadataUnavailable, metadataErr.Kind)
}

func TestProfitRoutingRequestStateInvalidMediaIsPermanentFailure(t *testing.T) {
	client := &fakeMetadataClient{
		errs: map[string]error{"https://assets.example/a.mp4": &VideoMetadataError{Kind: VideoMetadataInvalidMedia}},
	}
	state := NewProfitRoutingRequestState(client, []string{"https://assets.example/a.mp4"}, 1)
	_, err := state.Metadata(context.Background())
	require.Error(t, err)
	var metadataErr *VideoMetadataError
	require.ErrorAs(t, err, &metadataErr)
	assert.Equal(t, VideoMetadataInvalidMedia, metadataErr.Kind)
}

func TestProfitRoutingRequestStateCachesErrorAcrossCalls(t *testing.T) {
	calls := atomic.Int32{}
	client := &fakeMetadataClient{
		Calls: &calls,
		errs:  map[string]error{"https://assets.example/a.mp4": &VideoMetadataError{Kind: VideoMetadataUnavailable}},
	}
	state := NewProfitRoutingRequestState(client, []string{"https://assets.example/a.mp4"}, 1)
	_, firstErr := state.Metadata(context.Background())
	_, secondErr := state.Metadata(context.Background())
	require.Error(t, firstErr)
	require.Error(t, secondErr)
	assert.Equal(t, int32(1), calls.Load(), "error must be cached, not retried per Metadata call")
}

func TestVideoMetadataErrorIsUnwrappableAndComparable(t *testing.T) {
	inner := errors.New("network down")
	err := &VideoMetadataError{Kind: VideoMetadataUnavailable, Status: 503, Err: inner}
	assert.Equal(t, VideoMetadataUnavailable, err.Kind)
	assert.Equal(t, 503, err.Status)
	assert.ErrorIs(t, err, inner)
	assert.NotContains(t, err.Error(), "network down")
}

func TestRetryParamProfitRoutingStateNilWithoutReferenceVideos(t *testing.T) {
	urls := []string{"https://assets.example/a.mp4"}
	tests := []struct {
		name    string
		param   *RetryParam
		wantNil bool
	}{
		{"nil param", nil, true},
		{"no routing input", &RetryParam{}, true},
		{"empty reference videos", &RetryParam{RoutingInput: &modelrouting.FactsInput{}}, true},
		{"has reference videos", &RetryParam{RoutingInput: &modelrouting.FactsInput{ReferenceVideoURLs: urls}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := tt.param.ProfitRoutingState()
			if tt.wantNil {
				assert.Nil(t, state)
			} else {
				require.NotNil(t, state)
			}
		})
	}
}

func TestRetryParamProfitRoutingStateIsMemoized(t *testing.T) {
	urls := []string{"https://assets.example/a.mp4"}
	param := &RetryParam{RoutingInput: &modelrouting.FactsInput{ReferenceVideoURLs: urls}}
	first := param.ProfitRoutingState()
	second := param.ProfitRoutingState()
	require.NotNil(t, first)
	require.Same(t, first, second, "ProfitRoutingState must be built once and shared across retries")
}

func TestSetVideoMetadataClientSwapsGlobalClient(t *testing.T) {
	original := currentVideoMetadataClient()
	t.Cleanup(func() { SetVideoMetadataClient(original) })

	called := atomic.Int32{}
	SetVideoMetadataClient(&fakeMetadataClient{Calls: &called})
	urls := []string{"https://assets.example/a.mp4"}
	param := &RetryParam{RoutingInput: &modelrouting.FactsInput{ReferenceVideoURLs: urls}}
	state := param.ProfitRoutingState()
	require.NotNil(t, state)
	_, err := state.Metadata(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int32(1), called.Load())

	// Restoring nil must fall back to the fail-safe unavailable client.
	SetVideoMetadataClient(nil)
	SetVideoMetadataClient(currentVideoMetadataClient())
	_, err = NewProfitRoutingRequestState(currentVideoMetadataClient(), urls, len(urls)).Metadata(context.Background())
	require.Error(t, err)
	var metadataErr *VideoMetadataError
	require.ErrorAs(t, err, &metadataErr)
	assert.Equal(t, VideoMetadataUnavailable, metadataErr.Kind)
}

func TestNewHTTPVideoMetadataClientMissingConfigIsUnavailable(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		token   string
	}{
		{"empty url", "", "token"},
		{"blank url", "  ", "token"},
		{"empty token", "https://example.invalid", ""},
		{"blank token", "https://example.invalid", "   "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewHTTPVideoMetadataClient(tt.baseURL, tt.token, &http.Client{}, videometa.MaxVideoBytes)
			_, err := client.Metadata(context.Background(), "https://assets.example/input.mp4")
			require.Error(t, err)
			var metadataErr *VideoMetadataError
			require.ErrorAs(t, err, &metadataErr)
			assert.Equal(t, VideoMetadataUnavailable, metadataErr.Kind)
		})
	}
}

func TestResolveReferenceVideoDurationMS(t *testing.T) {
	original := currentVideoMetadataClient()
	t.Cleanup(func() { SetVideoMetadataClient(original) })
	SetVideoMetadataClient(&fakeMetadataClient{results: map[string]videometa.Metadata{
		"https://assets.example/a.mp4": {DurationMS: 9000, Width: 1280, Height: 720, FrameRateNum: 24, FrameRateDen: 1, Container: "mp4", ContentLength: 1},
		"https://assets.example/b.mp4": {DurationMS: 6000, Width: 1280, Height: 720, FrameRateNum: 24, FrameRateDen: 1, Container: "mp4", ContentLength: 1},
	}})

	duration, err := ResolveReferenceVideoDurationMS(context.Background(), []string{
		"https://assets.example/a.mp4",
		"https://assets.example/b.mp4",
	})
	require.NoError(t, err)
	assert.Equal(t, int64(15000), duration)
}

func TestResolveReferenceVideoDurationMSPreservesErrorKindWithoutURL(t *testing.T) {
	tests := []struct {
		name string
		kind VideoMetadataErrorKind
	}{
		{name: "invalid media", kind: VideoMetadataInvalidMedia},
		{name: "unavailable", kind: VideoMetadataUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := currentVideoMetadataClient()
			t.Cleanup(func() { SetVideoMetadataClient(original) })
			secretURL := "https://assets.example/input.mp4?signature=do-not-leak"
			SetVideoMetadataClient(&fakeMetadataClient{errs: map[string]error{
				secretURL: &VideoMetadataError{Kind: tt.kind},
			}})

			_, err := ResolveReferenceVideoDurationMS(context.Background(), []string{secretURL})
			var metadataErr *VideoMetadataError
			require.ErrorAs(t, err, &metadataErr)
			assert.Equal(t, tt.kind, metadataErr.Kind)
			assert.NotContains(t, err.Error(), "do-not-leak")
			assert.NotContains(t, err.Error(), "assets.example")
		})
	}
}
