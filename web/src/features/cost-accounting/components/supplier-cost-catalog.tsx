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
import { useMutation, useQuery } from '@tanstack/react-query'
import type {
  PaginationState,
  SortingState,
  Updater,
} from '@tanstack/react-table'
import { AlertTriangle, RefreshCw } from 'lucide-react'
import { useCallback, useMemo, useRef, useState } from 'react'
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
  exportSupplierCostCatalog,
  getSupplierCostCatalog,
} from '../api'
import {
  costCatalogParamsFromSearch,
  updateCatalogSearch,
} from '../lib/catalog'
import { downloadCostCatalogExport } from '../lib/catalog-export'
import type { CostAccountingSearch } from '../lib/report'
import type { CostCatalogSort } from '../types'
import { useSupplierCostCatalogColumns } from './supplier-cost-catalog-columns'
import { SupplierCostCatalogFilters } from './supplier-cost-catalog-filters'
import { SupplierCostCatalogMobile } from './supplier-cost-catalog-mobile'
import { SupplierCostCatalogPagination } from './supplier-cost-catalog-pagination'
import { SupplierCostCatalogSummary } from './supplier-cost-catalog-summary'
import { SupplierCostDetailDrawer } from './supplier-cost-detail-drawer'

function resolveUpdater<T>(updater: Updater<T>, previous: T): T {
  return typeof updater === 'function'
    ? (updater as (value: T) => T)(previous)
    : updater
}

export function SupplierCostCatalog(props: {
  enabled: boolean
  search: CostAccountingSearch
  onSearchChange: (search: CostAccountingSearch) => void
}) {
  const { t } = useTranslation()
  const params = useMemo(
    () => costCatalogParamsFromSearch(props.search),
    [props.search]
  )
  const catalogQuery = useQuery({
    queryKey: costAccountingQueryKeys.catalog(params),
    queryFn: () => getSupplierCostCatalog(params),
    enabled: props.enabled,
    placeholderData: (previous) => previous,
  })
  const page = catalogQuery.data?.data
  const columns = useSupplierCostCatalogColumns()
  const pagination = useMemo<PaginationState>(
    () => ({
      pageIndex: (params.page ?? 1) - 1,
      pageSize: params.page_size ?? 50,
    }),
    [params.page, params.page_size]
  )
  const sorting = useMemo<SortingState>(
    () => [
      {
        id: params.sort_by ?? 'channel_name',
        desc: params.sort_order === 'desc',
      },
    ],
    [params.sort_by, params.sort_order]
  )
  const handlePaginationChange = useCallback(
    (updater: Updater<PaginationState>) => {
      const next = resolveUpdater(updater, pagination)
      props.onSearchChange(
        updateCatalogSearch(props.search, {
          catalogPage: next.pageIndex + 1,
          catalogPageSize: next.pageSize as 25 | 50 | 100,
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
        updateCatalogSearch(props.search, {
          catalogSort: next.id as CostCatalogSort,
          catalogOrder: next.desc ? 'desc' : 'asc',
        })
      )
    },
    [props, sorting]
  )
  const ensurePageInRange = useCallback(
    (pageCount: number) => {
      if (pageCount > 0 && pagination.pageIndex >= pageCount) {
        props.onSearchChange({ ...props.search, catalogPage: pageCount })
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
    getRowId: (row) => String(row.rule_id),
    ensurePageInRange,
  })

  const [selectedRuleID, setSelectedRuleID] = useState<number | null>(null)
  const detailTrigger = useRef<HTMLElement | null>(null)
  const openRule = useCallback((ruleID: number, trigger: HTMLElement) => {
    detailTrigger.current = trigger
    setSelectedRuleID(ruleID)
  }, [])
  const closeDetail = useCallback(() => {
    setSelectedRuleID(null)
    queueMicrotask(() => detailTrigger.current?.focus())
  }, [])

  const [exportingScope, setExportingScope] = useState<
    'filtered' | 'all' | null
  >(null)
  const exportMutation = useMutation({
    mutationFn: async (scope: 'filtered' | 'all') => {
      setExportingScope(scope)
      const { page: _page, page_size: _pageSize, ...filteredParams } = params
      void _page
      void _pageSize
      const exportParams =
        scope === 'all'
          ? { sort_by: params.sort_by, sort_order: params.sort_order }
          : filteredParams
      return exportSupplierCostCatalog(scope, exportParams)
    },
    onSuccess: (result) => {
      downloadCostCatalogExport(result)
      toast.success(
        t('Export completed: {{count}} rows', { count: result.rowCount })
      )
    },
    onError: () => toast.error(t('Export failed')),
    onSettled: () => setExportingScope(null),
  })

  if (catalogQuery.error && !page) {
    return (
      <Empty className='min-h-80 rounded-lg border'>
        <EmptyHeader>
          <EmptyMedia variant='icon'>
            <AlertTriangle aria-hidden='true' />
          </EmptyMedia>
          <EmptyTitle>{t('Failed to load supplier costs')}</EmptyTitle>
          <EmptyDescription>{catalogQuery.error.message}</EmptyDescription>
        </EmptyHeader>
        <EmptyContent>
          <Button
            type='button'
            variant='outline'
            onClick={() => void catalogQuery.refetch()}
          >
            <RefreshCw data-icon='inline-start' aria-hidden='true' />
            {t('Retry')}
          </Button>
        </EmptyContent>
      </Empty>
    )
  }

  const hasFilters = Boolean(
    params.channel_id ||
    params.billable_upstream_model ||
    params.cost_mode ||
    params.currency ||
    params.source ||
    params.status !== 'active'
  )
  return (
    <>
      <DataTablePage
        table={table}
        columns={columns}
        className='max-sm:block max-sm:overflow-y-auto'
        isLoading={catalogQuery.isLoading}
        isFetching={catalogQuery.isFetching}
        toolbar={
          <SupplierCostCatalogFilters
            search={props.search}
            facets={page?.facets}
            onSearchChange={props.onSearchChange}
            onRefresh={() => void catalogQuery.refetch()}
            onExport={(scope) => exportMutation.mutate(scope)}
            exportingScope={exportingScope}
          />
        }
        beforeTable={
          <SupplierCostCatalogSummary
            summary={page?.summary}
            loading={catalogQuery.isLoading}
          />
        }
        emptyTitle={
          hasFilters
            ? t('No matching supplier costs')
            : t('No supplier cost rules')
        }
        emptyDescription={
          hasFilters
            ? t('Adjust or reset the current filters.')
            : t('Supplier cost rules will appear here after configuration.')
        }
        mobile={
          page && page.items.length > 0 ? (
            <SupplierCostCatalogMobile
              items={page.items}
              onOpenRule={openRule}
            />
          ) : undefined
        }
        renderRow={(row, helpers) => (
          <DataTableRow
            key={row.id}
            row={row}
            tabIndex={0}
            aria-label={`${row.original.channel_name} ${row.original.billable_upstream_model}`}
            className='focus-visible:ring-ring cursor-pointer outline-none focus-visible:ring-2 focus-visible:ring-inset'
            getColumnClassName={(columnID) =>
              helpers.getCellClassName(columnID)
            }
            onClick={(event) =>
              openRule(row.original.rule_id, event.currentTarget)
            }
            onKeyDown={(event) => {
              if (event.key === 'Enter' || event.key === ' ') {
                event.preventDefault()
                openRule(row.original.rule_id, event.currentTarget)
              }
            }}
          />
        )}
        pinnedColumns={[
          { columnId: 'channel_name', side: 'left' },
          { columnId: 'billable_upstream_model', side: 'left' },
        ]}
        applyHeaderSize
        showPagination={false}
        afterTable={
          <SupplierCostCatalogPagination
            page={params.page ?? 1}
            pageSize={params.page_size ?? 50}
            total={page?.total ?? 0}
            onPageChange={(catalogPage) =>
              props.onSearchChange({ ...props.search, catalogPage })
            }
            onPageSizeChange={(catalogPageSize) =>
              props.onSearchChange(
                updateCatalogSearch(props.search, { catalogPageSize })
              )
            }
          />
        }
        tableClassName='min-w-[1650px]'
        skeletonKeyPrefix='supplier-cost-catalog'
      />
      <SupplierCostDetailDrawer
        ruleId={selectedRuleID}
        onOpenChange={(open) => {
          if (!open) closeDetail()
        }}
      />
    </>
  )
}
