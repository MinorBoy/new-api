// @ts-expect-error Bun supplies mock.module at test runtime, but the frontend
// typecheck intentionally only includes Node's test declarations.
import { mock } from 'bun:test'
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

import type {
  ObjectStorageSettings,
  ObjectStorageSettingsRequest,
} from '../../types'

const browserWindow = new Window({ url: 'http://localhost/' })
browserWindow.document.write('<!doctype html><html><body></body></html>')
browserWindow.document.close()

Object.assign(globalThis as Record<string, unknown>, {
  window: browserWindow,
  document: browserWindow.document,
  navigator: browserWindow.navigator,
  Element: browserWindow.Element,
  HTMLElement: browserWindow.HTMLElement,
  HTMLInputElement: browserWindow.HTMLInputElement,
  HTMLTextAreaElement: browserWindow.HTMLTextAreaElement,
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

const savedSettings: ObjectStorageSettings = {
  enabled: false,
  endpoint: 'https://s3.internal.example.com',
  public_endpoint: 'https://s3.example.com',
  region: 'us-east-1',
  bucket: 'videos',
  access_key_id: 'access-key',
  secret_configured: true,
  use_path_style: false,
  max_video_size_mb: 512,
  expires_seconds: 86400,
  transfer_domain_whitelist: ['provider.example.com'],
  no_transfer_domain_blacklist: ['official.example.com'],
}

let savedRequest: ObjectStorageSettingsRequest | undefined
let testedRequest: ObjectStorageSettingsRequest | undefined

mock.module('@/features/system-settings/api', () => ({
  getObjectStorageSettings: async () => ({
    success: true,
    message: '',
    data: savedSettings,
  }),
  updateObjectStorageSettings: async (
    request: ObjectStorageSettingsRequest
  ) => {
    savedRequest = request
    return { success: true, message: '', data: savedSettings }
  },
  testObjectStorageSettings: async (request: ObjectStorageSettingsRequest) => {
    testedRequest = request
    return { success: true, message: '', data: { connected: true } }
  },
}))

const { createRoot } = await import('react-dom/client')
const { SettingsPageProvider } =
  await import('../../components/settings-page-context')
const { ObjectStorageSection } = await import('../object-storage-section')

const i18n = createInstance()
await i18n.init({
  lng: 'en',
  fallbackLng: 'en',
  resources: { en: { translation: {} } },
})

type Mounted = {
  actions: InstanceType<typeof browserWindow.HTMLDivElement>
  container: InstanceType<typeof browserWindow.HTMLDivElement>
  queryClient: QueryClient
  root: Root
}

async function renderSection(): Promise<Mounted> {
  const container = browserWindow.document.createElement('div')
  const actions = browserWindow.document.createElement('div')
  browserWindow.document.body.append(container, actions)
  const root = createRoot(container as unknown as Container)
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false, staleTime: Infinity },
      mutations: { retry: false },
    },
  })
  queryClient.setQueryData(['system-settings', 'object-storage'], {
    success: true,
    message: '',
    data: savedSettings,
  })

  await act(async () => {
    root.render(
      <I18nextProvider i18n={i18n}>
        <QueryClientProvider client={queryClient}>
          <SettingsPageProvider
            actionsContainer={actions as unknown as HTMLDivElement}
            suppressSectionHeader={false}
          >
            <ObjectStorageSection />
          </SettingsPageProvider>
        </QueryClientProvider>
      </I18nextProvider>
    )
    for (let index = 0; index < 4; index += 1) await Promise.resolve()
  })

  return { actions, container, queryClient, root }
}

async function unmount(mounted: Mounted) {
  await act(async () => mounted.root.unmount())
  mounted.container.remove()
  mounted.actions.remove()
  mounted.queryClient.clear()
}

function setInputValue(
  input: HTMLInputElement | HTMLTextAreaElement,
  value: string
) {
  const prototype =
    input instanceof browserWindow.HTMLTextAreaElement
      ? browserWindow.HTMLTextAreaElement.prototype
      : browserWindow.HTMLInputElement.prototype
  const valueSetter = Object.getOwnPropertyDescriptor(prototype, 'value')?.set
  assert.ok(valueSetter)
  valueSetter.call(input, value)
  input.dispatchEvent(
    new browserWindow.Event('input', { bubbles: true }) as unknown as Event
  )
}

beforeEach(() => {
  savedRequest = undefined
  testedRequest = undefined
})

after(() => browserWindow.close())

test('renders the object storage settings as one grouped form', async () => {
  const mounted = await renderSection()

  assert.match(mounted.container.textContent ?? '', /Connection/)
  assert.match(mounted.container.textContent ?? '', /Credentials/)
  assert.match(mounted.container.textContent ?? '', /Link and limits/)
  assert.match(mounted.container.textContent ?? '', /Domain rules/)
  const endpoint = mounted.container.querySelector(
    'input[aria-label="Endpoint"]'
  ) as unknown as HTMLInputElement
  const initialWhitelist = mounted.container.querySelector(
    'textarea[aria-label="Transfer domain whitelist"]'
  ) as unknown as HTMLTextAreaElement
  assert.equal(endpoint.value, savedSettings.endpoint)
  assert.equal(initialWhitelist.value, 'provider.example.com')

  await unmount(mounted)
})

test('explicitly clears the stored secret and normalizes domain lines on save', async () => {
  const mounted = await renderSection()
  const whitelist = mounted.container.querySelector(
    'textarea[aria-label="Transfer domain whitelist"]'
  ) as unknown as HTMLTextAreaElement
  const clearButton = [...mounted.container.querySelectorAll('button')].find(
    (button) => button.textContent?.includes('Clear stored secret')
  )
  assert.ok(whitelist)
  assert.ok(clearButton)

  await act(async () => {
    setInputValue(
      whitelist,
      ' Provider.Example.com\n*.media.example.com\nprovider.example.com '
    )
    clearButton.click()
  })

  const saveButton = [...mounted.actions.querySelectorAll('button')].find(
    (button) => button.textContent?.includes('Save object storage settings')
  )
  assert.ok(saveButton)

  await act(async () => {
    saveButton.click()
    await Promise.resolve()
  })

  assert.ok(savedRequest)
  assert.equal(savedRequest.clear_secret, true)
  assert.equal(savedRequest.secret_access_key, '')
  assert.deepEqual(savedRequest.transfer_domain_whitelist, [
    'provider.example.com',
    '*.media.example.com',
  ])

  await unmount(mounted)
})

test('tests the current unsaved connection settings without saving them', async () => {
  const mounted = await renderSection()
  const endpoint = mounted.container.querySelector(
    'input[aria-label="Endpoint"]'
  ) as unknown as HTMLInputElement
  const testButton = [...mounted.container.querySelectorAll('button')].find(
    (button) => button.textContent?.includes('Test connection')
  )
  assert.ok(endpoint)
  assert.ok(testButton)

  await act(async () => {
    setInputValue(endpoint, 'https://new-s3.example.com')
    testButton.click()
    await Promise.resolve()
  })

  assert.equal(testedRequest?.endpoint, 'https://new-s3.example.com')
  assert.equal(savedRequest, undefined)

  await unmount(mounted)
})
