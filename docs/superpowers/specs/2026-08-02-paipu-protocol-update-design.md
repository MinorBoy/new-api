# Paipu 渠道协议更新设计

## 1. 背景

`docs/new-channels/cn-paipu.html` 已更新。新文档确认 Paipu 继续通过以下任务接口提供视频生成能力：

```text
POST /v1/videos
GET  /v1/videos/{task_id}
GET  /v1/videos/{task_id}/content
```

当前 Paipu adaptor 仍复用纯文本 `textRequestProfile`，只允许一个文本内容项，并把 Ark `ratio` 编码为上游 `ratio`。这与新文档中的 `aspect_ratio`、`images`、`videos`、`audios` 数组协议不一致，也无法满足 Ark SDK 多模态请求零代码改动的要求。

本次更新只把 `cn-paipu.html` 作为渠道 API 协议依据。Paipu 的正确模型 ID、模型能力、路由约束和成本数据仍以 `sd收录表/渠道模板表` 的配置导入结果为唯一数据来源，不能从 HTML 文档硬编码第二份模型目录。

## 2. 目标

1. Ark SDK 用户继续只调用 `/api/v3/contents/generations/tasks/*`，客户端代码无需改动。
2. 将 Ark 文本、图片、视频和音频内容转换为 Paipu `/v1/videos` 数组协议。
3. 将协议实现与导入的模型目录、模型能力和价格配置解耦。
4. 禁止 Paipu 创建任务自动重试，避免上游结果不确定时产生重复任务和重复成本。
5. 保持公共任务 ID、所有权校验、计费、结算、退款和敏感信息清理继续复用共享 Ark 任务生命周期。

## 3. 非目标

- 不根据 `cn-paipu.html` 替换或补充导入的模型 ID。
- 不在代码中硬编码 Paipu 模型价格、按次/按秒计费方式或固定时长。
- 不为 Paipu 增加聊天、图片生成或通用 OpenAI 视频路由。
- 不向 Ark 公共请求增加 Paipu 私有的 `quality`、`size` 或 `seconds` 参数。
- 本次不实现 Paipu `/content` 私有任务 ID 代理；只有真实响应证明完成态可能没有结果 URL 时才增加该能力。
- 不修改 `sd收录表` 或渠道模板表中的业务数据。

## 4. 真相源边界

### 4.1 API 协议真相源

`docs/new-channels/cn-paipu.html` 定义：

- Bearer 鉴权；
- `/v1/videos` 创建路径；
- `/v1/videos/{task_id}` 轮询路径；
- `prompt`、`duration`、`aspect_ratio`、`resolution` 字段；
- `images`、`videos`、`audios` 素材数组；
- 创建请求不能自动重试；
- 创建和查询响应中的任务 ID、状态和结果 URL 形态。

### 4.2 模型和商业数据真相源

配置导入结果定义：

- 上游模型 ID；
- 客户端模型到上游模型的映射；
- 时长范围；
- 输出分辨率；
- 图片、视频、音频数量限制及最低数量；
- 渠道线路；
- 按次、按时长或其他成本模式；
- 成本价格、币种和汇率。

Paipu adaptor 不维护这些数据的副本。管理表单的静态模型目录保持为空，管理员通过配置导入或手动映射提供模型。

## 5. 架构

在 `relay/channel/task/newapivideo` 中增加 Paipu 专用 request dialect。该 dialect 只处理协议级转换和全局安全边界，继续复用共享 `TaskAdaptor` 的提交、轮询、响应投影、公共任务 ID、计费和退款能力。

```text
Ark SDK request
  -> shared Ark parser
  -> routing capability selection from imported configuration
  -> Paipu protocol validation
  -> Paipu request encoder
  -> POST /v1/videos
  -> shared public task persistence
  -> GET /v1/videos/{private_task_id}
  -> shared Ark public response projection
```

职责边界：

- 路由系统负责判断某个导入模型是否支持请求中的时长、分辨率和素材数量。
- Paipu dialect 负责字段名称、素材类型、URL 安全、协议全局上限和不支持字段。
- 计费系统继续读取导入的 cost rule，不从请求编码器推导价格。
- controller 重试层负责执行 Paipu 创建任务禁止重试策略。

## 6. Ark 到 Paipu 请求转换

### 6.1 上游 DTO

Paipu 专用 DTO 使用指针保存可选标量，确保缺省字段被省略，显式零值不会被错误吞掉：

```go
type paipuRequest struct {
	Model       string   `json:"model"`
	Prompt      string   `json:"prompt"`
	Duration    *int     `json:"duration,omitempty"`
	AspectRatio *string  `json:"aspect_ratio,omitempty"`
	Resolution  *string  `json:"resolution,omitempty"`
	Images      []string `json:"images,omitempty"`
	Videos      []string `json:"videos,omitempty"`
	Audios      []string `json:"audios,omitempty"`
}
```

所有 JSON 编码使用 `common.Marshal`。

### 6.2 字段映射

| Ark 字段 | Paipu 字段 | 规则 |
| --- | --- | --- |
| 映射后的上游模型 | `model` | 必填，不检查静态模型白名单 |
| 文本内容 | `prompt` | 恰好一个非空文本项 |
| `duration` | `duration` | 指针整数，缺省时省略 |
| `ratio` | `aspect_ratio` | 缺省时省略 |
| `resolution` | `resolution` | 缺省时省略，不注入模型默认值 |
| 图片内容 | `images` | 保持原顺序 |
| 视频内容 | `videos` | 保持原顺序 |
| 音频内容 | `audios` | 保持原顺序 |

Ark 用户不需要改用 `seconds`。Paipu 文档把 `duration` 定义为兼容字段，adaptor 统一发送 `duration`，避免按模型维护字段分支。

`size` 和 `quality` 没有 Ark 公共字段，本次不从其他字段猜测或通过未定义扩展透传。

## 7. 内容和素材校验

### 7.1 文本规则

- 请求必须包含且只包含一个非空文本项。
- 文本可与任意数量的合法引用素材组合。
- 多个文本项返回 `400 InvalidParameter.content`，不进行上游请求和预扣费。

### 7.2 图片规则

- 支持无角色或 `reference_image`。
- 拒绝 `first_frame` 和 `last_frame`，因为 Paipu 数组协议不保留首尾帧语义。
- 支持公网 HTTP(S) URL。
- 支持合法且 MIME 类型匹配的 `data:image/*` URI。
- 协议全局上限为 9 张。

### 7.3 视频规则

- 仅接受无角色或 `reference_video`。
- 支持公网 HTTP(S) URL。
- 支持合法且 MIME 类型匹配的 `data:video/*` URI。
- 协议全局上限为 3 个。

### 7.4 音频规则

- 仅接受无角色或 `reference_audio`。
- 支持公网 HTTP(S) URL。
- 支持合法且 MIME 类型匹配的 `data:audio/*` URI。
- 协议全局上限为 3 个。

更严格的模型限制，例如 4 图、1 音频、图片必填或完全禁止视频，由导入的 route target capability 在渠道选择前处理。Paipu dialect 不根据模型名称重复维护这些限制。

### 7.5 标量和不支持字段

- `duration` 必须为正整数且不超过 `relaycommon.MaxTaskDurationSeconds`。
- 显式空 `ratio` 或 `resolution` 返回对应 `InvalidParameter` 错误。
- `service_tier` 缺省或 `default` 时省略；其他值返回 `400`。
- `generate_audio`、`watermark`、`seed`、`callback_url`、启用的 `draft`、非空 `tools` 和 `draft_task` 返回确定性 `400`。
- 所有协议校验必须在上游请求、任务落库和预扣费之前完成。

## 8. 模型目录和路由契约

`paipuProtocolProfile().modelList` 改为空数组。后端 `GetModelList()` 和前端 `supportedModels` 不再声明旧的 24 模型目录。

Paipu 的管理提示改为：模型由配置导入或管理员手动映射提供。渠道类型仍为 task-only，真实验收前默认 disabled。

`relay/video_route_contract.go` 不再调用纯文本路由验证。新的 Paipu 路由契约只验证协议本身能表达的边界：

- 上游模型名非空；
- 时长范围不超过全局任务时长上限；
- 图片、视频和音频上限分别不超过 9、3、3；
- 最低素材数量不能超过对应上限；
- 输出分辨率由导入数据保留，不通过模型名后缀猜测。

该验证不限制模型 ID，也不修改导入成本数据。

## 9. 创建请求禁止自动重试

Paipu 文档明确创建任务不会自动重试。网关也必须遵守相同语义：一旦请求已经进入 Paipu 提交流程，任何 `429`、`5xx`、超时、连接中断或结果不确定错误都直接返回，不得自动选择另一个渠道再次调用 `POST /v1/videos`。

controller 任务重试判断增加 task-only 渠道类型策略集合，Paipu 首先进入禁止重试集合。该策略在 `shouldRetryTaskRelay` 中早于状态码重试判断执行。

本地请求校验错误原本就不会重试，行为保持不变。

## 10. 响应和内容下载

现有共享响应解析已经支持：

- 创建响应的 `id`、`task_id` 和 `taskId`；
- `submitted`、`queued`、`in_progress`、`running`、`completed`、`succeeded` 和 `failed`；
- 顶层、`content`、`data`、`output` 和 `metadata` 中的结果 URL；
- 公共任务 ID 投影和私有上游任务 ID 清理。

本次只增加 Paipu 文档响应 fixture，避免为已支持字段复制 provider-specific parser。

完成态仍要求存在结果 URL。当前不使用上游 `/content` 作为无 URL fallback，也不把私有任务 ID暴露给客户端。真实 Paipu 验收若发现完成态没有 URL，再单独设计安全代理。

## 11. 计费和退款

- 预扣和结算继续使用导入的 cost rule。
- adaptor 只提供请求时长，不判断某个模型是按次还是按时长。
- 请求时长继续受 `MaxTaskDurationSeconds` 保护。
- 创建明确失败按照现有账务流程退款一次。
- 创建结果不确定时不自动重试，避免产生第二笔无法关联的上游成本。
- 终态失败和重复轮询继续遵守现有幂等结算、退款逻辑。

## 12. 测试设计

### 12.1 请求契约测试

- 精确断言 `model`、`prompt`、`duration`、`aspect_ratio`、`resolution` 和三个素材数组。
- 断言 Ark 素材顺序在对应数组中保持不变。
- 断言缺省可选标量和空数组不出现在 JSON。
- 断言显式 `0`、空字符串、超长时长和不支持字段返回确定性错误。
- 断言 9/3/3 边界通过，超出边界失败。
- 断言 frame role、MIME 不匹配、私网 URL、本地路径和 `file://` 失败。
- 断言合法 HTTP(S) 和 data URI 通过。

### 12.2 路由契约测试

- 任意导入模型名均可通过协议模型检查。
- route target 的 9/3/3 上限通过，超过上限失败。
- 最低素材数量超过上限失败。
- 不再根据模型名后缀强制分辨率。

### 12.3 重试测试

- Paipu 创建返回 `429` 时只调用一次上游。
- Paipu 创建返回 `500` 时只调用一次上游。
- 其他既有允许重试的 task channel 行为不变。
- 本地 `400` 继续不重试且不产生任务或额度副作用。

### 12.4 生命周期测试

- Ark 创建返回本地公共任务 ID。
- 轮询使用私有上游任务 ID，但公共单查和列表不泄露该 ID。
- 成功响应返回允许的结果 URL。
- 失败响应只退款一次。
- 多模态请求通过 Ark `/api/v3/contents/generations/tasks/*` 完整执行。

### 12.5 前端和配置导入测试

- Paipu 静态模型列表为空。
- 管理表单提示使用配置导入或手动映射。
- `CH-PAIPU` 仍映射到渠道类型 206。
- 导入的 Paipu 模型、route target 和 cost rule 不被静态目录覆盖或过滤。

## 13. 验收标准

1. Ark SDK 的文本、图片、视频、音频请求无需客户端改动即可转换为 Paipu 数组协议。
2. Paipu adaptor 和管理表单不再硬编码模型目录。
3. 导入的模型 ID、能力约束和成本规则保持原值。
4. Paipu 创建请求在任何错误状态下最多调用一次上游。
5. 公共响应不包含上游任务 ID、API Key、渠道 ID、上游模型、原始响应或计费内部数据。
6. focused Go、Ark 生命周期、前端配置和配置导入测试通过。
7. 真实上游验收完成前，Paipu 渠道继续默认 disabled。
