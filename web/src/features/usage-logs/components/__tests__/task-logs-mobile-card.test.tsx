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
const { useTaskLogsColumns } = await import('../columns/task-logs-columns')

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
        Consumption: 'Consumption',
        Result: 'Result',
        'Submit Time': 'Submit Time',
      },
    },
  },
})

test('renders only public task result fields in the mobile task card', async () => {
  const container = document.createElement('div')
  document.body.append(container)
  const root = createRoot(container)
  const log: TaskLog = {
    id: 1,
    user_id: 1,
    platform: 'supplier-platform',
    task_id: 'task_public',
    action: 'generate',
    channel_id: 40,
    request_model: 'public-model',
    submit_time: 1,
    status: 'SUCCESS',
    quota: 125000,
    user_request_data: 'request-secret',
    upstream_response_data: 'upstream-secret',
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
    },
    fail_reason: '',
  }

  function MobileCardProbe() {
    const columns = useTaskLogsColumns(false)
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
  assert.match(text, /public-model/)
  assert.match(text, /Task Details/)
  assert.match(text, /Consumption/)
  assert.doesNotMatch(text, /Request Data/)
  assert.doesNotMatch(text, /request-secret/)
  assert.doesNotMatch(text, /Upstream Response \(Create Task\)/)
  assert.doesNotMatch(text, /upstream-secret/)
  assert.doesNotMatch(text, /supplier-platform/)

  const detailsTrigger = container.querySelector<HTMLButtonElement>(
    'button[title="View"]'
  )
  assert.ok(detailsTrigger)
  await act(async () => detailsTrigger.focus())

  const preview = document.querySelector('[data-slot="hover-card-content"]')
  assert.ok(preview)
  assert.match(preview.textContent ?? '', /\/v1\/videos\/task_public\/content/)
  assert.doesNotMatch(preview.textContent ?? '', /supplier\.example/)

  await act(async () => root.unmount())
  container.remove()
})

after(() => {
  domWindow.close()
})
