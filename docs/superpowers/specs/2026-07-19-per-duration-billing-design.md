# Per-Duration Model Pricing Design

## 1. Purpose

Add a first-class `per_duration` model billing mode for video generation. The
mode must represent duration pricing directly instead of treating a fixed
`ModelPrice` as a per-second price and multiplying it by an implicit
`OtherRatios["seconds"]` value.

Dimensio Seedance 2.0 is the first consumer. The design remains provider
independent so later video adaptors can opt in without duplicating pricing,
rounding, minimum-duration, quota-safety, persistence, or frontend behavior.

## 2. Confirmed Business Decisions

1. `ModelPrice` keeps its existing meaning: a fixed USD price for one request.
2. Duration pricing is stored separately and activated with
   `billing_mode = "per_duration"`.
3. Model pricing is a global user sale price. It is keyed by the client-facing
   `OriginModelName`; channel selection and channel model mapping do not change
   the user's price.
4. The default sale price is prefilled from the upstream public cost, converted
   to USD. Administrators may edit it, and user-group ratios remain the common
   markup or discount mechanism.
5. The system does not add a channel cost ledger, profit-margin engine, or
   automatic cost-plus pricing in this change.
6. Dimensio query responses do not contain a contractual duration field.
   Dimensio billing therefore uses the validated duration sent in the submit
   request and never replaces it with query-response data.

## 3. Existing Billing Modes

The system currently exposes three user-facing pricing styles:

| Mode | Storage | User charge base |
|---|---|---|
| Per token | `ModelRatio` and related ratio maps | Token usage and configured USD-per-million-token prices |
| Per request | `ModelPrice` | Fixed USD amount for one request |
| Expression | `billing_setting.billing_expr` | Expression result using actual or estimated token usage |

All three apply the effective user-group ratio. Built-in group ratios for
`default`, `vip`, and `svip` are `1`, so the configured base price is also the
default user price.

`per_duration` becomes the fourth user-facing pricing style. It is not an
alias for per-request pricing and does not reuse `ModelPrice`.

## 4. Configuration Model

The layered billing configuration gains a new map:

```text
billing_setting.duration_price
```

Each key is a client-facing model name. Each value has this shape:

```json
{
  "price": 0.0849315068,
  "unit": "second",
  "rounding_step_seconds": 1,
  "minimum_duration_seconds": 4
}
```

Field semantics:

| Field | Meaning |
|---|---|
| `price` | User sale price in USD per configured unit; zero is allowed for a free model |
| `unit` | `second` or `minute` |
| `rounding_step_seconds` | Positive integer billing step measured in seconds |
| `minimum_duration_seconds` | Non-negative minimum billable duration measured in seconds |

The step and minimum use explicit seconds even when `unit` is `minute`. This
allows rules such as USD/minute with 5-second increments without fractional or
ambiguous configuration values.

Configuration validation rejects:

- negative, NaN, or infinite prices;
- units other than `second` and `minute`;
- a zero or negative rounding step;
- negative minimum durations;
- step or minimum values above `relaycommon.MaxTaskDurationSeconds`.

The three direct Dimensio model names receive built-in rules. An administrator
using a custom ARK alias must configure the alias explicitly because pricing is
based on `OriginModelName`.

## 5. Central Calculation

The task adaptor returns a validated requested duration in seconds through an
explicit duration-estimation interface. It does not add duration to
`OtherRatios`.

The central task billing path calculates:

```text
normalized_seconds = max(requested_seconds, minimum_duration_seconds)
billable_seconds = ceil(normalized_seconds / rounding_step_seconds)
                   * rounding_step_seconds
unit_seconds = 1 for second, 60 for minute
duration_units = billable_seconds / unit_seconds
base_usd = price * duration_units
quota = base_usd * QuotaPerUnit * group_ratio * non_duration_ratios
```

Decimal arithmetic is used for price and unit conversion. Final quota uses
`common.QuotaFromDecimalChecked`; a saturation marker is attached to the
request and its admin-only log metadata. Request duration remains bounded by
`relaycommon.MaxTaskDurationSeconds` before calculation.

`seconds` and `duration` are reserved ratio names in `per_duration` mode. If an
adaptor supplies either through `OtherRatios`, the request is rejected before
the upstream call to prevent double charging.

## 6. Pricing and Model Mapping

The pricing lookup key is always `RelayInfo.OriginModelName`.

```text
client model -> global sale-price lookup
             -> channel selection
             -> channel model mapping -> upstream request model
```

The current uncommitted Dimensio-only exception that copies
`UpstreamModelName` into the pricing lookup must be removed. Capability checks
may use `UpstreamModelName`, but pricing may not.

This preserves the existing system contract: a client receives one configured
sale price for a model regardless of which eligible channel serves the
request. Administrators who want separately priced Dimensio variants expose
separate client-facing aliases and map each alias to its intended upstream
model.

## 7. Dimensio Defaults

The latest Dimensio cost is:

| Model | 720p | 1080p |
|---|---:|---:|
| `jimeng-video-seedance-2.0-fast-vip` | 48 points/second | Unsupported |
| `jimeng-video-seedance-2.0-mini` | 39 points/second | Unsupported |
| `jimeng-video-seedance-2.0-vip` | 62 points/second | 155 points/second |

With `1 point = CNY 0.01` and the project's default `1 USD = CNY 7.3`, the
built-in user sale prices are:

| Model and resolution | USD/second |
|---|---:|
| fast-vip 720p | `0.0657534247` |
| mini 720p | `0.0534246575` |
| vip 720p | `0.0849315068` |
| vip 1080p | `0.2123287670` |

Each direct model rule uses `unit=second`, `rounding_step_seconds=1`, and
`minimum_duration_seconds=4`. The VIP base price is its 720p price; the
Dimensio adaptor continues to supply the non-duration resolution ratio `2.5`
for 1080p. Fast-vip and mini reject 1080p before billing and before the
upstream request.

These are default user sale prices, not a live cost feed. Changing the site's
exchange rate does not silently rewrite saved model prices.

## 8. Submission, Snapshot, and Settlement

At submission:

1. The adaptor validates and normalizes the request duration.
2. The channel model mapping resolves the upstream capability target.
3. Pricing loads by `OriginModelName`.
4. The adaptor provides requested duration and non-duration ratios.
5. The central calculator derives billable duration and quota.
6. The full pricing rule and calculation inputs are frozen into
   `TaskBillingContext`.
7. The resulting quota is pre-consumed before contacting Dimensio.

The snapshot records:

- `billing_mode`;
- duration `price`, `unit`, rounding step, and minimum;
- `duration_source = "request"`;
- requested and billable duration seconds;
- group ratio and non-duration ratios;
- origin and upstream model names.

Successful Dimensio completion keeps the submitted charge because there is no
authoritative response duration. A terminal failure refunds the full charge
through the existing wallet/subscription, token, user-used-quota, channel-used-
quota, and quota-data paths. Retriable polling errors such as `1057` keep the
task unfinished and retain the pre-consumed amount.

## 9. Logs and Public Pricing

Task consume and refund logs expose the duration snapshot rather than a
misleading `model_price` value:

```json
{
  "billing_mode": "per_duration",
  "duration_price": 0.0849315068,
  "duration_unit": "second",
  "duration_source": "request",
  "requested_duration_seconds": 6,
  "billable_duration_seconds": 6,
  "rounding_step_seconds": 1,
  "minimum_duration_seconds": 4,
  "resolution": "1080p",
  "resolution_ratio": 2.5,
  "group_ratio": 1
}
```

The public pricing API includes `billing_mode=per_duration` and the structured
duration rule. The existing `quota_type` value remains compatible; frontend
logic uses `billing_mode` to distinguish fixed-request and duration-based
prices.

## 10. Frontend Editing

The default frontend model-pricing editor gains a fourth mode,
`Per-duration`. Selecting it shows:

- unit price in USD;
- a second/minute segmented option or select control;
- rounding step in seconds;
- minimum billable duration in seconds;
- a preview of the normalized pricing rule.

Saving `per_duration` removes conflicting per-token, per-request, and
expression values for the model and writes both the billing mode and duration
rule. Switching away removes the duration rule. Batch copy, delete, unset-price
filtering, JSON editing, snapshots, dirty-state comparison, and mode counts all
include duration pricing.

The public pricing table, cards, and detail view label these models
`Duration-based` and render the configured USD-derived display price per second
or minute. They never label a duration unit price as a per-request price.

All new user-facing strings are added to `en`, `zh`, `fr`, `ru`, `ja`, and
`vi`, followed by the project's i18n synchronization check.

## 11. Error Handling

The request is rejected before an upstream call when:

- `per_duration` is active but no valid duration rule exists;
- the selected task adaptor does not implement explicit duration estimation;
- requested duration is absent, non-positive, or above the shared maximum;
- a reserved duration multiplier appears in `OtherRatios`;
- the mapped Dimensio model/resolution combination is unsupported.

Configuration API updates return an actionable validation error and do not
replace the in-memory duration map when any entry is invalid.

## 12. Compatibility

- Existing per-token, fixed-request, expression, and task billing behavior is
  unchanged unless a model is explicitly or by built-in default configured as
  `per_duration`.
- Existing task rows remain readable because all new snapshot fields use
  `omitempty` and old `PerCallBilling` behavior remains supported.
- Existing task adaptors do not need to implement duration estimation unless a
  model routed through them enables `per_duration`.
- No database schema migration is needed; configuration remains in the options
  table and the task snapshot remains JSON.
- Existing `OtherRatios` continue to support resolution and other non-duration
  dimensions.

## 13. Verification and Acceptance

Deterministic tests must cover:

1. configuration validation and default Dimensio rules;
2. second and minute units, minimum duration, step rounding, group ratios, and
   non-duration multipliers;
3. quota saturation auditing and invalid-duration rejection;
4. origin-model pricing despite channel model mapping;
5. absence of `seconds` from Dimensio `OtherRatios`;
6. pricing snapshots and logs;
7. frontend parse, edit, preview, save, delete, batch-copy, filter, and public
   display behavior;
8. mock Dimensio success, terminal failure, `-2011`, and retriable `1057` for
   all three Seedance models;
9. full ARK prompt + reference image + reference video + reference audio
   conversion and both response directions;
10. success charge retention and complete failed-task refund across user,
    channel, and token ledgers.

The acceptance report is updated with exact ARK, Dimensio, and ARK response
structures plus the new duration pricing formula and observed quota values.
No real Dimensio request is made.

## 14. Out of Scope

- channel-specific user sale prices;
- a separate upstream cost ledger;
- automatic markup or margin calculation;
- dynamically refreshing Dimensio costs or currency rates;
- billing from media-file metadata;
- claiming an actual generated duration that Dimensio does not return.
