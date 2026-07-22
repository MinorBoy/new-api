# 接入上游 new-api 视频任务协议（设计文档）

> 状态：根据上游公开文档修订，待确认
> 日期：2026-07-23
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
4. OpenAI Video 入口的提交和查询均直接返回本项目已有的 `dto.OpenAIVideo` 结构，不使用 `code/data` 包装；ARK 入口返回 ARK task response。
5. 客户端只看到本地公开任务 ID；上游任务 ID 只用于服务内部通信。
6. 提交请求保留客户端的全部顶层 JSON 字段，仅替换映射后的 `model`。
7. 后台使用上游详细查询端点，完整保留响应数据供状态、结果、计费和诊断使用。
8. 本地同时提供 ARK SDK 入口 `POST/GET /api/v3/contents/generations/tasks*`；对上游已明确支持的图片和音频参考做协议转换，并返回包含 token usage 的 ARK 风格任务响应。
9. 未被上游文档或实测证明支持的 ARK 视频参考、`draft_task`、`draft: true` 和非空 `tools` 首版明确拒绝，不静默丢弃。

## 2. 上游协议依据

本设计以两类证据为准：

- 2026-07-22 测试报告：验证文生视频提交、详细轮询响应、状态、结果 URL 和 token usage。
- 上游当前公开文档：<https://new.seeinglab.top/docs/seedance>，验证图片和音频参考的请求字段。

公开文档的查询示例是精简的 `OpenAIVideo`，实测详细查询则返回 `TaskResponse<TaskDto>`。后台轮询必须兼容两种结构，但以实测详细结构作为完整数据和 token 结算的优先来源；不能根据公开文档的精简示例删减 `data.data`。

原设计假定上游使用 `dto.VideoRequest`、`dto.VideoResponse` 和 `dto.VideoTaskResponse`。现有证据证明该假定不成立，这三个 DTO 不作为本次接入契约。

### 2.1 已确认的媒体能力

| 能力 | 上游请求字段 | 证据强度 | 首版处理 |
|---|---|---|---|
| 文生视频 | `prompt` | 已实测 | 支持 |
| 单张首帧 | `image` | 文档标注已实测 | 支持 |
| 首尾帧 | `image_with_roles` | 文档标注已实测 | 支持 |
| 多图/参考图 | `images` | 文档称字段已接入、建议小流量验证 | 支持，但标记为试验能力 |
| 音频参考 | `content[].audio_url` + `generateAudio` | 文档标注已实测 | 支持 |
| 视频参考 | 无 | 未确认 | ARK 入口拒绝 |
| `draft_task` / `draft: true` | 无 | 未确认 | ARK 入口拒绝 |
| 非空 `tools` | 无 | 未确认 | ARK 入口拒绝 |

轮询响应中的 `data.data` 与 ARK task response 相似，只能证明结果字段存在，不能反向证明同名请求能力。尤其不能据此推断视频参考、draft 或 tools 可用。

### 2.2 提交请求

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

上游公开文档还声明以下 JSON 形态：

- `image`: 单张首帧，支持 HTTP(S) URL、Base64 和 data URI。
- `images`: 多张参考图数组。
- `image_with_roles`: `{url, role}` 数组，角色为 `first_frame` / `last_frame`。
- `content`: 多模态数组；已确认 `audio_url` + `reference_audio`。
- `generateAudio`: 是否生成音轨，注意上游字段为 camelCase。
- `seconds` / `duration`: 可选时长字段。
- `size`: 部分上游兼容的尺寸字段。
- `metadata`: 扩展对象，是否生效取决于上游模型。

测试报告另已验证 `ratio`、`duration` 和显式 `watermark: false` 可提交。其他未知顶层字段在 OpenAI Video 入口继续语义透传，但不因此升级为本项目承诺支持的能力。

### 2.3 提交响应

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

### 2.4 详细轮询响应

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
| 本地 `POST /api/v3/contents/generations/tasks` | 支持已确认的 ARK 子集 |
| 本地 `GET /api/v3/contents/generations/tasks/:task_id` | 支持包含 `usage` 的 ARK task response |
| 本地 `GET /api/v3/contents/generations/tasks` | 支持既有列表协议 |
| 上游 `POST /v1/video/generations` | 支持 |
| 上游 `GET /v1/video/generations/:task_id` | 支持 |
| `application/json` | 支持 |
| multipart | 不支持 |
| `POST /v1/videos` | 不支持 |
| `GET /v1/videos/:task_id` | 不作为首版契约 |
| ARK 图片参考 | 单首帧、首尾帧支持；多图参考为试验能力 |
| ARK 音频参考 | 支持 |
| ARK 视频参考 | 不支持，返回 400 |
| ARK draft/tools | 非默认能力不支持，返回 400 |
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

ARK SDK POST /api/v3/contents/generations/tasks
  -> 校验 ARK 请求和已确认能力边界
  -> 将 content 图片/音频转换为上游 new-api 字段
  -> POST {baseURL}/v1/video/generations
  -> 返回 {"id":"task_local_public_id"}

ARK SDK GET /api/v3/contents/generations/tasks/{public_task_id}
  -> 按用户和公开 ID 查询本地任务
  -> NewAPIVideo.ArkVideoTaskConverter
  -> 合并外层 TaskDto 与 data.data 的安全 ARK 字段
  -> 返回 ARK task response
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

### 6.2 OpenAI Video 入口保留请求语义

适配器读取原始 JSON 对象并保留所有顶层字段，只覆盖顶层 `model`。这里的“原样透传”表示字段和值的 JSON 语义不变，不保证空白、缩进和字段顺序不变。

以下显式值必须保留：

- `watermark: false`
- `seed: 0`
- 其他合法的零值、布尔值、数组、对象和未知扩展字段

不得使用 `TaskSubmitReq -> dto.VideoRequest` 转换，因为它会丢失字段，并且会把小数 `duration` 静默转成零值。

### 6.3 OpenAI Video 入口验证规则

| 字段 | 首版规则 |
|---|---|
| `model` | 必填、非空字符串 |
| `prompt` | 必填、非空字符串 |
| `duration` | 可省略；存在时必须是有限数值，`> 0` 且不超过 `relaycommon.MaxTaskDurationSeconds`；非 `per_duration` 模式允许小数 |
| `seconds` | 可省略；接受有限数值或规范数值字符串，边界与 `duration` 相同 |
| `n` | 可省略；存在时必须是整数 `1` |
| `watermark` | 存在时必须是布尔值 |
| `seed` | 存在时必须是整数，显式 `0` 合法 |
| 其他字段 | 不解释、不修改，由上游协议校验 |

`duration` 与 `seconds` 同时存在时必须数值相等。`metadata.duration`、`metadata.seconds` 和 `metadata.n` 也必须检查，防止通过 metadata 绕过计费边界。若 metadata 中的值缺少对应顶层字段，或与顶层值冲突，则返回 HTTP 400；首版只认顶层字段为权威计费来源。

首版限制 `n = 1`，因为本地任务和 `OpenAIVideo` 只表达一个视频，当前结算不能安全表示一次任务的多个独立视频结果。

### 6.4 ARK 入口转换规则

ARK 请求不能原样发给上游，因为上游要求顶层 `prompt`，图片字段也不是统一的 `content[]`。适配器必须解析、验证并构造新的上游 JSON 对象。

| ARK 输入 | 上游 new-api 输入 | 规则 |
|---|---|---|
| `model` | `model` | 使用渠道模型映射后的名称 |
| 唯一非空 `content[type=text].text` | `prompt` | 必填；同时在音频请求的 `content` 中保留文本项 |
| 单个 `image_url`，role 缺失或 `first_frame` | `image` | URL/Base64/data URI 原值保留 |
| `first_frame` + `last_frame` | `image_with_roles` | 转为 `{url, role}` 数组 |
| `reference_image` | `images` | 可多张；属于试验能力 |
| `audio_url` + `reference_audio` | `content[].audio_url` | 保留媒体对象和 role |
| `generate_audio` | `generateAudio` | 显式布尔值保留；音频存在且字段缺失时设为 `true` |
| `ratio`、`duration`、`watermark` | 同名字段 | 按已实测字段转发 |
| `resolution` | 不直接转发 | 必须与映射后模型名中的 480p/720p/1080p 档位一致；仅作为本地校验和响应信息 |
| `service_tier` | 不转发 | 仅接受缺失或 `default` |
| `draft: false`、空 `tools` | 不转发 | 允许中性默认值，不能宣称支持该能力 |

以下 ARK 请求首版返回 HTTP 400 和 ARK 错误 envelope：

- 任意 `content[type=video_url]`。
- 任意 `content[type=draft_task]`。
- 缺少非空 text，或存在多个非空 text。
- 多于一个音频参考。
- `draft: true` 或非空 `tools`。
- 音频参考与显式 `generate_audio: false` 的矛盾组合。
- 未被映射表声明、且忽略后可能改变请求语义的 ARK 顶层字段，例如 `frames`、`seed`、`camera_fixed`、`return_last_frame`、`priority`、`execution_expires_after` 和 `safety_identifier`。
- 首尾帧、参考图、音频三种媒体模式互相混合等未被上游文档确认的组合。

ARK 的 `duration` 使用整数秒并受 `relaycommon.MaxTaskDurationSeconds` 约束。音频参考可以只有文本和音频，不套用当前 Doubao 适配器“音频必须同时有图片或视频”的限制，因为目标上游文档已经给出纯音频参考的成功请求形态。

## 7. 提交流程

### 7.1 上游请求

- URL：`POST {baseURL}/v1/video/generations`
- Header：`Authorization: Bearer {apiKey}`
- Header：`Content-Type: application/json`
- Header：`Accept: application/json`
- OpenAI Video Body：客户端顶层 JSON 字段加映射后的 `model`
- ARK Body：按 6.4 构造的上游 new-api JSON

### 7.2 上游响应

上游提交响应统一解析为 `dto.OpenAIVideo` 投影：

1. 优先读取 `task_id`，兼容只返回 `id` 的上游。
2. `id` 和 `task_id` 同时存在但不一致时，按无效上游响应处理。
3. 两者均为空时返回无效上游响应错误，不创建本地任务。
4. 上游任务 ID 返回给任务框架，并以 `Task.PrivateData.UpstreamTaskID` 作为后续通信的权威索引；完整内部响应中可以保留其审计副本。
5. OpenAI Video 客户端响应中的 `id` 和 `task_id` 都替换为 `info.PublicTaskID`。
6. OpenAI Video 客户端响应中的 `model` 替换为 `info.OriginModelName`。
7. OpenAI Video 响应的 `object` 固定为 `video`，状态、进度和创建时间保留上游已验证语义。
8. ARK 客户端提交响应只返回 `{"id":"<public_task_id>"}`，与现有 ARK 入口行为一致。

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

同一任务的 ARK 提交响应：

```json
{
  "id": "task_local_public_id"
}
```

## 8. 轮询与完整数据保留

### 8.1 双轨处理

轮询响应必须采用“解析投影 + 完整存储”方式：

1. 将原始响应体完整保存到 `task.Data`。
2. 优先解析实测的 `TaskResponse<TaskDto>`；若没有 wrapper，则兼容公开文档展示的直接 `OpenAIVideo`。
3. 同一份响应只读解析为明确投影，用于提取任务状态、进度、URL、错误、时间和 usage。
4. 不得将精简 DTO 重新序列化后覆盖 `task.Data`。

投影中的嵌套 `data.data` 使用 `json.RawMessage` 或等价的保留方式。`draft: false`、`seed`、`duration`、`resolution`、`usage` 和上游未来增加的未知字段都必须保留。允许安全层对巨大的内嵌 Base64 媒体执行现有截断策略，但不能删除普通 JSON 字段。

完整响应是内部数据，不作为客户端响应直接返回。OpenAI Video 和 ARK 两种客户端输出都必须通过各自的明确字段白名单构造。

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

直接 `OpenAIVideo` 响应兼容 `queued`、`in_progress`、`running`、`completed`、`succeeded`、`failed` 和 `cancelled`。其中 `completed` / `succeeded` 映射成功，`failed` / `cancelled` 映射失败；未知状态同样保持任务原状态并等待后续轮询。

未知状态不得默认映射为成功、失败或处理中。本轮轮询返回解析错误并保持任务原状态，等待后续轮询或系统超时处理。

### 8.3 结果和错误

视频 URL 的读取顺序：

1. `data.result_url`
2. `data.data.content.video_url`
3. 直接响应的 `metadata.url`
4. 直接响应的 `content.video_url`
5. 直接响应的 `data.url`

成功状态但所有已知位置都没有 URL 时，不立即标记成功；保持原状态并等待下一轮，避免产生无法下载的视频任务。

失败原因的读取顺序：

1. `data.fail_reason`
2. `data.data.error.message`
3. 直接响应的 `error.message`
4. 上游错误 envelope 的 `message`
5. 统一回退文本 `task failed`

错误码优先读取 `data.data.error.code`，其次读取直接响应的 `error.code`。

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

### 9.1 OpenAI Video 响应

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

### 9.2 ARK task response

`GET /api/v3/contents/generations/tasks/:public_task_id` 直接返回 ARK 风格对象，不返回 `code/data` wrapper。成功示例：

```json
{
  "id": "task_local_public_id",
  "model": "seedance-720p-token",
  "status": "succeeded",
  "content": {
    "video_url": "https://example.com/video.mp4"
  },
  "created_at": 1784716214,
  "updated_at": 1784716351,
  "draft": false,
  "duration": 5,
  "execution_expires_after": 172800,
  "framespersecond": 24,
  "generate_audio": true,
  "priority": 0,
  "ratio": "16:9",
  "resolution": "720p",
  "seed": 47347,
  "service_tier": "default",
  "usage": {
    "completion_tokens": 108900,
    "total_tokens": 108900
  }
}
```

ARK 转换器从 `data.data` 复制以下已知安全字段，并保留显式 `false` 和 `0`：

- `content.video_url`、`created_at`、`updated_at`、`draft`、`duration`、`execution_expires_after`
- `framespersecond`、`generate_audio`、`priority`、`ratio`、`resolution`、`seed`、`service_tier`
- `usage.completion_tokens`、`usage.total_tokens` 和标准 `error.code` / `error.message`

`usage` 是对客户端公开的 ARK 契约，不只是内部计费数据。优先从详细响应的 `data.data.usage` 读取；兼容直接响应的顶层 `usage`。`completion_tokens` 和 `total_tokens` 必须原值返回，显式 `0` 不能被 `omitempty` 或真假值判断丢失。ARK 输出 DTO 必须用指针字段或等价的 presence 标记区分“缺失”和“显式零值”。上游没有返回 usage 时才省略，不能使用外层 `data.quota` 或本地 `Task.Quota` 推导 token。

随后执行强制覆盖：

- `id` 使用本地公开任务 ID，绝不返回 `data.data.id`。
- `model` 使用客户端提交的本地模型名，绝不返回 provider model。
- `status` 使用本地状态映射：queued -> `queued`，处理中 -> `running`，成功 -> `succeeded`，失败 -> `failed`。
- `content.video_url` 为空时使用已经解析的 `Task.PrivateData.ResultURL`。
- `created_at` / `updated_at` 优先使用嵌套 ARK 值，缺失时回退到外层任务时间。

失败任务返回相同顶层结构，并包含：

```json
{
  "id": "task_local_public_id",
  "model": "seedance-720p-token",
  "status": "failed",
  "error": {
    "code": "upstream_error_code",
    "message": "task failed"
  },
  "created_at": 1784716214,
  "updated_at": 1784716375
}
```

外层 wrapper 的 `user_id`、`channel_id`、`group`、`quota`、`platform` 和上游任务 ID 属于内部数据，不进入 ARK 响应。`data.data` 的未知未来字段保留在 `task.Data`，但未经审查不自动公开。

ARK 列表接口复用同一转换器，确保单查和列表字段、ID 改写及状态语义一致；平台白名单加入 `NewAPIVideo`，单查也必须执行相同白名单。

## 10. 计费设计

新适配器接入现有计费模式，不自行定义第三方价格公式。

### 10.1 固定价格

当客户端模型使用固定价格或 `UsePrice` 时，按配置的单次价格预扣和结算。轮询返回的 token 只保留用于诊断，不覆盖固定价格。

### 10.2 按时长计费

适配器实现 `TaskDurationEstimator`：

- `duration` 必填。
- OpenAI Video 请求可由规范化后的 `duration` 或 `seconds` 提供；ARK 请求使用 `duration`。
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
6. 若精简直接响应没有任何 usage，不能把“字段缺失”解释为零 token；沿用现有结算语义保留预扣额度，不产生免费任务。

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
- `/v1/video/generations/:task_id` 和 `/v1/videos/:task_id` 都识别为 OpenAI Video 查询；类型 60 走明确的 `OpenAIVideoConverter`，不返回通用 `TaskDto`。
- 类型 60 实现 `ArkVideoTaskConverter`，只公开第 9.2 节列出的 ARK 字段，并强制改写 ID、模型和状态。
- ARK 单任务查询必须与列表使用相同的平台白名单；类型 60 加入白名单，其他平台不得通过 `/api/v3/...` 的 raw-data 回退泄露内部响应。
- OpenAI Video 与 ARK 查询可以读取同一任务，但各自独立转换，不能互相复用序列化后的客户端响应。
- 首版不修改 `dto.VideoRequest`、`dto.VideoResponse` 和 `dto.VideoTaskResponse`，因为它们不是实测协议。
- 不新增数据库字段或迁移；继续使用现有 `Task`、`TaskPrivateData` 和 `TaskBillingContext`。

## 13. 预计改动

### 13.1 后端

| 文件 | 改动 |
|---|---|
| `relay/channel/task/newapivideo/constants.go` | 渠道名称；不提供虚假默认模型 |
| `relay/channel/task/newapivideo/dto.go` | OpenAI/ARK 请求投影、双形态轮询投影、ARK 输出和错误结构 |
| `relay/channel/task/newapivideo/native.go` | ARK 内容校验和到上游 new-api 字段的显式转换 |
| `relay/channel/task/newapivideo/adaptor.go` | 提交、轮询、两种客户端响应转换、计费接口和错误解析 |
| `relay/relay_adaptor.go` | 注册类型 60 适配器 |
| `constant/channel.go` | 新增类型 60，Dummy 后移到 61 |
| `relay/relay_task.go` | `/v1/video/generations/:id` 识别为 OpenAI Video，并走类型 60 转换分支 |
| `service/task_polling.go` | 正确解析 new-api `TaskDto.result_url`、完整保存响应、处理轮询 HTTP 状态 |
| `relay/seedance_task.go` | 类型 60 加入 ARK 白名单；单查与列表共用安全转换器，禁止原始数据回退 |
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
- 覆盖 `duration` / `seconds` 的缺失、整数、小数、数值字符串、冲突、零值、负数、超上限和 `per_duration` 限制。
- 覆盖 `n` 缺失、`1`、零值、负数、小数、超大整数和大于 `1`。
- 覆盖 metadata 计费字段绕过和冲突。
- 断言 `watermark: false`、`seed: 0`、未知字段和嵌套对象保留。
- 断言只修改顶层 `model`。
- ARK 覆盖文生、单首帧、首尾帧、多参考图和音频参考的精确上游 JSON。
- ARK 覆盖视频参考、draft task、`draft: true`、非空 tools、未知非中性字段和未确认媒体组合的明确拒绝。
- ARK 断言 `generate_audio` 转为 `generateAudio`，中性 `draft: false` / 空 tools 可接受但不发送。

### 14.2 适配器契约

- 提交方法和路径必须是 `POST /v1/video/generations`。
- 轮询方法和路径必须是 `GET /v1/video/generations/{upstream_task_id}`。
- 验证 Bearer 鉴权、Content-Type、Accept 和任务 ID URL 转义。
- 验证提交响应的 `id`/`task_id` 兼容、冲突和缺失情况。
- 验证两种上游错误 envelope。
- 验证 OpenAI Video 提交返回完整 `OpenAIVideo`，ARK 提交只返回公开 `id`。
- 验证所有客户端响应只包含本地公开 ID 和本地模型名。

### 14.3 轮询和完整数据

- 对所有已知状态做确定性表格测试。
- 验证 URL 和失败原因的回退顺序。
- 验证 `SUCCESS` 无 URL 保持可重试。
- 验证 429/5xx 不修改任务，404/410 和确定性 4xx 进入失败。
- 使用实测完整 `data.data` 样例，断言 `draft: false`、`seed`、`usage` 和未知字段都保留在 `task.Data`。
- 使用公开文档的直接 `OpenAIVideo` 样例验证兼容解析，以及 `metadata.url` / `content.video_url` / `data.url` 回退。
- 验证 `completion_tokens: 0` 与字段缺失的区别。
- 验证 token 超限进入现有饱和审计。
- 验证 queued、in_progress、completed、failed 四种本地 `OpenAIVideo`。
- 验证 ARK queued/running/succeeded/failed，且完整保留第 9.2 节的 `draft: false`、`priority: 0`、usage 等安全字段。
- 验证 ARK `usage` 来自 `data.data.usage` 或直接响应的顶层 `usage`，显式零值不丢失，且外层 `data.quota` 不能混入 usage。
- 验证任何客户端出口都不包含上游任务 ID。
- 验证 ARK 单查和列表只接受白名单平台，类型 60 走 converter，其他平台不能读取原始 `task.Data`。

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

提交 ARK content JSON
  -> 本地拒绝未确认能力或生成精确的 new-api 上游字段
  -> mock 上游返回任务 ID
  -> 本地返回 ARK {id: public_task_id}
  -> mock 详细轮询返回带完整 data.data 的 TaskResponse<TaskDto>
  -> ARK 单查和列表返回改写 ID/model/status 后的安全 task response
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
6. OpenAI Video 客户端使用本地 `POST /v1/video/generations` 提交，并使用本地 `GET /v1/video/generations/:task_id` 查询。
7. ARK SDK 客户端可使用 `/api/v3/contents/generations/tasks*`；首版仅承诺第 2.1 和 6.4 节列出的子集。
