# Channel Model Template Generator Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a Bun command that converts `sd收录.xlsx` plus a versioned rules file into an audited V1 channel-model cost-and-profit workbook.

**Architecture:** The generator reads only the raw `channel`, `sd`, and `sd官价` sheets, normalizes them into typed records with original row locations, then applies explicit JSON rules to build V1 rows. A writer loads a supplied V1 base workbook, replaces only managed data rows, restores formula patterns, and emits an Excel file and a JSON report. The existing V1 browser converter remains the downstream verifier.

**Tech Stack:** Bun, TypeScript, ExcelJS, Node `crypto` and `fs`, existing `web/src/channel-config-converter` V1 contract and Bun test runner.

---

## File Structure

- Create: `web/scripts/channel-model-template/types.ts` — source, rule, V1 output, issue and report types.
- Create: `web/scripts/channel-model-template/rules.ts` — JSON-rule parsing, invariant checks and stable defaulting.
- Create: `web/scripts/channel-model-template/source.ts` — safe raw workbook load and exact source-sheet/header extraction.
- Create: `web/scripts/channel-model-template/build.ts` — deterministic source-to-V1 mapping and monetary calculations.
- Create: `web/scripts/channel-model-template/validate.ts` — entity/reference/range validation and report issue sorting.
- Create: `web/scripts/channel-model-template/write.ts` — V1 base loading, data replacement, formula fill, validation worksheet update and report serialization.
- Create: `web/scripts/channel-model-template/generate.ts` — command-line parsing, overwrite protection and orchestration.
- Create: `web/scripts/channel-model-template/conversion-rules.json` — current source-specific mapping, defaults and confirmed corrections.
- Create: `web/scripts/channel-model-template/__fixtures__/sd-source-v1.xlsx` — current raw workbook regression fixture.
- Create: `web/scripts/channel-model-template/__tests__/source.test.ts` — raw workbook schema and row-location tests.
- Create: `web/scripts/channel-model-template/__tests__/build.test.ts` — mapping, cost calculation, `draft`, stable-ID and issue tests.
- Create: `web/scripts/channel-model-template/__tests__/generate.test.ts` — command behavior, output workbook and existing V1-converter compatibility tests.
- Modify: `web/package.json` — expose a `channel-model-template:generate` Bun script.

`web/src/channel-config-converter/__fixtures__/channel-config-v1-corrected.xlsx` remains the V1 base regression fixture. The generator takes its path through `--base`; it is not re-created from source data.

### Task 1: Define the typed contract and rules loader

**Files:**
- Create: `web/scripts/channel-model-template/types.ts`
- Create: `web/scripts/channel-model-template/rules.ts`
- Create: `web/scripts/channel-model-template/__tests__/build.test.ts`

- [ ] **Step 1: Write the failing rules-validation test**

```ts
import assert from 'node:assert/strict'
import test from 'node:test'

import { parseRules } from '../rules'

test('rejects a channel mapping without a stable channel code', () => {
  assert.throws(
    () => parseRules({ version: '1', channelCodes: { '1': 'clmm' } }),
    /channelCodes.1 must match CH-/
  )
})

test('canonicalizes decimal defaults as strings', () => {
  const rules = parseRules({
    version: '1',
    channelCodes: { '1': 'CH-CLMM' },
    defaults: { currency: 'CNY', currencyToUsd: '0.136986301369863' },
  })

  assert.equal(rules.defaults.currencyToUsd, '0.136986301369863')
})
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `bun --cwd web test scripts/channel-model-template/__tests__/build.test.ts`

Expected: FAIL because `../rules` does not exist.

- [ ] **Step 3: Add the minimal contract and parser**

Create `types.ts` with the exported values used across every module:

```ts
export type Severity = 'FAIL' | 'WARN' | 'INFO'

export type Issue = {
  code: string
  severity: Severity
  message: string
  sheet?: string
  row?: number
  field?: string
  businessId?: string
  suggestion?: string
}

export type Rules = {
  version: string
  channelCodes: Record<string, string>
  defaults: {
    currency: string
    currencyToUsd: string
    rechargeRatio: string
    purchaseDiscountRatio: string
    tokenDivisor: number
    groupRatio: string
  }
  modelRules: Record<string, ModelRule>
  overrides: Record<string, RowOverride>
}
```

Create `rules.ts` with `parseRules(input: unknown): Rules`. It must reject a non-object document, missing `version`, non-`CH-*` channel codes, non-positive Decimal strings, non-positive `tokenDivisor`, and rows that attempt to carry `apiKey`, `token`, `authorization`, `cookie`, or `secret` fields. It must return a deeply immutable normalized object and must not introduce any numeric `number` values for money or rates.

- [ ] **Step 4: Re-run the test to verify it passes**

Run: `bun --cwd web test scripts/channel-model-template/__tests__/build.test.ts`

Expected: PASS for both rules tests.

- [ ] **Step 5: Commit the contract**

```bash
git add web/scripts/channel-model-template/types.ts web/scripts/channel-model-template/rules.ts web/scripts/channel-model-template/__tests__/build.test.ts
git commit -m "feat: add channel template conversion rules"
```

### Task 2: Extract raw source data with row provenance

**Files:**
- Create: `web/scripts/channel-model-template/source.ts`
- Create: `web/scripts/channel-model-template/__fixtures__/sd-source-v1.xlsx`
- Create: `web/scripts/channel-model-template/__tests__/source.test.ts`

- [ ] **Step 1: Write the failing extraction tests**

```ts
import assert from 'node:assert/strict'
import test from 'node:test'
import { fileURLToPath } from 'node:url'

import { readSourceWorkbook } from '../source'

const fixture = fileURLToPath(
  new URL('../__fixtures__/sd-source-v1.xlsx', import.meta.url)
)

test('reads source records with their original worksheet and row', async () => {
  const source = await readSourceWorkbook(fixture)

  assert.equal(source.channels.length, 9)
  assert.equal(source.models[0]?.location.sheet, 'sd')
  assert.equal(source.models[0]?.location.row, 3)
  assert.equal(source.officialPrices[0]?.location.sheet, 'sd官价')
})

test('rejects a workbook whose sd header changes', async () => {
  await assert.rejects(
    () => readSourceWorkbook('scripts/channel-model-template/__fixtures__/bad-sd-header.xlsx'),
    /sd header mismatch/
  )
})
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `bun --cwd web test scripts/channel-model-template/__tests__/source.test.ts`

Expected: FAIL because `../source` does not exist.

- [ ] **Step 3: Implement strict workbook extraction**

Copy the provided raw `sd收录.xlsx` to `__fixtures__/sd-source-v1.xlsx`. Implement `readSourceWorkbook(filePath: string)` in `source.ts` using `ExcelJS.Workbook.xlsx.readFile` and three exact header arrays:

```ts
const CHANNEL_HEADERS = ['渠道', '名称', '链接', 'Base Url']
const SD_HEADERS = [
  '渠道', '充值汇率', '手续费', '计费倍率', '付费模式', '模型ID',
  '版本', '计费', '元/秒', '元/次', '元/1M', '素材限制', '清晰度',
  '超分', '时长范围', '比例', '视频输入', '过真人脸', '素材库', 'NSFW',
  '协议', '状态', '并发数', '折扣 秒 无V', '折扣 秒 含V',
  '折扣 M 无V', '折扣 M 含V', '接入', '已测', '售价', '利润', '备注',
] as const
const OFFICIAL_PRICE_HEADERS = [
  '模型', '版本', '分辨率', '不含视频 元/M', '包含视频 元/M', '帧率',
  '长边', '短边', '不含视频 元/秒', '包含视频 元/秒', '备注',
] as const
```

The extractor must find headers at `channel!A2:D2`, `sd!A2:AF2`, and `sd官价!A6:K6`; skip completely blank `sd` rows; convert Excel dates to ISO strings; preserve `location: { sheet, row }`; and reject missing/extra sheets, header differences, external links, macros, more than 20 sheets, more than 20,000 rows per sheet, or a file larger than 10 MiB.

- [ ] **Step 4: Re-run the extraction tests**

Run: `bun --cwd web test scripts/channel-model-template/__tests__/source.test.ts`

Expected: PASS and the source fixture produces 9 channels with source row locations.

- [ ] **Step 5: Commit the source extractor**

```bash
git add web/scripts/channel-model-template/source.ts web/scripts/channel-model-template/__fixtures__/sd-source-v1.xlsx web/scripts/channel-model-template/__tests__/source.test.ts
git commit -m "feat: read channel model source workbooks"
```

### Task 3: Build V1 entities and validate their business invariants

**Files:**
- Create: `web/scripts/channel-model-template/build.ts`
- Create: `web/scripts/channel-model-template/validate.ts`
- Modify: `web/scripts/channel-model-template/__tests__/build.test.ts`

- [ ] **Step 1: Write failing mapping and calculation tests**

```ts
import assert from 'node:assert/strict'
import test from 'node:test'

import { buildTemplateData } from '../build'

test('maps a second-priced source row to per_duration USD cost', () => {
  const output = buildTemplateData(source, rules)
  const cost = output.costs.find((item) => item.businessId === 'COST-CLMM-R3-720-DUR-NOV')

  assert.equal(cost?.mode, 'per_duration')
  assert.equal(cost?.nativePerSecond, '1.38')
  assert.equal(cost?.normalizedUsdUnitPrice, '0.189041095890411')
})

test('keeps a source row without a resolvable SKU as draft', () => {
  const output = buildTemplateData(sourceWithUnknownResolution, rules)

  assert.equal(output.costs[0]?.status, 'draft')
  assert.equal(output.issues[0]?.code, 'SKU_UNRESOLVED')
  assert.equal(output.issues[0]?.severity, 'WARN')
})
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `bun --cwd web test scripts/channel-model-template/__tests__/build.test.ts`

Expected: FAIL because `buildTemplateData` does not exist.

- [ ] **Step 3: Implement deterministic V1 construction and validation**

Implement `buildTemplateData(source, rules)` with these exact mappings:

```ts
const costModeBySource: Record<string, 'per_duration' | 'per_request' | 'per_token'> = {
  second: 'per_duration',
  call: 'per_request',
  token: 'per_token',
}

function normalizedUsd(
  native: Decimal,
  multiplier: Decimal,
  discount: Decimal,
  rechargeRatio: Decimal,
  feeRate: Decimal,
  currencyToUsd: Decimal
): Decimal {
  return native
    .mul(multiplier)
    .mul(discount)
    .div(rechargeRatio)
    .mul(feeRate.add(1))
    .mul(currencyToUsd)
}
```

Use `decimal.js` for every monetary calculation. Generate V1 business IDs from stable source fields and original row number, for example `COST-CLMM-R3-720-DUR-NOV` and `MAP-CLMM-R3-720`. Construct channels, SKUs, sales, costs, mappings, profit scenarios, sources and checks in lexical business-ID order. Apply row-specific corrections exclusively from `rules.overrides`, including the confirmed MegaByAI R102-R105 prices/capabilities now embedded in `build-channel-config-fixtures.mjs`.

Implement `validateTemplateData(data)` to return sorted issues for duplicate business IDs, missing channel/SKU/source references, unrecognized cost mode, invalid non-positive monetary values, invalid duration range, unknown price scenario, missing official price and formula input gaps. `FAIL` prevents generation; `WARN` forces the affected cost, mapping and SKU to `draft`.

- [ ] **Step 4: Re-run mapping tests**

Run: `bun --cwd web test scripts/channel-model-template/__tests__/build.test.ts`

Expected: PASS for the original rules validation tests and the new mapping/calculation tests.

- [ ] **Step 5: Commit source-to-V1 construction**

```bash
git add web/scripts/channel-model-template/build.ts web/scripts/channel-model-template/validate.ts web/scripts/channel-model-template/__tests__/build.test.ts
git commit -m "feat: build validated channel cost template data"
```

### Task 4: Write V1 workbooks and a stable conversion report

**Files:**
- Create: `web/scripts/channel-model-template/write.ts`
- Create: `web/scripts/channel-model-template/__tests__/generate.test.ts`

- [ ] **Step 1: Write failing output tests**

```ts
import assert from 'node:assert/strict'
import fs from 'node:fs/promises'
import test from 'node:test'

import { writeTemplateWorkbook } from '../write'
import { convertWorkbook } from '../../../src/channel-config-converter/conversion'

test('writes a V1 workbook recognized by the existing converter', async () => {
  const result = await writeTemplateWorkbook({ basePath, outputPath, reportPath, data })
  const bytes = await fs.readFile(outputPath)
  const converted = await convertWorkbook(
    new File([bytes], 'channel-model-template.xlsx', { type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet' })
  )

  assert.equal(result.hasFailures, false)
  assert.equal(converted.document.template_version, '1')
})
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `bun --cwd web test scripts/channel-model-template/__tests__/generate.test.ts`

Expected: FAIL because `../write` does not exist.

- [ ] **Step 3: Implement targeted V1 output writing**

`writeTemplateWorkbook` must load `basePath` with ExcelJS and verify the ten V1 sheet names and V1 headers from `web/src/channel-config-converter/workbook.ts`. For each managed data sheet, clear only data values from row 5 through the prior table end, preserve row 1-4 styling/merges/freeze panes, write new values from row 5, and fill the existing validation/formula pattern down to the last data row.

Write the generated date and converter version in `参数`; update the `使用说明` import summary using generated entity counts; rewrite `校验` with formula-error, reference, duplicate, cost coverage and status rows; and set each table reference to its new final row. Produce a JSON report with the exact shape:

```ts
type ConversionReport = {
  converterVersion: string
  source: { path: string; sha256: string }
  rules: { path: string; sha256: string; version: string }
  base: { path: string; sha256: string }
  generatedAt: string
  counts: Record<string, number>
  issues: Issue[]
  output: { path: string; sha256: string }
}
```

ExcelJS writes formulas but does not evaluate them. Scan generated formula text for broken references such as `#REF!`, preserve any cached error values already present in the supplied base, and set `workbook.calcProperties.fullCalcOnLoad = true` and `workbook.calcProperties.forceFullCalc = true` so Excel recalculates on opening. Append static formula-reference errors as `FAIL`. Save the workbook only after all FAIL checks pass, and always save the report.

- [ ] **Step 4: Re-run output tests**

Run: `bun --cwd web test scripts/channel-model-template/__tests__/generate.test.ts`

Expected: PASS and the generated workbook is accepted by `convertWorkbook` as V1.

- [ ] **Step 5: Commit the writer and report**

```bash
git add web/scripts/channel-model-template/write.ts web/scripts/channel-model-template/__tests__/generate.test.ts
git commit -m "feat: write channel model template workbooks"
```

### Task 5: Expose the safe command and capture corrections in rules

**Files:**
- Create: `web/scripts/channel-model-template/generate.ts`
- Create: `web/scripts/channel-model-template/conversion-rules.json`
- Modify: `web/package.json`
- Modify: `web/scripts/channel-model-template/__tests__/generate.test.ts`

- [ ] **Step 1: Write the failing command-behavior tests**

```ts
test('refuses to overwrite an existing workbook without --force', async () => {
  await fs.writeFile(outputPath, 'existing')

  await assert.rejects(
    () => runGenerator(['--source', sourcePath, '--rules', rulesPath, '--base', basePath, '--output', outputPath]),
    /Output already exists/
  )
})

test('writes only the report when the input has a FAIL issue', async () => {
  const result = await runGenerator(['--source', invalidSourcePath, '--rules', rulesPath, '--base', basePath, '--output', outputPath, '--report', reportPath])

  assert.equal(result.hasFailures, true)
  await assert.rejects(() => fs.access(outputPath))
  await fs.access(reportPath)
})
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `bun --cwd web test scripts/channel-model-template/__tests__/generate.test.ts`

Expected: FAIL because `runGenerator` does not exist.

- [ ] **Step 3: Implement the generator command and rules file**

Implement `runGenerator(args: string[])` and the `generate.ts` CLI entry point. It accepts exactly `--source`, `--rules`, `--base`, `--output`, and optional `--report`, `--allow-warnings`, `--force`. Default the report path to the output basename plus `.report.json`. Reject unknown flags, missing values, relative output paths outside the current worktree, identical input/output paths, duplicate flags and output overwrites without `--force`.

Add this script to `web/package.json`:

```json
"channel-model-template:generate": "bun scripts/channel-model-template/generate.ts"
```

Create `conversion-rules.json` with every channel mapping, default, model rule and R102-R105 correction from the current one-off generator represented as structured data. The existing `build-channel-config-fixtures.mjs` remains an independent historical-fixture generator and is not changed by this task.

- [ ] **Step 4: Re-run command tests and the existing converter tests**

Run: `bun --cwd web test scripts/channel-model-template/__tests__/generate.test.ts`

Expected: PASS for overwrite protection and report-only failures.

Run: `bun --cwd web run converter:test`

Expected: PASS; the V1/V2 converter fixtures remain valid.

- [ ] **Step 5: Commit the command surface**

```bash
git add web/package.json web/scripts/channel-model-template/generate.ts web/scripts/channel-model-template/conversion-rules.json web/scripts/channel-model-template/__tests__/generate.test.ts
git commit -m "feat: add channel model template generator command"
```

### Task 6: Execute the full regression, verify output, and document maintenance

**Files:**
- Create: `docs/channel-model-template-generator.md`
- Modify: `web/scripts/channel-model-template/__tests__/generate.test.ts`

- [ ] **Step 1: Write the failing baseline-regression assertion**

```ts
test('recreates the baseline workbook with the expected entity counts', async () => {
  const result = await runGenerator(baselineArguments)

  assert.deepEqual(result.report.counts, {
    channels: 9,
    modelSkus: 9,
    saleProposals: 16,
    costRules: 232,
    modelMappings: 125,
    profitScenarios: 228,
  })
})
```

- [ ] **Step 2: Run the regression to verify it fails before final reconciliation**

Run: `bun --cwd web test scripts/channel-model-template/__tests__/generate.test.ts`

Expected: FAIL until every required rule and correction has been represented in `conversion-rules.json`.

- [ ] **Step 3: Reconcile rules, document maintenance, and make the test pass**

Update only `conversion-rules.json` to reconcile each baseline count. Create `docs/channel-model-template-generator.md` with these operational instructions:

```text
1. Update facts in sd收录.xlsx: channel, sd, and sd官价 only.
2. Update conversion-rules.json for new channel IDs, source schemas, parsing rules, confirmed exceptions, or global assumptions.
3. Update the V1 base workbook only for layout, headers or formula-contract changes.
4. Run the Bun command with an unused output filename.
5. Review the JSON report; resolve every FAIL and every WARN that should become active.
6. Treat the generated workbook as output. Persist any manual correction in the rules file before regenerating.
```

- [ ] **Step 4: Run the final checks**

Run: `bun --cwd web test scripts/channel-model-template/__tests__`

Expected: PASS.

Run: `bun --cwd web run converter:test`

Expected: PASS.

Run: `bun --cwd web run typecheck`

Expected: PASS.

Run from `web/`: `bunx oxfmt --check scripts/channel-model-template scripts/build-channel-config-fixtures.mjs`

Expected: PASS.

Run the generator with the supplied raw source, V1 base fixture and rules file into a new `outputs/<timestamp>/` directory. Inspect all ten output sheets, scan generated formula text for broken references, confirm the workbook requests full recalculation on opening, and verify its V1 conversion with `bun --cwd web run converter:test`.

- [ ] **Step 5: Commit the documented and verified generator**

```bash
git add docs/channel-model-template-generator.md web/scripts/channel-model-template/__tests__/generate.test.ts web/scripts/channel-model-template/conversion-rules.json
git commit -m "docs: document channel model template maintenance"
```

## Plan Self-Review

- Spec coverage: Tasks 1-3 cover source/rules boundaries, deterministic mapping, Decimal calculation, draft policy and validation. Task 4 covers V1 preservation, formulas and reporting. Task 5 covers the command, overwrite protection and migration of one-off corrections. Task 6 covers baseline reconciliation, maintenance instructions and final verification.
- Placeholder scan: no open implementation placeholders; all file locations, commands, inputs and expected outcomes are explicit.
- Type consistency: `Rules`, `Issue`, `buildTemplateData`, `validateTemplateData`, `writeTemplateWorkbook`, `ConversionReport` and `runGenerator` are named consistently across tasks.
