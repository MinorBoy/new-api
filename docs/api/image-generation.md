# 图像生成 API

本文档列出 new-api 内置的图像生成相关 HTTP 接口（端点、请求体、响应体）。

所有 OpenAI 兼容图像端点定义于 `router/relay-router.go`，统一走 `controller.Relay(c, types.RelayFormatOpenAIImage)` → `relay.ImageHelper`（`relay/image_handler.go:24`）。请求/响应 DTO 定义于 `dto/openai_image.go`。

## 鉴权与中间件

- 鉴权使用 `Authorization: Bearer <API Key>`（`middleware.TokenAuth`）。
- `/v1/*` 端点经过 `Distribute`（按 `model` 选择渠道）。
- `/api/v3/images/generations`（Seedance/ARK 原生路径）由 `SeedanceRequestConvert` 中间件（`middleware/seedance_adapter.go:23`）改写为 `/v1/images/generations`，并额外经过 `ModelRequestRateLimit`。

## 接口端点

| 方法 | 端点 | 路由位置 | 说明 |
|---|---|---|---|
| POST | `/v1/images/generations` | `router/relay-router.go:121` | OpenAI 图像生成 |
| POST | `/v1/images/edits` | `router/relay-router.go:124` | OpenAI 图像编辑 |
| POST | `/v1/edits` | `router/relay-router.go:118` | 旧别名路径，映射为 `RelayModeEdits` |
| POST | `/v1/images/variations` | `router/relay-router.go:163` | `RelayNotImplemented`（未实现，显式注册占位） |
| POST | `/api/v3/images/generations` | `router/relay-router.go:88` | Seedance / 火山方舟 ARK 原生路径（中间件改写为 `/v1/images/generations`） |

路径 → RelayMode 映射定义于 `relay/constant/relay_mode.go:69-74`：

- `/v1/images/generations` → `RelayModeImagesGenerations`
- `/v1/images/edits` → `RelayModeImagesEdits`
- `/v1/edits` → `RelayModeEdits`

Handler 分发链：

- `controller.Relay`（`controller/relay.go:69`）→ 对 `RelayFormatOpenAIImage` 调用 `helper.GetAndValidateRequest`（`relay/helper/valid_request.go:246`，图像分支 `GetAndValidOpenAIImageRequest` 定义于 `:364`）→ `relay.ImageHelper`（`relay/image_handler.go:24`）。
- 上游响应处理：`OpenaiImageHandler`（`relay/channel/openai/relay_image.go:122`，非流式）/ `OpenaiImageStreamHandler`（`:192`，流式），响应体原样透传。

## 请求体

### JSON 请求（`dto/openai_image.go:18`，`ImageRequest`）

`/v1/images/generations` 与 `/v1/edits` 使用 JSON 请求体。该结构带有自定义 `UnmarshalJSON`/`MarshalJSON`（`dto/openai_image.go:46`/`:75`），未识别字段会被收进 `Extra` 透传上游。

| 字段 | 类型 | JSON | 说明 |
|---|---|---|---|
| `Model` | `string` | `model` | 模型名 |
| `Prompt` | `string` | `prompt`（`binding:"required"`） | 文本提示词 |
| `N` | `*uint` | `n,omitempty` | 生成数量，上限 `MaxImageN = 128`（`dto/openai_image.go:16`） |
| `Size` | `string` | `size,omitempty` | 尺寸，如 `1024x1024` |
| `Quality` | `string` | `quality,omitempty` | 质量，如 `hd` |
| `ResponseFormat` | `string` | `response_format,omitempty` | 响应格式，`url` 或 `b64_json` |
| `Style` | `json.RawMessage` | `style,omitempty` | 风格 |
| `User` | `json.RawMessage` | `user,omitempty` | 用户标识 |
| `ExtraFields` | `json.RawMessage` | `extra_fields,omitempty` | 额外字段 |
| `Background` | `json.RawMessage` | `background,omitempty` | 背景 |
| `Moderation` | `json.RawMessage` | `moderation,omitempty` | 内容审核 |
| `OutputFormat` | `json.RawMessage` | `output_format,omitempty` | 输出格式 |
| `OutputCompression` | `json.RawMessage` | `output_compression,omitempty` | 输出压缩 |
| `PartialImages` | `json.RawMessage` | `partial_images,omitempty` | 部分图像 |
| `Stream` | `*bool` | `stream,omitempty` | 是否流式 |
| `Images` | `json.RawMessage` | `images,omitempty` | 输入图像 |
| `Mask` | `json.RawMessage` | `mask,omitempty` | 蒙版 |
| `InputFidelity` | `json.RawMessage` | `input_fidelity,omitempty` | 输入保真度 |
| `Watermark` | `*bool` | `watermark,omitempty` | 水印 |
| `WatermarkEnabled` | `json.RawMessage` | `watermark_enabled,omitempty` | 水印开关（智谱 4v） |
| `UserId` | `json.RawMessage` | `user_id,omitempty` | 用户 ID |
| `Image` | `json.RawMessage` | `image,omitempty` | 输入图像 |
| `Extra` | `map[string]json.RawMessage` | `json:"-"` | 捕获未识别字段透传上游 |

> 说明：多数可选字段使用 `json.RawMessage` / 指针类型，以保留显式零值并原样透传上游。

### multipart 请求（`/v1/images/edits`）

`/v1/images/edits` 的 `multipart/form-data` 请求从表单解析而非 JSON，逻辑见 `relay/helper/valid_request.go:364-414`。

### 计费

`GetTokenCountMeta()`（`dto/openai_image.go:134`）按 DALL-E size/quality 计算价格比率，并把生成数量 `n` 作为独立计费维度写入 `BillingRatios`。

## 响应体

上游 OpenAI 响应原样透传，结构对应 `dto/openai_image.go:184`（`ImageResponse`）：

| 字段 | 类型 | JSON | 说明 |
|---|---|---|---|
| `Data` | `[]ImageData` | `data` | 生成的图像列表 |
| `Created` | `int64` | `created` | 创建时间（Unix 秒） |
| `Metadata` | `json.RawMessage` | `metadata,omitempty` | 元数据 |

`ImageData`（`dto/openai_image.go:189`）：

| 字段 | 类型 | JSON | 说明 |
|---|---|---|---|
| `Url` | `string` | `url` | 图像 URL |
| `B64Json` | `string` | `b64_json` | Base64 编码图像 |
| `RevisedPrompt` | `string` | `revised_prompt` | 修订后的提示词 |

## 各渠道图像适配器

各渠道通过实现 `ConvertImageRequest` 转换为厂商上游格式（调用见 `relay/image_handler.go:84`）。主要实现：

- `relay/channel/openai/relay_image.go` / `relay/channel/openai/adaptor.go` — OpenAI / 兼容图像处理与用量标准化。
- `relay/channel/ali/image.go`、`relay/channel/ali/image_wan.go` — 阿里云（通义万相）。
- `relay/channel/jimeng/image.go` — 即梦。
- `relay/channel/minimax/image.go` — MiniMax。
- `relay/channel/task/gemini/image.go` — Gemini 异步图像任务。
- `relay/channel/zhipu_4v/image.go` — 智谱 4v。
- `service/image.go` — 图像服务辅助程序。
