/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.
*/
import type { ConfigImportItemDetail, ConfigImportItemState } from '../types'

export interface ConfigImportItemDiffGroup {
  entityType: string
  state: ConfigImportItemState
  items: ConfigImportItemDetail[]
}

const stateOrder: ConfigImportItemState[] = [
  'new',
  'changed',
  'conflict',
  'unchanged',
  'excluded',
]

export function groupItemDiffs(
  items: ConfigImportItemDetail[]
): ConfigImportItemDiffGroup[] {
  const grouped = new Map<string, ConfigImportItemDiffGroup>()
  for (const item of items) {
    const key = `${item.entity_type}:${item.state}`
    const current = grouped.get(key)
    if (current) {
      current.items.push(item)
      continue
    }
    grouped.set(key, {
      entityType: item.entity_type,
      state: item.state,
      items: [item],
    })
  }

  return [...grouped.values()]
    .map((group) => ({
      ...group,
      items: [...group.items].sort((left, right) =>
        left.business_id.localeCompare(right.business_id)
      ),
    }))
    .sort((left, right) => {
      const entityComparison = left.entityType.localeCompare(right.entityType)
      if (entityComparison !== 0) return entityComparison
      return stateOrder.indexOf(left.state) - stateOrder.indexOf(right.state)
    })
}
