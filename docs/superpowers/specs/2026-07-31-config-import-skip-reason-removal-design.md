# Channel Binding Skip Reason Removal

## Goal

An operator can skip an imported channel line with one action. The channel
binding workflow must not render, request, validate, store, or restore a
skip-specific reason.

## Scope

- Remove the skip-reason input from `ChannelBindingStep`.
- Remove `reason` from the frontend `ConfigImportBinding` request schema.
- Remove `Reason` from the backend channel-binding DTO and its validation.
- Remove the reason from the saved skip-state snapshot. Skipped dependent items
  use an empty `exclusion_reason` while the skip binding owns them.
- Keep `ConfigImportItem.ExclusionReason` intact. It is a general import-item
  field used by other exclusion and resolution workflows, outside this change.

## Contract

A skipped binding is sent as:

```json
{
  "line_ref": "line-1",
  "action": "skip",
  "credentials_confirmed": false
}
```

The strict decoder rejects a legacy `reason` property because it is no longer
part of the channel-binding API. Existing persisted skip snapshots remain
readable because Go ignores their removed `reason` JSON property when decoding.

## Backend Behavior

Skipping sets the line's dependent import items to `excluded` with an empty
`exclusion_reason`. When the binding changes to `bind` or `create`, recovery
only restores rows that are still excluded with that empty reason. This retains
the existing protection against overwriting later, unrelated exclusion changes.

## Verification

- Frontend regression test: selecting `Skip` renders no skip-reason input and
  saving submits the reason-free binding contract.
- Backend regression test: a reason-free skip request succeeds; a request that
  still includes `reason` is rejected by the strict binding decoder.
- Run focused frontend and Go tests, TypeScript typecheck, affected-file lint
  and formatting checks, then perform browser acceptance on the binding page.
