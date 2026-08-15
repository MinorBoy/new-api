/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.
*/
import assert from 'node:assert/strict'
import test, { after } from 'node:test'

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { Window } from 'happy-dom'
import { createInstance } from 'i18next'
import { act } from 'react'
import type { Container, Root } from 'react-dom/client'
import { I18nextProvider } from 'react-i18next'

import { api } from '@/lib/api'

import type { ApiKey } from '../../keys/types'
import type { AssetListResponse } from '../types'

const browserWindow = new Window({ url: 'http://localhost/assets' })
browserWindow.document.open()
browserWindow.document.write('<!doctype html><html><body></body></html>')
browserWindow.document.close()
Object.defineProperty(browserWindow.document, 'compatMode', {
  value: 'CSS1Compat',
})
Object.assign(globalThis as Record<string, unknown>, {
  window: browserWindow,
  document: browserWindow.document,
  navigator: browserWindow.navigator,
  Element: browserWindow.Element,
  HTMLElement: browserWindow.HTMLElement,
  HTMLButtonElement: browserWindow.HTMLButtonElement,
  HTMLInputElement: browserWindow.HTMLInputElement,
  HTMLSelectElement: browserWindow.HTMLSelectElement,
  Node: browserWindow.Node,
  Event: browserWindow.Event,
  MouseEvent: browserWindow.MouseEvent,
  MutationObserver: browserWindow.MutationObserver,
  ResizeObserver: browserWindow.ResizeObserver,
  getComputedStyle: browserWindow.getComputedStyle.bind(browserWindow),
  requestAnimationFrame:
    browserWindow.requestAnimationFrame.bind(browserWindow),
  cancelAnimationFrame: browserWindow.cancelAnimationFrame.bind(browserWindow),
  IS_REACT_ACT_ENVIRONMENT: true,
})

const apiKeys: ApiKey[] = [
  {
    id: 7,
    name: 'Key Seven',
    key: 'sk-7**********even',
    status: 1,
    remain_quota: 0,
    used_quota: 0,
    unlimited_quota: true,
    expired_time: -1,
    created_time: 0,
    accessed_time: 0,
    group: 'default',
    cross_group_retry: false,
    model_limits_enabled: false,
    model_limits: '',
    allow_ips: '',
  },
  {
    id: 8,
    name: 'Key Eight',
    key: 'sk-8**********ight',
    status: 1,
    remain_quota: 0,
    used_quota: 0,
    unlimited_quota: true,
    expired_time: -1,
    created_time: 0,
    accessed_time: 0,
    group: 'default',
    cross_group_retry: false,
    model_limits_enabled: false,
    model_limits: '',
    allow_ips: '',
  },
]

function assetList(id: string): AssetListResponse {
  return {
    success: true,
    data: {
      items: [
        {
          id,
          type: 'image',
          url: `https://example.com/${id}.png`,
          status: 'active',
          provider: 'secure',
          reference: `asset://${id}`,
          created_at: 1_700_000_000,
          updated_at: 1_700_000_000,
        },
      ],
      total: 1,
      page: 1,
      page_size: 20,
    },
  }
}

type AssetRequest = { authToken?: string; method?: string }

type MountedAssets = {
  assetRequests: AssetRequest[]
  container: HTMLElement
  queryClient: QueryClient
  root: Root
}

const originalAdapter = api.defaults.adapter
const { createRoot } = await import('react-dom/client')
const { Assets } = await import('../index')
const i18n = createInstance()
await i18n.init({ lng: 'en', fallbackLng: 'en' })

async function waitFor(check: () => boolean): Promise<void> {
  for (let attempt = 0; attempt < 30; attempt += 1) {
    if (check()) return
    await act(
      async () =>
        await new Promise<void>((resolve) => {
          browserWindow.setTimeout(resolve, 0)
        })
    )
  }
  assert.fail('Timed out waiting for expected asset page state')
}

async function mountAssets(): Promise<MountedAssets> {
  const assetRequests: AssetRequest[] = []
  api.defaults.adapter = async (config) => {
    if (config.url?.startsWith('/api/token/?')) {
      return {
        data: {
          success: true,
          data: { items: apiKeys, total: 2, page: 1, page_size: 100 },
        },
        status: 200,
        statusText: 'OK',
        headers: {},
        config,
      }
    }
    if (
      config.url === '/api/token/7/key' ||
      config.url === '/api/token/8/key'
    ) {
      const key = config.url.includes('/7/') ? 'sk-seven' : 'sk-eight'
      return {
        data: { success: true, data: { key } },
        status: 200,
        statusText: 'OK',
        headers: {},
        config,
      }
    }
    if (config.url === '/api/v3/assets') {
      assetRequests.push({
        authToken: config.authToken,
        method: config.method,
      })
      const id =
        config.authToken === 'sk-eight' ? 'asset-for-eight' : 'asset-for-seven'
      return {
        data: assetList(id),
        status: 200,
        statusText: 'OK',
        headers: {},
        config,
      }
    }
    throw new Error(`Unexpected request: ${config.method} ${config.url}`)
  }

  const container = browserWindow.document.createElement('div')
  browserWindow.document.body.append(container)
  const root = createRoot(container as unknown as Container)
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false, staleTime: Infinity },
      mutations: { retry: false },
    },
  })

  await act(async () => {
    root.render(
      <I18nextProvider i18n={i18n}>
        <QueryClientProvider client={queryClient}>
          <Assets />
        </QueryClientProvider>
      </I18nextProvider>
    )
  })
  await waitFor(() => {
    const select = container.querySelector('#asset-api-key') as unknown as {
      options: { length: number }
    } | null
    return select?.options.length === 3
  })
  return {
    assetRequests,
    container: container as unknown as HTMLElement,
    queryClient,
    root,
  }
}

async function selectKey(mounted: MountedAssets, tokenID: string) {
  const select =
    mounted.container.querySelector<HTMLSelectElement>('#asset-api-key')
  assert.ok(select)
  await act(async () => {
    select.value = tokenID
    select.dispatchEvent(new Event('change', { bubbles: true }))
  })
}

async function unmountAssets(mounted: MountedAssets) {
  await act(async () => mounted.root.unmount())
  mounted.container.remove()
  mounted.queryClient.clear()
  api.defaults.adapter = originalAdapter
}

test('does not load assets and disables creation until an API key is selected', async () => {
  const mounted = await mountAssets()
  try {
    const input = mounted.container.querySelector<HTMLInputElement>(
      'input[aria-label="Public image URL"]'
    )
    const submit = mounted.container.querySelector<HTMLButtonElement>(
      'button[type="submit"]'
    )
    assert.ok(input)
    assert.ok(submit)
    assert.equal(mounted.assetRequests.length, 0)
    assert.equal(input.disabled, true)
    assert.equal(submit.disabled, true)
    assert.match(
      mounted.container.textContent ?? '',
      /Select an API key to view assets/
    )
  } finally {
    await unmountAssets(mounted)
  }
})

test('loads an isolated asset list for each selected API key', async () => {
  const mounted = await mountAssets()
  try {
    await selectKey(mounted, '7')
    await waitFor(() =>
      (mounted.container.textContent ?? '').includes('asset-for-seven')
    )
    assert.equal(mounted.assetRequests[0]?.authToken, 'sk-seven')

    await selectKey(mounted, '8')
    assert.doesNotMatch(mounted.container.textContent ?? '', /asset-for-seven/)
    await waitFor(() =>
      (mounted.container.textContent ?? '').includes('asset-for-eight')
    )
    assert.deepEqual(
      mounted.assetRequests.map((request) => request.authToken),
      ['sk-seven', 'sk-eight']
    )
  } finally {
    await unmountAssets(mounted)
  }
})

after(() => {
  api.defaults.adapter = originalAdapter
  browserWindow.close()
})
