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
import type { CostCatalogItem, CostCatalogPage } from '../../types'

const browserWindow = new Window({ url: 'http://localhost/' })
const mobileViewport = { current: false }
const matchMedia = (query: string) => ({
  matches: mobileViewport.current && query.includes('max-width: 640px'),
  media: query,
  onchange: null,
  addEventListener: () => {},
  removeEventListener: () => {},
  addListener: () => {},
  removeListener: () => {},
  dispatchEvent: () => true,
})
Object.defineProperty(browserWindow, 'matchMedia', { value: matchMedia })
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
  matchMedia,
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

beforeEach(() => {
  browserWindow.document.body.replaceChildren()
  mobileViewport.current = false
})

const { createRoot } = await import('react-dom/client')
const { TooltipProvider } = await import('@/components/ui/tooltip')
const { costAccountingQueryKeys } = await import('../../api')
const { costCatalogParamsFromSearch } = await import('../../lib/catalog')
const { SupplierCostCatalog } = await import('../supplier-cost-catalog')
const { SupplierCostCatalogMobile } =
  await import('../supplier-cost-catalog-mobile')
const i18n = createInstance()
await i18n.init({
  lng: 'en',
  fallbackLng: 'en',
  resources: { en: { translation: {} } },
})

const availableItem: CostCatalogItem = {
  rule_id: 11,
  channel_id: 7,
  channel_name: 'Primary supplier',
  channel_type: 1,
  channel_missing: false,
  billable_upstream_model: 'vendor-model',
  cost_variant_key: 'default',
  version: 1,
  status: 'active',
  cost_mode: 'per_request',
  schema_version: 1,
  currency: 'USD',
  prices: [
    {
      key: 'unit_price',
      unit: 'per_request',
      native_amount: '3',
      normalized_usd_amount: '3',
    },
  ],
  comparison_15s_equivalent_usd_per_second: '0.2',
  charge_event: 'task_succeeded',
  source: 'config_import',
  note: 'supplier contract',
  effective_from: 1_735_689_600,
  created_at: 1_735_689_500,
  updated_at: 1_735_689_600,
  price_status: 'available',
  issues: [],
}

const catalogPage: CostCatalogPage = {
  items: [availableItem],
  total: 1,
  page: 1,
  page_size: 50,
  summary: {
    channel_count: 1,
    active_rule_count: 1,
    draft_rule_count: 2,
    retired_rule_count: 3,
  },
  facets: {
    channels: [{ id: 7, name: 'Primary supplier', type: 1, missing: false }],
    currencies: ['USD'],
    sources: ['config_import'],
  },
}

async function mountCatalog(
  search: CostAccountingSearch,
  page: CostCatalogPage,
  onSearchChange: (next: CostAccountingSearch) => void = () => {}
): Promise<{ root: Root; container: { remove(): void }; client: QueryClient }> {
  const client = new QueryClient({
    defaultOptions: {
      queries: { retry: false, staleTime: Number.POSITIVE_INFINITY },
    },
  })
  client.setQueryData(
    costAccountingQueryKeys.catalog(costCatalogParamsFromSearch(search)),
    { success: true, message: '', data: page }
  )
  const container = browserWindow.document.createElement('div')
  browserWindow.document.body.append(container)
  const root = createRoot(container as unknown as Container)
  await act(async () => {
    root.render(
      <I18nextProvider i18n={i18n}>
        <QueryClientProvider client={client}>
          <TooltipProvider>
            <SupplierCostCatalog
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
  return { root, container, client }
}

async function unmountCatalog(mounted: {
  root: Root
  container: { remove(): void }
  client: QueryClient
}) {
  await act(async () => mounted.root.unmount())
  mounted.client.clear()
  mounted.container.remove()
}

test('renders summary, active rows, and pinned supplier identity columns', async () => {
  const mounted = await mountCatalog({ tab: 'catalog' }, catalogPage)
  try {
    const text = browserWindow.document.body.textContent ?? ''
    assert.match(text, /Channel count/)
    assert.match(text, /Primary supplier/)
    assert.match(text, /vendor-model/)
    assert.match(text, /USD 3/)
    const pinned = browserWindow.document.querySelector(
      '[data-column-id="channel_name"]'
    )
    assert.ok(pinned)
    assert.match(pinned.className, /sticky/)
  } finally {
    await unmountCatalog(mounted)
  }
})

test('renders unavailable supplier prices without a zero dollar fallback', async () => {
  const unavailable: CostCatalogItem = {
    ...availableItem,
    rule_id: 12,
    billable_upstream_model: 'unknown-price-model',
    prices: [],
    comparison_15s_equivalent_usd_per_second: undefined,
    price_status: 'unavailable',
    issues: ['missing_normalized_price'],
  }
  const mounted = await mountCatalog(
    { tab: 'catalog' },
    { ...catalogPage, items: [unavailable] }
  )
  try {
    const text = browserWindow.document.body.textContent ?? ''
    assert.match(text, /Unavailable/)
    assert.doesNotMatch(text, /\$0/)
  } finally {
    await unmountCatalog(mounted)
  }
})

test('expands mobile metadata independently from opening rule details', async () => {
  let opened = 0
  const container = browserWindow.document.createElement('div')
  browserWindow.document.body.append(container)
  const root = createRoot(container as unknown as Container)
  await act(async () => {
    root.render(
      <I18nextProvider i18n={i18n}>
        <SupplierCostCatalogMobile
          items={[availableItem]}
          onOpenRule={() => {
            opened++
          }}
        />
      </I18nextProvider>
    )
  })
  try {
    const expand = browserWindow.document.querySelector(
      'button[aria-label="Show supplier cost metadata"]'
    )
    assert.ok(expand instanceof browserWindow.HTMLButtonElement)
    await act(async () => expand.click())
    assert.equal(expand.getAttribute('aria-expanded'), 'true')
    assert.match(browserWindow.document.body.textContent ?? '', /config_import/)
    assert.equal(opened, 0)
  } finally {
    await act(async () => root.unmount())
    container.remove()
  }
})

test('uses page scrolling when stacked mobile catalog controls exceed the viewport', async () => {
  mobileViewport.current = true
  const mounted = await mountCatalog({ tab: 'catalog' }, catalogPage)
  try {
    const expand = browserWindow.document.querySelector(
      'button[aria-label="Show supplier cost metadata"]'
    )
    assert.ok(expand)
    let catalogPageRoot = expand.parentElement
    while (
      catalogPageRoot &&
      !catalogPageRoot.className.includes('flex h-full min-h-0 flex-col')
    ) {
      catalogPageRoot = catalogPageRoot.parentElement
    }
    assert.ok(catalogPageRoot)
    assert.match(catalogPageRoot.className, /max-sm:block/)
    assert.match(catalogPageRoot.className, /max-sm:overflow-y-auto/)
  } finally {
    await unmountCatalog(mounted)
  }
})
