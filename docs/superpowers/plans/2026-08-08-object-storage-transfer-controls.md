# 对象存储视频转存控制 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为对象存储运维页面增加全部转存/域名规则互斥模式和白名单/黑名单独立开关，并让视频转存服务、管理 API、旧配置迁移和测试保持一致。

**Architecture:** 在 `setting/object_storage` 保存明确的 `transfer_mode` 和两个规则开关；`pkg/objectstorage` 只负责基于传入策略判断域名；`service` 使用运行时配置执行转存；管理控制器和 React 表单负责 DTO 映射与即时互斥。旧配置在运行时归一化，不新增表结构。

**Tech Stack:** Go 1.22、GORM 配置选项、Gin、React 19、React Hook Form、Zod、TanStack Query、Bun test、i18next。

---

### Task 1: 扩展对象存储配置并覆盖旧配置迁移

**Files:**
- Modify: `setting/object_storage/config.go`
- Test: `setting/object_storage/config_test.go`

- [ ] **Step 1: 写归一化失败测试**

在 `config_test.go` 增加以下行为用例：

```go
func TestNormalizeConfigMigratesLegacyDomainLists(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TransferDomainWhitelist = []string{"Own.Example.com"}
	cfg.NoTransferDomainBlacklist = []string{"CDN.Example.com"}

	got := NormalizeConfig(cfg)
	require.Equal(t, "rules", got.TransferMode)
	require.True(t, got.WhitelistEnabled)
	require.True(t, got.BlacklistEnabled)
}

func TestNormalizeConfigDefaultsToNoTransfer(t *testing.T) {
	got := NormalizeConfig(DefaultConfig())
	require.Equal(t, "default", got.TransferMode)
	require.False(t, got.WhitelistEnabled)
	require.False(t, got.BlacklistEnabled)
}

func TestNormalizeConfigPreservesExplicitModeAndRules(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TransferMode = "all"
	cfg.WhitelistEnabled = true
	cfg.TransferDomainWhitelist = []string{"provider.example.com"}

	got := NormalizeConfig(cfg)
	require.Equal(t, "all", got.TransferMode)
	require.True(t, got.WhitelistEnabled)
	require.Equal(t, []string{"provider.example.com"}, got.TransferDomainWhitelist)
}

func TestNormalizeConfigRejectsUnknownModeByDefaulting(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TransferMode = "unsupported"
	require.Equal(t, "default", NormalizeConfig(cfg).TransferMode)
}
```

- [ ] **Step 2: 运行测试确认新用例先失败**

运行：`go test ./setting/object_storage -run 'TestNormalizeConfig(MigratesLegacyDomainLists|DefaultsToNoTransfer|PreservesExplicitModeAndRules|RejectsUnknownModeByDefaulting)'`

预期：因 `ObjectStorageConfig` 尚无新字段而编译失败。

- [ ] **Step 3: 实现配置字段和归一化**

在 `ObjectStorageConfig` 增加：

```go
TransferMode      string `json:"transfer_mode"`
WhitelistEnabled  bool   `json:"whitelist_enabled"`
BlacklistEnabled  bool   `json:"blacklist_enabled"`
```

在常量中定义 `default`、`all`、`rules` 三个字符串值；`DefaultConfig` 使用 `default` 和两个关闭的规则开关。`NormalizeConfig` 先规范化域名，再在 `TransferMode == ""` 时根据旧列表非空情况迁移，最后将未知值归一为 `default`。显式模式和规则开关必须保留，不清空列表。

- [ ] **Step 4: 运行配置测试确认通过**

运行：`go test ./setting/object_storage`

预期：该包全部通过。

- [ ] **Step 5: 提交配置模型变更**

```bash
git add setting/object_storage/config.go setting/object_storage/config_test.go
git commit -m "feat: add object storage transfer modes"
```

### Task 2: 实现三种转存模式和四种规则组合

**Files:**
- Modify: `pkg/objectstorage/rules.go`
- Test: `pkg/objectstorage/rules_test.go`

- [ ] **Step 1: 写转存判定失败测试**

将测试表扩展为显式传入模式与规则开关，并覆盖：`default`、`all`、白名单单开、黑名单单开、双开黑名单优先、双关默认不转存、通配域名和非法 URL。

```go
got, err := ShouldTransfer(
	"https://own.example.com/a.mp4",
	"rules",
	true,
	false,
	[]string{"own.example.com"},
	nil,
)
require.NoError(t, err)
assert.True(t, got)
```

必须增加一条 `all` 模式在空列表下仍返回 `true`，以及一条 `rules` 模式白名单和黑名单同时命中返回 `false`。

- [ ] **Step 2: 运行规则测试确认先失败**

运行：`go test ./pkg/objectstorage -run 'TestShouldTransfer'`

预期：因函数签名和模式判断尚未实现而失败。

- [ ] **Step 3: 实现最小判定逻辑**

将签名改为：

```go
func ShouldTransfer(rawURL, mode string, whitelistEnabled, blacklistEnabled bool, whitelist, blacklist []string) (bool, error)
```

先解析并校验 URL；`default` 返回 `false`，`all` 返回 `true`；`rules` 仅在对应开关开启时匹配列表，保留黑名单先判定的优先级。未知模式按 `default` 处理，避免错误配置导致意外转存。

- [ ] **Step 4: 运行规则测试确认通过**

运行：`go test ./pkg/objectstorage`

预期：规则测试全部通过。

- [ ] **Step 5: 提交规则判断变更**

```bash
git add pkg/objectstorage/rules.go pkg/objectstorage/rules_test.go
git commit -m "feat: support object storage transfer modes"
```

### Task 3: 接入视频转存服务并补充服务回归测试

**Files:**
- Modify: `service/video_result_storage.go`
- Test: `service/video_result_storage_test.go`

- [ ] **Step 1: 为服务写失败回归用例**

更新 `enabledVideoStorageValues` fixture 写入 `transfer_mode=rules`、`whitelist_enabled=true`、`blacklist_enabled=false`。新增表驱动用例验证：

```go
{"all transfers an unmatched URL", "all", false, false, true}
{"rules with both switches off keeps URL", "rules", false, false, false}
{"rules with blacklist only skips listed URL", "rules", false, true, false}
```

用 fake store 断言 `putCalls` 和 `task.PrivateData.ResultURL/ResultObjectKey`，确保不转存路径不下载、不上传。

- [ ] **Step 2: 运行服务测试确认先失败**

运行：`go test ./service -run 'TestProcessVideoResult'`

预期：新 fixture 字段无法参与判定或 `all` 模式仍被列表逻辑拒绝。

- [ ] **Step 3: 接入配置字段**

在 `ProcessVideoResultURL` 中将现有调用替换为：

```go
transfer, err := objectstorage.ShouldTransfer(
	sourceURL,
	cfg.TransferMode,
	cfg.WhitelistEnabled,
	cfg.BlacklistEnabled,
	cfg.TransferDomainWhitelist,
	cfg.NoTransferDomainBlacklist,
)
```

总开关逻辑保持在调用前，确保关闭总开关不会触发对象存储配置校验、源 URL 下载或上传。

- [ ] **Step 4: 运行服务和相关后端测试**

运行：`go test ./service ./pkg/objectstorage ./setting/object_storage`

预期：全部通过。

- [ ] **Step 5: 提交服务接入变更**

```bash
git add service/video_result_storage.go service/video_result_storage_test.go
git commit -m "test: cover object storage transfer mode routing"
```

### Task 4: 更新对象存储管理 API 和前端类型

**Files:**
- Modify: `controller/object_storage.go`
- Test: `controller/object_storage_test.go`
- Modify: `web/src/features/system-settings/types.ts`

- [ ] **Step 1: 写控制器字段回归测试**

扩展 GET 测试配置并断言响应包含：

```go
assert.Contains(t, recorder.Body.String(), `"transfer_mode":"rules"`)
assert.Contains(t, recorder.Body.String(), `"whitelist_enabled":true`)
assert.Contains(t, recorder.Body.String(), `"blacklist_enabled":false`)
```

新增 PUT 用例发送全部转存和规则开关字段，断言 `object_storage.Runtime()` 与响应中的值一致。

- [ ] **Step 2: 运行控制器测试确认先失败**

运行：`go test ./controller -run 'Test(GetObjectStorageSettings|UpdateObjectStorageSettings)'`

预期：响应 DTO 和请求 DTO 尚无新增字段，断言失败。

- [ ] **Step 3: 更新请求/响应 DTO 和配置映射**

在 `objectStorageSettingsRequest`、`objectStorageSettingsResponse` 和 `objectStorageConfigFromRequest` / `objectStorageResponse` 中增加三项字段，并通过 `NormalizeConfig` 返回最终归一化状态。

- [ ] **Step 4: 更新前端类型**

在 `ObjectStorageSettings` 增加：

```ts
transfer_mode: 'default' | 'all' | 'rules'
whitelist_enabled: boolean
blacklist_enabled: boolean
```

`ObjectStorageSettingsRequest` 通过现有 `Omit` 继承这些字段。

- [ ] **Step 5: 运行控制器测试**

运行：`go test ./controller -run 'Test(GetObjectStorageSettings|UpdateObjectStorageSettings)'`

预期：全部通过。

### Task 5: 重构对象存储表单的转存控制交互

**Files:**
- Modify: `web/src/features/system-settings/operations/object-storage-section.tsx`
- Test: `web/src/features/system-settings/operations/__tests__/object-storage-section.test.tsx`

- [ ] **Step 1: 先补组件失败测试**

更新 fixture 增加 `transfer_mode`、`whitelist_enabled`、`blacklist_enabled`。新增用户视角测试：

```ts
test('turning on all transfer turns off domain rules', async () => {
  const mounted = await renderSection()
  const all = getSwitch(mounted, 'Enable all video transfer')
  const rules = getSwitch(mounted, 'Enable domain rules')

  await act(async () => all.click())

  assert.equal(all.getAttribute('data-state'), 'checked')
  assert.equal(rules.getAttribute('data-state'), 'unchecked')
  await unmount(mounted)
})
```

再增加：规则开关关闭时对应 textarea 的 `disabled` 为真且文本未清空；保存请求携带 `transfer_mode` 和两个开关；关闭总开关再打开后值仍保留。

- [ ] **Step 2: 运行组件测试确认先失败**

运行：`cd web; bun test src/features/system-settings/operations/__tests__/object-storage-section.test.tsx`

预期：找不到新增开关或请求字段不匹配。

- [ ] **Step 3: 扩展表单值和转换函数**

在 `ObjectStorageFormValues`、默认值、`settingsToFormValues` 和 `formValuesToRequest` 中加入：

```ts
transferMode: 'default' | 'all' | 'rules'
whitelistEnabled: boolean
blacklistEnabled: boolean
```

切换“全部转存”时设置 `transferMode='all'`；关闭时设置 `default`。切换“域名规则”时设置 `rules`；关闭时设置 `default`。每个切换处理器同时更新表单中的另一模式值，使用 `shouldDirty: true`，不修改列表文本。

- [ ] **Step 4: 渲染互斥模式和规则行**

在“Enable object storage”分组中保留总开关，并增加两个 `FormField` + `SettingsSwitchItem`。规则分组仅在 `transferMode === 'rules'` 时显示；白名单和黑名单各自使用 `FormField`、`Switch`、`Textarea`，关闭开关时将 textarea 的 `disabled` 设为真。所有新可见文案使用字面量 `t('...')` 键。

- [ ] **Step 5: 更新提示和可访问性**

为三个模式开关和两个规则开关提供稳定的 `aria-label`；同时开启两个规则开关时显示黑名单优先提示；总开关关闭时显示暂停提示但不禁用保存操作。

- [ ] **Step 6: 运行组件测试确认通过**

运行：`cd web; bun test src/features/system-settings/operations/__tests__/object-storage-section.test.tsx`

预期：原有连接、密钥、域名规范化和连接测试用例，以及新增交互用例全部通过。

- [ ] **Step 7: 提交表单实现**

```bash
git add web/src/features/system-settings/operations/object-storage-section.tsx web/src/features/system-settings/operations/__tests__/object-storage-section.test.tsx web/src/features/system-settings/types.ts
git commit -m "feat: improve object storage transfer controls"
```

### Task 6: 补齐前端多语言文案

**Files:**
- Create temporarily: `web/scripts/add-missing-keys.mjs`
- Modify through script only: `web/src/i18n/locales/{en,zh,zh-TW,fr,ja,ru,vi}.json`
- Modify: `web/src/i18n/locales/_reports/_sync-report.json` (generated)

- [ ] **Step 1: 确认新增键**

执行 `node scripts/find-missing-keys.mjs`，只为本次页面使用的新增键补齐翻译。建议键包括：`Enable all video transfer`、`Enable domain rules`、`Enable transfer whitelist`、`Enable transfer blacklist`、`All eligible videos are uploaded.`、`Use whitelist and blacklist rules to decide.`、`Transfer is paused while the main switch is off.`、`The blacklist takes priority when both rules match.`。

- [ ] **Step 2: 通过脚本写入七种语言**

使用 `add-missing-keys.mjs` 的 `newKeys` 对象写入英文、简体中文、繁体中文、法语、日语、俄语和越南语；禁止直接编辑 JSON。

- [ ] **Step 3: 同步和检查键完整性**

运行：

```bash
cd web
node scripts/add-missing-keys.mjs
node scripts/find-missing-keys.mjs
bun run i18n:sync
```

预期：缺失键扫描输出 `All t() keys found in en.json!`，同步报告无本次新增键缺失。

- [ ] **Step 4: 删除临时脚本并提交文案**

```bash
Remove-Item web/scripts/add-missing-keys.mjs
git add web/src/i18n/locales
git commit -m "i18n: translate object storage transfer controls"
```

### Task 7: 完整验证和页面验收

**Files:**
- No production file changes; inspect generated frontend output and test reports.

- [ ] **Step 1: 运行后端相关测试**

运行：`go test ./pkg/objectstorage ./setting/object_storage ./service ./controller`

预期：所有测试通过，退出码为 0。

- [ ] **Step 2: 运行前端类型、lint、组件测试和构建**

运行：

```bash
cd web
bun test src/features/system-settings/operations/__tests__/object-storage-section.test.tsx
bun run typecheck
bun run lint -- src/features/system-settings/operations/object-storage-section.tsx src/features/system-settings/operations/__tests__/object-storage-section.test.tsx src/features/system-settings/types.ts
bun run build
```

预期：测试、类型检查、涉及文件 lint 和生产构建均以退出码 0 完成。

- [ ] **Step 3: 在对象存储页面手工验收**

打开 `/system-settings/operations/object-storage`，逐项验证：总开关关闭/开启；全部转存与域名规则互斥；规则双关、单关和双关关闭四种组合；关闭规则开关后文本保留；关闭总开关后重新开启配置仍保留；保存后刷新页面状态一致。

- [ ] **Step 4: 检查工作区范围**

运行：`git status --short`，确认提交只包含本计划相关文件，保留用户已有的渠道转换器改动。
