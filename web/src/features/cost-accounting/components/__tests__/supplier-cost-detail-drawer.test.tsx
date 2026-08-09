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

import type { CostCatalogDetail, CostCatalogHistoryEntry } from '../../types'

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
    const previous = previousBrowserGlobals[key]
    if (previous === undefined) {
      delete (globalThis as Record<string, unknown>)[key]
    } else {
      Object.defineProperty(globalThis, key, previous)
    }
  }
  browserWindow.close()
})

beforeEach(() => browserWindow.document.body.replaceChildren())

const { createRoot } = await import('react-dom/client')
const { costAccountingQueryKeys } = await import('../../api')
const { SupplierCostDetailDrawer } =
  await import('../supplier-cost-detail-drawer')
const i18n = createInstance()
await i18n.init({
  lng: 'en',
  fallbackLng: 'en',
  resources: { en: { translation: {} } },
})

const activeRule: CostCatalogHistoryEntry = {
  rule_id: 11,
  channel_id: 7,
  channel_name: 'Primary supplier',
  channel_type: 1,
  channel_missing: false,
  billable_upstream_model: 'vendor-model',
  cost_variant_key: 'default',
  version: 2,
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
  note: 'contract note',
  effective_from: 1_735_689_600,
  created_at: 1_735_689_500,
  updated_at: 1_735_689_600,
  price_status: 'available',
  issues: [],
  created_by: 1,
  activated_by: 1,
}

async function mountDetail(detail: CostCatalogDetail) {
  const client = new QueryClient({
    defaultOptions: {
      queries: { retry: false, staleTime: Number.POSITIVE_INFINITY },
    },
  })
  client.setQueryData(costAccountingQueryKeys.catalogDetail(11), {
    success: true,
    message: '',
    data: detail,
  })
  const container = browserWindow.document.createElement('div')
  browserWindow.document.body.append(container)
  const root = createRoot(container as unknown as Container)
  await act(async () => {
    root.render(
      <I18nextProvider i18n={i18n}>
        <QueryClientProvider client={client}>
          <SupplierCostDetailDrawer ruleId={11} onOpenChange={() => {}} />
        </QueryClientProvider>
      </I18nextProvider>
    )
  })
  await act(async () => undefined)
  return { client, container, root }
}

async function unmountDetail(mounted: {
  client: QueryClient
  container: { remove(): void }
  root: Root
}) {
  await act(async () => mounted.root.unmount())
  mounted.client.clear()
  mounted.container.remove()
}

test('shows structured prices, conversion parameters, and newest-first history', async () => {
  const detail: CostCatalogDetail = {
    rule: activeRule,
    config: {
      currency: 'USD',
      billing_multiplier: '1.2',
      purchase_discount_ratio: '0.8',
      recharge_exchange_ratio: '1',
      fee_rate: '0.05',
      currency_to_usd_rate: '1',
      unit_price: '3',
      normalized_usd_prices: { unit_price: '3' },
    },
    history: [
      activeRule,
      { ...activeRule, rule_id: 10, version: 1, status: 'retired' },
    ],
  }
  const mounted = await mountDetail(detail)
  try {
    const text = browserWindow.document.body.textContent ?? ''
    assert.match(text, /Supplier cost rule details/)
    assert.match(text, /USD 3/)
    assert.match(text, /1\.2/)
    assert.ok(text.indexOf('v2') < text.indexOf('v1'))
  } finally {
    await unmountDetail(mounted)
  }
})

test('shows invalid configuration as unavailable without raw JSON', async () => {
  const mounted = await mountDetail({
    rule: {
      ...activeRule,
      prices: [],
      price_status: 'unavailable',
      issues: ['invalid_config'],
      note: 'safe note',
    },
    history: [],
  })
  try {
    const text = browserWindow.document.body.textContent ?? ''
    assert.match(text, /unavailable/)
    assert.match(text, /invalid_config/)
    assert.doesNotMatch(text, /config_json|do-not-expose-json/)
  } finally {
    await unmountDetail(mounted)
  }
})
