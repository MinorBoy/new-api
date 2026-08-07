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
          group_routing_requirements: 0,
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
            display_name: 'Line one',
            entity_hash: 'd'.repeat(64),
            line_ref: 'line-one',
            row: 4,
            sheet: 'Lines',
            source_ref: 'source-one',
            status_proposal: 'disabled',
          },
        ],
        cost_rule_drafts: [],
        group_routing_requirements: [],
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

test('shows all preview tabs and enables selected JSON export for warnings', async () => {
  const mounted = await mount(conversion('warning'))
  try {
    await upload(mounted.container)

    for (const label of [
      'Overview',
      'Selection',
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
    assert.equal(
      button(mounted.container, 'Export selected JSON').disabled,
      true
    )
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

    await act(async () => button(mounted.container, 'Selection').click())
    const line = mounted.container.querySelector('[aria-label="Line one"]')
    assert.ok(line instanceof browserWindow.HTMLElement)
    await act(async () => line.click())
    assert.equal(
      button(mounted.container, 'Export selected JSON').disabled,
      false
    )

    await act(async () => button(mounted.container, 'Clear').click())
    assert.equal(mounted.container.querySelectorAll('[role="tab"]').length, 0)
  } finally {
    await act(async () => mounted.root.unmount())
  }
})

test('blocks selected JSON while retaining the issue report for selected failures', async () => {
  const mounted = await mount(conversion('error'))
  try {
    await upload(mounted.container)

    await act(async () => button(mounted.container, 'Selection').click())
    const line = mounted.container.querySelector('[aria-label="Line one"]')
    assert.ok(line instanceof browserWindow.HTMLElement)
    await act(async () => line.click())
    assert.equal(
      button(mounted.container, 'Export selected JSON').disabled,
      true
    )
    assert.equal(
      button(mounted.container, 'Download issue report').disabled,
      false
    )
  } finally {
    await act(async () => mounted.root.unmount())
  }
})

test('exports a valid selected scope without unselected line errors', async () => {
  const result = conversion('error')
  result.document.entities.channel_lines.push({
    business_id: 'line-two',
    channel_ref: 'channel-one',
    display_name: 'Line two',
    entity_hash: 'f'.repeat(64),
    line_ref: 'line-two',
    row: 5,
    sheet: 'Lines',
    source_ref: 'source-one',
    status_proposal: 'disabled',
  })
  result.document.manifest.counts.channel_lines = 2
  result.document.issues[0].entity_ref = 'line-two'

  const mounted = await mount(result)
  try {
    await upload(mounted.container)
    await act(async () => button(mounted.container, 'Selection').click())
    const line = mounted.container.querySelector('[aria-label="Line one"]')
    assert.ok(line instanceof browserWindow.HTMLElement)
    await act(async () => line.click())

    assert.equal(
      button(mounted.container, 'Export selected JSON').disabled,
      false
    )
    assert.equal(
      button(mounted.container, 'Download issue report').disabled,
      false
    )
  } finally {
    await act(async () => mounted.root.unmount())
  }
})

test('disables export while rebuilding a changed valid selection', async () => {
  const result = conversion('warning')
  result.document.entities.channel_lines.push({
    business_id: 'line-two',
    channel_ref: 'channel-one',
    display_name: 'Line two',
    entity_hash: 'f'.repeat(64),
    line_ref: 'line-two',
    row: 5,
    sheet: 'Lines',
    source_ref: 'source-one',
    status_proposal: 'disabled',
  })
  result.document.manifest.counts.channel_lines = 2

  const mounted = await mount(result)
  const originalCrypto = globalThis.crypto
  try {
    await upload(mounted.container)
    await act(async () => button(mounted.container, 'Selection').click())

    const firstLine = mounted.container.querySelector('[aria-label="Line one"]')
    assert.ok(firstLine instanceof browserWindow.HTMLElement)
    await act(async () => firstLine.click())
    assert.equal(
      button(mounted.container, 'Export selected JSON').disabled,
      false
    )

    Object.defineProperty(globalThis, 'crypto', {
      configurable: true,
      value: {
        subtle: {
          digest: () => new Promise<ArrayBuffer>(() => undefined),
        },
      },
    })
    const secondLine = mounted.container.querySelector(
      '[aria-label="Line two"]'
    )
    assert.ok(secondLine instanceof browserWindow.HTMLElement)
    await act(async () => secondLine.click())
    assert.equal(
      button(mounted.container, 'Export selected JSON').disabled,
      true
    )
    assert.equal(
      button(mounted.container, 'Download issue report').disabled,
      true
    )
  } finally {
    Object.defineProperty(globalThis, 'crypto', {
      configurable: true,
      value: originalCrypto,
    })
    await act(async () => mounted.root.unmount())
  }
})

test('keeps selector controls available until the latest scope rebuild resolves', async () => {
  const result = conversion('warning')
  result.document.entities.channel_lines.push({
    business_id: 'line-two',
    channel_ref: 'channel-one',
    display_name: 'Line two',
    entity_hash: 'f'.repeat(64),
    line_ref: 'line-two',
    row: 5,
    sheet: 'Lines',
    source_ref: 'source-one',
    status_proposal: 'disabled',
  })
  result.document.manifest.counts.channel_lines = 2

  const mounted = await mount(result)
  const originalCrypto = globalThis.crypto
  const resolvers: Array<() => void> = []
  try {
    await upload(mounted.container)
    await act(async () => button(mounted.container, 'Selection').click())

    const firstLine = mounted.container.querySelector('[aria-label="Line one"]')
    assert.ok(firstLine instanceof browserWindow.HTMLElement)
    await act(async () => firstLine.click())
    assert.equal(
      button(mounted.container, 'Export selected JSON').disabled,
      false
    )

    Object.defineProperty(globalThis, 'crypto', {
      configurable: true,
      value: {
        subtle: {
          digest: () =>
            new Promise<ArrayBuffer>((resolve) => {
              resolvers.push(() => resolve(new ArrayBuffer(32)))
            }),
        },
      },
    })
    const secondLine = mounted.container.querySelector(
      '[aria-label="Line two"]'
    )
    assert.ok(secondLine instanceof browserWindow.HTMLElement)
    await act(async () => secondLine.click())
    assert.equal(resolvers.length, 1)

    assert.ok(
      mounted.container.querySelector('[aria-label="Line one"]') instanceof
        browserWindow.HTMLElement
    )
    await act(async () => firstLine.click())
    assert.equal(resolvers.length, 2)

    await act(async () => {
      resolvers[0]()
      await Promise.resolve()
      await Promise.resolve()
    })
    assert.equal(
      button(mounted.container, 'Export selected JSON').disabled,
      true
    )

    await act(async () => {
      resolvers[1]()
      await Promise.resolve()
      await Promise.resolve()
    })
    assert.equal(
      button(mounted.container, 'Export selected JSON').disabled,
      false
    )
  } finally {
    Object.defineProperty(globalThis, 'crypto', {
      configurable: true,
      value: originalCrypto,
    })
    await act(async () => mounted.root.unmount())
  }
})
