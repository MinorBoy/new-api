# wxart Seedance 渠道接入设计

**日期：** 2026-08-15
**状态：** 已确认方案，待实施

## 目标

新增 wxart Seedance 视频渠道，使调用方继续使用现有 Ark SDK 的
`/api/v3/contents/generations/tasks/*` 完成任务提交、列表和单任务查询，无需了解 wxart 私有请求格式。
首轮只接入 `seedance2.0` 与 `seedance2.5`，不接入同一上游文档中的 Veo、Omni、MiniMax、图片模型
或其他视频模型。渠道默认保持禁用；没有真实 API Key 时只完成 Mock 契约和本地生命周期验证，不宣称
真实上游验收通过。

## 事实来源与优先级

1. 协议事实以 `docs/new-channels/cn-x-deal-api-docs.md` 为准。该文档声明 wxart Base URL、Bearer
   鉴权、提交和查询路径、两个 Seedance 模型的请求字段、素材限制以及查询响应结构。
2. 模型目录、分辨率、成本和导入能力以用户提供的在线 Google 表格 `sd收录` 为准。2026-08-15
   读取结果中，业务渠道编号为 `17`，名称为 `wxart`，Base URL 为 `https://api.wxart.space`。
3. 表格业务渠道编号 `17` 与 new-api 代码渠道类型编号相互独立。代码使用 YSR 保留范围内的下一个
   可用编号 `ChannelTypeWxArt = 215`，并将计数哨兵 `ChannelTypeDummy` 顺延为 `216`。
4. 若协议文档与收录表发生冲突，模型 ID、成本和导入能力使用收录表；HTTP 字段、路径和状态语义
   使用协议文档。真实响应与两者都不一致时，停止启用并先更新契约和设计。

## 方案选择

### 方案 A：复用 `newapivideo` 并增加 wxart profile 与请求方言（选用）

新增独立 task-only 渠道类型，在 `relay/channel/task/newapivideo` 内增加 wxart protocol profile、
请求 DTO、严格验证和失败响应投影。异步任务保存、后台轮询、Ark 公开任务转换、计费结算、失败退款、
错误脱敏和 quota 饱和审计继续复用现有共享能力。

该方案能表达 wxart 的模型白名单、字段命名、Seedance 2.5 素材上限和“失败原因写入
`video_url`”的特殊语义，同时避免复制整套任务生命周期。

### 方案 B：复用通用 `ChannelTypeNewAPIVideo`（不选用）

通用 profile 无法可靠区分 wxart 的默认地址、仅两个模型、`duration` 字段、首尾帧字段、2.5 素材上限
和失败响应语义。使用通用类型会允许未声明字段或错误地把失败原因当成视频 URL。

### 方案 C：新增完全独立 adaptor（不选用）

协议隔离最强，但会重复 Ark 转换、轮询、任务隐私、计费和错误处理代码，增加后续修复成本。
wxart 与现有 `newapivideo` 的提交/查询生命周期足够接近，不需要独立实现。

## 协议与能力矩阵

| 项目 | wxart 约定 | new-api 处理 | 依据 |
| --- | --- | --- | --- |
| 默认地址 | `https://api.wxart.space` | 新渠道默认 Base URL，允许管理员覆盖 | 文档与表格明确 |
| 鉴权 | `Authorization: Bearer <API Key>` | 复用渠道 Key，不记录或返回 Key | 文档明确 |
| 创建 | `POST /v1/videos` | Ark 请求转换为 wxart JSON | 文档明确 |
| 查询 | `GET /v1/videos/{task_id}` | 使用私有上游任务 ID 轮询 | 文档明确 |
| 删除 | 未声明 | 不实现上游删除，不宣称 Ark DELETE 支持 | 文档明确缺失 |
| 流式 | 未声明 | 不加入 `streamSupportedChannels` | 文档明确缺失 |
| 创建响应 | `id`、`status`、`created_at` | 保存上游 `id`，对外只返回公开任务 ID | 文档明确 |
| 查询成功 | `completed`、`progress=100`、`video_url` | 映射为 Ark 成功和公开结果 URL | 文档明确 |
| 查询失败 | 失败原因写入 `video_url` | 作为脱敏错误原因，不作为结果 URL | 文档明确 |
| 文生视频 | 支持 | 一个非空文本内容，无媒体 | 文档明确 |
| 首帧/尾帧 | `first_image`、`last_image` | 由 Ark `first_frame`、`last_frame` 映射 | 文档明确 |
| 参考图片 | `referenceImages` | 由 Ark `reference_image` 映射 | 文档明确 |
| 参考视频 | `referenceVideos` | 由 Ark `reference_video` 映射 | 文档明确 |
| 参考音频 | `referenceAudios` | 由 Ark `reference_audio` 映射 | 文档明确 |
| 媒体地址 | HTTP(S) URL | 只接受通过现有公网 URL/SSRF 校验的 HTTP(S) URL | 文档明确，安全约束 |

调用方继续使用：

- `POST /api/v3/contents/generations/tasks`
- `GET /api/v3/contents/generations/tasks`
- `GET /api/v3/contents/generations/tasks/:task_id`

不增加 wxart 专用客户端路径。wxart adaptor 必须实现现有 `channel.ArkVideoTaskConverter`，并通过
任务平台注册发现。

## 模型目录与成本

在线 `sd收录` 中 wxart 只有两个上游模型 ID：

| 上游模型 | 分辨率 | 计费方式 | 渠道成本 | 生成时长 |
| --- | --- | --- | ---: | --- |
| `seedance2.0` | 480p | 按秒 | 0.27 元/秒 | 4-15 秒 |
| `seedance2.0` | 720p | 按秒 | 0.42 元/秒 | 4-15 秒 |
| `seedance2.0` | 1080p | 按秒 | 0.85 元/秒 | 4-15 秒 |
| `seedance2.0` | 4K | 按秒 | 2.00 元/秒 | 4-15 秒 |
| `seedance2.5` | 480p | 按秒 | 0.35 元/秒 | 4-30 秒 |
| `seedance2.5` | 720p | 按秒 | 0.63 元/秒 | 4-30 秒 |

表格还声明渠道充值汇率 `1:1`、手续费 `3%`、计费倍率 `1`。这些价格和财务参数不写入 adaptor，
而由渠道模板生成器与配置导入流程产生渠道成本、模型映射和利润规则。代码只静态暴露并严格允许
`seedance2.0`、`seedance2.5` 两个模型 ID，不允许管理员误选同一 API 文档中的其他模型。

模板生成规则增加业务渠道 `17 -> CH-WXART`，配置导入增加 `CH-WXART -> 215`。生成的 wxart
渠道保持 disabled，待管理员绑定 Key、审阅价格和完成真实验收后再手动启用。

## Ark 请求映射

wxart 使用专属强类型 DTO，不使用无类型 `map` 拼接请求：

| Ark 字段 | wxart 字段 | 规则 |
| --- | --- | --- |
| 映射后的模型 | `model` | 只能是 `seedance2.0` 或 `seedance2.5` |
| 文本内容 | `prompt` | 必须恰好一个非空文本项 |
| `ratio` | `ratio` | 缺省省略，由上游采用默认值 |
| `duration` | `duration` | 缺省省略，但预扣按文档默认 4 秒计算 |
| `resolution` | `resolution` | 缺省省略；4K 向上游使用文档规定的规范值 |
| `first_frame` | `first_image` | 最多一个公网 HTTP(S) URL |
| `last_frame` | `last_image` | 最多一个，且必须位于首帧之后 |
| `reference_image` | `referenceImages[]` | 保持输入顺序 |
| `reference_video` | `referenceVideos[]` | 保持输入顺序 |
| `reference_audio` | `referenceAudios[]` | 保持输入顺序 |

未带角色的图片按参考图片处理，避免把普通参考图误解释为首帧。首尾帧模式不能与参考图片、视频或
音频混用；尾帧必须同时存在首帧。首尾帧成对出现时，显式 `ratio` 只能为 `Auto`（大小写规范化后
向上游发送协议值）。

wxart 文档未声明 `watermark`、`generate_audio`、`seed`、`callback_url`、`draft`、`tools` 或非默认
`service_tier`，显式传入时返回明确的 `InvalidParameter.*`，不得静默丢弃。

所有可选标量使用指针并配合 `omitempty`。缺省字段从上游 JSON 中省略；显式零值或 `false` 在协议
支持时必须保留。wxart 当前不支持可发送的零值布尔字段，因此相关显式输入直接拒绝。

## 模型级验证

### `seedance2.0`

- `duration` 为 4 至 15 的整数，缺省按 4 秒计费。
- `resolution` 允许 480p、720p、1080p、4K。
- `ratio` 允许 21:9、16:9、4:3、1:1、3:4、9:16、Auto。
- 参考图片最多 9 个、参考视频最多 3 个、参考音频最多 3 个，参考素材总数最多 12 个。
- 单个参考视频为 1.8 至 15 秒，参考视频总时长最多 15 秒。

### `seedance2.5`

- `duration` 为 4 至 30 的整数，缺省按 4 秒计费。
- `resolution` 允许 480p、720p。
- `ratio` 允许 21:9、16:9、4:3、1:1、3:4、9:16、Auto。
- 参考图片最多 30 个、参考视频最多 10 个、参考音频最多 10 个，参考素材总数最多 50 个。
- 单个参考视频和参考视频总时长最多 30 秒。
- 单个参考音频为 2 至 30 秒。

现有共享 Ark 语义校验器把参考媒体固定限制为 9/3/3。实施时把这些限制改成 profile 级配置，
默认值仍为 9/3/3，只有 wxart profile 预解析阶段放宽到 30/10/10；模型映射完成后再按具体模型
收紧并检查总素材数。这样 Seedance 2.5 可以使用文档声明的能力，而其他渠道不会被意外放宽。

参考视频和音频时长使用现有媒体元数据服务解析。元数据不可用时返回明确服务错误，不伪造通过；
解析结果继续受 `relaycommon.MaxTaskDurationSeconds` 和集中式饱和保护约束。

## 任务生命周期、响应与隐私

提交响应从 `id` 抽取上游任务 ID，执行现有多 ID 冲突保护，并保存到任务私有数据。向 Ark 调用方
只返回 new-api 公开任务 ID。

轮询状态至少支持文档示例的 `queued`、`completed`，并复用共享解析器已有的
`pending/in_progress/running/processing/succeeded/failed/error/canceled/cancelled/expired` 映射。
未知状态返回明确解析错误，不能默认为成功或继续无限轮询。

wxart 的特殊失败协议需要专属投影规则：

1. 成功终态下，`video_url` 是结果地址，写入任务结果并返回 Ark `content.video_url`。
2. 失败终态下，`video_url` 是失败原因，写入脱敏后的 `error.message`，公开结果 URL 必须为空。
3. 错误文本继续清理渠道 Key、上游任务 ID、带签名 URL 和其他敏感信息。
4. 对外列表和单任务响应不得出现上游模型 ID、上游任务 ID、渠道 ID、用户 ID、Key、quota 或私有
   轮询数据。

文档未声明独立 `/content` 下载路径，成功响应已返回 `video_url`，因此不增加视频代理分支。

## 计费与安全

wxart 复用现有 Seedance 任务计费链路：请求验证、能力路由、按秒预扣、异步结算、最终失败退款、
成本核算和 quota 饱和审计。

- 缺省 `duration` 的预扣和结算请求事实按 4 秒处理。
- 2.0 和 2.5 的最大时长在进入计费乘数前分别限制为 15 和 30，超限返回 400。
- 素材数量和媒体时长在发送上游前验证，不能通过 Ark content、任务 metadata 或其他透传路径绕过。
- 价格由导入的模型、分辨率和按秒成本规则决定，adaptor 不使用裸 `int` 转换计算 quota。
- 所有 quota 转换继续使用 `common.Quota*Checked`；发生饱和时记录 `relayInfo.QuotaClamp`，并通过
  `attachQuotaSaturation` 写入管理员审计信息。
- 失败、超时和重复终态更新不得产生负扣费、重复退款或重复供应商成本。

## 注册与管理端

- 新增 `ChannelTypeWxArt = 215`、名称 `WxArt`、默认 Base URL `https://api.wxart.space`，并把
  `ChannelTypeDummy` 顺延为 `216`。
- 注册 task adaptor、Ark converter、Seedance 平台白名单、视频路由合同、成本能力和通用渠道测试
  排除项；不加入聊天模型拉取或流式列表。
- 管理端增加 WxArt 类型、NewAPI 图标、默认地址、task-only 标记和默认 disabled 行为。
- 渠道模型选择只提供 `seedance2.0`、`seedance2.5`；配置导入仍可创建同一模型的多分辨率成本和
  路由目标。
- 模板生成器、V1 导入文档和服务端导入 staging 同步识别 `CH-WXART`。
- 所有新增用户可见文案使用 `t()`，并同步 en、zh、zh-TW、fr、ja、ru、vi 七种 locale。

## 测试与验收

实施遵循测试驱动，至少覆盖：

1. 常量与 profile：类型编号、Dummy 顺延、默认 Base URL、两个模型、提交/查询路径、Bearer 请求头、
   task-only 注册和未加入流式列表。
2. 请求编码：文生、首帧、首尾帧、参考图片、参考视频、参考音频和混合参考素材的精确 JSON。
3. 可选字段：缺省 `duration/resolution/ratio` 被省略，缺省时长按 4 秒计费，未支持显式字段返回 400。
4. 2.0 边界：9/3/3、总数 12、4-15 秒、四种分辨率和参考视频时长。
5. 2.5 边界：30/10/10、总数 50、4-30 秒、两种分辨率和视频/音频时长。
6. 安全：拒绝 data URL、本地文件、环回/私网 URL、空 URL、角色冲突、首尾帧和参考素材混用。
7. 响应：创建 `id`、排队、处理中、成功 URL、失败时 `video_url` 错误原因、未知状态、400、401、
   403、429 和 5xx 映射。
8. Ark 生命周期：提交后通过列表和单任务查询获取排队、成功或失败结构，且不泄露私有字段。
9. 计费：按秒预扣、成功结算、失败退款、缺省 4 秒、30 秒上界和 quota 饱和审计。
10. 配置与前端：`17 -> CH-WXART -> 215`、六条成本能力、两模型目录、默认地址、task-only 和
    默认禁用行为。

验证顺序为 wxart provider 单测、`newapivideo` 包测试、relay/Ark 生命周期、计费和配置导入测试、
前端 focused tests、类型检查与构建，最后运行 `git diff --check`。有真实 Key 时再执行文本、首尾帧、
多模态参考、完整轮询和账务核对；没有 Key 时验收报告必须明确真实 Canary 未执行。

## 风险、停止条件与回滚

- 文档只展示 `queued` 和 `completed` 响应，失败状态名及 HTTP 错误体没有完整样例。Mock 使用共享状态
  集合，真实 Canary 发现差异时先补失败测试再调整。
- 失败原因放在 `video_url` 是非标准协议，必须按终态分支解析；任何情况下都不能把失败文本作为可
  下载 URL 暴露。
- 收录表中 2.0 的比例集合比协议文档更窄。provider 校验遵守协议上限，正式路由由导入的模型能力
  合同进一步收紧；不能把某一成本行的比例扩大到所有路由目标。
- 远程媒体时长和可访问性可能在请求后变化。元数据验证减少明显错误，但真实上游仍可能拒绝资源。
- 无真实 Key、成本导入未审阅、失败语义未通过 Canary 或账务结算不一致时，渠道必须保持 disabled。

回滚时禁用或删除 WxArt 渠道实例，并移除类型注册、profile、请求方言和管理端映射。新类型没有旧数据
迁移；若已有任务正在轮询，必须先等待终态或保留 adaptor，不能直接删除平台解析能力。
