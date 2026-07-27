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
export type RoutingCostCandidate = {
  businessId: string
  lineRef: string
  nativeUnitPrice: string
  resolution: string
  scenario: string
  skuRef: string
  sourceBusinessIds: string[]
  supportsRealPerson?: boolean
  upstreamModel: string
}

export type MaterializedCost = {
  business_id: string
  cost_variant_key: string
  native_unit_price: string
  source_business_ids: string[]
}

export type RouteBlueprint = {
  cost_variant_key: string
  enabled: false
  line_ref: string
  route_target_ref: string
  sku_ref: string
  supports_real_person?: boolean
  upstream_model: string
}

export type RoutingIssue = {
  business_id: string
  code: 'COST_VARIANT_AMBIGUOUS'
  source_business_ids: string[]
}

function variantKey(candidate: RoutingCostCandidate): string {
  return candidate.resolution.trim().toLowerCase()
}

function routeConditionKey(candidate: RoutingCostCandidate): string {
  return [
    candidate.lineRef,
    candidate.upstreamModel,
    candidate.skuRef,
    variantKey(candidate),
    candidate.supportsRealPerson ? 'real-person' : 'no-real-person',
  ].join('/')
}

export function materializeRouting(candidates: RoutingCostCandidate[]): {
  costs: MaterializedCost[]
  issues: RoutingIssue[]
  routes: RouteBlueprint[]
} {
  const equivalent = new Map<string, RoutingCostCandidate[]>()
  for (const candidate of candidates) {
    const key = `${routeConditionKey(candidate)}/${candidate.nativeUnitPrice}`
    const group = equivalent.get(key) ?? []
    group.push(candidate)
    equivalent.set(key, group)
  }
  const deduplicated = [...equivalent.values()].map((group) => {
    const representative = [...group].sort((left, right) =>
      left.businessId.localeCompare(right.businessId)
    )[0]
    return {
      candidate: representative,
      sourceBusinessIds: group
        .flatMap((item) => item.sourceBusinessIds)
        .sort((left, right) => left.localeCompare(right)),
    }
  })

  const byRouteCondition = new Map<string, typeof deduplicated>()
  for (const item of deduplicated) {
    const key = routeConditionKey(item.candidate)
    const group = byRouteCondition.get(key) ?? []
    group.push(item)
    byRouteCondition.set(key, group)
  }

  const costs: MaterializedCost[] = []
  const issues: RoutingIssue[] = []
  const routes: RouteBlueprint[] = []
  for (const [condition, group] of byRouteCondition) {
    const prices = new Set(group.map((item) => item.candidate.nativeUnitPrice))
    if (prices.size > 1) {
      issues.push({
        code: 'COST_VARIANT_AMBIGUOUS',
        business_id: condition,
        source_business_ids: group
          .flatMap((item) => item.sourceBusinessIds)
          .sort((left, right) => left.localeCompare(right)),
      })
      continue
    }
    const item = group[0]
    const candidate = item.candidate
    const variant = variantKey(candidate)
    const capability = candidate.supportsRealPerson
      ? 'real-person'
      : 'no-real-person'
    costs.push({
      business_id: candidate.businessId,
      cost_variant_key: variant,
      native_unit_price: candidate.nativeUnitPrice,
      source_business_ids: item.sourceBusinessIds,
    })
    routes.push({
      route_target_ref: `route/${candidate.lineRef}/${candidate.upstreamModel}/${variant}/${capability}`,
      line_ref: candidate.lineRef,
      upstream_model: candidate.upstreamModel,
      sku_ref: candidate.skuRef,
      cost_variant_key: variant,
      ...(candidate.supportsRealPerson !== undefined
        ? { supports_real_person: candidate.supportsRealPerson }
        : {}),
      enabled: false,
    })
  }
  return { costs, issues, routes }
}
