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
import { AlertTriangle, RefreshCw } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import {
  SideDrawerSection,
  SideDrawerSectionHeader,
  sideDrawerContentClassName,
  sideDrawerFormClassName,
  sideDrawerHeaderClassName,
} from '@/components/drawer-layout'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Skeleton } from '@/components/ui/skeleton'

import { costAccountingQueryKeys, getSupplierCostCatalogDetail } from '../api'
import {
  catalogBillingSemantics,
  catalogCostModeLabel,
  catalogStatusLabel,
  formatCatalogItemPrices,
  formatCatalogTimestamp,
} from '../lib/catalog'

export function SupplierCostDetailDrawer(props: {
  ruleId: number | null
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation()
  const detailQuery = useQuery({
    queryKey: costAccountingQueryKeys.catalogDetail(props.ruleId ?? 0),
    queryFn: () => getSupplierCostCatalogDetail(props.ruleId ?? 0),
    enabled: props.ruleId !== null,
  })
  const detail = detailQuery.data?.data
  const rule = detail?.rule
  const description = rule
    ? `${rule.channel_name || t('Unknown channel')} · ${rule.billable_upstream_model}`
    : t('Review frozen supplier pricing and rule history.')

  return (
    <Sheet open={props.ruleId !== null} onOpenChange={props.onOpenChange}>
      <SheetContent className={sideDrawerContentClassName('sm:max-w-3xl')}>
        <SheetHeader className={sideDrawerHeaderClassName()}>
          <SheetTitle>{t('Supplier cost rule details')}</SheetTitle>
          <SheetDescription>{description}</SheetDescription>
        </SheetHeader>
        <div className={sideDrawerFormClassName()}>
          {detailQuery.isLoading
            ? Array.from({ length: 5 }, (_, index) => (
                <Skeleton
                  key={`supplier-detail-${index}`}
                  className='h-24 w-full'
                />
              ))
            : null}
          {!detailQuery.isLoading && detailQuery.error ? (
            <Alert variant='destructive'>
              <AlertTriangle aria-hidden='true' />
              <AlertTitle>{t('Failed to load supplier costs')}</AlertTitle>
              <AlertDescription>
                <Button
                  type='button'
                  variant='outline'
                  size='sm'
                  className='mt-2'
                  onClick={() => void detailQuery.refetch()}
                >
                  <RefreshCw data-icon='inline-start' aria-hidden='true' />
                  {t('Retry')}
                </Button>
              </AlertDescription>
            </Alert>
          ) : null}
          {!detailQuery.isLoading && !detailQuery.error && rule ? (
            <>
              <SideDrawerSection>
                <SideDrawerSectionHeader title={t('Price components')} />
                <DefinitionGrid
                  rows={[
                    [
                      t('Native price'),
                      formatCatalogItemPrices(rule, false, t) || '—',
                    ],
                    [
                      t('Normalized USD price'),
                      formatCatalogItemPrices(rule, true, t) || '—',
                    ],
                    [
                      t('15-second equivalent USD/second (comparison only)'),
                      rule.comparison_15s_equivalent_usd_per_second
                        ? `USD ${rule.comparison_15s_equivalent_usd_per_second}`
                        : '—',
                    ],
                    [t('Price status'), t(rule.price_status)],
                    [t('Issues'), rule.issues.join(', ') || '—'],
                  ]}
                />
              </SideDrawerSection>
              <SideDrawerSection>
                <SideDrawerSectionHeader title={t('Conversion parameters')} />
                <DefinitionGrid
                  rows={[
                    [t('Currency'), detail.config?.currency || '—'],
                    [
                      t('Billing multiplier'),
                      detail.config?.billing_multiplier || '—',
                    ],
                    [
                      t('Purchase discount ratio'),
                      detail.config?.purchase_discount_ratio || '—',
                    ],
                    [
                      t('Recharge exchange ratio'),
                      detail.config?.recharge_exchange_ratio || '—',
                    ],
                    [t('Fee rate'), detail.config?.fee_rate || '—'],
                    [
                      t('Currency to USD rate'),
                      detail.config?.currency_to_usd_rate || '—',
                    ],
                  ]}
                />
              </SideDrawerSection>
              <SideDrawerSection>
                <SideDrawerSectionHeader title={t('Billing semantics')} />
                <DefinitionGrid
                  rows={[
                    [t('Cost mode'), catalogCostModeLabel(rule.cost_mode, t)],
                    [t('Billing semantics'), catalogBillingSemantics(rule, t)],
                    [
                      t('Free cost reason'),
                      detail.config?.zero_cost_reason || '—',
                    ],
                  ]}
                />
              </SideDrawerSection>
              <SideDrawerSection>
                <SideDrawerSectionHeader title={t('Metadata')} />
                <DefinitionGrid
                  rows={[
                    [t('Rule ID'), String(rule.rule_id)],
                    [t('Cost variant'), rule.cost_variant_key],
                    [t('Source'), rule.source || '—'],
                    [t('Note'), rule.note || '—'],
                    [
                      t('Effective from'),
                      formatCatalogTimestamp(rule.effective_from) || '—',
                    ],
                    [
                      t('Effective to'),
                      formatCatalogTimestamp(rule.effective_to) || t('Current'),
                    ],
                  ]}
                />
              </SideDrawerSection>
              <SideDrawerSection>
                <SideDrawerSectionHeader title={t('Rule history')} />
                {detail.history.length === 0 ? (
                  <p className='text-muted-foreground text-sm'>
                    {t('No rule history')}
                  </p>
                ) : (
                  <div className='divide-y rounded-lg border'>
                    {detail.history.map((entry) => (
                      <div
                        key={entry.rule_id}
                        className='flex items-center justify-between gap-3 px-3 py-2.5'
                      >
                        <span>
                          <span className='font-medium'>v{entry.version}</span>
                          <span className='text-muted-foreground ml-2 text-xs'>
                            {formatCatalogTimestamp(entry.updated_at)}
                          </span>
                        </span>
                        <Badge
                          variant={
                            entry.status === 'active' ? 'default' : 'outline'
                          }
                        >
                          {catalogStatusLabel(entry.status, t)}
                        </Badge>
                      </div>
                    ))}
                  </div>
                )}
              </SideDrawerSection>
            </>
          ) : null}
        </div>
      </SheetContent>
    </Sheet>
  )
}

function DefinitionGrid(props: { rows: Array<[string, string]> }) {
  return (
    <dl className='grid gap-x-6 gap-y-3 sm:grid-cols-2'>
      {props.rows.map(([label, value]) => (
        <div key={label} className='min-w-0'>
          <dt className='text-muted-foreground text-xs'>{label}</dt>
          <dd className='mt-1 font-mono text-sm break-words'>{value}</dd>
        </div>
      ))}
    </dl>
  )
}
