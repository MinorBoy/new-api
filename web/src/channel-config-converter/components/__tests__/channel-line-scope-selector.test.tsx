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
import { act, useState } from 'react'
import { createRoot, type Container } from 'react-dom/client'
import { I18nextProvider } from 'react-i18next'

import {
  ChannelLineScopeSelector,
  type ChannelLineScopeSelectorProps,
} from '../channel-line-scope-selector'

const browserWindow = new Window({ url: 'http://localhost/' })
browserWindow.document.write('<!doctype html><html><body></body></html>')
Object.assign(globalThis as Record<string, unknown>, {
  window: browserWindow,
  document: browserWindow.document,
  navigator: browserWindow.navigator,
  HTMLElement: browserWindow.HTMLElement,
  HTMLButtonElement: browserWindow.HTMLButtonElement,
  HTMLInputElement: browserWindow.HTMLInputElement,
  Event: browserWindow.Event,
  KeyboardEvent: browserWindow.KeyboardEvent,
  IS_REACT_ACT_ENVIRONMENT: true,
})

after(() => browserWindow.close())

const i18n = createInstance()
i18n.init({ lng: 'en', resources: { en: { translation: {} } } })

type BrowserElement = InstanceType<typeof browserWindow.HTMLElement>
type BrowserButton = InstanceType<typeof browserWindow.HTMLButtonElement>
type BrowserInput = InstanceType<typeof browserWindow.HTMLInputElement>

const props: Omit<
  ChannelLineScopeSelectorProps,
  'onSelectionChange' | 'selectedLineRefs'
> = {
  groups: [
    {
      channel: {
        business_id: 'CH-SECURE',
        display_name: 'Secure',
        entity_hash: 'a'.repeat(64),
        source_ref: 'SOURCE-SECURE',
      },
      lines: [
        {
          business_id: 'secure-discount',
          display_name: 'Secure discount',
          entity_hash: 'b'.repeat(64),
          line_ref: 'secure-discount',
          source_ref: 'SOURCE-SECURE',
        },
        {
          business_id: 'secure-enterprise',
          display_name: 'Secure enterprise',
          entity_hash: 'c'.repeat(64),
          line_ref: 'secure-enterprise',
          source_ref: 'SOURCE-SECURE',
        },
      ],
    },
  ],
  summary: {
    blockingIssues: [],
    document: {
      entities: {
        route_blueprints: [],
      },
      manifest: {
        counts: {
          channels: 1,
          channel_lines: 1,
          cost_rule_drafts: 2,
          model_mappings: 2,
          model_skus: 1,
          route_blueprints: 2,
          sale_proposals: 1,
          sources: 1,
          unresolved_variants: 0,
        },
      },
    },
    selectedGroupCount: 0,
    selectedLineCount: 0,
    validationErrors: [],
    warnings: [],
  } as unknown as ChannelLineScopeSelectorProps['summary'],
}

beforeEach(() => browserWindow.document.body.replaceChildren())

async function mount() {
  const selections: Set<string>[] = []
  const container = browserWindow.document.createElement('div')
  browserWindow.document.body.append(container)
  const root = createRoot(container as unknown as Container)

  function Harness() {
    const [selectedLineRefs, setSelectedLineRefs] = useState(new Set<string>())
    return (
      <ChannelLineScopeSelector
        {...props}
        selectedLineRefs={selectedLineRefs}
        onSelectionChange={(next) => {
          selections.push(next)
          setSelectedLineRefs(next)
        }}
      />
    )
  }

  await act(async () => {
    root.render(
      <I18nextProvider i18n={i18n}>
        <Harness />
      </I18nextProvider>
    )
  })
  return { container, root, selections }
}

function checkbox(container: BrowserElement, label: string): BrowserElement {
  const candidate = container.querySelector(`[aria-label="${label}"]`)
  assert.ok(candidate instanceof browserWindow.HTMLElement)
  return candidate as BrowserElement
}

async function click(element: BrowserElement): Promise<void> {
  await act(async () => element.click())
}

test('group selection becomes indeterminate after one selected line is cleared', async () => {
  const mounted = await mount()
  try {
    const group = checkbox(mounted.container, 'Select all lines in Secure')
    await click(group)
    assert.deepEqual(
      mounted.selections.at(-1),
      new Set(['secure-discount', 'secure-enterprise'])
    )

    await click(checkbox(mounted.container, 'Secure enterprise'))
    assert.equal(group.getAttribute('aria-checked'), 'mixed')
    assert.deepEqual(mounted.selections.at(-1), new Set(['secure-discount']))
  } finally {
    await act(async () => mounted.root.unmount())
  }
})

test('global select, clear, local search, and keyboard selection update selection state', async () => {
  const mounted = await mount()
  try {
    const selectAll = [...mounted.container.querySelectorAll('button')].find(
      (button) => button.textContent === 'Select all channel lines'
    ) as BrowserButton | undefined
    assert.ok(selectAll instanceof browserWindow.HTMLButtonElement)
    await click(selectAll)
    assert.equal(mounted.selections.at(-1)?.size, 2)

    const clear = [...mounted.container.querySelectorAll('button')].find(
      (button) => button.textContent === 'Clear channel line selection'
    ) as BrowserButton | undefined
    assert.ok(clear instanceof browserWindow.HTMLButtonElement)
    await click(clear)
    assert.deepEqual(mounted.selections.at(-1), new Set())

    const input = mounted.container.querySelector(
      'input'
    ) as BrowserInput | null
    assert.ok(input instanceof browserWindow.HTMLInputElement)
    input.value = 'enterprise'
    await act(async () =>
      input.dispatchEvent(new browserWindow.Event('input', { bubbles: true }))
    )
    assert.equal(
      mounted.container.textContent?.includes('Secure discount'),
      false
    )

    const enterprise = checkbox(mounted.container, 'Secure enterprise')
    await act(async () =>
      enterprise.dispatchEvent(
        new browserWindow.KeyboardEvent('keydown', {
          bubbles: true,
          code: 'Space',
          key: ' ',
        })
      )
    )
    assert.deepEqual(mounted.selections.at(-1), new Set(['secure-enterprise']))
  } finally {
    await act(async () => mounted.root.unmount())
  }
})

test('selects every line in a group after filtering that group', async () => {
  const mounted = await mount()
  try {
    const input = mounted.container.querySelector(
      'input'
    ) as BrowserInput | null
    assert.ok(input instanceof browserWindow.HTMLInputElement)
    input.value = 'enterprise'
    await act(async () =>
      input.dispatchEvent(new browserWindow.Event('input', { bubbles: true }))
    )

    await click(checkbox(mounted.container, 'Select all lines in Secure'))
    assert.deepEqual(
      mounted.selections.at(-1),
      new Set(['secure-discount', 'secure-enterprise'])
    )
  } finally {
    await act(async () => mounted.root.unmount())
  }
})
