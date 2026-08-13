// @ts-expect-error Bun supplies mock.module at test runtime, but the frontend
// typecheck intentionally only includes Node's test declarations.
import { mock } from 'bun:test'
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

import type { SecureAssetSettingsResponse } from '../../types'

const browserWindow = new Window({ url: 'http://localhost/' })
browserWindow.document.write('<!doctype html><html><body></body></html>')
browserWindow.document.close()
Object.assign(globalThis as Record<string, unknown>, {
  window: browserWindow,
  document: browserWindow.document,
  navigator: browserWindow.navigator,
  Element: browserWindow.Element,
  HTMLElement: browserWindow.HTMLElement,
  HTMLSelectElement: browserWindow.HTMLSelectElement,
  Node: browserWindow.Node,
  Event: browserWindow.Event,
  MouseEvent: browserWindow.MouseEvent,
  MutationObserver: browserWindow.MutationObserver,
  ResizeObserver: browserWindow.ResizeObserver,
  getComputedStyle: browserWindow.getComputedStyle.bind(browserWindow),
  requestAnimationFrame: browserWindow.requestAnimationFrame.bind(browserWindow),
  cancelAnimationFrame: browserWindow.cancelAnimationFrame.bind(browserWindow),
  IS_REACT_ACT_ENVIRONMENT: true,
})

const settings: SecureAssetSettingsResponse = {
  success: true,
  message: '',
  data: {
    default_channel_id: 7,
    channels: [
      { id: 7, name: 'Secure enterprise', status: 1, default: true },
      { id: 8, name: 'Secure enterprise backup', status: 1, default: false },
    ],
  },
}
let savedChannelID: number | undefined
mock.module('@/features/system-settings/api', () => ({
  getSecureAssetSettings: async () => settings,
  updateSecureAssetSettings: async (channelID: number) => {
    savedChannelID = channelID
    return settings
  },
}))

const { createRoot } = await import('react-dom/client')
const { SettingsPageProvider } =
  await import('../../components/settings-page-context')
const { SecureAssetSection } = await import('../secure-asset-section')
const i18n = createInstance()
await i18n.init({ lng: 'en', fallbackLng: 'en', resources: { en: { translation: {} } } })

test('renders Secure role asset channel and saves selected channel', async () => {
  const container = browserWindow.document.createElement('div')
  const actions = browserWindow.document.createElement('div')
  browserWindow.document.body.append(container, actions)
  const root = createRoot(container as unknown as Container)
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  queryClient.setQueryData(['system-settings', 'secure-assets'], settings)

  await act(async () => {
    root.render(
      <I18nextProvider i18n={i18n}>
        <QueryClientProvider client={queryClient}>
          <SettingsPageProvider actionsContainer={actions as unknown as HTMLDivElement} suppressSectionHeader={false}>
            <SecureAssetSection />
          </SettingsPageProvider>
        </QueryClientProvider>
      </I18nextProvider>
    )
    for (let index = 0; index < 12; index += 1) await Promise.resolve()
  })

  const select = container.querySelector('select[aria-label="Secure asset channel"]') as HTMLSelectElement | null
  assert.ok(select)
  assert.equal(select.value, '7')
  assert.equal(select.options.length, 3)
  const save = [...actions.querySelectorAll('button')].find((button) => button.textContent?.includes('Save Secure asset settings'))
  assert.ok(save)

  await act(async () => {
    select.value = '8'
    select.dispatchEvent(new browserWindow.Event('change', { bubbles: true }) as unknown as Event)
    save?.click()
    await Promise.resolve()
  })
  assert.equal(savedChannelID, 8)

  await act(async () => root.unmount())
  container.remove()
  actions.remove()
  queryClient.clear()
})

after(() => browserWindow.close())
