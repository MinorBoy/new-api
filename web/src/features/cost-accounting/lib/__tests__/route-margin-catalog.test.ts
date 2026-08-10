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
  formatRouteMarginPercent,
  formatRouteMarginUSD,
  routeMarginParamsFromSearch,
  updateRouteMarginSearch,
} from '../route-margin-catalog'

test('maps route margin URL state to API parameters', () => {
  assert.deepEqual(
    routeMarginParamsFromSearch({
      tab: 'route-margin',
      marginMinimumPercent: 30,
      marginDurationSeconds: 4,
      marginGroupRatio: 1.25,
      marginScenario: 'with_video',
      marginResolution: '720p',
      marginStatus: 'eligible',
      marginPage: 2,
    }),
    {
      min_margin_ppm: 300000,
      duration_seconds: 4,
      group_ratio: 1.25,
      scenario: 'with_video',
      resolution: '720p',
      status: 'eligible',
      page: 2,
      page_size: 50,
      sort_by: 'gross_margin_ppm',
      sort_order: 'desc',
    }
  )
})

test('formats route margin amounts and PPM exactly', () => {
  assert.equal(formatRouteMarginUSD(1_234_500_000), 'USD 1.2345')
  assert.equal(formatRouteMarginPercent(299_999), '29.9999%')
  assert.equal(formatRouteMarginUSD(undefined), '')
  assert.equal(formatRouteMarginPercent(undefined), '')
})

test('preserves four-decimal percentage precision in PPM', () => {
  assert.equal(
    routeMarginParamsFromSearch({ marginMinimumPercent: 29.9999 })
      .min_margin_ppm,
    299999
  )
})

test('resets the route margin page when a filter changes', () => {
  const current = {
    tab: 'route-margin' as const,
    marginPage: 4,
    marginStatus: 'all' as const,
  }
  assert.equal(
    updateRouteMarginSearch(current, { marginStatus: 'eligible' }).marginPage,
    1
  )
  assert.equal(
    updateRouteMarginSearch(current, { marginPage: 3 }).marginPage,
    3
  )
})
