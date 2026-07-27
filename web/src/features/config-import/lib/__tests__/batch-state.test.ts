/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.
*/
import assert from 'node:assert/strict'
import test from 'node:test'

import type { ConfigImportBatchDetail } from '../../types'
import { deriveWizardState } from '../batch-state'

function batch(
  overrides: Partial<ConfigImportBatchDetail> = {}
): ConfigImportBatchDetail {
  return {
    id: 1,
    schema_version: 1,
    template_version: 'v1',
    source_sha256: 'source',
    payload_sha256: 'payload',
    status: 'binding',
    created_by: 1,
    item_counts: {
      channels: 0,
      channel_lines: 1,
      model_skus: 0,
      sale_proposals: 0,
      cost_rule_drafts: 0,
      model_mappings: 0,
      route_blueprints: 0,
      sources: 0,
      unresolved_variants: 0,
    },
    issue_count: 1,
    allowed_actions: ['bind'],
    created_at: 1,
    updated_at: 1,
    items: [],
    issues: [
      {
        id: 1,
        severity: 'error',
        code: 'CHANNEL_LINE_UNBOUND',
        message: 'Bind this channel line before continuing.',
        resolution_status: 'open',
      },
    ],
    ...overrides,
  }
}

test('derives a blocked binding state from unresolved backend issues', () => {
  assert.deepEqual(deriveWizardState(batch()), {
    step: 'channel_binding',
    canGoBack: true,
    canContinue: false,
    canPublish: false,
    blockingCodes: ['CHANNEL_LINE_UNBOUND'],
  })
})

test('does not promote warnings to a ready or publishable state', () => {
  const result = deriveWizardState(
    batch({
      status: 'ready',
      allowed_actions: ['publish'],
      issues: [
        {
          id: 2,
          severity: 'warning',
          code: 'MARGIN_WARNING',
          message: 'Margin is below the recommended threshold.',
          resolution_status: 'open',
        },
      ],
    })
  )

  assert.equal(result.step, 'publish_review')
  assert.equal(result.canPublish, false)
  assert.deepEqual(result.blockingCodes, ['MARGIN_WARNING'])
})

test('published and failed batches resolve to the terminal result step', () => {
  assert.equal(
    deriveWizardState(batch({ status: 'published', issues: [] })).step,
    'publish_result'
  )
  assert.equal(
    deriveWizardState(batch({ status: 'publish_failed', issues: [] })).step,
    'publish_result'
  )
})

test('does not allow navigation while the backend is validating or publishing', () => {
  assert.equal(
    deriveWizardState(
      batch({ status: 'validating', allowed_actions: [], issues: [] })
    ).canContinue,
    false
  )
  assert.equal(
    deriveWizardState(
      batch({ status: 'publishing', allowed_actions: [], issues: [] })
    ).canContinue,
    false
  )
})

test('does not block a ready batch for an issue excluded by the backend', () => {
  const result = deriveWizardState(
    batch({
      status: 'ready',
      allowed_actions: ['publish'],
      issues: [
        {
          id: 3,
          severity: 'warning',
          code: 'UNSUPPORTED_VARIANT',
          message: 'This variant was excluded.',
          resolution_status: 'excluded',
        },
      ],
    })
  )

  assert.equal(result.canPublish, true)
  assert.deepEqual(result.blockingCodes, [])
})
