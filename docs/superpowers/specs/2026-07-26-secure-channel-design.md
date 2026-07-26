# Secure Seedance 渠道接入设计

**日期：** 2026-07-26  
**状态：** 已确认，进入接入计划阶段

## 1. 目标

用一个 Secure 渠道类型接入三个上游 Video 分组：

- 特价 Video
- 海外 Video
- 企业 Video

管理员在 Secure 上游为每个分组创建一枚 API Key，再在 new-api 创建三个 Secure 渠道。三个渠道复用同一套任务 adaptor，通过渠道配置 `secure_video_group` 选择协议。用户始终使用标准 Ark SDK 的创建、单查和列表接口，调用代码不变。

## 2. 管理配置

`secure_video_group` 是渠道级枚举：

```text
discount
overseas
enterprise
```

- 该选择器只在渠道类型为 Secure 时显示。
- 切换到其他渠道类型时，提交 payload 必须删除 `secure_video_group`，不能留下隐藏配置。
- Secure 渠道保存时该字段必填；后端对缺失或未知枚举拒绝保存。
- 三个分组各建一个普通渠道，每个渠道保存一枚对应上游 Key。
- 不做一条渠道内的三 Key 自动分流，也不复用现有多 Key 轮询。

默认 Base URL 为 `https://token.secure-skill.com`，允许管理员覆盖。

## 3. 一套 adaptor、三种 profile

Secure 使用一个 task-only 渠道类型和一个 `TaskAdaptor` 构造入口。初始化时读取 `ChannelOtherSettings.SecureVideoGroup`，选择不可变 profile：

| 分组 | 创建 | 查询 | 请求类型 |
| --- | --- | --- | --- |
| `discount` | `POST /api/generate-video` | `GET /api/task/{id}` | multipart/form-data |
| `overseas` | `POST /api/generate-video` | `GET /api/task/{id}` | multipart/form-data |
| `enterprise` | `POST /v1/videos` | `GET /v1/videos/{id}` | JSON |

共享核心负责鉴权、网络请求、创建响应、轮询、状态/URL 投影、公开 ID、Ark 查询响应和计费。每个 profile 只负责请求校验与序列化。

备选方案及取舍：

- 三个独立渠道类型：UI 和后端直观，但重复注册、模型目录和任务逻辑，且与“优先设计为一套”的要求冲突，不采用。
- 一条渠道保存三枚 Key：减少渠道记录，但破坏现有路由对成本、优先级、可用性和 Key 的一一绑定，不采用。

## 4. 分组能力

### 4.1 特价 Video

- 模型：`video-2.0-fast`、`video-2.0-mini`、`video-2.0-pro`
- 必须至少有 1 张图片，最多 9 张，不支持纯文生
- 图片都映射为重复 multipart `files` URL；`last_frame` 不支持
- 视频映射为重复 `video_urls`，音频映射为重复 `audio_urls`
- 视频和音频合计最多 3 路
- 素材只支持 HTTP(S) URL
- 时长 4 至 15 秒
- 比例仅 `16:9`、`9:16`
- fast/mini 只支持 720p；pro 支持 720p、1080p、4k
- 完成地址为 Secure 转存地址

### 4.2 海外 Video

- 模型：`video-2.0-fast`、`video-2.0-mini`、`video-2.0-pro`
- 支持文生、首帧/首尾帧和全能参考
- 首尾帧模式使用 `functionMode=first_last_frames` 和最多两个 `image_file_N`
- 多模态使用 `functionMode=omni_reference`，图片/视频/音频分别映射到 `*_file_N`
- prompt 中按实际序号补齐缺失的 `@image_file_N`、`@video_file_N`、`@audio_file_N` 引用
- 图片最多 9、视频最多 3、音频最多 3，全能参考总素材最多 12，参考视频总时长不超过 15 秒
- 素材只支持 HTTP(S) URL
- 时长 4 至 15 秒
- 比例支持 `1:1`、`4:3`、`3:4`、`16:9`、`9:16`、`21:9`
- fast/mini 只支持 720p；pro 支持 720p、1080p
- 完成地址为上游源链接，不转存

### 4.3 企业 Video

- 仅模型 `video-2.0-pro`
- 固定 720p
- 支持文生和图片/视频/音频参考
- 第一张图片映射到 `image_url`，其余图片映射到 `extra_images`
- 视频、音频映射到 `extra_videos`、`extra_audios`
- `last_frame` 不支持，不能伪造严格首尾帧
- 图片最多 9、视频最多 3、音频最多 3，素材只支持 HTTP(S) URL
- 时长 5 至 15 秒，必填
- 比例支持 `16:9`、`9:16`、`1:1`
- 完成地址直接来自上游，不转存

## 5. Ark 兼容

- 三个分组都支持 Ark 创建、单查和列表。
- 用户模型名通过路由策略映射到 Secure 上游模型；公开响应仍返回用户原始模型名。
- 创建只返回公开 Ark 任务 ID；上游 ID 只存私有数据。
- `routing` 只供 new-api 选候选，不发送上游。
- 各分组不能兑现的字段返回明确 400，不静默丢弃影响能力或计费的参数。
- `callback_url`、Ark DELETE 和 Secure 的 `video-link/source-link/download` 辅助接口不在本期公开 API 范围。

## 6. 状态、结果与错误

- 支持 `queued/pending`、`in_progress/running/processing`、`completed/succeeded/success`、`failed/error/cancelled/canceled`。
- 成功必须同时满足终态成功状态和结果 URL 非空。
- URL 兼容 `video_url`、`url`、`result_url`、`output[0].url`、`metadata.*` 和 `content.video_url`。
- 404/410 作为不存在或过期终态；4xx 业务错误转换为失败；5xx/网络错误保持轮询重试。
- 普通用户响应和日志不得暴露 Key、上游任务 ID、渠道 ID、分组内部信息或签名 URL 的敏感查询串。

## 7. 路由与计费

- 三个 Secure 渠道是独立候选，分别拥有渠道 ID、Key、成本、优先级、权重和健康状态。
- 路由目标同时绑定 `channel_id + upstream_model` 并声明分组能力，不允许请求跨用错误分组 Key。
- 成本/利润路由可在三个分组和其他上游之间比较，不设置硬编码分组优先级。
- 价格按管理员配置的按次或按时长规则；上游没有权威 usage 时不得臆造 Token 结算。
- 失败任务走公共退款；成功任务按现有结算机制闭合。

## 8. 测试与验收

- 后端配置校验覆盖枚举缺失、未知值以及非 Secure 渠道携带该字段。
- 前端测试验证只有选择 Secure 类型才显示分组选择器，切换类型会清除字段。
- 每个 profile 都有精确请求体测试：multipart 字段重复顺序、JSON 数组、素材上限、角色和模型/分辨率约束。
- mock E2E 分别跑三条 Ark 生命周期，并覆盖公开 ID、单查、列表、成功、失败和退款。
- 路由 E2E 创建三个 Secure 渠道，证明请求只能进入能力匹配且分组正确的渠道。
- 真实验收分别使用三枚分组 Key，至少完成特价图生、海外全能参考和企业文生各一条。

## 9. 非目标

- 不实现一条 Secure 渠道保存三枚 Key。
- 不创建三个 Secure 渠道类型。
- 不实现 Ark DELETE、上游任务取消或视频代理下载。
- 不把不支持的首尾帧静默降级为普通参考图。

