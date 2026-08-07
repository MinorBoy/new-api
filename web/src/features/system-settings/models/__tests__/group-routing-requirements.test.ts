import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import { updateGroupRoutingRequirements } from '../group-routing-requirements'

describe('group routing requirements serialization', () => {
  test('enables real-person routing for one group without changing other group settings', () => {
    const next = updateGroupRoutingRequirements('{}', '真人分组', true)

    assert.deepEqual(JSON.parse(next), {
      真人分组: { require_real_person: true },
    })
  })

  test('removing the toggle writes false explicitly and preserves unknown groups', () => {
    const next = updateGroupRoutingRequirements(
      '{"真人分组":{"require_real_person":true},"default":{"require_real_person":false}}',
      '真人分组',
      false
    )

    assert.deepEqual(JSON.parse(next), {
      真人分组: { require_real_person: false },
      default: { require_real_person: false },
    })
  })

  test('reports malformed JSON with a stable validation error', () => {
    assert.throws(
      () => updateGroupRoutingRequirements('{', '真人分组', true),
      /must be valid JSON/
    )
  })

  test('rejects non-object JSON values', () => {
    assert.throws(
      () => updateGroupRoutingRequirements('[]', '真人分组', true),
      /must be a JSON object/
    )
  })
})
