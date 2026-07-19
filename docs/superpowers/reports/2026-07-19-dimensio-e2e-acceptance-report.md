# Dimensio Seedance 2.0 自动化 E2E 验收报告

## 1. 验收结论

Dimensio 渠道已经通过协议级和应用级自动化 E2E 验收。测试没有请求真实 `jimeng.dimensio.cn`，Dimensio 提交和查询接口均由本地 `httptest` mock 提供，因此没有产生上游积分成本。

覆盖的开放模型和结果矩阵：

| 模型 | 分辨率 | 成功任务 | 失败任务 | `-2011` 终态错误 | `1057` 可重试错误 |
|---|---:|---:|---:|---:|---:|
| `jimeng-video-seedance-2.0-fast-vip` | 720p | 通过 | 通过 | 通过 | 通过 |
| `jimeng-video-seedance-2.0-mini` | 720p | 通过 | 通过 | 通过 | 通过 |
| `jimeng-video-seedance-2.0-vip` | 1080p | 通过 | 通过 | 通过 | 通过 |

每个场景都使用完整多模态输入：提示词、参考图、参考视频和参考音频。

## 2. 验收清单

- [x] ARK SDK 标准入口为 `POST /api/v3/contents/generations/tasks`。
- [x] ARK 客户端模型通过渠道映射转换为三个 Dimensio Seedance 2.0 模型。
- [x] 映射后的目标模型严格限制为文档列出的三个开放模型。
- [x] fast-vip/mini 仅接受 720p；只有 vip 接受 1080p。
- [x] 请求使用 Dimensio Bearer API Key，客户端 Token 不会转发给上游。
- [x] `reference_image` 转换为顶层 `image_file_1`。
- [x] `reference_video` 转换为顶层 `video_file_1`。
- [x] `reference_audio` 转换为顶层 `audio_file_1`。
- [x] 提示词转换为顶层 `prompt`，完整多模态模式转换为 `functionMode=omni_reference`。
- [x] 上游请求不发送内部兼容字段 `file_paths`。
- [x] `duration`、`resolution`、`ratio`、`intelligent_ratio` 和 `face_grid` 保持预期语义。
- [x] 720p 计费倍率为 `seconds=6,resolution=1`。
- [x] VIP 1080p 计费倍率为 `seconds=6,resolution=2.5`。
- [x] 提交响应只返回 new-api 公开 `task_*` ID，不泄漏 Dimensio `task_id`。
- [x] 轮询使用内部 Dimensio `task_id` 和 `GET /v1/videos/tasks/:taskId`。
- [x] Dimensio `completed/result.url` 转换为 ARK `succeeded/content.video_url`。
- [x] Dimensio `failed/error/error_code` 转换为 ARK `failed/error.message/error.code`。
- [x] Dimensio `{code:-2011,message}` 请求级错误转换为可查询的 ARK 失败终态，而不是查询 500。
- [x] 任意 2xx 中的正数业务错误码（如 `1006`）保留为供应商错误并规范化为 502，不退化成 `invalid_response` 或错误的 2xx 响应。
- [x] 查询限流码 `1057/121101` 保持任务未完成和预扣额度，允许后续继续轮询，不触发退款。
- [x] 单查响应不泄漏 Dimensio 上游任务 ID。
- [x] 成功任务保持提交阶段按请求参数计算的预扣额度。
- [x] 失败任务退还用户、渠道和 Token 三个账本的预扣额度。
- [x] 查询响应未定义的 `duration` 扩展字段不能覆盖提交阶段计费快照。
- [x] 请求参数、媒体数量、角色组合和错误响应的单元/回归测试通过。

## 3. 实际协议样本

以下 JSON 来自自动化测试实际捕获值。示例使用 VIP 1080p；另外两个 720p 模型执行相同字段断言。

### 3.1 ARK SDK 请求

```json
{
  "model": "doubao-seedance-2-0-260128",
  "content": [
    {
      "type": "image_url",
      "role": "reference_image",
      "image_url": {"url": "https://mock.example/reference-image.jpg"}
    },
    {
      "type": "video_url",
      "role": "reference_video",
      "video_url": {"url": "https://mock.example/reference-video.mp4"}
    },
    {
      "type": "audio_url",
      "role": "reference_audio",
      "audio_url": {"url": "https://mock.example/reference-audio.mp3"}
    },
    {
      "type": "text",
      "text": "参考图中主体、参考视频动作和参考音频节奏，镜头缓慢向前推进"
    }
  ],
  "ratio": "16:9",
  "duration": 6,
  "resolution": "1080p",
  "intelligent_ratio": false,
  "face_grid": true
}
```

### 3.2 转换后的 Dimensio 提交请求

```json
{
  "audio_file_1": "https://mock.example/reference-audio.mp3",
  "duration": 6,
  "face_grid": true,
  "functionMode": "omni_reference",
  "image_file_1": "https://mock.example/reference-image.jpg",
  "intelligent_ratio": false,
  "model": "jimeng-video-seedance-2.0-vip",
  "prompt": "参考图中主体、参考视频动作和参考音频节奏，镜头缓慢向前推进",
  "ratio": "16:9",
  "resolution": "1080p",
  "video_file_1": "https://mock.example/reference-video.mp4"
}
```

请求目标与认证：

```text
POST /v1/videos/generations
Authorization: Bearer mock-dimensio-key
```

Mock 提交响应：

```json
{
  "created": 1709123456,
  "task_id": "dim-upstream",
  "status": "pending"
}
```

对 ARK SDK 的提交响应：

```json
{
  "id": "task_<new-api-public-id>"
}
```

## 4. 成功任务结构

Mock Dimensio 查询响应严格使用渠道 API 文档定义的完成结构：

```json
{
  "task_id": "dim-upstream",
  "status": "completed",
  "progress": 100,
  "result": {
    "url": "https://mock.dimensio/video.mp4"
  }
}
```

转换后的 ARK SDK 响应：

```json
{
  "content": {
    "video_url": "https://mock.dimensio/video.mp4"
  },
  "created_at": 1784449698,
  "id": "task_<new-api-public-id>",
  "model": "doubao-seedance-2-0-260128",
  "status": "succeeded",
  "updated_at": 1784449698,
  "usage": {}
}
```

验收点：公开响应不包含 `dim-upstream`；视频 URL 位于 `content.video_url`。

## 5. 失败任务结构

Mock Dimensio 查询响应：

```json
{
  "task_id": "dim-upstream",
  "status": "failed",
  "error": "视频安全审核不通过，请重试",
  "error_code": "2043"
}
```

转换后的 ARK SDK 响应：

```json
{
  "content": {},
  "created_at": 1784449698,
  "error": {
    "code": "2043",
    "message": "视频安全审核不通过，请重试"
  },
  "id": "task_<new-api-public-id>",
  "model": "doubao-seedance-2-0-260128",
  "status": "failed",
  "updated_at": 1784449698,
  "usage": {}
}
```

验收点：失败码和消息保持，公开响应不包含上游任务 ID，用户/渠道/Token 的已用额度均回到 0。

请求级资源过期响应也执行了完整 E2E：

```json
{
  "code": -2011,
  "message": "task expired",
  "data": null
}
```

对应的 ARK 终态为：

```json
{
  "id": "task_<new-api-public-id>",
  "model": "doubao-seedance-2-0-260128",
  "status": "failed",
  "content": {},
  "usage": {},
  "error": {
    "code": "-2011",
    "message": "task expired"
  }
}
```

查询返回 `1057/121101` 限流码时不是终态。自动化 E2E 验证一次轮询后公开状态仍为 `queued`，任务继续留在未完成集合，用户、渠道和 Token 保持原预扣金额。

## 6. 计费验收

本次测试删除了客户端别名 `doubao-seedance-2-0-260128` 的价格，只给映射后的三个目标模型配置不同的 `ModelRatio`，从而证明计费查价使用目标模型。分组倍率为 `1`，任务使用 6 秒时长：

这些 `ModelRatio` 是验收夹具，用于产生互不相同且可精确断言的额度，不是代码内置的生产价格；生产值仍由管理员按业务定价配置。

| 场景 | ModelRatio | OtherRatios | 预扣 quota | 终态处理 |
|---|---:|---|---:|---|
| fast-vip 720p | 0.088 | `seconds=6,resolution=1` | 132000 | 成功保持；失败全退；限流保持 |
| mini 720p | 0.072 | `seconds=6,resolution=1` | 108000 | 成功保持；失败全退；限流保持 |
| vip 1080p | 0.112 | `seconds=6,resolution=2.5` | 420000 | 成功保持；失败全退；限流保持 |

计费公式：

```text
baseQuota = ModelRatio / 2 * QuotaPerUnit * GroupRatio
quota = baseQuota * seconds * resolutionRatio
```

渠道文档的查询响应没有实际 `duration` 字段，因此终态不能按上游实际时长重算。系统以提交时经过 4-15 秒校验的请求时长计费，并把 `seconds`、`resolution` 保存到 `TaskBillingContext.OtherRatios`。

## 7. 自动化测试证据

协议级 E2E：

```text
go test ./relay/channel/task/dimensio/ -run 'TestDimensioSeedance20ProtocolE2E|TestParseTaskResultDoesNotAdjustRequestBasedBilling' -count=1 -v
PASS: 7 个测试/子场景
```

应用级 E2E：

```text
go test ./e2e -run TestDimensioSeedance20MultimodalLifecycleE2E -count=1 -v
PASS: 12 个模型/状态组合
```

相关出口回归：

```text
go test ./relay ./controller ./constant -run 'Dimensio|SeedanceTask|SeedanceTaskError' -count=1 -v
PASS
```

全项目回归与静态验证：

```text
go test ./...                                                        PASS
go vet ./relay/channel/task/dimensio/ ./relay ./controller ./constant ./e2e
                                                                     PASS
go build ./...                                                       PASS
git diff --check                                                    PASS
```

## 8. Mock 边界与剩余风险

- 本次没有连接真实 `https://jimeng.dimensio.cn`，未验证真实 API Key、网络、上游媒体抓取和生成质量。
- 参考视频总时长不超过 15 秒属于媒体内容属性；当前请求仅携带 URL，网关无法在不下载媒体的前提下验证时长，该限制由 Dimensio 上游执行。
- `result.url` 的真实有效期和重复查询刷新行为没有进行时间型测试。
- 渠道文档没有在完成响应中提供实际生成秒数，计费按请求时长冻结。这是契约限制，不是测试缺口。
