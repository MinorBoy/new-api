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
import { describe, test } from 'node:test'

import type { RatioType, RatioValue } from '../../types'
import { applyResolutionSelection } from '../upstream-ratio-sync-helpers'

const seedanceTokenPrice = {
  scenarios: {
    '480p:with_video': {
      price_per_million: '1.917808219178082',
      width: 864,
      height: 496,
      frame_rate: 24,
      pricing_version: 'official-token-v1',
      source: 'official-sheet',
    },
  },
}

describe('upstream ratio sync Seedance contract', () => {
  test('selecting Seedance billing mode carries its token price as one contract', () => {
    const seedanceField = 'seedance_token_price' as RatioType
    const differences = {
      'seedance-model': {
        billing_mode: {
          current: null,
          upstreams: { upstream: 'seedance_tokens' },
          confidence: { upstream: true },
        },
        [seedanceField]: {
          current: null,
          upstreams: { upstream: seedanceTokenPrice as RatioValue },
          confidence: { upstream: true },
        },
      },
    }

    const result = applyResolutionSelection({}, differences, {
      model: 'seedance-model',
      ratioType: 'billing_mode',
      value: 'seedance_tokens',
      sourceName: 'upstream',
    })

    assert.deepEqual(result['seedance-model'], {
      billing_mode: 'seedance_tokens',
      seedance_token_price: seedanceTokenPrice,
    })
  })

  test('selecting a fixed price removes the Seedance token contract', () => {
    const seedanceField = 'seedance_token_price' as RatioType
    const result = applyResolutionSelection(
      {
        'seedance-model': {
          billing_mode: 'seedance_tokens',
          seedance_token_price: seedanceTokenPrice as RatioValue,
        },
      },
      {},
      {
        model: 'seedance-model',
        ratioType: 'model_price',
        value: 0.5,
        sourceName: 'upstream',
      }
    )

    assert.deepEqual(result['seedance-model'], {
      billing_mode: 'ratio',
      model_price: 0.5,
    })
    assert.equal(seedanceField, 'seedance_token_price')
  })
})
