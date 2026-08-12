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
*/
import assert from 'node:assert/strict'
import test, { after, beforeEach } from 'node:test'

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { Window } from 'happy-dom'
import { act, createElement } from 'react'
import type { Container } from 'react-dom/client'

import { api } from '@/lib/api'

const browserWindow = new Window({ url: 'http://localhost/' })
browserWindow.document.write('<!doctype html><html><body></body></html>')
browserWindow.document.close()
const browserGlobals = {
  window: browserWindow,
  document: browserWindow.document,
  navigator: browserWindow.navigator,
  Element: browserWindow.Element,
  HTMLElement: browserWindow.HTMLElement,
  Node: browserWindow.Node,
  Event: browserWindow.Event,
  MutationObserver: browserWindow.MutationObserver,
  CompositionEvent: browserWindow.CompositionEvent,
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
const { costAccountingQueryKeys, getCostReportFilterOptions } =
  await import('../../api')
const { normalizeProfitFilterOptions, useProfitFilterOptions } =
  await import('../use-profit-filter-options')

test('requests profit filter options with the committed report parameters', async () => {
  const originalGet = api.get
  const requests: Array<{ url: string; params?: Record<string, unknown> }> = []
  api.get = (async (
    url: string,
    config?: { params?: Record<string, unknown> }
  ) => {
    requests.push({ url, params: config?.params })
    return { data: { success: true, data: {} } }
  }) as typeof api.get

  try {
    await getCostReportFilterOptions({
      time_basis: 'requested_at',
      start_time: 100,
      end_time: 200,
      channel_id: 7,
      billable_upstream_model: 'vendor-model',
      origin_model: 'client-model',
      user_group: 'default',
      using_group: 'premium',
    })

    assert.deepEqual(requests, [
      {
        url: '/api/cost-accounting/reports/filter-options',
        params: {
          time_basis: 'requested_at',
          start_time: 100,
          end_time: 200,
          channel_id: 7,
          billable_upstream_model: 'vendor-model',
          origin_model: 'client-model',
          user_group: 'default',
          using_group: 'premium',
        },
      },
    ])
  } finally {
    api.get = originalGet
  }
})

test('uses committed report parameters in the filter-options query key', () => {
  const params = {
    time_basis: 'requested_at' as const,
    start_time: 100,
    end_time: 200,
    channel_id: 7,
  }

  assert.deepEqual(costAccountingQueryKeys.reportFilterOptions(params), [
    'cost-accounting',
    'reports',
    'filter-options',
    params,
  ])
})

test('normalizes, deduplicates, sorts, and preserves committed filter values', () => {
  const options = normalizeProfitFilterOptions(
    {
      channels: [
        { id: 7, name: ' Primary ' },
        { id: 2, name: '' },
        { id: 7, name: 'Duplicate' },
        { id: 0, name: 'Invalid' },
      ],
      billable_upstream_models: ['vendor-z', '', ' vendor-a ', 'vendor-a'],
      origin_models: ['client-b', 'client-a', 'client-a'],
      user_groups: ['', 'default', ' default '],
      using_groups: ['premium', ''],
    },
    {
      channelId: 9,
      billableModel: 'historic-model',
      originModel: 'client-b',
      userGroup: 'missing-group',
      usingGroup: 'premium',
    }
  )

  assert.deepEqual(options.channels, [
    { value: '2', label: '2' },
    { value: '7', label: '7 - Primary' },
    { value: '9', label: '9' },
  ])
  assert.deepEqual(options.billableModels, [
    { value: 'historic-model', label: 'historic-model' },
    { value: 'vendor-a', label: 'vendor-a' },
    { value: 'vendor-z', label: 'vendor-z' },
  ])
  assert.deepEqual(options.originModels, [
    { value: 'client-a', label: 'client-a' },
    { value: 'client-b', label: 'client-b' },
  ])
  assert.deepEqual(options.userGroups, [
    { value: 'default', label: 'default' },
    { value: 'missing-group', label: 'missing-group' },
  ])
  assert.deepEqual(options.usingGroups, [
    { value: 'premium', label: 'premium' },
  ])
})

test('loads options from committed search with a thirty-second stale time', async () => {
  const originalGet = api.get
  const requests: Array<{ url: string; params?: Record<string, unknown> }> = []
  let resolveRequest = () => {}
  const requestStarted = new Promise<void>((resolve) => {
    resolveRequest = resolve
  })
  api.get = (async (
    url: string,
    config?: { params?: Record<string, unknown> }
  ) => {
    requests.push({ url, params: config?.params })
    resolveRequest()
    return {
      data: {
        success: true,
        message: '',
        data: {
          channels: [{ id: 7, name: 'Primary' }],
          billable_upstream_models: ['vendor-model'],
          origin_models: [],
          user_groups: [],
          using_groups: [],
        },
      },
    }
  }) as typeof api.get

  function Probe() {
    useProfitFilterOptions(
      {
        tab: 'profit',
        timeBasis: 'requested_at',
        startTime: 100,
        channelId: 7,
      },
      true
    )
    return null
  }

  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const container = browserWindow.document.createElement('div')
  browserWindow.document.body.append(container)
  const root = createRoot(container as unknown as Container)

  try {
    await act(async () => {
      root.render(
        createElement(
          QueryClientProvider,
          { client: queryClient },
          createElement(Probe)
        )
      )
    })
    await requestStarted
    const committedParams = {
      time_basis: 'requested_at' as const,
      start_time: 100,
      end_time: undefined,
      channel_id: 7,
      billable_upstream_model: undefined,
      origin_model: undefined,
      user_group: undefined,
      using_group: undefined,
      billing_source: undefined,
      status: undefined,
    }
    const queryKey =
      costAccountingQueryKeys.reportFilterOptions(committedParams)
    await act(async () => {
      await queryClient.getQueryCache().find({ queryKey })?.promise
      await new Promise<void>((resolve) =>
        browserWindow.queueMicrotask(resolve)
      )
    })

    assert.deepEqual(requests, [
      {
        url: '/api/cost-accounting/reports/filter-options',
        params: committedParams,
      },
    ])
    const queryData = queryClient.getQueryData(queryKey) as
      | Parameters<typeof normalizeProfitFilterOptions>[0]
      | undefined
    assert.deepEqual(
      normalizeProfitFilterOptions(queryData, { channelId: 7 }).channels,
      [{ value: '7', label: '7 - Primary' }]
    )

    const query = queryClient.getQueryCache().find({ queryKey })
    assert.equal(
      (query?.options as { staleTime?: number } | undefined)?.staleTime,
      30_000
    )
  } finally {
    api.get = originalGet
    await act(async () => root.unmount())
    container.remove()
    queryClient.clear()
  }
})

test('preserves committed values when the filter-options request fails', async () => {
  const originalGet = api.get
  api.get = (async () => {
    throw new Error('candidate request failed')
  }) as typeof api.get
  let renderedOptions: ReturnType<typeof useProfitFilterOptions> | undefined

  function Probe() {
    renderedOptions = useProfitFilterOptions(
      {
        tab: 'profit',
        channelId: 7,
        billableModel: 'vendor-model',
        originModel: 'client-model',
        userGroup: 'default',
        usingGroup: 'premium',
      },
      true
    )
    return null
  }

  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const container = browserWindow.document.createElement('div')
  browserWindow.document.body.append(container)
  const root = createRoot(container as unknown as Container)

  try {
    await act(async () => {
      root.render(
        createElement(
          QueryClientProvider,
          { client: queryClient },
          createElement(Probe)
        )
      )
    })
    await act(async () => {
      const queryPromise = queryClient.getQueryCache().getAll()[0]?.promise
      if (queryPromise) await queryPromise.catch(() => {})
    })

    assert.deepEqual(renderedOptions, {
      channels: [{ value: '7', label: '7' }],
      billableModels: [{ value: 'vendor-model', label: 'vendor-model' }],
      originModels: [{ value: 'client-model', label: 'client-model' }],
      userGroups: [{ value: 'default', label: 'default' }],
      usingGroups: [{ value: 'premium', label: 'premium' }],
    })
  } finally {
    await act(async () => root.unmount())
    container.remove()
    queryClient.clear()
    api.get = originalGet
  }
})
