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
import { act, createElement, type ReactNode } from 'react'
import type { Container, Root } from 'react-dom/client'

import type { LogCategory } from '../../types'

type ColumnKind = 'header' | 'cell'
type ColumnClassName = (
  columnId: string,
  kind: ColumnKind
) => string | undefined

interface FakeRow {
  id: string
  original: Record<string, unknown>
}

interface DataTablePageProbeProps {
  getColumnClassName?: ColumnClassName
  renderRow?: (row: FakeRow) => ReactNode
}

interface DataTableRowProbeProps {
  className?: string
  getColumnClassName?: ColumnClassName
}

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
  MutationObserver: browserWindow.MutationObserver,
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

const fakeRow: FakeRow = { id: 'row-1', original: {} }

mock.module('@tanstack/react-query', () => ({
  useQuery: () => ({
    data: { items: [], total: 0 },
    isLoading: false,
    isFetching: false,
  }),
}))

mock.module('@tanstack/react-router', () => ({
  getRouteApi: () => ({
    useSearch: () => ({}),
    useNavigate: () => () => {},
  }),
}))

mock.module('@/hooks', () => ({
  useMediaQuery: () => false,
}))

mock.module('@/hooks/use-table-url-state', () => ({
  useTableUrlState: () => ({
    columnFilters: [],
    onColumnFiltersChange: () => {},
    pagination: { pageIndex: 0, pageSize: 100 },
    onPaginationChange: () => {},
    ensurePageInRange: () => {},
  }),
}))

mock.module('@/components/data-table', () => ({
  DataTablePage: (props: DataTablePageProbeProps) =>
    createElement(
      'div',
      {
        'data-slot': 'density-probe',
        'data-header-class':
          props.getColumnClassName?.('status', 'header') ?? '',
        'data-audit-header-class':
          props.getColumnClassName?.('upstream_response_data', 'header') ?? '',
      },
      props.renderRow?.(fakeRow)
    ),
  DataTableRow: (props: DataTableRowProbeProps) =>
    createElement('div', {
      'data-slot': 'density-row',
      'data-cell-class': props.getColumnClassName?.('status', 'cell') ?? '',
      className: props.className,
    }),
  useDataTable: () => ({ table: {} }),
}))

mock.module('@/features/usage-logs/lib/columns', () => ({
  useColumnsByCategory: () => [],
}))

mock.module('@/features/usage-logs/components/usage-logs-provider', () => ({
  useLogsViewScope: () => ({ isAdminView: false }),
}))

mock.module('@/features/usage-logs/components/common-logs-filter-bar', () => ({
  CommonLogsFilterBar: () => null,
}))

mock.module('@/features/usage-logs/components/task-logs-filter-bar', () => ({
  TaskLogsFilterBar: () => null,
}))

mock.module('@/features/usage-logs/components/usage-logs-mobile-card', () => ({
  UsageLogsMobileList: () => null,
}))

const { createRoot } = await import('react-dom/client')
const { createInstance } = await import('i18next')
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const { UsageLogsTable } = await import('../usage-logs-table')

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
})

async function mountTable(logCategory: LogCategory): Promise<{
  container: HTMLElement
  root: Root
}> {
  const container = browserWindow.document.createElement('div')
  browserWindow.document.body.append(container)
  const root = createRoot(container as unknown as Container)

  await act(async () => {
    root.render(
      <I18nextProvider i18n={i18n}>
        <UsageLogsTable logCategory={logCategory} />
      </I18nextProvider>
    )
  })

  return { container: container as unknown as HTMLElement, root }
}

async function unmountTable(mounted: { container: HTMLElement; root: Root }) {
  await act(async () => mounted.root.unmount())
  mounted.container.remove()
}

for (const logCategory of ['task', 'drawing'] as const) {
  test(`${logCategory} logs use compact desktop column padding and row height`, async () => {
    const mounted = await mountTable(logCategory)
    const probe = mounted.container.querySelector('[data-slot="density-probe"]')
    const row = mounted.container.querySelector('[data-slot="density-row"]')
    assert.ok(probe)
    assert.ok(row)

    assert.match(probe.getAttribute('data-header-class') ?? '', /px-1\.5/)
    assert.match(
      probe.getAttribute('data-audit-header-class') ?? '',
      /whitespace-normal/
    )
    assert.match(row.getAttribute('data-cell-class') ?? '', /px-1\.5/)
    assert.match(row.getAttribute('data-cell-class') ?? '', /py-2\.5/)
    assert.match(row.className, /h-13!/)

    await unmountTable(mounted)
  })
}

test('common logs keep their existing desktop density', async () => {
  const mounted = await mountTable('common')
  const probe = mounted.container.querySelector('[data-slot="density-probe"]')
  const row = mounted.container.querySelector('[data-slot="density-row"]')
  assert.ok(probe)
  assert.ok(row)

  assert.equal(probe.getAttribute('data-header-class'), '')
  assert.equal(row.getAttribute('data-cell-class'), 'py-2')
  assert.doesNotMatch(row.className, /h-13!/)

  await unmountTable(mounted)
})
