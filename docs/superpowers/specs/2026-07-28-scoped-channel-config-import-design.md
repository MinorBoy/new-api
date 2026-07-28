# Scoped Excel Channel Configuration Import Design

**Date:** 2026-07-28

## Goal

Allow an administrator to convert a supported channel-cost workbook locally in
the browser, select channel groups and individual channel lines, then either
download a self-contained JSON configuration document or create a new
configuration-import batch from that selected scope.

The existing standalone converter remains available for offline review and
selected-JSON export. The administrator import page adds the direct-import
workflow; it does not upload Excel files to the backend.

## Decisions

- Selection is business-scoped by channel group and channel line, never by
  arbitrary JSON entity row.
- A group checkbox selects every line in the group. Individual lines remain
  selectable, including partial selection of a group.
- A selected scope contains only the selected lines and their dependency
  closure. Unselected-line conflicts do not block it.
- The scoped document must pass the existing import schema before it can be
  formally exported or submitted to the batch-create API.
- A direct import creates a new batch and immediately enters the existing
  channel-binding step. It never modifies an existing batch.
- The workbook, converted document, and selection live only in browser memory.
  Reloading or leaving the page requires a new upload. Server-side persistence
  begins only after batch creation.
- Imported channels and routing policies remain disabled by default. This work
  does not enter credentials or enable traffic.

## Scope

### Included

- Reusable client-side workbook conversion and scoped-document construction.
- Grouped, multi-select channel-line UI with all/select-none actions and
  indeterminate group state.
- Scoped JSON export in both converter surfaces.
- Direct selected-scope batch creation from the administrator import page.
- Dependency summary, scoped validation, i18n, and automated coverage.

### Excluded

- A backend Excel upload, storage, parsing, or conversion API.
- Selection of individual cost, model-mapping, route, or pricing JSON rows.
- Automatic credential entry, channel enablement, route enablement, or publish.
- Persisting raw workbooks, client-side selection state, or conversion results.

## Architecture

The current V1/V2 adapters, workbook preflight, snapshot loader, and document
builder become a reusable client-side conversion boundary rather than logic
owned by the standalone converter screen. Both surfaces consume it:

```text
Excel file
  -> preflight + adapter + canonical import document
  -> channel-line scope selection
  -> dependency-closed scoped document
  -> existing schema validation
  -> selected JSON download or existing batch-create API
```

The standalone converter uses the download branch. The administrator page uses
the batch-create branch, then delegates all further work to the existing
binding, conflict-resolution, pricing, routing-review, validation, and publish
steps.

No backend route or database schema changes are required. The server continues
to receive only `new-api.channel-config-import` JSON documents through the
existing batch-create API.

## Scoped Document Construction

`selectedLineRefs` is the authoritative selection. Group selection is expanded
to its child line references before building the document.

The builder produces a new import document rather than mutating the original:

1. Include selected `channel_lines` and the parent `channels` records needed by
   those lines.
2. Include cost-rule drafts and model mappings whose `line_ref` is selected.
3. Filter each route blueprint to selected targets; discard a blueprint with no
   targets after filtering.
4. Include referenced model SKUs, sale proposals, and sources recursively.
5. Include only issues and unresolved variants that are attributable to the
   selected scope. Items without a verified selected line identity are omitted.
6. Recalculate all manifest counts and `payload_sha256`.
7. Run the existing JSON schema and reference validation on the rebuilt
   document.

The result is exportable or importable only when at least one line is selected
and validation finds no blocking failure. Warnings remain visible in the scope
summary.

## Administrator UX

Before any batch exists, the import page exposes two source modes:

- **Excel conversion:** upload a supported workbook, select scope, review its
  dependency summary, export JSON or create a batch.
- **JSON import:** retain the current direct JSON-upload behavior unchanged.

After a successful Excel conversion, the selection screen provides:

- A group tree with expandable group headers and child line checkboxes.
- Three-state group checkboxes and global select-all/clear actions.
- Optional group or line filtering.
- A stable summary of selected groups, lines, attached costs, mappings, route
  targets, SKUs, sale proposals, and scoped warnings/errors.
- `Export selected JSON` and `Import selected configuration` commands.

The direct-import command sends the validated scoped document to the existing
batch-create API. On success, it replaces local conversion state with the new
batch detail and renders the current channel-binding step.

The standalone converter uses the same selector and summary but offers only
export. It has no dependency on authentication or the import API.

## Error Handling And Safety

- Workbook preflight, unsupported-template, parse, and source-reference errors
  are displayed locally and cause no network request.
- Empty selection, dangling references, an empty filtered route blueprint, or
  blocking selected-scope issues disable formal export and direct import.
- A batch-create failure leaves the converted document and selection in memory
  so the administrator can correct the scope or retry.
- An import batch is never created from raw Excel bytes. Only the canonical,
  scoped JSON document crosses the network boundary.
- Existing disabled-by-default channel, route, and credential-confirmation
  rules remain unchanged.

## Internationalization And Accessibility

- All new visible strings use `useTranslation()` and are added to the seven
  frontend locale files.
- Group and line checkboxes have clear accessible names; partial group state is
  represented with the native or component-supported indeterminate state.
- Keyboard selection, focus order, validation errors, and disabled command
  reasons remain accessible without relying on color.

## Verification

### Unit Tests

- Group expansion, partial group selection, select-all, and empty selection.
- Dependency closure for selected lines, including filtered routing targets.
- Exclusion of unselected conflicts and invalid unverified variants.
- Rebuilt counts, deterministic payload hash, schema validity, and no dangling
  references for V1 and V2 fixtures.

### Component Tests

- Group tri-state behavior, line selection, global actions, counters, and
  command availability.
- Export receives only the scoped document.
- Administrator direct import submits the scoped document and moves into the
  returned batch state.
- Existing JSON upload remains unchanged.

### Browser And End-to-End Tests

- Workbook conversion makes no network request before direct import and leaves
  no browser persistence.
- Selecting only `secure-enterprise` creates a batch whose staged output has
  only that line's cost, model-mapping, and disabled routing targets.
- Unselected Secure lines and their conflicts do not block the scoped batch or
  appear in its published artifacts.
- Existing binding confirmation and disabled-route acceptance checks continue
  to pass.

## Acceptance Criteria

An administrator can upload a supported workbook, select all or a subset of
channel lines, see exactly what will be included, export a valid selected JSON
document, or create a new configuration-import batch from it. The existing
post-batch review flow remains intact, and no conversion operation can enable
traffic or transmit the original workbook.
