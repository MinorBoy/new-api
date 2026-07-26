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

import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import type { Channel } from '../../../channels/types'
import { costAccountingQueryKeys } from '../../api'
import type { CostRule } from '../../types'

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

beforeEach(() => {
  browserWindow.document.body.replaceChildren()
  useAuthStore.getState().auth.setUser({
    id: 1,
    username: 'admin',
    role: ROLE.SUPER_ADMIN,
  })
})

const { createRoot } = await import('react-dom/client')
const { TooltipProvider } = await import('@/components/ui/tooltip')
const { ChannelCostDrawer } = await import('../channel-cost-drawer')
const { CostRuleDrawer } = await import('../cost-rule-drawer')
const i18n = createInstance()
await i18n.init({
  lng: 'en',
  fallbackLng: 'en',
  resources: { en: { translation: {} } },
})

const channel: Channel = {
  id: 7,
  type: 1,
  key: '',
  status: 1,
  name: 'Primary OpenAI',
  created_time: 1,
  test_time: 1,
  response_time: 120,
  other: '',
  balance: 0,
  balance_updated_time: 1,
  models: 'client-model',
  group: 'default',
  used_quota: 0,
  other_info: '',
  remark: '',
  max_input_tokens: 0,
  channel_info: {
    is_multi_key: false,
    multi_key_size: 0,
    multi_key_polling_index: 0,
    multi_key_mode: 'random',
  },
  settings: '{}',
  routing_target_count: 0,
}

const activeRule: CostRule = {
  id: 11,
  channel_id: 7,
  billable_upstream_model: 'vendor-model',
  cost_variant_key: 'default',
  version: 1,
  status: 'active',
  cost_mode: 'per_request',
  schema_version: 1,
  config: {
    currency: 'USD',
    billing_multiplier: '1',
    purchase_discount_ratio: '1',
    recharge_exchange_ratio: '1',
    fee_rate: '0',
    currency_to_usd_rate: '1',
    unit_price: '0.12',
    charge_event: 'response_succeeded',
    normalized_usd_prices: { unit_price: '0.12' },
  },
  source: 'manual',
  note: 'Initial supplier contract',
  created_by: 1,
  activated_by: 1,
  effective_from: 10,
  created_at: 9,
  updated_at: 10,
}

const draftRule: CostRule = {
  ...activeRule,
  id: 12,
  version: 2,
  status: 'draft',
  activated_by: 0,
  effective_from: undefined,
  created_at: 11,
  updated_at: 11,
}

function createBaseQueryClient(): QueryClient {
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

function createQueryClient(): QueryClient {
  const queryClient = createBaseQueryClient()
  queryClient.setQueryData(
    costAccountingQueryKeys.ruleList({ channel_id: channel.id }),
    { success: true, message: '', data: [activeRule] }
  )
  queryClient.setQueryData(
    costAccountingQueryKeys.coverage({ channel_id: channel.id }),
    {
      success: true,
      message: '',
      data: [
        {
          channel_id: channel.id,
          origin_model: 'client-model',
          predicted_upstream_model: 'vendor-model',
          covered: true,
        },
      ],
    }
  )
  queryClient.setQueryData(['pricing'], {
    success: true,
    data: [
      {
        id: 1,
        model_name: 'client-model',
        quota_type: 1,
        model_ratio: 0,
        completion_ratio: 1,
        model_price: 0.5,
        enable_groups: ['default'],
      },
    ],
    vendors: [],
    group_ratio: { default: 1 },
    usable_group: { default: { desc: 'Default', ratio: 1 } },
    supported_endpoint: {},
    auto_groups: [],
  })
  return queryClient
}

async function mount(
  element: React.ReactNode,
  queryClient = createQueryClient()
): Promise<{
  root: Root
  container: { remove(): void }
  queryClient: QueryClient
}> {
  const container = browserWindow.document.createElement('div')
  browserWindow.document.body.append(container)
  const root = createRoot(container as unknown as Container)

  await act(async () => {
    root.render(
      <I18nextProvider i18n={i18n}>
        <QueryClientProvider client={queryClient}>
          <TooltipProvider>{element}</TooltipProvider>
        </QueryClientProvider>
      </I18nextProvider>
    )
  })
  await act(async () => undefined)
  return { root, container, queryClient }
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

function findButton(label: string): HTMLButtonElement {
  const button = [...browserWindow.document.querySelectorAll('button')].find(
    (candidate) => candidate.textContent?.trim() === label
  )
  assert.ok(button instanceof browserWindow.HTMLButtonElement)
  return button as unknown as HTMLButtonElement
}

test('shows mapped models, official price, active rule, normalized price, and coverage', async () => {
  const mounted = await mount(
    <ChannelCostDrawer open channel={channel} onOpenChange={() => {}} />
  )
  try {
    const text = browserWindow.document.body.textContent ?? ''
    assert.match(text, /Primary OpenAI/)
    assert.match(text, /vendor-model/)
    assert.match(text, /client-model/)
    assert.match(text, /\$0\.5/)
    assert.match(text, /0\.12/)
    assert.match(text, /Active/)
    assert.match(text, /Covered/)
    assert.ok(
      browserWindow.document.querySelector(
        'button[aria-label="Show version history"]'
      )
    )
  } finally {
    await unmount(mounted)
  }
})

test('shows each cost variant as a distinct rule row', async () => {
  const queryClient = createQueryClient()
  const rule480 = {
    ...activeRule,
    id: 21,
    cost_variant_key: '480p',
  } as CostRule
  const rule720 = {
    ...activeRule,
    id: 22,
    cost_variant_key: '720p',
  } as CostRule
  queryClient.setQueryData(
    costAccountingQueryKeys.ruleList({ channel_id: channel.id }),
    { success: true, message: '', data: [rule480, rule720] }
  )

  const mounted = await mount(
    <ChannelCostDrawer open channel={channel} onOpenChange={() => {}} />,
    queryClient
  )
  try {
    const text = browserWindow.document.body.textContent ?? ''
    assert.match(text, /480p/)
    assert.match(text, /720p/)
  } finally {
    await unmount(mounted)
  }
})

test('opens an editable new cost variant from an existing rule', async () => {
  const mounted = await mount(
    <ChannelCostDrawer open channel={channel} onOpenChange={() => {}} />
  )
  try {
    const action = browserWindow.document.querySelector(
      'button[aria-label="New cost variant"]'
    ) as HTMLButtonElement | null
    assert.ok(action)
    await act(async () => action.click())

    const input = browserWindow.document.querySelector(
      '#cost-rule-cost-variant-key'
    ) as HTMLInputElement | null
    assert.ok(input)
    assert.equal(input.value, 'default')
    assert.equal(input.disabled, false)
  } finally {
    await unmount(mounted)
  }
})

test('filters cost rules by cost variant', async () => {
  const queryClient = createQueryClient()
  const rule720 = {
    ...activeRule,
    id: 22,
    cost_variant_key: '720p',
  }
  queryClient.setQueryData(
    costAccountingQueryKeys.ruleList({
      channel_id: channel.id,
      cost_variant_key: '720p',
    }),
    { success: true, message: '', data: [rule720] }
  )
  queryClient.setQueryData(
    costAccountingQueryKeys.coverage({
      channel_id: channel.id,
      cost_variant_key: '720p',
    }),
    {
      success: true,
      message: '',
      data: [
        {
          channel_id: channel.id,
          origin_model: 'client-model',
          predicted_upstream_model: 'vendor-model',
          cost_variant_key: '720p',
          covered: true,
        },
      ],
    }
  )

  const mounted = await mount(
    <ChannelCostDrawer open channel={channel} onOpenChange={() => {}} />,
    queryClient
  )
  try {
    const input = browserWindow.document.querySelector(
      '#channel-cost-variant-filter'
    ) as HTMLInputElement | null
    assert.ok(input)
    await act(async () => {
      const setValue = Object.getOwnPropertyDescriptor(
        browserWindow.HTMLInputElement.prototype,
        'value'
      )?.set
      assert.ok(setValue)
      setValue.call(input, '720p')
      input.dispatchEvent(
        new browserWindow.Event('input', { bubbles: true }) as unknown as Event
      )
    })

    const text = browserWindow.document.body.textContent ?? ''
    assert.match(text, /720p/)
    assert.doesNotMatch(text, /default/)
  } finally {
    await unmount(mounted)
  }
})

test('switching cost mode replaces conditional price and meter fields', async () => {
  const mounted = await mount(
    <CostRuleDrawer
      open
      channel={channel}
      billableModel='vendor-model'
      originModel='client-model'
      rule={null}
      canWrite
      onOpenChange={() => {}}
    />
  )
  try {
    assert.match(browserWindow.document.body.textContent ?? '', /Unit price/)
    await act(async () => findButton('Per duration').click())
    const text = browserWindow.document.body.textContent ?? ''
    assert.match(text, /Price per second/)
    assert.match(text, /Meter source/)
    assert.doesNotMatch(text, /Unit price/)
  } finally {
    await unmount(mounted)
  }
})

test('task-only channels default new rules to task completion', async () => {
  const mounted = await mount(
    <CostRuleDrawer
      open
      channel={{ ...channel, type: 59, name: 'Dimensio' }}
      billableModel='vendor-task-model'
      originModel='client-task-model'
      rule={null}
      canWrite
      onOpenChange={() => {}}
    />
  )
  try {
    assert.match(browserWindow.document.body.textContent ?? '', /Task succeeded/)
  } finally {
    await unmount(mounted)
  }
})

test('new cost rules default their cost variant key', async () => {
  const mounted = await mount(
    <CostRuleDrawer
      open
      channel={channel}
      billableModel='vendor-model'
      originModel='client-model'
      rule={null}
      canWrite
      onOpenChange={() => {}}
    />
  )
  try {
    const input = browserWindow.document.querySelector(
      '#cost-rule-cost-variant-key'
    ) as HTMLInputElement | null
    assert.ok(input)
    assert.equal(input.value, 'default')
  } finally {
    await unmount(mounted)
  }
})

test('read-only administrators keep preview and history without write actions', async () => {
  useAuthStore.getState().auth.setUser({
    id: 2,
    username: 'cost-reader',
    role: ROLE.ADMIN,
    permissions: {
      admin_permissions: {
        cost_accounting: { read: true, write: false },
      },
    },
  })

  const mounted = await mount(
    <ChannelCostDrawer open channel={channel} onOpenChange={() => {}} />
  )
  try {
    assert.ok(
      browserWindow.document.querySelector('button[aria-label="Preview cost"]')
    )
    assert.ok(
      browserWindow.document.querySelector(
        'button[aria-label="Show version history"]'
      )
    )
    assert.equal(
      browserWindow.document.querySelector(
        'button[aria-label="Create cost draft"]'
      ),
      null
    )
    assert.equal(
      browserWindow.document.querySelector(
        'button[aria-label="Retire cost rule"]'
      ),
      null
    )
  } finally {
    await unmount(mounted)
  }
})

test('requires confirmation before activating a draft rule', async () => {
  const queryClient = createQueryClient()
  queryClient.setQueryData(
    costAccountingQueryKeys.ruleList({ channel_id: channel.id }),
    { success: true, message: '', data: [activeRule, draftRule] }
  )

  const mounted = await mount(
    <ChannelCostDrawer open channel={channel} onOpenChange={() => {}} />,
    queryClient
  )
  try {
    const activateButton = browserWindow.document.querySelector(
      'button[aria-label="Activate cost rule"]'
    )
    assert.ok(activateButton instanceof browserWindow.HTMLButtonElement)
    await act(async () => activateButton.click())

    const text = browserWindow.document.body.textContent ?? ''
    assert.match(text, /Activate cost rule\?/)
    assert.match(
      text,
      /The new version becomes authoritative for future requests\./
    )
    assert.ok(findButton('Activate'))
  } finally {
    await unmount(mounted)
  }
})

test('shows a stable empty state when no mappings or rules exist', async () => {
  const queryClient = createBaseQueryClient()
  queryClient.setQueryData(
    costAccountingQueryKeys.ruleList({ channel_id: channel.id }),
    { success: true, message: '', data: [] }
  )
  queryClient.setQueryData(
    costAccountingQueryKeys.coverage({ channel_id: channel.id }),
    { success: true, message: '', data: [] }
  )
  queryClient.setQueryData(['pricing'], {
    success: true,
    data: [],
    vendors: [],
    group_ratio: {},
    usable_group: {},
    supported_endpoint: {},
    auto_groups: [],
  })

  const mounted = await mount(
    <ChannelCostDrawer open channel={channel} onOpenChange={() => {}} />,
    queryClient
  )
  try {
    assert.match(
      browserWindow.document.body.textContent ?? '',
      /No model cost data/
    )
  } finally {
    await unmount(mounted)
  }
})

test('shows a retry action when a cost query fails', async () => {
  const queryClient = createQueryClient()
  const queryKey = costAccountingQueryKeys.ruleList({
    channel_id: channel.id,
  })
  const query = queryClient.getQueryCache().find({
    queryKey,
    exact: true,
  })
  assert.ok(query)
  query.setState({
    ...query.state,
    status: 'error',
    fetchStatus: 'idle',
    error: new Error('Rule service unavailable'),
    errorUpdatedAt: Date.now(),
  })
  assert.equal(queryClient.getQueryState(queryKey)?.status, 'error')

  const mounted = await mount(
    <ChannelCostDrawer open channel={channel} onOpenChange={() => {}} />,
    queryClient
  )
  try {
    const text = browserWindow.document.body.textContent ?? ''
    assert.match(text, /Failed to load model costs/)
    assert.match(text, /Rule service unavailable/)
    assert.ok(findButton('Retry'))
  } finally {
    await unmount(mounted)
  }
})

test('uses fixed-height skeleton rows while cost queries are pending', async () => {
  const queryClient = createBaseQueryClient()
  const pending = () => new Promise<never>(() => {})
  void queryClient
    .fetchQuery({
      queryKey: costAccountingQueryKeys.ruleList({ channel_id: channel.id }),
      queryFn: pending,
    })
    .catch(() => {})
  void queryClient
    .fetchQuery({
      queryKey: costAccountingQueryKeys.coverage({ channel_id: channel.id }),
      queryFn: pending,
    })
    .catch(() => {})
  void queryClient
    .fetchQuery({ queryKey: ['pricing'], queryFn: pending })
    .catch(() => {})

  const mounted = await mount(
    <ChannelCostDrawer open channel={channel} onOpenChange={() => {}} />,
    queryClient
  )
  try {
    const loading = browserWindow.document.querySelector(
      '[aria-label="Loading"]'
    )
    assert.ok(loading)
    assert.equal(loading.querySelectorAll('[data-slot="skeleton"]').length, 4)
  } finally {
    await unmount(mounted)
  }
})
