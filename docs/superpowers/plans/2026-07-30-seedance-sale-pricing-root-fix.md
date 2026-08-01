# Seedance Sale Pricing Root Fix Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make imported Seedance sales pricing a real, request-aware per-duration billing configuration, eliminate the misleading sale-proposal `enabled:false` marker, and support both active Mini public model aliases.

**Architecture:** The shared Seedance pricing profile remains the single official price matrix. Task submission derives a bounded duration multiplier from output resolution, reference-video duration, and the matrix; import publication writes only the 720p text-to-video base duration price. The converter keeps the 16 source-audit rows but stops emitting a non-functional enable flag.

**Tech Stack:** Go, Gin, GORM options storage, React/TypeScript Excel converter, Bun, Go test.

---

### Task 1: Express the Seedance duration multiplier

**Files:**
- Modify: `pkg/seedancepricing/profile.go`
- Modify: `pkg/seedancepricing/profile_test.go`

- [ ] **Step 1: Write failing table tests** for Mini 720p text-only, Mini 720p with a five-second video input, and Standard 480p text-only. Assert the multiplier combines unit-price, pixel rate, and input/output duration.
- [ ] **Step 2: Run** `go test ./pkg/seedancepricing -run TestDurationMultiplier -count=1` and confirm it fails because `DurationMultiplier` is missing.
- [ ] **Step 3: Implement** `DurationMultiplier(modelName, resolution string, hasVideo bool, inputDurationMS int64, outputDurationSeconds int) (float64, bool)` using only the profile matrix and finite-positive validation.
- [ ] **Step 4: Run** the focused package test and confirm it passes.

### Task 2: Apply the matrix to all per-duration Seedance tasks

**Files:**
- Modify: `relay/relay_task.go`
- Modify: `relay/relay_task_seedance_test.go`

- [ ] **Step 1: Write failing task-submit tests** that configure the Mini base duration price and verify a 720p five-second text request charges the official base amount, while a reference-video request applies the matrix multiplier without retaining an adapter-specific `resolution` multiplier.
- [ ] **Step 2: Run** `go test ./relay -run TestSeedanceDurationPricing -count=1` and confirm the new assertions fail.
- [ ] **Step 3: Implement** a small submit-path overlay that, after validated request parsing and before pre-consumption, resolves reference-video metadata when required, deletes legacy `resolution` pricing for a recognized Seedance family, and adds a `seedance_price_matrix` other ratio. It must fail closed when mandatory reference-video metadata is unavailable.
- [ ] **Step 4: Run** the focused relay tests and confirm they pass.

### Task 3: Publish template sales as base duration prices

**Files:**
- Modify: `service/config_import_stage.go`
- Modify: `service/config_import_pricing_test.go`

- [ ] **Step 1: Write failing tests** that stage recognized Seedance sale rows into one identical per-duration base-price patch per runtime client model, rather than conflicting token-expression patches.
- [ ] **Step 2: Run** `go test ./service -run TestConfigImportSeedanceSale -count=1` and confirm failure.
- [ ] **Step 3: Implement** a Seedance-specific option-patch path using the shared 720p text-only official unit price and profile token rate, converted from CNY to USD. Preserve the normal generic path for non-Seedance proposals and emit no competing token-expression patch.
- [ ] **Step 4: Run** the focused service tests and confirm they pass.

### Task 4: Remove the non-functional proposal flag and normalize Mini aliases

**Files:**
- Modify: `web/src/channel-config-converter/document.ts`
- Modify: `web/src/channel-config-converter/__tests__/document.test.ts`
- Modify: `service/config_import_publish.go`
- Modify: `service/config_import_publish_test.go`

- [ ] **Step 1: Write failing converter and publication tests** proving new sale proposals omit `enabled` and a Seedance Mini publication writes equivalent pricing for the active `260615` and `260128` client aliases.
- [ ] **Step 2: Run** the focused Bun and Go tests and confirm failure.
- [ ] **Step 3: Implement** omission of the sale enable marker and alias expansion limited to the Mini pricing family. Do not alter channel or route target enable guards.
- [ ] **Step 4: Run** focused tests and confirm they pass.

### Task 5: Repair the published local configuration and verify

**Files:**
- No source file changes required.

- [ ] **Step 1: Back up and inspect** current pricing options and batch 6 proposal values.
- [ ] **Step 2: Apply** the generated base duration prices and billing modes to the three active public Seedance models, including both Mini aliases, through the service option path.
- [ ] **Step 3: Rebuild** the local `new-api` container and verify its health endpoint.
- [ ] **Step 4: Run** focused Go tests, converter tests, frontend typecheck, and a real-browser 720p/5-second text request acceptance. Confirm the request reaches the selected route and reports a non-zero usage amount.
