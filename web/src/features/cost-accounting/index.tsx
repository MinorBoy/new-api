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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { getRouteApi } from '@tanstack/react-router'
import { AlertTriangle, ChartNoAxesCombined } from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { SectionPageLayout } from '@/components/layout'
import { Badge } from '@/components/ui/badge'
import { Field, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import {
  ADMIN_PERMISSION_ACTIONS,
  ADMIN_PERMISSION_RESOURCES,
  hasPermission,
} from '@/lib/admin-permissions'
import {
  formatMarginBPSPercent,
  marginPercentInputToBPS,
} from '@/lib/margin-bps'
import { useAuthStore } from '@/stores/auth-store'

import {
  costAccountingQueryKeys,
  getCostAccountingSettings,
  getCostCoverage,
  getCostReportBreakdown,
  getCostReportSummary,
  updateCostAccountingSettings,
} from './api'
import { AnomalyQueue } from './components/anomaly-queue'
import { CostAccountingModeToggle } from './components/cost-accounting-mode-toggle'
import { ProfitFilters } from './components/profit-filters'
import { ProfitSummary } from './components/profit-summary'
import { ProfitTable } from './components/profit-table'
import { RouteMarginCatalog } from './components/route-margin-catalog'
import { SupplierCostCatalog } from './components/supplier-cost-catalog'
import { useProfitFilterOptions } from './hooks/use-profit-filter-options'
import {
  COST_ACCOUNTING_TABS,
  costReportParamsFromSearch,
  canEnableStrictCostAccounting,
  isCostCatalogTab,
  updateCostAccountingTab,
  type CostAccountingSearch,
} from './lib/report'

const route = getRouteApi('/_authenticated/cost-accounting/')

export function CostAccounting() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const currentUser = useAuthStore((state) => state.auth.user)
  const search = route.useSearch()
  const navigate = route.useNavigate()
  const tab = search.tab ?? 'profit'
  const reportParams = useMemo(
    () => costReportParamsFromSearch(search),
    [search]
  )
  const settingsQuery = useQuery({
    queryKey: costAccountingQueryKeys.settings(),
    queryFn: getCostAccountingSettings,
  })
  const coverageQuery = useQuery({
    queryKey: costAccountingQueryKeys.coverage(),
    queryFn: () => getCostCoverage(),
  })
  const summaryQuery = useQuery({
    queryKey: costAccountingQueryKeys.reportSummary(reportParams),
    queryFn: () => getCostReportSummary(reportParams),
    enabled: tab === 'profit',
  })
  const breakdownQuery = useQuery({
    queryKey: costAccountingQueryKeys.reportBreakdown(reportParams),
    queryFn: () => getCostReportBreakdown(reportParams),
    enabled: tab === 'profit',
  })
  const profitFilterOptions = useProfitFilterOptions(search, tab === 'profit')

  const updateSearch = (next: CostAccountingSearch) => {
    void navigate({ search: next, replace: true })
  }
  const coverage = coverageQuery.data?.data ?? []
  const uncovered = coverage.filter((item) => !item.covered).length
  const covered = coverage.length - uncovered
  const mode = settingsQuery.data?.data.mode ?? 'disabled'
  const minimumExpectedMarginBPS =
    settingsQuery.data?.data.minimum_expected_margin_bps ?? 0
  const canWrite = hasPermission(
    currentUser,
    ADMIN_PERMISSION_RESOURCES.COST_ACCOUNTING,
    ADMIN_PERMISSION_ACTIONS.WRITE
  )
  const canEnableStrict =
    !coverageQuery.isLoading &&
    !coverageQuery.error &&
    canEnableStrictCostAccounting(coverage)
  let modeLabel = t('Disabled')
  if (mode === 'strict') {
    modeLabel = t('Strict')
  } else if (mode === 'tracking') {
    modeLabel = t('Tracking')
  }
  const modeMutation = useMutation({
    mutationFn: updateCostAccountingSettings,
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({
          queryKey: costAccountingQueryKeys.settings(),
        }),
        queryClient.invalidateQueries({
          queryKey: costAccountingQueryKeys.coverages(),
        }),
      ])
      toast.success(t('Settings updated successfully'))
    },
    onError: () => toast.error(t('Failed to update settings')),
  })

  return (
    <SectionPageLayout fixedContent>
      <SectionPageLayout.Title>
        <span className='flex min-w-0 items-center gap-2'>
          <ChartNoAxesCombined className='size-4 shrink-0' aria-hidden='true' />
          <span className='truncate'>{t('Cost accounting')}</span>
          <Badge variant={mode === 'disabled' ? 'outline' : 'default'}>
            {modeLabel}
          </Badge>
          {!coverageQuery.isLoading ? (
            <Badge variant={uncovered === 0 ? 'outline' : 'destructive'}>
              {uncovered === 0
                ? t('{{count}} covered', { count: covered })
                : t('{{count}} uncovered', { count: uncovered })}
            </Badge>
          ) : null}
          {settingsQuery.error || coverageQuery.error ? (
            <AlertTriangle
              className='text-destructive size-4'
              aria-hidden='true'
            />
          ) : null}
        </span>
      </SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        {canWrite ? (
          <div className='flex flex-wrap items-center justify-end gap-3'>
            <Field className='w-52 gap-1'>
              <FieldLabel htmlFor='cost-minimum-margin' className='text-xs'>
                {t('Minimum expected gross margin')}
              </FieldLabel>
              <div className='flex items-center gap-1.5'>
                <Input
                  key={minimumExpectedMarginBPS}
                  id='cost-minimum-margin'
                  type='number'
                  min={0}
                  max={100}
                  step={0.01}
                  defaultValue={formatMarginBPSPercent(
                    minimumExpectedMarginBPS
                  )}
                  disabled={modeMutation.isPending || settingsQuery.isLoading}
                  aria-label={t('Minimum expected gross margin')}
                  onBlur={(event) => {
                    let nextMargin: number
                    try {
                      nextMargin = marginPercentInputToBPS(event.target.value)
                    } catch {
                      toast.error(
                        t(
                          'Enter a percentage from 0 to 100 with at most two decimals'
                        )
                      )
                      event.target.value = formatMarginBPSPercent(
                        minimumExpectedMarginBPS
                      )
                      return
                    }
                    if (nextMargin === minimumExpectedMarginBPS) {
                      return
                    }
                    modeMutation.mutate({
                      mode,
                      minimum_expected_margin_bps: nextMargin,
                    })
                  }}
                  onKeyDown={(event) => {
                    if (event.key === 'Enter') {
                      event.currentTarget.blur()
                    }
                  }}
                />
                <span className='text-muted-foreground text-sm'>%</span>
              </div>
            </Field>
            <CostAccountingModeToggle
              mode={mode}
              canEnableStrict={canEnableStrict}
              disabled={modeMutation.isPending || settingsQuery.isLoading}
              onChange={(value) => {
                if (value === 'strict' && !canEnableStrict) {
                  toast.error(
                    t('Resolve uncovered models before enabling strict mode')
                  )
                  return
                }
                modeMutation.mutate({
                  mode: value,
                  minimum_expected_margin_bps: minimumExpectedMarginBPS,
                })
              }}
            />
          </div>
        ) : null}
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <Tabs
          value={tab}
          onValueChange={(value) => {
            if ((COST_ACCOUNTING_TABS as readonly string[]).includes(value)) {
              updateSearch(
                updateCostAccountingTab(
                  search,
                  value as (typeof COST_ACCOUNTING_TABS)[number]
                )
              )
            }
          }}
          className='h-full min-h-0'
        >
          <TabsList variant='line' className='shrink-0'>
            <TabsTrigger value='profit'>{t('Profit report')}</TabsTrigger>
            <TabsTrigger value='catalog'>
              {t('Supplier cost catalog')}
            </TabsTrigger>
            <TabsTrigger value='route-margin'>{t('Route margin')}</TabsTrigger>
            <TabsTrigger value='anomalies'>{t('Anomalies')}</TabsTrigger>
          </TabsList>
          <TabsContent
            value='profit'
            className='min-h-0 space-y-4 overflow-auto pr-1 pb-2'
          >
            <ProfitFilters
              search={search}
              onChange={updateSearch}
              filterOptions={profitFilterOptions}
            />
            <ProfitSummary
              summary={summaryQuery.data?.data}
              loading={summaryQuery.isLoading}
              error={summaryQuery.error}
              onRetry={() => void summaryQuery.refetch()}
            />
            <ProfitTable
              rows={breakdownQuery.data?.data ?? []}
              loading={breakdownQuery.isLoading}
              error={breakdownQuery.error}
              onRetry={() => void breakdownQuery.refetch()}
            />
          </TabsContent>
          <TabsContent
            value='route-margin'
            className='min-h-0 overflow-hidden pr-1 pb-2'
          >
            <RouteMarginCatalog
              enabled={tab === 'route-margin'}
              search={search}
              onSearchChange={updateSearch}
            />
          </TabsContent>
          <TabsContent
            value='catalog'
            className='min-h-0 overflow-hidden pr-1 pb-2'
          >
            <SupplierCostCatalog
              enabled={isCostCatalogTab(tab)}
              search={search}
              onSearchChange={updateSearch}
            />
          </TabsContent>
          <TabsContent
            value='anomalies'
            className='min-h-0 overflow-auto pr-1 pb-2'
          >
            <AnomalyQueue enabled={tab === 'anomalies'} />
          </TabsContent>
        </Tabs>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
