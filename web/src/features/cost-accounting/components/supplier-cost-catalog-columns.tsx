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
import { getChannelTypeLabel } from '@/features/channels/lib/channel-utils'

import {
  catalogBillingSemantics,
  catalogCostModeLabel,
  catalogStatusLabel,
  formatCatalogItemPrices,
  formatCatalogTimestamp,
} from '../lib/catalog'
import type { CostCatalogItem } from '../types'

export function useSupplierCostCatalogColumns(): ColumnDef<
  CostCatalogItem,
  unknown
>[] {
  const { t } = useTranslation()
  return useMemo(
    () => [
      {
        id: 'channel_name',
        accessorKey: 'channel_name',
        size: 190,
        minSize: 190,
        header: ({ column }) => (
          <DataTableColumnHeader column={column} title={t('Channel')} />
        ),
        cell: ({ row }) => (
          <div className='min-w-0 leading-tight'>
            <p className='truncate font-medium'>
              {row.original.channel_name || t('Unknown channel')}
            </p>
            <p className='text-muted-foreground mt-1 truncate text-xs'>
              #{row.original.channel_id} ·{' '}
              {t(getChannelTypeLabel(row.original.channel_type))}
            </p>
          </div>
        ),
      },
      {
        id: 'billable_upstream_model',
        accessorKey: 'billable_upstream_model',
        size: 240,
        minSize: 240,
        header: ({ column }) => (
          <DataTableColumnHeader
            column={column}
            title={t('Billable upstream model')}
          />
        ),
        cell: ({ row }) => (
          <span className='block truncate font-mono text-xs'>
            {row.original.billable_upstream_model}
          </span>
        ),
      },
      {
        id: 'cost_variant_key',
        accessorKey: 'cost_variant_key',
        size: 130,
        header: ({ column }) => (
          <DataTableColumnHeader column={column} title={t('Cost variant')} />
        ),
        cell: ({ row }) => (
          <span className='font-mono text-xs'>
            {row.original.cost_variant_key}
          </span>
        ),
      },
      {
        id: 'status',
        accessorKey: 'status',
        size: 110,
        header: ({ column }) => (
          <DataTableColumnHeader column={column} title={t('Status')} />
        ),
        cell: ({ row }) => (
          <div className='flex items-center gap-1.5'>
            <Badge
              variant={row.original.status === 'active' ? 'default' : 'outline'}
            >
              {catalogStatusLabel(row.original.status, t)}
            </Badge>
            <span className='text-muted-foreground text-xs'>
              v{row.original.version}
            </span>
          </div>
        ),
      },
      {
        id: 'cost_mode',
        accessorKey: 'cost_mode',
        size: 130,
        header: ({ column }) => (
          <DataTableColumnHeader column={column} title={t('Cost mode')} />
        ),
        cell: ({ row }) => catalogCostModeLabel(row.original.cost_mode, t),
      },
      {
        id: 'native_price',
        size: 210,
        enableSorting: false,
        header: t('Native price'),
        cell: ({ row }) => (
          <span className='font-mono text-xs whitespace-normal'>
            {formatCatalogItemPrices(row.original, false, t) || '—'}
          </span>
        ),
      },
      {
        id: 'normalized_price',
        size: 220,
        enableSorting: false,
        header: t('Normalized USD price'),
        cell: ({ row }) => (
          <span className='font-mono text-xs whitespace-normal'>
            {formatCatalogItemPrices(row.original, true, t) || '—'}
          </span>
        ),
      },
      {
        id: 'billing_semantics',
        size: 220,
        enableSorting: false,
        header: t('Billing semantics'),
        cell: ({ row }) => (
          <span className='text-muted-foreground text-xs whitespace-normal'>
            {catalogBillingSemantics(row.original, t)}
          </span>
        ),
      },
      {
        id: 'source',
        accessorKey: 'source',
        size: 135,
        header: ({ column }) => (
          <DataTableColumnHeader column={column} title={t('Source')} />
        ),
        cell: ({ row }) => (
          <span className='font-mono text-xs'>
            {row.original.source || '—'}
          </span>
        ),
      },
      {
        id: 'effective_from',
        accessorKey: 'effective_from',
        size: 170,
        header: ({ column }) => (
          <DataTableColumnHeader column={column} title={t('Effective from')} />
        ),
        cell: ({ row }) =>
          formatCatalogTimestamp(row.original.effective_from) || '—',
      },
    ],
    [t]
  )
}
