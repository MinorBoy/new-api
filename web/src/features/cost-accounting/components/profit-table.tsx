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
import {
  createColumnHelper,
  flexRender,
  getCoreRowModel,
  useReactTable,
} from '@tanstack/react-table'
import { AlertTriangle, RefreshCw, SearchX } from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

import { formatMarginPPM, formatNanoUSD } from '../lib/cost-rule'
import type { CostProfitBreakdown } from '../types'

type ProfitTableProps = {
  rows: CostProfitBreakdown[]
  loading: boolean
  error?: Error | null
  onRetry?: () => void
}

const columnHelper = createColumnHelper<CostProfitBreakdown>()

function amount(value: string): string {
  try {
    return formatNanoUSD(value)
  } catch {
    return '—'
  }
}

function margin(value: string | undefined): string {
  if (value === undefined) return '—'
  try {
    return formatMarginPPM(value)
  } catch {
    return '—'
  }
}

export function ProfitTable(props: ProfitTableProps) {
  const { t } = useTranslation()
  const columns = useMemo(
    () => [
      columnHelper.accessor('channel_name', {
        header: t('Channel'),
        cell: (context) => (
          <div>
            <p className='font-medium'>
              {context.getValue() || t('Unknown channel')}
            </p>
            <p className='text-muted-foreground font-mono text-xs'>
              #{context.row.original.channel_id}
            </p>
          </div>
        ),
      }),
      columnHelper.accessor('billable_upstream_model', {
        header: t('Billable upstream model'),
        cell: (context) => (
          <span className='font-mono text-xs break-all'>
            {context.getValue()}
          </span>
        ),
      }),
      columnHelper.accessor('realized_revenue_nano_usd', {
        header: t('Billed revenue equivalent'),
        cell: (context) => (
          <span className='font-mono tabular-nums'>
            {amount(context.getValue())}
          </span>
        ),
      }),
      columnHelper.accessor('realized_cost_nano_usd', {
        header: t('Supplier cost'),
        cell: (context) => (
          <span className='font-mono tabular-nums'>
            {amount(context.getValue())}
          </span>
        ),
      }),
      columnHelper.accessor('realized_profit_nano_usd', {
        header: t('Billed gross profit'),
        cell: (context) => (
          <span className='font-mono tabular-nums'>
            {amount(context.getValue())}
          </span>
        ),
      }),
      columnHelper.accessor('gross_margin_ppm', {
        header: t('Gross margin'),
        cell: (context) => (
          <span className='font-mono tabular-nums'>
            {margin(context.getValue())}
          </span>
        ),
      }),
      columnHelper.accessor('attempt_count', {
        header: t('Attempts'),
        cell: (context) => (
          <span className='font-mono tabular-nums'>{context.getValue()}</span>
        ),
      }),
      columnHelper.display({
        id: 'anomalies',
        header: t('Anomalies'),
        cell: (context) => {
          const row = context.row.original
          const count =
            row.unknown_cost_count +
            row.settlement_failed_count +
            row.revenue_failed_count
          return <span className='font-mono tabular-nums'>{count}</span>
        },
      }),
    ],
    [t]
  )
  const table = useReactTable({
    data: props.rows,
    columns,
    getCoreRowModel: getCoreRowModel(),
  })

  if (props.loading) {
    return (
      <div
        className='min-h-[22rem] space-y-3 rounded-md border p-4'
        aria-label={t('Loading profit breakdown')}
      >
        {Array.from({ length: 6 }, (_, index) => (
          <Skeleton key={`profit-row-${index}`} className='h-12 w-full' />
        ))}
      </div>
    )
  }

  if (props.error) {
    return (
      <Empty className='min-h-[22rem] rounded-md border'>
        <EmptyHeader>
          <EmptyMedia variant='icon'>
            <AlertTriangle aria-hidden='true' />
          </EmptyMedia>
          <EmptyTitle>{t('Failed to load profit breakdown')}</EmptyTitle>
          <EmptyDescription>{props.error.message}</EmptyDescription>
        </EmptyHeader>
        {props.onRetry ? (
          <EmptyContent>
            <Button type='button' variant='outline' onClick={props.onRetry}>
              <RefreshCw data-icon='inline-start' aria-hidden='true' />
              {t('Retry')}
            </Button>
          </EmptyContent>
        ) : null}
      </Empty>
    )
  }

  if (props.rows.length === 0) {
    return (
      <Empty className='min-h-[22rem] rounded-md border'>
        <EmptyHeader>
          <EmptyMedia variant='icon'>
            <SearchX aria-hidden='true' />
          </EmptyMedia>
          <EmptyTitle>{t('No profit data')}</EmptyTitle>
          <EmptyDescription>
            {t('No completed cost accounting requests match these filters.')}
          </EmptyDescription>
        </EmptyHeader>
      </Empty>
    )
  }

  return (
    <div className='min-h-[22rem] overflow-auto rounded-md border'>
      <Table className='min-w-[1080px]'>
        <TableHeader className='bg-background sticky top-0'>
          {table.getHeaderGroups().map((headerGroup) => (
            <TableRow key={headerGroup.id}>
              {headerGroup.headers.map((header) => (
                <TableHead key={header.id}>
                  {header.isPlaceholder
                    ? null
                    : flexRender(
                        header.column.columnDef.header,
                        header.getContext()
                      )}
                </TableHead>
              ))}
            </TableRow>
          ))}
        </TableHeader>
        <TableBody>
          {table.getRowModel().rows.map((row) => (
            <TableRow key={row.id}>
              {row.getVisibleCells().map((cell) => (
                <TableCell key={cell.id}>
                  {flexRender(cell.column.columnDef.cell, cell.getContext())}
                </TableCell>
              ))}
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  )
}
