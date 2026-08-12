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
import test, { after, beforeEach } from 'node:test'

import { Window } from 'happy-dom'
import { createInstance } from 'i18next'
import { act } from 'react'
import type { Container, Root } from 'react-dom/client'
import { I18nextProvider } from 'react-i18next'

import type { CostAccountingSearch } from '../../lib/report'

const browserWindow = new Window({ url: 'http://localhost/' })
browserWindow.document.write('<!doctype html><html><body></body></html>')
browserWindow.document.close()
Object.defineProperty(browserWindow.document, 'compatMode', {
  configurable: true,
  value: 'CSS1Compat',
})
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
  CompositionEvent: browserWindow.CompositionEvent,
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

beforeEach(() => browserWindow.document.body.replaceChildren())

const { createRoot } = await import('react-dom/client')
const { ProfitFilters } = await import('../profit-filters')
const i18n = createInstance()
await i18n.init({
  lng: 'en',
  fallbackLng: 'en',
  resources: { en: { translation: {} } },
})

const filterOptions = {
  channels: [{ value: '7', label: '7 - Primary supplier' }],
  billableModels: [{ value: 'vendor-model', label: 'vendor-model' }],
  originModels: [{ value: 'client-model', label: 'client-model' }],
  userGroups: [{ value: 'default', label: 'default' }],
  usingGroups: [{ value: 'premium', label: 'premium' }],
}

async function mount(
  search: CostAccountingSearch = { tab: 'profit' },
  onChange: (next: CostAccountingSearch) => void = () => {}
): Promise<{ root: Root; container: HTMLElement }> {
  const container = browserWindow.document.createElement('div')
  browserWindow.document.body.append(container)
  const root = createRoot(container as unknown as Container)
  await act(async () => {
    root.render(
      <I18nextProvider i18n={i18n}>
        <ProfitFilters
          search={search}
          onChange={onChange}
          filterOptions={filterOptions}
        />
      </I18nextProvider>
    )
  })
  return { root, container: container as unknown as HTMLElement }
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

function findButton(
  container: HTMLElement,
  label: string
): HTMLButtonElement | undefined {
  return [...container.querySelectorAll<HTMLButtonElement>('button')].find(
    (button) => button.textContent?.includes(label)
  )
}

test('renders each editable profit filter as a searchable combobox', async () => {
  const mounted = await mount()
  try {
    for (const id of [
      'profit-channel-id',
      'profit-billable-model',
      'profit-origin-model',
      'profit-user-group',
      'profit-using-group',
    ]) {
      const field = mounted.container.querySelector(`#${id}`)
      assert.equal(field?.getAttribute('role'), 'combobox')
      assert.equal(field?.getAttribute('aria-expanded'), 'false')
    }
  } finally {
    await act(async () => mounted.root.unmount())
    mounted.container.remove()
  }
})

test('keeps a free-text draft until the user applies profit filters', async () => {
  const updates: CostAccountingSearch[] = []
  const mounted = await mount({ tab: 'profit' }, (next) => updates.push(next))
  try {
    const billableModel = mounted.container.querySelector(
      '#profit-billable-model'
    )
    assert.ok(billableModel instanceof browserWindow.HTMLInputElement)
    await act(async () => {
      setInputValue(billableModel, 'historic-model')
    })
    assert.equal(updates.length, 0)

    const applyButton = findButton(mounted.container, 'Apply filters')
    assert.ok(applyButton)
    await act(async () => applyButton.click())
    assert.equal(updates.length, 1)
    assert.equal(updates[0]?.billableModel, 'historic-model')
  } finally {
    await act(async () => mounted.root.unmount())
    mounted.container.remove()
  }
})

test('selects a channel label but submits only its channel ID', async () => {
  const updates: CostAccountingSearch[] = []
  const mounted = await mount({ tab: 'profit' }, (next) => updates.push(next))
  try {
    const channel =
      mounted.container.querySelector<HTMLInputElement>('#profit-channel-id')
    assert.ok(channel)
    await act(async () => channel.focus())

    const option = [
      ...mounted.container.querySelectorAll<HTMLElement>('[role="option"]'),
    ].find((item) => item.textContent?.includes('7 - Primary supplier'))
    assert.ok(option)
    await act(async () => {
      option.dispatchEvent(new MouseEvent('mousedown', { bubbles: true }))
    })
    assert.equal(channel.value, '7 - Primary supplier')
    assert.equal(updates.length, 0)

    const applyButton = findButton(mounted.container, 'Apply filters')
    assert.ok(applyButton)
    await act(async () => applyButton.click())
    assert.equal(updates[0]?.channelId, 7)
  } finally {
    await act(async () => mounted.root.unmount())
    mounted.container.remove()
  }
})

test('clears only the draft value and keeps the URL unchanged until apply', async () => {
  const updates: CostAccountingSearch[] = []
  const mounted = await mount(
    { tab: 'profit', originModel: 'client-model' },
    (next) => updates.push(next)
  )
  try {
    const originModel = mounted.container.querySelector<HTMLInputElement>(
      '#profit-origin-model'
    )
    assert.ok(originModel)
    await act(async () => {
      originModel.focus()
      setInputValue(originModel, '')
    })
    assert.equal(updates.length, 0)

    const applyButton = findButton(mounted.container, 'Apply filters')
    assert.ok(applyButton)
    await act(async () => applyButton.click())
    assert.equal(updates[0]?.originModel, undefined)
  } finally {
    await act(async () => mounted.root.unmount())
    mounted.container.remove()
  }
})

test('keeps IME composition in the draft and submits completed text on apply', async () => {
  const updates: CostAccountingSearch[] = []
  const mounted = await mount({ tab: 'profit' }, (next) => updates.push(next))
  try {
    const userGroup =
      mounted.container.querySelector<HTMLInputElement>('#profit-user-group')
    assert.ok(userGroup)
    await act(async () => {
      userGroup.dispatchEvent(
        new CompositionEvent('compositionstart', {
          bubbles: true,
        })
      )
      setInputValue(userGroup, '测试组')
      userGroup.dispatchEvent(
        new CompositionEvent('compositionend', {
          bubbles: true,
          data: '测试组',
        })
      )
    })
    assert.equal(updates.length, 0)

    const applyButton = findButton(mounted.container, 'Apply filters')
    assert.ok(applyButton)
    await act(async () => applyButton.click())
    assert.equal(updates[0]?.userGroup, '测试组')
  } finally {
    await act(async () => mounted.root.unmount())
    mounted.container.remove()
  }
})
