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
import type { ColumnDef } from '@tanstack/react-table'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { DataTableColumnHeader } from '@/components/data-table'
import { Badge } from '@/components/ui/badge'

import {
  formatRouteMarginPercent,
  formatRouteMarginUSD,
  routeMarginFailureReasonLabel,
  routeMarginRevenueSourceLabel,
  routeMarginScenarioLabel,
} from '../lib/route-margin-catalog'
import type { RouteMarginCatalogItem } from '../types'

export function useRouteMarginCatalogColumns(): ColumnDef<
  RouteMarginCatalogItem,
  unknown
>[] {
  const { t } = useTranslation()
  return useMemo(
    () => [
      {
        id: 'target_name',
        accessorKey: 'target_name',
        size: 220,
        minSize: 220,
        header: ({ column }) => (
          <DataTableColumnHeader column={column} title={t('Route target')} />
        ),
        cell: ({ row }) => (
          <div className='min-w-0 leading-tight'>
            <p className='font-medium break-words'>
              {row.original.target_name}
            </p>
            <p className='text-muted-foreground mt-1 font-mono text-xs'>
              #{row.original.target_id} · {row.original.group_name}
            </p>
          </div>
        ),
      },
      {
        id: 'channel_name',
        accessorKey: 'channel_name',
        size: 205,
        minSize: 205,
        header: ({ column }) => (
          <DataTableColumnHeader column={column} title={t('Channel')} />
        ),
        cell: ({ row }) => (
          <div className='min-w-0 leading-tight'>
            <p className='font-medium break-words'>
              {row.original.channel_name || t('Unknown channel')}
            </p>
            <p className='text-muted-foreground mt-1 font-mono text-xs'>
              #{row.original.channel_id}
            </p>
          </div>
        ),
      },
      {
        id: 'upstream_model',
        accessorKey: 'upstream_model',
        size: 245,
        minSize: 245,
        header: ({ column }) => (
          <DataTableColumnHeader column={column} title={t('Upstream model')} />
        ),
        cell: ({ row }) => (
          <div className='min-w-0 leading-tight'>
            <p className='font-mono text-xs break-all'>
              {row.original.upstream_model}
            </p>
            <p className='text-muted-foreground mt-1 font-mono text-xs break-all'>
              {row.original.canonical_model}
            </p>
          </div>
        ),
      },
      {
        id: 'scenario',
        size: 145,
        enableSorting: false,
        header: t('Scenario'),
        cell: ({ row }) => (
          <div className='leading-tight'>
            <p>
              {row.original.resolution} · {row.original.duration_seconds}s
            </p>
            <p className='text-muted-foreground mt-1 text-xs'>
              {routeMarginScenarioLabel(row.original.scenario, t)}
            </p>
          </div>
        ),
      },
      {
        id: 'estimated_revenue_nano_usd',
        size: 145,
        enableSorting: false,
        header: t('Estimated revenue'),
        cell: ({ row }) => (
          <span className='font-mono text-xs tabular-nums'>
            {formatRouteMarginUSD(row.original.estimated_revenue_nano_usd) ||
              '—'}
          </span>
        ),
      },
      {
        id: 'estimated_cost_nano_usd',
        size: 145,
        enableSorting: false,
        header: t('Estimated cost'),
        cell: ({ row }) => (
          <span className='font-mono text-xs tabular-nums'>
            {formatRouteMarginUSD(row.original.estimated_cost_nano_usd) || '—'}
          </span>
        ),
      },
      {
        id: 'estimated_profit_nano_usd',
        accessorKey: 'estimated_profit_nano_usd',
        size: 145,
        header: ({ column }) => (
          <DataTableColumnHeader
            column={column}
            title={t('Estimated profit')}
          />
        ),
        cell: ({ row }) => (
          <span className='font-mono text-xs tabular-nums'>
            {formatRouteMarginUSD(row.original.estimated_profit_nano_usd) ||
              '—'}
          </span>
        ),
      },
      {
        id: 'gross_margin_ppm',
        accessorKey: 'gross_margin_ppm',
        size: 125,
        header: ({ column }) => (
          <DataTableColumnHeader column={column} title={t('Gross margin')} />
        ),
        cell: ({ row }) => (
          <span className='font-mono font-semibold tabular-nums'>
            {formatRouteMarginPercent(row.original.gross_margin_ppm) || '—'}
          </span>
        ),
      },
      {
        id: 'status',
        size: 190,
        enableSorting: false,
        header: t('Status'),
        cell: ({ row }) => (
          <div className='flex min-w-0 flex-col items-start gap-1'>
            <Badge variant={row.original.eligible ? 'default' : 'destructive'}>
              {row.original.eligible ? t('Eligible') : t('Ineligible')}
            </Badge>
            {!row.original.eligible ? (
              <span className='text-muted-foreground text-xs break-words'>
                {routeMarginFailureReasonLabel(row.original.failure_reason, t)}
              </span>
            ) : null}
          </div>
        ),
      },
      {
        id: 'source',
        size: 175,
        enableSorting: false,
        header: t('Rule source'),
        cell: ({ row }) => (
          <div className='min-w-0 font-mono text-xs leading-tight'>
            <p className='break-all'>{row.original.cost_source || '—'}</p>
            <p className='text-muted-foreground mt-1 break-all'>
              {routeMarginRevenueSourceLabel(row.original.revenue_source, t)}
            </p>
          </div>
        ),
      },
    ],
    [t]
  )
}
