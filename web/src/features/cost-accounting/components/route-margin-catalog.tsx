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
import { Alert02Icon, RefreshIcon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useMutation, useQuery } from '@tanstack/react-query'
import type {
  PaginationState,
  SortingState,
  Updater,
} from '@tanstack/react-table'
import { useCallback, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  DataTablePage,
  DataTableRow,
  useDataTable,
} from '@/components/data-table'
import { Button } from '@/components/ui/button'
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'

import {
  costAccountingQueryKeys,
  exportRouteMarginCatalog,
  getRouteMarginCatalog,
} from '../api'
import { downloadCostCatalogExport } from '../lib/catalog-export'
import type { CostAccountingSearch } from '../lib/report'
import {
  routeMarginParamsFromSearch,
  updateRouteMarginSearch,
} from '../lib/route-margin-catalog'
import type { RouteMarginCatalogSort } from '../types'
import { useRouteMarginCatalogColumns } from './route-margin-catalog-columns'
import { RouteMarginCatalogFilters } from './route-margin-catalog-filters'
import { RouteMarginCatalogMobile } from './route-margin-catalog-mobile'
import { RouteMarginCatalogSummary } from './route-margin-catalog-summary'
import { SupplierCostCatalogPagination } from './supplier-cost-catalog-pagination'

function resolveUpdater<T>(updater: Updater<T>, previous: T): T {
  return typeof updater === 'function'
    ? (updater as (value: T) => T)(previous)
    : updater
}

export function RouteMarginCatalog(props: {
  enabled: boolean
  search: CostAccountingSearch
  onSearchChange: (search: CostAccountingSearch) => void
}) {
  const { t } = useTranslation()
  const params = useMemo(
    () => routeMarginParamsFromSearch(props.search),
    [props.search]
  )
  const catalogQuery = useQuery({
    queryKey: costAccountingQueryKeys.routeMarginCatalog(params),
    queryFn: () => getRouteMarginCatalog(params),
    enabled: props.enabled,
    placeholderData: (previous) => previous,
  })
  const page = catalogQuery.data?.data
  const columns = useRouteMarginCatalogColumns()
  const pagination = useMemo<PaginationState>(
    () => ({ pageIndex: params.page - 1, pageSize: params.page_size }),
    [params.page, params.page_size]
  )
  const sorting = useMemo<SortingState>(
    () => [{ id: params.sort_by, desc: params.sort_order === 'desc' }],
    [params.sort_by, params.sort_order]
  )
  const handlePaginationChange = useCallback(
    (updater: Updater<PaginationState>) => {
      const next = resolveUpdater(updater, pagination)
      props.onSearchChange(
        updateRouteMarginSearch(props.search, {
          marginPage: next.pageIndex + 1,
          marginPageSize: next.pageSize as 25 | 50 | 100,
        })
      )
    },
    [pagination, props]
  )
  const handleSortingChange = useCallback(
    (updater: Updater<SortingState>) => {
      const next = resolveUpdater(updater, sorting)[0]
      if (!next) return
      props.onSearchChange(
        updateRouteMarginSearch(props.search, {
          marginSort: next.id as RouteMarginCatalogSort,
          marginOrder: next.desc ? 'desc' : 'asc',
        })
      )
    },
    [props, sorting]
  )
  const ensurePageInRange = useCallback(
    (pageCount: number) => {
      if (pageCount > 0 && pagination.pageIndex >= pageCount) {
        props.onSearchChange({ ...props.search, marginPage: pageCount })
      }
    },
    [pagination.pageIndex, props]
  )
  const { table } = useDataTable({
    data: page?.items ?? [],
    columns,
    totalCount: page?.total ?? 0,
    pagination,
    sorting,
    onPaginationChange: handlePaginationChange,
    onSortingChange: handleSortingChange,
    manualPagination: true,
    manualSorting: true,
    manualFiltering: true,
    getRowId: (row) => `${row.target_id}-${row.resolution}-${row.scenario}`,
    ensurePageInRange,
  })
  const exportMutation = useMutation({
    mutationFn: async () => {
      const { page: _page, page_size: _pageSize, ...filteredParams } = params
      void _page
      void _pageSize
      return exportRouteMarginCatalog(filteredParams)
    },
    onSuccess: (result) => {
      downloadCostCatalogExport(result, 'route-margin-catalog.csv')
      toast.success(
        t('Export completed: {{count}} rows', { count: result.rowCount })
      )
    },
    onError: () => toast.error(t('Export failed')),
  })

  if (catalogQuery.error && !page) {
    return (
      <Empty className='min-h-80 rounded-lg border'>
        <EmptyHeader>
          <EmptyMedia variant='icon'>
            <HugeiconsIcon icon={Alert02Icon} strokeWidth={2} />
          </EmptyMedia>
          <EmptyTitle>{t('Failed to load route margins')}</EmptyTitle>
          <EmptyDescription>{catalogQuery.error.message}</EmptyDescription>
        </EmptyHeader>
        <EmptyContent>
          <Button
            type='button'
            variant='outline'
            onClick={() => void catalogQuery.refetch()}
          >
            <HugeiconsIcon
              icon={RefreshIcon}
              data-icon='inline-start'
              strokeWidth={2}
            />
            {t('Retry')}
          </Button>
        </EmptyContent>
      </Empty>
    )
  }

  const hasFilters = Boolean(
    params.channel_id ||
    params.model ||
    params.upstream_model ||
    params.route_target ||
    params.resolution ||
    params.status !== 'all' ||
    params.scenario !== 'all' ||
    params.min_margin_ppm !== 300_000 ||
    params.duration_seconds !== 4 ||
    params.group_ratio !== 1
  )
  return (
    <DataTablePage
      table={table}
      columns={columns}
      className='max-sm:block max-sm:overflow-y-auto'
      isLoading={catalogQuery.isLoading}
      isFetching={catalogQuery.isFetching}
      toolbar={
        <RouteMarginCatalogFilters
          search={props.search}
          facets={page?.facets}
          onSearchChange={props.onSearchChange}
          onRefresh={() => void catalogQuery.refetch()}
          onExport={() => exportMutation.mutate()}
          exporting={exportMutation.isPending}
        />
      }
      beforeTable={
        <RouteMarginCatalogSummary
          summary={page?.summary}
          loading={catalogQuery.isLoading}
        />
      }
      emptyTitle={
        hasFilters ? t('No matching route margins') : t('No route targets')
      }
      emptyDescription={t('Adjust the current route margin filters.')}
      mobile={
        page && page.items.length > 0 ? (
          <RouteMarginCatalogMobile items={page.items} />
        ) : undefined
      }
      renderRow={(row, helpers) => (
        <DataTableRow
          key={row.id}
          row={row}
          getColumnClassName={(columnID) => helpers.getCellClassName(columnID)}
        />
      )}
      pinnedColumns={[
        { columnId: 'target_name', side: 'left' },
        { columnId: 'channel_name', side: 'left' },
      ]}
      applyHeaderSize
      showPagination={false}
      afterTable={
        <SupplierCostCatalogPagination
          page={params.page}
          pageSize={params.page_size}
          total={page?.total ?? 0}
          onPageChange={(marginPage) =>
            props.onSearchChange({ ...props.search, marginPage })
          }
          onPageSizeChange={(marginPageSize) =>
            props.onSearchChange(
              updateRouteMarginSearch(props.search, { marginPageSize })
            )
          }
        />
      }
      tableClassName='min-w-[1780px]'
      skeletonKeyPrefix='route-margin-catalog'
    />
  )
}
