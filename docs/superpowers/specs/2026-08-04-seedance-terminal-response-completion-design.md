# Seedance 终态任务响应强制补全设计

## 背景

Ark SDK 视频矩阵 E2E 当前有 118 条成功任务。其中 25 条任务的上游 mock 返回完整的火山引擎 Ark 视频任务结构，另外 93 条任务只返回任务状态和视频 URL。后者覆盖 4SToken、8yes、Dimensio、Paipu、MegaByAI、Z5API，导致任务日志中的最终用户响应缺少 `usage`、`duration`、`resolution`、`framespersecond` 等字段。

根因不是任务日志页面裁剪字段，而是不同渠道适配器根据各自上游响应生成终态结果。上游没有返回的字段目前不会统一补齐，因此同一个 Ark SDK 接口产生了不同完整度的响应结构。

## 目标

- 所有 Seedance 渠道的成功任务必须返回完整、稳定的 Ark 视频终态结构。
- 所有 Seedance 渠道的失败任务必须返回完整公共字段和标准 `error`，不得伪造成功视频 URL。
- 任务日志保存的最终用户响应必须与实际返回给用户的响应一致。
- 上游明确返回的有效事实不得被请求快照、计费快照或默认值覆盖。
- 旧任务缺少请求或计费快照时，也必须通过固定默认值形成完整结构。
- E2E 必须覆盖每个视频渠道，并在任一渠道返回字段不完整时失败。

## 非目标

- 不修改供应商成本、用户售价、路由或渠道合同规则。
- 不要求各供应商返回火山引擎原生结构。
- 不回写历史数据库任务；重新运行 E2E 后产生新的完整验收数据。
- 不为失败任务伪造 `content.video_url`。

## 方案选择

采用集中式 Ark 终态响应补全器。各渠道适配器仍负责将供应商协议转换为 Ark 响应，统一补全器在转换完成后执行字段校验和缺省填充。

未采用逐渠道补全，因为这会重复实现相同优先级规则，并容易在新增渠道时再次遗漏。未采用只修改 E2E mock 的方案，因为它只能美化测试数据，不能保证真实上游响应不完整时的用户 API 合同。

## 数据来源优先级

每个字段按以下顺序取值，找到第一个有效值后停止：

1. 渠道适配器转换后的上游终态事实。
2. `TaskPrivateData.UserRequestData` 中的用户请求快照。
3. `TaskPrivateData.BillingContext` 中的冻结计费和请求事实。
4. 本设计规定的固定默认值。

补全器只填充缺失或无效字段，不覆盖上游明确返回的合法值。字符串必须非空，数值字段必须满足各自边界；超界上游值按缺失处理并进入后续回退。

## 成功响应合同

成功任务必须包含以下字段：

```json
{
  "id": "task_public_id",
  "model": "doubao-seedance-2-0-260128",
  "status": "succeeded",
  "content": {
    "video_url": "https://example.com/video.mp4"
  },
  "usage": {
    "completion_tokens": 0,
    "total_tokens": 0
  },
  "created_at": 0,
  "updated_at": 0,
  "seed": 0,
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

`id`、`model`、`status`、`content.video_url`、时间字段优先使用已持久化任务事实。`usage` 优先使用已校验上游用量或冻结的官方 Token 计费快照；只有旧任务完全没有可用 Token 事实时才使用零值。

## 默认值

| 字段 | 默认值 |
| --- | --- |
| `seed` | `0` |
| `resolution` | `720p` |
| `ratio` | `16:9` |
| `duration` | `5` |
| `framespersecond` | `24` |
| `service_tier` | `default` |
| `execution_expires_after` | `172800` |
| `generate_audio` | `true` |
| `draft` | `false` |
| `priority` | `0` |
| `usage.completion_tokens` | `0` |
| `usage.total_tokens` | `0` |

时间字段不使用常量时间。`created_at` 依次回退到 `submit_time`、任务 `created_at`；`updated_at` 依次回退到 `finish_time`、任务 `updated_at`，最终允许数据库时间字段的零值。

## 失败响应合同

失败任务必须包含与成功任务相同的公共元数据字段，并包含：

```json
{
  "status": "failed",
  "error": {
    "code": "task_failed",
    "message": "task failed"
  }
}
```

上游错误码和已脱敏错误消息优先。缺少错误事实时使用固定默认错误。失败响应不添加虚假的 `content.video_url`；如果上游错误响应意外包含成功视频 URL，统一补全层不依赖该 URL 判断任务成功。

## 组件与数据流

1. 任务轮询适配器解析供应商响应并更新 `Task.Data`、状态、结果 URL 和计费事实。
2. 渠道的 `ConvertToArkVideoTask` 生成初始 Ark 响应。
3. `seedanceTaskResponse` 对公共 ID、模型、状态、时间、视频 URL 和 Token 用量执行现有规范化。
4. 新增集中式终态补全步骤，解析用户请求快照和计费快照，按优先级补齐全部公共字段。
5. 完整响应返回给 Ark SDK 用户，并通过现有 `PersistTerminalTaskUserResponse` 保存到任务日志审计字段。

补全逻辑放在通用 Seedance 响应层，不放进前端。任务日志页面继续原样展示后端保存的最终用户响应。

## 请求快照解析

请求快照通过项目 `common.Unmarshal` 解析为只包含所需可选字段的内部结构。解析失败时不得影响任务查询，直接回退到计费快照和固定默认值。不会把请求中的媒体 URL、提示词或其他敏感字段复制到响应。

## E2E mock

所有视频 mock 轮询路径必须返回各自协议可表达的完整终态事实。通用固定事实为：

- `seed=78674`
- `resolution=720p`
- `ratio=16:9`
- `duration=5` 或测试目标请求时长
- `framespersecond=24`
- `service_tier=default`
- `execution_expires_after=172800`
- `generate_audio=true`
- `draft=false`
- `priority=0`
- 合法 `usage.completion_tokens` 和 `usage.total_tokens`

即使某个供应商协议只能返回简化结果，最终 Ark 用户响应仍由集中式补全器达到完整合同。

## 测试策略

### 单元测试

- 简化渠道响应只有状态和 URL 时，最终 Ark 响应包含全部字段和固定默认值。
- 用户请求快照和计费快照的值优先于固定默认值。
- 上游合法值优先于请求快照和计费快照。
- 无 Token 事实的旧任务返回显式零值 `usage`。
- 失败任务包含完整公共字段和默认 `error`，但不包含伪造视频 URL。
- 请求快照损坏时仍能稳定返回完整响应。
- 数值越界或非法字符串不会进入最终响应，而是触发回退值。

### E2E

- Ark 视频矩阵种子对每条成功任务断言完整字段集合。
- 对每条失败任务断言公共字段、`error` 和无伪造视频 URL。
- 重新执行完整矩阵并核对 4SToken、8yes、Dimensio、Paipu、MegaByAI、Z5API 不再出现简化任务详情。
- 核对任务日志保存的数据与用户任务查询响应 JSON 等价。

## 验收标准

- 新一轮 E2E 的所有成功任务详情均包含成功响应合同列出的全部字段。
- 新一轮 E2E 的所有失败任务详情均符合失败响应合同。
- 任务日志不再出现只有 `id`、`model`、`status`、`content`、时间字段的简化成功响应。
- 不影响现有用户售价、供应商成本和利润核算结果。
- 后端相关测试、Ark E2E、前端任务日志测试、类型检查和构建全部通过。
