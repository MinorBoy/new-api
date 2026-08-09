# 适配路由目标弹窗宽度 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将“适配后的路由目标”弹窗设置为桌面端视口宽度的 50%，同时保持移动端接近全屏宽度。

**Architecture:** 仅调整 `GroupRoutingTargetsDialog` 的 `DialogContent` 响应式 Tailwind 宽度约束，同时设置 `width` 与 `max-width` 以覆盖基础 Dialog 的断点限制。使用现有 happy-dom 组件测试保护类名所表达的稳定布局合同，不修改请求、筛选、分页或目标操作逻辑。

**Tech Stack:** React 19、TypeScript、Tailwind CSS、Base UI Dialog、Bun、Node test、happy-dom

---

### Task 1: 固化并实现响应式弹窗宽度

**Files:**
- Modify: `web/src/features/system-settings/models/group-routing-targets-dialog.tsx:294`
- Test: `web/src/features/system-settings/models/__tests__/group-routing-targets-dialog.test.tsx`

- [ ] **Step 1: 编写失败的布局回归测试**

在 `group-routing-targets-dialog.test.tsx` 的交互测试之前增加：

```tsx
test('uses half viewport width on desktop and near-full width on mobile', async () => {
  const mounted = await mountDialog()
  try {
    const dialog = browserWindow.document.querySelector('[role="dialog"]')
    assert.ok(dialog instanceof browserWindow.HTMLElement)
    const classes = dialog.className.split(/\s+/)
    assert.ok(classes.includes('w-[96vw]'))
    assert.ok(classes.includes('max-w-[96vw]'))
    assert.ok(classes.includes('sm:w-[50vw]'))
    assert.ok(classes.includes('sm:max-w-[50vw]'))
  } finally {
    await unmountDialog(mounted)
  }
})
```

- [ ] **Step 2: 运行测试并确认布局合同尚未满足**

Run:

```powershell
cd web
bun test --parallel=1 src/features/system-settings/models/__tests__/group-routing-targets-dialog.test.tsx
```

Expected: FAIL，提示至少一个 `96vw` 或 `50vw` 类不存在；现有五项交互测试仍通过。

- [ ] **Step 3: 实现最小响应式宽度调整**

将 `DialogContent` 修改为：

```tsx
<DialogContent className='flex max-h-[88vh] w-[96vw] max-w-[96vw] flex-col overflow-hidden p-0 sm:w-[50vw] sm:max-w-[50vw]'>
```

默认使用 `96vw`，`sm` 及以上同时用 `width` 和 `max-width` 固定为 `50vw`，覆盖基础组件的 `sm:max-w-lg`。

- [ ] **Step 4: 运行组件测试并确认通过**

Run:

```powershell
cd web
bun test --parallel=1 src/features/system-settings/models/__tests__/group-routing-targets-dialog.test.tsx
```

Expected: 6 tests PASS，0 fail。

- [ ] **Step 5: 执行前端静态验证**

Run:

```powershell
cd web
bunx oxlint -c .oxlintrc.json src/features/system-settings/models/group-routing-targets-dialog.tsx src/features/system-settings/models/__tests__/group-routing-targets-dialog.test.tsx
bun run typecheck
bun run build
```

Expected: 所有命令退出码为 0；受影响文件无 lint error；类型检查和生产构建成功。

- [ ] **Step 6: 执行浏览器布局验收**

在 `/system-settings/billing/group-pricing` 打开任一分组的“适配后的路由目标”弹窗：

```text
桌面视口：弹窗实际宽度约等于 window.innerWidth * 0.5
390px 移动视口：弹窗实际宽度约等于 390px * 0.96
页面 documentElement.scrollWidth 等于 clientWidth
```

Expected: 桌面弹窗为半屏宽度；移动端接近全宽；页面无横向溢出；目标表格仍只在弹窗内部滚动。

- [ ] **Step 7: 提交实现**

```powershell
git add web/src/features/system-settings/models/group-routing-targets-dialog.tsx web/src/features/system-settings/models/__tests__/group-routing-targets-dialog.test.tsx
git commit -m "fix(ui): widen adapted routing target dialog"
```

Expected: 产生一个仅包含弹窗宽度和回归测试的提交。
