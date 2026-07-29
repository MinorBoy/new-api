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

import { useAutoSearch } from '../hooks/use-auto-search'
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
          }

    cancel()
    setFilters(next)
  }, [
    props.logCategory,
    searchParams.startTime,
    searchParams.endTime,
    searchParams.channel,
    searchParams.filter,
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
  const placeholder =
    props.logCategory === 'drawing'
      ? t('Filter by MjProxy task ID')
      : t('Filter by task ID')
  const hasAdditionalFilters = !!filterValue || !!filters.channel
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
  const channelFilter = isAdmin ? (
    <LogsFilterField>
      <LogsFilterInput
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

  return (
    <LogsFilterToolbar
      table={props.table}
      primaryFilters={
        <>
          {dateRangeFilter}
          {taskIdFilter}
          {channelFilter}
        </>
      }
      mobilePinnedFilters={dateRangeFilter}
      mobileFilters={
        <>
          {taskIdFilter}
          {channelFilter}
        </>
      }
      mobileFilterCount={[filterValue, filters.channel].filter(Boolean).length}
      hasActiveFilters={hasAdditionalFilters}
      onReset={handleReset}
    />
  )
}
