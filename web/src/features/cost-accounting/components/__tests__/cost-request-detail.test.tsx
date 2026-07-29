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
import test, { after, afterEach, beforeEach } from 'node:test'

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { AxiosResponse, InternalAxiosRequestConfig } from 'axios'
import { Window } from 'happy-dom'
import { createInstance } from 'i18next'
import { act } from 'react'
import type { Container, Root } from 'react-dom/client'
import { I18nextProvider } from 'react-i18next'

import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { costAccountingQueryKeys } from '../../api'
import type {
  CostAccountingAttemptLedger,
  CostRequestDetail as CostRequestDetailData,
} from '../../types'

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

const originalAdapter = api.defaults.adapter

afterEach(() => {
  api.defaults.adapter = originalAdapter
})

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
const { TooltipProvider } = await import('@/components/ui/tooltip')
const { CostRequestDetail } = await import('../cost-request-detail')
const { CostReconcileDrawer } = await import('../reconcile-drawer')
const { AnomalyQueue } = await import('../anomaly-queue')
const i18n = createInstance()
await i18n.init({
  lng: 'en',
  fallbackLng: 'en',
  resources: { en: { translation: {} } },
})

const attempt: CostAccountingAttemptLedger = {
  id: 201,
  cost_request_id: 101,
  attempt_no: 1,
  channel_id: 7,
  channel_name: 'Primary OpenAI',
  channel_type: 1,
  predicted_upstream_model: 'vendor-predicted',
  billable_upstream_model: 'vendor-billable',
  rule_id: 11,
  rule_version: 3,
  cost_mode: 'per_token',
  schema_version: 1,
  rule_config_json: JSON.stringify({
    token_mode: 'input_output',
    normalized_usd_prices: {
      input_per_million: '1',
      output_per_million: '2',
    },
  }),
  charge_event: 'response_succeeded',
  meter_source: 'upstream_usage',
  billable_request_count: 1,
  request_meter_json: '{}',
  actual_meter_json: JSON.stringify({
    source: 'upstream_usage',
    input_tokens: 0,
    output_tokens: 12,
  }),
  original_cost: '0.000024',
  cost_nano_usd: '24000',
  upstream_accepted: true,
  http_status: 200,
  result_code: 'ok',
  failure_code: 'upstream_usage_missing',
  status: 'cost_unknown',
  reconciliation_status: 'none',
  prepared_at: 100,
  dispatching_at: 101,
  accepted_at: 102,
  terminal_at: 103,
  created_at: 100,
  updated_at: 103,
}

const requestDetail: CostRequestDetailData = {
  request: {
    id: 101,
    request_id: 'req-cost-101',
    user_id: 9,
    token_id: 4,
    user_group: 'default',
    using_group: 'premium',
    origin_model_name: 'client-model',
    billing_source: 'wallet',
    subscription_id: 0,
    subscription_plan_id: 0,
    final_user_quota: 500000,
    quota_per_unit_snapshot: '500000',
    billed_revenue_equivalent_nano_usd: '1000000000',
    confirmed_cost_nano_usd: '24000',
    attempt_count: 1,
    winning_attempt_id: 201,
    billed_gross_profit_nano_usd: '999976000',
    gross_margin_ppm: '999976',
    revenue_status: 'settled',
    profit_status: 'incomplete_cost',
    failure_code: '',
    requested_at: 100,
    revenue_settled_at: 104,
    created_at: 100,
    updated_at: 104,
  },
  attempts: [{ attempt, winning: true }],
  audits: [
    {
      id: 301,
      cost_request_id: 101,
      cost_attempt_id: 201,
      admin_id: 1,
      old_state: 'settlement_failed',
      new_state: 'cost_unknown',
      meter_json: attempt.actual_meter_json,
      rule_id: 11,
      rule_version: 3,
      old_amount_nano_usd: '0',
      new_amount_nano_usd: '24000',
      reason: 'Provider invoice review',
      created_at: 105,
    },
  ],
}

function responseFor<T>(
  config: InternalAxiosRequestConfig,
  data: T
): AxiosResponse<T> {
  return {
    data,
    status: 200,
    statusText: 'OK',
    headers: {},
    config,
  }
}

function createQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
        staleTime: Number.POSITIVE_INFINITY,
        refetchOnMount: false,
      },
      mutations: { retry: false },
    },
  })
}

async function mount(
  element: React.ReactNode,
  queryClient = createQueryClient()
): Promise<{
  root: Root
  container: { remove(): void }
  queryClient: QueryClient
  render: (next: React.ReactNode) => Promise<void>
}> {
  const container = browserWindow.document.createElement('div')
  browserWindow.document.body.append(container)
  const root = createRoot(container as unknown as Container)
  const render = async (next: React.ReactNode) => {
    await act(async () => {
      root.render(
        <I18nextProvider i18n={i18n}>
          <QueryClientProvider client={queryClient}>
            <TooltipProvider>{next}</TooltipProvider>
          </QueryClientProvider>
        </I18nextProvider>
      )
    })
  }
  await render(element)
  return { root, container, queryClient, render }
}

async function unmount(mounted: {
  root: Root
  container: { remove(): void }
  queryClient: QueryClient
}) {
  await act(async () => mounted.root.unmount())
  mounted.queryClient.clear()
  mounted.container.remove()
}

function setInputValue(input: object & { value: string }, value: string) {
  const setter = Object.getOwnPropertyDescriptor(
    Object.getPrototypeOf(input) as object,
    'value'
  )?.set
  assert.ok(setter)
  setter.call(input, value)
  const eventTarget = input as unknown as {
    dispatchEvent: (event: unknown) => boolean
  }
  eventTarget.dispatchEvent(new browserWindow.Event('input', { bubbles: true }))
  eventTarget.dispatchEvent(
    new browserWindow.Event('change', { bubbles: true })
  )
}

function findButton(label: string): HTMLButtonElement {
  const button = [...browserWindow.document.querySelectorAll('button')].find(
    (candidate) => candidate.textContent?.trim() === label
  )
  assert.ok(button instanceof browserWindow.HTMLButtonElement)
  return button as unknown as HTMLButtonElement
}

test('fetches request detail only after an administrator opens a referenced log', async () => {
  let calls = 0
  let release!: () => void
  const pending = new Promise<void>((resolve) => {
    release = resolve
  })
  api.defaults.adapter = async (config) => {
    calls += 1
    await pending
    return responseFor(config, {
      success: true,
      message: '',
      data: requestDetail,
    })
  }

  const mounted = await mount(
    <CostRequestDetail requestID={101} isAdmin open={false} />
  )
  try {
    assert.equal(calls, 0)

    await mounted.render(<CostRequestDetail requestID={101} isAdmin open />)
    assert.equal(calls, 1)

    const loaded = new Promise<void>((resolve, reject) => {
      const unsubscribe = mounted.queryClient.getQueryCache().subscribe(() => {
        const state = mounted.queryClient.getQueryState(
          costAccountingQueryKeys.request(101)
        )
        if (state?.status === 'success') {
          unsubscribe()
          resolve()
        }
        if (state?.status === 'error') {
          unsubscribe()
          reject(state.error)
        }
      })
    })
    await act(async () => {
      release()
      await loaded
    })
    await mounted.render(<CostRequestDetail requestID={101} isAdmin open />)
    assert.match(browserWindow.document.body.textContent ?? '', /req-cost-101/)
  } finally {
    await unmount(mounted)
  }
})

test('ordinary users and legacy logs never request supplier cost detail', async () => {
  let calls = 0
  api.defaults.adapter = async (config) => {
    calls += 1
    return responseFor(config, {
      success: true,
      message: '',
      data: requestDetail,
    })
  }

  const mounted = await mount(
    <CostRequestDetail requestID={101} isAdmin={false} open />
  )
  try {
    assert.equal(calls, 0)
    assert.doesNotMatch(
      browserWindow.document.body.textContent ?? '',
      /Supplier cost accounting/
    )

    await act(async () => {
      useAuthStore.getState().auth.setUser({
        id: 3,
        username: 'admin-without-cost-access',
        role: ROLE.ADMIN,
      })
    })
    await mounted.render(<CostRequestDetail requestID={101} isAdmin open />)
    assert.equal(calls, 0)

    await mounted.render(
      <CostRequestDetail requestID={undefined} isAdmin open />
    )
    await act(async () => {
      useAuthStore.getState().auth.setUser({
        id: 2,
        username: 'cost-reader',
        role: ROLE.ADMIN,
        permissions: {
          admin_permissions: {
            cost_accounting: { read: true },
          },
        },
      })
    })
    await mounted.render(
      <CostRequestDetail requestID={undefined} isAdmin open />
    )
    assert.equal(calls, 0)
    assert.match(
      browserWindow.document.body.textContent ?? '',
      /No supplier cost record/
    )
    assert.doesNotMatch(browserWindow.document.body.textContent ?? '', /\$0/)
  } finally {
    await unmount(mounted)
  }
})

test('shows winning attempt identity, frozen rule, zero meter values, and audits', async () => {
  const queryClient = createQueryClient()
  queryClient.setQueryData(costAccountingQueryKeys.request(101), {
    success: true,
    message: '',
    data: requestDetail,
  })

  const mounted = await mount(
    <CostRequestDetail requestID={101} isAdmin open />,
    queryClient
  )
  try {
    const text = browserWindow.document.body.textContent ?? ''
    assert.match(text, /Winning attempt/)
    assert.match(text, /Primary OpenAI/)
    assert.match(text, /vendor-predicted/)
    assert.match(text, /vendor-billable/)
    assert.match(text, /Rule v3/)
    assert.match(text, /Response succeeded/)
    assert.match(text, /Upstream usage/)
    assert.match(text, /input_tokens: 0/)
    assert.match(text, /Original amount/)
    assert.match(text, /Normalized amount/)
    assert.match(text, /Provider invoice review/)
  } finally {
    await unmount(mounted)
  }
})

test('requires a reconciliation reason and preserves explicit zero token meters', async () => {
  const submitted: Array<Record<string, unknown>> = []
  api.defaults.adapter = async (config) => {
    submitted.push(JSON.parse(String(config.data)) as Record<string, unknown>)
    return responseFor(config, { success: true, message: '', data: null })
  }

  const mounted = await mount(
    <CostReconcileDrawer
      open
      target={{ kind: 'attempt', attempt }}
      onOpenChange={() => {}}
    />
  )
  try {
    await act(async () => findButton('Reconcile').click())
    assert.match(
      browserWindow.document.body.textContent ?? '',
      /Reconciliation reason is required/
    )
    assert.equal(submitted.length, 0)

    const inputTokens = browserWindow.document.querySelector(
      '#reconcile-input-tokens'
    )
    const outputTokens = browserWindow.document.querySelector(
      '#reconcile-output-tokens'
    )
    const reason = browserWindow.document.querySelector('#reconcile-reason')
    assert.ok(inputTokens instanceof browserWindow.HTMLInputElement)
    assert.ok(outputTokens instanceof browserWindow.HTMLInputElement)
    assert.ok(reason instanceof browserWindow.HTMLTextAreaElement)

    await act(async () => {
      setInputValue(inputTokens, '0')
      setInputValue(outputTokens, '0')
      setInputValue(reason, 'Verified against provider invoice')
    })
    await act(async () => findButton('Reconcile').click())

    assert.equal(submitted[0]?.action, 'settle')
    assert.equal(submitted[0]?.reason, 'Verified against provider invoice')
    assert.deepEqual(submitted[0]?.meter, {
      source: 'upstream_usage',
      input_tokens: 0,
      output_tokens: 0,
    })
  } finally {
    await unmount(mounted)
  }
})

test('shows anomaly details to readers and repair commands only to reconcilers', async () => {
  useAuthStore.getState().auth.setUser({
    id: 2,
    username: 'cost-reader',
    role: ROLE.ADMIN,
    permissions: {
      admin_permissions: {
        cost_accounting: { read: true, reconcile: false },
      },
    },
  })
  const queryClient = createQueryClient()
  queryClient.setQueryData(
    costAccountingQueryKeys.anomalyList({ page: 1, page_size: 20 }),
    {
      success: true,
      message: '',
      data: {
        items: [
          {
            kind: 'cost_unknown',
            request: requestDetail.request,
            attempt,
            occurred_at: 103,
          },
          {
            kind: 'revenue_failed',
            request: {
              ...requestDetail.request,
              id: 102,
              request_id: 'req-cost-102',
              revenue_status: 'revenue_failed',
            },
            occurred_at: 106,
          },
        ],
        total: 2,
        page: 1,
        page_size: 20,
      },
    }
  )

  const mounted = await mount(<AnomalyQueue enabled />, queryClient)
  try {
    const readerText = browserWindow.document.body.textContent ?? ''
    assert.match(readerText, /Cost unknown/)
    assert.match(readerText, /Revenue failed/)
    assert.match(readerText, /Primary OpenAI/)
    assert.equal(
      [...browserWindow.document.querySelectorAll('button')].filter(
        (button) => button.textContent?.trim() === 'Reconcile'
      ).length,
      0
    )

    await act(async () => {
      useAuthStore.getState().auth.setUser({
        id: 1,
        username: 'super-admin',
        role: ROLE.SUPER_ADMIN,
      })
    })
    await mounted.render(<AnomalyQueue enabled />)
    assert.equal(
      [...browserWindow.document.querySelectorAll('button')].filter(
        (button) => button.textContent?.trim() === 'Reconcile'
      ).length,
      2
    )
  } finally {
    await unmount(mounted)
  }
})
