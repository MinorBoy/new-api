package service

import (
	"context"
	"errors"
	"io"
	"math"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
)

// ReferenceAudioDurationErrorKind classifies user-correctable media failures and
// transient resolver failures for request validation.
type ReferenceAudioDurationErrorKind string

const (
	ReferenceAudioInvalidMedia        ReferenceAudioDurationErrorKind = "invalid_media"
	ReferenceAudioMetadataUnavailable ReferenceAudioDurationErrorKind = "metadata_unavailable"

	referenceAudioMaxBytes      int64 = 50 << 20
	referenceAudioMaxDurationMS int64 = 15000
	referenceAudioOverLimitMS   int64 = referenceAudioMaxDurationMS + 1
	referenceAudioMaxConcurrent       = 3
	referenceAudioDeadline            = 30 * time.Second
)

var errReferenceAudioTooLarge = errors.New("reference audio response limit exceeded")

// ReferenceAudioDurationError reports a sanitized reference-audio failure without
// exposing the fetched URL or its signed query parameters.
type ReferenceAudioDurationError struct {
	Kind ReferenceAudioDurationErrorKind
	Err  error
}

func (e *ReferenceAudioDurationError) Error() string {
	if e != nil && e.Kind == ReferenceAudioInvalidMedia {
		return "reference audio is not supported"
	}
	return "reference audio metadata is unavailable"
}

func (e *ReferenceAudioDurationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// ReferenceAudioDurationResolver resolves the total duration of public audio URLs.
type ReferenceAudioDurationResolver interface {
	ResolveMS(ctx context.Context, urls []string) (int64, error)
}

type httpReferenceAudioDurationResolver struct {
	client   *http.Client
	maxBytes int64
	tempDir  string
}

var referenceAudioDurationResolverHolder = struct {
	mu       sync.RWMutex
	resolver ReferenceAudioDurationResolver
}{
	resolver: &httpReferenceAudioDurationResolver{maxBytes: referenceAudioMaxBytes},
}

func currentReferenceAudioDurationResolver() ReferenceAudioDurationResolver {
	referenceAudioDurationResolverHolder.mu.RLock()
	defer referenceAudioDurationResolverHolder.mu.RUnlock()
	return referenceAudioDurationResolverHolder.resolver
}

// SetReferenceAudioDurationResolver replaces the process-wide resolver. Passing nil
// restores the SSRF-protected production resolver.
func SetReferenceAudioDurationResolver(resolver ReferenceAudioDurationResolver) {
	referenceAudioDurationResolverHolder.mu.Lock()
	defer referenceAudioDurationResolverHolder.mu.Unlock()
	if resolver == nil {
		referenceAudioDurationResolverHolder.resolver = &httpReferenceAudioDurationResolver{maxBytes: referenceAudioMaxBytes}
		return
	}
	referenceAudioDurationResolverHolder.resolver = resolver
}

// ResolveReferenceAudioDurationMS returns the aggregate duration in milliseconds.
// Durations above 15 seconds are represented by the 15001 sentinel.
func ResolveReferenceAudioDurationMS(ctx context.Context, urls []string) (int64, error) {
	return currentReferenceAudioDurationResolver().ResolveMS(ctx, urls)
}

type referenceAudioFetchResult struct {
	durationMS decimal.Decimal
	err        error
}

func (r *httpReferenceAudioDurationResolver) ResolveMS(ctx context.Context, urls []string) (int64, error) {
	if len(urls) == 0 {
		return 0, nil
	}
	if len(urls) > referenceAudioMaxConcurrent {
		return 0, &ReferenceAudioDurationError{Kind: ReferenceAudioInvalidMedia}
	}

	client := r.client
	if client == nil {
		client = newProtectedFetchHTTPClient()
	}
	maxBytes := r.maxBytes
	if maxBytes <= 0 || maxBytes > referenceAudioMaxBytes {
		maxBytes = referenceAudioMaxBytes
	}
	resolveCtx, cancel := context.WithTimeout(ctx, referenceAudioDeadline)
	defer cancel()

	var consumed atomic.Int64
	results := make(chan referenceAudioFetchResult, len(urls))
	sem := make(chan struct{}, referenceAudioMaxConcurrent)
	for _, assetURL := range urls {
		go func(rawURL string) {
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-resolveCtx.Done():
				results <- referenceAudioFetchResult{err: &ReferenceAudioDurationError{Kind: ReferenceAudioMetadataUnavailable}}
				return
			}
			duration, err := r.resolveOne(resolveCtx, client, rawURL, maxBytes, &consumed)
			results <- referenceAudioFetchResult{durationMS: duration, err: err}
		}(assetURL)
	}

	fetched := make([]referenceAudioFetchResult, 0, len(urls))
	for range urls {
		fetched = append(fetched, <-results)
	}
	for _, result := range fetched {
		var durationErr *ReferenceAudioDurationError
		if errors.As(result.err, &durationErr) && durationErr.Kind == ReferenceAudioInvalidMedia {
			return 0, result.err
		}
	}
	for _, result := range fetched {
		if result.err != nil {
			return 0, result.err
		}
	}

	total := decimal.Zero
	for _, result := range fetched {
		total = total.Add(result.durationMS)
		if total.GreaterThan(decimal.NewFromInt(referenceAudioMaxDurationMS)) {
			return referenceAudioOverLimitMS, nil
		}
	}
	if total.LessThan(decimal.NewFromInt(1)) {
		return 0, &ReferenceAudioDurationError{Kind: ReferenceAudioInvalidMedia}
	}
	return total.IntPart(), nil
}

func (r *httpReferenceAudioDurationResolver) resolveOne(
	ctx context.Context,
	client *http.Client,
	rawURL string,
	maxBytes int64,
	consumed *atomic.Int64,
) (decimal.Decimal, error) {
	parsedURL, err := url.ParseRequestURI(strings.TrimSpace(rawURL))
	if err != nil || parsedURL.Host == "" || parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return decimal.Zero, &ReferenceAudioDurationError{Kind: ReferenceAudioInvalidMedia}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsedURL.String(), nil)
	if err != nil {
		return decimal.Zero, &ReferenceAudioDurationError{Kind: ReferenceAudioInvalidMedia}
	}
	resp, err := client.Do(req)
	if err != nil {
		return decimal.Zero, &ReferenceAudioDurationError{Kind: ReferenceAudioMetadataUnavailable, Err: err}
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		kind := ReferenceAudioInvalidMedia
		if resp.StatusCode >= http.StatusInternalServerError {
			kind = ReferenceAudioMetadataUnavailable
		}
		return decimal.Zero, &ReferenceAudioDurationError{Kind: kind}
	}

	responsePath := parsedURL.Path
	if resp.Request != nil && resp.Request.URL != nil {
		responsePath = resp.Request.URL.Path
	}
	extension := referenceAudioExtension(resp.Header.Get("Content-Type"), resp.Header.Get("Content-Disposition"), responsePath)
	if extension == "" {
		return decimal.Zero, &ReferenceAudioDurationError{Kind: ReferenceAudioInvalidMedia}
	}
	tempFile, err := os.CreateTemp(r.tempDir, "new-api-reference-audio-*"+extension)
	if err != nil {
		return decimal.Zero, &ReferenceAudioDurationError{Kind: ReferenceAudioMetadataUnavailable, Err: err}
	}
	tempName := tempFile.Name()
	defer func() {
		_ = tempFile.Close()
		_ = os.Remove(tempName)
	}()

	reader := &referenceAudioBudgetReader{reader: resp.Body, consumed: consumed, limit: maxBytes}
	if _, err := io.Copy(tempFile, reader); err != nil {
		if errors.Is(err, errReferenceAudioTooLarge) {
			return decimal.Zero, &ReferenceAudioDurationError{Kind: ReferenceAudioInvalidMedia}
		}
		return decimal.Zero, &ReferenceAudioDurationError{Kind: ReferenceAudioMetadataUnavailable, Err: err}
	}
	if _, err := tempFile.Seek(0, io.SeekStart); err != nil {
		return decimal.Zero, &ReferenceAudioDurationError{Kind: ReferenceAudioMetadataUnavailable, Err: err}
	}
	seconds, err := common.GetAudioDuration(ctx, tempFile, extension)
	if err != nil || math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds <= 0 {
		return decimal.Zero, &ReferenceAudioDurationError{Kind: ReferenceAudioInvalidMedia, Err: err}
	}
	durationMS := decimal.NewFromFloat(seconds).Mul(decimal.NewFromInt(1000))
	if durationMS.LessThan(decimal.NewFromInt(1)) {
		return decimal.Zero, &ReferenceAudioDurationError{Kind: ReferenceAudioInvalidMedia}
	}
	if durationMS.GreaterThan(decimal.NewFromInt(referenceAudioMaxDurationMS)) {
		return decimal.NewFromInt(referenceAudioOverLimitMS), nil
	}
	return durationMS, nil
}

type referenceAudioBudgetReader struct {
	reader   io.Reader
	consumed *atomic.Int64
	limit    int64
}

func (r *referenceAudioBudgetReader) Read(buffer []byte) (int, error) {
	remaining := r.limit - r.consumed.Load()
	if remaining <= 0 {
		return 0, errReferenceAudioTooLarge
	}
	if int64(len(buffer)) > remaining+1 {
		buffer = buffer[:remaining+1]
	}
	n, err := r.reader.Read(buffer)
	if n > 0 && r.consumed.Add(int64(n)) > r.limit {
		return n, errReferenceAudioTooLarge
	}
	return n, err
}

func referenceAudioExtension(contentType, contentDisposition, urlPath string) string {
	if mediaType, _, err := mime.ParseMediaType(contentType); err == nil {
		if extension := referenceAudioContentTypeExtension(mediaType); extension != "" {
			return extension
		}
	}
	if _, params, err := mime.ParseMediaType(contentDisposition); err == nil {
		if extension := allowedReferenceAudioExtension(filepath.Ext(filepath.Base(params["filename"]))); extension != "" {
			return extension
		}
	}
	return allowedReferenceAudioExtension(filepath.Ext(urlPath))
}

func referenceAudioContentTypeExtension(contentType string) string {
	switch strings.ToLower(contentType) {
	case "audio/mpeg", "audio/mp3":
		return ".mp3"
	case "audio/wav", "audio/x-wav", "audio/wave":
		return ".wav"
	case "audio/flac", "audio/x-flac":
		return ".flac"
	case "audio/mp4", "audio/x-m4a":
		return ".m4a"
	case "audio/ogg", "application/ogg":
		return ".ogg"
	case "audio/opus":
		return ".opus"
	case "audio/aac":
		return ".aac"
	case "audio/aiff", "audio/x-aiff":
		return ".aiff"
	default:
		return ""
	}
}

func allowedReferenceAudioExtension(extension string) string {
	switch strings.ToLower(extension) {
	case ".mp3", ".wav", ".flac", ".m4a", ".mp4", ".ogg", ".oga", ".opus", ".aac", ".aiff", ".aif", ".aifc":
		return strings.ToLower(extension)
	default:
		return ""
	}
}
