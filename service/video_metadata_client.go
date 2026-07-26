package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/videometa"
	"github.com/shopspring/decimal"
)

// VideoMetadataErrorKind classifies a video metadata lookup outcome into one of the
// buckets the profit-aware routing layer needs to decide whether to exclude a single
// candidate, exclude all candidates that depend on token prediction, or fail the
// whole request before dispatch.
type VideoMetadataErrorKind string

const (
	// VideoMetadataUnavailable means the standalone metadata service could not be
	// reached or could not complete the request (401, 5xx, timeout, network, or
	// missing configuration). Profit routing only excludes candidates whose cost
	// depends on the input video duration; per-request/per-duration/free candidates
	// keep working. If user revenue itself depends on the metadata, the whole group
	// has unknown revenue and is excluded.
	VideoMetadataUnavailable VideoMetadataErrorKind = "metadata_unavailable"
	// VideoMetadataInvalidMedia means the service inspected the asset and rejected
	// it (unsupported format, corrupted, too large). This is a permanent,
	// user-correctable failure: the request must return 400 and must never degrade
	// to zero seconds or zero tokens.
	VideoMetadataInvalidMedia VideoMetadataErrorKind = "invalid_media"
	// VideoMetadataInvalidResponse means the service replied with a structurally
	// valid envelope whose metadata failed local re-validation (out-of-range fields,
	// missing container, malformed JSON). This is a fail-closed internal error.
	VideoMetadataInvalidResponse VideoMetadataErrorKind = "internal_invalid_response"
)

// VideoMetadataError wraps a metadata lookup failure with its classified kind. The
// Error() text is intentionally generic: it never echoes the source URL, signed
// query parameters, the service token, or the underlying network error, all of
// which may carry sensitive material.
type VideoMetadataError struct {
	Kind   VideoMetadataErrorKind
	Status int
	Err    error
}

func (e *VideoMetadataError) Error() string {
	switch e.Kind {
	case VideoMetadataUnavailable:
		return "video metadata service is unavailable"
	case VideoMetadataInvalidMedia:
		return "input video is not supported"
	case VideoMetadataInvalidResponse:
		return "video metadata service returned an invalid response"
	default:
		return "video metadata lookup failed"
	}
}

func (e *VideoMetadataError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// VideoMetadataClient resolves a single input video URL into validated metadata by
// delegating to the standalone metadata service. The implementation must re-validate
// the response and must never leak the URL or service token into errors or logs.
type VideoMetadataClient interface {
	Metadata(ctx context.Context, url string) (videometa.Metadata, error)
}

const (
	// videoMetadataRequestPath is the single endpoint exposed by the standalone
	// video metadata service.
	videoMetadataRequestPath = "/v1/metadata/video"
	// videoMetadataSharedDeadline bounds the whole request-level metadata resolution
	// (all reference videos) so a single slow asset cannot stall routing indefinitely.
	videoMetadataSharedDeadline = 30 * time.Second
	// videoMetadataMaxConcurrent caps how many asset lookups run in parallel for one
	// request, matching Seedance's documented maximum of 3 reference videos.
	videoMetadataMaxConcurrent = 3
)

// httpVideoMetadataClient is the production VideoMetadataClient. The service token
// travels only in the Authorization header; it is never copied onto the request body,
// errors, or log fields.
type httpVideoMetadataClient struct {
	baseURL    string
	token      string
	httpClient *http.Client
	maxBytes   int64
}

// NewHTTPVideoMetadataClient builds a client that talks to the standalone metadata
// service at baseURL using bearer token authentication. A missing/blank baseURL or
// token yields a client whose every call fails as unavailable, so deployments that
// never route token-priced video requests keep working without a metadata service.
func NewHTTPVideoMetadataClient(baseURL, token string, httpClient *http.Client, maxBytes int64) VideoMetadataClient {
	baseURL = strings.TrimSpace(baseURL)
	token = strings.TrimSpace(token)
	if baseURL == "" || token == "" {
		return &unavailableVideoMetadataClient{}
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: videoMetadataSharedDeadline}
	}
	if maxBytes <= 0 || maxBytes > videometa.MaxVideoBytes {
		maxBytes = videometa.MaxVideoBytes
	}
	return &httpVideoMetadataClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		token:      token,
		httpClient: httpClient,
		maxBytes:   maxBytes,
	}
}

// unavailableVideoMetadataClient is the fail-safe client used when the metadata
// service is not configured. Every call returns a stable unavailable error so the
// routing layer excludes only token-dependent candidates and never treats the
// absence of the service as free cost.
type unavailableVideoMetadataClient struct{}

func (unavailableVideoMetadataClient) Metadata(_ context.Context, _ string) (videometa.Metadata, error) {
	return videometa.Metadata{}, &VideoMetadataError{Kind: VideoMetadataUnavailable}
}

// Metadata performs the authenticated POST and re-validates the response locally.
func (c *httpVideoMetadataClient) Metadata(ctx context.Context, url string) (videometa.Metadata, error) {
	if strings.TrimSpace(c.token) == "" || strings.TrimSpace(c.baseURL) == "" {
		return videometa.Metadata{}, &VideoMetadataError{Kind: VideoMetadataUnavailable}
	}
	requestBody := videometa.Request{
		URL:        url,
		MediaType:  "video",
		MaxBytes:   c.maxBytes,
		DeadlineMS: int64(videoMetadataSharedDeadline / time.Millisecond),
	}
	payload, err := common.Marshal(requestBody)
	if err != nil {
		return videometa.Metadata{}, &VideoMetadataError{Kind: VideoMetadataInvalidResponse}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+videoMetadataRequestPath, strings.NewReader(string(payload)))
	if err != nil {
		return videometa.Metadata{}, &VideoMetadataError{Kind: VideoMetadataUnavailable}
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return videometa.Metadata{}, &VideoMetadataError{Kind: VideoMetadataUnavailable, Err: err}
	}
	defer func() { _ = resp.Body.Close() }()

	body, readErr := io.ReadAll(io.LimitReader(resp.Body, videometa.MaxRequestBodyBytes+1))
	if readErr != nil {
		return videometa.Metadata{}, &VideoMetadataError{Kind: VideoMetadataUnavailable, Err: readErr}
	}

	if resp.StatusCode == http.StatusOK {
		return decodeValidatedMetadata(body)
	}
	return videometa.Metadata{}, classifyErrorResponse(resp.StatusCode, body)
}

// classifyErrorResponse maps a non-200 response onto a stable error kind without
// echoing the response body (which may quote the URL or internal service details).
func classifyErrorResponse(status int, _ []byte) error {
	switch {
	case status == http.StatusUnauthorized:
		return &VideoMetadataError{Kind: VideoMetadataUnavailable, Status: status}
	case status >= 500:
		return &VideoMetadataError{Kind: VideoMetadataUnavailable, Status: status}
	case status == http.StatusBadRequest:
		// The metadata service distinguishes invalid media (unsupported_format,
		// media_too_large, metadata_invalid) from a malformed request. From the
		// caller's perspective both are "the asset cannot be priced", which is the
		// fail-closed-but-user-correctable bucket.
		return &VideoMetadataError{Kind: VideoMetadataInvalidMedia, Status: status}
	default:
		return &VideoMetadataError{Kind: VideoMetadataUnavailable, Status: status}
	}
}

// decodeValidatedMetadata decodes a 200 body and re-validates every field locally.
// The standalone service is trusted to validate too, but defense-in-depth means a
// buggy or compromised service cannot smuggle impossible dimensions or durations
// into the billing path.
func decodeValidatedMetadata(body []byte) (videometa.Metadata, error) {
	var metadata videometa.Metadata
	if err := common.Unmarshal(body, &metadata); err != nil {
		return videometa.Metadata{}, &VideoMetadataError{Kind: VideoMetadataInvalidResponse, Err: err}
	}
	if err := metadata.Validate(); err != nil {
		return videometa.Metadata{}, &VideoMetadataError{Kind: VideoMetadataInvalidResponse, Err: err}
	}
	return metadata, nil
}

// videoMetadataResult is the request-level aggregated metadata consumed by profit
// routing. Only the fields the predictor needs are retained; the source URLs are
// dropped entirely.
type videoMetadataResult struct {
	TotalDurationMS    int64
	HasReferenceVideos bool
}

// ProfitRoutingRequestState holds the per-request metadata resolution for the input
// reference videos. It is created once per request (typically from RetryParam) and
// resolves all URLs at most once via sync.Once, sharing the result across every
// candidate evaluation and retry within the same request.
//
// The URL slice and the resolved metadata never escape this struct: they are not
// copied onto Facts, Audit, diagnostics, or logs.
type ProfitRoutingRequestState struct {
	client              VideoMetadataClient
	urls                []string
	referenceVideoCount int

	once   sync.Once
	result videoMetadataResult
	err    error
}

// NewProfitRoutingRequestState builds a request-level state that will resolve the
// fetchable URLs lazily. referenceVideoCount is the total number of reference videos
// in the request, including embedded media that cannot be fetched by the metadata
// service.
func NewProfitRoutingRequestState(client VideoMetadataClient, urls []string, referenceVideoCount int) *ProfitRoutingRequestState {
	if referenceVideoCount < 0 {
		referenceVideoCount = 0
	}
	return &ProfitRoutingRequestState{client: client, urls: urls, referenceVideoCount: referenceVideoCount}
}

// ResolveReferenceVideoDurationMS resolves and sums a request's public reference
// video URLs through the process-wide metadata service.
func ResolveReferenceVideoDurationMS(ctx context.Context, urls []string) (int64, error) {
	state := NewProfitRoutingRequestState(currentVideoMetadataClient(), urls, len(urls))
	result, err := state.Metadata(ctx)
	if err != nil {
		return 0, err
	}
	return result.TotalDurationMS, nil
}

// HasReferenceVideos reports whether the request carries any input reference video
// without triggering a metadata lookup. Callers use it to short-circuit the metadata
// service for text/image-only requests.
func (s *ProfitRoutingRequestState) HasReferenceVideos() bool {
	return s != nil && s.referenceVideoCount > 0
}

// Metadata resolves all reference video URLs at most once and returns the aggregate.
// When there are no reference videos it returns a zero result without calling the
// service. The shared 30s deadline bounds the whole resolution and at most 3 URLs
// are fetched concurrently.
func (s *ProfitRoutingRequestState) Metadata(ctx context.Context) (videoMetadataResult, error) {
	s.once.Do(func() {
		s.result, s.err = s.resolve(ctx)
	})
	return s.result, s.err
}

func (s *ProfitRoutingRequestState) resolve(ctx context.Context) (videoMetadataResult, error) {
	if s.referenceVideoCount == 0 {
		return videoMetadataResult{HasReferenceVideos: false}, nil
	}
	if len(s.urls) != s.referenceVideoCount {
		return videoMetadataResult{HasReferenceVideos: true}, &VideoMetadataError{Kind: VideoMetadataUnavailable}
	}
	resolveCtx, cancel := context.WithTimeout(ctx, videoMetadataSharedDeadline)
	defer cancel()

	type fetchResult struct {
		index    int
		metadata videometa.Metadata
		err      error
	}
	results := make([]videometa.Metadata, len(s.urls))
	sem := make(chan struct{}, videoMetadataMaxConcurrent)
	var wg sync.WaitGroup
	errs := make([]error, len(s.urls))

	for index, url := range s.urls {
		wg.Add(1)
		go func(i int, assetURL string) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-resolveCtx.Done():
				errs[i] = &VideoMetadataError{Kind: VideoMetadataUnavailable}
				return
			}
			defer func() { <-sem }()
			metadata, err := s.client.Metadata(resolveCtx, assetURL)
			if err != nil {
				errs[i] = err
				return
			}
			results[i] = metadata
		}(index, url)
	}
	wg.Wait()

	// Surface invalid-media errors first: a single bad asset must fail the whole
	// request with a 400 rather than degrade to zero tokens. Unavailable errors are
	// reported only if no invalid-media error exists, so the caller can exclude
	// token-dependent candidates instead of rejecting the request outright.
	for _, err := range errs {
		if err == nil {
			continue
		}
		var metadataErr *VideoMetadataError
		if errors.As(err, &metadataErr) && metadataErr.Kind == VideoMetadataInvalidMedia {
			return videoMetadataResult{HasReferenceVideos: true}, err
		}
	}
	for _, err := range errs {
		if err == nil {
			continue
		}
		return videoMetadataResult{HasReferenceVideos: true}, err
	}

	total := decimal.NewFromInt(0)
	for _, metadata := range results {
		if metadata.DurationMS <= 0 {
			return videoMetadataResult{HasReferenceVideos: true}, &VideoMetadataError{Kind: VideoMetadataInvalidResponse}
		}
		total = total.Add(decimal.NewFromInt(metadata.DurationMS))
	}
	return videoMetadataResult{
		TotalDurationMS:    total.IntPart(),
		HasReferenceVideos: true,
	}, nil
}
