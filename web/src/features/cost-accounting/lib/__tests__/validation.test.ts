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

import type { CostRuleFormValues } from '../../types'
import {
  addNanoUSD,
  formatMarginPPM,
  formatNanoUSD,
  parseCostRuleForm,
} from '../cost-rule'

const paidDefaults = {
  currency: 'USD',
  billing_multiplier: '1',
  purchase_discount_ratio: '1',
  recharge_exchange_ratio: '1',
  fee_rate: '0',
  currency_to_usd_rate: '1',
  charge_event: 'response_succeeded' as const,
}

test('formats and adds nano USD without Number conversion', () => {
  assert.equal(formatNanoUSD('9223372036854775807'), '$9,223,372,036.854775807')
  assert.equal(formatNanoUSD('-250000000'), '-$0.25')
  assert.equal(formatNanoUSD('0'), '$0')
  assert.equal(addNanoUSD('9223372036854775807', '193'), '9223372036854776000')
})

test('formats nullable parts-per-million margins exactly', () => {
  assert.equal(formatMarginPPM(null), 'N/A')
  assert.equal(formatMarginPPM('1000000'), '100%')
  assert.equal(formatMarginPPM('-250000'), '-25%')
  assert.equal(formatMarginPPM('333333'), '33.3333%')
})

test('requires an explicit reason and no pricing fields for free rules', () => {
  assert.deepEqual(
    parseCostRuleForm({
      cost_mode: 'free',
      zero_cost_reason: '  Included in the provider plan  ',
    }),
    {
      zero_cost_reason: 'Included in the provider plan',
      normalized_usd_prices: {},
    }
  )

  assert.throws(() =>
    parseCostRuleForm({
      cost_mode: 'free',
      zero_cost_reason: '   ',
    })
  )
  assert.throws(() =>
    parseCostRuleForm({
      cost_mode: 'free',
      zero_cost_reason: 'Provider promotion',
      unit_price: '0',
    } as unknown as CostRuleFormValues)
  )
})

test('parses only the fields allowed by a per-request rule', () => {
  assert.deepEqual(
    parseCostRuleForm({
      cost_mode: 'per_request',
      ...paidDefaults,
      unit_price: '0.25',
    }),
    {
      ...paidDefaults,
      unit_price: '0.25',
      normalized_usd_prices: {},
    }
  )

  assert.throws(() =>
    parseCostRuleForm({
      cost_mode: 'per_request',
      ...paidDefaults,
      unit_price: '0.25',
      meter_source: 'upstream_actual',
    } as unknown as CostRuleFormValues)
  )
})

test('rejects non-canonical or non-positive paid Decimal fields', () => {
  for (const unitPrice of ['0', '-1', '01', '1.0', '.5', '+1', '1e3', ' 1 ']) {
    assert.throws(
      () =>
        parseCostRuleForm({
          cost_mode: 'per_request',
          ...paidDefaults,
          unit_price: unitPrice,
        }),
      Error,
      unitPrice
    )
  }

  assert.throws(() =>
    parseCostRuleForm({
      cost_mode: 'per_request',
      ...paidDefaults,
      billing_multiplier: '0',
      unit_price: '1',
    })
  )
  assert.throws(() =>
    parseCostRuleForm({
      cost_mode: 'per_request',
      ...paidDefaults,
      fee_rate: '-0.1',
      unit_price: '1',
    })
  )
})

test('requires the duration price and a duration meter source', () => {
  assert.deepEqual(
    parseCostRuleForm({
      cost_mode: 'per_duration',
      ...paidDefaults,
      meter_source: 'validated_request',
      price_per_second: '0.0025',
    }),
    {
      ...paidDefaults,
      meter_source: 'validated_request',
      price_per_second: '0.0025',
      normalized_usd_prices: {},
    }
  )

  assert.throws(() =>
    parseCostRuleForm({
      cost_mode: 'per_duration',
      ...paidDefaults,
      meter_source: 'upstream_usage',
      price_per_second: '1',
    } as unknown as CostRuleFormValues)
  )
  assert.throws(() =>
    parseCostRuleForm({
      cost_mode: 'per_duration',
      ...paidDefaults,
      charge_event: 'submit_accepted',
      meter_source: 'upstream_actual',
      price_per_second: '1',
    })
  )
})

test('enforces token-mode-specific prices and rejects extra fields', () => {
  assert.deepEqual(
    parseCostRuleForm({
      cost_mode: 'per_token',
      ...paidDefaults,
      meter_source: 'upstream_usage',
      token_mode: 'total_tokens',
      total_per_million: '2.5',
    }),
    {
      ...paidDefaults,
      meter_source: 'upstream_usage',
      token_mode: 'total_tokens',
      total_per_million: '2.5',
      normalized_usd_prices: {},
    }
  )

  assert.deepEqual(
    parseCostRuleForm({
      cost_mode: 'per_token',
      ...paidDefaults,
      meter_source: 'local_usage',
      token_mode: 'input_output',
      input_per_million: '1.25',
      output_per_million: '5',
    }),
    {
      ...paidDefaults,
      meter_source: 'local_usage',
      token_mode: 'input_output',
      input_per_million: '1.25',
      output_per_million: '5',
      normalized_usd_prices: {},
    }
  )

  assert.throws(() =>
    parseCostRuleForm({
      cost_mode: 'per_token',
      ...paidDefaults,
      meter_source: 'local_usage',
      token_mode: 'input_output',
      input_per_million: '1.25',
    } as unknown as CostRuleFormValues)
  )
  assert.throws(() =>
    parseCostRuleForm({
      cost_mode: 'per_token',
      ...paidDefaults,
      meter_source: 'upstream_usage',
      token_mode: 'completion_tokens',
      completion_per_million: '5',
      total_per_million: '2.5',
    } as unknown as CostRuleFormValues)
  )
  assert.throws(() =>
    parseCostRuleForm({
      cost_mode: 'per_token',
      ...paidDefaults,
      charge_event: 'submit_accepted',
      meter_source: 'upstream_usage',
      token_mode: 'total_tokens',
      total_per_million: '2.5',
    } as unknown as CostRuleFormValues)
  )
})
