# Routing Target Automatic Name Design

## Goal

Automatically populate a new routing target's administrative name from the target's effective configuration so operators do not need to invent names manually.

The generated name format is:

```text
YYYYMMDD-channel-resolution-speed-duration
```

Example:

```text
20260724-A1-720p-fast-4-15s
```

## Inputs

The name is built from existing form state only:

- Date: the browser's current local date formatted as `YYYYMMDD`.
- Channel: the selected routing candidate's channel name.
- Resolution: selected output resolutions in the existing UI order, joined with `+` when more than one is selected.
- Speed: derived from the policy's canonical model ID. IDs containing `-fast-` use `fast`, IDs containing `-mini-` use `mini`, and the standard model uses `standard`.
- Duration: range mode uses `min-maxs`; discrete mode sorts and deduplicates values, joins them with `+`, and appends `s`.

The function returns no generated name until a channel has been selected. Existing validation continues to require a non-empty name before saving.

## Update Behavior

For a newly added target, selecting a channel generates the first name. The generated name then follows changes to the selected channel, canonical model, output resolutions, and allowed generation durations.

The UI tracks the most recent generated value locally. It may replace the name only when the current field is empty or still equals the most recent generated value. Once an operator types a different name, later capability changes preserve that manual value.

Existing targets loaded for editing are treated as manually named and are not renamed automatically. Copied targets retain the existing `copy` naming behavior and are also not renamed automatically.

## Scope

This is a frontend-only convenience feature. It does not add API fields, change the routing policy database schema, change matching behavior, or parse upstream model IDs.

The target `name` field remains editable and is still submitted through the existing request contract.

## Testing

Pure naming tests cover:

- Standard, fast, and mini speed derivation.
- Range and discrete duration formatting.
- Multiple resolution formatting.
- No name before channel selection.

Component tests cover:

- Initial generation after channel selection.
- Automatic refresh while the current value is system-generated.
- Preservation after manual editing.
- Preservation of names loaded from existing or copied targets.

Browser acceptance confirms the generated value appears in the routing target name input and changes with capability controls without layout regressions.
