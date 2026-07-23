# Seedance Capability Routing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在复用现有分组、渠道优先级、权重、亲和和重试链路的前提下，为三个本站 Seedance 2.0 模型 ID 增加按分辨率、时长、比例、参考素材数量和真人要求选择上游目标的能力路由。

**Architecture:** 新增独立的 `pkg/modelrouting` 纯领域包和 `(group_name, canonical_model)` 策略快照缓存；分发中间件只为官方 ARK 创建请求提取结构化事实，选择服务先在渠道内确定最高优先级兼容目标，再把允许的渠道集合交给现有跨渠道优先级/权重算法。最终上游模型通过显式路由决策覆盖普通 `model_mapping`，本站模型继续作为计费、响应和用户日志身份，策略、目标、事实与上游模型只进入任务私有数据和 `other.admin_info`。

**Tech Stack:** Go 1.22+、Gin、GORM v2、SQLite/MySQL/PostgreSQL、`sync/atomic`、`common.*` JSON wrappers、testify、React 19、TypeScript、React Query、React Hook Form、Zod、Base UI/shadcn、Tailwind CSS、Bun。

**Design Spec:** `docs/superpowers/specs/2026-07-23-seedance-capability-routing-design.md`

---

## Frozen Contracts

- 唯一启用完整事实提取的客户端入口是 `POST /api/v3/contents/generations/tasks`；Seedance 中间件改写后的内部路径仍是 `/v1/video/generations`。
- 本站公开模型固定为 `doubao-seedance-2-0-260128`、`doubao-seedance-2-0-fast-260128`、`doubao-seedance-2-0-mini-260615`。
- 策略键是具体 `group_name + canonical model`；`auto` 只按既有顺序展开具体分组，不保存 `auto` 策略。
- 没有启用策略时执行 `legacy`；存在启用策略时必须命中启用目标，禁止回退到普通模型映射或不兼容渠道。
- 同一渠道先选择最高 `target_priority` 的兼容目标；跨渠道仍使用现有渠道优先级层和同层权重随机。
- 同一策略、同一渠道、相同目标优先级的能力范围只要可能命中同一个请求，就拒绝保存。
- `routing.require_real_person=true` 才增加真人要求；`false` 和缺省不增加限制。整个 `routing` 对象在两条上游适配路径都必须删除。
- `duration=-1` 在能力模式返回 400 `no_compatible_route`；请求字段类型错误、超出全局边界或内容结构错误继续使用 `InvalidParameter.*`。
- 没有任何启用目标支持合法参数组合返回 400 `no_compatible_route`；存在兼容目标但对应渠道禁用、不可用或已在本请求失败返回 503 `compatible_channel_unavailable`；缓存或策略损坏返回 500 `routing_policy_error`。
- 路由匹配素材缩写固定为：`933 = 9 图/3 视频/3 音频`、`431 = 4 图/3 视频/1 音频`、`9 = 9 图/0 视频/0 音频`。
- `upscaled=true` 的目标按 `output_resolutions` 匹配客户端；例如 `generation_resolution=720p`、输出 `1080p` 的目标只匹配用户请求 `1080p`。
- 第一期不按上游成本选择，不把 `generate_audio`、`service_tier`、`draft` 或 `tools` 作为路由维度，不分析提示词或媒体内容，不下载远程 URL。
- 用户定价、预扣和结算只使用 canonical/origin model；能力模式的 upstream model、策略 ID、目标 ID 和事实只对管理员可见。

## File Map

**Create**

- `pkg/modelrouting/types.go`: 不依赖 Gin/GORM 的请求事实、默认值、约束、目标、策略快照、评估和审计类型。
- `pkg/modelrouting/match.go`: 默认值归一化、目标匹配、同渠道最高优先级选择和聚合不匹配原因。
- `pkg/modelrouting/validate.go`: 固定枚举、数值边界、启用策略默认请求、同优先级重叠校验。
- `pkg/modelrouting/match_test.go`, `pkg/modelrouting/validate_test.go`: 确定性能力矩阵和配置不变量测试。
- `model/routing_policy.go`: GORM 策略/目标模型、事务 CRUD、候选渠道和删除清理。
- `model/routing_policy_cache.go`: 不可变策略快照加载、原子发布、按键刷新和周期同步。
- `model/routing_policy_test.go`, `model/routing_policy_cache_test.go`, `model/channel_routing_filter_test.go`: SQLite 迁移、CRUD、原子快照、跨表清理及内存/DB 选择过滤测试。
- `service/routing_policy.go`: 管理写入 DTO 到领域快照的转换、完整替换和错误分类。
- `service/model_routing.go`: 具体分组策略求值、渠道 allowlist、特定渠道/亲和校验和 context 决策发布。
- `service/routing_policy_test.go`, `service/model_routing_test.go`: 管理写入及 legacy/capability、auto、亲和、特定渠道和 400/503 分类测试。
- `controller/routing_policy.go`, `controller/routing_policy_test.go`: 管理 API、结构化校验错误和审计。
- `router/routing-policy-router.go`, `router/routing_policy_router_test.go`: `ChannelRead`/`ChannelWrite` 路由注册。
- `middleware/model_routing.go`, `middleware/model_routing_test.go`: 官方 ARK JSON 事实提取、扩展参数校验和稳定错误响应。
- `relay/helper/model_mapped_routing_test.go`: 目标模型高于普通映射且不发生链式二次映射的回归测试。
- `e2e/seedance_capability_routing_e2e_test.go`: 双 mock 上游、能力矩阵、重试、计费身份和隐私验收。
- `web/default/src/features/model-routing/types.ts`: Zod 表单/API schema 和 TypeScript 类型。
- `web/default/src/features/model-routing/api.ts`: 策略 CRUD 与候选渠道 API。
- `web/default/src/features/model-routing/query-keys.ts`: React Query key factory。
- `web/default/src/features/model-routing/index.tsx`: 路由策略工作区入口。
- `web/default/src/features/model-routing/components/routing-policies-table.tsx`: 策略过滤、列表、启停、编辑和删除。
- `web/default/src/features/model-routing/components/routing-policy-drawer.tsx`: 策略默认值与目标集合编辑器。
- `web/default/src/features/model-routing/components/route-target-editor.tsx`: 单个目标的结构化能力控件。
- `web/default/src/features/model-routing/components/routing-policy-dialogs.tsx`: 删除与启停确认。

**Modify**

- `model/main.go`, `main.go`: 普通/快速 AutoMigrate、启动预热与周期同步。
- `model/channel.go`: 渠道路由目标计数和单个/批量/禁用删除时的目标清理。
- `model/ability.go`, `model/channel_cache.go`, `model/channel_satisfy.go`: 选择前 allowlist/exclusion 过滤，内存和 DB 路径保持相同优先级/权重行为。
- `service/channel_select.go`: `RetryParam` 路由事实与失败渠道集合，auto 分组逐组求值。
- `middleware/distributor.go`: 事实提取、特定渠道和亲和重新校验、稳定路由错误输出。
- `controller/relay.go`: 重试失败后在能力模式排除整个渠道，并保留稳定错误码。
- `constant/context_key.go`, `types/error.go`: 路由事实/决策 context key 和三种稳定错误码。
- `relay/common/relay_info.go`, `relay/helper/model_mapped.go`, `relay/relay_task.go`: 决策进入 RelayInfo、目标模型优先于普通映射、origin model 保持不变。
- `relay/channel/task/newapivideo/native.go`, `relay/channel/task/newapivideo/adaptor.go`, `relay/channel/task/newapivideo/native_test.go`: 接受但不转发 `routing`，能力模式跳过上游模型名推断分辨率。
- `relay/channel/task/doubao/native.go`, `relay/channel/task/doubao/native_test.go`: 删除原始字段中的 `routing`，能力模式避免与策略冲突的硬编码分辨率限制。
- `model/task.go`, `service/task_billing.go`, `service/log_info_generate.go`, `controller/relay.go`, `controller/task.go`: 私有路由审计、管理员日志和用户侧脱敏。
- `middleware/audit.go`, `controller/audit.go`: 策略管理操作的稳定审计动作。
- `router/api-router.go`: 注册管理路由。
- `controller/channel.go`, `controller/channel_authz_test.go`: 为渠道列表、搜索和详情填充并验证 `routing_target_count`。
- `web/default/src/features/models/section-registry.tsx`, `web/default/src/features/models/index.tsx`, `web/default/src/features/models/types.ts`, `web/default/src/features/models/components/models-provider.tsx`, `web/default/src/routes/_authenticated/models/$section.tsx`: 增加 `routing` 模型工作区和 URL 状态。
- `web/default/src/features/channels/types.ts`, `web/default/src/features/channels/components/channels-columns.tsx`: 目标数量与跳转入口。
- `web/default/src/i18n/locales/{en,zh,fr,ru,ja,vi}.json`: 所有新增管理文本。

---

### Task 1: Build the Pure Routing Domain

**Files:**
- Create: `pkg/modelrouting/types.go`
- Create: `pkg/modelrouting/match.go`
- Create: `pkg/modelrouting/match_test.go`

- [ ] **Step 1: Write failing normalization and matching tests**

Create table tests with the exact public models and collected material presets. The core assertions must cover explicit values overriding defaults, inclusive intervals, discrete durations, unrestricted ratios, unknown真人能力, and the upscale rule:

```go
func TestResolveFactsPrefersExplicitValues(t *testing.T) {
	resolution := "1080P"
	duration := 10
	ratio := "16:9"
	input := modelrouting.FactsInput{
		CanonicalModel: modelrouting.Seedance20, OutputResolution: &resolution,
		DurationSeconds: &duration, AspectRatio: &ratio,
		ReferenceImages: 9, ReferenceVideos: 3, ReferenceAudios: 3,
		RequireRealPerson: true,
	}
	facts, err := modelrouting.ResolveFacts("分组A", input, modelrouting.Defaults{
		OutputResolution: "720p", DurationSeconds: 5, AspectRatio: "9:16",
	})
	require.NoError(t, err)
	assert.Equal(t, "1080p", facts.OutputResolution)
	assert.Equal(t, 10, facts.DurationSeconds)
	assert.Equal(t, "16:9", facts.AspectRatio)
	assert.Equal(t, modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3}, facts.References)
	assert.True(t, facts.RequireRealPerson)
}

func TestEvaluateTargetsUsesOutputResolutionForUpscale(t *testing.T) {
	supportsRealPerson := true
	snapshot := modelrouting.PolicySnapshot{
		ID: 7, GroupName: "分组A", CanonicalModel: modelrouting.Seedance20, Enabled: true,
		TargetsByChannel: map[int][]modelrouting.Target{11: {{
			ID: 21, ChannelID: 11, Name: "720p generation to 1080p",
			UpstreamModel: "lec-feituo-seedance-2-0-my-upscaled-1080p", Priority: 50, Enabled: true,
			Constraints: modelrouting.Constraints{
				OutputResolutions: []string{"1080p"}, GenerationResolution: "720p", Upscaled: true,
				Durations: modelrouting.DurationConstraint{Min: intPtr(4), Max: intPtr(15)},
				AspectRatios: []string{"16:9", "9:16"},
				ReferenceLimits: modelrouting.ReferenceLimits{Images: 4, Videos: 3, Audios: 1},
				SupportsRealPerson: &supportsRealPerson,
			},
		}}},
	}

	matching := modelrouting.Evaluate(snapshot, modelrouting.Facts{
		GroupName: "分组A", CanonicalModel: modelrouting.Seedance20,
		OutputResolution: "1080p", DurationSeconds: 10, AspectRatio: "16:9",
		References: modelrouting.ReferenceLimits{Images: 4, Videos: 3, Audios: 1}, RequireRealPerson: true,
	})
	require.Contains(t, matching.CompatibleByChannel, 11)
	assert.Equal(t, 21, matching.CompatibleByChannel[11].ID)

	notMatching := modelrouting.Evaluate(snapshot, modelrouting.Facts{
		GroupName: "分组A", CanonicalModel: modelrouting.Seedance20,
		OutputResolution: "720p", DurationSeconds: 10, AspectRatio: "16:9",
	})
	assert.Empty(t, notMatching.CompatibleByChannel)
	assert.Equal(t, 1, notMatching.MismatchCounts[modelrouting.MismatchResolution])
}

func intPtr(value int) *int { return &value }
```

Add `TestMaterialPresetMatching` with `933 -> {9,3,3}`, `431 -> {4,3,1}`, and `9 -> {9,0,0}`. Each row asserts equality at the limit and a mismatch when exactly one relevant count is one above the limit. Add真人 rows proving request `false` matches target `true`, `false`, or `nil`, while request `true` matches only an explicit target `true`. Add a separate assertion that a higher-priority compatible target wins inside one channel and target ID is the deterministic tie-breaker only for corrupted runtime state.

- [ ] **Step 2: Run the package test and verify the package is missing**

```powershell
go test ./pkg/modelrouting -run 'TestResolveFacts|TestEvaluateTargets|TestMaterialPreset' -count=1
```

Expected: FAIL because `pkg/modelrouting` and its exported types do not exist.

- [ ] **Step 3: Define the immutable domain types**

Create `types.go` with these public contracts:

```go
package modelrouting

const (
	Seedance20     = "doubao-seedance-2-0-260128"
	Seedance20Fast = "doubao-seedance-2-0-fast-260128"
	Seedance20Mini = "doubao-seedance-2-0-mini-260615"
)

var CanonicalModels = []string{Seedance20, Seedance20Fast, Seedance20Mini}

type Defaults struct {
	OutputResolution string `json:"output_resolution"`
	DurationSeconds  int    `json:"duration_seconds"`
	AspectRatio      string `json:"aspect_ratio"`
}

type FactsInput struct {
	CanonicalModel    string
	OutputResolution  *string
	DurationSeconds   *int
	AspectRatio       *string
	ReferenceImages   int
	ReferenceVideos   int
	ReferenceAudios   int
	RequireRealPerson bool
}

type Facts struct {
	GroupName         string          `json:"group_name"`
	CanonicalModel    string          `json:"canonical_model"`
	OutputResolution  string          `json:"output_resolution"`
	DurationSeconds   int             `json:"duration_seconds"`
	AspectRatio       string          `json:"aspect_ratio"`
	References        ReferenceLimits `json:"references"`
	RequireRealPerson bool            `json:"require_real_person"`
}

type DurationConstraint struct {
	Values []int `json:"values,omitempty"`
	Min    *int  `json:"min,omitempty"`
	Max    *int  `json:"max,omitempty"`
}

type ReferenceLimits struct {
	Images int `json:"images"`
	Videos int `json:"videos"`
	Audios int `json:"audios"`
}

type Constraints struct {
	OutputResolutions    []string           `json:"output_resolutions"`
	GenerationResolution string             `json:"generation_resolution,omitempty"`
	Upscaled             bool               `json:"upscaled"`
	Durations            DurationConstraint `json:"durations"`
	AspectRatios         []string           `json:"aspect_ratios,omitempty"`
	ReferenceLimits      ReferenceLimits    `json:"reference_limits"`
	SupportsRealPerson   *bool              `json:"supports_real_person"`
}

type Target struct {
	ID            int         `json:"id"`
	PolicyID      int         `json:"policy_id"`
	ChannelID     int         `json:"channel_id"`
	Name          string      `json:"name"`
	UpstreamModel string      `json:"upstream_model"`
	Priority      int         `json:"target_priority"`
	Enabled       bool        `json:"enabled"`
	Constraints   Constraints `json:"constraints"`
}

type PolicySnapshot struct {
	ID               int              `json:"id"`
	GroupName        string           `json:"group_name"`
	CanonicalModel   string           `json:"model"`
	Enabled          bool             `json:"enabled"`
	Defaults         Defaults         `json:"defaults"`
	TargetsByChannel map[int][]Target `json:"-"`
}

type MismatchReason string

const (
	MismatchResolution      MismatchReason = "resolution"
	MismatchDuration        MismatchReason = "duration"
	MismatchAspectRatio     MismatchReason = "aspect_ratio"
	MismatchReferenceImages MismatchReason = "reference_images"
	MismatchReferenceVideos MismatchReason = "reference_videos"
	MismatchReferenceAudios MismatchReason = "reference_audios"
	MismatchRealPerson      MismatchReason = "real_person"
)

type Evaluation struct {
	CompatibleByChannel map[int]Target         `json:"-"`
	MismatchCounts      map[MismatchReason]int `json:"mismatch_counts"`
}

type Audit struct {
	PolicyID       int                    `json:"policy_id"`
	TargetID       int                    `json:"target_id"`
	TargetName     string                 `json:"target_name"`
	UpstreamModel  string                 `json:"upstream_model"`
	Facts          Facts                  `json:"facts"`
	MismatchCounts map[MismatchReason]int `json:"mismatch_counts,omitempty"`
}
```

- [ ] **Step 4: Implement normalization and deterministic matching**

Create `match.go`. Normalize case and whitespace, resolve request values before defaults, and select a target per channel by descending target priority then ascending ID:

```go
func ResolveFacts(group string, input FactsInput, defaults Defaults) (Facts, error) {
	resolution := defaults.OutputResolution
	if input.OutputResolution != nil { resolution = *input.OutputResolution }
	duration := defaults.DurationSeconds
	if input.DurationSeconds != nil { duration = *input.DurationSeconds }
	ratio := defaults.AspectRatio
	if input.AspectRatio != nil { ratio = *input.AspectRatio }
	facts := Facts{
		GroupName: strings.TrimSpace(group), CanonicalModel: strings.TrimSpace(input.CanonicalModel),
		OutputResolution: strings.ToLower(strings.TrimSpace(resolution)), DurationSeconds: duration,
		AspectRatio: strings.ToLower(strings.TrimSpace(ratio)),
		References: ReferenceLimits{Images: input.ReferenceImages, Videos: input.ReferenceVideos, Audios: input.ReferenceAudios},
		RequireRealPerson: input.RequireRealPerson,
	}
	if facts.GroupName == "" || facts.CanonicalModel == "" || facts.OutputResolution == "" || facts.DurationSeconds == 0 || facts.AspectRatio == "" {
		return Facts{}, fmt.Errorf("routing facts are incomplete")
	}
	return facts, nil
}

func Evaluate(snapshot PolicySnapshot, facts Facts) Evaluation {
	result := Evaluation{CompatibleByChannel: map[int]Target{}, MismatchCounts: map[MismatchReason]int{}}
	for channelID, targets := range snapshot.TargetsByChannel {
		ordered := append([]Target(nil), targets...)
		sort.SliceStable(ordered, func(i, j int) bool {
			if ordered[i].Priority != ordered[j].Priority { return ordered[i].Priority > ordered[j].Priority }
			return ordered[i].ID < ordered[j].ID
		})
		for _, target := range ordered {
			if !target.Enabled { continue }
			reasons := Match(target.Constraints, facts)
			if len(reasons) == 0 {
				if _, selected := result.CompatibleByChannel[channelID]; !selected { result.CompatibleByChannel[channelID] = target }
				continue
			}
			for _, reason := range reasons { result.MismatchCounts[reason]++ }
		}
	}
	return result
}
```

`Match` returns all applicable mismatch reasons. `aspect_ratios=[]` means any ratio;真人要求 only rejects `nil` or `false` when the request explicitly requires `true`; `generation_resolution` never participates in matching.

- [ ] **Step 5: Format and run the matching suite**

```powershell
gofmt -w pkg/modelrouting/types.go pkg/modelrouting/match.go pkg/modelrouting/match_test.go
go test ./pkg/modelrouting -run 'TestResolveFacts|TestEvaluateTargets|TestMaterialPreset' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit the pure matching domain**

```powershell
git add pkg/modelrouting/types.go pkg/modelrouting/match.go pkg/modelrouting/match_test.go
git commit -m "feat(routing): add capability matching domain"
```

---

### Task 2: Validate Policies and Reject Ambiguous Targets

**Files:**
- Create: `pkg/modelrouting/validate.go`
- Create: `pkg/modelrouting/validate_test.go`

- [ ] **Step 1: Write failing validation and overlap tests**

Cover every fixed enum and bound, both duration forms, upscale invariants, enabled-policy default coverage, and same-channel/same-priority overlap. Use `relaycommon.MaxTaskDurationSeconds` in tests rather than duplicating its value:

```go
func TestValidatePolicyRejectsAmbiguousSamePriorityTargets(t *testing.T) {
	policy := validPolicySnapshot()
	policy.TargetsByChannel[11] = []modelrouting.Target{
		validTarget(21, 11, 50, []string{"720p"}, modelrouting.DurationConstraint{Min: intPtr(4), Max: intPtr(15)}, []string{"16:9"}),
		validTarget(22, 11, 50, []string{"720p", "1080p"}, modelrouting.DurationConstraint{Values: []int{5, 10, 15}}, []string{"16:9", "9:16"}),
	}
	err := modelrouting.ValidatePolicy(policy, relaycommon.MaxTaskDurationSeconds)
	require.Error(t, err)
	var validationErr *modelrouting.ValidationError
	require.ErrorAs(t, err, &validationErr)
	assert.Equal(t, modelrouting.ValidationTargetOverlap, validationErr.Code)
	assert.Equal(t, []int{21, 22}, validationErr.TargetIDs)
}

func TestValidatePolicyAcceptsDisjointSamePriorityTargets(t *testing.T) {
	policy := validPolicySnapshot()
	policy.TargetsByChannel[11] = []modelrouting.Target{
		validTarget(21, 11, 50, []string{"720p"}, modelrouting.DurationConstraint{Min: intPtr(4), Max: intPtr(9)}, nil),
		validTarget(22, 11, 50, []string{"720p"}, modelrouting.DurationConstraint{Min: intPtr(10), Max: intPtr(15)}, nil),
	}
	require.NoError(t, modelrouting.ValidatePolicy(policy, relaycommon.MaxTaskDurationSeconds))
}
```

The table must assert these exact failures:

| Mutation | Expected code |
|---|---|
| canonical model outside the three IDs | `invalid_model` |
| `group_name` empty or `auto` | `invalid_group` |
| empty output resolutions or `1440p` | `invalid_output_resolution` |
| both `values` and `min/max` | `invalid_duration` |
| duration `0` or `MaxTaskDurationSeconds+1` | `invalid_duration` |
| ratio `2:1` | `invalid_aspect_ratio` |
| images 10, videos 4, or audios 4 | `invalid_reference_limit` |
| `upscaled=true` with two outputs or equal generation/output | `invalid_upscale` |
| `upscaled=false` with generation resolution | `invalid_upscale` |
| enabled policy with no enabled target matching defaults | `default_route_unavailable` |

- [ ] **Step 2: Run validation tests and verify the missing symbols fail**

```powershell
go test ./pkg/modelrouting -run 'TestValidatePolicy' -count=1
```

Expected: FAIL because `ValidatePolicy`, `ValidationError`, and validation codes are undefined.

- [ ] **Step 3: Implement typed validation errors and fixed sets**

Create `validate.go` with these error contracts:

```go
type ValidationCode string

const (
	ValidationInvalidModel            ValidationCode = "invalid_model"
	ValidationInvalidGroup            ValidationCode = "invalid_group"
	ValidationInvalidOutputResolution ValidationCode = "invalid_output_resolution"
	ValidationInvalidDuration         ValidationCode = "invalid_duration"
	ValidationInvalidAspectRatio      ValidationCode = "invalid_aspect_ratio"
	ValidationInvalidReferenceLimit   ValidationCode = "invalid_reference_limit"
	ValidationInvalidUpscale          ValidationCode = "invalid_upscale"
	ValidationDefaultRouteUnavailable ValidationCode = "default_route_unavailable"
	ValidationTargetOverlap           ValidationCode = "routing_target_overlap"
)

type ValidationError struct {
	Code      ValidationCode
	Field     string
	TargetIDs []int
	Message   string
}

func (e *ValidationError) Error() string { return e.Message }

var allowedResolutions = []string{"480p", "720p", "1080p", "4k"}
var allowedRatios = []string{"16:9", "4:3", "1:1", "3:4", "9:16", "21:9", "adaptive"}
```

`ValidatePolicy(policy, maxDuration)` must first validate policy defaults against the same resolution/ratio sets and `1..maxDuration`, then validate every target even when the policy or target is disabled. Only the enabled-policy default coverage test is conditional on `policy.Enabled`. Apply the exact global material limits `images <= 9`, `videos <= 3`, and `audios <= 3`; require non-negative values.

- [ ] **Step 4: Implement inclusive intersection-based overlap detection**

Only compare enabled targets in the same channel and at the same priority. Material limits and `supports_real_person` never make two ranges disjoint because a request with zero references and `require_real_person=false` can match both; resolution, duration, and constrained ratios are the disjointing dimensions:

```go
func validateOverlaps(policy PolicySnapshot) error {
	for _, targets := range policy.TargetsByChannel {
		for left := 0; left < len(targets); left++ {
			for right := left + 1; right < len(targets); right++ {
				a, b := targets[left], targets[right]
				if !a.Enabled || !b.Enabled || a.Priority != b.Priority || !constraintsOverlap(a.Constraints, b.Constraints) {
					continue
				}
				ids := []int{a.ID, b.ID}
				sort.Ints(ids)
				return &ValidationError{
					Code: ValidationTargetOverlap, Field: "targets", TargetIDs: ids,
					Message: fmt.Sprintf("targets %d and %d overlap at the same channel priority", ids[0], ids[1]),
				}
			}
		}
	}
	return nil
}

func constraintsOverlap(a, b Constraints) bool {
	return stringSetsOverlap(a.OutputResolutions, b.OutputResolutions, false) &&
		durationsOverlap(a.Durations, b.Durations) &&
		stringSetsOverlap(a.AspectRatios, b.AspectRatios, true)
}
```

Implement `durationsOverlap` without enumerating `1..maxDuration`: compare values-to-values, values-to-range, or two inclusive ranges. `stringSetsOverlap(..., emptyMeansAny=true)` returns true when either ratio set is empty. For new targets whose IDs are still zero, return target positions as negative one-based values (`-1`, `-2`) in `TargetIDs`; the service converts those to UI `target_indexes` before responding.

- [ ] **Step 5: Validate the enabled policy's default request**

After structural checks, resolve defaults with no explicit request values and require at least one enabled match:

```go
if policy.Enabled {
	facts, err := ResolveFacts(policy.GroupName, FactsInput{CanonicalModel: policy.CanonicalModel}, policy.Defaults)
	if err != nil {
		return &ValidationError{Code: ValidationDefaultRouteUnavailable, Field: "defaults", Message: err.Error()}
	}
	if len(Evaluate(policy, facts).CompatibleByChannel) == 0 {
		return &ValidationError{Code: ValidationDefaultRouteUnavailable, Field: "defaults", Message: "no enabled target matches the policy defaults"}
	}
}
```

- [ ] **Step 6: Format and run the complete domain suite**

```powershell
gofmt -w pkg/modelrouting/validate.go pkg/modelrouting/validate_test.go
go test ./pkg/modelrouting -count=1
```

Expected: PASS, including overlap, upscale, defaults, `933`, `431`, and `9` cases.

- [ ] **Step 7: Commit policy validation**

```powershell
git add pkg/modelrouting/validate.go pkg/modelrouting/validate_test.go
git commit -m "feat(routing): validate capability policies"
```

---

### Task 3: Persist Policies and Publish Immutable Cache Snapshots

**Files:**
- Create: `model/routing_policy.go`
- Create: `model/routing_policy_cache.go`
- Create: `model/routing_policy_test.go`
- Create: `model/routing_policy_cache_test.go`
- Modify: `model/main.go`
- Modify: `main.go`

- [ ] **Step 1: Write failing SQLite migration and replacement tests**

Use an isolated in-memory SQLite database, save and restore the package-level `model.DB`, and migrate `Channel`, `Ability`, `RoutingPolicy`, and `RouteTarget`. The contract test must replace a policy and its full target set atomically:

```go
func TestReplaceRoutingPolicyPersistsTypedConstraints(t *testing.T) {
	db := openRoutingTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.RoutingPolicy{}, &model.RouteTarget{}))
	policy := model.RoutingPolicy{
		GroupName: "分组A", Model: modelrouting.Seedance20, Enabled: true,
		DefaultResolution: "720p", DefaultDuration: 10, DefaultRatio: "16:9",
	}
	targets := []model.RouteTarget{{
		ChannelID: 11, Name: "A1 720 fast", UpstreamModel: "bb-seedance2.0-720p-fast-gz-15s",
		TargetPriority: 100, Enabled: true,
		Constraints: validConstraintsJSON(t, modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3}),
	}}
	created, err := model.ReplaceRoutingPolicy(0, policy, targets)
	require.NoError(t, err)
	require.NotZero(t, created.ID)
	require.Len(t, created.Targets, 1)

	loaded, err := model.GetRoutingPolicy(created.ID)
	require.NoError(t, err)
	assert.Equal(t, "bb-seedance2.0-720p-fast-gz-15s", loaded.Targets[0].UpstreamModel)
	assert.JSONEq(t, targets[0].Constraints, loaded.Targets[0].Constraints)
}
```

Add tests for the unique `(group_name, model)` index, transaction rollback when the second target is invalid, delete policy explicitly removing targets, and `ListRoutingCandidates("分组A", model)` returning channels whose abilities match the exact group/model without exposing `Channel.Key`. Candidate discovery queries `Ability` with `commonGroupCol` because `group` is reserved, includes enabled and disabled channels that still declare that group/model, then loads safe channel fields through GORM. Selection later intersects only enabled abilities. This keeps disabled targets editable without making them routable.

- [ ] **Step 2: Run the model tests and confirm the schema is missing**

```powershell
go test ./model -run 'TestReplaceRoutingPolicy|TestRoutingPolicyUnique|TestDeleteRoutingPolicy|TestListRoutingCandidates' -count=1
```

Expected: FAIL because the routing persistence models and methods do not exist.

- [ ] **Step 3: Add cross-database GORM models**

Use integer Unix timestamps and `TEXT` constraints. Do not add boolean default tags:

```go
type RoutingPolicy struct {
	ID                int           `json:"id"`
	GroupName         string        `json:"group_name" gorm:"type:varchar(64);not null;uniqueIndex:idx_routing_policy_group_model,priority:1"`
	Model             string        `json:"model" gorm:"type:varchar(191);not null;uniqueIndex:idx_routing_policy_group_model,priority:2"`
	Enabled           bool          `json:"enabled" gorm:"not null"`
	DefaultResolution string        `json:"default_resolution" gorm:"type:varchar(16);not null"`
	DefaultDuration   int           `json:"default_duration" gorm:"not null"`
	DefaultRatio      string        `json:"default_ratio" gorm:"type:varchar(16);not null"`
	CreatedAt         int64         `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt         int64         `json:"updated_at" gorm:"autoUpdateTime"`
	Targets           []RouteTarget `json:"targets" gorm:"-"`
}

type RouteTarget struct {
	ID             int    `json:"id"`
	PolicyID       int    `json:"policy_id" gorm:"not null;index"`
	ChannelID      int    `json:"channel_id" gorm:"not null;index"`
	Name           string `json:"name" gorm:"type:varchar(128);not null"`
	UpstreamModel  string `json:"upstream_model" gorm:"type:varchar(255);not null"`
	TargetPriority int    `json:"target_priority" gorm:"not null;index"`
	Constraints    string `json:"constraints" gorm:"type:text;not null"`
	Enabled        bool   `json:"enabled" gorm:"not null"`
	CreatedAt      int64  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt      int64  `json:"updated_at" gorm:"autoUpdateTime"`
}

type RoutingCandidateChannel struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Status   int    `json:"status"`
	Priority int64  `json:"priority"`
	Weight   uint   `json:"weight"`
}
```

`ReplaceRoutingPolicy` must decode and structurally validate every constraints JSON string before starting destructive replacement work. It then starts a GORM transaction, creates or updates the policy, explicitly deletes old targets, inserts the complete supplied slice with `PolicyID` assigned, and commits. It returns only after reloading the persisted policy and targets. Use `common.Marshal`/`common.UnmarshalJsonStr` for constraints in conversion helpers; never call `encoding/json` functions.

- [ ] **Step 4: Register both migration paths**

Add both models to the normal `DB.AutoMigrate` list and the fast migration list:

```go
&RoutingPolicy{},
&RouteTarget{},
```

and:

```go
{&RoutingPolicy{}, "RoutingPolicy"},
{&RouteTarget{}, "RouteTarget"},
```

Run the migration-focused test again:

```powershell
go test ./model -run 'TestReplaceRoutingPolicy|TestRoutingPolicyUnique|TestDeleteRoutingPolicy|TestListRoutingCandidates' -count=1
```

Expected: PASS.

- [ ] **Step 5: Write failing cache publication tests**

The cache test must prove that a successful refresh replaces only one key and a malformed JSON refresh leaves the previous snapshot intact:

```go
func TestRefreshRoutingPolicyCacheKeepsPreviousSnapshotOnDecodeFailure(t *testing.T) {
	openRoutingTestDB(t)
	seedPolicyRows(t, `{"output_resolutions":["720p"],"upscaled":false,"durations":{"min":4,"max":15},"reference_limits":{"images":9,"videos":3,"audios":3},"supports_real_person":true}`)
	require.NoError(t, model.InitRoutingPolicyCache())
	before, ok := model.GetRoutingPolicySnapshot("分组A", modelrouting.Seedance20)
	require.True(t, ok)

	require.NoError(t, model.DB.Model(&model.RouteTarget{}).Where("id = ?", 21).Update("constraints", `{broken`).Error)
	err := model.RefreshRoutingPolicyCache("分组A", modelrouting.Seedance20)
	require.Error(t, err)
	after, ok := model.GetRoutingPolicySnapshot("分组A", modelrouting.Seedance20)
	require.True(t, ok)
	assert.Equal(t, before, after)
}
```

- [ ] **Step 6: Implement atomic copy-on-write cache publication**

Use `atomic.Value` containing `map[RoutingPolicyKey]modelrouting.PolicySnapshot`. Never mutate a published map or nested target slice:

```go
type RoutingPolicyKey struct {
	GroupName string
	Model     string
}

var routingPolicySnapshots atomic.Value

func init() {
	routingPolicySnapshots.Store(map[RoutingPolicyKey]modelrouting.PolicySnapshot{})
}

func GetRoutingPolicySnapshot(group, canonicalModel string) (modelrouting.PolicySnapshot, bool) {
	key := RoutingPolicyKey{GroupName: group, Model: canonicalModel}
	snapshot, ok := routingPolicySnapshots.Load().(map[RoutingPolicyKey]modelrouting.PolicySnapshot)[key]
	return snapshot, ok
}

func RefreshRoutingPolicyCache(group, canonicalModel string) error {
	key := RoutingPolicyKey{GroupName: group, Model: canonicalModel}
	snapshot, enabled, err := loadRoutingPolicySnapshot(key)
	if err != nil { return err }
	current := routingPolicySnapshots.Load().(map[RoutingPolicyKey]modelrouting.PolicySnapshot)
	next := maps.Clone(current)
	if enabled { next[key] = snapshot } else { delete(next, key) }
	routingPolicySnapshots.Store(next)
	return nil
}
```

`InitRoutingPolicyCache` loads all enabled policies and all enabled targets into a fresh local map, validates every snapshot with `modelrouting.ValidatePolicy(..., relaycommon.MaxTaskDurationSeconds)`, then publishes once. `SyncRoutingPolicyCache` reloads on `common.SyncFrequency`; if loading fails, log with `common.SysError` and keep the old map.

- [ ] **Step 7: Initialize routing cache on every node**

Immediately after database initialization and before channel selection can serve traffic, add:

```go
if err := model.InitRoutingPolicyCache(); err != nil {
	common.FatalLog("failed to initialize routing policy cache: " + err.Error())
}
go model.SyncRoutingPolicyCache(common.SyncFrequency)
```

This cache is independent of `common.MemoryCacheEnabled`; capability requests must not fall back to a DB read on the hot path.

- [ ] **Step 8: Run cache tests and commit persistence**

```powershell
gofmt -w model/routing_policy.go model/routing_policy_cache.go model/routing_policy_test.go model/routing_policy_cache_test.go model/main.go main.go
go test ./model -run 'TestReplaceRoutingPolicy|TestRoutingPolicy|TestRefreshRoutingPolicyCache|TestInitRoutingPolicyCache' -count=1
```

Expected: PASS.

```powershell
git add model/routing_policy.go model/routing_policy_cache.go model/routing_policy_test.go model/routing_policy_cache_test.go model/main.go main.go
git commit -m "feat(routing): persist and cache capability policies"
```

---

### Task 4: Add Atomic Admin CRUD, Permissions, Audit, and Channel Cleanup

**Files:**
- Create: `service/routing_policy.go`
- Create: `service/routing_policy_test.go`
- Create: `controller/routing_policy.go`
- Create: `controller/routing_policy_test.go`
- Create: `router/routing-policy-router.go`
- Create: `router/routing_policy_router_test.go`
- Modify: `router/api-router.go`
- Modify: `middleware/audit.go`
- Modify: `controller/audit.go`
- Modify: `model/channel.go`
- Modify: `model/routing_policy.go`
- Modify: `model/routing_policy_test.go`

- [ ] **Step 1: Write failing service tests for complete replacement**

Use the same SQLite fixture and seed two candidate channels. The write DTO is structured and never exposes raw constraints JSON:

```go
request := service.RoutingPolicyWriteRequest{
	GroupName: "分组A", Model: modelrouting.Seedance20, Enabled: true,
	Defaults: modelrouting.Defaults{OutputResolution: "720P", DurationSeconds: 10, AspectRatio: "16:9"},
	Targets: []service.RouteTargetWriteRequest{{
		ChannelID: 11, Name: " A1 fast ", UpstreamModel: " bb-seedance2.0-720p-fast-gz-15s ",
		TargetPriority: 100, Enabled: true,
		Constraints: modelrouting.Constraints{
			OutputResolutions: []string{"720P", "720p"},
			Durations: modelrouting.DurationConstraint{Values: []int{15, 10, 10}},
			AspectRatios: []string{"9:16", "16:9", "16:9"},
			ReferenceLimits: modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3},
		},
	}},
}
saved, err := service.SaveRoutingPolicy(0, request)
require.NoError(t, err)
assert.Equal(t, []string{"720p"}, saved.Targets[0].Constraints.OutputResolutions)
assert.Equal(t, []int{10, 15}, saved.Targets[0].Constraints.Durations.Values)
assert.Equal(t, []string{"16:9", "9:16"}, saved.Targets[0].Constraints.AspectRatios)
```

Also assert: `auto` is rejected; unknown/non-candidate channel is rejected with `invalid_channel`; target overlap returns both zero-based `TargetIndexes`; an enabled request must have at least one target; and a successful save refreshes the policy cache only after the DB commit.

- [ ] **Step 2: Run service tests and verify they fail**

```powershell
go test ./service -run 'TestSaveRoutingPolicy' -count=1
```

Expected: FAIL because the service DTO and save operation do not exist.

- [ ] **Step 3: Implement service DTOs, normalization, and error mapping**

Use these external contracts:

```go
type RoutingPolicyWriteRequest struct {
	GroupName string                    `json:"group_name"`
	Model     string                    `json:"model"`
	Enabled   bool                      `json:"enabled"`
	Defaults  modelrouting.Defaults     `json:"defaults"`
	Targets   []RouteTargetWriteRequest `json:"targets"`
}

type RouteTargetWriteRequest struct {
	ChannelID      int                      `json:"channel_id"`
	Name           string                   `json:"name"`
	UpstreamModel  string                   `json:"upstream_model"`
	TargetPriority int                      `json:"target_priority"`
	Enabled        bool                     `json:"enabled"`
	Constraints    modelrouting.Constraints `json:"constraints"`
}

type RoutingPolicyServiceError struct {
	Code          string `json:"code"`
	Field         string `json:"field,omitempty"`
	TargetIndexes []int  `json:"target_indexes,omitempty"`
	Err           error  `json:"-"`
}

func (e *RoutingPolicyServiceError) Error() string { return e.Err.Error() }

type RoutingPolicyView struct {
	ID        int               `json:"id"`
	GroupName string            `json:"group_name"`
	Model     string            `json:"model"`
	Enabled   bool              `json:"enabled"`
	Defaults  modelrouting.Defaults `json:"defaults"`
	Targets   []RouteTargetView `json:"targets"`
	CreatedAt int64             `json:"created_at"`
	UpdatedAt int64             `json:"updated_at"`
}

type RouteTargetView struct {
	ID             int                      `json:"id"`
	ChannelID      int                      `json:"channel_id"`
	ChannelName    string                   `json:"channel_name"`
	Name           string                   `json:"name"`
	UpstreamModel  string                   `json:"upstream_model"`
	TargetPriority int                      `json:"target_priority"`
	Enabled        bool                     `json:"enabled"`
	Constraints    modelrouting.Constraints `json:"constraints"`
}
```

Normalize strings with `TrimSpace`/lowercase where semantically case-insensitive, sort/deduplicate enum and duration slices, reject empty target names/upstream models and non-positive channel IDs, validate candidate channel membership, call `modelrouting.ValidatePolicy`, marshal constraints with `common.Marshal`, then call `model.ReplaceRoutingPolicy`. Before validation assign each request target the synthetic ID `-(index+1)`; when `ValidationTargetOverlap` is returned, convert `-1/-2` back to zero-based `TargetIndexes` `0/1`. Persisted target IDs are assigned by GORM only after validation. Convert persistence rows to `RoutingPolicyView` so every API uses nested `defaults` and typed `constraints`; never serialize the flat GORM model directly. Refresh the cache and `model.InitChannelCache()` only after a successful commit; if cache refresh fails, return `routing_policy_error` while the periodic full reload remains the recovery path.

- [ ] **Step 4: Write failing controller and router tests**

Register these routes under `/api/routing-policies`:

| Method/path | Permission | Handler |
|---|---|---|
| `GET /` | `authz.ChannelRead` | `ListRoutingPolicies` |
| `GET /candidates` | `authz.ChannelRead` | `ListRoutingPolicyCandidates` |
| `GET /:id` | `authz.ChannelRead` | `GetRoutingPolicy` |
| `POST /` | `authz.ChannelWrite` | `CreateRoutingPolicy` |
| `PUT /:id` | `authz.ChannelWrite` | `UpdateRoutingPolicy` |
| `POST /:id/status` | `authz.ChannelWrite` | `UpdateRoutingPolicyStatus` |
| `DELETE /:id` | `authz.ChannelWrite` | `DeleteRoutingPolicy` |

The router test must use the same reflection pattern as `router/channel_router_test.go`:

```go
func TestRoutingPolicyRoutesUseChannelPermissions(t *testing.T) {
	assertRoutingPolicyRoutePermission(t, http.MethodGet, "/", authz.ChannelRead, controller.ListRoutingPolicies)
	assertRoutingPolicyRoutePermission(t, http.MethodPost, "/", authz.ChannelWrite, controller.CreateRoutingPolicy)
	assertRoutingPolicyRoutePermission(t, http.MethodPut, "/:id", authz.ChannelWrite, controller.UpdateRoutingPolicy)
	assertRoutingPolicyRoutePermission(t, http.MethodDelete, "/:id", authz.ChannelWrite, controller.DeleteRoutingPolicy)
}
```

The controller test posts one valid policy, asserts `201`, fetches it, updates it, toggles it, deletes it, and asserts exact error JSON for overlap:

```json
{"success":false,"message":"targets overlap at the same channel priority","code":"routing_target_overlap","data":{"field":"targets","target_indexes":[0,1]}}
```

- [ ] **Step 5: Implement handlers with stable response shapes**

Successful list responses use the project's business envelope:

```json
{"success":true,"message":"","data":{"items":[],"total":0,"page":1,"page_size":10}}
```

The list accepts `group_name`, `model`, `channel_id`, `p`, and `page_size`. Implement `channel_id` with a GORM subquery on `route_targets.policy_id`, not a database-specific join; apply the identical filtered query to count and page retrieval. Validation failures return HTTP 400; missing policies return 404; persistence/cache failures return 500. Do not include `Channel.Key` in policy, target, or candidate responses. After successful writes call:

```go
recordManageAudit(c, "routing_policy.update", map[string]interface{}{
	"policy_id": policy.ID, "group_name": policy.GroupName, "model": policy.Model,
})
```

Add `routing_policy.create`, `routing_policy.update`, `routing_policy.status_update`, and `routing_policy.delete` entries to both `middleware.auditRouteActions` and `controller.auditContentTemplates` so middleware fallback and explicit audit use stable action names.

- [ ] **Step 6: Register the route group**

Create a `routingPolicyPermissionRoutes` slice parallel to `channelPermissionRoutes`, use `middleware.AdminAuth()` once on the group, and call `registerRoutingPolicyRoutes(apiRouter)` immediately after `registerChannelRoutes(apiRouter)` in `router/api-router.go`.

Run:

```powershell
gofmt -w service/routing_policy.go service/routing_policy_test.go controller/routing_policy.go controller/routing_policy_test.go router/routing-policy-router.go router/routing_policy_router_test.go router/api-router.go middleware/audit.go controller/audit.go
go test ./service ./controller ./router -run 'TestSaveRoutingPolicy|TestRoutingPolicy' -count=1
```

Expected: PASS.

- [ ] **Step 7: Write failing channel-deletion cleanup tests**

Test all three paths: `channel.Delete()`, `BatchDeleteChannels`, and `DeleteDisabledChannel`. Seed targets for deleted and retained channels, then assert only deleted-channel targets are removed and affected policy cache keys are refreshed. The disabled deletion test must also assert its `Ability` rows are removed; do not preserve the existing orphan behavior.

- [ ] **Step 8: Make channel deletion transactional and refresh affected snapshots**

Before deleting targets, collect the affected policy keys through GORM queries. Use one transaction per public operation and explicit deletes in this order:

```go
keys, err := deleteRouteTargetsForChannels(tx, channelIDs)
if err != nil { tx.Rollback(); return err }
if err := tx.Where("channel_id IN ?", channelIDs).Delete(&Ability{}).Error; err != nil { tx.Rollback(); return err }
if err := tx.Where("id IN ?", channelIDs).Delete(&Channel{}).Error; err != nil { tx.Rollback(); return err }
if err := tx.Commit().Error; err != nil { return err }
return RefreshRoutingPolicyCacheKeys(keys)
```

For `DeleteDisabledChannel`, pluck the exact disabled IDs inside the transaction before deletion and return the channel row count, not target/ability row counts. For a single `Channel.Delete`, use the same helper with `[]int{channel.Id}`.

- [ ] **Step 9: Refresh associated snapshots after channel capability changes**

Add `RefreshRoutingPolicyCacheByChannelIDs(channelIDs []int) error`, which queries distinct policy IDs from `route_targets`, resolves their `(group_name, model)` keys with GORM, and calls `RefreshRoutingPolicyCacheKeys`. Call it after the transaction and existing `model.InitChannelCache()` for channel update, single/batch status update, tag group/model edits, copy, and other operations that change a channel's group/model/status. Deletion paths use the keys captured before target deletion. On refresh failure log `common.SysError` and return the admin write as failed; never publish a partial snapshot.

Extend service/controller tests by changing A1's models away from the canonical model and disabling A1. Assert its target remains administratively visible, the snapshot refresh succeeds, and the selection intersection returns `compatible_channel_unavailable` rather than choosing A1.

- [ ] **Step 10: Run cleanup tests and commit the admin surface**

```powershell
gofmt -w model/channel.go model/routing_policy.go model/routing_policy_test.go
go test ./model ./service ./controller ./router -run 'RoutingPolicy|RouteTarget|DeleteDisabledChannel|BatchDeleteChannels' -count=1
```

Expected: PASS.

```powershell
git add service/routing_policy.go service/routing_policy_test.go controller/routing_policy.go controller/routing_policy_test.go router/routing-policy-router.go router/routing_policy_router_test.go router/api-router.go middleware/audit.go controller/audit.go model/channel.go model/routing_policy.go model/routing_policy_test.go
git commit -m "feat(routing): add capability policy administration"
```

---

### Task 5: Extract Official Seedance Facts and Strip the Local Extension

**Files:**
- Create: `middleware/model_routing.go`
- Create: `middleware/model_routing_test.go`
- Modify: `constant/context_key.go`
- Modify: `relay/channel/task/newapivideo/native.go`
- Modify: `relay/channel/task/newapivideo/adaptor.go`
- Modify: `relay/channel/task/newapivideo/native_test.go`
- Modify: `relay/channel/task/doubao/native.go`
- Modify: `relay/channel/task/doubao/native_test.go`

- [ ] **Step 1: Write failing fact-extraction tests**

Build Gin contexts with `common.KeySeedanceOfficialAPI=true`, `POST`, `application/json`, and the rewritten `/v1/video/generations` path. Use a table with these exact expectations:

| Request | Result |
|---|---|
| explicit `1080p`, `10`, `16:9`, 4 images/3 videos/1 audio,真人 `true` | populated `FactsInput` |
| omitted resolution/duration/ratio | nil pointers for later policy defaults |
| `duration=-1` | valid extracted input containing `-1` |
| resolution as number | 400 `InvalidParameter.resolution` |
| duration as string/0/3601 | 400 `InvalidParameter.duration` |
| ratio `2:1` | 400 `InvalidParameter.ratio` |
| 10 images, 4 videos, or 4 audios | 400 `InvalidParameter.content` |
| `ratio=adaptive` with video/audio input | 400 `InvalidParameter.ratio` |
| audio without image/video, first/last frame mixed with reference media, or unsupported media role/type | 400 `InvalidParameter.content` |
| `routing` as array | 400 `InvalidParameter.routing` |
|真人 value as string | 400 `InvalidParameter.routing.require_real_person` |
| unknown routing key | 400 `InvalidParameter.routing.<key>` |
| non-official, non-POST, or non-canonical model | extractor returns nil and does not alter legacy behavior |

The success assertion must count every `image_url` item, including `first_frame`, `last_frame`, and `reference_image`, without fetching its URL:

```go
input, routeErr := extractSeedanceRoutingInput(c, modelrouting.Seedance20)
require.Nil(t, routeErr)
require.NotNil(t, input)
assert.Equal(t, 4, input.ReferenceImages)
assert.Equal(t, 3, input.ReferenceVideos)
assert.Equal(t, 1, input.ReferenceAudios)
assert.True(t, input.RequireRealPerson)
```

- [ ] **Step 2: Run extraction tests and confirm they fail**

```powershell
go test ./middleware -run 'TestExtractSeedanceRoutingInput' -count=1
```

Expected: FAIL because the extractor and context key do not exist.

- [ ] **Step 3: Add route context keys and a typed local error**

Add these keys to `constant/context_key.go`:

```go
ContextKeyRoutingFactsInput       ContextKey = "routing_facts_input"
ContextKeyRoutingCapabilityMode  ContextKey = "routing_capability_mode"
ContextKeyRoutingPolicyID        ContextKey = "routing_policy_id"
ContextKeyRoutingTargetID        ContextKey = "routing_target_id"
ContextKeyRoutingTargetName      ContextKey = "routing_target_name"
ContextKeyRoutingUpstreamModel   ContextKey = "routing_upstream_model"
ContextKeyRoutingFacts           ContextKey = "routing_facts"
ContextKeyRoutingMismatchCounts  ContextKey = "routing_mismatch_counts"
```

The extractor returns this internal error; it does not write a response itself:

```go
type routingInputError struct {
	Code    types.ErrorCode
	Message string
}

func (e *routingInputError) Error() string { return e.Message }
```

- [ ] **Step 4: Parse the cached JSON body once without inspecting media**

Use `common.GetBodyStorage`, `common.GetJsonType`, and `common.Unmarshal`. `encoding/json.RawMessage` is allowed only as a type. Decode the top-level object and routing object field-by-field so wrong JSON types map to the exact field code. The extractor must return pointers only for explicitly present resolution/duration/ratio fields:

```go
func extractSeedanceRoutingInput(c *gin.Context, canonicalModel string) (*modelrouting.FactsInput, *routingInputError) {
	if !c.GetBool(common.KeySeedanceOfficialAPI) || c.Request.Method != http.MethodPost ||
		c.Request.URL.Path != "/v1/video/generations" || !slices.Contains(modelrouting.CanonicalModels, canonicalModel) {
		return nil, nil
	}
	storage, err := common.GetBodyStorage(c)
	if err != nil { return nil, &routingInputError{Code: "InvalidParameter", Message: err.Error()} }
	body, err := storage.Bytes()
	if err != nil || common.GetJsonType(body) != "object" {
		return nil, &routingInputError{Code: "InvalidParameter", Message: "request body must be a JSON object"}
	}
	input, parseErr := parseSeedanceRoutingFields(body, canonicalModel)
	if parseErr != nil { return nil, parseErr }
	return &input, nil
}
```

Do not call `storage.Seek` or replace `c.Request.Body`; `GetBodyStorage` already provides reusable storage to the later adaptors. While counting content, enforce the common official structural rules that affect error precedence: allowed media types/roles, first/last-frame isolation, audio requiring image or video, global counts, and `adaptive` rejecting video/audio input. Do not read prompt text, URL contents, dimensions, duration metadata, `generate_audio`, `service_tier`, `draft`, or `tools`. All other channel-specific semantic checks remain in the selected adaptor.

- [ ] **Step 5: Accept and strip `routing` in NewAPIVideo**

Add the key to the strict allowlist:

```go
"service_tier": {}, "draft": {}, "tools": {}, "routing": {},
```

Keep `routing` out of `arkRequest` and `upstreamRequest`; the structured `arkToUpstream` conversion then drops it. Change `buildARKRequestBody` to receive `info` and skip legacy model-name resolution inference only when `info.Routing != nil`:

```go
func arkToUpstream(request arkRequest, upstreamModel string, resolutionPrevalidated bool) (upstreamRequest, error) {
	if err := validateARKSemantics(request); err != nil { return upstreamRequest{}, err }
	if !resolutionPrevalidated {
		if err := validateMappedResolution(request.Resolution, upstreamModel); err != nil {
			return upstreamRequest{}, &arkRequestError{Code: "InvalidParameter.resolution", Message: err.Error()}
		}
	}
	// existing structured conversion follows unchanged
}
```

Update all direct unit calls with `false`; add one capability test using upstream model `lec-feituo-seedance-2-0-my-upscaled-1080p`, client resolution `1080p`, and `true`, asserting success and absence of `routing` in the marshaled body.

- [ ] **Step 6: Strip `routing` from both Doubao raw maps**

Immediately after decoding metadata in `validateNativeRequest` and fields in `buildNativeRequestBody`, add:

```go
delete(metadata, "routing")
```

and:

```go
delete(fields, "routing")
```

Pass `info.Routing != nil` into `validateSeedanceNativeFields`. In capability mode keep global resolution enumeration validation but skip the legacy family restriction that rejects `1080p` for Fast/Mini or `4k` outside standard; the route target already decided the supported output. Preserve all non-routing semantic validation.

- [ ] **Step 7: Run adapter and extraction tests**

```powershell
gofmt -w middleware/model_routing.go middleware/model_routing_test.go constant/context_key.go relay/channel/task/newapivideo/native.go relay/channel/task/newapivideo/adaptor.go relay/channel/task/newapivideo/native_test.go relay/channel/task/doubao/native.go relay/channel/task/doubao/native_test.go
go test ./middleware ./relay/channel/task/newapivideo ./relay/channel/task/doubao -run 'Routing|ARK|Native' -count=1
```

Expected: PASS; assertions prove `routing` is absent from both upstream request paths.

- [ ] **Step 8: Commit fact extraction and extension stripping**

```powershell
git add middleware/model_routing.go middleware/model_routing_test.go constant/context_key.go relay/channel/task/newapivideo/native.go relay/channel/task/newapivideo/adaptor.go relay/channel/task/newapivideo/native_test.go relay/channel/task/doubao/native.go relay/channel/task/doubao/native_test.go
git commit -m "feat(routing): extract Seedance request capabilities"
```

---

### Task 6: Filter Existing Channel Selection Before Priority and Weight

**Files:**
- Modify: `model/channel_cache.go`
- Modify: `model/ability.go`
- Modify: `model/channel_satisfy.go`
- Create: `model/channel_routing_filter_test.go`

- [ ] **Step 1: Write failing memory and DB selection tests**

Run the same table twice with `common.MemoryCacheEnabled=false` and `true`. Seed three enabled channels for `分组A + doubao-seedance-2-0-260128`: channel 11 priority 100, channels 12/13 priority 50. Assert:

```go
filter := model.ChannelSelectFilter{
	AllowedChannelIDs: map[int]struct{}{12: {}, 13: {}},
	ExcludedChannelIDs: map[int]struct{}{13: {}},
}
selected, err := model.GetRandomSatisfiedChannel("分组A", modelrouting.Seedance20, 0, "/v1/video/generations", filter)
require.NoError(t, err)
require.NotNil(t, selected)
assert.Equal(t, 12, selected.Id)
```

This proves the incompatible priority-100 channel is removed before priority selection. Add cases for nil allowlist preserving all legacy candidates, exclusion removing a previously failed channel, retry index selecting the next remaining unique priority, normalized-model fallback, and Advanced Custom path filtering composing with the route filter.

- [ ] **Step 2: Run the focused model tests and confirm the signature mismatch**

```powershell
go test ./model -run 'TestChannelRoutingFilter' -count=1
```

Expected: FAIL because `ChannelSelectFilter` and the filtered selector signature do not exist.

- [ ] **Step 3: Add a transport-agnostic selector filter**

Define in `model/channel_satisfy.go`:

```go
type ChannelSelectFilter struct {
	AllowedChannelIDs  map[int]struct{}
	ExcludedChannelIDs map[int]struct{}
}

func (f ChannelSelectFilter) Allows(channelID int) bool {
	if len(f.AllowedChannelIDs) > 0 {
		if _, ok := f.AllowedChannelIDs[channelID]; !ok { return false }
	}
	_, excluded := f.ExcludedChannelIDs[channelID]
	return !excluded
}

func filterChannelIDs(ids []int, filter ChannelSelectFilter) []int {
	if len(ids) == 0 || len(filter.AllowedChannelIDs) == 0 && len(filter.ExcludedChannelIDs) == 0 { return ids }
	filtered := make([]int, 0, len(ids))
	for _, id := range ids {
		if filter.Allows(id) { filtered = append(filtered, id) }
	}
	return filtered
}
```

Nil/empty maps mean no restriction. Never mutate cached ID slices.

- [ ] **Step 4: Apply the filter in the memory path**

Change the selector signature to:

```go
func GetRandomSatisfiedChannel(group, modelName string, retry int, requestPath string, filter ChannelSelectFilter) (*Channel, error)
```

For exact and normalized model lookup, apply both filters before collecting unique priorities:

```go
channels := filterChannelsByRequestPathAndModel(group2model2channels[group][modelName], requestPath, modelName)
channels = filterChannelIDs(channels, filter)
```

Keep the existing priority ordering, retry clamping, zero-weight smoothing, and weighted random code unchanged after the filtered slice is built.

- [ ] **Step 5: Apply the filter before DB priority selection**

Change `GetChannel` to the same filter parameter. The existing `getChannelQuery` computes a maximum priority before request-path or capability filtering and therefore cannot be reused. Query all enabled abilities for exact model, fall back to normalized model only when exact is empty, then apply path and route filters, compute remaining unique priorities, select the retry tier, and run the existing DB weight algorithm on that tier:

```go
func GetChannel(group, modelName string, retry int, requestPath string, filter ChannelSelectFilter) (*Channel, error) {
	abilities, err := getSelectableAbilities(group, modelName)
	if err != nil { return nil, err }
	abilities = filterAbilitiesByRequestPathAndModel(abilities, requestPath, modelName)
	abilities = filterAbilitiesByChannelSelectFilter(abilities, filter)
	abilities = abilitiesAtRetryPriority(abilities, retry)
	if len(abilities) == 0 { return nil, nil }
	channelID := weightedAbilityChannelID(abilities)
	var channel Channel
	if err := DB.First(&channel, "id = ?", channelID).Error; err != nil { return nil, err }
	return &channel, nil
}
```

The four named helpers each have multiple callers/tests or a stable selection responsibility. Preserve the current `weight + 10` DB weighting; this task changes candidate eligibility, not legacy weighting semantics.

- [ ] **Step 6: Update the sole service call site with a legacy empty filter**

Until Task 7 supplies capability filters, change both calls in `service/channel_select.go` to pass:

```go
model.ChannelSelectFilter{}
```

Do not add policy behavior in this commit.

- [ ] **Step 7: Format and run all model/service selector tests**

```powershell
gofmt -w model/channel_cache.go model/ability.go model/channel_satisfy.go model/channel_routing_filter_test.go service/channel_select.go
go test ./model ./service -run 'ChannelRoutingFilter|RandomSatisfiedChannel|ChannelSelect' -count=1
```

Expected: PASS in both memory-cache modes.

- [ ] **Step 8: Commit selector filtering**

```powershell
git add model/channel_cache.go model/ability.go model/channel_satisfy.go model/channel_routing_filter_test.go service/channel_select.go
git commit -m "feat(routing): filter channel candidates before selection"
```

---

### Task 7: Integrate Capability Decisions with Auto Groups, Affinity, Specific Channels, and Retry

**Files:**
- Create: `service/model_routing.go`
- Create: `service/model_routing_test.go`
- Modify: `service/channel_select.go`
- Modify: `middleware/distributor.go`
- Modify: `controller/relay.go`
- Modify: `types/error.go`

- [ ] **Step 1: Write failing group-evaluation tests**

Use cached policy snapshots and deterministic one-channel priority tiers. Cover these service contracts:

```go
func TestSelectCapabilityChannelPublishesTargetDecision(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	input := seedanceFactsInput(modelrouting.Seedance20, "1080p", 10, "16:9")
	param := &service.RetryParam{
		Ctx: c, TokenGroup: "分组A", ModelName: modelrouting.Seedance20,
		RequestPath: "/v1/video/generations", Retry: common.GetPointer(0), RoutingInput: &input,
	}
	channel, group, err := service.CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, "分组A", group)
	assert.Equal(t, 11, channel.Id)
	assert.True(t, common.GetContextKeyBool(c, constant.ContextKeyRoutingCapabilityMode))
	assert.Equal(t, 21, common.GetContextKeyInt(c, constant.ContextKeyRoutingTargetID))
	assert.Equal(t, "provider-1080p", common.GetContextKeyString(c, constant.ContextKeyRoutingUpstreamModel))
}
```

Add exact cases for:

| State | Expected |
|---|---|
| no enabled policy | existing legacy selection, capability context false |
| policy and target match, enabled ability exists | selected channel + target decision |
| legal facts match no target | typed 400 `no_compatible_route` |
| targets match but all target channels lack enabled abilities | typed 503 `compatible_channel_unavailable` |
| malformed cache snapshot injected by test hook | typed 500 `routing_policy_error` |
| target exists only on excluded channel | 503, never reuse failed channel |
| `duration=-1` | 400 `no_compatible_route` |
| first auto group no match, second group matches | second group and target selected |
| all auto groups have policies but no match | 400 |
| any auto group has compatible targets but all unavailable | 503 |
| an auto group has no policy and its legacy channel is available | legacy channel remains eligible for that concrete group |

- [ ] **Step 2: Run the service tests and verify capability selection is absent**

```powershell
go test ./service -run 'TestSelectCapability|TestAutoGroupCapability' -count=1
```

Expected: FAIL.

- [ ] **Step 3: Add stable errors and selection state**

Add to `types/error.go`:

```go
ErrorCodeNoCompatibleRoute             ErrorCode = "no_compatible_route"
ErrorCodeCompatibleChannelUnavailable ErrorCode = "compatible_channel_unavailable"
ErrorCodeRoutingPolicyError           ErrorCode = "routing_policy_error"
```

Add to `RetryParam`:

```go
RoutingInput       *modelrouting.FactsInput
ExcludedChannelIDs map[int]struct{}
```

and:

```go
func (p *RetryParam) ExcludeCapabilityChannel(channelID int) {
	if p.ExcludedChannelIDs == nil { p.ExcludedChannelIDs = map[int]struct{}{} }
	p.ExcludedChannelIDs[channelID] = struct{}{}
}
```

Define a typed service error:

```go
type ChannelSelectionError struct {
	Code       types.ErrorCode
	StatusCode int
	Err        error
	Diagnostics []modelrouting.Audit
}

func (e *ChannelSelectionError) Error() string { return e.Err.Error() }
```

- [ ] **Step 4: Evaluate one concrete group without DB access**

Implement this stable internal result:

```go
type groupRoutingResult struct {
	Capability bool
	Snapshot   modelrouting.PolicySnapshot
	Facts      modelrouting.Facts
	Evaluation modelrouting.Evaluation
}
```

`evaluateGroupRouting(group, modelName, input)` returns legacy when `input==nil` or no enabled snapshot exists. For a snapshot, verify its key matches the requested group/model, resolve defaults, evaluate targets, and return `no_compatible_route` when `CompatibleByChannel` is empty. Do not inspect channel status here; this distinction preserves 400 versus 503.

- [ ] **Step 5: Select through the existing priority/weight function**

For capability mode build the allowlist from `Evaluation.CompatibleByChannel`, pass the retry exclusion map, and call the filtered model selector:

```go
filter := model.ChannelSelectFilter{
	AllowedChannelIDs: allowedIDs(result.Evaluation.CompatibleByChannel),
	ExcludedChannelIDs: param.ExcludedChannelIDs,
}
channel, err := model.GetRandomSatisfiedChannel(group, param.ModelName, priorityRetry, param.RequestPath, filter)
if err != nil { return nil, routingPolicyFailure(err) }
if channel == nil {
	return nil, &ChannelSelectionError{
		Code: types.ErrorCodeCompatibleChannelUnavailable, StatusCode: http.StatusServiceUnavailable,
		Err: errors.New("compatible channels are unavailable"),
	}
}
publishRoutingDecision(param.Ctx, result, result.Evaluation.CompatibleByChannel[channel.Id])
```

`publishRoutingDecision` writes only the selected policy, target, normalized facts, upstream model, and aggregated mismatch counts to the typed context keys. Selection errors carry one diagnostic per evaluated concrete policy; failure diagnostics leave target/upstream empty and retain only policy ID, normalized facts, and aggregate mismatch counts. Before every top-level selection attempt call `clearRoutingDecision` so a failed retry cannot reuse the previous target. Never put prompt text, URLs, channel name, or keys into facts/diagnostics.

- [ ] **Step 6: Preserve auto-group retry state while aggregating capability errors**

Keep the existing `ContextKeyAutoGroupIndex`, retry reset, cross-group retry, and `ContextKeyAutoGroup` behavior. Within its loop:

- A concrete-group `no_compatible_route` records `sawCapabilityNoMatch=true` and advances to the next group.
- A concrete-group `compatible_channel_unavailable` records `sawCompatibleUnavailable=true` and advances according to the existing retry/group rules.
- A selected legacy or capability channel returns immediately.
- After all groups, return 503 if `sawCompatibleUnavailable`; otherwise return 400 if `sawCapabilityNoMatch`; otherwise retain the legacy nil-channel result.

Do not publish facts from skipped groups.

- [ ] **Step 7: Add reusable validation for a known channel**

Implement:

```go
func ValidateKnownChannelForRouting(param *RetryParam, group string, channelID int) (bool, error)
```

It returns `true` and publishes the matching target for capability mode, returns `true` without a route decision for legacy, and returns `false,nil` when only this channel is incompatible so affinity can be ignored. It returns a typed 400 only when the complete policy has no compatible targets, a typed 503 when targets are compatible but the requested channel is unavailable and no alternative selection is requested, and 500 on policy corruption.

Use this single function for token-specific channels, affinity channels, and any locked task channel. In the `RelayTask` locked-channel branch, call it before reusing `lockedCh`; if it returns false, build a local 400 `no_compatible_route`, and if it returns a typed error preserve its status/code. This prevents three subtly different bypass rules.

- [ ] **Step 8: Insert extraction and known-channel checks into `Distribute`**

Immediately after `getModelRequest` succeeds:

```go
routingInput, inputErr := extractSeedanceRoutingInput(c, modelRequest.Model)
if inputErr != nil {
	abortSeedanceRoutingError(c, http.StatusBadRequest, inputErr.Code, inputErr.Message)
	return
}
if routingInput != nil {
	common.SetContextKey(c, constant.ContextKeyRoutingFactsInput, *routingInput)
}
```

Pass `routingInput` into every `RetryParam`. Refactor the specific-channel branch and affinity branch to call `ValidateKnownChannelForRouting` for each concrete group considered. An incompatible affinity entry is ignored and optionally cleared by the existing affinity setting; it is not immediately returned to the user when another channel can match. A specific channel is mandatory, so an incompatible specific channel returns the appropriate local error and never bypasses the policy.

Do not ignore the return value of `SetupContextForSelectedChannel`. In capability mode, `channel:no_available_key` marks that channel unavailable: add it to `ExcludedChannelIDs`, clear its route decision, and re-enter selection at the same retry tier so another compatible channel can be chosen. If every compatible channel fails setup, return 503 `compatible_channel_unavailable`. For a mandatory specific channel, return that 503 immediately. Outside capability mode, return the existing setup error code/status instead of continuing with partially initialized channel context.

Add one output helper for official ARK shape:

```go
func abortSeedanceRoutingError(c *gin.Context, status int, code types.ErrorCode, message string) {
	c.AbortWithStatusJSON(status, gin.H{"error": gin.H{"code": code, "message": message}})
}
```

For non-official formats, use the existing OpenAI error envelope with the same stable code. Before aborting, call `RecordRoutingSelectionFailure(c, canonicalModel, selectionErr)`. That service writes an error log with channel ID 0, canonical model, request path, stable code/status, and `other.admin_info.routing_diagnostics`; it must not log prompt text, raw body, or media URLs. Call the same recorder when a retry-time `getChannel` receives `ChannelSelectionError`, so 400/503/500 local decisions are auditable even though no upstream channel handled the final attempt. Public responses never include `Diagnostics`.

- [ ] **Step 9: Write middleware tests for specific and affinity behavior**

Add cases where the affinity channel supports only 720p but the request asks for 1080p and another channel matches; assert the second channel is selected. Add a token-specific incompatible channel case asserting 400 and no upstream handler invocation. Add an auto group case asserting the concrete selected group is stored in `ContextKeyAutoGroup` and facts use that group. Add a compatible first channel with no enabled key and assert selection continues to the second compatible channel; when both have no keys, assert 503. For a no-match failure, query the error log as an admin and assert it contains only normalized facts/policy/mismatch counts; the public response body must not contain channel, policy, target, or upstream identifiers.

- [ ] **Step 10: Exclude the failed channel on task retry**

When building retry params in both general relay and `RelayTask`, recover the fact input from context:

```go
if input, ok := common.GetContextKeyType[modelrouting.FactsInput](c, constant.ContextKeyRoutingFactsInput); ok {
	retryParam.RoutingInput = &input
}
```

After an upstream failure and before the next selection, call:

```go
if common.GetContextKeyBool(c, constant.ContextKeyRoutingCapabilityMode) {
	retryParam.ExcludeCapabilityChannel(channel.Id)
}
```

The selector ignores exclusions in legacy mode. Map `ChannelSelectionError` through `types.NewErrorWithStatusCode` without replacing its code with `get_channel_failed`. In `RelayTask`, build the local `TaskError` with `string(channelErr.GetErrorCode())` and `channelErr.StatusCode`, so a retry-exhausted capability request returns 503 `compatible_channel_unavailable`.

- [ ] **Step 11: Run selection, distributor, and retry tests**

```powershell
gofmt -w service/model_routing.go service/model_routing_test.go service/channel_select.go middleware/distributor.go controller/relay.go types/error.go
go test ./service ./middleware ./controller -run 'Capability|Routing|Affinity|SpecificChannel|Retry' -count=1
```

Expected: PASS; no case falls back to an incompatible channel.

- [ ] **Step 12: Commit routing integration**

```powershell
git add service/model_routing.go service/model_routing_test.go service/channel_select.go middleware/distributor.go controller/relay.go types/error.go
git commit -m "feat(routing): integrate capability-aware channel selection"
```

---

### Task 8: Apply the Target Model Once and Keep Routing Data Admin-Only

**Files:**
- Modify: `relay/common/relay_info.go`
- Modify: `relay/helper/model_mapped.go`
- Create: `relay/helper/model_mapped_routing_test.go`
- Modify: `relay/relay_task.go`
- Modify: `relay/relay_task_seedance_test.go`
- Modify: `model/task.go`
- Modify: `service/log_info_generate.go`
- Modify: `service/task_billing.go`
- Modify: `service/task_billing_test.go`
- Modify: `controller/relay.go`
- Modify: `controller/task.go`

- [ ] **Step 1: Write failing route-model precedence tests**

Set both a capability target and a conflicting ordinary mapping. Assert the target wins exactly once, the canonical identity stays unchanged, and the request receives the upstream target:

```go
func TestModelMappedHelperPrefersCapabilityTarget(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("model_mapping", `{"doubao-seedance-2-0-260128":"ordinary-map","provider-1080p":"must-not-chain"}`)
	common.SetContextKey(c, constant.ContextKeyRoutingUpstreamModel, "provider-1080p")
	request := &dto.GeneralOpenAIRequest{Model: modelrouting.Seedance20}
	info := &relaycommon.RelayInfo{
		OriginModelName: modelrouting.Seedance20,
		ChannelMeta: &relaycommon.ChannelMeta{Routing: &modelrouting.Audit{TargetID: 21, UpstreamModel: "provider-1080p"}},
	}
	require.NoError(t, helper.ModelMappedHelper(c, info, request))
	assert.Equal(t, modelrouting.Seedance20, info.OriginModelName)
	assert.Equal(t, "provider-1080p", info.UpstreamModelName)
	assert.Equal(t, "provider-1080p", request.Model)
	assert.True(t, info.IsModelMapped)
}
```

Keep existing chain mapping and compact-response tests to prove legacy behavior is unchanged.

- [ ] **Step 2: Add the decision to `ChannelMeta` and initialize it from context**

Add:

```go
Routing *modelrouting.Audit
```

to `ChannelMeta`. In `InitChannelMeta`, recover the typed normalized facts and construct `Audit` only when capability mode and non-zero policy/target IDs are present. Initialize `UpstreamModelName` from `ContextKeyRoutingUpstreamModel` when set, otherwise from `ContextKeyOriginalModel` as today.

Each retry calls `InitChannelMeta`, so this refreshes channel, key, and route target together.

- [ ] **Step 3: Give capability mapping first and final precedence**

At the top of `ModelMappedHelper`, after ensuring `ChannelMeta` exists, add:

```go
if info.Routing != nil && strings.TrimSpace(info.Routing.UpstreamModel) != "" {
	info.UpstreamModelName = info.Routing.UpstreamModel
	info.IsModelMapped = info.UpstreamModelName != info.OriginModelName
	if request != nil { request.SetModelName(info.UpstreamModelName) }
	return nil
}
```

In the legacy branch replace direct `json.Unmarshal` with:

```go
if err := basecommon.UnmarshalJsonStr(modelMapping, &modelMap); err != nil {
	return fmt.Errorf("unmarshal_model_mapping_failed")
}
```

Alias the repository `common` package as `basecommon` and retain `relay/common` as `relaycommon` to avoid import ambiguity. Remove the direct `encoding/json` import.

- [ ] **Step 4: Run mapping tests**

```powershell
gofmt -w relay/common/relay_info.go relay/helper/model_mapped.go relay/helper/model_mapped_routing_test.go
go test ./relay/helper ./relay/common -run 'ModelMapped|Routing' -count=1
```

Expected: PASS.

- [ ] **Step 5: Write failing persistence and log-privacy tests**

Create a capability `RelayInfo`, call `model.InitTask`, and assert:

```go
task := model.InitTask(constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeDoubaoVideo)), info)
assert.Equal(t, modelrouting.Seedance20, task.Properties.OriginModelName)
assert.Empty(t, task.Properties.UpstreamModelName)
require.NotNil(t, task.PrivateData.Routing)
assert.Equal(t, 21, task.PrivateData.Routing.TargetID)
assert.Equal(t, "provider-1080p", task.PrivateData.Routing.UpstreamModel)
```

For task consumption, serialize the resulting `other` twice through the existing log formatting path: admin output must contain `other.admin_info.routing.upstream_model`; non-admin output must not contain `provider-1080p`, `target_id`, or `policy_id`. Assert `RecordConsumeLogParams.ModelName` remains the canonical model. Add a user task-list projection test whose stored `Task.Data` contains `provider-1080p`; the non-admin DTO must omit raw `Data` and upstream properties, while the admin DTO may retain raw task data.

- [ ] **Step 6: Persist route audit only in task private data**

Add:

```go
Routing *modelrouting.Audit `json:"routing,omitempty"`
```

to `TaskPrivateData`. In `InitTask`:

```go
if relayInfo.Routing != nil {
	routingCopy := *relayInfo.Routing
	privateData.Routing = &routingCopy
} else if relayInfo.UpstreamModelName != "" {
	properties.UpstreamModelName = relayInfo.UpstreamModelName
}
```

Always keep `properties.OriginModelName`. Continue storing `BillingContext.UpstreamModelName` because polling and settlement need it, and `PrivateData` is never serialized to users. Legacy mapped tasks keep their existing public property behavior to avoid a broad API change.

Change `TaskModel2Dto` to accept `includeAdmin bool`. For a capability task and `includeAdmin=false`, return a copied `Properties` value containing only `OriginModelName` and set `Data=nil`; status, progress, public task ID, result URL, timestamps, action, and quota remain available through existing top-level fields. `controller.tasksToDto` passes its existing `fillUser` admin flag into this parameter. The official ARK/OpenAI task query converters remain the richer allowlisted public response paths.

- [ ] **Step 7: Centralize admin-only routing log fields**

Add to `service/log_info_generate.go`:

```go
func appendRoutingAdminInfo(other map[string]interface{}, routing *modelrouting.Audit) {
	if other == nil || routing == nil { return }
	adminInfo, _ := other["admin_info"].(map[string]interface{})
	if adminInfo == nil { adminInfo = map[string]interface{}{}; other["admin_info"] = adminInfo }
	adminInfo["routing"] = routing
}

func AppendRoutingAdminInfoFromContext(c *gin.Context, other map[string]interface{}) {
	if c == nil || !common.GetContextKeyBool(c, constant.ContextKeyRoutingCapabilityMode) { return }
	routing := routingAuditFromContext(c)
	appendRoutingAdminInfo(other, routing)
}
```

Call it from `GenerateTextOtherInfo`, `LogTaskConsumption`, error-log creation in `controller/relay.go`, and task settlement log builders. In capability mode do not write `upstream_model_name` at the top level; place it only inside `admin_info.routing`. Legacy mapped-model logging remains unchanged.

- [ ] **Step 8: Preserve canonical model for price and responses**

Extend `relay/relay_task_seedance_test.go` through the request lifecycle. The test must assert:

- `OriginModelName` is the canonical ID before and after `ModelMappedHelper`.
- `UpstreamModelName` is the route target.
- `ModelPriceHelperPerCall` looks up the canonical model configuration.
- `TaskBillingContext.OriginModelName` is canonical and its upstream field is private.
- ARK submit/query response `model` is canonical.
- `model.GetGroupEnabledModels` and model-list response contain canonical IDs, never target IDs.

Do not change billing expression or ratio lookup to the upstream model.

- [ ] **Step 9: Run relay, billing, task, and log tests**

```powershell
gofmt -w relay/relay_task.go relay/relay_task_seedance_test.go model/task.go service/log_info_generate.go service/task_billing.go service/task_billing_test.go controller/relay.go controller/task.go
go test ./relay ./relay/helper ./model ./service ./controller -run 'Routing|Seedance|TaskConsumption|ModelMapped|FormatUserLogs' -count=1
```

Expected: PASS and no user-visible serialization contains the route upstream model.

- [ ] **Step 10: Commit mapping, billing identity, and privacy**

```powershell
git add relay/common/relay_info.go relay/helper/model_mapped.go relay/helper/model_mapped_routing_test.go relay/relay_task.go relay/relay_task_seedance_test.go model/task.go service/log_info_generate.go service/task_billing.go service/task_billing_test.go controller/relay.go controller/task.go
git commit -m "feat(routing): apply targets without exposing upstream models"
```

---

### Task 9: Add the Frontend Routing Data Contract and Read-Only Models Section

**Files:**
- Create: `web/default/src/features/model-routing/types.ts`
- Create: `web/default/src/features/model-routing/api.ts`
- Create: `web/default/src/features/model-routing/query-keys.ts`
- Create: `web/default/src/features/model-routing/index.tsx`
- Create: `web/default/src/features/model-routing/components/routing-policies-table.tsx`
- Modify: `web/default/src/features/models/section-registry.tsx`
- Modify: `web/default/src/features/models/index.tsx`
- Modify: `web/default/src/features/models/types.ts`
- Modify: `web/default/src/features/models/components/models-provider.tsx`
- Modify: `web/default/src/routes/_authenticated/models/$section.tsx`

- [ ] **Step 1: Define exact Zod API and form schemas**

Create `types.ts` with fixed enum arrays and a duration XOR refinement:

```ts
import { z } from 'zod'

export const CANONICAL_SEEDANCE_MODELS = [
  'doubao-seedance-2-0-260128',
  'doubao-seedance-2-0-fast-260128',
  'doubao-seedance-2-0-mini-260615',
] as const

export const OUTPUT_RESOLUTIONS = ['480p', '720p', '1080p', '4k'] as const
export const MAX_TASK_DURATION_SECONDS = 3600
export const ASPECT_RATIOS = [
  '16:9',
  '4:3',
  '1:1',
  '3:4',
  '9:16',
  '21:9',
  'adaptive',
] as const

const durationConstraintSchema = z
  .object({
    mode: z.enum(['values', 'range']),
    values: z.array(z.number().int().min(1).max(MAX_TASK_DURATION_SECONDS)).default([]),
    min: z.number().int().min(1).max(MAX_TASK_DURATION_SECONDS).optional(),
    max: z.number().int().min(1).max(MAX_TASK_DURATION_SECONDS).optional(),
  })
  .superRefine((value, ctx) => {
    if (value.mode === 'values' && value.values.length === 0) {
      ctx.addIssue({ code: 'custom', path: ['values'], message: 'At least one duration is required' })
    }
    if (value.mode === 'range' && (value.min === undefined || value.max === undefined || value.min > value.max)) {
      ctx.addIssue({ code: 'custom', path: ['min'], message: 'Enter a valid inclusive duration range' })
    }
  })
```

Use the exported `MAX_TASK_DURATION_SECONDS` constant in both Zod bounds and numeric inputs. Its value mirrors the backend `relaycommon.MaxTaskDurationSeconds`, giving the duplicated protocol ceiling one frontend definition.

Define `routeTargetFormSchema` with channel ID, name, upstream model, integer target priority, enabled, output resolutions, optional generation resolution, upscaled, duration, ratios, reference limits (`9/3/3` bounds), and `supports_real_person` as `'unknown' | 'yes' | 'no'`. Its `superRefine` must enforce one output plus different generation resolution when upscaled, and no generation resolution when native.

Define separate API schemas matching backend JSON (`supports_real_person` as `boolean | null`, `durations` as values or min/max). Add `routingPolicyErrorSchema` for `{success:false,message,code,data?:{field?,target_indexes?}}`. Add pure `toWriteRequest` and `fromPolicyResponse` functions that convert between form and API shapes without retaining UI-only `mode`/tri-state strings.

- [ ] **Step 2: Add typed API operations and query keys**

Create these functions in `api.ts` using the shared `api` client:

```ts
export async function listRoutingPolicies(params: RoutingPolicyListParams) {
  const response = await api.get('/api/routing-policies', { params })
  return routingPolicyListResponseSchema.parse(response.data)
}

export async function getRoutingPolicy(id: number) {
  const response = await api.get(`/api/routing-policies/${id}`)
  return routingPolicyResponseSchema.parse(response.data)
}

export async function listRoutingCandidates(groupName: string, model: string) {
  const response = await api.get('/api/routing-policies/candidates', {
    params: { group_name: groupName, model },
  })
  return routingCandidateResponseSchema.parse(response.data)
}

export async function createRoutingPolicy(payload: RoutingPolicyWriteRequest) {
  const response = await api.post('/api/routing-policies', payload, { skipBusinessError: true })
  return routingPolicyResponseSchema.parse(response.data)
}

export async function updateRoutingPolicy(id: number, payload: RoutingPolicyWriteRequest) {
  const response = await api.put(`/api/routing-policies/${id}`, payload, { skipBusinessError: true })
  return routingPolicyResponseSchema.parse(response.data)
}

export async function updateRoutingPolicyStatus(id: number, enabled: boolean) {
  const response = await api.post(`/api/routing-policies/${id}/status`, { enabled }, { skipBusinessError: true })
  return routingPolicyResponseSchema.parse(response.data)
}

export async function deleteRoutingPolicy(id: number) {
  const response = await api.delete(`/api/routing-policies/${id}`, { skipBusinessError: true })
  return z.object({ success: z.literal(true) }).passthrough().parse(response.data)
}
```

Create key factories rooted at `['routing-policies']` for lists, detail, and candidates. Candidate keys include both group and model.

- [ ] **Step 3: Add the `routing` section and URL state**

Add this section after metadata and before deployments:

```ts
{
  id: 'routing',
  titleKey: 'Routing policies',
  build: () => null,
},
```

Extend `ModelTabCategory` to `'metadata' | 'routing' | 'deployments'`. Extend route search validation:

```ts
rPage: z.number().optional().catch(1),
rPageSize: z.number().optional().catch(10),
rGroup: z.string().optional().catch(''),
rModel: z.string().optional().catch(''),
rChannel: z.number().optional(),
```

Do not reuse metadata/deployment query keys; navigating among tabs must retain each section's filters independently.

- [ ] **Step 4: Render a focused routing workspace shell**

In `models/index.tsx`, add section metadata and render branches explicitly instead of a two-way fallback:

```tsx
<div className='min-h-0 flex-1'>
  {activeSection === 'metadata' && <ModelsTable />}
  {activeSection === 'routing' && <ModelRouting />}
  {activeSection === 'deployments' && <DeploymentsSection />}
</div>
```

Only metadata shows `ModelsPrimaryButtons`; only deployments shows `Create deployment`; `ModelRouting` owns its own compact create action so the models page does not need routing editor state.

Create `model-routing/index.tsx` as:

```tsx
export function ModelRouting() {
  return <RoutingPoliciesTable />
}
```

Create `components/routing-policies-table.tsx` as a complete read-only list: use `DataTablePage`/`useDataTable`, the Task 9 list query, URL-backed group/model/channel filters, pagination, and columns for group, canonical model, defaults, target count, enabled status, and updated time. The empty state is `No routing policies found`; the error state shows the server message and a retry icon button. Do not render create/edit/delete controls in this commit and do not add feature-explanation text.

- [ ] **Step 5: Run frontend static checks**

No frontend unit runner is configured. Verify the data contract with TypeScript and lint:

```powershell
bun run typecheck
bun run lint
```

Run from `web/default`. Expected: both commands exit 0.

- [ ] **Step 6: Commit frontend routing foundations**

```powershell
git add web/default/src/features/model-routing web/default/src/features/models/section-registry.tsx web/default/src/features/models/index.tsx web/default/src/features/models/types.ts web/default/src/features/models/components/models-provider.tsx 'web/default/src/routes/_authenticated/models/$section.tsx'
git commit -m "feat(web): add model routing workspace"
```

---

### Task 10: Add Policy Mutations and the Structured Target Editor

**Files:**
- Modify: `web/default/src/features/model-routing/components/routing-policies-table.tsx`
- Create: `web/default/src/features/model-routing/components/routing-policy-drawer.tsx`
- Create: `web/default/src/features/model-routing/components/route-target-editor.tsx`
- Create: `web/default/src/features/model-routing/components/routing-policy-dialogs.tsx`
- Modify: `web/default/src/features/model-routing/index.tsx`

- [ ] **Step 1: Extend the read-only policy list with mutation actions**

Keep the Task 9 `DataTablePage`/`useDataTable`, React Query query, URL-backed filters, pagination, loading, error, and empty states. Add an icon action menu to the existing columns and a compact `Plus` button labeled `Create policy`. The query remains:

```ts
const policiesQuery = useQuery({
  queryKey: routingPolicyQueryKeys.list({
    group_name: search.rGroup || undefined,
    model: search.rModel || undefined,
    channel_id: search.rChannel,
    p: search.rPage,
    page_size: search.rPageSize,
  }),
  queryFn: () => listRoutingPolicies({
    group_name: search.rGroup || undefined,
    model: search.rModel || undefined,
    channel_id: search.rChannel,
    p: search.rPage,
    page_size: search.rPageSize,
  }),
})
```

Use `Pencil`, `Copy`, `Power`, and `Trash2` icons with tooltips/menu labels. Status toggling and deletion require confirmation; copying opens a create drawer with the source policy ID removed and `enabled=false`.

- [ ] **Step 2: Set explicit editor defaults**

Export one factory, not a shared mutable object:

```ts
export function createEmptyPolicyForm(): RoutingPolicyFormValues {
  return {
    group_name: '',
    model: CANONICAL_SEEDANCE_MODELS[0],
    enabled: false,
    defaults: { output_resolution: '720p', duration_seconds: 10, aspect_ratio: '16:9' },
    targets: [],
  }
}

export function createEmptyTarget(): RouteTargetFormValues {
  return {
    channel_id: 0,
    name: '',
    upstream_model: '',
    target_priority: 0,
    enabled: true,
    output_resolutions: ['720p'],
    generation_resolution: undefined,
    upscaled: false,
    durations: { mode: 'range', values: [], min: 4, max: 15 },
    aspect_ratios: [],
    reference_limits: { images: 9, videos: 3, audios: 3 },
    supports_real_person: 'unknown',
  }
}
```

- [ ] **Step 3: Build the drawer around React Hook Form**

Use `zodResolver(routingPolicyFormSchema)` and `useFieldArray({ name: 'targets' })`. The top unframed form region contains group/model selectors, an enabled switch, and default resolution/duration/ratio controls. Group and model changes invalidate the candidate query and clear target channel selections that are no longer returned.

Fetch candidates only when both values are non-empty:

```ts
const candidatesQuery = useQuery({
  queryKey: routingPolicyQueryKeys.candidates(groupName, modelName),
  queryFn: () => listRoutingCandidates(groupName, modelName),
  enabled: groupName.length > 0 && modelName.length > 0,
})
```

When the candidate list is empty, render `No channels declare this group and canonical model` beside the channel selector; the administrator must add the canonical model ID to the channel's Models field before it can become a route target. The drawer footer has `Cancel` and `Save policy`; disable save while the mutation is pending. On success invalidate `routingPolicyQueryKeys.all`, close the drawer, and show a translated success toast.

- [ ] **Step 4: Build each target as one repeated bordered item**

`RouteTargetEditor` uses a single `rounded-md border` repeated-item container, never a card nested inside a card. Provide:

- Channel combobox showing channel name, ID, status, priority, and weight.
- Name and upstream-model text inputs.
- Numeric priority spinner and enabled switch.
- Resolution checkbox swatches for 480p/720p/1080p/4k.
- Native/upscaled toggle; upscaled reveals a generation-resolution select.
- Duration segmented control (`Discrete values` / `Range`); discrete values are editable integer chips, range uses two numeric inputs.
- Aspect ratio multi-select; empty state visibly reads `Any ratio`.
- Three bounded numeric inputs for images/videos/audios with `9/3/3` maxima.
-真人 support toggle group with `Unknown`, `Supported`, `Not supported`.
- `Copy` and icon-only `Trash2` actions with tooltips.

Copy inserts immediately after the source, clears any server-generated ID, appends ` copy` to the name, and retains constraints. Removing the last target is allowed only while the policy is disabled; when enabled, surface the schema error at `targets`.

- [ ] **Step 5: Map server overlap errors to exact target rows**

Use the backend error data rather than a generic toast:

```ts
function applyRoutingPolicyError(
  error: unknown,
  form: UseFormReturn<RoutingPolicyFormValues>
) {
  const payload = getApiErrorPayload(error)
  if (payload?.code === 'routing_target_overlap') {
    for (const index of payload.data?.target_indexes ?? []) {
      form.setError(`targets.${index}.target_priority`, {
        type: 'server',
        message: 'This target overlaps another target at the same priority',
      })
    }
    return
  }
  if (payload?.data?.field) {
    form.setError(payload.data.field as FieldPath<RoutingPolicyFormValues>, {
      type: 'server',
      message: payload.message,
    })
    return
  }
  toast.error(payload?.message ?? 'Failed to save routing policy')
}
```

Define the referenced extractor in the same drawer module and import `isAxiosError` from `axios`:

```ts
function getApiErrorPayload(error: unknown): RoutingPolicyError | undefined {
  if (!isAxiosError(error)) return undefined
  const parsed = routingPolicyErrorSchema.safeParse(error.response?.data)
  return parsed.success ? parsed.data : undefined
}
```

Pass every literal message through `t(...)` at render time; keep the mapping function's strings as English i18n keys.

- [ ] **Step 6: Implement enable/disable/delete dialogs**

`RoutingPolicyDialogs` uses `ConfirmDialog`. Enable calls the status endpoint and can surface `default_route_unavailable`; disable explains only that new requests return to legacy routing, not that data is deleted; delete shows group/model and confirms targets are removed. Invalidate list/detail/candidate queries after mutation.

- [ ] **Step 7: Wire mutations into the feature entry without duplicate state**

`model-routing/index.tsx` owns only `editingPolicy`, `copyingPolicy`, and dialog state, renders `RoutingPoliciesTable` and one `RoutingPolicyDrawer`, and passes callbacks. The table continues to own URL pagination/filter state; the drawer owns form state.

- [ ] **Step 8: Run frontend checks**

```powershell
bun run format
bun run typecheck
bun run lint
```

Run from `web/default`. Expected: all exit 0; `format` changes only touched frontend files and required generated formatting.

- [ ] **Step 9: Commit the editor workflow**

```powershell
git add web/default/src/features/model-routing
git commit -m "feat(web): manage capability routing targets"
```

---

### Task 11: Add Channel Links and Complete Six-Locale Translations

**Files:**
- Modify: `model/channel.go`
- Modify: `controller/channel.go`
- Modify: `controller/channel_authz_test.go`
- Modify: `web/default/src/features/channels/types.ts`
- Modify: `web/default/src/features/channels/components/channels-columns.tsx`
- Modify: `web/default/src/i18n/locales/en.json`
- Modify: `web/default/src/i18n/locales/zh.json`
- Modify: `web/default/src/i18n/locales/fr.json`
- Modify: `web/default/src/i18n/locales/ru.json`
- Modify: `web/default/src/i18n/locales/ja.json`
- Modify: `web/default/src/i18n/locales/vi.json`

- [ ] **Step 1: Write failing channel count response tests**

Seed two targets on channel 11, one target on channel 12, and an unrelated channel. Assert regular list, search, and detail responses contain `routing_target_count` without exposing keys:

```go
assert.Equal(t, float64(2), channelByID(response, 11)["routing_target_count"])
assert.NotContains(t, channelByID(response, 11), "key")
```

Tag-mode aggregate rows are assembled from child channels; assert the displayed aggregate count is the sum of its children in the frontend, not a synthetic database channel count.

- [ ] **Step 2: Populate counts with one grouped query per response**

Add a non-persistent field:

```go
RoutingTargetCount int64 `json:"routing_target_count" gorm:"-"`
```

and a batch loader:

```go
func FillRoutingTargetCounts(channels []*Channel) error {
	ids := make([]int, 0, len(channels))
	byID := make(map[int]*Channel, len(channels))
	for _, channel := range channels {
		if channel == nil { continue }
		ids = append(ids, channel.Id)
		byID[channel.Id] = channel
	}
	if len(ids) == 0 { return nil }
	var counts []struct { ChannelID int; Count int64 }
	if err := DB.Model(&RouteTarget{}).Select("channel_id, count(*) as count").Where("channel_id IN ?", ids).Group("channel_id").Scan(&counts).Error; err != nil { return err }
	for _, count := range counts { byID[count.ChannelID].RoutingTargetCount = count.Count }
	return nil
}
```

Call it before `clearChannelInfo` for `GetAllChannels`, `SearchChannels`, and `GetChannel`. A count-query failure returns the same business-error shape as the surrounding channel query; do not silently report zero.

- [ ] **Step 3: Add a compact target-count link to channel rows**

Extend the Zod schema:

```ts
routing_target_count: z.number().int().nonnegative().default(0),
```

Add a `Route`/`Waypoints` icon cell or name-adjacent badge. For a regular channel, clicking it navigates to:

```tsx
void navigate({
  to: '/models/$section',
  params: { section: 'routing' },
  search: (previous) => ({ ...previous, rChannel: channel.id, rPage: 1 }),
})
```

Show the numeric count even when zero; disable navigation for zero. In card layout, use icon + number with `Routing targets` tooltip. In tag aggregate rows, render the sum of `children[].routing_target_count` and leave it non-clickable because one row represents multiple channel IDs.

- [ ] **Step 4: Add exact base translations**

Use English source strings as flat keys. Add at least this exact core set; every additional key introduced by Tasks 9-10 must follow the same locale-specific pattern rather than retaining English values:

```json
{
  "Routing policies": "Routing policies",
  "Create policy": "Create policy",
  "Edit routing policy": "Edit routing policy",
  "Canonical model": "Canonical model",
  "Routing targets": "Routing targets",
  "Target priority": "Target priority",
  "Upstream model": "Upstream model",
  "Output resolutions": "Output resolutions",
  "Generation resolution": "Generation resolution",
  "Upscaled": "Upscaled",
  "Discrete values": "Discrete values",
  "Any ratio": "Any ratio",
  "Require real person": "Require real person",
  "Supported": "Supported",
  "Not supported": "Not supported",
  "This target overlaps another target at the same priority": "This target overlaps another target at the same priority",
  "No compatible route supports this request": "No compatible route supports this request",
  "Compatible channels are unavailable": "Compatible channels are unavailable"
}
```

Use these non-English values for the same keys, in the same order:

```text
zh: 模型路由策略｜新建策略｜编辑路由策略｜本站模型｜路由目标｜目标优先级｜上游模型｜输出分辨率｜生成分辨率｜自动超分｜离散值｜不限比例｜要求支持真人｜支持｜不支持｜该目标与相同优先级的另一目标重叠｜没有兼容路由支持此请求｜兼容渠道当前不可用
fr: Politiques de routage｜Créer une politique｜Modifier la politique de routage｜Modèle canonique｜Cibles de routage｜Priorité de la cible｜Modèle en amont｜Résolutions de sortie｜Résolution de génération｜Mise à l’échelle｜Valeurs discrètes｜Tous les formats｜Exiger la prise en charge des personnes réelles｜Pris en charge｜Non pris en charge｜Cette cible chevauche une autre cible de même priorité｜Aucune route compatible ne prend en charge cette requête｜Les canaux compatibles sont indisponibles
ru: Политики маршрутизации｜Создать политику｜Изменить политику маршрутизации｜Каноническая модель｜Цели маршрутизации｜Приоритет цели｜Вышестоящая модель｜Выходные разрешения｜Разрешение генерации｜Масштабирование｜Дискретные значения｜Любое соотношение｜Требовать поддержку реальных людей｜Поддерживается｜Не поддерживается｜Эта цель пересекается с другой целью того же приоритета｜Нет совместимого маршрута для этого запроса｜Совместимые каналы недоступны
ja: ルーティングポリシー｜ポリシーを作成｜ルーティングポリシーを編集｜正規モデル｜ルーティング先｜ターゲット優先度｜アップストリームモデル｜出力解像度｜生成解像度｜アップスケール｜離散値｜任意の比率｜実在人物のサポートを要求｜サポートあり｜サポートなし｜同じ優先度の別ターゲットと条件が重複しています｜このリクエストをサポートする互換ルートがありません｜互換チャネルを利用できません
vi: Chính sách định tuyến｜Tạo chính sách｜Chỉnh sửa chính sách định tuyến｜Mô hình chuẩn｜Đích định tuyến｜Độ ưu tiên đích｜Mô hình thượng nguồn｜Độ phân giải đầu ra｜Độ phân giải tạo｜Nâng độ phân giải｜Giá trị rời rạc｜Mọi tỷ lệ｜Yêu cầu hỗ trợ người thật｜Có hỗ trợ｜Không hỗ trợ｜Đích này chồng lấn với một đích khác có cùng độ ưu tiên｜Không có tuyến tương thích hỗ trợ yêu cầu này｜Các kênh tương thích hiện không khả dụng
```

- [ ] **Step 5: Synchronize and verify translation quality**

```powershell
bun run i18n:sync
bun run typecheck
bun run lint
bun run build
```

Run from `web/default`. Expected: all exit 0. After `i18n:sync`, inspect the diff and replace any newly copied English values in `zh/fr/ru/ja/vi`; the final diff must not leave English fallback values for model-routing keys.

- [ ] **Step 6: Run backend channel tests**

```powershell
gofmt -w model/channel.go controller/channel.go controller/channel_authz_test.go
go test ./model ./controller -run 'RoutingTargetCount|ChannelReadOnly|GetAllChannels|SearchChannels' -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit channel navigation and translations**

```powershell
git add model/channel.go controller/channel.go controller/channel_authz_test.go web/default/src/features/channels/types.ts web/default/src/features/channels/components/channels-columns.tsx web/default/src/i18n/locales
git commit -m "feat(web): link channels to routing policies"
```

---

### Task 12: Prove the Routing Matrix End to End and Perform Browser QA

**Files:**
- Create: `e2e/seedance_capability_routing_e2e_test.go`
- Modify: `docs/superpowers/specs/2026-07-23-seedance-capability-routing-design.md` only if implementation reveals an accepted-contract correction; otherwise leave it unchanged.

- [ ] **Step 1: Build recording mock upstreams and deterministic fixtures**

Create two `httptest.Server` instances that record method, path, body, and authorization. Configure two `constant.ChannelTypeNewAPIVideo` channels named `A1` and `A1_copy` in `分组A` with the three canonical models; this keeps the test protocol local and accepts arbitrary collected upstream IDs. Set channel priorities to 100 and 90 respectively and non-zero weights. Use policy fixtures equivalent to this exact matrix:

| Canonical model | Channel | Target priority | Upstream model | Output | Duration | Ratio | Refs |真人 |
|---|---|---:|---|---|---|---|---|---|
| standard | A1 | 100 | `bb-seedance2.0-1080p-pro-gz-15s` | 1080p | 15 | 9:16 | 9/3/3 | false |
| standard | A1 | 100 | `bb-seedance2.0-720p-pro-gz-15s` | 720p | 15 | 9:16 | 9/3/3 | false |
| standard | A1_copy | 90 | `mg-seedance2.0-720p-pro` | 720p | 4..15 | any | 4/3/1 | true |
| fast | A1 | 100 | `bb-seedance2.0-720p-fast-gz-15s` | 720p | 15 | 9:16 | 9/3/3 | false |
| fast | A1_copy | 90 | `mg-seedance2.0-720p-fast` | 720p | 4..15 | any | 4/3/1 | true |
| mini | A1_copy | 90 | `mg-seedance2.0-720p-mini` | 720p | 4..15 | any | 4/3/1 | true |
| standard | A1_copy | 110 | `lec-feituo-seedance-2-0-my-upscaled-1080p` | 1080p, generated 720p, upscaled | 4..15 | any | 4/3/1 | true |

The two A1 standard targets are allowed at equal priority because their output resolution sets are disjoint. The A1_copy 1080 target is higher priority than other targets in the same channel, proving target priority does not become channel priority.

- [ ] **Step 2: Write an end-to-end request matrix**

Submit official ARK requests through the full middleware/router/controller/relay stack and assert:

| Request | Expected |
|---|---|
| standard, 1080p, 15s, 9:16, no refs | A1 `bb-...1080p...` |
| standard, 1080p, 10s, 16:9,真人 true, 4/3/1 | A1_copy upscaled target |
| standard, 720p, 15s, 9:16, 9/3/3 | A1 720 target |
| standard, 720p, 10s, 16:9,真人 true, 4/3/1 | A1_copy 720 target |
| fast, 720p, 15s, 9:16 | A1 fast target |
| fast, 720p, 10s,真人 true | A1_copy fast target |
| mini, 720p, 10s,真人 true | A1_copy mini target |
| standard, 4k | 400 `no_compatible_route` |
| standard, `duration=-1` | 400 `no_compatible_route` |
| 5 images against a `431`-only compatible shape | 400 `no_compatible_route` |
| malformed duration or 10 images | `InvalidParameter.*`, not route error |
| matching request after both channels are disabled | 503 `compatible_channel_unavailable` |

For every successful request inspect the mock body and assert canonical `model` was replaced by the selected upstream model and `routing` is absent.

- [ ] **Step 3: Prove retry excludes the whole failed channel**

Configure A1 to return 500 and A1_copy to return success for a request both can match. Give A1 two matching targets at different target priorities. Assert the request sequence is exactly `[A1 once, A1_copy once]`; the second A1 target is never attempted. Assert the public task has one canonical model and the private audit records the successful A1_copy target.

- [ ] **Step 4: Prove model and log privacy end to end**

After a successful task:

- ARK submit/query/list responses contain only the canonical model.
- User task and user log endpoints do not contain any of the seven fixture upstream IDs, `policy_id`, or `target_id`.
- Admin log data contains `other.admin_info.routing` with policy ID, target ID, normalized facts, and the selected upstream model.
- Model list/pricing endpoint contains the three canonical IDs and no fixture upstream ID.
- The consumed quota equals the canonical model's configured price even when the target model has a different or missing ratio entry.

- [ ] **Step 5: Run the isolated E2E suite**

```powershell
gofmt -w e2e/seedance_capability_routing_e2e_test.go
go test ./e2e -run 'TestSeedanceCapabilityRouting' -count=1 -v
```

Expected: PASS with no external network calls and no real paid tasks.

- [ ] **Step 6: Run the complete backend and frontend verification**

From the repository root:

```powershell
go test ./pkg/modelrouting ./model ./service ./middleware ./controller ./router ./relay/... ./e2e -count=1
```

Expected: PASS.

From `web/default`:

```powershell
bun run format:check
bun run copyright:check
bun run typecheck
bun run lint
bun run build
```

Expected: all commands exit 0.

- [ ] **Step 7: Start the implementation build for browser verification**

Start the Go server on a free port using the normal local development environment:

```powershell
go run . --port 3002
```

Expected: server listens on `http://127.0.0.1:3002` and migrations finish without errors. Keep this process running.

In a second terminal from `web/default`:

```powershell
$env:VITE_REACT_APP_SERVER_URL='http://127.0.0.1:3002'
bun run dev -- --port 3003
```

Expected: Rsbuild reports `http://127.0.0.1:3003`. If either port is occupied, increment both ports and keep the proxy URL aligned.

- [ ] **Step 8: Verify the desktop workflow in the in-app browser**

Use the `browser:control-in-app-browser` skill, open `http://127.0.0.1:3003/models/routing`, and verify at 1440x900:

1. The Models page shows Metadata, Routing policies, and Deployments tabs without wrapping over the title/actions.
2. Create a disabled `分组A + doubao-seedance-2-0-260128` policy.
3. Add A1 and A1_copy targets, including one native 720p and one 720p-to-1080p upscaled target.
4. Switch duration between discrete and range modes and verify mutually exclusive fields are removed from the payload.
5. Enter an overlapping same-priority target; verify both target rows receive inline errors from the server.
6. Fix priority, save, reopen, and verify all structured values round-trip.
7. Enable, disable, copy, and delete with confirmation dialogs.
8. Open `/channels`, verify each regular channel shows target count, and click A1's count to return to `/models/routing` filtered by A1.

Capture `artifacts/seedance-routing-desktop.png` for review; do not commit the screenshot.

- [ ] **Step 9: Verify responsive layout and interaction safety**

At 390x844, repeat opening the list and editor. Confirm:

- no horizontal page overflow;
- drawer content scrolls while footer actions remain reachable;
- model IDs and upstream IDs wrap without overlapping buttons;
- resolution, ratio, and真人 controls remain within their parent;
- copy/delete icon buttons have tooltips and stable 36px minimum hit areas;
- no card is nested in another card, and repeated target items use at most `rounded-md`;
- empty, loading, error, disabled, and mutation-pending states are all visible and coherent.

Capture `artifacts/seedance-routing-mobile.png`; do not commit it.

- [ ] **Step 10: Review the final diff for scope and protected metadata**

```powershell
git status --short
git diff --check
git diff --stat
git diff -- docs/superpowers/specs/2026-07-23-seedance-capability-routing-design.md
```

Expected: no whitespace errors, no unrelated original-workspace files, no changes removing or replacing protected project/organization identifiers, and no spec diff unless an explicitly documented accepted-contract correction was necessary.

- [ ] **Step 11: Commit acceptance coverage**

```powershell
git add e2e/seedance_capability_routing_e2e_test.go
git commit -m "test(routing): cover Seedance capability routing end to end"
```

Do not commit screenshots, local databases, logs, or environment files.

---

## Rollout Checklist

1. Deploy migrations, cache, selector, API, and UI with no policy rows; every existing group/model remains in `legacy` mode.
2. In A1 and A1_copy, replace public channel model entries with the applicable canonical IDs while retaining ordinary `model_mapping` only as an emergency legacy rollback path.
3. Create three disabled policies for `分组A`, one per canonical model, and enter all verified targets with structured constraints. Do not enter cost fields.
4. Run the mock matrix and channel tests, then use the admin diagnostics to verify facts, policy IDs, target IDs, and mismatch reasons without issuing paid tasks.
5. Enable standard, Fast, and Mini policies one at a time. Observe counts for `no_compatible_route`, `compatible_channel_unavailable`, retries, and automatic channel disables before enabling the next model.
6. Add the remaining collected upstreams incrementally. Keep same-channel/same-priority ranges disjoint and model every upscaled 1080p target as output `1080p` with its actual generation resolution recorded separately.
7. To roll back one group/model immediately, disable its policy; the selector returns to the unchanged legacy channel/mapping behavior for that exact key.

---

## Completion Criteria

- All three canonical model IDs route only to targets compatible with normalized group-specific facts.
- Target priority is confined to a channel; existing channel priority/weight, auto-group, affinity, retry, automatic disable, and multi-key behavior remain in control across channels.
- Memory-cache and DB selection paths both filter before priority selection.
- Specific/affinity/locked channels cannot bypass a capability policy, and retry never reuses a failed capability channel.
- `routing` never reaches either upstream adapter.
- `no_compatible_route`, `compatible_channel_unavailable`, and `routing_policy_error` have stable status/code behavior.
- Capability target/upstream details appear only in task private data and admin log information.
- SQLite migration/CRUD/cache tests pass; implementation uses only GORM/portable SQL behavior supported by MySQL and PostgreSQL.
- Admin routing UI, channel target counts, six translations, desktop layout, and mobile layout pass static and browser verification.
