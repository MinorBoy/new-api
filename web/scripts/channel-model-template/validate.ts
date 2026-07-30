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
import type { Issue, TemplateData } from './types'

function duplicateIssues<T extends { businessId: string }>(
  values: T[],
  entity: string
): Issue[] {
  const counts = new Map<string, number>()
  for (const value of values) {
    counts.set(value.businessId, (counts.get(value.businessId) ?? 0) + 1)
  }
  return [...counts.entries()]
    .filter(([, count]) => count > 1)
    .map(([businessId]) => ({
      code: `${entity.toUpperCase()}_DUPLICATE_ID`,
      severity: 'FAIL' as const,
      businessId,
      message: `${entity} business ID is duplicated: ${businessId}`,
      suggestion: 'Make the source identity or rule override unique.',
    }))
}

export function validateTemplateData(data: TemplateData): Issue[] {
  const issues = [
    ...duplicateIssues(data.channels, 'channel'),
    ...duplicateIssues(data.skus, 'sku'),
    ...duplicateIssues(data.sales, 'sale'),
    ...duplicateIssues(data.costs, 'cost'),
    ...duplicateIssues(data.mappings, 'mapping'),
    ...duplicateIssues(data.profits, 'profit'),
  ]
  const channelIds = new Set(data.channels.map((row) => row.businessId))
  const skuIds = new Set(data.skus.map((row) => row.businessId))
  const sourceIds = new Set(data.sources.map((row) => row.businessId))
  for (const cost of data.costs) {
    if (!channelIds.has(cost.channelCode)) {
      issues.push({
        code: 'COST_CHANNEL_UNRESOLVED',
        severity: 'FAIL',
        businessId: cost.businessId,
        message: `Cost references unknown channel ${cost.channelCode}.`,
      })
    }
    if (!skuIds.has(cost.skuCode)) {
      issues.push({
        code: 'COST_SKU_UNRESOLVED',
        severity: 'WARN',
        businessId: cost.businessId,
        message: `Cost references draft SKU ${cost.skuCode}.`,
      })
    }
    if (!sourceIds.has(cost.sourceId)) {
      issues.push({
        code: 'COST_SOURCE_UNRESOLVED',
        severity: 'FAIL',
        businessId: cost.businessId,
        message: `Cost references unknown source ${cost.sourceId}.`,
      })
    }
  }
  return issues.sort((left, right) => {
    const severityOrder = { FAIL: 0, WARN: 1, INFO: 2 }
    return (
      severityOrder[left.severity] - severityOrder[right.severity] ||
      left.code.localeCompare(right.code) ||
      (left.businessId ?? '').localeCompare(right.businessId ?? '')
    )
  })
}
