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
*/
import assert from 'node:assert/strict'
import { after, describe, test } from 'node:test'

import type { ColumnDef } from '@tanstack/react-table'
import { Window } from 'happy-dom'

import type { TaskLog } from '../../types'

const domWindow = new Window()
const domGlobals = [
  'window',
  'document',
  'navigator',
  'HTMLElement',
  'SVGElement',
  'Node',
  'Element',
  'Event',
  'CustomEvent',
  'MutationObserver',
  'requestAnimationFrame',
  'cancelAnimationFrame',
  'getComputedStyle',
] as const

for (const key of domGlobals) {
  Object.defineProperty(globalThis, key, {
    configurable: true,
    value: domWindow[key],
  })
}

const { act, Fragment } = await import('react')
const { createRoot } = await import('react-dom/client')
const { createInstance } = await import('i18next')
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const { useTaskLogsColumns } = await import('../columns/task-logs-columns')

const i18n = createInstance()
await i18n.use(initReactI18next).init({
  lng: 'en',
  resources: {
    en: {
      translation: {
        Endpoint: 'Endpoint',
        Inbound: 'Inbound:',
        View: 'View',
        'Request Data': 'Request Data',
        'Upstream Response Data': 'Upstream Response Data',
        'User Response Data': 'User Response Data',
      },
    },
  },
})

const reactTestGlobals = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT?: boolean
}
reactTestGlobals.IS_REACT_ACT_ENVIRONMENT = true

async function getHeaders(isAdmin: boolean): Promise<string[]> {
  let columns: ColumnDef<TaskLog>[] = []
  const container = document.createElement('div')
  document.body.append(container)
  const root = createRoot(container)

  function ColumnsProbe() {
    columns = useTaskLogsColumns(isAdmin)
    return null
  }

  await act(async () => {
    root.render(
      <I18nextProvider i18n={i18n}>
        <ColumnsProbe />
      </I18nextProvider>
    )
  })
  await act(async () => root.unmount())
  container.remove()

  return columns.flatMap((column) => {
    if (typeof column.header === 'string') {
      return [column.header]
    }
    const label = (column.meta as { label?: string } | undefined)?.label
    return label ? [label] : []
  })
}

async function getAuditButtonLabels(): Promise<
  Array<{ text: string; title: string }>
> {
  let columns: ColumnDef<TaskLog>[] = []
  const container = document.createElement('div')
  document.body.append(container)
  const root = createRoot(container)
  const log: TaskLog = {
    id: 1,
    user_id: 1,
    platform: 'seedance',
    task_id: 'task_1',
    action: 'GENERATE',
    channel_id: 1,
    submit_time: 1,
    status: 'SUCCESS',
    user_request_data: { model: 'seedance' },
    upstream_response_data: { task_id: 'upstream_1' },
    user_response_data: { task_id: 'task_1' },
  }

  function ColumnsProbe() {
    columns = useTaskLogsColumns(true)
    return null
  }

  await act(async () => {
    root.render(
      <I18nextProvider i18n={i18n}>
        <ColumnsProbe />
      </I18nextProvider>
    )
  })

  const auditCells = columns
    .filter(
      (column) =>
        typeof column.header === 'string' &&
        [
          'Request Data',
          'Upstream Response Data',
          'User Response Data',
        ].includes(column.header)
    )
    .map((column, index) => {
      if (typeof column.cell !== 'function') {
        throw new Error('Task audit data column must render a cell component')
      }
      return {
        id: `audit-${index}`,
        content: column.cell({ row: { original: log } } as never),
      }
    })

  await act(async () => {
    root.render(
      <I18nextProvider i18n={i18n}>
        {auditCells.map((cell) => (
          <Fragment key={cell.id}>{cell.content}</Fragment>
        ))}
      </I18nextProvider>
    )
  })

  const labels = [...container.querySelectorAll('button[title]')].map(
    (button) => ({
      text: button.textContent?.trim() ?? '',
      title: button.getAttribute('title') ?? '',
    })
  )

  await act(async () => root.unmount())
  container.remove()

  return labels
}

async function getEndpointCellText(): Promise<string> {
  let columns: ColumnDef<TaskLog>[] = []
  const container = document.createElement('div')
  document.body.append(container)
  const root = createRoot(container)
  const log: TaskLog = {
    id: 1,
    user_id: 1,
    platform: 'seedance',
    task_id: 'task_1',
    action: 'GENERATE',
    channel_id: 1,
    request_path: '/v1/video/generations',
    submit_time: 1,
    status: 'SUCCESS',
  }

  function ColumnsProbe() {
    columns = useTaskLogsColumns(true)
    return null
  }

  await act(async () => {
    root.render(
      <I18nextProvider i18n={i18n}>
        <ColumnsProbe />
      </I18nextProvider>
    )
  })

  const endpointColumn = columns.find(
    (column) => 'accessorKey' in column && column.accessorKey === 'request_path'
  )
  const content =
    typeof endpointColumn?.cell === 'function'
      ? endpointColumn.cell({ row: { original: log } } as never)
      : null

  await act(async () => {
    root.render(<I18nextProvider i18n={i18n}>{content}</I18nextProvider>)
  })

  const text = container.textContent?.trim() ?? ''
  await act(async () => root.unmount())
  container.remove()

  return text
}

describe('task audit columns', () => {
  after(() => {
    domWindow.close()
  })

  test('shows request, upstream response, and user response data columns to administrators', async () => {
    const headers = await getHeaders(true)

    assert.equal(headers.includes('Request Data'), true)
    assert.equal(headers.includes('Upstream Response Data'), true)
    assert.equal(headers.includes('User Response Data'), true)
  })

  test('places administrator audit columns after channel and user identity', async () => {
    const headers = await getHeaders(true)

    assert.deepEqual(headers.slice(0, 7), [
      'Submit Time',
      'Channel',
      'Endpoint',
      'User',
      'Request Data',
      'Upstream Response Data',
      'User Response Data',
    ])
  })

  test('renders the persisted inbound request path next to channel', async () => {
    const text = await getEndpointCellText()

    assert.equal(text, 'Inbound:/v1/video/generations')
  })

  test('shows request data but hides upstream and user response data columns from regular users', async () => {
    const headers = await getHeaders(false)

    assert.equal(headers.includes('Request Data'), true)
    assert.equal(headers.includes('Upstream Response Data'), false)
    assert.equal(headers.includes('User Response Data'), false)
  })

  test('uses the compact View label for every task audit data action', async () => {
    const labels = await getAuditButtonLabels()

    assert.deepEqual(labels, [
      { text: 'View', title: 'View' },
      { text: 'View', title: 'View' },
      { text: 'View', title: 'View' },
    ])
  })
})
