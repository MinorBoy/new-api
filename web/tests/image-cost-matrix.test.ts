import { describe, expect, test } from 'bun:test'

import { buildImageCostMatrix, imageCostVariantKey } from '../src/features/cost-accounting/lib/image-cost-matrix'

describe('image cost matrix', () => {
  test('builds all nine stable image variants and preserves draft price', () => {
    const cells = buildImageCostMatrix('generations', [
      {
        id: 1, channel_id: 7, billable_upstream_model: 'vendor', cost_variant_key: 'gen-2k-high', version: 1,
        status: 'active', cost_mode: 'per_image', schema_version: 1,
        config: { unit_price: '0.02', normalized_usd_prices: { unit_price: '0.02' } },
        source: 'manual', note: '', created_by: 1, activated_by: 1, created_at: 1, updated_at: 1,
      },
      {
        id: 2, channel_id: 7, billable_upstream_model: 'vendor', cost_variant_key: 'gen-2k-high', version: 2,
        status: 'draft', cost_mode: 'per_image', schema_version: 1,
        config: { unit_price: '0.018', normalized_usd_prices: { unit_price: '0.018' } },
        source: 'manual', note: '', created_by: 1, activated_by: 0, created_at: 1, updated_at: 1,
      },
    ])
    expect(cells).toHaveLength(9)
    expect(cells.find((cell) => cell.key === 'gen-2k-high')?.unitPrice).toBe('0.018')
    expect(imageCostVariantKey('edits', '4k', 'low')).toBe('edit-4k-low')
  })
})
