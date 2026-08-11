# FYLink Seedance 渠道接入设计

**日期：** 2026-08-11
**状态：** 已确认方案，待实施

## 目标

新增 FYLink（表格名称为 `fflink`）Seedance 视频渠道，使下游继续使用 Ark SDK 的 `/api/v3/contents/generations/tasks/*` 完成任务提交、查询、取消和视频结果获取，无需使用 FYLink 私有请求格式。渠道默认保持禁用，完成本地契约测试后仍需真实 FYLink 凭据验收才能启用。

## 事实来源与优先级

1. 协议事实以 `C:\Users\880pro\Documents\new-api\docs\new-channels\cn-fflink.html` 和 FYLink 在线文档为准。
2. 模型、分辨率、成本和素材能力以 2026-08-11 在线 Google 表格 `sd收录` 为准。表内业务渠道编号为 `15`，名称为 `fflink`，Base URL 为 `https://api.fflink.top`。
3. 用户已明确要求：下游只能提交可公开访问的 HTTP(S) 素材 URL，new-api 不调用 FYLink `/v1/videos/uploads`，也不接收需要代上传的本地文件、Base64 或 data URL。
4. 表内业务渠道编号 `15` 与 new-api 的代码渠道类型编号相互独立。因 `212`、`213` 已被其他渠道占用，代码分配 `ChannelTypeFFLink = 214`，并把计数哨兵 `ChannelTypeDummy` 顺延到 `215`。

若文档示例模型与在线表格冲突，生产模型目录和计费使用在线表格；协议字段和状态使用 FYLink 文档。真实响应与两者都不一致时，停止启用并先更新设计和契约测试。

## 协议矩阵

| 项目 | FYLink 约定 | new-api 处理 |
|---|---|---|
| 默认地址 | `https://api.fflink.top` | 新增默认 Base URL，允许管理员覆盖 |
| 认证 | `Authorization: Bearer <API Key>` | 使用渠道 Key，不记录或返回 Key |
| 异步声明 | `Prefer: respond-async` | 提交请求固定添加该请求头 |
| 创建 | `POST /v1/videos/generations` | Ark 请求转换为 FYLink JSON |
| 创建响应 | `job_id`、`status`、`status_url` | 保存私有 `job_id`，向用户只返回公开任务 ID |
| 单任务查询 | `GET /v1/videos/jobs/{job_id}` | 后台轮询并投影成 Ark 任务 |
| 取消 | `DELETE /v1/videos/jobs/{job_id}` | 仅允许取消归属当前用户且仍可取消的任务 |
| 成品 | `GET /v1/videos/jobs/{job_id}/content` | 通过 new-api 视频代理转发 MP4 和 Range 请求 |
| 状态 | `pending`、`running`、`settling`、`completed`、`failed`、`canceled` | 映射为排队、运行、运行、成功、失败、失败 |
| 媒体 | 公网 HTTP(S) URL | 本地校验后直接透传，不上传 |
| 流式 | 文档未声明 | 不加入 `streamSupportedChannels` |

FYLink 查询文档没有承诺在 `completed` JSON 中返回成品 URL，成功状态也不应因此被判为无效响应。new-api 在 Ark 成功响应中返回自己的视频代理 URL，代理再携带渠道凭据读取 `/content`。这样既保持 Ark 客户端兼容，也不会暴露 FYLink Key 或上游任务 ID。

## 在线模型与成本

在线 `sd收录` 的有效 FYLink 行位于 `sd!A221:AN226`：

| 上游模型 | 版本 | 分辨率 | 渠道成本 |
|---|---|---:|---:|
| `seedance-2.0` | 标准 | 480p | 0.12 元/秒 |
| `seedance-2.0` | 标准 | 720p | 0.25 元/秒 |
| `seedance-2.0` | 标准 | 1080p | 0.60 元/秒 |
| `seedance-2.0-fast` | fast | 480p | 0.10 元/秒 |
| `seedance-2.0-fast` | fast | 720p | 0.20 元/秒 |
| `seedance-2.0-mini` | mini | 720p | 0.17 元/秒 |

每行均为按秒计费，素材上限为 4 张参考图、3 个参考视频、1 个参考音频、总素材数 8，参考视频总时长上限 15 秒，生成时长为 4 至 15 秒。比例按具体模型和分辨率的导入能力合同约束；不能把某一行的 `auto` 扩大解释为所有模型支持任意比例。

FYLink protocol profile 的 `modelList` 保持空数组，不把上述模型硬编码为静态目录。模型映射、分辨率能力、成本规则和销售定价继续由渠道模板及配置导入流程维护。配置导入映射增加 `CH-FFLINK -> 214`，导入的渠道默认禁用。

## Ark 请求映射

`relay/channel/task/newapivideo` 新增 FYLink 专属方言和请求 DTO，避免用无类型 `map` 拼接协议：

| Ark 字段 | FYLink 字段 | 规则 |
|---|---|---|
| 映射后的模型 | `model` | 发送路由选中的上游模型 ID |
| 文本内容 | `prompt` | 合并现有 Ark 文本语义 |
| `resolution` | `resolution` | 缺省时省略或使用路由能力默认值 |
| `duration` | `duration` | 必须为整数并受模型能力及公共最大值约束 |
| `ratio` | `aspect_ratio` | 按导入能力合同校验 |
| `generate_audio` | `audio` | 使用 `*bool`，显式 `false` 必须保留 |
| `first_frame` | `start_frame_url` | 只允许一个公网 URL |
| `last_frame` | `end_frame_url` | 只允许一个公网 URL，需与首帧规则兼容 |
| `reference_image` | `guidances.image_reference[].image` | `{url, type: "UPLOADED"}`，最多 4 个 |
| `reference_video` | `guidances.video_reference_base[].video` | `{url, type: "UPLOADED"}`，最多 3 个 |
| `reference_audio` | `guidances.audio_reference[].audio` | `{url, type: "UPLOADED"}`，最多 1 个 |

`type: "UPLOADED"` 是 FYLink guidance 对象的协议枚举，不表示 new-api 会执行上传。用户提供的公网 URL 直接写入该对象。

参考图不能与首帧/尾帧混用；参考音频必须同时存在参考图或参考视频。总素材数不得超过 8，参考视频总时长不得超过 15 秒。公网 URL 校验拒绝非 HTTP(S)、data URL、Base64、本地文件地址、环回地址和私网地址。媒体格式、大小和时长最终仍可能由 FYLink 在处理阶段拒绝。

Ark 中 FYLink 未声明支持的显式字段，例如 `watermark`、`seed`、`callback_url`、`draft`、`tools` 和非默认 `service_tier`，返回明确的 `InvalidParameter.*`，不能静默丢弃。所有可选标量使用指针并配合 `omitempty`，保证缺省字段被省略、显式零值或 `false` 不会被误删。

## 任务生命周期与取消

提交响应 DTO 增加 `job_id`，并继续执行多 ID 冲突检查。后台轮询识别 `settling` 为进行中；`completed` 即使没有 JSON 结果 URL也视为成功，并为公开任务写入 new-api 视频代理 URL。失败或取消消息必须清理上游任务 ID、Key 和渠道私有信息。

现有 Ark 路由只提供提交、列表和单任务查询。为满足 FYLink 已声明的取消协议，增加 Ark `DELETE /api/v3/contents/generations/tasks/:task_id`：

1. 使用公开任务 ID 查询任务并校验当前用户所有权。
2. 只有 adaptor 实现可选取消接口时才调用上游；其他渠道返回不支持，不改变现有 GET/POST 行为。
3. FYLink 使用私有上游 `job_id` 请求 `DELETE /v1/videos/jobs/{job_id}`，不得把用户提供的路径片段直接拼接到 URL。
4. 只允许对仍可取消的任务发起操作；重复取消或已完成任务返回稳定错误，避免重复退款或重复结算。
5. 上游确认取消后，通过现有任务结算状态机落失败/取消终态并保证退款只执行一次。

取消会触及共享任务和计费边界，实施时必须用行为测试锁定所有权、状态转换、幂等和退款，不能仅测试 HTTP 方法转发。

## 视频代理安全

`controller.VideoProxy` 增加 FYLink 分支，请求：

`GET {baseURL}/v1/videos/jobs/{escapedUpstreamTaskID}/content`

代理支持 HTTP Range 和上游内容类型，使用现有公共媒体 HTTP client。发生跨域重定向时不得把 `Authorization` 转发给新域名。用户只能用公开任务 ID访问自己的任务；响应、日志和错误中不得出现上游 `job_id` 或渠道 Key。

## 计费与安全

计费沿用现有 Seedance 预扣、异步结算、失败退款和成本核算链路：

- 请求时长先经过 4 至 15 秒的渠道能力校验及 `MaxTaskDurationSeconds` 公共上限。
- 在线表格的六条成本规则由配置导入生成，按模型、分辨率和请求时长结算，不在 adaptor 内写单价。
- 所有 quota 转换继续使用 `common.Quota*Checked`，饱和标记进入 `relayInfo.QuotaClamp` 和管理员日志。
- 取消、失败、轮询超时和重复终态更新不得产生负扣费、重复退款或重复供应商成本。
- 素材 metadata、透传字段和 unsigned 数量均不能绕过现有边界。

## 注册与管理端

- 新增 `ChannelTypeFFLink = 214`、名称 `FYLink`、默认 Base URL `https://api.fflink.top`，`ChannelTypeDummy` 顺延到 215。
- 注册 task-only adaptor、Ark converter、Seedance 平台白名单、视频路由合同、成本能力和渠道测试排除项。
- 管理端增加 FYLink 类型、默认地址、task-only 标记和配置导入映射；不加入聊天模型拉取或流式能力列表。
- 渠道 profile 不提供静态模型列表，管理员通过最新版模板/配置导入创建模型映射和成本规则。
- 新建和切换为 FYLink 的渠道均自动保持 disabled，直到真实验收通过。
- 若新增用户可见提示，必须使用 `t()` 并通过项目 i18n 脚本同步七种 locale。

## 测试与验收

实施遵循测试驱动，至少覆盖：

1. profile 的地址、路径、Bearer 认证、`Prefer` 请求头、空模型目录和注册发现。
2. 文生、首帧、首尾帧、参考图片、参考视频、参考音频和混合素材的精确上游 JSON。
3. 公网 URL 直传且上传接口调用次数为零；data URL、私网 URL、角色冲突、数量、总数、时长和音频依赖本地返回 400。
4. 可选字段缺省省略，显式 `false` 保留，未支持显式字段返回准确错误码。
5. `job_id` 提交响应，`pending -> running -> settling -> completed`，以及 `failed/canceled/unknown` 状态投影。
6. 成功无结果 URL 时使用视频代理，Range 请求可用，重定向不泄漏认证头。
7. Ark 单查、列表和删除只暴露公开任务 ID，验证所有权、幂等、已完成拒绝和退款一次性。
8. 按秒成本预扣、成功结算、失败/取消退款、quota 饱和审计和配置导入的六条模型能力。
9. 管理端类型、默认地址、task-only、导入映射和默认禁用行为。

本地验收顺序为 provider 单测、relay/Ark 生命周期、删除与代理、计费、配置导入、前端测试与构建、`git diff --check`。没有 FYLink API Key 时只报告 mock/契约测试结果，不能宣称真实上游验收通过，也不能启用渠道。真实验收至少覆盖一次文本生成、一次包含公网参考素材的生成、完整状态轮询、成品 Range 下载、取消 pending 任务和账务核对。

## 风险与停止条件

- 用户确认的公网 URL 直传与文档“先上传再引用”的推荐链路存在差异。必须用真实 FYLink 凭据验证该行为；若 FYLink 拒绝非其上传域名的 URL，停止启用并向用户报告，不能自动恢复代上传。
- FYLink `completed` 查询响应未声明成品 URL，必须依赖 `/content` 代理。若真实响应或重定向行为不同，先补契约再调整。
- 在线表格把 `seedance-2.0` 1080p 写为 4 至 15 秒，而文档写明 1080p 最长 12 秒。为避免上游拒绝，协议校验使用更严格的 12 秒上限；配置导入模板需要同步修正或标记冲突，不能把 13 至 15 秒路由到该组合。
- 在线表格未收录 `seedance-2.0-mini` 480p，因此首轮生产配置不得仅凭协议文档生成该组合。
- 无真实 Key、在线表格冲突未解决、删除退款不幂等或代理可能泄漏认证头时，渠道必须保持 disabled。
