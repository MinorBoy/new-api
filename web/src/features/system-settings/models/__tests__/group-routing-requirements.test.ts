import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import {
  effectiveRealPersonMode,
  isDynamicGroupRoutingProfile,
  parseGroupRoutingProfiles,
  removeDynamicGroupRoutingProfile,
  toggleGroupRoutingTargetExclusion,
  updateGroupRoutingProfile,
  updateGroupRoutingRequirements,
} from '../group-routing-requirements'

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

  test('creates an active default-backed profile without changing other groups', () => {
    const next = updateGroupRoutingProfile(
      '{"旧分组":{"legacy_field":"keep"}}',
      '客户A',
      {
        status: 'active',
        routing_source: 'default',
        real_person_mode: 'required',
        allowed_cost_modes: ['per_duration', 'per_request'],
        excluded_target_keys: [],
      }
    )

    assert.deepEqual(JSON.parse(next), {
      客户A: {
        status: 'active',
        routing_source: 'default',
        real_person_mode: 'required',
        allowed_cost_modes: ['per_duration', 'per_request'],
      },
      旧分组: { legacy_field: 'keep' },
    })
  })

  test('maps legacy require_real_person to the effective required mode', () => {
    const profiles = parseGroupRoutingProfiles(
      '{"真人分组":{"require_real_person":true}}'
    )

    assert.equal(effectiveRealPersonMode(profiles['真人分组']), 'required')
    assert.equal(isDynamicGroupRoutingProfile(profiles['真人分组']), false)
  })

  test('toggles a stable exclusion key without duplicates', () => {
    const excluded = toggleGroupRoutingTargetExclusion(
      '{"客户A":{"status":"draft","routing_source":"default"}}',
      '客户A',
      'grt_target',
      true
    )
    const excludedAgain = toggleGroupRoutingTargetExclusion(
      excluded,
      '客户A',
      'grt_target',
      true
    )
    const restored = toggleGroupRoutingTargetExclusion(
      excludedAgain,
      '客户A',
      'grt_target',
      false
    )

    assert.deepEqual(JSON.parse(excludedAgain).客户A.excluded_target_keys, [
      'grt_target',
    ])
    assert.equal(
      Object.hasOwn(JSON.parse(restored).客户A, 'excluded_target_keys'),
      false
    )
  })

  test('removes only dynamic fields and preserves legacy fields', () => {
    const next = removeDynamicGroupRoutingProfile(
      '{"客户A":{"require_real_person":true,"status":"draft","routing_source":"default","legacy_field":"keep"}}',
      '客户A'
    )

    assert.deepEqual(JSON.parse(next), {
      客户A: { require_real_person: true, legacy_field: 'keep' },
    })
  })
})
