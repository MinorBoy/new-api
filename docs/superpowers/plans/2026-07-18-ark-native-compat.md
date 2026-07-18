# ARK 原生 API 兼容入口 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让火山方舟（ARK）SDK 用户仅把 `base_url` 从官方地址替换为 `https://<new-api-host>/api/v3`，即可使用视频任务提交、单任务查询、任务列表查询和图像生成接口，同时保留 new-api 的鉴权、渠道模型映射、计费校验和公开任务 ID 隔离。

**Architecture:** 原生路由只负责标记请求和把提交路径改写到现有统一 relay pipeline。视频 adaptor 读取并验证 ARK 原生请求，转发时只改写模型映射字段；客户端始终看到 new-api 生成的公开 `task_*` ID，上游任务 ID 只保存在 `Task.PrivateData.UpstreamTaskID`。查询响应基于已落库的上游响应恢复 ARK 结构，列表查询通过 SQL 分页或有界批量扫描完成，禁止一次性加载七天内全部任务。

**Tech Stack:** Go 1.22+、Gin、GORM v2、testify（`require` + `assert`）；所有 JSON 编解码调用统一使用 `common.*`。

**Reference:** [PR #5737](https://github.com/QuantumNous/new-api/pull/5737) 仅作为行为参考，不直接照搬其上游 ID 暴露和无界内存过滤实现；正式契约以 `docs/channel/api-doc-doubao-video-generation.md` 和 `docs/channel/api-doc-doubao-image-generation.md` 为准。

---

## Review Decisions

本审定版修正原计划中的以下问题，后续实现不得恢复旧方案：

1. `common/gin.go` 已有 `KeyRequestBody` 和 `KeyBodyStorage`，只新增 `KeySeedanceOfficialAPI`，不得重复声明。
2. 提交响应返回 `info.PublicTaskID`，查询响应返回 `task.TaskID`；`GetUpstreamTaskID()` 仅用于 new-api 到上游的通信。
3. 原生视频请求仍必须执行时长边界校验；`duration=-1` 可作为 ARK 的智能时长值，其余正值不得超过 `relaycommon.MaxTaskDurationSeconds`。
4. 原生图像透传必须重写映射后的 `model`，否则渠道 `model_mapping` 会被原始请求体绕过。
5. 不因 JSON 过滤条件调用无 `LIMIT` 的 `Find`。需要检查 `Task.Data`/`Task.Properties` 时使用固定批大小的 keyset 扫描，只保留当前页结果。
6. `filter.task_ids` 只匹配公开任务 ID。客户端从未获得上游 ID，因此无需扫描 `private_data`。
7. 原生任务错误使用 ARK/OpenAI 风格的 `{"error":{"code":...,"message":...}}`，任务不存在返回 404。
8. 原生视频校验必须按官方模型能力矩阵执行：Mini 的精确 ID 是 `doubao-seedance-2-0-mini-260615`；2.0 Mini/Fast 禁止 1080p，2.0 系列禁止 `service_tier=flex`，纯音频和媒体数量越界在访问上游前返回 400。
9. 原生图像校验必须按 Seedream 模型能力执行：Pro 禁止组图和流式，Lite/4.5/4.0 才允许 `sequential_image_generation=auto`、`max_images` 和流式；参考图与输出图总数不得超过 15。
10. 原生图像的实际计费张数只信响应 `usage.generated_images`，该字段缺失时才回退兼容路径；`data.#` 不能作为权威张数，因为组图响应可能包含失败项。本计划只实现接入与协议兼容，计费倍率和视频任务结算在 `2026-07-18-seedance-billing.md` 中实现。
11. 新增图像资料只确认 Seedream 家族能力，没有给出可安全写入渠道 `ModelList` 的版本化 ID；不得自行臆造 Seedream ID，原生请求按已配置的模型映射透传。

## Scope

- 视频：`POST /api/v3/contents/generations/tasks`
- 视频：`GET /api/v3/contents/generations/tasks/:task_id`
- 视频：`GET /api/v3/contents/generations/tasks`
- 图像：`POST /api/v3/images/generations`
- 复用渠道类型 `VolcEngine(45)` / `DoubaoVideo(54)`
- 不增加 chat、embeddings、responses、rerank 原生入口

## File Structure

| 文件 | 责任 |
|---|---|
| `common/gin.go` | 新增 ARK 原生入口标记 |
| `middleware/seedance_adapter.go` | 标记原生入口并改写提交路径 |
| `middleware/seedance_adapter_test.go` | 路径和 relay mode 契约测试 |
| `router/video-router.go` | 视频提交、单查、列表路由 |
| `router/relay-router.go` | 图像生成路由 |
| `controller/seedance.go` | 原生任务查询控制器 |
| `controller/relay.go` | 原生任务错误格式分支 |
| `controller/seedance_test.go` | 错误响应契约测试 |
| `relay/channel/task/doubao/adaptor.go` | 原生视频校验、请求构建、提交响应和状态映射 |
| `relay/channel/task/doubao/adaptor_test.go` | adaptor 行为测试 |
| `relay/seedance_task.go` | 单任务和列表查询、ARK 响应恢复 |
| `relay/relay_task_seedance_test.go` | 查询响应、分页、归属隔离测试 |
| `relay/image_handler.go` | 原生图像字段保留并应用模型映射 |
| `relay/image_handler_test.go` | 图像原生请求重写测试 |
| `relay/channel/openai/relay_image.go` | 原生图像成功张数解析和流式计费张数收口 |
| `relay/channel/openai/image_stream_test.go` | 图像流式/失败项张数回归测试 |
| `dto/openai_response.go` | 公开 usage 中的 `generated_images` / `input_images` 字段 |

---

## Task 1: 请求标记、中间件和路由

**Files:**
- Modify: `common/gin.go`
- Create: `middleware/seedance_adapter.go`
- Create: `middleware/seedance_adapter_test.go`
- Modify: `router/video-router.go`
- Modify: `router/relay-router.go`

- [ ] **Step 1: 先写中间件表驱动测试**

测试三个稳定契约：视频 POST 改为 `/v1/video/generations` 和 `RelayModeVideoSubmit`；图像 POST 改为 `/v1/images/generations` 和 `RelayModeImagesGenerations`；视频 GET 保持原路径和 `task_id` 参数。每个场景都断言 `common.KeySeedanceOfficialAPI == true`。

Run: `go test ./middleware -run TestSeedanceRequestConvert -v`

Expected: FAIL with `undefined: SeedanceRequestConvert`。

- [ ] **Step 2: 添加唯一的新 context key**

在 `common/gin.go` 已有两个 body key 后追加：

```go
const KeySeedanceOfficialAPI = "seedance_official_api"
```

- [ ] **Step 3: 实现精确路径改写**

创建 `middleware/seedance_adapter.go`：

```go
package middleware

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/gin-gonic/gin"
)

func SeedanceRequestConvert() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(common.KeySeedanceOfficialAPI, true)

		if c.Request.Method == http.MethodPost {
			switch c.Request.URL.Path {
			case "/api/v3/contents/generations/tasks":
				c.Request.URL.Path = "/v1/video/generations"
				c.Set("relay_mode", relayconstant.RelayModeVideoSubmit)
			case "/api/v3/images/generations":
				c.Request.URL.Path = "/v1/images/generations"
				c.Set("relay_mode", relayconstant.RelayModeImagesGenerations)
			}
		}

		c.Next()
	}
}
```

使用精确路径而不是 `strings.Contains`，避免以后扩大路由组时误改写其他端点。

- [ ] **Step 4: 注册视频路由**

在 `router/video-router.go` 的 Jimeng 路由前添加：

```go
	seedanceVideoRouter := router.Group("/api/v3/contents/generations")
	seedanceVideoRouter.Use(middleware.RouteTag("relay"))
	seedanceVideoRouter.Use(middleware.SeedanceRequestConvert(), middleware.TokenAuth())
	{
		seedanceVideoRouter.POST("/tasks", middleware.Distribute(), controller.RelayTask)
		seedanceVideoRouter.GET("/tasks", controller.RelaySeedanceTaskFetch)
		seedanceVideoRouter.GET("/tasks/:task_id", controller.RelaySeedanceTaskFetch)
	}
```

POST 直接复用 `controller.RelayTask`；无需只做一次 `c.Set` 的 `RelaySeedanceTask` 包装函数。

- [ ] **Step 5: 注册图像路由且保留标准限流链**

在 `router/relay-router.go` 的 `SetRelayRouter` 内添加：

```go
	seedanceImageRouter := router.Group("/api/v3")
	seedanceImageRouter.Use(middleware.RouteTag("relay"))
	seedanceImageRouter.Use(middleware.SystemPerformanceCheck())
	seedanceImageRouter.Use(middleware.SeedanceRequestConvert(), middleware.TokenAuth())
	seedanceImageRouter.Use(middleware.ModelRequestRateLimit(), middleware.Distribute())
	seedanceImageRouter.POST("/images/generations", func(c *gin.Context) {
		controller.Relay(c, types.RelayFormatOpenAIImage)
	})
```

- [ ] **Step 6: 验证**

Run: `go test ./middleware -run TestSeedanceRequestConvert -v`

Run: `go build ./router ./common`

Expected: 全部通过。

- [ ] **Step 7: Commit**

```bash
git add common/gin.go middleware/seedance_adapter.go middleware/seedance_adapter_test.go router/video-router.go router/relay-router.go
git commit -m "feat(seedance): register ARK native routes"
```

---

## Task 2: 原生视频请求和提交响应

**Files:**
- Modify: `relay/channel/task/doubao/adaptor.go`
- Modify: `relay/common/relay_utils.go`
- Modify: `relay/channel/task/doubao/constants.go`
- Create: `relay/channel/task/doubao/adaptor_test.go`

- [ ] **Step 1: 写失败测试**

覆盖以下 API 契约：

- `model` 缺失、`content` 为空或类型错误时返回 400。
- `duration=0`、`duration < -1`、超出 `relaycommon.MaxTaskDurationSeconds` 或超出所选模型官方范围时返回 400；`duration=-1` 只对 1.5 Pro/2.0 系列通过，其他模型拒绝。
- `frames` 只对 1.0 Pro/1.0 Pro Fast 通过，且满足 `[29,289]` 和 `frames = 25 + 4n`；2.0/1.5 的 `frames` 返回 400。
- 2.0 多模态参考场景最多 9 张图片、3 个视频、3 个音频，音频没有图片或视频时返回 400；首帧、首尾帧和多模态参考三种场景互斥。
- `role` 只接受文档定义的 `first_frame`、`last_frame`、`reference_image`、`reference_video`、`reference_audio`，首帧/首尾帧的数量和角色组合错误时返回 400。
- 2.0/Fast/Mini 的 `resolution`、`service_tier`、`generate_audio`、`draft`、`priority`、`seed`、`camera_fixed` 按模型能力矩阵拒绝不支持组合；`priority` 范围为 0~9，`execution_expires_after` 范围为 3600~259200。
- 原生 `content` 中 image/video/audio/text 项、显式 `watermark:false` 和未知字段均保留。
- `info.UpstreamModelName` 非空时，请求体 `model` 被替换为映射后的值；为空时从原始 body 回填。
- 提交给客户端的 ID 是 `info.PublicTaskID`，adaptor 返回给持久化层的 ID 仍是上游 ID。
- `expired` / `cancelled` 映射为 `TaskStatusFailure`。

测试 body storage 直接由 `httptest.NewRequest` 提供；`common.GetBodyStorage` 会自行创建缓存，不使用不存在的 `common.NewBytesBodyStorage`。

Run: `go test ./relay/channel/task/doubao -run Seedance -v`

Expected: FAIL。

- [ ] **Step 2: 添加原生请求验证结构**

在 `adaptor.go` 中使用指针标量读取校验字段，避免把显式 `false` / `0` 当成缺失：

```go
type seedanceNativeRequest struct {
	Model                 string         `json:"model"`
	Content               []ContentItem  `json:"content"`
	ServiceTier           string         `json:"service_tier,omitempty"`
	ExecutionExpiresAfter *dto.IntValue  `json:"execution_expires_after,omitempty"`
	GenerateAudio         *dto.BoolValue `json:"generate_audio,omitempty"`
	Draft                 *dto.BoolValue `json:"draft,omitempty"`
	Priority               *dto.IntValue  `json:"priority,omitempty"`
	Resolution             string         `json:"resolution,omitempty"`
	Ratio                  string         `json:"ratio,omitempty"`
	Duration               *dto.IntValue  `json:"duration,omitempty"`
	Frames                 *dto.IntValue  `json:"frames,omitempty"`
	Seed                   *dto.IntValue  `json:"seed,omitempty"`
	CameraFixed            *dto.BoolValue `json:"camera_fixed,omitempty"`
	ReturnLastFrame        *dto.BoolValue `json:"return_last_frame,omitempty"`
	Watermark              *dto.BoolValue `json:"watermark,omitempty"`
	Tools                  []struct {
		Type string `json:"type,omitempty"`
	} `json:"tools,omitempty"`
	SafetyIdentifier       string         `json:"safety_identifier,omitempty"`
}
```

原生分支调用 `common.UnmarshalBodyReusable`，校验后把完整原始对象存入 `TaskSubmitReq.Metadata`，并把第一个非空 text 内容写入 `TaskSubmitReq.Prompt`。在 `relay/common/relay_utils.go` 的通用 task 校验中，只有 `common.KeySeedanceOfficialAPI` 请求允许 `duration=-1` 通过基础检查；显式 `duration=0` 仍由原生分支拒绝。映射完成后，doubao adaptor 再按以下规则做模型专属校验：

- 2.0（含 `doubao-seedance-2-0-260128` 和 `doubao-seedance-2-0-mini-260615`）支持 480p/720p/1080p/4k，但 Mini 禁止 1080p；Fast/Mini 只支持 480p/720p；4k 只对普通 2.0 通过。
- 1.5 Pro 支持 480p/720p/1080p；1.0 Pro/1.0 Pro Fast 使用官方默认 1080p，且 Fast 不接受首尾帧角色组合。
- `duration`：1.0 为 2~12，1.5 为 4~12 或 -1，2.0 为 4~15 或 -1；所有正值同时受 `relaycommon.MaxTaskDurationSeconds` 约束。
- `frames`：仅 1.0 系列接受 `[29,289]` 中满足 `25+4n` 的值；2.0/1.5 直接返回 400。
- `service_tier`：仅 1.5 Pro 接受 `default`/`flex`；2.0/Fast/Mini 只接受缺省或 `default`。`generate_audio` 仅 2.0/1.5 支持，缺省按 `true`；`draft` 仅 1.5 支持，且 `draft=true` 必须是 480p、不能 `return_last_frame=true` 或 `service_tier=flex`。
- `ratio` 只接受 `16:9`、`4:3`、`1:1`、`3:4`、`9:16`、`21:9`、`adaptive`；`adaptive` 仅 2.0/1.5 Pro 的文生或图生场景按文档规则通过。
- `priority` 仅 2.0 支持且范围 0~9；`execution_expires_after` 范围 3600~259200；2.0 不支持 `seed` 和 `camera_fixed`，其他模型的 `seed` 范围为 -1~`2^32-1`，参考图场景拒绝 `camera_fixed=true`。
- `tools` 仅 2.0 系列支持且每项 `type` 必须为 `web_search`；`safety_identifier` 提供时必须是长度不超过 64 的英文字符串，未提供时通过。
- `content` 按 type/role 统计：2.0 多模态参考场景图片 0~9、视频 0~3、音频 0~3，音频必须伴随至少一张图片或一个视频；`first_frame`、`last_frame` 与 `reference_*` 不得混用，且首帧/首尾帧必须分别为 1/2 张图片。
- 图片/视频/音频 URL、Base64 或 `asset://` 的格式、媒体尺寸、编码和总时长由 ARK 上游校验；new-api 不抓取远程媒体或凭字符串猜测时长，只检查对象结构、URL 非空和可验证的数量/角色约束。

未知字段不丢弃，但不把未知字段中的 `duration`、`frames`、`priority` 等旁路值带入计费上下文；计费 adaptor 只读取已解析的顶层字段。

- [ ] **Step 3: 构建上游请求时保留所有字段并应用模型映射**

原生分支读取 `BodyStorage.Bytes()`，使用 `map[string]json.RawMessage`（`encoding/json` 只作为类型来源）保存未知字段。解析失败必须返回错误，不能把已知无效 JSON 继续发给上游。

```go
	var fields map[string]json.RawMessage
	if err := common.Unmarshal(rawBody, &fields); err != nil {
		return nil, fmt.Errorf("invalid ARK request body: %w", err)
	}

	if info.UpstreamModelName == "" {
		var modelName string
		if err := common.Unmarshal(fields["model"], &modelName); err != nil {
			return nil, fmt.Errorf("invalid ARK model: %w", err)
		}
		info.UpstreamModelName = modelName
	}
	mappedModel, err := common.Marshal(info.UpstreamModelName)
	if err != nil {
		return nil, err
	}
	fields["model"] = mappedModel
	data, err := common.Marshal(fields)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
```

- [ ] **Step 4: 返回公开 ID**

在 `DoResponse` 验证上游 `dResp.ID` 后添加：

```go
	if c.GetBool(common.KeySeedanceOfficialAPI) {
		c.JSON(http.StatusOK, responsePayload{ID: info.PublicTaskID})
		return dResp.ID, responseBody, nil
	}
```

禁止返回 `dResp.ID` 给客户端。

- [ ] **Step 5: 状态映射**

在 `ParseTaskResult` 中把 `expired` 和 `cancelled` 归入失败终态，`Reason` 优先使用上游错误消息，空时使用状态字符串。

- [ ] **Step 6: 验证并提交**

Run: `go test ./relay/channel/task/doubao -run Seedance -v`

Run: `go test ./relay/channel/task/doubao -v`

```bash
git add relay/channel/task/doubao/adaptor.go relay/channel/task/doubao/constants.go relay/channel/task/doubao/adaptor_test.go relay/common/relay_utils.go
git commit -m "feat(seedance): preserve ARK video request contract"
```

---

## Task 3: 单任务和列表查询

**Files:**
- Create: `relay/seedance_task.go`
- Create: `relay/relay_task_seedance_test.go`
- Create: `controller/seedance.go`

- [ ] **Step 1: 写失败测试**

测试至少覆盖：

- 单任务只允许当前 token 用户按公开 `task_*` ID 查询。
- 查询响应 `id` 始终是 `Task.TaskID`，即使 `Task.Data.id` 和 `PrivateData.UpstreamTaskID` 是上游 ID。
- 内部 `SUCCESS` / `FAILURE` / `IN_PROGRESS` / `QUEUED` 映射为 `succeeded` / `failed` / `running` / `queued`。
- 成功任务缺少 `content.video_url` 时由 `PrivateData.ResultURL` 补齐。
- `filter.task_ids` 支持逗号分隔、重复参数和 `filter.task_ids[]`，并通过 `WHERE task_id IN ?` 下推。
- 列表仅返回当前用户、平台为 `45` 或 `54`、提交时间在七天窗口内的任务。
- 需要读取 JSON 的 `filter.model` / `filter.service_tier` 路径使用固定批大小扫描；测试插入超过一个批次的数据，验证分页和 `total` 正确。

测试在 `relay` 包内用 `glebarez/sqlite` 创建独立内存库，保存并在 `t.Cleanup` 恢复原 `model.DB`；不得 `t.Parallel()`。

Run: `go test ./relay -run SeedanceTask -v`

Expected: FAIL。

- [ ] **Step 2: 实现公开 ID 单查**

`seedanceFetchTaskByID` 只调用：

```go
task, exists, err := model.GetByTaskId(c.GetInt("id"), strings.TrimSpace(c.Param("task_id")))
```

不扫描 `private_data`，不存在时返回 `http.StatusNotFound`。

- [ ] **Step 3: 实现列表查询**

基础 GORM 条件必须包含：

```go
query := model.DB.Model(&model.Task{}).
	Where("user_id = ?", c.GetInt("id")).
	Where("platform IN ?", []string{
		strconv.Itoa(constant.ChannelTypeVolcEngine),
		strconv.Itoa(constant.ChannelTypeDoubaoVideo),
	}).
	Where("submit_time >= ?", time.Now().Add(-7*24*time.Hour).Unix())
```

`status` 和公开 `task_ids` 继续用 `Where` 下推。没有 JSON 过滤时执行 `Count` + `Order("id DESC").Offset(...).Limit(...)`。

存在 `model` 或 `service_tier` 过滤时，按 `id DESC` 每批 200 条做 keyset 扫描（下一批追加 `id < lastID`），每批查询都带 `Limit(200)`；只累计匹配数和保存当前页所需任务。禁止无 `Limit` 的 `Find`，禁止把全部候选任务保存在切片中。

分页参数规则固定为：`page_num` 默认 1、最大 500；`page_size` 默认 20、最大 100。溢出或非法值回退默认值。

- [ ] **Step 4: 恢复 ARK 响应**

先初始化空 map；若 `Task.Data` 非空则用 `common.Unmarshal(task.Data, &response)` 恢复上游字段，再强制覆盖以下字段：

```go
response["id"] = task.TaskID
response["status"] = seedanceTaskStatus(task.Status)
response["model"] = task.Properties.OriginModelName
if response["model"] == "" {
	response["model"] = task.Properties.UpstreamModelName
}
response["created_at"] = task.SubmitTime
if task.SubmitTime == 0 {
	response["created_at"] = task.CreatedAt
}
response["updated_at"] = task.UpdatedAt
```

上游 ID 不得出现在响应中。列表响应固定为：

```json
{"items": [], "total": 0}
```

- [ ] **Step 5: 添加查询控制器**

`controller/seedance.go` 只包含查询 handler：

```go
package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/relay"
	"github.com/gin-gonic/gin"
)

func RelaySeedanceTaskFetch(c *gin.Context) {
	responseBody, taskErr := relay.SeedanceTaskFetch(c)
	if taskErr != nil {
		respondTaskError(c, taskErr)
		return
	}
	c.Data(http.StatusOK, "application/json", responseBody)
}
```

- [ ] **Step 6: 验证并提交**

Run: `go test ./relay -run SeedanceTask -v`

Run: `go build ./controller ./router`

```bash
git add relay/seedance_task.go relay/relay_task_seedance_test.go controller/seedance.go
git commit -m "feat(seedance): add ARK task lookup and bounded list query"
```

---

## Task 4: 原生任务错误格式

**Files:**
- Modify: `controller/relay.go`
- Create: `controller/seedance_test.go`

- [ ] **Step 1: 写失败测试**

调用 `respondTaskError`，分别断言普通任务仍返回现有 `dto.TaskError`，ARK 标记请求返回：

```json
{"error":{"code":"task_not_exist","message":"task_not_exist"}}
```

并保留原 HTTP 状态码。

- [ ] **Step 2: 在统一错误出口添加窄分支**

在 `respondTaskError` 的 429 文案处理后、现有 `c.JSON` 前添加：

```go
	if c.GetBool(common.KeySeedanceOfficialAPI) {
		c.JSON(taskErr.StatusCode, gin.H{
			"error": gin.H{
				"code":    taskErr.Code,
				"message": taskErr.Message,
			},
		})
		return
	}
```

- [ ] **Step 3: 验证并提交**

Run: `go test ./controller -run Seedance -v`

```bash
git add controller/relay.go controller/seedance_test.go
git commit -m "fix(seedance): return ARK-compatible task errors"
```

---

## Task 5: 原生图像透传且保留模型映射

**Files:**
- Modify: `controller/relay.go`
- Modify: `relay/image_handler.go`
- Modify: `relay/helper/valid_request.go`
- Create: `relay/image_handler_test.go`
- Modify: `relay/helper/openai_image_request_test.go`
- Modify: `relay/channel/openai/relay_image.go`
- Modify: `relay/channel/openai/image_stream_test.go`
- Modify: `dto/openai_response.go`

- [ ] **Step 1: 写失败测试**

为原生图像 body 构造 `model`、多图 `image`、显式 `watermark:false`、`seed:0`、`sequential_image_generation`、`sequential_image_generation_options.max_images` 和未知对象字段。断言：

- `model` 等于映射后的 `info.UpstreamModelName`。
- 其余字段 JSON 语义完全相同，显式零值和 `false` 未丢失。
- 参考图数量超过 Pro 的 10 或 Lite/4.5/4.0 的 14 时返回 400；`max_images` 必须为 1~15；组图的实际输出上限为 `min(max_images, 15-input_images)`，并将预扣估算的 `request.N` 归一化为该有界输出上限。输入 14 张参考图且 `max_images=15` 应通过并按最多 1 张预扣。
- Pro 传 `sequential_image_generation=auto` 或 `stream=true` 返回 400；Lite/4.5/4.0 的 `auto`、`max_images` 和 `stream=true` 通过。
- `sequential_image_generation_options` 仅在 `sequential_image_generation=auto` 时接受；disabled 模式带 options 返回 400，且 Pro 传该字段始终返回 400。
- 原生响应含失败项时，`usage.generated_images=2` 只把 `n` 计费倍率设为 2，不使用 `data.#=3`；`generated_images=0` 不产生图片额度；字段缺失时保留兼容回退路径。
- Seedream 非流式响应的 `usage.output_tokens` 映射为 canonical `CompletionTokens`，`total_tokens` 与其一致且不虚构输入 token；Pro 返回的 `input_images` 只保留在响应/usage，不作为图片张数计费。
- 非原生请求仍走既有 `ConvertImageRequest` 分支。

Run: `go test ./relay -run SeedanceImage -v`

Expected: FAIL。

- [ ] **Step 2: 添加原生 body 重写**

在 `relay/helper/valid_request.go` 增加稳定函数 `func NormalizeSeedreamNativeImageRequest(c *gin.Context, request *dto.ImageRequest) error`，由 `controller/relay.go` 在 `GenRelayInfo` 后、token 估算和预扣前调用。它读取原始 JSON（只使用 `common.*`），统计 `image` 字符串/数组的参考图数量，验证 `max_images` 1~15，并把 `request.N` 设置为 `disabled=>1` 或 `auto=>min(max_images, 15-input_images)`；当 `input_images >= 15` 时返回 400。该值只作为预扣上限，不改变 outbound body，也不把 OpenAI `n` 当作 ARK 组图数量。

控制器只在 `common.KeySeedanceOfficialAPI` 和 `relayFormat == types.RelayFormatOpenAIImage` 时调用该函数；返回错误直接走现有 `invalid_request` 400 分支，确保不发生预扣。普通 OpenAI 图像请求不执行 Seedream 归一化。

在 `relay/image_handler.go` 的 `ModelMappedHelper` 成功后调用稳定函数 `func ValidateSeedreamNativeModelRequest(c *gin.Context, request *dto.ImageRequest, upstreamModel string) error`，用 `info.UpstreamModelName` 检查模型能力：Pro 只允许单张输出（仍允许最多 10 张参考图），并禁止 `sequential_image_generation`/`stream`；Lite/4.5/4.0 才允许 `auto`、`max_images` 和流式；`output_format` 仅 Pro/Lite，`tools` 仅 Lite，`optimize_prompt_options.mode=fast` 只对 4.0 通过。验证失败使用 400，且发生在上游请求前。

不要仅把 `c.GetBool(common.KeySeedanceOfficialAPI)` 加到现有 pass-through 条件。新增稳定领域函数 `buildSeedanceImageRequestBody(raw []byte, upstreamModel string) ([]byte, error)`，使用 `map[string]json.RawMessage` + `common.Unmarshal` / `common.Marshal`，只替换 `model`；当 `upstreamModel` 为空时保留原始 `model`。构建 outbound body 时沿用 `relaycommon.NewOutboundJSONBody`，更新 `info.UpstreamRequestBodySize`，并正确关闭 closer。

在 `ImageHelper` 完成 `ModelMappedHelper` 后优先处理原生分支，并在构建 body 后继续应用 `relaycommon.ApplyParamOverrideWithRelayInfo`。全局/渠道 pass-through 分支保持原行为。

在 `dto/openai_response.go` 的 `dto.Usage` 增加 `GeneratedImages` 和 `InputImages` 字段。`relay/channel/openai/relay_image.go` 的非流式和流式处理都优先读取原生入口响应 `usage.generated_images`；原生字段存在时允许 0，将负数按 0 处理、超过 15 时记录 `common.SysError` 后截断到 15，再更新 `PriceData` 的 `n` 倍率。普通 OpenAI 图像响应继续使用 `dto.MaxImageN` 的现有上限。成功张数为 0 时通过复制并过滤 `PriceData.OtherRatios()` 后调用 `ReplaceOtherRatios` 移除 `n`（不得写入 0，因为 `AddOtherRatio` 会拒绝非正值），同时将传给计费的 `usage.TotalTokens` 归零，确保按成功张数不扣图片额度；不要在 `relay/image_handler.go` 把 TotalTokens 强制改成 1。字段缺失才使用现有 `data.#`/请求数量兼容路径。所有倍率更新继续经 `PriceData.AddOtherRatio`/`ReplaceOtherRatios`，不得直接写 `OtherRatios`。

`ImageHelper` 的日志数量也优先使用同一个权威 `generated_images` 值（包括 0），只有字段缺失才记录规范化后的请求估算；预扣估算和终态实际张数必须在测试中分别断言。

- [ ] **Step 3: 验证并提交**

Run: `go test ./relay -run SeedanceImage -v`

Run: `go test ./relay/helper -run 'SeedreamNative|GetAndValidOpenAIImageRequestNBounds' -v`

Run: `go test ./relay/channel/openai -run 'Image.*(Stream|GeneratedImages|Count)' -v`

```bash
git add controller/relay.go relay/image_handler.go relay/helper/valid_request.go relay/image_handler_test.go relay/helper/openai_image_request_test.go relay/channel/openai/relay_image.go relay/channel/openai/image_stream_test.go dto/openai_response.go
git commit -m "feat(seedance): preserve native image fields with model mapping"
```

---

## Task 6: 回归验证

- [ ] **Step 1: 新增行为测试**

Run: `go test ./middleware ./controller ./relay ./relay/channel/task/doubao -run Seedance -v`

- [ ] **Step 2: 相关包全量测试**

Run: `go test ./middleware ./controller ./relay ./relay/channel/task/doubao ./router`

- [ ] **Step 3: 静态检查**

Run: `go vet ./middleware ./controller ./relay ./relay/channel/task/doubao ./router`

- [ ] **Step 4: 全项目回归**

Run: `go test ./...`

- [ ] **Step 5: 手工端到端验证**

使用 VolcEngine(45) 测试渠道验证：

1. 原生视频提交返回 `task_*`，不返回 `cgt-*`。
2. 用该 `task_*` 查询成功，响应仍是 ARK 字段结构。
3. 其他用户 token 查询该 ID 返回 404。
4. 图像请求配置模型映射后，上游实际收到映射目标模型。
5. `duration` 超过上限时在访问上游前返回 400。
6. 2.0 Mini 使用 `doubao-seedance-2-0-mini-260615`，1080p、`service_tier=flex`、纯音频和媒体数量越界均在上游调用前返回 400。
7. Seedream 组图输入 14 张参考图时最多生成 1 张；响应含失败项时按 `usage.generated_images` 而非 `data.#` 结算；Pro 的 `stream=true` 和 `sequential_image_generation=auto` 返回 400。

不使用 `git add -A`，不提交工作区中与本计划无关的文件。

## Execution Order

先完成本计划 Task 1-5，再执行 `2026-07-18-seedance-billing.md`。两份计划都会修改 doubao adaptor；计费计划以本计划已存在的原生验证分支和公开任务 ID 语义为前提。

## Project Module References

以下项目模块路径属于 new-api 的受保护项目身份，实施时保持不变：

```go
"github.com/QuantumNous/new-api/common"
relayconstant "github.com/QuantumNous/new-api/relay/constant"
"github.com/QuantumNous/new-api/relay"
relaycommon "github.com/QuantumNous/new-api/relay/common"
"github.com/QuantumNous/new-api/model"
"github.com/QuantumNous/new-api/constant"
"github.com/QuantumNous/new-api/dto"
"github.com/QuantumNous/new-api/service"
```
