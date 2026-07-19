# Dimensio Seedance 2.0 per-duration 自动化 E2E 验收报告

## 1. 验收结论与边界

Dimensio 渠道的协议翻译、源模型时长计费、任务快照和终态账本通过 mock 自动化验收。应用 E2E 完整执行 3 个模型 × 4 个终态，共 12 个场景；协议 E2E 另外执行 3 个模型 × 成功/失败转换。

所有 Dimensio 提交和查询都指向测试进程内的 `httptest` server。每个应用场景精确捕获 2 次请求（1 次提交、1 次查询），公开任务查询不会再次访问上游。测试未连接 `https://jimeng.dimensio.cn`，没有使用真实 API Key，也没有消耗任何上游积分。

## 2. 验收清单

### 后端计费、配置和公开 API

- [x] `ratio`、`tiered_expr`、`per_duration` 三种 billing mode 可区分，Dimensio 销售规则使用 `BillingModePerDuration`。
- [x] `DurationPrice` 包含 USD 单价、`second` 单位、1 秒步长和 4 秒最低计费时长。
- [x] 计费按客户端源模型 `doubao-seedance-2-0-260128` 查找；映射后的三个 Dimensio 目标模型没有测试 `ModelRatio` 夹具。
- [x] 配置通过 `billing_setting.billing_mode` / `billing_setting.duration_price` 更新，模型列表 API 暴露结构化时长价格。
- [x] `TaskBillingContext` 冻结 billing mode、duration rule、`duration_source=request`、请求时长、计费时长和分辨率倍率。
- [x] `seconds` 与 `duration` 是 `per_duration` 保留倍率名，不进入 `OtherRatios`。
- [x] quota 用 `shopspring/decimal` 组成乘积并通过 `common.QuotaFromDecimal` 转换，没有裸 `int` 转换。
- [x] completed 保留精确预扣；failed 与 `-2011` 退回钱包、用户已用、渠道、Token 和 quota-data；`1057` 保持任务与预扣。
- [x] failed 与 `-2011` 退款日志保留完整 duration snapshot 和 `resolution_ratio`。

### 前端编辑器与公开价格目录

- [x] 管理端模型价格编辑器支持 `Per-duration`，可编辑 price/unit/rounding step/minimum duration，并能在 mode 切换时保持规则一致性。实现与回归测试由 `56effb89c` 及后续修复提交覆盖。
- [x] 六个前端 locale 包含时长计费文案。
- [x] 公开模型 API 返回 `billing_mode=per_duration` 与 `duration_price`，由 `09eb6531a` 的 controller 回归测试覆盖。
- [x] 公开价格目录单独展示 `Duration-based` 与 `/ second` 或 `/ minute`，不会误标 `/ request`；过滤、排序、卡片、表格和详情由 `e3147f15b`、`e47572aa4` 及其测试覆盖。

### Dimensio 协议

- [x] ARK 标准入口为 `POST /api/v3/contents/generations/tasks`，提交只返回 new-api 公开 `task_*` ID。
- [x] `doubao-seedance-2-0-260128` 经渠道映射变为三个允许的 Dimensio 模型。
- [x] 每个应用场景都提交 prompt + reference image + reference video + reference audio。
- [x] 参考素材转换为 `image_file_1`、`video_file_1`、`audio_file_1`，模式为 `omni_reference`。
- [x] `duration=6`、resolution、`ratio=16:9`、`intelligent_ratio=false`、`face_grid=true` 精确传递。
- [x] fast-vip/mini 只支持 720p；VIP 协议请求使用 `resolution="1080p"`，计费快照使用 `resolution_ratio=2.5`。
- [x] completed、failed、`-2011`、`1057` mock 结构按当前协议处理。
- [x] Dimensio 查询响应不包含 `duration`；提交时已校验的请求时长是唯一权威计费时长。
- [x] completed/failed 查询转换为精确 ARK success/error 结构，公开响应不泄漏 `dim-upstream`。

## 3. 精确 ARK SDK 请求

应用 E2E 的请求体如下。三个模型场景只替换渠道映射目标和 `resolution`（VIP 场景为 `1080p`）；客户端始终使用 Doubao 源模型。

```json
{
  "model": "doubao-seedance-2-0-260128",
  "content": [
    {
      "type": "image_url",
      "image_url": {"url": "https://mock.example/reference-image.jpg"},
      "role": "reference_image"
    },
    {
      "type": "video_url",
      "video_url": {"url": "https://mock.example/reference-video.mp4"},
      "role": "reference_video"
    },
    {
      "type": "audio_url",
      "audio_url": {"url": "https://mock.example/reference-audio.mp3"},
      "role": "reference_audio"
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

## 4. 精确 Dimensio 提交结构

VIP 1080p 场景实际捕获的 JSON body：

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

请求边界和 mock 提交响应：

```text
POST /v1/videos/generations
Authorization: Bearer mock-dimensio-key
```

```json
{"created": 1709123456, "task_id": "dim-upstream", "status": "pending"}
```

ARK 提交响应只包含公开 ID：

```json
{"id": "task_<new-api-public-id>"}
```

## 5. Mock 查询和任务结构

### completed

```json
{
  "task_id": "dim-upstream",
  "status": "completed",
  "progress": 100,
  "result": {"url": "https://mock.dimensio/video.mp4"}
}
```

内部任务保留公开/上游 ID 隔离、成功状态、原始查询数据和提交时计费快照：

```json
{
  "task_id": "task_public",
  "status": "SUCCESS",
  "private_data": {
    "upstream_task_id": "dim-upstream",
    "billing_context": {
      "billing_mode": "per_duration",
      "duration_source": "request",
      "requested_duration_seconds": 6,
      "billable_duration_seconds": 6,
      "other_ratios": {"resolution": 2.5}
    }
  },
  "data": {
    "task_id": "dim-upstream",
    "status": "completed",
    "progress": 100,
    "result": {"url": "https://mock.dimensio/video.mp4"}
  }
}
```

### failed

```json
{
  "task_id": "dim-upstream",
  "status": "failed",
  "error": "视频安全审核不通过，请重试",
  "error_code": "2043"
}
```

失败任务保持相同的 duration snapshot，内部状态为 `FAILURE`，并保存上面的完整失败查询 JSON 供 ARK 转换。

### `-2011` 资源过期

```json
{"code": -2011, "message": "task expired", "data": null}
```

该结构是失败终态并触发公共退款路径。

### `1057` 可重试限流

```json
{"code": 1057, "message": "request too frequent", "data": null}
```

该结构不结束任务、不退款，公开状态保持 `queued`，后续轮询可以继续。

以上四种查询结构均没有 `duration`。网关不从查询结果读取、推断或重算时长。

## 6. 精确 ARK 查询转换

协议 E2E 使用固定公开 ID 和时间戳，成功响应精确为：

```json
{
  "id": "task_public",
  "model": "doubao-seedance-2-0-260128",
  "status": "succeeded",
  "content": {"video_url": "https://mock.dimensio/video.mp4"},
  "usage": {},
  "created_at": 1709123456,
  "updated_at": 1709123556
}
```

普通失败响应精确为：

```json
{
  "id": "task_public",
  "model": "doubao-seedance-2-0-260128",
  "status": "failed",
  "content": {},
  "usage": {},
  "error": {
    "code": "2043",
    "message": "视频安全审核不通过，请重试"
  },
  "created_at": 1709123456,
  "updated_at": 1709123556
}
```

应用 E2E 对 `-2011` 的公开错误结构为：

```json
{
  "id": "task_<new-api-public-id>",
  "model": "doubao-seedance-2-0-260128",
  "status": "failed",
  "content": {},
  "usage": {},
  "error": {"code": "-2011", "message": "task expired"}
}
```

`1057` 查询后的公开结构为非终态：

```json
{
  "id": "task_<new-api-public-id>",
  "model": "doubao-seedance-2-0-260128",
  "status": "queued",
  "content": {},
  "usage": {}
}
```

所有公开结构均不包含 `dim-upstream`。

## 7. 成本、售价和计费公式

供应商最新成本口径：

| Dimensio 模型 | 720p | 1080p |
|---|---:|---:|
| `jimeng-video-seedance-2.0-fast-vip` | 48 points/s | 不支持 |
| `jimeng-video-seedance-2.0-mini` | 39 points/s | 不支持 |
| `jimeng-video-seedance-2.0-vip` | 62 points/s | 155 points/s |

`1 point = CNY 0.01`。供应商以实际生成时长消耗 points；但查询 API 不返回实际 `duration`，系统销售计费明确冻结请求时长：

```text
billing_mode = per_duration
duration_source = request
requested_duration_seconds = 6
billable_duration_seconds = 6
OtherRatios = {resolution: 1 or 2.5}
OtherRatios does not contain seconds or duration
```

以默认 `CNY/USD = 7.3` 将 720p 成本换成用户销售 USD/秒基价：

| 模型 | CNY/s | USD/s `DurationPrice` | resolution ratio |
|---|---:|---:|---:|
| fast-vip 720p | 0.48 | `0.48 / 7.3 = 0.06575342465753424` | 1 |
| mini 720p | 0.39 | `0.39 / 7.3 = 0.05342465753424658` | 1 |
| vip 1080p | 720p base 0.62 | `0.62 / 7.3 = 0.08493150684931507` | 2.5 |

测试组倍率为 1，`common.QuotaPerUnit = 500000`：

```text
chargeUSD = duration_price * billable_duration_seconds * resolution_ratio * group_ratio
quotaDecimal = chargeUSD * QuotaPerUnit
quota = common.QuotaFromDecimal(quotaDecimal)
```

因此精确 quota 为 fast-vip `197260`、mini `160274`、VIP 1080p `636986`。

## 8. 12 场景应用 E2E 矩阵

初始钱包为 `2000000000`。`used/channel/token/quota-data` 列依次表示用户已用额度、渠道已用额度、Token 已用额度和 quota-data quota。失败退款后任务记录仍保留原预扣 `task.quota`，用于审计。

| 模型 | resolution | base USD/s | ratio | quota | 终态 | final wallet | used/channel/token/quota-data | refund log |
|---|---:|---:|---:|---:|---|---:|---|---:|
| fast-vip | 720p | 0.06575342465753424 | 1 | 197260 | completed | 1999802740 | 197260 / 197260 / 197260 / 197260 | 0 |
| fast-vip | 720p | 0.06575342465753424 | 1 | 197260 | failed | 2000000000 | 0 / 0 / 0 / 0 | 197260 |
| fast-vip | 720p | 0.06575342465753424 | 1 | 197260 | -2011 | 2000000000 | 0 / 0 / 0 / 0 | 197260 |
| fast-vip | 720p | 0.06575342465753424 | 1 | 197260 | 1057 queued | 1999802740 | 197260 / 197260 / 197260 / 197260 | 0 |
| mini | 720p | 0.05342465753424658 | 1 | 160274 | completed | 1999839726 | 160274 / 160274 / 160274 / 160274 | 0 |
| mini | 720p | 0.05342465753424658 | 1 | 160274 | failed | 2000000000 | 0 / 0 / 0 / 0 | 160274 |
| mini | 720p | 0.05342465753424658 | 1 | 160274 | -2011 | 2000000000 | 0 / 0 / 0 / 0 | 160274 |
| mini | 720p | 0.05342465753424658 | 1 | 160274 | 1057 queued | 1999839726 | 160274 / 160274 / 160274 / 160274 | 0 |
| vip | 1080p | 0.08493150684931507 | 2.5 | 636986 | completed | 1999363014 | 636986 / 636986 / 636986 / 636986 | 0 |
| vip | 1080p | 0.08493150684931507 | 2.5 | 636986 | failed | 2000000000 | 0 / 0 / 0 / 0 | 636986 |
| vip | 1080p | 0.08493150684931507 | 2.5 | 636986 | -2011 | 2000000000 | 0 / 0 / 0 / 0 | 636986 |
| vip | 1080p | 0.08493150684931507 | 2.5 | 636986 | 1057 queued | 1999363014 | 636986 / 636986 / 636986 / 636986 | 0 |

每个场景还断言 request count 为 1、quota-data count 为 1、mock call count 为 2。failed 与 `-2011` 的退款日志包含：

```json
{
  "billing_mode": "per_duration",
  "duration_source": "request",
  "requested_duration_seconds": 6,
  "billable_duration_seconds": 6,
  "duration_unit": "second",
  "rounding_step_seconds": 1,
  "minimum_duration_seconds": 4,
  "resolution_ratio": 1
}
```

VIP 1080p 的 `resolution_ratio` 为 `2.5`。

## 9. 实际执行命令与结果

TDD RED：先替换时长计费断言、保留 legacy fixture，确认测试在正确边界失败。

```text
go test ./e2e -run 'TestDimensioSeedance20MultimodalLifecycleE2E/fast_vip_720p/success' -count=1 -v
FAIL: HTTP 400 model_price_error; origin model doubao-seedance-2-0-260128 had no duration pricing fixture
```

补入源模型 `config.UpdateConfigFromMap` fixture 后：

```text
go test ./relay/channel/task/dimensio -run 'TestDimensioSeedance20ProtocolE2E|TestDurationBillingUsesValidatedRequestDuration' -count=1 -v
PASS: 1 duration contract + 6 protocol model/outcome subtests

go test ./e2e -run TestDimensioSeedance20MultimodalLifecycleE2E -count=1 -v
PASS: 12/12 application model/outcome scenarios

go test ./setting/billing_setting ./relay ./service ./controller -run 'TestDimensioDurationPriceDefaults|TestDimensioDurationBillingUsesOriginModelPrice|TestTaskBillingOtherIncludesDurationSnapshot|TestListModelsIncludesDurationBillingModel' -count=1 -v
PASS: all 4 focused backend contracts

gofmt -w e2e/seedance_native_e2e_test.go relay/channel/task/dimensio/e2e_test.go
PASS: exit 0

go vet ./relay/channel/task/dimensio ./e2e
PASS: exit 0, no output

git diff --check
PASS: exit 0, no whitespace errors
```

前端定向测试尝试结果：

```text
bun test src/features/system-settings/models/model-pricing-duration.test.ts src/features/pricing/lib/duration-pricing.test.ts src/features/pricing/components/model-card-duration.test.tsx
NOT RUN: bun executable is not installed or available on PATH in this environment
```

本报告不把未执行的前端命令记录为 PASS。前端验收项的证据来自已提交的实现及对应测试文件；完整重新执行留给具备 Bun 的前端验证环境。

## 10. 剩余风险

- 未调用真实 Dimensio，因此真实 API Key、网络、媒体抓取、生成质量和 points 结算未验证。
- 参考视频总时长属于远端媒体属性；网关不下载媒体时由上游执行该限制。
- `result.url` 的真实有效期和刷新行为未做时间型测试。
- 查询协议缺少实际生成 `duration`。系统选择请求时长作为明确且可审计的销售计费契约，这不是从查询响应补齐的值。
