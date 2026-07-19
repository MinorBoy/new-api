# Seedance 计费 E2E 验收报告

## 范围与结论

本轮聚焦验收通过。验收覆盖 Seedance 2.0、2.0 Fast、2.0 Mini 和 1.5 Pro 的能力矩阵、价格倍率、预扣、异步终态结算、差额、账务守恒、非法输入和上游拒绝退款。

测试使用本地 `httptest.Server` 模拟 ARK，上游请求仍经过公开 `POST /api/v3/contents/generations/tasks`、Doubao adaptor、真实任务入库、一次真实轮询和终态结算。数据库为 SQLite in-memory。没有访问真实 ARK、没有下载媒体、没有真实网络成本或供应商费用。

本报告只说明 new-api 如何消费 mock 返回的 authoritative `completion_tokens`。mock 公式是测试 fixture，不是 ARK 真实或私有 token 算法。

## 精确测试规模

| 层级 | 数量 | 说明 |
| --- | ---: | --- |
| adaptor fast explicit matrix | 60,348 | 60,096 个 Seedance 2.0 family 组合 + 252 个 Seedance 1.5 Pro 组合 |
| successful HTTP explicit | 636 | 所有独立 model/resolution/duration/has-video/image/1.5 billing 维度 |
| successful HTTP duration modes | 120 | 每个 unique billing class 的 omitted 与 `-1` |
| successful HTTP ordered video profiles | 312 | 14 个单视频 + 78 个双视频 + 220 个三视频 |
| successful HTTP lifecycle 总数 | 1,068 | `636 + 120 + 312` |
| adaptor direct invalid | 36 | 直接调用内容和字段验证器 |
| public HTTP local invalid | 38 | 上游请求数不变，domain snapshot 不变 |
| public HTTP upstream reject/refund | 4 | mock 收到请求并返回 ARK HTTP 400，随后完整退款 |
| mock handler standalone negative fixtures | 6 | 只验证测试 mock，不计入 38 个用户 HTTP 本地非法用例 |

60,348 的构成为:

```text
Seedance 2.0 families:
(4*12 + 2*12 + 2*12) model-resolution-duration cells
* (1 no-video + 312 ordered video profiles)
* 2 image states
= 60,096

Seedance 1.5 Pro:
3 resolutions * 9 durations * 2 image * 2 audio * 2 tiers
+ 1 Draft resolution * 9 durations * 2 image * 2 audio
= 216 + 36 = 252

total = 60,096 + 252 = 60,348
```

## 能力矩阵

| Model | 支持 resolution | 显式 duration | smart/default | 图像 role | 参考视频 |
| --- | --- | --- | --- | --- | --- |
| `doubao-seedance-2-0-260128` | 480p, 720p, 1080p, 4k | 4..15 | `-1` / omitted | `reference_image` | 支持 |
| `doubao-seedance-2-0-fast-260128` | 480p, 720p | 4..15 | `-1` / omitted | `reference_image` | 支持 |
| `doubao-seedance-2-0-mini-260615` | 480p, 720p | 4..15 | `-1` / omitted | `reference_image` | 支持 |
| `doubao-seedance-1-5-pro-251215` | 480p, 720p, 1080p | 4..12 | `-1` / omitted | `first_frame` | 不支持 |

Fast/Mini 的 `1080p/4k`、1.5 Pro 的 reference image/video/audio、2.0 families 的 flex/Draft、1.5 Pro Draft 的非 480p/flex 均在非法矩阵中拒绝。`duration=-1` 的 fixture 终态为 7 秒；omitted duration 的 fixture 终态为 5 秒。

参考视频 profile 是有序 tuple。每段 URL 编码整数 duration `2..15`，最多 3 段，总时长不超过 15 秒。无视频状态单独覆盖。图像 present/absent 均覆盖；图像存在不改变单价档，视频 count/duration 也不产生本地倍率，只有 `hasVideo` 选择 video-input 单价。

能力边界必须区分输入音频与生成音频: reference audio 属于 Seedance 2.0 完整多模态能力，不属于 1.5 Pro；1.5 Pro 图像输入使用 `first_frame`，`generate_audio` 是输出音频控制和计费维度，不是 reference audio。

## 官方价格与 normalized ModelRatio

官方单价单位为 RMB/百万 completion tokens。new-api 中 `ModelRatio=1` 对应 RMB 14/百万 token。

| Family | Resolution | 无视频输入 | 有视频输入 |
| --- | --- | ---: | ---: |
| Seedance 2.0 | 480p | 46 | 28 |
| Seedance 2.0 | 720p | 46 | 28 |
| Seedance 2.0 | 1080p | 51 | 31 |
| Seedance 2.0 | 4k | 26 | 16 |
| Seedance 2.0 Fast | 480p | 37 | 22 |
| Seedance 2.0 Fast | 720p | 37 | 22 |
| Seedance 2.0 Mini | 480p | 23 | 14 |
| Seedance 2.0 Mini | 720p | 23 | 14 |
| Seedance 1.5 Pro | 480p/720p/1080p | 8 | 不支持视频输入 |

| Family | 测试配置 ModelRatio | 十进制 |
| --- | --- | ---: |
| Seedance 2.0 | `46/14` | 3.2857142857142856 |
| Seedance 2.0 Fast | `37/14` | 2.642857142857143 |
| Seedance 2.0 Mini | `23/14` | 1.6428571428571428 |
| Seedance 1.5 Pro | `8/14` | 0.5714285714285714 |

2.0 family 的 `video_input` ratio 等于 video price/base price，例如 1080p 为 `31/46`。Fast 为 `22/37`，Mini 为 `14/23`。无视频且价格等于 base 时不存冗余 ratio。

Seedance 1.5 Pro 的 base price 为 RMB 8/百万 token:

- `generate_audio=true`: `audio=2`。
- `service_tier=flex`: `service_tier=0.5`。
- Draft submit estimate: 有 audio 时 `draft_estimate=0.6`，无 audio 时 `0.7`。
- terminal: authoritative completion tokens 已代表实际输出，移除 `draft_estimate`。

## Mock 公式与边界

mock 对成功任务返回:

```text
completion_tokens = 100000
                  + effectiveDuration * 1000
                  + videoTotalSeconds * 100
                  + videoCount * 10
                  + imageFlag
```

其中 `imageFlag` 为 0 或 1；omitted duration 的 `effectiveDuration=5`，`duration=-1` 的 `effectiveDuration=7`。每段视频必须是 2..15 秒、总时长不超过 15 秒、最多 3 段。mock 故意返回 `total_tokens=completion_tokens+97`，所有终态结算只使用 `completion_tokens`。

此公式只让 case ID、媒体 tuple 和终态 token 可复算，绝不表示 ARK 真实 token 计算方式。

## 代表性完整链路数据

以下四个样例都来自实际 generator 的合法 case。为稳定展示，将随机 public ID 归一化为 `task_public_*`，顺序相关 mock ID 归一化为 `cgt-billing-1`，动态 `httptest.Server` origin 归一化为 `http://mock-ark.local`。模型、内容顺序、控制字段、usage、终态字段和计费数值保持测试断言值。测试环境未配置 alias，所以 model 映射结果为原值。

### Seedance 2.0: 1080p、2 秒参考视频、参考图、duration 4

Case ID: `doubao-seedance-2-0-260128/1080p/duration-04/video-true/image-true`。

```json
{
  "client_post": {
    "method": "POST",
    "path": "/api/v3/contents/generations/tasks",
    "body": {
      "model": "doubao-seedance-2-0-260128",
      "content": [
        {
          "type": "text",
          "text": "Seedance explicit billing acceptance doubao-seedance-2-0-260128/1080p/duration-04/video-true/image-true"
        },
        {
          "type": "image_url",
          "role": "reference_image",
          "image_url": {"url": "https://mock.example/reference.png"}
        },
        {
          "type": "video_url",
          "role": "reference_video",
          "video_url": {"url": "https://mock.example/reference-2s-1.mp4"}
        }
      ],
      "resolution": "1080p",
      "duration": 4
    }
  },
  "mock_upstream_received": {
    "model": "doubao-seedance-2-0-260128",
    "content": [
      {
        "type": "text",
        "text": "Seedance explicit billing acceptance doubao-seedance-2-0-260128/1080p/duration-04/video-true/image-true"
      },
      {
        "type": "image_url",
        "role": "reference_image",
        "image_url": {"url": "https://mock.example/reference.png"}
      },
      {
        "type": "video_url",
        "role": "reference_video",
        "video_url": {"url": "https://mock.example/reference-2s-1.mp4"}
      }
    ],
    "resolution": "1080p",
    "duration": 4
  },
  "mock_create_response": {"id": "cgt-billing-1"},
  "mock_terminal_response": {
    "id": "cgt-billing-1",
    "model": "doubao-seedance-2-0-260128",
    "status": "succeeded",
    "content": {"video_url": "http://mock-ark.local/videos/cgt-billing-1.mp4"},
    "usage": {"completion_tokens": 104211, "total_tokens": 104308},
    "created_at": 1780000000,
    "updated_at": 1780000001,
    "seed": 900013,
    "resolution": "1080p",
    "ratio": "16:9",
    "duration": 4,
    "framespersecond": 24,
    "service_tier": "default",
    "execution_expires_after": 172800,
    "generate_audio": true,
    "draft": false,
    "priority": 0
  },
  "public_create_response": {"id": "task_public_20"},
  "public_task_response": {
    "id": "task_public_20",
    "model": "doubao-seedance-2-0-260128",
    "status": "succeeded",
    "content": {"video_url": "http://mock-ark.local/videos/cgt-billing-1.mp4"},
    "usage": {"completion_tokens": 104211, "total_tokens": 104308},
    "created_at": 1780000000,
    "updated_at": 1780000001,
    "seed": 900013,
    "resolution": "1080p",
    "ratio": "16:9",
    "duration": 4,
    "framespersecond": 24,
    "service_tier": "default",
    "execution_expires_after": 172800,
    "generate_audio": true,
    "draft": false,
    "priority": 0
  }
}
```

这是完整的 prompt + `reference_image` + `reference_video` 样例。mock token 可复算为 `100000 + 4*1000 + 2*100 + 1*10 + 1 = 104211`。

### Seedance 2.0 Fast: 720p、无视频、无图、duration 5

Case ID: `doubao-seedance-2-0-fast-260128/720p/duration-05/video-false/image-false`。

```json
{
  "client_post": {
    "method": "POST",
    "path": "/api/v3/contents/generations/tasks",
    "body": {
      "model": "doubao-seedance-2-0-fast-260128",
      "content": [{"type": "text", "text": "Seedance explicit billing acceptance doubao-seedance-2-0-fast-260128/720p/duration-05/video-false/image-false"}],
      "resolution": "720p",
      "duration": 5
    }
  },
  "mock_upstream_received": {
    "model": "doubao-seedance-2-0-fast-260128",
    "content": [{"type": "text", "text": "Seedance explicit billing acceptance doubao-seedance-2-0-fast-260128/720p/duration-05/video-false/image-false"}],
    "resolution": "720p",
    "duration": 5
  },
  "mock_create_response": {"id": "cgt-billing-1"},
  "mock_terminal_response": {
    "id": "cgt-billing-1",
    "model": "doubao-seedance-2-0-fast-260128",
    "status": "succeeded",
    "content": {"video_url": "http://mock-ark.local/videos/cgt-billing-1.mp4"},
    "usage": {"completion_tokens": 105000, "total_tokens": 105097},
    "created_at": 1780000000,
    "updated_at": 1780000001,
    "seed": 900013,
    "resolution": "720p",
    "ratio": "16:9",
    "duration": 5,
    "framespersecond": 24,
    "service_tier": "default",
    "execution_expires_after": 172800,
    "generate_audio": true,
    "draft": false,
    "priority": 0
  },
  "public_create_response": {"id": "task_public_fast"},
  "public_task_response": {
    "id": "task_public_fast",
    "model": "doubao-seedance-2-0-fast-260128",
    "status": "succeeded",
    "content": {"video_url": "http://mock-ark.local/videos/cgt-billing-1.mp4"},
    "usage": {"completion_tokens": 105000, "total_tokens": 105097},
    "created_at": 1780000000,
    "updated_at": 1780000001,
    "seed": 900013,
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
}
```

### Seedance 2.0 Mini: 720p、2 秒参考视频、无图、duration 6

Case ID: `doubao-seedance-2-0-mini-260615/720p/duration-06/video-true/image-false`。

```json
{
  "client_post": {
    "method": "POST",
    "path": "/api/v3/contents/generations/tasks",
    "body": {
      "model": "doubao-seedance-2-0-mini-260615",
      "content": [
        {"type": "text", "text": "Seedance explicit billing acceptance doubao-seedance-2-0-mini-260615/720p/duration-06/video-true/image-false"},
        {"type": "video_url", "role": "reference_video", "video_url": {"url": "https://mock.example/reference-2s-1.mp4"}}
      ],
      "resolution": "720p",
      "duration": 6
    }
  },
  "mock_upstream_received": {
    "model": "doubao-seedance-2-0-mini-260615",
    "content": [
      {"type": "text", "text": "Seedance explicit billing acceptance doubao-seedance-2-0-mini-260615/720p/duration-06/video-true/image-false"},
      {"type": "video_url", "role": "reference_video", "video_url": {"url": "https://mock.example/reference-2s-1.mp4"}}
    ],
    "resolution": "720p",
    "duration": 6
  },
  "mock_create_response": {"id": "cgt-billing-1"},
  "mock_terminal_response": {
    "id": "cgt-billing-1",
    "model": "doubao-seedance-2-0-mini-260615",
    "status": "succeeded",
    "content": {"video_url": "http://mock-ark.local/videos/cgt-billing-1.mp4"},
    "usage": {"completion_tokens": 106210, "total_tokens": 106307},
    "created_at": 1780000000,
    "updated_at": 1780000001,
    "seed": 900013,
    "resolution": "720p",
    "ratio": "16:9",
    "duration": 6,
    "framespersecond": 24,
    "service_tier": "default",
    "execution_expires_after": 172800,
    "generate_audio": true,
    "draft": false,
    "priority": 0
  },
  "public_create_response": {"id": "task_public_mini"},
  "public_task_response": {
    "id": "task_public_mini",
    "model": "doubao-seedance-2-0-mini-260615",
    "status": "succeeded",
    "content": {"video_url": "http://mock-ark.local/videos/cgt-billing-1.mp4"},
    "usage": {"completion_tokens": 106210, "total_tokens": 106307},
    "created_at": 1780000000,
    "updated_at": 1780000001,
    "seed": 900013,
    "resolution": "720p",
    "ratio": "16:9",
    "duration": 6,
    "framespersecond": 24,
    "service_tier": "default",
    "execution_expires_after": 172800,
    "generate_audio": true,
    "draft": false,
    "priority": 0
  }
}
```

### Seedance 1.5 Pro: 720p、audio、flex、first_frame、duration 7

Case ID: `doubao-seedance-1-5-pro-251215/720p/duration-07/image-true/audio-true/tier-flex/draft-false`。

```json
{
  "client_post": {
    "method": "POST",
    "path": "/api/v3/contents/generations/tasks",
    "body": {
      "model": "doubao-seedance-1-5-pro-251215",
      "content": [
        {"type": "text", "text": "Seedance explicit billing acceptance doubao-seedance-1-5-pro-251215/720p/duration-07/image-true/audio-true/tier-flex/draft-false"},
        {"type": "image_url", "role": "first_frame", "image_url": {"url": "https://mock.example/reference.png"}}
      ],
      "resolution": "720p",
      "duration": 7,
      "generate_audio": true,
      "service_tier": "flex",
      "draft": false
    }
  },
  "mock_upstream_received": {
    "model": "doubao-seedance-1-5-pro-251215",
    "content": [
      {"type": "text", "text": "Seedance explicit billing acceptance doubao-seedance-1-5-pro-251215/720p/duration-07/image-true/audio-true/tier-flex/draft-false"},
      {"type": "image_url", "role": "first_frame", "image_url": {"url": "https://mock.example/reference.png"}}
    ],
    "resolution": "720p",
    "duration": 7,
    "generate_audio": true,
    "service_tier": "flex",
    "draft": false
  },
  "mock_create_response": {"id": "cgt-billing-1"},
  "mock_terminal_response": {
    "id": "cgt-billing-1",
    "model": "doubao-seedance-1-5-pro-251215",
    "status": "succeeded",
    "content": {"video_url": "http://mock-ark.local/videos/cgt-billing-1.mp4"},
    "usage": {"completion_tokens": 107001, "total_tokens": 107098},
    "created_at": 1780000000,
    "updated_at": 1780000001,
    "seed": 900013,
    "resolution": "720p",
    "ratio": "16:9",
    "duration": 7,
    "framespersecond": 24,
    "service_tier": "flex",
    "execution_expires_after": 172800,
    "generate_audio": true,
    "draft": false,
    "priority": 0
  },
  "public_create_response": {"id": "task_public_15"},
  "public_task_response": {
    "id": "task_public_15",
    "model": "doubao-seedance-1-5-pro-251215",
    "status": "succeeded",
    "content": {"video_url": "http://mock-ark.local/videos/cgt-billing-1.mp4"},
    "usage": {"completion_tokens": 107001, "total_tokens": 107098},
    "created_at": 1780000000,
    "updated_at": 1780000001,
    "seed": 900013,
    "resolution": "720p",
    "ratio": "16:9",
    "duration": 7,
    "framespersecond": 24,
    "service_tier": "flex",
    "execution_expires_after": 172800,
    "generate_audio": true,
    "draft": false,
    "priority": 0
  }
}
```

## ID 转换与 BillingContext

| 边界 | public/private ID |
| --- | --- |
| client create response | 只返回 `task_*` |
| 数据库 `Task.TaskID` | `task_*` |
| 数据库 `PrivateData.UpstreamTaskID` | `cgt-billing-*` |
| mock create/get | 使用 `cgt-billing-*` |
| public task get 顶层 `id` | 用 `task_*` 替换上游顶层 task ID |
| public task get `content.video_url` | 上游 opaque URL 原样透传，不重写；可能包含 provider 生成标识 |

ID 隔离只约束任务对象的顶层 `id`。create response 只返回 `task_*`，public task get 的顶层 `id` 也始终是 `task_*`。测试中的 `content.video_url` 刻意包含 `cgt-billing-1`，这是 mock 生成的上游 opaque URL；网关按协议透传 URL，不承诺清除 URL 内的 provider 标识。因此不能把顶层 ID 替换扩大解释为整个 public task response 不包含该字符串。

以下是 1.5 Pro Draft `480p + audio + default + first_frame + duration 7` 的字段观测快照。为展示断言，提交时 `billing_tokens=0` 和 `has_video_input=false` 都被手工展开；二者分别带有 `omitempty`，持久化 JSON 可以省略这些零值/false 值。该代码块是断言视图，不是持久化 JSON 的逐字快照。

```json
{
  "submit_billing_context": {
    "model_ratio": 0.5714285714285714,
    "group_ratio": 1,
    "other_ratios": {"audio": 2, "draft_estimate": 0.6},
    "origin_model_name": "doubao-seedance-1-5-pro-251215",
    "upstream_model_name": "doubao-seedance-1-5-pro-251215",
    "has_video_input": false,
    "generate_audio": true,
    "draft": true,
    "service_tier": "default",
    "resolution": "480p",
    "billing_tokens": 0
  },
  "terminal_billing_context": {
    "model_ratio": 0.5714285714285714,
    "group_ratio": 1,
    "other_ratios": {"audio": 2},
    "origin_model_name": "doubao-seedance-1-5-pro-251215",
    "upstream_model_name": "doubao-seedance-1-5-pro-251215",
    "has_video_input": false,
    "generate_audio": true,
    "draft": true,
    "service_tier": "default",
    "resolution": "480p",
    "billing_tokens": 107001
  }
}
```

这证明 submit estimate 包含 `draft_estimate=0.6`，terminal 使用 `BillingTokens=107001` 并移除 `draft_estimate`。非 Draft flex 样例的终态 `OtherRatios` 为 `{"audio":2,"service_tier":0.5}`。

## 预扣、终态和差额

测试公式:

```text
preconsume = trunc((ModelRatio / 2) * QuotaPerUnit * product(submit OtherRatios))
final      = trunc(completion_tokens * ModelRatio * product(terminal OtherRatios))
delta      = final - preconsume
QuotaPerUnit = 500000
```

PowerShell 按同一公式复核得到:

| Case | tokens | preconsume | final | delta |
| --- | ---: | ---: | ---: | ---: |
| 2.0 1080p, video 2s, image, duration 4 | 104211 | 553571 | 230752 | -322819 |
| Fast 720p, no video/image, duration 5 | 105000 | 660714 | 277500 | -383214 |
| Mini 720p, video 2s, no image, duration 6 | 106210 | 250000 | 106210 | -143790 |
| 1.5 Pro 720p, audio+flex+first_frame, duration 7 | 107001 | 142857 | 61143 | -81714 |

第一行可复算为:

```text
tokens = 100000 + 4*1000 + 2*100 + 1*10 + 1 = 104211
pre    = trunc((46/14)/2 * 500000 * (31/46)) = 553571
final  = trunc(104211 * (46/14) * (31/46)) = 230752
delta  = 230752 - 553571 = -322819
```

每个成功生命周期的 domain delta 统一满足:

| Domain | 精确变化 |
| --- | --- |
| task count / task quota | `+1 / +finalQuota` |
| user available / used / request count | `-finalQuota / +finalQuota / +1` |
| channel used quota | `+finalQuota` |
| token remain / used | `-finalQuota / +finalQuota` |
| quota_data Count / Quota / TokenUsed | `+1 / +finalQuota / +completionTokens` |
| signed logs (`consume-refund`) | `+finalQuota` |
| BillingContext BillingTokens | `+completionTokens` |

四个代表 case 的最终 user/channel/token/quota_data quota 变化分别为 230752、277500、106210 和 61143；`quota_data.TokenUsed` 分别为 104211、105000、106210 和 107001。负 delta 以 settlement refund log 实现，不回退请求计数。

本报告的 1,068 个成功 HTTP E2E case 全部是 `final < preconsume`，因此精确覆盖的是负 delta 和 settlement refund 路径，不声称该 HTTP matrix 覆盖正 delta 或零 delta。正 delta、负 delta、零 delta 三个分支由 service 层 deterministic unit tests 分别保护；三例都先持久化 task，再从数据库回读并断言 task quota，同时核对 user、channel、token、quota_data 和 log 的精确变化。zero delta 额外断言 token remain/used 维持 `5000/0` 不变。

日志只声明测试实际断言的字段。submit consume log 包含 `is_task=true`、`request_path=/v1/video/generations`、`model_ratio` 和 `group_ratio=1`。有差额时 settlement log 包含 `task_id`、`billing_tokens`、`model_ratio`、`group_ratio`、`has_video_input`、`resolution`、`service_tier_value`、`generate_audio`、`draft` 和最终倍率。报告不声称日志中存在未断言字段，也不把 `total_tokens` 当作账务字段。

## 非法输入与退款

invalid 分层不能合并计数:

- 36 个 adaptor direct invalid 验证 model/resolution/duration/media role/tier/Draft 能力边界。
- 38 个 public HTTP local invalid 均返回 HTTP 400，mock request count 不变，task/user/channel/token/quota_data/log snapshot 完全不变。
- 4 个 public HTTP upstream duration rejection 实际到达 mock，发生中间预扣，再在 HTTP 返回前完整退款。
- mock handler 自身另有 6 个 standalone negative fixtures，它们不是用户 HTTP 本地非法总数的一部分。

malformed boolean 不泄露 decoder detail。`generate_audio="not-a-bool"` 和 object 形式的 `draft` 均归一化为:

```json
{
  "error": {
    "code": "InvalidParameter",
    "message": "request body contains invalid parameters"
  }
}
```

### 1 秒视频的上游拒绝与完整退款

该 case 在本地能力验证后到达 mock。2.0 720p video tier 的中间预扣精确为 500000。mock 解析 URL 时拒绝 1 秒视频，返回 ARK HTTP 400 envelope。

```json
{
  "upstream_request": {
    "model": "doubao-seedance-2-0-260128",
    "content": [
      {"type": "text", "text": "upstream duration refund one-1s"},
      {"type": "video_url", "role": "reference_video", "video_url": {"url": "https://mock.example/reference-1s.mp4"}}
    ],
    "resolution": "720p",
    "duration": 5
  },
  "ark_http_400_response": {
    "error": {
      "code": "InvalidParameter.content",
      "message": "each video_url.url must encode a duration from 2 to 15 seconds"
    }
  },
  "intermediate_preconsume_delta": {
    "user_quota": -500000,
    "user_used_quota": 0,
    "user_request_count": 0,
    "token_remain_quota": -500000,
    "token_used_quota": 500000,
    "channel_used_quota": 0
  },
  "after_http_return_delta": {
    "task_count": 0,
    "user_quota": 0,
    "user_used_quota": 0,
    "user_request_count": 0,
    "channel_used_quota": 0,
    "token_remain_quota": 0,
    "token_used_quota": 0,
    "quota_data_count": 0,
    "quota_data_quota": 0,
    "quota_data_token_used": 0,
    "log_count": 0
  }
}
```

mock 收到请求时的 observer 明确看到 user quota 和 token remain quota 各减少 500000、token used 增加 500000；user used、channel used、request count 不变。HTTP 返回后所有 domain snapshot 等于 before，没有 task、log 或 quota_data 残留。另 3 个退款 case 是 16 秒单视频、8+8 秒双视频、6+5+5 秒三视频。

## 验收发现并修复的 production defect

1. Raw base double truncation。旧的 adjusted-ratio 重算从已截断的 `PriceData.Quota` 反推 base，再应用新倍率，可能发生不可逆的二次截断。回归 fixture 中旧中间值为 499999，正确结果应为 500000。现在从 raw `ModelRatio/ModelPrice * GroupRatio * QuotaPerUnit` 重建 base，全部倍率应用后只调用一次 quota conversion；`TestTaskQuotaWithOtherRatiosUsesRawBase` 和 `TestTaskRecalcQuotaFromRatiosUsesRawBase` 固化该行为。

2. `service_tier` log overwrite。旧日志组装先写 numeric `OtherRatios["service_tier"]=0.5`，随后又用 raw 字符串 `flex` 覆盖同一 key。现在 numeric ratio 保持为 `service_tier=0.5`，原始控制值单独写为 `service_tier_value=flex`；default tier 没有虚构 numeric ratio，只记录 `service_tier_value=default`。service 单测和 636-case HTTP matrix 均回归该字段。

同时修复了错误 decoder detail 泄露: malformed bool 不再返回 Go JSON unmarshal 内部信息，而统一为稳定的 `InvalidParameter` envelope。adaptor 单测和 38-case local HTTP matrix 覆盖回归。

## Fresh 命令证据

2026-07-19 在当前工作树执行:

```text
go test ./relay/channel/task/doubao -run SeedanceBillingAcceptance -count=1 -v
```

结果: PASS。4 个测试通过，其中 explicit matrix 内部精确断言 60,348，direct invalid matrix 精确断言 36。最终 fresh Go package reported time 为 0.517s。

```text
go test ./e2e -run 'TestSeedanceBilling(ExplicitMatrixE2E|DurationModesE2E|ReferenceVideoProfilesE2E|InvalidCombinationsE2E|UpstreamDurationRefundE2E)$' -count=1 -v
```

结果: PASS。实际日志:

```text
local_invalid_cases=38
upstream_refund_cases=4
duration_mode_cases=120
explicit_cases=636
reference_video_profiles=312
```

各测试 reported time 分别为 0.04s、0.01s、0.83s、12.56s、3.75s；Go package reported time 为 17.615s，PowerShell wall time 为 19.953s。成功 HTTP 生命周期合计 `120+636+312=1068`。

```text
go test ./service -run 'TestRecalculate_(PositiveDelta|NegativeDelta|ZeroDelta)$' -count=1 -v
```

结果: PASS，`TestRecalculate_PositiveDelta`、`TestRecalculate_NegativeDelta`、`TestRecalculate_ZeroDelta` 3/3 通过，Go package reported time 为 0.402s。输出没有 `WHERE conditions required` 或 quota 回写失败。三例先持久化 task，并回读数据库 task quota；断言还覆盖 user/channel/token/quota_data/log，其中 zero delta 的 token remain/used 保持 `5000/0`。该命令提供 service 层三种 delta 分支证据；它与 1,068 个仅覆盖负 delta 的成功 HTTP E2E 属于不同测试层级。

## Final verification

Task 10 已在当前工作树完成最终 fresh 验证。

```text
go test ./e2e -run SeedanceBilling -count=1 -v
```

结果: PASS，Go package reported time 为 20.013s。该范围包含全部 Seedance billing matrix、mock parser/handler、environment 和 domain snapshot tests；生命周期日志再次得到 `local_invalid_cases=38`、`upstream_refund_cases=4`、`duration_mode_cases=120`、`explicit_cases=636`、`reference_video_profiles=312`。

```text
go test ./relay/channel/task/doubao ./service ./e2e -count=1
```

结果: PASS。三个 package reported time 依次为 0.549s、0.737s、21.094s。

```text
go test ./...
```

结果: exit 0，wall time 约 17.9s，无失败输出。

```text
go vet ./relay/channel/task/doubao ./service ./e2e
git diff --check
```

结果: 两条命令均 exit 0、无输出。

两份目标 Markdown 已提取所有 fenced `json` blocks 并逐块执行 PowerShell `ConvertFrom-Json`: 清单 0 块、报告 7 块，共 7/7 通过，未完成 checkbox 为 0。`git diff --check` 退出码为 0；由于两个目标文件尚未跟踪，另对每个文件执行 `git diff --no-index --check -- NUL <file>`，没有 whitespace error。

## 残余风险

- 上游是本地 mock，不是真实 ARK；不验证真实网络、供应商可用性或真实响应漂移。
- 数据库只使用 SQLite in-memory，本轮未连接 MySQL 或 PostgreSQL。
- 测试不下载 reference media，只从 fixture URL final path 解析 duration。
- mock token 公式不验证、推断或复现 ARK 私有 token 算法。
- 测试没有真实供应商成本，也不能作为真实账单对账凭证。
