package videometa

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
)

const (
	MaxVideoBytes int64 = 128 * 1024 * 1024
	MaxDeadlineMS int64 = 30_000
	MaxDimension        = 16_384
	MaxFrameRate  int64 = 240
)

type Request struct {
	URL        string `json:"url"`
	MediaType  string `json:"media_type"`
	MaxBytes   int64  `json:"max_bytes"`
	DeadlineMS int64  `json:"deadline_ms"`
}

func (r Request) Validate() error {
	if r.MediaType != "video" {
		return newServiceError(ErrorInvalidRequest, http.StatusBadRequest, errors.New("media type must be video"))
	}
	if r.MaxBytes <= 0 || r.MaxBytes > MaxVideoBytes {
		return newServiceError(ErrorInvalidRequest, http.StatusBadRequest, errors.New("max bytes is out of range"))
	}
	if r.DeadlineMS <= 0 || r.DeadlineMS > MaxDeadlineMS {
		return newServiceError(ErrorInvalidRequest, http.StatusBadRequest, errors.New("deadline is out of range"))
	}

	rawURL := strings.TrimSpace(r.URL)
	parsed, err := url.ParseRequestURI(rawURL)
	if err != nil || parsed.Hostname() == "" || parsed.User != nil {
		return newServiceError(ErrorInvalidRequest, http.StatusBadRequest, errors.New("media URL is invalid"))
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		return nil
	default:
		return newServiceError(ErrorInvalidRequest, http.StatusBadRequest, errors.New("media URL scheme is unsupported"))
	}
}

type Metadata struct {
	DurationMS    int64  `json:"duration_ms"`
	Width         int    `json:"width"`
	Height        int    `json:"height"`
	FrameRateNum  int64  `json:"frame_rate_num"`
	FrameRateDen  int64  `json:"frame_rate_den"`
	Container     string `json:"container"`
	ContentLength int64  `json:"content_length"`
	ETag          string `json:"etag,omitempty"`
	LastModified  string `json:"last_modified,omitempty"`
	CacheHit      bool   `json:"-"`
}

func (m Metadata) Validate() error {
	if m.DurationMS <= 0 {
		return newServiceError(ErrorMetadataInvalid, http.StatusBadRequest, errors.New("duration must be positive"))
	}
	if m.Width <= 0 || m.Width > MaxDimension || m.Height <= 0 || m.Height > MaxDimension {
		return newServiceError(ErrorMetadataInvalid, http.StatusBadRequest, errors.New("dimensions are out of range"))
	}
	if m.FrameRateNum <= 0 || m.FrameRateDen <= 0 || exceedsMaxFrameRate(m.FrameRateNum, m.FrameRateDen) {
		return newServiceError(ErrorMetadataInvalid, http.StatusBadRequest, errors.New("frame rate is out of range"))
	}
	if m.Container != "mp4" && m.Container != "mov" {
		return newServiceError(ErrorUnsupportedFormat, http.StatusBadRequest, errors.New("container is unsupported"))
	}
	if m.ContentLength < 0 {
		return newServiceError(ErrorMetadataInvalid, http.StatusBadRequest, errors.New("content length cannot be negative"))
	}
	return nil
}

func exceedsMaxFrameRate(numerator, denominator int64) bool {
	whole := numerator / denominator
	return whole > MaxFrameRate || whole == MaxFrameRate && numerator%denominator != 0
}

type ErrorCode string

const (
	ErrorInvalidRequest     ErrorCode = "invalid_request"
	ErrorUnauthorized       ErrorCode = "unauthorized"
	ErrorUnsupportedFormat  ErrorCode = "unsupported_format"
	ErrorMediaTooLarge      ErrorCode = "media_too_large"
	ErrorMetadataInvalid    ErrorCode = "metadata_invalid"
	ErrorFetchUnavailable   ErrorCode = "fetch_unavailable"
	ErrorDeadlineExceeded   ErrorCode = "deadline_exceeded"
	ErrorConcurrencyLimited ErrorCode = "concurrency_limited"
	ErrorInternal           ErrorCode = "internal_error"
)

type ServiceError struct {
	Code       ErrorCode
	HTTPStatus int
	Err        error
}

func (e *ServiceError) Error() string {
	if e == nil {
		return "video metadata service error"
	}
	switch e.Code {
	case ErrorInvalidRequest:
		return "video metadata request is invalid"
	case ErrorUnauthorized:
		return "video metadata request is unauthorized"
	case ErrorUnsupportedFormat:
		return "video format is unsupported"
	case ErrorMediaTooLarge:
		return "video exceeds the configured size limit"
	case ErrorMetadataInvalid:
		return "video metadata is invalid"
	case ErrorFetchUnavailable:
		return "video source is unavailable"
	case ErrorDeadlineExceeded:
		return "video metadata deadline exceeded"
	case ErrorConcurrencyLimited:
		return "video metadata concurrency limit reached"
	default:
		return "video metadata service failed"
	}
}

func (e *ServiceError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func newServiceError(code ErrorCode, status int, err error) *ServiceError {
	return &ServiceError{Code: code, HTTPStatus: status, Err: err}
}

type ErrorResponse struct {
	Error struct {
		Code    ErrorCode `json:"code"`
		Message string    `json:"message"`
	} `json:"error"`
}
