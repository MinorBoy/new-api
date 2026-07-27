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
