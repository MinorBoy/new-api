# Mikoto 视频渠道设计

## 目标

将 Mikoto 的 Sora 和 Seedance 视频能力接入现有任务渠道，使调用方继续使用 Ark SDK 的
`/api/v3/contents/generations/tasks/*`，不新增客户端专用接口。渠道默认禁用，且不在代码中
写入密钥或未核验的成本配置。

## 协议事实来源

本设计以 `docs/new-channels/cn-mikoto-sora.html` 和
`docs/new-channels/cn-mikoto-seedance.html` 为上游协议来源。两份文档都声明：

| 项目 | 值 |
| --- | --- |
| Base URL | `https://api.mikoto.vip`，实际由渠道 Base URL 配置提供 |
| 鉴权 | `Authorization: Bearer <API_KEY>` |
| 提交任务 | `POST /v1/videos` |
| 查询任务 | `GET /v1/videos/{task_id}` |
| 下载内容 | `GET /v1/videos/{task_id}/content` |
| 任务方式 | 异步提交、轮询查询 |
| 流式响应 | 文档未声明，不支持 StreamOptions |
| 删除任务 | 文档未声明，不实现删除上游任务 |

提交成功的最小公共响应为任务 ID 和状态。查询响应中的 `content_url`、`video_url` 或内容下载
路径是成功任务的结果来源；渠道适配器必须通过现有私有任务数据保存上游响应，且不得向普通用户
泄露上游任务 ID、密钥或错误中的敏感内容。

## 能力矩阵

| 能力 | Sora `sora-v3-pro` | Seedance 模型 | 依据 |
| --- | --- | --- | --- |
| 文生视频 | 支持 | 支持 | 文档明确 |
| 参考图片 | `image_url` 和 `reference_image_urls`，总数最多 9 | `images`，最多 9 | 文档明确 |
| 参考视频 | `reference_video` 或 `reference_videos`，最多 3 | `referenceVideos`，最多 3 | 文档明确 |
| 参考音频 | `audio_url`，最多 3 | `referenceAudios`，最多 3 | 文档明确 |
| data URI | 未声明，拒绝 | 图片、视频、音频均支持 | 文档明确 |
| 时长 | `seconds`，4 至 15 秒 | `duration`，4 至 15 秒 | 文档明确 |
| 画幅 | 必填，6 种枚举 | 可选，5 种枚举，默认 `16:9` | 文档明确 |
| 分辨率 | 仅 `720p` | 由模型决定，不额外传 `resolution` | 文档明确 |
| 生成音频 | 参考音频输入，无独立开关 | `generate_audio`，默认 `true` | 文档明确 |
| 参考模式 | `video_config.reference_mode` | `reference_mode` | 文档明确 |
| 失败退款 | 最终失败自动退款 | 最终失败自动退款 | 文档明确，本站仍按既有任务结算流程处理 |

Seedance 模型 ID 在文档中列为 `seedance-2.0-1080p`、`seedance-2.0-720p`、
`seedance-fast-480p` 和 `seedance-fast-720p`；Sora 模型 ID 为 `sora-v3-pro`。当前渠道模型
导入快照没有 Mikoto 行，因此这些 ID 仅用于协议分流和单元测试，不自动生成正式渠道成本或启用的
模型模板。正式模型、成本和利润以更新后的收录表或经审核的配置导入快照为准。

## 架构

新增 `Mikoto` 渠道类型并复用 `relay/channel/task/newapivideo` 的通用任务适配器、任务查询、
错误脱敏和 Ark 任务转换器。Mikoto profile 固定使用 `/v1/videos` 的提交和查询路径，使用现有
Bearer 鉴权实现；请求方言按实际的上游模型分成 Sora 与 Seedance 两支，避免把两个不兼容的请求
格式混入同一个编码器。

```text
Ark SDK 请求
  -> 现有 Seedance 任务路由和账务预扣
  -> Mikoto TaskAdaptor
       -> Sora 编码器 或 Seedance 编码器
       -> Mikoto POST /v1/videos
  -> 现有任务持久化
  -> Mikoto GET /v1/videos/{task_id}
  -> 现有 Ark / OpenAI 任务结果转换与结算
```

渠道专属代码仅负责 profile、模型分流、字段映射和严格验证。通用的 HTTP 调用、上游响应解析、
任务轮询、结果 URL 保存、失败退款、配额饱和审计和 Ark 返回结构保持不变。

## 请求映射与验证

### Sora

Ark 的文本、时长、画幅和媒体内容映射到 `model`、`prompt`、`seconds`、`aspect_ratio`、
`image_url`、`reference_image_urls`、`reference_video` 或 `reference_videos`、`audio_url` 和
`video_config.reference_mode`。显式零值或 `false` 的可选字段使用指针并保留；未提供的字段省略。

验证在发送上游前完成：时长必须为 4 至 15 的整数；分辨率只能是 `720p`；画幅限于文档枚举；图片、
视频、音频及素材总数不能超过文档上限；远程素材必须是可公开访问的 HTTPS URL；视频和音频的
时长上限通过现有媒体元数据服务校验。音频需要至少一张参考图片；`start_frame` 只能有一张图片，
`start_end` 只能有两张图片且不能同时使用参考视频。

### Seedance

Ark 的文本、时长、画幅、媒体和生成音频映射到 `model`、`prompt`、`duration`、
`aspect_ratio`、`images`、`reference_mode`、`referenceVideos`、`referenceAudios` 和
`generate_audio`。不发送 `resolution`，因为模型名决定清晰度。

验证时长为 4 至 15 的整数；画幅仅限文档枚举；最多 9 张图片、3 个参考视频、3 个参考音频；允许
各媒体的合法 data URI 或公开 HTTPS URL。参考视频与音频的单文件 50 MB 和整体请求 256 MB 约束
必须在现有请求体/媒体校验边界落实；不可验证的远程文件尺寸不在客户端伪造通过。`generate_audio`
缺省时不发送，让上游采用其默认值；显式 `false` 必须发送。

文档未提供真人图片的机器可验证规则，因此不会新增基于 URL 内容的“真人”检测；只保留上游返回的
参数错误映射。

## 状态、错误和计费

`queued`、`processing`、`completed` 与 `failed` 映射到既有任务状态。Seedance 的 `content_url`
和 `url`、Sora 的 `video_url` 由通用响应投影提取；成功状态缺少任何结果 URL 视为上游无效响应。
`400`、`401`、`403`、`429` 和 `5xx` 保留为现有安全错误结构，错误文本经过脱敏处理。

该变更不新增计费公式或硬编码价格。任务时长仍经过 `relaycommon.MaxTaskDurationSeconds` 上限检查，
后续预扣、结算和最终失败退款继续使用现有安全路径与配额饱和审计。待 Mikoto 进入经过审核的渠道
成本模板后，再由现有配置导入流程提供费率。

## 测试与验收

1. profile 测试断言提交、查询、鉴权头及模型方言选择。
2. 请求契约测试覆盖 Sora 的文字、图片/视频/音频参考及边界；覆盖 Seedance 的文字、data URI、
   显式 `generate_audio: false`、缺省字段省略和媒体数量上限。
3. 任务响应测试覆盖提交任务 ID、轮询中、成功结果 URL、失败状态和上游错误脱敏。
4. Ark 生命周期测试覆盖提交、列表、单任务查询和多模态请求，确认对外仍是 Ark 结构。
5. 执行目标包测试、相关 relay/router 测试和 `git diff --check`。没有 Mikoto 凭据时，不执行或
   声称真实上游验收。

## 风险与回滚

主要风险是文档未包含渠道模型导入记录和可用于真实验收的密钥。实现以单元测试固定已公开的协议，
真实连通性、模型可用性、成本和退款金额仍待部署前凭据验收。渠道默认禁用；回滚只需禁用或删除
Mikoto 渠道配置，不影响既有渠道和 Ark 路由。
