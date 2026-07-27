# YSR Channel Type ID Migration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move YSR-only channel types to `200-207` while preserving all existing channel and task records through idempotent migrations.

**Architecture:** Keep the established `59-66 -> 61-68` migration under its existing marker, then run a second marker-guarded transaction that moves `61-68 -> 200-207`. Channel constants, default base URLs, and all task-adaptor dispatches use the constants, so changing the constants updates runtime behavior after persisted values have been migrated.

**Tech Stack:** Go 1.22, GORM v2, SQLite unit tests, Testify.

---

### Task 1: Define Migration Regression Coverage

**Files:**
- Modify: `model/channel_type_migration_test.go`
- Modify: `constant/channel_test.go`

- [x] **Step 1: Write a failing migration test**

Seed channels and tasks at `59-66`; require the historical migration to produce `61-68`, and the new migration to produce `200-207`. Run both migrations twice and assert both migration markers exist.

- [x] **Step 2: Run the focused migration test and verify it fails**

Run: `go test ./model -run TestMigrateSecondaryChannelTypeIDsPreservesExistingChannelAndTaskSemantics -count=1`

Expected: FAIL because the second migration and its marker do not exist.

- [x] **Step 3: Write a failing constant test**

Require the eight YSR channel constants to use `200-207`, `ChannelTypeDummy` to be `208`, and `ChannelBaseURLs` to have an entry at each YSR channel type.

- [x] **Step 4: Run the constant tests and verify they fail**

Run: `go test ./constant -count=1`

Expected: FAIL because the current constants remain `61-68` and the base URL slice ends at `68`.

### Task 2: Implement Compatible Runtime and Data Migration

**Files:**
- Modify: `constant/channel.go`
- Modify: `model/main.go`

- [x] **Step 1: Keep the historical mapping stable**

Keep `migrateSecondaryChannelTypeIDs` mapping literal historical values `59-66` to literal intermediate values `61-68`, so changing runtime constants cannot rewrite its historical contract.

- [x] **Step 2: Add the second idempotent migration**

Create a separate marker-guarded GORM transaction that updates both `Channel.Type` and `Task.Platform` from each intermediate value `61-68` to its YSR constant `200-207`; invoke it after the historical migration.

- [x] **Step 3: Move runtime IDs and preserve safe base-URL indexing**

Set the eight YSR constants to `200-207`, set `ChannelTypeDummy` to `208`, and extend `ChannelBaseURLs` through index `207` while retaining existing default URLs at their new indexes.

- [x] **Step 4: Run focused tests and verify they pass**

Run: `go test ./constant ./model -run 'Test(MigrateSecondaryChannelTypeIDsPreservesExistingChannelAndTaskSemantics|.*ChannelConstants)' -count=1`

Expected: PASS.

### Task 3: Keep Generated Import Documents Compatible

**Files:**
- Modify: `web/src/channel-config-converter/document.ts`
- Modify: `web/src/channel-config-converter/__tests__/v1.test.ts`
- Regenerate: `e2e/testdata/channel-config-v1.json`

- [x] **Step 1: Write a failing converter contract test**

Build an import document from the corrected V1 workbook and require its Dimensio through Secure channel definitions to use `200-207`.

- [x] **Step 2: Run the converter test and verify it fails**

Run: `bun test --parallel=1 src/channel-config-converter/__tests__/v1.test.ts`

Expected: FAIL because `V1_CHANNEL_TYPES` still emits `61-68`.

- [x] **Step 3: Update the converter mapping and generated fixture**

Set the eight V1 converter channel identities to `200-207`, then regenerate `e2e/testdata/channel-config-v1.json` with `bun run scripts/build-channel-config-fixtures.mjs` so its entity and payload hashes remain valid.

- [x] **Step 4: Verify converter and E2E import behavior**

Run: `bun test --parallel=1 src/channel-config-converter && go test ./e2e -run TestConfigImportV1FixturePublishesDisabledConfigurationE2E -count=1`

Expected: PASS.

### Task 4: Keep Fast Migration and Admin Channel Types Aligned

**Files:**
- Modify: `model/main.go`
- Modify: `model/channel_type_migration_test.go`
- Modify: `web/src/features/channels/constants.ts`
- Modify: `web/src/features/channels/lib/channel-type-config.ts`
- Modify: `web/src/features/channels/lib/channel-utils.ts`
- Modify: `web/src/features/channels/lib/secure-video-group.ts`
- Modify: affected tests under `web/tests/` and `web/src/features/channels/**/__tests__/`

- [x] **Step 1: Write failing fast-migration and admin-registry tests**

Require the shared migration sequence to reach `200-207`, and require the channel admin registry to expose Secure as type `207` with the corresponding task-only behavior and default URL.

- [x] **Step 2: Verify the new tests fail**

Run: `go test ./model -run TestMigrateSecondaryChannelTypeIDsPreservesExistingChannelAndTaskSemantics -count=1` and `bun test --parallel=1 tests/channel-type-config.test.ts src/features/channels/lib/__tests__/secure-video-group.test.ts`

Expected: FAIL because the fast path omits the second migration and the admin registry still uses `61-68`.

- [x] **Step 3: Use a shared migration sequence and update the admin registry**

Invoke one migration-sequence function from both normal and fast database migrations. Move labels, options, task-only markers, provider configurations, base URLs, icons, Secure group rules, and their tests to `200-207`.

- [x] **Step 4: Verify frontend and backend contracts**

Run: `go test ./model -count=1`, `bun test --parallel=1 tests/channel-type-config.test.ts src/features/channels/lib/__tests__/secure-video-group.test.ts`, `bun run typecheck`, `bun run lint`, and `bun run format:check`.

Expected: PASS.

### Task 5: Document and Verify the Constraint

**Files:**
- Modify: `RULE.md`

- [x] **Step 1: Record the reserved-range rule**

Require YSR-specific channel type IDs to use `200-299`, and require transactional migration of `channels.type` and `tasks.platform` plus regression tests for any renumbering.

- [x] **Step 2: Run targeted verification**

Run: `go test ./constant ./model ./relay -count=1`

Expected: PASS.

- [ ] **Step 3: Commit**

Run: `git add RULE.md constant/channel.go constant/channel_test.go model/main.go model/channel_type_migration_test.go docs/superpowers/plans/2026-07-27-ysr-channel-type-id-migration.md && git commit -m "feat: migrate ysr channel types to reserved IDs"`
