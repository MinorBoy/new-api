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

import type { SeedanceVideoRequest } from '../types'

const browserWindow = new Window({ url: 'http://localhost/video-generation' })
browserWindow.document.write('<!doctype html><html><body></body></html>')
browserWindow.document.close()
Object.assign(globalThis as Record<string, unknown>, {
  window: browserWindow,
  document: browserWindow.document,
  navigator: browserWindow.navigator,
  Element: browserWindow.Element,
  HTMLElement: browserWindow.HTMLElement,
  HTMLButtonElement: browserWindow.HTMLButtonElement,
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

const originalAdapter = api.defaults.adapter
const assetRequests: Array<{ authToken?: string }> = []
api.defaults.adapter = async (config) => {
  let data: unknown
  if (config.url?.startsWith('/api/token/?')) {
    data = {
      success: true,
      data: {
        items: [
          {
            id: 7,
            name: 'Video key seven',
            key: 'sk-7**********ven',
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
            name: 'Video key eight',
            key: 'sk-8********ight',
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
        ],
        total: 1,
        page: 1,
        page_size: 100,
      },
    }
  } else if (config.url === '/api/token/7/key') {
    data = { success: true, data: { key: 'sk-seven' } }
  } else if (config.url === '/api/token/8/key') {
    data = { success: true, data: { key: 'sk-eight' } }
  } else if (config.url === '/api/user/models') {
    data = {
      success: true,
      data: ['doubao-seedance-2-0-260128', 'doubao-seedance-2-5-260628'],
    }
  } else if (config.url === '/api/v3/assets') {
    assetRequests.push({ authToken: config.authToken })
    const asset =
      config.authToken === 'sk-eight'
        ? {
            id: 'asset-20260401129959-eight',
            type: 'image',
            url: 'https://example.com/eight.png',
            status: 'active',
            provider: 'secure',
            reference: 'asset://asset-20260401129959-eight',
            created_at: 1_700_000_001,
            updated_at: 1_700_000_001,
          }
        : {
            id: 'asset-20260401123823-6d4x2',
            type: 'image',
            url: 'https://example.com/character.png',
            status: 'active',
            provider: 'secure',
            reference: 'asset://asset-20260401123823-6d4x2',
            created_at: 1_700_000_000,
            updated_at: 1_700_000_000,
          }
    data = {
      success: true,
      data: {
        items: [asset],
        total: 1,
        page: 1,
        page_size: 12,
      },
    }
  } else {
    throw new Error(`Unexpected request: ${config.url}`)
  }

  return {
    data,
    status: 200,
    statusText: 'OK',
    headers: {},
    config,
  }
}

const { createRoot } = await import('react-dom/client')
const { VideoGeneration } = await import('../index')
const i18n = createInstance()
await i18n.init({ lng: 'en', fallbackLng: 'en' })

type MountedPage = {
  container: HTMLElement
  root: Root
  queryClient: QueryClient
}

async function mountPage(): Promise<MountedPage> {
  assetRequests.length = 0
  const container = browserWindow.document.createElement('div')
  browserWindow.document.body.append(container)
  const root = createRoot(container as unknown as Container)
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })

  await act(async () => {
    root.render(
      <I18nextProvider i18n={i18n}>
        <QueryClientProvider client={queryClient}>
          <VideoGeneration />
        </QueryClientProvider>
      </I18nextProvider>
    )
  })

  return {
    container: container as unknown as HTMLElement,
    root,
    queryClient,
  }
}

async function waitFor<T>(find: () => T | null | undefined): Promise<T> {
  for (let attempt = 0; attempt < 40; attempt += 1) {
    const result = find()
    if (result) return result
    await act(
      async () =>
        await new Promise<void>((resolve) => {
          browserWindow.setTimeout(resolve, 0)
        })
    )
  }
  assert.fail('Timed out waiting for page state')
}

function findButton(container: HTMLElement, text: string) {
  return [...container.querySelectorAll('button')].find((button) =>
    button.textContent?.includes(text)
  )
}

function readRequest(container: HTMLElement): SeedanceVideoRequest {
  const preview = container.querySelector('pre')
  assert.ok(preview?.textContent)
  return JSON.parse(preview.textContent) as SeedanceVideoRequest
}

async function selectApiKey(container: HTMLElement, id: string) {
  const apiKey = await waitFor(() => {
    const select = container.querySelector(
      'select[aria-label="API key"]'
    ) as HTMLSelectElement | null
    return select?.querySelector(`option[value="${id}"]`) ? select : null
  })
  await act(async () => {
    apiKey.value = id
    apiKey.dispatchEvent(new Event('change', { bubbles: true }))
  })
  await waitFor(() =>
    container.querySelector(
      'select[aria-label="Model"] option[value="doubao-seedance-2-5-260628"]'
    )
  )
}

async function chooseApiKey(container: HTMLElement) {
  await selectApiKey(container, '7')
}

async function unmountPage(mounted: MountedPage) {
  await act(async () => mounted.root.unmount())
  mounted.container.remove()
  mounted.queryClient.clear()
}

test('keeps the request preview consistent across asset and model modes', async () => {
  const mounted = await mountPage()
  await chooseApiKey(mounted.container)
  const assetLibrary = await waitFor(() =>
    findButton(mounted.container, 'Asset library')
  )

  await act(async () => assetLibrary.click())
  const asset = await waitFor(
    () =>
      mounted.container.querySelector(
        'button[aria-label="Select asset asset-20260401123823-6d4x2"]'
      ) as HTMLButtonElement | null
  )
  let request = readRequest(mounted.container)
  assert.equal(
    request.content.some((item) => item.type === 'image_url'),
    false
  )
  assert.equal(
    request.content.some((item) => item.type === 'video_url'),
    false
  )
  assert.equal(
    request.content.some((item) => item.type === 'audio_url'),
    true
  )

  await act(async () => asset.click())
  request = readRequest(mounted.container)
  assert.equal(
    request.content.some(
      (item) =>
        item.type === 'image_url' &&
        item.image_url.url === 'asset://asset-20260401123823-6d4x2'
    ),
    true
  )

  const model = mounted.container.querySelector(
    'select[aria-label="Model"]'
  ) as HTMLSelectElement
  await act(async () => {
    model.value = 'doubao-seedance-2-5-260628'
    model.dispatchEvent(new Event('change', { bubbles: true }))
  })
  const disabledAssetLibrary = await waitFor(() => {
    const button = findButton(mounted.container, 'Asset library')
    return button?.disabled ? button : null
  })

  assert.equal(disabledAssetLibrary.disabled, true)
  assert.match(mounted.container.textContent ?? '', /Images 0–30/)
  assert.match(mounted.container.textContent ?? '', /Videos 0–10/)
  assert.match(mounted.container.textContent ?? '', /Audio 0–10/)
  request = readRequest(mounted.container)
  assert.equal(
    request.content.some(
      (item) =>
        item.type === 'image_url' && item.image_url.url.startsWith('asset://')
    ),
    false
  )
  await unmountPage(mounted)
})

test('clears selected assets and reloads the asset library when the API key changes', async () => {
  const mounted = await mountPage()
  await chooseApiKey(mounted.container)
  const assetLibrary = await waitFor(() =>
    findButton(mounted.container, 'Asset library')
  )
  await act(async () => assetLibrary.click())
  const asset = await waitFor(
    () =>
      mounted.container.querySelector(
        'button[aria-label="Select asset asset-20260401123823-6d4x2"]'
      ) as HTMLButtonElement | null
  )
  await act(async () => asset.click())
  assert.equal(
    readRequest(mounted.container).content.some(
      (item) =>
        item.type === 'image_url' &&
        item.image_url.url === 'asset://asset-20260401123823-6d4x2'
    ),
    true
  )

  await selectApiKey(mounted.container, '8')
  await waitFor(
    () =>
      mounted.container.querySelector(
        'button[aria-label="Select asset asset-20260401129959-eight"]'
      ) as HTMLButtonElement | null
  )

  assert.equal(
    readRequest(mounted.container).content.some(
      (item) =>
        item.type === 'image_url' && item.image_url.url.startsWith('asset://')
    ),
    false
  )
  assert.equal(assetRequests.at(-1)?.authToken, 'sk-eight')
  await unmountPage(mounted)
})

after(() => {
  api.defaults.adapter = originalAdapter
  browserWindow.close()
})
