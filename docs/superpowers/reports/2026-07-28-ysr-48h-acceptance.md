# YSR 48-Hour Acceptance Report

## Scope

- Branch: `ysr`
- Accepted head: `dc91933be0272bbc10061b4cb020ed8d63efbc6a`
- Review window: 2026-07-26 00:51 through 2026-07-27 23:40, Asia/Shanghai
- Commits in window: 112
- Local runtime: `new-api-local` on `http://127.0.0.1:3000`

## Acceptance Checklist

| Area | Recent change set | Acceptance evidence | Result |
| --- | --- | --- | --- |
| Cost variants and profit routing | `20ec2f74f` through `6a227f9f7` | Exact route-target `cost_variant_key` selection, strict no-fallback behavior, and accounting E2E coverage | Pass |
| Seedance supplier channels | Lucen, MegaByAI, Cangyuan, Paipu, and Secure commits | Task-only registration, request profiles, capability routing, Secure group rules, and frontend forms | Pass |
| Offline converter and JSON import | `e2ea8d316` through `18b81d802`, plus acceptance commits | Credential-free workbook conversion, deterministic JSON, staged bindings, publish transaction, default-disabled configuration, routing and pricing proposals | Pass |
| Import recovery and audit | publish, baseline, cache-refresh, and rollback tests | Atomic publish rollback, post-publish audit hash, stale detection, cache-only retry, and failed publish state | Pass |
| YSR channel-type migration | `dc91933be` | `59-66 -> 61-68 -> 200-207` migration, task-platform migration, converter fixture, and frontend registries | Pass |
| Branch governance | `RULE.md` | Reserved channel-type range `200-299`; explicit upstream-integration authorization rule | Pass |
| Upstream snapshot already in history | `034f8f179` | Recorded as an existing historical merge; no `main` or `origin/main` merge was performed in this acceptance run | Noted |

## Corrective Actions During Acceptance

1. Model mappings now use `canonical_model` normalized to the public runtime model in both publication and stale-baseline scope. A supplier alias can no longer leave pricing, routing, and channel model mapping under different keys.
2. A bound line without credential confirmation now persists `BINDING_CREDENTIALS_UNCONFIRMED` as an open error and keeps the batch in `binding`. The line does not materialize a cost draft; after confirmation and restaging, the issue resolves and the batch can become `ready`.
3. Two frontend regression fixtures still used the retired types `61` and `68`. Dimensio now uses `200`; the Secure fixture uses `SECURE_CHANNEL_TYPE`, so its edit-lock contract follows future constant changes.

## Runtime And Browser Acceptance

The local `new-api-local` images were rebuilt from the accepted source and its containers were force-recreated. The application, MySQL, Redis, and video-metadata services all reported healthy. `GET /api/status` returned `success: true`.

Two no-cache image-build attempts were interrupted by external download failures: Debian `tzdata` returned HTTP 500 with an unexpected EOF, then `proxy.golang.org` returned EOF while downloading a Go module. The successful rebuild reused only previously verified dependency layers and recompiled the current source layer; it produced `new-api:local` and `new-api-video-metadata:local` before container replacement.

`bunx playwright test -c playwright.config-import.config.ts` passed all six Chromium checks:

- Desktop and mobile conversion of the corrected V1 workbook.
- No converter network traffic or browser persistence.
- Converter layout and accessible controls at both viewports.
- Unauthenticated access guard for the configuration-import route at both viewports.

## Verification Evidence

| Command | Result |
| --- | --- |
| `go test ./... -count=1` | Pass |
| `go test ./service ./controller ./router -count=1` | Pass |
| `go test ./e2e -run 'Test(ConfigImport|ProfitRoutingUsesTargetCostVariantForMarginAndAttemptE2E)' -count=1` | Pass |
| `bun run converter:test` | 31 pass, 0 fail |
| `bun test --parallel=1 --timeout=15000` | 288 pass, 0 fail |
| `bun run typecheck` | Pass |
| `bun run format:check` | Pass |
| `bun run build` and `bun run converter:build` | Pass |
| `bunx playwright test -c playwright.config-import.config.ts` | 6 pass, 0 fail |

The frontend suite is deliberately serialized because its existing Happy DOM suites mutate shared browser globals. The explicit 15-second per-test allowance prevents false timeouts after prior suites while retaining serial execution; the two affected tests both pass independently and in the complete suite.

## Residual Diagnostic Baseline

`go vet ./...` remains nonzero for pre-existing diagnostics outside this change set, including value-copying `CustomEvent` methods, IPv6 address formatting in `common/email_test.go`, and unreachable placeholders in legacy relay adaptors. The same `CustomEvent`, email-test, and Palm adaptor diagnostics exist in `origin/main`. They are not modified or suppressed by this acceptance work.

## Continuation: Published-Configuration Runtime Check

This continuation used the rebuilt `new-api-acceptance` stack at
`http://127.0.0.1:3001` with MySQL. It covered the post-publication path that
cannot be exercised by the credential-free import fixture alone.

### Corrective Action

The first browser publication committed its database transaction, but returned
an HTTP 500 while refreshing option caches. The MySQL error was caused by an
unquoted reserved `options.key` column in `RefreshPublishedConfig`. The
publication transaction remained atomic and the batch was already published;
the failed cache refresh correctly recorded `CACHE_REFRESH_PENDING`.

`RefreshPublishedConfig` now uses GORM `clause.Eq` for the reserved column. A
new real-MySQL regression test covers the post-commit cache refresh path. The
administrator retry endpoint was then invoked for batch `1`; it completed
without replaying the publication and resolved the pending-cache issue.

### Acceptance Results

| Check | Observed result |
| --- | --- |
| Published batch | Batch `1` is `published`; one publish audit has outcome `published`. |
| Channel bindings | All 12 bindings point to channels `13` through `24` and have credential-confirmation metadata. |
| Disabled-by-default safety | All 12 imported mock channels remain `status = 2`; all 3 routing policies and all 104 route targets are disabled. |
| Cost and routing materialization | 104 `config_import` cost rules are active across the 12 bound channels; 3 routing policies and 104 route targets exist. |
| Cache recovery | The sole `CACHE_REFRESH_PENDING` issue is `resolved` after `POST /api/config-import/batches/1/refresh-cache`. |
| Mock video request | `POST /v1/video/generations` for `jimeng-video-seedance-2.0-vip` returned HTTP `503` with `model_not_found`: no available channel in `default`. |
| No execution or charge | The browser task-log page showed zero rows; MySQL also showed zero tasks and zero consumption logs for the acceptance user. |

### Fresh Verification

| Command | Result |
| --- | --- |
| `go test ./... -count=1` | Pass |
| `go test ./service -count=1` | Pass |
| `TEST_MYSQL_DSN=... go test ./service -run '^(TestCaptureConfigImportBaselineLoadsOptionRowsOnMySQL|TestPublishConfigImportSaleOptionsLoadsOptionRowsOnMySQL|TestRefreshPublishedConfigLoadsOptionRowsOnMySQL)$' -count=1` | Pass |
| `go test ./e2e -run TestConfigImportV1FixturePublishesDisabledConfigurationE2E -count=1` | Pass |
| `bun run build` | Pass |

The imported configuration is ready for credential entry and controlled
enablement. A real video-generation test requires selecting a bound channel,
entering its real upstream credential, and explicitly enabling that channel
and its routing policy; those production-affecting actions were intentionally
not performed in this acceptance environment.
