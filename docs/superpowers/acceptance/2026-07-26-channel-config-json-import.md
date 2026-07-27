# Channel Configuration JSON Import Acceptance

## Source Integrity

The original workbook was read after fixture generation. Its SHA-256 is `90525B07A0B95F2F9DBF6FEE269531616393C6721FDD579BE165D6EE43B31767`.

The corrected V1 workbook is generated separately. The generated V2 template has no formula errors and was rendered for all ten sheets. Its inspected SHA-256 is `79e5570ee0c59b269678001f1892971645498855dc714680997d7681f60d6db6`.

## Converter Contract

`bun run converter:test` passed 30 tests. The V1 JSON fixture has payload SHA-256 `40a2a80c713cf1773d53b6d2459a5305a1ed31c61cf5982b5d6456d9f56ac5bd` and these publishable counts:

| Entity | Count |
| --- | ---: |
| Channel masters | 9 |
| Channel lines | 12 |
| Model SKUs | 9 |
| Sale proposals | 16 |
| Cost rule drafts | 104 |
| Model mappings | 104 |
| Route blueprints | 104 |
| Sources | 13 |
| Unresolved variants | 1 |

The unresolved record is `CH-MEGABYAI/videos-standard`. Its 17 non-line-identifiable source rows are excluded from publishable cost, mapping, and route entities.

## Publication Evidence

`TestConfigImportV1FixturePublishesDisabledConfigurationE2E` verified idempotent re-upload, all 12 confirmed line bindings, explicit exclusion of the unresolved MegaByAI record, staging to `ready`, publication, post-publication audit hashes, and rejection of a second publish.

The test also verified that every created channel, routing policy, and route target remains disabled. `TestProfitRoutingUsesTargetCostVariantForMarginAndAttemptE2E` verified that an enabled route resolves its exact `720p` cost variant without falling back to the `480p` rule. `TestPublishConfigImportBatchRollsBackSaleMappingAndCostWhenRouteFails` verified transaction rollback and the `publish_failed` state.

`TestPublishConfigImportBatchIgnoresUnrelatedActiveCostRule`, `TestPublishConfigImportBatchRejectsChangedAffectedCostRule`, and `TestConfigImportBaselineTracksOnlyPublishedPricingMappingsAndRoutes` verify that the optimistic baseline covers only the costs, model-price fields, channel mappings, and routing policies this batch will publish. Unrelated changes do not produce a stale batch; a change to an affected value does.

## Browser And Database Evidence

`bunx playwright test -c playwright.config-import.config.ts` passed all six Chromium desktop and mobile tests. The converter test uses the built `file:` artifact, confirms no non-local network requests, and confirms empty localStorage and IndexedDB.

`bun run i18n:sync` reports zero missing or extra keys in all seven locales. The import wizard's publication and recovery messages are translated in every supported locale.

`powershell -ExecutionPolicy Bypass -File scripts/config-import-db-matrix.ps1` passed the MySQL 5.7 and PostgreSQL 9.6 migration contracts, plus the SQLite service and import E2E suites. The test-only MySQL and PostgreSQL Docker volumes were removed once while repairing their initial stale state and then recreated by the matrix.

## Security Evidence

The credential scan over `e2e/testdata`, converter fixtures, and this record returned no matches. The placeholder scan over this record and `docs/config-import.md` returned no matches.

## Repository Diagnostics

The import-focused checks, browser tests, full Go test suite, and database matrix pass. The full frontend suite passes with `bun test --parallel=1`; serialization is required because the existing tests share mutable Happy DOM globals. Repository-wide `go vet ./...` and `bun run lint` still report pre-existing diagnostics in unrelated packages; they do not originate in the channel configuration import files.
