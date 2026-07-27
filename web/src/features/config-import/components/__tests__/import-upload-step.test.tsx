/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the License,
or (at your option) any later version.
*/
import assert from 'node:assert/strict'
import test, { after, beforeEach } from 'node:test'

import { Window } from 'happy-dom'
import { createInstance } from 'i18next'
import { act } from 'react'
import { createRoot, type Container } from 'react-dom/client'
import { I18nextProvider } from 'react-i18next'

import type { ConfigImportBatchDetail } from '../../types'
import { ImportUploadStep } from '../import-upload-step'

const browserWindow = new Window({ url: 'http://localhost/' })
browserWindow.document.write('<!doctype html><html><body></body></html>')
Object.assign(globalThis as Record<string, unknown>, {
  window: browserWindow,
  document: browserWindow.document,
  navigator: browserWindow.navigator,
  HTMLElement: browserWindow.HTMLElement,
  HTMLInputElement: browserWindow.HTMLInputElement,
  Event: browserWindow.Event,
  IS_REACT_ACT_ENVIRONMENT: true,
})

after(() => browserWindow.close())

const i18n = createInstance()
i18n.init({
  lng: 'en',
  resources: { en: { translation: {} } },
})

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
    sources: 0,
    unresolved_variants: 0,
  },
  issue_count: 0,
  allowed_actions: ['bind'],
  created_at: 1,
  updated_at: 1,
  items: [],
  issues: [],
}

beforeEach(() => browserWindow.document.body.replaceChildren())

async function mount(
  onUpload: (document: unknown) => Promise<ConfigImportBatchDetail>
) {
  const uploaded: ConfigImportBatchDetail[] = []
  const container = browserWindow.document.createElement('div')
  browserWindow.document.body.append(container)
  const root = createRoot(container as unknown as Container)
  await act(async () => {
    root.render(
      <I18nextProvider i18n={i18n}>
        <ImportUploadStep
          onUpload={onUpload}
          onUploaded={(result) => uploaded.push(result)}
        />
      </I18nextProvider>
    )
  })
  return { container, root, uploaded }
}

async function selectFile(
  input: InstanceType<typeof browserWindow.HTMLInputElement>,
  file: InstanceType<typeof browserWindow.File>
) {
  Object.defineProperty(input, 'files', { value: [file], configurable: true })
  await act(async () => {
    input.dispatchEvent(new browserWindow.Event('change', { bubbles: true }))
  })
}

test('rejects a non-JSON local file before upload', async () => {
  const mounted = await mount(async () => batch)
  try {
    const input = mounted.container.querySelector('input[type="file"]')
    assert.ok(input instanceof browserWindow.HTMLInputElement)
    await selectFile(input, new browserWindow.File(['x'], 'import.txt'))

    assert.match(mounted.container.textContent || '', /Select a JSON file/)
    assert.equal(mounted.uploaded.length, 0)
  } finally {
    await act(async () => mounted.root.unmount())
  }
})

test('uploads parsed JSON without rendering its raw contents', async () => {
  let received: unknown
  const mounted = await mount(async (document) => {
    received = document
    return batch
  })
  try {
    const input = mounted.container.querySelector('input[type="file"]')
    assert.ok(input instanceof browserWindow.HTMLInputElement)
    await selectFile(
      input,
      new browserWindow.File(
        ['{"kind":"channel_config_import"}'],
        'import.json',
        {
          type: 'application/json',
        }
      )
    )

    assert.deepEqual(received, { kind: 'channel_config_import' })
    assert.deepEqual(mounted.uploaded, [batch])
    assert.doesNotMatch(mounted.container.innerHTML, /channel_config_import/)
  } finally {
    await act(async () => mounted.root.unmount())
  }
})
