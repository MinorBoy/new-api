# Dimensio Channel UI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make Dimensio selectable and configurable through the default frontend's standard administrator channel form.

**Architecture:** Register backend channel type `59` in the existing frontend registries, expose its provider defaults through `channel-type-config.ts`, and consume those defaults and hints in the existing channel drawer without creating a separate workflow. Keep Dimensio outside generic model discovery because its ARK task API has no compatible model-list endpoint.

**Tech Stack:** React 19, TypeScript, Bun test runner, i18next, React Hook Form, Rsbuild, Docker Compose, Playwright CLI

---

### Task 1: Add a failing Dimensio channel configuration test

**Files:**
- Create: `web/default/src/features/channels/lib/channel-type-config.test.ts`

- [ ] **Step 1: Write the failing test**

```ts
import { describe, expect, test } from 'bun:test'

import {
  CHANNEL_TYPE_OPTIONS,
  CHANNEL_TYPE_WARNINGS,
  CHANNEL_TYPES,
  MODEL_FETCHABLE_TYPES,
  TYPE_TO_KEY_PROMPT,
} from '../constants'
import {
  getChannelTypeConfig,
  getChannelTypeHints,
  getDefaultBaseUrl,
} from './channel-type-config'
import { getChannelTypeIcon } from './channel-utils'

describe('Dimensio channel configuration', () => {
  test('registers type 59 in the standard channel options', () => {
    expect(CHANNEL_TYPES[59]).toBe('Dimensio')
    expect(CHANNEL_TYPE_OPTIONS).toContainEqual({ value: 59, label: 'Dimensio' })
    expect(getChannelTypeIcon(59)).toBe('Dimensio')
  })

  test('provides the Dimensio form defaults and guidance', () => {
    expect(getChannelTypeConfig(59)).toMatchObject({
      id: 59,
      name: 'Dimensio',
      icon: 'Dimensio',
      defaultBaseUrl: 'https://jimeng.dimensio.cn',
      supportedModels: [
        'jimeng-video-seedance-2.0-fast-vip',
        'jimeng-video-seedance-2.0-mini',
        'jimeng-video-seedance-2.0-vip',
      ],
    })
    expect(getDefaultBaseUrl(59)).toBe('https://jimeng.dimensio.cn')
    expect(getChannelTypeHints(59)).toEqual({
      baseUrl: 'Default: https://jimeng.dimensio.cn',
      key: 'Enter the raw API key issued by Dimensio',
      models:
        'Supported upstream models: jimeng-video-seedance-2.0-fast-vip, jimeng-video-seedance-2.0-mini, jimeng-video-seedance-2.0-vip',
    })
    expect(TYPE_TO_KEY_PROMPT[59]).toBe(
      'Enter the raw API key issued by Dimensio'
    )
    expect(CHANNEL_TYPE_WARNINGS[59]).toBe(
      'Dimensio is task-only. Call it through the ARK /api/v3 task API.'
    )
  })

  test('does not enable generic model fetching for Dimensio', () => {
    expect(MODEL_FETCHABLE_TYPES.has(59)).toBe(false)
  })
})
```

- [ ] **Step 2: Run the test and verify RED**

Run from `web/default/`:

```bash
bun test src/features/channels/lib/channel-type-config.test.ts
```

Expected: FAIL because type `59` and its configuration are absent.

### Task 2: Register Dimensio and expose its provider configuration

**Files:**
- Modify: `web/default/src/features/channels/constants.ts`
- Modify: `web/default/src/features/channels/lib/channel-type-config.ts`
- Modify: `web/default/src/features/channels/lib/channel-utils.ts`
- Test: `web/default/src/features/channels/lib/channel-type-config.test.ts`

- [ ] **Step 1: Register type `59`, its display order, prompt, and warning**

Add `59: 'Dimensio'` to `CHANNEL_TYPES`, append `59` after the video-generation providers in `CHANNEL_TYPE_DISPLAY_ORDER`, and add:

```ts
export const TYPE_TO_KEY_PROMPT: Record<number, string> = {
  // existing entries
  59: 'Enter the raw API key issued by Dimensio',
}

export const CHANNEL_TYPE_WARNINGS: Record<number, string> = {
  // existing entries
  59: 'Dimensio is task-only. Call it through the ARK /api/v3 task API.',
}
```

Do not add `59` to `MODEL_FETCHABLE_TYPES`.

- [ ] **Step 2: Add the type-specific provider configuration**

Add this entry to `CHANNEL_TYPE_CONFIGS`:

```ts
59: {
  id: 59,
  name: CHANNEL_TYPES[59],
  icon: 'Dimensio',
  defaultBaseUrl: 'https://jimeng.dimensio.cn',
  supportedModels: [
    'jimeng-video-seedance-2.0-fast-vip',
    'jimeng-video-seedance-2.0-mini',
    'jimeng-video-seedance-2.0-vip',
  ],
  hints: {
    baseUrl: 'Default: https://jimeng.dimensio.cn',
    key: 'Enter the raw API key issued by Dimensio',
    models:
      'Supported upstream models: jimeng-video-seedance-2.0-fast-vip, jimeng-video-seedance-2.0-mini, jimeng-video-seedance-2.0-vip',
  },
},
```

- [ ] **Step 3: Give type `59` a stable icon-adapter key**

Add `59: 'Dimensio'` to `TYPE_TO_ICON` in `getChannelTypeIcon`. The current icon adapter will render its neutral initial fallback when no vendor artwork exists, with no new dependency.

- [ ] **Step 4: Run the focused test and verify GREEN**

Run from `web/default/`:

```bash
bun test src/features/channels/lib/channel-type-config.test.ts
```

Expected: 3 tests pass.

- [ ] **Step 5: Commit the tested provider registry**

```bash
git add web/default/src/features/channels/constants.ts web/default/src/features/channels/lib/channel-type-config.ts web/default/src/features/channels/lib/channel-utils.ts web/default/src/features/channels/lib/channel-type-config.test.ts
git commit -m "feat(channels): register Dimensio provider"
```

### Task 3: Wire Dimensio defaults and hints into the standard channel drawer

**Files:**
- Modify: `web/default/src/features/channels/components/drawers/channel-mutate-drawer.tsx`
- Test: `web/default/src/features/channels/lib/channel-type-config.test.ts`

- [ ] **Step 1: Read the selected provider configuration in the drawer**

Import `getChannelTypeHints` and `getDefaultBaseUrl` from `../../lib`, then derive the hints for `currentType`:

```ts
const currentChannelTypeHints = useMemo(
  () => getChannelTypeHints(currentType),
  [currentType]
)
```

- [ ] **Step 2: Apply the Dimensio default URL only during channel creation**

Extend the existing type-change effect without changing edit behavior or defaults for other providers:

```ts
if (currentType === 59) {
  const currentBaseUrlValue = form.getValues('base_url')
  if (!currentBaseUrlValue) {
    form.setValue('base_url', getDefaultBaseUrl(currentType))
  }
}
```

- [ ] **Step 3: Render provider-specific hints through the existing fields**

Use `currentChannelTypeHints.baseUrl` for the general Base URL description, `currentChannelTypeHints.key` for the normal API key description, and `currentChannelTypeHints.models` for the Models description, each falling back to the existing generic translated text. Preserve the existing edit and batch-mode descriptions because those describe active form behavior.

```tsx
{t(
  currentChannelTypeHints.models || FIELD_DESCRIPTIONS.MODELS
)}
```

The existing `CHANNEL_TYPE_WARNINGS[currentType]` alert will render the ARK `/api/v3` task-only warning.

- [ ] **Step 4: Re-run the focused test**

Run from `web/default/`:

```bash
bun test src/features/channels/lib/channel-type-config.test.ts
```

Expected: 3 tests pass.

### Task 4: Add all locale values through the sanctioned i18n script

**Files:**
- Create temporarily: `web/default/scripts/add-missing-keys.mjs`
- Modify through script: `web/default/src/i18n/locales/en.json`
- Modify through script: `web/default/src/i18n/locales/fr.json`
- Modify through script: `web/default/src/i18n/locales/ja.json`
- Modify through script: `web/default/src/i18n/locales/ru.json`
- Modify through script: `web/default/src/i18n/locales/vi.json`
- Modify through script: `web/default/src/i18n/locales/zh.json`
- Modify through script: `web/default/src/i18n/locales/zh-TW.json`
- Modify: `web/default/scripts/sync-i18n.mjs`

- [ ] **Step 1: Create the standard locale update script**

Use the `i18n-translate` skill's required `add-missing-keys.mjs` structure and populate these keys for all seven runtime locales:

```js
const newKeys = {
  en: {
    Dimensio: 'Dimensio',
    'Default: https://jimeng.dimensio.cn': 'Default: https://jimeng.dimensio.cn',
    'Enter the raw API key issued by Dimensio': 'Enter the raw API key issued by Dimensio',
    'Supported upstream models: jimeng-video-seedance-2.0-fast-vip, jimeng-video-seedance-2.0-mini, jimeng-video-seedance-2.0-vip': 'Supported upstream models: jimeng-video-seedance-2.0-fast-vip, jimeng-video-seedance-2.0-mini, jimeng-video-seedance-2.0-vip',
    'Dimensio is task-only. Call it through the ARK /api/v3 task API.': 'Dimensio is task-only. Call it through the ARK /api/v3 task API.',
  },
  zh: {
    Dimensio: 'Dimensio',
    'Default: https://jimeng.dimensio.cn': '默认：https://jimeng.dimensio.cn',
    'Enter the raw API key issued by Dimensio': '请输入 Dimensio 签发的原始 API Key',
    'Supported upstream models: jimeng-video-seedance-2.0-fast-vip, jimeng-video-seedance-2.0-mini, jimeng-video-seedance-2.0-vip': '支持的上游模型：jimeng-video-seedance-2.0-fast-vip、jimeng-video-seedance-2.0-mini、jimeng-video-seedance-2.0-vip',
    'Dimensio is task-only. Call it through the ARK /api/v3 task API.': 'Dimensio 仅支持任务接口，请通过 ARK /api/v3 任务 API 调用。',
  },
  'zh-TW': {
    Dimensio: 'Dimensio',
    'Default: https://jimeng.dimensio.cn': '預設：https://jimeng.dimensio.cn',
    'Enter the raw API key issued by Dimensio': '請輸入 Dimensio 簽發的原始 API Key',
    'Supported upstream models: jimeng-video-seedance-2.0-fast-vip, jimeng-video-seedance-2.0-mini, jimeng-video-seedance-2.0-vip': '支援的上游模型：jimeng-video-seedance-2.0-fast-vip、jimeng-video-seedance-2.0-mini、jimeng-video-seedance-2.0-vip',
    'Dimensio is task-only. Call it through the ARK /api/v3 task API.': 'Dimensio 僅支援任務介面，請透過 ARK /api/v3 任務 API 呼叫。',
  },
  fr: {
    Dimensio: 'Dimensio',
    'Default: https://jimeng.dimensio.cn': 'Par défaut : https://jimeng.dimensio.cn',
    'Enter the raw API key issued by Dimensio': 'Saisissez la clé API brute fournie par Dimensio',
    'Supported upstream models: jimeng-video-seedance-2.0-fast-vip, jimeng-video-seedance-2.0-mini, jimeng-video-seedance-2.0-vip': 'Modèles en amont pris en charge : jimeng-video-seedance-2.0-fast-vip, jimeng-video-seedance-2.0-mini, jimeng-video-seedance-2.0-vip',
    'Dimensio is task-only. Call it through the ARK /api/v3 task API.': "Dimensio prend uniquement en charge les tâches. Appelez-le via l'API de tâches ARK /api/v3.",
  },
  ja: {
    Dimensio: 'Dimensio',
    'Default: https://jimeng.dimensio.cn': 'デフォルト：https://jimeng.dimensio.cn',
    'Enter the raw API key issued by Dimensio': 'Dimensio が発行した API キーをそのまま入力してください',
    'Supported upstream models: jimeng-video-seedance-2.0-fast-vip, jimeng-video-seedance-2.0-mini, jimeng-video-seedance-2.0-vip': '対応アップストリームモデル：jimeng-video-seedance-2.0-fast-vip、jimeng-video-seedance-2.0-mini、jimeng-video-seedance-2.0-vip',
    'Dimensio is task-only. Call it through the ARK /api/v3 task API.': 'Dimensio はタスク専用です。ARK /api/v3 タスク API から呼び出してください。',
  },
  ru: {
    Dimensio: 'Dimensio',
    'Default: https://jimeng.dimensio.cn': 'По умолчанию: https://jimeng.dimensio.cn',
    'Enter the raw API key issued by Dimensio': 'Введите исходный API-ключ, выданный Dimensio',
    'Supported upstream models: jimeng-video-seedance-2.0-fast-vip, jimeng-video-seedance-2.0-mini, jimeng-video-seedance-2.0-vip': 'Поддерживаемые модели вышестоящего сервиса: jimeng-video-seedance-2.0-fast-vip, jimeng-video-seedance-2.0-mini, jimeng-video-seedance-2.0-vip',
    'Dimensio is task-only. Call it through the ARK /api/v3 task API.': 'Dimensio поддерживает только задачи. Вызывайте его через API задач ARK /api/v3.',
  },
  vi: {
    Dimensio: 'Dimensio',
    'Default: https://jimeng.dimensio.cn': 'Mặc định: https://jimeng.dimensio.cn',
    'Enter the raw API key issued by Dimensio': 'Nhập API key gốc do Dimensio cấp',
    'Supported upstream models: jimeng-video-seedance-2.0-fast-vip, jimeng-video-seedance-2.0-mini, jimeng-video-seedance-2.0-vip': 'Các mô hình thượng nguồn được hỗ trợ: jimeng-video-seedance-2.0-fast-vip, jimeng-video-seedance-2.0-mini, jimeng-video-seedance-2.0-vip',
    'Dimensio is task-only. Call it through the ARK /api/v3 task API.': 'Dimensio chỉ hỗ trợ tác vụ. Hãy gọi qua API tác vụ ARK /api/v3.',
  },
}
```

- [ ] **Step 2: Register the provider name as a literal brand**

Add `'Dimensio'` to `BRAND_AND_LITERAL_KEYS` in `web/default/scripts/sync-i18n.mjs` so the intentional brand spelling is not reported as untranslated.

- [ ] **Step 3: Apply and verify locales, then remove the temporary writer**

Run from `web/default/`:

```bash
node scripts/add-missing-keys.mjs
bun run i18n:sync
```

Expected: five new keys are present in every runtime locale, JSON remains valid and sorted, and the sync report records zero missing keys. Remove `scripts/add-missing-keys.mjs` after it has applied the values.

- [ ] **Step 4: Commit the drawer and translations**

```bash
git add web/default/src/features/channels/components/drawers/channel-mutate-drawer.tsx web/default/scripts/sync-i18n.mjs web/default/src/i18n/locales/en.json web/default/src/i18n/locales/fr.json web/default/src/i18n/locales/ja.json web/default/src/i18n/locales/ru.json web/default/src/i18n/locales/vi.json web/default/src/i18n/locales/zh.json web/default/src/i18n/locales/zh-TW.json
git commit -m "feat(channels): configure Dimensio form guidance"
```

### Task 5: Run frontend quality gates

**Files:**
- Verify all files changed in Tasks 1-4

- [ ] **Step 1: Run focused tests**

```bash
cd web/default
bun test src/features/channels/lib/channel-type-config.test.ts
```

Expected: 3 tests pass and 0 fail.

- [ ] **Step 2: Run TypeScript validation**

```bash
bun run typecheck
```

Expected: exit code 0.

- [ ] **Step 3: Lint the changed TypeScript files**

```bash
bunx oxlint -c .oxlintrc.json src/features/channels/constants.ts src/features/channels/lib/channel-type-config.ts src/features/channels/lib/channel-utils.ts src/features/channels/lib/channel-type-config.test.ts src/features/channels/components/drawers/channel-mutate-drawer.tsx
```

Expected: 0 errors.

- [ ] **Step 4: Verify formatting and i18n synchronization**

```bash
bun run format:check
bun run i18n:sync
git diff --check
```

Expected: all commands exit 0 and the final sync produces no unexpected locale changes.

- [ ] **Step 5: Build the production frontend**

```bash
bun run build
```

Expected: Rsbuild exits 0 and refreshes `web/default/dist` without tracked source errors.

### Task 6: Rebuild the local image and run browser acceptance

**Files:**
- Create: `output/playwright/dimensio-channel-option.png`
- Create: `docs/superpowers/reports/2026-07-20-dimensio-channel-ui-acceptance-report.md`

- [ ] **Step 1: Rebuild and restart the Compose application**

Use the repository's accepted local Compose command and existing environment from the prior local-stack acceptance. Verify `docker compose ps` shows the application, MySQL, and Redis healthy/running and `http://127.0.0.1:3000` responds.

- [ ] **Step 2: Open the console with Playwright CLI**

Verify `npx` is available, then use the bundled wrapper:

```powershell
$env:PWCLI = "$env:USERPROFILE/.codex/skills/playwright/scripts/playwright_cli.sh"
bash $env:PWCLI open http://127.0.0.1:3000 --headed
bash $env:PWCLI snapshot
```

- [ ] **Step 3: Exercise the administrator flow**

Using fresh snapshot refs, sign in with the existing local administrator session or credentials, open Channels, choose Add Channel, open the Type selector, search for `Dimensio`, and select it. Verify:

- the option label is `Dimensio`;
- the Base URL becomes `https://jimeng.dimensio.cn`;
- the raw API key hint is visible;
- the three supported upstream models are shown;
- the ARK `/api/v3` task-only warning is visible;
- the normal models, model mapping, groups, priority, weight, auto-ban, and save controls remain available;
- no generic Fetch from Upstream action appears for type `59`.

Capture `output/playwright/dimensio-channel-option.png`.

- [ ] **Step 4: Write and commit the acceptance report**

Document the image ID/build command, Compose health, browser route, assertions, screenshot path, automated test results, and the explicit limitation that no real Dimensio key or upstream request was used.

```bash
git add docs/superpowers/reports/2026-07-20-dimensio-channel-ui-acceptance-report.md output/playwright/dimensio-channel-option.png
git commit -m "docs: record Dimensio channel UI acceptance"
```

- [ ] **Step 5: Perform final verification**

Run `git status --short --branch`, `git diff --check HEAD^`, the focused Bun test, and inspect the acceptance screenshot. Confirm unrelated pre-existing worktree files were neither staged nor removed.
