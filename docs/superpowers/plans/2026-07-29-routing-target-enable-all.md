# Routing Target Enable-All Switch Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an accessible bulk switch beside `Add target` that enables or disables every current routing target without changing policy-level enablement.

**Architecture:** Derive the bulk checked state from the React Hook Form `targets` array with `useWatch`. Apply changes through indexed `form.setValue` calls so field-array identity, other target values, focus, and validation state are preserved.

**Tech Stack:** React 19, TypeScript, React Hook Form, Base UI/shadcn Switch, i18next, Bun test runner, Happy DOM, TanStack Query.

---

### Task 1: Protect Bulk Target Behavior With A Component Test

**Files:**
- Create: `web/src/features/model-routing/components/__tests__/routing-policy-drawer-target-bulk-toggle.test.tsx`

- [ ] **Step 1: Create the drawer test fixture**

Create a Happy DOM fixture that renders the real `RoutingPolicyDrawer` inside `I18nextProvider` and `QueryClientProvider`. Seed `routingPolicyQueryKeys.groups()` and `routingPolicyQueryKeys.candidates(group, model)` with successful data, and pass an editing policy whose policy-level `enabled` value is `false` and whose two target `enabled` values are mixed.

```tsx
const queryClient = new QueryClient({
  defaultOptions: {
    queries: { retry: false, staleTime: Number.POSITIVE_INFINITY },
    mutations: { retry: false },
  },
})
queryClient.setQueryData(routingPolicyQueryKeys.groups(), {
  success: true,
  message: '',
  data: ['default'],
})
queryClient.setQueryData(
  routingPolicyQueryKeys.candidates(policy.group_name, policy.model),
  { success: true, message: '', data: candidates }
)
```

- [ ] **Step 2: Add focused user-visible assertions**

Add independent tests proving:

```tsx
test('mixed targets render the bulk switch off and bulk changes preserve policy enablement', async () => {
  const bulkSwitch = getSwitch('Enable all targets')
  const policySwitch = getSwitch('Enabled', 0)
  assert.equal(bulkSwitch.getAttribute('aria-checked'), 'false')
  assert.equal(policySwitch.getAttribute('aria-checked'), 'false')

  await act(async () => bulkSwitch.click())
  assert.deepEqual(getTargetSwitches().map(checkedState), [true, true])
  assert.equal(policySwitch.getAttribute('aria-checked'), 'false')

  await act(async () => bulkSwitch.click())
  assert.deepEqual(getTargetSwitches().map(checkedState), [false, false])
})

test('adding the default disabled target turns an all-enabled bulk switch off', async () => {
  await act(async () => findButton('Add target').click())
  assert.equal(getSwitch('Enable all targets').getAttribute('aria-checked'), 'false')
})

test('an empty target list disables the bulk switch', async () => {
  assert.equal(getSwitch('Enable all targets').hasAttribute('disabled'), true)
})
```

- [ ] **Step 3: Run the focused test and verify RED**

Run: `cd web; bun test --parallel=1 src/features/model-routing/components/__tests__/routing-policy-drawer-target-bulk-toggle.test.tsx`

Expected: FAIL because no switch with accessible name `Enable all targets` exists.

### Task 2: Implement The Bulk Target Switch

**Files:**
- Modify: `web/src/features/model-routing/components/routing-policy-drawer.tsx`
- Test: `web/src/features/model-routing/components/__tests__/routing-policy-drawer-target-bulk-toggle.test.tsx`

- [ ] **Step 1: Derive the checked state from watched target values**

Add the existing shared `Label` import and watch the target array:

```tsx
import { Label } from '@/components/ui/label'

const targetValues = useWatch({ control: form.control, name: 'targets' })
const allTargetsEnabled =
  targetValues.length > 0 && targetValues.every((target) => target.enabled)
```

- [ ] **Step 2: Render the switch immediately before Add target**

Wrap the header actions and update each field individually:

```tsx
<div className='flex shrink-0 flex-wrap items-center justify-end gap-3'>
  <div className='flex items-center gap-2'>
    <Label htmlFor='routing-targets-enable-all' className='text-sm'>
      {t('Enable all targets')}
    </Label>
    <Switch
      id='routing-targets-enable-all'
      checked={allTargetsEnabled}
      disabled={targetValues.length === 0}
      onCheckedChange={(checked) => {
        targetValues.forEach((_, index) => {
          form.setValue(`targets.${index}.enabled`, checked, {
            shouldDirty: true,
            shouldValidate: true,
          })
        })
      }}
    />
  </div>
  <Button
    type='button'
    variant='outline'
    size='sm'
    onClick={() => targets.append(createEmptyTarget())}
  >
    <Plus data-icon='inline-start' aria-hidden='true' />
    {t('Add target')}
  </Button>
</div>
```

- [ ] **Step 3: Run the focused test and verify GREEN**

Run: `cd web; bun test --parallel=1 src/features/model-routing/components/__tests__/routing-policy-drawer-target-bulk-toggle.test.tsx`

Expected: all tests PASS with no warnings or errors.

### Task 2A: Present Routing Targets As A Collapsed Accordion

**Files:**
- Modify: `web/src/features/model-routing/components/routing-policy-drawer.tsx`
- Test: `web/src/features/model-routing/components/__tests__/routing-policy-drawer-target-bulk-toggle.test.tsx`

- [ ] **Step 1: Add the existing Accordion primitives around each target editor**

Import `Accordion`, `AccordionContent`, `AccordionItem`, and `AccordionTrigger` from `@/components/ui/accordion`. Render one item per `targets.fields` entry with the field-array id as its value, `multiple` on the root, and no `defaultValue` so every item starts closed:

```tsx
<Accordion multiple className='rounded-md border'>
  {targets.fields.map((target, index) => (
    <AccordionItem key={target.id} value={target.id}>
      <AccordionTrigger className='px-3 sm:px-4'>
        {form.watch(`targets.${index}.name`) ||
          `${t('Routing target')} ${index + 1}`}
      </AccordionTrigger>
      <AccordionContent className='px-3 sm:px-4'>
        <RouteTargetEditor ... />
      </AccordionContent>
    </AccordionItem>
  ))}
</Accordion>
```

- [ ] **Step 2: Verify the heading interaction**

Run: `cd web; bun test --parallel=1 src/features/model-routing/components/__tests__/routing-policy-drawer-target-bulk-toggle.test.tsx --test-name-pattern='start collapsed'`

Expected: the new test passes with two headings reporting `aria-expanded="false"`, then the first heading reporting `aria-expanded="true"` and its editor content visible.

- [ ] **Step 3: Run the full model-routing component tests**

Run: `cd web; bun test --parallel=1 src/features/model-routing/components`

Expected: the bulk-switch tests and existing target editor tests all pass.

### Task 3: Add All Locale Translations Through The Required Script

**Files:**
- Create temporarily: `web/scripts/add-missing-keys.mjs`
- Modify via script: `web/src/i18n/locales/en.json`
- Modify via script: `web/src/i18n/locales/zh.json`
- Modify via script: `web/src/i18n/locales/zh-TW.json`
- Modify via script: `web/src/i18n/locales/fr.json`
- Modify via script: `web/src/i18n/locales/ja.json`
- Modify via script: `web/src/i18n/locales/ru.json`
- Modify via script: `web/src/i18n/locales/vi.json`
- Delete after use: `web/scripts/add-missing-keys.mjs`

- [ ] **Step 1: Create the sanctioned locale update script**

Use the `i18n-translate` skill's required script structure with this `newKeys` payload:

```js
const newKeys = {
  en: { 'Enable all targets': 'Enable all targets' },
  zh: { 'Enable all targets': '全部启用' },
  'zh-TW': { 'Enable all targets': '全部啟用' },
  fr: { 'Enable all targets': 'Activer toutes les cibles' },
  ja: { 'Enable all targets': 'すべてのターゲットを有効化' },
  ru: { 'Enable all targets': 'Включить все цели' },
  vi: { 'Enable all targets': 'Bật tất cả đích' },
}
```

- [ ] **Step 2: Apply and normalize translations**

Run: `cd web; node scripts/add-missing-keys.mjs; bun run i18n:sync`

Expected: the key is present in all seven locales and the sync command exits 0.

- [ ] **Step 3: Remove the temporary script**

Delete `web/scripts/add-missing-keys.mjs` with `apply_patch` and verify only the seven locale files contain the new key.

### Task 4: Verify The Frontend Change

**Files:**
- Verify: `web/src/features/model-routing/components/routing-policy-drawer.tsx`
- Verify: `web/src/features/model-routing/components/__tests__/routing-policy-drawer-target-bulk-toggle.test.tsx`
- Verify: `web/src/i18n/locales/{en,zh,zh-TW,fr,ja,ru,vi}.json`

- [ ] **Step 1: Run affected component tests**

Run: `cd web; bun test --parallel=1 src/features/model-routing/components`

Expected: all model-routing component tests PASS.

- [ ] **Step 2: Run static verification**

Run: `cd web; bun run typecheck`

Run: `cd web; bunx oxlint -c .oxlintrc.json src/features/model-routing/components/routing-policy-drawer.tsx src/features/model-routing/components/__tests__/routing-policy-drawer-target-bulk-toggle.test.tsx`

Run: `cd web; bunx oxfmt --check src/features/model-routing/components/routing-policy-drawer.tsx src/features/model-routing/components/__tests__/routing-policy-drawer-target-bulk-toggle.test.tsx`

Expected: every command exits 0.

- [ ] **Step 3: Run the production build**

Run: `cd web; bun run build`

Expected: Rsbuild completes successfully.

### Task 5: Rebuild And Run Real Browser Acceptance

**Files:**
- Verify: `docker-compose.local.yml`

- [ ] **Step 1: Rebuild the local service container**

Run: `docker compose -f docker-compose.local.yml up -d --build new-api`

Expected: image build exits 0 and service is running.

- [ ] **Step 2: Verify the container and HTTP endpoint**

Run: `docker compose -f docker-compose.local.yml ps`

Run: `docker inspect --format='{{json .State.Health}}' new-api-local`

Expected: the service reports running/healthy and `http://localhost:3000/models/routing` responds.

- [ ] **Step 3: Run headed browser acceptance**

Open `http://localhost:3000/models/routing`, edit a policy with multiple targets, and verify the switch is immediately left of `Add target`. Verify mixed/off, enable-all, disable-all, empty-list disabled, newly added target default-off, policy-level state isolation, save/reopen persistence, keyboard operation, and no browser console errors.
