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

const { act } = await import('react')
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

  return columns.flatMap((column) =>
    typeof column.header === 'string' ? [column.header] : []
  )
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

  test('hides request, upstream response, and user response data columns from regular users', async () => {
    const headers = await getHeaders(false)

    assert.equal(headers.includes('Request Data'), false)
    assert.equal(headers.includes('Upstream Response Data'), false)
    assert.equal(headers.includes('User Response Data'), false)
  })
})
