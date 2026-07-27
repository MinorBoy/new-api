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
  type HTMLButtonElement as HappyHTMLButtonElement,
  type HTMLElement as HappyHTMLElement,
} from 'happy-dom'
import { createInstance } from 'i18next'
import { act } from 'react'
import { createRoot, type Container } from 'react-dom/client'
import { I18nextProvider } from 'react-i18next'

import type { ConfigImportBatchDetail } from '../../types'
import { RoutingDiffStep } from '../routing-diff-step'

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
i18n.init({ lng: 'en', resources: { en: { translation: {} } } })

const batch: ConfigImportBatchDetail = {
  id: 6,
  schema_version: 1,
  template_version: 'v1',
  source_sha256: 'source',
  payload_sha256: 'payload',
  status: 'staged',
  created_by: 1,
  item_counts: {
    channels: 0,
    channel_lines: 1,
    model_skus: 1,
    sale_proposals: 0,
    cost_rule_drafts: 0,
    model_mappings: 0,
    route_blueprints: 1,
    sources: 0,
    unresolved_variants: 0,
  },
  issue_count: 0,
  allowed_actions: ['validate'],
  created_at: 1,
  updated_at: 1,
  items: [
    {
      id: 1,
      entity_type: 'route_blueprints',
      business_id: 'route-video',
      entity_hash: 'route-hash',
      canonical_json:
        '{"canonical_model":"video-v2","merge_mode":"replace","targets":[{"route_target_ref":"target-720p","line_ref":"secure-no-face","upstream_model":"video-v2","sku_ref":"video-sku","cost_variant_key":"no-face","output_resolutions":["720p"],"duration_values":[5],"enabled":false}]}',
      state: 'changed',
      source_ref: '路由!21',
      source_sheet: '路由',
      source_row: 21,
    },
  ],
  issues: [],
}

beforeEach(() => browserWindow.document.body.replaceChildren())

function buttonByText(
  container: HappyHTMLElement,
  text: string
): HappyHTMLButtonElement {
  const button = [...container.querySelectorAll('button')].find(
    (candidate) => candidate.textContent === text
  ) as HappyHTMLButtonElement | undefined
  assert.ok(button)
  return button
}

test('shows replacement deletions, target constraints, and requires explicit confirmation', async () => {
  const reviewed: unknown[] = []
  const container = browserWindow.document.createElement('div')
  browserWindow.document.body.append(container)
  const root = createRoot(container as unknown as Container)
  await act(async () => {
    root.render(
      <I18nextProvider i18n={i18n}>
        <RoutingDiffStep
          batch={batch}
          existingTargets={{
            'route-video': ['legacy-target-a', 'legacy-target-b'],
          }}
          isStaleBaseline
          onReview={(value) => reviewed.push(value)}
        />
      </I18nextProvider>
    )
  })
  try {
    assert.match(container.textContent ?? '', /configuration changed/i)
    assert.match(container.textContent ?? '', /legacy-target-a/)
    assert.match(container.textContent ?? '', /no-face/)
    assert.match(container.textContent ?? '', /720p/)
    assert.match(container.textContent ?? '', /路由:21/)

    await act(async () => buttonByText(container, 'Continue').click())
    assert.equal(reviewed.length, 0)

    const confirm = container.querySelector(
      '[aria-label="Confirm replacement"]'
    )
    assert.ok(confirm instanceof browserWindow.HTMLInputElement)
    await act(async () => confirm.click())
    await act(async () => buttonByText(container, 'Continue').click())

    assert.deepEqual(reviewed, [
      [{ business_id: 'route-video', merge_mode: 'replace' }],
    ])
  } finally {
    await act(async () => root.unmount())
  }
})

test('allows merge and skip review modes without a replacement confirmation', async () => {
  const reviewed: unknown[] = []
  const container = browserWindow.document.createElement('div')
  browserWindow.document.body.append(container)
  const root = createRoot(container as unknown as Container)
  await act(async () => {
    root.render(
      <I18nextProvider i18n={i18n}>
        <RoutingDiffStep
          batch={batch}
          onReview={(value) => reviewed.push(value)}
        />
      </I18nextProvider>
    )
  })
  try {
    const mode = container.querySelector('[aria-label="Route merge mode"]')
    assert.ok(mode instanceof browserWindow.HTMLSelectElement)
    mode.value = 'merge'
    await act(async () =>
      mode.dispatchEvent(new browserWindow.Event('change', { bubbles: true }))
    )
    await act(async () => buttonByText(container, 'Continue').click())
    assert.deepEqual(reviewed, [
      [{ business_id: 'route-video', merge_mode: 'merge' }],
    ])

    mode.value = 'skip'
    await act(async () =>
      mode.dispatchEvent(new browserWindow.Event('change', { bubbles: true }))
    )
    await act(async () => buttonByText(container, 'Continue').click())
    assert.deepEqual(reviewed, [
      [{ business_id: 'route-video', merge_mode: 'merge' }],
      [{ business_id: 'route-video', merge_mode: 'skip' }],
    ])
  } finally {
    await act(async () => root.unmount())
  }
})
