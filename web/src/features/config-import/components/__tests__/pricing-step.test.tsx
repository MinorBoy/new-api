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

import type { ConfigImportBatchDetail } from '../../types'
import { PricingStep } from '../pricing-step'

const browserWindow = new Window({ url: 'http://localhost/' })
browserWindow.document.write('<!doctype html><html><body></body></html>')
Object.assign(globalThis as Record<string, unknown>, {
  window: browserWindow,
  document: browserWindow.document,
  navigator: browserWindow.navigator,
  HTMLElement: browserWindow.HTMLElement,
  HTMLInputElement: browserWindow.HTMLInputElement,
  IS_REACT_ACT_ENVIRONMENT: true,
})

after(() => browserWindow.close())

const i18n = createInstance()
i18n.init({ lng: 'en', resources: { en: { translation: {} } } })

const batch: ConfigImportBatchDetail = {
  id: 5,
  schema_version: 1,
  template_version: 'v1',
  source_sha256: 'source',
  payload_sha256: 'payload',
  status: 'staged',
  created_by: 1,
  item_counts: {
    channels: 0,
    channel_lines: 0,
    model_skus: 1,
    sale_proposals: 1,
    cost_rule_drafts: 1,
    model_mappings: 0,
    route_blueprints: 0,
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
      entity_type: 'sale_proposals',
      business_id: 'sale-video',
      entity_hash: 'sale-hash',
      canonical_json:
        '{"model_sku_ref":"video-sku","currency":"CNY","unit_price":"4.000","margin_ratio":"0.25","selected_groups":["default","vip"],"group_prices":{"default":"4.000","vip":"4.800"}}',
      state: 'changed',
      source_ref: '定价!11',
      source_sheet: '定价',
      source_row: 11,
    },
    {
      id: 2,
      entity_type: 'cost_rule_drafts',
      business_id: 'cost-video',
      entity_hash: 'cost-hash',
      canonical_json:
        '{"unit_price":"3.000","normalized_usd_unit_price":"0.420"}',
      state: 'changed',
      source_ref: '渠道成本!11',
      source_sheet: '渠道成本',
      source_row: 11,
    },
  ],
  issues: [],
}

beforeEach(() => browserWindow.document.body.replaceChildren())

test('selects the default group and displays current, proposed, and server pricing facts', async () => {
  const selectedGroups: string[][] = []
  const container = browserWindow.document.createElement('div')
  browserWindow.document.body.append(container)
  const root = createRoot(container as unknown as Container)
  await act(async () => {
    root.render(
      <I18nextProvider i18n={i18n}>
        <PricingStep
          batch={batch}
          currentValues={{ 'sale-video:unit_price': '3.500' }}
          onSelectedGroupsChange={(groups) => selectedGroups.push(groups)}
        />
      </I18nextProvider>
    )
  })
  try {
    const defaultGroup = container.querySelector('[aria-label="default"]')
    assert.ok(defaultGroup instanceof browserWindow.HTMLInputElement)
    assert.equal(defaultGroup.checked, true)
    assert.match(container.textContent ?? '', /3\.500/)
    assert.match(container.textContent ?? '', /4\.000/)
    assert.match(container.textContent ?? '', /0\.420/)
    assert.match(container.textContent ?? '', /0\.25/)
    assert.match(container.textContent ?? '', /定价:11/)

    const vipGroup = container.querySelector('[aria-label="vip"]')
    assert.ok(vipGroup instanceof browserWindow.HTMLInputElement)
    await act(async () => vipGroup.click())
    assert.deepEqual(selectedGroups.at(-1), ['default'])
  } finally {
    await act(async () => root.unmount())
  }
})

test('shows existing groups as selectable and submits the confirmed pricing scope', async () => {
  const submitted: string[][] = []
  const container = browserWindow.document.createElement('div')
  browserWindow.document.body.append(container)
  const root = createRoot(container as unknown as Container)
  await act(async () => {
    root.render(
      <I18nextProvider i18n={i18n}>
        <PricingStep
          batch={batch}
          availableGroups={['default', 'vip', '分组A']}
          onContinue={(groups) => submitted.push(groups)}
        />
      </I18nextProvider>
    )
  })
  try {
    const groupA = container.querySelector('[aria-label="分组A"]')
    assert.ok(groupA instanceof browserWindow.HTMLInputElement)
    assert.equal(groupA.checked, false)
    await act(async () => groupA.click())

    const continueButton = [...container.querySelectorAll('button')].find(
      (button) => button.textContent === 'Continue'
    )
    assert.ok(continueButton instanceof browserWindow.HTMLButtonElement)
    await act(async () => continueButton.click())
    assert.deepEqual(submitted, [['default', '分组A', 'vip']])
  } finally {
    await act(async () => root.unmount())
  }
})
