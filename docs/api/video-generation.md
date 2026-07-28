# 视频生成 API

本文档列出 new-api 内置的视频生成相关 HTTP 接口（端点、请求体、响应体）。

所有路由定义于 `router/video-router.go`（`SetVideoRouter`，由 `router/main.go` 注册）。统一请求/响应 DTO 定义于 `dto/video.go` 与 `dto/openai_video.go`。

## 鉴权与中间件

- 鉴权统一使用 `Authorization: Bearer <API Key>`（`middleware.TokenAuth`）。视频内容代理 `/v1/videos/:task_id/content` 接受 session 鉴权（控制台）或 token 鉴权（`TokenOrUserAuth`）。
- 提交类请求会经过 `VideoRequestPolicy`（请求体大小与 base64 媒体校验，`middleware/video_request_policy.go`）。
- 各厂商原生端点通过 `KlingRequestConvert` / `SeedanceRequestConvert` / `JimengRequestConvert` 中间件把原生请求体改写成统一形态 `{model, prompt, metadata}` 后注入 `/v1/video/generations`。
- 提交请求最后经过 `Distribute`（按 `model` 选择渠道）。

## 接口端点

| 方法 | 端点 | Handler | 说明 |
|---|---|---|---|
| POST | `/v1/video/generations` | `controller.RelayTask` | 统一视频生成提交（内部标准端点） |
| GET | `/v1/video/generations/:task_id` | `controller.RelayTaskFetch` | 查询任务状态 |
| POST | `/v1/videos` | `controller.RelayTask` | OpenAI/Sora 兼容视频创建（https://platform.openai.com/docs/api-reference/videos/create ） |
| GET | `/v1/videos/:task_id` | `controller.RelayTaskFetch` | OpenAI 兼容任务查询 |
| POST | `/v1/videos/:video_id/remix` | `controller.RelayTask` | 基于 `video_id` 的 remix 续作 |
| GET | `/v1/videos/:task_id/content` | `controller.VideoProxy` | 视频内容代理下载（session 或 token 鉴权） |
| POST | `/kling/v1/videos/text2video` | `controller.RelayTask` | Kling 文生视频 |
| POST | `/kling/v1/videos/image2video` | `controller.RelayTask` | Kling 图生视频 |
| GET | `/kling/v1/videos/text2video/:task_id` | `controller.RelayTaskFetch` | Kling 任务查询 |
| GET | `/kling/v1/videos/image2video/:task_id` | `controller.RelayTaskFetch` | Kling 任务查询 |
| POST | `/api/v3/contents/generations/tasks` | `controller.RelayTask` | Seedance / 火山方舟 ARK 兼容提交 |
| GET | `/api/v3/contents/generations/tasks` | `controller.RelaySeedanceTaskFetch` | Seedance 任务列表 |
| GET | `/api/v3/contents/generations/tasks/:task_id` | `controller.RelaySeedanceTaskFetch` | Seedance 单任务查询 |
| POST | `/jimeng/` | `controller.RelayTask` | 即梦官方 API（按 `?Action=` 区分提交/查询） |

Handler 位置：

| Handler | 文件:行号 |
|---|---|
| `RelayTask` | `controller/relay.go:495` |
| `RelayTaskFetch` | `controller/relay.go:480` |
| `RelaySeedanceTaskFetch` | `controller/seedance.go:11` |
| `VideoProxy` | `controller/video_proxy.go:33` |

> 说明：即梦的"查询"也走 `POST /jimeng/?Action=CVSync2AsyncGetResult`，由 `JimengRequestConvert` 改写成 `GET /v1/video/generations/:task_id`。

## 请求体

### 统一格式（`dto/video.go:3`，`VideoRequest`）

`/v1/video/generations`、`/v1/videos`、`/v1/videos/:video_id/remix` 直接使用此结构。Kling / 即梦 / Seedance 的原生请求体经中间件改写为 `{model, prompt, metadata: <原始请求>}` 后同样落到该结构。

| 字段 | 类型 | JSON | 说明 |
|---|---|---|---|
| `Model` | `string` | `model,omitempty` | 模型 / 风格 ID，如 `kling-v1` |
| `Prompt` | `string` | `prompt,omitempty` | 文本提示词 |
| `Image` | `string` | `image,omitempty` | 图片输入（URL 或 Base64） |
| `Duration` | `float64` | `duration` | 视频时长（秒） |
| `Width` | `int` | `width` | 视频宽度 |
| `Height` | `int` | `height` | 视频高度 |
| `Fps` | `int` | `fps,omitempty` | 帧率 |
| `Seed` | `int` | `seed,omitempty` | 随机种子 |
| `N` | `int` | `n,omitempty` | 生成数量 |
| `ResponseFormat` | `string` | `response_format,omitempty` | 响应格式 |
| `User` | `string` | `user,omitempty` | 用户标识 |
| `Metadata` | `map[string]any` | `metadata,omitempty` | 厂商特定 / 自定义参数（`negative_prompt`、`style`、`quality_level` 等） |

### 厂商原生请求体

各厂商原生端点（Kling、即梦、Seedance/ARK）接受厂商官方 API 格式，由 `middleware/kling_adapter.go`、`middleware/seedance_adapter.go`、`middleware/jimeng_adapter.go` 改写。

Gemini / Vertex (Veo) 的上游请求体在适配器内部构造，定义于 `relay/channel/task/gemini/dto.go`（`VeoRequestPayload` / `VeoInstance` / `VeoParameters` / `VeoImageInput`）。

## 响应体

### 提交任务响应（`dto/video.go:19`，`VideoResponse`）

```json
{
  "task_id": "string",
  "status": "string"
}
```

### 查询任务状态响应（`dto/video.go:25`，`VideoTaskResponse`）

| 字段 | 类型 | JSON | 说明 |
|---|---|---|---|
| `TaskId` | `string` | `task_id` | 任务 ID |
| `Status` | `string` | `status` | 任务状态 |
| `Url` | `string` | `url,omitempty` | 视频资源 URL（成功时） |
| `Format` | `string` | `format,omitempty` | 视频格式，如 `mp4` |
| `Metadata` | `*VideoTaskMetadata` | `metadata,omitempty` | 结果元数据 |
| `Error` | `*VideoTaskError` | `error,omitempty` | 错误信息（失败时） |

`VideoTaskMetadata`（`dto/video.go:35`）：

| 字段 | 类型 | JSON | 说明 |
|---|---|---|---|
| `Duration` | `float64` | `duration` | 实际生成时长 |
| `Fps` | `int` | `fps` | 实际帧率 |
| `Width` | `int` | `width` | 实际宽度 |
| `Height` | `int` | `height` | 实际高度 |
| `Seed` | `int` | `seed` | 使用的随机种子 |

`VideoTaskError`（`dto/video.go:44`）：

| 字段 | 类型 | JSON |
|---|---|---|
| `Code` | `int` | `code` |
| `Message` | `string` | `message` |

### OpenAI 兼容端点响应（`dto/openai_video.go:16`，`OpenAIVideo`）

`/v1/videos`、`/v1/videos/:task_id`、`/v1/videos/:video_id/remix` 各适配器通过实现 `OpenAIVideoConverter.ConvertToOpenAIVideo` 转换为该结构。状态取值（`dto/openai_video.go:8`）：`queued` / `in_progress` / `completed` / `failed` / `unknown`。

| 字段 | 类型 | JSON | 说明 |
|---|---|---|---|
| `ID` | `string` | `id` | 视频 ID |
| `TaskID` | `string` | `task_id,omitempty` | 兼容旧接口，待废弃 |
| `Object` | `string` | `object` | 固定 `video` |
| `Model` | `string` | `model` | 模型名 |
| `Status` | `string` | `status` | `queued` / `in_progress` / `completed` / `failed` |
| `Progress` | `int` | `progress` | 进度百分比 |
| `CreatedAt` | `int64` | `created_at` | 创建时间（Unix 秒） |
| `CompletedAt` | `int64` | `completed_at,omitempty` | 完成时间 |
| `ExpiresAt` | `int64` | `expires_at,omitempty` | 过期时间 |
| `Seconds` | `string` | `seconds,omitempty` | 视频秒数 |
| `Size` | `string` | `size,omitempty` | 视频尺寸 |
| `RemixedFromVideoID` | `string` | `remixed_from_video_id,omitempty` | remix 来源视频 ID |
| `Error` | `*OpenAIVideoError` | `error,omitempty` | 错误信息 |
| `Metadata` | `map[string]any` | `metadata,omitempty` | 额外元数据 |

`OpenAIVideoError`（`dto/openai_video.go:50`）：

| 字段 | 类型 | JSON |
|---|---|---|
| `Message` | `string` | `message` |
| `Code` | `string` | `code` |

### Seedance / ARK 任务查询响应

单任务为 map，列表响应结构定义于 `relay/seedance_task.go:23`（`seedanceTaskListResponse`）：

| 字段 | 类型 | JSON |
|---|---|---|
| `Items` | `[]map[string]any` | `items` |
| `Total` | `int` | `total` |

每项内容由各适配器实现 `ArkVideoTaskConverter.ConvertToArkVideoTask`（`relay/channel/adapter.go:97`）补全，包含 `id` / `status` / `model` / `created_at` / `updated_at` / `content.video_url` 等字段（逻辑见 `relay/seedance_task.go:202`）。

## 适配器与转换器接口

定义于 `relay/channel/adapter.go`：

- `TaskAdaptor`（`:34`）：视频 / 任务类厂商适配器核心接口（`ValidateRequestAndSetAction`、`BuildRequestBody`、`DoResponse`、`FetchTask`、`ParseTaskResult`、计费方法）。
- `OpenAIVideoConverter`（`:93`）：`ConvertToOpenAIVideo(*model.Task) ([]byte, error)` —— 输出 `OpenAIVideo`。
- `ArkVideoTaskConverter`（`:97`）：`ConvertToArkVideoTask(*model.Task) ([]byte, error)` —— 输出 Seedance / ARK 形态。

视频适配器实现位于 `relay/channel/task/{ali,dimensio,doubao,gemini,hailuo,jimeng,kling,sora,vertex,vidu}/adaptor.go`，其中 `sora` 对应 `/v1/videos` 的 OpenAI / Sora 渠道。
