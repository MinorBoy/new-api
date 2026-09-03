# 图像协议绑定默认配置快捷恢复 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让支持 OpenAI Images 的渠道在图像协议配置为空时自动获得标准档案，并提供一键恢复默认配置操作。

**Architecture:** 默认档案和空值判断位于渠道表单领域模块；渠道抽屉按表单生命周期应用默认值并渲染恢复按钮。通用 JSON 编辑器增加可选工具栏动作槽。后端保存、校验、兼容性测试和计费逻辑保持不变。

**Tech Stack:** React 19、TypeScript、React Hook Form、Zod、i18next、Yace、Bun test、happy-dom。

---

### Task 1: 默认图像档案与空值判断

**Files:**
- Modify: `web/src/features/channels/lib/channel-form.ts`
- Create: `web/src/features/channels/lib/__tests__/image-profile-defaults.test.ts`

- [ ] **Step 1: Write the failing test**

```ts
import assert from 'node:assert/strict'
import test from 'node:test'
import { DEFAULT_IMAGE_PROFILE_JSON, isEmptyImageProfile } from '../channel-form'

test('empty image profile values are only blank text or an empty object', () => {
  assert.equal(isEmptyImageProfile(undefined), true)
  assert.equal(isEmptyImageProfile('  '), true)
  assert.equal(isEmptyImageProfile('{}'), true)
  assert.equal(isEmptyImageProfile('{"profile":"custom"}'), false)
  assert.equal(isEmptyImageProfile('null'), false)
  assert.equal(isEmptyImageProfile('{invalid'), false)
})

test('default image profile contains standard OpenAI Images routes', () => {
  assert.deepEqual(JSON.parse(DEFAULT_IMAGE_PROFILE_JSON), {
    profile: 'openai_images', profile_version: 1,
    paths: { generations: '/v1/images/generations', edits: '/v1/images/edits' },
  })
})
```

- [ ] **Step 2: Run and verify failure**

Run from `web/`: `bun test src/features/channels/lib/__tests__/image-profile-defaults.test.ts`. Expected: FAIL because the exports do not exist.

- [ ] **Step 3: Implement the helper**

Add this export before `CHANNEL_FORM_DEFAULT_VALUES`:

```ts
export const DEFAULT_IMAGE_PROFILE_JSON = JSON.stringify({
  profile: 'openai_images', profile_version: 1,
  paths: { generations: '/v1/images/generations', edits: '/v1/images/edits' },
}, null, 2)

export function isEmptyImageProfile(value: string | undefined): boolean {
  const trimmed = value?.trim() || ''
  return trimmed === '' || trimmed === '{}'
}
```

Do not classify `null`, arrays, or invalid non-empty JSON as empty; existing Zod validation must report those values.

- [ ] **Step 4: Run and verify pass**

Run `bun test src/features/channels/lib/__tests__/image-profile-defaults.test.ts`; expected: PASS.

- [ ] **Step 5: Commit**

`git add web/src/features/channels/lib/channel-form.ts web/src/features/channels/lib/__tests__/image-profile-defaults.test.ts && git commit -m "feat: add default OpenAI Images profile"`

### Task 2: JSON 编辑器工具栏动作槽

**Files:**
- Modify: `web/src/components/json-code-editor.tsx`
- Modify: `web/src/components/json-code-editor/__tests__/json-code-editor.test.tsx`

- [ ] **Step 1: Write the failing test**

Add to the existing component test suite:

```tsx
test('renders an optional toolbar action', async () => {
  const rendered = await renderEditor({ value: '{}', onChange: () => undefined,
    toolbarActions: <button type='button' disabled>Restore</button> })
  const button = [...rendered.container.querySelectorAll('button')]
    .find((candidate) => candidate.textContent === 'Restore')
  assert.ok(button)
  assert.equal(button.disabled, true)
  await unmountEditor(rendered)
})
```

- [ ] **Step 2: Run and verify failure**

Run `bun test src/components/json-code-editor/__tests__/json-code-editor.test.tsx`; expected: FAIL because `toolbarActions` is not rendered.

- [ ] **Step 3: Implement the slot**

Import `ReactNode`, add `toolbarActions?: ReactNode` to `JsonCodeEditorProps`, destructure it, and render `{toolbarActions}` in the toolbar before Copy. Do not change editor effects or existing button behavior.

- [ ] **Step 4: Run and verify pass**

Run the same component test; expected: PASS for the new and existing lifecycle, controlled update, and formatting cases.

- [ ] **Step 5: Commit**

`git add web/src/components/json-code-editor.tsx web/src/components/json-code-editor/__tests__/json-code-editor.test.tsx && git commit -m "feat: support custom JSON editor toolbar actions"`

### Task 3: 渠道抽屉自动填充与恢复

**Files:**
- Modify: `web/src/features/channels/components/drawers/channel-mutate-drawer.tsx`
- Modify: `web/src/features/channels/components/drawers/__tests__/secure-identity-lock.test.tsx`

- [ ] **Step 1: Write failing drawer behavior tests**

Add an OpenAI Images fixture with `settings: '{}'`, then assert the mounted drawer's `textarea[name="image_profile"]` contains `"profile": "openai_images"` and `/v1/images/generations`. Add a custom-profile fixture, click the button named `Reset to default configuration`, and assert the textarea changes from `custom` to the default profile. Keep the Secure fixture and assert it has no reset button.

- [ ] **Step 2: Run and verify failure**

Run `bun test src/features/channels/components/drawers/__tests__/secure-identity-lock.test.tsx`; expected: the new behavior assertions fail while existing security and supplier-cost tests remain runnable.

- [ ] **Step 3: Implement lifecycle-safe default application**

Import `DEFAULT_IMAGE_PROFILE_JSON` and `isEmptyImageProfile`. Add `imageProfileAutoFilled` state. After editing data is transformed and reset, and after new-form/type-change initialization, call a callback that checks `OPENAI_IMAGES_CHANNEL_TYPES` and only calls `form.setValue('image_profile', DEFAULT_IMAGE_PROFILE_JSON, { shouldDirty: true, shouldValidate: true })` when `isEmptyImageProfile(form.getValues('image_profile'))` is true. Set the status flag only when the callback fills a value; valid non-empty edits remain untouched. Clear the flag on manual editor changes, drawer close, and non-image types.

- [ ] **Step 4: Render reset action and status**

Pass a `toolbarActions` button to `JsonCodeEditor` only inside the existing image-channel branch. Use the existing `Reset to default configuration` translation and a `RotateCcw` icon; on click set the default JSON with `shouldDirty` and `shouldValidate`. Disable it when `sensitiveLocked || isSubmitting`. Add translated status text for automatic vs custom configuration and shorten the placeholder. Keep compatibility-test save gating unchanged.

- [ ] **Step 5: Run and verify pass**

Run the focused drawer test; expected: PASS, including the existing Secure lock and supplier-cost cases.

- [ ] **Step 6: Commit**

`git add web/src/features/channels/components/drawers/channel-mutate-drawer.tsx web/src/features/channels/components/drawers/__tests__/secure-identity-lock.test.tsx && git commit -m "feat: auto-fill image protocol profile in channel editor"`

### Task 4: 国际化文案

**Files:**
- Modify: `web/src/i18n/static-keys.ts`
- Modify: `web/src/i18n/locales/en.json`
- Modify: `web/src/i18n/locales/zh.json`
- Modify: `web/src/i18n/locales/zh-TW.json`
- Modify: `web/src/i18n/locales/fr.json`
- Modify: `web/src/i18n/locales/ja.json`
- Modify: `web/src/i18n/locales/ru.json`
- Modify: `web/src/i18n/locales/vi.json`

- [ ] **Step 1: Register keys**

Add `Default image configuration applied automatically` and `Custom image configuration` to the OpenAI Images section of `STATIC_I18N_KEYS`; reuse the existing `Reset to default configuration` key.

- [ ] **Step 2: Sync translations**

From `web/`, run `bun run i18n:sync`. Verify all seven locale files contain both keys and preserve unrelated translations.

- [ ] **Step 3: Commit**

`git add web/src/i18n/static-keys.ts web/src/i18n/locales && git commit -m "i18n: add image profile default status labels"`

### Task 5: 全量验证

**Files:** Verify the files changed in Tasks 1-4.

- [ ] **Step 1: Run affected tests**

From `web/`, run `bun test src/features/channels/lib/__tests__/image-profile-defaults.test.ts src/components/json-code-editor/__tests__/json-code-editor.test.tsx src/features/channels/components/drawers/__tests__/secure-identity-lock.test.tsx`; expected: all PASS.

- [ ] **Step 2: Typecheck**

Run `bun run typecheck`; expected: `tsgo -b` exits successfully with no errors.

- [ ] **Step 3: Lint and format**

Run `bun run lint` and `bun run format:check`; expected: no lint errors and format check passes.

- [ ] **Step 4: Scope review**

Run `git diff --check` and `git status --short`. Confirm no API keys, passwords, credentials, or `.superpowers/` preview files are staged. Preserve the pre-existing untracked `web/test-results/` and do not commit it.

