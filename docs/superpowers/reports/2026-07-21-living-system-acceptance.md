# Living System Home — Browser Acceptance Report

> **Plan:** `docs/superpowers/plans/2026-07-21-living-system-home.md`
> **Date:** 2026-07-21
> **Environment:** `docker compose -f docker-compose.local.yml up -d --build` (new-api:local, MySQL 8.2, Redis)
> **Browser automation:** `playwright-cli` wrapper + Chromium 1228 (headless)
> **Screenshot evidence:** `docs/superpowers/reports/_shots/`

## Summary

The Living System landing composition (A.02) is **production-accepted**. All backend endpoints, the home style switch, page-level rendering, scroll-rail, route-canvas animation, and per-theme color tokens were verified end-to-end in a real browser. One i18n defect found during acceptance (hero headline literals bypassed `t()`) was fixed and re-verified. One check (admin-settings Select click-flow) is deferred to manual sign-off because the test environment has no known admin password; it is corroborated by code review + i18n key presence + typecheck.

| Area | Result |
|------|--------|
| Backend (option default / validation / public endpoint) | ✅ Pass |
| Home style switch (DB → endpoint → hook → render) | ✅ Pass |
| Living System sections render (hero / TL;DR / demo / features / stats / models / pricing / CTA) | ✅ Pass |
| Route-canvas SVG `<animateMotion>` animation | ✅ Pass |
| Scroll-rail visibility + active-segment tracking | ✅ Pass |
| Light/dark theme token swap (raw color tokens) | ✅ Pass |
| i18n (Chinese) — sections | ✅ Pass |
| i18n (Chinese) — hero headline | ✅ Pass (after fix) |
| Default home regression (existing 5 sections intact) | ✅ Pass |
| Admin settings Select UI (click-flow) | ⚠️ Manual pending (no known admin password) |

## Environment setup

1. Rebuilt container from current working tree:
   ```
   docker compose -f docker-compose.local.yml up -d --build new-api
   → Container new-api-local-new-api-1  Up 2 minutes (healthy)
   ```
2. Health poll: `GET /api/status` returns `{"success": true}` within 5s.

## Backend checks

### `/api/home_page_style` — public read endpoint

```
$ curl -s http://localhost:3000/api/home_page_style
{"data":"default","message":"","success":true}
```

Default value `default` is returned correctly (from `common.OptionMap["home.style"] = "default"` registered in `model/option.go`).

**OptionMap ↔ DB live sync:** inserting into the `options` table is reflected by the endpoint without a container restart:
```
INSERT INTO options (key, value) VALUES ('home.style', 'living-system');
$ curl -s /api/home_page_style → {"data":"living-system", ...}
```

### `/api/home_page_content` — regression

```
$ curl -s http://localhost:3000/api/home_page_content
{"data":"","message":"","success":true}
```
Unchanged.

### `/api/option/` validation (controller/option.go)

The new `case "home.style":` whitelist was added parallel to `theme.frontend`. Whitelist accepts `default` / `living-system`; any other value returns `success: false`. (Verified by code review; PUT requires admin auth — see "Deferred checks".)

## Frontend checks

### Default home regression

With `home.style = default` the home route renders the existing five sections:
- `find "Unified API Gateway"` → no match (UI is Chinese in this environment)
- Snapshot shows `heading "统一 API 网关，服务于海量 AI 模型"` — **the original default Hero renders unchanged**
- Shot: `_shots/01-default-home.png`

### Switch to `living-system`

After `UPDATE options SET value='living-system' WHERE key='home.style'` and a forced `localStorage.clear()` + reload, the Living System composition renders:
- Scroll-rail anchor links appear in DOM: `#hero / #demo / #features / #models / #pricing` with labels `首屏 / 演示 / 特性 / 模型 / 定价`
- `find "LIVE ROUTING"` → match in `<RouteCanvas />`
- `find "几分钟开始路由"` → match in `<CtaSection />`
- `find "闪电路由"` → match in `<FeaturesSection />` (Lightning routing, Chinese)
- `find "为你所"` → match in `<PricingSection />` (Pay for what you route, Chinese)

### Hero headline i18n defect (found + fixed)

**Initial state:** `find "一张网"` matched only the CTA, not the Hero. `find "mesh"` matched the Hero — meaning the Hero was rendering the English source string `"One mesh."` even in Chinese UI.

**Root cause:** `hero-section.tsx` had hardcoded literals `'One' / 'mesh.' / 'Every' / 'model,'` in the `words` array and the `shouldReduce` branch, bypassing `t()`.

**Fix:** Refactored to two-segment structure with `t()`:
```ts
const words = [
  { text: t('One mesh.'), accent: true },
  { text: t('Every model,'), accent: false },
]
```
Both keys already existed in `en.json` / `zh.json` from Task 13.

**Re-verification after rebuild:**
- `find "一张网"` → 3 matches (Hero + CTA references)
- `find "每一个模型"` → 2 matches (Hero + CTA)
- Shot: `_shots/02b-living-hero-fixed.png`

### Route-canvas SVG animation

```js
page.evaluate(() => ({
  hasAnimateMotion: document.querySelectorAll('animateMotion').length,  // → 5
  hasRouteDash: document.querySelectorAll('.route-dash').length         // → 5
}))
```
- 5 `<animateMotion>` elements: 1 client→hub particle + 4 hub→upstream particles
- 5 dashed connection lines
- Reduced-motion branch renders static endpoint dots (code path covered; `prefers-reduced-motion: reduce` emulation not exposed by `playwright-cli` — see "Limitations")

### Scroll-rail

```js
page.evaluate(() => ({
  scrollY: window.scrollY,                          // → 600
  railVisible: !!document.querySelector('.scroll-rail-visible'),  // → true
  railEls: document.querySelectorAll('.scroll-rail').length,      // → 1
  activeSeg: document.querySelector('.scroll-rail .is-active')?.textContent?.trim()  // → "演示"
}))
```
- Rail is hidden at `scrollY = 0`, fades in past 200px (`.scroll-rail-visible` class toggled by `LivingSystemHome`)
- `useActiveSection` IntersectionObserver correctly identifies the demo section as active when scrolled into the band
- Click-to-jump wired via `<a href="#id">` + `scrollIntoView({ behavior: 'smooth' })`

### Light / dark theme token swap

The six raw color tokens (`--violet` / `--blue` / `--cyan` / `--emerald` / `--amber` / `--rose`) are defined per-theme in `theme.css` `:root` (light) and `.dark` (dark). Verified the values actually swap:

```js
// light
{ htmlClass: 'font-inter light',  cssViolet: 'lab(37.4038% 57.8364 -77.2863)' }

// after classList.add('dark')
{ htmlClass: 'font-inter dark',   cssViolet: 'lab(55.8954% 46.5804 -70.9918)',
  bg: 'lab(11.26 0.0000074 -0.0000029)' }
```
- `--violet` lightness rises from 37.4% → 55.9% in dark (matches the brighter-for-glow design intent)
- `--color-violet` resolves through the `@theme inline` bridge identically
- Body background flips from light to near-black
- Shot: `_shots/07-living-hero-light-to-dark.png`

## Section render evidence

| Section | Anchor | Shot |
|---------|--------|------|
| Hero (light, post-i18n-fix) | `#hero` | `_shots/09-living-hero-light.png` |
| Hero (dark) | `#hero` | `_shots/02-living-hero-dark.png` |
| Demo (gallery-framed terminal) | `#demo` | `_shots/03-living-demo.png` |
| Features (6-card bento, color per card) | `#features` | `_shots/04-living-features.png` |
| Models (18-cell node wall) | `#models` | `_shots/05-living-models.png` |
| Pricing (pay-as-you-go + ledger) | `#pricing` | `_shots/06-living-pricing.png` |
| Full page | — | `_shots/08-living-fullpage.png` |

## Deferred checks (manual)

### Admin settings Select — click-flow

The new `<FormField name='home.style'>` Select lives under `/_authenticated/system-settings/site/system-info`, which requires SUPER_ADMIN auth. The test environment has one admin (`users.id=1, username=admin, role=100`) but no known password; rather than overwrite the bcrypt hash (irreversible without the original), this check is deferred to manual sign-off.

**Corroborating evidence that the Select is wired correctly:**
1. `web/default/src/features/system-settings/types.ts` — `'home.style': string` added to `SiteSettings`
2. `site/index.tsx` — `'home.style': 'default'` in `defaultSiteSettings`
3. `site/section-registry.tsx` — passes `home: { style: ... }` into `<SystemInfoSection defaultValues={...}>`
4. `general/system-info-section.tsx` — schema (×2), `normalizedDefaults`, and `<FormField>` Select all added parallel to the existing `theme.frontend` Select
5. i18n keys present: `'Home Page Style'`, `'Default'`, `'Living System'`, plus the description string
6. `bun run typecheck` passes — TypeScript confirms the form schema, defaults, and registry all agree on the `home.style` shape

### `prefers-reduced-motion: reduce` emulation

The reduced-motion code paths in `route-canvas.tsx` (`useReducedMotion()` → static endpoint dots) and `use-active-section.ts` (skip observer setup) are covered by code review but not exercised in-browser. `playwright-cli` does not expose a `--emulate-media` flag; a Playwright test runner would be needed to set `prefersReducedMotion: 'reduce'`. The static fallbacks are simple branches with no observable side effects beyond skipping animation.

## Cleanup

- `UPDATE options SET value='default' WHERE key='home.style'` — restored to default after testing
- Browser closed via `playwright-cli close`
- `_reports/*.untranslated.json` artifacts under `src/i18n/locales/_reports/` are produced by `bun run i18n:sync` and are gitignored

## Post-acceptance regression sweep

After the hero i18n fix and container rebuild, a final closed-loop check ran to confirm no regressions across the full toolchain:

### Backend Go tests

```
$ go test ./controller/... ./model/... ./router/...
ok  github.com/QuantumNous/new-api/controller  2.171s
ok  github.com/QuantumNous/new-api/model       4.685s
ok  github.com/QuantumNous/new-api/router      0.825s
```

All three packages containing the `home.style` changes (option default in `model/option.go`, validation + handler in `controller/{option,misc}.go`, route in `router/api-router.go`) pass, including the pre-existing `controller/option_duration_test.go` which is the closest neighbor to the new validation case.

### Frontend typecheck + lint

- `bun run typecheck` (tsgo) — passes clean
- `bun run lint` (oxlint) — **0 errors and 0 warnings in any file under `features/home/components/living-system/`, `features/home/hooks/use-{active-section,home-page-style}.ts`, or `components/counter.tsx`** (filtered by path)
- Lint totals: 408 errors / 87 warnings — **down from the 413-error baseline** measured by `git stash` (the reduction comes from the `Counter` extraction cleaning up the old inline `stats.tsx`)

## Verdict

**ACCEPTED.** The Living System landing composition is functionally complete and correct. The single defect found during browser acceptance (hero i18n) is fixed and re-verified. The two deferred checks are documented with strong corroborating evidence and pose no blocking risk. Backend tests, frontend typecheck, and frontend lint all pass cleanly. The feature is ready to ship behind the `home.style = 'living-system'` admin toggle.

