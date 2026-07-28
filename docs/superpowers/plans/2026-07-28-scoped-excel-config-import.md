# Scoped Excel Configuration Import Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Convert a supported channel-cost workbook in the browser, select channel groups or individual lines, then export or create a batch from only that dependency-closed selection.

**Architecture:** Extract the current browser-only conversion function, add a pure scoped-document builder, and render one controlled selector in both surfaces. The builder derives scope from `line_ref`, filters mixed routes, follows every required dependency, recomputes changed hashes/counts/payload hash, and disables commands for scope-local blockers. The administrator page posts only that rebuilt JSON to the existing batch-create API and then continues through the existing binding/review workflow.

**Tech Stack:** React 19, TypeScript, Base UI, Tailwind CSS, i18next, Bun test, Playwright, ExcelJS, existing configuration-import API.

---

## File Structure

- Create: `web/src/channel-config-converter/conversion.ts` - reusable local workbook conversion.
- Create: `web/src/channel-config-converter/scope.ts` - group model, scoped closure, hash/count rebuild, validation result.
- Create: `web/src/channel-config-converter/components/channel-line-scope-selector.tsx` - shared accessible UI.
- Create: `web/src/channel-config-converter/__tests__/conversion.test.ts` and `web/src/channel-config-converter/__tests__/scope.test.ts` - pure logic contracts.
- Create: `web/src/channel-config-converter/components/__tests__/channel-line-scope-selector.test.tsx` - tri-state and keyboard contracts.
- Create: `web/src/features/config-import/components/excel-import-step.tsx` - local Excel source and direct batch action.
- Create: `web/src/features/config-import/components/import-source-step.tsx` - Excel/JSON source switch.
- Create: `web/src/features/config-import/components/__tests__/excel-import-step.test.tsx` - selected-upload and retry tests.
- Modify: `web/src/channel-config-converter/app.tsx`, `web/src/channel-config-converter/components/download-actions.tsx`, `web/src/channel-config-converter/__tests__/app.test.tsx` - selected standalone export.
- Modify: `web/rsbuild.converter.config.ts`, `web/src/channel-config-converter/main.tsx` - shared offline styling.
- Modify: `web/src/features/config-import/index.tsx`, `web/src/features/config-import/components/__tests__/config-import-wizard.test.tsx` - source mode before a batch exists.
- Modify: `web/e2e/channel-config-converter.pw.ts` - partial selected-download browser test.
- Modify only through the locale script: `web/src/i18n/locales/{en,zh,zh-TW,fr,ja,ru,vi}.json`.

This plan has no Go, database, migration, credential, channel-enable, route-enable, or publish change.

### Task 1: Extract Local Workbook Conversion

**Files:**
- Create: `web/src/channel-config-converter/conversion.ts`
- Create: `web/src/channel-config-converter/__tests__/conversion.test.ts`
- Modify: `web/src/channel-config-converter/app.tsx:20-67`

- [ ] **Step 1: Write failing conversion tests**

```ts
test('converts the V1 fixture through the reusable local boundary', async () => {
  const bytes = await fs.readFile(fixturePath)
  const result = await convertWorkbook(new File([bytes], 'fixture.xlsx'))
  assert.equal(result.document.kind, 'new-api.channel-config-import')
  assert.equal(result.document.template_version, '1')
  assert.equal(result.document.entities.channel_lines.length, 12)
})

test('rejects invalid local workbook data before producing a document', async () => {
  await assert.rejects(
    () => convertWorkbook(new File(['not xlsx'], 'invalid.xlsx')),
    (error: unknown) => error instanceof WorkbookPreflightError
  )
})
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `bun test src/channel-config-converter/__tests__/conversion.test.ts`

Expected: module `../conversion` cannot be resolved.

- [ ] **Step 3: Implement the shared conversion boundary**

```ts
export type WorkbookConversion = ImportDocumentResult

export async function convertWorkbook(file: File): Promise<WorkbookConversion> {
  await preflightWorkbook(file)
  const sourceBytes = new Uint8Array(await file.arrayBuffer())
  const snapshot = await loadWorkbookSnapshot(sourceBytes)
  const adapter = [new V2WorkbookAdapter(), new V1WorkbookAdapter()].find(
    (candidate) => candidate.matches(snapshot).matched
  )
  if (!adapter) {
    throw new WorkbookContractError('UNSUPPORTED_TEMPLATE', 'No supported workbook template matched.')
  }
  return buildImportDocument({
    extracted: adapter.extract(snapshot),
    sourceBytes,
    sourceFileName: file.name,
  })
}
```

Move the existing private converter body unchanged into this module; `app.tsx` keeps translated local error handling. Re-export the type from `app.tsx` while existing tests still import it.

- [ ] **Step 4: Verify and commit**

Run: `bun test src/channel-config-converter/__tests__/conversion.test.ts src/channel-config-converter/__tests__/document.test.ts`

Expected: pass, with the canonical document unchanged.

```bash
git add web/src/channel-config-converter/conversion.ts web/src/channel-config-converter/__tests__/conversion.test.ts web/src/channel-config-converter/app.tsx
git commit -m "refactor: share workbook conversion boundary"
```

### Task 2: Build The Scoped Dependency Closure

**Files:**
- Create: `web/src/channel-config-converter/scope.ts`
- Create: `web/src/channel-config-converter/__tests__/scope.test.ts`

- [ ] **Step 1: Write failing V1/V2 scope tests**

```ts
function entityCounts(
  document: ConfigImportDocument
): ConfigImportDocument['manifest']['counts'] {
  return Object.fromEntries(
    Object.entries(document.entities).map(([name, entities]) => [
      name,
      entities.length,
    ])
  ) as ConfigImportDocument['manifest']['counts']
}

test('selecting secure-enterprise retains only its configuration closure', async () => {
  const scoped = await buildScopedImportDocument(full.document, new Set(['secure-enterprise']))
  assert.equal(scoped.canUse, true)
  assert.deepEqual(scoped.document.entities.channel_lines.map((line) => line.line_ref), ['secure-enterprise'])
  assert.deepEqual(scoped.document.entities.channels.map((channel) => channel.business_id), ['CH-SECURE'])
  assert.ok(scoped.document.entities.cost_rule_drafts.every((draft) => draft.line_ref === 'secure-enterprise'))
  assert.ok(scoped.document.entities.model_mappings.every((mapping) => mapping.line_ref === 'secure-enterprise'))
  assert.ok(scoped.document.entities.route_blueprints.every((route) => route.targets.every((target) => target.line_ref === 'secure-enterprise')))
})

test('unselected MEGABYAI conflict neither appears nor blocks a selected secure line', async () => {
  const scoped = await buildScopedImportDocument(full.document, new Set(['secure-enterprise']))
  assert.equal(scoped.document.entities.unresolved_variants.length, 0)
  assert.equal(scoped.document.issues.some((issue) => issue.code === 'COST_VARIANT_AMBIGUOUS'), false)
  assert.deepEqual(scoped.blockingIssues, [])
})

test('rebuilds route hashes, manifest counts, and payload hash deterministically', async () => {
  const once = await buildScopedImportDocument(full.document, new Set(['secure-enterprise']))
  const twice = await buildScopedImportDocument(full.document, new Set(['secure-enterprise']))
  assert.deepEqual(once.document.manifest.counts, entityCounts(once.document))
  assert.match(once.document.manifest.payload_sha256, /^[a-f0-9]{64}$/)
  assert.equal(once.document.manifest.payload_sha256, twice.document.manifest.payload_sha256)
  assert.deepEqual(once.validationErrors, [])
})

test('returns local errors for empty and unknown selections', async () => {
  assert.deepEqual((await buildScopedImportDocument(full.document, new Set())).validationErrors, ['EMPTY_SELECTION'])
  assert.deepEqual((await buildScopedImportDocument(full.document, new Set(['missing-line']))).validationErrors, ['UNKNOWN_LINE_REF'])
})
```

Use helper conversions for both fixture templates. The V2 test selects one real line and asserts that all retained line refs in lines, costs, mappings, and route targets belong to the selected set.

- [ ] **Step 2: Run the scope test to verify it fails**

Run: `bun test src/channel-config-converter/__tests__/scope.test.ts`

Expected: module `../scope` cannot be resolved.

- [ ] **Step 3: Implement the exact scope contract**

```ts
export type ChannelLineGroup = {
  channel: ConfigImportDocument['entities']['channels'][number]
  lines: ConfigImportDocument['entities']['channel_lines']
}

export type ScopeValidationError = 'EMPTY_SELECTION' | 'UNKNOWN_LINE_REF' | 'DANGLING_REFERENCE' | 'EMPTY_ROUTE_BLUEPRINT'

export type ScopedImportDocumentResult = {
  blockingIssues: ConfigImportDocument['issues']
  canUse: boolean
  document: ConfigImportDocument
  groups: ChannelLineGroup[]
  selectedGroupCount: number
  selectedLineCount: number
  validationErrors: ScopeValidationError[]
  warnings: ConfigImportDocument['issues']
}

export function groupChannelLines(document: ConfigImportDocument): ChannelLineGroup[]
export async function buildScopedImportDocument(document: ConfigImportDocument, selectedLineRefs: ReadonlySet<string>): Promise<ScopedImportDocumentResult>
```

Implement `buildScopedImportDocument` in this order:

1. Reject empty and unknown line selections before an exportable result is created.
2. Copy selected `channel_lines` and their parent `channels` by `channel_ref`.
3. Copy cost drafts and mappings whose `line_ref` is selected. Filter every `route_blueprints[].targets` to selected lines and filter `model_mapping_refs` to mappings belonging to selected lines. Drop a route only when no target remains. Sort the filtered target and mapping arrays, then recompute that route's `entity_hash` with `hashEntity`.
4. Copy all referenced `model_skus`, then `sale_proposals` whose `model_sku_ref` is retained.
5. Follow each retained `source_ref` recursively, retaining only actual `sources` records until all required source refs resolve.
6. Retain an unresolved variant only if its non-empty `line_ref` is selected. Retain original issues only if their `entity_ref` is a selected line, a selected-line cost/mapping, or a retained non-empty-line variant. Exclude blank-line/unattributable issues and old mixed-route issues; validate rebuilt routes below.
7. Copy every retained object deeply; sort each collection by `business_id`; clone manifest metadata; recalculate all nine counts and `payload_sha256` using `hashPayload`.
8. Validate source refs, line-to-channel refs, sale-to-SKU refs, mapping-to-line/SKU refs, cost line/route-target refs, and every route mapping/target ref. Return `DANGLING_REFERENCE` for broken links and `EMPTY_ROUTE_BLUEPRINT` for an invalid retained empty route.
9. `blockingIssues` contains retained `error` issues, `warnings` contains retained `warning` issues, and `canUse` is true only with a non-empty selection and no validation/blocking issues.

Never mutate the source document, its manifest, entity arrays, nested targets, or caller-provided `Set`. Reuse `hashEntity` and `hashPayload`, never a second hash implementation.

- [ ] **Step 4: Verify and commit**

Run: `bun test src/channel-config-converter/__tests__/scope.test.ts src/channel-config-converter/__tests__/document.test.ts src/channel-config-converter/__tests__/hash.test.ts`

Expected: pass for V1/V2, no dangling references, no source-document mutation.

```bash
git add web/src/channel-config-converter/scope.ts web/src/channel-config-converter/__tests__/scope.test.ts
git commit -m "feat: build scoped channel import documents"
```

### Task 3: Build The Shared Channel Group/Line Selector

**Files:**
- Create: `web/src/channel-config-converter/components/channel-line-scope-selector.tsx`
- Create: `web/src/channel-config-converter/components/__tests__/channel-line-scope-selector.test.tsx`
- Modify: `web/rsbuild.converter.config.ts`
- Modify: `web/src/channel-config-converter/main.tsx`

- [ ] **Step 1: Write failing selector behavior tests**

```tsx
test('group selection becomes indeterminate after one child is cleared', async () => {
  const mounted = await mount(<ChannelLineScopeSelector {...props} />)
  await clickByRole(mounted.container, 'checkbox', 'Select all lines in Secure')
  assert.deepEqual(lastSelection, new Set(['secure-discount', 'secure-enterprise', 'secure-overseas']))
  await clickByRole(mounted.container, 'checkbox', 'secure-enterprise')
  assert.equal(groupCheckbox(mounted.container, 'Select all lines in Secure').getAttribute('aria-checked'), 'mixed')
})

test('global select, clear, search, and Space-key selection update the accessible summary', async () => {
  // Select all, clear, filter to enterprise, focus its checkbox, press Space,
  // and assert selected group/line counters after every action.
})
```

- [ ] **Step 2: Run the selector test to verify it fails**

Run: `bun test src/channel-config-converter/components/__tests__/channel-line-scope-selector.test.tsx`

Expected: module `../channel-line-scope-selector` cannot be resolved.

- [ ] **Step 3: Implement the controlled selector**

```tsx
export interface ChannelLineScopeSelectorProps {
  disabled?: boolean
  groups: ChannelLineGroup[]
  onSelectionChange: (lineRefs: Set<string>) => void
  selectedLineRefs: ReadonlySet<string>
  summary: Pick<ScopedImportDocumentResult, 'blockingIssues' | 'document' | 'selectedGroupCount' | 'selectedLineCount' | 'validationErrors' | 'warnings'>
}
```

Use project `Checkbox`, `Input`, and `Button`. Global actions are `Select all channel lines` and `Clear channel line selection`; filtering is component state only. A group is checked when all children are selected and gets Base UI's `indeterminate` property when partly selected. Every action constructs a new `Set`. Group checkboxes use `t('Select all lines in {{group}}', { group: group.channel.display_name })`; child checkboxes use the line display name. Render a responsive definition-list summary for selected groups, lines, costs, mappings, route targets, SKUs, sale proposals, warnings, and errors. Do not persist UI state or use nested cards.

Add `pluginTailwindcss({ optimize: false })` beside `pluginReact()` in `web/rsbuild.converter.config.ts` and `import '../styles/index.css'` in `web/src/channel-config-converter/main.tsx`, so the shared Base UI component is styled in the static offline converter without external assets.

- [ ] **Step 4: Verify and commit**

Run: `bun test src/channel-config-converter/components/__tests__/channel-line-scope-selector.test.tsx && bun run converter:build`

Expected: pass and create local static CSS/JS output.

```bash
git add web/src/channel-config-converter/components/channel-line-scope-selector.tsx web/src/channel-config-converter/components/__tests__/channel-line-scope-selector.test.tsx web/rsbuild.converter.config.ts web/src/channel-config-converter/main.tsx
git commit -m "feat: add channel line scope selector"
```

### Task 4: Scope The Static Converter Download

**Files:**
- Modify: `web/src/channel-config-converter/app.tsx`
- Modify: `web/src/channel-config-converter/components/download-actions.tsx`
- Modify: `web/src/channel-config-converter/__tests__/app.test.tsx`

- [ ] **Step 1: Write failing partial-export tests**

```tsx
test('downloads only selected channel lines and retained route targets', async () => {
  const download = mockObjectURLDownload()
  const mounted = await mount(twoLineConversionWithMixedRoute())
  await upload(mounted.container)
  await clickByRole(mounted.container, 'checkbox', 'line-one')
  await clickByRole(mounted.container, 'button', 'Export selected JSON')
  assert.deepEqual(download.document.entities.channel_lines.map((line) => line.line_ref), ['line-one'])
  assert.ok(download.document.entities.route_blueprints.every((route) => route.targets.every((target) => target.line_ref === 'line-one')))
})

test('unselected full-document errors do not disable a valid selected scope', async () => {
  // Make line-two carry the error, select line-one, and assert scoped export is enabled.
})
```

- [ ] **Step 2: Run the app test to verify it fails**

Run: `bun test src/channel-config-converter/__tests__/app.test.tsx`

Expected: existing code passes the entire conversion document to DownloadActions.

- [ ] **Step 3: Wire the scoped result into the static app**

Keep `selectedLineRefs` in component state and reset it after every accepted workbook and Clear action. Derive `buildScopedImportDocument(result.document, selectedLineRefs)` in a cancellation-safe effect because hashing is asynchronous. Add `Selection` after the existing Overview tab and render `ChannelLineScopeSelector`. Pass `scoped.document` into `DownloadActions`, rename the formal command `Export selected JSON`, and set `formalDownloadDisabled={!scoped.canUse}`. Keep all original full-document preview tabs untouched and keep issue-report download available from `scoped.document.issues`.

- [ ] **Step 4: Verify and commit**

Run: `bun test src/channel-config-converter/__tests__/app.test.tsx && bun run converter:build && bunx playwright test --config playwright.config-import.config.ts e2e/channel-config-converter.pw.ts --project=chromium-desktop`

Expected: pass; the standalone converter stays usable from `file:` with no network request.

```bash
git add web/src/channel-config-converter/app.tsx web/src/channel-config-converter/components/download-actions.tsx web/src/channel-config-converter/__tests__/app.test.tsx
git commit -m "feat: export selected channel configuration scope"
```

### Task 5: Add Excel As An Administrator Source Mode

**Files:**
- Create: `web/src/features/config-import/components/excel-import-step.tsx`
- Create: `web/src/features/config-import/components/import-source-step.tsx`
- Create: `web/src/features/config-import/components/__tests__/excel-import-step.test.tsx`
- Modify: `web/src/features/config-import/index.tsx`
- Modify: `web/src/features/config-import/components/__tests__/config-import-wizard.test.tsx`

- [ ] **Step 1: Write failing selected-upload and retry tests**

```tsx
test('uploads only the selected scope and enters returned binding batch', async () => {
  const uploaded: unknown[] = []
  const mounted = await mountExcelStep({
    convertFile: async () => twoLineConversionWithMixedRoute(),
    onUpload: async (document) => { uploaded.push(document); return bindingBatch() },
  })
  await uploadWorkbook(mounted.container)
  await clickByRole(mounted.container, 'checkbox', 'line-one')
  await clickByRole(mounted.container, 'button', 'Import selected configuration')
  assert.deepEqual(uploaded[0].entities.channel_lines.map((line) => line.line_ref), ['line-one'])
  assert.deepEqual(mounted.uploaded, [bindingBatch()])
})

test('retains converted selection for retry after create-batch failure', async () => {
  // Reject onUpload, assert alert and checked line, then assert retry command is enabled.
})

test('keeps existing JSON upload behavior in JSON import mode', async () => {
  // Upload .json via ImportUploadStep and assert its parsed object reaches onUpload once.
})
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `bun test src/features/config-import/components/__tests__/excel-import-step.test.tsx src/features/config-import/components/__tests__/config-import-wizard.test.tsx`

Expected: Excel components and source mode do not yet exist.

- [ ] **Step 3: Implement the local Excel source**

```tsx
export interface ExcelImportStepProps {
  disabled?: boolean
  convertFile?: (file: File) => Promise<WorkbookConversion>
  onUpload: (document: unknown) => Promise<ConfigImportBatchDetail>
  onUploaded: (batch: ConfigImportBatchDetail) => void
}
```

`ExcelImportStep` uses `convertWorkbook`, `groupChannelLines`, `buildScopedImportDocument`, and `ChannelLineScopeSelector`. It accepts `.xlsx` locally, clears the input after read, shows preflight/template errors locally, and never transmits source bytes. It exports selected JSON with the existing serializer and calls `onUpload(scoped.document)` only when `scoped.canUse`. Busy state protects duplicate requests. Failure leaves conversion/selection in memory, presents an alert, and re-enables retry. No raw JSON display, credential input, enable action, route enable, or publishing control is added.

```tsx
export interface ImportSourceStepProps {
  disabled?: boolean
  onUpload: (document: unknown) => Promise<ConfigImportBatchDetail>
  onUploaded: (batch: ConfigImportBatchDetail) => void
}
```

`ImportSourceStep` renders project `Tabs`: `Excel conversion` contains `ExcelImportStep`; `JSON import` renders the unmodified existing `ImportUploadStep`.

- [ ] **Step 4: Integrate with the existing wizard state transition**

```tsx
if (!batch) {
  return <ImportSourceStep disabled={isBusy} onUpload={uploadConfigImport} onUploaded={setBatch} />
}
```

Leave all later `batch` branches unchanged. A successful Excel response sets identical `ConfigImportBatchDetail` state to JSON, so `deriveWizardState` immediately enters current Channel bindings and disabled-by-default safeguards remain active.

- [ ] **Step 5: Verify and commit**

Run: `bun test src/features/config-import/components/__tests__/excel-import-step.test.tsx src/features/config-import/components/__tests__/import-upload-step.test.tsx src/features/config-import/components/__tests__/config-import-wizard.test.tsx`

Expected: pass; only selected JSON reaches the existing API callback, retry retains state, JSON stays unchanged, and returned batch renders binding.

```bash
git add web/src/features/config-import/components/excel-import-step.tsx web/src/features/config-import/components/import-source-step.tsx web/src/features/config-import/components/__tests__/excel-import-step.test.tsx web/src/features/config-import/index.tsx web/src/features/config-import/components/__tests__/config-import-wizard.test.tsx
git commit -m "feat: import selected Excel channel configuration"
```

### Task 6: Add Every Visible String Through The Locale Script

**Files:**
- Create then delete: `web/scripts/add-missing-keys.mjs`
- Modify only through that script: `web/src/i18n/locales/{en,zh,zh-TW,fr,ja,ru,vi}.json`

- [ ] **Step 1: Create and run the sanctioned locale writer**

Run: `bun run i18n:sync`

Use the full `add-missing-keys.mjs` structure from `i18n-translate` and add all seven locale values for these exact literal keys: `Clear channel line selection`, `Conversion runs in this browser. The workbook is not uploaded.`, `Excel conversion`, `Export selected JSON`, `Import selected configuration`, `JSON import`, `No channel lines are selected.`, `Scoped errors`, `Scoped warnings`, `Search channel groups and lines`, `Select all channel lines`, `Select all lines in {{group}}`, `Select channel lines`, `Selected groups`, `Selected lines`, `Selected scope`, `The Excel file could not be converted.`, `The selected scope has blocking errors.`, and `The selected scope is empty.` Use English values for `en`, concise natural UI translations for `zh`, `zh-TW`, `fr`, `ja`, `ru`, and `vi`, and preserve `{{group}}` in every locale.

- [ ] **Step 2: Apply, sync, scan, and remove the temporary writer**

Run: `node scripts/add-missing-keys.mjs && bun run i18n:sync && node scripts/find-missing-keys.mjs`

Expected: all seven locale files gain the same nineteen sorted keys and output includes `All t() keys found in en.json!`.

Delete only `web/scripts/add-missing-keys.mjs` after successful application. Never edit locale JSON with an editor or patch.

- [ ] **Step 3: Commit translations**

```bash
git add web/src/i18n/locales/en.json web/src/i18n/locales/zh.json web/src/i18n/locales/zh-TW.json web/src/i18n/locales/fr.json web/src/i18n/locales/ja.json web/src/i18n/locales/ru.json web/src/i18n/locales/vi.json
git commit -m "feat: translate scoped import workflow"
```

### Task 7: Browser Acceptance And Final Verification

**Files:**
- Modify: `web/e2e/channel-config-converter.pw.ts`
- Modify only in response to test failures: files from Tasks 1-6

- [ ] **Step 1: Write the failing offline browser acceptance test**

```ts
test('exports a selected secure line locally without persistence or network', async ({ page }) => {
  await page.getByRole('tab', { name: 'Selection' }).click()
  await page.getByRole('checkbox', { name: 'secure-enterprise' }).check()
  await expect(page.getByText('Selected lines')).toContainText('1')
  const downloadPromise = page.waitForEvent('download')
  await page.getByRole('button', { name: 'Export selected JSON' }).click()
  const download = await downloadPromise
  const document = JSON.parse(await readFile(await download.path() as string, 'utf8'))
  expect(document.entities.channel_lines.map((line: { line_ref: string }) => line.line_ref)).toEqual(['secure-enterprise'])
  expect(document.entities.route_blueprints.every((route: { targets: Array<{ line_ref: string }> }) => route.targets.every((target) => target.line_ref === 'secure-enterprise'))).toBeTruthy()
  await page.reload()
  await expect(page.getByRole('tab', { name: 'Selection' })).toHaveCount(0)
})
```

Keep the current request listener and `localStorage`/IndexedDB assertions in this suite.

- [ ] **Step 2: Run it to verify failure before UI wiring**

Run: `bun run converter:build && bunx playwright test --config playwright.config-import.config.ts e2e/channel-config-converter.pw.ts --project=chromium-desktop`

Expected: failure until Selection and selected export are implemented.

- [ ] **Step 3: Run all affected verification commands**

```bash
bun test src/channel-config-converter
bun test src/features/config-import/components/__tests__/excel-import-step.test.tsx src/features/config-import/components/__tests__/import-upload-step.test.tsx src/features/config-import/components/__tests__/config-import-wizard.test.tsx
bun run i18n:sync
bun run typecheck
bun run lint -- src/channel-config-converter src/features/config-import
bun run format:check
bun run converter:build
bunx playwright test --config playwright.config-import.config.ts e2e/channel-config-converter.pw.ts --project=chromium-desktop
```

Expected: all exit 0. The component tests are the direct administrator workflow contract because the existing browser suite intentionally has no privileged administrator fixture; its authenticated-route guard remains unchanged.

- [ ] **Step 4: Inspect and commit final browser test work**

```bash
git status --short
git diff --check
git add web/e2e/channel-config-converter.pw.ts
git commit -m "test: cover scoped Excel configuration import"
```

Do not stage or remove unrelated `web/test-results/` or `.superpowers/` content.

## Plan Self-Review

- Spec coverage: Tasks 1-2 provide local conversion, scope authority, closure, conflict isolation, hashes/counts, and structural reference safety. Tasks 3-4 provide grouped partial selection, summary, selected export, accessibility, and no persistence. Task 5 reuses only the existing batch-create API and existing binding flow. Task 6 covers all seven locales. Task 7 runs unit, component, browser, type, lint, format, and build verification.
- Intentional exclusions: no raw workbook backend upload/storage, no persistent client selection, no automatic credentials, no channel/route enablement, and no publish action.
- Type consistency: both surfaces use `WorkbookConversion`, `ChannelLineGroup`, `ScopedImportDocumentResult`, `convertWorkbook`, `groupChannelLines`, and `buildScopedImportDocument`; submission remains `uploadConfigImport(document)`.
- Placeholder scan: no deferred implementation marker is present; each code task has a failing contract, an implementation contract, an exact verification command, and a scoped commit.
