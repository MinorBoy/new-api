# Cost Accounting Tracking And Dimensio Remediation Design

**Date:** 2026-07-29

## Goal

Restore supplier-cost records for the production `dimensio (5)` channel
without changing or disabling Dimensio's upstream automatic routing. Correct
the usage-log billing presentation, preserve traffic on channels whose costs
are not configured yet, and verify the result with a new real video request.

## Confirmed Root Causes

- Usage logs already store `billing_mode: per_duration`, but the details
  dialog falls through to the per-token presentation.
- Cost accounting is globally disabled, so the request and attempt ledgers are
  never created.
- Config-import batch 4 bound `channel-dimensio` to test channel 9. Its ten
  active supplier-cost rules therefore belong to channel 9, while production
  requests use channel 5.
- Strict mode cannot currently be enabled safely. Enabled channels 6 and 7,
  and default channel abilities, still have uncovered cost contracts.
- The historical log fallback incorrectly claims that any unlinked log
  predates cost-accounting references. Recent logs created while accounting
  was disabled receive the same misleading message.

## Scope

### Included

- Add a non-blocking `tracking` cost-accounting mode.
- Correct usage-log rendering for per-duration billing.
- Replace the misleading historical-cost fallback with a neutral statement
  that the request has no supplier-cost record.
- Update the supplied Excel workbook so Dimensio Mini 720p uses the currently
  effective procurement estimate of CNY 0.25 per second.
- Publish the ten Dimensio rules against production channel 5 through the
  existing config-import workflow.
- Enable tracking mode and verify a new end-to-end request and its ledger.

### Excluded

- Disabling or changing Dimensio's upstream automatic routing.
- Parsing `creditsConsumed` or `effective_model` from Dimensio responses.
- Adding a Dimensio-specific automatic-routing configuration model.
- Retrofitting supplier-cost ledger rows onto historical requests.
- Disabling channels 6 or 7, inventing placeholder costs, or enabling strict
  mode before global coverage is complete.

## Accounting Modes

The mode contract becomes:

- `disabled`: no supplier-cost request or attempt is created.
- `tracking`: create and settle supplier-cost ledgers when an active compatible
  rule exists, but never filter routes or block an upstream request because of
  missing coverage or an accounting-only failure.
- `strict`: retain the existing fail-closed coverage and minimum-margin gates.

Tracking mode uses the same immutable rule snapshots, attempt lifecycle,
settlement, revenue recognition, reports, and admin-only log linkage as strict
mode. Only admission behavior differs. A missing rule or accounting setup
failure is written as a request-correlated warning and the relay continues
without a cost ledger for that request. No zero or fabricated cost is written.

The settings API accepts and returns `tracking`. The cost-accounting page shows
a three-state control. Coverage completeness continues to gate only `strict`;
`tracking` remains selectable while coverage is incomplete.

## Task Data Flow

For task relays in tracking mode:

1. Validate the request and calculate user billing normally.
2. Confirm the final channel and upstream-model identity.
3. Look up an active cost rule for the channel, upstream model, and cost
   variant.
4. If a compatible rule exists, prepare and authorize the cost attempt before
   dispatch, then preserve its identifier on the task.
5. If coverage is missing or accounting preparation fails, emit a warning and
   continue dispatch without a cost handle.
6. On terminal task success, settle the existing per-duration rule from the
   validated requested duration. On task failure, follow the existing task
   cost outcome rules.
7. Link the completed cost request into the administrator's usage-log detail.

Strict mode keeps its current pre-dispatch profit recheck and retry/exclusion
behavior. Tracking mode does not run those gates.

## Dimensio Rule Remediation

The supplied workbook remains the durable source for imported configuration.
The Dimensio Mini 720p cost row is changed from CNY 0.39 per second to CNY 0.25
per second, including its normalized USD value. Other Dimensio rules keep their
existing imported values.

The corrected workbook is imported with `channel-dimensio` explicitly bound to
production channel 5. The existing staging and publishing services create and
activate versioned channel-5 rules. Test channel 9 is not deleted, and its rule
history is preserved. Before publication, the review must show ten Dimensio
rules for channel 5 and no unexpected changes to other channel bindings.

## Usage-Log Presentation

`BillingBreakdown` gets an explicit `per_duration` branch. It displays:

- billing mode as `Per-duration`;
- duration price when present;
- billable duration when present;
- the existing total charged to the user.

The supplier-cost section uses a neutral no-record message when
`cost_accounting_request_id` is absent. It must not infer request age from the
absence of a ledger reference.

All new user-facing strings use the existing i18n workflow and are translated
for every supported locale.

## Error Handling And Safety

- Tracking mode never changes user billing or silently credits/refunds users.
- Accounting failures are isolated from upstream relay success in tracking
  mode and remain fail-closed in strict mode.
- A missing rule produces no cost amount rather than an assumed zero.
- Rule creation and activation use the existing validated config-import path;
  direct ad hoc database inserts are not used.
- The Mini 720p rule continues to use the existing bounded, validated duration
  meter and centralized decimal cost conversion.
- Existing cost-accounting uniqueness, versioning, snapshot, and recovery
  invariants remain unchanged.

## Testing

Backend regression coverage will verify:

- `tracking` is a valid runtime and API mode.
- Tracking mode prepares and settles a task when coverage exists.
- Missing coverage in tracking mode does not block or reroute the request and
  does not create a fabricated zero-cost ledger.
- Strict mode retains its current fail-closed behavior.
- Disabled mode still creates no accounting records.

Frontend regression coverage will verify:

- `per_duration` renders as `Per-duration`, not `Per-token`.
- Duration price and billable duration are shown when supplied.
- A recent unlinked log uses the neutral no-record message.
- Tracking mode is selectable with incomplete coverage, while strict remains
  coverage-gated.

Verification includes focused Go and frontend tests, frontend type checking,
targeted lint and formatting checks, production build, container rebuild, and
a real browser/API task submission.

## Acceptance Criteria

- Dimensio automatic routing remains enabled and untouched.
- Production channel 5 has ten active imported Dimensio cost rules.
- Mini 720p is configured at CNY 0.25 per second in both the workbook and the
  active channel-5 rule.
- Cost accounting runs in tracking mode without interrupting uncovered
  channels.
- A new five-second Mini 720p request creates a cost request and settled attempt
  with CNY 1.25 supplier cost.
- The same ledger records the request revenue and exposes gross profit and
  margin in the admin UI.
- Usage-log details identify per-duration user billing correctly and link to
  the supplier-cost record.
