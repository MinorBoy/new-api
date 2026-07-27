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
import { createRoot, type Container } from 'react-dom/client'
import { I18nextProvider } from 'react-i18next'

import App, { type WorkbookConversion } from '../app'

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
  File: browserWindow.File,
  IS_REACT_ACT_ENVIRONMENT: true,
})

after(() => browserWindow.close())

const i18n = createInstance()
i18n.init({ lng: 'en', resources: { en: { translation: {} } } })

type BrowserElement = InstanceType<typeof browserWindow.HTMLElement>
type BrowserButton = InstanceType<typeof browserWindow.HTMLButtonElement>
type BrowserInput = InstanceType<typeof browserWindow.HTMLInputElement>

function conversion(severity: 'error' | 'warning'): WorkbookConversion {
  return {
    document: {
      kind: 'new-api.channel-config-import',
      schema_version: 1,
      template_version: '1',
      manifest: {
        converter_version: '1.0.0',
        counts: {
          channel_lines: 1,
          channels: 1,
          cost_rule_drafts: 0,
          model_mappings: 0,
          model_skus: 0,
          route_blueprints: 0,
          sale_proposals: 0,
          sources: 1,
          unresolved_variants: 0,
        },
        generated_at: '2026-07-27T00:00:00Z',
        payload_sha256: 'a'.repeat(64),
        source_file_name: 'channels.xlsx',
        source_sha256: 'b'.repeat(64),
        template_match: 'v1',
      },
      entities: {
        channels: [
          {
            business_id: 'channel-one',
            display_name: 'Channel one',
            entity_hash: 'c'.repeat(64),
            row: 4,
            sheet: 'Channels',
            source_ref: 'source-one',
          },
        ],
        channel_lines: [
          {
            business_id: 'line-one',
            channel_ref: 'channel-one',
            entity_hash: 'd'.repeat(64),
            line_ref: 'line-one',
            row: 4,
            sheet: 'Lines',
            source_ref: 'source-one',
            status_proposal: 'disabled',
          },
        ],
        cost_rule_drafts: [],
        model_mappings: [],
        model_skus: [],
        route_blueprints: [],
        sale_proposals: [],
        sources: [
          {
            business_id: 'source-one',
            entity_hash: 'e'.repeat(64),
            row: 4,
            sheet: 'Sources',
            source_ref: 'source-one',
          },
        ],
        unresolved_variants: [],
      },
      derived_preview: {},
      issues: [
        {
          code: 'COST_VARIANT_AMBIGUOUS',
          entity_ref: 'line-one',
          message: 'A verified line is required.',
          row: 4,
          severity,
          sheet: 'Lines',
        },
      ],
    },
    hasFailures: severity === 'error',
    hasWarnings: severity === 'warning',
  }
}

beforeEach(() => browserWindow.document.body.replaceChildren())

async function mount(result: WorkbookConversion) {
  const container = browserWindow.document.createElement('div')
  browserWindow.document.body.append(container)
  const root = createRoot(container as unknown as Container)
  await act(async () => {
    root.render(
      <I18nextProvider i18n={i18n}>
        <App convertFile={async () => result} />
      </I18nextProvider>
    )
  })
  return { container, root }
}

async function upload(
  container: BrowserElement,
  name = 'channels.xlsx'
): Promise<void> {
  const candidate = container.querySelector('input[type="file"]')
  assert.ok(candidate instanceof browserWindow.HTMLInputElement)
  const input = candidate as unknown as BrowserInput
  Object.defineProperty(input, 'files', {
    configurable: true,
    value: [new browserWindow.File(['fixture'], name)],
  })
  await act(async () => {
    input.dispatchEvent(new browserWindow.Event('change', { bubbles: true }))
    await Promise.resolve()
    await Promise.resolve()
  })
}

function button(container: BrowserElement, label: string): BrowserButton {
  const candidate = [...container.querySelectorAll('button')].find(
    (element) => element.textContent?.trim() === label
  )
  assert.ok(candidate instanceof browserWindow.HTMLButtonElement)
  return candidate as unknown as BrowserButton
}

test('shows all preview tabs, source locations, and allows formal JSON for warnings', async () => {
  const mounted = await mount(conversion('warning'))
  try {
    await upload(mounted.container)

    for (const label of [
      'Overview',
      'Channels and lines',
      'Model SKUs',
      'Sale pricing',
      'Channel costs',
      'Model mappings and routing',
      'Issues',
      'JSON',
    ]) {
      assert.ok(button(mounted.container, label))
    }
    assert.equal(button(mounted.container, 'Download JSON').disabled, false)
    assert.equal(
      button(mounted.container, 'Download issue report').disabled,
      false
    )

    await act(async () =>
      button(mounted.container, 'Channels and lines').click()
    )
    assert.match(mounted.container.textContent || '', /Channels!4/)

    await act(async () => button(mounted.container, 'Issues').click())
    assert.match(mounted.container.textContent || '', /COST_VARIANT_AMBIGUOUS/)

    await act(async () => button(mounted.container, 'Clear').click())
    assert.equal(mounted.container.querySelectorAll('[role="tab"]').length, 0)
  } finally {
    await act(async () => mounted.root.unmount())
  }
})

test('blocks formal JSON while retaining the issue report for failures', async () => {
  const mounted = await mount(conversion('error'))
  try {
    await upload(mounted.container)

    assert.equal(button(mounted.container, 'Download JSON').disabled, true)
    assert.equal(
      button(mounted.container, 'Download issue report').disabled,
      false
    )
  } finally {
    await act(async () => mounted.root.unmount())
  }
})
