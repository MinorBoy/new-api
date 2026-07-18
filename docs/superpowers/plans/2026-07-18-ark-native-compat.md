# ARK 原生 API 兼容入口 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让火山方舟(ARK) SDK 用户把 `base_url` 指向 new-api 即可零改造接入,原样支持 ARK 原生视频生成(`POST/GET /api/v3/contents/generations/tasks`)和图像生成(`POST /api/v3/images/generations`)两类 API 的请求与响应格式。

**Architecture:** 双向 Impersonation Pattern——一个 `SeedanceRequestConvert` 中间件把 `/seedance/api/v3/*` 原生入口改写为内部统一的 `/v1/*` 端点进入标准 pipeline,再通过 context 标记 `KeySeedanceOfficialAPI` 在 adaptor/relay 出口处把响应还原成 ARK 原生格式。视频部分以社区 PR #5737 为基线(已实现视频提交/查询/列表),本计划在其基础上:(1) 修复 CodeRabbit 指出的两个 review 问题,(2) 补齐图像原生入口(两 PR 都未覆盖)。

**Tech Stack:** Go 1.22+, Gin, GORM v2, testify(require+assert),JSON 统一走 `common.*`。

**参考基线:** [PR #5737](https://github.com/QuantumNous/new-api/pull/5737) `feat/seedance-native-compat` 分支(9 文件 583 行,REVIEW_REQUIRED)。

---

## Scope(本计划只做接入,不做计费)

✅ 视频原生入口(基于 #5737,含 review 修复)
✅ 图像原生入口(新增,#5737 未覆盖)
❌ 计费精确化(见独立计划 `2026-07-18-seedance-billing.md`)
❌ chat/embeddings/responses/rerank 入口(用户未选)
❌ 新增渠道类型(复用 VolcEngine 45)

---

## File Structure

| 文件 | 责任 | 来源 |
|---|---|---|
| `common/gin.go` | 新增 `KeySeedanceOfficialAPI` context key | #5737 |
| `middleware/seedance_adapter.go` | `SeedanceRequestConvert()` 请求改写中间件(视频+图像) | #5737 扩展 |
| `middleware/seedance_adapter_test.go` | 中间件测试 | #5737 + 补图像用例 |
| `controller/seedance.go` | `RelaySeedanceTask`/`RelaySeedanceTaskFetch` 控制器 | #5737 |
| `router/video-router.go` | 注册 `/seedance/api/v3/contents/generations/tasks` 视频路由组 | #5737 |
| `router/relay-router.go` | 注册 `/seedance/api/v3/images/generations` 图像路由组 | 新增 |
| `relay/seedance_task.go` | `SeedanceTaskFetch` + 响应还原 | #5737 + 性能修复 |
| `relay/relay_task_seedance_test.go` | relay 层测试 | #5737 |
| `relay/channel/task/doubao/adaptor.go` | Seedance 分支(Validate/BuildRequest/DoResponse/ParseTaskResult) | #5737 + 模型名回填修复 |
| `relay/channel/task/doubao/adaptor_test.go` | adaptor 测试 | #5737 |
| `relay/image_handler.go` | PassThrough 追加 Seedance 入口条件 | 新增(1 行) |

---

## Task 1: 落地 context key(基础设施)

**Files:**
- Modify: `common/gin.go`(在 `KeyBodyStorage` 之后追加)

- [ ] **Step 1: 添加 context key 常量**

修改 `common/gin.go`,在 `const KeyBodyStorage = "key_body_storage"` 之后追加:

```go
const KeyRequestBody = "key_request_body"
const KeyBodyStorage = "key_body_storage"
const KeySeedanceOfficialAPI = "seedance_official_api" // ARK 原生 API 入口标记,触发响应原生格式还原
```

- [ ] **Step 2: 验证编译**

Run: `go build ./common/`
Expected: 无输出(成功)

- [ ] **Step 3: Commit**

```bash
git add common/gin.go
git commit -m "feat(seedance): add KeySeedanceOfficialAPI context key"
```

---

## Task 2: 实现 `SeedanceRequestConvert` 中间件(视频+图像路径识别)

**Files:**
- Create: `middleware/seedance_adapter.go`
- Test: `middleware/seedance_adapter_test.go`

- [ ] **Step 1: 写失败测试(视频提交路径改写)**

创建 `middleware/seedance_adapter_test.go`:

```go
package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSeedanceRequestConvertVideoSubmit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(SeedanceRequestConvert())
	router.POST("/seedance/api/v3/contents/generations/tasks", func(c *gin.Context) {
		require.True(t, c.GetBool(common.KeySeedanceOfficialAPI), "context key must be set")
		require.Equal(t, "/v1/video/generations", c.Request.URL.Path, "POST video path must be rewritten")
		require.Equal(t, relayconstant.RelayModeVideoSubmit, c.GetInt("relay_mode"))
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodPost, "/seedance/api/v3/contents/generations/tasks", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusNoContent, recorder.Code)
}
```

- [ ] **Step 2: 运行测试,验证失败**

Run: `go test ./middleware/ -run TestSeedanceRequestConvertVideoSubmit -v`
Expected: FAIL with `undefined: SeedanceRequestConvert`

- [ ] **Step 3: 写最小实现**

创建 `middleware/seedance_adapter.go`:

```go
package middleware

import (
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"

	"github.com/gin-gonic/gin"
)

// SeedanceRequestConvert 把火山方舟(ARK)原生 API 入口请求改写为内部统一路径,
// 并标记 KeySeedanceOfficialAPI 供下游 adaptor/relay 还原 ARK 原生响应格式。
// 入口前缀 /seedance/api/v3/* 与官方 ARK SDK 路径对齐(用户只需把 base_url 指向 new-api/seedance)。
func SeedanceRequestConvert() func(c *gin.Context) {
	return func(c *gin.Context) {
		c.Set(common.KeySeedanceOfficialAPI, true)
		path := c.Request.URL.Path

		if c.Request.Method == http.MethodPost {
			switch {
			case strings.Contains(path, "/contents/generations/tasks"):
				// 视频任务提交:POST /seedance/api/v3/contents/generations/tasks
				c.Request.URL.Path = "/v1/video/generations"
				c.Set("relay_mode", relayconstant.RelayModeVideoSubmit)
			case strings.HasSuffix(path, "/images/generations"):
				// 图像生成:POST /seedance/api/v3/images/generations
				c.Request.URL.Path = "/v1/images/generations"
				c.Set("relay_mode", relayconstant.RelayModeImagesGenerations)
			}
		}
		c.Next()
	}
}
```

- [ ] **Step 4: 运行测试,验证通过**

Run: `go test ./middleware/ -run TestSeedanceRequestConvertVideoSubmit -v`
Expected: PASS

- [ ] **Step 5: 追加图像路径测试**

在 `middleware/seedance_adapter_test.go` 末尾追加:

```go
func TestSeedanceRequestConvertImageSubmit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(SeedanceRequestConvert())
	router.POST("/seedance/api/v3/images/generations", func(c *gin.Context) {
		require.True(t, c.GetBool(common.KeySeedanceOfficialAPI))
		require.Equal(t, "/v1/images/generations", c.Request.URL.Path)
		require.Equal(t, relayconstant.RelayModeImagesGenerations, c.GetInt("relay_mode"))
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodPost, "/seedance/api/v3/images/generations", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusNoContent, recorder.Code)
}
```

- [ ] **Step 6: 运行新测试**

Run: `go test ./middleware/ -run TestSeedanceRequestConvertImageSubmit -v`
Expected: PASS

- [ ] **Step 7: 追加查询路径(不改写)测试**

在 `middleware/seedance_adapter_test.go` 追加(验证 GET 路径保持不变,由后续 controller 处理 task_id 提取):

```go
func TestSeedanceRequestConvertFetchKeepsPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(SeedanceRequestConvert())
	router.GET("/seedance/api/v3/contents/generations/tasks/:task_id", func(c *gin.Context) {
		require.True(t, c.GetBool(common.KeySeedanceOfficialAPI))
		// GET 路径不改写,task_id 由 controller.RelaySeedanceTaskFetch 从 c.Param 读取
		require.Equal(t, "/seedance/api/v3/contents/generations/tasks/task_public", c.Request.URL.Path)
		require.Equal(t, "task_public", c.Param("task_id"))
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/seedance/api/v3/contents/generations/tasks/task_public", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusNoContent, recorder.Code)
}
```

- [ ] **Step 8: 运行全部中间件测试**

Run: `go test ./middleware/ -run TestSeedance -v`
Expected: 3 个测试全部 PASS

- [ ] **Step 9: Commit**

```bash
git add middleware/seedance_adapter.go middleware/seedance_adapter_test.go
git commit -m "feat(seedance): add SeedanceRequestConvert middleware for video+image paths"
```

---

## Task 3: 实现控制器 `RelaySeedanceTask` / `RelaySeedanceTaskFetch`

**Files:**
- Create: `controller/seedance.go`

- [ ] **Step 1: 写实现(控制器本身较薄,无独立单元测试,依赖 Task 4/6 的集成验证)**

创建 `controller/seedance.go`:

```go
package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relay"
	"github.com/gin-gonic/gin"
)

// RelaySeedanceTask 处理 ARK 原生视频任务提交。
// 设置 KeySeedanceOfficialAPI 标记后委托通用 RelayTask。
func RelaySeedanceTask(c *gin.Context) {
	c.Set(common.KeySeedanceOfficialAPI, true)
	RelayTask(c)
}

// RelaySeedanceTaskFetch 处理 ARK 原生视频任务查询(单个或列表)。
// 响应格式由 relay.SeedanceTaskFetch 按 ARK 原生结构组装。
func RelaySeedanceTaskFetch(c *gin.Context) {
	c.Set(common.KeySeedanceOfficialAPI, true)
	respBody, taskErr := relay.SeedanceTaskFetch(c)
	if taskErr != nil {
		respondTaskError(c, taskErr)
		return
	}
	c.Data(http.StatusOK, "application/json", respBody)
}
```

- [ ] **Step 2: 验证编译(此时 relay.SeedanceTaskFetch 尚未实现,预期编译失败)**

Run: `go build ./controller/`
Expected: FAIL with `undefined: relay.SeedanceTaskFetch`(Task 6 会实现)

- [ ] **Step 3: 暂不 commit,继续 Task 4-6 后统一编译验证**

---

## Task 4: 注册视频路由组

**Files:**
- Modify: `router/video-router.go`(在 klingV1Router 块之后、jimengOfficialGroup 块之前插入)

- [ ] **Step 1: 追加视频路由组**

修改 `router/video-router.go`,在 jimeng 路由组(`// Jimeng official API routes`)之前插入:

```go
	// Seedance (火山方舟 ARK) 原生 API 路由 - 视频任务
	// 用户把 ARK SDK 的 base_url 指向 https://your-host/seedance 即可零改造接入
	// 路径与官方 ARK 视频接口对齐:/api/v3/contents/generations/tasks
	seedanceOfficialGroup := router.Group("/seedance/api/v3/contents/generations")
	seedanceOfficialGroup.Use(middleware.RouteTag("relay"))
	seedanceOfficialGroup.Use(middleware.SeedanceRequestConvert(), middleware.TokenAuth())
	{
		// POST 提交任务,Distribute 在 handler 内由 RelayTask 触发
		seedanceOfficialGroup.POST("/tasks", middleware.Distribute(), controller.RelaySeedanceTask)
		// GET 查询单个任务 / 列表任务(无 task_id 参数走列表)
		seedanceOfficialGroup.GET("/tasks", controller.RelaySeedanceTaskFetch)
		seedanceOfficialGroup.GET("/tasks/:task_id", controller.RelaySeedanceTaskFetch)
	}
```

> **注意:** POST 路由的 `middleware.Distribute()` 必须在 `controller.RelaySeedanceTask` 之前(与 jimeng 模式一致:`JimengRequestConvert → TokenAuth → Distribute`)。GET 查询路由不需要 Distribute(任务记录已固化渠道)。

- [ ] **Step 2: 验证编译**

Run: `go build ./router/`
Expected: 无输出(成功,因为 controller.RelaySeedanceTask 已在 Task 3 创建)

- [ ] **Step 3: Commit(暂不 push,等 Task 6 relay.SeedanceTaskFetch 实现后整体编译通过再 push)**

```bash
git add router/video-router.go controller/seedance.go
git commit -m "feat(seedance): register ARK native video routes and controllers"
```

---

## Task 5: doubao adaptor 的 Seedance 分支(请求/响应方向)

**Files:**
- Modify: `relay/channel/task/doubao/adaptor.go`
- Test: `relay/channel/task/doubao/adaptor_test.go`

> 此 Task 同时包含 **CodeRabbit 指出的"模型名回填"review 修复**。

- [ ] **Step 1: 写失败测试(BuildRequestBody 保留原生 content)**

创建 `relay/channel/task/doubao/adaptor_test.go`:

```go
package doubao

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSeedanceBuildRequestBodyPreservesNativeContent 验证 ARK 原生请求体的
// content[] 多模态结构被原样透传到上游(仅改写 model 字段)。
// 这是问题1中"9图3视频1音频"场景能否工作的关键不变量。
func TestSeedanceBuildRequestBodyPreservesNativeContent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set(common.KeySeedanceOfficialAPI, true)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/seedance/api/v3/contents/generations/tasks", bytes.NewBufferString(`{
		"model":"doubao-seedance-2-0-260128",
		"content":[
			{"type":"image_url","image_url":{"url":"https://example.com/a.png"},"role":"first_frame"},
			{"type":"video_url","video_url":{"url":"https://example.com/ref.mp4"}},
			{"type":"audio_url","audio_url":{"url":"https://example.com/bg.mp3"}},
			{"type":"text","text":"镜头缓慢推进"}
		],
		"duration":15,
		"resolution":"720p",
		"ratio":"16:9",
		"watermark":false
	}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	// 模拟 common.GetBodyStorage 读取请求体(实际由 middleware 预存)
	// 这里直接通过 GetBodyStorage 需要 storage middleware,改用更简单的注入方式
	adaptor := &TaskAdaptor{}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "upstream-mapped-model"},
	}

	body, err := adaptor.BuildRequestBody(ctx, info)
	require.NoError(t, err)

	raw, err := io.ReadAll(body)
	require.NoError(t, err)

	var payload map[string]any
	require.NoError(t, common.Unmarshal(raw, &payload))

	// model 字段被渠道模型映射覆盖
	assert.Equal(t, "upstream-mapped-model", payload["model"])

	// 原生字段全部保留(透传,不丢字段)
	assert.Equal(t, float64(15), payload["duration"])
	assert.Equal(t, "720p", payload["resolution"])
	assert.Equal(t, "16:9", payload["ratio"])
	assert.Equal(t, false, payload["watermark"])

	// content[] 多模态结构完整保留(4 项:image/video/audio/text)
	content, ok := payload["content"].([]any)
	require.True(t, ok, "content must be preserved as array")
	require.Len(t, content, 4, "all 4 content items must survive")

	first, ok := content[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "image_url", first["type"])
	assert.Equal(t, "first_frame", first["role"])
}
```

> **注意:** 上面的测试依赖 `common.GetBodyStorage(c)` 能读出请求体。在真实流程中,`middleware.TokenAuth` 之前的 `middleware.Common` 链会通过 `UnmarshalBodyReusable` 把 body 缓存到 storage。**测试中需要手动设置 storage**,否则 `GetBodyStorage` 会报错。在 Step 3 的实现中会处理这个测试 setup。如果测试因 storage 报错,在测试开头加:
> ```go
> ctx.Set(common.KeyBodyStorage, common.NewBytesBodyStorage([]byte(`{...}`)))
> ```
> (具体 storage 构造函数名需 grep `common/` 确认,执行时核实)

- [ ] **Step 2: 运行测试,验证失败**

Run: `go test ./relay/channel/task/doubao/ -run TestSeedanceBuildRequestBody -v`
Expected: FAIL(当前 BuildRequestBody 无 Seedance 分支,会走默认路径尝试 `relaycommon.GetTaskRequest(c)` 报 "request not found in context")

- [ ] **Step 3: 实现 BuildRequestBody 的 Seedance 分支(含模型名回填修复)**

修改 `relay/channel/task/doubao/adaptor.go`,在 `BuildRequestBody` 函数开头(`req, err := relaycommon.GetTaskRequest(c)` 之前)插入:

```go
// BuildRequestBody converts request into Doubao specific format.
func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	// ARK 原生入口:原样透传请求体(仅改写 model 字段),保留 content[] 多模态结构
	if c.GetBool(common.KeySeedanceOfficialAPI) {
		storage, err := common.GetBodyStorage(c)
		if err != nil {
			return nil, err
		}
		cachedBody, err := storage.Bytes()
		if err != nil {
			return nil, err
		}

		var bodyMap map[string]interface{}
		if err := common.Unmarshal(cachedBody, &bodyMap); err != nil {
			// 解析失败时原样转发(尽量不阻断请求)
			return bytes.NewReader(cachedBody), nil
		}

		// 渠道模型映射:若配置了 UpstreamModelName 则覆盖,否则反向回填保证计费归属正确
		// (CodeRabbit review:修复模型名回填缺失导致的计费归属错误)
		if info.UpstreamModelName != "" {
			bodyMap["model"] = info.UpstreamModelName
		} else if m, _ := bodyMap["model"].(string); m != "" {
			info.UpstreamModelName = m
		}

		data, err := common.Marshal(bodyMap)
		if err != nil {
			return nil, err
		}
		return bytes.NewReader(data), nil
	}

	req, err := relaycommon.GetTaskRequest(c)
	// ... 原有逻辑保留 ...
```

- [ ] **Step 4: 运行测试,验证通过**

Run: `go test ./relay/channel/task/doubao/ -run TestSeedanceBuildRequestBody -v`
Expected: PASS(如果 storage setup 正确;若报 storage 错误,调整测试 setup 后再跑)

- [ ] **Step 5: 写失败测试(ValidateRequestAndSetAction 的 Seedance 分支)**

在 `adaptor_test.go` 追加:

```go
// TestSeedanceValidateRejectsMissingModel 验证 ARK 原生请求必须有 model 和 content 字段
func TestSeedanceValidateRejectsMissingModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set(common.KeySeedanceOfficialAPI, true)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/seedance/api/v3/contents/generations/tasks",
		bytes.NewBufferString(`{"content":[{"type":"text","text":"hi"}]}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	adaptor := &TaskAdaptor{}
	info := &relaycommon.RelayInfo{}
	taskErr := adaptor.ValidateRequestAndSetAction(ctx, info)

	require.NotNil(t, taskErr, "missing model must be rejected")
	assert.Contains(t, taskErr.Code, "missing_model")
}

func TestSeedanceValidateRejectsMissingContent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set(common.KeySeedanceOfficialAPI, true)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/seedance/api/v3/contents/generations/tasks",
		bytes.NewBufferString(`{"model":"doubao-seedance-2-0-260128"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	adaptor := &TaskAdaptor{}
	info := &relaycommon.RelayInfo{}
	taskErr := adaptor.ValidateRequestAndSetAction(ctx, info)

	require.NotNil(t, taskErr, "missing content must be rejected")
	assert.Contains(t, taskErr.Code, "missing_content")
}
```

- [ ] **Step 6: 运行,验证失败**

Run: `go test ./relay/channel/task/doubao/ -run "TestSeedanceValidate" -v`
Expected: FAIL(当前 ValidateRequestAndSetAction 无 Seedance 分支)

- [ ] **Step 7: 实现 ValidateRequestAndSetAction 的 Seedance 分支**

修改 `adaptor.go` 的 `ValidateRequestAndSetAction`(`return relaycommon.ValidateBasicTaskRequest(...)` 之前)插入:

```go
// ValidateRequestAndSetAction parses body, validates fields and sets default action.
func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) (taskErr *dto.TaskError) {
	// ARK 原生入口:解析原生请求体,提取 model/content,把整个 body 存入 metadata 供 BuildRequestBody 透传
	if c.GetBool(common.KeySeedanceOfficialAPI) {
		var body map[string]interface{}
		if err := common.UnmarshalBodyReusable(c, &body); err != nil {
			return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
		}

		modelName, _ := body["model"].(string)
		if strings.TrimSpace(modelName) == "" {
			return service.TaskErrorWrapperLocal(fmt.Errorf("field model is required"), "missing_model", http.StatusBadRequest)
		}
		if _, ok := body["content"]; !ok {
			return service.TaskErrorWrapperLocal(fmt.Errorf("field content is required"), "missing_content", http.StatusBadRequest)
		}

		info.Action = constant.TaskActionGenerate
		// 把整个原生 body 存入 metadata,后续 BuildRequestBody 会从 storage 读原始字节,
		// 这里设置 task_request 主要是为了填 Prompt 供校验日志
		c.Set("task_request", relaycommon.TaskSubmitReq{
			Model:    modelName,
			Prompt:   seedanceTextPrompt(body),
			Metadata: body,
		})
		return nil
	}

	// Accept only POST /v1/video/generations as "generate" action.
	return relaycommon.ValidateBasicTaskRequest(c, info, constant.TaskActionGenerate)
}
```

并新增 import `"strings"` 和 `"fmt"`(若未导入),以及辅助函数(放在 `convertToRequestPayload` 附近):

```go
// seedanceTextPrompt 从 ARK 原生 content[] 数组中提取第一个 text 项作为 prompt。
// 用于校验/日志,不影响 BuildRequestBody 的原始字节透传。
func seedanceTextPrompt(body map[string]interface{}) string {
	content, ok := body["content"].([]interface{})
	if !ok {
		return ""
	}
	for _, item := range content {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if itemMap["type"] != "text" {
			continue
		}
		text, _ := itemMap["text"].(string)
		if strings.TrimSpace(text) != "" {
			return text
		}
	}
	return ""
}
```

- [ ] **Step 8: 运行,验证通过**

Run: `go test ./relay/channel/task/doubao/ -run "TestSeedanceValidate" -v`
Expected: PASS

- [ ] **Step 9: 写失败测试(DoResponse 返回原生 {id} 格式)**

在 `adaptor_test.go` 追加:

```go
// TestSeedanceDoResponseReturnsUpstreamTaskID 验证 ARK 原生入口的提交响应是 {id} 而非 OpenAI Video 格式
func TestSeedanceDoResponseReturnsUpstreamTaskID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set(common.KeySeedanceOfficialAPI, true)

	adaptor := &TaskAdaptor{}
	resp := &http.Response{
		Body: io.NopCloser(bytes.NewBufferString(`{"id":"cgt-upstream-123"}`)),
	}
	info := &relaycommon.RelayInfo{
		TaskRelayInfo: &relaycommon.TaskRelayInfo{PublicTaskID: "task_public"},
	}

	taskID, taskData, taskErr := adaptor.DoResponse(ctx, resp, info)
	require.Nil(t, taskErr)
	assert.Equal(t, "cgt-upstream-123", taskID, "must return upstream task id")
	assert.JSONEq(t, `{"id":"cgt-upstream-123"}`, string(taskData), "taskData must be original upstream response")
	assert.JSONEq(t, `{"id":"cgt-upstream-123"}`, recorder.Body.String(), "client must receive ARK native {id} shape")
}
```

- [ ] **Step 10: 运行,验证失败**

Run: `go test ./relay/channel/task/doubao/ -run TestSeedanceDoResponse -v`
Expected: FAIL(当前 DoResponse 总是返回 OpenAIVideo 格式)

- [ ] **Step 11: 实现 DoResponse 的 Seedance 分支**

修改 `adaptor.go` 的 `DoResponse`,在 `if dResp.ID == "" { ... }` 检查之后、`ov := dto.NewOpenAIVideo()` 之前插入:

```go
	if dResp.ID == "" {
		taskErr = service.TaskErrorWrapper(fmt.Errorf("task_id is empty"), "invalid_response", http.StatusInternalServerError)
		return
	}

	// ARK 原生入口:返回上游原生的 {id} 简洁格式
	if c.GetBool(common.KeySeedanceOfficialAPI) {
		c.JSON(http.StatusOK, gin.H{
			"id": dResp.ID,
		})
		return dResp.ID, responseBody, nil
	}

	ov := dto.NewOpenAIVideo()
	// ... 原有逻辑保留 ...
```

- [ ] **Step 12: 运行,验证通过**

Run: `go test ./relay/channel/task/doubao/ -run TestSeedanceDoResponse -v`
Expected: PASS

- [ ] **Step 13: 写测试(ParseTaskResult 新增 expired/cancelled 状态)**

在 `adaptor_test.go` 追加:

```go
// TestParseTaskResultMapsExpiredAndCancelled 验证 expired/cancelled 被映射为 FAILURE
func TestParseTaskResultMapsExpiredAndCancelled(t *testing.T) {
	adaptor := &TaskAdaptor{}

	for _, status := range []string{`"expired"`, `"cancelled"`} {
		body := []byte(`{"id":"t1","status":` + status + `}`)
		result, err := adaptor.ParseTaskResult(body)
		require.NoError(t, err)
		assert.Equal(t, model.TaskStatusFailure, result.Status, "status %s must map to FAILURE", status)
		assert.Equal(t, "100%", result.Progress)
	}
}
```

- [ ] **Step 14: 运行,验证失败**

Run: `go test ./relay/channel/task/doubao/ -run TestParseTaskResultMapsExpiredAndCancelled -v`
Expected: FAIL(当前 ParseTaskResult 无 expired/cancelled case,落入 default 返回 InProgress)

- [ ] **Step 15: 实现 expired/cancelled 状态映射**

修改 `adaptor.go` 的 `ParseTaskResult`,在 `case "failed":` 之后、`default:` 之前插入:

```go
	case "failed":
		taskResult.Status = model.TaskStatusFailure
		taskResult.Progress = "100%"
		taskResult.Reason = resTask.Error.Message
	case "expired", "cancelled":
		// ARK 原生状态补充:任务过期或被取消,统一映射为 FAILURE
		taskResult.Status = model.TaskStatusFailure
		taskResult.Progress = "100%"
		taskResult.Reason = resTask.Status
	default:
```

- [ ] **Step 16: 运行,验证通过**

Run: `go test ./relay/channel/task/doubao/ -run TestParseTaskResultMapsExpiredAndCancelled -v`
Expected: PASS

- [ ] **Step 17: 运行 doubao 包全部测试**

Run: `go test ./relay/channel/task/doubao/ -v`
Expected: 全部 PASS

- [ ] **Step 18: Commit**

```bash
git add relay/channel/task/doubao/adaptor.go relay/channel/task/doubao/adaptor_test.go
git commit -m "feat(seedance): doubao adaptor Seedance branches (validate/build/doResponse) + model name backfill fix"
```

---

## Task 6: 实现 `relay.SeedanceTaskFetch`(查询/列表 + 响应还原 + 性能修复)

**Files:**
- Create: `relay/seedance_task.go`
- Test: `relay/relay_task_seedance_test.go`

> 此 Task 包含 **CodeRabbit 指出的"列表查询性能"review 修复**:把能下推 SQL 的过滤条件下推到 GORM,避免全量加载到内存。

- [ ] **Step 1: 写失败测试(任务 ID 过滤参数解析)**

创建 `relay/relay_task_seedance_test.go`:

```go
package relay

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSeedanceTaskIDFilters 验证 filter.task_ids 同时支持逗号分隔和重复参数两种风格
func TestSeedanceTaskIDFilters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(
		http.MethodGet,
		"/seedance/api/v3/contents/generations/tasks?filter.task_ids=cgt-a,cgt-b&filter.task_ids=task_c&filter.task_ids[]=cgt-d",
		nil,
	)

	require.Equal(t, []string{"cgt-a", "cgt-b", "task_c", "cgt-d"}, seedanceTaskIDFilters(ctx))
}
```

- [ ] **Step 2: 运行,验证失败**

Run: `go test ./relay/ -run TestSeedanceTaskIDFilters -v`
Expected: FAIL with `undefined: seedanceTaskIDFilters`

- [ ] **Step 3: 写失败测试(响应还原 ARK 原生格式)**

在 `relay_task_seedance_test.go` 追加:

```go
// TestSeedanceTaskResponseUsesUpstreamShape 验证查询响应使用 ARK 原生 responseTask 形态
// 而非 OpenAI Video 格式(这是 ARK SDK 用户零改造的关键)
func TestSeedanceTaskResponseUsesUpstreamShape(t *testing.T) {
	task := &model.Task{
		TaskID:     "task_public",
		Status:     model.TaskStatusSuccess,
		SubmitTime: 1710000000,
		UpdatedAt:  1710000100,
		Properties: model.Properties{
			OriginModelName: "doubao-seedance-2-0-260128",
		},
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID: "cgt-upstream",
			ResultURL:      "https://example.com/video.mp4",
		},
		Data: json.RawMessage(`{"id":"cgt-upstream","status":"running","content":{},"service_tier":"default"}`),
	}

	resp := seedanceTaskResponse(task)
	assert.Equal(t, "cgt-upstream", resp["id"], "id must be upstream task id")
	assert.Equal(t, "doubao-seedance-2-0-260128", resp["model"])
	assert.Equal(t, "succeeded", resp["status"], "internal SUCCESS must map to ARK 'succeeded'")
	assert.Equal(t, int64(1710000000), resp["created_at"])
	assert.Equal(t, int64(1710000100), resp["updated_at"])

	content, ok := resp["content"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "https://example.com/video.mp4", content["video_url"], "video_url must be filled from ResultURL")
}
```

- [ ] **Step 4: 运行,验证失败**

Run: `go test ./relay/ -run TestSeedanceTaskResponseUsesUpstreamShape -v`
Expected: FAIL with `undefined: seedanceTaskResponse`

- [ ] **Step 5: 实现核心逻辑(含性能修复)**

创建 `relay/seedance_task.go`:

```go
package relay

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// SeedanceTaskFetch 处理 ARK 原生任务查询入口:有 task_id 走单任务查询,无 task_id 走列表查询。
func SeedanceTaskFetch(c *gin.Context) (respBody []byte, taskResp *dto.TaskError) {
	taskID := strings.TrimSpace(c.Param("task_id"))
	if taskID != "" {
		return seedanceFetchTaskByID(c, taskID)
	}
	return seedanceFetchTaskList(c)
}

// seedanceFetchTaskByID 查询单个任务并按 ARK 原生格式返回。
// 先按公开 task_id 查,未命中再扫近期任务匹配 upstream task_id(ARK SDK 可能用上游 id 查询)。
func seedanceFetchTaskByID(c *gin.Context, taskID string) (respBody []byte, taskResp *dto.TaskError) {
	originTask, exist, err := seedanceGetTaskByID(c.GetInt("id"), taskID)
	if err != nil {
		taskResp = service.TaskErrorWrapper(err, "get_task_failed", http.StatusInternalServerError)
		return
	}
	if !exist {
		taskResp = service.TaskErrorWrapperLocal(errors.New("task_not_exist"), "task_not_exist", http.StatusBadRequest)
		return
	}

	respBody, err = common.Marshal(seedanceTaskResponse(originTask))
	if err != nil {
		taskResp = service.TaskErrorWrapper(err, "marshal_response_failed", http.StatusInternalServerError)
	}
	return
}

// seedanceFetchTaskList 列表查询。
// 性能修复(CodeRabbit review):user_id/platform/submit_time/status 等可下推 SQL 的条件下推到 GORM,
// 仅 task_ids(跨公开id+upstream id 匹配)/model/service_tier(涉及 JSON 字段)保留内存过滤。
func seedanceFetchTaskList(c *gin.Context) (respBody []byte, taskResp *dto.TaskError) {
	pageNum := parseSeedancePositiveInt(c.Query("page_num"), 1, 500)
	pageSize := parseSeedancePositiveInt(c.Query("page_size"), 20, 500)
	offset := (pageNum - 1) * pageSize

	weekAgo := time.Now().Add(-7 * 24 * time.Hour).Unix()
	query := model.DB.
		Where("user_id = ?", c.GetInt("id")).
		Where("platform in ?", seedanceTaskPlatforms()).
		Where("submit_time >= ?", weekAgo)

	// status 可下推(内部状态字符串直接对应)
	statusFilter := strings.TrimSpace(c.Query("filter.status"))
	if statusFilter != "" {
		if internalStatus := seedanceStatusToModelStatus(statusFilter); internalStatus != "" {
			query = query.Where("status = ?", internalStatus)
		}
	}

	// 先 Count 拿 total(基于可下推条件)
	var total int64
	if err := query.Model(&model.Task{}).Count(&total).Error; err != nil {
		taskResp = service.TaskErrorWrapper(err, "get_tasks_failed", http.StatusInternalServerError)
		return
	}

	// 涉及 JSON 字段或跨字段匹配的过滤保留内存:task_ids / model / service_tier
	taskIDFilter := seedanceTaskIDFilters(c)
	modelFilter := strings.TrimSpace(c.Query("filter.model"))
	serviceTierFilter := strings.TrimSpace(c.Query("filter.service_tier"))
	needsInMemoryFilter := len(taskIDFilter) > 0 || modelFilter != "" || serviceTierFilter != ""

	var filtered []*model.Task
	if needsInMemoryFilter {
		// 需要内存过滤时只能全量加载(但限定 7 天窗口 + 已下推的条件)
		if err := query.Order("id desc").Find(&filtered).Error; err != nil {
			taskResp = service.TaskErrorWrapper(err, "get_tasks_failed", http.StatusInternalServerError)
			return
		}
		filtered = seedanceApplyInMemoryFilters(filtered, taskIDFilter, modelFilter, serviceTierFilter)
		total = int64(len(filtered))
	} else {
		// 纯 SQL 路径:直接分页
		if err := query.Order("id desc").Offset(offset).Limit(pageSize).Find(&filtered).Error; err != nil {
			taskResp = service.TaskErrorWrapper(err, "get_tasks_failed", http.StatusInternalServerError)
			return
		}
	}

	// 内存路径需要分页切片
	if needsInMemoryFilter {
		if offset > len(filtered) {
			filtered = []*model.Task{}
		} else {
			end := offset + pageSize
			if end > len(filtered) {
				end = len(filtered)
			}
			filtered = filtered[offset:end]
		}
	}

	items := make([]map[string]any, 0, len(filtered))
	for _, task := range filtered {
		items = append(items, seedanceTaskResponse(task))
	}

	respBody, err := common.Marshal(map[string]any{
		"items": items,
		"total": total,
	})
	if err != nil {
		taskResp = service.TaskErrorWrapper(err, "marshal_response_failed", http.StatusInternalServerError)
	}
	return
}

// seedanceApplyInMemoryFilters 应用无法下推 SQL 的过滤条件
func seedanceApplyInMemoryFilters(tasks []*model.Task, taskIDs []string, modelFilter, serviceTierFilter string) []*model.Task {
	filtered := make([]*model.Task, 0, len(tasks))
	for _, task := range tasks {
		if len(taskIDs) > 0 && !seedanceTaskMatchesID(task, taskIDs) {
			continue
		}
		if modelFilter != "" && !seedanceTaskMatchesModel(task, modelFilter) {
			continue
		}
		if serviceTierFilter != "" && !seedanceTaskFieldEquals(task, "service_tier", serviceTierFilter) {
			continue
		}
		filtered = append(filtered, task)
	}
	return filtered
}

// seedanceGetTaskByID 先按公开 task_id 查,未命中再扫近期任务匹配 upstream task_id
func seedanceGetTaskByID(userID int, taskID string) (*model.Task, bool, error) {
	task, exist, err := model.GetByTaskId(userID, taskID)
	if err != nil || exist {
		return task, exist, err
	}

	// 扫描近期任务匹配 upstream id(低频路径,加 Limit 兜底防极端情况)
	var tasks []*model.Task
	err = model.DB.
		Where("user_id = ?", userID).
		Where("platform in ?", seedanceTaskPlatforms()).
		Where("submit_time >= ?", time.Now().Add(-7*24*time.Hour).Unix()).
		Order("id desc").
		Limit(500).
		Find(&tasks).Error
	if err != nil {
		return nil, false, err
	}
	for _, candidate := range tasks {
		if candidate.GetUpstreamTaskID() == taskID {
			return candidate, true, nil
		}
	}
	return nil, false, nil
}

func seedanceTaskPlatforms() []string {
	return []string{
		strconv.Itoa(constant.ChannelTypeVolcEngine),
		strconv.Itoa(constant.ChannelTypeDoubaoVideo),
	}
}

func seedanceTaskIDFilters(c *gin.Context) []string {
	rawIDs := append(c.QueryArray("filter.task_ids"), c.QueryArray("filter.task_ids[]")...)
	taskIDs := make([]string, 0, len(rawIDs))
	for _, rawID := range rawIDs {
		for _, taskID := range strings.Split(rawID, ",") {
			taskID = strings.TrimSpace(taskID)
			if taskID != "" {
				taskIDs = append(taskIDs, taskID)
			}
		}
	}
	return taskIDs
}

func seedanceTaskMatchesID(task *model.Task, taskIDs []string) bool {
	for _, taskID := range taskIDs {
		if task.TaskID == taskID || task.GetUpstreamTaskID() == taskID {
			return true
		}
	}
	return false
}

func seedanceTaskMatchesModel(task *model.Task, modelName string) bool {
	if task.Properties.OriginModelName == modelName || task.Properties.UpstreamModelName == modelName {
		return true
	}
	return seedanceTaskFieldEquals(task, "model", modelName)
}

func seedanceTaskFieldEquals(task *model.Task, field string, value string) bool {
	var data map[string]any
	if err := common.Unmarshal(task.Data, &data); err != nil {
		return false
	}
	fieldValue, _ := data[field].(string)
	return fieldValue == value
}

func parseSeedancePositiveInt(raw string, fallback, maxValue int) int {
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		value = fallback
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

// seedanceStatusToModelStatus 把 ARK 原生 status 反向映射到内部 model.TaskStatus(用于列表查询 SQL 下推)
func seedanceStatusToModelStatus(status string) string {
	switch status {
	case "succeeded":
		return string(model.TaskStatusSuccess)
	case "failed":
		return string(model.TaskStatusFailure)
	case "running":
		return string(model.TaskStatusInProgress)
	case "queued", "pending":
		return string(model.TaskStatusQueued)
	default:
		return ""
	}
}

// seedanceTaskStatus 把内部 model.TaskStatus 映射到 ARK 原生 status 字符串
func seedanceTaskStatus(status model.TaskStatus) string {
	switch status {
	case model.TaskStatusSuccess:
		return "succeeded"
	case model.TaskStatusFailure:
		return "failed"
	case model.TaskStatusInProgress:
		return "running"
	default:
		return "queued"
	}
}

// seedanceTaskResponse 把存储的 Task 还原为 ARK 原生 responseTask 格式。
// task.Data 已持有完整上游响应(由轮询器写入),这里在其基础上补全 id/model/status/created_at/updated_at/video_url。
func seedanceTaskResponse(task *model.Task) map[string]any {
	resp := map[string]any{}
	_ = common.Unmarshal(task.Data, &resp)

	resp["id"] = task.GetUpstreamTaskID()
	if modelName := task.Properties.OriginModelName; modelName != "" {
		resp["model"] = modelName
	} else if modelName = task.Properties.UpstreamModelName; modelName != "" {
		resp["model"] = modelName
	}
	resp["status"] = seedanceTaskStatus(task.Status)

	if createdAt := task.SubmitTime; createdAt > 0 {
		resp["created_at"] = createdAt
	} else if task.CreatedAt > 0 {
		resp["created_at"] = task.CreatedAt
	}
	if task.UpdatedAt > 0 {
		resp["updated_at"] = task.UpdatedAt
	}

	// 成功时确保 content.video_url 存在(优先用上游响应里的,缺失则用 ResultURL)
	if resultURL := task.GetResultURL(); resultURL != "" && task.Status == model.TaskStatusSuccess {
		content, _ := resp["content"].(map[string]any)
		if content == nil {
			content = map[string]any{}
		}
		if _, ok := content["video_url"]; !ok {
			content["video_url"] = resultURL
		}
		resp["content"] = content
	}

	// 失败时确保 error.message 存在
	if task.Status == model.TaskStatusFailure && resp["error"] == nil && task.FailReason != "" {
		resp["error"] = map[string]any{
			"message": task.FailReason,
		}
	}
	return resp
}
```

- [ ] **Step 6: 运行两个测试,验证通过**

Run: `go test ./relay/ -run "TestSeedanceTaskIDFilters|TestSeedanceTaskResponseUsesUpstreamShape" -v`
Expected: 2 个测试 PASS

- [ ] **Step 7: 整体编译验证**

Run: `go build ./...`
Expected: 无输出(全部编译通过,controller/seedance.go 的依赖闭环完成)

- [ ] **Step 8: 运行 relay 包全部 seedance 测试**

Run: `go test ./relay/ -run Seedance -v`
Expected: 全部 PASS

- [ ] **Step 9: Commit**

```bash
git add relay/seedance_task.go relay/relay_task_seedance_test.go
git commit -m "feat(seedance): implement SeedanceTaskFetch with native response shape + SQL pushdown perf fix"
```

---

## Task 7: 注册图像路由组 + 强制 PassThrough

**Files:**
- Modify: `router/relay-router.go`(新增 `/seedance` 图像路由组)
- Modify: `relay/image_handler.go`(PassThrough 追加 Seedance 条件)

- [ ] **Step 1: 在 relay-router.go 注册图像路由**

修改 `router/relay-router.go`,在文件末尾(`SetRelayRouter` 函数结束前)追加:

```go
	// Seedance (火山方舟 ARK) 原生 API 路由 - 图像生成
	// 与视频路由(video-router.go)独立,因为图像走同步 relay 而非 task 流程
	seedanceImageRouter := router.Group("/seedance")
	seedanceImageRouter.Use(middleware.RouteTag("relay"))
	seedanceImageRouter.Use(middleware.SeedanceRequestConvert(), middleware.TokenAuth(), middleware.Distribute())
	{
		seedanceImageRouter.POST("/api/v3/images/generations", func(c *gin.Context) {
			controller.Relay(c, types.RelayFormatOpenAIImage)
		})
	}
```

> **注意:** `Distribute()` 在路由组 middleware 链中,会在 `controller.Relay` 之前执行。`SeedanceRequestConvert` 已经把路径改写为 `/v1/images/generations` 并设了 `relay_mode`,Distribute 能正确识别(参照现有 jimeng 模式)。**需核实** `Distribute` 对 `c.GetBool(common.KeySeedanceOfficialAPI)` 时的路径判断顺序——因为路径已改写为 `/v1/images/generations`,`Path2RelayMode` 能识别它为 `RelayModeImagesGenerations`。

- [ ] **Step 2: 追加 PassThrough 条件**

修改 `relay/image_handler.go:49`,把:

```go
	if model_setting.GetGlobalSettings().PassThroughRequestEnabled || info.ChannelSetting.PassThroughBodyEnabled {
```

改为:

```go
	if model_setting.GetGlobalSettings().PassThroughRequestEnabled ||
		info.ChannelSetting.PassThroughBodyEnabled ||
		c.GetBool(common.KeySeedanceOfficialAPI) {
```

> **原理:** ARK 原生图像请求的 content/字段(如多图 `image` 数组、特殊 size 值)需要原样透传到上游。`dto.ImageRequest.MarshalJSON` 的 `Extra` 回写被禁用(注释明示"不能合并 ExtraFields"),开启 PassThrough 后用原始字节转发绕过此限制。计费不受影响(`image_handler.go:121-131` 的 N 计费用 `request.N`,与请求体序列化无关)。

- [ ] **Step 3: 验证编译**

Run: `go build ./...`
Expected: 无输出

- [ ] **Step 4: 端到端冒烟测试(手动,需运行实例)**

启动 new-api 后,配置一个 VolcEngine(45)渠道(填 ARK API Key),然后:

```bash
# 视频提交
curl -X POST http://localhost:3000/seedance/api/v3/contents/generations/tasks \
  -H "Authorization: Bearer <new-api-token>" \
  -H "Content-Type: application/json" \
  -d '{"model":"doubao-seedance-2-0-260128","content":[{"type":"text","text":"一只猫在弹钢琴"}],"duration":5,"resolution":"720p"}'
# 预期响应: {"id":"cgt-xxx-yyy"}

# 视频查询
curl http://localhost:3000/seedance/api/v3/contents/generations/tasks/<返回的id> \
  -H "Authorization: Bearer <new-api-token>"
# 预期响应: {"id":"cgt-xxx","model":"doubao-seedance-2-0-260128","status":"queued"/"succeeded","content":{"video_url":"..."},...}

# 图像生成
curl -X POST http://localhost:3000/seedance/api/v3/images/generations \
  -H "Authorization: Bearer <new-api-token>" \
  -H "Content-Type: application/json" \
  -d '{"model":"doubao-seedream-4-0-250828","prompt":"一只猫","size":"1024x1024"}'
# 预期响应: {"created":...,"data":[{"url":"..."}]}
```

- [ ] **Step 5: Commit**

```bash
git add router/relay-router.go relay/image_handler.go
git commit -m "feat(seedance): add ARK native image route + force passthrough for ARK image requests"
```

---

## Task 8: 数据库索引(性能配套)

**Files:**
- Modify: `model/main.go`(为 tasks 表加复合索引)

- [ ] **Step 1: 定位 AutoMigrate / 索引声明位置**

Run: `grep -n "AutoMigrate\|tasks.*Index\|CreateIndex" model/main.go | head -20`

找到 Task 模型的 AutoMigrate 调用位置和现有索引声明风格。

- [ ] **Step 2: 追加复合索引**

按项目现有风格(可能是 GORM tag 或显式 `Migrator().CreateIndex`),为 Task 表加复合索引 `(user_id, platform, submit_time)`。若用 GORM tag,在 `model/task.go` 的 Task 结构体字段上加:

```go
type Task struct {
	// ... 现有字段 ...
	UserId     int    `json:"user_id" gorm:"index:idx_task_user_platform_time,priority:1"`
	Platform   constant.TaskPlatform `json:"platform" gorm:"index:idx_task_user_platform_time,priority:2"`
	SubmitTime int64  `json:"submit_time" gorm:"index:idx_task_user_platform_time,priority:3"`
	// ... 其他字段 ...
}
```

> **注意:** 需核实 `model/task.go` 中 Task 结构体的实际字段名和现有 index tag,执行时按实际为准。若项目用 `Migrator().CreateIndex` 风格,则按那个风格写。

- [ ] **Step 3: 验证迁移在 SQLite/MySQL/PostgreSQL 都能跑**

Run: `go test ./model/ -run TaskMigrate -v`(若有相关测试)
或 Run: `go build ./model/`

Expected: 编译通过(三个数据库兼容性由 GORM tag 抽象保证)

- [ ] **Step 4: Commit**

```bash
git add model/task.go model/main.go
git commit -m "perf(seedance): add composite index on tasks(user_id, platform, submit_time)"
```

---

## Task 9: 整体回归测试

- [ ] **Step 1: 运行所有新增测试**

Run: `go test ./middleware/ ./relay/ ./relay/channel/task/doubao/ -run Seedance -v`
Expected: 全部 PASS

- [ ] **Step 2: 运行全项目测试确保无回归**

Run: `go test ./... 2>&1 | tail -50`
Expected: 无新增 FAIL(原有测试保持原状)

- [ ] **Step 3: go vet 检查**

Run: `go vet ./middleware/ ./relay/ ./relay/channel/task/doubao/ ./controller/ ./router/`
Expected: 无新增 warning

- [ ] **Step 4: 最终 Commit(若有遗漏的改动)**

```bash
git status
# 若有未提交改动:
git add -A
git commit -m "test(seedance): full regression pass"
```

---

## Self-Review

### Spec 覆盖核对

| 需求点 | 覆盖 Task |
|---|---|
| 视频提交 `POST /api/v3/contents/generations/tasks` | Task 4(路由)+ Task 5(adaptor)+ Task 7(冒烟) |
| 视频查询 `GET /api/v3/contents/generations/tasks/:id` | Task 4 + Task 6 |
| 视频列表查询 | Task 6 |
| 提交响应 `{id}` 格式 | Task 5 Step 9-12 |
| 查询响应 ARK 原生 responseTask 格式 | Task 6 Step 5 |
| 多模态 content[] 原样透传 | Task 5 Step 1-4 |
| 图像 `POST /api/v3/images/generations` | Task 7 |
| 图像字段不丢失(PassThrough) | Task 7 Step 2 |
| CodeRabbit review:列表查询性能 | Task 6 Step 5(SQL 下推)+ Task 8(索引) |
| CodeRabbit review:模型名回填 | Task 5 Step 3 |
| expired/cancelled 状态 | Task 5 Step 13-16 |

### 待确认项(执行时核实)

1. **`common.GetBodyStorage` 测试 setup**:Task 5 Step 1 的测试需要正确构造 body storage,执行时 grep `common.NewBytesBodyStorage` 或类似构造函数名,确保测试能读出请求体。
2. **Task 结构体字段名**:Task 8 的索引声明需以 `model/task.go` 实际字段名为准(SubmitTime/CreatedAt 等)。
3. **Distribute 中间件顺序**:Task 7 Step 1 的 `SeedanceRequestConvert → TokenAuth → Distribute` 顺序需验证 Distribute 能看到改写后的路径(参照 jimeng 模式,应该可以)。

### 类型一致性

- `KeySeedanceOfficialAPI` 在 Task 1 定义,Task 2/5/6/7 全部用 `common.KeySeedanceOfficialAPI` ✓
- `seedanceTaskResponse` 在 Task 6 Step 5 定义,Task 6 Step 3 测试调用,签名一致 ✓
- `seedanceTaskIDFilters` 在 Task 6 Step 5 定义,Task 6 Step 1 测试调用,返回 `[]string` ✓
- `RelaySeedanceTask`/`RelaySeedanceTaskFetch` 在 Task 3 定义,Task 4/7 调用 ✓
- `seedanceTextPrompt` 在 Task 5 Step 7 定义 ✓

无类型不一致问题。

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-07-18-ark-native-compat.md`. 见对话中的执行选项(subagent-driven vs inline)。
