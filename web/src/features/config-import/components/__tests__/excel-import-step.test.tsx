/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import assert from 'node:assert/strict'
import test, { after, beforeEach } from 'node:test'

import { Window } from 'happy-dom'
import { createInstance } from 'i18next'
import { act } from 'react'
import { createRoot, type Container } from 'react-dom/client'
import { I18nextProvider } from 'react-i18next'

import type { WorkbookConversion } from '@/channel-config-converter/conversion'

import type { ConfigImportBatchDetail } from '../../types'
import { ExcelImportStep } from '../excel-import-step'
import { ImportSourceStep } from '../import-source-step'

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

const batch: ConfigImportBatchDetail = {
  id: 3,
  schema_version: 1,
  template_version: 'v1',
  source_sha256: 'source',
  payload_sha256: 'payload',
  status: 'binding',
  created_by: 1,
  item_counts: {
    channels: 1,
    channel_lines: 1,
    model_skus: 0,
    sale_proposals: 0,
    cost_rule_drafts: 0,
    model_mappings: 0,
    route_blueprints: 0,
    sources: 1,
    unresolved_variants: 0,
  },
  issue_count: 0,
  allowed_actions: ['bind'],
  created_at: 1,
  updated_at: 1,
  items: [],
  issues: [],
}

function conversion(): WorkbookConversion {
  return {
    document: {
      kind: 'new-api.channel-config-import',
      schema_version: 1,
      template_version: '1',
      manifest: {
        converter_version: '1.0.0',
        counts: {
          channel_lines: 2,
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
            source_ref: 'source-one',
            status_proposal: 'disabled',
          },
          {
            business_id: 'line-two',
            channel_ref: 'channel-one',
            display_name: 'Line two',
            entity_hash: 'e'.repeat(64),
            line_ref: 'line-two',
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
            entity_hash: 'f'.repeat(64),
            source_ref: 'source-one',
          },
        ],
        unresolved_variants: [],
      },
      derived_preview: {},
      issues: [],
    },
    hasFailures: false,
    hasWarnings: false,
  }
}

beforeEach(() => browserWindow.document.body.replaceChildren())

function button(container: BrowserElement, label: string): BrowserButton {
  const candidate = [...container.querySelectorAll('button')].find(
    (element) => element.textContent?.trim() === label
  )
  assert.ok(candidate instanceof browserWindow.HTMLButtonElement)
  return candidate as unknown as BrowserButton
}

async function uploadWorkbook(container: BrowserElement): Promise<void> {
  const candidate = container.querySelector('input[type="file"]')
  assert.ok(candidate instanceof browserWindow.HTMLInputElement)
  const input = candidate as unknown as BrowserInput
  Object.defineProperty(input, 'files', {
    configurable: true,
    value: [new browserWindow.File(['fixture'], 'channels.xlsx')],
  })
  await act(async () => {
    input.dispatchEvent(new browserWindow.Event('change', { bubbles: true }))
    await Promise.resolve()
    await Promise.resolve()
  })
}

test('uploads only the selected converted scope and advances to the binding batch', async () => {
  const uploaded: unknown[] = []
  const receivedBatches: ConfigImportBatchDetail[] = []
  const container = browserWindow.document.createElement('div')
  browserWindow.document.body.append(container)
  const root = createRoot(container as unknown as Container)
  await act(async () => {
    root.render(
      <I18nextProvider i18n={i18n}>
        <ExcelImportStep
          convertFile={async () => conversion()}
          onUpload={async (document) => {
            uploaded.push(document)
            return batch
          }}
          onUploaded={(nextBatch) => receivedBatches.push(nextBatch)}
        />
      </I18nextProvider>
    )
  })

  try {
    await uploadWorkbook(container)
    const line = container.querySelector('[aria-label="Line one"]')
    assert.ok(line instanceof browserWindow.HTMLElement)
    await act(async () => line.click())
    await act(async () =>
      button(container, 'Import selected configuration').click()
    )

    const document = uploaded[0] as WorkbookConversion['document']
    assert.deepEqual(
      document.entities.channel_lines.map((entry) => entry.line_ref),
      ['line-one']
    )
    assert.deepEqual(receivedBatches, [batch])
  } finally {
    await act(async () => root.unmount())
  }
})

test('retains a converted selection after create-batch failure', async () => {
  const container = browserWindow.document.createElement('div')
  browserWindow.document.body.append(container)
  const root = createRoot(container as unknown as Container)
  await act(async () => {
    root.render(
      <I18nextProvider i18n={i18n}>
        <ExcelImportStep
          convertFile={async () => conversion()}
          onUpload={async () => {
            throw new Error('create batch failed')
          }}
          onUploaded={() => undefined}
        />
      </I18nextProvider>
    )
  })

  try {
    await uploadWorkbook(container)
    const line = container.querySelector('[aria-label="Line one"]')
    assert.ok(line instanceof browserWindow.HTMLElement)
    await act(async () => line.click())
    await act(async () =>
      button(container, 'Import selected configuration').click()
    )

    assert.match(container.textContent || '', /create batch failed/)
    assert.equal(line.getAttribute('aria-checked'), 'true')
    assert.equal(
      button(container, 'Import selected configuration').disabled,
      false
    )
  } finally {
    await act(async () => root.unmount())
  }
})

test('keeps existing JSON upload behavior in JSON import mode', async () => {
  const uploaded: unknown[] = []
  const receivedBatches: ConfigImportBatchDetail[] = []
  const container = browserWindow.document.createElement('div')
  browserWindow.document.body.append(container)
  const root = createRoot(container as unknown as Container)
  await act(async () => {
    root.render(
      <I18nextProvider i18n={i18n}>
        <ImportSourceStep
          onUpload={async (document) => {
            uploaded.push(document)
            return batch
          }}
          onUploaded={(nextBatch) => receivedBatches.push(nextBatch)}
        />
      </I18nextProvider>
    )
  })

  try {
    await act(async () => button(container, 'JSON import').click())
    const candidate = container.querySelector('input[type="file"]')
    assert.ok(candidate instanceof browserWindow.HTMLInputElement)
    const input = candidate as unknown as BrowserInput
    Object.defineProperty(input, 'files', {
      configurable: true,
      value: [new browserWindow.File(['{"kind":"import"}'], 'import.json')],
    })
    await act(async () => {
      input.dispatchEvent(new browserWindow.Event('change', { bubbles: true }))
    })

    assert.deepEqual(uploaded, [{ kind: 'import' }])
    assert.deepEqual(receivedBatches, [batch])
  } finally {
    await act(async () => root.unmount())
  }
})
