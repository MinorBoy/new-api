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
*/
import { useQuery } from '@tanstack/react-query'
import { useMemo } from 'react'

import type { ComboboxInputOption } from '@/components/ui/combobox-input'

import { costAccountingQueryKeys, getCostReportFilterOptions } from '../api'
import {
  costReportParamsFromSearch,
  type CostAccountingSearch,
} from '../lib/report'
import type { CostReportFilterOptions } from '../types'

export type ProfitFilterOptions = {
  channels: ComboboxInputOption[]
  billableModels: ComboboxInputOption[]
  originModels: ComboboxInputOption[]
  userGroups: ComboboxInputOption[]
  usingGroups: ComboboxInputOption[]
}

type ProfitFilterSelections = Pick<
  CostAccountingSearch,
  'channelId' | 'billableModel' | 'originModel' | 'userGroup' | 'usingGroup'
>

const EMPTY_PROFIT_FILTER_OPTIONS: ProfitFilterOptions = {
  channels: [],
  billableModels: [],
  originModels: [],
  userGroups: [],
  usingGroups: [],
}

function stringOptions(
  values: string[] | undefined,
  selectedValue: string | undefined
): ComboboxInputOption[] {
  const uniqueValues = new Set<string>()
  for (const value of values ?? []) {
    const trimmed = value.trim()
    if (trimmed) uniqueValues.add(trimmed)
  }
  const selected = selectedValue?.trim()
  if (selected) uniqueValues.add(selected)
  return [...uniqueValues]
    .sort((left, right) => left.localeCompare(right))
    .map((value) => ({ value, label: value }))
}

export function normalizeProfitFilterOptions(
  data: CostReportFilterOptions | undefined,
  selections: ProfitFilterSelections
): ProfitFilterOptions {
  if (!data && !Object.values(selections).some(Boolean)) {
    return EMPTY_PROFIT_FILTER_OPTIONS
  }

  const channels = new Map<number, ComboboxInputOption>()
  for (const channel of data?.channels ?? []) {
    if (
      !Number.isInteger(channel.id) ||
      channel.id <= 0 ||
      channels.has(channel.id)
    ) {
      continue
    }
    const name = channel.name.trim()
    channels.set(channel.id, {
      value: String(channel.id),
      label: name ? `${channel.id} - ${name}` : String(channel.id),
    })
  }
  if (selections.channelId && !channels.has(selections.channelId)) {
    channels.set(selections.channelId, {
      value: String(selections.channelId),
      label: String(selections.channelId),
    })
  }

  return {
    channels: [...channels.entries()]
      .sort(([left], [right]) => left - right)
      .map(([, option]) => option),
    billableModels: stringOptions(
      data?.billable_upstream_models,
      selections.billableModel
    ),
    originModels: stringOptions(data?.origin_models, selections.originModel),
    userGroups: stringOptions(data?.user_groups, selections.userGroup),
    usingGroups: stringOptions(data?.using_groups, selections.usingGroup),
  }
}

export function useProfitFilterOptions(
  search: CostAccountingSearch,
  enabled = true
): ProfitFilterOptions {
  const params = useMemo(() => costReportParamsFromSearch(search), [search])
  const query = useQuery({
    queryKey: costAccountingQueryKeys.reportFilterOptions(params),
    queryFn: async () => (await getCostReportFilterOptions(params)).data,
    enabled,
    staleTime: 30_000,
  })

  return useMemo(
    () => normalizeProfitFilterOptions(query.data, search),
    [query.data, search]
  )
}
