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

import { Window } from 'happy-dom'
import { act } from 'react'
import type { Container, Root } from 'react-dom/client'

const browserWindow = new Window({ url: 'http://localhost/usage-logs/task' })
const matchMedia = (query: string) => ({
  matches: false,
  media: query,
  onchange: null,
  addEventListener: () => {},
  removeEventListener: () => {},
  addListener: () => {},
  removeListener: () => {},
  dispatchEvent: () => true,
})
Object.defineProperty(browserWindow, 'matchMedia', { value: matchMedia })

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
  matchMedia,
  IS_REACT_ACT_ENVIRONMENT: true,
}
const previousBrowserGlobals = Object.fromEntries(
  Object.keys(browserGlobals).map((key) => [
    key,
    Object.getOwnPropertyDescriptor(globalThis, key),
  ])
)
Object.assign(globalThis as Record<string, unknown>, browserGlobals)

const adminView = { current: true }
const searchState = { current: {} as Record<string, unknown> }
const lastNavigate = { current: null as Record<string, unknown> | null }

mock.module('@tanstack/react-router', () => ({
  getRouteApi: () => ({ useSearch: () => searchState.current }),
  useNavigate: () => (navigation: Record<string, unknown>) => {
    lastNavigate.current = navigation
  },
}))

mock.module('@/hooks/use-admin', () => ({
  useIsAdmin: () => adminView.current,
}))

mock.module('@/features/usage-logs/hooks/use-task-log-filter-options', () => ({
  useTaskLogFilterOptions: () => ({
    channelOptions: [
      { value: '29', label: '29 - paipu' },
      { value: '40', label: '40 - backup' },
    ],
    statusOptions: ['FAILURE', 'SUCCESS'],
    requestModelOptions: [
      { value: 'model-a', label: 'model-a' },
      { value: 'model-b', label: 'model-b' },
    ],
    userOptions: [
      { value: '10', label: '10 - alice' },
      { value: '11', label: '11 - bob' },
    ],
  }),
}))

const cancelAutoSearch = () => {}

mock.module('@/features/usage-logs/hooks/use-auto-search', () => ({
  useAutoSearch: <TValue,>(onSearch: (value: TValue) => void) => ({
    schedule: onSearch,
    flush: onSearch,
    cancel: cancelAutoSearch,
  }),
}))

const { createRoot } = await import('react-dom/client')
const { QueryClient, QueryClientProvider } =
  await import('@tanstack/react-query')
const { createInstance } = await import('i18next')
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const { UsageLogsProvider } = await import('../usage-logs-provider')
const { TaskLogsFilterBar } = await import('../task-logs-filter-bar')

const i18n = createInstance()
await i18n.use(initReactI18next).init({
  lng: 'en',
  resources: { en: { translation: {} } },
})

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
  adminView.current = true
  searchState.current = {}
  lastNavigate.current = null
})

async function mountFilterBar() {
  const container = browserWindow.document.createElement('div')
  browserWindow.document.body.append(container)
  const root = createRoot(container as unknown as Container)
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  })

  await act(async () => {
    root.render(
      <I18nextProvider i18n={i18n}>
        <QueryClientProvider client={queryClient}>
          <UsageLogsProvider>
            <TaskLogsFilterBar
              table={{ getAllColumns: () => [] } as never}
              logCategory='task'
            />
          </UsageLogsProvider>
        </QueryClientProvider>
      </I18nextProvider>
    )
  })

  return { container: container as unknown as HTMLElement, root }
}

async function unmountFilterBar(mounted: {
  root: Root
  container: HTMLElement
}) {
  await act(async () => mounted.root.unmount())
  mounted.container.remove()
}

function setInputValue(input: HTMLInputElement, value: string) {
  const valueSetter = Object.getOwnPropertyDescriptor(
    browserWindow.HTMLInputElement.prototype,
    'value'
  )?.set
  assert.ok(valueSetter)
  valueSetter.call(input, value)
  input.dispatchEvent(new Event('input', { bubbles: true }))
}

test('admin task view renders channel, status, request model, and searchable user filters', async () => {
  const mounted = await mountFilterBar()

  const channelInput = mounted.container.querySelector(
    '[role="combobox"][aria-label="Channel ID"]'
  )
  assert.ok(channelInput)
  assert.equal(channelInput.tagName, 'BUTTON')
  assert.ok(mounted.container.querySelector('[aria-label="Status"]'))
  assert.ok(mounted.container.querySelector('[aria-label="Request Model"]'))
  assert.ok(
    mounted.container.querySelector('[role="combobox"][aria-label="User"]')
  )
  assert.doesNotMatch(mounted.container.textContent ?? '', /__all__/)

  await unmountFilterBar(mounted)
})

test('self task view hides channel and user filters', async () => {
  adminView.current = false
  const mounted = await mountFilterBar()

  assert.equal(
    mounted.container.querySelector('[aria-label="Channel ID"]'),
    null
  )
  assert.ok(mounted.container.querySelector('[aria-label="Status"]'))
  assert.ok(mounted.container.querySelector('[aria-label="Request Model"]'))
  assert.equal(
    mounted.container.querySelector('[role="combobox"][aria-label="User"]'),
    null
  )

  await unmountFilterBar(mounted)
})

test('selecting a channel immediately navigates to page one', async () => {
  const mounted = await mountFilterBar()
  const channelFilter = mounted.container.querySelector(
    '[role="combobox"][aria-label="Channel ID"]'
  ) as HTMLElement | null
  assert.ok(channelFilter)

  await act(async () =>
    channelFilter.dispatchEvent(new MouseEvent('click', { bubbles: true }))
  )
  const option = [
    ...browserWindow.document.querySelectorAll('[role="option"]'),
  ].find((item) => item.textContent?.trim() === '40 - backup') as
    | HTMLElement
    | undefined
  assert.ok(option)
  await act(async () => {
    option.dispatchEvent(new MouseEvent('pointerdown', { bubbles: true }))
    option.dispatchEvent(new MouseEvent('click', { bubbles: true }))
  })

  const search = lastNavigate.current?.search as
    | Record<string, unknown>
    | undefined
  assert.equal(search?.channel, '40')
  assert.equal(search?.page, 1)

  await unmountFilterBar(mounted)
})

test('channel filter is a controlled select without custom values', async () => {
  const mounted = await mountFilterBar()
  const channelFilter = mounted.container.querySelector(
    '[role="combobox"][aria-label="Channel ID"]'
  )
  assert.ok(channelFilter)
  assert.equal(channelFilter.tagName, 'BUTTON')
  assert.equal(
    mounted.container.querySelector(
      'input[role="combobox"][aria-label="Channel ID"]'
    ),
    null
  )
  assert.equal(lastNavigate.current, null)

  await unmountFilterBar(mounted)
})

test('user filter accepts search text but does not submit a custom value', async () => {
  const mounted = await mountFilterBar()
  const input = mounted.container.querySelector(
    '[role="combobox"][aria-label="User"]'
  ) as HTMLInputElement | null
  assert.ok(input)

  await act(async () => {
    input?.focus()
    if (input) setInputValue(input, 'missing-user')
    input?.dispatchEvent(
      new KeyboardEvent('keydown', { key: 'Enter', bubbles: true })
    )
  })

  assert.equal(lastNavigate.current, null)

  await unmountFilterBar(mounted)
})
