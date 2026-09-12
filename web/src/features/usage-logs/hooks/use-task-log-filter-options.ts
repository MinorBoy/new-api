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
