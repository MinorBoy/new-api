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
import { ArrowDown01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'

import {
  catalogBillingSemantics,
  catalogCostModeLabel,
  catalogStatusLabel,
  formatCatalogItemPrices,
  formatCatalogTimestamp,
} from '../lib/catalog'
import type { CostCatalogItem } from '../types'

export function SupplierCostCatalogMobile(props: {
  items: CostCatalogItem[]
  onOpenRule: (ruleId: number, trigger: HTMLElement) => void
}) {
  const { t } = useTranslation()
  return (
    <div className='divide-y overflow-hidden rounded-lg border'>
      {props.items.map((item) => (
        <Collapsible key={item.rule_id}>
          <div className='flex items-start gap-2 p-3'>
            <button
              type='button'
              className='focus-visible:ring-ring min-w-0 flex-1 text-left outline-none focus-visible:ring-2'
              onClick={(event) =>
                props.onOpenRule(item.rule_id, event.currentTarget)
              }
            >
              <span className='flex items-start justify-between gap-2'>
                <span className='min-w-0'>
                  <span className='block truncate font-medium'>
                    {item.channel_name || t('Unknown channel')} · #
                    {item.channel_id}
                  </span>
                  <span className='text-muted-foreground mt-1 block truncate font-mono text-xs'>
                    {item.billable_upstream_model}
                  </span>
                </span>
                <Badge
                  variant={item.status === 'active' ? 'default' : 'outline'}
                >
                  {catalogStatusLabel(item.status, t)}
                </Badge>
              </span>
              <span className='mt-2 flex items-center justify-between gap-2 text-xs'>
                <span>{catalogCostModeLabel(item.cost_mode, t)}</span>
                <span className='truncate font-mono'>
                  {formatCatalogItemPrices(item, true, t) || '—'}
                </span>
              </span>
            </button>
            <CollapsibleTrigger
              render={<Button type='button' variant='ghost' size='icon-sm' />}
              aria-label={t('Show supplier cost metadata')}
            >
              <HugeiconsIcon icon={ArrowDown01Icon} strokeWidth={2} />
            </CollapsibleTrigger>
          </div>
          <CollapsibleContent className='bg-muted/20 border-t px-3 py-3 text-xs'>
            <dl className='grid grid-cols-[auto_1fr] gap-x-4 gap-y-2'>
              <dt className='text-muted-foreground'>{t('Cost variant')}</dt>
              <dd className='text-right font-mono'>{item.cost_variant_key}</dd>
              <dt className='text-muted-foreground'>{t('Native price')}</dt>
              <dd className='text-right font-mono'>
                {formatCatalogItemPrices(item, false, t) || '—'}
              </dd>
              <dt className='text-muted-foreground'>
                {t('Billing semantics')}
              </dt>
              <dd className='text-right'>{catalogBillingSemantics(item, t)}</dd>
              <dt className='text-muted-foreground'>{t('Source')}</dt>
              <dd className='text-right font-mono'>{item.source || '—'}</dd>
              <dt className='text-muted-foreground'>{t('Effective period')}</dt>
              <dd className='text-right'>
                {formatCatalogTimestamp(item.effective_from) || '—'} –{' '}
                {formatCatalogTimestamp(item.effective_to) || t('Current')}
              </dd>
            </dl>
          </CollapsibleContent>
        </Collapsible>
      ))}
    </div>
  )
}
