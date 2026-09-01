# Paipu MiniMax H3 渠道接入设计

**日期：** 2026-08-24  
**状态：** 设计完成，等待按计划实施  
**协议来源：** `docs/new-channels/cn-paipu.html`、`docs/new-channels/cn-paipu-h3.html`

## 1. 结论

MiniMax H3 与 Paipu Seedance 使用同一套上游任务协议，不需要新增渠道类型或复制一套任务 adaptor：

```text
POST /v1/videos
GET  /v1/videos/{task_id}
GET  /v1/videos/{task_id}/content
Authorization: Bearer <API key>
```

现有 `constant.ChannelTypePaipu`、`newapivideo.NewPaipuTaskAdaptor()`、异步任务生命周期、公共任务 ID 隔离、失败退款和创建禁止重试逻辑继续复用。

需要新增的是 Paipu adaptor 内的 H3 模型契约：模型级时长、分辨率、画幅、素材数量、素材角色和 URL 类型校验，以及模型级默认时长和计费测量来源。

第一期不把 `lec-h3video-2k`、`lec-minimax-h3`、`lec-minimax-h3-768p` 加入 `pkg/modelrouting.CanonicalModels`，也不让它们进入 Seedance 专用 `/v1/video/generations`。这些 ID 是 Paipu 上游模型名，最终通过配置导入的模型映射和 route target 使用；这样不会把 H3 私有协议误暴露为 Seedance 公共协议。

## 2. 共享能力与专属能力边界

### 2.1 直接复用

- Paipu Base URL 默认值：`https://api.paipu.net`。
- Bearer 鉴权和 JSON 请求头。
- `POST /v1/videos` 创建、`GET /v1/videos/{task_id}` 轮询。
- 创建请求不自动重试；不确定结果时只查询原任务。
- 上游任务 ID 与网关公共任务 ID 的隔离。
- `queued`、`in_progress`、`completed`、`failed` 等状态映射。
- 结果 URL 投影、失败信息清理、预扣/结算/退款和 Ark 路由。
- `paipuRequest` 数组字段：`images`、`videos`、`audios`。

### 2.2 H3 专属

- `lec-h3video-2k` 固定 15 秒、1440p、最多 9 图、按次成本。
- `lec-minimax-h3` 必须 6-15 秒、必须 1-9 图，可选 3 视频和 3 音频；音频不能单独提交。
- `lec-minimax-h3-768p` 1-15 秒、默认 5 秒、固定 768p，最多 9 图和 3 音频，不支持视频。
- H3 仅允许 `16:9`、`9:16`。
- H3 1/2/3+ 图片的首帧、首尾帧和参考素材语义。
- 模型级默认时长在计费预扣前生效。
- H3 源表和 `h3官价` 的独立解析与价格归一化。

## 3. H3 能力矩阵

| 上游模型 | 输出 | 时长 | 图片 | 视频 | 音频 | URL/画幅 | 计费 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `lec-h3video-2k` | 固定 1440p | 固定 15 秒；请求省略时使用 15 | 0-9；公网 HTTPS 或 `data:image` | 不支持 | 不支持 | `16:9`、`9:16` | 按次 |
| `lec-minimax-h3` | 固定 720p | 必填 6-15 秒 | 必填 1-9；1 图首帧、2 图首尾帧、3+ 图参考 | 0-3；公网 HTTP(S) | 0-3；公网 HTTP(S)，不能单独提交 | `16:9`、`9:16` | ¥0.15/秒文档价，按时长 |
| `lec-minimax-h3-768p` | 固定 768p | 省略默认 5 秒，允许 1-15 秒 | 0-9；文档写公网 HTTPS | 不支持 | 0-3；文档写公网 HTTPS | `16:9`、`9:16` | ¥0.02/秒文档价，按时长 |

### 3.1 文档冲突和待实测项

`lec-minimax-h3-768p` 的参数表写有“由网关映射到上游分辨率字段”，但 cURL 示例没有 `resolution` 字段。实现采用以下门禁：

1. mock 契约先只发送文档示例中的核心字段，并保留 `resolution` 的模型级映射开关。
2. 真实验收必须分别验证省略 `resolution`、发送 `resolution: "768p"` 的结果。
3. 在真实 fixture 固化前，H3 渠道保持 disabled；不得依赖猜测启用。

## 4. Ark 请求到 Paipu H3 的转换

网关公共请求继续使用 Ark 内容数组。转换器只在最终 `UpstreamModelName` 已解析后选择 H3 契约。

```go
type paipuH3Contract struct {
	Model                  string
	MinDuration            int
	MaxDuration            int
	DefaultDuration        int
	DurationRequired       bool
	FixedResolution        string
	AllowedRatios          []string
	MinImages              int
	MaxImages              int
	MaxVideos              int
	MaxAudios              int
	AudioRequiresVisual    bool
	ImageURLPolicy         paipuMediaURLPolicy
	VideoURLPolicy         paipuMediaURLPolicy
	AudioURLPolicy         paipuMediaURLPolicy
	ResolutionWireMode     paipuResolutionWireMode
	BillingMode            paipuH3BillingMode
}
```

契约查找只按完整上游模型 ID 匹配；未知模型继续走现有 Paipu Seedance 动态契约，不把未知 ID 当作 H3。

请求编码规则：

- `model` 使用导入映射后的上游模型名。
- 文本内容合并到 `prompt`，必须恰好一个非空文本项。
- Ark `duration` 显式值保留；省略值由 H3 契约设置计费默认值，但是否写回上游由该模型的 wire mode 决定。
- Ark `ratio` 写成 Paipu `aspect_ratio`。
- 图片、视频、音频保持各自数组内的输入顺序。
- 不向 H3 发送 Seedance 私有的 `seconds`、`size`、`quality`、`watermark`、`generate_audio`、`seed`、`callback_url` 等字段。
- 显式 `0`/`false` 继续通过指针字段保留；缺省字段不出现在 JSON。

### 4.1 图片角色

- 未带 role 的图片按 H3 数组顺序解释。
- `reference_image` 可与未带 role 的图片混用，但不改变顺序。
- `first_frame`/`last_frame` 只在能无损转换为 H3 的 1 图或 2 图顺序时接受；不能把首尾帧与 3+ 参考图混合后静默丢失角色。
- H3 数组协议不保留 role；转换前必须完成确定性校验，失败返回 `InvalidParameter.content`。

### 4.2 URL 安全

沿用 `ParseTaskMediaURL` 和 Paipu 公网目标校验，按模型/媒体类型配置：

- 图片允许的 data URI 必须是 `data:image/*`，且 MIME 与媒体类型匹配。
- 文档只声明公网 HTTPS 的模型/媒体不得接受 `http://`、`asset://`、`file://`、私网 IP 或本地路径。
- 视频、音频 data URI 是否可用不得由 Seedance 通用能力推断；以 H3 契约和真实验收 fixture 为准。

## 5. 路由与模型身份

### 5.1 第一阶段身份规则

- `lec-*` 只作为 `route target.upstream_model` 和渠道模型映射中的上游模型名。
- `client_model`/`canonical_model` 继续来自配置导入的公共模型目录，不在本次把 H3 ID 加入 `CanonicalModels`。
- `middleware.extractSeedanceRoutingInput` 的 `/v1/video/generations` 仍只处理 Seedance 公共模型；H3 仅走 Ark `/api/v3/contents/generations/tasks/*`。
- 如果未来需要用户直接以 H3 模型 ID 调用公共端点，另开模型路由设计，不在本计划中隐式扩展。

### 5.2 Route target 契约

`ValidateVideoRouteTargetContract` 在 Paipu 分支先按 `target.UpstreamModel` 查 H3 契约：

- 分辨率必须是模型固定分辨率：`1440p`、`720p` 或 `768p`。
- 时长范围不能超出模型契约；固定 15 秒模型的 min/max 必须都是 15。
- 图片、视频、音频上限和最小值必须与模型契约一致。
- `aspect_ratios` 只能是 `16:9`、`9:16`。
- H3 route 不得沿用 Seedance 模型名后缀推断分辨率。

配置 schema 允许 `768p` 和 `1440p` 作为任务路由分辨率；其他渠道仍由各自 route contract 拒绝不支持的档位。

## 6. 计费与默认时长

- `lec-h3video-2k`：渠道成本按次；请求默认/固定时长 15 秒只用于能力校验和账务审计，不按时长倍增成本。
- `lec-minimax-h3`：渠道成本按秒，预扣和结算使用请求时长，最小 6 秒。
- `lec-minimax-h3-768p`：渠道成本按秒，省略时长按 5 秒预扣，显式时长按请求值计费。
- 默认时长必须在 `ValidateBillingRequest`、预扣和 `EstimateDurationSeconds` 之前写入 request state；不得只在 `BuildRequestBody` 中补默认值。
- 所有时长继续受 `relaycommon.MaxTaskDurationSeconds` 和 H3 更严格上限双重保护。
- 计费转换使用已有 duration quota 饱和 helper；不能新增裸 `int(float64(...))` 计算。
- 2K 官方固定单价若进入现有销售模板，归一化为 min=max=15 的 `per_duration` 销售价，确保用户实收固定；渠道成本仍保留 `per_request`。

## 7. 配置导入和模型价格

当前 V1 SD 模板对 `h3`/`h3官价` 只做阻断，不能把 H3 数据静默写进 Seedance Token 模板。本次增加独立 H3 归一化路径：

- `h3`：读取渠道、模型 ID、系列、版本、清晰度、计费方式、单价；生成 Paipu 渠道成本、模型映射和 route target。
- `h3官价`：读取系列、模型、版本、分辨率、输入素材价格和输出价格；只生成 H3 的按次/按秒销售数据，不套用 Seedance token 公式。
- H3 的素材限制、最小时长、默认时长和画幅进入 route blueprint；缺失或冲突时 `FAIL`。
- 2K 固定按次销售转换为固定 15 秒的 `per_duration` 销售行；H3 720p/768p 使用 `per_duration`。
- 生成器输出中的 `source_ref`、原始表名、行号和价格单位必须可追溯。
- 真实 H3 行在生成、暂存、审阅、发布、激活前保持 disabled；没有真实上游验收不得生成“已通过”报告。

## 8. 隐私和错误处理

- 公开 Ark 响应不泄漏 API Key、Paipu 私有任务 ID、上游模型名、渠道 ID、原始响应或内部计费字段。
- H3 参数错误在上游请求、任务落库和预扣前返回 400。
- 创建 429、5xx、超时和连接中断不重试；只退款明确失败，结果不确定时保持查询路径。
- 轮询 404/410 映射为任务不存在/过期；完成态没有结果 URL 时按现有 Paipu 失败闭环处理，除非真实 H3 fixture 证明 `/content` 是唯一结果来源并另行设计代理。

## 9. 验收标准

1. 同一个 Paipu adaptor 可按上游模型区分三种 H3 契约，不新增渠道类型。
2. Ark 文本、图片、首尾帧、视频、音频和默认时长请求在 H3 合法范围内生成精确 Paipu JSON。
3. H3 的不合法时长、分辨率、画幅、素材数量、角色和 URL 在预扣前确定性失败。
4. 2K 按次、720p/768p 按秒的预扣、结算、退款和重复轮询幂等通过测试。
5. route target 能表达 `1440p`、`720p`、`768p` 和 H3 时长/素材边界，Seedance 其他渠道不回归。
6. H3 源表生成结果经过配置导入 staging、review、publish、activate 流程，默认仍 disabled。
7. 有凭据时完成三个模型的真实创建、轮询、成功/失败和 `resolution` wire-mode 验收；无凭据时明确记录阻塞，不伪造通过。

## 10. 非目标

- 不复制 Paipu adaptor，不新增 `ChannelTypeMiniMaxH3`。
- 不修改 Seedance `/v1/video/generations` 的公共字段协议。
- 不从 HTML 价格直接硬编码生产价格，不把 `h3` 数据写入 Seedance Token 销售公式。
- 不在没有真实契约证据时实现 H3 `/content` 私有结果代理。
- 不在本计划中把 H3 ID 注册成全局公开 canonical model。
