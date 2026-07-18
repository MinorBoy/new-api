# ARK Native / Seedance E2E 测试验收报告

## 结论

本轮自动化验收通过。测试全程使用本地 `httptest.Server` 作为 mock ARK Doubao 上游，没有访问真实 ARK endpoint，也没有产生供应商费用。

核心场景是 Seedance 2.0 的四元素多模态请求：参考图、参考视频、参考音频和提示词同时存在。请求从公开 Seedance Native 路由进入，经过鉴权、渠道分发、模型映射、上游提交、任务入库、一次真实轮询和终态结算，再通过公开单查和列表接口验收。

## 测试环境

| 项目 | 值 |
| --- | --- |
| 测试入口 | `e2e/seedance_native_e2e_test.go` |
| 数据库 | SQLite in-memory，真实 GORM 模型和计费表 |
| 上游 | 本地 `httptest.NewServer` |
| 渠道类型 | `ChannelTypeDoubaoVideo`（54） |
| 渠道 Key | `mock-ark-key` |
| 客户端模型 | `doubao-seedance-2-0-260128`，与 ARK 官方 SDK 示例一致 |
| 模型倍率 | `0.1`，仅为测试环境中的 token 结算配置 |
| 轮询 | `disable_task_polling_sleep=true`，`TaskQueryLimit=100` |

## 验收清单

| 验收项 | 结果 | 自动化证据 |
| --- | --- | --- |
| ARK SDK 仅替换 `base_url`，不改 SDK 调用代码 | 通过 | 官方 SDK 5.0.41 实际调用 mock new-api 路由 |
| 用户提交端点为 `/api/v3/contents/generations/tasks` | 通过 | 完整 HTTP E2E；旧 `/seedance/api/v3/...` 返回 404 |
| 提示词 + 2 张参考图 + 参考视频 + 参考音频同时提交 | 通过 | `TestSeedanceNativeSeedance20MultimodalE2E` |
| `content` 顺序、媒体 URL、`role` 和显式标量完整透传 | 通过 | mock 上游请求快照精确断言 |
| 渠道模型映射只替换 `model`，未知字段和显式零值不丢失 | 通过 | `TestBuildNativeRequestBodyAppliesMappedModel` |
| 创建响应符合 `ContentGenerationTaskID` | 通过 | 精确响应仅含公开 `id`；官方 SDK 解析成功 |
| 上游 `cgt-*` ID 不向用户泄露 | 通过 | 创建、单查、列表和失败对象均断言 |
| 官方成功任务的 17 个顶层字段完整返回 | 通过 | 完整 HTTP E2E 逐字段断言；仅 `id` 替换为公开 ID |
| 成功任务轮询、公开单查和列表结构正确 | 通过 | 完整 HTTP E2E + 真实轮询链路；单查与列表 item 精确相等 |
| 视频完成 token 计费、差额结算和看板数据一致 | 通过 | 用户、渠道、token、`quota_data` 精确对账 |
| 纯音频、Fast 1080p 等非法能力在上游前返回 400 | 通过 | mock 请求数保持不变 |
| 合法 ARK 5xx 错误 code/message 和 HTTP 状态保留 | 通过 | `TestSeedanceNativeUpstreamErrorUsesARKEnvelopeE2E` |
| 网络超时返回净化后的 `InternalServiceError` | 通过 | 1 秒真实 HTTP client 超时 E2E |
| 渠道 A 失败后渠道 B 成功，只返回最终成功对象 | 通过 | 双 mock 渠道 HTTP E2E |
| 全渠道失败返回最后渠道的 ARK error envelope | 通过 | HTTP 503 `ServiceUnavailable` 精确断言 |
| 真实 ARK 异步失败对象所有字段完整返回 | 通过 | 用户提供 fixture + 官方 SDK 5.0.41 解析 |
| 异步任务失败只退款一次 | 通过 | 用户、渠道、token、`quota_data` 和退款日志断言 |
| 同步提交失败在 HTTP 返回前完成预扣退款 | 通过 | `TestBillingSessionRefundWaitsForFundingRefund` + 失败 E2E 即时查账 |
| 全仓后端回归和指定静态检查 | 通过 | `go test ./...`、指定范围 `go vet`、`git diff --check` |

## 四元素请求

客户端请求：

```json
{
  "model": "doubao-seedance-2-0-260128",
  "content": [
    {"type":"text","text":"全程使用视频1的第一视角构图，全程使用音频1作为背景音乐。第一人称视角果茶宣传广告。"},
    {"type":"image_url","image_url":{"url":"https://mock.example/reference-image-1.jpg"},"role":"reference_image"},
    {"type":"image_url","image_url":{"url":"https://mock.example/reference-image-2.jpg"},"role":"reference_image"},
    {"type":"video_url","video_url":{"url":"https://mock.example/reference-video.mp4"},"role":"reference_video"},
    {"type":"audio_url","audio_url":{"url":"https://mock.example/reference-audio.mp3"},"role":"reference_audio"}
  ],
  "generate_audio": true,
  "ratio": "16:9",
  "duration": 11,
  "watermark": true
}
```

请求路径：`POST /api/v3/contents/generations/tasks`。

对应 Python SDK 的正确配置是 `base_url="https://<new-api-host>/api/v3"`。SDK 会在该 Base URL 后追加 `contents/generations/tasks`；如果直接使用 HTTP/curl，则完整路径如上。

模型别名和未知字段不是官方 SDK 示例的一部分，因此不再混入主 E2E payload。它们由 `TestBuildNativeRequestBodyAppliesMappedModel` 独立验证：只替换映射后的 `model`，并保留显式零值、`watermark:false` 和未知字段。

## 转换前后

| 字段 | 客户端/转换前 | mock 上游/转换后 |
| --- | --- | --- |
| `model` | `doubao-seedance-2-0-260128` | 原值保留 |
| `content` | 5 项：文本、2 张参考图、参考视频、参考音频 | 5 项，顺序和字段均保留 |
| 媒体角色 | image/video/audio 对应 reference role | 原值保留 |
| `resolution` | 未传 | 不凭空添加；计费默认快照为 `720p` |
| `duration` | `11` | `11` |
| `ratio` | `16:9` | `16:9` |
| `generate_audio` | `true` | `true` |
| `watermark` | `true` | `true` |

mock 实际收到 `/api/v3/contents/generations/tasks`，Authorization 为 `Bearer mock-ark-key`。

网关响应 HTTP 200，严格使用 ARK SDK `ContentGenerationTaskID` 结构；本例仅返回公开 ID：

```json
{
  "id": "task_<public>"
}
```

如果上游创建响应包含官方可选字段（例如 `safety_identifier`），该字段原样保留；只把上游 `cgt-*` ID 替换为 `task_<public>`。响应不包含 `task_id/object/model/status/progress/created_at` 等 OpenAI Video 字段，也不包含 `cgt-mock-seedance-2-0`。旧 `/seedance/api/v3/...` 路径返回 404。

## 入库与隔离

| 字段 | 结果 |
| --- | --- |
| `Task.TaskID` | `task_<random>` |
| `Task.PrivateData.UpstreamTaskID` | `cgt-mock-seedance-2-0` |
| `OriginModelName` | `doubao-seedance-2-0-260128` |
| `UpstreamModelName` | `doubao-seedance-2-0-260128` |
| `HasVideoInput` / `GenerateAudio` | `true` / `true` |
| 提交分辨率 / 服务层级 | `720p` / `default` |
| 提交预扣 quota | `15217` |

提交后单查为 `queued`。另一用户查询同一公开 ID 返回 HTTP 404 和 `task_not_exist`。公开单查和列表只接受公开 ID，不暴露上游任务 ID。

## 官方成功终态响应

```json
{
  "id": "cgt-mock-seedance-2-0",
  "model": "doubao-seedance-2-0-260128",
  "status": "succeeded",
  "content": {
    "video_url": "https://ark-content-generation-cn-beijing.tos-cn-beijing.volces.com/xxx"
  },
  "usage": {
    "completion_tokens": 108900,
    "total_tokens": 108900
  },
  "created_at": 1779348818,
  "updated_at": 1779348874,
  "seed": 78674,
  "resolution": "720p",
  "ratio": "16:9",
  "duration": 5,
  "framespersecond": 24,
  "service_tier": "default",
  "execution_expires_after": 172800,
  "generate_audio": true,
  "draft": false,
  "priority": 0
}
```

该对象来自用户补充的 ARK 官方成功示例，E2E 将同样字段作为 mock 上游终态。轮询断言：`succeeded` 转为内部 `SUCCESS` 和公开 `succeeded`；视频 URL 写入结果；`completion_tokens=108900` 用于结算；公开单查完整保留 `model/status/content/usage/created_at/updated_at/seed/resolution/ratio/duration/framespersecond/service_tier/execution_expires_after/generate_audio/draft/priority`，只把 `id` 从上游 `cgt-*` 替换为公开 `task_*`。任务列表中的 item 与单查对象精确相等。

用户收到的成功任务响应为：

```json
{
  "id": "task_<public>",
  "model": "doubao-seedance-2-0-260128",
  "status": "succeeded",
  "content": {
    "video_url": "https://ark-content-generation-cn-beijing.tos-cn-beijing.volces.com/xxx"
  },
  "usage": {
    "completion_tokens": 108900,
    "total_tokens": 108900
  },
  "created_at": 1779348818,
  "updated_at": 1779348874,
  "seed": 78674,
  "resolution": "720p",
  "ratio": "16:9",
  "duration": 5,
  "framespersecond": 24,
  "service_tier": "default",
  "execution_expires_after": 172800,
  "generate_audio": true,
  "draft": false,
  "priority": 0
}
```

## 计费对账

```text
actual quota = 108900 × 0.1 × 1 × (28/46) = 6628（截断）
pre-consumed quota = 15217
settlement delta = 6628 - 15217 = -8589
```

| 账务事实 | 结果 |
| --- | --- |
| `Task.Quota` | `6628` |
| 用户 `Quota + UsedQuota` | 始终为 `2000000000` |
| 用户 / 渠道 / 令牌 used quota | 均为 `6628` |
| 用户 `RequestCount` | `1`，差额结算未重复计数 |
| `CacheQuotaData.Count` | `1` |
| `CacheQuotaData.Quota` | `6628` |
| `CacheQuotaData.TokenUsed` | `108900` |
| 看板维度 | UserID `1001` / TokenID `1` / ChannelID `1` |

## 错误路径

- 音频单独提交：HTTP 400，`audio input requires an image or video`，未访问 mock 上游。
- Fast `1080p`：HTTP 400，`resolution 1080p is not supported`，未访问 mock 上游。

## 异常、退款与多渠道重试响应结构

以下结构均适用于 ARK SDK 使用的标准入口 `POST /api/v3/contents/generations/tasks`，不引入 `/seedance` 前缀。提交成功和 HTTP 错误发生在创建调用；异步任务失败通过后续 `GET /api/v3/contents/generations/tasks/task_<public>` 以 HTTP 200 返回。两类失败不能混为一种响应。

重试次数由 `RetryTimes` 控制，默认值为 `0`：`0` 表示只请求一次，`1` 表示首次请求后最多再尝试一次。以下需要发生重试的示例均假设 `RetryTimes >= 1`，且没有通过 `specific_channel_id` 锁定渠道。ARK 异步任务提交路径当前会重试 `429`、`307` 和除 `504/524` 外的 `5xx`；`400`、`408`、`504`、`524` 以及本地校验错误不重试。

### 网络超时

客户端仍提交本报告“`四元素请求`”中的完整 payload。mock 上游接收到请求后阻塞到 HTTP client 超时，因此本次尝试没有 HTTP 状态码，也没有 JSON 响应体。

网关将传输错误转换为 HTTP 500。存在剩余重试次数时，当前错误只记录在内部渠道错误日志中，客户端不会收到中间响应；后续尝试成功时，客户端只收到最终成功响应：

```json
{
  "id": "task_<public>"
}
```

所有尝试均发生网络超时后，客户端收到 HTTP 500：

```json
{
  "error": {
    "code": "InternalServiceError",
    "message": "The service encountered an unexpected internal error. Please retry later."
  }
}
```

该结构已通过服务端 HTTP client 的 1 秒超时 E2E 验证。底层的 `context deadline exceeded`、`Client.Timeout`、目标 URL 和鉴权信息不会写入客户端响应；详细传输错误仅进入服务端日志。同步提交最终失败时不会插入任务，请求级退款会在错误响应处理完成前同步退回钱包和 token 预扣额度；E2E 在收到响应后立即查库，额度已恢复到请求前数值。

### 上游 5xx 重试

mock 上游第一次返回 HTTP 503：

```json
{
  "error": {
    "code": "ServiceUnavailable",
    "message": "mock upstream unavailable"
  }
}
```

该响应是渠道内部响应，不直接返回给客户端。后续尝试成功时，mock 上游返回 HTTP 200：

```json
{
  "id": "cgt-mock-retry-success"
}
```

客户端最终只收到 HTTP 200 的公开任务响应，上游 ID 被替换为 `task_<public>`：

```json
{
  "id": "task_<public>"
}
```

如果所有尝试都返回上述 HTTP 503，客户端最终收到 HTTP 503，最后一次上游 ARK `error.code/error.message` 原样返回：

```json
{
  "error": {
    "code": "ServiceUnavailable",
    "message": "mock upstream unavailable"
  }
}
```

最后一次上游状态码会成为客户端 HTTP 状态码。`504` 和 `524` 不重试，但合法 ARK 错误 envelope 仍原样返回；上游返回非 ARK/不可解析错误体时，网关改为 `InternalServiceError`，不会向 SDK 暴露内部 `fail_to_fetch_task`。

### 异步任务失败与退款

任务提交成功时，mock 上游先返回 HTTP 200：

```json
{
  "id": "cgt-20260717171624-cr2n9"
}
```

客户端收到 HTTP 200，且只能看到公开任务 ID：

```json
{
  "id": "task_<public>"
}
```

后续轮询内部上游 ID 时，mock 上游返回用户提供的真实 ARK HTTP 200 任务失败数据：

```json
{
  "id": "cgt-20260717171624-cr2n9",
  "model": "doubao-seedance-2-0-260128",
  "status": "failed",
  "error": {
    "code": "OutputVideoSensitiveContentDetected.PolicyViolation",
    "message": "The request failed because the output video may be related to copyright restrictions. Request id: 02178427978698300000000000000000000ffffac1923a9fc42b8"
  },
  "created_at": 1784279786,
  "updated_at": 1784280145,
  "service_tier": "default",
  "execution_expires_after": 172800,
  "generate_audio": true,
  "draft": false,
  "priority": 0
}
```

轮询器把状态转换为内部 `FAILURE`，把内部 `progress` 设置为 `100%`，并把 `error.message` 写入 `FailReason`。用户随后查询公开 ID，收到 HTTP 200；除 ID 隔离外，官方字段和值完整保留：

```json
{
  "id": "task_<public>",
  "model": "doubao-seedance-2-0-260128",
  "status": "failed",
  "error": {
    "code": "OutputVideoSensitiveContentDetected.PolicyViolation",
    "message": "The request failed because the output video may be related to copyright restrictions. Request id: 02178427978698300000000000000000000ffffac1923a9fc42b8"
  },
  "created_at": 1784279786,
  "updated_at": 1784280145,
  "service_tier": "default",
  "execution_expires_after": 172800,
  "generate_audio": true,
  "draft": false,
  "priority": 0
}
```

`id` 必须替换为 new-api 的公开任务 ID，否则用户无法用返回值继续查询且会暴露渠道内部标识；`created_at/updated_at` 不再被本地数据库时间覆盖。官方 `expired`、`cancelled` 也保持原状态字符串，内部仍按失败终态退款。该失败对象已通过 `volcenginesdkarkruntime` 5.0.41 的真实 `client.content_generation.tasks.get()` 解析链验证。

退款不是另一份 HTTP 响应，而是轮询状态 CAS 更新成功后的账务副作用。以本报告预扣 `15217` quota 为例，退款后应满足：

| 退款验收项 | 退款后结果 |
| --- | --- |
| `Task.Status` / `Task.Progress` | `FAILURE` / `100%` |
| `Task.FailReason` | 完整版权限制错误文案 |
| `Task.Quota` | 保留 `15217` 作为该任务的预扣审计值，不改写为负数 |
| 用户可用 quota | 退回 `15217` |
| 用户 / 渠道 / token used quota | 各自回退 `15217`；完整提交链路下恢复到提交前数值 |
| `quota_data.Count` | 仍为 `1`，退款不重复增减请求次数 |
| `quota_data.Quota` | 从 `15217` 回退为 `0` |
| 退款日志 | `type=refund`、`quota=15217`，`other` 中记录公开 `task_id` 和失败原因 |

同一终态只有 CAS 获胜的轮询实例执行一次退款，重复轮询不会重复退款。

### 多渠道重试

验收配置需要让同一模型存在至少两个可选渠道，并通过优先级或测试桩保证首次选择渠道 A、重试选择渠道 B。渠道 A 返回 HTTP 503：

```json
{
  "error": {
    "code": "ServiceUnavailable",
    "message": "channel A unavailable"
  }
}
```

渠道 B 返回 HTTP 200：

```json
{
  "id": "cgt-channel-b-success"
}
```

用户不会收到渠道 A 的中间错误，只收到渠道 B 成功后的 HTTP 200：

```json
{
  "id": "task_<public>"
}
```

数据库任务绑定最终成功的渠道 B，并只产生一次成功提交和一次预扣记录；`use_channel=[A,B]` 只写入管理员日志，不进入用户响应。

如果渠道 A 和渠道 B 都失败，且渠道 B 的最后响应为 HTTP 503：

```json
{
  "error": {
    "code": "ServiceUnavailable",
    "message": "channel B unavailable"
  }
}
```

客户端最终收到最后一次失败对应的 HTTP 503：

```json
{
  "error": {
    "code": "ServiceUnavailable",
    "message": "channel B unavailable"
  }
}
```

此时不插入异步任务，请求级预扣费在 HTTP 响应完成前退回。指定 `specific_channel_id` 或渠道亲和策略要求停止重试时，不会切换到渠道 B，直接返回渠道 A 对应的最终错误 envelope。上述“渠道 A 失败、B 成功”和“A/B 均失败”均已通过真实 HTTP E2E，不再只是实现推导。

## 命令与结果

```text
go test ./service -run TestBillingSessionRefundWaitsForFundingRefund -count=1 -v
go test ./e2e -run 'TestSeedanceNative.*E2E' -count=3 -v
go test ./relay ./relay/channel/task/doubao ./controller ./service -count=1
go test ./...
go vet ./middleware ./router ./controller ./model ./relay/channel/task/doubao ./relay/channel/openai ./relay/helper ./relay ./service ./e2e
git diff --check
```

结果：全部 `PASS`，`go vet` 和 `git diff --check` 退出码均为 0。Seedance 原生 E2E 共 6 个场景，连续执行 3 轮，共 18 次均通过。

报告内 19 个 `json` 代码块已逐一执行 JSON 解析校验。官方 `volcenginesdkarkruntime==5.0.41` 通过真实 HTTP client 调用本地 mock 路由，创建响应解析为 `ContentGenerationTaskID task_public`，成功查询响应解析为：

```text
ContentGenerationTask task_public succeeded 108900 108900 78674 720p 16:9 5 24 default 172800 True False 0
```

失败查询响应解析为：

```text
ContentGenerationTask task_public failed OutputVideoSensitiveContentDetected.PolicyViolation default 172800 True False 0
```

全量 `go vet ./...` 仍有仓库既有告警，未由本轮引入：`common/custom-event.go` 锁按值传递、`common/email_test.go` IPv6 地址格式以及若干旧 adaptor 的 unreachable code。

## 未覆盖项与残余风险

- 本轮 E2E 只使用 SQLite，未连接真实 MySQL/PostgreSQL。
- 网络超时、上游 5xx、真实任务失败退款、多渠道重试成功及全部失败均已有独立 HTTP E2E；上游仍为本地 mock，不访问真实 ARK。
- Seedream Native 图片完整 HTTP E2E 未在本轮新增；模型字段透传、组图预扣、`generated_images` 和 usage normalization 由现有回归覆盖。
- mock 视频 URL 未下载媒体字节；本轮验收协议转换、状态、权限和计费事实。
