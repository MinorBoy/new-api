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
  CheckCircle2,
  CircleDollarSign,
  History,
  RefreshCw,
  Scale,
  Trophy,
} from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import {
  ADMIN_PERMISSION_ACTIONS,
  ADMIN_PERMISSION_RESOURCES,
  hasPermission,
} from '@/lib/admin-permissions'
import dayjs from '@/lib/dayjs'
import { useAuthStore } from '@/stores/auth-store'

import { costAccountingQueryKeys, getCostAccountingRequest } from '../api'
import { formatMarginPPM, formatNanoUSD } from '../lib/cost-rule'
import type {
  CostAccountingAttemptLedger,
  CostAttemptStatus,
  CostChargeEvent,
  CostMeterSource,
  CostProfitStatus,
  CostReconciliationStatus,
  CostRevenueStatus,
} from '../types'
import {
  CostReconcileDrawer,
  type CostReconcileTarget,
} from './reconcile-drawer'

type CostRequestDetailProps = {
  requestID?: number
  isAdmin: boolean
  open: boolean
}

function statusLabel(status: CostAttemptStatus, t: (key: string) => string) {
  const labels: Record<CostAttemptStatus, string> = {
    prepared: 'Prepared',
    dispatching: 'Dispatching',
    not_dispatched: 'Not dispatched',
    awaiting_meter: 'Awaiting meter',
    settled: 'Settled',
    confirmed_zero: 'Confirmed zero',
    cost_unknown: 'Cost unknown',
    settlement_failed: 'Settlement failed',
  }
  return t(labels[status])
}

function chargeEventLabel(
  event: CostChargeEvent | '',
  t: (key: string) => string
) {
  if (event === 'response_succeeded') return t('Response succeeded')
  if (event === 'submit_accepted') return t('Submit accepted')
  if (event === 'task_succeeded') return t('Task succeeded')
  return t('Not recorded')
}

function meterSourceLabel(
  source: CostMeterSource | '',
  t: (key: string) => string
) {
  if (source === 'validated_request') return t('Validated request')
  if (source === 'upstream_actual') return t('Upstream actual')
  if (source === 'upstream_usage') return t('Upstream usage')
  if (source === 'local_usage') return t('Local usage')
  return t('Not recorded')
}

function reconciliationLabel(
  status: CostReconciliationStatus,
  t: (key: string) => string
) {
  return status === 'reconciled' ? t('Reconciled') : t('Not reconciled')
}

function revenueStatusLabel(
  status: CostRevenueStatus,
  t: (key: string) => string
): string {
  if (status === 'pending') return t('Revenue pending')
  if (status === 'settled') return t('Revenue settled')
  if (status === 'confirmed_zero') return t('Revenue confirmed zero')
  return t('Revenue failed')
}

function profitStatusLabel(
  status: CostProfitStatus,
  t: (key: string) => string
): string {
  if (status === 'complete') return t('Profit complete')
  if (status === 'incomplete_cost') return t('Incomplete cost')
  return t('Incomplete revenue')
}

function safeNanoUSD(value: string | undefined, unavailable: string): string {
  if (value === undefined) return unavailable
  try {
    return formatNanoUSD(value)
  } catch {
    return unavailable
  }
}

function safeMargin(value: string | undefined, unavailable: string): string {
  if (value === undefined) return unavailable
  try {
    return formatMarginPPM(value)
  } catch {
    return unavailable
  }
}

function meterEntryValue(entry: unknown): string {
  if (entry === null || entry === undefined) return 'null'
  if (typeof entry === 'object') return JSON.stringify(entry)
  return String(entry)
}

function meterEntries(value: string): Array<[string, string]> {
  if (!value.trim()) {
    return []
  }
  try {
    const parsed = JSON.parse(value) as unknown
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
      return []
    }
    return Object.entries(parsed as Record<string, unknown>).map(
      ([key, entry]) => [key, meterEntryValue(entry)]
    )
  } catch {
    return []
  }
}

function MeterSnapshot(props: { label: string; value: string }) {
  const { t } = useTranslation()
  const entries = meterEntries(props.value)
  return (
    <div className='min-w-0'>
      <dt className='text-muted-foreground text-xs'>{props.label}</dt>
      <dd className='mt-1 min-h-6 font-mono text-xs leading-5 break-all'>
        {entries.length > 0
          ? entries.map(([key, value]) => (
              <span key={key} className='mr-3 inline-block'>
                {key}: {value}
              </span>
            ))
          : t('Not recorded')}
      </dd>
    </div>
  )
}

function AttemptTimelineItem(props: {
  attempt: CostAccountingAttemptLedger
  winning: boolean
  canReconcile: boolean
  onReconcile: (target: CostReconcileTarget) => void
}) {
  const { t } = useTranslation()
  const attempt = props.attempt
  const canRepair =
    props.canReconcile &&
    (attempt.status === 'cost_unknown' ||
      attempt.status === 'settlement_failed')
  return (
    <li className='relative border-b pb-5 last:border-b-0 last:pb-0'>
      <span
        className='bg-background border-border absolute top-0 -left-[1.2rem] flex size-6 items-center justify-center rounded-full border'
        aria-hidden='true'
      >
        {props.winning ? (
          <Trophy className='text-warning size-3.5' />
        ) : (
          <CircleDollarSign className='text-muted-foreground size-3.5' />
        )}
      </span>

      <div className='flex flex-wrap items-start justify-between gap-2'>
        <div>
          <div className='flex flex-wrap items-center gap-2'>
            <h4 className='text-sm font-semibold'>
              {t('Attempt')} #{attempt.attempt_no}
            </h4>
            {props.winning ? (
              <Badge variant='outline'>{t('Winning attempt')}</Badge>
            ) : null}
            <Badge
              variant={
                attempt.status === 'settled' ||
                attempt.status === 'confirmed_zero'
                  ? 'secondary'
                  : 'destructive'
              }
            >
              {statusLabel(attempt.status, t)}
            </Badge>
          </div>
          <p className='text-muted-foreground mt-1 text-xs'>
            {attempt.channel_name || t('Unknown channel')} · #
            {attempt.channel_id}
          </p>
        </div>
        {canRepair ? (
          <Button
            type='button'
            variant='outline'
            size='sm'
            onClick={() =>
              props.onReconcile({ kind: 'attempt', attempt: props.attempt })
            }
          >
            <Scale data-icon='inline-start' aria-hidden='true' />
            {t('Reconcile')}
          </Button>
        ) : null}
      </div>

      <dl className='mt-4 grid gap-x-4 gap-y-3 text-sm sm:grid-cols-2'>
        <DetailValue
          label={t('Predicted upstream model')}
          value={attempt.predicted_upstream_model || t('Not recorded')}
          mono
        />
        <DetailValue
          label={t('Billable upstream model')}
          value={attempt.billable_upstream_model || t('Not recorded')}
          mono
        />
        <DetailValue
          label={t('Frozen rule')}
          value={`${t('Rule')} v${attempt.rule_version} · #${attempt.rule_id}`}
        />
        <DetailValue
          label={t('Charge event')}
          value={chargeEventLabel(attempt.charge_event, t)}
        />
        <DetailValue
          label={t('Meter source')}
          value={meterSourceLabel(attempt.meter_source, t)}
        />
        <DetailValue
          label={t('Reconciliation status')}
          value={reconciliationLabel(attempt.reconciliation_status, t)}
        />
        <DetailValue
          label={t('Result code')}
          value={attempt.result_code || t('Not recorded')}
          mono
        />
        <DetailValue
          label={t('Failure code')}
          value={attempt.failure_code || t('None')}
          mono
        />
        <MeterSnapshot
          label={t('Request meter')}
          value={attempt.request_meter_json}
        />
        <MeterSnapshot
          label={t('Actual meter')}
          value={attempt.actual_meter_json}
        />
        <DetailValue
          label={t('Original amount')}
          value={attempt.original_cost || t('Unavailable')}
          mono
        />
        <DetailValue
          label={t('Normalized amount')}
          value={safeNanoUSD(attempt.cost_nano_usd, t('Unavailable'))}
          mono
        />
      </dl>
    </li>
  )
}

function DetailValue(props: {
  label: string
  value: React.ReactNode
  mono?: boolean
}) {
  return (
    <div className='min-w-0'>
      <dt className='text-muted-foreground text-xs'>{props.label}</dt>
      <dd className={`mt-1 break-all ${props.mono ? 'font-mono text-xs' : ''}`}>
        {props.value}
      </dd>
    </div>
  )
}

export function CostRequestDetail(props: CostRequestDetailProps) {
  const { t } = useTranslation()
  const currentUser = useAuthStore((state) => state.auth.user)
  const [reconcileTarget, setReconcileTarget] =
    useState<CostReconcileTarget | null>(null)
  const requestID = props.requestID ?? 0
  const detailQuery = useQuery({
    queryKey: costAccountingQueryKeys.request(requestID),
    queryFn: () => getCostAccountingRequest(requestID),
    enabled:
      props.isAdmin &&
      props.open &&
      requestID > 0 &&
      hasPermission(
        currentUser,
        ADMIN_PERMISSION_RESOURCES.COST_ACCOUNTING,
        ADMIN_PERMISSION_ACTIONS.READ
      ),
  })
  const canRead =
    props.isAdmin &&
    hasPermission(
      currentUser,
      ADMIN_PERMISSION_RESOURCES.COST_ACCOUNTING,
      ADMIN_PERMISSION_ACTIONS.READ
    )
  const canReconcile = hasPermission(
    currentUser,
    ADMIN_PERMISSION_RESOURCES.COST_ACCOUNTING,
    ADMIN_PERMISSION_ACTIONS.RECONCILE
  )

  if (!canRead || !props.open) return null
  if (requestID === 0) {
    return (
      <Alert>
        <History aria-hidden='true' />
        <AlertTitle>{t('Historical cost unavailable')}</AlertTitle>
        <AlertDescription>
          {t('This log predates supplier cost accounting references.')}
        </AlertDescription>
      </Alert>
    )
  }
  if (detailQuery.isLoading) {
    return (
      <section aria-label={t('Loading cost accounting')} className='space-y-3'>
        <Skeleton className='h-16 w-full' />
        <Skeleton className='h-28 w-full' />
        <Skeleton className='h-28 w-full' />
      </section>
    )
  }
  if (detailQuery.error || !detailQuery.data?.data) {
    return (
      <Alert variant='destructive'>
        <AlertTriangle aria-hidden='true' />
        <AlertTitle>{t('Failed to load supplier cost accounting')}</AlertTitle>
        <AlertDescription>
          <span className='block'>
            {detailQuery.error instanceof Error
              ? detailQuery.error.message
              : t('Try again')}
          </span>
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
    )
  }

  const detail = detailQuery.data.data
  const request = detail.request
  return (
    <>
      <section className='border-border/60 space-y-5 border-t pt-4'>
        <div className='flex flex-wrap items-start justify-between gap-3'>
          <div>
            <div className='flex items-center gap-2'>
              <CircleDollarSign
                className='text-primary size-4'
                aria-hidden='true'
              />
              <h3 className='text-sm font-semibold'>
                {t('Supplier cost accounting')}
              </h3>
            </div>
            <p className='text-muted-foreground mt-1 font-mono text-xs break-all'>
              {request.request_id}
            </p>
          </div>
          <div className='flex flex-wrap gap-2'>
            <Badge variant='outline'>
              {revenueStatusLabel(request.revenue_status, t)}
            </Badge>
            <Badge variant='secondary'>
              {profitStatusLabel(request.profit_status, t)}
            </Badge>
            {canReconcile && request.revenue_status === 'revenue_failed' ? (
              <Button
                type='button'
                variant='outline'
                size='sm'
                onClick={() => setReconcileTarget({ kind: 'revenue', request })}
              >
                <Scale data-icon='inline-start' aria-hidden='true' />
                {t('Reconcile revenue')}
              </Button>
            ) : null}
          </div>
        </div>

        <dl className='grid gap-4 border-y py-4 sm:grid-cols-2 lg:grid-cols-4'>
          <DetailValue
            label={t('Billed revenue equivalent')}
            value={safeNanoUSD(
              request.billed_revenue_equivalent_nano_usd,
              t('Unavailable')
            )}
            mono
          />
          <DetailValue
            label={t('Supplier cost')}
            value={safeNanoUSD(
              request.confirmed_cost_nano_usd,
              t('Unavailable')
            )}
            mono
          />
          <DetailValue
            label={t('Billed gross profit')}
            value={safeNanoUSD(
              request.billed_gross_profit_nano_usd,
              t('Unavailable')
            )}
            mono
          />
          <DetailValue
            label={t('Gross margin')}
            value={safeMargin(request.gross_margin_ppm, t('Unavailable'))}
            mono
          />
        </dl>

        <div>
          <h4 className='text-sm font-semibold'>{t('Attempt timeline')}</h4>
          <ol className='border-border/70 mt-4 ml-3 space-y-5 border-l pl-5'>
            {detail.attempts.map((item) => (
              <AttemptTimelineItem
                key={item.attempt.id}
                attempt={item.attempt}
                winning={item.winning}
                canReconcile={canReconcile}
                onReconcile={setReconcileTarget}
              />
            ))}
          </ol>
        </div>

        <div>
          <div className='flex items-center gap-2'>
            <CheckCircle2
              className='text-muted-foreground size-4'
              aria-hidden='true'
            />
            <h4 className='text-sm font-semibold'>{t('Audit history')}</h4>
          </div>
          {detail.audits.length > 0 ? (
            <div className='mt-3 divide-y border-y'>
              {detail.audits.map((audit) => (
                <div
                  key={audit.id}
                  className='grid gap-1 py-3 text-xs sm:grid-cols-[9rem_1fr] sm:gap-4'
                >
                  <time className='text-muted-foreground'>
                    {dayjs.unix(audit.created_at).format('YYYY-MM-DD HH:mm:ss')}
                  </time>
                  <div>
                    <p>
                      <span className='font-mono'>{audit.old_state}</span>
                      {' -> '}
                      <span className='font-mono'>{audit.new_state}</span>
                      {' · '}
                      {t('Admin')} #{audit.admin_id}
                    </p>
                    <p className='text-muted-foreground mt-1 break-words'>
                      {audit.reason}
                    </p>
                  </div>
                </div>
              ))}
            </div>
          ) : (
            <p className='text-muted-foreground mt-2 text-xs'>
              {t('No reconciliation audits')}
            </p>
          )}
        </div>
      </section>

      <CostReconcileDrawer
        open={reconcileTarget !== null}
        target={reconcileTarget}
        onOpenChange={(open) => !open && setReconcileTarget(null)}
      />
    </>
  )
}
