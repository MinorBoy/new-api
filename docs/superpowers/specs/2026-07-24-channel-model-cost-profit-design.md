# Channel Model Cost and Gross Profit Design

## 1. Purpose

Add channel-level upstream cost accounting and administrator-only gross-profit
reporting without changing the existing user billing contract. The user sale
price remains global and keyed by the client-facing model. The effective user
charge remains the official sale price multiplied by the effective user-group
ratio. Upstream cost is resolved independently from the actual selected
channel and mapped upstream model.

Phase 1 provides manual, versioned cost configuration for three structured
cost modes: per request, per duration, and per token. Phase 2 imports local
Excel or CSV price sheets, such as the Lucen table, into reviewable sale-price
and channel-cost proposals. A future expression cost mode is documented as an
extension point but cannot be configured or activated in Phase 1.

## 2. Confirmed Business Decisions

1. User sale pricing and upstream cost accounting are independent ledgers.
2. The official model sale price remains keyed by `OriginModelName` and is not
   changed by channel selection or model mapping.
3. The effective user-group ratio is the customer discount or markup.
4. Realized gross profit is:

   ```text
   gross_profit_usd = final_user_charge_usd - actual_upstream_cost_usd
   ```

5. Profit may be negative. Revenue and cost may never be negative.
6. Upstream cost is keyed by the actual `channel_id + upstream_model`, not by
   the client model.
7. Cost configuration retains the original currency and every conversion
   input. A request snapshots both the original rule and its normalized USD
   values when the channel is selected.
8. Later rule, exchange-rate, model-mapping, or sale-price changes never
   recalculate historical records automatically.
9. Missing or invalid cost configuration uses strict routing: the channel is
   ineligible. If every candidate is ineligible, the request is rejected.
   The system never assumes that cost equals sale price.
10. If an accepted request later lacks a declared authoritative duration or
    token measurement, client delivery is not changed. Cost settlement becomes
    `settlement_failed`, raises an administrator warning, and is excluded from
    profit totals until reconciled.
11. Failed requests have zero recognized provider cost by default. A rule may
    instead declare that an accepted submission is billable.
12. Phase 1 includes the cost configuration view, administrator request-log
    details, and aggregate profit reporting.

## 3. Existing Contracts That Remain Unchanged

The existing user billing paths continue to own pre-consume, settlement,
refund, subscription, token quota, user used-quota, and channel used-quota
behavior. `Channel.UsedQuota` remains user-billing quota and must not be
reinterpreted as provider cost.

The existing sale-pricing modes remain global:

- token pricing through `ModelRatio` and related ratios;
- fixed request pricing through `ModelPrice`;
- duration pricing through `per_duration` rules;
- expression pricing through `billing_expr`.

Cost accounting observes the final user charge produced by those paths. It
does not insert a second user deduction and does not derive user price from
provider cost.

## 4. Cost Rule Identity and Versioning

Add a `channel_model_cost_rules` table in the main database. A business rule
is identified by:

```text
channel_id + upstream_model + version
```

The mapped upstream model is resolved before cost lookup. A client alias can
therefore keep one official sale price while different channels have different
cost rules for their actual provider models.

Each rule has these common fields:

| Field | Meaning |
|---|---|
| `id` | Database-generated rule ID |
| `channel_id` | Actual provider channel |
| `upstream_model` | Model sent to that provider |
| `version` | Monotonically increasing version within the business key |
| `status` | `draft`, `active`, `inactive`, or `invalid` |
| `cost_mode` | `per_request`, `per_duration`, or `per_token` |
| `effective_from` | Inclusive activation timestamp |
| `effective_to` | Exclusive end timestamp; null for the current version |
| `source` | `manual` in Phase 1; `import` is added in Phase 2 |
| `source_batch_id` | Optional Phase 2 import batch reference |
| `note` | Administrator note, including future expression guidance |
| `created_by` | Administrator ID |
| timestamps | Creation and update audit timestamps |

Editing an active rule is forbidden. An edit creates a draft successor. Rule
activation locks the business key with the shared `lockForUpdate` helper,
ends the previous version, activates the new version, and invalidates the
rule cache. This avoids database-specific partial indexes and works on SQLite,
MySQL, and PostgreSQL.

Exactly one version may be effective for a business key at a given instant.
Overlapping effective windows are rejected transactionally.

## 5. Original Currency and Normalized Cost

Each rule stores these original pricing and conversion inputs as validated
canonical decimal strings rather than binary floating-point numbers:

| Field | Constraint |
|---|---|
| `currency` | ISO-style uppercase currency code, for example `CNY` or `USD` |
| `billing_multiplier` | Positive; defaults to `1` |
| `purchase_discount_ratio` | Positive; defaults to `1` |
| `recharge_exchange_ratio` | Positive credited units per paid unit; defaults to `1` |
| `fee_rate` | Non-negative fractional rate; defaults to `0` |
| `currency_to_usd_rate` | Positive USD value of one original-currency unit |

The normalized USD unit price is frozen using:

```text
normalized_usd_unit_price =
    original_currency_unit_price
    * billing_multiplier
    * purchase_discount_ratio
    / recharge_exchange_ratio
    * (1 + fee_rate)
    * currency_to_usd_rate
```

The direction of `currency_to_usd_rate` is explicit. For example, a CNY rule
stores the USD value of CNY 1 rather than an ambiguous `7.3` exchange rate.

Lucen columns such as the applicable no-V or with-V discount are configuration
inputs. In Phase 1 the administrator selects the discount that applies to the
channel account. It does not switch dynamically during a request.

All calculation uses `shopspring/decimal`. Persisted aggregate amounts use
signed `int64` nano-USD, where USD 1 equals `1,000,000,000` nano-USD. Checked
conversion rejects invalid negative values and reports saturation; bare float
to integer casts are forbidden.

## 6. Structured Cost Modes

### 6.1 Per Request

`per_request` stores one original-currency price for a billable request. Its
default charge event is `task_succeeded`. A rule may select
`submit_accepted` when the provider contract charges after successful
submission even if the eventual task fails.

```text
original_cost = request_unit_price * billable_request_count
```

Phase 1 supports a billable count of one per gateway request or local task.

### 6.2 Per Duration

`per_duration` stores an original-currency price per second and declares one
measurement source:

- `upstream_actual`: authoritative duration returned by the provider or media
  metadata accepted by the adaptor;
- `validated_request`: the bounded, normalized billable duration submitted to
  the provider.

```text
original_cost = price_per_second * billable_duration_seconds
```

The duration remains bounded by `relaycommon.MaxTaskDurationSeconds`. An
adaptor must explicitly expose the declared measurement source. Unexpectedly
missing measurement produces `settlement_failed`; it does not silently switch
to the other source.

### 6.3 Per Token

`per_token` stores prices per one million tokens and declares a measurement
source:

- `upstream_usage`: authoritative usage returned by the provider;
- `local_usage`: deterministic usage produced by an adaptor or an existing
  local token counter.

It has two structured pricing variants:

1. `total`: one price for a configured `total_tokens` or
   `completion_tokens` metric;
2. `input_output`: separate input-token and output-token prices.

Calculations are:

```text
single_cost = selected_tokens / 1,000,000 * total_token_price

split_cost = input_tokens / 1,000,000 * input_token_price
           + output_tokens / 1,000,000 * output_token_price
```

For NewAPIVideo, detailed polling already preserves `completion_tokens` and
`total_tokens`; a Lucen rule can select the provider-billed metric. A provider
that supplies neither upstream nor reliable local usage cannot complete a
Phase 1 `per_token` settlement. Provider-specific derivation such as
resolution multiplied by duration and a token coefficient belongs to the
future expression mode and must be labeled as estimated.

## 7. Future Expression Mode

`expression` is reserved in documentation and validation messages but is not
an activatable Phase 1 cost mode. It is intended for provider-specific rules
whose actual cost depends on multiple request or result facts and cannot be
represented by the three structured modes.

Future implementation must follow `pkg/billingexpr/expr.md`, use a separately
versioned cost-expression contract, bound every user-controlled multiplier,
and identify estimated rather than provider-authoritative measurements. A
note on a Phase 1 rule may explain the future requirement but cannot execute
an expression.

## 8. Cost Snapshot and Accounting Record

Add a `cost_accounting_records` table in the main database. It is the source
of truth for request-level cost and profit reporting. Using the main database
allows portable conditional updates and idempotency without depending on the
configured log database or cross-database transactions.

Each record contains:

- stable `request_id` and optional local `task_id`;
- user, group, token, channel, client model, and upstream model dimensions;
- cost rule ID, version, mode, measurement source, and charge event;
- complete original-currency rule and conversion snapshot;
- requested and actual measurement values used for settlement;
- `quota_per_unit` snapshot and final user quota;
- original-currency cost as a canonical decimal string;
- `revenue_nano_usd`, `cost_nano_usd`, and
  `gross_profit_nano_usd`;
- nullable `gross_margin_ppm`, where `1,000,000` represents 100%;
- settlement status, failure code, reconciliation metadata, and timestamps.

The cost snapshot is also attached to `RelayInfo` for a synchronous request.
For an asynchronous task, its record ID and snapshot are persisted in
`Task.PrivateData` before the upstream submission. This allows polling and
reconciliation to use the selected version after rule or model-map changes.

Existing consume logs retain a cost-record reference and denormalized final
amounts for administrator display. Public and non-administrator log shaping
must remove all cost, profit, conversion, and rule fields just as it removes
other `admin_info` fields. Aggregate reports query the accounting table, not
log JSON.

## 9. Revenue, Cost, and Margin Recognition

Revenue is recognized from the final user charge after normal settlement and
refund behavior:

```text
revenue_nano_usd =
    final_user_quota / quota_per_unit_snapshot * 1,000,000,000
```

This preserves the actual group-discounted charge rather than recalculating
revenue from the current sale configuration. The quota conversion uses
decimal arithmetic and checked rounding.

Cost is finalized from the snapshotted normalized unit prices and the selected
authoritative measurements. Profit and margin are:

```text
gross_profit_nano_usd = revenue_nano_usd - cost_nano_usd
gross_margin = gross_profit_nano_usd / revenue_nano_usd
```

Margin is null when revenue is zero. A negative profit and margin are valid.
For non-zero revenue, `gross_margin_ppm` is calculated with checked decimal
rounding from `gross_profit / revenue * 1,000,000`. The API converts that
integer to percentage display values; it must not treat a negative result as
a quota credit.

## 10. Settlement State Machine and Idempotency

Cost records use this state machine:

```text
pending -> settled
pending -> zero_cost
pending -> settlement_failed
settlement_failed -> settled
```

- `settled` means recognized revenue and provider cost are complete.
- `zero_cost` means the terminal outcome is contractually not billed by the
  provider. Its cost is zero and it may participate in aggregate totals.
- `settlement_failed` means a rule existed but required measurement or
  arithmetic could not be completed. Amounts are excluded from aggregate
  totals until an administrator reconciles the record.

`request_id` is the synchronous idempotency key. Local `task_id` is the
asynchronous idempotency key. State transitions use conditional GORM updates,
so polling retries, duplicate callbacks, and reconciliation retries cannot
double-recognize cost.

A `submit_accepted` rule may settle cost after accepted submission. If the
task later fails and user quota is refunded, revenue becomes zero while the
recognized provider cost remains, producing a valid negative profit.

## 11. Routing and Strict Enforcement

Cost accounting has a global disabled/strict setting. It defaults to disabled
after migration so existing installations do not lose all channels before
cost rules are configured. Once an administrator enables strict mode, there
is no permissive fallback.

Strict enforcement occurs twice:

1. Candidate construction resolves each candidate's mapped upstream model and
   filters candidates without a complete active cost rule.
2. Immediately before the upstream request, the final channel and model are
   resolved again and the immutable cost snapshot is created.

The second check handles model-routing targets, ordinary channel model maps,
rule activation races, and stale distributed caches. If every candidate is
ineligible, the client receives the existing safe channel-unavailable shape
with a stable error code. It must not expose which rule or price is missing.
The administrator warning includes the channel and upstream model.

Active rules are cached by channel and upstream model under a monotonic shared
cost-rule revision. Activation, deactivation, channel deletion, and relevant
model-map changes advance the revision and invalidate affected entries on all
nodes. The final pre-send check may use a cache entry only after observing the
current revision; otherwise it reloads the authoritative database row. If the
current revision or authoritative rule cannot be obtained, strict mode fails
closed before upstream traffic.

## 12. Synchronous and Asynchronous Data Flow

### 12.1 Synchronous Request

1. Existing pricing resolves the official sale price from the client model.
2. Distribution selects a cost-eligible channel and mapped upstream model.
3. Cost accounting creates a `pending` record and attaches its snapshot to
   `RelayInfo` before upstream traffic. Failure to persist this record rejects
   the request in strict mode.
4. Existing billing performs pre-consume and final settlement unchanged.
5. The adaptor exposes authoritative token or duration usage when required.
6. Cost accounting reads the final user quota, finalizes cost, and writes
   revenue, profit, and administrator log details.
7. A missing declared measurement changes only cost status and warning data;
   it does not replace a successful client response with an error.

### 12.2 Asynchronous Task

1. Submission follows the same sale-price, routing, and cost-rule resolution.
2. The accounting record ID and cost snapshot are written to
   `Task.PrivateData` before contacting the provider. Failure to persist either
   side rejects submission before provider traffic.
3. The submit result records an accepted-submission cost when configured;
   otherwise the record remains `pending`.
4. Polling persists authoritative result duration or token usage.
5. Existing task billing settles or refunds user quota.
6. Cost accounting finalizes from the task snapshot and terminal result with
   an idempotent state transition.
7. A terminal provider failure becomes `zero_cost` unless the accepted
   submission was already contractually billable.

## 13. Administrator User Interface

### 13.1 Channel Model Cost Configuration

Add a model-cost tab to channel administration. Its table shows upstream
model, mapped client models, current official sale price, cost mode, original
currency cost, normalized USD cost, active version, source, and status.

The create/edit drawer changes fields by cost mode and provides:

- original-currency and conversion inputs;
- `total` versus `input_output` token configuration;
- explicit measurement source and charge event;
- a line-by-line USD conversion preview;
- current official sale price, selected group-discount preview, estimated
  profit, and estimated margin;
- draft save, validation, activation, deactivation, and version history.

The preview is labeled as an estimate. Historical realized profit always uses
request snapshots.

### 13.2 Administrator Log Details

Request details show realized revenue, provider cost, gross profit, gross
margin, status, actual channel, upstream model, rule version, measurement,
original-currency amount, and conversion snapshot. A `settlement_failed`
record provides an audited measurement backfill and reconciliation action.

No cost or profit fields appear in ordinary user logs or public pricing APIs.

### 13.3 Profit Report

The report filters by time, channel, upstream model, client model, user group,
and cost status. It displays:

- total revenue, total cost, gross profit, and gross margin;
- settled request count, negative-profit count, pending count, and failed
  settlement count;
- channel and model breakdown rows.

Only `settled` and `zero_cost` records participate in amount totals. Pending
and failed records appear as separate counts. Async tasks become realized only
after final settlement.

## 14. Phase 2 Excel and CSV Import

Phase 2 adds a browser upload workflow:

```text
upload -> map columns -> validate and preview -> confirm proposals
```

The importer accepts local `.xlsx` and `.csv` files with size and row-count
limits. It does not retain the original file after processing. Reusable import
templates store only column mappings and parsing choices.

For a Lucen-style sheet:

- model ID maps to `upstream_model`;
- non-empty `元/秒` maps to `per_duration`;
- non-empty `元/次` maps to `per_request`;
- non-empty `元/1M` maps to `per_token`;
- recharge ratio, fee, billing multiplier, currency, and selected discount
  columns map to their structured fields;
- unsupported capability columns may be ignored or copied into the proposal
  note, but cannot silently alter cost calculation.

One import produces two independent proposals:

1. an official global sale-price proposal keyed by client model;
2. a channel-cost proposal keyed by channel and upstream model.

The administrator reviews and confirms each proposal separately. Confirmation
creates new versions; it never updates active rows or historical records in
place. Preview classifies rows as new, changed, unchanged, conflicting, or
invalid. Invalid or ambiguous rows cannot be activated.

An import batch stores its ID, file hash, selected channel, parsing template,
column mapping, selected discount column, actor, confirmation choices, and
result summary. Re-uploading an identical file is detectable, but the
administrator may intentionally create a later version after explicit review.

## 15. API Boundaries

Administrator-only endpoints cover:

- rule list, draft creation, validation preview, activation, deactivation, and
  version history;
- accounting-record details and audited reconciliation;
- aggregate profit summary and grouped breakdown;
- Phase 2 import upload preview and independent proposal confirmation.

Request DTOs use explicit nullable scalar fields where omission differs from
zero. All JSON operations use the wrappers in `common/json.go`. API responses
return decimal display strings and integer nano-USD source amounts so clients
do not depend on binary floating-point serialization.

## 16. Validation and Error Handling

Rule activation rejects:

- missing channel or upstream model;
- unsupported cost mode or measurement source;
- zero, negative, NaN, infinite, or unparsable required prices and ratios;
- negative fees;
- incomplete token variant fields;
- a measurement source the selected channel/adaptor cannot provide;
- overlapping effective versions;
- normalized values that overflow the supported accounting range.

Unexpected measurement absence after an accepted request records a stable
failure code and request-correlated administrator warning. It never invents
usage, assumes cost equals sale price, silently changes measurement source, or
turns cost arithmetic into user quota credit.

Before provider traffic, rule lookup, shared revision, snapshot validation, or
pending-record persistence errors fail closed in strict mode. After provider
acceptance, accounting persistence errors must not hide a successful client
result; they retain or move the record to `settlement_failed`, emit an
administrator warning, and remain eligible for idempotent reconciliation.

Reconciliation records the administrator, old state, supplied measurement,
new result, timestamp, and reason. It uses the original snapshot, not the
current active rule.

## 17. Database Compatibility and Migration

All tables use GORM and must work on SQLite, MySQL 5.7.8+, and PostgreSQL 9.6+.
Decimal rule values and original amounts are stored as canonical text to avoid
dialect-specific decimal normalization. Aggregate nano-USD fields are
`int64`. Boolean business defaults are assigned in code rather than GORM
boolean default tags.

Migration adds the rule and accounting tables plus only the compact log fields
needed for administrator display. The feature setting remains disabled. The
administrator configures and validates rules, reviews uncovered candidate
channels, and explicitly enables strict mode.

Existing requests and logs have no fabricated cost record and appear as
historical cost unavailable rather than zero-profit data.

## 18. Test Strategy

Backend tests must cover observable accounting and routing contracts:

1. exact per-request, per-duration, total-token, and input/output-token cost;
2. original currency, multiplier, discount, recharge ratio, fee, and FX
   conversion snapshots;
3. active-version transitions and historical immutability;
4. strict candidate filtering and all-candidates-ineligible behavior;
5. the final pre-send rule check after model routing and ordinary model maps;
6. synchronous completion and final user-charge revenue recognition;
7. async success, zero-cost failure, and accepted-submission provider cost;
8. missing token or duration measurement becoming `settlement_failed` without
   changing the client result;
9. polling, callback, and reconciliation idempotency;
10. negative profit, zero-revenue null margin, invalid negative values, and
    checked overflow behavior;
11. aggregate totals including only settled and zero-cost records;
12. administrator authorization and non-administrator field isolation;
13. deterministic GORM behavior on SQLite, MySQL, and PostgreSQL.

Frontend tests cover mode-specific form fields, conversion previews, rule
history behavior, administrator-only log rendering, report filters and totals,
empty/error/loading states, and responsive text containment. All new UI text
is added to every supported locale.

Phase 2 adds deterministic import fixtures for Lucen-style Excel and CSV,
column mapping, locale-aware numbers, duplicate files, conflicting rows,
invalid rows, preview classifications, and independent confirmation of the
sale and cost proposals.

## 19. Scope Boundaries

Phase 1 does not include:

- live provider pricing synchronization;
- automatic retroactive profit recalculation;
- executable provider-specific cost expressions;
- silently estimated token usage;
- recharge-income, payment-processor, gift-credit, tax, or company-level net
  profit accounting;
- exposing upstream costs or profit to ordinary users.

Phase 2 adds only local Excel/CSV proposal import. Automatic remote price-feed
synchronization remains a separate future design.
