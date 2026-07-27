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

import { materializeRouting } from '../conflicts'

const candidate = {
  lineRef: 'secure-discount',
  upstreamModel: 'video-2.0-pro',
  skuRef: 'SKU-STD-720',
  resolution: '720p',
  nativeUnitPrice: '6.8',
  supportsRealPerson: true,
}

test('deduplicates equivalent scenarios while retaining both source IDs and binding a disabled route to one variant', () => {
  const result = materializeRouting([
    {
      ...candidate,
      businessId: 'COST-ONE-NOV',
      scenario: 'no_video',
      sourceBusinessIds: ['COST-ONE-NOV'],
    },
    {
      ...candidate,
      businessId: 'COST-ONE-VID',
      scenario: 'with_video',
      sourceBusinessIds: ['COST-ONE-VID'],
    },
  ])

  assert.equal(result.costs.length, 1)
  assert.deepEqual(result.costs[0].source_business_ids, [
    'COST-ONE-NOV',
    'COST-ONE-VID',
  ])
  assert.deepEqual(result.routes, [
    {
      route_target_ref: 'route/secure-discount/video-2.0-pro/720p/real-person',
      line_ref: 'secure-discount',
      upstream_model: 'video-2.0-pro',
      sku_ref: 'SKU-STD-720',
      cost_variant_key: '720p',
      supports_real_person: true,
      enabled: false,
    },
  ])
  assert.deepEqual(result.issues, [])
})

test('rejects multiple prices for the same structured request conditions without choosing one', () => {
  const result = materializeRouting([
    {
      ...candidate,
      businessId: 'COST-LOW',
      scenario: 'no_video',
      sourceBusinessIds: ['COST-LOW'],
    },
    {
      ...candidate,
      businessId: 'COST-HIGH',
      nativeUnitPrice: '8.8',
      scenario: 'no_video',
      sourceBusinessIds: ['COST-HIGH'],
    },
  ])

  assert.equal(result.routes.length, 0)
  assert.deepEqual(result.issues, [
    {
      code: 'COST_VARIANT_AMBIGUOUS',
      business_id: 'secure-discount/video-2.0-pro/SKU-STD-720/720p/real-person',
      source_business_ids: ['COST-HIGH', 'COST-LOW'],
    },
  ])
})
