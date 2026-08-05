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
// @ts-expect-error Bun supplies mock.module at test runtime, but the frontend
// typecheck intentionally only includes Node's test declarations.
import { mock } from 'bun:test'
import assert from 'node:assert/strict'
import test, { after, beforeEach } from 'node:test'

import { Window } from 'happy-dom'
import { act } from 'react'
import type { Container, Root } from 'react-dom/client'

const browserWindow = new Window({ url: 'http://localhost/usage-logs/common' })
const browserGlobals = {
  window: browserWindow,
  document: browserWindow.document,
  navigator: browserWindow.navigator,
  Element: browserWindow.Element,
  HTMLElement: browserWindow.HTMLElement,
  Node: browserWindow.Node,
  Event: browserWindow.Event,
  MutationObserver: browserWindow.MutationObserver,
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

const lastNavigate = { current: null as Record<string, unknown> | null }

// Only mock @tanstack/react-router — it is not imported by the pure helper
// modules under test elsewhere (e.g. use-common-log-filter-options), so this
// mock does not leak across files in a batched `bun test` run. A real
// QueryClient is provided below instead of mocking @tanstack/react-query.
mock.module('@tanstack/react-router', () => {
  // No `type` in the search params simulates the default landing state
  // (no type chosen yet), so the component falls back to its default tab.
  const stubSearch = () => ({})
  return {
    getRouteApi: () => ({
      useSearch: stubSearch,
      useNavigate: () => () => {},
    }),
    useNavigate: () => (search: Record<string, unknown>) => {
      lastNavigate.current = search
    },
  }
})

const { createRoot } = await import('react-dom/client')
const { QueryClient, QueryClientProvider } =
  await import('@tanstack/react-query')
const { createInstance } = await import('i18next')
const { I18nextProvider, initReactI18next } = await import('react-i18next')

const { LOG_TYPE_FILTERS } = await import('../../constants')
const { UsageLogsProvider } = await import('../usage-logs-provider')
const { CommonLogsFilterBar } = await import('../common-logs-filter-bar')

const i18n = createInstance()
await i18n.use(initReactI18next).init({
  lng: 'en',
  resources: {
    en: {
      translation: Object.fromEntries(
        LOG_TYPE_FILTERS.map((type) => [type.label, type.label])
      ),
    },
  },
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
  lastNavigate.current = null
})

function createQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: 0, staleTime: 0 },
    },
  })
}

async function mountFilterBar() {
  const container = browserWindow.document.createElement('div')
  browserWindow.document.body.append(container)
  const root = createRoot(container as unknown as Container)

  await act(async () => {
    root.render(
      <I18nextProvider i18n={i18n}>
        <QueryClientProvider client={createQueryClient()}>
          <UsageLogsProvider>
            <CommonLogsFilterBar table={{ getAllColumns: () => [] } as never} />
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

test('renders a log-type Tabs row above the filter toolbar with every type', async () => {
  const mounted = await mountFilterBar()
  const tabTriggers = [
    ...mounted.container.querySelectorAll('[data-slot="tabs-trigger"]'),
  ]

  // Every LOG_TYPE_FILTERS entry (All Types + 7 log types) is a Tab.
  assert.equal(tabTriggers.length, LOG_TYPE_FILTERS.length)
  const labels = tabTriggers.map(
    (trigger) => (trigger as HTMLElement).textContent?.trim() ?? ''
  )
  assert.deepEqual(
    labels,
    LOG_TYPE_FILTERS.map((type) => type.label)
  )

  // The Tabs group sits above the rest of the filter bar — the first
  // child of the rendered fragment is the tabs, not the toolbar.
  const tabsList = mounted.container.querySelector('[data-slot="tabs-list"]')
  const toolbar = mounted.container.querySelector('.bg-card\\/50')
  assert.ok(tabsList)
  assert.ok(toolbar)
  const domOrder =
    (tabsList as HTMLElement).compareDocumentPosition(toolbar as HTMLElement) &
    Node.DOCUMENT_POSITION_FOLLOWING
  assert.ok(
    domOrder,
    'log-type tabs must precede the filter toolbar in the DOM'
  )

  await unmountFilterBar(mounted)
})

test('selecting a log-type Tab navigates with that type in the URL', async () => {
  const mounted = await mountFilterBar()
  const errorTrigger = (
    [
      ...mounted.container.querySelectorAll('[data-slot="tabs-trigger"]'),
    ] as HTMLElement[]
  ).find((trigger) => trigger.textContent?.trim() === 'Error')

  assert.ok(errorTrigger, 'Error tab must be present')
  if (errorTrigger) {
    await act(async () => {
      errorTrigger.click()
    })
  }

  const search =
    (lastNavigate.current?.search as Record<string, unknown> | undefined) ??
    undefined
  assert.deepEqual(search?.type, ['5'])

  await unmountFilterBar(mounted)
})

test('"Consume" is the active Tab when no type is selected (default)', async () => {
  const mounted = await mountFilterBar()
  const activeTriggers = [
    ...mounted.container.querySelectorAll(
      '[data-slot="tabs-trigger"][aria-selected="true"]'
    ),
  ]

  assert.equal(activeTriggers.length, 1)
  assert.equal(
    (activeTriggers[0] as HTMLElement).textContent?.trim(),
    'Consume'
  )

  await unmountFilterBar(mounted)
})

test('regular users do not receive supplier filters or an audit masking toggle', async () => {
  const mounted = await mountFilterBar()
  const text = mounted.container.textContent ?? ''
  const placeholders = new Set([
    ...mounted.container.querySelectorAll<HTMLInputElement>('[placeholder]'),
  ].map((input) => input.placeholder))

  assert.equal(text.includes('Group'), false)
  assert.equal(placeholders.has('Channel ID'), false)
  assert.equal(placeholders.has('Upstream Request ID'), false)
  assert.equal(mounted.container.querySelector('[aria-label="Hide"]'), null)
  assert.equal(mounted.container.querySelector('[aria-label="Show"]'), null)

  await unmountFilterBar(mounted)
})
