/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import assert from 'node:assert/strict'
import test, { after, beforeEach } from 'node:test'

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { Window } from 'happy-dom'
import { createInstance } from 'i18next'
import { act } from 'react'
import type { Container, Root } from 'react-dom/client'
import { I18nextProvider } from 'react-i18next'

import type { CostAccountingSearch } from '../../lib/report'
import type { RouteMarginCatalogPage } from '../../types'

const browserWindow = new Window({ url: 'http://localhost/' })
const browserGlobals = {
  window: browserWindow,
  document: browserWindow.document,
  navigator: browserWindow.navigator,
  Element: browserWindow.Element,
  HTMLElement: browserWindow.HTMLElement,
  Node: browserWindow.Node,
  Event: browserWindow.Event,
  MouseEvent: browserWindow.MouseEvent,
  KeyboardEvent: browserWindow.KeyboardEvent,
  MutationObserver: browserWindow.MutationObserver,
  ResizeObserver: browserWindow.ResizeObserver,
  IntersectionObserver: browserWindow.IntersectionObserver,
  getComputedStyle: browserWindow.getComputedStyle.bind(browserWindow),
  requestAnimationFrame:
    browserWindow.requestAnimationFrame.bind(browserWindow),
  cancelAnimationFrame: browserWindow.cancelAnimationFrame.bind(browserWindow),
  IS_REACT_ACT_ENVIRONMENT: true,
}
const previousBrowserGlobals = Object.fromEntries(
  Object.keys(browserGlobals).map((key) => [
    key,
    Object.getOwnPropertyDescriptor(globalThis, key),
  ])
)
Object.assign(globalThis as Record<string, unknown>, browserGlobals)

after(() => {
  for (const key of Object.keys(browserGlobals)) {
    const previousDescriptor = previousBrowserGlobals[key]
    if (previousDescriptor === undefined) {
      delete (globalThis as Record<string, unknown>)[key]
      continue
    }
    Object.defineProperty(globalThis, key, previousDescriptor)
  }
  browserWindow.close()
})

beforeEach(() => browserWindow.document.body.replaceChildren())

const { createRoot } = await import('react-dom/client')
const { TooltipProvider } = await import('@/components/ui/tooltip')
const { api } = await import('@/lib/api')
const { costAccountingQueryKeys } = await import('../../api')
const { routeMarginParamsFromSearch } =
  await import('../../lib/route-margin-catalog')
const { RouteMarginCatalog } = await import('../route-margin-catalog')
const i18n = createInstance()
await i18n.init({
  lng: 'en',
  fallbackLng: 'en',
  resources: { en: { translation: {} } },
})

const page: RouteMarginCatalogPage = {
  items: [
    {
      target_id: 17,
      target_name: 'secure/vendor-model',
      policy_id: 4,
      group_name: 'default',
      canonical_model: 'doubao-seedance-2-0-260128',
      channel_id: 7,
      channel_name: 'Primary supplier',
      channel_type: 1,
      upstream_model: 'vendor-model',
      cost_variant_key: 'default',
      resolution: '720p',
      duration_seconds: 4,
      scenario: 'no_video',
      group_ratio: '1',
      cost_mode: 'per_request',
      rule_id: 11,
      rule_version: 1,
      estimated_revenue_nano_usd: 1_000_000_000,
      estimated_cost_nano_usd: 700_000_000,
      estimated_profit_nano_usd: 300_000_000,
      gross_margin_ppm: 300_000,
      requested_minimum_margin_ppm: 300_000,
      eligible: true,
      cost_source: 'config_import',
      revenue_source: 'runtime_billing_settings',
    },
  ],
  total: 1,
  page: 1,
  page_size: 50,
  summary: {
    target_count: 156,
    scenario_count: 312,
    eligible_target_count: 75,
    fully_eligible_target_count: 60,
    partially_eligible_target_count: 15,
    ineligible_target_count: 81,
    eligible_scenario_count: 135,
  },
  facets: {
    channels: [{ id: 7, name: 'Primary supplier', type: 1, missing: false }],
    resolutions: ['720p'],
    canonical_models: ['doubao-seedance-2-0-260128'],
  },
}

async function mountRouteMarginCatalog(
  search: CostAccountingSearch = { tab: 'route-margin' },
  onSearchChange: (next: CostAccountingSearch) => void = () => {},
  cachedPage: RouteMarginCatalogPage | null = page
): Promise<{ root: Root; container: HTMLElement; client: QueryClient }> {
  const client = new QueryClient({
    defaultOptions: {
      queries: { retry: false, staleTime: Number.POSITIVE_INFINITY },
    },
  })
  if (cachedPage) {
    client.setQueryData(
      costAccountingQueryKeys.routeMarginCatalog(
        routeMarginParamsFromSearch(search)
      ),
      { success: true, message: '', data: cachedPage }
    )
  }
  const container = browserWindow.document.createElement('div')
  browserWindow.document.body.append(container)
  const root = createRoot(container as unknown as Container)
  await act(async () => {
    root.render(
      <I18nextProvider i18n={i18n}>
        <QueryClientProvider client={client}>
          <TooltipProvider>
            <RouteMarginCatalog
              enabled
              search={search}
              onSearchChange={onSearchChange}
            />
          </TooltipProvider>
        </QueryClientProvider>
      </I18nextProvider>
    )
  })
  await act(async () => undefined)
  return { root, container: container as unknown as HTMLElement, client }
}

test('shows the default 30 percent route margin matrix', async () => {
  const mounted = await mountRouteMarginCatalog()
  try {
    const text = mounted.container.textContent ?? ''
    assert.match(text, /75/)
    assert.match(text, /60/)
    assert.match(text, /15/)
    assert.match(text, /Primary supplier/)
    assert.match(text, /30%/)
  } finally {
    await act(async () => mounted.root.unmount())
    mounted.client.clear()
    mounted.container.remove()
  }
})

test('reveals advanced filters and resets the page after duration changes', async () => {
  const updates: CostAccountingSearch[] = []
  const mounted = await mountRouteMarginCatalog(
    { tab: 'route-margin', marginPage: 4 },
    (next) => updates.push(next)
  )
  try {
    assert.equal(
      browserWindow.document.querySelector('#route-margin-duration'),
      null
    )
    const advancedButton = [
      ...browserWindow.document.querySelectorAll('button'),
    ].find((button) => button.textContent?.includes('Advanced mode'))
    assert.ok(advancedButton instanceof browserWindow.HTMLButtonElement)
    await act(async () => advancedButton.click())
    const duration = browserWindow.document.querySelector(
      '#route-margin-duration'
    )
    assert.ok(duration instanceof browserWindow.HTMLInputElement)
    await act(async () => {
      duration.focus()
      duration.value = '6'
      duration.blur()
    })
    assert.equal(updates.at(-1)?.marginDurationSeconds, 6)
    assert.equal(updates.at(-1)?.marginPage, 1)
  } finally {
    await act(async () => mounted.root.unmount())
    mounted.client.clear()
    mounted.container.remove()
  }
})

test('keeps the mobile route margin surface vertically scrollable', async () => {
  const mounted = await mountRouteMarginCatalog()
  try {
    const target = [...mounted.container.querySelectorAll('*')].find(
      (element) => element.textContent === 'secure/vendor-model'
    )
    assert.ok(target)
    let pageRoot = target.parentElement
    while (
      pageRoot &&
      !pageRoot.className.includes('flex h-full min-h-0 flex-col')
    ) {
      pageRoot = pageRoot.parentElement
    }
    assert.ok(pageRoot)
    assert.match(pageRoot.className, /max-sm:block/)
    assert.match(pageRoot.className, /max-sm:overflow-y-auto/)
  } finally {
    await act(async () => mounted.root.unmount())
    mounted.client.clear()
    mounted.container.remove()
  }
})

test('writes the eligible status filter to URL state and resets pagination', async () => {
  const updates: CostAccountingSearch[] = []
  const mounted = await mountRouteMarginCatalog(
    { tab: 'route-margin', marginPage: 3 },
    (next) => updates.push(next)
  )
  try {
    const statusTrigger = browserWindow.document.querySelector(
      'button[aria-label="Status"]'
    )
    assert.ok(statusTrigger instanceof browserWindow.HTMLButtonElement)
    await act(async () => statusTrigger.click())
    const eligibleOption = [
      ...browserWindow.document.querySelectorAll('[role="option"]'),
    ].find((option) => option.textContent?.trim() === 'Eligible')
    assert.ok(eligibleOption instanceof browserWindow.HTMLElement)
    await act(async () => eligibleOption.click())
    assert.equal(updates.at(-1)?.marginStatus, 'eligible')
    assert.equal(updates.at(-1)?.marginPage, 1)
  } finally {
    await act(async () => mounted.root.unmount())
    mounted.client.clear()
    mounted.container.remove()
  }
})

test('exports the current route margin filters without pagination', async () => {
  const originalGet = api.get
  const originalAnchorClick = browserWindow.HTMLAnchorElement.prototype.click
  const requests: Array<{ url: string; params: Record<string, unknown> }> = []
  api.get = (async (
    url: string,
    config?: { params?: Record<string, unknown> }
  ) => {
    requests.push({ url, params: config?.params ?? {} })
    return {
      data: new Blob(['csv']),
      headers: {
        'content-disposition': 'attachment; filename="route-margin.csv"',
        'x-exported-row-count': '1',
      },
    }
  }) as typeof api.get
  browserWindow.HTMLAnchorElement.prototype.click = () => {}
  const mounted = await mountRouteMarginCatalog({
    tab: 'route-margin',
    marginStatus: 'eligible',
    marginPage: 3,
  })
  try {
    const exportButton = [...mounted.container.querySelectorAll('button')].find(
      (button) => button.textContent?.includes('Export current results')
    )
    assert.ok(exportButton instanceof browserWindow.HTMLButtonElement)
    await act(async () =>
      (exportButton as unknown as { click(): void }).click()
    )
    await act(async () => undefined)
    assert.equal(
      requests.at(-1)?.url,
      '/api/cost-accounting/route-margin-catalog/export'
    )
    assert.equal(requests.at(-1)?.params.status, 'eligible')
    assert.equal('page' in (requests.at(-1)?.params ?? {}), false)
    assert.equal('page_size' in (requests.at(-1)?.params ?? {}), false)
  } finally {
    api.get = originalGet
    browserWindow.HTMLAnchorElement.prototype.click = originalAnchorClick
    await act(async () => mounted.root.unmount())
    mounted.client.clear()
    mounted.container.remove()
  }
})

test('shows a retry action after the route margin query fails', async () => {
  const originalGet = api.get
  let requestCount = 0
  api.get = (async () => {
    requestCount++
    throw new Error('catalog unavailable')
  }) as typeof api.get
  const mounted = await mountRouteMarginCatalog(
    { tab: 'route-margin' },
    () => {},
    null
  )
  try {
    await act(async () => undefined)
    const retryButton = [...mounted.container.querySelectorAll('button')].find(
      (button) => button.textContent?.includes('Retry')
    )
    assert.ok(retryButton instanceof browserWindow.HTMLButtonElement)
    assert.match(mounted.container.textContent ?? '', /catalog unavailable/)
    await act(async () => (retryButton as unknown as { click(): void }).click())
    await act(async () => undefined)
    assert.equal(requestCount, 2)
  } finally {
    api.get = originalGet
    await act(async () => mounted.root.unmount())
    mounted.client.clear()
    mounted.container.remove()
  }
})
