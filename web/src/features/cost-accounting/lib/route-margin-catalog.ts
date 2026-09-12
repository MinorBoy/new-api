import Decimal from 'decimal.js'

import type { RouteMarginCatalogParams } from '../types'
import type { CostAccountingSearch } from './report'

const routeMarginPageResetFields = new Set<keyof CostAccountingSearch>([
  'marginMinimumPercent',
  'marginDurationSeconds',
  'marginGroupRatio',
  'marginScenario',
  'marginChannelId',
  'marginModel',
  'marginUpstreamModel',
  'marginRouteTarget',
  'marginResolution',
  'marginStatus',
  'marginPageSize',
  'marginSort',
  'marginOrder',
])

function optionalRouteMarginText(
  value: string | undefined
): string | undefined {
  const trimmed = value?.trim() ?? ''
  return trimmed || undefined
}

export function routeMarginParamsFromSearch(
  search: CostAccountingSearch
): RouteMarginCatalogParams {
  const params: RouteMarginCatalogParams = {
    min_margin_ppm: new Decimal(search.marginMinimumPercent ?? 30)
      .mul(10_000)
      .toDecimalPlaces(0, Decimal.ROUND_HALF_UP)
      .toNumber(),
    duration_seconds: search.marginDurationSeconds ?? 4,
    group_ratio: search.marginGroupRatio ?? 1,
    scenario: search.marginScenario ?? 'all',
    status: search.marginStatus ?? 'all',
    page: search.marginPage ?? 1,
    page_size: search.marginPageSize ?? 50,
    sort_by: search.marginSort ?? 'gross_margin_ppm',
    sort_order: search.marginOrder ?? 'desc',
  }
  if (search.marginChannelId !== undefined) {
    params.channel_id = search.marginChannelId
  }
  const model = optionalRouteMarginText(search.marginModel)
  if (model !== undefined) params.model = model
  const upstreamModel = optionalRouteMarginText(search.marginUpstreamModel)
  if (upstreamModel !== undefined) params.upstream_model = upstreamModel
  const routeTarget = optionalRouteMarginText(search.marginRouteTarget)
  if (routeTarget !== undefined) params.route_target = routeTarget
  const resolution = optionalRouteMarginText(search.marginResolution)
  if (resolution !== undefined) params.resolution = resolution.toLowerCase()
  return params
}

export function updateRouteMarginSearch(
  search: CostAccountingSearch,
  patch: Partial<CostAccountingSearch>
): CostAccountingSearch {
  const resetsPage = Object.keys(patch).some((key) =>
    routeMarginPageResetFields.has(key as keyof CostAccountingSearch)
  )
  return {
    ...search,
    ...patch,
    marginPage: resetsPage ? 1 : (patch.marginPage ?? search.marginPage),
  }
}

export function formatRouteMarginUSD(value?: number): string {
  if (value === undefined || !Number.isSafeInteger(value)) return ''
  return `USD ${new Decimal(value).div(1_000_000_000).toString()}`
}

export function formatRouteMarginPercent(value?: number): string {
  if (value === undefined || !Number.isSafeInteger(value)) return ''
  return `${new Decimal(value).div(10_000).toString()}%`
}

export function routeMarginScenarioLabel(
  scenario: 'no_video' | 'with_video',
  translate: (key: string) => string
): string {
  return translate(scenario === 'with_video' ? 'With video' : 'No video')
}

export function routeMarginFailureReasonLabel(
  reason: string | undefined,
  translate: (key: string) => string
): string {
  const keys: Record<string, string> = {
    revenue_unknown: 'Revenue unavailable',
    cost_rule_missing: 'Cost rule missing',
    meter_unknown: 'Cost meter unavailable',
    margin_below_threshold: 'Margin below threshold',
    calculation_error: 'Margin calculation failed',
    metadata_unavailable: 'Required metadata unavailable',
  }
  return reason ? translate(keys[reason] ?? reason) : ''
}

export function routeMarginRevenueSourceLabel(
  source: string,
  translate: (key: string) => string
): string {
  return source === 'runtime_billing_settings'
    ? translate('Runtime billing settings')
    : source
}
