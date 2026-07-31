# Channel Binding Skip Reason Removal Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove the skip-specific reason from the channel-binding UI and API so an operator can save a skipped line directly.

**Architecture:** The frontend binding draft and Zod request schema stop modeling `reason`. The backend binding DTO, validation, and skip-state snapshot stop carrying it; skipped item dependencies use an empty `exclusion_reason` until a later bind/create action restores them. The generic import-item exclusion reason remains unchanged for other workflows.

**Tech Stack:** React 19, TypeScript, Zod, Vitest-style Bun tests, Go 1.22, GORM, testify.

---

### Task 1: Remove The Frontend Skip Reason Contract

**Files:**
- Modify: `web/src/features/config-import/components/channel-binding-step.tsx`
- Modify: `web/src/features/config-import/types.ts`
- Test: `web/src/features/config-import/components/__tests__/channel-binding-step.test.tsx`

- [ ] **Step 1: Write the failing frontend regression test**

Replace the existing skip-reason test with a test that selects `Skip`, verifies no `Skip reason` input exists, saves, and expects the reason-free payload:

```tsx
test('saves a skipped channel line without a reason input', async () => {
  const mounted = await mount()
  try {
    const skip = [...mounted.container.querySelectorAll('button')].find(
      (button) => button.textContent === 'Skip'
    )
    assert.ok(skip instanceof browserWindow.HTMLButtonElement)
    await act(async () => skip.click())

    assert.equal(
      mounted.container.querySelector('input[aria-label="Skip reason"]'),
      null
    )

    const save = [...mounted.container.querySelectorAll('button')].find(
      (button) => button.textContent === 'Save bindings'
    )
    assert.ok(save instanceof browserWindow.HTMLButtonElement)
    await act(async () => save.click())

    assert.deepEqual(mounted.saved, [
      {
        bindings: [
          {
            line_ref: 'line-1',
            action: 'skip',
            credentials_confirmed: false,
          },
        ],
      },
    ])
  } finally {
    await act(async () => mounted.root.unmount())
  }
})
```

- [ ] **Step 2: Run the frontend test to verify it fails**

Run: `bun test --parallel=1 src/features/config-import/components/__tests__/channel-binding-step.test.tsx`

Expected: FAIL because the current `Skip` action renders `input[aria-label="Skip reason"]` and saving an empty reason is rejected.

- [ ] **Step 3: Remove the frontend field and validation**

In `channel-binding-step.tsx`:

```tsx
interface BindingDraft {
  action: ConfigImportBinding['action']
  channelID?: number
  credentialsConfirmed: boolean
}
```

Remove every default `reason: ''`, remove the `Input` import and conditional skip-input branch, and save skips with:

```tsx
bindings.push({
  line_ref: line.lineRef,
  action: 'skip',
  credentials_confirmed: false,
})
```

In `types.ts`, remove `reason` from `configImportBindingSchema` and delete both skip-reason refinement branches. Keep the existing channel and credential rules for skipped and bound actions.

- [ ] **Step 4: Run the frontend test to verify it passes**

Run: `bun test --parallel=1 src/features/config-import/components/__tests__/channel-binding-step.test.tsx`

Expected: PASS with all channel-binding tests green.

### Task 2: Remove The Backend Skip Reason Contract

**Files:**
- Modify: `dto/config_import.go`
- Modify: `service/config_import_stage.go`
- Test: `service/config_import_stage_test.go`

- [ ] **Step 1: Write failing backend regression tests**

Add a reason-free skip success test using `UpdateConfigImportBindings` and a strict-decoding test for the removed legacy field:

```go
func TestConfigImportBindingAllowsSkipWithoutReason(t *testing.T) {
  prepareConfigImportBindingDB(t)
  batch := createConfigImportBindingBatch(t, configImportBindingLineFixture{
    lineRef: "line-openai", channelRef: "supplier-openai", channelType: constant.ChannelTypeOpenAI,
    models: []string{"gpt-test"},
  })

  _, err := UpdateConfigImportBindings(context.Background(), 42, batch.ID, []dto.ConfigImportBindingInput{{
    LineRef: "line-openai", Action: types.ConfigImportBindingActionSkip,
  }})

  require.NoError(t, err)
  var binding model.ConfigImportBinding
  require.NoError(t, model.DB.Where("batch_id = ? AND line_ref = ?", batch.ID, "line-openai").First(&binding).Error)
  assert.Equal(t, string(types.ConfigImportBindingActionSkip), binding.Action)
}

func TestConfigImportBindingStrictDecodeRejectsRemovedSkipReason(t *testing.T) {
  _, err := DecodeConfigImportBindingRequest(strings.NewReader(`{"bindings":[{"line_ref":"line-openai","action":"skip","reason":"legacy"}]}`))
  require.Error(t, err)
  assert.Contains(t, err.Error(), "reason")
}
```

- [ ] **Step 2: Run the backend tests to verify they fail**

Run: `go test ./service -run 'TestConfigImportBinding(AllowsSkipWithoutReason|StrictDecodeRejectsRemovedSkipReason)' -count=1`

Expected: the reason-free skip test fails with `SCHEMA_BINDING_REASON`, and the strict-decoding test fails because `reason` is still accepted.

- [ ] **Step 3: Remove the DTO, validation, and snapshot reason**

Remove `Reason` from `dto.ConfigImportBindingInput`. In `validateConfigImportBindingInputs`, remove reason trimming, credential-pattern matching, the bound-action `reason` rejection, and the skip-action required-reason branch.

Remove `Reason` from `configImportSkipStateSnapshot`; call `excludeConfigImportLineDependents` without a reason argument; and write skipped dependent items with:

```go
"exclusion_reason": "",
```

In `reconcileConfigImportLineDependents`, retain the existing guarded restore query but use the empty skip marker:

```go
Where("id = ? AND batch_id = ? AND state = ? AND exclusion_reason = ?", item.ID, batchID,
  string(types.ConfigImportItemStateExcluded), "")
```

Delete the credential-like skip-reason test because the strict decoder now rejects the whole removed field.

- [ ] **Step 4: Run the backend tests to verify they pass**

Run: `go test ./service -run 'TestConfigImportBinding(AllowsSkipWithoutReason|StrictDecodeRejectsRemovedSkipReason)' -count=1`

Expected: PASS.

### Task 3: Verify, Accept, And Commit

**Files:**
- Verify: `web/src/features/config-import/components/channel-binding-step.tsx`
- Verify: `web/src/features/config-import/types.ts`
- Verify: `dto/config_import.go`
- Verify: `service/config_import_stage.go`

- [ ] **Step 1: Run affected test suites and static checks**

Run:

```powershell
cd web
bun test --parallel=1 src/features/config-import/components/__tests__/channel-binding-step.test.tsx
bun run typecheck
bunx oxlint -c .oxlintrc.json src/features/config-import/components/channel-binding-step.tsx src/features/config-import/types.ts
bunx oxfmt --check src/features/config-import/components/channel-binding-step.tsx src/features/config-import/types.ts src/features/config-import/components/__tests__/channel-binding-step.test.tsx
cd ..
go test ./service -run 'TestConfigImportBinding' -count=1
gofmt -w dto/config_import.go service/config_import_stage.go service/config_import_stage_test.go
git diff --check
```

Expected: all tests and checks pass with no formatting errors.

- [ ] **Step 2: Perform browser acceptance**

Open the local `/config-import` binding flow, choose `Skip` for a channel line, verify there is no skip-reason input, click `Save bindings`, and verify the request completes without a required-reason error.

- [ ] **Step 3: Commit only the scoped code and tests on `ysr`**

Run:

```powershell
git add -- dto/config_import.go service/config_import_stage.go service/config_import_stage_test.go web/src/features/config-import/components/channel-binding-step.tsx web/src/features/config-import/components/__tests__/channel-binding-step.test.tsx web/src/features/config-import/types.ts docs/superpowers/plans/2026-07-31-config-import-skip-reason-removal.md
git commit -m "feat: remove channel binding skip reason"
```

Expected: a new commit on `ysr`; do not stage unrelated worktree changes and never commit or merge to `main`.
