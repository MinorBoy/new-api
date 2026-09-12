import type {
  CostCatalogSort,
  CostCoverageItem,
  CostMode,
  CostReportParams,
  CostRuleStatus,
  RouteMarginCatalogSort,
  RouteMarginScenario,
  RouteMarginStatus,
} from '../types'

export type CostAccountingSearch = {
  tab?: 'profit' | 'catalog' | 'route-margin' | 'anomalies'
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
  marginMinimumPercent?: number
  marginDurationSeconds?: number
  marginGroupRatio?: number
  marginScenario?: RouteMarginScenario
  marginChannelId?: number
  marginModel?: string
  marginUpstreamModel?: string
  marginRouteTarget?: string
  marginResolution?: string
  marginStatus?: RouteMarginStatus
  marginPage?: number
  marginPageSize?: 25 | 50 | 100
  marginSort?: RouteMarginCatalogSort
  marginOrder?: 'asc' | 'desc'
}

export const COST_ACCOUNTING_TABS = [
  'profit',
  'catalog',
  'route-margin',
  'anomalies',
] as const

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

export function trimCostReportDimension(value: string): string {
  return value.replace(/^ +/, '').replace(/ +$/, '')
}

function optionalCostReportDimension(
  value: string | undefined
): string | undefined {
  const trimmed = trimCostReportDimension(value ?? '')
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
    billable_upstream_model: optionalCostReportDimension(search.billableModel),
    origin_model: optionalCostReportDimension(search.originModel),
    user_group: optionalCostReportDimension(search.userGroup),
    using_group: optionalCostReportDimension(search.usingGroup),
    billing_source: optionalText(search.billingSource),
    status: optionalText(search.status),
  }
}

export function canEnableStrictCostAccounting(
  coverage: CostCoverageItem[]
): boolean {
  return coverage.every((item) => item.covered)
}
