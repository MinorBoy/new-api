# Channel Model Cost and Profit Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add versioned channel-model upstream cost rules, attempt-level cost accounting, strict cost-aware routing, administrator reconciliation, and billed gross-profit reporting without changing the existing user billing contract.

**Architecture:** Keep user billing and supplier cost accounting as separate ledgers. A request ledger owns billed revenue equivalent, an attempt ledger snapshots and settles every real upstream dispatch, and an append-only audit ledger records manual repairs; strict routing performs a predicted coverage check and an authoritative pre-dispatch rule check. All authoritative accounting data lives in the main database, uses Decimal strings plus checked `int64` nano-USD values, and is exposed only through a dedicated administrator permission resource.

**Tech Stack:** Go 1.22+, Gin, GORM v2, SQLite/MySQL/PostgreSQL, `shopspring/decimal`, Testify, React 19, TypeScript, TanStack Query/Router/Table, React Hook Form, Zod, Base UI/Tailwind, Bun, i18next.

---

## File Map And Dependency Rules

The implementation must preserve the dependency direction `router -> controller -> service -> model`; shared cost enums, rule config, meters, and runtime handles live in `types` so `model`, `service`, and `relay` do not import each other cyclically.

**Domain and configuration**

- Create `types/cost_accounting.go`: statuses, modes, charge events, meter sources, versioned rule config, capability contract, nullable meter, request/attempt runtime handles.
- Create `setting/cost_setting/config.go`: atomic `disabled`/`strict` runtime setting, registered as `cost_setting.mode` and defaulting to `disabled`.
- Modify `model/option.go`: call `cost_setting.UpdateAndSync()` after layered option updates.
- Create `service/cost_amount.go`: canonical Decimal parsing, rule normalization, cost/revenue conversion, checked nano-USD arithmetic, and aggregate margin calculation.

**Persistence and business services**

- Create `model/channel_model_cost_rule.go`: versioned drafts and transactional activation/retirement using `lockForUpdate`.
- Create `model/cost_accounting.go`: request, attempt, and append-only audit models plus conditional state transitions and transactional request recomputation.
- Modify `model/main.go`: add all four tables to normal and fast migrations.
- Create `service/cost_rule.go`: config validation, capability validation, active-rule coverage cache, draft/version lifecycle, and preview.
- Create `service/cost_accounting.go`: prepare/dispatch/settle APIs, revenue recognition, winning-attempt attribution, and non-client-cancelled persistence contexts.
- Create `service/cost_recovery.go`: stale-state recovery and manual reconciliation using immutable snapshots.
- Create `service/cost_report.go`: request detail, anomaly queue, checked report aggregation, and channel/model attribution.

**Relay and routing integration**

- Modify `relay/common/relay_info.go`: predicted/final cost identity and current accounting handles; extend `TaskInfo` with an optional authoritative cost meter.
- Modify `relay/channel/adapter.go`: optional sync and async cost-contract interfaces; do not expand the mandatory base adaptor interfaces.
- Create `relay/cost_accounting_adaptor.go`: wrapper around sync adaptors and the explicit protocol/task capability registry.
- Modify `relay/relay_adaptor.go`: wrap sync adaptors and expose capability lookup to `service` through an injected function initialized in `main.go`.
- Modify JSON relay handlers after request conversion and parameter overrides: `relay/compatible_handler.go`, `relay/responses_handler.go`, `relay/claude_handler.go`, `relay/gemini_handler.go`, `relay/embedding_handler.go`, `relay/image_handler.go`, `relay/audio_handler.go`, `relay/rerank_handler.go`, and `relay/chat_completions_via_responses.go`.
- Modify `service/model_routing.go`, `service/channel_select.go`, `model/channel_satisfy.go`, and `controller/relay.go`: apply exclusions to ordinary routing, capability routing, known channels, affinity, and retries.
- Modify `relay/relay_task.go`, `model/task.go`, `service/task_polling.go`, and `service/task_billing.go`: prepare before async submit, persist the cost request ID, and settle from the frozen attempt snapshot during polling.
- Modify `service/billing.go`, `service/log_info_generate.go`, `service/quota.go`, and `service/text_quota.go`: recognize revenue after user settlement and attach only an administrator ledger reference to consume logs.

**Administrator API**

- Create `dto/cost_accounting.go`: pointer-based request DTOs and string-serialized nano-USD response DTOs.
- Create `service/authz/resources_cost_accounting.go`: `read`, `write`, and `reconcile` permissions.
- Create `controller/cost_accounting.go`: rule, coverage, settings, detail, anomaly, reconciliation, preview, and report handlers.
- Create `router/cost-accounting-router.go`; modify `router/api-router.go` to register it.

**Frontend**

- Create `web/src/features/cost-accounting/types.ts`, `api.ts`, `lib/cost-rule.ts`, and focused tests.
- Create `web/src/features/cost-accounting/components/channel-cost-drawer.tsx`, `cost-rule-drawer.tsx`, `coverage-panel.tsx`, `cost-request-detail.tsx`, `anomaly-queue.tsx`, `profit-summary.tsx`, `profit-table.tsx`, and `profit-filters.tsx`.
- Create `web/src/features/cost-accounting/index.tsx` and `web/src/routes/_authenticated/cost-accounting/index.tsx`.
- Modify channel provider/dialog/action files, usage-log detail/types, administrator permission constants, sidebar data/config, and all seven locale files.

The first implementation must not add Excel/CSV import, live price synchronization, executable cost expressions, historical backfill, cash-revenue allocation, or mutation of already confirmed amounts.

### Task 1: Define Cost Domain Types, Safe Amount Math, And Runtime Mode

**Files:**

- Create: `types/cost_accounting.go`
- Create: `types/cost_accounting_test.go`
- Create: `service/cost_amount.go`
- Create: `service/cost_amount_test.go`
- Create: `setting/cost_setting/config.go`
- Create: `setting/cost_setting/config_test.go`
- Modify: `model/option.go`

- [ ] **Step 1: Write failing domain, amount, and setting tests**

Define table tests that cover all four modes, explicit zero versus missing meter values, canonical Decimal normalization, the complete currency formula, half-away-from-zero nano rounding, `int64` overflow rejection, negative cost/revenue rejection, zero-revenue null margin, negative profit, and the default-disabled setting. Use these exact boundary assertions:

```go
func TestDecimalToNanoUSD(t *testing.T) {
	value, err := DecimalToNanoUSD(decimal.RequireFromString("1.0000000005"))
	require.NoError(t, err)
	assert.Equal(t, int64(1_000_000_001), value)

	_, err = DecimalToNanoUSD(decimal.RequireFromString("9223372036.854775808"))
	assert.ErrorIs(t, err, ErrNanoUSDOverflow)
}

func TestNormalizeRulePrice(t *testing.T) {
	unitPrice := "10"
	config := types.CostRuleConfigV1{
		Currency: "CNY", BillingMultiplier: "1.2",
		PurchaseDiscountRatio: "0.8", RechargeExchangeRatio: "2",
		FeeRate: "0.05", CurrencyToUSDRate: "0.14",
		UnitPrice: &unitPrice,
	}
	normalized, err := NormalizeCostRuleConfig(types.CostModePerRequest, config)
	require.NoError(t, err)
	assert.Equal(t, "0.7056", *normalized.NormalizedUSDPrices.UnitPrice)
}

func TestRevenueEquivalentNanoUSD(t *testing.T) {
	got, err := RevenueEquivalentNanoUSD(500_000, "500000")
	require.NoError(t, err)
	assert.Equal(t, int64(1_000_000_000), got)
	_, err = RevenueEquivalentNanoUSD(-1, "500000")
	assert.Error(t, err)
}
```

- [ ] **Step 2: Run the focused tests and verify RED**

Run:

```text
go test ./types ./service ./setting/cost_setting -run 'Cost|NanoUSD|RevenueEquivalent' -count=1
```

Expected: compilation fails because cost domain types, amount helpers, and `cost_setting` do not exist.

- [ ] **Step 3: Add the exact shared contracts and checked math**

Use string-backed enums and pointer fields wherever absence differs from zero. The public contract must contain these declarations and no floating-point accounting fields:

```go
type CostAccountingMode string
const (
	CostAccountingDisabled CostAccountingMode = "disabled"
	CostAccountingStrict   CostAccountingMode = "strict"
)

type CostMode string
const (
	CostModeFree        CostMode = "free"
	CostModePerRequest  CostMode = "per_request"
	CostModePerDuration CostMode = "per_duration"
	CostModePerToken    CostMode = "per_token"
)

type CostRuleStatus string
const (
	CostRuleDraft   CostRuleStatus = "draft"
	CostRuleActive  CostRuleStatus = "active"
	CostRuleRetired CostRuleStatus = "retired"
)

type CostRevenueStatus string
const (
	CostRevenuePending       CostRevenueStatus = "pending"
	CostRevenueSettled       CostRevenueStatus = "settled"
	CostRevenueConfirmedZero CostRevenueStatus = "confirmed_zero"
	CostRevenueFailed        CostRevenueStatus = "revenue_failed"
)

type CostProfitStatus string
const (
	CostProfitComplete          CostProfitStatus = "complete"
	CostProfitIncompleteCost    CostProfitStatus = "incomplete_cost"
	CostProfitIncompleteRevenue CostProfitStatus = "incomplete_revenue"
)

type CostChargeEvent string
const (
	CostChargeResponseSucceeded CostChargeEvent = "response_succeeded"
	CostChargeSubmitAccepted    CostChargeEvent = "submit_accepted"
	CostChargeTaskSucceeded     CostChargeEvent = "task_succeeded"
)

type CostMeterSource string
const (
	CostMeterValidatedRequest CostMeterSource = "validated_request"
	CostMeterUpstreamActual   CostMeterSource = "upstream_actual"
	CostMeterUpstreamUsage    CostMeterSource = "upstream_usage"
	CostMeterLocalUsage       CostMeterSource = "local_usage"
)

type CostAttemptStatus string
const (
	CostAttemptPrepared         CostAttemptStatus = "prepared"
	CostAttemptDispatching      CostAttemptStatus = "dispatching"
	CostAttemptNotDispatched    CostAttemptStatus = "not_dispatched"
	CostAttemptAwaitingMeter    CostAttemptStatus = "awaiting_meter"
	CostAttemptSettled          CostAttemptStatus = "settled"
	CostAttemptConfirmedZero    CostAttemptStatus = "confirmed_zero"
	CostAttemptUnknown          CostAttemptStatus = "cost_unknown"
	CostAttemptSettlementFailed CostAttemptStatus = "settlement_failed"
)

type CostMeter struct {
	Source            CostMeterSource `json:"source"`
	DurationSeconds   *string         `json:"duration_seconds,omitempty"`
	InputTokens       *int64          `json:"input_tokens,omitempty"`
	OutputTokens      *int64          `json:"output_tokens,omitempty"`
	CompletionTokens  *int64          `json:"completion_tokens,omitempty"`
	TotalTokens       *int64          `json:"total_tokens,omitempty"`
}

type CostCapabilities struct {
	CanResolveBillableModel bool              `json:"can_resolve_billable_model"`
	ChargeEvents            []CostChargeEvent `json:"charge_events"`
	MeterSources            []CostMeterSource `json:"meter_sources"`
}

type CostAttemptHandle struct {
	CostRequestID int64
	AttemptID     int64
	AttemptNo     int
	CostMode      CostMode
	ChargeEvent   CostChargeEvent
}

type CostOutcome struct {
	Status           CostAttemptStatus
	UpstreamAccepted bool
	FailureCode      string
}
```

`CostRuleConfigV1` must include `currency`, positive multiplier/exchange/rate strings, non-negative `fee_rate`, mode-specific pointer prices, `zero_cost_reason`, `charge_event`, `meter_source`, `token_mode`, and a `normalized_usd_prices` object. Implement:

```text
normalized_usd_price = original_price
  * billing_multiplier
  * purchase_discount_ratio
  / recharge_exchange_ratio
  * (1 + fee_rate)
  * currency_to_usd_rate
```

```go
func NormalizeCostRuleConfig(mode types.CostMode, in types.CostRuleConfigV1) (types.CostRuleConfigV1, error)
func CalculateAttemptCost(mode types.CostMode, config types.CostRuleConfigV1, meter types.CostMeter) (originalCost string, nanoUSD int64, err error)
func DecimalToNanoUSD(amount decimal.Decimal) (int64, error)
func RevenueEquivalentNanoUSD(finalQuota int64, quotaPerUnitSnapshot string) (int64, error)
func GrossMarginPPM(profitNanoUSD, revenueNanoUSD int64) (*int64, error)
func CheckedNanoAdd(left, right int64) (int64, error)
func CheckedNanoSubtract(left, right int64) (int64, error)
```

Check the Decimal against `math.MaxInt64`/`math.MinInt64` before `IntPart()`. Cost and revenue helpers reject negative results; profit and margin allow negative values. Register `cost_setting.mode`, publish an atomic snapshot, validate only `disabled` and `strict`, and call `cost_setting.UpdateAndSync()` from `model.handleConfigUpdate`.

- [ ] **Step 4: Run focused tests and verify GREEN**

Run:

```text
go test ./types ./service ./setting/cost_setting ./model -run 'Cost|NanoUSD|RevenueEquivalent|ConfigUpdate' -count=1
```

Expected: all focused tests pass; no Decimal conversion silently saturates or wraps.

- [ ] **Step 5: Commit the domain foundation**

```text
git add types/cost_accounting.go types/cost_accounting_test.go service/cost_amount.go service/cost_amount_test.go setting/cost_setting/config.go setting/cost_setting/config_test.go model/option.go
git commit -m "feat: add cost accounting domain primitives"
```

### Task 2: Add Versioned Rules And Accounting Ledgers

**Files:**

- Create: `model/channel_model_cost_rule.go`
- Create: `model/channel_model_cost_rule_test.go`
- Create: `model/cost_accounting.go`
- Create: `model/cost_accounting_test.go`
- Modify: `model/main.go`

- [ ] **Step 1: Write failing migration, uniqueness, and state-transition tests**

Use an in-memory SQLite database with one open connection. Assert that rule versions are unique on `(channel_id, billable_upstream_model, version)`, requests are unique by `request_id`, non-null `task_id` is unique while multiple null task IDs are allowed, attempts are unique by `(cost_request_id, attempt_no)`, and an attempt settlement plus request recomputation commits atomically. Add a conditional-update regression:

```go
func TestTransitionCostAttemptRequiresExpectedState(t *testing.T) {
	prepareCostAccountingDB(t)
	attempt := seedPreparedAttempt(t)

	require.NoError(t, TransitionCostAttempt(attempt.ID, types.CostAttemptPrepared, types.CostAttemptDispatching, nil))
	err := TransitionCostAttempt(attempt.ID, types.CostAttemptPrepared, types.CostAttemptDispatching, nil)
	assert.ErrorIs(t, err, ErrCostStateConflict)
}
```

- [ ] **Step 2: Run model tests and verify RED**

Run:

```text
go test ./model -run 'ChannelModelCostRule|CostAccounting|CostAttempt' -count=1
```

Expected: compilation fails because the models and transition functions do not exist.

- [ ] **Step 3: Implement four cross-database models and model transactions**

Create the following persistence shapes with finite `varchar` indexes and `text` JSON/Decimal snapshots:

```go
type ChannelModelCostRule struct {
	ID int64 `gorm:"primaryKey"`
	ChannelID int `gorm:"uniqueIndex:idx_cost_rule_version,priority:1;index"`
	BillableUpstreamModel string `gorm:"type:varchar(191);uniqueIndex:idx_cost_rule_version,priority:2;index"`
	Version int `gorm:"uniqueIndex:idx_cost_rule_version,priority:3"`
	Status string `gorm:"type:varchar(32);index"`
	CostMode string `gorm:"type:varchar(32)"`
	SchemaVersion int
	ConfigJSON string `gorm:"type:text"`
	Source string `gorm:"type:varchar(32)"`
	Note string `gorm:"type:text"`
	CreatedBy int
	ActivatedBy int
	EffectiveFrom *int64 `gorm:"index"`
	EffectiveTo *int64
	CreatedAt int64
	UpdatedAt int64
}

type CostAccountingRequest struct {
	ID int64 `gorm:"primaryKey"`
	RequestID string `gorm:"type:varchar(64);uniqueIndex"`
	TaskID *string `gorm:"type:varchar(191);uniqueIndex"`
	UserID int `gorm:"index"`
	TokenID int
	UserGroup string `gorm:"type:varchar(64);index"`
	UsingGroup string `gorm:"type:varchar(64);index"`
	OriginModelName string `gorm:"type:varchar(191);index"`
	BillingSource string `gorm:"type:varchar(32);index"`
	SubscriptionID int
	SubscriptionPlanID int
	FinalUserQuota *int64
	QuotaPerUnitSnapshot string `gorm:"type:varchar(64)"`
	BilledRevenueEquivalentNanoUSD *int64
	ConfirmedCostNanoUSD int64
	AttemptCount int
	WinningAttemptID *int64
	BilledGrossProfitNanoUSD *int64
	GrossMarginPPM *int64
	RevenueStatus string `gorm:"type:varchar(32);index"`
	ProfitStatus string `gorm:"type:varchar(32);index"`
	FailureCode string `gorm:"type:varchar(64);index"`
	RequestedAt int64 `gorm:"index"`
	RevenueSettledAt *int64
	ProfitRecognizedAt *int64 `gorm:"index"`
	CreatedAt int64
	UpdatedAt int64
}
```

`CostAccountingAttempt` must contain every channel/model/rule snapshot, `billable_request_count` fixed to one, request and actual meter JSON, nullable `cost_nano_usd`, original-currency Decimal, stable result/failure codes, reconciliation status, and lifecycle timestamps. It must never store keys, authorization headers, prompts, full request bodies, or raw response bodies. `CostAccountingAudit` must contain request/attempt IDs, administrator, old/new state, meter JSON, rule ID/version, old/new amount, reason, and timestamp; provide create/list functions only, never update/delete functions.

Expose model operations with exact state checks:

```go
func CreateCostRuleDraft(rule *ChannelModelCostRule) error
func ActivateChannelModelCostRule(id int64, adminID int, now int64, validate func(*ChannelModelCostRule) error) (*ChannelModelCostRule, error)
func RetireChannelModelCostRule(id int64, adminID int, now int64) error
func PrepareCostAttempt(request *CostAccountingRequest, attempt *CostAccountingAttempt) error
func TransitionCostAttempt(id int64, from, to types.CostAttemptStatus, updates map[string]any) error
func SettleCostAttempt(input SettleCostAttemptInput) error
func RecognizeCostRevenue(input RecognizeCostRevenueInput) error
func ReconcileCostAttempt(input ReconcileCostAttemptInput) error
```

Activation and reconciliation transactions must call `lockForUpdate(tx)`; every CAS update must require `RowsAffected == 1`. Request recomputation sets `profit_recognized_at` only on the first transition to `complete` and never moves that timestamp later. Add all four models to both `migrateDB` and `migrateDBFast`.

- [ ] **Step 4: Run model tests and verify GREEN**

Run:

```text
go test ./model -run 'ChannelModelCostRule|CostAccounting|CostAttempt|Migrate' -count=1
```

Expected: all model tests pass on SQLite and the generated schema uses no JSON column, partial index, cascade delete, or boolean default tag.

- [ ] **Step 5: Commit persistence**

```text
git add model/channel_model_cost_rule.go model/channel_model_cost_rule_test.go model/cost_accounting.go model/cost_accounting_test.go model/main.go
git commit -m "feat: add cost accounting ledgers"
```

### Task 3: Implement Rule Validation, Version Activation, Coverage Cache, And Preview

**Files:**

- Create: `service/cost_rule.go`
- Create: `service/cost_rule_test.go`
- Create: `service/cost_preview.go`
- Create: `service/cost_preview_test.go`

- [ ] **Step 1: Write failing rule lifecycle and preview tests**

Cover every invalid configuration from the design: missing model/channel, unknown mode/event/source, free without reason, zero or negative non-free price, invalid multiplier/rate, extra token-mode fields, capability mismatch, duplicate active version, and inconsistent multi-key/path contract. Assert activation retires the previous version at the same timestamp and leaves historical config unchanged. Assert cache invalidation after activation/retirement and authoritative DB fallback on a cached miss.

For preview, use a deterministic example with selected group, final user quota, and meter:

```go
func TestPreviewCostAndBilledGrossProfit(t *testing.T) {
	preview, err := PreviewChannelModelCost(PreviewCostInput{
		FinalUserQuota: 500_000, QuotaPerUnitSnapshot: "500000",
		CostMode: types.CostModePerRequest, Config: normalizedUSDPerRequest("0.2"),
		Meter: types.CostMeter{Source: types.CostMeterValidatedRequest},
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1_000_000_000), preview.RevenueNanoUSD)
	assert.Equal(t, int64(200_000_000), preview.CostNanoUSD)
	assert.Equal(t, int64(800_000_000), preview.ProfitNanoUSD)
	assert.Equal(t, int64(800_000), *preview.MarginPPM)
}
```

- [ ] **Step 2: Run service tests and verify RED**

Run:

```text
go test ./service -run 'CostRule|CostCoverage|PreviewChannelModelCost' -count=1
```

Expected: compilation fails because rule lifecycle, coverage cache, and preview services do not exist.

- [ ] **Step 3: Implement validation and lifecycle services**

Add an injected capability lookup to avoid a `service -> relay` cycle:

```go
var CostCapabilityLookup func(channelType int, requestPath string, taskPlatform constant.TaskPlatform) types.CostCapabilities

func ValidateCostRuleDraft(rule *model.ChannelModelCostRule, capabilities types.CostCapabilities) (types.CostRuleConfigV1, error)
func CreateCostRuleDraft(input CreateCostRuleInput) (*model.ChannelModelCostRule, error)
func UpdateCostRuleDraft(id int64, input UpdateCostRuleInput) (*model.ChannelModelCostRule, error)
func ActivateCostRule(id int64, adminID int) (*model.ChannelModelCostRule, error)
func RetireCostRule(id int64, adminID int) error
func ActiveCostRule(channelID int, billableModel string, authoritative bool) (*model.ChannelModelCostRule, error)
func CheckPredictedCostCoverage(input PredictedCoverageInput) (bool, error)
func InvalidateCostCoverage(channelID int, billableModel string)
func PreviewFinalUserQuota(ctx *gin.Context, info *relaycommon.RelayInfo, input UserBillingPreviewInput) (int64, error)
func PreviewChannelModelCost(input PreviewCostInput) (CostPreview, error)
```

Draft creation sets `schema_version=1` and `source=manual`, assigns the next version in a transaction, and retries a version-unique conflict once. Serialize only the validated versioned DTO through `common.Marshal`; decode snapshots through `common.Unmarshal`/`common.UnmarshalJsonStr`, never direct `encoding/json` calls or caller-supplied JSON text. Draft updates reject `active` and `retired` rows; active configuration is never edited in place. Activation is immediate, stamps one `effective_from`, retires the old active version with the same `effective_to`, and never supports reactivation or scheduled effective times. Activation passes a validator callback into the model transaction so rule config and current adaptor capabilities are checked while the business key is locked. A cache miss must query the main DB once before returning uncovered. Channel deletion and model-mapping updates call `InvalidateCostCoverage(channelID, "")`.

`PreviewFinalUserQuota` reuses the package's existing text, per-request, per-duration, and expression settlement calculations against sample usage without mutating quota, counters, or logs. `PreviewChannelModelCost` accepts that computed user-billing quota plus its immutable `quota_per_unit_snapshot`; the controller-facing DTO obtains both through the existing price helper. It must return an `estimated: true` marker and never write a ledger row.

- [ ] **Step 4: Run service tests and verify GREEN**

Run:

```text
go test ./service ./model -run 'CostRule|CostCoverage|PreviewChannelModelCost' -count=1
```

Expected: rule lifecycle, cache fallback, immutable snapshots, and exact preview arithmetic pass.

- [ ] **Step 5: Commit rule services**

```text
git add service/cost_rule.go service/cost_rule_test.go service/cost_preview.go service/cost_preview_test.go
git commit -m "feat: add channel cost rule lifecycle"
```

### Task 4: Add Administrator Permissions And Rule APIs

**Files:**

- Create: `dto/cost_accounting.go`
- Create: `service/authz/resources_cost_accounting.go`
- Create: `controller/cost_accounting.go`
- Create: `controller/cost_accounting_test.go`
- Create: `relay/helper/cost_preview.go`
- Create: `relay/helper/cost_preview_test.go`
- Create: `router/cost-accounting-router.go`
- Create: `router/cost_accounting_router_test.go`
- Modify: `router/api-router.go`
- Modify: `service/authz/authz_test.go`

- [ ] **Step 1: Write failing permission, route, validation, and privacy tests**

Assert the administrator baseline receives `read`, `write`, and `reconcile`; ordinary users receive none. Assert each route maps to its intended permission and all responses serialize Decimal strings and nano-USD as base-10 strings. Send missing versus explicit-zero request fields and verify the controller preserves the distinction.

```go
func TestCostAccountingRoutesUseDedicatedPermissions(t *testing.T) {
	assertCostRoute(t, http.MethodGet, "/rules", authz.CostAccountingRead)
	assertCostRoute(t, http.MethodPost, "/rules", authz.CostAccountingWrite)
	assertCostRoute(t, http.MethodPut, "/rules/:id", authz.CostAccountingWrite)
	assertCostRoute(t, http.MethodPost, "/attempts/:id/reconcile", authz.CostAccountingReconcile)
	assertCostRoute(t, http.MethodGet, "/reports/summary", authz.CostAccountingRead)
}
```

- [ ] **Step 2: Run controller/router/authz tests and verify RED**

Run:

```text
go test ./controller ./router ./service/authz ./relay/helper -run 'CostAccounting|CostRoute|CostPreview' -count=1
```

Expected: compilation fails because the DTOs, resource, handlers, and routes do not exist.

- [ ] **Step 3: Implement the dedicated administrator API surface**

Register `/api/cost-accounting` behind `AdminAuth` and dedicated permissions with these routes:

```text
GET    /settings
PUT    /settings
GET    /rules
POST   /rules
PUT    /rules/:id
POST   /rules/:id/validate
POST   /rules/:id/activate
POST   /rules/:id/retire
GET    /rules/:id/history
POST   /preview
GET    /coverage
GET    /requests/:id
GET    /anomalies
POST   /attempts/:id/reconcile
POST   /requests/:id/reconcile-revenue
GET    /reports/summary
GET    /reports/breakdown
```

Use request DTO pointer fields such as:

```go
type ReconcileCostAttemptRequest struct {
	Action string `json:"action" binding:"required"`
	Meter *types.CostMeter `json:"meter,omitempty"`
	Reason string `json:"reason" binding:"required"`
}

type UpdateCostAccountingModeRequest struct {
	Mode types.CostAccountingMode `json:"mode" binding:"required"`
}
```

Enabling `strict` must run authoritative coverage and reject with a stable `cost_coverage_incomplete` code when any enabled channel/model/path is uncovered. The client message may identify affected channel/model to administrators, but relay errors must remain generic. Response DTOs expose nano values as strings produced by `strconv.FormatInt`, never JavaScript-unsafe JSON numbers.

The preview request must carry `origin_model`, selected `user_group`, relay mode, sample usage, optional validated duration, optional expression request input, and the draft rule/meter. Implement the adapter between the existing user-pricing path and the cost preview with this exact entry point:

```go
func PreviewUserBillingQuota(c *gin.Context, input dto.CostPreviewRequest) (finalQuota int64, quotaPerUnitSnapshot string, err error)
```

Build an isolated Gin context and `RelayInfo`, reuse `ModelPriceHelper`/`ModelPriceHelperPerCall`, and call the pure preview calculation in `service/cost_preview.go` for token, per-request, per-duration, and `billing_expr` user-price modes. The helper must never pre-consume, settle, update usage counters, or write logs. Pass its returned quota and snapshot to `service.PreviewChannelModelCost`; this makes the selected group affect billed revenue through the same pricing rules used by real requests.

- [ ] **Step 4: Run API tests and verify GREEN**

Run:

```text
go test ./controller ./router ./service/authz ./relay/helper -run 'CostAccounting|CostRoute|CostPreview' -count=1
```

Expected: permissions, request validation, route registration, and response privacy tests pass.

- [ ] **Step 5: Commit the rule API**

```text
git add dto/cost_accounting.go service/authz/resources_cost_accounting.go service/authz/authz_test.go controller/cost_accounting.go controller/cost_accounting_test.go relay/helper/cost_preview.go relay/helper/cost_preview_test.go router/cost-accounting-router.go router/cost_accounting_router_test.go router/api-router.go
git commit -m "feat: add cost accounting admin api"
```

### Task 5: Establish Final Billable Identity And Adaptor Cost Contracts

**Files:**

- Modify: `relay/common/relay_info.go`
- Modify: `relay/channel/adapter.go`
- Create: `relay/cost_accounting_adaptor.go`
- Create: `relay/cost_accounting_adaptor_test.go`
- Modify: `relay/relay_adaptor.go`
- Modify: `main.go`
- Modify: `relay/compatible_handler.go`
- Modify: `relay/responses_handler.go`
- Modify: `relay/claude_handler.go`
- Modify: `relay/gemini_handler.go`
- Modify: `relay/embedding_handler.go`
- Modify: `relay/image_handler.go`
- Modify: `relay/audio_handler.go`
- Modify: `relay/rerank_handler.go`
- Modify: `relay/chat_completions_via_responses.go`
- Modify: `relay/helper/model_mapped_routing_test.go`

- [ ] **Step 1: Write failing identity and capability tests**

Test final identity after ordinary model mapping, capability target mapping, adaptor suffix rewriting, JSON parameter override, and pass-through JSON. Assert strict mode rejects an empty final identity before any fake upstream transport is called. Assert a token rule cannot activate against a contract that only supports per-request events.

```go
func TestConfirmCostIdentityUsesFinalOverriddenModel(t *testing.T) {
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "mapped-model"}}
	contract := jsonModelCostContract()
	err := contract.ConfirmCostIdentity(info, []byte(`{"model":"final-override-model"}`))
	require.NoError(t, err)
	assert.Equal(t, "final-override-model", info.BillableUpstreamModel)
}
```

- [ ] **Step 2: Run relay tests and verify RED**

Run:

```text
go test ./relay ./relay/helper -run 'CostIdentity|CostCapabilities|ModelMapped' -count=1
```

Expected: compilation fails because the final identity fields and adaptor cost contracts do not exist.

- [ ] **Step 3: Add optional adaptor contracts and explicit final identity confirmation**

Add these optional interfaces without changing `Adaptor` or `TaskAdaptor`:

```go
type CostAccountingAdaptor interface {
	CostCapabilities(info *relaycommon.RelayInfo) types.CostCapabilities
	ConfirmCostIdentity(info *relaycommon.RelayInfo, finalRequestBody []byte) error
	NormalizeCostMeter(info *relaycommon.RelayInfo, usage any) (types.CostMeter, error)
	ClassifyCostOutcome(info *relaycommon.RelayInfo, response *http.Response, requestErr error) types.CostOutcome
}

type TaskCostAccountingAdaptor interface {
	CostCapabilities(info *relaycommon.RelayInfo) types.CostCapabilities
	ConfirmTaskCostIdentity(info *relaycommon.RelayInfo) error
	NormalizeTaskCostMeter(task *model.Task, result *relaycommon.TaskInfo) (types.CostMeter, error)
}
```

Add `PredictedUpstreamModel`, `BillableUpstreamModel`, `CostRequestID`, and `CostAttempt *types.CostAttemptHandle` to `RelayInfo`. Add `CostMeter *types.CostMeter` to `TaskInfo` with `json:"-"`.

Implement `costAccountingAdaptor` by embedding the original `channel.Adaptor`; override only `DoRequest`, `DoResponse`, and the optional cost-contract methods. The contract registry must explicitly declare supported charge events, sources, model resolution, and failure classification per protocol/task platform. An absent contract means uncovered, not zero cost. For OpenAI/Claude/Gemini usage, normalize only authoritative `dto.BillingUsage` or the protocol-native authoritative usage; reject `Estimated == true` for `upstream_usage`.

Explicitly register unsupported send paths such as realtime WebSocket or legacy Midjourney protocols as uncovered until they have a tested identity/meter contract. Strict coverage must block those paths before sending; disabled mode preserves their existing behavior.

Each JSON handler must call `ConfirmCostIdentity` after `RemoveDisabledFields` and `ApplyParamOverrideWithRelayInfo`, immediately before constructing the outbound reader. Pass-through paths must inspect the stored body through structured JSON decoding without retaining it in the ledger. Non-JSON paths must use a contract-specific resolver.

Set `service.CostCapabilityLookup = relay.CostCapabilitiesForRoute` in `main.go` alongside the existing task adaptor injection.

- [ ] **Step 4: Run identity and capability tests and verify GREEN**

Run:

```text
go test ./relay ./relay/helper ./dto -run 'CostIdentity|CostCapabilities|ModelMapped|BillingUsage' -count=1
```

Expected: all final-model permutations and authoritative-meter capability tests pass; strict mode never calls transport without a confirmed identity.

- [ ] **Step 5: Commit adaptor contracts**

```text
git add relay/common/relay_info.go relay/channel/adapter.go relay/cost_accounting_adaptor.go relay/cost_accounting_adaptor_test.go relay/relay_adaptor.go main.go relay/compatible_handler.go relay/responses_handler.go relay/claude_handler.go relay/gemini_handler.go relay/embedding_handler.go relay/image_handler.go relay/audio_handler.go relay/rerank_handler.go relay/chat_completions_via_responses.go relay/helper/model_mapped_routing_test.go
git commit -m "feat: add upstream cost adaptor contracts"
```

### Task 6: Apply Strict Coverage To Every Routing Path

**Files:**

- Modify: `service/channel_select.go`
- Modify: `service/model_routing.go`
- Modify: `service/model_routing_test.go`
- Modify: `model/channel_satisfy.go`
- Modify: `model/channel_routing_filter_test.go`
- Modify: `controller/relay.go`
- Create: `controller/cost_routing_test.go`
- Modify: `relay/helper/model_mapped.go`
- Create: `service/model_mapping.go`
- Create: `service/model_mapping_test.go`

- [ ] **Step 1: Write failing routing coverage tests**

Cover normal random selection, capability routing, known/locked channel, affinity, sync retry, async retry, and auto-group retry. For each path, seed one uncovered higher-priority channel and one covered lower-priority channel and assert the covered channel is selected without sending to the uncovered one. Assert cached-all-missing triggers one authoritative DB lookup before rejection. Assert all-uncovered returns the existing generic unavailable response and does not include `cost`, `price`, `rule`, or the upstream model.

- [ ] **Step 2: Run routing tests and verify RED**

Run:

```text
go test ./model ./service ./controller -run 'CostRouting|ChannelRoutingFilter|ModelMapping' -count=1
```

Expected: at least the ordinary-route case fails because exclusions are currently attached only when capability routing is active.

- [ ] **Step 3: Generalize exclusions and predicted coverage checks**

Rename the method and make exclusions unconditional:

```go
func (p *RetryParam) ExcludeChannel(channelID int) {
	if p.ExcludedChannelIDs == nil {
		p.ExcludedChannelIDs = map[int]struct{}{}
	}
	p.ExcludedChannelIDs[channelID] = struct{}{}
}
```

Always assign `filter.ExcludedChannelIDs = param.ExcludedChannelIDs` in `selectChannelForGroup`, including non-capability routing. Make `ValidateKnownChannelForRouting` reject excluded channels even without a capability policy.

Extract the deterministic mapping chain from `relay/helper/model_mapped.go` into:

```go
func ResolveMappedModel(originModel, mappingJSON string) (mapped string, changed bool, err error)
```

Use it in both `ModelMappedHelper` and predicted coverage. After `SetupContextForSelectedChannel`, calculate the predicted model from the capability target or channel model mapping; call `CheckPredictedCostCoverage`. On an uncovered result, call `ExcludeChannel` and continue selection before request conversion. When authoritative pre-send validation later returns `*service.CostCoverageError`, exclude the same channel and retry without exposing the internal error.

- [ ] **Step 4: Run routing tests and verify GREEN**

Run:

```text
go test ./model ./service ./controller ./relay/helper -run 'CostRouting|ChannelRoutingFilter|ModelRouting|ModelMapping' -count=1
```

Expected: all selection paths skip uncovered channels, and legacy behavior is unchanged while cost mode is disabled.

- [ ] **Step 5: Commit strict routing**

```text
git add service/channel_select.go service/model_routing.go service/model_routing_test.go service/model_mapping.go service/model_mapping_test.go model/channel_satisfy.go model/channel_routing_filter_test.go controller/relay.go controller/cost_routing_test.go relay/helper/model_mapped.go
git commit -m "feat: enforce strict cost coverage routing"
```

### Task 7: Implement Attempt Prepare, Dispatch, Settlement, And Recovery-Safe Contexts

**Files:**

- Create: `service/cost_accounting.go`
- Create: `service/cost_accounting_test.go`
- Modify: `relay/common/relay_utils.go`
- Modify: `relay/helper/valid_request.go`
- Modify: `relay/cost_accounting_adaptor.go`
- Modify: `relay/cost_accounting_adaptor_test.go`

- [ ] **Step 1: Write failing attempt lifecycle tests**

Use a fake transport counter and assert: `prepared` is committed first; `dispatching` is committed by CAS before transport; failed CAS performs zero sends; each real retry gets a new monotonic attempt number; pre-send conversion failure creates no attempt; explicit free becomes `confirmed_zero`; known no-charge becomes `confirmed_zero`; ambiguous timeout becomes `cost_unknown`; missing meter becomes `settlement_failed`; client cancellation does not cancel post-send persistence.

```go
func TestAuthorizeDispatchBeforeTransport(t *testing.T) {
	handle, err := PrepareCostAttempt(context.Background(), preparedInput())
	require.NoError(t, err)
	require.Equal(t, types.CostAttemptPrepared, loadAttempt(t, handle.AttemptID).Status)
	require.NoError(t, AuthorizeCostDispatch(context.Background(), handle))
	assert.Equal(t, types.CostAttemptDispatching, loadAttempt(t, handle.AttemptID).Status)
}
```

- [ ] **Step 2: Run lifecycle tests and verify RED**

Run:

```text
go test ./service ./relay -run 'CostAttempt|AuthorizeDispatch|CostOutcome' -count=1
```

Expected: compilation fails because the lifecycle service is not implemented.

- [ ] **Step 3: Implement authoritative pre-send and post-send state handling**

Add these service entry points:

```go
func PrepareCostAttempt(ctx context.Context, input PrepareCostAttemptInput) (*types.CostAttemptHandle, error)
func AuthorizeCostDispatch(ctx context.Context, handle *types.CostAttemptHandle) error
func RecordCostDispatchOutcome(ctx context.Context, handle *types.CostAttemptHandle, outcome types.CostOutcome) error
func SettleSyncCostAttempt(ctx context.Context, handle *types.CostAttemptHandle, meter types.CostMeter) error
func MarkWinningCostAttempt(ctx context.Context, handle *types.CostAttemptHandle) error
```

Use this internal retry signal; its message is never returned directly to relay clients:

```go
type CostCoverageError struct { ChannelID int }
func (e *CostCoverageError) Error() string { return "channel cost coverage unavailable" }
```

`PrepareCostAttempt` must perform an authoritative active-rule read in the main DB, validate final identity/capabilities, create or reuse the request ledger by stable `request_id`, allocate `attempt_no` while locking the request row, snapshot the full normalized rule, and commit `prepared`. `AuthorizeCostDispatch` must commit the `prepared -> dispatching` CAS before returning.

The sync adaptor wrapper must use a bounded server context for post-send state changes:

```go
func costPersistenceContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 10*time.Second)
}
```

Move the existing `maxTokensLimit = math.MaxInt32 / 2` value to `relaycommon.MaxTokensLimit` and make request validators consume that exported constant. Validate every token meter as non-negative and no larger than `relaycommon.MaxTokensLimit`, and every duration as non-negative and no larger than `relaycommon.MaxTaskDurationSeconds`; pointer zero remains a present value. Do not convert arbitrary HTTP errors to zero cost. Only the explicit contract classifier may return `confirmed_zero`; ambiguous post-dispatch failures become `cost_unknown`. Settlement errors never replace a successful client response.

- [ ] **Step 4: Run lifecycle tests and verify GREEN**

Run:

```text
go test ./service ./relay -run 'CostAttempt|AuthorizeDispatch|CostOutcome|ClientCancel' -count=1
```

Expected: state order, send authorization, retries, and failure classification all pass.

- [ ] **Step 5: Commit attempt lifecycle**

```text
git add service/cost_accounting.go service/cost_accounting_test.go relay/common/relay_utils.go relay/helper/valid_request.go relay/cost_accounting_adaptor.go relay/cost_accounting_adaptor_test.go
git commit -m "feat: track upstream cost attempts"
```

### Task 8: Recognize Billed Revenue, Winner Attribution, And Admin Log Linkage

**Files:**

- Modify: `service/billing.go`
- Modify: `service/billing_session_test.go`
- Modify: `service/log_info_generate.go`
- Modify: `service/quota.go`
- Modify: `service/text_quota.go`
- Create: `service/cost_revenue_test.go`
- Modify: `model/log_format_test.go`

- [ ] **Step 1: Write failing revenue and privacy tests**

Test wallet and subscription settlement with the same equivalent formula, funding-source snapshots, full refund to confirmed zero, user-billing persistence failure to `revenue_failed`, winning-attempt assignment, zero revenue with null margin, negative profit, and request completion only after every attempt reaches a terminal confirmed state. Assert non-admin log formatting strips the entire cost reference.

- [ ] **Step 2: Run revenue/log tests and verify RED**

Run:

```text
go test ./service ./model -run 'CostRevenue|BillingSession|CostAccountingAdminInfo|FormatUserLogs' -count=1
```

Expected: revenue-ledger and log-reference assertions fail because settlement is not connected to accounting.

- [ ] **Step 3: Connect user settlement without changing billing behavior**

After the existing wallet/subscription settlement succeeds, call the non-fatal accounting hook:

```go
func RecognizeBilledRevenue(ctx context.Context, info *relaycommon.RelayInfo, finalQuota int) error
func MarkCostRevenueFailed(ctx context.Context, info *relaycommon.RelayInfo, failureCode string) error
```

Snapshot `common.QuotaPerUnit` as a canonical Decimal string at request-ledger creation and use that immutable value during final recognition. On existing billing failure, mark `revenue_failed` and return the original billing error. On accounting persistence failure after successful user billing, log a request-correlated warning and preserve the existing successful client result.

Add one shared log helper and call it beside `attachQuotaSaturation` on text, audio, websocket, image/per-call, and task logs:

```go
func attachCostAccountingAdminInfo(info *relaycommon.RelayInfo, other map[string]interface{}) {
	if info == nil || info.CostRequestID == 0 { return }
	adminInfo, _ := other["admin_info"].(map[string]interface{})
	if adminInfo == nil { adminInfo = map[string]interface{}{} }
	adminInfo["cost_accounting_request_id"] = info.CostRequestID
	other["admin_info"] = adminInfo
}
```

Do not include cost amount, profit, rule ID, meter, or attempt status in the consume log JSON.

- [ ] **Step 4: Run revenue/log tests and verify GREEN**

Run:

```text
go test ./service ./model -run 'CostRevenue|BillingSession|CostAccountingAdminInfo|FormatUserLogs' -count=1
```

Expected: wallet/subscription accounting and admin-only linkage pass without changing quota settlement assertions.

- [ ] **Step 5: Commit revenue recognition**

```text
git add service/billing.go service/billing_session_test.go service/log_info_generate.go service/quota.go service/text_quota.go service/cost_revenue_test.go model/log_format_test.go
git commit -m "feat: recognize billed revenue equivalent"
```

### Task 9: Integrate Async Submit, Task Persistence, And Polling Settlement

**Files:**

- Modify: `relay/relay_task.go`
- Modify: `relay/relay_task_billing_test.go`
- Modify: `controller/relay.go`
- Create: `controller/cost_task_relay_test.go`
- Modify: `model/task.go`
- Modify: `service/task_polling.go`
- Modify: `service/task_polling_test.go`
- Modify: `service/task_billing.go`
- Modify: `service/task_billing_test.go`

- [ ] **Step 1: Write failing async lifecycle tests**

Cover public task ID creation before send, cost request/attempt persistence before submit, `submit_accepted`, `task_succeeded`, failed-task zero cost, authoritative duration/token meter, duplicate poll idempotency, cost request ID in `Task.PrivateData`, rule retirement after submit not changing settlement, and upstream acceptance followed by local task insert failure producing an orphan anomaly.

```go
func TestTaskPrivateDataCarriesCostRequestID(t *testing.T) {
	data := model.TaskPrivateData{CostRequestID: 42}
	raw, err := common.Marshal(data)
	require.NoError(t, err)
	var decoded model.TaskPrivateData
	require.NoError(t, common.Unmarshal(raw, &decoded))
	assert.Equal(t, int64(42), decoded.CostRequestID)
}
```

- [ ] **Step 2: Run async tests and verify RED**

Run:

```text
go test ./relay ./controller ./model ./service -run 'CostTask|TaskPrivateData|TaskPollingCost' -count=1
```

Expected: compilation or assertions fail because async cost linkage is absent.

- [ ] **Step 3: Wire async submit and polling to the frozen snapshot**

After `BuildRequestBody` and before `DoRequest`, call the task contract to confirm `BillableUpstreamModel`, then `PrepareCostAttempt` and `AuthorizeCostDispatch`. Preserve the public `task_id` in the request ledger even though the `Task` row does not exist yet. On accepted submit, settle `submit_accepted` rules or transition to `awaiting_meter`; mark the successful submitted attempt as winner.

Add:

```go
type TaskPrivateData struct {
	// existing fields
	CostRequestID int64 `json:"cost_request_id,omitempty"`
}

func SettleAsyncCostAttempt(ctx context.Context, costRequestID int64, task *model.Task, result *relaycommon.TaskInfo) error
func MarkOrphanedCostTask(ctx context.Context, costRequestID int64, failureCode string) error
```

Populate `TaskInfo.CostMeter` only from provider-authoritative results or explicitly accepted media metadata. Reuse `relaycommon.MaxTaskDurationSeconds` and the existing max-token semantic; preserve explicit zero with pointer fields. Poll settlement must load `rule_snapshot_json` from the attempt, never the current rule. The same conditional transition handles repeated polling/callbacks without double cost.

If `task.Insert()` fails after provider acceptance, call `MarkOrphanedCostTask`, retain request/task/upstream identifiers, and emit a warning; do not delete or zero the attempt.

- [ ] **Step 4: Run async tests and verify GREEN**

Run:

```text
go test ./relay ./controller ./model ./service -run 'CostTask|TaskPrivateData|TaskPollingCost|TaskBilling' -count=1
```

Expected: all async charge-event, meter, idempotency, snapshot, and orphan tests pass.

- [ ] **Step 5: Commit async integration**

```text
git add relay/relay_task.go relay/relay_task_billing_test.go controller/relay.go controller/cost_task_relay_test.go model/task.go service/task_polling.go service/task_polling_test.go service/task_billing.go service/task_billing_test.go
git commit -m "feat: account for asynchronous task costs"
```

### Task 10: Add Recovery Scanning And Append-Only Manual Reconciliation

**Files:**

- Create: `service/cost_recovery.go`
- Create: `service/cost_recovery_test.go`
- Modify: `model/system_task.go`
- Modify: `controller/system_task_handlers.go`
- Modify: `controller/system_task_handlers_test.go`
- Modify: `controller/cost_accounting.go`
- Modify: `controller/cost_accounting_test.go`

- [ ] **Step 1: Write failing stale-state and reconciliation tests**

Assert stale `prepared -> not_dispatched`, stale `dispatching -> cost_unknown`, complete-meter `awaiting_meter -> settled`, unknown/failed entries stay queued, and repeated scans are idempotent. Reconciliation must reject settled attempts, require a non-blank reason, use the original rule snapshot, atomically write amount/request summary/audit, and leave one audit row per successful manual action.

- [ ] **Step 2: Run recovery tests and verify RED**

Run:

```text
go test ./service ./controller -run 'CostRecovery|CostReconcile|CostAccountingSystemTask' -count=1
```

Expected: compilation fails because recovery and reconciliation services are missing.

- [ ] **Step 3: Implement a leased recovery job and audited repairs**

Add `SystemTaskTypeCostAccountingRecovery = "cost_accounting_recovery"`, register a handler running every minute only when cost accounting is strict or recoverable rows exist, and process bounded batches ordered by oldest timestamp:

```go
type CostRecoverySummary struct {
	PreparedClosed int `json:"prepared_closed"`
	DispatchingUnknown int `json:"dispatching_unknown"`
	AwaitingSettled int `json:"awaiting_settled"`
}

func RecoverStaleCostAccounting(ctx context.Context, now time.Time, limit int) (CostRecoverySummary, error)
func ReconcileCostAttempt(ctx context.Context, attemptID int64, adminID int, action string, meter *types.CostMeter, reason string) error
func ReconcileCostRevenue(ctx context.Context, requestID int64, adminID int, finalQuota int64, reason string) error
```

Use `prepared` age for provable non-dispatch, `dispatching` age for conservative unknown, and the attempt snapshot for manual amount calculation. Audit insert, attempt/request updates, and recomputation must share one main-DB transaction. Do not expose update/delete routes for audits or confirmed settled amounts.

- [ ] **Step 4: Run recovery tests and verify GREEN**

Run:

```text
go test ./service ./controller ./model -run 'CostRecovery|CostReconcile|CostAccountingSystemTask|CostAudit' -count=1
```

Expected: recovery and reconciliation state machines pass and duplicate executions do not change confirmed totals.

- [ ] **Step 5: Commit recovery and reconciliation**

```text
git add service/cost_recovery.go service/cost_recovery_test.go model/system_task.go controller/system_task_handlers.go controller/system_task_handlers_test.go controller/cost_accounting.go controller/cost_accounting_test.go
git commit -m "feat: recover and reconcile cost accounting"
```

### Task 11: Implement Request Detail, Coverage, Anomaly, And Profit Queries

**Files:**

- Create: `service/cost_report.go`
- Create: `service/cost_report_test.go`
- Modify: `controller/cost_accounting.go`
- Modify: `controller/cost_accounting_test.go`

- [ ] **Step 1: Write failing attribution and aggregation tests**

Seed a request with one failed retry on channel A and one winning attempt on channel B. Assert request revenue appears once on B, both costs remain on their own channels, A has negative contribution, and summing channel rows equals the request ledger. Assert default filtering uses `profit_recognized_at`, only `complete` requests enter amounts, aggregate margin is `sum(profit)/sum(revenue)`, zero total revenue has null margin, and checked addition returns a stable overflow error.

- [ ] **Step 2: Run report tests and verify RED**

Run:

```text
go test ./service ./controller -run 'CostReport|ProfitAttribution|CostAnomaly|CostCoverageResponse' -count=1
```

Expected: compilation fails because report/detail query functions do not exist.

- [ ] **Step 3: Implement checked application-layer aggregation**

Expose:

```go
func GetCostRequestDetail(id int64) (*CostRequestDetail, error)
func ListCostAnomalies(filter CostAnomalyFilter) ([]CostAnomalyRow, int64, error)
func CheckCostCoverage(filter CostCoverageFilter) ([]CostCoverageRow, error)
func SummarizeCostProfit(filter CostReportFilter) (CostProfitSummary, error)
func BreakDownCostProfit(filter CostReportFilter) ([]CostProfitBreakdownRow, error)
```

Use GORM for filtering and joins, then scan rows and aggregate with `CheckedNanoAdd`/`CheckedNanoSubtract` in Go so database `SUM` cannot overflow silently. The request summary reads only `complete` rows. The breakdown scans attempts joined to requests: each attempt contributes its own cost; only `attempt.id == request.winning_attempt_id` contributes request revenue. Counts for pending/unknown/failed states remain visible outside realized amount totals, together with a separately named `known_incomplete_cost_nano_usd` subtotal that is never mixed into realized revenue, cost, profit, or margin.

Support filters for profit-recognized time, requested time, channel, billable upstream model, origin model, user/using group, billing source, and status. Return stable `cost_report_overflow` and log a warning on arithmetic overflow.

- [ ] **Step 4: Run report tests and verify GREEN**

Run:

```text
go test ./service ./controller -run 'CostReport|ProfitAttribution|CostAnomaly|CostCoverageResponse' -count=1
```

Expected: request, channel, and model totals reconcile exactly and overflow never wraps.

- [ ] **Step 5: Commit reporting services**

```text
git add service/cost_report.go service/cost_report_test.go controller/cost_accounting.go controller/cost_accounting_test.go
git commit -m "feat: add billed gross profit reports"
```

### Task 12: Add Cross-Database And End-To-End Backend Regression Coverage

**Files:**

- Create: `model/cost_accounting_migration_test.go`
- Create: `e2e/cost_accounting_e2e_test.go`

- [ ] **Step 1: Write the cross-database migration test**

Follow `model/user_session_migration_test.go`: always run SQLite, and run MySQL/PostgreSQL when their DSNs are present. For each dialect, migrate the four tables and assert nullable task uniqueness, composite attempt uniqueness, rule-version uniqueness, activation transaction behavior, and CAS settlement behavior.

```go
tests := []struct {
	name string
	env string
	dialector func(string) gorm.Dialector
}{
	{name: "mysql", env: "TEST_MYSQL_DSN", dialector: func(dsn string) gorm.Dialector { return mysql.Open(dsn) }},
	{name: "postgres", env: "TEST_POSTGRES_DSN", dialector: func(dsn string) gorm.Dialector { return postgres.Open(dsn) }},
}
```

- [ ] **Step 2: Write end-to-end sync and async accounting tests**

Create deterministic fake providers that exercise: two-attempt sync retry with one revenue; stream client cancellation with persisted cost state; async `submit_accepted`; async `task_succeeded`; orphan task insertion; strict all-uncovered rejection; and non-admin log privacy. Assert exact request, attempt, and report values rather than implementation-specific query counts.

- [ ] **Step 3: Run focused backend integration tests**

Start disposable MySQL and PostgreSQL containers, wait for both readiness probes, and set the test DSNs in PowerShell:

```powershell
docker run --rm -d --name new-api-cost-mysql -e MYSQL_ROOT_PASSWORD=costtest -e MYSQL_DATABASE=newapi_cost_test -p 33306:3306 mysql:8.0
docker run --rm -d --name new-api-cost-postgres -e POSTGRES_PASSWORD=costtest -e POSTGRES_DB=newapi_cost_test -p 35432:5432 postgres:16
for ($i = 0; $i -lt 30; $i++) { docker exec new-api-cost-mysql mysqladmin ping -h 127.0.0.1 -pcosttest; if ($LASTEXITCODE -eq 0) { break }; Start-Sleep -Seconds 1 }
for ($i = 0; $i -lt 30; $i++) { docker exec new-api-cost-postgres pg_isready -U postgres -d newapi_cost_test; if ($LASTEXITCODE -eq 0) { break }; Start-Sleep -Seconds 1 }
$env:TEST_MYSQL_DSN='root:costtest@tcp(127.0.0.1:33306)/newapi_cost_test?charset=utf8mb4&parseTime=True&loc=Local'
$env:TEST_POSTGRES_DSN='host=127.0.0.1 user=postgres password=costtest dbname=newapi_cost_test port=35432 sslmode=disable TimeZone=UTC'
```

Run:

```text
go test ./model -run CostAccountingMigration -count=1 -v
go test ./e2e -run CostAccounting -count=1 -v
go test ./types ./setting/cost_setting ./model ./service ./controller ./router ./relay/... ./e2e -run 'Cost|Profit' -count=1
```

Expected: SQLite, MySQL, PostgreSQL, and all focused E2E cases pass; neither external-dialect subtest is skipped.

- [ ] **Step 4: Run the full backend suite**

Run:

```text
go test ./... -count=1
docker stop new-api-cost-mysql new-api-cost-postgres
```

Expected: every Go package passes and both disposable database containers stop successfully.

- [ ] **Step 5: Commit backend integration coverage**

```text
git add model/cost_accounting_migration_test.go e2e/cost_accounting_e2e_test.go
git commit -m "test: cover cost accounting across databases"
```

### Task 13: Add Frontend API Types, Exact Money Formatting, And Rule Validation

**Files:**

- Create: `web/src/features/cost-accounting/types.ts`
- Create: `web/src/features/cost-accounting/api.ts`
- Create: `web/src/features/cost-accounting/lib/cost-rule.ts`
- Create: `web/src/features/cost-accounting/lib/cost-rule.test.ts`
- Modify: `web/src/lib/admin-permissions.ts`

- [ ] **Step 1: Write failing frontend domain tests**

Use `node:test` and cover conditional rule fields, canonical positive Decimal strings, explicit free reason, extra-field rejection, token mode requirements, nano-USD `BigInt` formatting beyond `Number.MAX_SAFE_INTEGER`, negative profit, and null zero-revenue margin.

```ts
test('formats nano USD without Number conversion', () => {
  assert.equal(formatNanoUSD('9223372036854775807'), '$9,223,372,036.854775807')
  assert.equal(formatNanoUSD('-250000000'), '-$0.25')
})
```

- [ ] **Step 2: Run the frontend test and verify RED**

Run from `web/`:

```text
bun test src/features/cost-accounting/lib/cost-rule.test.ts
```

Expected: the test fails because the new feature modules do not exist.

- [ ] **Step 3: Implement exact API mirrors and pure helpers**

Define all nano-USD fields as `string` and use `BigInt` for formatting and addition. Export discriminated rule form types and helpers:

```ts
export type CostMode = 'free' | 'per_request' | 'per_duration' | 'per_token'
export type CostAttemptStatus =
  | 'prepared' | 'dispatching' | 'not_dispatched' | 'awaiting_meter'
  | 'settled' | 'confirmed_zero' | 'cost_unknown' | 'settlement_failed'

export function formatNanoUSD(value: string): string
export function formatMarginPPM(value: string | null): string
export function parseCostRuleForm(values: CostRuleFormValues): CostRuleConfigV1
```

Create typed API functions for every route from Task 4 and TanStack query-key factories. Add `COST_ACCOUNTING: 'cost_accounting'` plus `RECONCILE: 'reconcile'` to administrator permission constants.

- [ ] **Step 4: Run tests, typecheck, and lint**

Run:

```text
bun test src/features/cost-accounting/lib/cost-rule.test.ts
bun run typecheck
bun run lint
```

Expected: exact-money and validation tests pass; TypeScript and oxlint exit successfully.

- [ ] **Step 5: Commit frontend domain code**

```text
git add web/src/features/cost-accounting/types.ts web/src/features/cost-accounting/api.ts web/src/features/cost-accounting/lib/cost-rule.ts web/src/features/cost-accounting/lib/cost-rule.test.ts web/src/lib/admin-permissions.ts
git commit -m "feat(web): add cost accounting client domain"
```

### Task 14: Build The Channel Model Cost Drawer And Rule Editor

**Files:**

- Create: `web/src/features/cost-accounting/components/channel-cost-drawer.tsx`
- Create: `web/src/features/cost-accounting/components/cost-rule-drawer.tsx`
- Create: `web/src/features/cost-accounting/components/coverage-panel.tsx`
- Create: `web/src/features/cost-accounting/components/channel-cost-drawer.test.tsx`
- Modify: `web/src/features/channels/components/channels-provider.tsx`
- Modify: `web/src/features/channels/components/channels-dialogs.tsx`
- Modify: `web/src/features/channels/components/data-table-row-actions.tsx`

- [ ] **Step 1: Write failing drawer render and interaction tests**

Render with a test QueryClient and i18next instance. Assert the channel menu shows “Model Costs” only with read permission; the drawer lists final upstream model, client mappings, official price, active version, normalized USD price, capability status, and history; changing mode changes visible fields; save creates a draft; activate requires confirmation; preview is visibly marked estimated; loading/error/empty states have stable content and retry actions.

- [ ] **Step 2: Run the drawer test and verify RED**

Run from `web/`:

```text
bun test src/features/cost-accounting/components/channel-cost-drawer.test.tsx
```

Expected: imports fail because the cost drawer components do not exist.

- [ ] **Step 3: Implement focused drawers and channel entry point**

Add `'model-costs'` to the channel dialog union. The row action uses the existing `DollarSign` icon, sets the channel as `currentRow`, and opens the cost drawer. Mount `ChannelCostDrawer` from `ChannelsDialogs`; do not add cost fields to the large channel mutation drawer.

The cost drawer uses a dense table for models and versions, a coverage status column, and icon actions with tooltips. The rule editor uses a segmented mode control, currency and canonical Decimal inputs, event/source selects, and mode-specific fields. Draft save, edit, activation, retirement, and mode controls require `cost_accounting.write`; read-only administrators still see rules, history, coverage, and preview. Submit operations invalidate rule, coverage, and channel cost queries. Preview submits a sample meter and selected user group to `/preview`, displays billed revenue equivalent, supplier cost, billed gross profit, and margin, and labels every result “Estimated”.

Use drawers at `sm:max-w-2xl`, no nested cards, no color-only statuses, and stable scrollable table dimensions.

- [ ] **Step 4: Run drawer tests and frontend checks**

Run:

```text
bun test src/features/cost-accounting/components/channel-cost-drawer.test.tsx
bun run typecheck
bun run lint
```

Expected: form modes, permissions, mutations, preview, and states pass with no type or lint errors.

- [ ] **Step 5: Commit channel cost management UI**

```text
git add web/src/features/cost-accounting/components/channel-cost-drawer.tsx web/src/features/cost-accounting/components/cost-rule-drawer.tsx web/src/features/cost-accounting/components/coverage-panel.tsx web/src/features/cost-accounting/components/channel-cost-drawer.test.tsx web/src/features/channels/components/channels-provider.tsx web/src/features/channels/components/channels-dialogs.tsx web/src/features/channels/components/data-table-row-actions.tsx
git commit -m "feat(web): add channel model cost rules"
```

### Task 15: Add Usage-Log Cost Detail And The Anomaly Queue

**Files:**

- Create: `web/src/features/cost-accounting/components/cost-request-detail.tsx`
- Create: `web/src/features/cost-accounting/components/anomaly-queue.tsx`
- Create: `web/src/features/cost-accounting/components/reconcile-drawer.tsx`
- Create: `web/src/features/cost-accounting/components/cost-request-detail.test.tsx`
- Modify: `web/src/features/usage-logs/types.ts`
- Modify: `web/src/features/usage-logs/api.ts`
- Modify: `web/src/features/usage-logs/components/dialogs/details-dialog.tsx`

- [ ] **Step 1: Write failing detail and reconciliation tests**

Assert an administrator log with `cost_accounting_request_id` fetches request detail only while the dialog is open; ordinary users and legacy logs never fetch it. Assert the timeline shows every attempt, channel/model, winner, rule version, event/source, nullable meter, original/normalized amounts, status, failure code, and audits. Assert reconciliation is available only for unknown/failed states, requires reason, and preserves explicit zero meter values.

- [ ] **Step 2: Run detail tests and verify RED**

Run from `web/`:

```text
bun test src/features/cost-accounting/components/cost-request-detail.test.tsx
```

Expected: component and admin reference types are missing.

- [ ] **Step 3: Implement lazy admin detail and anomaly workflows**

Add only this field to `LogOtherData.admin_info`:

```ts
cost_accounting_request_id?: number
```

Mount `CostRequestDetail` inside the existing details dialog when `isAdmin`, open, and the ID exists. Use a vertical attempt timeline with status badges and a compact definition grid; show “Historical cost unavailable” for old logs without a reference, never `$0`.

Build the anomaly queue as a filterable table for `cost_unknown`, `settlement_failed`, `revenue_failed`, and orphan async requests. Show reconciliation commands only with `cost_accounting.reconcile`; other readers retain detail access. The reconcile drawer sends `action`, optional pointer-preserving meter fields, and required reason. Successful reconciliation invalidates request detail, anomaly, and report queries.

- [ ] **Step 4: Run detail tests and checks**

Run:

```text
bun test src/features/cost-accounting/components/cost-request-detail.test.tsx
bun run typecheck
bun run lint
```

Expected: lazy fetching, privacy gating, timeline, anomaly states, and reconciliation pass.

- [ ] **Step 5: Commit detail and anomaly UI**

```text
git add web/src/features/cost-accounting/components/cost-request-detail.tsx web/src/features/cost-accounting/components/anomaly-queue.tsx web/src/features/cost-accounting/components/reconcile-drawer.tsx web/src/features/cost-accounting/components/cost-request-detail.test.tsx web/src/features/usage-logs/types.ts web/src/features/usage-logs/api.ts web/src/features/usage-logs/components/dialogs/details-dialog.tsx
git commit -m "feat(web): add cost detail and reconciliation"
```

### Task 16: Build The Billed Gross-Profit Report Route And Navigation

**Files:**

- Create: `web/src/features/cost-accounting/index.tsx`
- Create: `web/src/features/cost-accounting/components/profit-summary.tsx`
- Create: `web/src/features/cost-accounting/components/profit-table.tsx`
- Create: `web/src/features/cost-accounting/components/profit-filters.tsx`
- Create: `web/src/features/cost-accounting/components/profit-report.test.tsx`
- Create: `web/src/routes/_authenticated/cost-accounting/index.tsx`
- Modify: `web/src/routeTree.gen.ts` (generated by the TanStack Router plugin)
- Modify: `web/src/hooks/use-sidebar-data.ts`
- Modify: `web/src/hooks/use-sidebar-config.ts`
- Modify: `web/src/features/system-settings/maintenance/config.ts`
- Modify: `web/src/components/layout/types.ts`
- Modify: `web/src/hooks/use-sidebar-view.ts`

- [ ] **Step 1: Write failing route, permission, and report tests**

Assert the route redirects without `cost_accounting.read`; the sidebar item follows both permission and `SidebarModulesAdmin.admin.cost_accounting`; filters map to URL search and API parameters; summary displays exact revenue/cost/profit/margin and anomaly counts; breakdown rows show channel/model attribution; loading/error/empty states do not resize the table; zero revenue renders an em dash for margin.

- [ ] **Step 2: Run report tests and verify RED**

Run from `web/`:

```text
bun test src/features/cost-accounting/components/profit-report.test.tsx
```

Expected: route and report components do not exist.

- [ ] **Step 3: Implement the work-focused report page**

Create `/cost-accounting` with a compact page header, mode/coverage status, tabs for “Profit Report” and “Anomalies”, a horizontal filter bar, four summary metrics, operational counts, and a TanStack table grouped by channel/final upstream model. Use `requested_at` or `profit_recognized_at` as an explicit segmented time basis, defaulting to profit recognition.

Add `requiredPermission?: { resource: string; action: string }` to navigation item types and filter it in `useSidebarView` with `hasPermission`. Add the sidebar item with `ChartNoAxesCombined`, route mapping `{section: 'admin', module: 'cost_accounting'}`, and default module flag `true` in both runtime and system-settings defaults.

The route guard must use:

```ts
if (!hasPermission(auth.user, ADMIN_PERMISSION_RESOURCES.COST_ACCOUNTING, ADMIN_PERMISSION_ACTIONS.READ)) {
  throw redirect({ to: '/403' })
}
```

Do not call the values cash revenue or net profit; UI labels must say “Billed Revenue Equivalent” and “Billed Gross Profit”.

- [ ] **Step 4: Run report tests and frontend checks**

Run:

```text
bun run build
bun test src/features/cost-accounting/components/profit-report.test.tsx
bun run typecheck
bun run lint
```

Expected: the router plugin adds `/cost-accounting` to `routeTree.gen.ts`; the production build and permission, filter, exact-money, summary, attribution, state, type, and lint checks pass.

- [ ] **Step 5: Commit report and navigation**

```text
git add web/src/features/cost-accounting/index.tsx web/src/features/cost-accounting/components/profit-summary.tsx web/src/features/cost-accounting/components/profit-table.tsx web/src/features/cost-accounting/components/profit-filters.tsx web/src/features/cost-accounting/components/profit-report.test.tsx web/src/routes/_authenticated/cost-accounting/index.tsx web/src/routeTree.gen.ts web/src/hooks/use-sidebar-data.ts web/src/hooks/use-sidebar-config.ts web/src/features/system-settings/maintenance/config.ts web/src/components/layout/types.ts web/src/hooks/use-sidebar-view.ts
git commit -m "feat(web): add billed gross profit report"
```

### Task 17: Complete I18n, Full Verification, And Browser Acceptance

**Files:**

- Modify: `web/src/i18n/locales/en.json`
- Modify: `web/src/i18n/locales/zh.json`
- Modify: `web/src/i18n/locales/zh-TW.json`
- Modify: `web/src/i18n/locales/fr.json`
- Modify: `web/src/i18n/locales/ja.json`
- Modify: `web/src/i18n/locales/ru.json`
- Modify: `web/src/i18n/locales/vi.json`
- Artifact: `output/playwright/cost-accounting-channel-desktop.png`
- Artifact: `output/playwright/cost-accounting-profit-desktop.png`
- Artifact: `output/playwright/cost-accounting-profit-mobile.png`

- [ ] **Step 1: Synchronize and translate every new user-facing string**

Run from `web/`:

```text
bun run i18n:sync
```

Translate every new key in all seven locale files. Scan for untranslated English fallbacks and ensure terminology consistently distinguishes billed revenue equivalent, supplier cost, billed gross profit, and anomalies.

- [ ] **Step 2: Run all backend and frontend verification**

Run from the repository root:

```text
go test ./... -count=1
git diff --check
```

Run from `web/`:

```text
bun test src/features/cost-accounting
bun run typecheck
bun run lint
bun run format:check
bun run build
```

Expected: all Go tests, focused frontend tests, typecheck, lint, formatting, production build, and whitespace checks pass.

- [ ] **Step 3: Start the application and verify administrator workflows**

Start the existing local stack or the frontend dev server on a free port. For a frontend-only session, run from `web/`:

```text
bun run dev -- --port 3010
```

Open the served URL as an administrator and verify: create/validate/activate a rule; coverage changes from missing to covered; strict mode cannot enable with uncovered rows; a channel model cost drawer survives refresh; a usage log opens the matching attempt timeline; an anomaly reconciliation requires a reason; the profit report filters and totals update.

- [ ] **Step 4: Verify responsive layout and privacy boundaries**

Capture screenshots at `1440x900` and `390x844`. Confirm no overlapping text, clipped controls, nested cards, horizontal page overflow, layout shift between loading and data, or inaccessible icon buttons. Sign in as a non-admin and confirm `/cost-accounting` redirects to `/403`, public/user APIs contain no cost fields, and usage logs contain no `admin_info` cost reference.

- [ ] **Step 5: Commit i18n and acceptance-ready UI**

```text
git add web/src/i18n/locales/en.json web/src/i18n/locales/zh.json web/src/i18n/locales/zh-TW.json web/src/i18n/locales/fr.json web/src/i18n/locales/ja.json web/src/i18n/locales/ru.json web/src/i18n/locales/vi.json
git commit -m "chore: finalize cost accounting translations"
```

## Completion Criteria

- Every real upstream send has one immutable attempt row; retries never overwrite prior cost facts.
- No upstream send occurs in strict mode before final identity, authoritative rule snapshot, `prepared`, and committed `dispatching` authorization.
- Ambiguous post-dispatch outcomes are unknown, never implicit zero; only explicit free or provider-proven no-charge outcomes are zero.
- Wallet and subscription revenue use the same billed-equivalent formula and retain funding-source snapshots.
- Request totals equal channel/model breakdown totals; revenue appears only on the winning attempt and every attempt keeps its own cost.
- Manual repairs use frozen snapshots and append an audit in the same transaction as the repaired state and request recomputation.
- SQLite, MySQL, and PostgreSQL share the same migration, uniqueness, transaction, and CAS behavior.
- Administrator APIs and UI expose exact Decimal/string amounts; normal users and public surfaces expose no supplier cost or profit data.
- The feature remains `disabled` after migration and can enter `strict` only after an authoritative coverage check succeeds.
