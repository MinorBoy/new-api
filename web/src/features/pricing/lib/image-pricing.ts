export type ImagePriceInput = {
  tier: string
  quality: string
  price_usd: string
}

export type ImagePriceGroup = {
  tier: string
  prices: Array<{ quality: string; priceUSD: number }>
}

export function formatImagePriceGroups(
  prices: ImagePriceInput[] | undefined,
  format: (priceUSD: number) => string
): ImagePriceGroup[] {
  return groupImagePrices(prices).map((group) => ({
    ...group,
    prices: group.prices.map((price) => ({
      ...price,
      priceUSD: Number(format(price.priceUSD)),
    })),
  }))
}

const tierOrder = ['1k', '2k', '4k']
const qualityOrder = ['low', 'medium', 'high']

export function groupImagePrices(
  prices: ImagePriceInput[] | undefined
): ImagePriceGroup[] {
  const grouped = new Map<string, ImagePriceGroup>()
  for (const item of prices ?? []) {
    const priceUSD = Number(item.price_usd)
    if (
      !tierOrder.includes(item.tier) ||
      !qualityOrder.includes(item.quality) ||
      !Number.isFinite(priceUSD)
    ) {
      continue
    }
    const group = grouped.get(item.tier) ?? { tier: item.tier, prices: [] }
    group.prices.push({ quality: item.quality, priceUSD })
    grouped.set(item.tier, group)
  }
  return [...grouped.values()]
    .sort((a, b) => tierOrder.indexOf(a.tier) - tierOrder.indexOf(b.tier))
    .map((group) => ({
      ...group,
      prices: group.prices.sort(
        (a, b) =>
          qualityOrder.indexOf(a.quality) - qualityOrder.indexOf(b.quality)
      ),
    }))
}
