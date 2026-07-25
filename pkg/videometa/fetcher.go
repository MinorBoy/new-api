package videometa

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const maxValidatorLength = 1_024

type FetcherOptions struct {
	Client            *http.Client
	Cache             *Cache
	MaxBytes          int64
	TempDir           string
	CacheTTL          time.Duration
	SignedURLCacheTTL time.Duration
}

type Fetcher struct {
	client            *http.Client
	cache             *Cache
	maxBytes          int64
	tempDir           string
	cacheTTL          time.Duration
	signedURLCacheTTL time.Duration
}

func NewFetcher(options FetcherOptions) *Fetcher {
	if options.Client == nil {
		options.Client = http.DefaultClient
	}
	if options.MaxBytes <= 0 || options.MaxBytes > MaxVideoBytes {
		options.MaxBytes = MaxVideoBytes
	}
	if options.CacheTTL <= 0 {
		options.CacheTTL = 10 * time.Minute
	}
	if options.SignedURLCacheTTL <= 0 {
		options.SignedURLCacheTTL = time.Minute
	}
	return &Fetcher{
		client:            options.Client,
		cache:             options.Cache,
		maxBytes:          options.MaxBytes,
		tempDir:           options.TempDir,
		cacheTTL:          options.CacheTTL,
		signedURLCacheTTL: options.SignedURLCacheTTL,
	}
}

type assetHeaders struct {
	etag          string
	lastModified  string
	contentLength int64
}

func (f *Fetcher) Metadata(ctx context.Context, request Request) (Metadata, error) {
	if err := request.Validate(); err != nil {
		return Metadata{}, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	requestContext, cancel := context.WithTimeout(ctx, time.Duration(request.DeadlineMS)*time.Millisecond)
	defer cancel()

	limit := request.MaxBytes
	if f.maxBytes < limit {
		limit = f.maxBytes
	}
	headers, canUseHeadCache, err := f.probe(requestContext, request.URL, limit)
	if err != nil {
		return Metadata{}, err
	}
	if canUseHeadCache && f.cache != nil {
		if metadata, ok := f.cache.Get(CacheKey(CacheKeyInput{
			URL: request.URL, ETag: headers.etag, LastModified: headers.lastModified, ContentLength: headers.contentLength,
		})); ok {
			return metadata, nil
		}
	}

	httpRequest, err := http.NewRequestWithContext(requestContext, http.MethodGet, request.URL, nil)
	if err != nil {
		return Metadata{}, newServiceError(ErrorInvalidRequest, http.StatusBadRequest, err)
	}
	response, err := f.client.Do(httpRequest)
	if err != nil {
		return Metadata{}, fetchError(requestContext, err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return Metadata{}, newServiceError(ErrorFetchUnavailable, http.StatusServiceUnavailable, fmt.Errorf("GET returned status %d", response.StatusCode))
	}
	declaredLength, err := responseContentLength(response)
	if err != nil {
		return Metadata{}, newServiceError(ErrorMetadataInvalid, http.StatusBadRequest, err)
	}
	if declaredLength > limit {
		return Metadata{}, newServiceError(ErrorMediaTooLarge, http.StatusRequestEntityTooLarge, errors.New("declared content length exceeds limit"))
	}

	tempFile, err := os.CreateTemp(f.tempDir, "video-metadata-*")
	if err != nil {
		return Metadata{}, newServiceError(ErrorInternal, http.StatusInternalServerError, err)
	}
	tempPath := tempFile.Name()
	defer os.Remove(tempPath)
	defer tempFile.Close()

	written, err := io.Copy(tempFile, io.LimitReader(response.Body, limit+1))
	if err != nil {
		return Metadata{}, fetchError(requestContext, err)
	}
	if written > limit {
		return Metadata{}, newServiceError(ErrorMediaTooLarge, http.StatusRequestEntityTooLarge, errors.New("downloaded content exceeds limit"))
	}
	if _, err := tempFile.Seek(0, io.SeekStart); err != nil {
		return Metadata{}, newServiceError(ErrorInternal, http.StatusInternalServerError, err)
	}
	metadata, err := Parse(tempFile, written)
	if err != nil {
		return Metadata{}, parseError(err)
	}

	getHeaders := assetHeaders{
		etag:          boundedHeader(response.Header.Get("ETag")),
		lastModified:  boundedHeader(response.Header.Get("Last-Modified")),
		contentLength: written,
	}
	if getHeaders.etag == "" {
		getHeaders.etag = headers.etag
	}
	if getHeaders.lastModified == "" {
		getHeaders.lastModified = headers.lastModified
	}
	metadata.ETag = getHeaders.etag
	metadata.LastModified = getHeaders.lastModified
	if err := metadata.Validate(); err != nil {
		return Metadata{}, newServiceError(ErrorMetadataInvalid, http.StatusBadRequest, err)
	}
	if f.cache != nil {
		f.cache.Set(CacheKey(CacheKeyInput{
			URL: request.URL, ETag: getHeaders.etag, LastModified: getHeaders.lastModified, ContentLength: written,
		}), metadata, f.ttl(request.URL))
	}
	return metadata, nil
}

func (f *Fetcher) probe(ctx context.Context, rawURL string, limit int64) (assetHeaders, bool, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodHead, rawURL, nil)
	if err != nil {
		return assetHeaders{}, false, newServiceError(ErrorInvalidRequest, http.StatusBadRequest, err)
	}
	response, err := f.client.Do(request)
	if err != nil {
		return assetHeaders{}, false, fetchError(ctx, err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusMethodNotAllowed || response.StatusCode == http.StatusNotImplemented {
		return assetHeaders{}, false, nil
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return assetHeaders{}, false, newServiceError(ErrorFetchUnavailable, http.StatusServiceUnavailable, fmt.Errorf("HEAD returned status %d", response.StatusCode))
	}
	contentLength, err := responseContentLength(response)
	if err != nil {
		return assetHeaders{}, false, newServiceError(ErrorMetadataInvalid, http.StatusBadRequest, err)
	}
	if contentLength > limit {
		return assetHeaders{}, false, newServiceError(ErrorMediaTooLarge, http.StatusRequestEntityTooLarge, errors.New("declared content length exceeds limit"))
	}
	return assetHeaders{
		etag:          boundedHeader(response.Header.Get("ETag")),
		lastModified:  boundedHeader(response.Header.Get("Last-Modified")),
		contentLength: contentLength,
	}, contentLength >= 0, nil
}

func responseContentLength(response *http.Response) (int64, error) {
	rawValue := strings.TrimSpace(response.Header.Get("Content-Length"))
	if rawValue == "" {
		return response.ContentLength, nil
	}
	value, err := strconv.ParseInt(rawValue, 10, 64)
	if err != nil || value < 0 {
		return 0, errors.New("content length is invalid")
	}
	return value, nil
}

func boundedHeader(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > maxValidatorLength {
		return ""
	}
	return value
}

func (f *Fetcher) ttl(rawURL string) time.Duration {
	parsed, err := url.Parse(rawURL)
	if err == nil && parsed.RawQuery != "" {
		return f.signedURLCacheTTL
	}
	return f.cacheTTL
}

func fetchError(ctx context.Context, err error) error {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return newServiceError(ErrorDeadlineExceeded, http.StatusGatewayTimeout, err)
	}
	return newServiceError(ErrorFetchUnavailable, http.StatusServiceUnavailable, err)
}

func parseError(err error) error {
	var serviceError *ServiceError
	if errors.As(err, &serviceError) {
		return serviceError
	}
	if errors.Is(err, ErrUnsupportedContainer) {
		return newServiceError(ErrorUnsupportedFormat, http.StatusBadRequest, err)
	}
	return newServiceError(ErrorMetadataInvalid, http.StatusBadRequest, err)
}
