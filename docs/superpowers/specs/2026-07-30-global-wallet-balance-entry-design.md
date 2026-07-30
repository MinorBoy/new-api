# Global Wallet Balance Entry Design

## Goal

Show the authenticated user's current wallet balance in the global application
header, between the theme settings control and account menu. Selecting the
control navigates to `/wallet`.

## UI

- Desktop renders a compact ghost button with a `Wallet` icon and the formatted
  current balance. It keeps a stable width and tabular number layout.
- Small viewports retain the icon-only control. Its accessible name and tooltip
  identify it as the current balance and expose the formatted amount.
- The control uses the existing wallet `formatQuota` formatting path so it
  follows the configured display currency and precision rules.
- The button is part of `AppHeader`'s default right-side controls. It appears
  immediately after `ConfigDrawer` and immediately before `ProfileDropdown`.
- Headers that supply `rightContent` keep ownership of their explicit layout;
  this change does not inject content into custom header compositions.

## Data Flow

- The balance source is `getSelf()`, not the login-time auth-store value. This
  makes the displayed amount authoritative after quota changes.
- A dedicated query fetches the current user once the authenticated header is
  mounted, refetches on window focus, and refreshes every 60 seconds.
- Initial loading reserves the button's width with a skeleton. A failed initial
  read does not show a guessed or stale amount; the control remains available
  so the user can open the wallet page.
- The query is read-only and does not mutate the auth session or wallet page
  state.

## Accessibility And Navigation

- The element is a normal button with a clear localized accessible name that
  includes the current formatted balance when available.
- It supports keyboard focus and activation through the project's existing
  button component.
- Activation uses the TanStack router to navigate to `/wallet`.

## Validation

- A focused component test proves the initial loading state, formatted balance,
  fetch failure fallback, and wallet navigation.
- A browser acceptance pass verifies desktop placement between the theme and
  account controls, visible currency-formatted balance, mobile icon-only
  behavior, and navigation to `/wallet`.
