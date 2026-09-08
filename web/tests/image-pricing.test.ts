import { describe, expect, test } from 'bun:test'

import { groupImagePrices } from '../src/features/pricing/lib/image-pricing'

describe('groupImagePrices', () => {
  test('groups prices by resolution tier in display order', () => {
    expect(
      groupImagePrices([
        { tier: '2k', quality: 'high', price_usd: '0.12' },
        { tier: '1k', quality: 'medium', price_usd: '0.05' },
        { tier: '1k', quality: 'low', price_usd: '0.04' },
      ])
    ).toEqual([
      {
        tier: '1k',
        prices: [
          { quality: 'low', priceUSD: 0.04 },
          { quality: 'medium', priceUSD: 0.05 },
        ],
      },
      {
        tier: '2k',
        prices: [{ quality: 'high', priceUSD: 0.12 }],
      },
    ])
  })
})
