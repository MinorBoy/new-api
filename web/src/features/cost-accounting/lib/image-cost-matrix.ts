import type { CostRule } from '../types'

export const IMAGE_COST_TIERS = ['1k', '2k', '4k'] as const
export const IMAGE_COST_QUALITIES = ['low', 'medium', 'high'] as const

export type ImageCostTier = (typeof IMAGE_COST_TIERS)[number]
export type ImageCostQuality = (typeof IMAGE_COST_QUALITIES)[number]

export type ImageCostMatrixCell = {
  key: string
  tier: ImageCostTier
  quality: ImageCostQuality
  unitPrice: string
  rule: CostRule | null
}

export function imageCostVariantKey(
  endpoint: 'generations' | 'edits',
  tier: ImageCostTier,
  quality: ImageCostQuality
): string {
  return `${endpoint === 'edits' ? 'edit' : 'gen'}-${tier}-${quality}`
}

export function buildImageCostMatrix(
  endpoint: 'generations' | 'edits',
  rules: CostRule[]
): ImageCostMatrixCell[] {
  const byKey = new Map<string, CostRule>()
  for (const rule of rules) {
    if (rule.cost_mode !== 'per_image') continue
    const current = byKey.get(rule.cost_variant_key)
    if (!current || (rule.status === 'draft' && current.status !== 'draft') || rule.version > current.version) {
      byKey.set(rule.cost_variant_key, rule)
    }
  }
  return IMAGE_COST_TIERS.flatMap((tier) =>
    IMAGE_COST_QUALITIES.map((quality) => {
      const key = imageCostVariantKey(endpoint, tier, quality)
      const rule = byKey.get(key) ?? null
      return {
        key,
        tier,
        quality,
        unitPrice:
          rule?.config.unit_price ?? rule?.config.normalized_usd_prices.unit_price ?? '',
        rule,
      }
    })
  )
}
