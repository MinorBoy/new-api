# Cost Accounting Tracking And Dimensio Remediation Implementation Plan

> For agentic workers: use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox syntax for tracking.

**Goal:** Record supplier cost for covered Dimensio production traffic without blocking uncovered routes, correctly present duration billing in usage logs, and import the CNY 0.25/second Mini 720p procurement rule to channel 5.

**Architecture:** Add tracking beside disabled and strict. The relay prepares and settles a ledger only when it can determine a final upstream model and lock a compatible active rule; otherwise it writes a request-correlated warning and preserves the upstream dispatch. Strict admission, profit rechecks, and coverage gating remain unchanged. The config-import workflow stays the sole writer for production cost-rule data.

**Tech Stack:** Go 1.22, Gin, GORM v2, MySQL/SQLite/PostgreSQL-compatible data access, React 19, TypeScript, i18next, Bun, artifact-tool, Docker Compose, Playwright.

---

## File Structure

- types/cost_accounting.go defines the accepted runtime-mode enum.
- setting/cost_setting/config.go validates and snapshots the three modes.
- controller/cost_accounting.go accepts tracking without relaxing the strict coverage guard.
- relay/cost_accounting_adaptor.go records synchronous covered requests without turning tracking-only failures into relay errors.
- relay/relay_task.go applies the same non-blocking policy to asynchronous tasks and supplies the validated duration meter.
- web/src/features/cost-accounting exposes a three-state settings control and matching API types.
- web/src/features/usage-logs/components/dialogs/details-dialog.tsx renders per-duration billing accurately.
- web/src/features/cost-accounting/components/cost-request-detail.tsx distinguishes missing cost data from pre-feature historical data.
- web/src/i18n/locales receives every new key through scripts/add-missing-keys.mjs and the i18n sync command.
- outputs/019f9dbb-4e5d-7933-8531-d38e417ec068/渠道模型成本与利润模板-v1-修正版.xlsx is the durable import source updated for the channel-5 Dimensio rule set.

### Task 1: Add The Tracking Runtime Contract

**Files:**
- Modify: types/cost_accounting.go
- Modify: types/cost_accounting_test.go
- Modify: setting/cost_setting/config.go
- Modify: setting/cost_setting/config_test.go
- Modify: controller/cost_accounting.go
- Modify: controller/cost_accounting_test.go

- [ ] Step 1: Write failing runtime and HTTP tests

Add tests that assert tracking is preserved by cost_setting.Runtime and that PUT /api/cost-accounting/settings with mode tracking returns HTTP 200 even when authoritative coverage has uncovered rows. Retain the strict-conflict test unchanged.

- [ ] Step 2: Run the focused tests and verify RED

Run:
    go test ./setting/cost_setting ./controller -run 'TestCostSettingAcceptsTrackingMode|TestCostAccountingTrackingModeAllowsIncompleteCoverage|TestCostAccountingStrictModeRejectsIncompleteCoverage' -count=1

Expected: FAIL because tracking is not an accepted CostAccountingMode.

- [ ] Step 3: Implement the minimal enum, validation, and API acceptance

Add CostAccountingTracking CostAccountingMode = "tracking", accept it in ValidateMode, and leave the coverage lookup nested under the strict-mode condition. Do not add schema, alter strict behavior, or change margin semantics.

- [ ] Step 4: Run focused tests and commit

Run the Step 2 command and require PASS.
    git add types/cost_accounting.go types/cost_accounting_test.go setting/cost_setting/config.go setting/cost_setting/config_test.go controller/cost_accounting.go controller/cost_accounting_test.go
    git commit -m "feat(cost): add non-blocking tracking mode"

### Task 2: Record Covered Sync Requests In Tracking Mode

**Files:**
- Modify: relay/cost_accounting_adaptor.go
- Modify: relay/cost_accounting_adaptor_test.go

- [ ] Step 1: Write failing relay tests

Add one tracking test with an active per-request rule that asserts the fake transport is called and the created attempt settles. Add one tracking test with no active rule that asserts the fake transport is called and no CostAccountingRequest is created. Strict missing-identity and missing-coverage tests stay fail-closed.

- [ ] Step 2: Run the focused tests and verify RED

Run:
    go test ./relay -run 'TestTrackingCostAdaptorRecordsCoveredRequest|TestTrackingCostAdaptorPreservesUncoveredTransport|TestStrictCostAdaptorRejectsEmptyIdentityBeforeTransport' -count=1

Expected: the tracking covered test FAILS because the adaptor bypasses accounting unless mode is strict.

- [ ] Step 3: Implement non-blocking attempt preparation

Return directly only for disabled mode. For tracking, confirm a non-empty final billable model, call PrepareCostAttempt and AuthorizeCostDispatch, and on every identity, coverage, preparation, or authorization error log the request ID/channel ID then dispatch with no handle. Strict retains its existing recheck and all error returns. Preserve dispatch-outcome, settlement, and winner persistence when a handle exists.

- [ ] Step 4: Run focused tests and commit

Run the Step 2 command and require PASS.
    git add relay/cost_accounting_adaptor.go relay/cost_accounting_adaptor_test.go
    git commit -m "feat(cost): track covered synchronous relays"

### Task 3: Record Covered Async Task Requests In Tracking Mode

**Files:**
- Modify: relay/relay_task.go
- Modify: relay/relay_task_billing_test.go
- Modify: relay/relay_task_recheck_test.go

- [ ] Step 1: Write failing task-submit tests

Add a tracking covered task test with a per-duration validated-request rule. Assert upstream dispatch, a five-second request meter, and settlement. Add an uncovered tracking test that submits upstream, persists the task, and leaves info.CostAttempt nil.

- [ ] Step 2: Run the focused tests and verify RED

Run:
    go test ./relay -run 'TestTrackingTaskSubmitSettlesCoveredDurationCost|TestTrackingTaskSubmitPreservesUncoveredTransport|TestProfitRecheckBlocksDispatchAfterRuleChange' -count=1

Expected: tracking tests FAIL because RelayTaskSubmit prepares costs only in strict mode.

- [ ] Step 3: Implement tracking task admission

Confirm identity, construct a validated duration meter, prepare the attempt, and authorize dispatch in strict or tracking. Strict errors keep their existing 503/500 task errors. Tracking logs each accounting-only error and continues with CostAttempt unset. RecheckSelectedChannelProfit remains strict-only. Keep completion settlement, recovery, revenue recognition, and retry behavior unchanged for prepared handles.

- [ ] Step 4: Run focused tests and commit

Run the Step 2 command and require PASS.
    git add relay/relay_task.go relay/relay_task_billing_test.go relay/relay_task_recheck_test.go
    git commit -m "feat(cost): track covered asynchronous tasks"

### Task 4: Correct The Admin UI And Translations

**Files:**
- Modify: web/src/features/cost-accounting/types.ts
- Modify: web/src/features/cost-accounting/index.tsx
- Modify: web/src/features/cost-accounting/components/__tests__/profit-report.test.tsx
- Modify: web/src/features/usage-logs/components/dialogs/details-dialog.tsx
- Create: web/src/features/usage-logs/components/__tests__/billing-breakdown.test.tsx
- Modify: web/src/features/cost-accounting/components/cost-request-detail.tsx
- Modify: web/src/features/cost-accounting/components/__tests__/cost-request-detail.test.tsx
- Modify through script: web/src/i18n/locales/{en,zh,zh-TW,fr,ja,ru,vi}.json

- [ ] Step 1: Write failing frontend tests

Add coverage that a per_duration usage log displays Per-duration, duration price, and billable duration instead of Per-token. Update the no-request-id detail test to expect a neutral no-record message. Add a settings interaction test that accepts tracking while strict stays disabled when coverage is incomplete.

- [ ] Step 2: Run focused frontend tests and verify RED

Run from web:
    bun test --parallel=1 src/features/cost-accounting/components/__tests__/profit-report.test.tsx src/features/cost-accounting/components/__tests__/cost-request-detail.test.tsx src/features/usage-logs/components/__tests__/billing-breakdown.test.tsx

Expected: FAIL because tracking is not a UI mode and the billing breakdown falls through to per-token.

- [ ] Step 3: Implement the UI behavior

Extend CostAccountingMode to include tracking. Render Tracking in status badges and add a ToggleGroup item that is not coverage-gated. Insert a per_duration branch before per-call/per-token with duration price and billable/requested duration fields from LogOtherData. Replace the request-ID-zero fallback with neutral no-record text; do not infer request age.

- [ ] Step 4: Add translations with the mandated script

Populate the new English keys and all seven locale values in web/scripts/add-missing-keys.mjs, run bun run i18n:sync, and run the locale checker. Do not hand-edit locale JSON.

- [ ] Step 5: Run focused frontend verification and commit

Run the Step 2 command and:
    bun run i18n:sync
    bun run typecheck
    bun run lint -- src/features/cost-accounting src/features/usage-logs
    git add web/src/features/cost-accounting web/src/features/usage-logs web/src/i18n/locales web/scripts/add-missing-keys.mjs
    git commit -m "fix(admin): show tracked duration cost details"

Expected: all commands PASS with no new locale gaps or lint errors in touched files.

### Task 5: Correct And Validate The Dimensio Import Workbook

**Files:**
- Modify: outputs/019f9dbb-4e5d-7933-8531-d38e417ec068/渠道模型成本与利润模板-v1-修正版.xlsx

- [ ] Step 1: Inspect before modification

Use artifact-tool to render every sheet, inspect Dimensio supplier/model/cost rows, preserve existing formatting/formulas, and identify the Mini 720p record. Confirm its current cost is CNY 0.39 per second and normalized USD amount follows the workbook conversion input.

- [ ] Step 2: Make a targeted source-data correction

Set only jimeng-video-seedance-2.0-mini at 720p to CNY 0.25 per second. Update its normalized USD numeric value under the workbook's existing conversion convention. Do not change mappings, meter, Dimensio automatic routing, reference limits, credentials, or other rules.

- [ ] Step 3: Validate and visually inspect

Inspect edited cells and render the changed sheet. Confirm no formula errors, preserved number formats, and a visible 0.25 Mini 720p price. Export to the same requested workbook path only after the visual check passes.

### Task 6: Publish To Production Channel 5 And Accept The Live Lifecycle

**Files:**
- Verify: Docker Compose services, MySQL new-api, browser workflow, and cost ledgers.

- [ ] Step 1: Build and start the corrected application

Run focused Go tests then:
    go test ./relay ./service ./controller ./setting/cost_setting -count=1
Run from web:
    bun run typecheck
    bun run lint
    bun run build
Rebuild:
    docker compose -f docker-compose.local.yml up -d --build
Require the status endpoint to be healthy.

- [ ] Step 2: Import through the real browser UI

Open /config-import, upload 渠道模型成本与利润模板-v1-修正版.xlsx, bind channel-dimensio specifically to production dimensio (5), review the staged diff, and publish. Do not bind channel 9 or alter Dimensio automatic routing.

- [ ] Step 3: Verify published supplier rules and mode

Require ten active channel-5 Dimensio rules. Require Mini 720p to use per_duration, validated_request, and CNY 0.25 per second. Set global cost accounting to tracking and confirm strict remains unavailable until coverage is complete.

- [ ] Step 4: Run a real five-second Mini 720p request

Submit the existing ARK-compatible request. On terminal success require one linked cost-accounting request and one settled winning attempt for channel 5 / jimeng-video-seedance-2.0-mini, a five-second meter, CNY 1.25 supplier cost, CNY 2.484 revenue equivalent, and roughly CNY 1.234 gross profit. Do not parse or infer creditsConsumed or an automatic effective model.

- [ ] Step 5: Browser acceptance and final verification

Verify usage-log details show Per-duration, the user charge, and linked supplier cost, revenue, gross profit, and margin. Capture a Playwright screenshot, run all changed tests, and commit only remediation-owned files.

## Self-Review

- Spec coverage: Tasks 1-3 implement non-blocking backend tracking, Task 4 fixes confirmed UI defects, and Tasks 5-6 correct the durable workbook source, production binding, deployment, and real-request acceptance.
- Placeholder scan: no TBD, TODO, or unspecified implementation/test step remains.
- Type consistency: backend and frontend use the literal tracking; strict-only coverage enforcement and CostAttemptHandle lifecycle remain preserved across sync and task relays.

