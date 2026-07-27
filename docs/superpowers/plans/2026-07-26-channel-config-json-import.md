# Channel Configuration JSON Import Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build an offline Excel-to-JSON converter and a staged, auditable new-api import workflow that publishes channel mappings, cost variants, sale pricing, and model routing atomically without exposing credentials.

**Architecture:** The browser-only converter owns Excel parsing and emits a deterministic, versioned, credential-free JSON contract. The backend strictly revalidates that contract, stores resumable import batches, binds logical channel lines to real disabled channels, materializes drafts, and publishes reviewed cost, pricing, and routing changes in one transaction; runtime routing carries a `cost_variant_key` so every selected target resolves exactly one supplier cost rule. The existing source workbook remains untouched, while a corrected v1 regression fixture and a structured v2 template make the contract reproducible.

**Tech Stack:** Go 1.22+, Gin, GORM v2, SQLite/MySQL/PostgreSQL, React 19, TypeScript, Rsbuild, TanStack Router/Query, Base UI, Tailwind CSS, Zod, ExcelJS, zip.js, decimal.js, json-canonicalize, Vitest, Playwright, Bun, `@oai/artifact-tool`.

---

## Delivery Boundaries

The approved behavior is defined by `docs/superpowers/specs/2026-07-26-channel-config-json-import-design.md`. Implementation must preserve these invariants throughout every phase:

- Excel bytes never reach new-api; only canonical JSON is uploaded.
- Excel, JSON, database items, reports, logs, and audits never contain API keys, tokens, cookies, authorization headers, or secrets.
- New channels, policies, and route targets are disabled by default.
- The cost business key is `(channel_id, billable_upstream_model, cost_variant_key)`; `default` preserves existing behavior.
- A route target binds one `cost_variant_key`; strict mode never falls back to another variant.
- Upload/stage/publish is idempotent by `payload_sha256` and entity hash.
- Cost drafts may be materialized during staging; active cost, sale pricing, mappings, and routing change only in the publish transaction.
- Publish order is cost rules, sale pricing, model mappings, routing definitions, then reviewed enablement flags.
- Database commit happens before cache refresh; cache failures create `CACHE_REFRESH_PENDING` and never rerun the publish transaction.
- SQLite, MySQL 5.7.8+, and PostgreSQL 9.6+ must behave identically.

## File Map

### Cost Variant Core

- Modify `model/channel_model_cost_rule.go`: add normalized variant identity and transaction-aware activation.
- Modify `model/routing_policy.go`: persist route-target variants and expose transaction-aware replacement.
- Modify `model/main.go`: run cross-database variant/index/backfill migrations before normal auto-migration.
- Modify `model/cost_accounting.go`, `types/cost_accounting.go`: snapshot the selected variant in attempts and profit rechecks.
- Modify `service/cost_rule.go`, `service/model_routing.go`, `service/profit_routing.go`, `service/routing_policy.go`: key caches, coverage, matching, DTOs, and audit context by variant.
- Modify `pkg/modelrouting/types.go`, `model/routing_policy_cache.go`: carry the selected variant in runtime policy snapshots.
- Modify `controller/cost_accounting.go`: accept and return `cost_variant_key`.
- Modify `web/src/features/cost-accounting/*` and `web/src/features/model-routing/*`: edit and display variants.

### Import Backend

- Create `types/config_import.go`: canonical document and import-domain enums.
- Create `dto/config_import.go`: admin API requests and responses.
- Create `model/config_import.go`: batch, item, binding, issue, resolution, and audit persistence.
- Create `service/config_import_schema.go`: strict contract, limits, secret scan, normalization, and server recomputation.
- Create `service/config_import.go`: upload, idempotency, list/detail, state transitions, and baseline capture.
- Create `service/config_import_stage.go`: bindings, resolutions, proposal generation, and validation gates.
- Create `service/config_import_publish.go`: ordered locking, stale detection, atomic publication, and post-commit refresh.
- Create `controller/config_import.go`, `router/config-import-router.go`, `service/authz/resources_config_import.go`: permissioned admin API.
- Modify `common/json.go`: add strict decoding through the approved JSON wrapper.
- Modify `model/option.go`: split transaction persistence from post-commit in-memory option refresh.
- Modify `controller/channel.go` and channel frontend API/types: return created channel IDs so the import wizard can bind newly created disabled channels.

### Admin Wizard

- Create `web/src/features/config-import/`: schemas, API hooks, state derivation, seven wizard steps, diffs, and tests.
- Create `web/src/routes/_authenticated/config-import/index.tsx`: permission-protected route.
- Modify `web/src/lib/admin-permissions.ts`, `web/src/hooks/use-sidebar-data.ts`, `web/src/hooks/use-sidebar-config.ts`: permission and navigation registration.
- Modify all seven `web/src/i18n/locales/*.json` files and generated `web/src/routeTree.gen.ts`.

### Static Converter And Fixtures

- Create `web/rsbuild.converter.config.ts`, `web/src/channel-config-converter/`, and converter scripts in `web/package.json`.
- Add `exceljs`, `@zip.js/zip.js`, `decimal.js`, and `json-canonicalize` dependencies.
- Create `web/scripts/build-channel-config-fixtures.mjs`: produce the corrected v1 fixture and structured v2 template using stable business IDs.
- Add `web/src/channel-config-converter/__fixtures__/channel-config-v1-corrected.xlsx` and `docs/templates/channel-config-v2.xlsx`.
- Preserve `outputs/019f9dbb-4e5d-7933-8531-d38e417ec068/渠道模型成本与利润模板-更新.xlsx` byte-for-byte.

## Phase 1: Cost Variant Foundation

### Task 1: Persist Cost Variants Across All Databases

**Files:**
- Modify: `model/channel_model_cost_rule.go`
- Modify: `model/routing_policy.go`
- Modify: `model/main.go`
- Modify: `types/cost_accounting.go`
- Create: `model/cost_variant_migration_test.go`
- Modify: `model/channel_model_cost_rule_test.go`

- [ ] **Step 1: Write failing model and migration tests**

Add tests that create two version-1 rules for the same channel/model with variants `480p` and `720p`, reject a duplicate version within `480p`, backfill blank legacy variants to `default`, and verify route targets receive `default` after migration. The core assertion is:

```go
require.NoError(t, CreateCostRuleDraft(&ChannelModelCostRule{
	ChannelID: 7, BillableUpstreamModel: "videos-fast", CostVariantKey: "480p", Version: 1,
	Status: string(types.CostRuleDraft),
}))
require.NoError(t, CreateCostRuleDraft(&ChannelModelCostRule{
	ChannelID: 7, BillableUpstreamModel: "videos-fast", CostVariantKey: "720p", Version: 1,
	Status: string(types.CostRuleDraft),
}))
require.Error(t, CreateCostRuleDraft(&ChannelModelCostRule{
	ChannelID: 7, BillableUpstreamModel: "videos-fast", CostVariantKey: "480p", Version: 1,
	Status: string(types.CostRuleDraft),
}))
```

- [ ] **Step 2: Run the focused tests and confirm the new contract fails**

Run: `go test ./model -run 'TestCostVariant|TestChannelModelCostRule' -count=1`

Expected: FAIL because `CostVariantKey` does not exist and the old unique index still uses three columns.

- [ ] **Step 3: Add fields, normalization, and explicit migration**

Add these fields without a GORM default tag:

```go
CostVariantKey string `json:"cost_variant_key" gorm:"type:varchar(64);not null;uniqueIndex:idx_cost_rule_version,priority:3;index"`
Version        int    `json:"version" gorm:"uniqueIndex:idx_cost_rule_version,priority:4"`
```

Add `CostVariantKey string` to `RouteTarget`. Implement `types.NormalizeCostVariantKey(string) (string, error)` with the exact regex `^[a-z0-9][a-z0-9._-]{0,63}$`, mapping blank input to `types.DefaultCostVariantKey` (`default`). Add `migrateCostVariantKeys()` in `model/main.go` that checks whether the tables exist, drops the old `idx_cost_rule_version`, adds/backfills both columns through GORM-compatible operations, and recreates the four-column index. Call it before `AutoMigrate` in both `migrateDB` and `migrateDBFast`.

- [ ] **Step 4: Make activation variant-scoped**

Change the locked business-key query to:

```go
lockForUpdate(tx).
	Where(
		"channel_id = ? AND billable_upstream_model = ? AND cost_variant_key = ?",
		candidate.ChannelID,
		candidate.BillableUpstreamModel,
		candidate.CostVariantKey,
	).
	Order("id ASC").Find(&businessRules)
```

Normalize the variant before `Create`, and ensure retiring an active rule only affects the same extended key.

- [ ] **Step 5: Run model tests and commit**

Run: `go test ./model -run 'TestCostVariant|TestChannelModelCostRule|TestCostAccountingMigration' -count=1`

Expected: PASS, including the legacy `default` backfill and two distinct version-1 variants.

```bash
git add model/channel_model_cost_rule.go model/routing_policy.go model/main.go types/cost_accounting.go model/cost_variant_migration_test.go model/channel_model_cost_rule_test.go
git commit -m "feat: persist channel cost variants"
```

### Task 2: Key Cost Rule Services And Caches By Variant

**Files:**
- Modify: `service/cost_rule.go`
- Modify: `service/cost_rule_test.go`
- Modify: `service/cost_rule_batch_test.go`
- Modify: `controller/cost_accounting.go`
- Modify: `controller/cost_accounting_test.go`
- Modify: `web/src/features/cost-accounting/types.ts`
- Modify: `web/src/features/cost-accounting/api.ts`
- Modify: `web/src/features/cost-accounting/components/cost-rule-drawer.tsx`
- Modify: `web/src/features/cost-accounting/components/channel-cost-drawer.tsx`
- Modify: `web/src/features/cost-accounting/components/__tests__/channel-cost-drawer.test.tsx`

- [ ] **Step 1: Add failing service and API tests**

Change the public identities to include a variant:

```go
type CostRuleCandidate struct {
	ChannelID             int
	BillableUpstreamModel string
	CostVariantKey        string
}

type PredictedCoverageInput struct {
	ChannelID             int
	PredictedUpstreamModel string
	CostVariantKey        string
	Authoritative         bool
	RequestPath           string
	TaskPlatform          constant.TaskPlatform
}
```

Test that `ActiveCostRule(7, "videos-fast", "480p", true)` and the `720p` lookup return different rules, while blank input resolves `default`. Test controller list/create responses always include `cost_variant_key`.

- [ ] **Step 2: Run focused backend tests and confirm failure**

Run: `go test ./service ./controller -run 'Test.*CostRule.*Variant|Test.*CostCoverage.*Variant' -count=1`

Expected: FAIL because cache keys and queries only use channel/model.

- [ ] **Step 3: Update service queries, cache keys, and invalidation**

Use these signatures consistently:

```go
func ActiveCostRule(channelID int, billableModel, costVariantKey string, authoritative bool) (*model.ChannelModelCostRule, error)
func ActiveCostRules(candidates []CostRuleCandidate, authoritative bool) (map[CostRuleCandidate]*model.ChannelModelCostRule, error)
func InvalidateCostCoverage(channelID int, billableModel, costVariantKey string)
func ListCostRules(channelID int, billableModel, costVariantKey string) ([]model.ChannelModelCostRule, error)
```

Add `CostVariantKey` to `CreateCostRuleInput`; call `types.NormalizeCostVariantKey` before version calculation. Every query, cache key, conflict check, coverage result, history lookup, and activation invalidation must use the extended key.

- [ ] **Step 4: Add the frontend field and client validation**

Use the same regex in Zod and default new forms to `default`:

```ts
export const costVariantKeySchema = z
  .string()
  .trim()
  .toLowerCase()
  .regex(/^[a-z0-9][a-z0-9._-]{0,63}$/)

export interface CostRuleWriteRequest {
  channel_id: number
  billable_upstream_model: string
  cost_variant_key: string
  cost_mode: CostMode
  config: CostRuleConfigV1
  note?: string
  request_path?: string
  task_platform?: 'suno' | 'mj'
}
```

Expose the value in the rule table/drawer and include it in list filters.

- [ ] **Step 5: Verify backend and frontend, then commit**

Run: `go test ./service ./controller -run 'Test.*CostRule|Test.*CostCoverage' -count=1`

Run: `cd web && bun test src/features/cost-accounting && bun run typecheck`

Expected: PASS; blank legacy requests return `default`, and explicit variants remain isolated.

```bash
git add service/cost_rule.go service/cost_rule_test.go service/cost_rule_batch_test.go controller/cost_accounting.go controller/cost_accounting_test.go web/src/features/cost-accounting
git commit -m "feat: resolve cost rules by variant"
```

### Task 3: Bind Routing Targets To Cost Variants

**Files:**
- Modify: `pkg/modelrouting/types.go`
- Modify: `model/routing_policy_cache.go`
- Modify: `model/routing_policy.go`
- Modify: `service/routing_policy.go`
- Modify: `service/routing_policy_test.go`
- Modify: `web/src/features/model-routing/types.ts`
- Modify: `web/src/features/model-routing/components/route-target-editor.tsx`
- Modify: `web/src/features/model-routing/components/route-target-editor-client.test.tsx`
- Modify: `web/src/features/model-routing/components/route-target-editor-accessibility.test.tsx`

- [ ] **Step 1: Write failing round-trip and validation tests**

Assert that a route target with `CostVariantKey: "720p"` survives service write, database read, cache snapshot, API view, and frontend `toWriteRequest`/`fromPolicyResponse`. Assert blank values become `default` and invalid keys return `invalid_cost_variant_key`.

- [ ] **Step 2: Run focused tests and confirm failure**

Run: `go test ./service ./model -run 'TestRoutingPolicy.*CostVariant' -count=1`

Run: `cd web && bun test src/features/model-routing/components/route-target-editor-client.test.tsx`

Expected: FAIL because `modelrouting.Target`, write/view DTOs, and form schemas do not contain the field.

- [ ] **Step 3: Carry the value through backend routing types**

Add the same JSON property to every layer:

```go
type Target struct {
	ID             int
	PolicyID       int
	ChannelID      int
	Name           string
	UpstreamModel  string
	CostVariantKey string
	Priority       int
	Enabled        bool
	Constraints    Constraints
}
```

Add `CostVariantKey string \`json:"cost_variant_key"\`` to `RouteTargetWriteRequest` and `RouteTargetView`, normalize it through `types.NormalizeCostVariantKey`, and populate it in `routingPolicySnapshotFromRows`.

- [ ] **Step 4: Add the route-target form control**

Add a labeled text input with default `default`, validation feedback, and accessible description. Ensure copy/clone helpers retain the field and empty-target construction returns:

```ts
{
  channel_id: 0,
  channel_name: '',
  name: '',
  upstream_model: '',
  cost_variant_key: 'default',
  target_priority: 0,
  minimum_expected_margin_bps: null,
  enabled: false,
  output_resolutions: ['720p'],
  durations: { mode: 'range', values: [], min: 4, max: 15 },
  aspect_ratios: [],
  input_modes: [...INPUT_MODES],
  reference_minimums: { images: 0, videos: 0, audios: 0 },
  reference_limits: { images: 9, videos: 3, audios: 3 },
  supports_real_person: 'unknown',
}
```

- [ ] **Step 5: Verify and commit**

Run: `go test ./service ./model -run 'TestRoutingPolicy' -count=1`

Run: `cd web && bun test src/features/model-routing && bun run typecheck`

Expected: PASS; old targets read as `default`, and copied targets preserve explicit variants.

```bash
git add pkg/modelrouting/types.go model/routing_policy_cache.go model/routing_policy.go service/routing_policy.go service/routing_policy_test.go web/src/features/model-routing
git commit -m "feat: bind route targets to cost variants"
```

### Task 4: Use The Selected Variant At Runtime And In Accounting

**Files:**
- Modify: `types/cost_accounting.go`
- Modify: `model/cost_accounting.go`
- Modify: `service/model_routing.go`
- Modify: `service/profit_routing.go`
- Modify: `service/cost_accounting.go`
- Modify: `relay/common/relay_info.go`
- Modify: `service/model_routing_test.go`
- Modify: `service/profit_routing_test.go`
- Modify: `service/cost_accounting_test.go`
- Modify: `e2e/profit_routing_e2e_test.go`

- [ ] **Step 1: Write failing strict-routing regression tests**

Seed active `480p` and `720p` rules with different prices, then select a `720p` route target. Assert profit filtering and the persisted attempt use only the `720p` rule. Add a negative test where only `default` exists and the target asks for `720p`; strict mode must return uncovered instead of falling back.

```go
assert.Equal(t, "720p", attempt.CostVariantKey)
assert.Equal(t, rule720.ID, attempt.RuleID)
assert.Equal(t, "720p", snapshot.RouteTarget.CostVariantKey)
```

- [ ] **Step 2: Run the runtime tests and confirm failure**

Run: `go test ./service ./e2e -run 'Test.*CostVariant.*Routing|TestProfitRouting' -count=1`

Expected: FAIL because `profitCandidateKeys` and selected-channel rechecks currently construct channel/model-only keys.

- [ ] **Step 3: Publish the routing decision with variant identity**

Extend routing decision context and snapshots:

```go
type CostRoutingTargetSnapshot struct {
	PolicyID       int
	TargetID       int
	ChannelID      int
	UpstreamModel  string
	CostVariantKey string
	Priority       int
}
```

Add `CostVariantKey` to `CostProfitRecheckSnapshot` and `CostAccountingAttempt`. `publishRoutingDecision` writes it into the Gin context and `clearRoutingDecision` removes it. Legacy distribution paths explicitly set `default`.

- [ ] **Step 4: Update all coverage and locked comparison paths**

Build every `CostRuleCandidate` from `(channel, predicted model, selected variant)`. In `PrepareCostAttemptWithRuleValidation`, lock and compare the route target by policy ID, target ID, channel ID, upstream model, and variant; lock the active cost rule by the same extended business key. Return the existing snapshot conflict error when any value changed.

- [ ] **Step 5: Run full cost/routing suites and commit**

Run: `go test ./model ./service ./controller ./relay/... ./e2e -run 'Cost|ProfitRouting|ModelRouting' -count=1`

Expected: PASS; exact-variant misses remain misses and accounting attempts preserve the selected identity.

```bash
git add types/cost_accounting.go model/cost_accounting.go service/model_routing.go service/profit_routing.go service/cost_accounting.go relay/common/relay_info.go service/*routing_test.go service/cost_accounting_test.go e2e/profit_routing_e2e_test.go
git commit -m "feat: account for routed cost variants"
```

## Phase 2: Canonical Contract And Import Backend

### Task 5: Add Strict JSON Decoding And The Canonical Go Contract

**Files:**
- Modify: `common/json.go`
- Create: `common/json_test.go`
- Create: `types/config_import.go`
- Create: `dto/config_import.go`
- Create: `service/config_import_schema.go`
- Create: `service/config_import_schema_test.go`

- [ ] **Step 1: Write failing strict-decoder tests**

Cover unknown fields, trailing JSON values, a valid single document, 10 MiB input, nesting depth 32, 5,000 authoritative entities, 10,000 issues, 4 KiB ordinary strings, and 2 KiB notes. The wrapper contract is:

```go
func DecodeJsonStrict(reader io.Reader, v any) error
```

The test must assert `{"known":1,"unknown":2}` is rejected and `{"known":1}{"known":2}` is rejected.

- [ ] **Step 2: Run the strict decoder tests and confirm failure**

Run: `go test ./common ./service -run 'TestDecodeJsonStrict|TestValidateConfigImportDocument' -count=1`

Expected: FAIL because `DecodeJsonStrict` and the import document do not exist.

- [ ] **Step 3: Implement the shared decoder inside `common/json.go`**

This is the only place that may call the standard decoder directly:

```go
func DecodeJsonStrict(reader io.Reader, v any) error {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(v); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values are not allowed")
		}
		return err
	}
	return nil
}
```

- [ ] **Step 4: Define exact contract and domain enums**

Define these top-level types in `types/config_import.go`:

```go
type ConfigImportDocument struct {
	Kind           string                     `json:"kind"`
	SchemaVersion  int                        `json:"schema_version"`
	TemplateVersion string                    `json:"template_version"`
	Manifest       ConfigImportManifest       `json:"manifest"`
	Entities       ConfigImportEntities       `json:"entities"`
	DerivedPreview ConfigImportDerivedPreview `json:"derived_preview"`
	Issues         []ConfigImportSourceIssue  `json:"issues"`
}

type ConfigImportManifest struct {
	SourceFileName  string                  `json:"source_file_name"`
	SourceSHA256    string                  `json:"source_sha256"`
	PayloadSHA256   string                  `json:"payload_sha256"`
	GeneratedAt     string                  `json:"generated_at"`
	ConverterVersion string                 `json:"converter_version"`
	TemplateMatch   string                  `json:"template_match"`
	Counts          ConfigImportEntityCounts `json:"counts"`
}

type ConfigImportEntities struct {
	Channels           []ConfigImportChannel           `json:"channels"`
	ChannelLines       []ConfigImportChannelLine       `json:"channel_lines"`
	ModelSKUs          []ConfigImportModelSKU          `json:"model_skus"`
	SaleProposals      []ConfigImportSaleProposal      `json:"sale_proposals"`
	CostRuleDrafts     []ConfigImportCostRuleDraft     `json:"cost_rule_drafts"`
	ModelMappings      []ConfigImportModelMapping      `json:"model_mappings"`
	RouteBlueprints    []ConfigImportRouteBlueprint    `json:"route_blueprints"`
	Sources            []ConfigImportSource            `json:"sources"`
	UnresolvedVariants []ConfigImportUnresolvedVariant `json:"unresolved_variants"`
}
```

Each entity struct has a stable business ID, `entity_hash`, `source_ref`, and only the fields listed in design sections 8-10. Decimal-bearing fields use `string`; optional scalar facts use pointers. Define enum constants for batch status, item state, issue severity, binding action, resolution action, and route merge mode. `dto/config_import.go` exposes upload, binding, resolution, stage, validate, publish, list, and detail request/response shapes without credential fields.

Use these exact enum values throughout backend and frontend:

```go
const (
	ConfigImportStatusValidating    ConfigImportBatchStatus = "validating"
	ConfigImportStatusBlocked       ConfigImportBatchStatus = "blocked"
	ConfigImportStatusBinding       ConfigImportBatchStatus = "binding"
	ConfigImportStatusStaged        ConfigImportBatchStatus = "staged"
	ConfigImportStatusReady         ConfigImportBatchStatus = "ready"
	ConfigImportStatusPublishing    ConfigImportBatchStatus = "publishing"
	ConfigImportStatusPublished     ConfigImportBatchStatus = "published"
	ConfigImportStatusPublishFailed ConfigImportBatchStatus = "publish_failed"
)

const (
	ConfigImportItemNew       ConfigImportItemState = "new"
	ConfigImportItemUnchanged ConfigImportItemState = "unchanged"
	ConfigImportItemChanged   ConfigImportItemState = "changed"
	ConfigImportItemConflict  ConfigImportItemState = "conflict"
	ConfigImportItemExcluded  ConfigImportItemState = "excluded"
)
```

- [ ] **Step 5: Add schema, security, and canonical-hash validation**

`ParseConfigImportDocument(io.Reader)` must apply `http.MaxBytesReader` at the controller and `io.LimitReader` defensively in the service, call `common.DecodeJsonStrict`, reject credential field names matching `(?i)(api[_-]?key|access[_-]?token|auth[_-]?token|authorization|cookie|secret|password)`, sanitize source URLs to scheme/host/path, validate all references and Decimal strings with `shopspring/decimal`, and recompute `payload_sha256` from canonical authoritative entities. Legitimate billing fields such as `token_mode`, `input_tokens`, and `output_tokens` remain allowed. Return stable codes from the families `SCHEMA_*`, `LIMIT_*`, `SECURITY_*`, `REFERENCE_*`, and `DUPLICATE_*`.

- [ ] **Step 6: Verify and commit**

Run: `go test ./common ./types ./dto ./service -run 'TestDecodeJsonStrict|TestConfigImportSchema|TestValidateConfigImportDocument' -count=1`

Expected: PASS; unknown/trailing fields, credentials, non-finite or negative prices, broken references, and mismatched hashes are rejected.

```bash
git add common/json.go common/json_test.go types/config_import.go dto/config_import.go service/config_import_schema.go service/config_import_schema_test.go
git commit -m "feat: define strict config import contract"
```

### Task 6: Persist Resumable Import Batches

**Files:**
- Create: `model/config_import.go`
- Create: `model/config_import_test.go`
- Create: `model/config_import_migration_test.go`
- Modify: `model/main.go`

- [ ] **Step 1: Write failing persistence and uniqueness tests**

Test creation/loading of a batch with items, bindings, issues, and resolutions; unique `(batch_id, entity_type, business_id)`, unique `(batch_id, line_ref)`, and unique payload hash; `TEXT` round-trips for canonical JSON; and status compare-and-swap.

```go
updated, err := UpdateConfigImportBatchStatus(
	tx,
	batch.ID,
	types.ConfigImportStatusBinding,
	types.ConfigImportStatusStaged,
)
require.NoError(t, err)
assert.True(t, updated)
```

- [ ] **Step 2: Run model tests and confirm failure**

Run: `go test ./model -run 'TestConfigImport' -count=1`

Expected: FAIL because the import tables and functions are undefined.

- [ ] **Step 3: Implement cross-database models**

Create these GORM models with ordinary indexes and `type:text` JSON fields:

```go
type ConfigImportBatch struct {
	ID               int64  `json:"id" gorm:"primaryKey"`
	SchemaVersion    int    `json:"schema_version"`
	TemplateVersion  string `json:"template_version" gorm:"type:varchar(32)"`
	SourceSHA256     string `json:"source_sha256" gorm:"type:varchar(64);index"`
	PayloadSHA256    string `json:"payload_sha256" gorm:"type:varchar(64);uniqueIndex"`
	Status           string `json:"status" gorm:"type:varchar(32);index"`
	CreatedBy        int    `json:"created_by" gorm:"index"`
	SummaryJSON      string `json:"summary_json" gorm:"type:text"`
	BaselineJSON     string `json:"baseline_json" gorm:"type:text"`
	FailureCode      string `json:"failure_code" gorm:"type:varchar(64)"`
	FailureMessage   string `json:"failure_message" gorm:"type:text"`
	ValidatedAt      *int64 `json:"validated_at,omitempty"`
	PublishedAt      *int64 `json:"published_at,omitempty"`
	FailedAt         *int64 `json:"failed_at,omitempty"`
	CreatedAt        int64  `json:"created_at"`
	UpdatedAt        int64  `json:"updated_at"`
}
```

`ConfigImportItem` stores entity/state/canonical JSON/source/materialization/exclusion; `ConfigImportBinding` stores line action/channel ID and credential-confirmation operator/time but no credential; `ConfigImportIssue` stores the exact section-13.4 fields; `ConfigImportResolution` stores action and normalized decision JSON; `ConfigImportPublishAudit` stores admin, batch, before/after hashes, outcome, and time.

- [ ] **Step 4: Register both normal and fast migrations**

Add all five models to `migrateDB` and `migrateDBFast`. Use `lockForUpdate(tx)` for status transitions and no dialect-specific JSON or partial indexes.

- [ ] **Step 5: Verify and commit**

Run: `go test ./model -run 'TestConfigImport|TestCostVariant' -count=1`

Expected: PASS on SQLite; generated DDL contains `TEXT` JSON storage and compatible composite indexes.

```bash
git add model/config_import.go model/config_import_test.go model/config_import_migration_test.go model/main.go
git commit -m "feat: persist config import batches"
```

### Task 7: Implement Upload Idempotency, Listing, And Detail Recovery

**Files:**
- Create: `service/config_import.go`
- Create: `service/config_import_test.go`
- Modify: `service/config_import_schema.go`

- [ ] **Step 1: Write failing workflow tests**

Cover first upload, FAIL-to-`blocked`, valid-to-`binding`, same-hash upload returning the existing batch without duplicate items, changed hash creating a new batch, list pagination, detail recovery, and normalized input bytes being discarded.

```go
first, created, err := CreateConfigImportBatch(ctx, adminID, bytes.NewReader(payload))
require.NoError(t, err)
assert.True(t, created)

again, created, err := CreateConfigImportBatch(ctx, adminID, bytes.NewReader(payload))
require.NoError(t, err)
assert.False(t, created)
assert.Equal(t, first.ID, again.ID)
```

- [ ] **Step 2: Run focused service tests and confirm failure**

Run: `go test ./service -run 'TestConfigImportUpload|TestConfigImportList|TestConfigImportDetail' -count=1`

Expected: FAIL because batch orchestration is absent.

- [ ] **Step 3: Implement upload as a single normalization transaction**

Use this service surface:

```go
func CreateConfigImportBatch(ctx context.Context, adminID int, reader io.Reader) (*dto.ConfigImportBatchDetail, bool, error)
func ListConfigImportBatches(ctx context.Context, page, pageSize int) (*dto.ConfigImportBatchPage, error)
func GetConfigImportBatch(ctx context.Context, batchID int64) (*dto.ConfigImportBatchDetail, error)
```

Parse and validate before starting the transaction. Inside the transaction, lock/select by payload hash, insert the batch, normalized items, server-produced issues, and source issues, then set `blocked` when any FAIL exists or `binding` otherwise. Store canonical entity JSON using `common.Marshal`; never store the original request body or `derived_preview` as authoritative data. Secret-field scanning permits legitimate billing names such as `token_mode`, `input_tokens`, and `output_tokens`, while rejecting `api_key`, `access_token`, `auth_token`, `authorization`, `cookie`, `secret`, and `password` names plus high-confidence credential values.

- [ ] **Step 4: Derive allowed actions centrally**

Return `allowed_actions` from status and issue gates using one pure function. The exact action set is `bind`, `resolve`, `stage`, `validate`, and `publish`; `published` returns none, `blocked` returns none until a replacement document is uploaded, and `publish_failed` returns `validate` only.

- [ ] **Step 5: Verify and commit**

Run: `go test ./service -run 'TestConfigImport' -count=1`

Expected: PASS; repeated uploads are read-only and return the original materialization IDs.

```bash
git add service/config_import.go service/config_import_test.go service/config_import_schema.go
git commit -m "feat: orchestrate config import uploads"
```

### Task 8: Bind Channel Lines Without Accepting Credentials

**Files:**
- Create: `service/config_import_stage.go`
- Create: `service/config_import_stage_test.go`
- Modify: `controller/channel.go`
- Modify: `controller/channel_test_internal_test.go`
- Modify: `web/src/features/channels/types.ts`
- Modify: `web/src/features/channels/api.ts`
- Modify: `web/src/features/channels/hooks/use-channel-mutate-form.ts`

- [ ] **Step 1: Write failing binding and channel-response tests**

Cover existing-channel binding, skipped lines, type/capability mismatch, disabled new-channel binding, Secure line separation, MegaByAI fast real-person/no-real-person separation, and credential confirmation metadata. Assert a request containing `key`, `api_key`, or `secret` is rejected by strict DTO decoding.

- [ ] **Step 2: Run focused tests and confirm failure**

Run: `go test ./service ./controller -run 'TestConfigImportBinding|TestAddChannelReturnsCreatedIDs' -count=1`

Expected: FAIL because the binding service is absent and `AddChannel` returns no IDs.

- [ ] **Step 3: Implement binding validation and persistence**

Use a credential-free request:

```go
type ConfigImportBindingInput struct {
	LineRef              string `json:"line_ref"`
	Action               string `json:"action"`
	ChannelID            *int   `json:"channel_id,omitempty"`
	CredentialsConfirmed bool   `json:"credentials_confirmed"`
}
```

For `existing`, verify provider type, declared models, business identity, and line capability. For `create`, require a positive channel ID returned by the normal channel form, verify `status == common.ChannelStatusDisabled`, then bind it. For `skip`, mark dependent items excluded and retain the reason. Record only admin ID, line ref, confirmation boolean, and timestamp.

- [ ] **Step 4: Return channel IDs from the existing channel creation flow**

Keep the existing response envelope backward compatible and set:

```go
type AddChannelResponse struct {
	ChannelIDs []int `json:"channel_ids"`
}
```

After `BatchInsertChannels`, collect generated IDs and return them. Update the frontend mutation callback so the import wizard can consume the ID while normal channel creation keeps its current success behavior.

- [ ] **Step 5: Verify and commit**

Run: `go test ./service ./controller -run 'TestConfigImportBinding|TestAddChannel' -count=1`

Run: `cd web && bun run typecheck`

Expected: PASS; no binding request or response contains credential material, and all newly bound channels are disabled.

```bash
git add service/config_import_stage.go service/config_import_stage_test.go controller/channel.go controller/channel_test_internal_test.go web/src/features/channels
git commit -m "feat: bind imported channel lines"
```

### Task 9: Stage Cost, Sale, Mapping, And Routing Proposals

**Files:**
- Modify: `service/config_import_stage.go`
- Modify: `service/config_import_stage_test.go`
- Create: `service/config_import_pricing_test.go`
- Modify: `setting/billing_setting/tiered_billing.go`
- Modify: `setting/billing_setting/duration_billing.go`
- Modify: `setting/ratio_setting/model_ratio.go`
- Modify: `pkg/billingexpr/expr.md` only if the implementation exposes a new versioned variable; otherwise leave it untouched.

- [ ] **Step 1: Write failing stage and recomputation tests**

Cover server-side Decimal normalization, `no_video/with_video` merge when identical, negative-margin WARN gate, per-request/per-duration/token/expression sale proposals, one unresolved `CH-MEGABYAI / videos-standard` conflict, three Secure lines, two MegaByAI fast lines, excluded Secure 480p entries, route merge/replace/skip, and exact v1 counts `9/12/9/16/121/121/17/16/1`.

- [ ] **Step 2: Run focused staging tests and confirm failure**

Run: `go test ./service -run 'TestConfigImportStage|TestConfigImportPricing|TestConfigImportV1Baseline' -count=1`

Expected: FAIL because proposal generation and server recomputation are not implemented.

- [ ] **Step 3: Implement resolutions and deterministic staging**

Use exact resolution actions `split_line`, `bind_variant`, and `exclude`. Sort all business keys lexicographically before processing. A WARN remains blocking for its related item until the structured resolution changes it to PASS or exclusion. Materialize cost drafts through `CreateCostRuleDraft` with source `config_import`; store sale, mapping, and route proposals as canonical item JSON only.

- [ ] **Step 4: Recompute sale pricing from authoritative inputs**

Compile billing expressions with the current version and allowed variables documented in `pkg/billingexpr/expr.md`. Validate duration with `relaycommon.MaxTaskDurationSeconds`, reuse existing price map formats, and generate option patches for the selected user groups with `default` initially selected. Since existing base price maps are model-global and group ratios are group-wide, publish a proposal only when one supported base price plus current/proposed group ratios represents every selected group without changing effective price for an unselected group that can access the model; otherwise emit `PRICING_GROUP_SCOPE_UNREPRESENTABLE` WARN. Compare recomputed values to `derived_preview` only to create `COST_NORMALIZATION_MISMATCH` or pricing issues; never persist preview values as configuration.

- [ ] **Step 5: Capture the optimistic baseline and produce diffs**

At successful stage, hash the affected active cost rules, option keys, channel model mappings, and routing policies into `BaselineJSON`. Store field-level add/change/delete diffs in items, set the batch to `staged`, and set `ready` only after every publishable item is PASS and reviewed.

- [ ] **Step 6: Verify and commit**

Run: `go test ./service ./setting/... -run 'TestConfigImport|TestBilling' -count=1`

Expected: PASS; staging changes no active option, mapping, or routing row, and only creates inactive cost drafts.

```bash
git add service/config_import_stage.go service/config_import_stage_test.go service/config_import_pricing_test.go setting/billing_setting setting/ratio_setting/model_ratio.go
git commit -m "feat: stage imported configuration proposals"
```

### Task 10: Add Transaction-Aware Writers And Post-Commit Refresh

**Files:**
- Modify: `model/channel_model_cost_rule.go`
- Modify: `model/routing_policy.go`
- Modify: `model/option.go`
- Create: `model/config_publish_transaction_test.go`
- Modify: `service/routing_policy.go`

- [ ] **Step 1: Write failing transaction-boundary tests**

Start a transaction, call each new writer, force rollback, and assert the database and in-memory caches are unchanged. Commit another transaction and assert explicit refresh makes the new values visible.

- [ ] **Step 2: Run focused tests and confirm failure**

Run: `go test ./model ./service -run 'TestConfigPublishTransaction|Test.*WithTx' -count=1`

Expected: FAIL because existing writers own their own transaction or refresh caches immediately.

- [ ] **Step 3: Extract transaction-aware model functions**

Add these signatures and make existing public functions delegate to them:

```go
func ActivateChannelModelCostRuleWithTx(tx *gorm.DB, id int64, adminID int, now int64, validate func(*ChannelModelCostRule) error) (*ChannelModelCostRule, error)
func ReplaceRoutingPolicyWithTx(tx *gorm.DB, id int, policy RoutingPolicy, targets []RouteTarget) (*RoutingPolicy, error)
func UpdateOptionsWithTx(tx *gorm.DB, values map[string]string) error
```

These functions perform database work only. They must not call `updateOptionMap`, `RefreshRoutingPolicyCache`, `InitChannelCache`, or cost cache invalidation.

- [ ] **Step 4: Add explicit post-commit refresh**

Add:

```go
func RefreshOptions(values map[string]string) error
func RefreshPublishedConfig(keys ConfigImportRefreshKeys) error
```

Refresh options, cost coverage, affected routing policy keys, and channel cache in that order. Return a structured list of failed cache domains so the caller can create `CACHE_REFRESH_PENDING` without changing database publication state.

- [ ] **Step 5: Verify and commit**

Run: `go test ./model ./service -run 'TestConfigPublishTransaction|Test.*WithTx|TestUpdateOptionsBulk' -count=1`

Expected: PASS; rollback never leaks cache values and commit visibility requires the explicit refresh.

```bash
git add model/channel_model_cost_rule.go model/routing_policy.go model/option.go model/config_publish_transaction_test.go service/routing_policy.go
git commit -m "refactor: expose transaction-aware config writers"
```

### Task 11: Publish Atomically With Stale And Concurrent Protection

**Files:**
- Create: `service/config_import_publish.go`
- Create: `service/config_import_publish_test.go`
- Modify: `service/config_import.go`
- Modify: `model/config_import.go`

- [ ] **Step 1: Write failing publication tests**

Cover successful ordered publication, failure injected at each of the five write stages, stale baseline, two concurrent publishers, `publish_failed` retry after validation, no second publish after success, and cache-refresh failure after commit.

```go
require.NoError(t, PublishConfigImportBatch(ctx, batch.ID, adminID))
assert.Equal(t, types.ConfigImportStatusPublished, loadBatch(t, batch.ID).Status)
assert.Equal(t, "active", loadImportedRule(t).Status)
assert.False(t, loadImportedRoute(t).Enabled)
```

- [ ] **Step 2: Run publish tests and confirm failure**

Run: `go test ./service -run 'TestPublishConfigImportBatch' -count=1`

Expected: FAIL because atomic publish orchestration is absent.

- [ ] **Step 3: Implement ordered locks and compare-and-swap status**

Begin with a short transaction that locks the batch through `lockForUpdate`, requires `ready`, and recomputes the baseline. When hashes differ, write `STALE_BASE_VERSION`, move the batch to `staged`, commit that state, and return the service error without starting publication. When hashes match, the publication transaction again locks the batch, compare-and-swaps `ready` to `publishing`, and sorts/locks affected cost keys/options/channels/policies. Only one concurrent transaction may make that transition.

- [ ] **Step 4: Apply reviewed changes in exact order**

The transaction sequence is:

```go
if err := publishCostRules(tx, batch, adminID); err != nil { return err }
if err := publishSalePricing(tx, batch); err != nil { return err }
if err := publishModelMappings(tx, batch); err != nil { return err }
if err := publishRoutingDefinitions(tx, batch); err != nil { return err }
if err := publishRoutingEnablement(tx, batch); err != nil { return err }
return markConfigImportPublished(tx, batch, adminID)
```

Re-run schema/business validation, cost coverage, expression compilation, margin gates, and `modelrouting.ValidatePolicy` before the first mutation. Write a publish audit inside the same transaction.

If any publication write fails, the transaction rolls back to the pre-publish active configuration. In a separate follow-up transaction, lock the still-unpublished batch, set `publish_failed`, persist the redacted failure code/message, and keep the saved proposals for validate/retry. This status update must never contain or replay partial configuration writes.

- [ ] **Step 5: Refresh caches only after commit**

Call `RefreshPublishedConfig` after the transaction returns success. If it fails, keep `published`, insert an admin-visible `CACHE_REFRESH_PENDING` issue, and expose a cache-only retry via the existing `validate` action. Never call the publication transaction again.

- [ ] **Step 6: Verify and commit**

Run: `go test ./service -run 'TestPublishConfigImportBatch|TestConfigImportStale|TestConfigImportConcurrent' -count=1`

Expected: PASS; every injected transactional failure leaves active configuration unchanged, while cache failure preserves committed rows and one retryable issue.

```bash
git add service/config_import_publish.go service/config_import_publish_test.go service/config_import.go model/config_import.go
git commit -m "feat: publish imported config atomically"
```

### Task 12: Expose Permissioned Admin Import APIs

**Files:**
- Create: `service/authz/resources_config_import.go`
- Modify: `service/authz/authz_test.go`
- Create: `controller/config_import.go`
- Create: `controller/config_import_test.go`
- Create: `router/config-import-router.go`
- Create: `router/config_import_router_test.go`
- Modify: `router/api-router.go`

- [ ] **Step 1: Write failing authorization and HTTP contract tests**

Test unauthenticated rejection, ordinary-user rejection, separate read/write/publish grants, JSON-only upload, 10 MiB request cap, strict DTO rejection, stable service-error envelopes, all eight routes, and redaction of secrets from errors.

- [ ] **Step 2: Run controller/router tests and confirm failure**

Run: `go test ./controller ./router ./service/authz -run 'TestConfigImport|TestAuthz.*ConfigImport' -count=1`

Expected: FAIL because the resource and routes are unregistered.

- [ ] **Step 3: Register the permission resource**

Define:

```go
const (
	ResourceConfigImport = "config_import"
	ActionPublish         = "publish"
)

var (
	ConfigImportRead    = Permission{Resource: ResourceConfigImport, Action: ActionRead}
	ConfigImportWrite   = Permission{Resource: ResourceConfigImport, Action: ActionWrite}
	ConfigImportPublish = Permission{Resource: ResourceConfigImport, Action: ActionPublish}
)
```

Grant all three to the built-in admin role and preserve superuser behavior.

- [ ] **Step 4: Implement the exact API table**

Register with `middleware.AdminAuth()` plus `RequirePermission`:

```go
var configImportPermissionRoutes = []permissionRoute{
	{method: http.MethodPost, path: "/batches", permission: authz.ConfigImportWrite, handler: controller.CreateConfigImportBatch},
	{method: http.MethodGet, path: "/batches", permission: authz.ConfigImportRead, handler: controller.ListConfigImportBatches},
	{method: http.MethodGet, path: "/batches/:id", permission: authz.ConfigImportRead, handler: controller.GetConfigImportBatch},
	{method: http.MethodPut, path: "/batches/:id/bindings", permission: authz.ConfigImportWrite, handler: controller.UpdateConfigImportBindings},
	{method: http.MethodPut, path: "/batches/:id/resolutions", permission: authz.ConfigImportWrite, handler: controller.UpdateConfigImportResolutions},
	{method: http.MethodPost, path: "/batches/:id/stage", permission: authz.ConfigImportWrite, handler: controller.StageConfigImportBatch},
	{method: http.MethodPost, path: "/batches/:id/validate", permission: authz.ConfigImportWrite, handler: controller.ValidateConfigImportBatch},
	{method: http.MethodPost, path: "/batches/:id/publish", permission: authz.ConfigImportPublish, handler: controller.PublishConfigImportBatch},
}
```

- [ ] **Step 5: Verify and commit**

Run: `go test ./controller ./router ./service/authz -run 'TestConfigImport|TestAuthz' -count=1`

Expected: PASS; read-only admins cannot mutate, writers cannot publish, and publish permission alone does not grant detail access.

```bash
git add service/authz/resources_config_import.go service/authz/authz_test.go controller/config_import.go controller/config_import_test.go router/config-import-router.go router/config_import_router_test.go router/api-router.go
git commit -m "feat: expose config import admin api"
```

## Phase 3: Admin Import Wizard

### Task 13: Add Typed Import API And Resumable Client State

**Files:**
- Create: `web/src/features/config-import/types.ts`
- Create: `web/src/features/config-import/api.ts`
- Create: `web/src/features/config-import/query-keys.ts`
- Create: `web/src/features/config-import/lib/batch-state.ts`
- Create: `web/src/features/config-import/lib/diff.ts`
- Create: `web/src/features/config-import/lib/__tests__/batch-state.test.ts`
- Create: `web/src/features/config-import/lib/__tests__/diff.test.ts`
- Modify: `web/src/lib/admin-permissions.ts`

- [ ] **Step 1: Write failing Zod and state-derivation tests**

Mirror every backend enum and response field with Zod. Test seven wizard steps, back/forward availability, resume from each backend status, blocked issues, stale baseline, publish failure, and published terminal state.

```ts
expect(deriveWizardState(batch)).toEqual({
  step: 'channel_binding',
  canGoBack: true,
  canContinue: false,
  canPublish: false,
  blockingCodes: ['CHANNEL_LINE_UNBOUND'],
})
```

- [ ] **Step 2: Run unit tests and confirm failure**

Run: `cd web && bun test src/features/config-import/lib`

Expected: FAIL because the feature modules do not exist.

- [ ] **Step 3: Define client schemas and permission constants**

Add `CONFIG_IMPORT: 'config_import'` and `PUBLISH: 'publish'`. Define `configImportBatchDetailSchema`, `configImportItemSchema`, `configImportIssueSchema`, `configImportBindingSchema`, `configImportResolutionSchema`, and the exact request schemas. Preserve Decimal values as strings and never define a credential property.

- [ ] **Step 4: Implement API calls and query keys**

Use the existing Axios wrapper and these functions:

```ts
export async function uploadConfigImport(document: unknown): Promise<ConfigImportBatchDetail>
export async function listConfigImportBatches(params: ConfigImportListParams): Promise<ConfigImportBatchPage>
export async function getConfigImportBatch(id: number): Promise<ConfigImportBatchDetail>
export async function saveConfigImportBindings(id: number, request: ConfigImportBindingsRequest): Promise<ConfigImportBatchDetail>
export async function saveConfigImportResolutions(id: number, request: ConfigImportResolutionsRequest): Promise<ConfigImportBatchDetail>
export async function stageConfigImport(id: number, request: ConfigImportStageRequest): Promise<ConfigImportBatchDetail>
export async function validateConfigImport(id: number): Promise<ConfigImportBatchDetail>
export async function publishConfigImport(id: number): Promise<ConfigImportBatchDetail>
```

Parse every response with Zod. Query keys are stable arrays rooted at `['config-import']`.

- [ ] **Step 5: Implement pure state and diff helpers**

`deriveWizardState` uses backend status and issue gates only; it never promotes WARN to PASS. `groupItemDiffs` groups by entity type and state, preserves source sheet/row, and sorts by business ID. No localStorage or IndexedDB is used; recovery always reloads the batch by ID.

- [ ] **Step 6: Verify and commit**

Run: `cd web && bun test src/features/config-import/lib && bun run typecheck`

Expected: PASS; all backend states resolve to a deterministic visible wizard step.

```bash
git add web/src/features/config-import/types.ts web/src/features/config-import/api.ts web/src/features/config-import/query-keys.ts web/src/features/config-import/lib web/src/lib/admin-permissions.ts
git commit -m "feat: add config import client contract"
```

### Task 14: Build Upload And Channel Binding Steps

**Files:**
- Create: `web/src/features/config-import/components/import-upload-step.tsx`
- Create: `web/src/features/config-import/components/channel-binding-step.tsx`
- Create: `web/src/features/config-import/components/import-batch-list.tsx`
- Create: `web/src/features/config-import/components/__tests__/import-upload-step.test.tsx`
- Create: `web/src/features/config-import/components/__tests__/channel-binding-step.test.tsx`
- Modify: `web/src/features/channels/components/drawers/channel-mutate-drawer.tsx`

- [ ] **Step 1: Write failing interaction tests**

Test JSON file selection, client-side 10 MiB/type check, upload error display, resuming an existing batch, bind-existing, open-create-channel, receive a created disabled channel ID, skip with reason, credential-confirmation checkbox, and secret-free DOM snapshots.

- [ ] **Step 2: Run component tests and confirm failure**

Run: `cd web && bun test src/features/config-import/components/import-upload-step.test.tsx src/features/config-import/components/channel-binding-step.test.tsx`

Expected: FAIL because the components do not exist.

- [ ] **Step 3: Implement dense operational layouts**

Use a full-width step layout, stable table columns, a native file picker, icon buttons with tooltips, and no nested cards. The upload step accepts `.json` only and shows hash, schema/template versions, counts, issues, and next action. The batch list allows recovery without browser persistence.

- [ ] **Step 4: Reuse the existing channel drawer safely**

Add optional props:

```ts
type ChannelMutateDrawerProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  currentRow?: Channel
  initialDisabled?: boolean
  onCreated?: (channelIds: number[]) => void
}
```

When opened from import, force disabled status in the initial form, preserve all normal credential handling inside the existing channel API, and pass only returned channel IDs to the import binding step.

- [ ] **Step 5: Verify and commit**

Run: `cd web && bun test src/features/config-import/components src/features/channels && bun run typecheck`

Expected: PASS; created channel IDs bind correctly and no key value is rendered into import state or test snapshots.

```bash
git add web/src/features/config-import/components web/src/features/channels/components/drawers/channel-mutate-drawer.tsx
git commit -m "feat: add import upload and channel binding ui"
```

### Task 15: Build Conflict, Pricing, And Routing Review Steps

**Files:**
- Create: `web/src/features/config-import/components/conflict-resolution-step.tsx`
- Create: `web/src/features/config-import/components/pricing-step.tsx`
- Create: `web/src/features/config-import/components/routing-diff-step.tsx`
- Create: `web/src/features/config-import/components/diff-table.tsx`
- Create: `web/src/features/config-import/components/issue-list.tsx`
- Create: `web/src/features/config-import/components/__tests__/conflict-resolution-step.test.tsx`
- Create: `web/src/features/config-import/components/__tests__/pricing-step.test.tsx`
- Create: `web/src/features/config-import/components/__tests__/routing-diff-step.test.tsx`

- [ ] **Step 1: Write failing review-flow tests**

Test the three conflict actions, required structured fields, inability to dismiss WARN, `default` group initially selected, per-field current/proposed pricing, merge/replace/skip route modes, explicit replace deletion confirmation, source sheet/row links, and stale-baseline banner.

- [ ] **Step 2: Run component tests and confirm failure**

Run: `cd web && bun test src/features/config-import/components/__tests__/conflict-resolution-step.test.tsx src/features/config-import/components/__tests__/pricing-step.test.tsx src/features/config-import/components/__tests__/routing-diff-step.test.tsx`

Expected: FAIL because the review components are absent.

- [ ] **Step 3: Implement conflict resolution controls**

Use a segmented control for `split_line`, `bind_variant`, and `exclude`. `split_line` requires a bound `line_ref`; `bind_variant` requires a valid `cost_variant_key` and route target reference; `exclude` requires a non-empty reason. Display the v1 note as read-only evidence, never as a structured routing value.

- [ ] **Step 4: Implement pricing and routing diffs**

Use checkboxes for groups, with `default` selected on first stage. The pricing table shows authoritative inputs, server recomputation, current value, proposal, margin, severity, and source. The routing table shows target constraints and `cost_variant_key`; replace mode shows every deletion and requires explicit confirmation.

- [ ] **Step 5: Verify and commit**

Run: `cd web && bun test src/features/config-import/components && bun run typecheck`

Expected: PASS; unresolved WARN items cannot advance and replacement deletions are visible before review.

```bash
git add web/src/features/config-import/components
git commit -m "feat: add import proposal review steps"
```

### Task 16: Build Publish Review, Result, And Wizard Shell

**Files:**
- Create: `web/src/features/config-import/components/publish-review-step.tsx`
- Create: `web/src/features/config-import/components/publish-result-step.tsx`
- Create: `web/src/features/config-import/components/config-import-stepper.tsx`
- Create: `web/src/features/config-import/components/__tests__/publish-review-step.test.tsx`
- Create: `web/src/features/config-import/components/__tests__/config-import-wizard.test.tsx`
- Create: `web/src/features/config-import/index.tsx`

- [ ] **Step 1: Write failing end-to-end component tests**

Test restoring a staged batch, stage/validate transitions, publish permission visibility, final confirmation, `STALE_BASE_VERSION` returning to diffs, transactional failure retry, cache refresh pending, and published summary with created/changed/excluded counts.

- [ ] **Step 2: Run wizard tests and confirm failure**

Run: `cd web && bun test src/features/config-import/components/__tests__/publish-review-step.test.tsx src/features/config-import/components/__tests__/config-import-wizard.test.tsx`

Expected: FAIL because the shell and final steps do not exist.

- [ ] **Step 3: Implement the shell and fixed step sequence**

Use exactly:

```ts
export const CONFIG_IMPORT_STEPS = [
  'upload',
  'channel_binding',
  'conflict_resolution',
  'pricing',
  'routing_diff',
  'publish_review',
  'publish_result',
] as const
```

The backend is the state authority. Mutations invalidate batch detail; the shell never advances optimistically. Keep table dimensions stable, allow horizontal scrolling for data grids, and collapse the stepper to a compact numbered header on mobile.

- [ ] **Step 4: Implement final review and publish behavior**

Show counts, unresolved issues, affected active keys, enablement states, and publish order. Require a confirmation checkbox and publish permission. `publish_failed` exposes validate/retry guidance; `CACHE_REFRESH_PENDING` exposes cache-only validation; `published` is terminal.

- [ ] **Step 5: Verify and commit**

Run: `cd web && bun test src/features/config-import && bun run typecheck`

Expected: PASS; no UI state can bypass backend issue gates or publication permission.

```bash
git add web/src/features/config-import
git commit -m "feat: complete config import wizard"
```

### Task 17: Register Navigation, Route Guard, And Seven Locales

**Files:**
- Create: `web/src/routes/_authenticated/config-import/index.tsx`
- Modify: `web/src/hooks/use-sidebar-data.ts`
- Modify: `web/src/hooks/use-sidebar-config.ts`
- Modify: `web/src/i18n/locales/en.json`
- Modify: `web/src/i18n/locales/zh.json`
- Modify: `web/src/i18n/locales/zh-TW.json`
- Modify: `web/src/i18n/locales/fr.json`
- Modify: `web/src/i18n/locales/ja.json`
- Modify: `web/src/i18n/locales/ru.json`
- Modify: `web/src/i18n/locales/vi.json`
- Modify: `web/src/routeTree.gen.ts`

- [ ] **Step 1: Write the route guard**

Require `config_import/read` with `hasPermission` and redirect to `/403` when absent. Add the admin sidebar item using a `FileUp` Lucide icon and register `/config-import` under the `config_import` sidebar module.

- [ ] **Step 2: Add all user-facing English source keys**

Every label, issue filter, action, status, empty state, error, table header, and confirmation uses `t('English source')`. Run the project sync command rather than hand-copying keys between locales.

- [ ] **Step 3: Sync and complete all translations**

Run: `cd web && bun run i18n:sync`

Expected: locale files contain every new key; complete accurate translations in all seven locale files and remove untranslated report entries for these keys.

- [ ] **Step 4: Regenerate routes and run frontend checks**

Run: `cd web && bun run build`

Expected: `routeTree.gen.ts` includes `/_authenticated/config-import/` and production build succeeds.

Run: `cd web && bun run typecheck && bun run lint && bun run format:check && bun run i18n:sync`

Expected: all commands exit 0 and the second i18n sync produces no diff.

- [ ] **Step 5: Commit**

```bash
git add web/src/routes/_authenticated/config-import/index.tsx web/src/hooks/use-sidebar-data.ts web/src/hooks/use-sidebar-config.ts web/src/i18n/locales web/src/routeTree.gen.ts
git commit -m "feat: register config import admin workflow"
```

## Phase 4: Corrected Fixture, Static Converter, And V2 Template

### Task 18: Build A Corrected V1 Regression Fixture Without Touching The Source

**Files:**
- Create: `web/scripts/build-channel-config-fixtures.mjs`
- Create: `web/src/channel-config-converter/__fixtures__/channel-config-v1-corrected.xlsx`
- Create: `web/src/channel-config-converter/__fixtures__/v1-expected-counts.json`
- Generate: `outputs/019f9dbb-4e5d-7933-8531-d38e417ec068/渠道模型成本与利润模板-v1-修正版.xlsx` (acceptance artifact, not committed)
- Preserve: `outputs/019f9dbb-4e5d-7933-8531-d38e417ec068/渠道模型成本与利润模板-更新.xlsx`

- [x] **Step 1: Record the source hash before any workbook operation**

Run:

```powershell
Get-FileHash 'outputs/019f9dbb-4e5d-7933-8531-d38e417ec068/渠道模型成本与利润模板-更新.xlsx' -Algorithm SHA256
```

Expected: capture one SHA-256 value in the builder's generated metadata and verify the same value again after export.

- [x] **Step 2: Write the deterministic workbook builder with stable-ID lookup**

Use only the bundled Node runtime and `@oai/artifact-tool`. Load the source workbook, find cost rows by `成本规则ID` values `COST-MEGABYAI-R102-480-REQ`, `COST-MEGABYAI-R103-720-REQ`, `COST-MEGABYAI-R104-480-REQ`, and `COST-MEGABYAI-R105-720-REQ`, and patch authoritative fields. Do not select physical row numbers in code.

Set R102/R103 `原币按次` to numeric `3`/`4` and confirmed真人能力 to true; set R104/R105 to numeric `1.2`/`1.6` and confirmed真人能力 to false. V1 exposes these capability facts in mapping/cost audit notes rather than dedicated columns, so locate the matching cost and mapping business IDs and rebuild their notes from typed correction data. The v1 adapter derives publishable capability from the confirmed MegaByAI line-group contract, never by parsing the note.

- [x] **Step 3: Restore formulas instead of hardcoding normalized USD**

For each corrected cost row, set `原币基础单价` to reference the authoritative per-request cell and set `标准USD单价` to the same row's auditable formula:

```excel
=O{row}*P{row}*Q{row}*R{row}*(1+S{row})*T{row}
```

The builder must preserve existing formats, styles, formulas, dates, and all unrelated cells. It exports identical corrected workbook content to `web/src/channel-config-converter/__fixtures__/channel-config-v1-corrected.xlsx` and `outputs/019f9dbb-4e5d-7933-8531-d38e417ec068/渠道模型成本与利润模板-v1-修正版.xlsx`; only the fixture is committed.

- [x] **Step 4: Inspect, scan, and visually render every sheet**

Use `workbook.inspect` to confirm the four IDs, CNY values, capabilities, and formulas. Scan all sheets for `#REF!|#DIV/0!|#VALUE!|#NAME?|#N/A`. Render the used range of each of the ten sheets and inspect every resulting PNG for clipping, blank output, or changed styling.

Expected normalized USD values use the workbook's current `原币兑USD` and other factors; they are formula results, not fixture constants.

- [x] **Step 5: Assert the source stayed unchanged and write expected counts**

Run the source hash command again and require byte-for-byte equality. Write:

```json
{
  "channels": 9,
  "channel_lines": 12,
  "model_skus": 9,
  "sale_proposals": 16,
  "cost_rule_drafts": 121,
  "model_mappings": 121,
  "detected_conflict_keys": 17,
  "automatic_conflict_keys": 16,
  "manual_conflict_keys": 1,
  "manual_conflict_business_id": "CH-MEGABYAI/videos-standard"
}
```

- [x] **Step 6: Run the builder twice and commit deterministic outputs**

Run: `cd web && bun run scripts/build-channel-config-fixtures.mjs`

Run: `cd web && bun run scripts/build-channel-config-fixtures.mjs`

Expected: the second run produces no Git diff, the source hash is unchanged, and visual verification is recorded by the script.

```bash
git add web/scripts/build-channel-config-fixtures.mjs web/src/channel-config-converter/__fixtures__/channel-config-v1-corrected.xlsx web/src/channel-config-converter/__fixtures__/v1-expected-counts.json
git commit -m "test: add corrected channel import fixture"
```

### Task 19: Scaffold A Truly Offline Converter Build

**Files:**
- Modify: `web/package.json`
- Modify: `web/bun.lock`
- Create: `web/rsbuild.converter.config.ts`
- Create: `web/src/channel-config-converter/index.html`
- Create: `web/src/channel-config-converter/main.tsx`
- Create: `web/src/channel-config-converter/i18n.ts`
- Create: `web/src/channel-config-converter/app.tsx`
- Create: `web/src/channel-config-converter/security.ts`
- Create: `web/src/channel-config-converter/__tests__/security.test.ts`

- [ ] **Step 1: Add failing security preflight tests**

Test rejection of `.xls`, `.xlsm`, OLE signatures, `vbaProject.bin`, external links/connections, files over 10 MiB, decompressed ZIP content over 100 MiB, more than 20 sheets, more than 20,000 rows per sheet, and more than 5,000 entities.

- [ ] **Step 2: Install the planned dependencies**

Run: `cd web && bun add exceljs @zip.js/zip.js decimal.js json-canonicalize`

Expected: `package.json` and `bun.lock` contain all four production dependencies.

- [ ] **Step 3: Add dedicated build scripts and relative assets**

Add:

```json
"converter:dev": "rsbuild dev -c rsbuild.converter.config.ts",
"converter:build": "rsbuild build -c rsbuild.converter.config.ts",
"converter:test": "bun test src/channel-config-converter"
```

The config emits `dist/channel-config-converter/index.html`, uses `assetPrefix: './'`, bundles every dependency/font locally, and injects CSP including `default-src 'self' blob: data:`, `connect-src 'none'`, `object-src 'none'`, and `base-uri 'none'`.

- [ ] **Step 4: Implement security preflight before ExcelJS parsing**

Use zip.js to inspect entry names and declared/uncompressed sizes before extraction. Reject macro and external-connection entries, enforce size/sheet bounds, and pass the ArrayBuffer to ExcelJS only after preflight. Override `window.fetch`, `XMLHttpRequest`, `WebSocket`, and `EventSource` in development tests to record any attempted network access.

- [ ] **Step 5: Initialize converter-only i18n without persistence**

Import the shared locale JSON files into a new `i18next.createInstance()`, choose language from `navigator.language`, and do not install the browser language detector. Do not read or write localStorage/IndexedDB.

- [ ] **Step 6: Verify offline build and commit**

Run: `cd web && bun run converter:test && bun run converter:build`

Expected: tests pass and the output HTML references only relative local assets. `rg -n 'src=["'"']https?://|href=["'"']https?://' dist/channel-config-converter/index.html` returns no matches, and `rg -n "connect-src 'none'" dist/channel-config-converter/index.html` finds exactly one policy.

```bash
git add web/package.json web/bun.lock web/rsbuild.converter.config.ts web/src/channel-config-converter
git commit -m "feat: scaffold offline channel converter"
```

### Task 20: Implement V1 And V2 Workbook Adapters

**Files:**
- Create: `web/src/channel-config-converter/types.ts`
- Create: `web/src/channel-config-converter/schema.ts`
- Create: `web/src/channel-config-converter/workbook.ts`
- Create: `web/src/channel-config-converter/adapters/v1.ts`
- Create: `web/src/channel-config-converter/adapters/v2.ts`
- Create: `web/src/channel-config-converter/__tests__/v1.test.ts`
- Create: `web/src/channel-config-converter/__tests__/v2.test.ts`
- Create: `web/src/channel-config-converter/__fixtures__/invalid/` test workbooks generated in-memory by the tests

- [ ] **Step 1: Write failing adapter tests against exact table contracts**

For v1, assert the ten expected sheets and exact row-4 headers, stable-ID extraction, typed Excel values, formulas treated as non-authoritative cache values, missing-sheet/header-change/duplicate-ID/broken-reference errors, and the corrected fixture baseline. For v2, assert explicit `渠道线路` and `路由目标` sheets are consumed without note parsing.

- [ ] **Step 2: Run adapter tests and confirm failure**

Run: `cd web && bun test src/channel-config-converter/__tests__/v1.test.ts src/channel-config-converter/__tests__/v2.test.ts`

Expected: FAIL because no adapter exists.

- [ ] **Step 3: Implement the adapter boundary**

Use:

```ts
export interface WorkbookAdapter {
  readonly templateVersion: '1' | '2'
  matches(workbook: WorkbookSnapshot): AdapterMatch
  extract(workbook: WorkbookSnapshot): ExtractedWorkbook
}

export interface SourceLocation {
  sheet: string
  row: number
  business_id: string
}
```

`workbook.ts` converts ExcelJS cells to typed snapshots and retains formula plus cached result separately. Adapters output extracted business rows and source locations only; they do not compute new-api database IDs or canonical hashes.

- [ ] **Step 4: Encode v1 compatibility rules as data mappings**

Implement known structural rules for SKU/resolution variants, Secure line groups (`secure-discount`, `secure-overseas`, `secure-enterprise`), MegaByAI fast account groups, scenario deduplication, and Secure unsupported 480p exclusions. These mappings use channel code, upstream model, stable cost/mapping IDs, structured SKU/capability fields, and group contracts. They must not use workbook filename, physical row number, or fixed price replacement.

- [ ] **Step 5: Verify exact corrected-fixture counts and commit**

Run: `cd web && bun test src/channel-config-converter/__tests__/v1.test.ts src/channel-config-converter/__tests__/v2.test.ts`

Expected: PASS with 9 channel masters, 12 lines, 9 SKUs, 16 sale proposals, 121 cost candidates, 121 mappings, 17 detected keys, 16 automatic keys, and only `CH-MEGABYAI/videos-standard` unresolved.

```bash
git add web/src/channel-config-converter/types.ts web/src/channel-config-converter/schema.ts web/src/channel-config-converter/workbook.ts web/src/channel-config-converter/adapters web/src/channel-config-converter/__tests__ web/src/channel-config-converter/__fixtures__/invalid
git commit -m "feat: parse channel workbook templates"
```

### Task 21: Normalize, Recompute, Detect Conflicts, And Hash Deterministically

**Files:**
- Create: `web/src/channel-config-converter/normalize.ts`
- Create: `web/src/channel-config-converter/conflicts.ts`
- Create: `web/src/channel-config-converter/hash.ts`
- Create: `web/src/channel-config-converter/security-scan.ts`
- Create: `web/src/channel-config-converter/__tests__/normalize.test.ts`
- Create: `web/src/channel-config-converter/__tests__/conflicts.test.ts`
- Create: `web/src/channel-config-converter/__tests__/hash.test.ts`

- [ ] **Step 1: Write failing deterministic normalization tests**

Test whitespace/case/enums, Decimal canonicalization, stable entity order, stable business IDs, entity hashes, payload hash exclusion of filename/generated time/issues/preview, source URL query/fragment removal, credential field/value detection, formula preview mismatch, and two byte-different workbooks with identical authoritative entities producing the same payload hash.

- [ ] **Step 2: Run focused tests and confirm failure**

Run: `cd web && bun test src/channel-config-converter/__tests__/normalize.test.ts src/channel-config-converter/__tests__/conflicts.test.ts src/channel-config-converter/__tests__/hash.test.ts`

Expected: FAIL because normalization and hashing are absent.

- [ ] **Step 3: Implement Decimal-only authoritative arithmetic**

Parse every money/rate/multiplier with `Decimal`; reject NaN, Infinity, negative prices, non-positive required multipliers, and excess precision. Canonical strings must satisfy `^-?(0|[1-9][0-9]*)(\.[0-9]+)?$`, remove trailing fractional zeros, and normalize negative zero to `0`.

- [ ] **Step 4: Implement conflict grouping and route blueprints**

Deduplicate identical scenario contracts while retaining every source ID. Build `cost_variant_key` from structured SKU/resolution or confirmed line identity. Emit `COST_VARIANT_AMBIGUOUS` for conditions that still select multiple prices, never select min/max/last. Generate route blueprints with `enabled: false` and line capability constraints.

- [ ] **Step 5: Implement canonical hashes with Web Crypto**

Canonicalize each entity excluding `entity_hash`, hash UTF-8 with `crypto.subtle.digest('SHA-256')`, then hash the canonical object containing authoritative entities and source locations only. Sort arrays by stable business ID before hashing.

- [ ] **Step 6: Verify and commit**

Run: `cd web && bun test src/channel-config-converter/__tests__/normalize.test.ts src/channel-config-converter/__tests__/conflicts.test.ts src/channel-config-converter/__tests__/hash.test.ts`

Expected: PASS; repeated conversion yields identical IDs, ordering, entity hashes, and payload hash.

```bash
git add web/src/channel-config-converter/normalize.ts web/src/channel-config-converter/conflicts.ts web/src/channel-config-converter/hash.ts web/src/channel-config-converter/security-scan.ts web/src/channel-config-converter/__tests__
git commit -m "feat: normalize and hash channel imports"
```

### Task 22: Build Converter Preview And Download Experience

**Files:**
- Modify: `web/src/channel-config-converter/app.tsx`
- Create: `web/src/channel-config-converter/components/converter-header.tsx`
- Create: `web/src/channel-config-converter/components/file-dropzone.tsx`
- Create: `web/src/channel-config-converter/components/summary-view.tsx`
- Create: `web/src/channel-config-converter/components/entity-table.tsx`
- Create: `web/src/channel-config-converter/components/issue-view.tsx`
- Create: `web/src/channel-config-converter/components/json-view.tsx`
- Create: `web/src/channel-config-converter/components/download-actions.tsx`
- Create: `web/src/channel-config-converter/__tests__/app.test.tsx`

- [ ] **Step 1: Write failing UI behavior tests**

Test upload progress, tabs `Overview/Channels and lines/Model SKUs/Sale pricing/Channel costs/Model mappings and routing/Issues/JSON`, source row display, filters, FAIL disabling formal JSON, WARN allowing JSON while marking affected entities non-publishable, issue-report download, and clearing all in-memory data.

- [ ] **Step 2: Run UI tests and confirm failure**

Run: `cd web && bun test src/channel-config-converter/__tests__/app.test.tsx`

Expected: FAIL because the preview components do not exist.

- [ ] **Step 3: Implement the converter workflow**

Keep the selected `File`, workbook bytes, parsed data, and JSON only in React memory. Use tabs for entity views, virtualized tables for large lists, severity badges, source sheet/row columns, and a read-only JSON code view. Avoid nested cards and keep compact headings suitable for an operational tool.

- [ ] **Step 4: Implement deterministic downloads**

Formal output uses MIME `application/json`, the canonical property/array order, UTF-8, and a terminal newline. FAIL permits only a read-only issue report; WARN permits formal JSON but every affected entity carries a non-publishable state and stable issue reference. Generated filenames may include source name/time but those fields do not affect payload hash.

- [ ] **Step 5: Verify and commit**

Run: `cd web && bun test src/channel-config-converter && bun run converter:build && bun run typecheck`

Expected: PASS; the corrected fixture can be previewed and downloaded without a network call or persistent browser storage.

```bash
git add web/src/channel-config-converter
git commit -m "feat: add channel converter preview and export"
```

### Task 23: Generate And Verify The Structured V2 Template

**Files:**
- Modify: `web/scripts/build-channel-config-fixtures.mjs`
- Create: `docs/templates/channel-config-v2.xlsx`
- Create: `web/src/channel-config-converter/__fixtures__/channel-config-v2-golden.xlsx`
- Modify: `web/src/channel-config-converter/__tests__/v2.test.ts`

- [ ] **Step 1: Extend the builder to create v2 from normalized v1 data**

Create explicit sheets `渠道线路` and `路由目标`; extend `渠道成本` with `line_ref`, `cost_variant_key`, and `route_target_ref`. Preserve visible source/audit fields, use formulas for derived normalized cost/profit/check cells, and make all input IDs text cells.

- [ ] **Step 2: Apply workbook validation and established visual style**

Use dropdown validation for status, cost mode, booleans, route merge mode, and known enums. Match the source workbook's header/freeze/filter/number formats. Add concise cell comments to complex formula headers and keep source URLs as plain text.

- [ ] **Step 3: Inspect, error-scan, and render every v2 sheet**

Inspect representative authoritative and formula ranges, scan for formula errors, render all used sheets, and correct clipped headers/columns or unreadable formatting. Export the same verified workbook bytes to the docs template and golden fixture locations.

- [ ] **Step 4: Add v2 golden-contract assertions**

Assert v2 parses with zero unresolved structural conflicts, all route targets have explicit line/variant references, every new channel/route proposal is disabled, and two consecutive builder runs have the same canonical hash over inspected authoritative values, formulas, sheet order, and styles.

- [ ] **Step 5: Verify and commit**

Run: `cd web && bun run scripts/build-channel-config-fixtures.mjs && bun test src/channel-config-converter/__tests__/v2.test.ts`

Expected: PASS; all sheets render legibly and converter output requires no note inference.

```bash
git add web/scripts/build-channel-config-fixtures.mjs docs/templates/channel-config-v2.xlsx web/src/channel-config-converter/__fixtures__/channel-config-v2-golden.xlsx web/src/channel-config-converter/__tests__/v2.test.ts
git commit -m "feat: add structured channel import template"
```

## Phase 5: Cross-Database, Browser, And End-To-End Acceptance

### Task 24: Add Import And Runtime End-To-End Coverage

**Files:**
- Create: `e2e/config_import_e2e_test.go`
- Create: `e2e/config_import_runtime_e2e_test.go`
- Create: `e2e/testdata/channel-config-v1.json`
- Create: `scripts/config-import-db-matrix.ps1`
- Create: `docker-compose.config-import-test.yml`
- Modify: `model/config_import_migration_test.go`

- [ ] **Step 1: Write a failing full workflow test**

Exercise upload, idempotent re-upload, 12 line bindings, one explicit `videos-standard` resolution, default user group, staging, diff review, publication, second publish rejection, one-price change import, and rollback injection. Assert expected entity counts and disabled new channels/routes.

- [ ] **Step 2: Write a failing runtime selection test**

After publication, issue representative 480p/720p, real-person/no-real-person, Secure discount/overseas/enterprise, duration, input-mode, and reference-resource requests. Assert selected channel, upstream model, `cost_variant_key`, rule ID, cost, and audit snapshots.

- [ ] **Step 3: Generate a secret-free canonical JSON fixture**

Build it from the corrected workbook through the converter library, then scan it:

Run: `rg -ni 'api[_-]?key|authorization|bearer|cookie|secret|password' e2e/testdata/channel-config-v1.json`

Expected: no matches. The fixture contains the exact `9/12/9/16/121/121/17/16/1` baseline and deterministic payload hash.

- [ ] **Step 4: Run SQLite E2E tests and confirm the final behavior**

Run: `go test ./e2e -run 'TestConfigImport' -count=1 -v`

Expected: PASS; active configuration remains unchanged before publish and after every injected failure.

- [ ] **Step 5: Add real MySQL/PostgreSQL matrix services**

`docker-compose.config-import-test.yml` runs MySQL `5.7.44` and PostgreSQL `9.6` on test-only ports with disposable named volumes. `scripts/config-import-db-matrix.ps1` waits for readiness, runs the same model/service/E2E suites once with each `SQL_DSN`, and always stops the services without deleting unrelated containers or volumes.

- [ ] **Step 6: Run the database matrix and commit**

Run: `powershell -ExecutionPolicy Bypass -File scripts/config-import-db-matrix.ps1`

Expected: SQLite, MySQL, and PostgreSQL all PASS migration, idempotency, locking, rollback, unique-index, and runtime selection tests.

```bash
git add e2e/config_import_e2e_test.go e2e/config_import_runtime_e2e_test.go e2e/testdata/channel-config-v1.json scripts/config-import-db-matrix.ps1 docker-compose.config-import-test.yml model/config_import_migration_test.go
git commit -m "test: cover config import across databases"
```

### Task 25: Verify Converter And Wizard In Real Browsers

**Files:**
- Modify: `web/package.json`
- Modify: `web/bun.lock`
- Create: `web/playwright.config-import.config.ts`
- Create: `web/e2e/channel-config-converter.spec.ts`
- Create: `web/e2e/config-import-wizard.spec.ts`
- Create: `web/e2e/helpers/config-import-fixtures.ts`

- [ ] **Step 1: Add Playwright and browser test configuration**

Run: `cd web && bun add -d @playwright/test && bunx playwright install chromium`

Expected: dependency and Chromium install succeed. Configure desktop `1440x900` and mobile `390x844` projects.

- [ ] **Step 2: Write converter browser tests**

Open `dist/channel-config-converter/index.html` with a `file:///` URL, upload corrected v1 and v2 fixtures, assert visible counts/views/issues/JSON, download and parse canonical JSON, check FAIL/WARN download gates, and assert `request`, `websocket`, and `worker` events never target a network URL. Inspect localStorage and IndexedDB before and after conversion and require both empty.

- [ ] **Step 3: Write wizard browser tests with API fixtures**

Serve the production admin build and intercept only `/api/config-import/**` plus channel-creation responses. Exercise all seven steps, existing/new/skip bindings, conflict resolution, default-group pricing, route diffs, stale response, retry, and published result. Verify route guard behavior under read/write/publish permission combinations.

- [ ] **Step 4: Add screenshot and layout assertions**

Capture each wizard step and all converter tabs at both viewports. Assert no horizontal document overflow, table overflow stays inside its scroll region, buttons retain stable dimensions, text does not overlap, focus order is usable, and visible controls have accessible names.

- [ ] **Step 5: Run browser tests and commit**

Run: `cd web && bun run converter:build && bun run build && bunx playwright test -c playwright.config-import.config.ts`

Expected: all Chromium desktop/mobile tests PASS, converter network request count is zero, and screenshots show no overlaps or clipped text.

```bash
git add web/package.json web/bun.lock web/playwright.config-import.config.ts web/e2e
git commit -m "test: verify config import browser workflows"
```

### Task 26: Run Final Security, Compatibility, And Acceptance Review

**Files:**
- Create: `docs/config-import.md`
- Create: `docs/superpowers/acceptance/2026-07-26-channel-config-json-import.md`
- Modify: `docs/superpowers/specs/2026-07-26-channel-config-json-import-design.md` only when implementation evidence requires a factual clarification; preserve approved decisions.

- [ ] **Step 1: Document the operator workflow and recovery states**

Document offline conversion, JSON upload, channel credential setup through the existing channel form, bindings, resolution choices, staging, diff review, publication, stale baseline recovery, cache refresh retry, and rollback behavior. State explicitly that Excel and credentials never enter the import API.

- [ ] **Step 2: Run all backend checks**

Run: `go test ./... -count=1`

Expected: PASS.

Run: `go vet ./...`

Expected: exit 0.

- [ ] **Step 3: Run all frontend and converter checks**

Run: `cd web && bun test && bun run typecheck && bun run lint && bun run format:check && bun run copyright:check && bun run i18n:sync && bun run build && bun run converter:build`

Expected: every command exits 0 and i18n sync leaves no diff.

- [ ] **Step 4: Run cross-database and browser matrices**

Run: `powershell -ExecutionPolicy Bypass -File scripts/config-import-db-matrix.ps1`

Run: `cd web && bunx playwright test -c playwright.config-import.config.ts`

Expected: all database and desktop/mobile projects PASS.

- [ ] **Step 5: Execute the approved acceptance scenario**

Convert the corrected output workbook offline, upload the JSON to a clean local new-api instance, bind twelve lines, resolve `CH-MEGABYAI/videos-standard`, stage, review, and publish. Record evidence for all twelve acceptance statements from design section 26, including exact counts, idempotent re-upload, one-price change, runtime variant selection, rollback, default-disabled state, and zero credential leakage.

- [ ] **Step 6: Run repository-wide credential and placeholder scans**

Run:

```powershell
rg -ni 'api[_-]?key|authorization|bearer|cookie|secret|password' e2e/testdata web/src/channel-config-converter/__fixtures__ docs/superpowers/acceptance/2026-07-26-channel-config-json-import.md
$patterns = @('T' + 'ODO', 'T' + 'BD', 'implement ' + 'later', 'fill in ' + 'details')
Select-String -Path 'docs/config-import.md','docs/superpowers/acceptance/2026-07-26-channel-config-json-import.md' -Pattern $patterns
```

Expected: the credential scan finds only explanatory statements that explicitly say credentials are excluded, and the placeholder scan returns no matches.

- [ ] **Step 7: Commit documentation and acceptance evidence**

```bash
git add docs/config-import.md docs/superpowers/acceptance/2026-07-26-channel-config-json-import.md
git commit -m "docs: record config import acceptance"
```

## Completion Criteria

- The original workbook hash is unchanged, and the corrected copy exists separately.
- Converter output matches `9/12/9/16/121/121/17/16/1` and only `CH-MEGABYAI/videos-standard` remains manual.
- `videos-fast` creates two disabled MegaByAI lines with independent credential confirmation and correct real-person constraints.
- Secure creates three disabled lines and excludes unsupported fast/mini 480p records.
- Existing calls that omit `cost_variant_key` continue to use `default`.
- Strict routing resolves the exact selected variant and never falls back across variants.
- All eight import endpoints enforce read/write/publish permissions and strict secret-free JSON.
- Re-upload, resume, stale baseline, concurrent publish, rollback, and cache-refresh retry pass tests.
- Wizard and converter pass desktop/mobile browser checks with complete seven-locale i18n.
- SQLite, MySQL, and PostgreSQL pass the same migration and publication matrix.
