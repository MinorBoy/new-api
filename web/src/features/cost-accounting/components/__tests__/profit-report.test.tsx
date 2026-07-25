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

import { Window } from 'happy-dom'
import { createInstance } from 'i18next'
import { act } from 'react'
import type { Container, Root } from 'react-dom/client'
import { I18nextProvider } from 'react-i18next'

import type { NavGroup } from '@/components/layout/types'
import {
  ADMIN_PERMISSION_ACTIONS,
  ADMIN_PERMISSION_RESOURCES,
} from '@/lib/admin-permissions'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import type { CostProfitBreakdown, CostProfitSummary } from '../../types'

const browserWindow = new Window({ url: 'http://localhost/' })
browserWindow.document.write('<!doctype html><html><body></body></html>')
browserWindow.document.close()
Object.defineProperty(browserWindow.document, 'compatMode', {
  configurable: true,
  value: 'CSS1Compat',
})
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

beforeEach(() => {
  browserWindow.document.body.replaceChildren()
  useAuthStore.getState().auth.setUser({
    id: 1,
    username: 'super-admin',
    role: ROLE.SUPER_ADMIN,
  })
})

const { createRoot } = await import('react-dom/client')
const { ProfitSummary } = await import('../profit-summary')
const { ProfitTable } = await import('../profit-table')
const { costReportParamsFromSearch, canEnableStrictCostAccounting } =
  await import('../../lib/report')
const { filterNavGroupsByAccess } = await import('@/hooks/use-sidebar-view')
const { isSidebarModuleEnabled } = await import('@/hooks/use-sidebar-config')
const { requireCostAccountingRead } =
  await import('@/routes/_authenticated/cost-accounting/index')
const i18n = createInstance()
await i18n.init({
  lng: 'en',
  fallbackLng: 'en',
  resources: { en: { translation: {} } },
})

const summary: CostProfitSummary = {
  realized_revenue_nano_usd: '1000000000',
  realized_cost_nano_usd: '250000000',
  realized_profit_nano_usd: '750000000',
  gross_margin_ppm: '750000',
  known_incomplete_cost_nano_usd: '125000000',
  complete_request_count: 12,
  negative_profit_request_count: 1,
  retry_attempt_count: 3,
  awaiting_meter_count: 2,
  unknown_cost_count: 4,
  settlement_failed_count: 5,
  revenue_failed_count: 6,
}

const breakdown: CostProfitBreakdown = {
  ...summary,
  channel_id: 7,
  channel_name: 'Primary OpenAI',
  billable_upstream_model: 'vendor-model',
  attempt_count: 15,
}

async function mount(element: React.ReactNode): Promise<{
  root: Root
  container: { remove(): void }
}> {
  const container = browserWindow.document.createElement('div')
  browserWindow.document.body.append(container)
  const root = createRoot(container as unknown as Container)
  await act(async () => {
    root.render(<I18nextProvider i18n={i18n}>{element}</I18nextProvider>)
  })
  return { root, container }
}

async function unmount(mounted: { root: Root; container: { remove(): void } }) {
  await act(async () => mounted.root.unmount())
  mounted.container.remove()
}

test('renders exact billed profit totals and attributed channel rows', async () => {
  const mounted = await mount(
    <>
      <ProfitSummary summary={summary} loading={false} />
      <ProfitTable rows={[breakdown]} loading={false} />
    </>
  )
  try {
    const text = browserWindow.document.body.textContent ?? ''
    assert.match(text, /Billed revenue equivalent/)
    assert.match(text, /\$1/)
    assert.match(text, /\$0\.25/)
    assert.match(text, /\$0\.75/)
    assert.match(text, /75%/)
    assert.match(text, /Primary OpenAI/)
    assert.match(text, /vendor-model/)
    assert.match(text, /15/)
  } finally {
    await unmount(mounted)
  }
})

test('renders an em dash for margin when revenue is zero', async () => {
  const mounted = await mount(
    <ProfitSummary
      summary={{
        ...summary,
        realized_revenue_nano_usd: '0',
        realized_cost_nano_usd: '0',
        realized_profit_nano_usd: '0',
        gross_margin_ppm: undefined,
      }}
      loading={false}
    />
  )
  try {
    const margin = browserWindow.document.querySelector(
      '[data-metric="gross-margin"]'
    )
    assert.equal(margin?.textContent?.trim(), '—')
  } finally {
    await unmount(mounted)
  }
})

test('maps validated URL search fields to cost report API parameters', () => {
  assert.deepEqual(
    costReportParamsFromSearch({
      tab: 'profit',
      timeBasis: 'requested_at',
      startTime: 100,
      endTime: 200,
      channelId: 7,
      billableModel: 'vendor-model',
      originModel: 'client-model',
      userGroup: 'default',
      usingGroup: 'premium',
      billingSource: 'wallet',
      status: 'complete',
    }),
    {
      time_basis: 'requested_at',
      start_time: 100,
      end_time: 200,
      channel_id: 7,
      billable_upstream_model: 'vendor-model',
      origin_model: 'client-model',
      user_group: 'default',
      using_group: 'premium',
      billing_source: 'wallet',
      status: 'complete',
    }
  )
})

test('allows strict mode only when authoritative coverage is complete', () => {
  assert.equal(canEnableStrictCostAccounting([]), true)
  assert.equal(
    canEnableStrictCostAccounting([
      {
        channel_id: 7,
        origin_model: 'client-model',
        predicted_upstream_model: 'vendor-model',
        covered: false,
      },
    ]),
    false
  )
  assert.equal(
    canEnableStrictCostAccounting([
      {
        channel_id: 7,
        origin_model: 'client-model',
        predicted_upstream_model: 'vendor-model',
        covered: true,
      },
    ]),
    true
  )
})

test('filters sidebar items by cost-accounting permission', () => {
  const groups: NavGroup[] = [
    {
      id: 'admin',
      title: 'Admin',
      items: [
        { title: 'Channels', url: '/channels' },
        {
          title: 'Cost accounting',
          url: '/cost-accounting',
          requiredPermission: {
            resource: ADMIN_PERMISSION_RESOURCES.COST_ACCOUNTING,
            action: ADMIN_PERMISSION_ACTIONS.READ,
          },
        },
      ],
    },
  ]
  const withoutPermission = filterNavGroupsByAccess(groups, {
    id: 2,
    username: 'admin',
    role: ROLE.ADMIN,
  })
  assert.deepEqual(
    withoutPermission[0]?.items.map((item) => item.title),
    ['Channels']
  )

  const withPermission = filterNavGroupsByAccess(groups, {
    id: 3,
    username: 'cost-reader',
    role: ROLE.ADMIN,
    permissions: {
      admin_permissions: {
        cost_accounting: { read: true },
      },
    },
  })
  assert.deepEqual(
    withPermission[0]?.items.map((item) => item.title),
    ['Channels', 'Cost accounting']
  )
})

test('hides the report when the administrator disables its sidebar module', () => {
  assert.equal(
    isSidebarModuleEnabled(
      '/cost-accounting',
      { admin: { enabled: true, cost_accounting: false } },
      null
    ),
    false
  )
  assert.equal(
    isSidebarModuleEnabled(
      '/cost-accounting',
      { admin: { enabled: true, cost_accounting: true } },
      null
    ),
    true
  )
})

test('route guard redirects administrators without cost read permission', () => {
  useAuthStore.getState().auth.setUser({
    id: 2,
    username: 'admin',
    role: ROLE.ADMIN,
  })
  let redirectThrown = false
  try {
    requireCostAccountingRead()
  } catch {
    redirectThrown = true
  }
  assert.equal(redirectThrown, true)

  useAuthStore.getState().auth.setUser({
    id: 3,
    username: 'cost-reader',
    role: ROLE.ADMIN,
    permissions: {
      admin_permissions: {
        cost_accounting: { read: true },
      },
    },
  })
  assert.doesNotThrow(() => requireCostAccountingRead())
})
