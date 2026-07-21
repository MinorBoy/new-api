package middleware

import (
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/setting/video_setting"
	"github.com/gin-gonic/gin"
)

const (
	videoBase64DisabledMessage = "base64 reference media is disabled for video generation; use an HTTP(S) URL instead"
	videoBodyTooLargeMessage   = "video JSON request body exceeds the configured limit"

	videoBase64DisabledCode = "video_base64_input_disabled"
	videoBodyTooLargeCode   = "video_request_body_too_large"

	minRawBase64MediaLength = 64
)

type videoBase64Hit struct {
	Param string
}

func VideoRequestPolicy() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !isVideoGenerationSubmitRequest(c) || !isJSONRequest(c.GetHeader("Content-Type")) {
			c.Next()
			return
		}

		snapshot := video_setting.Runtime()
		if c.Request.ContentLength > snapshot.JSONRequestBodyMaxBytes {
			logVideoPolicyReject(c, "", c.Request.ContentLength, snapshot.JSONRequestBodyMaxMB, "body_too_large")
			abortVideoRequestTooLarge(c)
			return
		}

		body, tooLarge, err := readVideoPolicyBody(c.Request.Body, snapshot.JSONRequestBodyMaxBytes)
		_ = c.Request.Body.Close()
		if err != nil {
			abortVideoPolicyOpenAI(c, http.StatusBadRequest, "failed to read request body", "", "invalid_request_error")
			return
		}
		if tooLarge {
			logVideoPolicyReject(c, "", int64(len(body)), snapshot.JSONRequestBodyMaxMB, "body_too_large")
			abortVideoRequestTooLarge(c)
			return
		}

		storage, err := common.CreateBodyStorage(body)
		if err != nil {
			abortVideoPolicyOpenAI(c, http.StatusBadRequest, "failed to cache request body", "", "invalid_request_error")
			return
		}
		if _, err := storage.Seek(0, io.SeekStart); err != nil {
			_ = storage.Close()
			abortVideoPolicyOpenAI(c, http.StatusBadRequest, "failed to prepare request body", "", "invalid_request_error")
			return
		}
		c.Set(common.KeyRequestBody, body)
		c.Set(common.KeyBodyStorage, storage)
		c.Request.Body = io.NopCloser(storage)

		if snapshot.Base64InputEnabled {
			c.Next()
			return
		}

		var payload any
		if err := common.Unmarshal(body, &payload); err != nil {
			c.Next()
			return
		}

		if hit, ok := findVideoBase64Media(payload); ok {
			logVideoPolicyReject(c, hit.Param, int64(len(body)), snapshot.JSONRequestBodyMaxMB, "base64_disabled")
			abortVideoBase64Disabled(c, hit.Param)
			return
		}

		c.Next()
	}
}

func isVideoGenerationSubmitRequest(c *gin.Context) bool {
	if c.Request.Method != http.MethodPost {
		return false
	}

	path := c.Request.URL.Path
	if isJimengGetResult(c) {
		return false
	}

	if path == "/v1/video/generations" ||
		path == "/v1/videos" ||
		path == "/kling/v1/videos/text2video" ||
		path == "/kling/v1/videos/image2video" ||
		path == "/api/v3/contents/generations/tasks" ||
		path == "/jimeng" ||
		path == "/jimeng/" {
		return true
	}

	return strings.HasPrefix(path, "/v1/videos/") && strings.HasSuffix(path, "/remix")
}

func isJimengGetResult(c *gin.Context) bool {
	return c.Query("Action") == "CVSync2AsyncGetResult"
}

func isJSONRequest(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	mediaType = strings.ToLower(mediaType)
	return mediaType == "application/json" || strings.HasSuffix(mediaType, "+json")
}

func readVideoPolicyBody(body io.Reader, maxBytes int64) ([]byte, bool, error) {
	data, err := io.ReadAll(io.LimitReader(body, maxBytes+1))
	if err != nil {
		return nil, false, err
	}
	return data, int64(len(data)) > maxBytes, nil
}

func findVideoBase64Media(value any) (videoBase64Hit, bool) {
	return findVideoBase64MediaAt(value, "")
}

func findVideoBase64MediaAt(value any, path string) (videoBase64Hit, bool) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			childPath := key
			if path != "" {
				childPath = path + "." + key
			}
			if hit, ok := inspectMediaField(key, child, childPath); ok {
				return hit, true
			}
			if hit, ok := findVideoBase64MediaAt(child, childPath); ok {
				return hit, true
			}
		}
	case []any:
		for index, child := range typed {
			childPath := fmt.Sprintf("%s[%d]", path, index)
			if hit, ok := findVideoBase64MediaAt(child, childPath); ok {
				return hit, true
			}
		}
	}
	return videoBase64Hit{}, false
}

func inspectMediaField(key string, value any, path string) (videoBase64Hit, bool) {
	if !isProtectedVideoMediaField(key) {
		return videoBase64Hit{}, false
	}

	switch typed := value.(type) {
	case string:
		if isBase64MediaString(typed) {
			return videoBase64Hit{Param: path}, true
		}
	case map[string]any:
		if nested, ok := typed["url"].(string); ok && isBase64MediaString(nested) {
			return videoBase64Hit{Param: path + ".url"}, true
		}
	case []any:
		for index, child := range typed {
			childPath := fmt.Sprintf("%s[%d]", path, index)
			switch childValue := child.(type) {
			case string:
				if isBase64MediaString(childValue) {
					return videoBase64Hit{Param: childPath}, true
				}
			case map[string]any:
				if nested, ok := childValue["url"].(string); ok && isBase64MediaString(nested) {
					return videoBase64Hit{Param: childPath + ".url"}, true
				}
			}
		}
	}
	return videoBase64Hit{}, false
}

func isProtectedVideoMediaField(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	normalized = strings.ReplaceAll(normalized, "-", "_")
	switch normalized {
	case "image", "images", "image_url", "image_urls",
		"video", "videos", "video_url", "video_urls",
		"image_tail", "input_reference", "binary_data_base64":
		return true
	default:
		return false
	}
}

func isBase64MediaString(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || isAllowedRemoteMediaURL(trimmed) {
		return false
	}

	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "data:image/") || strings.HasPrefix(lower, "data:video/") {
		comma := strings.Index(trimmed, ",")
		if comma <= 0 {
			return false
		}
		header := strings.ToLower(trimmed[:comma])
		return strings.Contains(header, ";base64") && canStreamDecodeBase64(trimmed[comma+1:])
	}

	if len(trimmed) < minRawBase64MediaLength {
		return false
	}
	if !looksLikeRawBase64(trimmed) {
		return false
	}
	return canStreamDecodeBase64(trimmed) || hasRawBase64Marker(trimmed)
}

func isAllowedRemoteMediaURL(value string) bool {
	parsed, err := url.Parse(value)
	if err != nil {
		return false
	}
	scheme := strings.ToLower(parsed.Scheme)
	return scheme == "http" || scheme == "https"
}

func looksLikeRawBase64(value string) bool {
	hasURLSafe := false
	hasStandard := false
	for _, r := range value {
		switch {
		case r >= 'A' && r <= 'Z':
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '+' || r == '/':
			hasStandard = true
		case r == '-' || r == '_':
			hasURLSafe = true
		case r == '=':
		case r == '\r' || r == '\n' || r == '\t' || r == ' ':
		default:
			return false
		}
	}
	return !(hasURLSafe && hasStandard)
}

func hasRawBase64Marker(value string) bool {
	return strings.ContainsAny(value, "+/-_=")
}

func canStreamDecodeBase64(value string) bool {
	candidate := strings.NewReplacer("\r", "", "\n", "", "\t", "", " ", "").Replace(value)
	if candidate == "" {
		return false
	}
	for _, encoding := range []*base64.Encoding{
		base64.StdEncoding.Strict(),
		base64.RawStdEncoding.Strict(),
		base64.URLEncoding.Strict(),
		base64.RawURLEncoding.Strict(),
	} {
		reader := base64.NewDecoder(encoding, strings.NewReader(candidate))
		if _, err := io.CopyBuffer(io.Discard, reader, make([]byte, 1024)); err == nil {
			return true
		}
	}
	return false
}

func abortVideoBase64Disabled(c *gin.Context, param string) {
	if c.GetBool(common.KeySeedanceOfficialAPI) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"code":    "InvalidParameter.content",
				"message": videoBase64DisabledMessage,
			},
		})
		c.Abort()
		return
	}
	abortVideoPolicyOpenAI(c, http.StatusBadRequest, videoBase64DisabledMessage, param, videoBase64DisabledCode)
}

func abortVideoRequestTooLarge(c *gin.Context) {
	if c.GetBool(common.KeySeedanceOfficialAPI) {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{
			"error": gin.H{
				"code":    "InvalidParameter",
				"message": videoBodyTooLargeMessage,
			},
		})
		c.Abort()
		return
	}
	abortVideoPolicyOpenAI(c, http.StatusRequestEntityTooLarge, videoBodyTooLargeMessage, "", videoBodyTooLargeCode)
}

func abortVideoPolicyOpenAI(c *gin.Context, status int, message string, param string, code string) {
	c.JSON(status, gin.H{
		"error": gin.H{
			"message": message,
			"type":    "invalid_request_error",
			"param":   param,
			"code":    code,
		},
	})
	c.Abort()
}

func logVideoPolicyReject(c *gin.Context, param string, bodyBytes int64, limitMB int, reason string) {
	logger.LogWarn(c.Request.Context(), fmt.Sprintf(
		"video request policy rejected request: reason=%s path=%s bytes=%d limit_mb=%d param=%s",
		reason,
		c.Request.URL.Path,
		bodyBytes,
		limitMB,
		param,
	))
}
