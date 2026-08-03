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
  AlertTriangle,
  Banknote,
  CircleDollarSign,
  Percent,
  RefreshCw,
  TrendingUp,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'

import {
  formatMarginPPM,
  formatNanoUSD,
  signedMetricClass,
} from '../lib/cost-rule'
import type { CostProfitSummary } from '../types'

type ProfitSummaryProps = {
  summary?: CostProfitSummary
  loading: boolean
  error?: Error | null
  onRetry?: () => void
}

function amount(value: string | undefined): string {
  if (value === undefined) return '—'
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

function SummaryMetric(props: {
  label: string
  value: string
  icon: React.ElementType
  metric?: string
  valueClassName?: string
}) {
  const Icon = props.icon
  return (
    <div className='min-h-24 px-4 py-3 sm:px-5 sm:py-4'>
      <div className='text-muted-foreground flex items-center gap-2 text-xs'>
        <Icon className='size-3.5' aria-hidden='true' />
        <span>{props.label}</span>
      </div>
      <p
        className={`mt-2 font-mono text-lg font-semibold tabular-nums sm:text-xl ${props.valueClassName ?? ''}`}
        data-metric={props.metric}
      >
        {props.value}
      </p>
    </div>
  )
}

export function ProfitSummary(props: ProfitSummaryProps) {
  const { t } = useTranslation()

  if (props.error) {
    return (
      <Alert variant='destructive'>
        <AlertTriangle aria-hidden='true' />
        <AlertTitle>{t('Failed to load profit summary')}</AlertTitle>
        <AlertDescription>
          <span className='block'>{props.error.message}</span>
          {props.onRetry ? (
            <Button
              type='button'
              variant='outline'
              size='sm'
              className='mt-2'
              onClick={props.onRetry}
            >
              <RefreshCw data-icon='inline-start' aria-hidden='true' />
              {t('Retry')}
            </Button>
          ) : null}
        </AlertDescription>
      </Alert>
    )
  }

  if (props.loading) {
    return (
      <div
        className='grid min-h-24 grid-cols-2 divide-x divide-y rounded-md border lg:grid-cols-4 lg:divide-y-0'
        aria-label={t('Loading profit summary')}
      >
        {Array.from({ length: 4 }, (_, index) => (
          <div key={`summary-${index}`} className='min-h-24 p-4'>
            <Skeleton className='h-3 w-24' />
            <Skeleton className='mt-3 h-6 w-32' />
          </div>
        ))}
      </div>
    )
  }

  const summary = props.summary
  if (!summary) return null
  const operationalCounts = [
    [t('Complete requests'), summary.complete_request_count],
    [t('Negative profit'), summary.negative_profit_request_count],
    [t('Retry attempts'), summary.retry_attempt_count],
    [t('Awaiting meter'), summary.awaiting_meter_count],
    [t('Unknown cost'), summary.unknown_cost_count],
    [t('Settlement failed'), summary.settlement_failed_count],
    [t('Revenue failed'), summary.revenue_failed_count],
  ] as const

  return (
    <div className='overflow-hidden rounded-md border'>
      <div className='grid grid-cols-2 divide-x divide-y lg:grid-cols-4 lg:divide-y-0'>
        <SummaryMetric
          label={t('Billed revenue equivalent')}
          value={amount(summary.realized_revenue_nano_usd)}
          icon={Banknote}
        />
        <SummaryMetric
          label={t('Supplier cost')}
          value={amount(summary.realized_cost_nano_usd)}
          icon={CircleDollarSign}
        />
        <SummaryMetric
          label={t('Billed gross profit')}
          value={amount(summary.realized_profit_nano_usd)}
          icon={TrendingUp}
          metric='gross-profit'
          valueClassName={signedMetricClass(
            summary.realized_profit_nano_usd
          )}
        />
        <SummaryMetric
          label={t('Gross margin')}
          value={margin(summary.gross_margin_ppm)}
          icon={Percent}
          metric='gross-margin'
          valueClassName={signedMetricClass(summary.gross_margin_ppm)}
        />
      </div>
      <div className='bg-muted/20 flex flex-wrap gap-2 border-t px-4 py-3'>
        {operationalCounts.map(([label, value]) => (
          <Badge key={label} variant='outline' className='font-normal'>
            {label}: <span className='ml-1 font-mono'>{value}</span>
          </Badge>
        ))}
        <Badge variant='outline' className='font-normal'>
          {t('Known incomplete cost')}:{' '}
          <span className='ml-1 font-mono'>
            {amount(summary.known_incomplete_cost_nano_usd)}
          </span>
        </Badge>
      </div>
    </div>
  )
}
