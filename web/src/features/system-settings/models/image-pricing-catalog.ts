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
import Decimal from 'decimal.js'

export type ImageCatalogSKU = {
  endpoint: string
  size: string
  quality: string
  unit: string
  sale_price_usd: string
}

export type ImageCatalogEndpoint = {
  capability?: {
    enabled?: boolean
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
  size: string
  quality: string
  skuKey: string
  salePriceUSD: string
  upstreamCostUSD?: string
  sourceCount: number
  expectedMarginBps: number | null
  enabled: boolean
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
    for (const skuKey of Object.keys(skus).sort()) {
      const sku = skus[skuKey]
      const endpoint = endpoints[sku.endpoint]
      const capability = endpoint?.capability
      rows.push({
        id: `${modelName}|${skuKey}`,
        model: modelName,
        endpoint: sku.endpoint,
        size: sku.size,
        quality: sku.quality,
        skuKey,
        salePriceUSD: sku.sale_price_usd,
        sourceCount: 0,
        expectedMarginBps: null,
        enabled: Boolean(endpoint && capability?.enabled !== false),
      })
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
