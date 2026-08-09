# 分组能力路由档案实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将现有计费分组扩展为可分配给下游账号和 Token 的动态能力套餐，实时继承 `default` 路由目标池，并按真人能力、供应商成本模式和人工排除条件筛选目标。

**Architecture:** 扩展现有 `GroupRoutingRequirements` JSON 配置，新增可兼容旧字段的动态档案。后端新增一个共享静态匹配器，管理端预览、模型目录和真实请求路由都调用它；真实请求继续在静态匹配结果上执行素材约束、严格成本和毛利门禁。前端在现有分组详情中增加能力编辑器，并提供分页目标目录，不复制 `default` 路由策略。

**Tech Stack:** Go 1.22、Gin、GORM v2、SQLite/MySQL/PostgreSQL、React 19、TypeScript、TanStack Query、Zod、Base UI、Tailwind CSS、Bun、Vitest/Node test、React Testing Library、Go E2E。

**Spec:** `docs/superpowers/specs/2026-08-09-group-capability-routing-profiles-design.md`

---

## 文件结构

### 后端新增

- `service/group_routing_profile.go`：稳定目标键、静态能力匹配、批量成本规则解析、预览分页、激活校验。
- `service/group_routing_profile_test.go`：静态匹配器和稳定目标键行为测试。
- `controller/group_routing_profile.go`：管理端摘要和目标预览接口。
- `controller/group_routing_profile_test.go`：请求校验、分页和敏感字段隔离测试。
- `service/group_routing_profile_models_test.go`：动态分组模型目录测试。
- `web/src/features/system-settings/models/group-routing-profile-api.ts`：Zod 合同和预览请求。
- `web/src/features/system-settings/models/group-routing-profile-editor.tsx`：能力档案编辑控件和摘要。
- `web/src/features/system-settings/models/group-routing-targets-dialog.tsx`：宽版目标目录和人工排除交互。
- `web/src/features/system-settings/models/__tests__/group-routing-profile-editor.test.tsx`：编辑器行为和无障碍测试。
- `web/src/features/system-settings/models/__tests__/group-routing-targets-dialog.test.tsx`：筛选、分页、排除和空状态测试。
- `e2e/group_capability_routing_profiles_e2e_test.go`：账号分组到真实路由、日志和成本核算链路测试。

### 后端修改

- `setting/ratio_setting/group_routing_requirements.go`
- `setting/ratio_setting/group_routing_requirements_test.go`
- `pkg/modelrouting/types.go`
- `pkg/modelrouting/privacy_test.go`
- `model/routing_policy.go`
- `model/routing_policy_test.go`
- `model/routing_policy_cache.go`
- `service/model_routing.go`
- `service/model_routing_test.go`
- `service/group.go`
- `controller/model.go`
- `controller/user.go`
- `controller/model_list_test.go`
- `controller/public_models_test.go`
- `controller/option.go`
- `controller/option_test.go`
- `controller/routing_policy.go`
- `router/routing-policy-router.go`
- `router/routing_policy_router_test.go`
- `types/config_import.go`
- `service/config_import_schema.go`
- `service/config_import_schema_test.go`
- `service/config_import_stage.go`
- `service/config_import_stage_test.go`
- `service/config_import_activation.go`
- `service/config_import_activation_test.go`

### 前端修改

- `web/src/features/system-settings/models/group-routing-requirements.ts`
- `web/src/features/system-settings/models/__tests__/group-routing-requirements.test.ts`
- `web/src/features/system-settings/models/group-ratio-visual-editor.tsx`
- `web/src/features/system-settings/models/group-ratio-form.tsx`
- `web/src/features/system-settings/models/ratio-settings-card.tsx`
- `web/src/i18n/locales/{en,zh,zh-TW,fr,ja,ru,vi}.json`，仅通过 i18n 技能规定的脚本生成。

---

### Task 1: 扩展分组路由配置合同

**Files:**
- Modify: `setting/ratio_setting/group_routing_requirements.go`
- Modify: `setting/ratio_setting/group_routing_requirements_test.go`

- [ ] **Step 1: 编写新旧配置兼容的失败测试**

在 `setting/ratio_setting/group_routing_requirements_test.go` 增加表驱动测试，覆盖动态档案、旧字段兼容和非法配置：

```go
func TestParseGroupRoutingRequirementsNormalizesDynamicProfile(t *testing.T) {
	profiles, err := ParseGroupRoutingRequirementsJSONString(`{
		"客户A": {
			"status": "active",
			"routing_source": "default",
			"real_person_mode": "required",
			"allowed_cost_modes": ["per_duration", "per_request", "per_duration"],
			"excluded_target_keys": ["grt_a", "grt_a", "grt_b"]
		}
	}`)
	require.NoError(t, err)
	profile := profiles["客户A"]
	assert.Equal(t, GroupRoutingProfileActive, profile.Status)
	assert.Equal(t, GroupRealPersonRequired, profile.EffectiveRealPersonMode())
	assert.Equal(t, []types.CostMode{types.CostModePerDuration, types.CostModePerRequest}, profile.AllowedCostModes)
	assert.Equal(t, []string{"grt_a", "grt_b"}, profile.ExcludedTargetKeys)
}

func TestParseGroupRoutingRequirementsKeepsLegacyRealPersonSemantics(t *testing.T) {
	profiles, err := ParseGroupRoutingRequirementsJSONString(`{"真人分组":{"require_real_person":true}}`)
	require.NoError(t, err)
	profile := profiles["真人分组"]
	assert.False(t, profile.IsDynamic())
	assert.Equal(t, GroupRealPersonRequired, profile.EffectiveRealPersonMode())
}

func TestCheckGroupRoutingRequirementsRejectsUnsafeDynamicProfiles(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{name: "default self inheritance", raw: `{"default":{"status":"active","routing_source":"default"}}`},
		{name: "auto pseudo group", raw: `{"auto":{"status":"active","routing_source":"default"}}`},
		{name: "invalid real person mode", raw: `{"客户A":{"status":"draft","routing_source":"default","real_person_mode":"sometimes"}}`},
		{name: "invalid cost mode", raw: `{"客户A":{"status":"draft","routing_source":"default","allowed_cost_modes":["per_minute"]}}`},
		{name: "legacy conflict", raw: `{"客户A":{"require_real_person":true,"real_person_mode":"forbidden"}}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Error(t, CheckGroupRoutingRequirements(test.raw))
		})
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./setting/ratio_setting -run 'Test(Parse|Check)GroupRoutingRequirements' -count=1`

Expected: FAIL，提示新类型或 `ParseGroupRoutingRequirementsJSONString` 未定义。

- [ ] **Step 3: 实现配置类型、规范化和限制**

在 `group_routing_requirements.go` 增加以下公开合同，并让 `Check...` 与 `Update...` 共同调用解析器：

```go
type GroupRoutingProfileStatus string
type GroupRealPersonMode string

const (
	GroupRoutingProfileDraft  GroupRoutingProfileStatus = "draft"
	GroupRoutingProfileActive GroupRoutingProfileStatus = "active"

	GroupRealPersonAny       GroupRealPersonMode = "any"
	GroupRealPersonRequired  GroupRealPersonMode = "required"
	GroupRealPersonForbidden GroupRealPersonMode = "forbidden"

	GroupRoutingSourceDefault = "default"
	maxGroupRoutingProfiles   = 200
	maxExcludedTargetsPerGroup = 500
	maxExcludedTargetKeyLength = 128
)

type GroupRoutingRequirements struct {
	RequireRealPerson  *bool                     `json:"require_real_person,omitempty"`
	Status             GroupRoutingProfileStatus `json:"status,omitempty"`
	RoutingSource      string                    `json:"routing_source,omitempty"`
	RealPersonMode     GroupRealPersonMode       `json:"real_person_mode,omitempty"`
	AllowedCostModes   []types.CostMode          `json:"allowed_cost_modes,omitempty"`
	ExcludedTargetKeys []string                  `json:"excluded_target_keys,omitempty"`
}

func (r GroupRoutingRequirements) IsDynamic() bool {
	return r.RoutingSource != ""
}

func (r GroupRoutingRequirements) EffectiveRealPersonMode() GroupRealPersonMode {
	if r.RealPersonMode != "" {
		return r.RealPersonMode
	}
	if r.RequireRealPerson != nil && *r.RequireRealPerson {
		return GroupRealPersonRequired
	}
	return GroupRealPersonAny
}
```

`ParseGroupRoutingRequirementsJSONString` 必须使用 `common.DecodeJsonStrict` 解码每个分组对象，排序并去重数组，拒绝未知字段、空键、非法枚举、过量排除项、`default` 自继承和 `auto` 档案。动态档案必须显式提供 `draft` 或 `active`，首版 `routing_source` 只能为 `default`。

- [ ] **Step 4: 让运行时快照保存规范化结果**

修改 `UpdateGroupRoutingRequirementsByJSONString`：先解析规范化 map，再使用 `common.Marshal` 写回线程安全 map，避免仅校验原字符串后保留重复数组。

```go
func UpdateGroupRoutingRequirementsByJSONString(value string) error {
	profiles, err := ParseGroupRoutingRequirementsJSONString(value)
	if err != nil {
		return err
	}
	encoded, err := common.Marshal(profiles)
	if err != nil {
		return err
	}
	return groupRoutingRequirementsMap.UnmarshalJSON(encoded)
}
```

- [ ] **Step 5: 运行配置测试**

Run: `go test ./setting/ratio_setting -count=1`

Expected: PASS。

- [ ] **Step 6: 提交配置合同**

```bash
git add setting/ratio_setting/group_routing_requirements.go setting/ratio_setting/group_routing_requirements_test.go
git commit -m "feat(routing): extend group capability profiles"
```

---

### Task 2: 实现稳定目标键和共享静态匹配器

**Files:**
- Create: `service/group_routing_profile.go`
- Create: `service/group_routing_profile_test.go`
- Modify: `model/routing_policy.go`
- Modify: `model/routing_policy_test.go`
- Modify: `model/routing_policy_cache.go`

- [ ] **Step 1: 编写稳定键和多目标匹配失败测试**

测试必须证明数据库 ID 变化不影响稳定键，并保护“同渠道高优先级目标不匹配时，仍能选择低优先级兼容目标”的业务合同：

```go
func TestGroupRoutingTargetKeyIgnoresDatabaseIDs(t *testing.T) {
	left := modelrouting.Target{ID: 10, ChannelID: 23, Name: "stable", UpstreamModel: "vendor-model", CostVariantKey: "default"}
	right := left
	right.ID = 99
	assert.Equal(t,
		GroupRoutingTargetKey("default", modelrouting.Seedance20, left),
		GroupRoutingTargetKey("default", modelrouting.Seedance20, right),
	)
}

func TestEvaluateGroupRoutingProfileSelectsLowerPriorityCompatibleTarget(t *testing.T) {
	profile := ratio_setting.GroupRoutingRequirements{
		Status: ratio_setting.GroupRoutingProfileActive,
		RoutingSource: "default",
		AllowedCostModes: []types.CostMode{types.CostModePerDuration},
	}
	snapshot := profileTestSnapshot(
		profileTarget(11, 23, 100, "per-request-target", "request-model"),
		profileTarget(12, 23, 50, "per-duration-target", "duration-model"),
	)
	rules := profileTestRules(
		profileRule(23, "request-model", types.CostModePerRequest),
		profileRule(23, "duration-model", types.CostModePerDuration),
	)
	available := map[GroupRoutingAvailabilityKey]struct{}{
		{CanonicalModel: snapshot.CanonicalModel, ChannelID: 23}: {},
	}

	result := EvaluateGroupRoutingProfile(profile, snapshot, rules, available)
	require.Contains(t, result.Snapshot.TargetsByChannel, 23)
	require.Len(t, result.Snapshot.TargetsByChannel[23], 1)
	assert.Equal(t, "per-duration-target", result.Snapshot.TargetsByChannel[23][0].Name)
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./service -run 'Test(GroupRoutingTargetKey|EvaluateGroupRoutingProfile)' -count=1`

Expected: FAIL，新文件和函数不存在。

- [ ] **Step 3: 实现稳定键和状态合同**

在新文件定义明确状态，不修改 `pkg/modelrouting.Match` 的请求素材职责：

```go
type GroupRoutingTargetStatus string

const (
	GroupRoutingTargetMatched            GroupRoutingTargetStatus = "matched"
	GroupRoutingTargetRealPersonMismatch GroupRoutingTargetStatus = "real_person_mismatch"
	GroupRoutingTargetRealPersonUnknown  GroupRoutingTargetStatus = "real_person_unknown"
	GroupRoutingTargetCostModeMismatch   GroupRoutingTargetStatus = "cost_mode_mismatch"
	GroupRoutingTargetCostRuleMissing    GroupRoutingTargetStatus = "cost_rule_missing"
	GroupRoutingTargetExcluded           GroupRoutingTargetStatus = "excluded"
	GroupRoutingTargetDisabled           GroupRoutingTargetStatus = "target_disabled"
	GroupRoutingTargetChannelUnavailable GroupRoutingTargetStatus = "channel_unavailable"
)

type GroupRoutingTargetEvaluation struct {
	Target       modelrouting.Target
	TargetKey    string
	Eligible     bool
	Status       GroupRoutingTargetStatus
	Issues       []GroupRoutingTargetStatus
	CostMode     types.CostMode
	CostRuleID   int64
	CostVersion  int
}

type GroupRoutingProfileEvaluation struct {
	Snapshot       modelrouting.PolicySnapshot
	Targets        []GroupRoutingTargetEvaluation
	MismatchCounts map[GroupRoutingTargetStatus]int
	CostRules      map[CostRuleCandidate]*model.ChannelModelCostRule
}

type GroupRoutingAvailabilityKey struct {
	CanonicalModel string
	ChannelID      int
}
```

稳定键使用规范化字符串和 `common.Sha256Raw`：

```go
func GroupRoutingTargetKey(sourceGroup, canonicalModel string, target modelrouting.Target) string {
	identity := strings.Join([]string{
		strings.TrimSpace(sourceGroup),
		modelrouting.NormalizeCanonicalModel(canonicalModel),
		strconv.Itoa(target.ChannelID),
		strings.TrimSpace(target.UpstreamModel),
		strings.TrimSpace(target.CostVariantKey),
		strings.TrimSpace(target.Name),
	}, "\x1f")
	return "grt_" + fmt.Sprintf("%x", common.Sha256Raw([]byte(identity)))
}
```

- [ ] **Step 4: 实现逐目标静态过滤**

`EvaluateGroupRoutingProfile` 必须遍历所有目标，按目标而不是按渠道先过滤；只有 `Eligible=true` 的目标进入返回的过滤后快照。人工排除、真人三态和成本模式都在此处理。成本模式未限制时不因缺少成本规则排除目标：此时 `Eligible=true`、`Status=matched`，同时在 `Issues` 中加入 `cost_rule_missing` 供管理预览告警。选择了成本模式时，缺少规则才是硬不匹配。

```go
func EvaluateGroupRoutingProfile(
	profile ratio_setting.GroupRoutingRequirements,
	snapshot modelrouting.PolicySnapshot,
	rules map[CostRuleCandidate]*model.ChannelModelCostRule,
	available map[GroupRoutingAvailabilityKey]struct{},
) GroupRoutingProfileEvaluation {
	filtered := snapshot
	filtered.TargetsByChannel = make(map[int][]modelrouting.Target)
	result := GroupRoutingProfileEvaluation{
		Snapshot: filtered,
		Targets: make([]GroupRoutingTargetEvaluation, 0),
		MismatchCounts: make(map[GroupRoutingTargetStatus]int),
		CostRules: rules,
	}
	excluded := stringSet(profile.ExcludedTargetKeys)
	for channelID, targets := range snapshot.TargetsByChannel {
		for _, target := range targets {
			evaluation := evaluateGroupRoutingTarget(profile, snapshot, target, rules, available, excluded)
			result.Targets = append(result.Targets, evaluation)
			if !evaluation.Eligible {
				result.MismatchCounts[evaluation.Status]++
			}
			if evaluation.Eligible {
				result.Snapshot.TargetsByChannel[channelID] = append(result.Snapshot.TargetsByChannel[channelID], target)
			}
		}
	}
	return result
}
```

- [ ] **Step 5: 增加按来源分组批量加载策略**

在 `model/routing_policy.go` 增加 `ListEnabledRoutingPoliciesByGroup(groupName string)`，使用 GORM 两次查询加载启用策略和全部目标，不使用数据库方言 SQL。返回策略顺序固定为 `model ASC, id ASC`，目标顺序固定为 `policy_id ASC, channel_id ASC, target_priority DESC, id ASC`。

同时增加 `ListRoutingAvailability(groupName string, canonicalModels []string)`：返回 `(规范模型, 渠道 ID)` 可用集合。启用内存缓存时直接从现有渠道能力缓存构建；禁用缓存时使用 GORM 批量读取启用的 `Ability` 和对应 `Channel` 状态，禁止逐目标查询。动态档案只使用来源分组 `default` 的能力记录；禁用渠道、禁用 Ability 和缺失渠道都归类为 `channel_unavailable`，不能计入模型目录或激活匹配数。

- [ ] **Step 6: 增加批量成本规则解析入口**

在 `service/group_routing_profile.go` 增加：

```go
func ResolveGroupRoutingProfilePolicies(
	profile ratio_setting.GroupRoutingRequirements,
	policies []model.RoutingPolicy,
) ([]GroupRoutingProfileEvaluation, error) {
	keys := collectProfileCostRuleCandidates(policies)
	rules, err := ActiveCostRules(keys, false)
	if err != nil {
		return nil, err
	}
	models := routingPolicyModels(policies)
	available, err := model.ListRoutingAvailability(profile.RoutingSource, models)
	if err != nil {
		return nil, err
	}
	results := make([]GroupRoutingProfileEvaluation, 0, len(policies))
	for index := range policies {
		snapshot, err := model.RoutingPolicySnapshotFromRows(policies[index], policies[index].Targets)
		if err != nil {
			return nil, err
		}
		results = append(results, EvaluateGroupRoutingProfile(profile, snapshot, rules, available))
	}
	return results, nil
}
```

将 `model/routing_policy_cache.go` 现有 `routingPolicySnapshotFromRows` 重命名并导出为 `model.RoutingPolicySnapshotFromRows`，缓存加载和共享匹配器共同调用它，不复制 JSON 约束解析。

- [ ] **Step 7: 运行静态匹配和模型查询测试**

Run: `go test ./model ./service -run 'Test(GroupRouting|ListEnabledRoutingPolicies)' -count=1`

Expected: PASS。

- [ ] **Step 8: 提交共享匹配器**

```bash
git add service/group_routing_profile.go service/group_routing_profile_test.go model/routing_policy.go model/routing_policy_test.go model/routing_policy_cache.go
git commit -m "feat(routing): evaluate dynamic group targets"
```

---

### Task 3: 接入真实请求路由、指定渠道和重试

**Files:**
- Modify: `service/model_routing.go`
- Modify: `service/model_routing_test.go`
- Modify: `pkg/modelrouting/types.go`
- Modify: `pkg/modelrouting/privacy_test.go`

- [ ] **Step 1: 编写动态来源组和严格拒绝失败测试**

新增测试至少覆盖：

```go
func TestSelectCapabilityChannelUsesDefaultPoolForDynamicGroup(t *testing.T) {
	setGroupRoutingRequirementsForTest(t, `{
		"客户A":{"status":"active","routing_source":"default","real_person_mode":"required","allowed_cost_modes":["per_duration"]}
	}`)
	fixture := seedDynamicGroupRoutingFixture(t, "客户A", "default")
	channel, result, err := selectChannelForGroup(fixture.Param, "客户A", 0)
	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, fixture.DurationChannelID, channel.Id)
	assert.Equal(t, "default", result.SourceGroup)
}

func TestDynamicGroupDraftFailsClosed(t *testing.T) {
	setGroupRoutingRequirementsForTest(t, `{"客户A":{"status":"draft","routing_source":"default"}}`)
	_, _, err := selectChannelForGroup(profileRetryParam(t, "客户A"), "客户A", 0)
	var selectionErr *ChannelSelectionError
	require.ErrorAs(t, err, &selectionErr)
	assert.Equal(t, relaytypes.ErrorCodeNoCompatibleRoute, selectionErr.Code)
}

func TestForbiddenRealPersonGroupRejectsRequiredRequest(t *testing.T) {
	setGroupRoutingRequirementsForTest(t, `{
		"卡真人":{"status":"active","routing_source":"default","real_person_mode":"forbidden"}
	}`)
	param := profileRetryParam(t, "卡真人")
	param.RoutingInput.RequireRealPerson = true
	_, _, err := selectChannelForGroup(param, "卡真人", 0)
	require.Error(t, err)
}
```

同时扩展现有指定渠道、自动分组和重试测试，证明它们使用来源组能力表且不能绕过人工排除和成本模式。

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./service -run 'Test(SelectCapabilityChannelUsesDefaultPool|DynamicGroupDraft|ForbiddenRealPersonGroup)' -count=1`

Expected: FAIL，当前实现仍查找实际分组策略和能力表。

- [ ] **Step 3: 扩展路由结果并解析来源组**

修改 `groupRoutingResult`：

```go
type groupRoutingResult struct {
	Capability             bool
	SourceGroup            string
	Profile                ratio_setting.GroupRoutingRequirements
	Snapshot               modelrouting.PolicySnapshot
	Facts                  modelrouting.Facts
	Evaluation             modelrouting.Evaluation
	ProfileMismatchCounts  map[GroupRoutingTargetStatus]int
	CostRules              map[CostRuleCandidate]*model.ChannelModelCostRule
}
```

`evaluateGroupRouting` 的来源规则必须明确：

```go
profile := ratio_setting.GetGroupRoutingRequirements(group)
sourceGroup := group
if profile.IsDynamic() {
	if profile.Status != ratio_setting.GroupRoutingProfileActive {
		return groupRoutingResult{SourceGroup: profile.RoutingSource, Profile: profile}, noCompatibleRouteError()
	}
	sourceGroup = profile.RoutingSource
}
snapshot, ok := routingPolicySnapshotLookup(sourceGroup, canonicalModel)
```

动态档案先调用共享静态匹配器生成过滤后快照，再调用 `modelrouting.Evaluate` 处理请求级素材事实。`required` 将请求事实合并为 `true`；`forbidden + 请求要求真人` 在查询上游前失败。

运行时为当前规范模型调用一次 `model.ListRoutingAvailability(sourceGroup, []string{canonicalModel})`。生产缓存开启时该调用不访问数据库；测试或缓存关闭时走一次批量 GORM 查询。

- [ ] **Step 4: 所有渠道能力检查改用来源组**

修改以下调用：

```go
channel, err := model.GetRandomSatisfiedChannel(
	result.SourceGroup,
	routingModelName,
	priorityRetry,
	param.RequestPath,
	filter,
)
```

指定渠道检查使用：

```go
if result.Capability && !model.IsChannelEnabledForGroupModel(result.SourceGroup, routingModelName, channelID) {
	return false, compatibleChannelUnavailable(result)
}
```

利润收入计算仍传实际计费分组 `group`，不能改成 `result.SourceGroup`。

- [ ] **Step 5: 复用静态匹配阶段读取的成本规则**

在 `applyProfitFilter` 和 `knownChannelPassesProfitFilter` 增加小型合并函数：若 `result.CostRules` 已覆盖候选就直接使用；缺少的候选再调用 `ActiveCostRules`，并合并到 map。禁止再次逐目标调用 `ActiveCostRule`。

- [ ] **Step 6: 扩展管理员诊断但保持普通响应脱敏**

在 `pkg/modelrouting/types.go` 的 `Audit` 增加可选字段：

```go
SourceGroup           string         `json:"source_group,omitempty"`
ProfileMismatchCounts map[string]int `json:"profile_mismatch_counts,omitempty"`
```

更新 `privacy_test.go`，确认这些字段只存在管理员诊断结构，普通公开日志序列化仍不包含目标和来源组内部信息。

- [ ] **Step 7: 运行路由回归测试**

Run: `go test ./service ./pkg/modelrouting -run 'Test.*(Routing|Profile|Privacy|AutoGroup|KnownChannel|Retry)' -count=1`

Expected: PASS。

- [ ] **Step 8: 提交运行时路由**

```bash
git add service/model_routing.go service/model_routing_test.go pkg/modelrouting/types.go pkg/modelrouting/privacy_test.go
git commit -m "feat(routing): apply group capability profiles at runtime"
```

---

### Task 4: 让模型目录遵守动态分组能力

**Files:**
- Modify: `service/group.go`
- Create: `service/group_routing_profile_models_test.go`
- Modify: `controller/model.go`
- Modify: `controller/user.go`
- Modify: `controller/model_list_test.go`
- Modify: `controller/public_models_test.go`

- [ ] **Step 1: 编写模型可见性失败测试**

```go
func TestGetGroupsEnabledModelsUsesDynamicProfileMatches(t *testing.T) {
	seedProfileModelCatalog(t, []profileModelFixture{
		{Model: "duration-model", CostMode: types.CostModePerDuration},
		{Model: "request-model", CostMode: types.CostModePerRequest},
	})
	setGroupRoutingRequirementsForTest(t, `{
		"按秒客户":{"status":"active","routing_source":"default","allowed_cost_modes":["per_duration"]}
	}`)
	assert.Equal(t, []string{"duration-model"}, GetGroupsEnabledModels([]string{"按秒客户"}))
}

func TestGetGroupsEnabledModelsHidesDraftProfile(t *testing.T) {
	setGroupRoutingRequirementsForTest(t, `{"草稿客户":{"status":"draft","routing_source":"default"}}`)
	assert.Empty(t, GetGroupsEnabledModels([]string{"草稿客户"}))
}
```

在 controller 测试中增加 Token 模型白名单包含不兼容模型的场景，响应仍必须隐藏该模型。

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./service ./controller -run 'Test(GetGroupsEnabledModels|GetUserModels|ListModels).*Profile' -count=1`

Expected: FAIL，当前目录只读取 `Ability` 分组。

- [ ] **Step 3: 实现动态分组模型枚举**

在 `service/group.go` 增加：

```go
func GetGroupEnabledModelsForRouting(group string) []string {
	profile := ratio_setting.GetGroupRoutingRequirements(group)
	if !profile.IsDynamic() {
		return model.GetGroupEnabledModels(group)
	}
	if profile.Status != ratio_setting.GroupRoutingProfileActive {
		return []string{}
	}
	policies, err := model.ListEnabledRoutingPoliciesByGroup(profile.RoutingSource)
	if err != nil {
		common.SysError("list dynamic profile models: " + err.Error())
		return []string{}
	}
	evaluations, err := ResolveGroupRoutingProfilePolicies(profile, policies)
	if err != nil {
		common.SysError("evaluate dynamic profile models: " + err.Error())
		return []string{}
	}
	return matchedProfileModels(evaluations)
}
```

修改 `GetGroupsEnabledModels` 调用该函数，并继续保持输入分组顺序和模型去重顺序。

- [ ] **Step 4: 统一过滤 Token 模型白名单路径**

在 `controller/model.go` 中先构造 `groupModels := stringSet(service.GetGroupsEnabledModels(ownerGroups))`。无论 Token 是否启用模型白名单，最终追加模型前都要求存在于 `groupModels`，从而避免白名单绕过分组能力。

`controller/user.go:GetUserModels` 继续调用同一服务，不新增第二套过滤逻辑。

- [ ] **Step 5: 运行模型目录测试**

Run: `go test ./service ./controller -run 'Test(GetGroupsEnabledModels|GetUserModels|ListModels)' -count=1`

Expected: PASS。

- [ ] **Step 6: 提交模型目录适配**

```bash
git add service/group.go service/group_routing_profile_models_test.go controller/model.go controller/user.go controller/model_list_test.go controller/public_models_test.go
git commit -m "feat(models): filter catalog by group capability profile"
```

---

### Task 5: 增加管理端预览、分页目录和激活校验接口

**Files:**
- Modify: `service/group_routing_profile.go`
- Modify: `service/group_routing_profile_test.go`
- Create: `controller/group_routing_profile.go`
- Create: `controller/group_routing_profile_test.go`
- Modify: `controller/option.go`
- Modify: `controller/option_test.go`
- Modify: `router/routing-policy-router.go`
- Modify: `router/routing_policy_router_test.go`

- [ ] **Step 1: 编写服务分页和激活失败测试**

```go
func TestPreviewGroupRoutingProfileReturnsMatchedAndExcludedTargets(t *testing.T) {
	seedGroupRoutingProfileCatalog(t)
	page, err := PreviewGroupRoutingProfile(GroupRoutingProfilePreviewInput{
		GroupName: "客户A",
		Profile: ratio_setting.GroupRoutingRequirements{
			Status: ratio_setting.GroupRoutingProfileDraft,
			RoutingSource: "default",
			AllowedCostModes: []types.CostMode{types.CostModePerDuration},
		},
		Page: 1,
		PageSize: 25,
	})
	require.NoError(t, err)
	assert.Greater(t, page.Summary.MatchedTargets, 0)
	assert.NotEmpty(t, page.Items)
	assert.NotEmpty(t, page.Facets.CostModes)
}

func TestValidateActiveGroupRoutingProfilesRejectsZeroMatches(t *testing.T) {
	err := ValidateActiveGroupRoutingProfiles(`{
		"客户A":{"status":"active","routing_source":"default","allowed_cost_modes":["per_token"]}
	}`)
	require.ErrorContains(t, err, "客户A")
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./service -run 'Test(PreviewGroupRoutingProfile|ValidateActiveGroupRoutingProfiles)' -count=1`

Expected: FAIL，新服务合同不存在。

- [ ] **Step 3: 实现预览输入、摘要、分页和筛选**

在共享服务文件增加：

```go
type GroupRoutingProfilePreviewInput struct {
	GroupName  string
	Profile    ratio_setting.GroupRoutingRequirements
	Model      string
	ChannelID  int
	CostMode   types.CostMode
	Status     GroupRoutingTargetStatus
	Page       int
	PageSize   int
}

type GroupRoutingProfileSummary struct {
	Models             int `json:"models"`
	MatchedModels      int `json:"matched_models"`
	Targets            int `json:"targets"`
	MatchedTargets     int `json:"matched_targets"`
	StaleExclusions    int `json:"stale_exclusions"`
}

type GroupRoutingProfileTargetPage struct {
	Items    []GroupRoutingProfileTargetView `json:"items"`
	Summary  GroupRoutingProfileSummary      `json:"summary"`
	Facets   GroupRoutingProfileFacets       `json:"facets"`
	Page     int                             `json:"page"`
	PageSize int                             `json:"page_size"`
	Total    int                             `json:"total"`
}
```

页面大小只允许 25、50、100；排序固定为模型、渠道、优先级、目标名。目标视图只返回展示字段、成本模式、规则 ID/版本和不透明目标键，禁止返回渠道 Key、原始成本 JSON 或其他凭据。

- [ ] **Step 4: 实现批量摘要接口**

增加 `PreviewGroupRoutingProfileSummaries(map[string]GroupRoutingRequirements)`，一次加载 `default` 策略和成本规则，并为所有动态档案返回摘要 map，避免前端每行一次请求。

- [ ] **Step 5: 接入 option 激活校验**

在 `controller/option.go` 的 `GroupRoutingRequirements` 分支中：

```go
raw := option.Value.(string)
if err := ratio_setting.CheckGroupRoutingRequirements(raw); err != nil {
	writeOptionValidationError(c, err)
	return
}
if err := service.ValidateActiveGroupRoutingProfiles(raw); err != nil {
	writeOptionValidationError(c, err)
	return
}
```

`ValidateActiveGroupRoutingProfiles` 只校验动态 `active` 档案；旧静态配置和 `draft` 档案不要求匹配目标。它还必须确认分组存在于当前 GroupRatio 或 UserUsableGroups 中。新增 controller 测试确认校验失败时数据库 Option 和运行时快照均未更新。

- [ ] **Step 6: 增加控制器和路由**

新增两个只读计算接口：

```go
POST /api/routing-policies/group-profile/summaries
POST /api/routing-policies/group-profile/targets
```

请求使用 JSON，因为需要携带未保存档案；权限使用 `authz.ChannelRead`。在 `routingPolicyPermissionRoutes` 中将两个静态路径放在 `/:id` 之前。错误使用稳定代码：`invalid_group_profile`、`group_profile_unavailable`、`group_profile_preview_failed`。

- [ ] **Step 7: 记录非敏感审计摘要**

`GroupRoutingRequirements` 保存成功时，读取保存前后规范化配置，在现有 `option.update` 审计参数中增加 `changed_groups`、`activated_groups`、`draft_groups`、`exclusions_added` 和 `exclusions_removed`。不记录完整 JSON 或目标键。

- [ ] **Step 8: 运行服务、控制器和路由测试**

Run: `go test ./service ./controller ./router -run 'Test.*GroupRoutingProfile' -count=1`

Expected: PASS。

- [ ] **Step 9: 提交管理接口**

```bash
git add service/group_routing_profile.go service/group_routing_profile_test.go controller/group_routing_profile.go controller/group_routing_profile_test.go controller/option.go controller/option_test.go router/routing-policy-router.go router/routing_policy_router_test.go
git commit -m "feat(admin): preview group capability targets"
```

---

### Task 6: 扩展配置导入并保持人工排除

**Files:**
- Modify: `types/config_import.go`
- Modify: `service/config_import_schema.go`
- Modify: `service/config_import_schema_test.go`
- Modify: `service/config_import_stage.go`
- Modify: `service/config_import_stage_test.go`
- Modify: `service/config_import_activation.go`
- Modify: `service/config_import_activation_test.go`

- [ ] **Step 1: 编写导入兼容和排除保留失败测试**

增加测试：

```go
func TestConfigImportSchemaAcceptsDynamicGroupRoutingProfile(t *testing.T) {
	document := validConfigImportDocument(t)
	document.Entities.GroupRoutingRequirements = []types.ConfigImportGroupRoutingRequirement{{
		ConfigImportAuthoritativeEntity: validAuthoritativeEntity("group-profile-customer-a"),
		GroupName: "客户A",
		Requirements: types.ConfigImportGroupRoutingValues{
			Status: "active",
			RoutingSource: "default",
			RealPersonMode: "required",
			AllowedCostModes: []string{"per_duration"},
		},
	}}
	require.NoError(t, validateConfigImportDocument(&document))
}

func TestActivateConfigImportPreservesManualTargetExclusions(t *testing.T) {
	seedGroupRoutingOption(t, `{
		"客户A":{"status":"active","routing_source":"default","excluded_target_keys":["grt_keep"]}
	}`)
	batch := seedImportedGroupProfileBatch(t, "客户A", types.ConfigImportGroupRoutingValues{
		Status: "active", RoutingSource: "default", AllowedCostModes: []string{"per_duration"},
	})
	_, err := ActivateConfigImportBatch(context.Background(), batch.ID, 42)
	require.NoError(t, err)
	assertOptionJSONContains(t, "GroupRoutingRequirements", `"excluded_target_keys":["grt_keep"]`)
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./service -run 'Test(ConfigImportSchemaAcceptsDynamic|ActivateConfigImportPreservesManual)' -count=1`

Expected: FAIL，导入 DTO 仍只有 `require_real_person`。

- [ ] **Step 3: 扩展导入 DTO，不允许模板权威管理排除项**

```go
type ConfigImportGroupRoutingValues struct {
	RequireRealPerson *bool    `json:"require_real_person,omitempty"`
	Status            string   `json:"status,omitempty"`
	RoutingSource     string   `json:"routing_source,omitempty"`
	RealPersonMode    string   `json:"real_person_mode,omitempty"`
	AllowedCostModes  []string `json:"allowed_cost_modes,omitempty"`
}
```

不要在导入 DTO 增加 `excluded_target_keys`，从合同层阻止模板覆盖人工排除。

- [ ] **Step 4: 复用 ratio_setting 校验语义**

在 schema 校验中将导入值转换为 `ratio_setting.GroupRoutingRequirements`，调用公开规范化函数。`group_name=default` 或 `auto` 的动态档案必须拒绝，旧 `require_real_person` 节点继续通过。

- [ ] **Step 5: 暂存阶段合并当前人工排除**

在 `stageConfigImportGroupRoutingRequirements` 中构造导入值后：

```go
currentValue := current[imported.GroupName]
importedValue := groupRoutingRequirementsFromImport(imported.Requirements)
importedValue.ExcludedTargetKeys = append([]string(nil), currentValue.ExcludedTargetKeys...)
```

状态比较使用规范化后的 JSON，避免数组顺序和重复值制造伪变更。

- [ ] **Step 6: 调整激活事务顺序并校验新路由目标**

将 `publishConfigImportGroupRoutingRequirements` 移到路由策略和目标应用完成之后、事务提交之前。发布函数合并当前排除项，并调用接收 `*gorm.DB` 的 `ValidateActiveGroupRoutingProfilesWithDB(tx, encoded)`，确保本批次新成本规则和新 `default` 目标在同一事务中可见。

- [ ] **Step 7: 补充节点缺失和回滚测试**

保留现有“节点缺失不修改 Option”测试，并新增激活校验失败时以下对象全部回滚的断言：成本规则状态、路由目标启用状态、`GroupRoutingRequirements` Option、激活审计。

- [ ] **Step 8: 运行配置导入测试**

Run: `go test ./service -run 'Test.*ConfigImport.*GroupRouting' -count=1`

Expected: PASS。

- [ ] **Step 9: 提交配置导入**

```bash
git add types/config_import.go service/config_import_schema.go service/config_import_schema_test.go service/config_import_stage.go service/config_import_stage_test.go service/config_import_activation.go service/config_import_activation_test.go
git commit -m "feat(config-import): publish group capability profiles"
```

---

### Task 7: 实现前端档案序列化和 API 合同

**Files:**
- Modify: `web/src/features/system-settings/models/group-routing-requirements.ts`
- Modify: `web/src/features/system-settings/models/__tests__/group-routing-requirements.test.ts`
- Create: `web/src/features/system-settings/models/group-routing-profile-api.ts`

- [ ] **Step 1: 编写纯函数失败测试**

```ts
test('creates an active default-backed profile without changing other groups', () => {
  const next = updateGroupRoutingProfile('{}', '客户A', {
    status: 'active',
    routing_source: 'default',
    real_person_mode: 'required',
    allowed_cost_modes: ['per_duration', 'per_request'],
    excluded_target_keys: [],
  })
  assert.deepEqual(JSON.parse(next), {
    客户A: {
      status: 'active',
      routing_source: 'default',
      real_person_mode: 'required',
      allowed_cost_modes: ['per_duration', 'per_request'],
    },
  })
})

test('maps legacy require_real_person to the effective required mode', () => {
  const profiles = parseGroupRoutingProfiles(
    '{"真人分组":{"require_real_person":true}}'
  )
  assert.equal(effectiveRealPersonMode(profiles['真人分组']), 'required')
  assert.equal(isDynamicGroupRoutingProfile(profiles['真人分组']), false)
})

test('toggles a stable exclusion key without duplicates', () => {
  const next = toggleGroupRoutingTargetExclusion(
    '{"客户A":{"status":"draft","routing_source":"default"}}',
    '客户A',
    'grt_target',
    true
  )
  assert.deepEqual(JSON.parse(next).客户A.excluded_target_keys, ['grt_target'])
})
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd web && bun test src/features/system-settings/models/__tests__/group-routing-requirements.test.ts`

Expected: FAIL，新纯函数不存在。

- [ ] **Step 3: 实现前端类型和稳定序列化**

```ts
export type GroupRoutingProfileStatus = 'draft' | 'active'
export type GroupRealPersonMode = 'any' | 'required' | 'forbidden'
export type GroupCostMode = 'per_request' | 'per_duration' | 'per_token' | 'free'

export type GroupRoutingRequirements = {
  require_real_person?: boolean
  status?: GroupRoutingProfileStatus
  routing_source?: 'default'
  real_person_mode?: GroupRealPersonMode
  allowed_cost_modes?: GroupCostMode[]
  excluded_target_keys?: string[]
}
```

实现 `parseGroupRoutingProfiles`、`serializeGroupRoutingProfiles`、`updateGroupRoutingProfile`、`removeDynamicGroupRoutingProfile`、`effectiveRealPersonMode` 和 `toggleGroupRoutingTargetExclusion`。序列化时删除空数组和默认 `any`，按分组名排序，但保留未知旧分组对象字段以避免前端破坏后端兼容数据。

- [ ] **Step 4: 定义预览 Zod 合同**

在新 API 文件定义 `groupRoutingTargetStatusSchema`、`groupRoutingProfileSummarySchema`、`groupRoutingProfileTargetPageSchema` 和请求函数：

```ts
export async function previewGroupRoutingProfileTargets(
  input: GroupRoutingProfileTargetRequest
) {
  const response = await api.post(
    '/api/routing-policies/group-profile/targets',
    input
  )
  return groupRoutingProfileTargetPageResponseSchema.parse(response.data)
}

export async function previewGroupRoutingProfileSummaries(
  profiles: Record<string, GroupRoutingRequirements>
) {
  const response = await api.post(
    '/api/routing-policies/group-profile/summaries',
    { profiles }
  )
  return groupRoutingProfileSummariesResponseSchema.parse(response.data)
}
```

- [ ] **Step 5: 运行前端纯函数测试**

Run: `cd web && bun test src/features/system-settings/models/__tests__/group-routing-requirements.test.ts`

Expected: PASS。

- [ ] **Step 6: 提交前端合同**

```bash
git add web/src/features/system-settings/models/group-routing-requirements.ts web/src/features/system-settings/models/__tests__/group-routing-requirements.test.ts web/src/features/system-settings/models/group-routing-profile-api.ts
git commit -m "feat(ui): add group capability profile client contract"
```

---

### Task 8: 增加分组能力编辑器和批量摘要

**Files:**
- Create: `web/src/features/system-settings/models/group-routing-profile-editor.tsx`
- Create: `web/src/features/system-settings/models/__tests__/group-routing-profile-editor.test.tsx`
- Modify: `web/src/features/system-settings/models/group-ratio-visual-editor.tsx`
- Modify: `web/src/features/system-settings/models/group-ratio-form.tsx`

- [ ] **Step 1: 编写编辑器交互失败测试**

使用 React Testing Library 覆盖用户行为：

```tsx
test('enables default-backed routing and combines real-person with cost modes', async () => {
  const user = userEvent.setup()
  const onChange = vi.fn()
  render(
    <GroupRoutingProfileEditor
      groupName='客户A'
      source='{}'
      disabled={false}
      summary={EMPTY_SUMMARY}
      summaryLoading={false}
      onChange={onChange}
      onViewTargets={vi.fn()}
    />
  )

  await user.click(screen.getByRole('switch', { name: /adapt from default/i }))
  await user.click(screen.getByRole('button', { name: /must support real person/i }))
  await user.click(screen.getByRole('checkbox', { name: /per-duration/i }))

  expect(onChange).toHaveBeenCalled()
})

test('does not allow active status when preview has zero matched targets', async () => {
  renderProfileEditor({ summary: { ...EMPTY_SUMMARY, matched_targets: 0 } })
  expect(screen.getByRole('button', { name: /active/i })).toBeDisabled()
  expect(screen.getByText(/no compatible targets/i)).toBeVisible()
})
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd web && bun test src/features/system-settings/models/__tests__/group-routing-profile-editor.test.tsx`

Expected: FAIL，组件不存在。

- [ ] **Step 3: 实现能力编辑器**

组件必须使用项目现有控件：

- `Switch`：开启“从 default 自动适配”。
- `ToggleGroup type='single'`：`any`、`required`、`forbidden`。
- `Checkbox`：四种成本模式。
- `ToggleGroup type='single'`：`draft`、`active`。
- `Button` + `ListFilter` 图标：查看目标。

开启自动适配时创建默认草稿：

```ts
const nextProfile: GroupRoutingRequirements = {
  status: 'draft',
  routing_source: 'default',
  real_person_mode: 'any',
  allowed_cost_modes: [],
}
props.onChange(updateGroupRoutingProfile(props.source, props.groupName, nextProfile))
```

关闭时删除动态字段，但若原对象是旧 `require_real_person` 配置则保留旧字段，避免静默迁移。

- [ ] **Step 4: 在分组列表一次加载所有摘要**

`GroupRatioVisualEditor` 解析当前 `groupRoutingRequirements`，使用一个 TanStack Query 调用 `previewGroupRoutingProfileSummaries`。Query key 包含规范化后的配置字符串；请求失败时列表显示警告，不阻止其他分组倍率编辑。

每行显示档案状态、真人模式、成本模式、匹配目标数和失效排除数。旧配置显示“旧静态策略”。

- [ ] **Step 5: 将详情抽屉的旧真人开关替换为新组件**

删除 `GroupDetailSheet` 内单独的 `Require real person` Switch，传入 `groupName`、完整 JSON、摘要、保存禁用状态和查看目标回调。不要在 `group-ratio-visual-editor.tsx` 继续添加匹配业务逻辑。

- [ ] **Step 6: 运行编辑器和现有分组测试**

Run: `cd web && bun test src/features/system-settings/models/__tests__/group-routing-profile-editor.test.tsx src/features/system-settings/models/__tests__/group-routing-requirements.test.ts`

Expected: PASS。

- [ ] **Step 7: 提交分组能力编辑器**

```bash
git add web/src/features/system-settings/models/group-routing-profile-editor.tsx web/src/features/system-settings/models/__tests__/group-routing-profile-editor.test.tsx web/src/features/system-settings/models/group-ratio-visual-editor.tsx web/src/features/system-settings/models/group-ratio-form.tsx
git commit -m "feat(ui): edit group capability profiles"
```

---

### Task 9: 增加适配目标目录和人工排除

**Files:**
- Create: `web/src/features/system-settings/models/group-routing-targets-dialog.tsx`
- Create: `web/src/features/system-settings/models/__tests__/group-routing-targets-dialog.test.tsx`
- Modify: `web/src/features/system-settings/models/group-ratio-visual-editor.tsx`
- Modify: `web/src/features/system-settings/models/group-routing-requirements.ts`

- [ ] **Step 1: 编写目录交互失败测试**

```tsx
test('filters targets and excludes only matched rows', async () => {
  const user = userEvent.setup()
  const onSourceChange = vi.fn()
  renderTargetsDialog({
    page: targetPageFixture([
      matchedTarget({ target_key: 'grt_match' }),
      mismatchedTarget({ target_key: 'grt_mismatch' }),
    ]),
    onSourceChange,
  })

  await user.click(screen.getByRole('button', { name: /exclude target/i }))
  expect(onSourceChange).toHaveBeenCalledWith(
    expect.stringContaining('grt_match')
  )
  expect(
    screen.queryByRole('button', { name: /force include/i })
  ).not.toBeInTheDocument()
})

test('shows stale exclusions and provides a cleanup command', async () => {
  renderTargetsDialog({ summary: { ...EMPTY_SUMMARY, stale_exclusions: 2 } })
  expect(screen.getByText(/2 stale exclusions/i)).toBeVisible()
  expect(screen.getByRole('button', { name: /clean stale exclusions/i })).toBeEnabled()
})
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd web && bun test src/features/system-settings/models/__tests__/group-routing-targets-dialog.test.tsx`

Expected: FAIL，目录组件不存在。

- [ ] **Step 3: 实现宽版目标目录**

使用 `Dialog`，内容宽度使用稳定响应式约束，例如 `w-[min(96vw,1400px)] max-w-none`，高度使用 `max-h-[88vh]`，内部表格区域独立滚动。桌面使用 Table，移动端使用无嵌套卡片的紧凑列表。

筛选控件：模型输入、渠道选择、成本模式选择、状态选择；分页使用 25、50、100。每次筛选或档案条件变化调用服务端预览接口，并保留上一次数据直到新请求完成。

- [ ] **Step 4: 实现排除、恢复和失效清理**

匹配目标显示带 `Ban` 图标的排除命令；已排除目标显示恢复命令；不兼容目标只显示原因。所有命令只更新主表单中的 JSON，等待页面右上角“保存分组倍率”统一提交。

增加纯函数：

```ts
export function removeStaleGroupRoutingExclusions(
  source: string,
  groupName: string,
  liveTargetKeys: string[]
): string {
  const live = new Set(liveTargetKeys)
  const profiles = parseGroupRoutingProfiles(source)
  const profile = profiles[groupName]
  if (!profile) return source
  return updateGroupRoutingProfile(source, groupName, {
    ...profile,
    excluded_target_keys: (profile.excluded_target_keys ?? []).filter((key) =>
      live.has(key)
    ),
  })
}
```

- [ ] **Step 5: 接入分组详情查看按钮**

`GroupRatioVisualEditor` 维护当前查看的 `groupName`，将目标目录置于详情 Sheet 的同级，避免嵌套 Sheet/Dialog 焦点冲突。关闭目录后恢复焦点到“查看目标”按钮。

- [ ] **Step 6: 运行目录组件测试**

Run: `cd web && bun test src/features/system-settings/models/__tests__/group-routing-targets-dialog.test.tsx src/features/system-settings/models/__tests__/group-routing-profile-editor.test.tsx`

Expected: PASS。

- [ ] **Step 7: 提交目标目录**

```bash
git add web/src/features/system-settings/models/group-routing-targets-dialog.tsx web/src/features/system-settings/models/__tests__/group-routing-targets-dialog.test.tsx web/src/features/system-settings/models/group-ratio-visual-editor.tsx web/src/features/system-settings/models/group-routing-requirements.ts
git commit -m "feat(ui): inspect group-adapted routing targets"
```

---

### Task 10: 完成七语言国际化

**Files:**
- Modify by script: `web/src/i18n/locales/en.json`
- Modify by script: `web/src/i18n/locales/zh.json`
- Modify by script: `web/src/i18n/locales/zh-TW.json`
- Modify by script: `web/src/i18n/locales/fr.json`
- Modify by script: `web/src/i18n/locales/ja.json`
- Modify by script: `web/src/i18n/locales/ru.json`
- Modify by script: `web/src/i18n/locales/vi.json`

- [ ] **Step 1: 加载并遵循项目 `i18n-translate` 技能**

实现者必须读取 `C:\Users\880pro\Documents\new-api\.agents\skills\i18n-translate\SKILL.md`，不得手工直接编辑 locale JSON。

- [ ] **Step 2: 扫描新键**

Run: `cd web && node scripts/find-missing-keys.mjs`

Expected: 输出本功能新增的缺失键，包括：

```text
Adapt routing targets from default
Legacy static routing policy
Group capability profile
Draft profile
Active profile
Real-person capability
No restriction
Must support real person
Must not support real person
Allowed supplier cost modes
Per-request
Per-duration
Per-token
Free contract
Matched targets
View adapted targets
No compatible targets
Target is manually excluded
Real-person capability is unknown
Supplier cost rule is missing
Supplier cost mode does not match
Stale exclusions
Clean stale exclusions
Exclude target
Restore target
Request-time material and margin checks still apply.
```

- [ ] **Step 3: 使用临时脚本写入七语言翻译**

按技能创建 `web/scripts/add-missing-keys.mjs`，为上述每个键提供 `en`、`zh`、`zh-TW`、`fr`、`ja`、`ru`、`vi` 的自然翻译。脚本必须读取 JSON、按键排序、只补充缺失或本任务指定键，并使用稳定两空格格式写回。

- [ ] **Step 4: 校验并删除临时脚本**

Run:

```bash
cd web
node scripts/add-missing-keys.mjs
node scripts/find-missing-keys.mjs
bun run i18n:sync
```

Expected: `find-missing-keys` 报告全部 `t()` 键存在。随后删除 `web/scripts/add-missing-keys.mjs`。

- [ ] **Step 5: 运行 i18n 和组件测试**

Run: `cd web && bun test src/features/system-settings/models/__tests__/group-routing-profile-editor.test.tsx src/features/system-settings/models/__tests__/group-routing-targets-dialog.test.tsx`

Expected: PASS。

- [ ] **Step 6: 提交翻译**

```bash
git add web/src/i18n/locales/en.json web/src/i18n/locales/zh.json web/src/i18n/locales/zh-TW.json web/src/i18n/locales/fr.json web/src/i18n/locales/ja.json web/src/i18n/locales/ru.json web/src/i18n/locales/vi.json
git commit -m "feat(i18n): translate group capability profiles"
```

---

### Task 11: 增加端到端运营分组验收

**Files:**
- Create: `e2e/group_capability_routing_profiles_e2e_test.go`

- [ ] **Step 1: 编写 E2E 失败测试**

建立四个 mock 上游目标：支持真人按次、支持真人按时长、不支持真人按次、真人或成本未知。创建以下分组和账号：

```go
profiles := map[string]ratio_setting.GroupRoutingRequirements{
	"真人按秒": {
		Status: ratio_setting.GroupRoutingProfileActive,
		RoutingSource: "default",
		RealPersonMode: ratio_setting.GroupRealPersonRequired,
		AllowedCostModes: []types.CostMode{types.CostModePerDuration},
	},
	"卡真人按次": {
		Status: ratio_setting.GroupRoutingProfileActive,
		RoutingSource: "default",
		RealPersonMode: ratio_setting.GroupRealPersonForbidden,
		AllowedCostModes: []types.CostMode{types.CostModePerRequest},
	},
}
```

测试必须通过真实 HTTP 路由和 Ark SDK 兼容任务端点验证：

1. 真人按秒账号只命中支持真人按时长目标。
2. 卡真人按次账号只命中不支持真人按次目标。
3. 请求级真人要求与卡真人分组冲突时不上游。
4. 人工排除唯一匹配目标后严格拒绝。
5. `auto` 跳过无候选分组并命中后续分组。
6. 任务日志、使用日志和成本核算记录实际账号分组、来源策略目标和真实成本模式。

- [ ] **Step 2: 运行 E2E 确认失败**

Run: `go test ./e2e -run TestGroupCapabilityRoutingProfiles -count=1 -v`

Expected: FAIL，动态能力档案尚未完整接入或 fixture 尚未创建。

- [ ] **Step 3: 补齐确定性 fixture 和日志断言**

Fixture 使用显式 ID、模型名、成本规则和响应，不使用随机输入、固定 sleep 或真实外部网络。任务完成通过已有轮询辅助函数等待明确状态。

- [ ] **Step 4: 运行 E2E**

Run: `go test ./e2e -run TestGroupCapabilityRoutingProfiles -count=1 -v`

Expected: PASS，所有 mock 上游调用次数与预期一致。

- [ ] **Step 5: 提交 E2E**

```bash
git add e2e/group_capability_routing_profiles_e2e_test.go
git commit -m "test(e2e): cover group capability routing profiles"
```

---

### Task 12: 全量验证、浏览器验收和报告

**Files:**
- Create: `docs/superpowers/reports/2026-08-09-group-capability-routing-profiles-acceptance.md`

- [ ] **Step 1: 运行后端定向测试**

Run:

```bash
go test ./setting/ratio_setting ./pkg/modelrouting ./model ./service ./controller ./router -count=1
```

Expected: PASS，零失败。

- [ ] **Step 2: 运行全量后端测试**

Run: `go test ./... -count=1`

Expected: PASS，零失败。

- [ ] **Step 3: 运行前端定向测试**

Run:

```bash
cd web
bun test src/features/system-settings/models/__tests__/group-routing-requirements.test.ts src/features/system-settings/models/__tests__/group-routing-profile-editor.test.tsx src/features/system-settings/models/__tests__/group-routing-targets-dialog.test.tsx
```

Expected: PASS。

- [ ] **Step 4: 运行类型、Lint 和生产构建**

Run:

```bash
cd web
bun run typecheck
bunx oxlint src/features/system-settings/models/group-routing-requirements.ts src/features/system-settings/models/group-routing-profile-api.ts src/features/system-settings/models/group-routing-profile-editor.tsx src/features/system-settings/models/group-routing-targets-dialog.tsx src/features/system-settings/models/group-ratio-visual-editor.tsx
bun run build
```

Expected: 所有命令退出码为 0。

- [ ] **Step 5: 更新本地容器并执行浏览器验收**

Run:

```bash
docker compose up -d --build
docker compose ps
```

Expected: 应用和依赖服务均为运行或健康状态。随后使用浏览器控制工具打开 `http://localhost:3000/system-settings/billing/group-pricing`，在桌面和移动视口检查：

1. 分组列表显示旧静态策略和动态档案摘要。
2. 新建草稿、选择真人三态和多个成本模式。
3. 零匹配目标不能激活。
4. 宽版目标目录筛选、分页、排除和恢复正常。
5. 模板更新后的新增目标自动出现，旧排除仍生效。
6. 移动端文本不溢出、目录可滚动、Dialog 焦点可恢复。

- [ ] **Step 6: 执行 Canary 验收**

先使用 mock E2E，再选择一个内部测试账号执行真实上游低成本视频 Canary。核对任务日志、上游目标、供应商成本、严格毛利门禁和管理员诊断。不得使用正式客户账号作为首个 Canary。

- [ ] **Step 7: 编写验收报告**

报告使用简体中文，记录：提交范围、分组配置、适配目标数量、测试命令及结果、浏览器桌面/移动结果、Canary 任务 ID、实际渠道和成本模式、遗留风险。不得记录 API Key 或完整敏感请求。

- [ ] **Step 8: 提交验收报告**

```bash
git add docs/superpowers/reports/2026-08-09-group-capability-routing-profiles-acceptance.md
git commit -m "docs: report group capability routing acceptance"
```

- [ ] **Step 9: 核对最终工作区和提交序列**

Run:

```bash
git status --short --branch
git log --oneline --decorate -12
```

Expected: 工作区干净，提交按配置合同、共享匹配器、运行时、模型目录、管理接口、配置导入、前端、翻译、E2E、验收报告顺序排列。
