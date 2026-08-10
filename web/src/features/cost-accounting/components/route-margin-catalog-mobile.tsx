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
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'

import {
  formatRouteMarginPercent,
  formatRouteMarginUSD,
  routeMarginFailureReasonLabel,
  routeMarginScenarioLabel,
} from '../lib/route-margin-catalog'
import type { RouteMarginCatalogItem } from '../types'

export function RouteMarginCatalogMobile(props: {
  items: RouteMarginCatalogItem[]
}) {
  const { t } = useTranslation()
  return (
    <div className='divide-y overflow-hidden border-y'>
      {props.items.map((item) => (
        <article
          key={`${item.target_id}-${item.resolution}-${item.scenario}`}
          className='min-w-0 px-1 py-3'
        >
          <div className='flex min-w-0 items-start justify-between gap-3'>
            <div className='min-w-0'>
              <h3 className='text-sm font-medium break-words'>
                {item.target_name}
              </h3>
              <p className='text-muted-foreground mt-1 font-mono text-xs break-all'>
                {item.channel_name || t('Unknown channel')} ·{' '}
                {item.upstream_model}
              </p>
            </div>
            <Badge variant={item.eligible ? 'default' : 'destructive'}>
              {item.eligible ? t('Eligible') : t('Ineligible')}
            </Badge>
          </div>
          <div className='mt-3 grid grid-cols-2 gap-x-3 gap-y-2 text-xs'>
            <span className='text-muted-foreground'>{t('Scenario')}</span>
            <span className='text-right'>
              {item.resolution} · {routeMarginScenarioLabel(item.scenario, t)}
            </span>
            <span className='text-muted-foreground'>
              {t('Estimated profit')}
            </span>
            <span className='text-right font-mono tabular-nums'>
              {formatRouteMarginUSD(item.estimated_profit_nano_usd) || '—'}
            </span>
            <span className='text-muted-foreground'>{t('Gross margin')}</span>
            <span className='text-right font-mono font-semibold tabular-nums'>
              {formatRouteMarginPercent(item.gross_margin_ppm) || '—'}
            </span>
            {!item.eligible ? (
              <>
                <span className='text-muted-foreground'>
                  {t('Failure reason')}
                </span>
                <span className='text-right break-words'>
                  {routeMarginFailureReasonLabel(item.failure_reason, t)}
                </span>
              </>
            ) : null}
          </div>
        </article>
      ))}
    </div>
  )
}
