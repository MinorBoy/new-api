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

// @ts-expect-error Bun supplies mock.module at test runtime, but the frontend
// typecheck intentionally only includes Node's test declarations.
import { mock } from 'bun:test'

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
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
import {
  formatMarginBPSPercent,
  marginPercentInputToBPS,
} from '@/lib/margin-bps'
import { api } from '@/lib/api'
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

const pageSearch = {
  current: {
    tab: 'profit' as const,
    timeBasis: 'requested_at' as const,
    startTime: 100,
    endTime: 200,
    channelId: 7,
    billableModel: 'vendor-model',
    originModel: 'client-model',
    userGroup: 'default',
    usingGroup: 'premium',
  },
}
const pageNavigations: Array<Record<string, unknown>> = []

mock.module('@tanstack/react-router', () => ({
  createFileRoute: () => (config: Record<string, unknown>) => config,
  getRouteApi: () => ({
    useSearch: () => pageSearch.current,
    useNavigate: () => (options: { search: Record<string, unknown> }) => {
      pageNavigations.push(options.search)
    },
  }),
  useNavigate: () => () => {},
  useBlocker: () => ({ status: 'idle', proceed: () => {}, reset: () => {} }),
  useRouterState: () => ({ location: { pathname: '/cost-accounting' } }),
  Link: (props: { children?: React.ReactNode }) => props.children ?? null,
  Outlet: () => null,
  redirect: (options: Record<string, unknown>) => new Error(String(options.to)),
  useLocation: () => ({ pathname: '/cost-accounting' }),
}))

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
  pageNavigations.length = 0
})

const { createRoot } = await import('react-dom/client')
const { ProfitSummary } = await import('../profit-summary')
const { ProfitTable } = await import('../profit-table')
const { CostAccounting } = await import('../../index')
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

function setInputValue(input: HTMLInputElement, value: string) {
  const valueSetter = Object.getOwnPropertyDescriptor(
    browserWindow.HTMLInputElement.prototype,
    'value'
  )?.set
  assert.ok(valueSetter)
  valueSetter.call(input, value)
  input.dispatchEvent(new Event('input', { bubbles: true }))
}

test('keeps report requests on committed URL filters until draft filters are applied', async () => {
  const originalGet = api.get
  const reportRequests: Array<{
    url: string
    params?: Record<string, unknown>
  }> = []
  api.get = (async (
    url: string,
    config?: { params?: Record<string, unknown> }
  ) => {
    if (url.includes('/reports/')) {
      reportRequests.push({ url, params: config?.params })
    }
    let data: unknown = []
    if (url.endsWith('/settings')) {
      data = { mode: 'disabled', minimum_expected_margin_bps: 0 }
    } else if (url.endsWith('/reports/summary')) {
      data = summary
    } else if (url.endsWith('/reports/breakdown')) {
      data = [breakdown]
    } else if (url.endsWith('/reports/filter-options')) {
      data = {
        channels: [{ id: 7, name: 'Primary OpenAI' }],
        billable_upstream_models: ['vendor-model'],
        origin_models: ['client-model'],
        user_groups: ['default'],
        using_groups: ['premium'],
      }
    }
    return { data: { success: true, message: '', data } }
  }) as typeof api.get
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })

  let mounted: Awaited<ReturnType<typeof mount>> | undefined
  try {
    mounted = await mount(
      <QueryClientProvider client={queryClient}>
        <CostAccounting />
      </QueryClientProvider>
    )
    await act(async () => {
      await Promise.all(
        queryClient
          .getQueryCache()
          .getAll()
          .map((query) => query.promise?.catch(() => {}))
      )
    })

    const committedParams = {
      time_basis: 'requested_at',
      start_time: 100,
      end_time: 200,
      channel_id: 7,
      billable_upstream_model: 'vendor-model',
      origin_model: 'client-model',
      user_group: 'default',
      using_group: 'premium',
      billing_source: undefined,
      status: undefined,
    }
    assert.deepEqual(
      reportRequests.map((request) => request.params),
      [committedParams, committedParams, committedParams]
    )

    const billableModel = browserWindow.document.querySelector(
      '#profit-billable-model'
    )
    assert.ok(billableModel instanceof browserWindow.HTMLInputElement)
    await act(async () =>
      setInputValue(
        billableModel as unknown as HTMLInputElement,
        'draft-model'
      )
    )
    assert.equal(reportRequests.length, 3)
    assert.equal(pageNavigations.length, 0)

    const applyButton = [...browserWindow.document.querySelectorAll('button')]
      .map((button) => button as unknown as HTMLButtonElement)
      .find((button) => button.textContent?.includes('Apply filters'))
    assert.ok(applyButton)
    await act(async () => applyButton.click())
    assert.equal(pageNavigations.length, 1)
    assert.equal(pageNavigations[0]?.billableModel, 'draft-model')
  } finally {
    api.get = originalGet
    queryClient.clear()
    if (mounted) await unmount(mounted)
  }
})

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
    const profit = browserWindow.document.querySelector(
      '[data-metric="gross-profit"]'
    )
    const grossMargin = browserWindow.document.querySelector(
      '[data-metric="gross-margin"]'
    )
    assert.equal(profit?.classList.contains('text-success'), true)
    assert.equal(grossMargin?.classList.contains('text-success'), true)
  } finally {
    await unmount(mounted)
  }
})

test('renders negative channel profit and margin in the destructive color', async () => {
  const mounted = await mount(
    <ProfitTable
      rows={[
        {
          ...breakdown,
          realized_profit_nano_usd: '-250000000',
          gross_margin_ppm: '-125000',
        },
      ]}
      loading={false}
    />
  )
  try {
    const profit = browserWindow.document.querySelector(
      '[data-metric="gross-profit"]'
    )
    const grossMargin = browserWindow.document.querySelector(
      '[data-metric="gross-margin"]'
    )
    assert.equal(profit?.classList.contains('text-destructive'), true)
    assert.equal(grossMargin?.classList.contains('text-destructive'), true)
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
    const profit = browserWindow.document.querySelector(
      '[data-metric="gross-profit"]'
    )
    assert.equal(profit?.classList.contains('text-success'), false)
    assert.equal(profit?.classList.contains('text-destructive'), false)
    assert.equal(margin?.classList.contains('text-success'), false)
    assert.equal(margin?.classList.contains('text-destructive'), false)
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
      billableModel: ' \tvendor-model\t ',
      originModel: ' \u00a0client-model\u00a0 ',
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
      billable_upstream_model: '\tvendor-model\t',
      origin_model: '\u00a0client-model\u00a0',
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

test('converts minimum expected margin percentage input to basis points', () => {
  assert.equal(marginPercentInputToBPS('10.25'), 1025)
  assert.equal(marginPercentInputToBPS('0'), 0)
  assert.equal(marginPercentInputToBPS('100'), 10000)
  assert.equal(formatMarginBPSPercent(1025), '10.25')
  assert.equal(formatMarginBPSPercent(0), '0')
  assert.equal(formatMarginBPSPercent(10000), '100')
  assert.throws(() => marginPercentInputToBPS('-0.01'))
  assert.throws(() => marginPercentInputToBPS('100.01'))
  assert.throws(() => marginPercentInputToBPS('1.234'))
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
