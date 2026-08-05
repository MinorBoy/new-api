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
import { useQuery } from '@tanstack/react-query'
import { useMemo } from 'react'

import { getChannels } from '@/features/channels/api'
import { getApiKeys } from '@/features/keys/api'
import { getGroups } from '@/features/users/api'

import { getLogModels, getUserLogModels } from '../api'

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

/**
 * Filters that scope the "used models" dropdown. These mirror the Common Logs
 * filter bar fields (minus the model filter itself, which would be circular).
 * Passing the current values keeps the dropdown in sync with the time range /
 * log type / token / group / channel / username the user has selected.
 */
export type LogModelScope = {
  type?: number
  start_timestamp?: number
  end_timestamp?: number
  token_name?: string
  group?: string
  channel?: number
  username?: string
}

export function useCommonLogFilterOptions(
  isAdmin: boolean,
  modelScope: LogModelScope
): {
  modelOptions: LogFilterOption[]
  groupOptions: LogFilterOption[]
  tokenOptions: LogFilterOption[]
  channelOptions: LogFilterOption[]
} {
  const groupsQuery = useQuery({
    queryKey: ['usage-log-filter-options', 'groups'],
    queryFn: getGroups,
    enabled: isAdmin,
    staleTime: 5 * 60 * 1000,
  })
  const tokensQuery = useQuery({
    queryKey: ['usage-log-filter-options', 'tokens'],
    queryFn: () => getApiKeys({ p: 1, size: 1000 }),
    staleTime: 5 * 60 * 1000,
  })
  // Model names come from the logs themselves (logs.model_name) so the dropdown
  // only shows models the user actually requested, scoped by the current
  // filters — not the configured upstream channel models.
  const logModelsQuery = useQuery({
    queryKey: ['usage-log-filter-options', 'log-models', isAdmin, modelScope],
    queryFn: async () => {
      const result = isAdmin
        ? await getLogModels(modelScope)
        : await getUserLogModels(modelScope)
      return result.data ?? []
    },
    staleTime: 30 * 1000,
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
        createOptions(
          (logModelsQuery.data ?? []).map((model) => ({
            value: model,
            label: model,
          }))
        ),
      [logModelsQuery.data]
    ),
    groupOptions: useMemo(
      () =>
        isAdmin
          ? createOptions(
              (groupsQuery.data?.data ?? []).map((group) => ({
                value: group,
                label: group,
              }))
            )
          : [],
      [groupsQuery.data, isAdmin]
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
