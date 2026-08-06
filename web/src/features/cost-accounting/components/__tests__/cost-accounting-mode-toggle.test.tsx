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
import { createInstance } from 'i18next'
import { act } from 'react'
import type { Container, Root } from 'react-dom/client'
import { I18nextProvider } from 'react-i18next'

const browserWindow = new Window({ url: 'http://localhost/' })
browserWindow.document.write('<!doctype html><html><body></body></html>')
browserWindow.document.close()

const browserGlobals = {
  window: browserWindow,
  document: browserWindow.document,
  navigator: browserWindow.navigator,
  Element: browserWindow.Element,
  HTMLElement: browserWindow.HTMLElement,
  Node: browserWindow.Node,
  Event: browserWindow.Event,
  CustomEvent: browserWindow.CustomEvent,
  MouseEvent: browserWindow.MouseEvent,
  PointerEvent: browserWindow.PointerEvent,
  KeyboardEvent: browserWindow.KeyboardEvent,
  FocusEvent: browserWindow.FocusEvent,
  MutationObserver: browserWindow.MutationObserver,
  ResizeObserver: browserWindow.ResizeObserver,
  IntersectionObserver: browserWindow.IntersectionObserver,
  getComputedStyle: browserWindow.getComputedStyle.bind(browserWindow),
  requestAnimationFrame:
    browserWindow.requestAnimationFrame.bind(browserWindow),
  cancelAnimationFrame:
    browserWindow.cancelAnimationFrame.bind(browserWindow),
  IS_REACT_ACT_ENVIRONMENT: true,
}
const previousBrowserGlobals = Object.fromEntries(
  Object.keys(browserGlobals).map((key) => [
    key,
    Object.getOwnPropertyDescriptor(globalThis, key),
  ])
)
Object.assign(globalThis as Record<string, unknown>, browserGlobals)

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

const { createRoot } = await import('react-dom/client')
const { CostAccountingModeToggle } =
  await import('../cost-accounting-mode-toggle')
const i18n = createInstance()
await i18n.init({
  lng: 'en',
  fallbackLng: 'en',
  resources: {
    en: {
      translation: {
        'Cost accounting mode': 'Cost accounting mode',
        Disabled: 'Disabled',
        Tracking: 'Tracking',
        Strict: 'Strict',
        'Turns off provider cost accounting and profit guardrails. Existing user billing continues.':
          'Turns off provider cost accounting and profit guardrails. Existing user billing continues.',
        'Records revenue, provider cost, profit, and anomalies without blocking missing-cost or low-margin routes.':
          'Records revenue, provider cost, profit, and anomalies without blocking missing-cost or low-margin routes.',
        'Records cost and blocks routes with unknown cost, missing rules, or expected margin below the minimum. Requires complete cost coverage.':
          'Records cost and blocks routes with unknown cost, missing rules, or expected margin below the minimum. Requires complete cost coverage.',
      },
    },
  },
})

type MountedToggle = {
  root: Root
  container: HTMLElement
  changes: string[]
}

async function mountToggle(): Promise<MountedToggle> {
  const container = browserWindow.document.createElement('div')
  browserWindow.document.body.append(container)
  const root = createRoot(container as unknown as Container)
  const changes: string[] = []

  await act(async () => {
    root.render(
      <I18nextProvider i18n={i18n}>
        <CostAccountingModeToggle
          mode='tracking'
          canEnableStrict={false}
          disabled={false}
          onChange={(mode) => changes.push(mode)}
        />
      </I18nextProvider>
    )
  })

  return { root, container, changes }
}

async function unmountToggle(mounted: MountedToggle) {
  await act(async () => mounted.root.unmount())
  mounted.container.remove()
}

function findButton(container: HTMLElement, label: string): HTMLButtonElement {
  const button = Array.from(container.querySelectorAll('button')).find(
    (item) => item.textContent?.trim() === label
  )
  assert.ok(button, `Expected to find the ${label} mode button`)
  return button
}

async function hover(element: HTMLElement) {
  await act(async () => {
    element.dispatchEvent(
      new browserWindow.PointerEvent('pointerover', {
        bubbles: true,
        pointerType: 'mouse',
      })
    )
    element.dispatchEvent(
      new browserWindow.MouseEvent('mouseenter', { bubbles: false })
    )
    await Promise.resolve()
  })
}

test('shows the matching description when an enabled mode receives focus', async () => {
  const mounted = await mountToggle()
  try {
    const tracking = findButton(mounted.container, 'Tracking')
    await act(async () => {
      tracking.focus()
      await Promise.resolve()
    })
    assert.match(
      browserWindow.document.body.textContent ?? '',
      /Records revenue, provider cost, profit, and anomalies/
    )
  } finally {
    await unmountToggle(mounted)
  }
})

test('shows the strict description on hover even when strict mode is disabled', async () => {
  const mounted = await mountToggle()
  try {
    const strict = findButton(mounted.container, 'Strict')
    assert.equal(strict.disabled, false)
    assert.equal(strict.getAttribute('aria-disabled'), 'true')
    await hover(strict)
    assert.match(
      browserWindow.document.body.textContent ?? '',
      /Requires complete cost coverage/
    )
  } finally {
    await unmountToggle(mounted)
  }
})

test('does not select strict mode while it is disabled', async () => {
  const mounted = await mountToggle()
  try {
    const strict = findButton(mounted.container, 'Strict')
    await act(async () => strict.click())
    assert.deepEqual(mounted.changes, [])
  } finally {
    await unmountToggle(mounted)
  }
})

test('reports an enabled mode selection', async () => {
  const mounted = await mountToggle()
  try {
    const disabled = findButton(mounted.container, 'Disabled')
    await act(async () => disabled.click())
    assert.deepEqual(mounted.changes, ['disabled'])
  } finally {
    await unmountToggle(mounted)
  }
})
