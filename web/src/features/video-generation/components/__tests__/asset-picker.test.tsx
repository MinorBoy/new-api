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
import { act, useState } from 'react'
import type { Container, Root } from 'react-dom/client'
import { I18nextProvider } from 'react-i18next'

import { api } from '@/lib/api'

import type { Asset, AssetListResponse } from '../../../assets/types'

const browserWindow = new Window({ url: 'http://localhost/video-generation' })
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

const assets: Asset[] = [
  {
    id: 'asset-20260401123823-6d4x2',
    type: 'image',
    url: 'https://example.com/active.png',
    status: 'active',
    provider: 'secure',
    reference: 'asset://asset-20260401123823-6d4x2',
    created_at: 1_700_000_000,
    updated_at: 1_700_000_000,
  },
  {
    id: 'asset-20260401123924-a1b2c',
    type: 'image',
    url: 'https://example.com/processing.png',
    status: 'processing',
    provider: 'secure',
    created_at: 1_700_000_001,
    updated_at: 1_700_000_001,
  },
  {
    id: 'asset-20260401124025-z9y8x',
    type: 'image',
    url: 'https://example.com/failed.png',
    status: 'failed',
    provider: 'secure',
    created_at: 1_700_000_002,
    updated_at: 1_700_000_002,
  },
  {
    id: 'asset-20260401124126-q7w6e',
    type: 'image',
    url: 'https://example.com/second-active.png',
    status: 'active',
    provider: 'secure',
    reference: 'asset://asset-20260401124126-q7w6e',
    created_at: 1_700_000_003,
    updated_at: 1_700_000_003,
  },
]

const assetList: AssetListResponse = {
  success: true,
  data: { items: assets, total: assets.length, page: 1, page_size: 12 },
}

const originalAdapter = api.defaults.adapter
const { createRoot } = await import('react-dom/client')
const { AssetPicker } = await import('../asset-picker')
const i18n = createInstance()
await i18n.init({ lng: 'en', fallbackLng: 'en' })

type MountedPicker = {
  container: HTMLElement
  root: Root
  queryClient: QueryClient
  changes: string[][]
  requests: number[]
}

type AssetPageLoader = (
  page: number,
  requestCount: number
) => AssetListResponse | Promise<AssetListResponse>

async function mountPicker(
  limit = 9,
  pages: Record<number, AssetListResponse> = { 1: assetList },
  loadPage?: AssetPageLoader
): Promise<MountedPicker> {
  const requests: number[] = []
  api.defaults.adapter = async (config) => {
    const page = Number(config.params?.page ?? 1)
    requests.push(page)
    return {
      data: loadPage
        ? await loadPage(page, requests.length)
        : (pages[page] ?? pages[1]),
      status: 200,
      statusText: 'OK',
      headers: {},
      config,
    }
  }
  const container = browserWindow.document.createElement('div')
  browserWindow.document.body.append(container)
  const root = createRoot(container as unknown as Container)
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  })
  const changes: string[][] = []

  function Harness() {
    const [selectedIds, setSelectedIds] = useState<string[]>([])
    return (
      <AssetPicker
        selectedIds={selectedIds}
        limit={limit}
        onChange={(ids) => {
          changes.push(ids)
          setSelectedIds(ids)
        }}
      />
    )
  }

  await act(async () => {
    root.render(
      <I18nextProvider i18n={i18n}>
        <QueryClientProvider client={queryClient}>
          <Harness />
        </QueryClientProvider>
      </I18nextProvider>
    )
  })
  return {
    container: container as unknown as HTMLElement,
    root,
    queryClient,
    changes,
    requests,
  }
}

async function waitForButton(container: HTMLElement, label: string) {
  for (let attempt = 0; attempt < 25; attempt += 1) {
    const button = container.querySelector(
      `button[aria-label="${label}"]`
    ) as HTMLButtonElement | null
    if (button) return button
    await act(
      async () =>
        await new Promise<void>((resolve) => {
          browserWindow.setTimeout(resolve, 0)
        })
    )
  }
  assert.fail(`Timed out waiting for ${label}`)
}

async function waitForLink(container: HTMLElement, href: string) {
  for (let attempt = 0; attempt < 25; attempt += 1) {
    const link = container.querySelector(`a[href="${href}"]`)
    if (link) return link
    await act(
      async () =>
        await new Promise<void>((resolve) => {
          browserWindow.setTimeout(resolve, 0)
        })
    )
  }
  assert.fail(`Timed out waiting for link ${href}`)
}

async function unmountPicker(mounted: MountedPicker) {
  await act(async () => mounted.root.unmount())
  mounted.container.remove()
  mounted.queryClient.clear()
}

test('allows only active assets to be selected', async () => {
  const mounted = await mountPicker()
  const activeButton = await waitForButton(
    mounted.container,
    'Select asset asset-20260401123823-6d4x2'
  )
  const processingButton = await waitForButton(
    mounted.container,
    'Select asset asset-20260401123924-a1b2c'
  )
  const failedButton = await waitForButton(
    mounted.container,
    'Select asset asset-20260401124025-z9y8x'
  )

  assert.equal(activeButton.disabled, false)
  assert.equal(processingButton.disabled, true)
  assert.equal(failedButton.disabled, true)

  await act(async () => activeButton.click())

  assert.deepEqual(mounted.changes, [['asset-20260401123823-6d4x2']])
  assert.equal(activeButton.getAttribute('aria-pressed'), 'true')
  await unmountPicker(mounted)
})

test('keeps selected assets removable after the selection limit is reached', async () => {
  const mounted = await mountPicker(1)
  const first = await waitForButton(
    mounted.container,
    'Select asset asset-20260401123823-6d4x2'
  )
  const second = await waitForButton(
    mounted.container,
    'Select asset asset-20260401124126-q7w6e'
  )

  await act(async () => first.click())

  assert.equal(first.disabled, false)
  assert.equal(second.disabled, true)
  await act(async () => first.click())
  assert.deepEqual(mounted.changes, [['asset-20260401123823-6d4x2'], []])
  await unmountPicker(mounted)
})

test('updates the selected asset count after choosing an asset', async () => {
  const mounted = await mountPicker(9)
  const first = await waitForButton(
    mounted.container,
    'Select asset asset-20260401123823-6d4x2'
  )
  assert.match(mounted.container.textContent ?? '', /Selected 0 of 9/)

  await act(async () => first.click())

  assert.match(mounted.container.textContent ?? '', /Selected 1 of 9/)
  await unmountPicker(mounted)
})

test('keeps selections while moving between asset pages', async () => {
  const secondPageAsset: Asset = {
    ...assets[0],
    id: 'asset-20260401124227-r5t4y',
    url: 'https://example.com/page-two.png',
  }
  const mounted = await mountPicker(9, {
    1: {
      success: true,
      data: { items: [assets[0]], total: 13, page: 1, page_size: 12 },
    },
    2: {
      success: true,
      data: { items: [secondPageAsset], total: 13, page: 2, page_size: 12 },
    },
  })
  const first = await waitForButton(
    mounted.container,
    'Select asset asset-20260401123823-6d4x2'
  )
  await act(async () => first.click())

  const next = await waitForButton(mounted.container, 'Next page')
  await act(async () => next.click())
  const second = await waitForButton(
    mounted.container,
    'Select asset asset-20260401124227-r5t4y'
  )
  await act(async () => second.click())

  assert.deepEqual(mounted.requests, [1, 2])
  assert.deepEqual(mounted.changes.at(-1), [
    'asset-20260401123823-6d4x2',
    'asset-20260401124227-r5t4y',
  ])
  await unmountPicker(mounted)
})

test('shows a retry action after loading assets fails', async () => {
  let shouldFail = true
  const mounted = await mountPicker(9, {}, () => {
    if (shouldFail) throw new Error('asset request failed')
    return assetList
  })

  const retry = await waitForButton(mounted.container, 'Retry loading assets')
  shouldFail = false
  await act(async () => retry.click())

  await waitForButton(
    mounted.container,
    'Select asset asset-20260401123823-6d4x2'
  )
  assert.equal(mounted.requests.length, 2)
  await unmountPicker(mounted)
})

test('shows asset card placeholders while the first page is loading', async () => {
  let resolveRequest: (value: AssetListResponse) => void = () => undefined
  const loadingRequest = new Promise<AssetListResponse>((resolve) => {
    resolveRequest = resolve
  })
  const mounted = await mountPicker(9, {}, () => loadingRequest)

  try {
    const skeletons = mounted.container.querySelectorAll(
      '[data-slot="skeleton"]'
    )
    assert.equal(skeletons.length, 4)
  } finally {
    resolveRequest(assetList)
    await unmountPicker(mounted)
  }
})

test('shows a fallback when an asset preview image fails to load', async () => {
  const mounted = await mountPicker()
  await waitForButton(
    mounted.container,
    'Select asset asset-20260401123823-6d4x2'
  )
  const image = mounted.container.querySelector(
    'img[src="https://example.com/active.png"]'
  )
  assert.ok(image)

  await act(async () => {
    image.dispatchEvent(new Event('error', { bubbles: true }))
  })

  assert.ok(
    mounted.container.querySelector(
      '[role="img"][aria-label="Preview unavailable"]'
    )
  )
  await unmountPicker(mounted)
})

test('links to the asset library when the user has no assets', async () => {
  const mounted = await mountPicker(9, {
    1: {
      success: true,
      data: { items: [], total: 0, page: 1, page_size: 12 },
    },
  })

  const link = await waitForLink(mounted.container, '/assets')

  assert.match(link.textContent ?? '', /Open asset library/)
  assert.match(mounted.container.textContent ?? '', /No assets yet/)
  await unmountPicker(mounted)
})

after(() => {
  api.defaults.adapter = originalAdapter
  browserWindow.close()
})
