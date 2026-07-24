# Origin Main Into Ysr Merge Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Merge the reviewed `origin/main` into local `ysr`, preserve all committed second-development behavior, adopt the upstream single-frontend layout, and return protected user changes without pushing.

**Architecture:** Perform one atomic Git merge in the current `ysr` worktree because this branch and its protected stash/untracked baseline are the subject of the operation. Resolve backend conflicts by composing observable contracts, migrate every committed `web/default` customization to upstream's `web` root, regenerate derived frontend artifacts, verify before creating the merge commit, and restore protected working changes afterward.

**Tech Stack:** Git, Go 1.22+, GORM, Gin, React 19, TypeScript, Base UI, Tailwind CSS, Bun, i18next.

---

## Execution Rules

- Execute in `C:\Users\880pro\Documents\new-api` on local branch `ysr`; do not create or switch to another worktree for this merge.
- Use `apply_patch` for manual source and JSON edits. Use `gofmt`, Bun, and project generators only for mechanical formatting or generated files.
- Do not run `git clean`, `git reset --hard`, broad `git checkout`, or whole-tree `ours`/`theirs` resolution.
- Do not stage the pre-existing untracked files listed in Task 1.
- Do not commit while conflicts remain. Tasks 3-7 use the index as checkpoints; Task 8 creates the single merge commit.
- Do not push.

### Task 1: Revalidate the Pinned Safety Baseline

**Files:**
- Read: `docs/superpowers/specs/2026-07-24-origin-main-ysr-merge-conflict-resolution-design.md`
- Read: `.dockerignore` from stash object `2f8e6c4374060ba4af4e721550a3f0bde2c9a8c7`
- Read: `web/default/src/i18n/locales/_reports/_sync-report.json` from stash object `7664d0c4bd40da8fd41a84018b7ec4e7daa7ab95`
- Create outside repository: `$env:TEMP\new-api-ysr-premerge-status.txt`
- Create outside repository: `$env:TEMP\new-api-ysr-frontend-inventory.txt`

- [ ] **Step 1: Fetch the requested upstream branch without merging**

Run:

```powershell
git fetch origin main
git rev-parse origin/main
```

Expected: `84a79b6807ac1a679ca86f34c8c6f39175c294d8`. If it differs, stop before merging because the conflict set and approved design are pinned to the reviewed commit.

- [ ] **Step 2: Verify branch ancestry and absence of an active merge**

Run:

```powershell
git branch --show-current
git merge-base --is-ancestor 5350b4696018882a353ae8c5222238355b8298b3 HEAD
git rev-parse -q --verify MERGE_HEAD
git diff --name-only --diff-filter=U
```

Expected: branch is `ysr`; the ancestry command exits 0; `MERGE_HEAD` is absent; the unmerged-file command has no output.

- [ ] **Step 3: Verify the protected stash objects by immutable IDs**

Run:

```powershell
git cat-file -t 7664d0c4bd40da8fd41a84018b7ec4e7daa7ab95
git cat-file -t 2f8e6c4374060ba4af4e721550a3f0bde2c9a8c7
git stash show --stat 7664d0c4bd40da8fd41a84018b7ec4e7daa7ab95
git stash show --stat 2f8e6c4374060ba4af4e721550a3f0bde2c9a8c7
```

Expected: both objects are commits; the first contains only the i18n sync report change and the second contains only `.dockerignore`.

- [ ] **Step 4: Record and verify the user-owned working-tree baseline**

Run:

```powershell
git status --short --branch | Tee-Object -FilePath "$env:TEMP\new-api-ysr-premerge-status.txt"
git ls-files --others --exclude-standard
```

Expected untracked roots and files:

```text
.superpowers/
docs/acceptance/
docs/api/image-generation.md
docs/api/video-generation.md
docs/channel/api-doc-clmm-mall-video-generation.md
web/default/src/i18n/locales/_reports/fr.untranslated.json
web/default/src/i18n/locales/_reports/ja.untranslated.json
web/default/src/i18n/locales/_reports/ru.untranslated.json
web/default/src/i18n/locales/_reports/vi.untranslated.json
```

- [ ] **Step 5: Record the complete committed frontend customization inventory**

Run:

```powershell
$mergeBase = git merge-base HEAD origin/main
git diff --name-status $mergeBase HEAD -- web/default | Tee-Object -FilePath "$env:TEMP\new-api-ysr-frontend-inventory.txt"
(git diff --name-only $mergeBase HEAD -- web/default | Measure-Object).Count
```

Expected: 92 changed paths. Keep the inventory for Task 7; every added or modified path must have an accounted `web/**` destination.

- [ ] **Step 6: Preview the conflict set without changing the worktree**

Run:

```powershell
git merge-tree --write-tree HEAD origin/main
```

Expected: exit 1 with 25 unresolved paths: five backend files and twenty frontend/package/locale files documented in Tasks 3-6.

### Task 2: Start the Atomic Merge

**Files:**
- Modify: all paths changed by `origin/main`
- Preserve untracked: paths recorded in `$env:TEMP\new-api-ysr-premerge-status.txt`

- [ ] **Step 1: Start the reviewed merge without committing**

Run:

```powershell
git merge --no-commit --no-ff 84a79b6807ac1a679ca86f34c8c6f39175c294d8
```

Expected: Git stops on conflicts and writes `MERGE_HEAD` equal to the reviewed upstream commit.

- [ ] **Step 2: Confirm the merge parent and exact unresolved set**

Run:

```powershell
git rev-parse MERGE_HEAD
git diff --name-only --diff-filter=U
```

Expected `MERGE_HEAD`:

```text
84a79b6807ac1a679ca86f34c8c6f39175c294d8
```

Expected backend conflicts:

```text
controller/channel.go
controller/model_list_test.go
model/channel.go
model/option.go
service/task_billing_test.go
```

Expected frontend conflicts:

```text
web/bun.lock
web/default/package.json
web/src/components/counter.tsx
web/src/features/home/hooks/use-active-section.ts
web/src/features/home/hooks/use-home-page-style.ts
web/src/features/pricing/components/model-card-duration.test.tsx
web/src/features/pricing/components/pricing-toolbar-accessibility.test.tsx
web/src/features/pricing/lib/duration-pricing.test.ts
web/src/features/system-settings/general/system-info-section.tsx
web/src/features/system-settings/models/model-pricing-duration.test.ts
web/src/features/system-settings/site/index.tsx
web/src/features/system-settings/site/section-registry.tsx
web/src/features/system-settings/types.ts
web/src/i18n/locales/en.json
web/src/i18n/locales/fr.json
web/src/i18n/locales/ja.json
web/src/i18n/locales/ru.json
web/src/i18n/locales/vi.json
web/src/i18n/locales/zh-TW.json
web/src/i18n/locales/zh.json
```

If the set differs, inspect the new conflict from stages `:1:`, `:2:`, and `:3:` before editing. Abort with `git merge --abort` if the difference invalidates the approved design.

### Task 3: Compose the Backend Conflict Contracts

**Files:**
- Modify: `controller/channel.go`
- Modify: `controller/model_list_test.go`
- Modify: `model/channel.go`
- Modify: `model/option.go`
- Modify: `service/task_billing_test.go`
- Verify: `model/routing_policy_test.go`
- Verify: `controller/channel_authz_test.go`
- Verify: `controller/option_video_setting_test.go`

- [ ] **Step 1: Resolve channel controller cache behavior**

Use `git show :2:controller/channel.go` and `git show :3:controller/channel.go` to inspect both sides, then apply a focused patch. The resolved `UpdateChannel` sequence must be:

```go
model.InitChannelCache()
if !refreshRoutingPolicySnapshots(c, []int{channel.Id}) {
	return
}
if proxyChanged {
	service.InvalidateProxyClient(originProxy)
}
```

Keep upstream's `originProxy` normalization and `proxyChanged` calculation. Keep `ResetProxyClientCache()` for channel status and batch-status changes because eligibility changes can invalidate any cached client. Keep upstream's targeted invalidation for delete when the old proxy is known, with full reset only when lookup failed. Do not add a second controller-level routing refresh to delete paths because `Channel.Delete`, `BatchDeleteChannels`, and `DeleteDisabledChannel` already refresh affected routing keys transactionally.

- [ ] **Step 2: Resolve transactional channel deletion**

Keep the `ysr` `DeleteDisabledChannel` transaction and upstream's `(int64, error)` contract. The resolved function must retain this control flow:

```go
func DeleteDisabledChannel() (int64, error) {
	tx := DB.Begin()
	// Pluck matching channel IDs.
	// Delete routing targets and abilities in the same transaction.
	// Delete channels and retain result.RowsAffected.
	// Commit before refreshing the affected routing cache keys.
	return result.RowsAffected, nil
}
```

Use GORM calls already present in stage 2; do not replace them with dialect-specific SQL. Preserve rollback on every failure and return the deleted count even when post-commit cache refresh reports an error.

- [ ] **Step 3: Resolve retired theme and video option behavior**

Keep upstream's early removal of the retired key:

```go
if key == retiredThemeOptionKey {
	common.OptionMapRWMutex.Lock()
	delete(common.OptionMap, key)
	common.OptionMapRWMutex.Unlock()
	return nil
}
```

Keep video post-update synchronization, but remove the deleted theme branch:

```go
} else if configName == "billing_setting" {
	InvalidatePricingCache()
	ratio_setting.InvalidateExposedDataCache()
} else if configName == video_setting.ConfigName {
	video_setting.UpdateAndSync()
}
```

The resolved imports include `video_setting` and exclude the now-deleted theme synchronization dependency.

- [ ] **Step 4: Resolve model-list and task-billing tests by behavior**

For `controller/model_list_test.go`, keep all upstream authentication/model-list cases and all `ysr` model metadata/routing cases. For `service/task_billing_test.go`, retain:

```go
setDurationBillingContext(task)
seedTaskQuotaData(t, task)
```

alongside upstream settlement assertions that the reserved task quota is cleared. Preserve `testify/require` for fatal fixture setup and `testify/assert` for value checks. Remove imports only when `go test` proves they are unused.

- [ ] **Step 5: Format, stage, and run focused backend tests**

Run:

```powershell
gofmt -w controller/channel.go controller/model_list_test.go model/channel.go model/option.go service/task_billing_test.go
git add -- controller/channel.go controller/model_list_test.go model/channel.go model/option.go service/task_billing_test.go
git diff --name-only --diff-filter=U -- controller model service
go test ./model -run 'Test(ChannelDeletePathsCleanRoutingTargetsAndAbilities|RoutingPolicy|RefreshRoutingPolicyCache)' -count=1
go test ./controller -run 'Test(Channel|UpdateOption|RoutingPolicy|Model)' -count=1
go test ./service -run 'Test(RefundTaskQuota|UpdateTaskQuota|TaskBilling|Routing)' -count=1
```

Expected: no backend unmerged paths; all focused commands pass.

### Task 4: Adopt the Single Frontend and Reconcile Dependencies

**Files:**
- Delete: `web/default/package.json`
- Keep deleted: `web/classic/**`
- Keep deleted: `setting/system_setting/theme.go`
- Modify: `web/package.json`
- Regenerate: `web/bun.lock`
- Migrate: all 92 committed `web/default/**` customization paths to `web/**`

- [ ] **Step 1: Accept the upstream topology without restoring classic**

Run read-only checks first:

```powershell
git status --short -- web/classic setting/system_setting/theme.go web/default/package.json web/package.json
git ls-files -u -- web/default/package.json
```

Then remove only the obsolete conflicted package file:

```powershell
git rm -- web/default/package.json
```

Keep Git's upstream deletions for `web/classic/**` and `setting/system_setting/theme.go` staged. Do not remove the pre-existing untracked locale reports still under `web/default`.

- [ ] **Step 2: Merge only dependencies that still have migrated consumers**

Patch upstream `web/package.json` to retain these `ysr` dependencies in addition to upstream's dependencies:

```json
{
  "dependencies": {
    "@fontsource-variable/geist": "^5.3.0",
    "@fontsource-variable/geist-mono": "^5.3.0",
    "@fontsource/instrument-serif": "^5.3.0"
  },
  "devDependencies": {
    "happy-dom": "^20.11.1"
  }
}
```

The three font packages are required by `web/src/styles/index.css`; `happy-dom` is required by the migrated routing target editor client test. Preserve all upstream scripts, dependencies, overrides, and package metadata.

- [ ] **Step 3: Regenerate the single lockfile**

Run from `web/`:

```powershell
bun install
```

Expected: install succeeds and `web/bun.lock` contains no conflict markers. Stage only package artifacts:

```powershell
git add -- web/package.json web/bun.lock
git diff --name-only --diff-filter=U -- web/bun.lock web/default/package.json
```

Expected: neither path remains unmerged.

- [ ] **Step 4: Stage Git's conflict-free path migrations and location-only conflicts**

Verify that each location-only stage-2 file exists at its new path, then stage it:

```powershell
git add -- web/src/components/counter.tsx
git add -- web/src/features/home/hooks/use-active-section.ts web/src/features/home/hooks/use-home-page-style.ts
git add -- web/src/features/pricing/components/model-card-duration.test.tsx web/src/features/pricing/components/pricing-toolbar-accessibility.test.tsx
git add -- web/src/features/pricing/lib/duration-pricing.test.ts
git add -- web/src/features/system-settings/models/model-pricing-duration.test.ts
```

Expected: these are no longer in `git diff --name-only --diff-filter=U`.

### Task 5: Resolve Frontend Settings Semantics

**Files:**
- Modify: `web/src/features/system-settings/general/system-info-section.tsx`
- Modify: `web/src/features/system-settings/site/index.tsx`
- Modify: `web/src/features/system-settings/site/section-registry.tsx`
- Modify: `web/src/features/system-settings/types.ts`
- Verify: `web/src/features/home/hooks/use-home-page-style.ts`
- Verify: `web/src/features/system-settings/models/model-pricing-duration.test.ts`

- [ ] **Step 1: Remove the retired frontend selector and keep home style**

In `system-info-section.tsx`, keep the Base UI `Select` imports because `home.style` uses them. The schema and normalized defaults contain `home` but not `theme`:

```tsx
home: z.object({
  style: z.enum(['default', 'living-system']),
}),
```

Keep upstream's direct `for (const [key, value] of Object.entries(changedFields))` submit loop; remove the obsolete delayed frontend replacement logic. Retain the existing `Home Page Style` selector with `default` and `living-system` values.

- [ ] **Step 2: Reconcile site defaults and section wiring**

The resolved site defaults include:

```ts
const defaultSiteSettings: SiteSettings = {
  'home.style': 'default',
  Notice: '',
  // Preserve the remaining upstream fields unchanged.
}
```

The resolved system-info section builder includes:

```tsx
home: {
  style:
    settings['home.style'] === 'living-system'
      ? 'living-system'
      : 'default',
},
```

It must not pass `theme.frontend`.

- [ ] **Step 3: Reconcile shared settings types**

Remove `'theme.frontend'` from `SiteSettings`. Keep the following `ysr` fields together with all upstream fields:

```ts
export type SiteSettings = {
  'home.style': string
  // upstream site fields remain
}

export type RatioValue = number | string | DurationPrice
```

Also keep `DurationPrice`, `billing_setting.duration_price`, and both `video_setting` operation fields already introduced on `ysr`.

- [ ] **Step 4: Stage settings conflicts and run their focused tests**

Run:

```powershell
git add -- web/src/features/system-settings/general/system-info-section.tsx web/src/features/system-settings/site/index.tsx web/src/features/system-settings/site/section-registry.tsx web/src/features/system-settings/types.ts
git diff --name-only --diff-filter=U -- web/src/features/system-settings
Set-Location web
bun test src/features/system-settings/models/model-pricing-duration.test.ts
bun test src/features/model-routing/components/route-target-editor-client.test.tsx src/features/model-routing/components/route-target-editor-accessibility.test.tsx
Set-Location ..
```

Expected: no system-settings conflicts and all focused tests pass. Task 7's full TypeScript check covers the migrated `use-home-page-style.ts` hook, which has no standalone test file.

### Task 6: Merge Locales Structurally and Regenerate i18n Artifacts

**Files:**
- Modify: `web/src/i18n/locales/en.json`
- Modify: `web/src/i18n/locales/fr.json`
- Modify: `web/src/i18n/locales/ja.json`
- Modify: `web/src/i18n/locales/ru.json`
- Modify: `web/src/i18n/locales/vi.json`
- Modify: `web/src/i18n/locales/zh-TW.json`
- Modify: `web/src/i18n/locales/zh.json`
- Generate but leave unstaged: `web/src/i18n/locales/_reports/_sync-report.json`

- [ ] **Step 1: Merge each locale as JSON data**

For each locale, use stage 1 as the base, retain all keys added or changed in stage 2, and retain all keys added or changed in stage 3. When both sides changed the same key, prefer the translation that matches the resolved single-frontend UI: remove classic-selector text only when it has no remaining caller; preserve routing, duration billing, `home.style`, and video-policy text.

Do not concatenate conflict blocks. Use `apply_patch` so every result is a single valid flat JSON object.

- [ ] **Step 2: Parse every locale before running the generator**

Run:

```powershell
Get-ChildItem web\src\i18n\locales\*.json | ForEach-Object {
  Get-Content -Raw -LiteralPath $_.FullName | ConvertFrom-Json | Out-Null
}
```

Expected: exit 0 with no parser errors.

- [ ] **Step 3: Run i18n synchronization from the new root**

Run:

```powershell
Set-Location web
bun run i18n:sync
Set-Location ..
```

Expected: synchronization succeeds. Stage the seven locale JSON files and any source-controlled i18n key file changed by the generator, but intentionally leave `_reports/_sync-report.json` unstaged:

```powershell
git add -- web/src/i18n/locales/en.json web/src/i18n/locales/fr.json web/src/i18n/locales/ja.json web/src/i18n/locales/ru.json web/src/i18n/locales/vi.json web/src/i18n/locales/zh-TW.json web/src/i18n/locales/zh.json web/src/i18n/static-keys.ts
git diff --name-only --diff-filter=U -- web/src/i18n
git status --short -- web/src/i18n/locales/_reports/_sync-report.json
```

Expected: no locale conflicts. The sync report may appear only as an unstaged modification; it must not be in `git diff --cached --name-only`.

### Task 7: Audit Feature Migration and Verify the Whole Merge

**Files:**
- Verify: all paths in `$env:TEMP\new-api-ysr-frontend-inventory.txt`
- Verify: all staged merge paths
- Preserve unstaged: `web/src/i18n/locales/_reports/_sync-report.json`

- [ ] **Step 1: Account for all 92 frontend customizations**

Run:

```powershell
$mergeBase = git merge-base ORIG_HEAD origin/main
$missing = @()
git diff --name-only $mergeBase ORIG_HEAD -- web/default | ForEach-Object {
  $destination = $_ -replace '^web/default/', 'web/'
  if (-not (git ls-files --error-unmatch -- $destination 2>$null)) {
    $missing += "$_ -> $destination"
  }
}
$missing
```

Expected: no missing added/modified destination. Review deletions and package-path exceptions separately: `web/default/package.json` is replaced by `web/package.json`; upstream `web/classic` and theme files remain deleted.

- [ ] **Step 2: Confirm repository and conflict integrity**

Run:

```powershell
git diff --name-only --diff-filter=U
git grep -n -E '^(<<<<<<<|=======|>>>>>>>)' -- ':!docs/superpowers/plans/*'
git diff --check
git diff --cached --check
git status --short
```

Expected: no unmerged files, conflict markers, or whitespace errors. Status shows staged merge changes, the intentionally unstaged sync report if generated, and the original untracked user files.

- [ ] **Step 3: Run focused routing, Seedance, billing, and option tests**

Run:

```powershell
go test ./pkg/modelrouting ./model ./middleware ./controller ./service -run 'Test.*(Routing|Route|Duration|TaskBilling|RefundTaskQuota|Video|Channel)' -count=1
go test ./relay/... -run 'Test.*(Seedance|Ark|Duration|Routing|Native)' -count=1
go test ./router -run 'Test.*(Routing|Video)' -count=1
```

Expected: all focused tests pass.

- [ ] **Step 4: Run the full Go suite**

Run:

```powershell
go test ./...
```

Expected: exit 0.

- [ ] **Step 5: Run the complete frontend verification from `web/`**

Run:

```powershell
Set-Location web
bun run typecheck
bun test
bun run lint
bun run format:check
bun run build
Set-Location ..
```

Expected: every command exits 0. Do not weaken tests, lint, formatting, or TypeScript settings to obtain a pass.

- [ ] **Step 6: Verify the final index excludes user-owned files**

Run:

```powershell
git diff --cached --name-only | Select-String -Pattern '^\.superpowers/|^docs/acceptance/|^docs/api/|^docs/channel/|^web/default/src/i18n/locales/_reports/|^web/src/i18n/locales/_reports/_sync-report\.json$'
```

Expected: no output.

### Task 8: Create and Validate the Local Merge Commit

**Files:**
- Commit: staged merge result only
- Leave unstaged/untracked: protected user files and generated sync report

- [ ] **Step 1: Inspect the staged semantic summary**

Run:

```powershell
git diff --cached --stat
git diff --cached -- controller/channel.go model/channel.go model/option.go service/task_billing_test.go web/package.json web/src/features/system-settings
```

Expected: combined backend behavior, single frontend, migrated custom features, and no protected user file.

- [ ] **Step 2: Create the merge commit**

Run:

```powershell
git commit -m "Merge origin/main into ysr"
```

Expected: one merge commit is created without staging the sync report or original untracked files.

- [ ] **Step 3: Validate merge ancestry and branch state**

Run:

```powershell
git rev-parse HEAD^2
git merge-base --is-ancestor 5350b4696018882a353ae8c5222238355b8298b3 HEAD
git show --summary --format=fuller HEAD
git status --short --branch
```

Expected: `HEAD^2` is `84a79b6807ac1a679ca86f34c8c6f39175c294d8`; the functional baseline remains an ancestor; the branch is ahead of `fork/ysr`; no push has occurred.

### Task 9: Restore and Prove Protected Working Changes

**Files:**
- Restore unstaged: `.dockerignore`
- Compare/migrate unstaged: `web/src/i18n/locales/_reports/_sync-report.json`
- Preserve untracked: all Task 1 user files

- [ ] **Step 1: Reapply the exact `.dockerignore` stash object**

Run:

```powershell
git stash apply 2f8e6c4374060ba4af4e721550a3f0bde2c9a8c7
git diff -- .dockerignore
```

Expected: `.dockerignore` is an unstaged modification containing the user entries for `.worktrees`, `.agents`, `.codex`, `.claude`, `.opencode`, `.playwright-cli`, `.superpowers`, `.zcode`, `output`, `temp`, and `tmp`, while retaining upstream entries.

- [ ] **Step 2: Compare the old-path report stash with the regenerated new-path report**

Run:

```powershell
git show 7664d0c4bd40da8fd41a84018b7ec4e7daa7ab95:web/default/src/i18n/locales/_reports/_sync-report.json | ConvertFrom-Json | Out-Null
Get-Content -Raw -LiteralPath 'web\src\i18n\locales\_reports\_sync-report.json' | ConvertFrom-Json | Out-Null
git stash show -p 7664d0c4bd40da8fd41a84018b7ec4e7daa7ab95
git diff -- web/src/i18n/locales/_reports/_sync-report.json
```

Expected: both parse successfully. The new report reflects the merged source tree and remains unstaged. Do not apply the old stash over the new path because upstream key counts changed; keep the stash object as a recovery checkpoint unless semantic equivalence can be demonstrated field by field.

- [ ] **Step 3: Verify the final user-owned state and stash recoverability**

Run:

```powershell
git status --short --branch
git stash list
git ls-files --others --exclude-standard
git log -1 --oneline --decorate
```

Expected:

- `.dockerignore` and the new-root sync report are unstaged user changes.
- The original untracked paths from Task 1 still exist and are untracked.
- The i18n stash remains until equivalence is certain.
- The `.dockerignore` stash may be dropped only after its restored diff is confirmed; dropping it is not required for merge acceptance.
- `ysr` contains the local merge commit and has not been pushed.

### Task 10: Produce the Verification Handoff

**Files:**
- Do not create or stage a repository report unless the user separately requests one.

- [ ] **Step 1: Summarize evidence in the final response**

Report:

```text
merge commit and both parent hashes
resolved conflict count and final unmerged count
single-frontend migration result and 92-path audit result
focused and full Go test results
frontend typecheck, test, lint, format, and build results
protected stash and untracked-file state
whether any unrelated pre-existing failure remains
explicit confirmation that no push occurred
```

- [ ] **Step 2: Do not claim completion without fresh command output**

Before the final response, invoke `superpowers:verification-before-completion` and re-run its required evidence checks. If any required command failed, report the exact failure instead of describing the merge as complete.
