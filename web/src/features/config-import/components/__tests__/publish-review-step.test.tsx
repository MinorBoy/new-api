/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import assert from 'node:assert/strict'
import test, { after, beforeEach } from 'node:test'

import {
  Window,
  type HTMLButtonElement,
  type HTMLElement as HappyHTMLElement,
  type HTMLInputElement,
} from 'happy-dom'
import { createInstance } from 'i18next'
import { act } from 'react'
import { createRoot, type Container } from 'react-dom/client'
import { I18nextProvider } from 'react-i18next'

import type { ConfigImportBatchDetail } from '../../types'
import { PublishReviewStep } from '../publish-review-step'

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
  MouseEvent: browserWindow.MouseEvent,
  IS_REACT_ACT_ENVIRONMENT: true,
})

after(() => browserWindow.close())

const i18n = createInstance()
i18n.init({ lng: 'en', resources: { en: { translation: {} } } })

function batch(
  overrides: Partial<ConfigImportBatchDetail> = {}
): ConfigImportBatchDetail {
  return {
    id: 10,
    schema_version: 1,
    template_version: 'v1',
    source_sha256: 'source',
    payload_sha256: 'payload',
    status: 'ready',
    created_by: 1,
    item_counts: {
      channels: 2,
      channel_lines: 2,
      model_skus: 1,
      sale_proposals: 1,
      cost_rule_drafts: 2,
      model_mappings: 1,
      route_blueprints: 1,
      sources: 1,
      unresolved_variants: 0,
    },
    issue_count: 0,
    allowed_actions: ['publish'],
    created_at: 1,
    updated_at: 1,
    items: [
      {
        id: 1,
        entity_type: 'route_blueprints',
        business_id: 'route-video',
        entity_hash: 'hash',
        canonical_json: '{}',
        state: 'changed',
        source_ref: '路由!12',
        source_sheet: '路由',
        source_row: 12,
      },
    ],
    issues: [],
    ...overrides,
  }
}

beforeEach(() => browserWindow.document.body.replaceChildren())

async function mount(
  options: {
    canPublish?: boolean
    currentBatch?: ConfigImportBatchDetail
    onPublish?: () => Promise<void>
  } = {}
) {
  const published: number[] = []
  const container = browserWindow.document.createElement('div')
  browserWindow.document.body.append(container)
  const root = createRoot(container as unknown as Container)
  await act(async () => {
    root.render(
      <I18nextProvider i18n={i18n}>
        <PublishReviewStep
          batch={options.currentBatch ?? batch()}
          canPublish={options.canPublish ?? true}
          onPublish={async () => {
            published.push(options.currentBatch?.id ?? 10)
            await options.onPublish?.()
          }}
        />
      </I18nextProvider>
    )
  })
  return { container, root, published }
}

function button(container: HappyHTMLElement, text: string): HTMLButtonElement {
  const value = [...container.querySelectorAll('button')].find(
    (candidate) => candidate.textContent === text
  ) as HTMLButtonElement | undefined
  assert.ok(value)
  return value
}

test('requires publish permission and final confirmation before publishing', async () => {
  const mounted = await mount({ canPublish: false })
  try {
    const confirm = mounted.container.querySelector(
      '[aria-label="Confirm publish"]'
    ) as HTMLInputElement | null
    assert.ok(confirm)
    await act(async () => confirm.click())
    const publish = button(mounted.container, 'Publish import')
    assert.equal(publish.disabled, true)
    await act(async () => publish.click())
    assert.deepEqual(mounted.published, [])
  } finally {
    await act(async () => mounted.root.unmount())
  }
})

test('publishes only after confirmation when the backend allows publishing', async () => {
  const mounted = await mount()
  try {
    const publish = button(mounted.container, 'Publish import')
    await act(async () => publish.click())
    assert.deepEqual(mounted.published, [])

    const confirm = mounted.container.querySelector(
      '[aria-label="Confirm publish"]'
    ) as HTMLInputElement | null
    assert.ok(confirm)
    await act(async () => confirm.click())
    await act(async () => publish.click())
    assert.deepEqual(mounted.published, [10])
    assert.match(mounted.container.textContent ?? '', /publish order/i)
  } finally {
    await act(async () => mounted.root.unmount())
  }
})

test('keeps publishing disabled when a warning is unresolved', async () => {
  const mounted = await mount({
    currentBatch: batch({
      issues: [
        {
          id: 2,
          severity: 'warning',
          code: 'MARGIN_WARNING',
          message: 'Margin needs review.',
          resolution_status: 'open',
        },
      ],
    }),
  })
  try {
    const publish = button(mounted.container, 'Publish import')
    assert.equal(publish.disabled, true)
    assert.match(mounted.container.textContent ?? '', /MARGIN_WARNING/)
  } finally {
    await act(async () => mounted.root.unmount())
  }
})
