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
