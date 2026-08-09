# 分组列表生命周期管理实施计划

> **供执行代理使用：** 必须使用 `superpowers:subagent-driven-development`（推荐）或 `superpowers:executing-plans`，逐项执行本计划。所有步骤使用复选框跟踪。

**目标：** 为分组增加可恢复的启用/停用状态、受保护的删除流程，并在分组停用时从分配、展示及真实请求链路中统一阻断。

**架构：** 状态作为 `group_ratio_setting.group_status` JSON 映射接入现有分层配置，缺失键默认启用。新增分组设置批量保存接口，在一个数据库事务中校验和持久化相关设置；运行时通过 `ratio_setting.IsGroupEnabled` 形成唯一状态判定边界。前端继续使用现有分组表单，但改为一次批量保存，并在可视化编辑器中加入 `Switch` 和确认弹窗。

**技术栈：** Go 1.22、Gin、GORM、React 19、TypeScript、TanStack Query、React Hook Form、Base UI、Bun、Node Test Runner。

---

## 文件职责

- `setting/ratio_setting/group_status.go`：状态 JSON 的解析、校验、读取和兼容默认值。
- `setting/ratio_setting/group_ratio.go`：把状态映射注册到既有 `group_ratio_setting`。
- `controller/group_settings.go`：原子校验并保存整组分组设置。
- `controller/group.go`、`service/group.go`、`middleware/auth.go`：可用列表过滤和真实请求前置拦截。
- `controller/user.go`、`controller/token.go`：用户和令牌分组分配校验。
- `model/group_reference.go`：跨 SQLite、MySQL、PostgreSQL 查询分组引用。
- `web/src/features/system-settings/models/group-ratio-visual-editor.tsx`：状态列、默认启用、删除级联和确认交互。
- `web/src/features/system-settings/models/group-ratio-form.tsx`、`ratio-settings-card.tsx`：表单字段和批量保存接线。
- `web/src/features/system-settings/api.ts`、`types.ts`：批量保存 API 契约。
- `web/src/features/system-settings/models/__tests__/group-ratio-visual-editor.test.tsx`：交互回归测试。
- `web/src/i18n/locales/*.json`：七种语言新增文案，只允许通过临时脚本写入。

### 任务 1：建立后端分组状态配置

**文件：**
- 新建：`setting/ratio_setting/group_status.go`
- 新建：`setting/ratio_setting/group_status_test.go`
- 修改：`setting/ratio_setting/group_ratio.go`

- [ ] **步骤 1：先写兼容默认值和严格解析失败测试**

```go
func TestGroupStatusDefaultsMissingGroupsToEnabled(t *testing.T) {
    original := GroupStatus2JSONString()
    t.Cleanup(func() { require.NoError(t, UpdateGroupStatusByJSONString(original)) })
    require.NoError(t, UpdateGroupStatusByJSONString(`{"paused":false,"active":true}`))
    assert.False(t, IsGroupEnabled("paused"))
    assert.True(t, IsGroupEnabled("active"))
    assert.True(t, IsGroupEnabled("legacy"))
    assert.True(t, IsGroupEnabled("auto"))
}

func TestParseGroupStatusRejectsInvalidValues(t *testing.T) {
    _, err := ParseGroupStatusJSONString(`{"default":"yes"}`)
    require.Error(t, err)
}
```

- [ ] **步骤 2：运行测试，确认因状态 API 不存在而失败**

运行：`go test ./setting/ratio_setting -run GroupStatus -count=1`

预期：编译失败，提示 `GroupStatus2JSONString`、`IsGroupEnabled` 等符号未定义。

- [ ] **步骤 3：实现集中状态映射和解析**

```go
var groupStatusMap = types.NewRWMap[string, bool]()

func ParseGroupStatusJSONString(value string) (map[string]bool, error) {
    statuses := map[string]bool{}
    if err := common.UnmarshalJsonStr(value, &statuses); err != nil {
        return nil, err
    }
    for name := range statuses {
        if strings.TrimSpace(name) == "" {
            return nil, errors.New("group status name must not be empty")
        }
    }
    return statuses, nil
}

func UpdateGroupStatusByJSONString(value string) error {
    statuses, err := ParseGroupStatusJSONString(value)
    if err != nil {
        return err
    }
    groupStatusMap.Clear()
    groupStatusMap.AddAll(statuses)
    return nil
}

func GroupStatus2JSONString() string { return groupStatusMap.MarshalJSONString() }

func IsGroupEnabled(name string) bool {
    if name == "auto" {
        return true
    }
    enabled, exists := groupStatusMap.Get(name)
    return !exists || enabled
}
```

在 `GroupRatioSetting` 中增加：

```go
GroupStatus *types.RWMap[string, bool] `json:"group_status"`
```

并在 `init` 与 `GetGroupRatioSetting` 中绑定或恢复 `groupStatusMap`。

- [ ] **步骤 4：运行状态单测并提交**

运行：`go test ./setting/ratio_setting -run GroupStatus -count=1`

预期：PASS。

```powershell
git add setting/ratio_setting/group_status.go setting/ratio_setting/group_status_test.go setting/ratio_setting/group_ratio.go
git commit -m "feat: add group lifecycle status setting"
```

### 任务 2：增加原子分组设置保存接口

**文件：**
- 新建：`model/group_reference.go`
- 新建：`model/group_reference_test.go`
- 新建：`controller/group_settings.go`
- 新建：`controller/group_settings_test.go`
- 修改：`router/api-router.go`

- [ ] **步骤 1：写分组引用查询的数据库测试**

```go
func TestFindGroupReferencesCountsUsersAndTokens(t *testing.T) {
    fixture := modeltest.NewSQLiteFixture(t)
    require.NoError(t, fixture.DB.Create(&User{Username: "paused-user", Group: "paused"}).Error)
    require.NoError(t, fixture.DB.Create(&Token{UserId: 1, Name: "paused-token", Group: "paused"}).Error)
    refs, err := FindGroupReferences(fixture.DB, []string{"paused"})
    require.NoError(t, err)
    assert.Equal(t, int64(1), refs.Users)
    assert.Equal(t, int64(1), refs.Tokens)
}
```

- [ ] **步骤 2：运行测试并确认失败**

运行：`go test ./model -run FindGroupReferences -count=1`

预期：编译失败，`FindGroupReferences` 未定义。

- [ ] **步骤 3：用 GORM 实现跨数据库引用查询**

```go
type GroupReferences struct {
    Users  int64
    Tokens int64
}

func FindGroupReferences(db *gorm.DB, groups []string) (GroupReferences, error) {
    refs := GroupReferences{}
    if len(groups) == 0 {
        return refs, nil
    }
    if err := db.Model(&User{}).Where(commonGroupCol+" IN ?", groups).Count(&refs.Users).Error; err != nil {
        return refs, err
    }
    if err := db.Model(&Token{}).Where(commonGroupCol+" IN ?", groups).Count(&refs.Tokens).Error; err != nil {
        return refs, err
    }
    return refs, nil
}
```

- [ ] **步骤 4：写批量保存接口的原子性和删除保护测试**

请求 DTO 使用完整设置快照：

```go
type GroupSettingsUpdateRequest struct {
    GroupRatio               string `json:"group_ratio"`
    GroupStatus              string `json:"group_status"`
    TopupGroupRatio          string `json:"topup_group_ratio"`
    UserUsableGroups         string `json:"user_usable_groups"`
    GroupGroupRatio          string `json:"group_group_ratio"`
    AutoGroups               string `json:"auto_groups"`
    DefaultUseAutoGroup      bool   `json:"default_use_auto_group"`
    GroupSpecialUsableGroup  string `json:"group_special_usable_group"`
    GroupRoutingRequirements string `json:"group_routing_requirements"`
}
```

测试必须断言：引用中的分组被移除时响应失败且所有 Option 原值不变；合法请求一次更新全部 Option；状态包含孤立键时失败；动态路由画像校验仍被调用。

- [ ] **步骤 5：运行控制器测试，确认路由和处理器尚不存在**

运行：`go test ./controller -run UpdateGroupSettings -count=1`

预期：编译失败或 404，`UpdateGroupSettings` 未定义。

- [ ] **步骤 6：实现预校验、事务保存和内存刷新**

处理器按以下顺序执行：解析全部 JSON；调用 `CheckGroupRatio`、`ParseGroupStatusJSONString`、`CheckGroupRoutingRequirements` 和 `ValidateActiveGroupRoutingProfiles`；比较旧新 `GroupRatio` 键找出删除项；调用 `FindGroupReferences`；验证 `GroupStatus` 的键全部存在于新倍率表；最后调用 `model.UpdateOptionsBulk`。

```go
values := map[string]string{
    "GroupRatio": req.GroupRatio,
    "group_ratio_setting.group_status": req.GroupStatus,
    "TopupGroupRatio": req.TopupGroupRatio,
    "UserUsableGroups": req.UserUsableGroups,
    "GroupGroupRatio": req.GroupGroupRatio,
    "AutoGroups": req.AutoGroups,
    "DefaultUseAutoGroup": strconv.FormatBool(req.DefaultUseAutoGroup),
    "group_ratio_setting.group_special_usable_group": req.GroupSpecialUsableGroup,
    "GroupRoutingRequirements": req.GroupRoutingRequirements,
}
if err := model.UpdateOptionsBulk(values); err != nil {
    common.ApiError(c, err)
    return
}
recordManageAudit(c, "group_settings.update", map[string]interface{}{"removed_groups": removedGroups})
common.ApiSuccess(c, nil)
```

路由增加：

```go
optionRoute.PUT("/group-settings", controller.UpdateGroupSettings)
```

- [ ] **步骤 7：运行接口与模型测试并提交**

运行：`go test ./model ./controller -run 'FindGroupReferences|UpdateGroupSettings' -count=1`

预期：PASS。

```powershell
git add model/group_reference.go model/group_reference_test.go controller/group_settings.go controller/group_settings_test.go router/api-router.go
git commit -m "feat: save group lifecycle settings atomically"
```

### 任务 3：在分组列表、分配入口和真实请求链路执行状态

**文件：**
- 修改：`controller/group.go`
- 修改：`service/group.go`
- 修改：`middleware/auth.go`
- 修改：`controller/user.go`
- 修改：`controller/token.go`
- 修改：`controller/group_routing_profile_test.go`
- 修改：`middleware/auth_test.go`
- 新建：`controller/group_status_test.go`

- [ ] **步骤 1：写可用列表和路由模型过滤测试**

```go
func TestDisabledGroupIsExcludedFromUsableGroupsAndModels(t *testing.T) {
    restoreGroupStatus(t, `{"paused":false}`)
    groups := service.GetUserUsableGroups("default")
    assert.NotContains(t, groups, "paused")
    assert.Empty(t, service.GetGroupEnabledModelsForRouting("paused"))
}
```

- [ ] **步骤 2：写令牌鉴权在渠道选择前拒绝停用分组的测试**

测试创建启用用户、有效令牌和 `paused=false` 状态，执行 `TokenAuth`，断言 HTTP 403、错误码为 `access_denied`、下游 handler 未执行。

- [ ] **步骤 3：运行定向测试，确认停用状态尚未被执行**

运行：`go test ./service ./controller ./middleware -run DisabledGroup -count=1`

预期：FAIL，停用分组仍出现在列表或请求仍进入下游。

- [ ] **步骤 4：在集中服务边界过滤停用分组**

```go
func GetUserUsableGroups(userGroup string) map[string]string {
    groupsCopy := setting.GetUserUsableGroupsCopy()
    // 保留现有特殊分组合并逻辑。
    for group := range groupsCopy {
        if !ratio_setting.IsGroupEnabled(group) {
            delete(groupsCopy, group)
        }
    }
    if userGroup != "" && ratio_setting.IsGroupEnabled(userGroup) {
        if _, ok := groupsCopy[userGroup]; !ok {
            groupsCopy[userGroup] = "用户分组"
        }
    }
    return groupsCopy
}

func GetGroupEnabledModelsForRouting(group string) []string {
    if !ratio_setting.IsGroupEnabled(group) {
        return []string{}
    }
    // 继续现有能力画像分支。
}
```

`controller.GetGroups` 和 `controller.GetUserGroups` 遍历倍率时跳过 `!IsGroupEnabled(groupName)`。

- [ ] **步骤 5：在 TokenAuth 中加入计费和分发前置拦截**

```go
if !ratio_setting.IsGroupEnabled(userGroup) {
    abortWithOpenAiMessage(c, http.StatusForbidden, fmt.Sprintf("分组 %s 已停用", userGroup), types.ErrorCodeAccessDenied)
    return
}
if tokenGroup != "" && tokenGroup != "auto" && !ratio_setting.IsGroupEnabled(tokenGroup) {
    abortWithOpenAiMessage(c, http.StatusForbidden, fmt.Sprintf("分组 %s 已停用", tokenGroup), types.ErrorCodeAccessDenied)
    return
}
```

- [ ] **步骤 6：阻止用户和令牌新分配到停用分组**

在 `UpdateUser` 修改数据库前校验非空 `updatedUser.Group`；在 `AddToken` 和 `UpdateToken` 写入前校验非空且非 `auto` 的 `token.Group`。统一条件：

```go
if group != "" && group != "auto" && (!ratio_setting.ContainsGroupRatio(group) || !ratio_setting.IsGroupEnabled(group)) {
    common.ApiErrorMsg(c, fmt.Sprintf("分组 %s 不可用", group))
    return
}
```

- [ ] **步骤 7：运行定向测试并提交**

运行：`go test ./service ./controller ./middleware -run 'DisabledGroup|GroupStatus' -count=1`

预期：PASS，且鉴权测试证明下游未执行。

```powershell
git add controller/group.go controller/user.go controller/token.go controller/group_status_test.go controller/group_routing_profile_test.go service/group.go middleware/auth.go middleware/auth_test.go
git commit -m "feat: enforce disabled groups before routing"
```

### 任务 4：接入前端状态字段和原子保存 API

**文件：**
- 修改：`web/src/features/system-settings/types.ts`
- 修改：`web/src/features/system-settings/api.ts`
- 修改：`web/src/features/system-settings/billing/index.tsx`
- 修改：`web/src/features/system-settings/billing/section-registry.tsx`
- 修改：`web/src/features/system-settings/models/group-ratio-form.tsx`
- 修改：`web/src/features/system-settings/models/ratio-settings-card.tsx`
- 新建：`web/src/features/system-settings/models/__tests__/group-settings-api.test.ts`

- [ ] **步骤 1：写 API 契约测试**

断言 `updateGroupSettings` 使用 `PUT /api/option/group-settings`，并发送 snake_case 的九个字段，其中 `default_use_auto_group` 保持布尔类型。

- [ ] **步骤 2：运行测试并确认 API 函数未定义**

运行：`cd web; bun test src/features/system-settings/models/__tests__/group-settings-api.test.ts`

预期：FAIL，`updateGroupSettings` 未导出。

- [ ] **步骤 3：定义前端请求类型和 API**

```ts
export type GroupSettingsUpdateRequest = {
  group_ratio: string
  group_status: string
  topup_group_ratio: string
  user_usable_groups: string
  group_group_ratio: string
  auto_groups: string
  default_use_auto_group: boolean
  group_special_usable_group: string
  group_routing_requirements: string
}

export async function updateGroupSettings(request: GroupSettingsUpdateRequest) {
  const res = await api.put<UpdateOptionResponse>('/api/option/group-settings', request)
  return res.data
}
```

- [ ] **步骤 4：把 `GroupStatus` 接入默认值、Schema、表单和保存**

在设置类型中加入：

```ts
'group_ratio_setting.group_status': string
```

表单字段使用 `GroupStatus`，其 API 来源和保存目标均映射到 `group_ratio_setting.group_status`。`saveGroupRatios` 不再循环调用 `updateOption`，改为一次调用 `updateGroupSettings`；成功后更新 `groupNormalizedDefaults.current`、重置 dirty 状态并刷新 `system-options`。

- [ ] **步骤 5：运行 API 测试、类型检查并提交**

运行：`cd web; bun test src/features/system-settings/models/__tests__/group-settings-api.test.ts; bun run typecheck`

预期：全部 PASS。

```powershell
git add web/src/features/system-settings/types.ts web/src/features/system-settings/api.ts web/src/features/system-settings/billing/index.tsx web/src/features/system-settings/billing/section-registry.tsx web/src/features/system-settings/models/group-ratio-form.tsx web/src/features/system-settings/models/ratio-settings-card.tsx web/src/features/system-settings/models/__tests__/group-settings-api.test.ts
git commit -m "feat(ui): save group lifecycle settings atomically"
```

### 任务 5：实现状态开关、风险确认和删除确认

**文件：**
- 修改：`web/src/features/system-settings/models/group-ratio-visual-editor.tsx`
- 新建：`web/src/features/system-settings/models/__tests__/group-ratio-visual-editor.test.tsx`

- [ ] **步骤 1：写新建默认启用和普通启停测试**

挂载 `GroupRatioVisualEditor` 后点击“Add group”，断言最近一次 `onChange('GroupStatus', ...)` 的 JSON 包含 `group_1: true`；切换该行开关后断言为 `false`，并确认倍率和路由画像未变化。

- [ ] **步骤 2：写 `default` 停用确认测试**

点击 `default` 的开关后断言确认弹窗出现且状态未变化；点击取消后仍为 `true`；再次触发并确认后序列化为 `false`。

- [ ] **步骤 3：写删除确认和关联配置清理测试**

点击删除图标后先断言行仍存在；取消后所有 `onChange` 均未收到删除结果；确认后断言以下 JSON 中目标键被删除：`GroupRatio`、`GroupStatus`、`TopupGroupRatio`、`UserUsableGroups`、`GroupGroupRatio` 的外层和内层引用、`GroupSpecialUsableGroup`、`GroupRoutingRequirements`。

- [ ] **步骤 4：运行组件测试并确认失败**

运行：`cd web; bun test src/features/system-settings/models/__tests__/group-ratio-visual-editor.test.tsx`

预期：FAIL，页面没有状态列和确认弹窗。

- [ ] **步骤 5：扩展行模型和序列化**

```ts
type GroupPricingRow = {
  _id: string
  name: string
  ratio: string
  topupRatio: string
  selectable: boolean
  description: string
  enabled: boolean
}

const statusMap = parseBooleanMap(groupStatus)
enabled: Object.hasOwn(statusMap, name) ? statusMap[name] : true
```

`serializeGroupPricingRows` 必须为每行写入显式布尔值，并返回 `GroupStatus`。重命名时以当前行名重新生成映射，旧状态键不会残留。

`GroupRatioVisualEditorProps` 增加 `groupStatus`；父组件把该值和 `disabled` 传给 `GroupPricingTable`。`GroupPricingTable` 只负责行内编辑并通过新增的 `onRequestDelete(name, rowId)` 上报删除意图，避免子表直接解析其职责之外的覆盖、特殊规则和路由画像。

- [ ] **步骤 6：加入状态列和风险确认**

```tsx
{
  id: 'status',
  header: t('Status'),
  className: 'w-24 text-center',
  cell: (row) => (
    <div className='flex justify-center'>
      <Switch
        checked={row.enabled}
        disabled={disabled}
        onCheckedChange={(checked) => requestStatusChange(row, checked)}
        aria-label={t('Toggle group {{name}}', { name: row.name })}
      />
    </div>
  ),
}
```

使用现有 `ConfirmDialog` 分别承载 `default` 停用确认和删除确认。弹窗关闭后焦点由 Base UI 返回触发按钮；测试以 `document.activeElement` 验证。

- [ ] **步骤 7：在父编辑器实现确认后的删除级联**

`GroupRatioVisualEditor` 保存 `{ name, rowId }` 形式的待删除状态并呈现确认弹窗。确认函数调用表格提供的行删除回调，同时一次构造其余新映射并连续调用现有 `onChange`；`GroupGroupRatio` 同时删除以该组为用户组的外层键及所有内层倍率引用；`AutoGroups` 删除该名称；`GroupSpecialUsableGroup` 同时删除该组拥有的规则及其他用户组中指向该分组的 `+:/-:` 规则；动态路由画像删除同名键。完成后关闭弹窗，取消时不调用任何删除回调。

- [ ] **步骤 8：运行组件测试并提交**

运行：`cd web; bun test src/features/system-settings/models/__tests__/group-ratio-visual-editor.test.tsx`

预期：PASS。

```powershell
git add web/src/features/system-settings/models/group-ratio-visual-editor.tsx web/src/features/system-settings/models/__tests__/group-ratio-visual-editor.test.tsx
git commit -m "feat(ui): protect group lifecycle actions"
```

### 任务 6：补齐七种语言

**文件：**
- 临时新建后删除：`web/scripts/add-missing-keys.mjs`
- 修改：`web/src/i18n/locales/en.json`
- 修改：`web/src/i18n/locales/zh.json`
- 修改：`web/src/i18n/locales/zh-TW.json`
- 修改：`web/src/i18n/locales/fr.json`
- 修改：`web/src/i18n/locales/ja.json`
- 修改：`web/src/i18n/locales/ru.json`
- 修改：`web/src/i18n/locales/vi.json`

- [ ] **步骤 1：运行同步报告，确认新增键缺失**

运行：`cd web; bun run i18n:sync`

预期：报告列出新增 `t(...)` 键。

- [ ] **步骤 2：严格通过临时脚本写入翻译**

`newKeys` 至少包含以下文案及七语翻译：

```js
const newKeys = {
  en: {
    'Delete group': 'Delete group',
    'Delete group {{name}} and all of its pricing and routing settings?': 'Delete group {{name}} and all of its pricing and routing settings?',
    'Disable default group?': 'Disable default group?',
    'Disabling the default group blocks requests from accounts that still use it.': 'Disabling the default group blocks requests from accounts that still use it.',
    'Toggle group {{name}}': 'Toggle group {{name}}',
  },
  zh: {
    'Delete group': '删除分组',
    'Delete group {{name}} and all of its pricing and routing settings?': '删除分组 {{name}} 及其全部定价和路由设置？',
    'Disable default group?': '停用 default 分组？',
    'Disabling the default group blocks requests from accounts that still use it.': '停用 default 分组后，仍使用该分组的账号将无法发起请求。',
    'Toggle group {{name}}': '切换分组 {{name}} 状态',
  },
  'zh-TW': {
    'Delete group': '刪除分組',
    'Delete group {{name}} and all of its pricing and routing settings?': '刪除分組 {{name}} 及其全部定價與路由設定？',
    'Disable default group?': '停用 default 分組？',
    'Disabling the default group blocks requests from accounts that still use it.': '停用 default 分組後，仍使用該分組的帳號將無法發出請求。',
    'Toggle group {{name}}': '切換分組 {{name}} 狀態',
  },
  fr: {
    'Delete group': 'Supprimer le groupe',
    'Delete group {{name}} and all of its pricing and routing settings?': 'Supprimer le groupe {{name}} et tous ses paramètres de tarification et de routage ?',
    'Disable default group?': 'Désactiver le groupe default ?',
    'Disabling the default group blocks requests from accounts that still use it.': 'La désactivation du groupe default bloque les requêtes des comptes qui l’utilisent encore.',
    'Toggle group {{name}}': 'Activer ou désactiver le groupe {{name}}',
  },
  ja: {
    'Delete group': 'グループを削除',
    'Delete group {{name}} and all of its pricing and routing settings?': 'グループ {{name}} とその料金・ルーティング設定をすべて削除しますか？',
    'Disable default group?': 'default グループを無効にしますか？',
    'Disabling the default group blocks requests from accounts that still use it.': 'default グループを無効にすると、このグループを使用中のアカウントからのリクエストが拒否されます。',
    'Toggle group {{name}}': 'グループ {{name}} の状態を切り替え',
  },
  ru: {
    'Delete group': 'Удалить группу',
    'Delete group {{name}} and all of its pricing and routing settings?': 'Удалить группу {{name}} и все её настройки тарификации и маршрутизации?',
    'Disable default group?': 'Отключить группу default?',
    'Disabling the default group blocks requests from accounts that still use it.': 'Отключение группы default блокирует запросы учётных записей, которые всё ещё её используют.',
    'Toggle group {{name}}': 'Переключить состояние группы {{name}}',
  },
  vi: {
    'Delete group': 'Xóa nhóm',
    'Delete group {{name}} and all of its pricing and routing settings?': 'Xóa nhóm {{name}} cùng toàn bộ cấu hình định giá và định tuyến?',
    'Disable default group?': 'Tắt nhóm default?',
    'Disabling the default group blocks requests from accounts that still use it.': 'Việc tắt nhóm default sẽ chặn yêu cầu từ các tài khoản vẫn đang sử dụng nhóm này.',
    'Toggle group {{name}}': 'Chuyển trạng thái nhóm {{name}}',
  },
}
```

- [ ] **步骤 3：执行脚本、同步、验证并删除临时脚本**

运行：

```powershell
cd web
node scripts/add-missing-keys.mjs
bun run i18n:sync
Remove-Item -LiteralPath scripts/add-missing-keys.mjs
```

预期：七个 locale 的 `missingCount` 均为 0，JSON 保持排序。

- [ ] **步骤 4：提交翻译**

```powershell
git add web/src/i18n/locales/en.json web/src/i18n/locales/zh.json web/src/i18n/locales/zh-TW.json web/src/i18n/locales/fr.json web/src/i18n/locales/ja.json web/src/i18n/locales/ru.json web/src/i18n/locales/vi.json
git commit -m "feat(i18n): translate group lifecycle controls"
```

### 任务 7：全量验证、浏览器验收和低成本 Canary

**文件：**
- 新建：`docs/superpowers/reports/2026-08-09-group-list-lifecycle-acceptance.md`

- [ ] **步骤 1：运行后端全量测试**

运行：`go test ./... -count=1`

预期：PASS，无数据竞争、数据库方言或鉴权回归。

- [ ] **步骤 2：运行前端测试和静态检查**

运行：

```powershell
cd web
bun test src/features/system-settings/models/__tests__/group-settings-api.test.ts src/features/system-settings/models/__tests__/group-ratio-visual-editor.test.tsx
bun run typecheck
bun run lint
bun run build
```

预期：全部 PASS。

- [ ] **步骤 3：重建本地容器并执行桌面浏览器验收**

在 `/system-settings/billing/group-pricing` 验证：新建临时分组默认开启；普通分组可停用和恢复；`default` 停用必须确认；删除点击后数据仍存在，取消保留，确认才删除；保存并刷新后状态保持；弹窗关闭后焦点返回触发按钮。

- [ ] **步骤 4：执行移动端验收**

在 `390 × 844` 视口验证状态列、操作按钮和弹窗无重叠，无水平溢出，页面可垂直滚动，最长法语和俄语文案可读。

- [ ] **步骤 5：执行低成本真实链路 Canary**

使用专用测试分组和现有 Ark SDK 测试账号：先将分组停用并保存，发起请求，确认 HTTP 403、无任务、无成本请求、无上游调用；随后重新启用，发起一个已知低成本视频请求，确认路由、任务日志和成本核算正常。恢复测试分组和所有临时状态，不输出凭据。

- [ ] **步骤 6：编写验收报告并执行差异检查**

报告记录提交、桌面/移动结果、停用拦截证据、恢复后 Canary 的任务 ID、目标、渠道、收入、供应商成本、利润和毛利率，以及未暴露任何凭据的声明。

运行：`git diff --check; git status --short`

预期：无空白错误，只有验收报告待提交。

- [ ] **步骤 7：提交验收报告**

```powershell
git add docs/superpowers/reports/2026-08-09-group-list-lifecycle-acceptance.md
git commit -m "docs: report group lifecycle acceptance"
```

### 任务 8：最终审阅并合并到本地 `ysr`

**文件：** 无新增文件。

- [ ] **步骤 1：确认功能分支干净且提交完整**

运行：`git status --short; git log --oneline ysr..HEAD`

预期：工作区干净，日志包含状态配置、原子保存、运行时拦截、前端交互、翻译和验收报告提交。

- [ ] **步骤 2：在本地 `ysr` 执行非破坏性合并**

先确认 `ysr` 工作区没有无法绕开的用户改动；然后运行：

```powershell
git switch ysr
git merge --no-ff codex/group-capability-routing-profiles
```

预期：合并成功，不覆盖与本任务无关的用户改动。

- [ ] **步骤 3：在合并后的 `ysr` 复跑关键验证**

运行：`go test ./setting/ratio_setting ./service ./controller ./middleware -count=1; cd web; bun run typecheck; bun run build`

预期：全部 PASS，`git status --short` 干净。
