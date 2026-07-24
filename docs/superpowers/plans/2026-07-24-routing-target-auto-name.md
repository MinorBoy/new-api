# Routing Target Automatic Name Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Automatically name newly added routing targets from the local date, selected channel, resolution, canonical model speed, and duration while preserving manual names.

**Architecture:** Keep naming as pure, unit-tested functions in the existing model-routing types module. The target editor observes its current form state and uses a local ref to update only values that are empty or still equal to the previous generated value; existing and copied targets remain unchanged unless the operator clears the name.

**Tech Stack:** React 19, TypeScript, React Hook Form, Bun test, i18next.

---

### Task 1: Define the naming contract with failing tests

**Files:**
- Modify: `web/default/tests/model-routing-types.test.ts`
- Modify: `web/default/src/features/model-routing/types.ts`

- [ ] **Step 1: Add tests for canonical speed and formatted names**

Import `buildRoutingTargetName` and `shouldUpdateRoutingTargetName` from `types.ts`. Add deterministic tests using `new Date(2026, 6, 24)` that assert:

```ts
expect(
  buildRoutingTargetName({
    date: new Date(2026, 6, 24),
    channelName: 'A1',
    model: 'doubao-seedance-2-0-fast-260128',
    outputResolutions: ['720p', '1080p'],
    durations: { mode: 'range', values: [], min: 4, max: 15 },
  })
).toBe('20260724-A1-720p+1080p-fast-4-15s')
```

Cover standard and mini models, discrete durations (`5+10+15s`), empty channel (`undefined`), and the manual-name predicate (`''` and the previous generated value update; any other value does not).

- [ ] **Step 2: Run the focused test and verify the expected RED failure**

Run from `web/default/`:

```text
bun test tests/model-routing-types.test.ts
```

Expected: the new tests fail because the two exports do not exist yet.

### Task 2: Implement deterministic name helpers

**Files:**
- Modify: `web/default/src/features/model-routing/types.ts`

- [ ] **Step 1: Implement the minimal helpers**

Add these exported functions near the existing form constructors:

```ts
export function buildRoutingTargetName(input: {
  date: Date
  channelName: string
  model: (typeof CANONICAL_SEEDANCE_MODELS)[number]
  outputResolutions: string[]
  durations: RouteTargetFormValues['durations']
}): string | undefined

export function shouldUpdateRoutingTargetName(
  currentName: string,
  previousGeneratedName: string | undefined
): boolean
```

Format the date with local `getFullYear/getMonth/getDate`, derive speed from `-fast-` and `-mini-`, preserve the resolution array order, sort and deduplicate discrete durations, and return `undefined` when `channelName.trim()` is empty. Keep the helper independent of React and do not parse `upstream_model`.

- [ ] **Step 2: Run the focused tests and verify GREEN**

```text
bun test tests/model-routing-types.test.ts
```

Expected: all model-routing type tests pass.

### Task 3: Wire automatic naming into the target editor

**Files:**
- Modify: `web/default/src/features/model-routing/components/route-target-editor.tsx`
- Modify: `web/default/src/features/model-routing/components/route-target-editor-accessibility.test.tsx`

- [ ] **Step 1: Add the editor effect**

Import `useEffect`, `useRef`, `buildRoutingTargetName`, and `shouldUpdateRoutingTargetName`. Watch the policy model and target capability fields. Keep the last generated value in a ref. When the helper returns a name and the current field is empty or equals the ref, call `form.setValue` for `targets.${index}.name` without marking a manual dirty edit, then update the ref. Do not initialize the ref from a non-empty target name, so edit and copy flows are preserved.

- [ ] **Step 2: Extend the render contract test**

Keep the existing accessible-label assertions and add an assertion that the name input remains editable and is rendered with the generated-name field label. The pure helper tests remain the contract for exact formatting; the browser check covers the live effect.

- [ ] **Step 3: Run focused tests and typecheck**

```text
bun test src/features/model-routing/components/route-target-editor-accessibility.test.tsx tests/model-routing-types.test.ts
bun run typecheck
```

Expected: all tests pass and `tsgo -b` exits successfully.

### Task 4: Browser verification and finish

**Files:**
- No additional source files.
- Artifact: `output/playwright/routing-target-auto-name.png`

- [ ] **Step 1: Open the routing policy drawer in the local preview**

Open `http://127.0.0.1:3010/`, navigate to `/models/routing`, create a policy, add a target, select an available channel, and verify the name input is populated in the `YYYYMMDD-channel-resolution-speed-duration` format.

- [ ] **Step 2: Verify update and manual override behavior**

Change resolution or duration and verify the generated name follows the change. Type a custom name, change a capability again, and verify the custom value remains. Capture the drawer screenshot to `output/playwright/routing-target-auto-name.png`.

- [ ] **Step 3: Run the production checks**

```text
bunx oxfmt --check src/features/model-routing/components/route-target-editor.tsx src/features/model-routing/components/route-target-editor-accessibility.test.tsx src/features/model-routing/types.ts src/i18n/locales/en.json src/i18n/locales/fr.json src/i18n/locales/ja.json src/i18n/locales/ru.json src/i18n/locales/vi.json src/i18n/locales/zh.json src/i18n/locales/zh-TW.json
bunx oxlint -c .oxlintrc.json src/features/model-routing/components/route-target-editor.tsx src/features/model-routing/components/route-target-editor-accessibility.test.tsx src/features/model-routing/types.ts
bun run build
```

Expected: formatting, lint, and Rsbuild production build all exit with code 0.
