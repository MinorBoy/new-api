# Origin Main Into Ysr Merge Conflict Resolution Design

## Goal

Merge the current `origin/main` into the local `ysr` branch while preserving the behavior introduced by the `ysr` second-development commits and adopting upstream's current repository structure.

The merge is designed against these reviewed functional commits:

```text
ysr:         5350b4696018882a353ae8c5222238355b8298b3
origin/main: 84a79b6807ac1a679ca86f34c8c6f39175c294d8
```

At design time, `ysr` contains 133 commits not in `origin/main`, and `origin/main` contains 15 commits not in `ysr`. The design and implementation-plan documentation commits may advance the local `ysr` tip before the merge, but they do not change this functional baseline. The result is a local merge commit on `ysr`. Pushing the result is outside this task.

## Safety Invariants

The following state must survive the merge:

- The two protected stashes for `.dockerignore` and the i18n sync report.
- All pre-existing untracked files under `.superpowers/`, `docs/acceptance/`, `docs/api/`, `docs/channel/`, and `web/default/src/i18n/locales/_reports/`.
- All Seedance capability routing, duration billing, Ark compatibility, channel UI, routing target mapping, and routing target automatic naming behavior already committed on `ysr`.
- Upstream fixes included in the reviewed `origin/main` commit.

No broad checkout, reset, clean, recursive deletion, or whole-tree `ours`/`theirs` resolution is permitted. Only merge-conflict paths and files explicitly required for the frontend migration, generated artifacts, formatting, or conflict-related fixes may be edited or staged. The protected stashes remain available until their working-tree equivalents have been restored and checked.

Before restarting the merge, record the branch name, `HEAD`, `origin/main`, stash object IDs, untracked paths, and the absence of unmerged entries. The only expected movement from the reviewed `ysr` functional baseline is the approved design and implementation-plan documentation. If any other value differs, re-evaluate the affected step before continuing.

## Repository Structure Decision

Accept upstream's single-frontend structure as the target layout:

```text
before on ysr                 after merge
web/default/package.json  ->  web/package.json
web/default/bun.lock      ->  web/bun.lock
web/default/src/**        ->  web/src/**
web/classic/**            ->  deleted
```

The removal of `web/classic` is intentional. The merge must not recreate it or retain configuration that builds it. All committed `ysr` changes under `web/default` are migrated into the corresponding paths under `web`.

Git's rename detection is an aid, not the acceptance criterion. Build a complete inventory from the merge base to `ysr` for `web/default/**`, then account for every added or modified `ysr` path at its new `web/**` location. Files deleted by `ysr` require a semantic check against the upstream replacement before carrying the deletion forward.

## Conflict Resolution

Restart the merge with `git merge --no-commit --no-ff origin/main`. Resolve each conflict from the three-way stages and surrounding call sites, preserving both sides when they implement independent behavior.

Known backend resolutions are:

| File | Required result |
| --- | --- |
| `controller/channel.go` | Keep routing-policy snapshot refreshes. Preserve upstream targeted proxy invalidation when proxy configuration changes and the broader proxy-cache reset required by channel status changes. Avoid duplicate invalidation. |
| `model/channel.go` | Keep upstream's `(int64, error)` deletion result and actual deleted count. Also delete routing targets owned by the removed channels and refresh the routing cache only after a successful database operation. |
| `model/option.go` | Keep upstream's retirement of `theme.frontend`, including removal from the runtime option map. Do not retain the deleted theme configuration registration or theme synchronization. Preserve the independent `home.style` option and the `ysr` video-setting post-update synchronization. |
| `controller/model_list_test.go` | Preserve the union of meaningful test fixtures and assertions. Let compilation and `gofmt` identify unused imports rather than dropping either side's coverage by default. |
| `service/task_billing_test.go` | Preserve duration-billing context and quota-data fixtures together with upstream assertions that settled tasks clear reserved quota. |

For any newly appearing backend conflict, first state each side's observable contract, then create the smallest combined implementation that preserves both contracts. Database changes must continue to support SQLite, MySQL, and PostgreSQL. JSON operations must continue to use the project's `common` wrappers.

Known frontend content conflicts fall into three groups:

1. Direct path migrations whose `ysr` content has been mapped from `web/default/src/**` to `web/src/**`. Reconcile those files against upstream edits at the new path.
2. Package metadata and `web/bun.lock`. Merge required `ysr` dependencies and scripts into upstream `web/package.json`, remove the obsolete `web/default/package.json`, and regenerate the single lockfile with Bun. Do not hand-merge lockfile conflict markers.
3. Locale JSON files. Parse and merge them structurally, keeping upstream keys and all routing/duration/Seedance keys. The English-source-key convention and all supported locales remain intact. Run the project's i18n synchronization after the structural merge.

Frontend conflict resolution must preserve the current component conventions in `web/AGENTS.md`, existing Base UI/Tailwind styling, accessibility behavior, and i18n calls. Generated files such as the route tree and lockfile are regenerated through project tooling after their source files are correct.

Because the classic frontend is removed, frontend settings conflicts must remove `theme.frontend` fields and controls while retaining the independent `home.style` selector and its `default`/`living-system` values. The obsolete `setting/system_setting/theme.go` deletion is accepted. Backend validation continues to reject attempts to restore a non-default retired theme.

## Protected Working Changes

The two stashes are recovery checkpoints, not merge inputs.

- Reapply the `.dockerignore` change after the merge commit and verification so it returns as an unstaged user change. Confirm that its ignore entries are present without discarding any new upstream entries.
- Do not blindly restore the old-path i18n sync report. Run i18n synchronization from the new `web/` root, compare the new report with the stashed report, and retain the equivalent report at `web/src/i18n/locales/_reports/_sync-report.json` as an unstaged user change. Preserve any user-only report data when the report schema permits it.
- Keep both stash objects until the restored working-tree diffs have been inspected. Drop only the exact stash whose contents have been restored equivalently; if equivalence is uncertain, keep that stash and report the uncertainty.

The untracked locale reports under the old `web/default` path are left untouched. They are not moved, removed, staged, or committed as part of the merge.

## Verification

Verification proceeds from repository integrity to focused behavior and then the full build:

1. Confirm the index has no unmerged entries and scan tracked source for conflict markers.
2. Compare the complete `ysr` frontend-change inventory with the migrated `web/**` result. Every committed customization must be accounted for.
3. Run `gofmt` on resolved Go files and focused Go tests for channel deletion/cache behavior, model listing, routing, task billing, and Seedance relay behavior.
4. Run `go test ./...`.
5. From `web/`, run `bun install`, `bun run i18n:sync`, `bun run typecheck`, `bun test`, `bun run lint`, and `bun run build`.
6. Inspect `git diff --check`, the final staged merge diff, and `git status` before committing.
7. After the local merge commit, restore and verify the protected user changes as described above. Confirm that only the expected unstaged and untracked user files remain.

A failing check is investigated and fixed within the merge when it results from conflict resolution or path migration. Pre-existing failures unrelated to the merge are documented with the exact command and evidence; they are not hidden by weakening tests or lint rules.

## Recovery

Before the merge commit exists, `git merge --abort` is the recovery path. The recorded baseline and untouched stash objects must return the branch to the reviewed state.

After the merge commit exists, do not rewrite or delete it automatically. If verification later reveals a semantic error, fix it with a follow-up commit or ask for explicit approval before reverting the merge. No force push is permitted.

## Acceptance Criteria

The merge is accepted only when:

- `ysr` contains a local merge commit whose first parent is the reviewed pre-merge `ysr` tip and whose second parent is the reviewed `origin/main` commit. The original `ysr` functional baseline remains in the first-parent ancestry.
- The repository uses the upstream single-frontend layout and contains no tracked `web/classic` or obsolete `web/default` application files.
- The committed `ysr` routing, Seedance, duration-billing, Ark, and UI behavior is present at the new paths and its focused tests pass.
- Upstream channel cache, authentication, task settlement, model listing, and other incoming fixes remain represented.
- There are no unresolved conflict markers, unmerged index entries, or unaccounted frontend customizations.
- Full Go and frontend verification passes, or any unrelated pre-existing failure is explicitly evidenced.
- The protected user changes and untracked files remain recoverable and are not included in the merge commit.
- The branch has not been pushed.
