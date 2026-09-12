import Decimal from 'decimal.js'

export type ImageCatalogSKU = {
  endpoint: string
  tier?: string
  size?: string
  quality: string
  unit: string
  sale_price_usd: string
}

export type ImageCatalogEndpoint = {
  capability?: {
    enabled?: boolean
    resolution_tiers?: readonly string[]
    sizes?: readonly string[]
    qualities?: readonly string[]
    response_formats?: readonly string[]
    max_n?: number
  }
  default_size?: string
  default_quality?: string
  default_response_format?: string
}

export type ImageCatalogModel = {
  profile?: string
  profile_version?: number
  endpoints?: Record<string, ImageCatalogEndpoint>
  skus?: Record<string, ImageCatalogSKU>
}

export type ImageCatalog = {
  version: number
  models: Record<string, ImageCatalogModel>
}

export type ImagePricingRow = {
  id: string
  model: string
  endpoint: string
  tier: string
  size?: string
  quality: string
  skuKey: string
  salePriceUSD: string
  upstreamCostUSD?: string
  sourceCount: number
  expectedMarginBps: number | null
  enabled: boolean
  configured: boolean
  legacySkuKey?: string
}

export const IMAGE_RESOLUTION_TIERS = ['1k', '2k', '4k'] as const
export const IMAGE_QUALITY_TIERS = ['low', 'medium', 'high'] as const

export type ImageResolutionTier = (typeof IMAGE_RESOLUTION_TIERS)[number]

export function resolveImageResolutionTier(size: string | undefined): string {
  const normalized = String(size || '')
    .trim()
    .toLowerCase()
  if (!normalized || normalized === 'auto') return '1k'
  const match = /^(\d+)x(\d+)$/.exec(normalized)
  if (!match) return ''
  const pixels = Number(match[1]) * Number(match[2])
  // This helper is used only to read historical catalog entries. Old
  // catalogs may contain 4096x4096 SKUs even though new requests are capped
  // at the 2880x2880 4K pixel budget; keep those prices visible under 4K.
  if (!Number.isSafeInteger(pixels) || pixels <= 0) {
    return ''
  }
  if (pixels <= 1048576) return '1k'
  if (pixels <= 4194304) return '2k'
  return '4k'
}

function tierFromSKUKey(skuKey: string): string {
  const parts = skuKey.trim().split('-')
  return IMAGE_RESOLUTION_TIERS.includes(parts[1] as ImageResolutionTier)
    ? parts[1]
    : ''
}

function isUnifiedImageModel(model: ImageCatalogModel): boolean {
  return model.profile === 'openai_images' && model.profile_version === 1
}

function findLegacyTierSKU(
  entries: Array<[string, ImageCatalogSKU]>,
  endpoint: string,
  tier: string,
  quality: string
): [string, ImageCatalogSKU] | undefined {
  return entries
    .filter(([, sku]) => sku.endpoint === endpoint && sku.quality === quality)
    .map(([skuKey, sku]) => [skuKey, sku] as [string, ImageCatalogSKU])
    .sort(([left], [right]) => left.localeCompare(right))
    .find(([, sku]) => resolveImageResolutionTier(sku.size) === tier)
}

function parseCatalog(raw: string | ImageCatalog): ImageCatalog {
  let value: unknown = raw
  if (typeof raw === 'string') {
    try {
      value = JSON.parse(raw)
    } catch {
      throw new Error('Image catalog must be valid JSON')
    }
  }
  if (!value || typeof value !== 'object' || Array.isArray(value)) {
    throw new Error('Image catalog must be a JSON object')
  }
  const catalog = value as Partial<ImageCatalog>
  if (!catalog.models || typeof catalog.models !== 'object') {
    throw new Error('Image catalog models must be an object')
  }
  return catalog as ImageCatalog
}

export function flattenImagePricingCatalog(
  raw: string | ImageCatalog
): ImagePricingRow[] {
  const catalog = parseCatalog(raw)
  const rows: ImagePricingRow[] = []
  for (const modelName of Object.keys(catalog.models).sort()) {
    const model = catalog.models[modelName]
    const endpoints = model.endpoints ?? {}
    const skus = model.skus ?? {}
    const skuEntries = Object.entries(skus)
    const tieredEndpoints = new Set(
      Object.entries(skus)
        .filter(([, sku]) => sku.endpoint in endpoints)
        .filter(([skuKey, sku]) =>
          IMAGE_RESOLUTION_TIERS.includes(
            (sku.tier || tierFromSKUKey(skuKey)) as ImageResolutionTier
          )
        )
        .map(([, sku]) => sku.endpoint)
    )
    if (tieredEndpoints.size > 0 || isUnifiedImageModel(model)) {
      for (const endpointName of Object.keys(endpoints).sort()) {
        const endpoint = endpoints[endpointName]
        const capability = endpoint.capability
        if (capability?.enabled === false) continue
        if (!tieredEndpoints.has(endpointName) && !isUnifiedImageModel(model)) {
          for (const [skuKey, sku] of Object.entries(skus)) {
            if (sku.endpoint !== endpointName) continue
            rows.push({
              id: `${modelName}|${skuKey}`,
              model: modelName,
              endpoint: sku.endpoint,
              tier:
                sku.tier ||
                tierFromSKUKey(skuKey) ||
                resolveImageResolutionTier(sku.size),
              size: sku.size,
              quality: sku.quality,
              skuKey,
              salePriceUSD: sku.sale_price_usd,
              sourceCount: 0,
              expectedMarginBps: null,
              enabled: true,
              configured: true,
            })
          }
          continue
        }
        // Tiered catalogs use the fixed 3 x 3 billing matrix. Provider-specific
        // concrete sizes and quality lists are not a pricing catalog boundary.
        const tiers = IMAGE_RESOLUTION_TIERS
        const qualities = IMAGE_QUALITY_TIERS
        for (const tier of tiers) {
          for (const quality of qualities) {
            const skuKey = `${endpointName === 'edits' ? 'edit' : 'gen'}-${tier}-${quality}`
            const sku = skus[skuKey]
            const legacy = sku
              ? undefined
              : findLegacyTierSKU(skuEntries, endpointName, tier, quality)
            const effectiveSKU = sku ?? legacy?.[1]
            rows.push({
              id: `${modelName}|${skuKey}`,
              model: modelName,
              endpoint: endpointName,
              tier,
              size: effectiveSKU?.size,
              quality,
              skuKey,
              salePriceUSD: effectiveSKU?.sale_price_usd ?? '',
              sourceCount: 0,
              expectedMarginBps: null,
              enabled: Boolean(effectiveSKU),
              configured: Boolean(effectiveSKU),
              legacySkuKey: legacy?.[0],
            })
          }
        }
      }
    } else {
      for (const skuKey of Object.keys(skus).sort()) {
        const sku = skus[skuKey]
        const endpoint = endpoints[sku.endpoint]
        const capability = endpoint?.capability
        rows.push({
          id: `${modelName}|${skuKey}`,
          model: modelName,
          endpoint: sku.endpoint,
          tier:
            sku.tier ||
            tierFromSKUKey(skuKey) ||
            resolveImageResolutionTier(sku.size),
          size: sku.size,
          quality: sku.quality,
          skuKey,
          salePriceUSD: sku.sale_price_usd,
          sourceCount: 0,
          expectedMarginBps: null,
          enabled: Boolean(endpoint && capability?.enabled !== false),
          configured: true,
        })
      }
    }
  }
  return rows.sort((left, right) => {
    if (left.model !== right.model) return left.model.localeCompare(right.model)
    if (left.endpoint !== right.endpoint) {
      return left.endpoint.localeCompare(right.endpoint)
    }
    return left.skuKey.localeCompare(right.skuKey)
  })
}

export function updateCatalogSalePrices(
  raw: string | ImageCatalog,
  prices: Record<string, string>
): ImageCatalog {
  const catalog = parseCatalog(raw)
  const updated: ImageCatalog = {
    ...catalog,
    models: Object.fromEntries(
      Object.entries(catalog.models).map(([modelName, model]) => [
        modelName,
        {
          ...model,
          endpoints: model.endpoints
            ? Object.fromEntries(Object.entries(model.endpoints))
            : model.endpoints,
          skus: Object.fromEntries(
            Object.entries(model.skus ?? {}).map(([skuKey, sku]) => {
              const nextPrice = prices[`${modelName}|${skuKey}`]
              return [
                skuKey,
                nextPrice === undefined
                  ? { ...sku }
                  : { ...sku, sale_price_usd: nextPrice },
              ]
            })
          ),
        },
      ])
    ),
  }
  for (const [id, salePrice] of Object.entries(prices)) {
    const separator = id.indexOf('|')
    if (separator < 0 || salePrice === undefined) continue
    const modelName = id.slice(0, separator)
    const skuKey = id.slice(separator + 1)
    const model = updated.models[modelName]
    if (!model || model.skus?.[skuKey]) {
      continue
    }
    const parts = skuKey.split('-')
    if (
      parts.length !== 3 ||
      !IMAGE_RESOLUTION_TIERS.includes(parts[1] as ImageResolutionTier)
    ) {
      continue
    }
    const endpoint = parts[0] === 'edit' ? 'edits' : 'generations'
    const legacy = findLegacyTierSKU(
      Object.entries(model.skus ?? {}),
      endpoint,
      parts[1],
      parts[2]
    )
    if (legacy) {
      model.skus = {
        ...model.skus,
        [legacy[0]]: { ...legacy[1], sale_price_usd: salePrice },
      }
      continue
    }
    model.skus = {
      ...model.skus,
      [skuKey]: {
        endpoint,
        tier: parts[1],
        quality: parts[2],
        unit: 'image',
        sale_price_usd: salePrice,
      },
    }
    const endpointCatalog = model.endpoints?.[endpoint]
    if (endpointCatalog?.capability) {
      const tiers = new Set(endpointCatalog.capability.resolution_tiers ?? [])
      tiers.add(parts[1])
      endpointCatalog.capability.resolution_tiers = [...tiers]
      const qualities = new Set(endpointCatalog.capability.qualities ?? [])
      qualities.add(parts[2])
      endpointCatalog.capability.qualities = [...qualities]
    }
  }
  return updated
}

export function validateSalePrice(value: string): string | null {
  const trimmed = value.trim()
  if (trimmed === '') return 'Price is required'
  if (!/^(?:0|[0-9]+)(?:\.[0-9]+)?$/.test(trimmed)) {
    return 'Price must be non-negative'
  }
  const decimalPlaces = trimmed.includes('.')
    ? trimmed.length - trimmed.indexOf('.') - 1
    : 0
  if (decimalPlaces > 8) return 'Price has too many decimal places'
  return null
}

export function calculateExpectedMarginBps(
  salePriceUSD: string,
  costUSD?: string
): number | null {
  if (
    !costUSD ||
    validateSalePrice(salePriceUSD) ||
    validateSalePrice(costUSD)
  ) {
    return null
  }
  try {
    const sale = new Decimal(salePriceUSD)
    const cost = new Decimal(costUSD)
    if (!sale.isFinite() || !cost.isFinite() || sale.isZero()) return null
    const margin = sale.minus(cost).div(sale).mul(10000).trunc()
    const value = margin.toNumber()
    return Number.isFinite(value) ? value : null
  } catch {
    return null
  }
}

export function formatImagePrice(value: string): string {
  try {
    return new Decimal(value).toFixed(8).replace(/0+$/, '').replace(/\.$/, '')
  } catch {
    return value
  }
}
