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
import { useQueryClient } from '@tanstack/react-query'
import { useNavigate, getRouteApi } from '@tanstack/react-router'
import type { Table } from '@tanstack/react-table'
import { useState, useEffect, useCallback, useRef } from 'react'
import { useTranslation } from 'react-i18next'

import { Combobox } from '@/components/ui/combobox'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

import { TASK_STATUS_MAPPINGS } from '../constants'
import { useAutoSearch } from '../hooks/use-auto-search'
import {
  useTaskLogFilterOptions,
  type TaskLogFilterOption,
} from '../hooks/use-task-log-filter-options'
import { buildSearchParams } from '../lib/filter'
import { getDefaultTimeRange } from '../lib/time-range'
import type { DrawingLogFilters, LogCategory, TaskLogFilters } from '../types'
import { CompactDateTimeRangePicker } from './compact-date-time-range-picker'
import {
  LogsFilterField,
  LogsFilterInput,
  LogsFilterToolbar,
} from './logs-filter-toolbar'
import { useLogsViewScope } from './usage-logs-provider'

const route = getRouteApi('/_authenticated/usage-logs/$section')

type TaskLikeLogCategory = Extract<LogCategory, 'drawing' | 'task'>
type TaskLogsFilters = DrawingLogFilters | TaskLogFilters

interface TaskLogsFilterBarProps<TData> {
  table: Table<TData>
  logCategory: TaskLikeLogCategory
}

interface TaskLogSelectFilterProps {
  value?: string
  ariaLabel: string
  allLabel: string
  options: TaskLogFilterOption[]
  onValueChange: (value: string) => void
}

const ALL_TASK_FILTER_VALUE = '__all__'

function TaskLogSelectFilter(props: TaskLogSelectFilterProps) {
  const selectedLabel =
    props.options.find((option) => option.value === props.value)?.label ??
    props.allLabel

  return (
    <Select
      value={props.value || ALL_TASK_FILTER_VALUE}
      onValueChange={(value) =>
        props.onValueChange(
          value === ALL_TASK_FILTER_VALUE ? '' : (value ?? '')
        )
      }
    >
      <SelectTrigger aria-label={props.ariaLabel}>
        <SelectValue>{selectedLabel}</SelectValue>
      </SelectTrigger>
      <SelectContent align='start'>
        <SelectGroup>
          <SelectItem value={ALL_TASK_FILTER_VALUE}>
            {props.allLabel}
          </SelectItem>
          {props.options.map((option) => (
            <SelectItem key={option.value} value={option.value}>
              {option.label}
            </SelectItem>
          ))}
        </SelectGroup>
      </SelectContent>
    </Select>
  )
}

function getFilterValue(
  filters: TaskLogsFilters,
  logCategory: TaskLikeLogCategory
): string {
  if (logCategory === 'drawing') {
    return (filters as DrawingLogFilters).mjId || ''
  }
  return (filters as TaskLogFilters).taskId || ''
}

function setFilterValue(
  filters: TaskLogsFilters,
  logCategory: TaskLikeLogCategory,
  value: string
): TaskLogsFilters {
  if (logCategory === 'drawing') {
    return { ...filters, mjId: value }
  }
  return { ...filters, taskId: value }
}

export function TaskLogsFilterBar<TData>(props: TaskLogsFilterBarProps<TData>) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const searchParams = route.useSearch()
  const { isAdminView: isAdmin } = useLogsViewScope()

  const [filters, setFilters] = useState<TaskLogsFilters>(() => {
    const { start, end } = getDefaultTimeRange()
    return { startTime: start, endTime: end }
  })
  const submitFilters = useCallback(
    (nextFilters: TaskLogsFilters) => {
      const filterParams = buildSearchParams(nextFilters, props.logCategory)
      navigate({
        to: '/usage-logs/$section',
        params: { section: props.logCategory },
        search: {
          ...filterParams,
          page: 1,
        },
      })
      queryClient.invalidateQueries({ queryKey: ['logs'] })
    },
    [navigate, props.logCategory, queryClient]
  )
  const { schedule, flush, cancel } = useAutoSearch(submitFilters)
  const composingFieldsRef = useRef<Set<'filter' | 'channel'>>(new Set())
  const taskFilterOptions = useTaskLogFilterOptions({
    isAdmin,
    startTime: filters.startTime,
    endTime: filters.endTime,
    enabled: props.logCategory === 'task',
  })

  useEffect(() => {
    const { start, end } = getDefaultTimeRange()
    const baseFilters = {
      startTime: searchParams.startTime
        ? new Date(searchParams.startTime)
        : start,
      endTime: searchParams.endTime ? new Date(searchParams.endTime) : end,
      ...(searchParams.channel
        ? { channel: String(searchParams.channel) }
        : {}),
    }
    const next: TaskLogsFilters =
      props.logCategory === 'drawing'
        ? {
            ...baseFilters,
            ...(searchParams.filter ? { mjId: searchParams.filter } : {}),
          }
        : {
            ...baseFilters,
            ...(searchParams.filter ? { taskId: searchParams.filter } : {}),
            ...(searchParams.status ? { status: searchParams.status } : {}),
            ...(searchParams.requestModel
              ? { requestModel: searchParams.requestModel }
              : {}),
            ...(isAdmin && searchParams.userId
              ? { userId: searchParams.userId }
              : {}),
          }

    cancel()
    setFilters(next)
  }, [
    props.logCategory,
    searchParams.startTime,
    searchParams.endTime,
    searchParams.channel,
    searchParams.filter,
    searchParams.status,
    searchParams.requestModel,
    searchParams.userId,
    isAdmin,
    cancel,
  ])

  const handleImmediateChange = useCallback(
    (changes: Partial<TaskLogsFilters>) => {
      const nextFilters = { ...filters, ...changes } as TaskLogsFilters
      setFilters(nextFilters)
      flush(nextFilters)
    },
    [filters, flush]
  )

  const handleTextChange = useCallback(
    (field: 'filter' | 'channel', value: string) => {
      const nextFilters =
        field === 'filter'
          ? setFilterValue(filters, props.logCategory, value)
          : { ...filters, channel: value || undefined }
      setFilters(nextFilters)
      if (!composingFieldsRef.current.has(field)) {
        schedule(nextFilters)
      }
    },
    [filters, props.logCategory, schedule]
  )

  const handleCompositionStart = useCallback((field: 'filter' | 'channel') => {
    composingFieldsRef.current.add(field)
  }, [])

  const handleCompositionEnd = useCallback(
    (field: 'filter' | 'channel', value: string) => {
      composingFieldsRef.current.delete(field)
      handleTextChange(field, value)
    },
    [handleTextChange]
  )

  const handleReset = useCallback(() => {
    const { start, end } = getDefaultTimeRange()
    const resetFilters: TaskLogsFilters = { startTime: start, endTime: end }
    setFilters(resetFilters)
    flush(resetFilters)
  }, [flush])

  const filterValue = getFilterValue(filters, props.logCategory)
  const taskFilters =
    props.logCategory === 'task' ? (filters as TaskLogFilters) : undefined
  const placeholder =
    props.logCategory === 'drawing'
      ? t('Filter by MjProxy task ID')
      : t('Filter by task ID')
  const hasAdditionalFilters =
    !!filterValue ||
    !!filters.channel ||
    !!taskFilters?.status ||
    !!taskFilters?.requestModel ||
    !!taskFilters?.userId
  const dateRangeFilter = (
    <LogsFilterField wide>
      <CompactDateTimeRangePicker
        start={filters.startTime}
        end={filters.endTime}
        onChange={({ start, end }) => {
          handleImmediateChange({ startTime: start, endTime: end })
        }}
      />
    </LogsFilterField>
  )
  const taskIdFilter = (
    <LogsFilterField>
      <LogsFilterInput
        aria-label={t('Task ID')}
        placeholder={placeholder}
        value={filterValue}
        onChange={(e) => handleTextChange('filter', e.target.value)}
        onCompositionStart={() => handleCompositionStart('filter')}
        onCompositionEnd={(e) =>
          handleCompositionEnd('filter', e.currentTarget.value)
        }
      />
    </LogsFilterField>
  )
  const drawingChannelFilter =
    isAdmin && props.logCategory === 'drawing' ? (
      <LogsFilterField>
        <LogsFilterInput
          aria-label={t('Channel ID')}
          placeholder={t('Channel ID')}
          value={filters.channel || ''}
          onChange={(e) => handleTextChange('channel', e.target.value)}
          onCompositionStart={() => handleCompositionStart('channel')}
          onCompositionEnd={(e) =>
            handleCompositionEnd('channel', e.currentTarget.value)
          }
        />
      </LogsFilterField>
    ) : null
  const channelFilter =
    isAdmin && taskFilters ? (
      <LogsFilterField>
        <TaskLogSelectFilter
          options={taskFilterOptions.channelOptions}
          value={taskFilters.channel}
          ariaLabel={t('Channel ID')}
          allLabel={t('All channels')}
          onValueChange={(value) =>
            handleImmediateChange({ channel: value || undefined })
          }
        />
      </LogsFilterField>
    ) : null
  const statusFilter = taskFilters ? (
    <LogsFilterField>
      <TaskLogSelectFilter
        ariaLabel={t('Status')}
        allLabel={t('All statuses')}
        value={taskFilters.status}
        options={taskFilterOptions.statusOptions.map((status) => ({
          value: status,
          label: t(TASK_STATUS_MAPPINGS[status]?.label ?? status),
        }))}
        onValueChange={(value) =>
          handleImmediateChange({ status: value || undefined })
        }
      />
    </LogsFilterField>
  ) : null
  const requestModelFilter = taskFilters ? (
    <LogsFilterField>
      <TaskLogSelectFilter
        ariaLabel={t('Request Model')}
        allLabel={t('All models')}
        value={taskFilters.requestModel}
        options={taskFilterOptions.requestModelOptions}
        onValueChange={(value) =>
          handleImmediateChange({ requestModel: value || undefined })
        }
      />
    </LogsFilterField>
  ) : null
  const userFilter =
    isAdmin && taskFilters ? (
      <LogsFilterField>
        <Combobox
          options={[
            { value: ALL_TASK_FILTER_VALUE, label: t('All users') },
            ...taskFilterOptions.userOptions,
          ]}
          ariaLabel={t('User')}
          placeholder={t('User')}
          value={taskFilters.userId || ALL_TASK_FILTER_VALUE}
          onValueChange={(value) =>
            handleImmediateChange({
              userId:
                value && value !== ALL_TASK_FILTER_VALUE ? value : undefined,
            })
          }
          allowCustomValue={false}
          openOnFocus
          className='h-8 min-w-0 text-sm leading-5'
        />
      </LogsFilterField>
    ) : null

  const taskSpecificFilters = (
    <>
      {channelFilter}
      {statusFilter}
      {requestModelFilter}
      {userFilter}
    </>
  )

  return (
    <LogsFilterToolbar
      table={props.table}
      primaryFilters={
        <>
          {dateRangeFilter}
          {taskIdFilter}
          {drawingChannelFilter}
          {taskSpecificFilters}
        </>
      }
      mobilePinnedFilters={dateRangeFilter}
      mobileFilters={
        <>
          {taskIdFilter}
          {drawingChannelFilter}
          {taskSpecificFilters}
        </>
      }
      mobileFilterCount={
        [
          filterValue,
          filters.channel,
          taskFilters?.status,
          taskFilters?.requestModel,
          taskFilters?.userId,
        ].filter(Boolean).length
      }
      hasActiveFilters={hasAdditionalFilters}
      onReset={handleReset}
    />
  )
}
