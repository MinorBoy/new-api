# Routing Target Enable-All Switch Design

**Date:** 2026-07-29

## Goal

Add an `Enable all targets` switch to the routing-policy editor, immediately
to the left of the existing `Add target` button. The control lets an
administrator enable or disable every currently configured route target
without changing any other target fields.

## Decisions

- The switch is bidirectional: switching it on enables every current target;
  switching it off disables every current target.
- The switch is on only when at least one target exists and every target is
  enabled. A mixed target state is represented as off because the Switch
  component has no indeterminate state.
- The switch is disabled when the policy has no targets.
- A newly added target keeps the existing disabled default. If all earlier
  targets were enabled, adding a target therefore returns the bulk switch to
  off.
- The bulk target switch does not change the policy-level `Enabled` field.

## UI And State Flow

The control is rendered in the routing-target section header in a compact
label-and-switch group before `Add target`. Existing responsive wrapping is
preserved so the controls do not overflow narrow drawers.

`RoutingPolicyDrawer` watches the target array through React Hook Form. The
bulk checked state is derived from the current target values. When the switch
changes, the drawer updates only each `targets.<index>.enabled` field with
validation and dirty-state tracking enabled. It does not replace the complete
target array, so target editor identity, unsaved values, focus, and field
errors remain intact.

## Internationalization And Accessibility

- The visible label uses the English source key `Enable all targets` through
  `useTranslation()`.
- All supported frontend locales receive a translation for the new key.
- The label is associated with the switch through its accessible name, making
  the control discoverable and operable by keyboard and assistive technology.

## Testing

Component coverage will verify these user-visible contracts:

- The switch is disabled for an empty target list.
- Switching on enables all current targets.
- Switching off disables all current targets.
- Mixed target state renders the switch as off.
- Adding a disabled target after an all-enabled state returns the switch to
  off without changing the earlier targets.
- The policy-level enabled field remains unchanged by bulk target operations.

The affected component test, model-routing component tests, TypeScript type
check, targeted lint, formatting check, and production frontend build will be
run before completion.

## Acceptance Criteria

In the routing-policy editor, an administrator can use a clearly labeled
switch beside `Add target` to enable or disable all current route targets. The
control always reflects the current form state, preserves unrelated target
edits, remains usable by keyboard, and does not alter policy-level enablement.
