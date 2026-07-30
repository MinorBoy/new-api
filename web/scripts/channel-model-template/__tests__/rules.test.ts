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

import { parseRules } from '../rules'

const validRules = {
  version: '1',
  channelCodes: { '1': 'CH-CLMM' },
  defaults: {
    currency: 'CNY',
    currencyToUsd: '0.136986301369863',
    rechargeRatio: '1',
    purchaseDiscountRatio: '1',
    tokenDivisor: 1024,
    groupRatio: '1',
  },
  modelRules: {},
  overrides: {},
}

test('rejects a channel mapping without a stable channel code', () => {
  assert.throws(
    () => parseRules({ ...validRules, channelCodes: { '1': 'clmm' } }),
    /channelCodes.1 must match CH-/
  )
})

test('keeps decimal defaults as canonical strings', () => {
  const rules = parseRules(validRules)

  assert.equal(rules.defaults.currencyToUsd, '0.136986301369863')
  assert.equal(rules.defaults.purchaseDiscountRatio, '1')
})

test('rejects credential-like rule fields', () => {
  assert.throws(
    () => parseRules({ ...validRules, apiKey: 'do-not-store-me' }),
    /credential-like field: apiKey/
  )
})
