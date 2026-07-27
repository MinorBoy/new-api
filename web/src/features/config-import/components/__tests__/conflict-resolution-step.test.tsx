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
  type HTMLSelectElement as HappyHTMLSelectElement,
} from 'happy-dom'
import { createInstance } from 'i18next'
import { act } from 'react'
import { createRoot, type Container } from 'react-dom/client'
import { I18nextProvider } from 'react-i18next'

import type { ConfigImportBatchDetail } from '../../types'
import { ConflictResolutionStep } from '../conflict-resolution-step'

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
  id: 4,
  schema_version: 1,
  template_version: 'v1',
  source_sha256: 'source',
  payload_sha256: 'payload',
  status: 'staged',
  created_by: 1,
  item_counts: {
    channels: 0,
    channel_lines: 1,
    model_skus: 0,
    sale_proposals: 0,
    cost_rule_drafts: 0,
    model_mappings: 0,
    route_blueprints: 1,
    sources: 0,
    unresolved_variants: 1,
  },
  issue_count: 1,
  allowed_actions: ['resolve'],
  created_at: 1,
  updated_at: 1,
  items: [
    {
      id: 1,
      entity_type: 'channel_lines',
      business_id: 'secure-no-face',
      entity_hash: 'line-hash',
      canonical_json: '{"line_ref":"secure-no-face"}',
      state: 'changed',
      source_ref: '渠道!4',
      source_sheet: '渠道',
      source_row: 4,
    },
    {
      id: 2,
      entity_type: 'route_blueprints',
      business_id: 'route-video',
      entity_hash: 'route-hash',
      canonical_json:
        '{"targets":[{"route_target_ref":"target-no-face","cost_variant_key":"no-face"}]}',
      state: 'changed',
      source_ref: '路由!12',
      source_sheet: '路由',
      source_row: 12,
    },
    {
      id: 3,
      entity_type: 'unresolved_variants',
      business_id: 'variant-no-face',
      entity_hash: 'variant-hash',
      canonical_json:
        '{"line_ref":"secure-no-face","reason":"Face capability differs"}',
      state: 'conflict',
      source_ref: '渠道成本!8',
      source_sheet: '渠道成本',
      source_row: 8,
      conflict_reason: 'Cost variant requires a structured resolution',
    },
  ],
  issues: [
    {
      id: 1,
      severity: 'warning',
      code: 'COST_VARIANT_AMBIGUOUS',
      business_id: 'variant-no-face',
      sheet: '渠道成本',
      row: 8,
      message: 'Cost variant requires a structured resolution',
      resolution_status: 'open',
    },
  ],
}

beforeEach(() => browserWindow.document.body.replaceChildren())

async function mount() {
  const saved: unknown[] = []
  const container = browserWindow.document.createElement('div')
  browserWindow.document.body.append(container)
  const root = createRoot(container as unknown as Container)
  await act(async () => {
    root.render(
      <I18nextProvider i18n={i18n}>
        <ConflictResolutionStep
          batch={batch}
          onSave={async (request) => {
            saved.push(request)
          }}
        />
      </I18nextProvider>
    )
  })
  return { container, root, saved }
}

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

async function selectValue(
  container: HappyHTMLElement,
  label: string,
  value: string
) {
  const select = container.querySelector(
    `[aria-label="${label}"]`
  ) as HappyHTMLSelectElement | null
  assert.ok(select)
  select.value = value
  await act(async () =>
    select.dispatchEvent(new browserWindow.Event('change', { bubbles: true }))
  )
}

async function chooseAction(container: HappyHTMLElement, text: string) {
  await act(async () => buttonByText(container, text).click())
}

test('requires an explicit conflict action before a warning can be saved', async () => {
  const mounted = await mount()
  try {
    await act(async () =>
      buttonByText(mounted.container, 'Save resolutions').click()
    )
    assert.equal(mounted.saved.length, 0)
    assert.match(mounted.container.textContent ?? '', /resolution action/i)
  } finally {
    await act(async () => mounted.root.unmount())
  }
})

test('saves a split-line decision with its selected line reference', async () => {
  const mounted = await mount()
  try {
    await chooseAction(mounted.container, 'Split line')
    await selectValue(mounted.container, 'Line reference', 'secure-no-face')
    await act(async () =>
      buttonByText(mounted.container, 'Save resolutions').click()
    )

    assert.deepEqual(mounted.saved, [
      {
        resolutions: [
          {
            item_business_id: 'variant-no-face',
            action: 'split_line',
            line_ref: 'secure-no-face',
          },
        ],
      },
    ])
  } finally {
    await act(async () => mounted.root.unmount())
  }
})

test('saves a variant binding with matching cost and route references', async () => {
  const mounted = await mount()
  try {
    await chooseAction(mounted.container, 'Bind variant')
    await selectValue(mounted.container, 'Cost variant key', 'no-face')
    await selectValue(
      mounted.container,
      'Route target reference',
      'target-no-face'
    )
    await act(async () =>
      buttonByText(mounted.container, 'Save resolutions').click()
    )

    assert.deepEqual(mounted.saved[0], {
      resolutions: [
        {
          item_business_id: 'variant-no-face',
          action: 'bind_variant',
          cost_variant_key: 'no-face',
          route_target_ref: 'target-no-face',
        },
      ],
    })
    assert.match(mounted.container.textContent ?? '', /Face capability differs/)
  } finally {
    await act(async () => mounted.root.unmount())
  }
})

test('requires a reason before excluding a conflict', async () => {
  const mounted = await mount()
  try {
    await chooseAction(mounted.container, 'Exclude')
    await act(async () =>
      buttonByText(mounted.container, 'Save resolutions').click()
    )
    assert.equal(mounted.saved.length, 0)

    const reason = mounted.container.querySelector(
      '[aria-label="Exclusion reason"]'
    )
    assert.ok(reason instanceof browserWindow.HTMLInputElement)
    reason.value = 'Not offered by this supplier'
    await act(async () =>
      reason.dispatchEvent(new browserWindow.Event('input', { bubbles: true }))
    )
    await act(async () =>
      buttonByText(mounted.container, 'Save resolutions').click()
    )

    assert.deepEqual(mounted.saved[0], {
      resolutions: [
        {
          item_business_id: 'variant-no-face',
          action: 'exclude',
          reason: 'Not offered by this supplier',
        },
      ],
    })
  } finally {
    await act(async () => mounted.root.unmount())
  }
})
