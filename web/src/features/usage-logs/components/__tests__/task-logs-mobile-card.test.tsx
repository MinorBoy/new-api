/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.
*/
import assert from 'node:assert/strict'
import { after, test } from 'node:test'

import {
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

const { act } = await import('react')
const { createRoot } = await import('react-dom/client')
const { createInstance } = await import('i18next')
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const { UsageLogsMobileList } = await import('../usage-logs-mobile-card')

const reactTestGlobals = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT?: boolean
}
reactTestGlobals.IS_REACT_ACT_ENVIRONMENT = true

const i18n = createInstance()
await i18n.use(initReactI18next).init({
  lng: 'en',
  resources: {
    en: {
      translation: {
        'Request Model': 'Request Model',
        'Request Data': 'Request Data',
        'Upstream Response (Create Task)': 'Upstream Response (Create Task)',
        'Task Details': 'Task Details',
        Result: 'Result',
        'Submit Time': 'Submit Time',
      },
    },
  },
})

const columns: ColumnDef<TaskLog>[] = [
  { accessorKey: 'task_id', cell: ({ getValue }) => String(getValue()) },
  { accessorKey: 'status', cell: ({ getValue }) => String(getValue()) },
  { accessorKey: 'submit_time', cell: ({ getValue }) => String(getValue()) },
  {
    accessorKey: 'request_model',
    cell: ({ getValue }) => String(getValue()),
  },
  {
    accessorKey: 'user_request_data',
    cell: ({ getValue }) => String(getValue()),
  },
  {
    accessorKey: 'upstream_response_data',
    cell: ({ getValue }) => String(getValue()),
  },
  {
    accessorKey: 'user_response_data',
    cell: ({ getValue }) => String(getValue()),
  },
  { accessorKey: 'fail_reason', cell: ({ getValue }) => String(getValue()) },
]

test('renders task payload fields in the mobile task card', async () => {
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
    user_request_data: 'request-payload',
    upstream_response_data: 'upstream-payload',
    user_response_data: 'task-result',
    fail_reason: '',
  }

  function MobileCardProbe() {
    const table = useReactTable({
      data: [log],
      columns,
      getCoreRowModel: getCoreRowModel(),
    })
    return <UsageLogsMobileList table={table} logCategory='task' />
  }

  await act(async () => {
    root.render(
      <I18nextProvider i18n={i18n}>
        <MobileCardProbe />
      </I18nextProvider>
    )
  })

  const text = container.textContent ?? ''
  assert.match(text, /Request Model/)
  assert.match(text, /client-model/)
  assert.match(text, /Request Data/)
  assert.match(text, /request-payload/)
  assert.match(text, /Upstream Response \(Create Task\)/)
  assert.match(text, /upstream-payload/)
  assert.match(text, /Task Details/)
  assert.match(text, /task-result/)

  await act(async () => root.unmount())
  container.remove()
})

after(() => {
  domWindow.close()
})
