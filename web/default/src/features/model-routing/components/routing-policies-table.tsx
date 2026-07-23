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
import { getRouteApi } from '@tanstack/react-router'
import type { ColumnDef, ColumnFiltersState } from '@tanstack/react-table'
import { RefreshCw } from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { DataTablePage, useDataTable } from '@/components/data-table'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { useTableUrlState } from '@/hooks/use-table-url-state'
import dayjs from '@/lib/dayjs'

import { listRoutingPolicies } from '../api'
import { routingPolicyQueryKeys } from '../query-keys'
import { CANONICAL_SEEDANCE_MODELS, type RoutingPolicy } from '../types'

const route = getRouteApi('/_authenticated/models/$section')

function updateStringFilter(
  filters: ColumnFiltersState,
  columnId: string,
  value: string
): ColumnFiltersState {
  const next = filters.filter((filter) => filter.id !== columnId)
  if (value.trim()) {
    next.push({ id: columnId, value })
  }
  return next
}

export function RoutingPoliciesTable() {
  const { t } = useTranslation()
  const search = route.useSearch()
  const navigate = route.useNavigate()
  const {
    globalFilter,
    onGlobalFilterChange,
    columnFilters,
    onColumnFiltersChange,
    pagination,
    onPaginationChange,
    ensurePageInRange,
  } = useTableUrlState({
    search,
    navigate,
    pagination: {
      pageKey: 'rPage',
      pageSizeKey: 'rPageSize',
      defaultPage: 1,
      defaultPageSize: 10,
    },
    globalFilter: { enabled: true, key: 'rModel' },
    columnFilters: [
      { columnId: 'group_name', searchKey: 'rGroup', type: 'string' },
      {
        columnId: 'channel_id',
        searchKey: 'rChannel',
        type: 'string',
        deserialize: (value) =>
          typeof value === 'number' && value > 0 ? String(value) : '',
        serialize: (value) => {
          const channelID = Number(value)
          return Number.isInteger(channelID) && channelID > 0
            ? channelID
            : undefined
        },
      },
    ],
  })

  const groupFilter =
    (columnFilters.find((filter) => filter.id === 'group_name')?.value as
      | string
      | undefined) ?? ''
  const channelFilter =
    (columnFilters.find((filter) => filter.id === 'channel_id')?.value as
      | string
      | undefined) ?? ''
  const channelID = Number(channelFilter)
  const params = {
    group_name: groupFilter || undefined,
    model: globalFilter || undefined,
    channel_id:
      Number.isInteger(channelID) && channelID > 0 ? channelID : undefined,
    p: pagination.pageIndex + 1,
    page_size: pagination.pageSize,
  }
  const policiesQuery = useQuery({
    queryKey: routingPolicyQueryKeys.list(params),
    queryFn: () => listRoutingPolicies(params),
    placeholderData: (previous) => previous,
  })

  const columns = useMemo<ColumnDef<RoutingPolicy, unknown>[]>(
    () => [
      {
        accessorKey: 'group_name',
        header: t('Group'),
      },
      {
        accessorKey: 'model',
        header: t('Canonical model'),
        cell: ({ row }) => (
          <span className='block max-w-72 font-mono text-xs break-all'>
            {row.original.model}
          </span>
        ),
      },
      {
        id: 'defaults',
        header: t('Defaults'),
        cell: ({ row }) => (
          <span className='text-muted-foreground whitespace-nowrap'>
            {row.original.defaults.output_resolution} ·{' '}
            {row.original.defaults.duration_seconds}s ·{' '}
            {row.original.defaults.aspect_ratio}
          </span>
        ),
      },
      {
        id: 'targets',
        header: t('Routing targets'),
        cell: ({ row }) => row.original.targets.length,
      },
      {
        accessorKey: 'enabled',
        header: t('Status'),
        cell: ({ row }) => (
          <span
            className={
              row.original.enabled
                ? 'text-emerald-600 dark:text-emerald-400'
                : 'text-muted-foreground'
            }
          >
            {row.original.enabled ? t('Enabled') : t('Disabled')}
          </span>
        ),
      },
      {
        accessorKey: 'updated_at',
        header: t('Updated time'),
        cell: ({ row }) => (
          <span className='text-muted-foreground whitespace-nowrap'>
            {dayjs.unix(row.original.updated_at).format('YYYY-MM-DD HH:mm')}
          </span>
        ),
      },
    ],
    [t]
  )

  const policies = policiesQuery.data?.data.items ?? []
  const totalCount = policiesQuery.data?.data.total ?? 0
  const { table } = useDataTable({
    data: policies,
    columns,
    totalCount,
    columnFilters,
    pagination,
    globalFilter,
    onColumnFiltersChange,
    onPaginationChange,
    onGlobalFilterChange,
    manualPagination: true,
    manualFiltering: true,
    withSortedRowModel: false,
    ensurePageInRange,
  })

  if (policiesQuery.isError) {
    const message =
      policiesQuery.error instanceof Error
        ? policiesQuery.error.message
        : t('Failed to load routing policies')
    return (
      <div className='flex min-h-48 flex-col items-center justify-center gap-3 text-center'>
        <p className='text-destructive text-sm'>{message}</p>
        <Tooltip>
          <TooltipTrigger
            render={
              <Button
                type='button'
                variant='outline'
                size='icon'
                aria-label={t('Retry')}
                onClick={() => void policiesQuery.refetch()}
              />
            }
          >
            <RefreshCw />
          </TooltipTrigger>
          <TooltipContent>{t('Retry')}</TooltipContent>
        </Tooltip>
      </div>
    )
  }

  return (
    <DataTablePage
      table={table}
      columns={columns}
      isLoading={policiesQuery.isLoading}
      isFetching={policiesQuery.isFetching}
      emptyTitle={t('No routing policies found')}
      skeletonKeyPrefix='routing-policy-skeleton'
      applyHeaderSize
      toolbar={
        <div className='grid gap-2 sm:grid-cols-3'>
          <Input
            value={groupFilter}
            onChange={(event) =>
              onColumnFiltersChange(
                updateStringFilter(
                  columnFilters,
                  'group_name',
                  event.target.value
                )
              )
            }
            placeholder={t('Filter by group...')}
            aria-label={t('Group')}
          />
          <NativeSelect
            className='w-full'
            value={globalFilter ?? ''}
            onChange={(event) => onGlobalFilterChange?.(event.target.value)}
            aria-label={t('Canonical model')}
          >
            <NativeSelectOption value=''>{t('All models')}</NativeSelectOption>
            {CANONICAL_SEEDANCE_MODELS.map((model) => (
              <NativeSelectOption key={model} value={model}>
                {model}
              </NativeSelectOption>
            ))}
          </NativeSelect>
          <Input
            type='number'
            min={1}
            value={channelFilter}
            onChange={(event) =>
              onColumnFiltersChange(
                updateStringFilter(
                  columnFilters,
                  'channel_id',
                  event.target.value
                )
              )
            }
            placeholder={t('Filter by channel ID...')}
            aria-label={t('Channel ID')}
          />
        </div>
      }
    />
  )
}
