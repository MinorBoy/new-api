# Routing Policy Drawer Semantics and Theme Design

**Status:** Approved on 2026-07-23

## Context

The first capability-routing implementation modeled an upstream model that returns 1080p from an internal 720p generation process as an `upscaled` routing capability. That interpretation is incorrect. The gateway does not request or control super-resolution. It selects an upstream model ID whose externally observable output capability is 1080p.

The routing-policy drawer also mixes native browser selects with project-themed controls, uses neutral selected states in several segmented controls, accepts a free-form group name, and labels allowed request duration as elapsed time.

## Decisions

### Remove Super-Resolution Routing Fields

Remove `upscaled` and `generation_resolution` from the complete routing contract:

- `pkg/modelrouting.Constraints` and validation errors;
- policy service and controller request/response DTOs;
- persisted route-target constraint JSON serialization;
- frontend API schemas, form schemas, defaults, converters, and controls;
- unit, integration, end-to-end fixtures, and documentation.

An upstream model such as `lec-feituo-seedance-2-0-my-upscaled-1080p` is configured as an ordinary target with `output_resolutions: ["1080p"]`. Its target name may explain that the provider internally derives 1080p from 720p, but this note does not participate in request matching, upstream parameters, pricing, or auditing.

Existing constraint JSON may still contain the removed properties. Normal decoding ignores unknown JSON properties, so no destructive database migration is required. Cache loading and API responses use the reduced typed structure. The obsolete properties disappear when a target is next saved.

### Use Project-Themed Dropdowns

Replace every native select in the routing-policy drawer and target editor with the existing Base UI `Select` or searchable `Combobox` components. This includes:

- group;
- canonical model;
- default output resolution;
- default aspect ratio;
- channel;
- any remaining single-choice structured option rendered as a dropdown.

Triggers, popups, items, hover states, selected states, focus rings, disabled states, and dark-theme colors come from the shared project components and semantic theme tokens. No routing-specific popup palette is introduced.

### Load Groups from the Existing Admin API

The drawer fetches `/api/group/` through the model-routing API module and a dedicated React Query key while the drawer is open. Returned group names are normalized, deduplicated, sorted, and exposed through a searchable single-select Combobox.

`auto` is not a valid policy key and is omitted if returned. When editing or copying a policy whose current group is absent from the latest response, the current group is retained as a selectable compatibility value instead of clearing the form.

The group control shows a loading state while the query is pending and a retryable error state when loading fails. An empty successful response shows an empty-state message and prevents creating an invalid free-form group. Group changes continue to refresh routing candidates through the existing group/model query key.

### Correct Labels and Selected-State Styling

Rename the target section label from `Duration` to `Allowed generation durations`. Keep the existing mutually exclusive discrete-values and inclusive-range representations; they describe which client-requested video durations may select the target.

Remove the entire resolution-mode area, including `Native`, `Upscaled`, and generation-resolution controls.

Within the routing-policy drawer, every selected or enabled interactive state uses the semantic `primary` palette:

- segmented options and toggle buttons use a primary-tinted background, primary border, and readable foreground;
- switches and checkboxes continue to use shared primary checked tokens;
- focus-visible rings use the shared ring token;
- dark mode resolves these tokens to the project's blue-accented visual treatment.

Unselected controls remain neutral. Destructive actions retain the destructive palette. Styling is scoped to the drawer controls or shared component variants only when the variant already expresses the same project-wide state contract.

## Data Flow

1. Opening the drawer resets the selected policy form and starts the group query.
2. The group query populates the themed searchable Combobox and preserves an existing unavailable value when necessary.
3. Group or canonical-model changes refresh compatible channel candidates through the current candidate endpoint.
4. The form serializes only observable routing constraints: output resolution, allowed duration, aspect ratio, reference limits, and real-person support.
5. The backend validates, stores, caches, and evaluates the same reduced constraint structure.

## Error Handling

- A group-query failure does not clear an existing editing value.
- New policy submission is blocked until a valid loaded group is selected.
- Existing server validation and overlap errors remain mapped to their current fields.
- Legacy unknown JSON properties do not cause cache or policy-load failures.

## Testing

Backend tests must prove that:

- the removed properties are absent from typed policy responses and newly saved JSON;
- legacy JSON containing the removed properties still loads and routes by output resolution;
- a provider-internal 720p-to-1080p model matches only an `1080p` client request because its configured output capability is `1080p`;
- existing duration, ratio, reference-limit, real-person, overlap, retry, privacy, and billing contracts remain unchanged.

Frontend tests must prove that:

- all routing drawer dropdowns use project-themed Select/Combobox primitives;
- group options load from `/api/group/`, are normalized and sorted, exclude `auto`, and preserve a missing current value;
- form serialization never emits `upscaled` or `generation_resolution`;
- the duration label uses `Allowed generation durations`;
- selected segmented controls use the primary-state classes.

Browser verification covers desktop and 390px mobile layouts in dark mode, including popup colors, selected/hover/focus states, group loading and empty states, footer reachability, text wrapping, and the absence of horizontal overflow.

## Non-Goals

- Do not add a replacement super-resolution flag, note field, or request parameter.
- Do not infer provider implementation details from upstream model IDs.
- Do not change route selection, channel priority, weight, affinity, retry, billing, or public canonical model identity.
- Do not redesign dropdown components outside the routing-policy drawer.

## Acceptance Criteria

- Administrators can only select a group returned by the existing group API, except that an existing unavailable value remains editable without data loss.
- Every dropdown and selected state in the drawer follows project theme tokens in light and dark themes.
- No UI, API response, new persistence payload, cache snapshot, or runtime matcher contains super-resolution routing fields.
- A target whose provider name mentions upscaling is configured and matched solely by its externally observable output resolution.
- The drawer labels durations as allowed generation durations and retains working discrete/range validation.
