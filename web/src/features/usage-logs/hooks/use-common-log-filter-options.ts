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

import {
  getAllModels,
  getChannels,
  getEnabledModels,
} from '@/features/channels/api'
import { getApiKeys } from '@/features/keys/api'
import { getGroups } from '@/features/users/api'

export type LogFilterOption = {
  value: string
  label: string
}

function createOptions(
  entries: Array<{ value: string; label: string }>
): LogFilterOption[] {
  const options = new Map<string, LogFilterOption>()
  for (const entry of entries) {
    if (!entry.value) continue
    options.set(entry.value, entry)
  }
  return [...options.values()].sort((left, right) =>
    left.label.localeCompare(right.label)
  )
}

export function getModelFilterOptions(
  isAdmin: boolean,
  allModels: Array<{ id: string }>,
  enabledModels: string[]
): LogFilterOption[] {
  const modelNames = isAdmin
    ? allModels.map((model) => model.id)
    : enabledModels
  return createOptions(
    modelNames.map((model) => ({ value: model, label: model }))
  )
}

export function useCommonLogFilterOptions(isAdmin: boolean): {
  modelOptions: LogFilterOption[]
  groupOptions: LogFilterOption[]
  tokenOptions: LogFilterOption[]
  channelOptions: LogFilterOption[]
} {
  const groupsQuery = useQuery({
    queryKey: ['usage-log-filter-options', 'groups'],
    queryFn: getGroups,
    staleTime: 5 * 60 * 1000,
  })
  const tokensQuery = useQuery({
    queryKey: ['usage-log-filter-options', 'tokens'],
    queryFn: () => getApiKeys({ p: 1, size: 1000 }),
    staleTime: 5 * 60 * 1000,
  })
  const modelsQuery = useQuery({
    queryKey: ['usage-log-filter-options', 'models'],
    queryFn: getAllModels,
    enabled: isAdmin,
    staleTime: 5 * 60 * 1000,
  })
  const enabledModelsQuery = useQuery({
    queryKey: ['usage-log-filter-options', 'enabled-models'],
    queryFn: getEnabledModels,
    enabled: !isAdmin,
    staleTime: 5 * 60 * 1000,
  })
  const channelsQuery = useQuery({
    queryKey: ['usage-log-filter-options', 'channels'],
    queryFn: () => getChannels({ p: 1, page_size: 1000 }),
    enabled: isAdmin,
    staleTime: 5 * 60 * 1000,
  })

  return {
    modelOptions: useMemo(
      () =>
        getModelFilterOptions(
          isAdmin,
          modelsQuery.data?.data ?? [],
          enabledModelsQuery.data?.data ?? []
        ),
      [enabledModelsQuery.data, isAdmin, modelsQuery.data]
    ),
    groupOptions: useMemo(
      () =>
        createOptions(
          (groupsQuery.data?.data ?? []).map((group) => ({
            value: group,
            label: group,
          }))
        ),
      [groupsQuery.data]
    ),
    tokenOptions: useMemo(
      () =>
        createOptions(
          (tokensQuery.data?.data?.items ?? []).map((token) => ({
            value: token.name,
            label: token.name,
          }))
        ),
      [tokensQuery.data]
    ),
    channelOptions: useMemo(
      () =>
        createOptions(
          (channelsQuery.data?.data?.items ?? []).map((channel) => ({
            value: String(channel.id),
            label: `${channel.id} - ${channel.name}`,
          }))
        ),
      [channelsQuery.data]
    ),
  }
}
