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

import { getTaskLogFilterOptions } from '../api'
import type {
  GetTaskLogFilterOptionsParams,
  TaskLogFilterOptionsData,
} from '../types'

export type TaskLogFilterOption = {
  value: string
  label: string
}

export type NormalizedTaskLogFilterOptions = {
  channelOptions: TaskLogFilterOption[]
  statusOptions: string[]
  requestModelOptions: TaskLogFilterOption[]
  userOptions: TaskLogFilterOption[]
}

const EMPTY_TASK_LOG_FILTER_OPTIONS: NormalizedTaskLogFilterOptions = {
  channelOptions: [],
  statusOptions: [],
  requestModelOptions: [],
  userOptions: [],
}

export function buildTaskLogFilterOptionsQueryKey(
  isAdmin: boolean,
  params: GetTaskLogFilterOptionsParams
) {
  return [
    'usage-log-filter-options',
    'task',
    isAdmin,
    params.start_timestamp,
    params.end_timestamp,
  ] as const
}

export function normalizeTaskLogFilterOptions(
  data: TaskLogFilterOptionsData
): NormalizedTaskLogFilterOptions {
  const channels = new Map<number, TaskLogFilterOption>()
  for (const channel of data.channels ?? []) {
    if (channels.has(channel.id)) continue
    channels.set(channel.id, {
      value: String(channel.id),
      label: channel.name
        ? `${channel.id} - ${channel.name}`
        : String(channel.id),
    })
  }
  const channelOptions = [...channels.entries()]
    .sort(([left], [right]) => left - right)
    .map(([, option]) => option)
  const statusOptions = [...new Set(data.statuses ?? [])].sort((left, right) =>
    left.localeCompare(right)
  )
  const requestModelOptions = [...new Set(data.request_models ?? [])]
    .sort((left, right) => left.localeCompare(right))
    .map((model) => ({ value: model, label: model }))
  const users = new Map<number, TaskLogFilterOption>()
  for (const user of data.users ?? []) {
    users.set(user.id, {
      value: String(user.id),
      label: user.username ? `${user.id} - ${user.username}` : String(user.id),
    })
  }

  return {
    channelOptions,
    statusOptions,
    requestModelOptions,
    userOptions: [...users.entries()]
      .sort(([left], [right]) => left - right)
      .map(([, option]) => option),
  }
}

export function useTaskLogFilterOptions(config: {
  isAdmin: boolean
  startTime?: Date
  endTime?: Date
  enabled?: boolean
}): NormalizedTaskLogFilterOptions {
  const params: GetTaskLogFilterOptionsParams = {
    start_timestamp: config.startTime
      ? Math.floor(config.startTime.getTime() / 1000)
      : undefined,
    end_timestamp: config.endTime
      ? Math.floor(config.endTime.getTime() / 1000)
      : undefined,
  }
  const query = useQuery({
    queryKey: buildTaskLogFilterOptionsQueryKey(config.isAdmin, params),
    queryFn: async () => {
      const response = await getTaskLogFilterOptions(params, config.isAdmin)
      return normalizeTaskLogFilterOptions(response.data ?? {})
    },
    enabled: config.enabled ?? true,
    staleTime: 30 * 1000,
  })

  return query.data ?? EMPTY_TASK_LOG_FILTER_OPTIONS
}
