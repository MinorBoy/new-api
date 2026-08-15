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
import type { Container } from 'react-dom/client'
import { I18nextProvider } from 'react-i18next'

import type { AssetListResponse } from '../types'

const browserWindow = new Window({ url: 'http://localhost/assets' })
browserWindow.document.open()
browserWindow.document.write('<!doctype html><html><body></body></html>')
browserWindow.document.close()
Object.defineProperty(browserWindow.document, 'compatMode', {
  value: 'CSS1Compat',
})
let writeText: (value: string) => Promise<void> = async () => undefined
Object.defineProperty(browserWindow.navigator, 'clipboard', {
  configurable: true,
  value: { writeText: (value: string) => writeText(value) },
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

const assetList: AssetListResponse = {
  success: true,
  data: {
    items: [
      {
        id: 'asset-test',
        type: 'image',
        url: 'https://example.com/character.png',
        status: 'active',
        provider: 'secure',
        reference: 'asset://asset-test',
        created_at: 1_700_000_000,
        updated_at: 1_700_000_000,
      },
    ],
    total: 1,
    page: 1,
    page_size: 20,
  },
}

const { createRoot } = await import('react-dom/client')
const { Assets } = await import('../index')
const i18n = createInstance()
await i18n.init({
  lng: 'en',
  fallbackLng: 'en',
  resources: {
    en: {
      translation: {
        'Copy to clipboard': 'Copy to clipboard',
        Copied: 'Copied',
      },
    },
  },
})

test('copies the asset ID when its ID button is clicked', async () => {
  const copiedValues: string[] = []
  writeText = async (value: string) => {
    copiedValues.push(value)
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
  queryClient.setQueryData(['role-assets'], assetList)

  await act(async () => {
    root.render(
      <I18nextProvider i18n={i18n}>
        <QueryClientProvider client={queryClient}>
          <Assets />
        </QueryClientProvider>
      </I18nextProvider>
    )
    for (let index = 0; index < 12; index += 1) await Promise.resolve()
  })

  const copyButton = browserWindow.document.querySelector(
    'button[aria-label="Copy to clipboard"]'
  ) as HTMLButtonElement | null
  assert.ok(copyButton)
  assert.equal(copyButton.textContent?.includes('asset-test'), true)

  await act(async () => copyButton.click())

  assert.deepEqual(copiedValues, ['asset-test'])

  await act(async () => root.unmount())
  container.remove()
  queryClient.clear()
})

after(() => browserWindow.close())
