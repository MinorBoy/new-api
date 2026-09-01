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

import dayjs from '@/lib/dayjs'

import type { CostCatalogItem, CostCatalogParams } from '../types'
import type { CostAccountingSearch } from './report'

const catalogPageResetFields = new Set<keyof CostAccountingSearch>([
  'catalogChannelId',
  'catalogModel',
  'catalogCostMode',
  'catalogStatus',
  'catalogCurrency',
  'catalogSource',
  'catalogPageSize',
  'catalogSort',
  'catalogOrder',
])

function optionalCatalogText(value: string | undefined): string | undefined {
  const trimmed = value?.trim() ?? ''
  return trimmed || undefined
}

export function costCatalogParamsFromSearch(
  search: CostAccountingSearch
): CostCatalogParams {
  const currency = optionalCatalogText(search.catalogCurrency)?.toUpperCase()
  const params: CostCatalogParams = {
    status: search.catalogStatus ?? 'active',
    page: search.catalogPage ?? 1,
    page_size: search.catalogPageSize ?? 50,
    sort_by: search.catalogSort ?? 'channel_name',
    sort_order: search.catalogOrder ?? 'asc',
  }
  if (search.catalogChannelId !== undefined) {
    params.channel_id = search.catalogChannelId
  }
  const model = optionalCatalogText(search.catalogModel)
  if (model !== undefined) {
    params.billable_upstream_model = model
  }
  if (search.catalogCostMode !== undefined) {
    params.cost_mode = search.catalogCostMode
  }
  if (currency !== undefined) {
    params.currency = currency
  }
  const source = optionalCatalogText(search.catalogSource)
  if (source !== undefined) {
    params.source = source
  }
  return params
}

export function updateCatalogSearch(
  search: CostAccountingSearch,
  patch: Partial<CostAccountingSearch>
): CostAccountingSearch {
  const resetsPage = Object.keys(patch).some((key) =>
    catalogPageResetFields.has(key as keyof CostAccountingSearch)
  )
  return {
    ...search,
    ...patch,
    catalogPage: resetsPage ? 1 : (patch.catalogPage ?? search.catalogPage),
  }
}

export function formatCatalogPrice(
  amount: string,
  currency: string,
  unitLabel: string
): string {
  const trimmed = amount.trim()
  if (!trimmed) {
    return ''
  }
  let parsed: Decimal
  try {
    parsed = new Decimal(trimmed)
  } catch {
    return ''
  }
  if (!parsed.isFinite() || parsed.isNegative()) {
    return ''
  }
  const currencyCode = currency.trim().toUpperCase()
  const prefix = currencyCode ? `${currencyCode} ` : ''
  const unit = unitLabel.trim()
  return `${prefix}${parsed.toString()}${unit ? ` · ${unit}` : ''}`
}

export function formatCatalogTimestamp(value?: number): string {
  if (!value || !Number.isSafeInteger(value) || value < 0) {
    return ''
  }
  return dayjs.unix(value).format('YYYY-MM-DD HH:mm:ss')
}

type CatalogTranslate = (key: string) => string

export function catalogPriceUnitLabel(
  unit: string,
  translate: CatalogTranslate
): string {
  const keys: Record<string, string> = {
    per_request: 'Per request',
    per_image: 'Per image',
    per_second: 'Per second',
    per_million_tokens: 'Per 1M tokens',
    per_million_completion_tokens: 'Per 1M completion tokens',
    per_million_input_tokens: 'Per 1M input tokens',
    per_million_output_tokens: 'Per 1M output tokens',
  }
  return translate(keys[unit] ?? unit)
}

export function formatCatalogItemPrices(
  item: CostCatalogItem,
  normalizedUSD: boolean,
  translate: CatalogTranslate
): string {
  if (item.price_status !== 'available') {
    return translate('Unavailable')
  }
  return item.prices
    .map((price) =>
      formatCatalogPrice(
        normalizedUSD ? price.normalized_usd_amount : price.native_amount,
        normalizedUSD ? 'USD' : item.currency,
        catalogPriceUnitLabel(price.unit, translate)
      )
    )
    .filter(Boolean)
    .join('; ')
}

export function catalogCostModeLabel(
  mode: CostCatalogItem['cost_mode'],
  translate: CatalogTranslate
): string {
  const keys = {
    free: 'Free',
    per_request: 'Per request',
    per_image: 'Per image',
    per_duration: 'Per duration',
    per_token: 'Per token',
  } as const
  return translate(keys[mode])
}

export function catalogStatusLabel(
  status: CostCatalogItem['status'],
  translate: CatalogTranslate
): string {
  const keys = { active: 'Active', draft: 'Draft', retired: 'Retired' } as const
  return translate(keys[status])
}

export function catalogBillingSemantics(
  item: CostCatalogItem,
  translate: CatalogTranslate
): string {
  const parts: string[] = []
  if (item.charge_event) parts.push(item.charge_event)
  if (item.meter_source) parts.push(item.meter_source)
  if (item.token_mode) parts.push(item.token_mode)
  return parts.length > 0
    ? parts.map((value) => translate(value)).join(' · ')
    : '—'
}
