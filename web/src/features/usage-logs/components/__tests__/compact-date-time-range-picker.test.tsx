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
import { after, test } from 'node:test'

import { Window } from 'happy-dom'

const domWindow = new Window()
const domGlobals = [
  'window',
  'document',
  'navigator',
  'HTMLElement',
  'HTMLInputElement',
  'SVGElement',
  'Node',
  'Element',
  'Event',
  'MouseEvent',
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
const { CompactDateTimeRangePicker } = await import(
  '../compact-date-time-range-picker'
)

const i18n = createInstance()
await i18n.use(initReactI18next).init({
  lng: 'en',
  resources: {
    en: {
      translation: {
        'Date Range': 'Date Range',
        'Last 24 Hours': 'Last 24 Hours',
        'Last Month': 'Last Month',
      },
    },
  },
})

const reactTestGlobals = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT?: boolean
}
reactTestGlobals.IS_REACT_ACT_ENVIRONMENT = true

after(() => {
  domWindow.close()
})

test('offers and applies the last-month shortcut', async () => {
  let changedRange: { start?: Date; end?: Date } | undefined
  const container = document.createElement('div')
  document.body.append(container)
  const root = createRoot(container)

  await act(async () => {
    root.render(
      <I18nextProvider i18n={i18n}>
        <CompactDateTimeRangePicker
          onChange={(range) => {
            changedRange = range
          }}
        />
      </I18nextProvider>
    )
  })

  const trigger = [...container.querySelectorAll('button')].find(
    (button) => button.textContent?.trim() === 'Date Range'
  )
  assert.ok(trigger)

  await act(async () => {
    trigger.dispatchEvent(
      new domWindow.MouseEvent('click', { bubbles: true }) as unknown as Event
    )
  })

  const shortcut = [...document.querySelectorAll('button')].find(
    (button) => button.textContent?.trim() === 'Last Month'
  )
  assert.ok(shortcut)
  assert.equal(
    [...document.querySelectorAll('button')].some(
      (button) => button.textContent?.trim() === 'Last 24 Hours'
    ),
    true
  )

  await act(async () => {
    shortcut.dispatchEvent(
      new domWindow.MouseEvent('click', { bubbles: true }) as unknown as Event
    )
  })

  assert.ok(changedRange?.start)
  assert.ok(changedRange?.end)
  const now = new Date()
  const lastMonthEnd = new Date(now.getFullYear(), now.getMonth(), 0)
  assert.equal(changedRange.start.getDate(), 1)
  assert.equal(changedRange.start.getMonth(), (now.getMonth() + 11) % 12)
  assert.equal(changedRange.end.getDate(), lastMonthEnd.getDate())

  await act(async () => root.unmount())
  container.remove()
})
