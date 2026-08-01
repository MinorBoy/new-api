# Seedance Video Generation Internal Page Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an authenticated native React video-generation workbench below Playground in the sidebar, using an existing API key selector, group-scoped model list, and multiple concurrent Seedance task cards.

**Architecture:** Add a focused `features/video-generation` module. Session-authenticated APIs load the user's masked keys and group models; the selected token's full key is fetched only in memory and sent to existing `/api/v3/contents/generations/tasks` endpoints via an explicit token-auth request option. The page owns a task collection so each submitted request can poll independently.

**Tech Stack:** React 19, TypeScript, TanStack Router/Query, Axios wrapper, Base UI/Tailwind, i18next, Vitest/Bun.

---

### Task 1: Define and test request/model helpers

**Files:**
- Create: `web/src/features/video-generation/types.ts`
- Create: `web/src/features/video-generation/lib/request.ts`
- Test: `web/src/features/video-generation/__tests__/request.test.ts`

- [ ] Write tests for Seedance default payload construction, URL media roles/count limits, and API-key model-limit filtering.
- [ ] Run `bun test src/features/video-generation/__tests__/request.test.ts`; verify it fails because the module does not exist.
- [ ] Implement typed request/task helpers with the supplied default prompt and media URLs.
- [ ] Run the focused test again and verify it passes.

### Task 2: Add explicit API-key authorization support

**Files:**
- Modify: `web/src/lib/http-client.ts`
- Test: `web/src/features/video-generation/__tests__/api.test.ts`

- [ ] Add a request config field that lets a feature set a token-auth bearer value and disables session refresh for that request.
- [ ] Test that task requests preserve the selected API key instead of replacing it with the dashboard session token.
- [ ] Run the focused test and typecheck.

### Task 3: Implement the workbench feature

**Files:**
- Create: `web/src/features/video-generation/api.ts`
- Create: `web/src/features/video-generation/index.tsx`
- Create: `web/src/features/video-generation/components/video-task-card.tsx`
- Create: `web/src/features/video-generation/__tests__/video-generation.test.tsx`

- [ ] Add API-key/model queries, in-memory full-key loading, request form, URL-only reference media editors, advanced controls, JSON preview, cURL copy, and independent task polling.
- [ ] Add tests for key selection loading models, creating multiple tasks, polling success/error, and disabled submit states.
- [ ] Run the focused component tests and correct failures.

### Task 4: Register route and sidebar navigation

**Files:**
- Create: `web/src/routes/_authenticated/video-generation/index.tsx`
- Modify: `web/src/hooks/use-sidebar-data.ts`
- Regenerate: `web/src/routeTree.gen.ts` through the router build/plugin.

- [ ] Add `/video-generation` under the authenticated layout and place a `Video` sidebar link immediately after Playground.
- [ ] Verify the sidebar active state and route guard through the route/typecheck build.

### Task 5: Add translations and verify delivery

**Files:**
- Modify through script only: `web/src/i18n/locales/*.json`
- Modify: `web/src/i18n/static-keys.ts` if required by extraction.

- [ ] Add all new labels, statuses, validation, and error copy with `web/scripts/add-missing-keys.mjs`.
- [ ] Run `bun run i18n:sync`, focused tests, lint, `bun run typecheck`, and `bun run build`.
- [ ] Run the page in a browser at desktop and mobile widths; verify sidebar navigation, key/model dependency, multiple tasks, no horizontal overflow, and no console errors.

