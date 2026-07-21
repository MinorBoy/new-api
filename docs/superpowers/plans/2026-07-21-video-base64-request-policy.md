# Video Base64 Request Policy Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Reject Base64 reference images and videos in video-generation JSON submissions by default, while letting administrators enable Base64 globally and tune a dedicated video JSON body limit.

**Architecture:** Add a `video_setting` config module with an atomic runtime snapshot, then enforce it in a Gin middleware that runs before request conversion, distribution, billing, retries, and upstream calls. The middleware reads only JSON video-submit bodies up to the configured limit, reuses the existing request-body cache, and performs structured media-field inspection.

**Tech Stack:** Go 1.22, Gin, GORM options, `common.*` JSON wrappers, `testify`, React 19, TypeScript, React Hook Form, Zod, Bun, i18next

---

## File Structure

- Create `setting/video_setting/config.go`: owns defaults, validation constants, `config.GlobalConfig` registration, and atomic runtime snapshot.
- Create `setting/video_setting/config_test.go`: verifies defaults, bounds, fallback behavior, and immediate runtime sync.
- Modify `model/option.go`: calls `video_setting.UpdateAndSync()` after `video_setting.*` option updates.
- Modify `controller/option.go`: rejects invalid admin values before persistence.
- Create `controller/option_video_setting_test.go`: proves invalid values are not saved and valid values update runtime state.
- Create `middleware/video_request_policy.go`: enforces JSON body limit, Base64 detection, protocol-specific errors, and safe logging.
- Create `middleware/video_request_policy_test.go`: covers detection, false positives, size limits, body reuse, malformed JSON pass-through, multipart skip, and Seedance errors.
- Modify `router/video-router.go`: mounts the policy in the approved route order.
- Create `router/video_router_test.go`: proves Kling/Jimeng/Seedance are rejected before auth/converters and Jimeng query POST skips the policy.
- Modify `middleware/kling_adapter.go` and `middleware/jimeng_adapter.go`: replace direct `encoding/json.Marshal` calls with `common.Marshal`.
- Modify `web/default/src/features/system-settings/types.ts`: adds the two `video_setting.*` keys to `OperationsSettings`.
- Modify `web/default/src/features/system-settings/operations/index.tsx`: supplies frontend defaults.
- Modify `web/default/src/features/system-settings/operations/section-registry.tsx`: passes video defaults into the performance section.
- Modify `web/default/src/features/system-settings/maintenance/performance-section.tsx`: adds the "Video Request Protection" settings group and validation.
- Modify locale files only through `web/default/scripts/add-missing-keys.mjs`, followed by `bun run i18n:sync`: adds en/zh/fr/ru/ja/vi translations.

The current worktree already has unrelated dirty files under `web/default/src/i18n/locales/_reports/`. During implementation, do not stage `_reports` unless the user explicitly asks.

### Task 1: Add the Video Setting Runtime Module

**Files:**
- Create: `setting/video_setting/config_test.go`
- Create: `setting/video_setting/config.go`

- [ ] **Step 1: Write the failing setting tests**

Create `setting/video_setting/config_test.go`:

```go
package video_setting

import (
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/setting/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func withVideoSettingConfig(t *testing.T, values map[string]string) {
	t.Helper()

	original := videoSetting
	t.Cleanup(func() {
		videoSetting = original
		UpdateAndSync()
	})

	require.NoError(t, config.UpdateConfigFromMap(&videoSetting, values))
	UpdateAndSync()
}

func TestRuntimeDefaultsDisableBase64AndLimitJSONBody(t *testing.T) {
	withVideoSettingConfig(t, map[string]string{})

	snapshot := Runtime()

	assert.False(t, snapshot.Base64InputEnabled)
	assert.Equal(t, DefaultJSONRequestBodyMaxMB, snapshot.JSONRequestBodyMaxMB)
	assert.Equal(t, int64(DefaultJSONRequestBodyMaxMB)<<20, snapshot.JSONRequestBodyMaxBytes)
}

func TestRuntimeAcceptsConfiguredValues(t *testing.T) {
	withVideoSettingConfig(t, map[string]string{
		KeyBase64InputEnabled:   "true",
		KeyJSONRequestBodyMaxMB: "32",
	})

	snapshot := Runtime()

	assert.True(t, snapshot.Base64InputEnabled)
	assert.Equal(t, 32, snapshot.JSONRequestBodyMaxMB)
	assert.Equal(t, int64(32)<<20, snapshot.JSONRequestBodyMaxBytes)
}

func TestRuntimeFallsBackForInvalidJSONBodyLimit(t *testing.T) {
	for _, value := range []string{"0", "-1", "129", "999"} {
		t.Run(value, func(t *testing.T) {
			withVideoSettingConfig(t, map[string]string{
				KeyJSONRequestBodyMaxMB: value,
			})

			snapshot := Runtime()

			assert.Equal(t, DefaultJSONRequestBodyMaxMB, snapshot.JSONRequestBodyMaxMB)
			assert.Equal(t, int64(DefaultJSONRequestBodyMaxMB)<<20, snapshot.JSONRequestBodyMaxBytes)
		})
	}
}

func TestValidateJSONRequestBodyMaxMB(t *testing.T) {
	for _, value := range []int{MinJSONRequestBodyMaxMB, 16, MaxJSONRequestBodyMaxMB} {
		require.NoError(t, ValidateJSONRequestBodyMaxMB(value))
	}

	for _, value := range []int{0, -1, MaxJSONRequestBodyMaxMB + 1} {
		require.Error(t, ValidateJSONRequestBodyMaxMB(value))
	}
}

func TestConfigExportsFlatVideoSettingKeys(t *testing.T) {
	withVideoSettingConfig(t, map[string]string{
		KeyBase64InputEnabled:   "true",
		KeyJSONRequestBodyMaxMB: "24",
	})

	exported, err := config.ConfigToMap(&videoSetting)
	require.NoError(t, err)

	assert.Equal(t, "true", exported[KeyBase64InputEnabled])
	assert.Equal(t, strconv.Itoa(24), exported[KeyJSONRequestBodyMaxMB])
}
```

- [ ] **Step 2: Run the setting tests and verify RED**

Run:

```bash
go test ./setting/video_setting
```

Expected: FAIL because `setting/video_setting` does not exist yet.

- [ ] **Step 3: Implement the setting module**

Create `setting/video_setting/config.go`:

```go
package video_setting

import (
	"fmt"
	"sync/atomic"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
)

const (
	ConfigName = "video_setting"

	KeyBase64InputEnabled   = "base64_input_enabled"
	KeyJSONRequestBodyMaxMB = "json_request_body_max_mb"

	DefaultJSONRequestBodyMaxMB = 16
	MinJSONRequestBodyMaxMB     = 1
	MaxJSONRequestBodyMaxMB     = 128
)

type VideoSetting struct {
	Base64InputEnabled   bool `json:"base64_input_enabled"`
	JSONRequestBodyMaxMB int  `json:"json_request_body_max_mb"`
}

type RuntimeSnapshot struct {
	Base64InputEnabled    bool
	JSONRequestBodyMaxMB  int
	JSONRequestBodyMaxBytes int64
}

var videoSetting = VideoSetting{
	Base64InputEnabled:   false,
	JSONRequestBodyMaxMB: DefaultJSONRequestBodyMaxMB,
}

var runtimeSnapshot atomic.Value

func init() {
	config.GlobalConfig.Register(ConfigName, &videoSetting)
	UpdateAndSync()
}

func ValidateJSONRequestBodyMaxMB(value int) error {
	if value < MinJSONRequestBodyMaxMB || value > MaxJSONRequestBodyMaxMB {
		return fmt.Errorf("video JSON request body limit must be between %d and %d MB", MinJSONRequestBodyMaxMB, MaxJSONRequestBodyMaxMB)
	}
	return nil
}

func normalizedJSONRequestBodyMaxMB(value int) int {
	if err := ValidateJSONRequestBodyMaxMB(value); err != nil {
		common.SysError(err.Error() + "; using safe default")
		return DefaultJSONRequestBodyMaxMB
	}
	return value
}

func Runtime() RuntimeSnapshot {
	if loaded := runtimeSnapshot.Load(); loaded != nil {
		if snapshot, ok := loaded.(RuntimeSnapshot); ok {
			return snapshot
		}
	}
	return buildRuntimeSnapshot(videoSetting)
}

func UpdateAndSync() {
	runtimeSnapshot.Store(buildRuntimeSnapshot(videoSetting))
}

func buildRuntimeSnapshot(setting VideoSetting) RuntimeSnapshot {
	limitMB := normalizedJSONRequestBodyMaxMB(setting.JSONRequestBodyMaxMB)
	return RuntimeSnapshot{
		Base64InputEnabled:     setting.Base64InputEnabled,
		JSONRequestBodyMaxMB:   limitMB,
		JSONRequestBodyMaxBytes: int64(limitMB) << 20,
	}
}
```

- [ ] **Step 4: Run the setting tests and verify GREEN**

Run:

```bash
go test ./setting/video_setting
```

Expected: PASS.

- [ ] **Step 5: Commit the setting module**

```bash
git add setting/video_setting/config.go setting/video_setting/config_test.go
git commit -m "feat: add video request settings"
```

### Task 2: Validate Admin Updates and Sync Runtime State

**Files:**
- Create: `controller/option_video_setting_test.go`
- Modify: `controller/option.go`
- Modify: `model/option.go`
- Test: `setting/video_setting/config_test.go`

- [ ] **Step 1: Write failing admin option tests**

Create `controller/option_video_setting_test.go`:

```go
package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/video_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupOptionControllerVideoSettingTest(t *testing.T) *gorm.DB {
	t.Helper()

	gin.SetMode(gin.TestMode)
	originalDB := model.DB
	originalLogDB := model.LOG_DB
	originalRedisEnabled := common.RedisEnabled
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db
	common.RedisEnabled = false
	t.Cleanup(func() {
		model.DB = originalDB
		model.LOG_DB = originalLogDB
		common.RedisEnabled = originalRedisEnabled
		sqlDB, sqlErr := db.DB()
		if sqlErr == nil {
			_ = sqlDB.Close()
		}
	})

	require.NoError(t, db.AutoMigrate(&model.Option{}, &model.User{}, &model.Log{}))
	return db
}

func performUpdateOptionRequest(t *testing.T, key string, rawValue string) *httptest.ResponseRecorder {
	t.Helper()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/option/", strings.NewReader(
		fmt.Sprintf(`{"key":%q,"value":%s}`, key, rawValue),
	))
	ctx.Request.Header.Set("Content-Type", "application/json")

	UpdateOption(ctx)

	return recorder
}

func decodeOptionUpdateResponse(t *testing.T, recorder *httptest.ResponseRecorder) struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
} {
	t.Helper()

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	return response
}

func TestUpdateOptionRejectsInvalidVideoJSONBodyLimitBeforePersistence(t *testing.T) {
	db := setupOptionControllerVideoSettingTest(t)

	for _, rawValue := range []string{`0`, `-1`, `129`, `"1.5"`, `"abc"`} {
		t.Run(rawValue, func(t *testing.T) {
			recorder := performUpdateOptionRequest(t, "video_setting.json_request_body_max_mb", rawValue)
			response := decodeOptionUpdateResponse(t, recorder)

			assert.False(t, response.Success)
			assert.Contains(t, response.Message, "video JSON request body limit")

			var count int64
			require.NoError(t, db.Model(&model.Option{}).Where("key = ?", "video_setting.json_request_body_max_mb").Count(&count).Error)
			assert.Zero(t, count)
		})
	}
}

func TestUpdateOptionAcceptsVideoJSONBodyLimitAndSyncsRuntime(t *testing.T) {
	setupOptionControllerVideoSettingTest(t)

	recorder := performUpdateOptionRequest(t, "video_setting.json_request_body_max_mb", `32`)
	response := decodeOptionUpdateResponse(t, recorder)

	require.True(t, response.Success)
	assert.Equal(t, 32, video_setting.Runtime().JSONRequestBodyMaxMB)
}

func TestUpdateOptionRejectsInvalidVideoBase64Switch(t *testing.T) {
	setupOptionControllerVideoSettingTest(t)

	recorder := performUpdateOptionRequest(t, "video_setting.base64_input_enabled", `"yes"`)
	response := decodeOptionUpdateResponse(t, recorder)

	assert.False(t, response.Success)
	assert.Contains(t, response.Message, "video Base64 input setting")
}
```

- [ ] **Step 2: Run the focused controller test and verify RED**

Run:

```bash
go test ./controller -run 'TestUpdateOption.*Video'
```

Expected: FAIL because controller validation and model runtime sync are not wired.

- [ ] **Step 3: Wire model option updates to the video setting snapshot**

In `model/option.go`, add the import:

```go
	"github.com/QuantumNous/new-api/setting/video_setting"
```

In `handleConfigUpdate`, add the video branch:

```go
	if configName == "performance_setting" {
		performance_setting.UpdateAndSync()
	} else if configName == "tool_price_setting" {
		operation_setting.RebuildToolPriceIndex()
	} else if configName == "billing_setting" {
		InvalidatePricingCache()
		ratio_setting.InvalidateExposedDataCache()
	} else if configName == "theme" {
		system_setting.UpdateAndSyncTheme()
	} else if configName == video_setting.ConfigName {
		video_setting.UpdateAndSync()
	}
```

- [ ] **Step 4: Add controller-side validation before persistence**

In `controller/option.go`, add the import:

```go
	"github.com/QuantumNous/new-api/setting/video_setting"
```

Add these cases inside the existing `switch option.Key` in `UpdateOption`:

```go
	case "video_setting.base64_input_enabled":
		if option.Value != "true" && option.Value != "false" {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "video Base64 input setting must be true or false",
			})
			return
		}
	case "video_setting.json_request_body_max_mb":
		trimmed := strings.TrimSpace(option.Value.(string))
		limit, parseErr := strconv.Atoi(trimmed)
		if parseErr != nil || strconv.Itoa(limit) != trimmed {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "video JSON request body limit must be an integer between 1 and 128 MB",
			})
			return
		}
		if err := video_setting.ValidateJSONRequestBodyMaxMB(limit); err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
```

`controller/option.go` already imports `strconv` and `strings`, so only the `video_setting` import is new.

- [ ] **Step 5: Run focused tests and verify GREEN**

Run:

```bash
go test ./setting/video_setting ./controller -run 'TestRuntime|TestValidate|TestConfigExports|TestUpdateOption.*Video'
```

Expected: PASS.

- [ ] **Step 6: Commit admin validation and runtime sync**

```bash
git add controller/option.go controller/option_video_setting_test.go model/option.go
git commit -m "feat: validate video request settings"
```

### Task 3: Add Failing Middleware Behavior Tests

**Files:**
- Create: `middleware/video_request_policy_test.go`

- [ ] **Step 1: Write middleware tests for size limits, Base64 detection, and body reuse**

Create `middleware/video_request_policy_test.go`:

```go
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
```

- [ ] **Step 2: Run middleware tests and verify RED**

Run:

```bash
go test ./middleware -run 'TestVideoRequestPolicy'
```

Expected: FAIL because `VideoRequestPolicy` is not implemented.

- [ ] **Step 3: Commit the failing middleware tests**

```bash
git add middleware/video_request_policy_test.go
git commit -m "test: cover video request policy"
```

### Task 4: Implement Video Request Policy Middleware

**Files:**
- Create: `middleware/video_request_policy.go`
- Test: `middleware/video_request_policy_test.go`

- [ ] **Step 1: Implement route scoping, JSON detection, and size limiting**

Create `middleware/video_request_policy.go` with this structure:

```go
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
```

- [ ] **Step 2: Add request classification helpers**

Add these helpers below `VideoRequestPolicy`:

```go
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
```

- [ ] **Step 3: Add structured media-field scanning**

Add these helpers:

```go
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
```

`binary_data_base64` is included because the Jimeng video adaptor treats it as image-reference media.

- [ ] **Step 4: Add Base64 classification without storing decoded media**

Add these helpers:

```go
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
	return canStreamDecodeBase64(trimmed)
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
```

- [ ] **Step 5: Add protocol-specific abort helpers and safe logging**

Add these helpers:

```go
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
```

This log format intentionally excludes request body contents, media URLs, auth headers, and token values.

- [ ] **Step 6: Run middleware tests and verify GREEN**

Run:

```bash
go test ./middleware -run 'TestVideoRequestPolicy'
```

Expected: PASS.

- [ ] **Step 7: Run all middleware tests**

Run:

```bash
go test ./middleware
```

Expected: PASS.

- [ ] **Step 8: Commit the middleware implementation**

```bash
git add middleware/video_request_policy.go middleware/video_request_policy_test.go
git commit -m "feat: enforce video request policy"
```

### Task 5: Mount Policy in Video Routes and Keep Converters Compliant

**Files:**
- Modify: `router/video-router.go`
- Create: `router/video_router_test.go`
- Modify: `middleware/kling_adapter.go`
- Modify: `middleware/jimeng_adapter.go`
- Test: `router/video_router_test.go`

- [ ] **Step 1: Write route-order tests**

Create `router/video_router_test.go`:

```go
package router

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/video_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func withRouterVideoPolicyDefaults(t *testing.T) {
	t.Helper()

	cfg := config.GlobalConfig.Get(video_setting.ConfigName)
	require.NotNil(t, cfg)
	saved := video_setting.Runtime()
	t.Cleanup(func() {
		require.NoError(t, config.UpdateConfigFromMap(cfg, map[string]string{
			video_setting.KeyBase64InputEnabled:   strconv.FormatBool(saved.Base64InputEnabled),
			video_setting.KeyJSONRequestBodyMaxMB: strconv.Itoa(saved.JSONRequestBodyMaxMB),
		}))
		video_setting.UpdateAndSync()
	})

	require.NoError(t, config.UpdateConfigFromMap(cfg, map[string]string{
		video_setting.KeyBase64InputEnabled:   "false",
		video_setting.KeyJSONRequestBodyMaxMB: "1",
	}))
	video_setting.UpdateAndSync()
}

func performVideoRouterRequest(router http.Handler, path string, body string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)
	return recorder
}

func TestVideoRouterRejectsBase64BeforePreAuthConverters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	withRouterVideoPolicyDefaults(t)

	router := gin.New()
	SetVideoRouter(router)

	tests := []struct {
		name     string
		path     string
		wantCode string
	}{
		{name: "kling text2video", path: "/kling/v1/videos/text2video", wantCode: "video_base64_input_disabled"},
		{name: "kling image2video", path: "/kling/v1/videos/image2video", wantCode: "video_base64_input_disabled"},
		{name: "jimeng submit", path: "/jimeng/?Action=CVSync2AsyncSubmitTask&Version=2022-08-31", wantCode: "video_base64_input_disabled"},
		{name: "seedance submit", path: "/api/v3/contents/generations/tasks", wantCode: "InvalidParameter.content"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := performVideoRouterRequest(router, tt.path, `{"model":"m","image":"data:image/png;base64,`+strings.Repeat("A", 80)+`"}`)

			require.Equal(t, http.StatusBadRequest, recorder.Code)
			assert.Contains(t, recorder.Body.String(), tt.wantCode)
			assert.NotContains(t, recorder.Body.String(), strings.Repeat("A", 80))
		})
	}
}

func TestVideoRouterSkipsJimengPostQueryAction(t *testing.T) {
	gin.SetMode(gin.TestMode)
	withRouterVideoPolicyDefaults(t)

	router := gin.New()
	SetVideoRouter(router)

	recorder := performVideoRouterRequest(router, "/jimeng/?Action=CVSync2AsyncGetResult&Version=2022-08-31", `{"task_id":"task_1","image":"data:image/png;base64,`+strings.Repeat("A", 80)+`"}`)

	assert.NotEqual(t, http.StatusBadRequest, recorder.Code)
	assert.NotContains(t, recorder.Body.String(), "video_base64_input_disabled")
}
```

- [ ] **Step 2: Run route tests and verify RED**

Run:

```bash
go test ./router -run 'TestVideoRouter'
```

Expected: FAIL because routes do not use `VideoRequestPolicy` yet.

- [ ] **Step 3: Mount the middleware in approved order**

In `router/video-router.go`, change the OpenAI-compatible group to:

```go
	videoV1Router.Use(middleware.TokenAuth(), middleware.VideoRequestPolicy(), middleware.Distribute())
```

Change the Kling group to:

```go
	klingV1Router.Use(middleware.VideoRequestPolicy(), middleware.KlingRequestConvert(), middleware.TokenAuth(), middleware.Distribute())
```

Change the Seedance group to:

```go
	seedanceVideoRouter.Use(middleware.SeedanceRequestConvert(), middleware.VideoRequestPolicy(), middleware.TokenAuth())
```

Change the Jimeng group to:

```go
	jimengOfficialGroup.Use(middleware.VideoRequestPolicy(), middleware.JimengRequestConvert(), middleware.TokenAuth(), middleware.Distribute())
```

- [ ] **Step 4: Replace direct JSON marshal calls in pre-auth converters**

In `middleware/kling_adapter.go`, remove `encoding/json` from imports and replace:

```go
	jsonData, err := json.Marshal(unifiedReq)
```

with:

```go
	jsonData, err := common.Marshal(unifiedReq)
```

In `middleware/jimeng_adapter.go`, remove `encoding/json` from imports and make the same replacement:

```go
	jsonData, err := common.Marshal(unifiedReq)
```

- [ ] **Step 5: Run route and middleware tests**

Run:

```bash
go test ./router ./middleware -run 'TestVideoRouter|TestVideoRequestPolicy|TestKling|TestJimeng|TestSeedance'
```

Expected: PASS.

- [ ] **Step 6: Commit route wiring and converter cleanup**

```bash
git add router/video-router.go router/video_router_test.go middleware/kling_adapter.go middleware/jimeng_adapter.go
git commit -m "feat: protect video generation routes"
```

### Task 6: Add Administrator UI and Locale Values

**Files:**
- Modify: `web/default/src/features/system-settings/types.ts`
- Modify: `web/default/src/features/system-settings/operations/index.tsx`
- Modify: `web/default/src/features/system-settings/operations/section-registry.tsx`
- Modify: `web/default/src/features/system-settings/maintenance/performance-section.tsx`
- Create temporarily: `web/default/scripts/add-missing-keys.mjs`
- Modify through script: `web/default/src/i18n/locales/en.json`
- Modify through script: `web/default/src/i18n/locales/zh.json`
- Modify through script: `web/default/src/i18n/locales/fr.json`
- Modify through script: `web/default/src/i18n/locales/ru.json`
- Modify through script: `web/default/src/i18n/locales/ja.json`
- Modify through script: `web/default/src/i18n/locales/vi.json`

- [ ] **Step 1: Add typed defaults to operations settings**

In `web/default/src/features/system-settings/types.ts`, add these keys to `OperationsSettings`:

```ts
  'video_setting.base64_input_enabled': boolean
  'video_setting.json_request_body_max_mb': number
```

In `web/default/src/features/system-settings/operations/index.tsx`, add defaults to `defaultOperationsSettings`:

```ts
  'video_setting.base64_input_enabled': false,
  'video_setting.json_request_body_max_mb': 16,
```

- [ ] **Step 2: Pass video defaults into the performance section**

In `web/default/src/features/system-settings/operations/section-registry.tsx`, extend the `PerformanceSection` `defaultValues` object:

```tsx
          'video_setting.base64_input_enabled':
            settings['video_setting.base64_input_enabled'] ?? false,
          'video_setting.json_request_body_max_mb':
            settings['video_setting.json_request_body_max_mb'] ?? 16,
```

- [ ] **Step 3: Extend the performance form schema and flattening helpers**

In `web/default/src/features/system-settings/maintenance/performance-section.tsx`, update `perfSchema`:

```ts
const perfSchema = z.object({
  video_setting: z.object({
    base64_input_enabled: z.boolean(),
    json_request_body_max_mb: z.coerce.number().int().min(1).max(128),
  }),
  performance_setting: z.object({
    disk_cache_enabled: z.boolean(),
    disk_cache_threshold_mb: z.coerce.number().min(1),
    disk_cache_max_size_mb: z.coerce.number().min(100),
    disk_cache_path: z.string(),
    monitor_enabled: z.boolean(),
    monitor_cpu_threshold: z.coerce.number().min(0),
    monitor_memory_threshold: z.coerce.number().min(0).max(100),
    monitor_disk_threshold: z.coerce.number().min(0).max(100),
  }),
})
```

Extend `FlatPerfDefaults`:

```ts
  'video_setting.base64_input_enabled': boolean
  'video_setting.json_request_body_max_mb': number
```

Extend `buildFormDefaults`:

```ts
  video_setting: {
    base64_input_enabled: defaults['video_setting.base64_input_enabled'],
    json_request_body_max_mb:
      defaults['video_setting.json_request_body_max_mb'],
  },
```

Extend `normalizeFormValues`:

```ts
  'video_setting.base64_input_enabled':
    values.video_setting.base64_input_enabled,
  'video_setting.json_request_body_max_mb':
    values.video_setting.json_request_body_max_mb,
```

- [ ] **Step 4: Add the settings group above disk cache settings**

In `PerformanceSection`, render this block before the existing Disk Cache Settings block:

```tsx
          <div>
            <h4 className='font-medium'>{t('Video Request Protection')}</h4>
            <p className='text-muted-foreground mt-1 text-xs'>
              {t(
                'Controls JSON request safeguards for video generation submissions.'
              )}
            </p>
          </div>

          <div className='grid grid-cols-1 gap-4 md:grid-cols-2'>
            <FormField
              control={form.control}
              name='video_setting.base64_input_enabled'
              render={({ field }) => (
                <SettingsSwitchItem>
                  <SettingsSwitchContent>
                    <FormLabel>
                      {t('Allow Base64 reference media for video generation')}
                    </FormLabel>
                    <FormDescription>
                      {t(
                        'When disabled, clients should use HTTP(S) URLs or supported multipart uploads for reference images and videos.'
                      )}
                    </FormDescription>
                  </SettingsSwitchContent>
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                </SettingsSwitchItem>
              )}
            />
            <FormField
              control={form.control}
              name='video_setting.json_request_body_max_mb'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Video JSON Request Body Limit (MB)')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={1}
                      max={128}
                      step={1}
                      {...safeNumberFieldProps(field)}
                    />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'Applies only to video JSON requests. Base64 input remains subject to this limit when enabled.'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>

          <Separator />
```

- [ ] **Step 5: Add locale values through the sanctioned script**

Create `web/default/scripts/add-missing-keys.mjs` with the i18n skill's required structure and this `newKeys` object:

```js
const newKeys = {
  en: {
    'Allow Base64 reference media for video generation':
      'Allow Base64 reference media for video generation',
    'Applies only to video JSON requests. Base64 input remains subject to this limit when enabled.':
      'Applies only to video JSON requests. Base64 input remains subject to this limit when enabled.',
    'Controls JSON request safeguards for video generation submissions.':
      'Controls JSON request safeguards for video generation submissions.',
    'Video JSON Request Body Limit (MB)': 'Video JSON Request Body Limit (MB)',
    'Video Request Protection': 'Video Request Protection',
    'When disabled, clients should use HTTP(S) URLs or supported multipart uploads for reference images and videos.':
      'When disabled, clients should use HTTP(S) URLs or supported multipart uploads for reference images and videos.',
  },
  zh: {
    'Allow Base64 reference media for video generation':
      '允许视频生成使用 Base64 参考媒体',
    'Applies only to video JSON requests. Base64 input remains subject to this limit when enabled.':
      '仅作用于视频 JSON 请求。启用 Base64 输入后仍受此上限约束。',
    'Controls JSON request safeguards for video generation submissions.':
      '控制视频生成提交的 JSON 请求防护。',
    'Video JSON Request Body Limit (MB)': '视频 JSON 请求体上限（MB）',
    'Video Request Protection': '视频请求保护',
    'When disabled, clients should use HTTP(S) URLs or supported multipart uploads for reference images and videos.':
      '关闭后，客户端应使用 HTTP(S) URL 或受支持的 multipart 上传来提交参考图片和视频。',
  },
  fr: {
    'Allow Base64 reference media for video generation':
      'Autoriser les médias de référence Base64 pour la génération vidéo',
    'Applies only to video JSON requests. Base64 input remains subject to this limit when enabled.':
      'S’applique uniquement aux requêtes JSON vidéo. Les entrées Base64 restent soumises à cette limite lorsqu’elles sont activées.',
    'Controls JSON request safeguards for video generation submissions.':
      'Contrôle les protections des requêtes JSON pour les soumissions de génération vidéo.',
    'Video JSON Request Body Limit (MB)':
      'Limite du corps JSON vidéo (Mo)',
    'Video Request Protection': 'Protection des requêtes vidéo',
    'When disabled, clients should use HTTP(S) URLs or supported multipart uploads for reference images and videos.':
      'Lorsqu’elle est désactivée, les clients doivent utiliser des URL HTTP(S) ou les téléversements multipart pris en charge pour les images et vidéos de référence.',
  },
  ja: {
    'Allow Base64 reference media for video generation':
      '動画生成で Base64 参照メディアを許可',
    'Applies only to video JSON requests. Base64 input remains subject to this limit when enabled.':
      '動画 JSON リクエストのみに適用されます。Base64 入力を有効にしてもこの上限の対象です。',
    'Controls JSON request safeguards for video generation submissions.':
      '動画生成送信の JSON リクエスト保護を制御します。',
    'Video JSON Request Body Limit (MB)':
      '動画 JSON リクエスト本文上限（MB）',
    'Video Request Protection': '動画リクエスト保護',
    'When disabled, clients should use HTTP(S) URLs or supported multipart uploads for reference images and videos.':
      '無効な場合、クライアントは参照画像や動画に HTTP(S) URL または対応する multipart アップロードを使用してください。',
  },
  ru: {
    'Allow Base64 reference media for video generation':
      'Разрешить справочные медиа Base64 для генерации видео',
    'Applies only to video JSON requests. Base64 input remains subject to this limit when enabled.':
      'Применяется только к видео-запросам JSON. Ввод Base64 при включении также ограничен этим лимитом.',
    'Controls JSON request safeguards for video generation submissions.':
      'Управляет защитой JSON-запросов при отправке задач генерации видео.',
    'Video JSON Request Body Limit (MB)':
      'Лимит тела JSON-запроса видео (МБ)',
    'Video Request Protection': 'Защита видео-запросов',
    'When disabled, clients should use HTTP(S) URLs or supported multipart uploads for reference images and videos.':
      'Если отключено, клиенты должны использовать HTTP(S) URL или поддерживаемые multipart-загрузки для справочных изображений и видео.',
  },
  vi: {
    'Allow Base64 reference media for video generation':
      'Cho phép phương tiện tham chiếu Base64 cho tạo video',
    'Applies only to video JSON requests. Base64 input remains subject to this limit when enabled.':
      'Chỉ áp dụng cho yêu cầu JSON video. Dữ liệu Base64 vẫn chịu giới hạn này khi được bật.',
    'Controls JSON request safeguards for video generation submissions.':
      'Kiểm soát các lớp bảo vệ yêu cầu JSON khi gửi tác vụ tạo video.',
    'Video JSON Request Body Limit (MB)':
      'Giới hạn nội dung JSON video (MB)',
    'Video Request Protection': 'Bảo vệ yêu cầu video',
    'When disabled, clients should use HTTP(S) URLs or supported multipart uploads for reference images and videos.':
      'Khi tắt, máy khách nên dùng URL HTTP(S) hoặc tải lên multipart được hỗ trợ cho ảnh và video tham chiếu.',
  },
}
```

The rest of the script must be exactly the i18n skill's `add-missing-keys.mjs` wrapper: read each locale JSON, apply `newKeys`, sort keys, and write the files.

- [ ] **Step 6: Run i18n script and sync**

Run from `web/default/`:

```bash
node scripts/add-missing-keys.mjs
bun run i18n:sync
```

Expected: all six locale files contain the six new keys. Existing `_reports` changes may update; do not stage `_reports`.

- [ ] **Step 7: Delete the temporary locale script**

Run:

```bash
Remove-Item -LiteralPath .\web\default\scripts\add-missing-keys.mjs
```

Expected: `web/default/scripts/add-missing-keys.mjs` is removed after applying translations.

- [ ] **Step 8: Run frontend checks**

Run from `web/default/`:

```bash
bun run typecheck
bun run build
```

Expected: both commands pass.

- [ ] **Step 9: Commit administrator UI and locale updates**

```bash
git add web/default/src/features/system-settings/types.ts web/default/src/features/system-settings/operations/index.tsx web/default/src/features/system-settings/operations/section-registry.tsx web/default/src/features/system-settings/maintenance/performance-section.tsx web/default/src/i18n/locales/en.json web/default/src/i18n/locales/zh.json web/default/src/i18n/locales/fr.json web/default/src/i18n/locales/ru.json web/default/src/i18n/locales/ja.json web/default/src/i18n/locales/vi.json
git commit -m "feat(web): add video request protection settings"
```

### Task 7: Full Verification and Acceptance

**Files:**
- Verify all files touched by Tasks 1-6

- [ ] **Step 1: Run focused backend tests**

Run:

```bash
go test ./setting/video_setting ./controller ./middleware ./router -run 'TestRuntime|TestValidate|TestConfigExports|TestUpdateOption.*Video|TestVideoRequestPolicy|TestVideoRouter'
```

Expected: PASS.

- [ ] **Step 2: Run broader backend tests around relay task behavior**

Run:

```bash
go test ./controller ./middleware ./router ./relay ./relay/common ./relay/channel/task/...
```

Expected: PASS.

- [ ] **Step 3: Run frontend verification**

Run from `web/default/`:

```bash
bun run i18n:sync
bun run typecheck
bun run build
```

Expected: PASS. Locale sync may refresh `_reports`; leave pre-existing `_reports` files unstaged unless the user requests them.

- [ ] **Step 4: Inspect the final diff for security invariants**

Run:

```bash
git diff --stat
git diff -- middleware/video_request_policy.go router/video-router.go setting/video_setting/config.go controller/option.go model/option.go
```

Confirm:

- Base64 and oversized JSON rejections happen before `Distribute`, channel selection, billing, retry, upstream calls, and task insertion.
- `common.Marshal`, `common.Unmarshal`, or `common.DecodeJson` are used for JSON operations in changed backend business code.
- Logs include only path, byte count, limit, reason, and parameter path.
- Error responses do not include media values or full request bodies.
- HTTP(S) URLs, malformed JSON, multipart bodies, task queries, and downloads are not rejected by this policy.
- Seedance errors use the ARK `{ "error": { "code", "message" } }` envelope.

- [ ] **Step 5: Commit any verification-only fixes**

If verification required fixes, stage only the changed implementation and locale files:

```bash
git add setting/video_setting/config.go setting/video_setting/config_test.go controller/option.go controller/option_video_setting_test.go model/option.go middleware/video_request_policy.go middleware/video_request_policy_test.go middleware/kling_adapter.go middleware/jimeng_adapter.go router/video-router.go router/video_router_test.go web/default/src/features/system-settings/types.ts web/default/src/features/system-settings/operations/index.tsx web/default/src/features/system-settings/operations/section-registry.tsx web/default/src/features/system-settings/maintenance/performance-section.tsx web/default/src/i18n/locales/en.json web/default/src/i18n/locales/zh.json web/default/src/i18n/locales/fr.json web/default/src/i18n/locales/ru.json web/default/src/i18n/locales/ja.json web/default/src/i18n/locales/vi.json
git commit -m "fix: finalize video request policy"
```

Expected: no `_reports` files are staged.

## Self-Review

**Spec coverage:** The plan covers default Base64 rejection, global admin enablement, 1-128 MB JSON limit, safe fallback to 16 MB, pre-converter/pre-distribution enforcement, structured media-field detection, Jimeng query skip, Seedance ARK errors, OpenAI-compatible errors, safe logging, frontend settings, six-locale i18n, and backend/frontend verification.

**Placeholder scan:** No task depends on placeholder text, vague follow-up work, or unspecified error handling. Each code-changing task includes the concrete code shape or exact snippets to add.

**Type consistency:** The backend keys are consistently `video_setting.base64_input_enabled` and `video_setting.json_request_body_max_mb`; the Go runtime uses `Base64InputEnabled`, `JSONRequestBodyMaxMB`, and `JSONRequestBodyMaxBytes`; the frontend form models dotted keys internally as nested `video_setting` fields and flattens them only at save time.
