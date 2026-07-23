# CLMM Mall Ark 视频渠道接入设计

## 背景

CLMM Mall 提供异步视频生成接口：

- `POST /v1/videos` 创建任务。
- `GET /v1/videos/{task_id}` 查询任务。

客户端侧要求继续使用火山 Ark SDK 的内容生成任务接口和官方模型名。除把 SDK 的 `base_url` 和 API Key 指向本项目外，现有创建、查询代码不改动。管理员通过渠道模型映射，把官方 Ark 模型名映射为 CLMM Mall 的模型名。

本设计基于 `docs/channel/api-doc-clmm-mall-video-generation.md` 以及用户提供的最新版 `clmm-mall-skill-v3.md`。当两者冲突时，以最新版文档为准；公开任务 ID 隔离、Ark 入站边界和本项目计费安全规则仍由本设计约束。

## 目标

1. 新增独立的 `CLMM Mall` 渠道类型，不改变现有 `Jimeng`、`DoubaoVideo` 或 `Dimensio` 渠道语义。
2. 兼容 Ark SDK 的创建、单任务查询和任务列表接口。
3. 将 Ark 请求转换为 CLMM Mall 请求，并将 CLMM Mall 任务转换回 Ark 响应。
4. 公开任务 ID 与上游任务 ID 隔离。
5. 支持现有按次计费和 `per_duration` 计费，遵守项目额度边界和饱和审计规则。
6. 支持 SQLite、MySQL 和 PostgreSQL，不新增数据库迁移。

## 非目标

- 不把 CLMM Mall 接入现有 `Jimeng` 渠道类型。
- 不新增客户端可见路由。
- 不支持 OpenAI `/v1/videos` 入站协议。
- 不暴露 CLMM Mall 专有的 `bypass_face_check` 和 `grid_strength` 参数。
- 不在本地下载远程参考视频以检测总时长。
- 不为 CLMM Mall 实现会实际创建收费任务的通用渠道测试。

## 方案选择

采用独立渠道类型和独立任务适配器。

没有选择复用 `Jimeng` 渠道类型，因为现有 `Jimeng` 代表火山官方 Visual API 协议：它使用 `AccessKey|SecretKey` 签名、`Action=CVSync2AsyncSubmitTask` 路径、`req_key` 请求体和 POST 查询。CLMM Mall 使用 Bearer Key、`/v1/videos` 路径、另一套请求体和 GET 查询。把两种协议放进同一个类型会要求提交、鉴权、轮询、转换、计费和管理端提示全部按渠道设置分支，同时让历史任务只凭 platform 无法可靠判断响应协议。

也不采用高级自定义渠道。异步轮询、任务 ID 隔离、状态归一化和任务结算无法由静态请求模板完整表达。

## 渠道注册

新增：

```go
ChannelTypeClmmMall = 61
ChannelTypeDummy    = 62
```

渠道属性：

| 属性 | 值 |
|---|---|
| 名称 | `CLMM Mall` |
| 默认 Base URL | `https://clmm-mall.top` |
| Key 格式 | 上游签发的原始 Bearer API Key |
| 入站协议 | 仅 Ark `/api/v3/contents/generations/tasks` |
| 上游协议 | CLMM Mall `/v1/videos` |
| 通用渠道测试 | 禁用 |

该任务渠道不需要新增 `APIType`。它通过 `GetTaskAdaptor` 按 channel type 注册，与现有 Dimensio 任务渠道模式一致。

## 客户端 API

继续使用现有路由：

```text
POST /api/v3/contents/generations/tasks
GET  /api/v3/contents/generations/tasks/{task_id}
GET  /api/v3/contents/generations/tasks
```

典型 Ark SDK 配置只改变连接信息：

```python
client = Ark(
    base_url="https://gateway.example.com/api/v3",
    api_key="new-api-token",
)
```

业务代码继续调用 Ark SDK 的内容生成任务 create/get/list 方法，并继续传官方 Ark 模型名。

## 提交数据流

```text
Ark SDK
  -> SeedanceRequestConvert
  -> VideoRequestPolicy
  -> TokenAuth
  -> Distribute
  -> RelayTaskSubmit
  -> CLMM Mall TaskAdaptor
  -> POST {base_url}/v1/videos
```

具体流程：

1. `SeedanceRequestConvert` 标记 Ark 原生请求并将提交请求导向现有视频任务处理链路。
2. CLMM Mall 适配器只接受带 Ark 原生标记的请求。其他视频入站协议选择到该渠道时返回 400。
3. `ValidateRequestAndSetAction` 解析 Ark 请求，执行协议无关的结构校验，并把规范化请求放入 Gin context。
4. 现有 `ModelMappedHelper` 将客户端官方模型名解析为 CLMM Mall 上游模型名。
5. `ValidateBillingRequest` 在模型映射后执行 CLMM 渠道前缀和控制后缀校验，包括时长、分辨率、最少参考图和禁用参考视频规则。
6. 适配器构造 CLMM Mall 请求，使用 Bearer Key 调用上游。
7. 上游响应中的 `task_id` 或 `id` 作为私有上游 ID 保存。
8. 客户端只收到本项目预生成的公开任务 ID：`{"id":"task_public_xxx"}`。

结构校验和模型校验分为两个阶段，因为 `RelayTaskSubmit` 在适配器首次校验之后才应用渠道模型映射。所有依赖 CLMM 模型名的判断必须使用映射后的 `info.UpstreamModelName`。

## Ark 请求结构

CLMM Mall 适配器定义本渠道私有的 Ark 请求 DTO。可选标量使用指针和 `omitempty`，保证显式 `0`、`false` 与缺省值可区分。JSON 编解码全部使用 `common` 包装函数。

适配器识别以下 Ark 字段：

- `model`
- `content`
- `resolution`
- `ratio`
- `duration`
- `service_tier`
- 官方 Ark 中其他已知但 CLMM Mall 不支持的可选字段，用于显式拒绝而不是静默丢弃

未知顶层字段返回 400。CLMM Mall 专有字段不扩展到 Ark 入站 DTO。

## 请求字段转换

| Ark 入站 | CLMM Mall 上游 | 转换规则 |
|---|---|---|
| `model` | `model` | 使用渠道模型映射后的名称 |
| `content[].text` | `prompt` | 按 content 顺序用换行连接所有非空文本 |
| `content[].image_url.url` | `reference_image_urls[]` | 按 content 顺序收集 |
| `content[].video_url.url` | `reference_videos[]` | 按 content 顺序收集 |
| `ratio` | `aspect_ratio` | 仅 `16:9`、`9:16` |
| `resolution` | `resolution` | 支持 `480p`、`720p`，缺省 `480p`；模型分辨率后缀优先 |
| `ratio + resolution` | `size` | `1280x720` 或 `720x1280` |
| `duration` | `seconds` / `mySeconds` | 无 `-Ns` 时发送 `seconds`；有 `-Ns` 时发送 `seconds="1"` 并按后缀规则设置 `mySeconds` |

图片角色转换：

- `first_frame`、`last_frame` 和 `reference_image` 均降级为普通 `reference_image_urls`。
- 空 role 按 Ark 默认的 `first_frame` 处理，然后同样降级为普通参考图。
- 降级不改变图片在 content 中的相对顺序。

该降级会丢失首尾帧语义，是经确认的兼容策略。设计不声称 CLMM Mall 能保持 Ark 首尾帧行为。

视频只接受 `role=reference_video`。`audio_url`、`draft_task` 和所有 CLMM Mall 无对应语义的 Ark 字段返回 400。

`service_tier` 缺省或为 `default` 时接受但不发送上游；其他值返回 400。

## 请求校验

通用校验：

- `model` 必填。
- 至少存在一个非空文本，转换后的 `prompt` 必填。
- 所有媒体 URL 必须非空；HTTP URL 和 Base64 Data URI 原样透传。
- `ratio` 仅允许 `16:9` 和 `9:16`，缺省为 `16:9`。
- `resolution` 允许 `480p` 和 `720p`，缺省为 `480p`；模型名存在 `-480p/-480P/-720p/-720P` 控制后缀时，该后缀覆盖 Ark `resolution`。
- 参考图全局最多 9 张。
- 参考视频最多 3 个。
- 普通模型时长为 5 到 15 秒，缺省为 5 秒，并且不得超过 `relaycommon.MaxTaskDurationSeconds`。

模型映射后的校验：

- 映射后的模型必须保留最新版定价列表给出的完整渠道前缀。初版识别文档列出的 `sh-`、`grok-`、`veo-`、`bbv3-`、`bbv4-`、`me-`、`hj-`、`mowc-` 和 `op-`；不对白名单前缀后的具体基础模型名做本地枚举。
- `-{n}s` 表示时长上限，`n` 必须为正且不超过统一最大任务时长。Ark 未传 `duration` 时实际时长为 `n`；显式 `duration` 必须为正且不超过 `n`，并作为 `mySeconds` 发送。
- 独立 `-gz` 段表示固定时长，必须与 `-{n}s` 同时存在。请求固定发送 `seconds="1"`、`mySeconds="n"`；显式 Ark `duration` 不改变实际时长或计费时长。
- `-{n}img` 要求至少提供 n 张参考图，并继续受全局 9 张上限约束。
- 独立 `-nv` 段表示上游不接受参考视频；适配器在提交前丢弃 `reference_videos`，但不改变其他媒体顺序。

模型后缀只解析文档定义的分辨率、时长、`gz`、`Nimg` 和 `nv` 控制段。`sr`、`nsp`、`nyp`、`nyy` 等其他文本视为基础模型名的一部分，不自行赋予行为。可用性最终由 CLMM Mall 校验；网关不在每次提交时请求 `/api/pricing`。

远程参考视频总时长 15 秒无法在不下载媒体的情况下可靠判断。本地只验证数量；总时长由 CLMM Mall 上游校验。该值不参与本项目计费乘数。

## 时长后缀模型

普通模型：

```text
Ark duration=8 -> CLMM seconds="8" -> 计费时长 8
```

带上限后缀模型：

```text
mapped model="me-videos-720P-10s"
Ark duration=6
-> CLMM seconds="1", mySeconds="6"
-> 计费时长 6
```

带固定时长后缀模型：

```text
mapped model="op-video-gz-10s"
Ark duration omitted or any valid value
-> CLMM seconds="1", mySeconds="10"
-> 计费时长 10
```

没有 `-{n}s` 后缀的模型按普通模型处理，使用已校验的 Ark `duration` 作为 `seconds` 和计费时长。`-gz` 缺少 `-{n}s` 时直接返回 400。

## 鉴权与上游请求

提交：

```http
POST {base_url}/v1/videos
Authorization: Bearer {channel_key}
Content-Type: application/json
Accept: application/json
```

查询：

```http
GET {base_url}/v1/videos/{escaped_upstream_task_id}
Authorization: Bearer {channel_key}
Accept: application/json
```

任务 ID 必须使用 `url.PathEscape` 拼接。HTTP 客户端继续使用项目代理配置。

## 提交响应

CLMM Mall 提交响应同时兼容 `task_id` 和 `id`：

1. 优先取非空 `task_id`。
2. 回退到非空 `id`。
3. 两者均为空时视为无效上游响应。

上游 ID 只写入 `TaskPrivateData`，不出现在客户端响应、任务列表或错误消息中。

## 轮询和状态映射

| CLMM Mall 状态 | 内部状态 | Ark 状态 |
|---|---|---|
| `queued`, `pending` | queued | `queued` |
| `processing`, `running`, `in_progress` | in progress | `running` |
| `completed`, `succeeded`, `success` | success | `succeeded` |
| `failed`, `error`, `cancelled`, `canceled` | failure | `failed` |

状态比较忽略大小写和首尾空白。

进度值限制在 0 到 100，并转换为项目使用的百分比字符串。上游未返回有效进度时，排队、处理中和终态分别使用稳定默认值。

成功结果 URL 按以下顺序选择第一个非空值：

1. `video_url`
2. `url`
3. `result_url`
4. `metadata.url`

失败原因兼容字符串 `error` 和带 `message`/`code` 的错误对象。未知状态、无效 JSON、临时限流和上游服务异常返回轮询错误，使调度器保留当前任务状态并在后续轮次重试；只有文档定义的失败状态进入终态。

## Ark 查询响应

适配器实现 `channel.ArkVideoTaskConverter`。转换结果包含：

- 公开 `id`
- 客户端原始官方模型名
- Ark 状态
- `content.video_url`（成功时）
- 稳定的 `error.code` 和 `error.message`（失败时）
- 创建和更新时间

`seedanceTaskResponse` 继续以本地任务状态为权威，适配器仅负责转换上游数据。新 platform 加入 Ark 列表查询允许的 platform 集合，列表继续按当前用户、最近七天、模型、状态和 service tier 过滤。

## 错误处理

适配器实现 `TaskErrorParser`：

- 优先读取嵌套或字符串形式的 `error`。
- 回退读取非空 `message` 或 `detail`。
- 无法识别时返回稳定的网关错误，不把完整原始响应作为用户消息。
- 上游 400/422 请求校验错误映射为客户端 400；上游 429 保留限流语义以允许现有重试策略处理。
- 上游 401/403 表示渠道凭据或权限配置错误，映射为 502，不能伪装成客户端提交给本项目的令牌无效。
- 其他上游 4xx 和所有 5xx 映射为稳定的网关错误。
- 日志可记录必要的诊断上下文，但不记录渠道 Key。

Ark 入站请求的错误继续由现有 `respondTaskError` 输出 Ark 错误 envelope。

## 计费

价格配置使用客户端请求的官方 Ark 模型名。模型映射后的 CLMM Mall 名称只作为 upstream model 记录在任务属性和消费日志中。

适配器实现 `TaskDurationEstimator`：

- 普通模型返回已校验的 Ark `duration`。
- 普通 `-{n}s` 模型返回提交时确定的 `mySeconds`。
- `-gz` 固定模型返回后缀中的 `n`，不使用 Ark `duration` 或上游占位值 `1`。

`EstimateBilling` 不增加 resolution ratio。`AdjustBillingOnSubmit` 和 `AdjustBillingOnComplete` 不根据上游 `seconds` 改写费用：上游字段是非权威输入，普通、上限后缀和固定后缀模型的真实计费时长在提交前已经确定。

计费流程继续使用现有实现：

1. 校验所有用户控制的时长边界。
2. `ModelPriceHelperPerCall` 按官方模型名读取价格。
3. `taskDurationQuota` 使用 decimal 和 `common.QuotaFromDecimalChecked` 计算额度。
4. 饱和信息写入 `relayInfo.QuotaClamp`，由现有日志链路注入 `admin_info.quota_saturation`。
5. 预扣额度不足时提交失败，不调用上游。
6. 成功任务保留预扣额度。
7. 失败任务走现有任务全额退款。

不新增裸 `int` 转换，不直接写 `PriceData.OtherRatios`，不把上游 `seconds` 或媒体元数据作为未经边界校验的计费乘数。

## 管理端

默认前端新增 channel type 61：

- 名称：`CLMM Mall`
- 图标：复用已有 `Jimeng` 图标
- 默认地址：`https://clmm-mall.top`
- Key 提示：输入 CLMM Mall 签发的原始 API Key
- 模型提示：渠道 models 填客户端可见的官方 Ark 模型名，并通过 model mapping 映射到 CLMM Mall 模型名
- 警告：该渠道仅支持 Ark `/api/v3` 视频任务 API
- 加入通用渠道测试禁用集合

所有用户可见文案使用英文源字符串作为 i18n key，并同步到仓库当前所有 locale：`en`、`zh`、`zh-TW`、`fr`、`ja`、`ru`、`vi`。

## 文件边界

预计后端文件：

- `constant/channel.go`
- `constant/channel_test.go`
- `relay/relay_adaptor.go`
- `relay/seedance_task.go`
- `relay/relay_task_seedance_test.go`
- `controller/channel-test.go`
- `controller/channel_test_internal_test.go`
- `relay/channel/task/clmmmall/constants.go`
- `relay/channel/task/clmmmall/dto.go`
- `relay/channel/task/clmmmall/translate.go`
- `relay/channel/task/clmmmall/adaptor.go`
- `relay/channel/task/clmmmall/translate_test.go`
- `relay/channel/task/clmmmall/adaptor_test.go`
- `relay/channel/task/clmmmall/e2e_test.go`

预计前端文件：

- `web/default/src/features/channels/constants.ts`
- `web/default/src/features/channels/lib/channel-type-config.ts`
- `web/default/src/features/channels/lib/channel-utils.ts`
- `web/default/src/i18n/locales/*.json`
- 相关 channel config 测试

不修改现有 Dimensio 或 Jimeng 请求 DTO 和适配器，避免无关回归。

## 测试设计

### 转换测试

使用确定性表测试精确断言：

- 文生视频请求。
- `first_frame`、`last_frame`、`reference_image` 降级并保持顺序。
- 多参考视频转换。
- `ratio`、`resolution` 和派生 `size`。
- 普通时长、可缩短的 `-Ns`、固定 `-gz` 模型，以及对应的 `mySeconds`。
- 分辨率后缀覆盖、`-Nimg` 最少图片数和 `-nv` 丢弃参考视频。
- 模型映射后只发送上游模型名。

### 校验测试

- 缺少 model、content 或 prompt。
- 普通时长 4、5、15、16 和超过统一上限。
- `-gz` 缺少时长、后缀超过上限、普通 `-Ns` 的显式 duration 超限，以及 `-gz` 覆盖显式 duration。
- `-Nimg` 数量不足和全局 9/10 张图片边界。
- 3/4 个参考视频。
- 不支持的 audio、draft、service tier 和未知字段。
- 非 Ark 入站路径选择到该渠道时拒绝。

### 响应测试

- 提交响应 `task_id` 和 `id` 回退。
- 所有状态别名。
- 进度边界。
- `video_url`、`url`、`result_url`、`metadata.url` 回退优先级。
- 字符串和对象错误。
- 未知状态保持可重试。
- 客户端响应不包含上游任务 ID。

### 计费测试

- 普通模型使用请求时长。
- 普通 `-Ns` 使用实际 `mySeconds`，`-gz` 使用固定后缀时长而不是 1。
- 超界时长在预扣前拒绝。
- 成功保持预扣、失败全额退款。
- 日志保留 origin/upstream 模型映射信息和现有饱和审计行为。

### 集成测试

使用 `httptest.Server` 验证：

- Bearer 鉴权头和准确提交路径。
- 完整上游 JSON。
- 私有上游 ID 与公开 ID 分离。
- GET 轮询路径包含转义后的上游 ID。
- 成功、失败和可重试响应更新本地任务的行为。
- Ark 单查、列表、用户所有权和过滤。

### 前端测试

- 新渠道名称、默认地址、Key 提示和图标映射。
- 通用渠道测试禁用。
- Base URL 自动填充只在表单未被用户修改时发生。
- locale 完整性、TypeScript 类型检查、lint 和生产构建。

## 验收标准

1. 使用真实 Ark SDK，仅修改 `base_url` 和 API Key 后，原创建和查询代码可以通过本项目调用 CLMM Mall。
2. 客户端继续使用官方 Ark 模型名；管理员模型映射决定 CLMM Mall 上游模型。
3. 文生视频、参考图和参考视频均能提交。
4. 三种 Ark 图片角色按确认策略降级为普通参考图。
5. 创建、查询和列表响应符合 Ark 任务结构，且不泄露上游任务 ID。
6. 所有文档状态均得到稳定映射，临时异常不会误终止任务。
7. 普通、`-Ns` 上限和 `-gz` 固定模型不会使用占位值或未验证输入造成少扣、负扣或溢出。
8. 新渠道可在默认管理端创建和编辑，全部文案具备 i18n。
9. 相关 Go 和前端测试、类型检查、lint、i18n 检查及生产构建通过。

## 风险与控制

### 首尾帧语义降级

CLMM Mall 只有普通参考图数组。适配器按确认策略转换，但不能保证生成结果保留首尾帧约束。管理文档和渠道提示应明确该差异。

### 动态模型列表

CLMM Mall 的具体基础模型会随 `/api/pricing` 变化。网关只验证文档定义的渠道前缀与控制后缀，不枚举 `op-` 等前缀后的具体模型名；不可用模型由上游返回稳定的请求错误。

### 上游错误格式未完整定义

错误解析采用保守兼容策略。未识别错误不进入任务失败终态，避免临时服务响应触发退款和任务终止。

### 上游模型命名变化

模型映射由管理员配置。模型判断只用于最新版文档列出的渠道前缀和控制后缀；具体基础模型是否可用由 CLMM Mall 判定，网关不会猜测未文档化后缀的语义。
