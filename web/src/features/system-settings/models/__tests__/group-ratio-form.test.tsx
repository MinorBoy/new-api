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
import test, { after } from 'node:test'

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { Window } from 'happy-dom'
import { createInstance } from 'i18next'
import type { Container } from 'react-dom/client'

const browserWindow = new Window({ url: 'http://localhost/' })
const browserGlobals = {
  window: browserWindow,
  document: browserWindow.document,
  navigator: browserWindow.navigator,
  customElements: browserWindow.customElements,
  Element: browserWindow.Element,
  HTMLElement: browserWindow.HTMLElement,
  HTMLButtonElement: browserWindow.HTMLButtonElement,
  Node: browserWindow.Node,
  Event: browserWindow.Event,
  MouseEvent: browserWindow.MouseEvent,
  KeyboardEvent: browserWindow.KeyboardEvent,
  MutationObserver: browserWindow.MutationObserver,
  ResizeObserver: browserWindow.ResizeObserver,
  IntersectionObserver: browserWindow.IntersectionObserver,
  matchMedia: browserWindow.matchMedia.bind(browserWindow),
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
for (const [key, value] of Object.entries(browserGlobals)) {
  Object.defineProperty(globalThis, key, {
    configurable: true,
    writable: true,
    value,
  })
}

const { act } = await import('react')
const { createRoot } = await import('react-dom/client')
const { useForm } = await import('react-hook-form')
const { I18nextProvider } = await import('react-i18next')
const { api } = await import('@/lib/api')
const { GroupRatioForm } = await import('../group-ratio-form')

const originalApiAdapter = api.defaults.adapter
api.defaults.adapter = async (config) => ({
  data: {
    success: true,
    data: {
      premium: {
        models: 1,
        matched_models: 1,
        targets: 1,
        matched_targets: 1,
        stale_exclusions: 0,
      },
    },
  },
  status: 200,
  statusText: 'OK',
  headers: {},
  config,
})

const i18n = createInstance()
await i18n.init({
  lng: 'en',
  fallbackLng: 'en',
  resources: { en: { translation: {} } },
})

type GroupFormValues = {
  GroupRatio: string
  TopupGroupRatio: string
  UserUsableGroups: string
  GroupGroupRatio: string
  AutoGroups: string
  DefaultUseAutoGroup: boolean
  GroupSpecialUsableGroup: string
  GroupStatus: string
  GroupRoutingRequirements: string
}

after(() => {
  api.defaults.adapter = originalApiAdapter
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

test('toggles adaptation when the visible field label is clicked', async () => {
  const container = browserWindow.document.createElement('div')
  browserWindow.document.body.append(container)
  const root = createRoot(container as unknown as Container)
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  let readValues: (() => GroupFormValues) | undefined
  const onSave = async () => {}

  function Fixture() {
    const form = useForm<GroupFormValues>({
      defaultValues: {
        GroupRatio: '{"premium":2}',
        TopupGroupRatio: '{}',
        UserUsableGroups: '{"premium":"Premium"}',
        GroupGroupRatio: '{}',
        AutoGroups: '[]',
        DefaultUseAutoGroup: false,
        GroupSpecialUsableGroup: '{}',
        GroupStatus: '{"premium":true}',
        GroupRoutingRequirements:
          '{"premium":{"status":"active","routing_source":"default"}}',
      },
    })
    readValues = form.getValues
    return (
      <I18nextProvider i18n={i18n}>
        <QueryClientProvider client={queryClient}>
          <GroupRatioForm form={form} onSave={onSave} isSaving={false} />
        </QueryClientProvider>
      </I18nextProvider>
    )
  }

  try {
    await act(async () => root.render(<Fixture />))
    const detailsButton = browserWindow.document.querySelector(
      'button[aria-label="Details"]'
    )
    assert.ok(detailsButton instanceof browserWindow.HTMLButtonElement)
    await act(async () => detailsButton.click())

    const adaptSwitch = browserWindow.document.querySelector(
      '[role="switch"][aria-label="Adapt from default"]'
    )
    assert.ok(adaptSwitch instanceof browserWindow.HTMLElement)
    assert.equal(adaptSwitch.getAttribute('aria-checked'), 'true')
    const adaptLabel = [
      ...browserWindow.document.querySelectorAll('button, label'),
    ].find(
      (element) =>
        element instanceof browserWindow.HTMLElement &&
        element.children.length === 0 &&
        element.textContent?.trim() === 'Adapt from default'
    )
    assert.ok(adaptLabel instanceof browserWindow.HTMLElement)
    assert.equal(adaptLabel.tagName, 'BUTTON')
    await act(async () => adaptLabel.click())

    assert.deepEqual(
      JSON.parse(readValues?.().GroupRoutingRequirements ?? '{}'),
      {}
    )
    assert.equal(adaptSwitch.getAttribute('aria-checked'), 'false')
  } finally {
    await act(async () => root.unmount())
    queryClient.clear()
    container.remove()
  }
})
