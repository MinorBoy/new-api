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
import {
  AlertTriangle,
  ChevronLeft,
  ChevronRight,
  RefreshCw,
  Scale,
  SearchX,
} from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import { Field, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import {
  ADMIN_PERMISSION_ACTIONS,
  ADMIN_PERMISSION_RESOURCES,
  hasPermission,
} from '@/lib/admin-permissions'
import dayjs from '@/lib/dayjs'
import { useAuthStore } from '@/stores/auth-store'

import { costAccountingQueryKeys, listCostAnomalies } from '../api'
import type { CostAnomaly, CostAnomalyParams } from '../types'
import {
  CostReconcileDrawer,
  type CostReconcileTarget,
} from './reconcile-drawer'

type AnomalyQueueProps = {
  enabled?: boolean
}

const DEFAULT_PARAMS: CostAnomalyParams = { page: 1, page_size: 20 }

function anomalyLabel(kind: string, t: (key: string) => string): string {
  if (kind === 'cost_unknown') return t('Cost unknown')
  if (kind === 'settlement_failed') return t('Settlement failed')
  if (kind === 'revenue_failed') return t('Revenue failed')
  if (kind === 'orphaned_task') return t('Orphaned async request')
  return kind
}

function reconcileTarget(anomaly: CostAnomaly): CostReconcileTarget | null {
  if (
    anomaly.attempt &&
    (anomaly.attempt.status === 'cost_unknown' ||
      anomaly.attempt.status === 'settlement_failed')
  ) {
    return { kind: 'attempt', attempt: anomaly.attempt }
  }
  if (anomaly.request.revenue_status === 'revenue_failed') {
    return { kind: 'revenue', request: anomaly.request }
  }
  return null
}

export function AnomalyQueue(props: AnomalyQueueProps) {
  const { t } = useTranslation()
  const currentUser = useAuthStore((state) => state.auth.user)
  const [params, setParams] = useState<CostAnomalyParams>(DEFAULT_PARAMS)
  const [channelFilter, setChannelFilter] = useState('')
  const [repairTarget, setRepairTarget] = useState<CostReconcileTarget | null>(
    null
  )
  const canReconcile = hasPermission(
    currentUser,
    ADMIN_PERMISSION_RESOURCES.COST_ACCOUNTING,
    ADMIN_PERMISSION_ACTIONS.RECONCILE
  )
  const anomalyQuery = useQuery({
    queryKey: costAccountingQueryKeys.anomalyList(params),
    queryFn: () => listCostAnomalies(params),
    enabled: props.enabled ?? true,
  })

  const applyChannelFilter = () => {
    const parsed = Number(channelFilter)
    setParams((current) => ({
      ...current,
      page: 1,
      channel_id:
        channelFilter !== '' && Number.isInteger(parsed) && parsed > 0
          ? parsed
          : undefined,
    }))
  }

  const page = anomalyQuery.data?.data.page ?? params.page ?? 1
  const pageSize = anomalyQuery.data?.data.page_size ?? params.page_size ?? 20
  const total = anomalyQuery.data?.data.total ?? 0
  const pageCount = Math.max(1, Math.ceil(total / pageSize))

  return (
    <section className='flex min-h-0 flex-col gap-4'>
      <div className='flex flex-wrap items-end gap-3'>
        <Field className='w-48'>
          <FieldLabel>{t('Anomaly type')}</FieldLabel>
          <Select
            value={params.kind ?? 'all'}
            onValueChange={(value) =>
              setParams((current) => ({
                ...current,
                page: 1,
                kind: value && value !== 'all' ? value : undefined,
              }))
            }
          >
            <SelectTrigger className='w-full'>
              <SelectValue />
            </SelectTrigger>
            <SelectContent align='start'>
              <SelectGroup>
                <SelectItem value='all'>{t('All anomalies')}</SelectItem>
                <SelectItem value='cost_unknown'>
                  {t('Cost unknown')}
                </SelectItem>
                <SelectItem value='settlement_failed'>
                  {t('Settlement failed')}
                </SelectItem>
                <SelectItem value='revenue_failed'>
                  {t('Revenue failed')}
                </SelectItem>
                <SelectItem value='orphaned_task'>
                  {t('Orphaned async request')}
                </SelectItem>
              </SelectGroup>
            </SelectContent>
          </Select>
        </Field>
        <Field className='w-40'>
          <FieldLabel htmlFor='anomaly-channel-id'>
            {t('Channel ID')}
          </FieldLabel>
          <Input
            id='anomaly-channel-id'
            inputMode='numeric'
            value={channelFilter}
            onChange={(event) => setChannelFilter(event.target.value)}
            onBlur={applyChannelFilter}
            onKeyDown={(event) => {
              if (event.key === 'Enter') applyChannelFilter()
            }}
          />
        </Field>
        <Button type='button' variant='outline' onClick={applyChannelFilter}>
          {t('Apply filters')}
        </Button>
      </div>

      <div className='min-h-[22rem] overflow-auto rounded-md border'>
        {anomalyQuery.isLoading ? (
          <div className='space-y-3 p-4' aria-label={t('Loading anomalies')}>
            {Array.from({ length: 5 }, (_, index) => (
              <Skeleton key={`anomaly-${index}`} className='h-12 w-full' />
            ))}
          </div>
        ) : null}

        {anomalyQuery.error ? (
          <Empty className='min-h-[22rem]'>
            <EmptyHeader>
              <EmptyMedia variant='icon'>
                <AlertTriangle aria-hidden='true' />
              </EmptyMedia>
              <EmptyTitle>{t('Failed to load anomalies')}</EmptyTitle>
              <EmptyDescription>
                {anomalyQuery.error instanceof Error
                  ? anomalyQuery.error.message
                  : t('Try again')}
              </EmptyDescription>
            </EmptyHeader>
            <EmptyContent>
              <Button
                type='button'
                variant='outline'
                onClick={() => void anomalyQuery.refetch()}
              >
                <RefreshCw data-icon='inline-start' aria-hidden='true' />
                {t('Retry')}
              </Button>
            </EmptyContent>
          </Empty>
        ) : null}

        {!anomalyQuery.isLoading &&
        !anomalyQuery.error &&
        (anomalyQuery.data?.data.items.length ?? 0) === 0 ? (
          <Empty className='min-h-[22rem]'>
            <EmptyHeader>
              <EmptyMedia variant='icon'>
                <SearchX aria-hidden='true' />
              </EmptyMedia>
              <EmptyTitle>{t('No cost anomalies')}</EmptyTitle>
              <EmptyDescription>
                {t('No anomalies match the current filters.')}
              </EmptyDescription>
            </EmptyHeader>
          </Empty>
        ) : null}

        {!anomalyQuery.isLoading &&
        !anomalyQuery.error &&
        (anomalyQuery.data?.data.items.length ?? 0) > 0 ? (
          <Table className='min-w-[920px]'>
            <TableHeader className='bg-background sticky top-0'>
              <TableRow>
                <TableHead>{t('Occurred at')}</TableHead>
                <TableHead>{t('Anomaly type')}</TableHead>
                <TableHead>{t('Request')}</TableHead>
                <TableHead>{t('Channel')}</TableHead>
                <TableHead>{t('Model')}</TableHead>
                <TableHead>{t('Failure code')}</TableHead>
                <TableHead className='text-right'>{t('Actions')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {anomalyQuery.data?.data.items.map((anomaly) => {
                const target = reconcileTarget(anomaly)
                return (
                  <TableRow
                    key={`${anomaly.kind}-${anomaly.request.id}-${anomaly.attempt?.id ?? 0}`}
                  >
                    <TableCell className='whitespace-nowrap'>
                      {dayjs
                        .unix(anomaly.occurred_at)
                        .format('YYYY-MM-DD HH:mm:ss')}
                    </TableCell>
                    <TableCell>
                      <Badge variant='destructive'>
                        {anomalyLabel(anomaly.kind, t)}
                      </Badge>
                    </TableCell>
                    <TableCell className='font-mono text-xs'>
                      {anomaly.request.request_id}
                    </TableCell>
                    <TableCell>
                      {anomaly.attempt
                        ? `${anomaly.attempt.channel_name || t('Unknown channel')} · #${anomaly.attempt.channel_id}`
                        : t('Not attributed')}
                    </TableCell>
                    <TableCell className='font-mono text-xs'>
                      {anomaly.attempt?.billable_upstream_model ||
                        anomaly.request.origin_model_name}
                    </TableCell>
                    <TableCell className='font-mono text-xs'>
                      {anomaly.attempt?.failure_code ||
                        anomaly.request.failure_code ||
                        t('None')}
                    </TableCell>
                    <TableCell className='text-right'>
                      {canReconcile && target ? (
                        <Button
                          type='button'
                          variant='outline'
                          size='sm'
                          onClick={() => setRepairTarget(target)}
                        >
                          <Scale data-icon='inline-start' aria-hidden='true' />
                          {t('Reconcile')}
                        </Button>
                      ) : null}
                    </TableCell>
                  </TableRow>
                )
              })}
            </TableBody>
          </Table>
        ) : null}
      </div>

      <div className='flex items-center justify-between gap-3'>
        <p className='text-muted-foreground text-xs'>
          {t('{{count}} anomalies', { count: total })}
        </p>
        <div className='flex items-center gap-2'>
          <Button
            type='button'
            variant='outline'
            size='icon-sm'
            aria-label={t('Previous page')}
            disabled={page <= 1}
            onClick={() =>
              setParams((current) => ({ ...current, page: page - 1 }))
            }
          >
            <ChevronLeft aria-hidden='true' />
          </Button>
          <span className='min-w-16 text-center text-xs tabular-nums'>
            {page} / {pageCount}
          </span>
          <Button
            type='button'
            variant='outline'
            size='icon-sm'
            aria-label={t('Next page')}
            disabled={page >= pageCount}
            onClick={() =>
              setParams((current) => ({ ...current, page: page + 1 }))
            }
          >
            <ChevronRight aria-hidden='true' />
          </Button>
        </div>
      </div>

      <CostReconcileDrawer
        open={repairTarget !== null}
        target={repairTarget}
        onOpenChange={(open) => !open && setRepairTarget(null)}
      />
    </section>
  )
}
