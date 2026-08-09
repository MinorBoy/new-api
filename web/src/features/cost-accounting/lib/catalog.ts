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

import type { CostCatalogParams } from '../types'
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
