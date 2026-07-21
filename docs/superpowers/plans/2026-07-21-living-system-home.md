# Living System Home Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the "Living System" landing page design (A.02 from `docs/frontend/landing-ysrouter-2026.html`) as an administrator-selectable home page style, alongside the existing default home — without modifying any of the five existing sections.

**Architecture:** Introduce a new admin option `home.style` (`'default' | 'living-system'`) persisted in the `Option` key-value table, exposed to anonymous visitors via a new public endpoint `GET /api/home_page_style` (mirroring the existing `home_page_content` mechanism). On the frontend, build a self-contained `features/home/components/living-system/` component tree that the `Home` entry renders conditionally. Colors are added as raw `--color-violet/blue/cyan/emerald/amber/rose` tokens in `theme.css` (Tailwind auto-generates utilities); the route-canvas animation uses SVG `<animateMotion>` (project already uses this pattern in `components/ai-elements/edge.tsx`).

**Tech Stack:** Go (gin/gorm), React 19, TypeScript, TanStack Router/Query, motion v12, Tailwind CSS v4, i18next, @fontsource fonts, shadcn base-nova.

**Design source:** `docs/frontend/landing-ysrouter-2026.html` (Webflow 2026 trends: 6-color explosion, LIVE ROUTING animation, scroll-rail guided scrolling, dot-grid infinite canvas, staggered word entry).

---

## Background & constraints

- The `Home` component (`web/default/src/features/home/index.tsx`) currently renders five sections when no admin-configured custom content exists: `Hero / Stats / Features / HowItWorks / CTA`. Custom content (URL / HTML / markdown from `HomePageContent` option) always takes precedence.
- An existing precedent for an admin "style" switch is `theme.frontend` (`'default' | 'classic'`): default registered in `model/option.go`, validated in `controller/option.go`, surfaced in `features/system-settings/general/system-info-section.tsx`.
- An existing precedent for a public option read is `GetHomePageContent` (`controller/misc.go:225`) + `GET /api/home_page_content` (`router/api-router.go:33`) + `useHomePageContent` hook with localStorage cache.
- The existing `Counter` component (`features/home/components/sections/stats.tsx:30-84`) implements IntersectionObserver-triggered count-up and should be reused (extracted to a shared component).
- The project's existing motion infrastructure (`lib/motion.ts`, `components/page-transition.tsx`, `components/animate-in-view.tsx`) is the established pattern for staggered/in-view animations; new components should consume these rather than reinvent.

**Non-goals:**
- Do NOT modify the five existing default sections (they must remain byte-for-byte intact).
- Do NOT modify `constants.ts` exports (Living System inlines its own data).
- Do NOT delete the seven orphaned components (`gateway-card.tsx`, `hero-buttons.tsx`, etc.) — out of scope.
- Do NOT touch `routes/index.tsx` (Home stays mounted at `/`).
- Do NOT use Google Fonts CDN — only `@fontsource*` local packages (project policy, see `AGENTS.md`).
- Do NOT introduce a sixth theme preset — colors are added as raw tokens, not a preset.

---

### Task 1: Backend — register `home.style` option default

**Files:**
- Modify: `model/option.go`

- [x] **Step 1: Add default value in `InitOptionMap`**

In `model/option.go`, immediately after the existing `common.OptionMap["HomePageContent"] = ""` line (around line 71), add:

```go
common.OptionMap["HomePageContent"] = ""
// home.style selects which landing page composition the public home
// route renders. Mirrors the OptionMap read pattern used by
// HomePageContent; consumed by controller.GetHomePageStyle.
common.OptionMap["home.style"] = "default"
```

**Why here:** Co-located with `HomePageContent` because both are public home-page concerns read through parallel mechanisms.

**Verify:** `go build ./...` succeeds.

---

### Task 2: Backend — validate `home.style` on update

**Files:**
- Modify: `controller/option.go`

- [ ] **Step 1: Add whitelist case in `UpdateOption` switch**

In `controller/option.go`, find the `case "theme.frontend":` block (around line 227) and add a parallel case immediately after it:

```go
case "home.style":
    if option.Value != "default" && option.Value != "living-system" {
        c.JSON(http.StatusOK, gin.H{
            "success": false,
            "message": "无效的首页风格，可选值：default（默认首页）、living-system（活力系统首页）",
        })
        return
    }
```

**Verify:** Manually PUT an invalid value (`{"key":"home.style","value":"bogus"}`) and confirm the 200/false response.

---

### Task 3: Backend — public read endpoint `GetHomePageStyle`

**Files:**
- Modify: `controller/misc.go`
- Modify: `router/api-router.go`

- [ ] **Step 1: Add handler in `controller/misc.go`**

Immediately after `GetHomePageContent` (around line 234), add a parallel handler:

```go
// GetHomePageStyle returns the admin-configured home page style
// ("default" | "living-system"). Public endpoint — consumed by
// anonymous visitors on the landing route, mirroring GetHomePageContent.
func GetHomePageStyle(c *gin.Context) {
    common.OptionMapRWMutex.RLock()
    defer common.OptionMapRWMutex.RUnlock()
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "",
        "data":    common.OptionMap["home.style"],
    })
    return
}
```

- [ ] **Step 2: Register route in `router/api-router.go`**

Find the existing `apiRouter.GET("/home_page_content", controller.GetHomePageContent)` line (around line 33) and add immediately after:

```go
apiRouter.GET("/home_page_style", controller.GetHomePageStyle)
```

**Verify:** `go build ./...` succeeds; start the server and `curl /api/home_page_style` returns `{"success":true,"data":"default"}`.

---

### Task 4: Fonts — install @fontsource packages

**Files:**
- Modify: `web/default/package.json` (via `bun add`)

- [ ] **Step 1: Install three font packages**

From the `web/default/` directory:

```bash
bun add @fontsource-variable/geist @fontsource-variable/geist-mono @fontsource/instrument-serif
```

**Note:** `@fontsource/instrument-serif` is a static (non-variable) package — Instrument Serif only ships regular + italic. Use the non-`-variable` package name.

**Verify:** `package.json` lists all three; `bun install` succeeds; no lock-file warnings.

---

### Task 5: Fonts & colors — wire CSS tokens

**Files:**
- Modify: `web/default/src/styles/index.css`
- Modify: `web/default/src/styles/theme.css`

- [ ] **Step 1: Add font `@import`s to `index.css`**

At the top of `web/default/src/styles/index.css`, immediately after the existing `@import '@fontsource-variable/lora';` line (around line 28), add:

```css
@import '@fontsource-variable/geist';
@import '@fontsource-variable/geist-mono';
@import '@fontsource/instrument-serif';
```

- [ ] **Step 2: Add font + raw-color tokens to `theme.css` `@theme inline` block**

In `web/default/src/styles/theme.css`, inside the `@theme inline { ... }` block (after the existing `--font-serif`/`--font-inter`/`--font-manrope` declarations, around lines 22-41), add font tokens and six raw color tokens:

```css
/* Geist sans + mono for the Living System landing composition.
 * Registered as raw font tokens (not swapped into --font-sans/--font-mono)
 * so the rest of the app keeps using Public Sans / default mono. */
--font-geist:
  'Geist Variable', -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
--font-geist-mono:
  'Geist Mono Variable', ui-monospace, SFMono-Regular, monospace;
/* Instrument Serif — italic-only display face for headline accents. */
--font-display: 'Instrument Serif', Georgia, serif;

/* Raw color palette for the Living System landing ("explosion of color").
 * These are intentionally NOT semantic tokens — they are local to the
 * landing composition and exposed as bg-violet / text-cyan / border-amber
 * etc. via Tailwind utilities. Values are defined per-theme in :root /
 * .dark below. */
--color-violet: var(--violet);
--color-blue: var(--blue);
--color-cyan: var(--cyan);
--color-emerald: var(--emerald);
--color-amber: var(--amber);
--color-rose: var(--rose);
```

- [ ] **Step 3: Define raw color values in `:root` (light)**

In the `:root { ... }` block (around lines 96-175), add six raw color variables with light-mode-appropriate OKLCH values (deeper saturation for contrast on light backgrounds):

```css
/* Living System 6-color palette — light values (deeper for contrast). */
--violet: oklch(0.5 0.24 295);
--blue: oklch(0.5 0.21 255);
--cyan: oklch(0.5 0.13 205);
--emerald: oklch(0.5 0.13 160);
--amber: oklch(0.65 0.17 65);
--rose: oklch(0.55 0.22 15);
```

- [ ] **Step 4: Define raw color values in `.dark` (dark)**

In the `.dark { ... }` block (around lines 177-239), add the dark-mode counterparts (brighter, matching the original design `#8b5cf6` etc.):

```css
/* Living System 6-color palette — dark values (brighter for glow). */
--violet: oklch(0.65 0.22 295);
--blue: oklch(0.62 0.19 255);
--cyan: oklch(0.7 0.14 205);
--emerald: oklch(0.7 0.15 160);
--amber: oklch(0.78 0.16 65);
--rose: oklch(0.65 0.22 15);
```

**Verify:** `bun run build` succeeds; in DevTools, `getComputedStyle(document.documentElement).getPropertyValue('--color-violet')` returns a non-empty value; `bg-violet` utility applies.

---

### Task 6: Admin UI — surface `home.style` in system settings

**Files:**
- Modify: `web/default/src/features/system-settings/site/index.tsx`
- Modify: `web/default/src/features/system-settings/general/system-info-section.tsx`
- Modify: `web/default/src/features/system-settings/hooks/use-update-option.ts`

- [ ] **Step 1: Add default value to `defaultSiteSettings`**

In `web/default/src/features/system-settings/site/index.tsx`, in the `defaultSiteSettings` object (which already declares `'theme.frontend': 'default'` and `HomePageContent: ''`), add:

```ts
'home.style': 'default' as const,
```

- [ ] **Step 2: Add a Select field in the system-info section**

In `web/default/src/features/system-settings/general/system-info-section.tsx`, find the existing `<FormField>` for `theme.frontend` (a `<Select>` with options `default` / `classic`). Immediately after it, add a parallel `<FormField>` for `home.style` with options `default` (label: `Default`) and `living-system` (label: `Living System`). Use the i18n keys `'Default'` and `'Living System'` (new keys, see Task 10).

**Label text:** `Home page style` (i18n key: `'Home page style'`).

- [ ] **Step 3: Add `'home.style'` to `STATUS_RELATED_KEYS`**

In `web/default/src/features/system-settings/hooks/use-update-option.ts`, find the `STATUS_RELATED_KEYS` array (which already includes `'theme.frontend'` and `HomePageContent`-related keys) and add `'home.style'`. This ensures the anonymous-visible config refreshes when an admin changes the style.

**Verify:** Visit `/system-settings/site/system-info` (or wherever the section is registered) as SUPER_ADMIN; the new Select appears; changing it fires `PUT /api/option/` with `{key:'home.style', value:'living-system'}`; the value persists across reload.

---

### Task 7: Home — option read API + hook

**Files:**
- Modify: `web/default/src/features/home/api.ts`
- Create: `web/default/src/features/home/hooks/use-home-page-style.ts`

- [ ] **Step 1: Add `getHomePageStyle` to `api.ts`**

In `web/default/src/features/home/api.ts`, immediately after the existing `getHomePageContent` function, add a parallel function:

```ts
export async function getHomePageStyle(): Promise<string> {
  const res = await api.get<ApiResponse<string>>('/api/home_page_style')
  return res.data.data ?? 'default'
}
```

(Match the exact `api.get` typing pattern used by `getHomePageContent` — verify before writing.)

- [ ] **Step 2: Create `useHomePageStyle` hook**

Create `web/default/src/features/home/hooks/use-home-page-style.ts` mirroring `use-home-page-content.ts`:

```ts
import { useEffect, useState } from 'react'

import { getHomePageStyle } from '../api'

const STORAGE_KEY = 'home_page_style'
const VALID = ['default', 'living-system'] as const
type Style = (typeof VALID)[number]

function readCache(): Style {
  try {
    const v = localStorage.getItem(STORAGE_KEY)
    if (v === 'living-system') return 'living-system'
  } catch {
    // ignore
  }
  return 'default'
}

export function useHomePageStyle() {
  const [style, setStyle] = useState<Style>(readCache)
  const [loaded, setLoaded] = useState(false)

  useEffect(() => {
    let cancelled = false
    getHomePageStyle()
      .then((v) => {
        if (cancelled) return
        const next: Style = v === 'living-system' ? 'living-system' : 'default'
        setStyle(next)
        setLoaded(true)
        try {
          localStorage.setItem(STORAGE_KEY, next)
        } catch {
          // ignore
        }
      })
      .catch(() => {
        if (cancelled) return
        setLoaded(true)
      })
    return () => {
      cancelled = true
    }
  }, [])

  return { style, loaded }
}
```

- [ ] **Step 3: Export from hooks barrel**

In `web/default/src/features/home/hooks/index.ts`, add `export { useHomePageStyle } from './use-home-page-style'`.

**Verify:** `bun run typecheck` passes; calling the hook in isolation returns `{ style: 'default', loaded: true }` against the dev server.

---

### Task 8: Shared `Counter` extraction

**Files:**
- Create: `web/default/src/components/counter.tsx`
- Modify: `web/default/src/features/home/components/sections/stats.tsx`

- [ ] **Step 1: Extract `Counter` to a shared component**

Create `web/default/src/components/counter.tsx` containing the `Counter` component currently inline in `stats.tsx:30-84` (same props, same IntersectionObserver + rAF logic, same reduced-motion handling). Export it as a named export. Add the AGENTS.md copyright header.

- [ ] **Step 2: Refactor `stats.tsx` to import the shared `Counter`**

In `web/default/src/features/home/components/sections/stats.tsx`:
- Remove the local `Counter` function (lines ~30-84).
- Add `import { Counter } from '@/components/counter'` at the top.
- Verify no other changes to behavior.

**Verify:** `bun run typecheck` passes; the default home `/` still shows animated stats identically.

---

### Task 9: Living System — supporting primitives

**Files:**
- Create: `web/default/src/features/home/components/living-system/route-canvas.tsx`
- Create: `web/default/src/features/home/components/living-system/scroll-rail.tsx`
- Create: `web/default/src/features/home/hooks/use-active-section.ts`

- [ ] **Step 1: Create `route-canvas.tsx`**

SVG component rendering the LIVE ROUTING diagram: a left "client" node, a central "YSRouter" hub (violet circle), and four right-side upstream nodes (OpenAI / Claude / Gemini / Mistral). Use `<animateMotion path="..." dur="2s" repeatCount="indefinite" />` for the four upstream particles and one client→hub particle (precedent: `components/ai-elements/edge.tsx:150`).

```tsx
import { useReducedMotion } from 'motion/react'
// ... see design source: docs/frontend/landing-ysrouter-2026.html
//       `.route-canvas` / `.route-svg` / `<animateMotion>` blocks
```

Honor `useReducedMotion()`: when reduced, render the particles as static circles at their path endpoints instead of `<animateMotion>`.

- [ ] **Step 2: Create `useActiveSection` hook**

Create `web/default/src/features/home/hooks/use-active-section.ts`:

```ts
import { useEffect, useState } from 'react'

export function useActiveSection(ids: string[]): string {
  const [active, setActive] = useState(ids[0] ?? '')
  useEffect(() => {
    const mq = window.matchMedia('(prefers-reduced-motion: reduce)')
    if (mq.matches) return // skip tracking when reduced motion
    const observer = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          if (entry.isIntersecting) setActive(entry.target.id)
        }
      },
      { rootMargin: '-40% 0px -55% 0px', threshold: 0 }
    )
    for (const id of ids) {
      const el = document.getElementById(id)
      if (el) observer.observe(el)
    }
    return () => observer.disconnect()
  }, [ids])
  return active
}
```

- [ ] **Step 3: Create `scroll-rail.tsx`**

Fixed left-side vertical rail of segments (one per section). Consumes `useActiveSection(ids)` to highlight the current segment, with clickable jump (`scrollIntoView({ behavior: 'smooth' })`). Hidden below `lg` breakpoint. Follow the design source's `.scroll-rail` styles.

**Verify:** `bun run typecheck` passes; each component renders in isolation.

---

### Task 10: Living System — section components

**Files (all under `web/default/src/features/home/components/living-system/sections/`):**
- Create: `hero-section.tsx`
- Create: `tldr-section.tsx`
- Create: `demo-section.tsx`
- Create: `features-section.tsx`
- Create: `stats-section.tsx`
- Create: `models-section.tsx`
- Create: `pricing-section.tsx`
- Create: `cta-section.tsx`

Each section file follows the same structural pattern as the existing `features/home/components/sections/*.tsx`:
- AGENTS.md copyright header
- `useTranslation` for user-facing copy
- Consumes shared primitives (`Counter`, `AnimateInView`, motion variants)
- Props: `{ isAuthenticated?: boolean }` for `hero-section` and `cta-section` only

- [ ] **Step 1: `hero-section.tsx`**

Headline "One mesh. Every model," with staggered word entry using `MOTION_VARIANTS.pageEnter` + `STAGGER_VARIANTS` from `lib/motion.ts` (each word wrapped in `motion.span` with 0.12s stagger). `useReducedMotion()` → render statically. Includes the badge pill, dual CTAs (login-aware: "Go to Dashboard" → `/dashboard` when authenticated, else "Start routing" → `/sign-up` + "Read the docs" → docs link), and `<RouteCanvas />` below.

- [ ] **Step 2: `tldr-section.tsx`**

TL;DR card with four numbered items (`01 Drop-in base URL` / `02 50+ upstreams` / `03 Pay per token` / `04 Self-host free`). See design `.tldr` markup.

- [ ] **Step 3: `demo-section.tsx`**

Gallery-framed terminal demo. **Reuse** the rendering logic from the existing `features/home/components/hero-terminal-demo.tsx` — either import it directly or extract shared terminal primitives. Wrap in the `.gallery-frame` double-border aesthetic. Caption: `FIG. 01 — A SINGLE REQUEST, FULLY INSTRUMENTED`.

- [ ] **Step 4: `features-section.tsx`**

Six-card bento grid. Each card uses an inline `style={{ '--card-color': 'var(--color-violet)' }}` to drive its icon background / hover border / accent. Cards: Lightning routing / Secure by default / Global coverage / Transparent billing / Developer-first / Self-host free. Lucide icons: `Zap, ShieldCheck, Globe, DollarSign, Code2, Server`.

- [ ] **Step 5: `stats-section.tsx`**

Four-column grid with `<Counter>` (shared from Task 8) per cell. Data: 50+ upstream services / 100+ model billing / 50+ compatible routes / 10+ scheduling controls. Each cell has a colored top accent using one of the six raw colors.

- [ ] **Step 6: `models-section.tsx`**

6-column grid of 18 cells (17 providers + one dashed "+30 more"). Each cell: italic serif glyph (Instrument Serif, gradient text), provider name (mono), green status dot. Providers: OpenAI, Anthropic, Google Gemini, Mistral, DeepSeek, xAI, Cohere, Moonshot, Qwen, Zhipu, Doubao, Yi, Baichuan, Minimax, Spark, Azure, Bedrock.

- [ ] **Step 7: `pricing-section.tsx`**

Pay-as-you-go layout (NO subscription tiers). Two-column: main card (purple radial glow, `$0.0021 / 1K tokens`, "no markup" copy, CTA "Top up & start routing") + side ledger card (mono usage rows + `balance after $80.10` + footer "no markup · no seat fees · no commitment"). Below: three feature mini-cards (Pre-pay / Pass-through / Self-host).

- [ ] **Step 8: `cta-section.tsx`**

Final CTA card with radial glow. Headline "Start routing in minutes." If `isAuthenticated`, **`return null`** (matching existing `cta.tsx` behavior).

**Verify:** `bun run typecheck` passes; each section imports cleanly with no missing references.

---

### Task 11: Living System — top-level composition

**Files:**
- Create: `web/default/src/features/home/components/living-system/index.tsx`
- Modify: `web/default/src/features/home/components/index.ts`

- [ ] **Step 1: Create `LivingSystemHome` composition**

Create `web/default/src/features/home/components/living-system/index.tsx` exporting `LivingSystemHome({ isAuthenticated }: { isAuthenticated?: boolean })`. It composes:

```tsx
<PublicLayout showMainContainer={false}>
  <ScrollRail sections={[...]} />
  <HeroSection isAuthenticated={isAuthenticated} />
  <TldrSection />
  <DemoSection />
  <FeaturesSection />
  <StatsSection />
  <ModelsSection />
  <PricingSection />
  <CtaSection isAuthenticated={isAuthenticated} />
  <Footer />
</PublicLayout>
```

Each section gets an `id` attribute matching the scroll-rail segment (`hero` / `demo` / `features` / `models` / `pricing`).

- [ ] **Step 2: Export from `components/index.ts` barrel**

In `web/default/src/features/home/components/index.ts`, add:

```ts
export { LivingSystemHome } from './living-system'
```

**Verify:** `bun run typecheck` passes; importing `LivingSystemHome` from `@/features/home/components` resolves.

---

### Task 12: Home entry — switch on `home.style`

**Files:**
- Modify: `web/default/src/features/home/index.tsx`

- [ ] **Step 1: Read the style and branch in `Home`**

In `web/default/src/features/home/index.tsx`:
- Import `useHomePageStyle` and `LivingSystemHome`.
- Call `const { style } = useHomePageStyle()` alongside the existing `useHomePageContent()`.
- In the default-home branch (currently lines 123-132), branch on style:

```tsx
if (style === 'living-system') {
  return <LivingSystemHome isAuthenticated={isAuthenticated} />
}

return (
  <PublicLayout showMainContainer={false}>
    <Hero isAuthenticated={isAuthenticated} />
    <Stats />
    <Features />
    <HowItWorks />
    <CTA isAuthenticated={isAuthenticated} />
    <Footer />
  </PublicLayout>
)
```

**Critical:** The custom-content branches (URL iframe / HTML / markdown from `useHomePageContent`) remain unchanged and take precedence over both styles — they come earlier in the function and `return` first.

**Verify:** With `home.style = 'default'` (the default), `/` renders the existing five sections identically. With `home.style = 'living-system'`, `/` renders the new composition.

---

### Task 13: i18n — English + Chinese keys

**Files:**
- Modify: `web/default/src/i18n/locales/en.json`
- Modify: `web/default/src/i18n/locales/zh.json`

- [ ] **Step 1: Add all new English keys**

Add every new English-source-string key used by the Living System sections to `en.json` (flat keys, English values — i18next convention). Examples:
- `'One mesh.'`, `'Every model,'`
- `'Fifty upstream AI services. One OpenAI-compatible URL. Zero client changes.'`
- `'Start routing'`, `'Watch it route'`, `'Read the docs'`
- `'Drop-in base URL'`, `'Pay per token'`, `'Self-host free'`
- `'Home page style'`, `'Living System'`, `'Default'`
- Section eyebrows: `'live demo'`, `'capabilities'`, `'upstream index'`, `'pricing'`
- Feature titles, model labels, ledger strings, etc.

- [ ] **Step 2: Add Chinese translations**

Add the same keys to `zh.json` with Chinese values. Examples:
- `'One mesh.'` → `'一个网格。'`
- `'Every model,'` → `'每一个模型，'`
- `'Start routing'` → `'开始路由'`
- `'Home page style'` → `'首页风格'`
- `'Living System'` → `'活力系统'`

- [ ] **Step 3: Sync remaining locales**

From `web/default/`, run `bun run i18n:sync` to backfill fr/ja/ru/vi/zh-TW with the English source as placeholder (per project convention).

**Verify:** `bun run i18n:sync` exits 0; no missing-key warnings in console when browsing `/` in any language.

---

### Task 14: Verification

- [ ] **Step 1: Type check**

From `web/default/`: `bun run typecheck` (tsgo) — must exit 0.

- [ ] **Step 2: Lint**

`bun run lint` (oxlint) — must exit 0.

- [ ] **Step 3: Build**

`bun run build` — must succeed with no errors. Check that the three new fonts are bundled (search the output for `geist` / `instrument-serif`).

- [ ] **Step 4: Manual browser test**

Start the dev server and verify:
1. `/` with `home.style = 'default'` → identical to current behavior (existing five sections).
2. As SUPER_ADMIN, set `home.style = 'living-system'` in system settings → reload `/` → Living System renders.
3. Toggle light/dark theme → all six raw colors update correctly.
4. Scroll the page → scroll-rail highlights the current section; clicking a segment jumps to it.
5. Route-canvas particles animate; with `prefers-reduced-motion: reduce` (DevTools → Rendering), they render static.
6. Stats counters animate on scroll into view; reduced-motion shows final values.
7. Mobile viewport (375px) → scroll-rail hidden, sections stack, no horizontal overflow.
8. Logged-in user → hero CTA is "Go to Dashboard"; CTA section is absent.
9. Custom-content branch still takes precedence (set `HomePageContent` to a URL → that wins regardless of `home.style`).
10. Run `bun run i18n:sync` once more — should be a no-op (all keys present).

---

## Risks & mitigations

| Risk | Mitigation |
|------|-----------|
| New fonts bloat bundle | Variable fonts are subset by @fontsource; verify bundle size delta < 200KB |
| SVG `<animateMotion>` SSR issues | Project is SPA (no SSR) — non-issue |
| Raw color tokens "leak" into other features | Tokens are only consumed by Living System components; not registered as semantic tokens, so shadcn components are unaffected |
| `useHomePageStyle` fires extra request per visit | Mirrors existing `useHomePageContent` pattern with localStorage cache — same trade-off the project already accepts |
| Admin changes style but visitor sees stale | Added `'home.style'` to `STATUS_RELATED_KEYS` so status refreshes; localStorage cache updates on next load |

---

## Out of scope (explicit)

- Migrating the other four design alternatives (Switching Yard / Gradient Tech / Editorial / Atelier) to React
- Deleting orphaned home components (`gateway-card.tsx`, etc.)
- Modifying the existing five default sections
- Adding a sixth theme preset (raw tokens are intentional, not a preset)
- Backend tests for the new option (no existing option has dedicated tests — out of scope, follow existing precedent)
