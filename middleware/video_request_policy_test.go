package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/video_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testImageDataURI = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/p9sAAAAASUVORK5CYII="
const testVideoDataURI = "data:video/mp4;base64,AAAAIGZ0eXBpc29tAAACAGlzb21pc28yYXZjMW1wNDE="
const testRawBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8x8AAwMCAO9p9sAAAAASUVORK5CYII="
const testRawURLSafeBase64 = "aW1hZ2UtcmVmZXJlbmNlLWJ5dGVzX3dpdGgtdXJsLXNhZmUtY2hhcnMtMDEyMzQ1Njc4OTAxMjM0NTY3ODkwMTIzNDU2Nzg5"

func withVideoPolicySettings(t *testing.T, base64Enabled bool, limitMB int) {
	t.Helper()

	cfg := config.GlobalConfig.Get(video_setting.ConfigName)
	require.NotNil(t, cfg)
	saved := video_setting.Runtime()
	t.Cleanup(func() {
		require.NoError(t, config.UpdateConfigFromMap(cfg, map[string]string{
			video_setting.KeyBase64InputEnabled:   boolString(saved.Base64InputEnabled),
			video_setting.KeyJSONRequestBodyMaxMB: strconv.Itoa(saved.JSONRequestBodyMaxMB),
		}))
		video_setting.UpdateAndSync()
	})

	require.NoError(t, config.UpdateConfigFromMap(cfg, map[string]string{
		video_setting.KeyBase64InputEnabled:   boolString(base64Enabled),
		video_setting.KeyJSONRequestBodyMaxMB: strconv.Itoa(limitMB),
	}))
	video_setting.UpdateAndSync()
}

func boolString(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func newVideoPolicyRouter(t *testing.T, handler gin.HandlerFunc) *gin.Engine {
	t.Helper()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(VideoRequestPolicy())
	router.Any("/*path", handler)
	return router
}

func performPolicyRequest(router *gin.Engine, method string, path string, body string, contentType string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", contentType)
	router.ServeHTTP(recorder, req)
	return recorder
}

func TestVideoRequestPolicyRejectsBase64ReferenceMediaByDefault(t *testing.T) {
	withVideoPolicySettings(t, false, 1)

	tests := []struct {
		name      string
		body      string
		wantParam string
	}{
		{name: "top-level image data URI", body: `{"model":"m","image":"` + testImageDataURI + `"}`, wantParam: "image"},
		{name: "top-level video data URI", body: `{"model":"m","video":"` + testVideoDataURI + `"}`, wantParam: "video"},
		{name: "image_tail data URI", body: `{"model":"m","image_tail":"` + testImageDataURI + `"}`, wantParam: "image_tail"},
		{name: "images raw base64", body: `{"model":"m","images":["` + testRawBase64 + `"]}`, wantParam: "images[0]"},
		{name: "URL-safe raw base64", body: `{"model":"m","input_reference":"` + testRawURLSafeBase64 + `"}`, wantParam: "input_reference"},
		{name: "content image url object", body: `{"model":"m","content":[{"type":"image_url","image_url":{"url":"` + testImageDataURI + `"}}]}`, wantParam: "content[0].image_url.url"},
		{name: "content video url object", body: `{"model":"m","content":[{"type":"video_url","video_url":{"url":"` + testVideoDataURI + `"}}]}`, wantParam: "content[0].video_url.url"},
		{name: "metadata nested media", body: `{"model":"m","metadata":{"vendor":{"image":"` + testImageDataURI + `"}}}`, wantParam: "metadata.vendor.image"},
		{name: "jimeng binary data", body: `{"model":"m","metadata":{"binary_data_base64":["` + testRawBase64 + `"]}}`, wantParam: "metadata.binary_data_base64[0]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := newVideoPolicyRouter(t, func(c *gin.Context) {
				c.Status(http.StatusNoContent)
			})

			recorder := performPolicyRequest(router, http.MethodPost, "/v1/video/generations", tt.body, "application/json")

			require.Equal(t, http.StatusBadRequest, recorder.Code)
			assert.Contains(t, recorder.Body.String(), "video_base64_input_disabled")
			assert.Contains(t, recorder.Body.String(), tt.wantParam)
			assert.NotContains(t, recorder.Body.String(), testImageDataURI)
			assert.NotContains(t, recorder.Body.String(), testRawBase64)
		})
	}
}

func TestVideoRequestPolicyAllowsURLsAndNonMediaText(t *testing.T) {
	withVideoPolicySettings(t, false, 1)

	tests := []string{
		`{"model":"m","image":"https://example.com/image.png","video_url":{"url":"https://example.com/video.mp4"}}`,
		`{"model":"m","prompt":"literal data:image/png;base64,` + testRawBase64 + `"}`,
		`{"model":"m","metadata":{"note":"data:video/mp4;base64,` + testRawBase64 + `"}}`,
		`{"model":"m","image":"","images":["short-id","not base64 ***"]}`,
		`{"model":"m","content":[{"type":"audio_url","audio_url":{"url":"` + testVideoDataURI + `"}}]}`,
	}

	for _, body := range tests {
		t.Run(body, func(t *testing.T) {
			router := newVideoPolicyRouter(t, func(c *gin.Context) {
				c.Status(http.StatusNoContent)
			})

			recorder := performPolicyRequest(router, http.MethodPost, "/v1/video/generations", body, "application/json; charset=utf-8")

			assert.Equal(t, http.StatusNoContent, recorder.Code)
		})
	}
}

func TestVideoRequestPolicyAllowsBase64WhenGloballyEnabled(t *testing.T) {
	withVideoPolicySettings(t, true, 1)
	router := newVideoPolicyRouter(t, func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	recorder := performPolicyRequest(router, http.MethodPost, "/v1/video/generations", `{"model":"m","image":"`+testImageDataURI+`"}`, "application/json")

	assert.Equal(t, http.StatusNoContent, recorder.Code)
}

type countingReadCloser struct {
	readCount atomic.Int32
}

func (c *countingReadCloser) Read(_ []byte) (int, error) {
	c.readCount.Add(1)
	return 0, io.EOF
}

func (c *countingReadCloser) Close() error {
	return nil
}

func TestVideoRequestPolicyRejectsKnownOversizeContentLengthWithoutReading(t *testing.T) {
	withVideoPolicySettings(t, false, 1)
	body := &countingReadCloser{}
	router := newVideoPolicyRouter(t, func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/video/generations", nil)
	req.Body = body
	req.ContentLength = int64(2 << 20)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusRequestEntityTooLarge, recorder.Code)
	assert.Zero(t, body.readCount.Load())
	assert.Contains(t, recorder.Body.String(), "video_request_body_too_large")
}

func TestVideoRequestPolicyRejectsChunkedOversizeAtLimitPlusOne(t *testing.T) {
	withVideoPolicySettings(t, false, 1)
	router := newVideoPolicyRouter(t, func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(strings.Repeat("x", (1<<20)+1)))
	req.ContentLength = -1
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusRequestEntityTooLarge, recorder.Code)
}

func TestVideoRequestPolicyPreservesReusableBodyForNextHandler(t *testing.T) {
	withVideoPolicySettings(t, false, 1)
	router := newVideoPolicyRouter(t, func(c *gin.Context) {
		var payload map[string]any
		require.NoError(t, common.UnmarshalBodyReusable(c, &payload))
		assert.Equal(t, "m", payload["model"])
		storage, err := common.GetBodyStorage(c)
		require.NoError(t, err)
		assert.Equal(t, int64(len(`{"model":"m","image":"https://example.com/a.png"}`)), storage.Size())
		c.Status(http.StatusNoContent)
	})

	recorder := performPolicyRequest(router, http.MethodPost, "/v1/video/generations", `{"model":"m","image":"https://example.com/a.png"}`, "application/json")

	assert.Equal(t, http.StatusNoContent, recorder.Code)
}

func TestVideoRequestPolicyPassesMalformedJSONToNextHandler(t *testing.T) {
	withVideoPolicySettings(t, false, 1)
	router := newVideoPolicyRouter(t, func(c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		require.NoError(t, err)
		assert.Equal(t, "{bad-json", string(body))
		c.Status(http.StatusAccepted)
	})

	recorder := performPolicyRequest(router, http.MethodPost, "/v1/video/generations", `{bad-json`, "application/json")

	assert.Equal(t, http.StatusAccepted, recorder.Code)
}

func TestVideoRequestPolicySkipsMultipartAndQueries(t *testing.T) {
	withVideoPolicySettings(t, false, 1)
	router := newVideoPolicyRouter(t, func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	assert.Equal(t, http.StatusNoContent, performPolicyRequest(router, http.MethodPost, "/v1/video/generations", `{"image":"`+testImageDataURI+`"}`, "multipart/form-data; boundary=x").Code)
	assert.Equal(t, http.StatusNoContent, performPolicyRequest(router, http.MethodGet, "/v1/video/generations/task_1", "", "application/json").Code)
	assert.Equal(t, http.StatusNoContent, performPolicyRequest(router, http.MethodPost, "/jimeng/?Action=CVSync2AsyncGetResult", `{"image":"`+testImageDataURI+`"}`, "application/json").Code)
}

func TestVideoRequestPolicyUsesSeedanceErrorEnvelope(t *testing.T) {
	withVideoPolicySettings(t, false, 1)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(common.KeySeedanceOfficialAPI, true)
		c.Next()
	})
	router.Use(VideoRequestPolicy())
	router.POST("/v1/video/generations", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	recorder := performPolicyRequest(router, http.MethodPost, "/v1/video/generations", `{"model":"m","image":"`+testImageDataURI+`"}`, "application/json")

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"code":"InvalidParameter.content"`)
	assert.NotContains(t, recorder.Body.String(), "video_base64_input_disabled")
}
