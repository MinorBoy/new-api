# Seedance 计费精确化 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让 doubao-seedance 系列视频模型按火山方舟(ARK)官方价格表 `ark-video-price.md` 精确计费——支持 3 个维度(模型 × 输出分辨率 × 是否含视频输入)的全档位倍率,并以上游响应的实际分辨率为准结算(防请求侧旁路),同时修复异步任务退款的额度守恒 bug。

**Architecture:** 以社区 PR #5387 的"响应分辨率结算"架构为骨架(`AdjustBillingOnComplete` 用 `taskResult.Resolution` 重算 `video_input` 倍率),按官方文档 `ark-video-price.md` 补全价格表(#5387 缺 4k 档、mini 模型、1.5-pro 声音维度),并从 PR #4560 摘取异步任务退款额度守恒修复(普适 bug,与计费倍率无关)。

**Tech Stack:** Go 1.22+, GORM v2, testify(require+assert),JSON 统一走 `common.*`。计费安全遵循 AGENTS.md:`common.QuotaFromFloat`/`QuotaRound` 集中转换,饱和用 `*Checked` 变体。

**参考基线:**
- [PR #5387](https://github.com/QuantumNous/new-api/pull/5387) `feat/seedance-resolution-billing`(响应分辨率结算架构,17 文件,含前端)
- [PR #4560](https://github.com/QuantumNous/new-api/pull/4560) `fix/seedance-price`(额度守恒修复,8 文件)
- 官方价格表:`ark-video-price.md`(项目根目录)

**依赖:** 本计划假设 `2026-07-18-ark-native-compat.md` 已合并或至少 Task 5(adaptor 改动)已落地,因为两者都改 `relay/channel/task/doubao/adaptor.go` 的 `ParseTaskResult`。

---

## Scope

✅ 价格表按官方文档全档位补全(4k/mini/1.5-pro 声音)
✅ 响应分辨率结算(防 `--rs 1080p` 旁路,采纳 #5387 架构)
✅ 异步任务退款额度守恒修复(摘自 #4560)
❌ 前端退款对账展示(#5387 的前端 i18n 改动,非计费核心,可后续独立 PR)
❌ 完整复刻 #5387 前端日志"计费过程"展示

---

## 官方价格表(计费依据,来自 ark-video-price.md)

| 模型 | 分辨率 | 输入不含视频 | 输入含视频 | 计费维度 |
|---|---|---|---|---|
| doubao-seedance-2.0 | 480p/720p | 46 | 28 | 分辨率×视频 |
| doubao-seedance-2.0 | 1080p | 51 | 31 | 分辨率×视频 |
| doubao-seedance-2.0 | 4k | 26 | 16 | 分辨率×视频 |
| doubao-seedance-2.0-fast | 480p/720p | 37 | 22 | 分辨率×视频(不支持1080/4k) |
| doubao-seedance-2.0-mini | 480p/720p | 23 | 14 | 分辨率×视频(不支持1080/4k) |
| doubao-seedance-1.5-pro | - | 有声16/无声8 | - | **声音维度**(非分辨率) |
| doubao-seedance-1.0-pro | - | 15 | - | 单价 |
| doubao-seedance-1.0-pro-fast | - | 4.2 | - | 单价 |

**关键规则(官方文档 line 44-48):**
- 仅对成功生成的视频计费,审核失败不收费
- 准确 token 用量以 `usage.completion_tokens` 为准
- token 用量公式:`(输入视频时长+输出视频时长) × 宽 × 高 × 帧率 / 1024`
- 含视频输入时有最低 token 用量限制(低于则按最低计费)

---

## File Structure

| 文件 | 责任 | 来源 |
|---|---|---|
| `relay/common/relay_info.go` | TaskInfo 加 `Resolution` 字段 | #5387 |
| `relay/channel/task/doubao/constants.go` | 重写价格表(全档位)+ `GetVideoBillingRatio` | #5387 + 官方文档补全 |
| `relay/channel/task/doubao/constants_test.go` | 价格表测试(新增) | 新建 |
| `relay/channel/task/doubao/adaptor.go` | ParseTaskResult 回填 Resolution + AdjustBillingOnComplete + 1.5-pro 声音分支 | #5387 + 1.5-pro 扩展 |
| `service/task_billing.go` | 退款额度守恒修复 | #4560 摘取 |

---

## Task 1: TaskInfo 加 Resolution 字段(基础设施)

**Files:**
- Modify: `relay/common/relay_info.go:772-782`(TaskInfo 结构体)

- [ ] **Step 1: 加字段**

修改 `relay/common/relay_info.go` 的 `TaskInfo` 结构体,在 `TotalTokens` 之后追加:

```go
type TaskInfo struct {
	Code             int    `json:"code"`
	TaskID           string `json:"task_id"`
	Status           string `json:"status"`
	Reason           string `json:"reason,omitempty"`
	Url              string `json:"url,omitempty"`
	RemoteUrl        string `json:"remote_url,omitempty"`
	Progress         string `json:"progress,omitempty"`
	CompletionTokens int    `json:"completion_tokens,omitempty"` // 用于按倍率计费
	TotalTokens      int    `json:"total_tokens,omitempty"`      // 用于按倍率计费
	Resolution       string `json:"resolution,omitempty"`        // 上游实际输出分辨率,用于按真实分辨率结算
}
```

- [ ] **Step 2: 验证编译**

Run: `go build ./relay/common/`
Expected: 无输出

- [ ] **Step 3: Commit**

```bash
git add relay/common/relay_info.go
git commit -m "feat(seedance-billing): add Resolution field to TaskInfo for response-based settlement"
```

---

## Task 2: 重写价格表(全档位,按官方文档)

**Files:**
- Modify: `relay/channel/task/doubao/constants.go`
- Test: `relay/channel/task/doubao/constants_test.go`(新建)

> 这是计费核心,从纯函数 `GetVideoBillingRatio` 开始 TDD。

- [ ] **Step 1: 写失败测试(2.0 全档位)**

创建 `relay/channel/task/doubao/constants_test.go`:

```go
package doubao

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ratioEqual 比较两个 float64 倍率是否相等(允许 1e-6 误差,处理 28/46 这类除法)
func ratioEqual(t *testing.T, expected, actual float64) {
	t.Helper()
	assert.True(t, math.Abs(expected-actual) < 1e-6,
		"ratio mismatch: expected %v, got %v", expected, actual)
}

// TestSeedance20AllTiers 覆盖 doubao-seedance-2-0-260128 全部分辨率档×视频档
// 价格依据 ark-video-price.md:基准 46(480p/720p 不含视频)
func TestSeedance20AllTiers(t *testing.T) {
	const model = "doubao-seedance-2-0-260128"
	const base = 46.0

	cases := []struct {
		name       string
		resolution string
		hasVideo   bool
		price      float64 // 官方单价
	}{
		{"480p_no_video", "480p", false, 46},
		{"720p_no_video", "720p", false, 46},
		{"480p_with_video", "480p", true, 28},
		{"720p_with_video", "720p", true, 28},
		{"1080p_no_video", "1080p", false, 51},
		{"1080p_with_video", "1080p", true, 31},
		{"4k_no_video", "4k", false, 26},     // ★ 官方有,#5387 缺
		{"4k_with_video", "4k", true, 16},    // ★ 官方有,#5387 缺
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ratio, ok := GetVideoBillingRatio(model, tc.resolution, tc.hasVideo)
			require.True(t, ok, "model %s must have pricing config", model)
			ratioEqual(t, tc.price/base, ratio)
		})
	}
}

// TestSeedance20FastTiers 覆盖 doubao-seedance-2-0-fast-260128(不支持 1080p/4k)
func TestSeedance20FastTiers(t *testing.T) {
	const model = "doubao-seedance-2-0-fast-260128"
	const base = 37.0

	// 480p/720p 两档
	ratio, ok := GetVideoBillingRatio(model, "720p", false)
	require.True(t, ok)
	ratioEqual(t, 37.0/base, ratio)

	ratio, ok = GetVideoBillingRatio(model, "480p", true)
	require.True(t, ok)
	ratioEqual(t, 22.0/base, ratio)
}

// TestSeedance20MiniTiers 覆盖 doubao-seedance-2-0-mini(★ 官方有,所有 PR 都缺)
// 官方价格:不含视频 23,含视频 14,不支持 1080p/4k
func TestSeedance20MiniTiers(t *testing.T) {
	// 注意:实际模型名常量需先在 ModelList 注册(见 Task 2 Step 5)
	// 这里用官方文档模型名,执行时以 ARK 实际发布的模型 id 为准
	const model = "doubao-seedance-2-0-mini-XXXXXX" // ★ 待确认实际后缀,见 Task 2 Step 5 说明
	const base = 23.0

	ratio, ok := GetVideoBillingRatio(model, "720p", false)
	require.True(t, ok, "mini model must be configured")
	ratioEqual(t, 23.0/base, ratio)

	ratio, ok = GetVideoBillingRatio(model, "720p", true)
	require.True(t, ok)
	ratioEqual(t, 14.0/base, ratio)
}
```

- [ ] **Step 2: 运行测试,验证失败**

Run: `go test ./relay/channel/task/doubao/ -run TestSeedance20 -v`
Expected: FAIL(当前函数名是 `GetVideoInputRatio` 而非 `GetVideoBillingRatio`,且无 4k/mini 配置)

- [ ] **Step 3: 重写价格表(替换现有 videoPriceTable + GetVideoInputRatio)**

修改 `relay/channel/task/doubao/constants.go`,**完全替换** line 16-56 的 `videoPriceKey`/`videoPriceTable`/`GetVideoInputRatio`:

```go
package doubao

import "strings"

var ModelList = []string{
	"doubao-seedance-1-0-pro-250528",
	"doubao-seedance-1-0-lite-t2v",
	"doubao-seedance-1-0-lite-i2v",
	"doubao-seedance-1-5-pro-251215",
	"doubao-seedance-2-0-260128",
	"doubao-seedance-2-0-fast-260128",
	// ★ 新增(需确认实际模型名,见 Step 5)
	// "doubao-seedance-2-0-mini-XXXXXX",
}

var ChannelName = "doubao-video"

// videoPricing 单个模型的分辨率×视频档位价格表。
// base 为基准价(480p/720p 不含视频),其他档位以 base 为 1.0 折算倍率。
type videoPricing struct {
	base    float64                          // 基准单价(元/百万token)
	tiers   map[pricingTier]float64          // 各档位实际单价
}

// pricingTier 分辨率 × 是否含视频输入 的组合键
type pricingTier struct {
	resolution string  // "480p"/"720p"(基准)/"1080p"/"4k"
	hasVideo   bool
}

// videoPricingMap 严格依据 ark-video-price.md(2026-01-28 官方价格表)。
// 未登记的模型(seedance-1.x)跳过倍率,由管理员配 ModelRatio 全额计费。
var videoPricingMap = map[string]videoPricing{
	// doubao-seedance-2.0:支持 480p/720p/1080p/4k 全档
	"doubao-seedance-2-0-260128": {
		base: 46,
		tiers: map[pricingTier]float64{
			{"480p", false}: 46,  {"480p", true}: 28,
			{"720p", false}: 46,  {"720p", true}: 28,
			{"1080p", false}: 51, {"1080p", true}: 31,
			{"4k", false}: 26,    {"4k", true}: 16,
		},
	},
	// doubao-seedance-2.0-fast:不支持 1080p/4k
	"doubao-seedance-2-0-fast-260128": {
		base: 37,
		tiers: map[pricingTier]float64{
			{"480p", false}: 37, {"480p", true}: 22,
			{"720p", false}: 37, {"720p", true}: 22,
		},
	},
	// ★ doubao-seedance-2.0-mini(待确认实际模型名后取消注释)
	// 官方价格:不含视频 23,含视频 14,不支持 1080p/4k
	// "doubao-seedance-2-0-mini-XXXXXX": {
	// 	base: 23,
	// 	tiers: map[pricingTier]float64{
	// 		{"480p", false}: 23, {"480p", true}: 14,
	// 		{"720p", false}: 23, {"720p", true}: 14,
	// 	},
	// },
}

// normalizeResolution 把 "480P"/"1080p"/"4K" 等大小写变体统一为标准形式。
// 空或未知值回退为 "720p"(豆包默认输出分辨率)。
func normalizeResolution(r string) string {
	switch strings.ToLower(strings.TrimSpace(r)) {
	case "480p":
		return "480p"
	case "1080p":
		return "1080p"
	case "4k":
		return "4k"
	default:
		return "720p"
	}
}

// GetVideoBillingRatio 返回指定模型在(输出分辨率, 是否含视频输入)下相对基准价的计费倍率。
// 第二返回值表示该模型是否配置了价格表;未配置时返回 (0, false),调用方应保持原 ModelRatio 全额计费。
// 倍率为 1.0 表示命中基准档,调用方可忽略(不产生 OtherRatio)。
func GetVideoBillingRatio(modelName, resolution string, hasVideo bool) (float64, bool) {
	p, ok := videoPricingMap[modelName]
	if !ok || p.base <= 0 {
		return 0, false
	}
	res := normalizeResolution(resolution)
	price, ok := p.tiers[pricingTier{resolution: res, hasVideo: hasVideo}]
	if !ok {
		// 未配置组合(如 fast 传 1080p):上游会自行拒绝,这里按基准价计费
		price = p.base
	}
	return price / p.base, true
}
```

- [ ] **Step 4: 运行 2.0 和 fast 测试,验证通过(mini 测试暂时跳过)**

Run: `go test ./relay/channel/task/doubao/ -run "TestSeedance20AllTiers|TestSeedance20FastTiers" -v`
Expected: PASS

Run: `go test ./relay/channel/task/doubao/ -run TestSeedance20MiniTiers -v`
Expected: FAIL(mini 未配置,这是预期,等 Step 5)

- [ ] **Step 5: 确认 mini 模型名并补全**

**执行前必须先确认** `doubao-seedance-2.0-mini` 的实际模型 id。官方文档 `ark-video-price.md` 只写 "doubao-seedance-2.0-mini",但 new-api 的模型名约定带日期后缀(如 `-260128`)。

确认方式(任选其一):
1. 查火山方舟控制台的模型列表
2. 用 ARK API 查询:`GET https://ark.cn-beijing.volces.com/api/v3/models`
3. 临时先不注册 mini,跳过 `TestSeedance20MiniTiers`,等模型名确认后单独补 PR

**若已确认模型名**(假设是 `doubao-seedance-2-0-mini-260128`):

1. 在 `ModelList` 取消注释该行(填实际名)
2. 在 `videoPricingMap` 取消注释 mini 配置(填实际名)
3. 在 `constants_test.go` 把 `TestSeedance20MiniTiers` 的 `const model` 改为实际名

Run: `go test ./relay/channel/task/doubao/ -run TestSeedance20MiniTiers -v`
Expected: PASS

- [ ] **Step 6: 运行全部价格表测试**

Run: `go test ./relay/channel/task/doubao/ -run TestSeedance20 -v`
Expected: 全部 PASS(mini 待 Step 5 确认后)

- [ ] **Step 7: Commit**

```bash
git add relay/channel/task/doubao/constants.go relay/channel/task/doubao/constants_test.go
git commit -m "feat(seedance-billing): full-tier pricing per ark-video-price.md (4k/fast/mini)"
```

---

## Task 3: ParseTaskResult 回填 Resolution + AdjustBillingOnComplete(响应分辨率结算)

**Files:**
- Modify: `relay/channel/task/doubao/adaptor.go`
- Test: `relay/channel/task/doubao/adaptor_test.go`(已在原生入口计划创建,这里追加)

- [ ] **Step 1: 写失败测试(ParseTaskResult 回填 Resolution)**

在 `relay/channel/task/doubao/adaptor_test.go` 追加:

```go
// TestParseTaskResultFillsResolution 验证 succeeded 时 Resolution 被回填到 TaskInfo
// 这是响应分辨率结算的前提:ParseTaskResult 必须把上游实际分辨率传给结算阶段
func TestParseTaskResultFillsResolution(t *testing.T) {
	adaptor := &TaskAdaptor{}
	body := []byte(`{"id":"t1","status":"succeeded","resolution":"1080p","content":{"video_url":"https://x/v.mp4"},"usage":{"completion_tokens":1000,"total_tokens":1200}}`)

	result, err := adaptor.ParseTaskResult(body)
	require.NoError(t, err)
	require.Equal(t, model.TaskStatusSuccess, result.Status)
	assert.Equal(t, "1080p", result.Resolution, "Resolution must be filled from upstream response")
	assert.Equal(t, 1000, result.CompletionTokens)
	assert.Equal(t, 1200, result.TotalTokens)
}
```

- [ ] **Step 2: 运行,验证失败**

Run: `go test ./relay/channel/task/doubao/ -run TestParseTaskResultFillsResolution -v`
Expected: FAIL(当前 succeeded 分支未填 Resolution)

- [ ] **Step 3: 实现 Resolution 回填**

修改 `relay/channel/task/doubao/adaptor.go` 的 `ParseTaskResult`,在 `case "succeeded":` 分支追加一行:

```go
	case "succeeded":
		taskResult.Status = model.TaskStatusSuccess
		taskResult.Progress = "100%"
		taskResult.Url = resTask.Content.VideoURL
		taskResult.CompletionTokens = resTask.Usage.CompletionTokens
		taskResult.TotalTokens = resTask.Usage.TotalTokens
		taskResult.Resolution = resTask.Resolution // ★ 新增:回填上游实际分辨率供结算
```

- [ ] **Step 4: 运行,验证通过**

Run: `go test ./relay/channel/task/doubao/ -run TestParseTaskResultFillsResolution -v`
Expected: PASS

- [ ] **Step 5: 写失败测试(AdjustBillingOnComplete 按响应分辨率重算)**

在 `adaptor_test.go` 追加:

```go
// TestAdjustBillingOnCompleteUsesResponseResolution 验证结算时用响应分辨率(而非请求分辨率)
// 防 --rs 1080p 旁路:即使请求侧解析不到 1080p,响应返回 1080p 仍按 1080p 计费
func TestAdjustBillingOnCompleteUsesResponseResolution(t *testing.T) {
	adaptor := &TaskAdaptor{}

	// 模拟任务提交时按 720p 预扣(请求侧),但上游实际输出 1080p
	task := &model.Task{
		Properties: model.Properties{OriginModelName: "doubao-seedance-2-0-260128"},
		PrivateData: model.TaskPrivateData{
			BillingContext: &model.TaskBillingContext{
				OriginModelName: "doubao-seedance-2-0-260128",
				HasVideoInput:   false,
				OtherRatios:     map[string]float64{}, // 预扣时 720p 基准,无 video_input
			},
		},
	}
	// 上游响应返回 1080p
	taskResult := &relaycommon.TaskInfo{
		Status:     model.TaskStatusSuccess,
		Resolution: "1080p",
		TotalTokens: 1200,
	}

	// AdjustBillingOnComplete 返回 0(不直接给额度),而是修正 BillingContext.OtherRatios
	quota := adaptor.AdjustBillingOnComplete(task, taskResult)
	assert.Equal(t, 0, quota, "must return 0, let generic token-based settlement handle it")

	// 验证 OtherRatios 被修正为 1080p 档(51/46)
	ratio, ok := task.PrivateData.BillingContext.OtherRatios["video_input"]
	require.True(t, ok, "video_input ratio must be set for 1080p")
	assert.InDelta(t, 51.0/46.0, ratio, 1e-6)

	// Resolution 和 TotalTokens 被记录到 BillingContext(供日志展示)
	assert.Equal(t, "1080p", task.PrivateData.BillingContext.Resolution)
	assert.Equal(t, 1200, task.PrivateData.BillingContext.TotalTokens)
}
```

- [ ] **Step 6: 运行,验证失败**

Run: `go test ./relay/channel/task/doubao/ -run TestAdjustBillingOnCompleteUsesResponseResolution -v`
Expected: FAIL(当前 AdjustBillingOnComplete 是 BaseBilling 的 no-op,返回 0 但不修正 OtherRatios)

- [ ] **Step 7: 实现 AdjustBillingOnComplete**

首先,`TaskBillingContext` 需要加 `HasVideoInput`/`Resolution`/`TotalTokens` 字段(若 #5387 未合并,需手动加)。修改 `model/task.go` 的 `TaskBillingContext`:

```go
type TaskBillingContext struct {
	ModelPrice      float64           `json:"model_price,omitempty"`
	GroupRatio      float64           `json:"group_ratio,omitempty"`
	ModelRatio      float64           `json:"model_ratio,omitempty"`
	OtherRatios     map[string]float64 `json:"other_ratios,omitempty"`
	OriginModelName string            `json:"origin_model_name,omitempty"`
	PerCallBilling  bool              `json:"per_call_billing,omitempty"`
	// ★ 新增(计费精确化)
	HasVideoInput bool   `json:"has_video_input,omitempty"` // 输入是否含视频(请求侧事实)
	Resolution    string `json:"resolution,omitempty"`     // 结算时上游实际输出分辨率(日志展示)
	TotalTokens   int    `json:"total_tokens,omitempty"`   // 结算时上游 total_tokens(日志展示)
}
```

> **注意:** 需先 grep `model/task.go` 确认 `TaskBillingContext` 的实际定义位置和现有字段,按实际为准。

然后,在 `relay/channel/task/doubao/adaptor.go` 实现 `AdjustBillingOnComplete`(覆盖 BaseBilling 的 no-op):

```go
// AdjustBillingOnComplete 在任务完成时,用上游"实际输出分辨率"重算 video_input 倍率。
// 这样结算严格按真实出片分辨率计费,而非提交时请求的分辨率,防 --rs 1080p 等请求侧旁路。
// "是否含视频输入"是请求侧事实(响应无法反推),取自冻结的 BillingContext.HasVideoInput。
// 返回 0:不直接给最终额度,而是修正 BillingContext.OtherRatios 后交由通用 token 重算结算,
// 复用其差额结算与日志记录逻辑。
func (a *TaskAdaptor) AdjustBillingOnComplete(task *model.Task, taskResult *relaycommon.TaskInfo) int {
	if task == nil || taskResult == nil {
		return 0
	}
	bc := task.PrivateData.BillingContext
	if bc == nil {
		return 0
	}
	// 上游未返回分辨率时不覆盖,保留提交时按请求分辨率冻结的倍率
	actualResolution := taskResult.Resolution
	if actualResolution == "" {
		return 0
	}
	ratio, ok := GetVideoBillingRatio(bc.OriginModelName, actualResolution, bc.HasVideoInput)
	if !ok {
		return 0
	}
	if bc.OtherRatios == nil {
		bc.OtherRatios = map[string]float64{}
	}
	if ratio == 1.0 {
		delete(bc.OtherRatios, "video_input")
	} else {
		bc.OtherRatios["video_input"] = ratio
	}
	bc.Resolution = actualResolution
	bc.TotalTokens = taskResult.TotalTokens
	return 0
}
```

同时需要修改 `EstimateBilling`(预扣阶段)在冻结 BillingContext 时记录 `HasVideoInput`。定位 `relay/relay_task.go` 中 `BillingContext` 构造处(controller/relay.go:587-594 或 relay_task.go 类似位置),在构造时追加:

```go
task.PrivateData.BillingContext = &model.TaskBillingContext{
	// ... 现有字段 ...
	HasVideoInput: seedanceHasVideoInput(relayInfo), // ★ 新增
}
```

并实现辅助函数(放在 `relay/channel/task/doubao/adaptor.go`):

```go
// HasVideoInputForBilling 导出供 relay_task.go 在冻结 BillingContext 时调用,
// 判断请求是否含视频输入(从 metadata.content 检测 video_url)。
func HasVideoInputForBilling(metadata map[string]interface{}) bool {
	return hasVideoInMetadata(metadata)
}
```

> **注意:** `HasVideoInput` 的设置点需 grep `BillingContext` 的构造位置确认。若实现复杂,可简化:在 `EstimateBilling` 返回时同时通过其他渠道传递。执行时核实最优路径。

- [ ] **Step 8: 运行,验证通过**

Run: `go test ./relay/channel/task/doubao/ -run TestAdjustBillingOnCompleteUsesResponseResolution -v`
Expected: PASS

- [ ] **Step 9: Commit**

```bash
git add relay/channel/task/doubao/adaptor.go relay/channel/task/doubao/adaptor_test.go model/task.go
# 视实际改动文件增减
git commit -m "feat(seedance-billing): response-resolution settlement via AdjustBillingOnComplete"
```

---

## Task 4: 1.5-pro 声音维度计费(独立分支)

**Files:**
- Modify: `relay/channel/task/doubao/adaptor.go`(EstimateBilling 增加 1.5-pro 声音分支)

> **关键差异:** 官方文档 line 33-36 明确 `doubao-seedance-1.5-pro` 按"是否有声"计费(有声 16/无声 8),**不按分辨率**。两个 PR(#5387/#4560)都错误地把它纳入分辨率矩阵。

- [ ] **Step 1: 写失败测试(1.5-pro 有声/无声倍率)**

在 `constants_test.go` 追加:

```go
// TestSeedance15ProAudioTiers 验证 1.5-pro 按声音维度计费(非分辨率)
// 官方:有声 16,无声 8,基准取无声 8
func TestSeedance15ProAudioTiers(t *testing.T) {
	const model = "doubao-seedance-1-5-pro-251215"

	// 有声:16/8 = 2.0
	ratio, ok := GetSeedance15ProAudioRatio(true)
	require.True(t, ok)
	ratioEqual(t, 16.0/8.0, ratio)

	// 无声:8/8 = 1.0(基准,不产生 OtherRatio)
	ratio, ok = GetSeedance15ProAudioRatio(false)
	require.True(t, ok)
	ratioEqual(t, 1.0, ratio)
}
```

- [ ] **Step 2: 运行,验证失败**

Run: `go test ./relay/channel/task/doubao/ -run TestSeedance15ProAudioTiers -v`
Expected: FAIL(`GetSeedance15ProAudioRatio` 未定义)

- [ ] **Step 3: 实现 1.5-pro 声音倍率函数**

在 `constants.go` 追加:

```go
// GetSeedance15ProAudioRatio 返回 doubao-seedance-1.5-pro 按声音维度的计费倍率。
// 官方:有声 16 元/百万token,无声 8 元/百万token。基准取无声(8),有声倍率 2.0。
// hasAudio 为 true 表示生成有声视频(generate_audio=true 或响应含音频)。
func GetSeedance15ProAudioRatio(hasAudio bool) (float64, bool) {
	if hasAudio {
		return 16.0 / 8.0, true
	}
	return 1.0, true
}
```

- [ ] **Step 4: 运行,验证通过**

Run: `go test ./relay/channel/task/doubao/ -run TestSeedance15ProAudioTiers -v`
Expected: PASS

- [ ] **Step 5: 在 EstimateBilling 增加 1.5-pro 分支**

修改 `adaptor.go` 的 `EstimateBilling`:

```go
func (a *TaskAdaptor) EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64 {
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil
	}

	// ★ 1.5-pro 走声音维度,不走分辨率矩阵
	if strings.HasPrefix(info.OriginModelName, "doubao-seedance-1-5-pro") {
		hasAudio := seedanceHasAudioInRequest(req)
		ratio, ok := GetSeedance15ProAudioRatio(hasAudio)
		if !ok || ratio == 1.0 {
			return nil
		}
		return map[string]float64{"audio": ratio}
	}

	// 其他 2.x 系列走分辨率×视频矩阵
	hasVideo := hasVideoInMetadata(req.Metadata)
	resolution, _ := req.Metadata["resolution"].(string)
	ratio, ok := GetVideoBillingRatio(info.OriginModelName, resolution, hasVideo)
	if !ok || ratio == 1.0 {
		return nil
	}
	return map[string]float64{"video_input": ratio}
}

// seedanceHasAudioInRequest 判断 1.5-pro 请求是否生成有声视频。
// 判断依据:metadata.generate_audio == true(官方字段),或 content[] 含 audio_url。
func seedanceHasAudioInRequest(req relaycommon.TaskSubmitReq) bool {
	if req.Metadata != nil {
		if genAudio, ok := req.Metadata["generate_audio"]; ok {
			if b, ok := toBool(genAudio); ok {
				return b
			}
		}
	}
	// content[] 含 audio_url 也认为需要音频
	if content, ok := req.Metadata["content"].([]interface{}); ok {
		for _, item := range content {
			if m, ok := item.(map[string]interface{}); ok {
				if m["type"] == "audio_url" {
					return true
				}
			}
		}
	}
	return false
}

// toBool 容忍 JSON 反序列化后 bool 可能是 bool 或 float64(json.Number)
func toBool(v interface{}) (bool, bool) {
	switch x := v.(type) {
	case bool:
		return x, true
	case float64:
		return x != 0, true
	case string:
		return x == "true" || x == "1", true
	}
	return false, false
}
```

- [ ] **Step 6: 验证编译 + 运行全部计费测试**

Run: `go build ./relay/channel/task/doubao/`
Run: `go test ./relay/channel/task/doubao/ -v`
Expected: 全部 PASS

- [ ] **Step 7: Commit**

```bash
git add relay/channel/task/doubao/adaptor.go relay/channel/task/doubao/constants.go relay/channel/task/doubao/constants_test.go
git commit -m "feat(seedance-billing): 1.5-pro audio-dimension billing per official doc"
```

---

## Task 5: 异步任务退款额度守恒修复(摘自 #4560)

**Files:**
- Modify: `service/task_billing.go`(及可能的 model 层)

> **背景(#4560 描述):** `RefundTaskQuota`/`RecalculateTaskQuota` 只动了资金来源(钱包/订阅)与令牌额度,但没有同步回退 `User.UsedQuota`/`Channel.UsedQuota`/`quota_data` 看板统计。导致视频任务失败退款或事后差额结算时:`UsedQuota` 永远偏高、渠道用量偏高、看板与实际收费不一致。这是**普适 bug**,对所有视频渠道有效。

- [ ] **Step 1: 定位现有退款函数**

Run: `grep -n "func.*RefundTaskQuota\|func.*RecalculateTaskQuota\|func.*SettleTaskBilling" service/task_billing.go`

记录函数签名和当前实现。

- [ ] **Step 2: 写失败测试(退款后 UsedQuota 守恒)**

创建 `service/task_billing_test.go`(若已存在则追加):

```go
package service

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRefundTaskQuotaDecrementsUsedQuota 验证退款时 User.UsedQuota / Channel.UsedQuota 同步回退
// 这是 #4560 修复的核心不变量:退款后"总额度 = Quota + UsedQuota"必须守恒
func TestRefundTaskQuotaDecrementsUsedQuota(t *testing.T) {
	// Setup:初始化 DB(SQLite 内存)、用户、渠道、任务记录
	// (参考 model 包现有测试的 DB 初始化方式)
	setupTestDB(t)
	user := setupTestUser(t, usedQuota: 1000, quota: 500)
	channel := setupTestChannel(t, usedQuota: 1000)
	task := setupTestTask(t, userId: user.Id, channelId: channel.Id, quota: 200)

	// Act:退款 200
	err := RefundTaskQuota(task, 200, "test refund")
	require.NoError(t, err)

	// Assert:UsedQuota 同步回退
	refreshedUser := getUser(t, user.Id)
	assert.Equal(t, int64(800), refreshedUser.UsedQuota, "User.UsedQuota must decrease by refund amount")

	refreshedChannel := getChannel(t, channel.Id)
	assert.Equal(t, int64(800), refreshedChannel.UsedQuota, "Channel.UsedQuota must decrease by refund amount")

	// 看板数据(quota_data)也同步回退(若适用)
	// ... 视实际 quota_data 表结构补充 ...
}
```

> **注意:** 测试的具体 setup 函数(`setupTestDB`/`setupTestUser` 等)需参考 `model/` 包现有测试风格。执行时先 grep `model/*_test.go` 找 DB 初始化的 helper。

- [ ] **Step 3: 运行,验证失败**

Run: `go test ./service/ -run TestRefundTaskQuotaDecrementsUsedQuota -v`
Expected: FAIL(当前 RefundTaskQuota 未回退 UsedQuota)

- [ ] **Step 4: 摘取 #4560 的修复**

获取 #4560 的 task_billing.go 改动:

```bash
gh pr diff 4560 > /tmp/4560.diff
# 手动审查 /tmp/4560.diff 中 service/task_billing.go 的改动,
# 摘取 RefundTaskQuota/RecalculateTaskQuota 里同步 UsedQuota/Channel.UsedQuota/quota_data 的部分
```

按 #4560 的实现方式,在 `RefundTaskQuota`/`RecalculateTaskQuota` 中追加 `User.UsedQuota`/`Channel.UsedQuota`/`quota_data` 的同步回退逻辑。**只摘取额度守恒部分,不摘取 #4560 的 constants.go 价格表**(我们用 Task 2 的版本)。

具体改动参照 #4560 diff,核心模式:

```go
func RefundTaskQuota(task *model.Task, quota int, reason string) error {
	// ... 现有退款逻辑(钱包/订阅/令牌额度) ...

	// ★ 摘自 #4560:同步回退统计字段,保证额度守恒
	if err := model.DecreaseUserUsedQuota(task.UserId, quota); err != nil {
		return err
	}
	if err := model.DecreaseChannelUsedQuota(task.ChannelId, quota); err != nil {
		return err
	}
	// quota_data 看板同步(若 #4560 有)
	// ...
	return nil
}
```

> **注意:** `model.DecreaseUserUsedQuota`/`DecreaseChannelUsedQuota` 函数需确认是否存在,若 #4560 新增了这些函数,一并摘取。执行时以 #4560 diff 为准。

- [ ] **Step 5: 运行,验证通过**

Run: `go test ./service/ -run TestRefundTaskQuotaDecrementsUsedQuota -v`
Expected: PASS

- [ ] **Step 6: 补 RecalculateTaskQuota 的守恒测试(差额结算场景)**

类似 Step 2,写一个测试验证 `RecalculateTaskQuota`(事后差额结算)也同步 UsedQuota。具体参考 #4560 的测试用例(`gh pr view 4560 --json files` 找测试文件)。

- [ ] **Step 7: Commit**

```bash
git add service/task_billing.go service/task_billing_test.go model/*.go
# 视实际改动文件增减
git commit -m "fix(task-billing): sync User/Channel UsedQuota on async task refund/settlement (#4560)"
```

---

## Task 6: 整体回归 + 端到端计费验证

- [ ] **Step 1: 运行所有计费相关测试**

Run: `go test ./relay/channel/task/doubao/ ./service/ -v -run "Seedance|TaskQuota|Billing"`
Expected: 全部 PASS

- [ ] **Step 2: 全项目测试无回归**

Run: `go test ./... 2>&1 | tail -30`
Expected: 无新增 FAIL

- [ ] **Step 3: go vet**

Run: `go vet ./relay/channel/task/doubao/ ./service/`
Expected: 无 warning

- [ ] **Step 4: 端到端计费验证(手动,需 ARK 渠道)**

配置 VolcEngine(45)渠道,设 `ModelRatio` 对应基准价(如 2.0 设 46),然后:

```bash
# 场景1:2.0 720p 不含视频 → 应按基准(倍率1.0)计费
# 场景2:2.0 1080p 不含视频 → 应按 51/46 ≈ 1.109 倍率计费
# 场景3:2.0 4k 含视频 → 应按 16/46 ≈ 0.348 倍率计费
# 场景4:1.5-pro 有声 → 应按 2.0 倍率计费
# 场景5:--rs 1080p 旁路 → 请求写 720p 但响应 1080p,应按 1080p 计费(验证防旁路)
```

每个场景后查日志的 quota 扣减是否符合预期倍率。

- [ ] **Step 5: 最终 Commit**

```bash
git status
git add -A
git commit -m "test(seedance-billing): full regression + e2e billing verification"
```

---

## Self-Review

### Spec 覆盖核对(对照 ark-video-price.md)

| 官方价格表条目 | 覆盖 Task |
|---|---|
| 2.0 480p/720p(46/28) | Task 2 |
| 2.0 1080p(51/31) | Task 2 |
| **2.0 4k(26/16)** | Task 2(★ 补全) |
| 2.0-fast(37/22) | Task 2 |
| **2.0-mini(23/14)** | Task 2 Step 5(★ 补全,待模型名确认) |
| **1.5-pro 声音维度(16/8)** | Task 4(★ 修正两个 PR 的错误) |
| 1.0-pro/1.0-pro-fast 单价 | 未纳入倍率表(管理员配 ModelRatio 全额计费,与现有一致) |
| 响应分辨率结算(防旁路) | Task 3 |
| 退款额度守恒 | Task 5 |
| 仅成功计费 | 现有逻辑(FAILURE 状态走 RefundTaskQuota) |

### 待确认项(执行时核实)

1. **`doubao-seedance-2.0-mini` 实际模型名**(Task 2 Step 5):官方文档未给日期后缀,需查 ARK 模型列表
2. **`TaskBillingContext` 字段添加位置**(Task 3 Step 7):grep `model/task.go` 确认现有定义
3. **`BillingContext` 构造时设 `HasVideoInput`**(Task 3 Step 7):grep `BillingContext{` 找所有构造点
4. **`model.DecreaseUserUsedQuota` 等函数**(Task 5 Step 4):若不存在需从 #4560 摘取定义
5. **1.5-pro 响应是否返回音频信息**(Task 4):若响应能反推是否有声,可在 AdjustBillingOnComplete 用响应值;否则保持请求侧判断

### 类型一致性

- `GetVideoBillingRatio(modelName, resolution string, hasVideo bool) (float64, bool)` 在 Task 2 定义,Task 3 Step 7 调用 ✓
- `GetSeedance15ProAudioRatio(hasAudio bool) (float64, bool)` 在 Task 4 定义并同 Task 调用 ✓
- `AdjustBillingOnComplete(task *model.Task, taskResult *relaycommon.TaskInfo) int` 符合 `TaskAdaptor` 接口(`taskcommon.BaseBilling` 的方法签名)✓
- `TaskInfo.Resolution` 在 Task 1 定义,Task 3 Step 3 填充 ✓
- `TaskBillingContext.HasVideoInput/Resolution/TotalTokens` 在 Task 3 Step 7 定义并使用 ✓

无类型不一致问题。

---

## 与原生入口计划的协调

本计划(计费)与 `2026-07-18-ark-native-compat.md`(原生入口)都改:
- `relay/channel/task/doubao/adaptor.go`(`ParseTaskResult`/`EstimateBilling`)

**协调建议:**
1. 先执行原生入口计划(它改 adaptor 的 Validate/BuildRequest/DoResponse + ParseTaskResult 加状态)
2. 再执行本计划(它在 ParseTaskResult 加 Resolution 回填 + 新增 AdjustBillingOnComplete)
3. 两者的 `ParseTaskResult` 改动不冲突:原生入口加 `expired/cancelled` case,本计划在 `succeeded` case 加一行,位置不同

**独立 PR 策略:** 两个计划产出两个独立 PR,分别 review/merge,降低冲突面。

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-07-18-seedance-billing.md`. 见对话中的执行选项(subagent-driven vs inline)。
