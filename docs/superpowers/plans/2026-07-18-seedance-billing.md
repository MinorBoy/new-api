# Seedance 计费精确化 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 按火山方舟 `ark-video-price.md` 对 Seedance 视频任务进行精确预扣和终态结算，覆盖模型、实际输出分辨率、是否含视频输入、`service_tier`、是否有声和 Draft 折算，并修复异步退款/差额结算后的用户、渠道及 `quota_data` 统计不守恒。

**Architecture:** 提交阶段由 doubao adaptor 根据映射后的上游模型和请求事实生成受控 `OtherRatios`，并把无法从响应反推的事实冻结到 `TaskBillingContext`。轮询成功后优先使用上游实际 `resolution` 修正单价档，使用官方指定的 `usage.completion_tokens`（缺失时才回退 `total_tokens`）通过现有 `common.QuotaFromFloatChecked` 链路结算。所有统计调整以任务提交小时为桶，差额只调整金额/token，不增加请求次数。

**Tech Stack:** Go 1.22+、GORM v2、testify（`require` + `assert`）、`common.QuotaFromFloatChecked`、`types.PriceData.AddOtherRatio`。

**Required reading:** 实施前必须完整阅读 `pkg/billingexpr/expr.md`。本功能保留 adaptor `OtherRatios`，因为最终分辨率只在异步响应阶段可知，不能只用请求时 billing expression 决定最终价格。

**References:** [PR #5387](https://github.com/QuantumNous/new-api/pull/5387)、[PR #4560](https://github.com/QuantumNous/new-api/pull/4560) 仅作参考；正式视频字段/模型能力以 `docs/channel/api-doc-doubao-video-generation.md` 为准，必须修复其模型映射、统计时桶和前端契约 review 问题后再采用。

---

## Review Decisions

本审定版替代原计划中的以下错误或缺口：

1. 价格查表使用 `info.UpstreamModelName`；`TaskBillingContext` 同时保存 origin 和 upstream 模型，结算优先使用 upstream。
2. 新增资料确认 Mini 的可调用模型 ID 为 `doubao-seedance-2-0-mini-260615`。价格匹配仍按稳定 family 前缀识别版本化 ID，但 `relay/channel/task/doubao/constants.go` 的 `ModelList` 必须显式加入该精确 ID，并由回归测试锁定。
3. 未知分辨率或不支持的模型/分辨率组合必须在请求校验阶段返回 400，不能静默按基准价少收。
4. Seedance 1.5 Pro family 的 `generate_audio` 默认值是 `true`。是否有声只看输出控制字段，输入 `audio_url` 不等于有声输出。
5. 1.5 Pro 同时支持 `service_tier=flex`（单价 0.5 倍）和 Draft token 预估折算（有声 0.6、无声 0.7）；原计划漏算这两个维度。Draft 折算已反映在上游实际 `completion_tokens` 中，终态结算前必须移除预估倍率，禁止重复折扣。
6. Seedance 2.0 系列当前不支持 `service_tier=flex`，请求阶段拒绝，而不是应用 0.5。
7. 官方说明准确 token 用量以 `usage.completion_tokens` 为准；不能无条件使用 `usage.total_tokens`。
8. 原生视频的模型能力校验以 `2026-07-18-ark-native-compat.md` 为入口；本计划在映射后再次拒绝会影响价格或预扣的模型/分辨率/服务等级组合，不能依赖上游返回 400。
9. 图片生成不使用本计划的视频 token 结算逻辑；其成功张数由原生图像计划按 `usage.generated_images` 收口。
10. `AdjustBillingOnComplete` 返回 0 时仍可先修正内存中的 BillingContext，再由通用 token 结算读取；但日志字段必须由 `taskBillingOther` 显式写入。
11. 退款函数当前签名是 `RefundTaskQuota(ctx, task, reason)` 且无返回值，计划不得编造 `error` 返回值或不存在的测试 helper。
12. `User.UsedQuota` 的负向调整不能调用会增加 `request_count` 的 `UpdateUserUsedQuotaAndRequestCount`；新增只调整 used quota 的 API。
13. `quota_data` 调整写入任务提交小时，并沿用 `group/token/channel/node` 维度；差额记录的 `Count` 必须是 0。
14. 不新增裸 `int(float64(...))`。预扣和终态结算继续使用带 clamp 的公共额度转换，并把 clamp 写入 `admin_info.quota_saturation`。

## Billing Baselines

管理员对每个模型配置“在线、无附加条件”的基准 `ModelRatio`：

| Family | 基准单价（元/百万 token） | 附加规则 |
|---|---:|---|
| Seedance 2.0 | 46 | 分辨率 × 视频输入 |
| Seedance 2.0 Fast | 37 | 视频输入；仅 480p/720p |
| Seedance 2.0 Mini | 23 | 视频输入；仅 480p/720p |
| Seedance 1.5 Pro | 8 | 有声 × flex × Draft |

2.0 精确档位：46/28（480p、720p），51/31（1080p），26/16（4k）。Fast 为 37/22，Mini 为 23/14。斜杠前后分别表示输入不含/包含视频。

1.5 Pro：有声单价倍率 2；`flex` 单价倍率 0.5；Draft 在预扣阶段使用有声 0.6、无声 0.7 的 `draft_estimate`。终态拿到权威 `completion_tokens` 后删除 `draft_estimate`，只保留单价倍率。

## File Structure

| 文件 | 责任 |
|---|---|
| `relay/channel/task/doubao/constants.go` | family 归一化、价格和请求组合校验 |
| `relay/channel/task/doubao/constants_test.go` | 官方价格矩阵测试 |
| `relay/channel/task/doubao/adaptor.go` | 请求事实、预扣倍率、响应权威字段、终态倍率修正 |
| `relay/channel/task/doubao/adaptor_test.go` | 预扣和终态结算测试 |
| `relay/channel/adapter.go` | 可选的映射后计费请求校验接口 |
| `relay/relay_task.go` | 模型映射后、预扣前执行计费组合校验 |
| `constant/context_key.go` | 传递任务计费事实 |
| `relay/common/relay_info.go` | `TaskInfo.Resolution` |
| `model/task.go` | 持久化计费快照 |
| `controller/relay.go` | 冻结 origin/upstream 模型和请求事实 |
| `service/task_polling.go` | 选择权威 billing token |
| `service/task_billing.go` | 差额结算、日志、统计守恒 |
| `service/task_billing_test.go` | 钱包/订阅/统计不变量测试 |
| `model/user.go` | used quota 有符号增量 API |
| `model/usedata.go` | `quota_data` 不增 count 的调整 API |
| `model/log.go` | 任务差额日志不再自行重复写 dashboard 统计 |

---

## Task 1: 价格 family 和完整倍率矩阵

**Files:**
- Modify: `relay/channel/task/doubao/constants.go`
- Create: `relay/channel/task/doubao/constants_test.go`

- [ ] **Step 1: 写失败的官方价格表测试**

使用确定性表驱动测试覆盖：

- 2.0 的 480p/720p/1080p/4k × 有无视频八个组合。
- Fast 和 Mini 的 480p/720p × 有无视频。
- Mini 的无后缀 ID 和任意日期后缀 ID 命中同一 family。
- `doubao-seedance-2-0-mini-260615` 必须出现在 `ModelList`，并命中 Mini 的 23/14 价格。
- `1080p`/`4k` 在 Fast/Mini 返回“不支持”，未知分辨率返回“不支持”。
- 1.5 Pro 有声/无声、default/flex、draft/non-draft 组合。
- 未知模型返回 `ok=false`。

断言使用 `assert.InDelta`，不增加只包装一次比较的手写 helper。

Run: `go test ./relay/channel/task/doubao -run SeedancePricing -v`

Expected: FAIL。

- [ ] **Step 2: 实现稳定 family 归一化**

先匹配更具体的 Fast/Mini，再匹配普通 2.0，避免前缀吞掉子 family：

```go
const (
	seedance20Family     = "seedance-2.0"
	seedance20FastFamily = "seedance-2.0-fast"
	seedance20MiniFamily = "seedance-2.0-mini"
	seedance15ProFamily  = "seedance-1.5-pro"
)

func seedancePricingFamily(modelName string) string {
	modelName = strings.ToLower(strings.TrimSpace(modelName))
	switch {
	case strings.HasPrefix(modelName, "doubao-seedance-2-0-fast"):
		return seedance20FastFamily
	case strings.HasPrefix(modelName, "doubao-seedance-2-0-mini"):
		return seedance20MiniFamily
	case strings.HasPrefix(modelName, "doubao-seedance-2-0"):
		return seedance20Family
	case strings.HasPrefix(modelName, "doubao-seedance-1-5-pro"):
		return seedance15ProFamily
	default:
		return ""
	}
}
```

在同一文件的 `ModelList` 中保留已有模型并追加官方确认的精确 ID：

```go
var ModelList = []string{
	"doubao-seedance-1-0-pro-250528",
	"doubao-seedance-1-0-lite-t2v",
	"doubao-seedance-1-0-lite-i2v",
	"doubao-seedance-1-5-pro-251215",
	"doubao-seedance-2-0-260128",
	"doubao-seedance-2-0-fast-260128",
	"doubao-seedance-2-0-mini-260615",
}
```

测试同时断言列表包含 `doubao-seedance-2-0-mini-260615`，价格函数对该 ID 和任意 `doubao-seedance-2-0-mini-*` 后缀返回同一 family。

- [ ] **Step 3: 实现 2.0 系列价格表**

用 `(family, resolution, hasVideo)` 查实际单价，再除以 family 基准价。空分辨率按官方默认 `720p`；只有大小写变体可归一化。未知值和未配置组合返回 `(0, false)`，由请求验证返回 400。

```go
type videoPricingTier struct {
	family     string
	resolution string
	hasVideo   bool
}

var videoPrices = map[videoPricingTier]float64{
	{seedance20Family, "480p", false}: 46,
	{seedance20Family, "480p", true}:  28,
	{seedance20Family, "720p", false}: 46,
	{seedance20Family, "720p", true}:  28,
	{seedance20Family, "1080p", false}: 51,
	{seedance20Family, "1080p", true}:  31,
	{seedance20Family, "4k", false}:    26,
	{seedance20Family, "4k", true}:     16,
	{seedance20FastFamily, "480p", false}: 37,
	{seedance20FastFamily, "480p", true}:  22,
	{seedance20FastFamily, "720p", false}: 37,
	{seedance20FastFamily, "720p", true}:  22,
	{seedance20MiniFamily, "480p", false}: 23,
	{seedance20MiniFamily, "480p", true}:  14,
	{seedance20MiniFamily, "720p", false}: 23,
	{seedance20MiniFamily, "720p", true}:  14,
}

var videoBasePrices = map[string]float64{
	seedance20Family: 46,
	seedance20FastFamily: 37,
	seedance20MiniFamily: 23,
}

func GetVideoBillingRatio(modelName, resolution string, hasVideo bool) (float64, bool) {
	family := seedancePricingFamily(modelName)
	base, baseOK := videoBasePrices[family]
	if !baseOK || base <= 0 {
		return 0, false
	}
	resolution = strings.ToLower(strings.TrimSpace(resolution))
	if resolution == "" {
		resolution = "720p"
	}
	if resolution != "480p" && resolution != "720p" && resolution != "1080p" && resolution != "4k" {
		return 0, false
	}
	price, ok := videoPrices[videoPricingTier{family: family, resolution: resolution, hasVideo: hasVideo}]
	if !ok {
		return 0, false
	}
	return price / base, true
}
```

- [ ] **Step 4: 实现 1.5 Pro 倍率**

稳定函数返回独立倍率，调用方通过 `PriceData.AddOtherRatio` 合并：

```go
func GetSeedance15ProRatios(generateAudio, draft bool, serviceTier string) (map[string]float64, bool) {
	serviceTier = strings.ToLower(strings.TrimSpace(serviceTier))
	if serviceTier == "" {
		serviceTier = "default"
	}
	if serviceTier != "default" && serviceTier != "flex" {
		return nil, false
	}

	ratios := make(map[string]float64)
	if generateAudio {
		ratios["audio"] = 2
	}
	if serviceTier == "flex" {
		ratios["service_tier"] = 0.5
	}
	if draft {
		if generateAudio {
			ratios["draft_estimate"] = 0.6
		} else {
			ratios["draft_estimate"] = 0.7
		}
	}
	return ratios, true
}
```

- [ ] **Step 5: 验证并提交**

Run: `go test ./relay/channel/task/doubao -run SeedancePricing -v`

```bash
git add relay/channel/task/doubao/constants.go relay/channel/task/doubao/constants_test.go
git commit -m "feat(seedance-billing): cover official pricing matrix"
```

---

## Task 2: 请求校验、预扣倍率和计费快照

**Files:**
- Modify: `constant/context_key.go`
- Modify: `relay/channel/adapter.go`
- Modify: `relay/relay_task.go`
- Modify: `relay/channel/task/doubao/adaptor.go`
- Modify: `relay/channel/task/doubao/adaptor_test.go`
- Modify: `model/task.go`
- Modify: `controller/relay.go`

- [ ] **Step 1: 写失败测试**

测试以下可观察行为：

- 模型映射 alias -> `doubao-seedance-2-0-260128` 时，按 upstream 模型命中 2.0 价格。
- 模型映射到精确 Mini ID `doubao-seedance-2-0-mini-260615` 时，按 Mini 价格；Mini/Fast 的 1080p、普通 2.0 的 4k 组合分别按支持/不支持处理。
- 2.0 1080p + 视频输入得到 `31/46`；4k 无视频得到 `26/46`。
- Fast/Mini 的 1080p、2.0 系列的 `service_tier=flex` 返回 400。
- 1.5 Pro 未传 `generate_audio` 按 `true`；显式 `false` 保留并按无声计费。
- 1.5 Pro `flex + draft + generate_audio=true` 同时得到 `audio=2`、`service_tier=0.5`、`draft_estimate=0.6`。
- 1.5 Pro `draft=true` 只允许 480p，且拒绝 `service_tier=flex` 和 `return_last_frame=true`；2.0/Fast/Mini 的 `draft=true` 返回 400，但这些模型的 `generate_audio` 仍按官方支持范围处理。
- 视频时长遵循原生计划的模型范围（1.0: 2~12、1.5: 4~12/-1、2.0: 4~15/-1），不能把 `duration=-1` 当成负的计费倍率。
- 预扣计算出的每个倍率最终都通过 `PriceData.AddOtherRatio`，不存在 0、负数、NaN 或 +Inf。

Run: `go test ./relay/channel/task/doubao -run SeedanceEstimateBilling -v`

Expected: FAIL。

- [ ] **Step 2: 增加请求事实 context key**

在 `constant/context_key.go` 添加：

```go
ContextKeyTaskVideoHasInput ContextKey = "task_video_has_input"
ContextKeyTaskGenerateAudio ContextKey = "task_generate_audio"
ContextKeyTaskDraft         ContextKey = "task_draft"
ContextKeyTaskServiceTier   ContextKey = "task_service_tier"
```

- [ ] **Step 3: 基础解析后、模型映射后分别校验**

`ValidateRequestAndSetAction` 只完成现有/ARK 原生基础解析和与模型无关的类型、范围校验。模型 mapping 在该方法之后才执行，因此不能只在这里校验 family 组合。

在 `relay/channel/adapter.go` 增加可选接口：

```go
type TaskBillingRequestValidator interface {
	ValidateBillingRequest(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError
}
```

在 `relay.RelayTaskSubmit` 调用 `helper.ModelMappedHelper` 成功后、`ModelPriceHelperPerCall` 和任何预扣之前执行：

```go
if validator, ok := adaptor.(channel.TaskBillingRequestValidator); ok {
	if taskErr := validator.ValidateBillingRequest(c, info); taskErr != nil {
		return nil, taskErr
	}
}
```

doubao adaptor 的 `ValidateBillingRequest` 从 `relaycommon.GetTaskRequest(c)` 读取 metadata，并使用已经映射完成的 `info.UpstreamModelName` 校验 family 组合。这样 alias 映射到 Fast/Mini 后也能在预扣前拒绝 1080p。

规则：

- 2.0：分辨率仅 480p/720p/1080p/4k，`service_tier` 仅空/default。
- Fast/Mini：分辨率仅 480p/720p，`service_tier` 仅空/default。
- 1.5 Pro：分辨率仅 480p/720p/1080p，`service_tier` 仅空/default/flex。
- `generate_audio` 和 `draft` 非 boolean 时返回 400；2.0/Fast/Mini 与 1.5 Pro 支持 `generate_audio`，但 `draft` 仅 1.5 Pro 支持，1.0 系列拒绝两者；1.5 Pro 的 `draft` 约束由原生校验执行。
- `resolution` 缺省按官方 720p 进入价格查表；`duration`、`frames`、`priority` 等已在原生校验中完成上限检查，计费 adaptor 不从 `metadata` 旁路字段读取第二份值。

- [ ] **Step 4: 预扣时记录请求事实并返回倍率**

`EstimateBilling` 必须使用 `info.UpstreamModelName`，为空才回退 `info.OriginModelName`。1.5 Pro 的 `generate_audio` 默认 true；不要根据 `content[].audio_url` 推断。

把原生校验归一化后的 `generate_audio` 以指针写入 `TaskBillingContext`，这样显式 `false` 不会因 `omitempty` 丢失；历史任务 `GenerateAudio == nil` 时，1.5 Pro 按官方默认 `true`，其他模型不应用声音倍率。

返回的 map 只包含倍率不为 1 的项；`RelayTaskSubmit` 已逐项调用 `info.PriceData.AddOtherRatio`，不得直接写 `PriceData` 内部 map。`draft_estimate` 只用于降低预扣，不能作为最终单价倍率保留。

- [ ] **Step 5: 扩展持久化快照**

在 `model.TaskBillingContext` 添加：

```go
UpstreamModelName string `json:"upstream_model_name,omitempty"`
HasVideoInput     bool   `json:"has_video_input,omitempty"`
GenerateAudio     *bool  `json:"generate_audio,omitempty"`
Draft             bool   `json:"draft,omitempty"`
ServiceTier       string `json:"service_tier,omitempty"`
Resolution        string `json:"resolution,omitempty"`
BillingTokens     int    `json:"billing_tokens,omitempty"`
```

在 `controller.RelayTask` 创建 `TaskBillingContext` 时追加：

```go
UpstreamModelName: relayInfo.UpstreamModelName,
HasVideoInput:     c.GetBool(string(constant.ContextKeyTaskVideoHasInput)),
GenerateAudio:     common.GetPointer(c.GetBool(string(constant.ContextKeyTaskGenerateAudio))),
Draft:             c.GetBool(string(constant.ContextKeyTaskDraft)),
ServiceTier:       c.GetString(string(constant.ContextKeyTaskServiceTier)),
```

保留 `OriginModelName` 用于用户日志，upstream 只用于供应商价格 family 查表。

- [ ] **Step 6: 验证并提交**

Run: `go test ./relay/channel/task/doubao -run SeedanceEstimateBilling -v`

Run: `go test ./controller ./model`

```bash
git add constant/context_key.go relay/channel/adapter.go relay/relay_task.go relay/channel/task/doubao/adaptor.go relay/channel/task/doubao/adaptor_test.go model/task.go controller/relay.go
git commit -m "feat(seedance-billing): snapshot mapped pricing context"
```

---

## Task 3: 响应分辨率和权威 token 结算

**Files:**
- Modify: `relay/common/relay_info.go`
- Modify: `relay/channel/task/doubao/adaptor.go`
- Modify: `relay/channel/task/doubao/adaptor_test.go`
- Modify: `service/task_polling.go`
- Modify: `service/task_polling_test.go`
- Modify: `service/task_billing.go`

- [ ] **Step 1: 写失败测试**

覆盖：

- `ParseTaskResult` 同时保留 `CompletionTokens`、`TotalTokens` 和 `Resolution`。
- `completion_tokens=1000,total_tokens=1200` 时结算使用 1000；completion 缺失时回退 1200。
- `completion_tokens=0,total_tokens=1200` 且字段存在时结算使用 0；不能用 `>0` 把明确的零值误判成缺失。负数或超过 `common.MaxQuota` 的上游 token 值必须在进入额度乘法前截断并记录异常。
- 提交按 720p 预扣、响应为 1080p 时，把 `video_input` 更新为 1080p 对应倍率，同时保留 `service_tier` 等单价倍率。
- 1.5 Pro Draft 成功并返回权威 completion tokens 时删除 `draft_estimate`，避免 token 已折算后再次乘 0.6/0.7。
- 模型 mapping 后，终态查表使用 `BillingContext.UpstreamModelName`；历史任务为空时回退 origin。
- 上游无 resolution 时保留预扣倍率。
- clamp 继续进入结算日志 `admin_info.quota_saturation`。

Run: `go test ./relay/channel/task/doubao ./service -run 'Seedance.*Complete|TaskBillingTokens' -v`

Expected: FAIL。

- [ ] **Step 2: `TaskInfo` 加实际分辨率**

在 `relay/common/relay_info.go` 的 `TaskInfo` 末尾添加：

```go
Resolution string `json:"resolution,omitempty"`
CompletionTokensPresent bool `json:"-"`
```

doubao `ParseTaskResult` 成功分支填入 `resTask.Resolution`，同时保留两个 usage 字段，并用 `gjson.GetBytes(respBody, "usage.completion_tokens").Exists()` 设置 `CompletionTokensPresent`。不要通过重新计算 `TotalTokens` 覆盖上游协议字段。

- [ ] **Step 3: 终态只替换视频价格倍率**

`AdjustBillingOnComplete` 选择模型：

```go
modelName := billingContext.UpstreamModelName
if modelName == "" {
	modelName = billingContext.OriginModelName
}
```

只对 2.0 family 更新 `video_input`。先把历史 `BillingContext.OtherRatios` 装入临时 `types.PriceData`，通过 `AddOtherRatio`/`OtherRatios()` 获得净化后的副本，再替换目标项，避免传播非法历史倍率。对所有 Seedance family，只要响应提供权威 completion/total tokens，就从最终倍率副本删除 `draft_estimate`，因为 Draft 折算已反映在 token 数中。把响应分辨率写入 `Resolution`，billing token 在 service 选定后写入 `BillingTokens`。

- [ ] **Step 4: 统一选择 billing token**

在 `service/task_polling.go` 增加稳定领域函数：

```go
func taskBillingTokens(taskResult *relaycommon.TaskInfo) int {
	if taskResult == nil {
		return 0
	}
	billingTokens := taskResult.TotalTokens
	if taskResult.CompletionTokensPresent {
		billingTokens = taskResult.CompletionTokens
	}
	// Preserve existing non-Doubao adaptors that only populate completion_tokens
	// while leaving total_tokens empty.
	if !taskResult.CompletionTokensPresent && taskResult.CompletionTokens > 0 && taskResult.TotalTokens == 0 {
		billingTokens = taskResult.CompletionTokens
	}
	if billingTokens < 0 {
		common.SysError("negative task billing token count; clamped to zero")
		return 0
	}
	if billingTokens > common.MaxQuota {
		common.SysError("task billing token count exceeds quota bound; clamped to common.MaxQuota")
		return common.MaxQuota
	}
	return billingTokens
}
```

`TaskInfo` 增加 `CompletionTokensPresent bool json:"-"`；doubao 解析时用 `gjson.GetBytes(respBody, "usage.completion_tokens").Exists()` 保存字段存在性。`taskBillingTokens` 对负数返回 0，对超过 `common.MaxQuota` 的值截断到 `common.MaxQuota` 并通过 `common.SysError` 记录。`settleTaskBillingOnComplete` 的 adaptor quota 分支和 token 重算分支都使用该值。不要覆盖 `TaskInfo.TotalTokens` 的协议含义。

具体调用顺序必须是：先调用 adaptor 的 `AdjustBillingOnComplete`（使其删除 `draft_estimate` / 更新 `video_input`），再将 `taskBillingTokens(taskResult)` 传给 `RecalculateTaskQuotaWithTokens` 或 `RecalculateTaskQuotaByTokens`。这样 token 结算读取的是已经修正的 `BillingContext`。

- [ ] **Step 5: 保留旧 API，新增带 token 的结算入口**

保留现有调用兼容性：

```go
func RecalculateTaskQuota(ctx context.Context, task *model.Task, actualQuota int, reason string, clamps ...*common.QuotaClamp) {
	recalculateTaskQuota(ctx, task, actualQuota, 0, reason, clamps...)
}

func RecalculateTaskQuotaWithTokens(ctx context.Context, task *model.Task, actualQuota, billingTokens int, reason string, clamps ...*common.QuotaClamp) {
	recalculateTaskQuota(ctx, task, actualQuota, billingTokens, reason, clamps...)
}
```

`RecalculateTaskQuotaByTokens` 调用带 token 版本；`settleTaskBillingOnComplete` 的 adaptor 正额度分支也必须传入同一个 `billingTokens`。额度计算仍为：

```go
actualQuota, clamp := common.QuotaFromFloatChecked(
	float64(billingTokens) * modelRatio * finalGroupRatio * otherMultiplier,
)
```

- [ ] **Step 6: 日志暴露权威结算事实**

`taskBillingOther` 加入 `resolution`、`billing_tokens`、`service_tier`；在 `BillingContext` 存在时同时记录 `has_video_input`、`draft`，并在 `GenerateAudio != nil` 时记录 `generate_audio`，不能因显式 `false` 或 0 而丢失事实。这些字段只在 backend log `other` 中记录；本计划不包含前端展示改造。

- [ ] **Step 7: 验证并提交**

Run: `go test ./relay/channel/task/doubao ./service -run 'Seedance.*Complete|TaskBillingTokens' -v`

```bash
git add relay/common/relay_info.go relay/channel/task/doubao/adaptor.go relay/channel/task/doubao/adaptor_test.go service/task_polling.go service/task_polling_test.go service/task_billing.go
git commit -m "feat(seedance-billing): settle by actual output and completion tokens"
```

---

## Task 4: 异步退款和差额结算守恒

**Files:**
- Modify: `model/user.go`
- Modify: `model/usedata.go`
- Modify: `model/log.go`
- Modify: `service/task_billing.go`
- Modify: `service/task_billing_test.go`

- [ ] **Step 1: 先扩展现有测试 fixture**

在现有 `service/task_billing_test.go` 中复用 `TestMain`、`truncate`、`seedUser`、`seedChannel`、`makeTask`，添加明确 helper：

```go
func seedUserWithUsage(t *testing.T, id, quota, usedQuota, requestCount int) {
	t.Helper()
	user := &model.User{
		Id: id, Username: "test_user", Quota: quota, UsedQuota: usedQuota,
		RequestCount: requestCount, Status: common.UserStatusEnabled,
	}
	require.NoError(t, model.DB.Create(user).Error)
}

func seedChannelWithUsage(t *testing.T, id int, usedQuota int64) {
	t.Helper()
	channel := &model.Channel{
		Id: id, Name: "test_channel", Key: "sk-test",
		Status: common.ChannelStatusEnabled, UsedQuota: usedQuota,
	}
	require.NoError(t, model.DB.Create(channel).Error)
}
```

同时添加 `getUserUsedQuota`、`getUserRequestCount`、`getChannelUsedQuota`。`TestMain` 的 AutoMigrate 添加 `&model.QuotaData{}`，`truncate` 清空 `quota_data` 和同步清空 `model.CacheQuotaData`。

- [ ] **Step 2: 写失败的不变量测试**

测试钱包和订阅两类资金来源：

- 全额失败退款：`Quota + UsedQuota` 总和不变，UsedQuota 和 Channel.UsedQuota 减少，request_count 不变。
- 正差额：钱包/订阅再扣，UsedQuota 和 Channel.UsedQuota 增加，request_count 不变。
- 负差额：钱包/订阅退款，UsedQuota 和 Channel.UsedQuota 减少，request_count 不变。
- 零差额但有 billing tokens：金额不动，`quota_data.token_used` 补齐，count 不变。
- `quota_data` 的调整命中任务提交小时以及相同 group/token/channel/node key。

直接在同步 `CacheQuotaData` 中断言，不使用 sleep、`Eventually` 或时序比较。

Run: `go test ./service -run 'RefundTaskQuota.*Conservation|RecalculateTaskQuota.*Conservation|QuotaData.*Task' -v`

Expected: FAIL。

- [ ] **Step 3: 新增只调整 UsedQuota 的 model API**

在 `model/user.go` 添加：

```go
func UpdateUserUsedQuotaDelta(id, delta int) {
	if delta == 0 {
		return
	}
	if common.BatchUpdateEnabled {
		addNewRecord(BatchUpdateTypeUsedQuota, id, delta)
		return
	}
	updateUserUsedQuota(id, delta)
}
```

它不修改 `request_count`，底层 `used_quota + ?` 同时支持正负值。

- [ ] **Step 4: 新增 `quota_data` 调整 API**

在 `model/usedata.go` 新增 `LogQuotaDataAdjust(params QuotaDataLogParams)`。它复用与 `LogQuotaData` 相同的小时归一化和维度 key，但创建的 `QuotaData.Count` 固定为 0，`Quota`/`TokenUsed` 接受有符号增量。

在 `service/task_billing.go` 新增 `taskAdjustQuotaData(task, quotaDelta, tokenDelta)`：时间优先 `task.SubmitTime`，再回退 `task.CreatedAt` 和当前时间；传入 `task.Group`、`TokenId`、`ChannelId`、`NodeName`，保证与提交日志落在同一桶。

- [ ] **Step 5: 修正 RefundTaskQuota**

资金来源退款成功、token 退款之后执行：

```go
model.UpdateUserUsedQuotaDelta(task.UserId, -quota)
if task.ChannelId > 0 {
	model.UpdateChannelUsedQuota(task.ChannelId, -quota)
}
taskAdjustQuotaData(task, -quota, 0)
```

保持现有函数无返回值和失败日志行为。

- [ ] **Step 6: 修正差额结算**

在内部 `recalculateTaskQuota` 的资金调整和 token 调整成功后，无论 delta 正负都执行：

```go
model.UpdateUserUsedQuotaDelta(task.UserId, quotaDelta)
if task.ChannelId > 0 {
	model.UpdateChannelUsedQuota(task.ChannelId, quotaDelta)
}
taskAdjustQuotaData(task, quotaDelta, billingTokens)
```

`quotaDelta == 0` 时仍在 `billingTokens > 0` 的情况下调用 `taskAdjustQuotaData(task, 0, billingTokens)`。

- [ ] **Step 7: 防止 dashboard 重复计数**

`model.RecordTaskBillingLog` 仅记录日志，不再自动调用 `LogQuotaData`。当前仓库的调用点只有 `service/task_billing.go`，dashboard 调整由上一步显式完成。新增测试确保正差额不会把 `QuotaData.Count` 从 1 增加到 2。

- [ ] **Step 8: 验证并提交**

Run: `go test ./service -run 'RefundTaskQuota|RecalculateTaskQuota|QuotaData' -v`

Run: `go test ./model -run 'UserUsedQuota|QuotaData' -v`

```bash
git add model/user.go model/usedata.go model/log.go service/task_billing.go service/task_billing_test.go
git commit -m "fix(task-billing): keep async quota statistics conserved"
```

---

## Task 5: 全链路回归

- [ ] **Step 1: doubao 价格和 adaptor 测试**

Run: `go test ./relay/channel/task/doubao -v`

- [ ] **Step 2: 任务轮询和计费测试**

Run: `go test ./service -run 'Task|Quota|Billing' -v`

- [ ] **Step 3: 额度数学回归**

Run: `go test ./common -run Quota -v`

Run: `go test ./relay/common -run 'Relay|Quota|Billing' -v`

- [ ] **Step 4: 静态检查**

Run: `go vet ./relay/channel/task/doubao ./relay/common ./service ./model ./controller`

- [ ] **Step 5: 全项目测试**

Run: `go test ./...`

- [ ] **Step 6: 手工对账**

使用 ARK 测试渠道验证以下任务，并对比日志中的 `billing_tokens`、倍率和最终 quota：

1. 2.0：720p 无视频、1080p 无视频、4k 有视频。
2. Fast/Mini：720p 有视频；1080p 在访问上游前返回 400。
3. 1.5 Pro：默认有声、显式无声、flex、有声 Draft、无声 Draft。
4. 提示词 `--rs 1080p` 但结构化请求未写 resolution：最终按响应 1080p 档结算。
5. 成功响应同时返回 completion/total 且数值不同时，最终使用 completion。
6. 失败退款后用户总额度、渠道 used quota 和提交小时 dashboard 桶守恒。

不使用 `git add -A`，不提交工作区中的无关文件。

## Execution Order

先完成 `2026-07-18-ark-native-compat.md`，再按本计划 Task 1-4 实施。本计划完成的判定条件是自动测试证明预扣、终态结算、退款、正差额和负差额均保持额度与统计不变量；仅手工观察日志不能替代这些测试。

## Project Module Reference

以下 new-api 项目模块路径属于受保护项目身份，实施时保持不变：

```go
"github.com/QuantumNous/new-api/model"
```
