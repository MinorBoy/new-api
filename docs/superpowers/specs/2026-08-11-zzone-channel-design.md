# ZZone 渠道接入设计

## 1. 背景与目标

接入 `zzone.cc.cd` 视频任务 API，使 new-api 用户继续通过现有 Ark SDK 任务接口提交、查询和获取视频结果，不需要为 ZZone 修改客户端。由于当前没有可用的 ZZone API Key，本阶段只完成 Mock HTTP 契约测试和代码级验收；渠道默认保持禁用，不宣称真实上游验收通过。

目标包括：

- 新增独立的 ZZone task-only 渠道类型，复用现有异步视频任务、计费和公开任务 ID 能力。
- 把 Ark 请求稳定映射为 ZZone 的 `POST /v1/videos` 请求。
- 通过 `GET /v1/videos/{task_id}` 轮询状态，并通过 `/content` 代理获取完成的视频。
- 在请求入口拦截 ZZone 不支持或超限的字段，避免静默丢参或把不兼容参数传给上游。
- 让管理端能识别该渠道为 task-only、受管 Base URL 且默认 disabled；模型目录继续来自收录表/渠道模板/配置导入，不从 API 文档示例硬编码。

## 2. 协议事实与不确定性

协议事实来源为 `docs/new-channels/cn-zzone.html`。该文档给出的接口和字段如下：

| 能力 | 协议事实 | 本阶段处理 |
| --- | --- | --- |
| Base URL | `https://zzone.cc.cd` | profile 默认值；管理端允许受管 Base URL 覆盖 |
| 鉴权 | `Authorization: Bearer <key>` | 每次提交、查询和内容下载均带上游渠道 Key |
| 创建 | `POST /v1/videos` | JSON 请求，创建响应保存私有任务 ID |
| 查询 | `GET /v1/videos/{task_id}` | 轮询共享任务 adaptor |
| 内容 | `GET /v1/videos/{task_id}/content` | 由本地公开任务 ID 的 `/content` 路由代理 |
| 输入 | `model`、`prompt`、`seconds`、`aspect_ratio`、`images`、`videos`、`audios` | 使用 ZZone 专属 request dialect |
| 媒体限制 | 图片最多 4 张，视频最多 3 个，音频最多 1 个；仅公网 URL | 入口校验并拒绝本地路径/非 `http`/`https` scheme |
| 比例 | `16:9`、`9:16`、`1:1` | 只接受这三个值 |
| 时长 | 文档示例为字符串 `"5"`、`"10"`、`"15"`，未声明完整枚举 | 复用现有任务时长上限，转换为十进制字符串，不擅自锁死枚举 |
| 创建响应 | `id/object/model/status/progress/created_at/seconds/completed_at/expires_at/size/error/metadata` | 复用共享响应投影；不向客户端暴露私有字段 |
| 查询响应 | `id/object/model/status/progress/created_at/seconds` | 轮询时只依赖共享状态和错误解析 |
| 错误 | 400 为 OpenAI 风格 `error{type,message,param,code}`；查询不存在为 404 `resource_not_found` | 映射为现有任务服务错误 |
| DELETE、流式、resolution、seed、watermark、generate_audio | 文档未声明 | 不在 ZZone profile 中提供 |

文档没有给出 `status` 的正式枚举或成功/失败终态样例。实现先按共享 `newapivideo` 解析器已有集合建立 Mock 契约：`queued/pending`、`in_progress/running/processing`、`success/succeeded/completed`、`failure/failed/error/canceled/cancelled/expired`。集合以外的状态必须返回明确的未知状态错误，并在真实 Canary 时复核。

文档示例中的 `video-ds-*` 仅用于说明调用格式，不是生产模型目录来源。生产模型绑定只接受 `sd收录.xlsx`、渠道模板或配置导入快照。

## 3. 方案与选型

### 方案 A：独立 ZZone task-only 类型（选用）

新增 ZZone channel type 和 profile，复用 `relay/channel/task/newapivideo` 的共享任务生命周期、Ark 转换、计费、轮询和隐私投影；新增 ZZone request dialect，以及 `controller/video_proxy.go` 的内容下载分支。

优点是专属字段边界、Base URL、默认禁用和验收门禁都能被代码表达；ZZone 的变化不会污染其他通用 NewAPIVideo profile。代价是需要新增一个 channel type 和少量管理端/i18n 映射。

### 方案 B：复用 `ChannelTypeNewAPIVideo`

改动较少，但无法可靠区分 ZZone 的字段限制、默认地址和 `/content` 私有下载语义；通用 profile 也容易允许上游不支持的字段。不选用。

### 方案 C：完全独立 adaptor

协议隔离最强，但会重复异步任务状态、Ark 结果转换、计费和日志逻辑，增加长期维护面。不选用。

## 4. 组件与职责边界

### 4.1 渠道类型与 profile

- 在 `constant` 中分配下一个保留渠道类型 `212`，并将 `ChannelTypeDummy` 顺延为 `213`，同步更新常量/映射测试。
- 增加 ZZone 名称、Base URL、API 类型和 task platform 注册；不要把 ZZone 加入 `streamSupportedChannels`。
- profile 声明 `https://zzone.cc.cd`、Bearer 鉴权、JSON 请求和 ZZone request dialect。
- 渠道新建默认 `disabled`；管理端可编辑 Key 和受管 Base URL，但不会把模型示例自动写入模型目录。

### 4.2 ZZone request dialect

复用现有 Ark 请求解析状态，把文本和参考媒体转换为：

```json
{
  "model": "<imported-model>",
  "prompt": "<text>",
  "seconds": "<decimal-seconds>",
  "aspect_ratio": "16:9",
  "images": ["https://..."],
  "videos": ["https://..."],
  "audios": ["https://..."]
}
```

可选字段使用指针/可省略表示，缺省字段不序列化，显式的零值或 `false`（若上游未来支持且 profile 明确声明）不能因 `omitempty` 丢失。当前 ZZone 协议没有可发送的布尔可选字段；`generate_audio`、`watermark`、`seed` 等显式输入直接返回 400。

媒体 URL 必须经过现有公网 URL/SSRF 策略；只做 `http`/`https` 校验不足以允许本地或内网地址。数量限制在 adaptor 入口再次执行，防止绕过 Ark 标准 DTO 的路径进入计费或上游请求。

### 4.3 任务生命周期与结果

- 创建阶段调用 `POST /v1/videos`，抽取 `id`（必要时兼容共享解析器支持的任务 ID 字段）并写入任务私有数据。
- 轮询阶段使用私有任务 ID调用 `GET /v1/videos/{id}`，映射共享状态、进度和错误。
- 成功状态不要求上游查询响应提供公开结果 URL；任务记录保留私有 ID，公开响应只返回 Ark 允许的字段。
- `VideoProxy` 针对 ZZone 构造 `/v1/videos/{private_id}/content`，设置 Bearer 鉴权，复用已有超时、SSRF/媒体类型和响应头安全策略。错误只返回通用服务错误，不回显 Key 或私有 URL。
- 文档未声明 DELETE，因此不在 Ark 路由中宣称或调用删除能力。

### 4.4 计费与模型目录

ZZone 不在 adaptor 内实现计费公式。沿用现有 Seedance/视频任务 `EstimateBilling`、预扣、结算、退款和任务超时链路，模型成本与利润来自配置导入。任何时长/数量进入 quota 前仍使用集中式上限校验和 `common` quota 饱和转换，必要时记录 `quota_saturation` 审计标记。

模型模板保留现有 `CH-ZZONE` 转换规则；实现不得添加 `video-ds-2.0` 等 HTML 示例模型的静态生产列表。没有导入快照时，渠道可以创建但不应路由到未绑定模型。

### 4.5 管理端与 i18n

管理端新增 ZZone 渠道类型显示、task-only 能力提示、Base URL 默认值和 disabled 默认值。所有新增用户可见文案使用 i18next 英文 key，并补齐 `en/zh/zh-TW/fr/ru/ja/vi`；不在组件中写裸中文/英文提示。前端不新增客户端调用协议，只补表单枚举、校验和能力展示。

## 5. Mock 契约测试设计

使用 `httptest.Server` 或现有 provider mock 设施，不访问 `zzone.cc.cd`。

### 5.1 profile 与请求编码

- 断言默认 Base URL、Bearer header、JSON `Content-Type` 和超时配置。
- 文本请求断言 `model/prompt` 和 `duration -> seconds`、`ratio -> aspect_ratio` 映射。
- 图片 4 张、视频 3 个、音频 1 个的边界通过；超限、空 URL、本地路径、`data:`/其他 scheme 失败。
- 缺省时长/比例/媒体数组被省略；显式值完整保留。
- `resolution`、`seed`、`watermark`、`generate_audio` 等未声明字段返回 400，且不会发出上游请求。

### 5.2 生命周期与错误

- Mock `POST /v1/videos` 返回创建体，验证私有 ID 保存和 Ark 公开任务结构。
- Mock 查询依次返回排队、执行、成功，验证状态和进度投影。
- 覆盖每个失败状态、未知状态，以及 400/401/403/404/429/500 上游响应的服务错误映射。
- Mock `/content` 验证本地公开任务 ID 能下载视频、带 Bearer header、保留安全媒体响应头，并确认私有 ID/Key 不出现在客户端 JSON 或日志断言中。
- 覆盖上游超时、断连、错误 `Content-Type` 和内容代理失败。

### 5.3 Ark 路由、计费和配置

- 通过现有 `/api/v3/contents/generations/tasks` 提交，随后调用列表、单任务查询和内容代理，验证完整生命周期；不新增客户端专用路由。
- 使用现有任务计费 fixture 验证预扣、成功结算、失败退款、超时处理和 quota 饱和保护仍由共享链路负责。
- 验证 ZZone 在任务 adaptor 注册表可发现、未加入 stream channel 集合、默认渠道状态为 disabled。
- 验证配置导入/模型模板能绑定 `CH-ZZONE`，而未导入模型时不会自动生成 HTML 示例模型。

## 6. 验收、风险与回滚

### 必须通过

- ZZone 专属 provider、`relay/channel/task/newapivideo`、Ark 路由/任务生命周期和内容代理 Mock 契约测试。
- 相关计费安全测试、常量/注册表测试、前端类型检查和 `bun run build`（如构建环境可用）。
- `git diff --check`，无 API Key、无无关文件、无静态示例模型目录。

### 明确未完成

- 当前无 ZZone Key，未执行真实提交、轮询、内容下载或真实限流/鉴权验收；不得把 Mock 通过描述为上游可用。
- `status` 别名集合和上游是否接受全部公网 URL 类型，需 Canary 时确认；若不一致，优先收紧 profile 与测试。
- 基线 `go test ./...` 在根包因缺少未提交 `web/dist` 失败；其他包通过。此问题与 ZZone 无关，后续验证需单独记录前端构建产物是否已生成。

### 回滚

若 Mock 或 Ark 生命周期验证失败，保留渠道类型和 profile 的最小变更可单独回滚；在任何真实启用前，渠道状态保持 disabled，删除 ZZone channel type 注册、profile、内容代理分支和对应管理端映射即可恢复原有渠道行为。不得删除用户已有的 `cn-zzone.html` 文档修改。

## 7. 实施顺序

1. 先写 provider/profile、请求编码、响应状态和内容代理的失败测试。
2. 实现 ZZone profile、channel type、task adaptor 注册和请求/响应映射。
3. 接入公开任务内容代理、管理端枚举和 i18n；补配置导入/模型模板契约测试。
4. 运行 focused Go tests、Ark 生命周期测试、计费安全测试和前端构建；最后运行 `git diff --check`。
5. 生成验收报告，明确 Mock 结果、基线缺失 `web/dist` 的影响和真实 Canary 待办。
