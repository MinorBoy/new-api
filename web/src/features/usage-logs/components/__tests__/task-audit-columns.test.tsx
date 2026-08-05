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

import {
  flexRender,
  getCoreRowModel,
  type ColumnDef,
  useReactTable,
} from '@tanstack/react-table'
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
  'customElements',
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

Object.defineProperty(domWindow, 'matchMedia', {
  configurable: true,
  value: () => ({
    matches: false,
    addEventListener: () => {},
    removeEventListener: () => {},
  }),
})
Object.defineProperty(globalThis, 'matchMedia', {
  configurable: true,
  value: domWindow.matchMedia,
})

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
        'Request Model': 'Request Model',
        'Request Data': 'Request Data',
        'Upstream Response (Create Task)': 'Upstream Response (Create Task)',
        'Task Details': 'Task Details',
        'Copy to clipboard': 'Copy to clipboard',
        'View the full data captured for this task.':
          'View the full data captured for this task.',
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
    request_model: 'client-model',
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
          'Upstream Response (Create Task)',
          'Task Details',
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

async function getRequestModelCellText(): Promise<string> {
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
    request_model: 'client-model',
    submit_time: 1,
    status: 'SUCCESS',
  }

  function ColumnsProbe() {
    columns = useTaskLogsColumns(false)
    return null
  }

  await act(async () => {
    root.render(
      <I18nextProvider i18n={i18n}>
        <ColumnsProbe />
      </I18nextProvider>
    )
  })

  const requestModelColumn = columns.find(
    (column) =>
      'accessorKey' in column && column.accessorKey === 'request_model'
  )
  const content =
    typeof requestModelColumn?.cell === 'function'
      ? requestModelColumn.cell({ row: { original: log } } as never)
      : null

  await act(async () => {
    root.render(<I18nextProvider i18n={i18n}>{content}</I18nextProvider>)
  })

  const text = container.textContent?.trim() ?? ''
  await act(async () => root.unmount())
  container.remove()

  return text
}

async function renderRequestDataCell(data: unknown) {
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
    user_request_data: data,
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

  const requestDataColumn = columns.find(
    (column) =>
      'accessorKey' in column && column.accessorKey === 'user_request_data'
  )
  if (typeof requestDataColumn?.cell !== 'function') {
    throw new Error('Request data column must render a cell component')
  }
  const content = requestDataColumn.cell({ row: { original: log } } as never)

  await act(async () => {
    root.render(<I18nextProvider i18n={i18n}>{content}</I18nextProvider>)
  })

  return {
    container,
    root,
    async cleanup() {
      await act(async () => root.unmount())
      container.remove()
    },
  }
}

async function renderTaskRow(log: TaskLog, isAdmin = false) {
  const container = document.createElement('div')
  document.body.append(container)
  const root = createRoot(container)

  function TaskRowProbe() {
    const columns = useTaskLogsColumns(isAdmin)
    const table = useReactTable({
      data: [log],
      columns,
      getCoreRowModel: getCoreRowModel(),
    })

    return (
      <div>
        {table.getRowModel().rows[0]?.getVisibleCells().map((cell) => (
          <div key={cell.id} data-column-id={cell.column.id}>
            {flexRender(cell.column.columnDef.cell, cell.getContext())}
          </div>
        ))}
      </div>
    )
  }

  await act(async () => {
    root.render(
      <I18nextProvider i18n={i18n}>
        <TaskRowProbe />
      </I18nextProvider>
    )
  })

  return {
    container,
    root,
    async cleanup() {
      await act(async () => root.unmount())
      container.remove()
    },
  }
}

describe('task audit columns', () => {
  after(() => {
    domWindow.close()
  })

  test('shows request model, request, upstream create response, and task detail columns to administrators', async () => {
    const headers = await getHeaders(true)

    assert.equal(headers.includes('Request Model'), true)
    assert.equal(headers.includes('Request Data'), true)
    assert.equal(headers.includes('Upstream Response (Create Task)'), true)
    assert.equal(headers.includes('Task Details'), true)
  })

  test('places administrator audit columns after channel and user identity', async () => {
    const headers = await getHeaders(true)

    assert.deepEqual(headers.slice(0, 8), [
      'Submit Time',
      'Channel',
      'Endpoint',
      'User',
      'Request Model',
      'Request Data',
      'Upstream Response (Create Task)',
      'Task Details',
    ])
  })

  test('renders the persisted inbound request path next to channel', async () => {
    const text = await getEndpointCellText()

    assert.equal(text, 'Inbound:/v1/video/generations')
  })

  test('renders the request model returned by the task API', async () => {
    const text = await getRequestModelCellText()

    assert.equal(text, 'client-model')
  })

  test('shows public task details but hides every administrator audit column from regular users', async () => {
    const headers = await getHeaders(false)

    assert.equal(headers.includes('Request Model'), true)
    assert.equal(headers.includes('Request Data'), false)
    assert.equal(headers.includes('Upstream Response (Create Task)'), false)
    assert.equal(headers.includes('Task Details'), true)
    assert.equal(headers.includes('Consumption'), true)
    assert.equal(headers.includes('Channel'), false)
    assert.equal(headers.includes('Endpoint'), false)
    assert.equal(headers.includes('User'), false)
  })

  test('regular task rows render only the Ark public result and local proxy URL', async () => {
    const mounted = await renderTaskRow({
      id: 91,
      user_id: 10,
      username: 'supplier-user',
      platform: 'supplier-platform',
      task_id: 'task_public',
      action: 'generate',
      channel_id: 40,
      request_path: '/supplier/private/path',
      request_model: 'public-model',
      submit_time: 1,
      status: 'SUCCESS',
      user_request_data: { secret: 'request-secret' },
      upstream_response_data: { id: 'cgt-private' },
      user_response_data: {
        id: 'task_public',
        model: 'public-model',
        status: 'succeeded',
        content: { video_url: '/v1/videos/task_public/content' },
        usage: { completion_tokens: 108900, total_tokens: 108900 },
        created_at: 1779348818,
        updated_at: 1779348874,
        seed: 78674,
        resolution: '720p',
        ratio: '16:9',
        duration: 5,
        framespersecond: 24,
        service_tier: 'default',
        execution_expires_after: 172800,
        generate_audio: true,
        draft: false,
        priority: 0,
        supplier_url: 'https://supplier.example/private',
        upstream_model: 'provider-model',
      },
      fail_reason: '',
    })

    const text = mounted.container.textContent ?? ''
    assert.match(text, /public-model/)
    assert.match(text, /task_public/)
    assert.doesNotMatch(text, /supplier-platform/)
    assert.doesNotMatch(text, /request-secret/)
    assert.doesNotMatch(text, /cgt-private/)

    const detailsTrigger = mounted.container.querySelector<HTMLButtonElement>(
      '[data-column-id="user_response_data"] button[title="View"]'
    )
    assert.ok(detailsTrigger)
    await act(async () => detailsTrigger.focus())

    const preview = document.querySelector('[data-slot="hover-card-content"]')
    assert.ok(preview)
    const previewText = preview.textContent ?? ''
    assert.match(previewText, /"completion_tokens": 108900/)
    assert.match(previewText, /\/v1\/videos\/task_public\/content/)
    assert.doesNotMatch(previewText, /supplier\.example/)
    assert.doesNotMatch(previewText, /provider-model/)

    const videoLink = mounted.container.querySelector<HTMLAnchorElement>(
      '[data-column-id="fail_reason"] a'
    )
    assert.equal(videoLink?.getAttribute('href'), '/v1/videos/task_public/content')

    await mounted.cleanup()
  })

  test('uses the compact View label for every task audit data action', async () => {
    const labels = await getAuditButtonLabels()

    assert.deepEqual(labels, [
      { text: 'View', title: 'View' },
      { text: 'View', title: 'View' },
      { text: 'View', title: 'View' },
    ])
  })

  test('shows the complete request JSON in a scrollable preview when the View action receives focus', async () => {
    const mounted = await renderRequestDataCell({
      model: 'seedance',
      prompt: 'A cinematic city at night',
    })
    const trigger = mounted.container.querySelector<HTMLButtonElement>(
      'button[title="View"]'
    )
    assert.ok(trigger)

    await act(async () => trigger.focus())

    const preview = document.querySelector('[data-slot="hover-card-content"]')
    assert.ok(preview)
    assert.match(preview.textContent ?? '', /"model": "seedance"/)
    assert.match(
      preview.textContent ?? '',
      /"prompt": "A cinematic city at night"/
    )
    const scrollArea = preview.querySelector('[data-slot="scroll-area"]')
    assert.ok(scrollArea)
    assert.equal(
      scrollArea.classList.contains('h-[min(26rem,calc(100vh-8rem))]'),
      true
    )
    assert.ok(preview.querySelector('button[aria-label="Copy to clipboard"]'))

    await mounted.cleanup()
  })

  test('keeps click as the full request data dialog action', async () => {
    const mounted = await renderRequestDataCell({ model: 'seedance' })
    const trigger = mounted.container.querySelector<HTMLButtonElement>(
      'button[title="View"]'
    )
    assert.ok(trigger)

    await act(async () => trigger.click())

    const dialog = document.querySelector('[role="dialog"]')
    assert.ok(dialog)
    assert.match(dialog.textContent ?? '', /Request Data/)
    assert.match(dialog.textContent ?? '', /"model": "seedance"/)

    await mounted.cleanup()
  })

  test('does not render a preview or dialog action for empty request data', async () => {
    const mounted = await renderRequestDataCell('')

    assert.equal(mounted.container.textContent?.trim(), '-')
    assert.equal(mounted.container.querySelector('button'), null)

    await mounted.cleanup()
  })
})
