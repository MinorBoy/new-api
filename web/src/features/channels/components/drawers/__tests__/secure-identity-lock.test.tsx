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

import type { QueryClient as QueryClientType } from '@tanstack/react-query'
import { Window } from 'happy-dom'
import { createInstance } from 'i18next'
import type { Container, Root } from 'react-dom/client'

import type { Channel } from '../../../types'

const browserWindow = new Window({ url: 'http://localhost/' })
browserWindow.document.write('<!doctype html><html><body></body></html>')
Object.defineProperty(browserWindow.document, 'compatMode', {
  value: 'CSS1Compat',
})
const browserGlobals = {
  window: browserWindow,
  document: browserWindow.document,
  navigator: browserWindow.navigator,
  customElements: browserWindow.customElements,
  localStorage: browserWindow.localStorage,
  Element: browserWindow.Element,
  HTMLElement: browserWindow.HTMLElement,
  HTMLButtonElement: browserWindow.HTMLButtonElement,
  HTMLFieldSetElement: browserWindow.HTMLFieldSetElement,
  HTMLInputElement: browserWindow.HTMLInputElement,
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
  api.defaults.adapter = originalApiAdapter
  browserWindow.close()
})

const { QueryClient, QueryClientProvider } =
  await import('@tanstack/react-query')
const { act } = await import('react')
const { createRoot } = await import('react-dom/client')
const { I18nextProvider } = await import('react-i18next')
const { api } = await import('@/lib/api')
const originalApiAdapter = api.defaults.adapter
api.defaults.adapter = async (config) => ({
  data: { success: true, data: { enabled: false } },
  status: 200,
  statusText: 'OK',
  headers: {},
  config,
})
const { TooltipProvider } = await import('@/components/ui/tooltip')
const { ROLE } = await import('@/lib/roles')
const { useAuthStore } = await import('@/stores/auth-store')
const { channelsQueryKeys } = await import('../../../lib/channel-actions')
const { ChannelsProvider } = await import('../../channels-provider')
const { ChannelMutateDrawer } = await import('../channel-mutate-drawer')

beforeEach(() => {
  browserWindow.document.body.replaceChildren()
  browserWindow.localStorage.clear()
  useAuthStore.getState().auth.setUser({
    id: 1,
    username: 'admin',
    role: ROLE.SUPER_ADMIN,
  })
})

const i18n = createInstance()
await i18n.init({
  lng: 'en',
  fallbackLng: 'en',
  resources: { en: { translation: {} } },
})

const secureChannel: Channel = {
  id: 66,
  type: 66,
  key: '',
  status: 1,
  name: 'Secure discount',
  weight: 0,
  created_time: 1,
  test_time: 1,
  response_time: 0,
  base_url: 'https://token.secure-skill.com',
  other: '',
  balance: 0,
  balance_updated_time: 1,
  models: 'video-2.0-pro',
  group: 'default',
  used_quota: 0,
  priority: 0,
  auto_ban: 1,
  other_info: '',
  remark: '',
  max_input_tokens: 0,
  channel_info: {
    is_multi_key: false,
    multi_key_size: 0,
    multi_key_polling_index: 0,
    multi_key_mode: 'random',
  },
  settings: '{"secure_video_group":"discount"}',
  routing_target_count: 0,
}

function createQueryClient(): QueryClientType {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
        staleTime: Number.POSITIVE_INFINITY,
        refetchOnMount: false,
      },
      mutations: { retry: false },
    },
  })
  queryClient.setQueryData(channelsQueryKeys.detail(secureChannel.id), {
    success: true,
    data: secureChannel,
  })
  queryClient.setQueryData(['groups'], {
    success: true,
    data: ['default'],
  })
  queryClient.setQueryData(['channel_models'], {
    success: true,
    data: [{ id: 'video-2.0-pro' }],
  })
  queryClient.setQueryData(['prefill_groups', 'model'], {
    success: true,
    data: [],
  })
  return queryClient
}

async function mountDrawer(): Promise<{
  root: Root
  container: { remove(): void }
  queryClient: QueryClientType
}> {
  const queryClient = createQueryClient()
  const container = browserWindow.document.createElement('div')
  browserWindow.document.body.append(container)
  const root = createRoot(container as unknown as Container)

  await act(async () => {
    root.render(
      <I18nextProvider i18n={i18n}>
        <QueryClientProvider client={queryClient}>
          <TooltipProvider>
            <ChannelsProvider>
              <ChannelMutateDrawer
                open
                currentRow={secureChannel}
                onOpenChange={() => {}}
              />
            </ChannelsProvider>
          </TooltipProvider>
        </QueryClientProvider>
      </I18nextProvider>
    )
  })
  await act(async () => undefined)
  return { root, container, queryClient }
}

async function unmountDrawer(mounted: {
  root: Root
  container: { remove(): void }
  queryClient: QueryClientType
}) {
  await act(async () => mounted.root.unmount())
  mounted.queryClient.clear()
  mounted.container.remove()
}

test('editing a Secure channel disables its type and video group controls', async () => {
  const mounted = await mountDrawer()
  let hasInvalidApiKeyDescriptionContent = false
  try {
    const labels = [...browserWindow.document.querySelectorAll('label')]
    const typeLabel = labels.find(
      (label) => label.textContent?.trim() === 'Type *'
    )
    assert.ok(typeLabel, browserWindow.document.body.innerHTML)
    const typeFieldset = typeLabel.closest('fieldset')
    assert.ok(typeFieldset instanceof browserWindow.HTMLFieldSetElement)
    assert.equal(typeFieldset.disabled, true)

    const groupLabel = labels.find(
      (label) => label.textContent?.trim() === 'Secure video group'
    )
    assert.ok(groupLabel)
    const groupFormItem = groupLabel.closest('[data-slot="form-item"]')
    assert.ok(groupFormItem)
    const groupTrigger = groupFormItem.querySelector('button[role="combobox"]')
    assert.ok(groupTrigger, groupFormItem.outerHTML)
    assert.equal(groupTrigger.hasAttribute('disabled'), true)

    const apiKeyLabel = labels.find(
      (label) => label.textContent?.trim() === 'API Key *'
    )
    assert.ok(apiKeyLabel)
    const apiKeyFormItem = apiKeyLabel.closest('[data-slot="form-item"]')
    assert.ok(apiKeyFormItem)
    const description = apiKeyFormItem.querySelector(
      '[data-slot="form-description"]'
    )
    assert.ok(description)
    hasInvalidApiKeyDescriptionContent =
      description.querySelector(':scope > div') !== null
  } finally {
    await unmountDrawer(mounted)
  }
  assert.equal(hasInvalidApiKeyDescriptionContent, false)
})
