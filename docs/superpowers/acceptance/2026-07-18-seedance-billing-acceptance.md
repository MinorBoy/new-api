# Seedance 计费验收清单

验收日期: 2026-07-19

验收边界: 本地 mock ARK、SQLite in-memory、真实 HTTP 路由和真实计费/任务/日志模型。测试不访问 ARK 网络、不下载媒体、不产生真实供应商成本，也不验证 ARK 私有 token 算法。

## 模型与能力

- [x] Seedance 2.0 `doubao-seedance-2-0-260128` 已覆盖。
- [x] Seedance 2.0 Fast `doubao-seedance-2-0-fast-260128` 已覆盖。
- [x] Seedance 2.0 Mini `doubao-seedance-2-0-mini-260615` 已覆盖。
- [x] Seedance 1.5 Pro `doubao-seedance-1-5-pro-251215` 已覆盖。
- [x] Seedance 2.0 已覆盖支持的 `480p/720p/1080p/4k`。
- [x] Seedance 2.0 Fast 和 Mini 已覆盖支持的 `480p/720p`，并拒绝 `1080p/4k`。
- [x] Seedance 1.5 Pro 已覆盖支持的 `480p/720p/1080p`。
- [x] Fast/Mini 的 `1080p/4k` 与 1.5 Pro Draft 的非 `480p` resolution 已由直接 adaptor 和本地 HTTP 非法矩阵覆盖。

## Duration

- [x] Seedance 2.0、Fast、Mini 的每个显式整数 duration `4..15` 已覆盖。
- [x] Seedance 1.5 Pro 的每个显式整数 duration `4..12` 已覆盖。
- [x] 每个模型族都覆盖 `duration=-1`，mock 终态确定为 7 秒。
- [x] 每个模型族都覆盖省略 duration，mock 终态确定为 5 秒。
- [x] `0`、`3`、超过模型上限和 `-2` 均在访问上游前被拒绝。

## 参考媒体

- [x] Seedance 2.0 三个模型族覆盖无参考视频状态。
- [x] 312 个 ordered reference-video profiles 全部通过 HTTP 提交与轮询，分布为 14 个单视频、78 个双视频、220 个三视频组合。
- [x] 每段 URL fixture duration 为 `2..15` 秒，总时长不超过 15 秒，视频数不超过 3。
- [x] 参考图 absent/present 两种状态均覆盖。
- [x] Seedance 2.0 三个模型族的参考图使用 `role=reference_image`。
- [x] Seedance 1.5 Pro 的图像输入使用 `role=first_frame`，并拒绝 `reference_image`。
- [x] Seedance 1.5 Pro 拒绝 reference video 和 reference audio；reference audio 只属于 Seedance 2.0 完整多模态能力边界。
- [x] 参考图存在不会改变 unit-price tier；视频数量和时长不会引入本地额外倍率，只有 `hasVideo` 选择 video-input 价格档。

## Seedance 1.5 Pro

- [x] `generate_audio=false/true` 均覆盖，audio 输出倍率为 2。
- [x] `service_tier=default/flex` 均覆盖，flex 倍率为 0.5。
- [x] 非 Draft 覆盖全部 `480p/720p/1080p`、audio 和 tier 组合。
- [x] Draft 覆盖合法的 `480p + default`、audio false/true 和 image absent/present 组合。
- [x] Draft 预扣使用 `draft_estimate=0.7`，有 audio 时使用 `0.6`。
- [x] 终态结算移除 `draft_estimate`，只保留终态有效倍率。
- [x] Draft 的非 480p 和 flex 组合在上游前被拒绝。

## Token 与结算

- [x] mock `usage.completion_tokens` 是终态计费权威值。
- [x] mock 故意令 `total_tokens=completion_tokens+97`，测试证明 `total_tokens` 不参与结算。
- [x] 预扣 quota 使用 normalized `ModelRatio` 与提交时 `OtherRatios`。
- [x] 终态 quota 使用 authoritative completion tokens 与最终 `OtherRatios`。
- [x] 1,068 个成功 HTTP E2E case 均精确覆盖负 delta，按 `final - preconsume` 生成 settlement refund，并完成账务对账。
- [x] 正/负/零 delta 均由 service 层 deterministic unit tests 独立保护；三例先持久化 task、回读并断言 task quota，同时核对 user/channel/token/quota_data/log；zero delta 的 token remain/used 保持 `5000/0`。
- [x] `BillingContext.BillingTokens` 从提交时 0 更新为终态 completion tokens。
- [x] public create response 和 public task response 的顶层 `id` 均为 `task_*`，create response 不泄露 private task ID。
- [x] `content.video_url` 作为上游 opaque URL 原样透传，网关不重写；URL 可以包含 provider 生成的 `cgt-billing-*` 标识。

## 账务守恒

- [x] 用户 available quota 变化为 `-finalQuota`，used quota 变化为 `+finalQuota`。
- [x] 用户 `RequestCount` 每个成功生命周期只增加 1，差额结算不重复计数。
- [x] channel used quota、token used quota 与 final quota 一致；token remain quota 反向变化。
- [x] `quota_data.Count/Quota/TokenUsed` 分别为 `1/finalQuota/completionTokens`。
- [x] task quota、consume/refund log signed quota 与 final quota 守恒。
- [x] settlement log 记录真实 `billing_tokens` 和最终倍率；numeric `service_tier=0.5` 与 raw `service_tier_value=flex` 分开保存。

## 非法输入与退款

- [x] 36 个 adaptor 直接非法组合全部拒绝。
- [x] 38 个 public HTTP 本地非法组合全部返回 HTTP 400，且 mock 未收到请求、domain snapshot 不变。
- [x] malformed `generate_audio`/`draft` decoder detail 统一为稳定 ARK envelope，未泄露 Go JSON 内部错误。
- [x] 4 个上游 reference-duration 拒绝场景均实际发生 500000 quota 中间预扣。
- [x] 上游拒绝后 user/channel/token/quota_data/log 全部恢复到请求前，且没有 task。
- [x] mock handler 自身的 6 个负例只验证测试上游，不计入 38 个 public HTTP 本地非法用例。

## 规模与成本

- [x] adaptor fast matrix 精确执行并断言 60,348 个合法显式组合。
- [x] 成功 HTTP 生命周期精确为 1,068 个: 636 explicit + 120 smart/omitted + 312 ordered video profiles。
- [x] mock token 公式只用于确定性验收，不声称复现 ARK 私有 token 算法。
- [x] 测试全程本地、无真实网络和供应商成本。

## Final verification

- [x] `go test ./relay/channel/task/doubao -run SeedanceBillingAcceptance -count=1 -v` 最终 fresh PASS。
- [x] `go test ./e2e -run SeedanceBilling -count=1 -v` 最终 fresh PASS，包含 billing matrix、mock、environment 和 snapshot tests。
- [x] `go test ./relay/channel/task/doubao ./service ./e2e -count=1` 相关包回归 PASS。
- [x] `go test ./...` 全仓后端回归 exit 0，无失败输出。
- [x] `go vet ./relay/channel/task/doubao ./service ./e2e` 与 `git diff --check` 均 exit 0、无输出。
- [x] 两份 Markdown 的 fenced JSON 共 7 块，7/7 可解析，未完成 checkbox 为 0。
