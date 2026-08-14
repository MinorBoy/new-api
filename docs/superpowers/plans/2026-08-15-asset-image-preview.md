# 素材图片大图预览实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让用户可从素材库表格点击缩略图，在可访问的弹窗中查看适配当前视口的原图。

**Architecture:** 在现有素材页中复用 Base UI 封装的 `Dialog` 组件，以缩略图按钮作为触发器。弹窗只负责展示当前行图片，不新增全局状态、接口或第三方依赖。

**Tech Stack:** React 19、TypeScript、Base UI Dialog、Tailwind CSS、Bun test、happy-dom

---

### Task 1: 素材大图预览交互

**Files:**
- Create: `web/src/features/assets/__tests__/image-preview.test.tsx`
- Modify: `web/src/features/assets/index.tsx`

- [ ] **Step 1: 编写失败的交互测试**

测试使用固定素材查询数据渲染 `Assets`，从用户可访问名称定位预览按钮，点击后断言出现 `dialog`、标题和原图。

```tsx
test('opens a large image dialog when the asset thumbnail is clicked', async () => {
  const previewButton = document.querySelector(
    'button[aria-label="Image Preview"]'
  ) as HTMLButtonElement | null
  assert.ok(previewButton)

  await act(async () => previewButton.click())

  const dialog = document.querySelector('[role="dialog"]')
  assert.ok(dialog)
  assert.equal(dialog.getAttribute('aria-labelledby') !== null, true)
  assert.equal(
    dialog.querySelector('img')?.getAttribute('src'),
    'https://example.com/character.png'
  )
})
```

- [ ] **Step 2: 运行测试并确认因功能缺失而失败**

Run: `cd web && bun test --parallel=1 src/features/assets/__tests__/image-preview.test.tsx`

Expected: FAIL，找不到带有 `Image Preview` 可访问名称的按钮。

- [ ] **Step 3: 实现缩略图触发器和大图弹窗**

在每行预览单元格中复用现有 Dialog：

```tsx
<Dialog>
  <DialogTrigger
    aria-label={t('Image Preview')}
    className='group rounded outline-none focus-visible:ring-2 focus-visible:ring-ring'
  >
    <img
      src={asset.url}
      alt=''
      className='size-12 cursor-zoom-in rounded object-cover transition-opacity group-hover:opacity-80'
    />
  </DialogTrigger>
  <DialogContent className='w-auto max-w-[calc(100vw-2rem)] p-2 sm:max-w-[calc(100vw-4rem)]'>
    <DialogTitle className='sr-only'>{t('Image Preview')}</DialogTitle>
    <img
      src={asset.url}
      alt={t('Image Preview')}
      className='max-h-[calc(100vh-4rem)] max-w-full rounded object-contain'
    />
  </DialogContent>
</Dialog>
```

- [ ] **Step 4: 运行测试并确认通过**

Run: `cd web && bun test --parallel=1 src/features/assets/__tests__/image-preview.test.tsx`

Expected: PASS，1 个测试通过。

- [ ] **Step 5: 执行前端静态检查与构建**

Run:

```powershell
cd web
bun run typecheck
bunx oxlint -c .oxlintrc.json src/features/assets/index.tsx src/features/assets/__tests__/image-preview.test.tsx
bun run build
```

Expected: 所有命令退出码均为 0。

- [ ] **Step 6: 提交变更**

```powershell
git add docs/superpowers/plans/2026-08-15-asset-image-preview.md web/src/features/assets/index.tsx web/src/features/assets/__tests__/image-preview.test.tsx
git commit -m "feat(assets): add image preview dialog"
```
