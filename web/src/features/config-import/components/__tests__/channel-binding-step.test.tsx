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

import type {
  ConfigImportBatchDetail,
  ConfigImportBindingsRequest,
} from '../../types'
import { ChannelBindingStep } from '../channel-binding-step'

const browserWindow = new Window({ url: 'http://localhost/' })
browserWindow.document.write('<!doctype html><html><body></body></html>')
Object.assign(globalThis as Record<string, unknown>, {
  window: browserWindow,
  document: browserWindow.document,
  navigator: browserWindow.navigator,
  HTMLElement: browserWindow.HTMLElement,
  HTMLInputElement: browserWindow.HTMLInputElement,
  HTMLSelectElement: browserWindow.HTMLSelectElement,
  Event: browserWindow.Event,
  MouseEvent: browserWindow.MouseEvent,
  IS_REACT_ACT_ENVIRONMENT: true,
})

after(() => browserWindow.close())

const i18n = createInstance()
i18n.init({
  lng: 'en',
  resources: { en: { translation: {} } },
})

const batch: ConfigImportBatchDetail = {
  id: 4,
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
  items: [
    {
      id: 1,
      entity_type: 'channel_lines',
      business_id: 'line-1',
      entity_hash: 'line-1-hash',
      canonical_json:
        '{"line_ref":"line-1","display_name":"Secure Pro","note":"sk-secret"}',
      state: 'new',
      source_ref: '渠道!4',
      source_sheet: '渠道',
      source_row: 4,
    },
  ],
  issues: [],
}

beforeEach(() => browserWindow.document.body.replaceChildren())

async function mount(
  options: {
    channels?: Array<{ id: number; name: string; status: number }>
    createdChannelIDs?: Record<string, number>
  } = {}
) {
  const saved: ConfigImportBindingsRequest[] = []
  const created: string[] = []
  const container = browserWindow.document.createElement('div')
  browserWindow.document.body.append(container)
  const root = createRoot(container as unknown as Container)
  await act(async () => {
    root.render(
      <I18nextProvider i18n={i18n}>
        <ChannelBindingStep
          batch={batch}
          channels={
            options.channels ?? [
              { id: 12, name: 'Existing disabled channel', status: 2 },
            ]
          }
          createdChannelIDs={options.createdChannelIDs}
          onSave={async (request) => {
            saved.push(request)
          }}
          onCreateChannel={(lineRef) => created.push(lineRef)}
        />
      </I18nextProvider>
    )
  })
  return { container, root, saved, created }
}

test('saves an existing channel binding only after credential confirmation', async () => {
  const mounted = await mount()
  try {
    const select = mounted.container.querySelector('select')
    assert.ok(select instanceof browserWindow.HTMLSelectElement)
    select.value = '12'
    await act(async () =>
      select.dispatchEvent(new browserWindow.Event('change', { bubbles: true }))
    )

    const unconfirmedSave = [
      ...mounted.container.querySelectorAll('button'),
    ].find((button) => button.textContent === 'Save bindings')
    assert.ok(unconfirmedSave instanceof browserWindow.HTMLButtonElement)
    await act(async () => unconfirmedSave.click())
    assert.equal(mounted.saved.length, 0)

    const checkbox = mounted.container.querySelector('input[type="checkbox"]')
    assert.ok(checkbox instanceof browserWindow.HTMLInputElement)
    await act(async () => checkbox.click())

    const save = [...mounted.container.querySelectorAll('button')].find(
      (button) => button.textContent === 'Save bindings'
    )
    assert.ok(save instanceof browserWindow.HTMLButtonElement)
    await act(async () => save.click())

    assert.deepEqual(mounted.saved, [
      {
        bindings: [
          {
            line_ref: 'line-1',
            action: 'bind',
            channel_id: 12,
            credentials_confirmed: true,
          },
        ],
      },
    ])
    assert.doesNotMatch(mounted.container.innerHTML, /sk-secret/)
  } finally {
    await act(async () => mounted.root.unmount())
  }
})

test('saves a skipped channel line without a reason input', async () => {
  const mounted = await mount()
  try {
    const skip = [...mounted.container.querySelectorAll('button')].find(
      (button) => button.textContent === 'Skip'
    )
    assert.ok(skip instanceof browserWindow.HTMLButtonElement)
    await act(async () => skip.click())

    assert.equal(
      mounted.container.querySelector('input[aria-label="Skip reason"]'),
      null
    )

    const save = [...mounted.container.querySelectorAll('button')].find(
      (button) => button.textContent === 'Save bindings'
    )
    assert.ok(save instanceof browserWindow.HTMLButtonElement)
    await act(async () => save.click())

    assert.deepEqual(mounted.saved, [
      {
        bindings: [
          {
            line_ref: 'line-1',
            action: 'skip',
            credentials_confirmed: false,
          },
        ],
      },
    ])
  } finally {
    await act(async () => mounted.root.unmount())
  }
})

test('opens channel creation and selects the returned disabled channel ID', async () => {
  const mounted = await mount({
    channels: [{ id: 25, name: 'New disabled channel', status: 2 }],
    createdChannelIDs: { 'line-1': 25 },
  })
  try {
    const select = mounted.container.querySelector('select')
    assert.ok(select instanceof browserWindow.HTMLSelectElement)
    assert.equal(select.value, '25')

    const create = [...mounted.container.querySelectorAll('button')].find(
      (button) => button.textContent === 'Create channel'
    )
    assert.ok(create instanceof browserWindow.HTMLButtonElement)
    await act(async () => create.click())
    assert.deepEqual(mounted.created, ['line-1'])
  } finally {
    await act(async () => mounted.root.unmount())
  }
})

test('selects a newly created channel over an earlier existing-channel draft', async () => {
  const channels = [
    { id: 12, name: 'Existing disabled channel', status: 2 },
    { id: 25, name: 'New disabled channel', status: 2 },
  ]
  const mounted = await mount({ channels })
  try {
    const select = mounted.container.querySelector('select')
    assert.ok(select instanceof browserWindow.HTMLSelectElement)
    select.value = '12'
    await act(async () =>
      select.dispatchEvent(new browserWindow.Event('change', { bubbles: true }))
    )

    await act(async () => {
      mounted.root.render(
        <I18nextProvider i18n={i18n}>
          <ChannelBindingStep
            batch={batch}
            channels={channels}
            createdChannelIDs={{ 'line-1': 25 }}
            onSave={async (request) => {
              mounted.saved.push(request)
            }}
            onCreateChannel={(lineRef) => mounted.created.push(lineRef)}
          />
        </I18nextProvider>
      )
    })

    assert.equal(select.value, '25')
  } finally {
    await act(async () => mounted.root.unmount())
  }
})

test('restores a persisted create binding after the batch is reloaded', async () => {
  const persistedBatch: ConfigImportBatchDetail = {
    ...batch,
    bindings: [
      {
        line_ref: 'line-1',
        action: 'create',
        channel_id: 25,
        credentials_confirmed: false,
      },
    ],
  }
  const saved: ConfigImportBindingsRequest[] = []
  const container = browserWindow.document.createElement('div')
  browserWindow.document.body.append(container)
  const root = createRoot(container as unknown as Container)
  await act(async () => {
    root.render(
      <I18nextProvider i18n={i18n}>
        <ChannelBindingStep
          batch={persistedBatch}
          channels={[{ id: 25, name: 'Created disabled channel', status: 2 }]}
          onSave={async (request) => {
            saved.push(request)
          }}
          onCreateChannel={() => undefined}
        />
      </I18nextProvider>
    )
  })
  try {
    const select = container.querySelector('select')
    assert.ok(select instanceof browserWindow.HTMLSelectElement)
    assert.equal(select.value, '25')

    const checkbox = container.querySelector('input[type="checkbox"]')
    assert.ok(checkbox instanceof browserWindow.HTMLInputElement)
    await act(async () => checkbox.click())

    const save = [...container.querySelectorAll('button')].find(
      (button) => button.textContent === 'Save bindings'
    )
    assert.ok(save instanceof browserWindow.HTMLButtonElement)
    await act(async () => save.click())

    assert.deepEqual(saved, [
      {
        bindings: [
          {
            line_ref: 'line-1',
            action: 'create',
            channel_id: 25,
            credentials_confirmed: true,
          },
        ],
      },
    ])
  } finally {
    await act(async () => root.unmount())
  }
})
