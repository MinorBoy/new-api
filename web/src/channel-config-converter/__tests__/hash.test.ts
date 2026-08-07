/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import assert from 'node:assert/strict'
import test from 'node:test'

import { hashEntity, hashPayload } from '../hash'

const sourceLocations = [{ sheet: '渠道成本', row: 9, business_id: 'COST-A' }]

test('hashes equivalent entities identically despite object property order and an existing entity hash', async () => {
  const first = {
    business_id: 'COST-A',
    fields: { native_unit_price: '1.2', line_ref: 'line-a' },
    source_locations: sourceLocations,
  }
  const second = {
    entity_hash: 'ignored',
    source_locations: sourceLocations,
    fields: { line_ref: 'line-a', native_unit_price: '1.2' },
    business_id: 'COST-A',
  }

  assert.equal(await hashEntity(first), await hashEntity(second))
})

test('hashes unordered contract arrays identically to the backend contract', async () => {
  const first = {
    business_id: 'route-a',
    targets: [
      {
        route_target_ref: 'target-b',
        aspect_ratios: ['1:1', '16:9'],
        input_modes: ['text', 'omni_reference'],
        duration_values: [10, 5],
      },
      { route_target_ref: 'target-a', input_modes: ['first_frame'] },
    ],
  }
  const second = {
    business_id: 'route-a',
    targets: [
      { route_target_ref: 'target-a', input_modes: ['first_frame'] },
      {
        route_target_ref: 'target-b',
        aspect_ratios: ['16:9', '1:1'],
        input_modes: ['omni_reference', 'text'],
        duration_values: [5, 10],
      },
    ],
  }

  assert.equal(await hashEntity(first), await hashEntity(second))
  assert.equal(
    await hashEntity(first),
    '45360db378a7815e4df79ad5f42da2788e80da8a57c26572c89ab3222497d2e4'
  )
})

test('payload hashing ignores source bytes, generated metadata, issues, and previews while sorting entities by business ID', async () => {
  const first = {
    manifest: {
      source_file_name: 'first.xlsx',
      source_sha256: 'first-bytes',
      generated_at: '2026-07-26T00:00:00Z',
    },
    entities: {
      cost_rule_drafts: [
        {
          business_id: 'B',
          fields: { native_unit_price: '2' },
          source_locations: sourceLocations,
        },
        {
          business_id: 'A',
          fields: { native_unit_price: '1' },
          source_locations: sourceLocations,
        },
      ],
    },
    issues: [{ code: 'WARN' }],
    derived_preview: { margin: '0.1' },
  }
  const second = {
    manifest: {
      source_file_name: 'second.xlsx',
      source_sha256: 'different-bytes',
      generated_at: '2026-07-27T00:00:00Z',
    },
    entities: {
      cost_rule_drafts: [
        {
          business_id: 'A',
          fields: { native_unit_price: '1' },
          source_locations: sourceLocations,
        },
        {
          business_id: 'B',
          fields: { native_unit_price: '2' },
          source_locations: sourceLocations,
        },
      ],
    },
    issues: [],
    derived_preview: { margin: '0.9' },
  }

  assert.equal(await hashPayload(first), await hashPayload(second))
})

test('payload hashing follows the import contract by retaining entity hashes and excluding document metadata', async () => {
  const entity = {
    business_id: 'SOURCE-A',
    entity_hash: 'a'.repeat(64),
    source_ref: 'SOURCE-A',
  }
  const first = {
    kind: 'new-api.channel-config-import',
    schema_version: 1,
    template_version: '1',
    entities: { sources: [entity] },
  }
  const changedEntityHash = {
    ...first,
    entities: {
      sources: [{ ...entity, entity_hash: 'b'.repeat(64) }],
    },
  }
  const changedMetadata = {
    ...first,
    kind: 'different-kind',
    schema_version: 99,
    template_version: 'future',
  }

  assert.notEqual(
    await hashPayload(first),
    await hashPayload(changedEntityHash)
  )
  assert.equal(await hashPayload(first), await hashPayload(changedMetadata))
})

test('empty group routing requirements preserve legacy payload hashes', async () => {
  const legacy = { entities: { sources: [] } }
  const extended = {
    entities: { sources: [], group_routing_requirements: [] },
  }

  assert.equal(await hashPayload(legacy), await hashPayload(extended))
})

test('payload hashing sorts typed route constraints but preserves reference mode order', async () => {
  const route = {
    business_id: 'route-a',
    targets: [
      {
        route_target_ref: 'target-a',
        line_ref: 'line-a',
        sku_ref: 'sku-a',
        input_modes: ['text', 'omni_reference'],
        reference_modes: ['first_last_frames', 'agentic'],
      },
    ],
  }
  const reorderedInputModes = {
    ...route,
    targets: [
      {
        ...route.targets[0],
        input_modes: ['omni_reference', 'text'],
      },
    ],
  }
  const reorderedReferenceModes = {
    ...route,
    targets: [
      {
        ...route.targets[0],
        reference_modes: ['agentic', 'first_last_frames'],
      },
    ],
  }

  assert.equal(
    await hashPayload({ entities: { route_blueprints: [route] } }),
    await hashPayload({
      entities: { route_blueprints: [reorderedInputModes] },
    })
  )
  assert.notEqual(
    await hashPayload({ entities: { route_blueprints: [route] } }),
    await hashPayload({
      entities: { route_blueprints: [reorderedReferenceModes] },
    })
  )
})
