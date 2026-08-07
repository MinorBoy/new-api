# 分组可扩展路由要求实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为每个使用分组增加可扩展的路由要求配置，第一期实现真人脸要求，并让它与请求级要求、模型路由策略、自动分组、配置导入和管理员页面一致工作。

**Architecture:** 继续使用现有 `ratio_setting` 全局配置和系统选项持久化分组要求，运行时按实际使用分组读取 `GroupRoutingRequirements`。请求要求与分组要求采用 OR 合并，复用已有 `modelrouting.Match` 和 `supports_real_person` 目标能力过滤；要求真人脸但没有可验证能力路由时 fail-closed。配置导入扩展为带审计字段的分组要求实体，并让路由蓝图显式携带可选 `group_name`，缺省保持 `default` 兼容。

**Tech Stack:** Go 1.22、Gin、GORM、SQLite/MySQL/PostgreSQL 兼容配置选项、React 19、TypeScript、React Hook Form、Zod、Vitest/Bun Test、Go Test。

---

## 文件结构与职责

- `setting/ratio_setting/group_routing_requirements.go`：分组路由要求类型、运行时 map、JSON 解码/校验/序列化。
- `setting/ratio_setting/group_ratio.go`：把新 map 挂载到现有分组设置注册对象。
- `model/option.go`、`controller/option.go`：系统选项读写、校验和运行时刷新入口。
- `service/model_routing.go`：实际使用分组与请求事实合并、无策略 fail-closed。
- `service/model_routing_test.go`、`middleware/distributor_routing_test.go`：显式分组、自动分组、指定渠道和重试回归。
- `types/config_import.go`、`service/config_import_schema.go`、`service/config_import.go`：导入文档类型、实体规范化、计数、哈希和引用校验。
- `service/config_import_stage.go`、`service/config_import_publish.go`、`service/config_import_activation.go`：暂存差异、发布路由分组、激活分组要求和刷新运行时配置。
- `service/config_import_*_test.go`：导入 schema、暂存、发布、激活事务回归。
- `web/src/channel-config-converter/document.ts`、`scope.ts`、`hash.ts` 及对应测试：模板生成、范围裁剪和确定性哈希。
- `web/src/features/system-settings/types.ts`、`billing/index.tsx`、`billing/section-registry.tsx`、`models/ratio-settings-card.tsx`、`models/group-ratio-form.tsx`、`models/group-ratio-visual-editor.tsx`：管理员配置数据流和详情抽屉 UI。
- `web/src/features/system-settings/models/__tests__/`：分组配置 UI 行为测试。
- `web/src/i18n/locales/*.json`：只通过 i18n 脚本同步新增文案。
- `e2e/group_routing_requirements_e2e_test.go`：mock 渠道的完整真人脸路由验收。

## Task 1: 后端分组路由要求配置

**Files:**
- Create: `setting/ratio_setting/group_routing_requirements.go`
- Create: `setting/ratio_setting/group_routing_requirements_test.go`
- Modify: `setting/ratio_setting/group_ratio.go`
- Modify: `model/option.go`
- Modify: `controller/option.go`
- Test: `controller/option_test.go` 或现有系统选项控制器测试文件

- [ ] **Step 1: 先写配置校验失败测试**

在 `setting/ratio_setting/group_routing_requirements_test.go` 增加 `require`/`assert` 测试：

```go
func TestCheckGroupRoutingRequirementsRejectsInvalidShape(t *testing.T) {
    err := CheckGroupRoutingRequirements(`{"真人分组":{"require_real_person":"yes"}}`)
    require.Error(t, err)
}

func TestCheckGroupRoutingRequirementsRejectsUnknownField(t *testing.T) {
    err := CheckGroupRoutingRequirements(`{"真人分组":{"unknown":true}}`)
    require.Error(t, err)
}

func TestGroupRoutingRequirementsDefaultsMissingGroupToFalse(t *testing.T) {
    original := GroupRoutingRequirements2JSONString()
    t.Cleanup(func() { require.NoError(t, UpdateGroupRoutingRequirementsByJSONString(original)) })

    require.NoError(t, UpdateGroupRoutingRequirementsByJSONString(`{"真人分组":{"require_real_person":true}}`))
    require.NotNil(t, GetGroupRoutingRequirements("真人分组").RequireRealPerson)
    assert.True(t, *GetGroupRoutingRequirements("真人分组").RequireRealPerson)
    assert.Nil(t, GetGroupRoutingRequirements("未配置分组").RequireRealPerson)
}
```

- [ ] **Step 2: 运行失败测试确认缺少配置实现**

运行：`go test ./setting/ratio_setting -run 'Test(CheckGroupRoutingRequirements|GroupRoutingRequirements)' -count=1`

预期：测试因 `CheckGroupRoutingRequirements`、类型或读取函数不存在而失败，不能是编译器找不到测试文件之外的错误。

- [ ] **Step 3: 实现最小运行时配置**

在 `group_routing_requirements.go` 定义：

```go
type GroupRoutingRequirements struct {
    RequireRealPerson *bool `json:"require_real_person,omitempty"`
}

var groupRoutingRequirementsMap = types.NewRWMap[string, GroupRoutingRequirements]()

func GetGroupRoutingRequirements(group string) GroupRoutingRequirements
func GroupRoutingRequirements2JSONString() string
func UpdateGroupRoutingRequirementsByJSONString(value string) error
func CheckGroupRoutingRequirements(value string) error
```

用 `common.Unmarshal` 将 JSON 解码为 `map[string]GroupRoutingRequirements`，校验分组名非空、字段类型和未知字段；更新时整体替换 map，避免删除旧字段时留下半份状态。把 `GroupRoutingRequirements *types.RWMap[string, GroupRoutingRequirements]` 加入 `GroupRatioSetting` 并在初始化时建立空默认 map。

- [ ] **Step 4: 接入系统选项读写**

在 `model/option.go` 的默认 `OptionMap` 增加 `GroupRoutingRequirements`，在更新分支调用 `UpdateGroupRoutingRequirementsByJSONString`；在 `controller/option.go` 对该 key 调用 `CheckGroupRoutingRequirements`。保存失败时返回现有选项 API 的错误响应，不修改内存 map。

- [ ] **Step 5: 运行配置测试并提交**

运行：

```bash
go test ./setting/ratio_setting ./controller -run 'Test(CheckGroupRoutingRequirements|.*Option.*)' -count=1
git diff --check
```

预期：新增配置测试通过，非法配置不会改变旧运行时值。提交：

```bash
git add setting/ratio_setting/group_routing_requirements.go setting/ratio_setting/group_routing_requirements_test.go setting/ratio_setting/group_ratio.go model/option.go controller/option.go controller/option_test.go
git commit -m "feat: add group routing requirements setting"
```

## Task 2: 合并分组要求并接入能力路由

**Files:**
- Modify: `service/model_routing.go`
- Modify: `service/model_routing_test.go`
- Modify: `middleware/distributor_routing_test.go`
- Test: `pkg/modelrouting/match_test.go`（仅在现有真人脸分支缺少边界时补充）

- [ ] **Step 1: 先写可观察的失败测试**

在 `service/model_routing_test.go` 增加三个行为测试。第一个复用现有 `capabilityPolicyRequest`（其默认目标已经是 `SupportsRealPerson=true`），复制第二个目标并将其 `Constraints.SupportsRealPerson` 设为 `false`：

```go
func TestSelectCapabilityChannelAppliesGroupRealPersonRequirement(t *testing.T) {
    prepareCapabilitySelectionTest(t)
    seedRoutingCandidate(t, 11, "supports", "真人分组", modelrouting.Seedance20, true)
    seedRoutingCandidate(t, 12, "generic", "真人分组", modelrouting.Seedance20, true)
    policy := capabilityPolicyRequest("真人分组", modelrouting.Seedance20, 11, "provider-face", "720p")
    genericTarget := policy.Targets[0]
    genericTarget.ChannelID = 12
    genericTarget.Name = "provider-generic"
    genericTarget.UpstreamModel = "provider-generic"
    genericTarget.Constraints.SupportsRealPerson = common.GetPointer(false)
    policy.Targets = append(policy.Targets, genericTarget)
    require.NoError(t, service.SaveRoutingPolicy(0, policy))
    require.NoError(t, ratio_setting.UpdateGroupRoutingRequirementsByJSONString(`{"真人分组":{"require_real_person":true}}`))

    input := seedanceFactsInput(modelrouting.Seedance20, "720p", 8, "16:9")
    channel, _, err := service.CacheGetRandomSatisfiedChannel(&service.RetryParam{
        Ctx: capabilitySelectionContext(), TokenGroup: "真人分组", ModelName: modelrouting.Seedance20,
        RequestPath: "/v1/video/generations", Retry: common.GetPointer(0),
        RoutingInput: &input,
    })
    require.NoError(t, err)
    require.NotNil(t, channel)
    assert.Equal(t, 11, channel.Id)
}
```

第二个测试只创建普通 legacy channel，不创建能力路由策略，设置分组要求为 `true`，调用 `CacheGetRandomSatisfiedChannel` 后断言返回 `ChannelSelectionError`，错误码为 `NoCompatibleRoute`，且没有选中 legacy channel。第三个测试把第一自动分组设为真人脸要求但只配置不支持目标，把第二自动分组配置一个支持目标，断言最终选择第二分组和支持目标。

Use existing test fixtures and add only domain-specific helpers needed to create a target with `SupportsRealPerson=true/false`; do not duplicate production matching logic inside tests.

- [ ] **Step 2: 运行失败测试**

运行：`go test ./service -run 'Test(SelectCapabilityChannelAppliesGroupRealPersonRequirement|RequiredGroupRealPersonFailsClosed|AutoGroupRoutingAppliesRequirement)' -count=1 -p=1`

预期：真人分组测试仍可能选择不支持目标，或旧渠道被错误选中，证明测试覆盖了缺失行为。

- [ ] **Step 3: 在统一能力评估入口合并要求**

在 `evaluateGroupRouting` 确定实际 `group` 后读取 `ratio_setting.GetGroupRoutingRequirements(group)`，使用 `groupRequiresRealPerson := requirements.RequireRealPerson != nil && *requirements.RequireRealPerson`，再将它与 `param.RoutingInput.RequireRealPerson` 做 OR，最后调用现有 `modelrouting.ResolveFacts`。不要在 `Match` 或各个渠道选择分支重复合并。

当分组要求为 `true` 且 `param.RoutingInput == nil`，在进入 legacy fallback 前返回 `ChannelSelectionError`，错误码沿用 `NoCompatibleRoute`；`group=auto` 的上层循环继续尝试后续分组。

保留现有请求级校验、目标级 `supports_real_person` 和 `MismatchRealPerson` 统计；利润过滤必须接收已经完成能力过滤的候选集。

- [ ] **Step 4: 运行路由测试并修复具体回归**

运行：

```bash
go test ./pkg/modelrouting ./service ./middleware -run 'Test.*(RealPerson|CapabilityRouting|AutoGroup)' -count=1 -p=1
```

预期：新测试及既有 Seedance 能力路由测试全部通过，普通模型和未配置分组要求的旧路由不变。

- [ ] **Step 5: 提交路由行为**

```bash
git add service/model_routing.go service/model_routing_test.go middleware/distributor_routing_test.go pkg/modelrouting/match_test.go
git commit -m "feat: apply group requirements to model routing"
```

## Task 3: 扩展导入文档和路由蓝图分组

**Files:**
- Modify: `types/config_import.go`
- Modify: `service/config_import_schema.go`
- Modify: `service/config_import.go`
- Modify: `service/config_import_schema_test.go`
- Modify: `service/config_import_test.go`（若该文件承载文档解码测试）
- Modify: `web/src/channel-config-converter/document.ts`
- Modify: `web/src/channel-config-converter/scope.ts`
- Modify: `web/src/channel-config-converter/hash.ts`
- Modify: `web/src/channel-config-converter/__tests__/document.test.ts`
- Modify: `web/src/channel-config-converter/__tests__/scope.test.ts`
- Modify: `web/src/channel-config-converter/__tests__/hash.test.ts`
- Modify: `web/scripts/channel-model-template/generate.ts` 及对应测试（若模板生成器直接构造 v2 实体）

- [ ] **Step 1: 先写导入 schema 失败测试**

在 `service/config_import_schema_test.go` 增加一个带最小合法来源、分组要求和路由蓝图的文档 fixture，断言：

```go
require.NoError(t, validateConfigImportDocument(document))
assert.Equal(t, "真人分组", document.Entities.GroupRoutingRequirements[0].GroupName)
assert.Equal(t, "真人分组", document.Entities.RouteBlueprints[0].GroupName)
```

再增加失败用例：重复 `business_id`、空 `group_name`、非法 `require_real_person` 类型、重复同一 `group_name`、声明计数不一致。

- [ ] **Step 2: 运行导入 schema 测试确认失败**

运行：`go test ./service -run 'Test.*ConfigImport.*(GroupRouting|RouteGroup|Manifest)' -count=1`

预期：类型字段不存在或 schema 校验未识别新集合，测试失败。

- [ ] **Step 3: 增加类型和规范化逻辑**

在 `types/config_import.go` 增加：

```go
type ConfigImportGroupRoutingRequirement struct {
    ConfigImportAuthoritativeEntity
    GroupName    string                         `json:"group_name"`
    Requirements ConfigImportGroupRoutingValues `json:"requirements"`
}

type ConfigImportGroupRoutingValues struct {
    RequireRealPerson *bool `json:"require_real_person,omitempty"`
}
```

把集合加入 `ConfigImportEntities`、manifest/entity counts、`normalizedConfigImportItems`、`configImportBusinessIDs` 和 `canonicalizeConfigImportEntities`。给 `ConfigImportRouteBlueprint` 增加 `GroupName string \`json:"group_name,omitempty"\``，规范化空值为 `default`，保留旧文档兼容。

在 `service/config_import_schema.go` 校验分组名、要求对象和重复分组，未知要求字段必须报 schema 错误；分组要求实体仍要求来源、业务 ID 和 SHA-256 实体哈希。

- [ ] **Step 4: 同步前端 converter 的实体集合和哈希排序**

在 `document.ts` 的 `ImportEntities`、`emptyEntities`、manifest counts 和文档构造中增加 `group_routing_requirements: []`；在 `scope.ts` 的集合名单、裁剪和来源引用处理中保留它；在 `hash.ts` 的集合排序中按 `business_id` 排序。路由蓝图生成器默认写 `group_name: "default"`，支持模板输入覆盖为具体业务分组。

- [ ] **Step 5: 运行 Go 与 Bun converter 测试**

运行：

```bash
go test ./service -run 'Test.*ConfigImport' -count=1 -p=1
cd web
bun run converter:test
```

预期：旧 fixture 仍通过，新增 fixture 能稳定生成、裁剪和计算 payload hash。

- [ ] **Step 6: 提交导入合同**

```bash
git add types/config_import.go service/config_import_schema.go service/config_import.go service/config_import_schema_test.go web/src/channel-config-converter web/scripts/channel-model-template
git commit -m "feat: extend config import with group routing requirements"
```

## Task 4: 暂存、发布和激活分组路由要求

**Files:**
- Modify: `service/config_import_stage.go`
- Modify: `service/config_import_publish.go`
- Modify: `service/config_import_activation.go`
- Modify: `service/config_import_stage_test.go`
- Modify: `service/config_import_publish_test.go`
- Modify: `service/config_import_activation_test.go`

- [ ] **Step 1: 先写暂存和激活失败测试**

覆盖以下契约：

```go
func TestStageConfigImportPreservesGroupRequirementsWhenSectionMissing(t *testing.T)
func TestStageConfigImportRejectsUnknownRoutingRequirementGroup(t *testing.T)
func TestActivateConfigImportPublishesGroupRoutingRequirementsAtomically(t *testing.T)
func TestActivateConfigImportDoesNotPartiallyWriteGroupRequirements(t *testing.T)
func TestPublishRouteBlueprintUsesDeclaredGroupName(t *testing.T)
```

每个测试都在 fixture 内显式初始化 option、分组倍率、路由策略和批次状态；失败测试必须验证数据库中旧 option 和旧策略均保持不变。

- [ ] **Step 2: 运行测试确认缺失行为**

运行：`go test ./service -run 'Test(Stage|Publish|Activate).*ConfigImport.*(Group|Routing)' -count=1 -p=1`

预期：新实体未进入暂存/激活计划，或蓝图仍被写入 `default`，测试失败。

- [ ] **Step 3: 将分组要求纳入暂存计划**

在 `service/config_import_stage.go` 读取 `group_routing_requirements` item：

- 缺少集合时不生成任何 option patch；
- 同一分组重复或不存在于 `GroupRatio`/`UserUsableGroups` 注册表时生成 error blocker；
- 对现有值做规范化 JSON 比较，生成 `new`、`changed` 或 `unchanged` 状态；
- 不把凭据或数据库 ID 写入 canonical JSON。

- [ ] **Step 4: 将路由蓝图的 group_name 传入发布计划**

在 `buildConfigImportPublishedRoutePlans`、`configImportRoutePolicy` 和刷新 key 生成处统一使用 `blueprint.GroupName`；空值在 schema 层已归一为 `default`。同一 `group_name + canonical_model` 的蓝图保持既有 merge/replace/skip 冲突规则。

- [ ] **Step 5: 在激活事务内写入配置 option**

在 `ActivateConfigImportBatch` 生成并锁定 `GroupRoutingRequirements` option patch：

1. 读取当前 option 并按分组合并导入项；
2. 将 option 更新与路由策略、渠道成本激活放入同一个 GORM transaction；
3. 任一校验或写入失败全部回滚；
4. 把 `GroupRoutingRequirements` 加入 `ConfigImportRefreshKeys.OptionKeys`；
5. 事务提交后由 `RefreshPublishedConfig` 调用 `model.RefreshOptions`，使运行时 map 与数据库一致。

- [ ] **Step 6: 运行导入全套回归并提交**

运行：

```bash
go test ./service -run 'Test.*ConfigImport' -count=1 -p=1
git diff --check
```

预期：旧批次不受影响，新批次可以预览、发布、激活和回滚分组要求，发布失败没有部分配置。

```bash
git add service/config_import_stage.go service/config_import_publish.go service/config_import_activation.go service/config_import_stage_test.go service/config_import_publish_test.go service/config_import_activation_test.go
git commit -m "feat: activate group routing requirements with imports"
```

## Task 5: 管理员分组设置界面

**Files:**
- Modify: `web/src/features/system-settings/types.ts`
- Modify: `web/src/features/system-settings/billing/index.tsx`
- Modify: `web/src/features/system-settings/billing/section-registry.tsx`
- Modify: `web/src/features/system-settings/models/ratio-settings-card.tsx`
- Modify: `web/src/features/system-settings/models/group-ratio-form.tsx`
- Modify: `web/src/features/system-settings/models/group-ratio-visual-editor.tsx`
- Create: `web/src/features/system-settings/models/group-routing-requirements.ts`
- Create: `web/src/features/system-settings/models/__tests__/group-routing-requirements.test.ts`
- Modify: `web/src/i18n/static-keys.ts`（仅当项目现有提取规则需要登记）

- [ ] **Step 1: 先写前端失败测试**

在 `group-routing-requirements.test.ts` 测试真实序列化逻辑：

```ts
test('enables real-person routing for one group without changing other group settings', () => {
  const next = updateGroupRoutingRequirements('{}', '真人分组', true)
  expect(JSON.parse(next)).toEqual({ 真人分组: { require_real_person: true } })
})

test('removing the toggle writes false explicitly and preserves unknown groups', () => {
  const next = updateGroupRoutingRequirements(
    '{"真人分组":{"require_real_person":true},"default":{"require_real_person":false}}',
    '真人分组',
    false
  )
  expect(JSON.parse(next)).toEqual({
    真人分组: { require_real_person: false },
    default: { require_real_person: false },
  })
})
```

- [ ] **Step 2: 运行失败测试**

运行：`cd web; bun test src/features/system-settings/models/__tests__/group-routing-requirements.test.ts`

预期：序列化 helper 不存在，测试失败。

- [ ] **Step 3: 实现可扩展序列化 helper**

在 `group-routing-requirements.ts` 提供：

```ts
export type GroupRoutingRequirements = {
  require_real_person?: boolean
}

export function updateGroupRoutingRequirements(
  source: string,
  groupName: string,
  requireRealPerson: boolean
): string
```

使用现有 `safeJsonParse` 风格，按分组名排序输出，空分组名抛出可展示错误；不要在组件内复制 JSON 解析和排序逻辑。

- [ ] **Step 4: 接入 React Hook Form 和详情抽屉**

在分组表单值、默认值、保存 normalized values 和 `saveGroupRatios` 中加入 `GroupRoutingRequirements`，系统选项写入使用 `group_ratio_setting.group_routing_requirements`。把该字符串传给 `GroupRatioVisualEditor`，在 `GroupDetailSheet` 的路由要求区域增加 `Switch`，并在主表详情摘要显示已启用状态。复用现有 `Require real person` 翻译键，新增区域标题使用 `Group routing requirements` i18n key。

- [ ] **Step 5: 运行前端测试、类型检查和 lint**

运行：

```bash
cd web
bun test src/features/system-settings/models/__tests__/group-routing-requirements.test.ts
bun run typecheck
bun run lint
```

预期：测试、类型检查和 lint 均通过；开关支持键盘操作并在保存期间禁用。

- [ ] **Step 6: 通过 i18n 脚本补齐七种语言**

使用 `web/scripts/add-missing-keys.mjs` 写入 `Group routing requirements`，七种语言值固定为：

```js
{
  en: { 'Group routing requirements': 'Group routing requirements' },
  zh: { 'Group routing requirements': '分组路由要求' },
  'zh-TW': { 'Group routing requirements': '分組路由要求' },
  fr: { 'Group routing requirements': 'Exigences de routage du groupe' },
  ja: { 'Group routing requirements': 'グループのルーティング要件' },
  ru: { 'Group routing requirements': 'Требования маршрутизации группы' },
  vi: { 'Group routing requirements': 'Yêu cầu định tuyến của nhóm' },
}
```

完成后删除临时脚本并执行：

```bash
cd web
node scripts/add-missing-keys.mjs
bun run i18n:sync
```

确认新增 key 同时存在于 `en.json`、`zh.json`、`zh-TW.json`、`fr.json`、`ja.json`、`ru.json`、`vi.json`，禁止直接编辑 locale JSON。

- [ ] **Step 7: 提交前端配置界面**

```bash
git add web/src/features/system-settings web/src/i18n/static-keys.ts web/src/i18n/locales
git commit -m "feat: add group routing requirements editor"
```

## Task 6: 真实 mock E2E 验收

**Files:**
- Create: `e2e/group_routing_requirements_e2e_test.go`
- Modify: `e2e/seedance_capability_routing_e2e_test.go`（仅复用或补充现有 mock fixture，不修改既有断言语义）

- [ ] **Step 1: 先写失败的完整链路测试**

建立两个分组和至少两个 mock 渠道目标：

- `真人分组`：目标 1 `supports_real_person=true`，目标 2 `false`；
- `普通分组`：目标 3 `false`；
- 请求分别覆盖显式分组、`group=auto`、请求级 `routing.require_real_person=true`、指定渠道和重试。

每个用例断言选中的 channel、routing policy target、最终 `Facts.RequireRealPerson` 和 `MismatchRealPerson`，不向上游发真实网络请求。

- [ ] **Step 2: 运行 E2E 确认缺失行为**

运行：`go test ./e2e -run '^TestGroupRoutingRequirements' -count=1 -p=1`

预期：新增真人分组用例会选到不兼容目标或错误回退到 legacy channel，证明测试有效。

- [ ] **Step 3: 修复测试 fixture 与日志断言**

使用现有 Seedance mock 上游和任务日志 fixture，保证测试结束后恢复：分组路由要求 option、自动分组顺序、路由策略 cache、数据库和 mock server。管理员诊断可以断言真人脸 mismatch；普通响应只断言通用错误码和不包含供应商细节。

- [ ] **Step 4: 运行完整后端验证**

```bash
go test ./pkg/modelrouting ./setting/ratio_setting ./service ./middleware ./e2e -count=1 -p=1
git diff --check
```

预期：所有受影响后端测试通过，已有成本门禁和 Seedance 素材矩阵测试不回归。

- [ ] **Step 5: 提交 E2E 验收**

```bash
git add e2e/group_routing_requirements_e2e_test.go e2e/seedance_capability_routing_e2e_test.go
git commit -m "test: accept group routing requirements flow"
```

## Task 7: 最终验证与交付

**Files:**
- Verify: 所有本计划涉及的 Go、TypeScript、locale、converter 和 E2E 文件
- Update: `docs/superpowers/reports/` 新增简体中文验收报告

- [ ] **Step 1: 运行后端与前端验证**

```bash
go test ./... -count=1 -p=1
cd web
bun run converter:test
bun run typecheck
bun run lint
bun run build:check
```

- [ ] **Step 2: 运行容器级 smoke 检查**

确认 `GET http://localhost:3000/api/status` 返回 HTTP 200，相关容器为 `healthy`；使用已登录管理页面确认分组详情可以读取和保存 `require_real_person`。

- [ ] **Step 3: 编写验收报告**

在 `docs/superpowers/reports/2026-08-07-group-routing-requirements-acceptance.md` 记录：配置 schema、显式分组、自动分组、无策略 fail-closed、导入发布、UI 交互、日志审计和所有测试命令结果。报告使用简体中文，不写入任何 API Key、Token 或供应商凭据。

- [ ] **Step 4: 检查工作区并提交最终改动**

只暂存本功能相关文件，保留已有的 `cmd/ark-video-material-seed` 和其验收报告改动，不覆盖或回退用户已有修改：

执行 `git status --short`、`git diff --check` 和 `git diff --stat`，然后只暂存 Tasks 1-6 中列出的本功能路径；不要使用目录级通配符，不要暂存既有 `cmd/ark-video-material-seed` 改动。最终提交信息使用 `feat: support group-aware model routing requirements`。

预期：提交包含分组路由要求功能、导入合同、管理 UI、i18n、测试和验收报告；工作区中其他用户改动保持原样。
