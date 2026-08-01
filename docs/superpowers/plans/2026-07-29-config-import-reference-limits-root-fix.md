# Config Import Reference Limits Root Fix Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Preserve explicit image, video, and audio reference limits from channel configuration workbooks, reject incomplete route constraints, and repair the currently published targets without changing their enabled state.

**Architecture:** The converter is the source-of-truth boundary: V1 compact `素材限制=XYZ` notes are decoded into structured limits, while V2 exposes explicit structured columns. Backend schema and publish validation reject missing components so an absent value can never silently become `0/0/0`. Existing MySQL rows are repaired in one transaction by updating only `constraints.reference_limits` from the newly generated import document.

**Tech Stack:** React/TypeScript converter, Bun tests, Go/Gin service validation, MySQL JSON updates, Docker Compose.

---

### Task 1: Decode V1 Compact Reference Limits

**Files:**
- Modify: `web/src/channel-config-converter/__tests__/v1.test.ts`
- Modify: `web/src/channel-config-converter/document.ts`

- [ ] **Step 1: Write the failing converter regression test**

Assert that `MAP-DIMENSIO-R83-720`, a `431` target, and a `900` target produce explicit zero minimums and decoded maximums, and that every generated V1 target has all three maximum fields.

- [ ] **Step 2: Run the test and verify RED**

Run: `bun test --parallel=1 src/channel-config-converter/__tests__/v1.test.ts`

Expected: FAIL because generated targets do not contain `reference_minimums` or `reference_limits`.

- [ ] **Step 3: Implement strict compact-code parsing**

Add a durable converter helper that accepts only a semicolon-delimited `素材限制=` followed by exactly three decimal digits and returns:

```ts
{
  minimums: { images: 0, videos: 0, audios: 0 },
  limits: {
    images: Number(match[1]),
    videos: Number(match[2]),
    audios: Number(match[3]),
  },
}
```

For V1 mappings, emit an error issue and omit the unsafe route blueprint when the code is missing or invalid. Otherwise write both structured objects into the route target.

- [ ] **Step 4: Run the focused converter test and verify GREEN**

Run: `bun test --parallel=1 src/channel-config-converter/__tests__/v1.test.ts`

Expected: PASS.

### Task 2: Make V2 Limits Explicit

**Files:**
- Modify: `web/src/channel-config-converter/workbook.ts`
- Modify: `web/src/channel-config-converter/document.ts`
- Modify: `web/src/channel-config-converter/__tests__/v2.test.ts`
- Modify: `web/scripts/build-channel-config-fixtures.mjs`
- Regenerate: `docs/templates/channel-config-v2.xlsx`
- Regenerate: `web/src/channel-config-converter/__fixtures__/channel-config-v2-golden.xlsx`

- [ ] **Step 1: Write the failing V2 structured-field test**

Extend the V2 test record with `reference_min_images`, `reference_min_videos`, `reference_min_audios`, `reference_max_images`, `reference_max_videos`, and `reference_max_audios`; assert the resulting target retains every value.

- [ ] **Step 2: Run the V2 test and verify RED**

Run: `bun test --parallel=1 src/channel-config-converter/__tests__/v2.test.ts`

Expected: FAIL because the V2 schema and document builder ignore those fields.

- [ ] **Step 3: Extend the V2 route schema and generator**

Add the six exact headers, parse them with `optionalInteger`, and populate `reference_minimums` and `reference_limits`. Generate V2 route rows from the V1 decoded limits and update enabled-column validation/formulas to the new column address.

- [ ] **Step 4: Regenerate fixtures and verify GREEN**

Run: `bun web/scripts/build-channel-config-fixtures.mjs`

Run: `bun test --parallel=1 src/channel-config-converter/__tests__/v2.test.ts`

Expected: fixture generation and tests PASS.

### Task 3: Reject Incomplete Limits in the Backend

**Files:**
- Modify: `service/config_import_schema_test.go`
- Modify: `service/config_import_schema.go`
- Modify: `service/config_import_publish_test.go`
- Modify: `service/config_import_publish.go`

- [ ] **Step 1: Write failing schema and publish tests**

Add cases proving a target with missing `reference_limits`, or with any missing image/video/audio component, is rejected with `SCHEMA_ROUTE_REFERENCE`; add a publish-layer test proving direct staged data is rejected before zero-value serialization.

- [ ] **Step 2: Run focused Go tests and verify RED**

Run: `go test ./service -run 'TestConfigImportSchema.*Reference|TestConfigImportRouteRows.*Reference' -count=1`

Expected: FAIL because nil bounds are currently accepted.

- [ ] **Step 3: Add defense-in-depth validation**

Require non-nil `ReferenceMinimums` and `ReferenceLimits`, with all three pointer fields present, before range validation or publish conversion. Preserve explicit zero values.

- [ ] **Step 4: Run focused Go tests and verify GREEN**

Run the same focused command; expected PASS.

### Task 4: Regenerate Import Artifacts and Repair Live Data

**Files:**
- Regenerate: `e2e/testdata/channel-config-v1.json`

- [ ] **Step 1: Regenerate the canonical V1 JSON fixture**

Run: `bun web/scripts/build-channel-config-fixtures.mjs`

Verify every route target contains complete `reference_minimums` and `reference_limits`.

- [ ] **Step 2: Validate the live repair set before mutation**

Compare current MySQL route target names with the generated document and require exactly 52 one-to-one matches. Abort if any target is missing or duplicated.

- [ ] **Step 3: Update only live JSON limit fields in one transaction**

Use a temporary in-session mapping table and `JSON_SET` to update `$.reference_limits.images`, `$.reference_limits.videos`, and `$.reference_limits.audios`. Do not change policies, priorities, target enabled flags, credentials, or other constraint fields.

- [ ] **Step 4: Verify the live distribution**

Expected current distribution: 33 targets at `9/3/3`, 16 at `4/3/1`, and 3 at `9/0/0`; R83 must be `9/3/3`.

### Task 5: Full Verification and Local Deployment

**Files:**
- Verify all modified files and generated artifacts.

- [ ] **Step 1: Run frontend verification**

Run: `bun run converter:test`

Run: `bun run typecheck`

Run: `bun run lint -- src/channel-config-converter web/scripts/build-channel-config-fixtures.mjs`

- [ ] **Step 2: Run backend verification**

Run: `go test ./service -count=1`

- [ ] **Step 3: Build the application**

Run from `web/`: `bun run build`

Run from repository root: `go test ./... -count=1`

- [ ] **Step 4: Rebuild and restart local containers**

Run: `docker compose -f docker-compose.local.yml up -d --build`

Verify `/api/status` is healthy and re-query the live reference-limit distribution after restart.
