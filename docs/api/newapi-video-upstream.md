# 接入上游 new-api 视频任务协议（设计文档）

> 状态：设计已确认，待实现计划
> 日期：2026-07-22
> 关联分支：`docs/ark-native-compat-plans`

## 1. 背景与目标

目标上游是另一个 new-api 服务。实测确认它通过以下端点提供视频生成：

- 提交：`POST /v1/video/generations`
- 详细查询：`GET /v1/video/generations/:task_id`
- 鉴权：`Authorization: Bearer <API Key>`

本仓库当前没有能够同时匹配这两个上游端点和响应结构的任务渠道。现有 Sora、Doubao、Dimensio 等适配器分别面向其他协议，不能通过只修改 Base URL 完成本次接入。

首版目标：

1. 新增独立的 `NewAPIVideo` 任务渠道。
2. 本地客户端通过 `POST /v1/video/generations` 提交任务。
3. 本地客户端通过 `GET /v1/video/generations/:public_task_id` 查询任务。
4. 提交和查询均直接返回本项目已有的 `dto.OpenAIVideo` 结构，不使用 `code/data` 包装。
5. 客户端只看到本地公开任务 ID；上游任务 ID 只用于服务内部通信。
6. 提交请求保留客户端的全部顶层 JSON 字段，仅替换映射后的 `model`。
7. 后台使用上游详细查询端点，完整保留响应数据供状态、结果、计费和诊断使用。

## 2. 实测上游协议

原设计假定上游使用 `dto.VideoRequest`、`dto.VideoResponse` 和 `dto.VideoTaskResponse`。测试报告证明该假定不成立，这三个 DTO 不作为本次接入契约。

### 2.1 提交请求

```http
POST /v1/video/generations HTTP/1.1
Content-Type: application/json
Accept: application/json
Authorization: Bearer <API_KEY>
```

```json
{
  "model": "seedance-720p-token",
  "prompt": "生成一段视频",
  "ratio": "16:9",
  "duration": 5,
  "watermark": false
}
```

上游会读取顶层 `prompt`，不读取 Chat Completions 风格的 `messages`。`ratio`、`watermark` 等字段不在当前 `TaskSubmitReq` 或 `dto.VideoRequest` 的完整可表达范围内，因此不能经过这两个结构重建请求。

### 2.2 提交响应

上游实际返回 `OpenAIVideo` 结构：

```json
{
  "id": "task_upstream_id",
  "task_id": "task_upstream_id",
  "object": "video",
  "model": "seedance-720p-token",
  "status": "queued",
  "progress": 0,
  "created_at": 1784728184
}
```

### 2.3 详细轮询响应

```http
GET /v1/video/generations/task_upstream_id HTTP/1.1
Accept: application/json
Authorization: Bearer <API_KEY>
```

响应是 `TaskResponse<TaskDto>`，不是 `OpenAIVideo`：

```json
{
  "code": "success",
  "message": "",
  "data": {
    "task_id": "task_upstream_id",
    "status": "SUCCESS",
    "fail_reason": "",
    "result_url": "https://example.com/video.mp4",
    "submit_time": 1784728184,
    "start_time": 1784728195,
    "finish_time": 1784728390,
    "progress": "100%",
    "properties": {
      "upstream_model_name": "doubao-seedance-2.0",
      "origin_model_name": "seedance-720p-token"
    },
    "data": {
      "content": {
        "video_url": "https://example.com/video.mp4"
      },
      "created_at": 1784728184,
      "draft": false,
      "duration": 5,
      "execution_expires_after": 172800,
      "framespersecond": 24,
      "generate_audio": true,
      "id": "cgt-provider-id",
      "model": "doubao-seedance-2.0",
      "priority": 0,
      "ratio": "16:9",
      "resolution": "720p",
      "seed": 47347,
      "service_tier": "default",
      "status": "succeeded",
      "updated_at": 1784728379,
      "usage": {
        "completion_tokens": 108900,
        "total_tokens": 108900
      }
    }
  }
}
```

上游也提供 `GET /v1/videos/:task_id`，但该端点只返回标准 `OpenAIVideo`，信息少于详细查询端点。首版后台轮询明确使用 `/v1/video/generations/:task_id`。

## 3. 首版范围

| 能力 | 首版状态 |
|---|---|
| 本地 `POST /v1/video/generations` | 支持 |
| 本地 `GET /v1/video/generations/:task_id` | 支持 |
| 上游 `POST /v1/video/generations` | 支持 |
| 上游 `GET /v1/video/generations/:task_id` | 支持 |
| `application/json` | 支持 |
| multipart | 不支持 |
| `POST /v1/videos` | 不支持 |
| `GET /v1/videos/:task_id` | 不作为首版契约 |
| ARK `/api/v3/contents/generations/tasks*` | 不支持 |
| remix | 不支持 |

## 4. 总体架构

新增 `ChannelTypeNewAPIVideo = 60` 和独立的 `relay/channel/task/newapivideo` 适配器。该渠道是纯任务渠道，不注册通用 API type。

```text
客户端 POST /v1/video/generations
  -> Distributor 按客户端模型选择 NewAPIVideo 渠道
  -> 校验 JSON 和计费边界
  -> 保留全部顶层字段，仅替换 model
  -> POST {baseURL}/v1/video/generations
  -> 提取上游任务 ID
  -> 返回使用本地公开 ID 的 OpenAIVideo

后台任务轮询
  -> GET {baseURL}/v1/video/generations/{upstream_task_id}
  -> 完整保存 TaskResponse<TaskDto>
  -> 提取状态、进度、URL、错误和 usage
  -> 更新本地任务并完成结算或退款

客户端 GET /v1/video/generations/{public_task_id}
  -> 按用户和公开 ID 查询本地任务
  -> NewAPIVideo.OpenAIVideoConverter
  -> 返回使用本地公开 ID 的 OpenAIVideo
```

客户端查询只读取本地任务记录，不在请求线程中实时访问上游。网络波动由后台轮询统一处理。

## 5. 渠道注册与配置

### 5.1 后端

- `ChannelTypeNewAPIVideo = 60`。
- `ChannelTypeDummy` 后移到 `61`。
- `ChannelBaseURLs[60] = ""`，不内置第三方地址。
- `ChannelTypeNames[60] = "NewAPIVideo"`。
- `GetTaskAdaptor` 为渠道类型 60 返回新适配器。
- `ChannelType2APIType` 保持不映射，避免将任务模型暴露为通用聊天模型。
- 通用渠道测试将类型 60 标记为不支持，避免错误调用聊天端点。

### 5.2 管理员配置

- Base URL 填上游协议根地址，例如 `https://upstream.example.com`。
- 适配器只去掉 Base URL 末尾的 `/`，随后追加固定协议路径。
- 不提供虚假的默认模型 `newapi-video`。
- 管理员手工填写客户端模型名，并通过现有模型映射配置上游模型名。

### 5.3 默认前端

- `CHANNEL_TYPES` 和显示顺序加入类型 60。
- `CHANNEL_TYPE_CONFIGS` 使用 new-api 图标、空默认 Base URL和空默认模型列表。
- `channel-utils.ts` 为类型 60 注册 `NewAPI` 图标，避免回退为错误图标。
- 类型 60 加入不支持通用渠道测试的集合。
- 新增提示文本必须同步 en、zh、fr、ru、ja、vi。

## 6. 请求验证与透传

### 6.1 Content-Type 和 JSON 形状

- 只接受 `application/json`，允许 `application/json; charset=utf-8`。
- 不支持的媒体类型返回 HTTP 415。
- 损坏 JSON、数组根节点、标量根节点返回 HTTP 400。
- 所有 JSON 编解码必须使用 `common.*` 包装方法。

### 6.2 保留请求语义

适配器读取原始 JSON 对象并保留所有顶层字段，只覆盖顶层 `model`。这里的“原样透传”表示字段和值的 JSON 语义不变，不保证空白、缩进和字段顺序不变。

以下显式值必须保留：

- `watermark: false`
- `seed: 0`
- 其他合法的零值、布尔值、数组、对象和未知扩展字段

不得使用 `TaskSubmitReq -> dto.VideoRequest` 转换，因为它会丢失字段，并且会把小数 `duration` 静默转成零值。

### 6.3 验证规则

| 字段 | 首版规则 |
|---|---|
| `model` | 必填、非空字符串 |
| `prompt` | 必填、非空字符串 |
| `duration` | 可省略；存在时必须是有限数值，`> 0` 且不超过 `relaycommon.MaxTaskDurationSeconds`；非 `per_duration` 模式允许小数 |
| `n` | 可省略；存在时必须是整数 `1` |
| `watermark` | 存在时必须是布尔值 |
| `seed` | 存在时必须是整数，显式 `0` 合法 |
| 其他字段 | 不解释、不修改，由上游协议校验 |

`metadata.duration` 和 `metadata.n` 也必须检查，防止通过 metadata 绕过计费边界。若 metadata 中的值缺少对应顶层字段，或与顶层值冲突，则返回 HTTP 400；首版只认顶层字段为权威计费来源。

首版限制 `n = 1`，因为本地任务和 `OpenAIVideo` 只表达一个视频，当前结算不能安全表示一次任务的多个独立视频结果。

## 7. 提交流程

### 7.1 上游请求

- URL：`POST {baseURL}/v1/video/generations`
- Header：`Authorization: Bearer {apiKey}`
- Header：`Content-Type: application/json`
- Header：`Accept: application/json`
- Body：客户端顶层 JSON 字段加映射后的 `model`

### 7.2 上游响应

提交响应解析为 `dto.OpenAIVideo`：

1. 优先读取 `task_id`，兼容只返回 `id` 的上游。
2. `id` 和 `task_id` 同时存在但不一致时，按无效上游响应处理。
3. 两者均为空时返回无效上游响应错误，不创建本地任务。
4. 上游任务 ID 返回给任务框架，并以 `Task.PrivateData.UpstreamTaskID` 作为后续通信的权威索引；完整内部响应中可以保留其审计副本。
5. 客户端响应中的 `id` 和 `task_id` 都替换为 `info.PublicTaskID`。
6. 客户端响应中的 `model` 替换为 `info.OriginModelName`。
7. `object` 固定为 `video`，状态、进度和创建时间保留上游已验证语义。

提交成功响应示例：

```json
{
  "id": "task_local_public_id",
  "task_id": "task_local_public_id",
  "object": "video",
  "model": "seedance-720p-token",
  "status": "queued",
  "progress": 0,
  "created_at": 1784728184
}
```

## 8. 轮询与完整数据保留

### 8.1 双轨处理

轮询响应必须采用“解析投影 + 完整存储”方式：

1. 将原始响应体完整保存到 `task.Data`。
2. 同一份响应只读解析为明确投影，用于提取任务状态、进度、URL、错误、时间和 usage。
3. 不得将精简 DTO 重新序列化后覆盖 `task.Data`。

投影中的嵌套 `data.data` 使用 `json.RawMessage` 或等价的保留方式。`draft: false`、`seed`、`duration`、`resolution`、`usage` 和上游未来增加的未知字段都必须保留。允许安全层对巨大的内嵌 Base64 媒体执行现有截断策略，但不能删除普通 JSON 字段。

完整响应是内部数据，不作为客户端响应直接返回。客户端输出必须通过明确的 `OpenAIVideo` 字段白名单构造。

### 8.2 状态映射

外层 `data.status` 是权威状态：

| 上游 `data.status` | 本地状态 | 客户端状态 |
|---|---|---|
| `NOT_START` | `NOT_START` | `queued` |
| `SUBMITTED` | `SUBMITTED` | `queued` |
| `QUEUED` | `QUEUED` | `queued` |
| `IN_PROGRESS` | `IN_PROGRESS` | `in_progress` |
| `SUCCESS` | `SUCCESS` | `completed` |
| `FAILURE` | `FAILURE` | `failed` |

未知状态不得默认映射为成功、失败或处理中。本轮轮询返回解析错误并保持任务原状态，等待后续轮询或系统超时处理。

### 8.3 结果和错误

视频 URL 的读取顺序：

1. `data.result_url`
2. `data.data.content.video_url`

`SUCCESS` 但两个位置都没有 URL 时，不立即标记成功；保持原状态并等待下一轮，避免产生无法下载的视频任务。

失败原因的读取顺序：

1. `data.fail_reason`
2. `data.data.error.message`
3. 上游错误 envelope 的 `message`
4. 统一回退文本 `task failed`

错误码优先读取 `data.data.error.code`。

### 8.4 HTTP 和网络错误

后台轮询需要一个可选的 HTTP 错误解析能力，使轮询服务能同时看到响应状态码和响应体：

| 情况 | 行为 |
|---|---|
| 网络错误、HTTP 429、HTTP 5xx | 临时错误，不修改任务，后续继续轮询 |
| HTTP 404/410 | 标记失败，记录任务不存在或已过期 |
| 其他确定性 HTTP 4xx | 标记失败并保留上游错误 |
| HTTP 2xx 但 JSON 损坏、缺少 `data`、未知状态 | 不修改任务，记录解析错误并继续轮询 |
| 达到系统任务超时 | 使用现有超时和退款机制 |

现有 `service/task_polling.go` 对 new-api wrapper 的通用解析必须改用包含 `result_url` 的明确 DTO。当前直接解码为 `model.Task` 会忽略 JSON 中的 `result_url`，可能错误生成本地代理占位 URL。

## 9. 本地查询响应

`GET /v1/video/generations/:public_task_id` 直接返回 `dto.OpenAIVideo`，不带 `code/data` 包装。

完成示例：

```json
{
  "id": "task_local_public_id",
  "task_id": "task_local_public_id",
  "object": "video",
  "model": "seedance-720p-token",
  "status": "completed",
  "progress": 100,
  "created_at": 1784728184,
  "completed_at": 1784728390,
  "metadata": {
    "url": "https://example.com/video.mp4"
  }
}
```

处理中示例：

```json
{
  "id": "task_local_public_id",
  "task_id": "task_local_public_id",
  "object": "video",
  "model": "seedance-720p-token",
  "status": "in_progress",
  "progress": 50,
  "created_at": 1784728184,
  "metadata": {
    "url": ""
  }
}
```

失败示例：

```json
{
  "id": "task_local_public_id",
  "task_id": "task_local_public_id",
  "object": "video",
  "model": "seedance-720p-token",
  "status": "failed",
  "progress": 100,
  "created_at": 1784728184,
  "completed_at": 1784728390,
  "error": {
    "code": "upstream_error_code",
    "message": "task failed"
  }
}
```

字段来源：

| 字段 | 来源 |
|---|---|
| `id`、`task_id` | 本地公开任务 ID |
| `object` | 固定为 `video` |
| `model` | 客户端提交的本地模型名 |
| `status` | 本地任务状态映射 |
| `progress` | 上游进度，缺失时按状态使用确定性默认值，并限制为 `0..100` |
| `created_at` | 本地任务提交时间 |
| `completed_at` | 仅终态填写本地完成时间 |
| `metadata.url` | 已解析的视频 URL；非成功状态为空字符串 |
| `error` | 已解析的上游错误，回退到本地 `fail_reason` |

`expires_at`、`seconds`、`size` 和 `remixed_from_video_id` 在没有可靠来源时省略。处理中不复制上游错误提前填充的完成时间。

## 10. 计费设计

新适配器接入现有计费模式，不自行定义第三方价格公式。

### 10.1 固定价格

当客户端模型使用固定价格或 `UsePrice` 时，按配置的单次价格预扣和结算。轮询返回的 token 只保留用于诊断，不覆盖固定价格。

### 10.2 按时长计费

适配器实现 `TaskDurationEstimator`：

- `duration` 必填。
- 当前中央接口使用整数秒，因此首版 `per_duration` 模式只接受整数秒。
- 小数时长在固定价格或 token 模式下可以透传；在 `per_duration` 模式下返回 HTTP 400。
- 使用 `relaycommon.MaxTaskDurationSeconds` 上限。
- `duration` 不重复写入 `OtherRatios`。
- 首版不从 `ratio` 或 `resolution` 自动推导价格倍率；价格差异由本地模型和现有配置表达。

### 10.3 token 结算

ratio/token 模式从 `data.data.usage` 获取实际用量：

1. 优先使用存在的 `completion_tokens`，包括显式零值。
2. `completion_tokens` 缺失时回退到 `total_tokens`。
3. 字段缺失与显式零值必须区分。
4. 负数、超大值和转换溢出必须经过现有 quota 饱和保护并记录 `QuotaClamp`。
5. 禁止对未界定数据做裸 `int` 转换。

成功终态进入现有差额结算，失败终态进入现有退款路径。多节点轮询继续使用任务 CAS，避免重复结算或退款。

## 11. 错误响应

提交阶段兼容两种实测错误结构：

```json
{
  "code": "invalid_request",
  "message": "prompt is required",
  "data": null
}
```

```json
{
  "error": {
    "code": "model_not_found",
    "message": "No available channel",
    "type": "new_api_error"
  }
}
```

本地验证错误不重试。上游 429 和 5xx 可以进入现有渠道重试。HTTP 2xx 但缺少任务 ID、ID 冲突或响应无法解析时返回无效上游响应错误。

## 12. 隔离与兼容性

- `Task.PrivateData.UpstreamTaskID` 是上游任务 ID 的权威通信索引；内部完整响应可以包含其审计副本，但客户端响应统一使用本地公开 ID。
- `task.Data` 中的完整轮询响应属于内部数据，不得由通用回退分支直接返回给客户端。
- `/v1/video/generations/:task_id` 为类型 60 增加明确的 `OpenAIVideoConverter` 分支，不再返回通用 `TaskDto`。
- ARK 单任务查询必须与列表使用相同的平台白名单；类型 60 不进入该白名单，避免 `/api/v3/...` 回退暴露原始数据。
- 首版不实现 `ArkVideoTaskConverter`。
- 首版不修改 `dto.VideoRequest`、`dto.VideoResponse` 和 `dto.VideoTaskResponse`，因为它们不是实测协议。
- 不新增数据库字段或迁移；继续使用现有 `Task`、`TaskPrivateData` 和 `TaskBillingContext`。

## 13. 预计改动

### 13.1 后端

| 文件 | 改动 |
|---|---|
| `relay/channel/task/newapivideo/constants.go` | 渠道名称；不提供虚假默认模型 |
| `relay/channel/task/newapivideo/dto.go` | 请求验证投影、详细轮询投影和错误结构 |
| `relay/channel/task/newapivideo/adaptor.go` | 提交、轮询、响应转换、计费接口和错误解析 |
| `relay/relay_adaptor.go` | 注册类型 60 适配器 |
| `constant/channel.go` | 新增类型 60，Dummy 后移到 61 |
| `relay/relay_task.go` | 类型 60 的本地查询转换分支 |
| `service/task_polling.go` | 正确解析 new-api `TaskDto.result_url`、完整保存响应、处理轮询 HTTP 状态 |
| `relay/seedance_task.go` | ARK 单查使用平台白名单，禁止原始数据回退 |
| `controller/channel-test.go` | 类型 60 禁用通用聊天测试 |
| 对应 `_test.go` | 请求、响应、轮询、计费、隔离和生命周期回归测试 |

如轮询 HTTP 错误需要适配器参与分类，在 `service/task_polling.go` 定义最小可选接口，例如 `ParseTaskPollingHTTPError(body, statusCode)`；不扩大所有现有任务适配器的必选接口。

### 13.2 默认前端

| 文件 | 改动 |
|---|---|
| `web/default/src/features/channels/constants.ts` | 类型、顺序和通用测试限制 |
| `web/default/src/features/channels/lib/channel-type-config.ts` | Base URL、图标和配置提示 |
| `web/default/src/features/channels/lib/channel-utils.ts` | 类型 60 的 `NewAPI` 图标 |
| `web/default/src/i18n/locales/{lang}.json` | 六种语言的新增文案 |
| 对应测试 | 渠道类型标签、图标和配置行为 |

## 14. 测试与验收

### 14.1 请求验证

- 仅接受 JSON，拒绝 multipart、数组根节点和损坏 JSON。
- 覆盖缺失/空 `model`、`prompt`。
- 覆盖 `duration` 的缺失、整数、小数、零值、负数、超上限和 `per_duration` 限制。
- 覆盖 `n` 缺失、`1`、零值、负数、小数、超大整数和大于 `1`。
- 覆盖 metadata 计费字段绕过和冲突。
- 断言 `watermark: false`、`seed: 0`、未知字段和嵌套对象保留。
- 断言只修改顶层 `model`。

### 14.2 适配器契约

- 提交方法和路径必须是 `POST /v1/video/generations`。
- 轮询方法和路径必须是 `GET /v1/video/generations/{upstream_task_id}`。
- 验证 Bearer 鉴权、Content-Type、Accept 和任务 ID URL 转义。
- 验证提交响应的 `id`/`task_id` 兼容、冲突和缺失情况。
- 验证两种上游错误 envelope。
- 验证所有客户端响应只包含本地公开 ID 和本地模型名。

### 14.3 轮询和完整数据

- 对所有已知状态做确定性表格测试。
- 验证 URL 和失败原因的回退顺序。
- 验证 `SUCCESS` 无 URL 保持可重试。
- 验证 429/5xx 不修改任务，404/410 和确定性 4xx 进入失败。
- 使用实测完整 `data.data` 样例，断言 `draft: false`、`seed`、`usage` 和未知字段都保留在 `task.Data`。
- 验证 `completion_tokens: 0` 与字段缺失的区别。
- 验证 token 超限进入现有饱和审计。
- 验证 queued、in_progress、completed、failed 四种本地 `OpenAIVideo`。
- 验证任何客户端出口都不包含上游任务 ID。
- 验证类型 60 不能通过 ARK 查询入口读取原始 `task.Data`。

### 14.4 生命周期

使用 mock HTTP server 和明确测试数据覆盖：

```text
提交完整客户端 JSON
  -> mock 上游收到全部字段和映射后的 model
  -> mock 返回上游任务 ID
  -> 本地保存公开 ID 和私有上游 ID
  -> mock 详细轮询返回 TaskResponse<TaskDto>
  -> 本地保存完整响应并更新状态/计费
  -> 本地查询返回 OpenAIVideo
```

### 14.5 验证命令

```powershell
go test ./relay/channel/task/newapivideo/ ./relay/ ./service/ ./constant/
go test ./...
go build ./...

cd web/default
bun run i18n:sync
bun run typecheck
bun run lint
bun run build
```

最终使用真实上游进行一次人工验收，但 API Key 不写入仓库、测试夹具或日志。

## 15. 实现后配置步骤

1. 渠道管理中新建 `NewAPIVideo` 渠道。
2. Base URL 填上游 new-api 根地址。
3. Key 填上游 API Key。
4. 添加本地可用模型，并按需配置上游模型映射。
5. 为本地模型配置固定价格、按时长价格或 ratio/token 计费。
6. 客户端使用本地 `POST /v1/video/generations` 提交，并使用本地 `GET /v1/video/generations/:task_id` 查询。
