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
import { Eye, EyeOff } from 'lucide-react'
import {
  useState,
  useCallback,
  useMemo,
  useRef,
  type CompositionEvent,
} from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Combobox } from '@/components/ui/combobox'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'

import { LOG_TYPE_ALL_VALUE, LOG_TYPE_FILTERS } from '../constants'
import { useAutoSearch } from '../hooks/use-auto-search'
import { useCommonLogFilterOptions } from '../hooks/use-common-log-filter-options'
import { buildSearchParams } from '../lib/filter'
import { getDefaultTimeRange } from '../lib/time-range'
import type { CommonLogFilters } from '../types'
import { CommonLogsStats } from './common-logs-stats'
import { CompactDateTimeRangePicker } from './compact-date-time-range-picker'
import {
  LogsFilterField,
  LogsFilterInput,
  LogsFilterToolbar,
} from './logs-filter-toolbar'
import { useLogsViewScope, useUsageLogsContext } from './usage-logs-provider'

const route = getRouteApi('/_authenticated/usage-logs/$section')

type LogTypeValue = (typeof LOG_TYPE_FILTERS)[number]['value']
const logTypeValueSet = new Set<string>(
  LOG_TYPE_FILTERS.map((type) => type.value)
)

type CommonLogDraft = {
  sourceKey: string
  filters: CommonLogFilters
  logType: LogTypeValue
}

function isLogTypeValue(value: string): value is LogTypeValue {
  return logTypeValueSet.has(value)
}

function getLogTypeValue(value: unknown): LogTypeValue {
  return Array.isArray(value) &&
    value.length === 1 &&
    typeof value[0] === 'string' &&
    isLogTypeValue(value[0])
    ? value[0]
    : LOG_TYPE_ALL_VALUE
}

function buildSearchSourceKey(values: {
  startTime?: unknown
  endTime?: unknown
  channel?: unknown
  model?: unknown
  token?: unknown
  group?: unknown
  username?: unknown
  requestId?: unknown
  upstreamRequestId?: unknown
  type?: unknown
}) {
  return [
    values.startTime,
    values.endTime,
    values.channel,
    values.model,
    values.token,
    values.group,
    values.username,
    values.requestId,
    values.upstreamRequestId,
    Array.isArray(values.type) ? values.type.join(',') : values.type,
  ]
    .map((value) => String(value ?? ''))
    .join('\u001f')
}

interface CommonLogsFilterBarProps<TData> {
  table: Table<TData>
}

export function CommonLogsFilterBar<TData>(
  props: CommonLogsFilterBarProps<TData>
) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const searchParams = route.useSearch()
  const { isAdminView: isAdmin } = useLogsViewScope()
  const { sensitiveVisible, setSensitiveVisible } = useUsageLogsContext()
  const { modelOptions, groupOptions, tokenOptions, channelOptions } =
    useCommonLogFilterOptions(isAdmin)

  const searchState = useMemo<CommonLogDraft>(() => {
    const { start, end } = getDefaultTimeRange()
    const sourceValues = {
      startTime: searchParams.startTime,
      endTime: searchParams.endTime,
      channel: searchParams.channel,
      model: searchParams.model,
      token: searchParams.token,
      group: searchParams.group,
      username: searchParams.username,
      requestId: searchParams.requestId,
      upstreamRequestId: searchParams.upstreamRequestId,
      type: searchParams.type,
    }
    const filters: CommonLogFilters = {
      startTime: searchParams.startTime
        ? new Date(searchParams.startTime)
        : start,
      endTime: searchParams.endTime ? new Date(searchParams.endTime) : end,
      channel: searchParams.channel || undefined,
      model: searchParams.model || undefined,
      token: searchParams.token || undefined,
      group: searchParams.group || undefined,
      username: searchParams.username || undefined,
      requestId: searchParams.requestId || undefined,
      upstreamRequestId: searchParams.upstreamRequestId || undefined,
    }
    return {
      sourceKey: buildSearchSourceKey(sourceValues),
      filters,
      logType: getLogTypeValue(searchParams.type),
    }
  }, [
    searchParams.startTime,
    searchParams.endTime,
    searchParams.channel,
    searchParams.model,
    searchParams.token,
    searchParams.group,
    searchParams.username,
    searchParams.requestId,
    searchParams.upstreamRequestId,
    searchParams.type,
  ])
  const [draft, setDraft] = useState<CommonLogDraft>(() => searchState)
  const activeDraft =
    draft.sourceKey === searchState.sourceKey ? draft : searchState
  const filters = activeDraft.filters
  const logType = activeDraft.logType

  const submitDraft = useCallback(
    (nextDraft: CommonLogDraft) => {
      const filterParams = buildSearchParams(nextDraft.filters, 'common')
      navigate({
        to: '/usage-logs/$section',
        params: { section: 'common' },
        search: {
          ...filterParams,
          type: [nextDraft.logType],
          page: 1,
        },
      })
      queryClient.invalidateQueries({ queryKey: ['logs'] })
      queryClient.invalidateQueries({ queryKey: ['usage-logs-stats'] })
    },
    [navigate, queryClient]
  )
  const { schedule, flush } = useAutoSearch(submitDraft)
  const composingFieldsRef = useRef<Set<keyof CommonLogFilters>>(new Set())

  const createDraft = useCallback(
    (
      nextFilters: CommonLogFilters,
      nextLogType: LogTypeValue = logType
    ): CommonLogDraft => ({
      sourceKey: searchState.sourceKey,
      filters: nextFilters,
      logType: nextLogType,
    }),
    [logType, searchState.sourceKey]
  )

  const handleImmediateChange = useCallback(
    (changes: Partial<CommonLogFilters>) => {
      const nextDraft = createDraft({ ...filters, ...changes })
      setDraft(nextDraft)
      flush(nextDraft)
    },
    [createDraft, filters, flush]
  )

  const handleTextChange = useCallback(
    (field: keyof CommonLogFilters, value: string) => {
      const nextDraft = createDraft({ ...filters, [field]: value || undefined })
      setDraft(nextDraft)
      if (!composingFieldsRef.current.has(field)) {
        schedule(nextDraft)
      }
    },
    [createDraft, filters, schedule]
  )

  const handleCompositionStart = useCallback(
    (field: keyof CommonLogFilters) => {
      composingFieldsRef.current.add(field)
    },
    []
  )

  const handleCompositionEnd = useCallback(
    (field: keyof CommonLogFilters, value: string) => {
      composingFieldsRef.current.delete(field)
      handleTextChange(field, value)
    },
    [handleTextChange]
  )

  const handleReset = useCallback(() => {
    const { start, end } = getDefaultTimeRange()
    const resetFilters: CommonLogFilters = { startTime: start, endTime: end }
    const resetDraft: CommonLogDraft = {
      sourceKey: buildSearchSourceKey({
        type: [LOG_TYPE_ALL_VALUE],
        startTime: start.getTime(),
        endTime: end.getTime(),
      }),
      filters: resetFilters,
      logType: LOG_TYPE_ALL_VALUE,
    }
    setDraft(resetDraft)
    flush(resetDraft)
  }, [flush])

  const hasExpandedFilters =
    !!filters.token ||
    !!filters.username ||
    !!filters.channel ||
    !!filters.requestId ||
    !!filters.upstreamRequestId

  const hasTypeFilter = logType !== LOG_TYPE_ALL_VALUE
  const hasAdditionalFilters =
    !!filters.model || !!filters.group || hasTypeFilter || hasExpandedFilters

  const expandedFilterCount = [
    filters.token,
    isAdmin ? filters.username : undefined,
    isAdmin ? filters.channel : undefined,
    filters.requestId,
    filters.upstreamRequestId,
  ].filter(Boolean).length
  const sensitiveType = sensitiveVisible ? 'text' : 'password'
  const logTypeItems = useMemo(
    () =>
      LOG_TYPE_FILTERS.map((type) => ({
        value: type.value,
        label: t(type.label),
      })),
    [t]
  )
  const logTypeLabel =
    logTypeItems.find((type) => type.value === logType)?.label ?? t('All Types')

  const statsBar = (
    <div className='flex flex-wrap items-center gap-2'>
      <CommonLogsStats />
    </div>
  )
  const sensitiveToggle = (
    <Tooltip>
      <TooltipTrigger
        render={
          <Button
            variant='ghost'
            size='icon'
            onClick={() => setSensitiveVisible(!sensitiveVisible)}
            aria-label={sensitiveVisible ? t('Hide') : t('Show')}
            className='text-muted-foreground hover:text-foreground size-7'
          />
        }
      >
        {sensitiveVisible ? <Eye /> : <EyeOff />}
      </TooltipTrigger>
      <TooltipContent>
        {sensitiveVisible ? t('Hide') : t('Show')}
      </TooltipContent>
    </Tooltip>
  )

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
  const modelFilter = (
    <LogsFilterField>
      <Combobox
        options={modelOptions}
        placeholder={t('Model Name')}
        value={filters.model || ''}
        onValueChange={(value) => handleTextChange('model', value ?? '')}
        onCompositionStart={() => handleCompositionStart('model')}
        onCompositionEnd={(event: CompositionEvent<HTMLInputElement>) =>
          handleCompositionEnd('model', event.currentTarget.value)
        }
        allowCustomValue
        openOnFocus
        className='h-8 min-w-0 text-sm leading-5'
      />
    </LogsFilterField>
  )
  const groupFilter = (
    <LogsFilterField>
      <Combobox
        options={groupOptions}
        placeholder={t('Group')}
        value={filters.group || ''}
        onValueChange={(value) => handleTextChange('group', value ?? '')}
        onCompositionStart={() => handleCompositionStart('group')}
        onCompositionEnd={(event: CompositionEvent<HTMLInputElement>) =>
          handleCompositionEnd('group', event.currentTarget.value)
        }
        allowCustomValue
        openOnFocus
        className='h-8 min-w-0 text-sm leading-5'
      />
    </LogsFilterField>
  )
  const typeFilter = (
    <LogsFilterField>
      <Select
        items={logTypeItems}
        value={logType}
        onValueChange={(value) => {
          const nextLogType =
            value !== null && isLogTypeValue(value) ? value : LOG_TYPE_ALL_VALUE
          const nextDraft = createDraft(filters, nextLogType)
          setDraft(nextDraft)
          flush(nextDraft)
        }}
      >
        <SelectTrigger>
          <SelectValue>{logTypeLabel}</SelectValue>
        </SelectTrigger>
        <SelectContent alignItemWithTrigger={false}>
          <SelectGroup>
            {LOG_TYPE_FILTERS.map((type) => (
              <SelectItem key={type.value} value={type.value}>
                {t(type.label)}
              </SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
    </LogsFilterField>
  )
  const advancedFilters = (
    <>
      <LogsFilterField>
        <Combobox
          options={tokenOptions}
          placeholder={t('Token Name')}
          value={filters.token || ''}
          onValueChange={(value) => handleTextChange('token', value ?? '')}
          onCompositionStart={() => handleCompositionStart('token')}
          onCompositionEnd={(event: CompositionEvent<HTMLInputElement>) =>
            handleCompositionEnd('token', event.currentTarget.value)
          }
          allowCustomValue
          openOnFocus
          className='h-8 min-w-0 text-sm leading-5'
        />
      </LogsFilterField>
      {isAdmin && (
        <LogsFilterField>
          <LogsFilterInput
            placeholder={t('Username')}
            type={sensitiveType}
            value={filters.username || ''}
            onChange={(e) => handleTextChange('username', e.target.value)}
            onCompositionStart={() => handleCompositionStart('username')}
            onCompositionEnd={(e) =>
              handleCompositionEnd('username', e.currentTarget.value)
            }
          />
        </LogsFilterField>
      )}
      {isAdmin && (
        <LogsFilterField>
          <Combobox
            options={channelOptions}
            placeholder={t('Channel ID')}
            value={filters.channel || ''}
            onValueChange={(value) => handleTextChange('channel', value ?? '')}
            onCompositionStart={() => handleCompositionStart('channel')}
            onCompositionEnd={(event: CompositionEvent<HTMLInputElement>) =>
              handleCompositionEnd('channel', event.currentTarget.value)
            }
            allowCustomValue
            openOnFocus
            className='h-8 min-w-0 text-sm leading-5'
          />
        </LogsFilterField>
      )}
      <LogsFilterField>
        <LogsFilterInput
          placeholder={t('Request ID')}
          value={filters.requestId || ''}
          onChange={(e) => handleTextChange('requestId', e.target.value)}
          onCompositionStart={() => handleCompositionStart('requestId')}
          onCompositionEnd={(e) =>
            handleCompositionEnd('requestId', e.currentTarget.value)
          }
        />
      </LogsFilterField>
      <LogsFilterField>
        <LogsFilterInput
          placeholder={t('Upstream Request ID')}
          value={filters.upstreamRequestId || ''}
          onChange={(e) =>
            handleTextChange('upstreamRequestId', e.target.value)
          }
          onCompositionStart={() => handleCompositionStart('upstreamRequestId')}
          onCompositionEnd={(e) =>
            handleCompositionEnd('upstreamRequestId', e.currentTarget.value)
          }
        />
      </LogsFilterField>
    </>
  )

  return (
    <LogsFilterToolbar
      table={props.table}
      stats={statsBar}
      actionStart={sensitiveToggle}
      primaryFilters={
        <>
          {dateRangeFilter}
          {modelFilter}
          {groupFilter}
          {typeFilter}
        </>
      }
      advancedFilters={advancedFilters}
      mobilePinnedFilters={dateRangeFilter}
      mobileFilters={
        <>
          {modelFilter}
          {groupFilter}
          {typeFilter}
          {advancedFilters}
        </>
      }
      mobileFilterCount={
        [filters.model, filters.group, hasTypeFilter].filter(Boolean).length +
        expandedFilterCount
      }
      hasAdvancedActiveFilters={hasExpandedFilters}
      advancedFilterCount={expandedFilterCount}
      hasActiveFilters={hasAdditionalFilters}
      onReset={handleReset}
    />
  )
}
