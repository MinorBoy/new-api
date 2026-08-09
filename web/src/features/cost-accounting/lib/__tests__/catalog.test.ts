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

import {
  costCatalogParamsFromSearch,
  formatCatalogPrice,
  formatCatalogTimestamp,
  updateCatalogSearch,
} from '../catalog'

test('maps catalog URL state to trimmed API parameters', () => {
  assert.deepEqual(
    costCatalogParamsFromSearch({
      tab: 'catalog',
      catalogChannelId: 23,
      catalogModel: '  vendor-model  ',
      catalogCostMode: 'per_request',
      catalogStatus: 'all',
      catalogCurrency: 'cny',
      catalogSource: '  config_import ',
      catalogPage: 2,
      catalogPageSize: 100,
      catalogSort: 'version',
      catalogOrder: 'desc',
    }),
    {
      channel_id: 23,
      billable_upstream_model: 'vendor-model',
      cost_mode: 'per_request',
      status: 'all',
      currency: 'CNY',
      source: 'config_import',
      page: 2,
      page_size: 100,
      sort_by: 'version',
      sort_order: 'desc',
    }
  )
})

test('uses stable catalog URL defaults without profit filter leakage', () => {
  assert.deepEqual(costCatalogParamsFromSearch({ channelId: 91 }), {
    status: 'active',
    page: 1,
    page_size: 50,
    sort_by: 'channel_name',
    sort_order: 'asc',
  })
})

test('resets catalog page when filters or sorting change', () => {
  const current = {
    tab: 'catalog' as const,
    catalogPage: 4,
    catalogStatus: 'active' as const,
  }
  assert.equal(
    updateCatalogSearch(current, { catalogCurrency: 'USD' }).catalogPage,
    1
  )
  assert.equal(
    updateCatalogSearch(current, { catalogSort: 'version' }).catalogPage,
    1
  )
  assert.equal(updateCatalogSearch(current, { catalogPage: 3 }).catalogPage, 3)
})

test('formats exact catalog prices and leaves unknown values blank', () => {
  assert.equal(
    formatCatalogPrice('3.5000', 'usd', 'per request'),
    'USD 3.5 · per request'
  )
  assert.equal(formatCatalogPrice('', 'USD', 'per request'), '')
  assert.equal(formatCatalogPrice('NaN', 'USD', 'per request'), '')
  assert.equal(formatCatalogPrice('Infinity', 'USD', 'per request'), '')
  assert.doesNotMatch(formatCatalogPrice('', 'USD', 'per request'), /\$0/)
})

test('formats optional Unix timestamps without inventing an epoch value', () => {
  assert.equal(formatCatalogTimestamp(undefined), '')
  assert.equal(formatCatalogTimestamp(0), '')
  assert.match(formatCatalogTimestamp(1_735_689_600), /^2025-01-01 /)
})
