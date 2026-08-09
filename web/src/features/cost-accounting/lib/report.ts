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
import type {
  CostCatalogSort,
  CostCoverageItem,
  CostMode,
  CostReportParams,
  CostRuleStatus,
} from '../types'

export type CostAccountingSearch = {
  tab?: 'profit' | 'catalog' | 'anomalies'
  timeBasis?: 'profit_recognized_at' | 'requested_at'
  startTime?: number
  endTime?: number
  channelId?: number
  billableModel?: string
  originModel?: string
  userGroup?: string
  usingGroup?: string
  billingSource?: string
  status?: string
  catalogChannelId?: number
  catalogModel?: string
  catalogCostMode?: CostMode
  catalogStatus?: CostRuleStatus | 'all'
  catalogCurrency?: string
  catalogSource?: string
  catalogPage?: number
  catalogPageSize?: 25 | 50 | 100
  catalogSort?: CostCatalogSort
  catalogOrder?: 'asc' | 'desc'
}

export const COST_ACCOUNTING_TABS = ['profit', 'catalog', 'anomalies'] as const

export type CostAccountingTab = (typeof COST_ACCOUNTING_TABS)[number]

export function isCostCatalogTab(tab: CostAccountingTab): boolean {
  return tab === 'catalog'
}

export function updateCostAccountingTab(
  search: CostAccountingSearch,
  tab: CostAccountingTab
): CostAccountingSearch {
  return { ...search, tab }
}

function optionalText(value: string | undefined): string | undefined {
  const trimmed = value?.trim() ?? ''
  return trimmed || undefined
}

export function costReportParamsFromSearch(
  search: CostAccountingSearch
): CostReportParams {
  return {
    time_basis: search.timeBasis ?? 'profit_recognized_at',
    start_time: search.startTime,
    end_time: search.endTime,
    channel_id: search.channelId,
    billable_upstream_model: optionalText(search.billableModel),
    origin_model: optionalText(search.originModel),
    user_group: optionalText(search.userGroup),
    using_group: optionalText(search.usingGroup),
    billing_source: optionalText(search.billingSource),
    status: optionalText(search.status),
  }
}

export function canEnableStrictCostAccounting(
  coverage: CostCoverageItem[]
): boolean {
  return coverage.every((item) => item.covered)
}
