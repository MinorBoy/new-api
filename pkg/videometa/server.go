package videometa

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
)

const MaxRequestBodyBytes int64 = 64 * 1024

type ServerOptions struct {
	Token          string
	MaxConcurrency int
	Metadata       func(context.Context, Request) (Metadata, error)
	Log            func(message string, fields map[string]any)
}

type metadataServer struct {
	tokenHash [sha256.Size]byte
	hasToken  bool
	metadata  func(context.Context, Request) (Metadata, error)
	log       func(string, map[string]any)
	slots     chan struct{}
}

func NewServer(options ServerOptions) http.Handler {
	if options.MaxConcurrency <= 0 {
		options.MaxConcurrency = 1
	}
	return &metadataServer{
		tokenHash: sha256.Sum256([]byte("Bearer " + options.Token)),
		hasToken:  strings.TrimSpace(options.Token) != "",
		metadata:  options.Metadata,
		log:       options.Log,
		slots:     make(chan struct{}, options.MaxConcurrency),
	}
}

func (s *metadataServer) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if request == nil {
		s.writeError(writer, ErrorInvalidRequest)
		return
	}
	if request.URL.Path == "/healthz" {
		if request.Method != http.MethodGet {
			writer.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		s.writeJSON(writer, http.StatusOK, map[string]string{"status": "ok"})
		return
	}
	if request.URL.Path != "/v1/metadata/video" {
		writer.WriteHeader(http.StatusNotFound)
		return
	}
	if request.Method != http.MethodPost {
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !s.authorized(request.Header.Get("Authorization")) {
		s.writeError(writer, ErrorUnauthorized)
		return
	}
	if s.metadata == nil {
		s.writeError(writer, ErrorInternal)
		return
	}

	startedAt := time.Now()
	resultCode := ErrorInternal
	var responseBytes int64
	var cacheHit bool
	defer func() {
		if s.log == nil {
			return
		}
		s.log("video metadata request completed", map[string]any{
			"request_id":  safeRequestID(request.Header.Get("X-Request-ID")),
			"result_code": resultCode,
			"elapsed_ms":  time.Since(startedAt).Milliseconds(),
			"bytes":       responseBytes,
			"cache_hit":   cacheHit,
		})
	}()

	request.Body = http.MaxBytesReader(writer, request.Body, MaxRequestBodyBytes)
	body, err := io.ReadAll(request.Body)
	if err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			resultCode = ErrorMediaTooLarge
			s.writeJSON(writer, http.StatusRequestEntityTooLarge, errorEnvelope(ErrorInvalidRequest))
			return
		}
		resultCode = ErrorInvalidRequest
		s.writeError(writer, ErrorInvalidRequest)
		return
	}
	var input Request
	if err := common.Unmarshal(body, &input); err != nil {
		resultCode = ErrorInvalidRequest
		s.writeError(writer, ErrorInvalidRequest)
		return
	}
	if err := input.Validate(); err != nil {
		resultCode = ErrorInvalidRequest
		s.writeError(writer, ErrorInvalidRequest)
		return
	}

	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	default:
		resultCode = ErrorConcurrencyLimited
		s.writeError(writer, ErrorConcurrencyLimited)
		return
	}

	deadline := time.Duration(input.DeadlineMS) * time.Millisecond
	if deadline > time.Duration(MaxDeadlineMS)*time.Millisecond {
		deadline = time.Duration(MaxDeadlineMS) * time.Millisecond
	}
	metadataContext, cancel := context.WithTimeout(request.Context(), deadline)
	defer cancel()
	metadata, err := s.metadata(metadataContext, input)
	if err != nil {
		var serviceError *ServiceError
		if errors.As(err, &serviceError) {
			resultCode = serviceError.Code
			s.writeError(writer, serviceError.Code)
			return
		}
		resultCode = ErrorInternal
		s.writeError(writer, ErrorInternal)
		return
	}
	if err := metadata.Validate(); err != nil {
		resultCode = ErrorMetadataInvalid
		s.writeError(writer, ErrorMetadataInvalid)
		return
	}
	resultCode = "success"
	responseBytes = metadata.ContentLength
	cacheHit = metadata.CacheHit
	s.writeJSON(writer, http.StatusOK, metadata)
}

func safeRequestID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 128 {
		return ""
	}
	for _, character := range value {
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' ||
			character >= '0' && character <= '9' || character == '-' || character == '_' || character == '.' {
			continue
		}
		return ""
	}
	return value
}

func (s *metadataServer) authorized(authorization string) bool {
	if !s.hasToken {
		return false
	}
	candidate := sha256.Sum256([]byte(authorization))
	return subtle.ConstantTimeCompare(candidate[:], s.tokenHash[:]) == 1
}

func (s *metadataServer) writeError(writer http.ResponseWriter, code ErrorCode) {
	s.writeJSON(writer, errorStatus(code), errorEnvelope(code))
}

func errorEnvelope(code ErrorCode) ErrorResponse {
	response := ErrorResponse{}
	response.Error.Code = code
	response.Error.Message = (&ServiceError{Code: code}).Error()
	return response
}

func errorStatus(code ErrorCode) int {
	switch code {
	case ErrorUnauthorized:
		return http.StatusUnauthorized
	case ErrorMediaTooLarge:
		return http.StatusRequestEntityTooLarge
	case ErrorInvalidRequest, ErrorUnsupportedFormat, ErrorMetadataInvalid:
		return http.StatusBadRequest
	case ErrorFetchUnavailable, ErrorDeadlineExceeded, ErrorConcurrencyLimited, ErrorInternal:
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

func (s *metadataServer) writeJSON(writer http.ResponseWriter, status int, value any) {
	payload, err := common.Marshal(value)
	if err != nil {
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_, _ = writer.Write(payload)
}
