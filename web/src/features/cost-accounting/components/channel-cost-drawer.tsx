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
import type { TFunction } from 'i18next'
import {
  CircleDollarSign,
  Eye,
  FileCheck2,
  History,
  Pencil,
  Plus,
  Power,
  PowerOff,
  RefreshCw,
  TriangleAlert,
} from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import {
  sideDrawerContentClassName,
  sideDrawerFooterClassName,
  sideDrawerFormClassName,
  sideDrawerHeaderClassName,
} from '@/components/drawer-layout'
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
import {
  Sheet,
  SheetClose,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
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
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import {
  ADMIN_PERMISSION_ACTIONS,
  ADMIN_PERMISSION_RESOURCES,
  hasPermission,
} from '@/lib/admin-permissions'
import dayjs from '@/lib/dayjs'
import { useAuthStore } from '@/stores/auth-store'

import type { Channel } from '../../channels/types'
import { getPricing } from '../../pricing/api'
import {
  formatDurationPrice,
  formatPrice,
  formatRequestPrice,
} from '../../pricing/lib/price'
import type { PricingData, PricingModel } from '../../pricing/types'
import {
  activateCostRule,
  costAccountingQueryKeys,
  getCostCoverage,
  listCostRules,
  retireCostRule,
  validateCostRule,
} from '../api'
import type { CostCoverageItem, CostRule } from '../types'
import { CostRuleDrawer } from './cost-rule-drawer'
import { CoveragePanel } from './coverage-panel'

type ChannelCostDrawerProps = {
  open: boolean
  channel: Channel | null
  onOpenChange: (open: boolean) => void
}

type ChannelCostRow = {
  billableModel: string
  originModels: string[]
  coverage: CostCoverageItem[]
  versions: CostRule[]
  activeRule: CostRule | null
  draftRule: CostRule | null
}

type EditorState = {
  billableModel: string
  originModel: string
  rule: CostRule | null
}

type LifecycleConfirmation = {
  action: 'activate' | 'retire'
  rule: CostRule
}

function buildChannelCostRows(
  rules: CostRule[],
  coverage: CostCoverageItem[]
): ChannelCostRow[] {
  const rows = new Map<string, ChannelCostRow>()
  const rowFor = (billableModel: string) => {
    const existing = rows.get(billableModel)
    if (existing) return existing
    const row: ChannelCostRow = {
      billableModel,
      originModels: [],
      coverage: [],
      versions: [],
      activeRule: null,
      draftRule: null,
    }
    rows.set(billableModel, row)
    return row
  }

  for (const item of coverage) {
    const row = rowFor(item.predicted_upstream_model)
    if (!row.originModels.includes(item.origin_model)) {
      row.originModels.push(item.origin_model)
    }
    row.coverage.push(item)
  }
  for (const rule of rules) {
    const row = rowFor(rule.billable_upstream_model)
    row.versions.push(rule)
    if (rule.status === 'active') row.activeRule = rule
    if (
      rule.status === 'draft' &&
      (!row.draftRule || rule.version > row.draftRule.version)
    ) {
      row.draftRule = rule
    }
  }

  for (const row of rows.values()) {
    row.originModels.sort((left, right) => left.localeCompare(right))
    row.versions.sort((left, right) => right.version - left.version)
  }
  return [...rows.values()].sort((left, right) =>
    left.billableModel.localeCompare(right.billableModel)
  )
}

function formatPricingModel(model: PricingModel, t: TFunction): string {
  if (model.billing_mode === 'per_duration' && model.duration_price) {
    const unit =
      model.duration_price.unit === 'second' ? t('Per second') : t('Per minute')
    return `${formatDurationPrice(model)} · ${unit}`
  }
  if (model.quota_type === 1) {
    return `${formatRequestPrice(model)} · ${t('Per request')}`
  }
  return `${t('Input')}: ${formatPrice(model, 'input', 'M')} · ${t(
    'Output'
  )}: ${formatPrice(model, 'output', 'M')} · ${t('Per 1M tokens')}`
}

function officialPrice(
  row: ChannelCostRow,
  pricing: PricingData | undefined,
  unavailable: string,
  t: TFunction
): string {
  const pricingByModel = new Map(
    (pricing?.data ?? []).map((model) => [model.model_name, model])
  )
  const prices = new Set<string>()
  for (const originModel of row.originModels) {
    const model = pricingByModel.get(originModel)
    if (model) prices.add(formatPricingModel(model, t))
  }
  return prices.size > 0 ? [...prices].join(' · ') : unavailable
}

function rulePrice(
  rule: CostRule | null,
  normalized: boolean,
  t: TFunction
): string {
  if (!rule) return '-'
  if (rule.cost_mode === 'free') return '0'
  const prices = normalized ? rule.config.normalized_usd_prices : rule.config
  const currency = normalized ? 'USD' : (rule.config.currency ?? '')
  if (rule.cost_mode === 'per_request') {
    return `${currency} ${prices.unit_price ?? '-'} · ${t('Per request')}`
  }
  if (rule.cost_mode === 'per_duration') {
    return `${currency} ${prices.price_per_second ?? '-'} · ${t('Per second')}`
  }
  if (rule.config.token_mode === 'total_tokens') {
    return `${currency} ${prices.total_per_million ?? '-'} · ${t('Per 1M tokens')}`
  }
  if (rule.config.token_mode === 'completion_tokens') {
    return `${currency} ${prices.completion_per_million ?? '-'} · ${t('Per 1M output tokens')}`
  }
  return `${t('Input')}: ${currency} ${prices.input_per_million ?? '-'} · ${t(
    'Output'
  )}: ${currency} ${prices.output_per_million ?? '-'} · ${t('Per 1M tokens')}`
}

function ruleModeLabel(rule: CostRule | null, t: TFunction) {
  if (!rule) return t('Not configured')
  if (rule.cost_mode === 'free') return t('Free')
  if (rule.cost_mode === 'per_request') return t('Per request')
  if (rule.cost_mode === 'per_duration') return t('Per duration')
  return t('Per token')
}

function ruleStatusLabel(rule: CostRule, t: TFunction) {
  if (rule.status === 'active') return t('Active')
  if (rule.status === 'draft') return t('Draft')
  return t('Retired')
}

export function ChannelCostDrawer(props: ChannelCostDrawerProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const currentUser = useAuthStore((state) => state.auth.user)
  const channelID = props.channel?.id ?? 0
  const canRead = hasPermission(
    currentUser,
    ADMIN_PERMISSION_RESOURCES.COST_ACCOUNTING,
    ADMIN_PERMISSION_ACTIONS.READ
  )
  const canWrite = hasPermission(
    currentUser,
    ADMIN_PERMISSION_RESOURCES.COST_ACCOUNTING,
    ADMIN_PERMISSION_ACTIONS.WRITE
  )
  const [editor, setEditor] = useState<EditorState | null>(null)
  const [historyModel, setHistoryModel] = useState<string | null>(null)
  const [confirmation, setConfirmation] =
    useState<LifecycleConfirmation | null>(null)

  const rulesQuery = useQuery({
    queryKey: costAccountingQueryKeys.ruleList({ channel_id: channelID }),
    queryFn: () => listCostRules({ channel_id: channelID }),
    enabled: props.open && channelID > 0 && canRead,
  })
  const coverageQuery = useQuery({
    queryKey: costAccountingQueryKeys.coverage({ channel_id: channelID }),
    queryFn: () => getCostCoverage({ channel_id: channelID }),
    enabled: props.open && channelID > 0 && canRead,
  })
  const pricingQuery = useQuery({
    queryKey: ['pricing'],
    queryFn: getPricing,
    enabled: props.open && channelID > 0 && canRead,
    staleTime: 5 * 60 * 1000,
  })

  const invalidateCostQueries = async () => {
    await Promise.all([
      queryClient.invalidateQueries({
        queryKey: costAccountingQueryKeys.rules(),
      }),
      queryClient.invalidateQueries({
        queryKey: costAccountingQueryKeys.coverages(),
      }),
    ])
  }

  const validateMutation = useMutation({
    mutationFn: (rule: CostRule) => validateCostRule(rule.id),
    onSuccess: () => toast.success(t('Cost rule is valid')),
    onError: () => toast.error(t('Cost rule validation failed')),
  })
  const lifecycleMutation = useMutation({
    mutationFn: async (value: LifecycleConfirmation) => {
      if (value.action === 'activate') {
        await activateCostRule(value.rule.id)
        return
      }
      await retireCostRule(value.rule.id)
    },
    onSuccess: async (_, value) => {
      await invalidateCostQueries()
      toast.success(
        t(
          value.action === 'activate'
            ? 'Cost rule activated'
            : 'Cost rule retired'
        )
      )
      setConfirmation(null)
    },
    onError: () => toast.error(t('Failed to update cost rule status')),
  })

  const handleOpenChange = (open: boolean) => {
    if (!open) {
      setEditor(null)
      setHistoryModel(null)
      setConfirmation(null)
    }
    props.onOpenChange(open)
  }

  const rules = rulesQuery.data?.data ?? []
  const coverage = coverageQuery.data?.data ?? []
  const rows = buildChannelCostRows(rules, coverage)
  const history = rows.find((row) => row.billableModel === historyModel)
  const isLoading =
    rulesQuery.isLoading || coverageQuery.isLoading || pricingQuery.isLoading
  const error = rulesQuery.error ?? coverageQuery.error ?? pricingQuery.error

  const retry = async () => {
    await Promise.all([
      rulesQuery.refetch(),
      coverageQuery.refetch(),
      pricingQuery.refetch(),
    ])
  }

  return (
    <>
      <Sheet open={props.open} onOpenChange={handleOpenChange}>
        <SheetContent className={sideDrawerContentClassName('sm:max-w-2xl')}>
          <SheetHeader className={sideDrawerHeaderClassName()}>
            <SheetTitle>{t('Model costs')}</SheetTitle>
            <SheetDescription>
              {props.channel
                ? `${props.channel.name} · #${props.channel.id}`
                : t('Select a channel')}
            </SheetDescription>
          </SheetHeader>

          <div className={sideDrawerFormClassName('gap-4')}>
            {!canRead ? (
              <Empty className='min-h-64'>
                <EmptyHeader>
                  <EmptyMedia variant='icon'>
                    <TriangleAlert aria-hidden='true' />
                  </EmptyMedia>
                  <EmptyTitle>{t('Permission required')}</EmptyTitle>
                  <EmptyDescription>
                    {t('You cannot view supplier cost accounting.')}
                  </EmptyDescription>
                </EmptyHeader>
              </Empty>
            ) : null}
            {canRead && isLoading ? (
              <div
                className='flex min-h-64 flex-col gap-3'
                aria-label={t('Loading')}
              >
                <Skeleton className='h-16 w-full' />
                <Skeleton className='h-10 w-full' />
                <Skeleton className='h-14 w-full' />
                <Skeleton className='h-14 w-full' />
              </div>
            ) : null}
            {canRead && !isLoading && error ? (
              <Empty className='min-h-64'>
                <EmptyHeader>
                  <EmptyMedia variant='icon'>
                    <TriangleAlert aria-hidden='true' />
                  </EmptyMedia>
                  <EmptyTitle>{t('Failed to load model costs')}</EmptyTitle>
                  <EmptyDescription>
                    {error instanceof Error ? error.message : t('Try again')}
                  </EmptyDescription>
                </EmptyHeader>
                <EmptyContent>
                  <Button
                    type='button'
                    variant='outline'
                    onClick={() => void retry()}
                  >
                    <RefreshCw data-icon='inline-start' aria-hidden='true' />
                    {t('Retry')}
                  </Button>
                </EmptyContent>
              </Empty>
            ) : null}
            {canRead && !isLoading && !error ? (
              <>
                <CoveragePanel items={coverage} />

                {rows.length === 0 ? (
                  <Empty className='min-h-56'>
                    <EmptyHeader>
                      <EmptyMedia variant='icon'>
                        <CircleDollarSign aria-hidden='true' />
                      </EmptyMedia>
                      <EmptyTitle>{t('No model cost data')}</EmptyTitle>
                      <EmptyDescription>
                        {t(
                          'No enabled model mappings or cost rules were found.'
                        )}
                      </EmptyDescription>
                    </EmptyHeader>
                  </Empty>
                ) : (
                  <div className='max-h-[48vh] overflow-auto rounded-md border'>
                    <Table className='min-w-[1040px]'>
                      <TableHeader className='bg-background sticky top-0'>
                        <TableRow>
                          <TableHead>{t('Billable upstream model')}</TableHead>
                          <TableHead>{t('Client models')}</TableHead>
                          <TableHead>{t('Official price')}</TableHead>
                          <TableHead>{t('Rule')}</TableHead>
                          <TableHead>{t('Supplier price')}</TableHead>
                          <TableHead>{t('Normalized USD price')}</TableHead>
                          <TableHead>{t('Coverage')}</TableHead>
                          <TableHead className='text-right'>
                            {t('Actions')}
                          </TableHead>
                        </TableRow>
                      </TableHeader>
                      <TableBody>
                        {rows.map((row) => {
                          const displayedRule = row.draftRule ?? row.activeRule
                          const covered =
                            row.coverage.length > 0 &&
                            row.coverage.every((item) => item.covered)
                          const originModel =
                            row.originModels[0] ?? row.billableModel
                          const previewRule = displayedRule
                          return (
                            <TableRow key={row.billableModel}>
                              <TableCell className='max-w-52 font-mono text-xs break-all whitespace-normal'>
                                {row.billableModel}
                              </TableCell>
                              <TableCell className='max-w-52 whitespace-normal'>
                                {row.originModels.length > 0
                                  ? row.originModels.join(', ')
                                  : t('No enabled mapping')}
                              </TableCell>
                              <TableCell className='max-w-56 whitespace-normal'>
                                {officialPrice(
                                  row,
                                  pricingQuery.data,
                                  t('Unavailable'),
                                  t
                                )}
                              </TableCell>
                              <TableCell>
                                <div className='flex items-center gap-2'>
                                  <span>{ruleModeLabel(displayedRule, t)}</span>
                                  {displayedRule ? (
                                    <Badge
                                      variant={
                                        displayedRule.status === 'active'
                                          ? 'default'
                                          : 'secondary'
                                      }
                                    >
                                      {displayedRule.status === 'active'
                                        ? t('Active')
                                        : t('Draft')}{' '}
                                      v{displayedRule.version}
                                    </Badge>
                                  ) : null}
                                </div>
                              </TableCell>
                              <TableCell className='font-mono text-xs'>
                                {rulePrice(displayedRule, false, t)}
                              </TableCell>
                              <TableCell className='font-mono text-xs'>
                                {rulePrice(displayedRule, true, t)}
                              </TableCell>
                              <TableCell>
                                <Badge
                                  variant={covered ? 'outline' : 'destructive'}
                                >
                                  {covered ? t('Covered') : t('Uncovered')}
                                </Badge>
                              </TableCell>
                              <TableCell>
                                <div className='flex justify-end gap-1'>
                                  {previewRule ? (
                                    <Tooltip>
                                      <TooltipTrigger
                                        render={
                                          <Button
                                            type='button'
                                            variant='ghost'
                                            size='icon-sm'
                                            aria-label={t('Preview cost')}
                                            onClick={() =>
                                              setEditor({
                                                billableModel:
                                                  row.billableModel,
                                                originModel,
                                                rule: previewRule,
                                              })
                                            }
                                          />
                                        }
                                      >
                                        <Eye aria-hidden='true' />
                                      </TooltipTrigger>
                                      <TooltipContent>
                                        {t('Preview cost')}
                                      </TooltipContent>
                                    </Tooltip>
                                  ) : null}

                                  {canWrite ? (
                                    <Tooltip>
                                      <TooltipTrigger
                                        render={
                                          <Button
                                            type='button'
                                            variant='ghost'
                                            size='icon-sm'
                                            aria-label={
                                              row.draftRule
                                                ? t('Edit cost draft')
                                                : t('Create cost draft')
                                            }
                                            onClick={() =>
                                              setEditor({
                                                billableModel:
                                                  row.billableModel,
                                                originModel,
                                                rule:
                                                  row.draftRule ??
                                                  row.activeRule,
                                              })
                                            }
                                          />
                                        }
                                      >
                                        {row.draftRule ? (
                                          <Pencil aria-hidden='true' />
                                        ) : (
                                          <Plus aria-hidden='true' />
                                        )}
                                      </TooltipTrigger>
                                      <TooltipContent>
                                        {row.draftRule
                                          ? t('Edit cost draft')
                                          : t('Create cost draft')}
                                      </TooltipContent>
                                    </Tooltip>
                                  ) : null}

                                  {canWrite && row.draftRule ? (
                                    <>
                                      <Tooltip>
                                        <TooltipTrigger
                                          render={
                                            <Button
                                              type='button'
                                              variant='ghost'
                                              size='icon-sm'
                                              aria-label={t(
                                                'Validate cost rule'
                                              )}
                                              disabled={
                                                validateMutation.isPending
                                              }
                                              onClick={() =>
                                                validateMutation.mutate(
                                                  row.draftRule as CostRule
                                                )
                                              }
                                            />
                                          }
                                        >
                                          <FileCheck2 aria-hidden='true' />
                                        </TooltipTrigger>
                                        <TooltipContent>
                                          {t('Validate cost rule')}
                                        </TooltipContent>
                                      </Tooltip>
                                      <Tooltip>
                                        <TooltipTrigger
                                          render={
                                            <Button
                                              type='button'
                                              variant='ghost'
                                              size='icon-sm'
                                              aria-label={t(
                                                'Activate cost rule'
                                              )}
                                              onClick={() =>
                                                setConfirmation({
                                                  action: 'activate',
                                                  rule: row.draftRule as CostRule,
                                                })
                                              }
                                            />
                                          }
                                        >
                                          <Power aria-hidden='true' />
                                        </TooltipTrigger>
                                        <TooltipContent>
                                          {t('Activate cost rule')}
                                        </TooltipContent>
                                      </Tooltip>
                                    </>
                                  ) : null}

                                  {canWrite && row.activeRule ? (
                                    <Tooltip>
                                      <TooltipTrigger
                                        render={
                                          <Button
                                            type='button'
                                            variant='ghost'
                                            size='icon-sm'
                                            aria-label={t('Retire cost rule')}
                                            onClick={() =>
                                              setConfirmation({
                                                action: 'retire',
                                                rule: row.activeRule as CostRule,
                                              })
                                            }
                                          />
                                        }
                                      >
                                        <PowerOff aria-hidden='true' />
                                      </TooltipTrigger>
                                      <TooltipContent>
                                        {t('Retire cost rule')}
                                      </TooltipContent>
                                    </Tooltip>
                                  ) : null}

                                  <Tooltip>
                                    <TooltipTrigger
                                      render={
                                        <Button
                                          type='button'
                                          variant='ghost'
                                          size='icon-sm'
                                          aria-label={t('Show version history')}
                                          onClick={() =>
                                            setHistoryModel((current) =>
                                              current === row.billableModel
                                                ? null
                                                : row.billableModel
                                            )
                                          }
                                        />
                                      }
                                    >
                                      <History aria-hidden='true' />
                                    </TooltipTrigger>
                                    <TooltipContent>
                                      {t('Show version history')}
                                    </TooltipContent>
                                  </Tooltip>
                                </div>
                              </TableCell>
                            </TableRow>
                          )
                        })}
                      </TableBody>
                    </Table>
                  </div>
                )}

                {history ? (
                  <section className='flex flex-col gap-3'>
                    <div>
                      <h3 className='text-sm font-semibold'>
                        {t('Version history')}
                      </h3>
                      <p className='text-muted-foreground font-mono text-xs'>
                        {history.billableModel}
                      </p>
                    </div>
                    <div className='max-h-48 overflow-y-auto rounded-md border'>
                      <Table>
                        <TableHeader>
                          <TableRow>
                            <TableHead>{t('Version')}</TableHead>
                            <TableHead>{t('Status')}</TableHead>
                            <TableHead>{t('Supplier price')}</TableHead>
                            <TableHead>{t('Normalized USD price')}</TableHead>
                            <TableHead>{t('Effective time')}</TableHead>
                          </TableRow>
                        </TableHeader>
                        <TableBody>
                          {history.versions.map((rule) => (
                            <TableRow key={rule.id}>
                              <TableCell>v{rule.version}</TableCell>
                              <TableCell>{ruleStatusLabel(rule, t)}</TableCell>
                              <TableCell className='font-mono text-xs'>
                                {rulePrice(rule, false, t)}
                              </TableCell>
                              <TableCell className='font-mono text-xs'>
                                {rulePrice(rule, true, t)}
                              </TableCell>
                              <TableCell className='text-muted-foreground'>
                                {rule.effective_from
                                  ? dayjs
                                      .unix(rule.effective_from)
                                      .format('YYYY-MM-DD HH:mm')
                                  : '-'}
                              </TableCell>
                            </TableRow>
                          ))}
                        </TableBody>
                      </Table>
                    </div>
                  </section>
                ) : null}
              </>
            ) : null}
          </div>

          <SheetFooter className={sideDrawerFooterClassName()}>
            <SheetClose render={<Button type='button' variant='outline' />}>
              {t('Close')}
            </SheetClose>
          </SheetFooter>
        </SheetContent>
      </Sheet>

      {props.channel && editor ? (
        <CostRuleDrawer
          open
          channel={props.channel}
          billableModel={editor.billableModel}
          originModel={editor.originModel}
          rule={editor.rule}
          canWrite={canWrite}
          onOpenChange={(open) => !open && setEditor(null)}
        />
      ) : null}

      <ConfirmDialog
        open={confirmation !== null}
        onOpenChange={(open) => !open && setConfirmation(null)}
        title={
          confirmation?.action === 'activate'
            ? t('Activate cost rule?')
            : t('Retire cost rule?')
        }
        desc={
          confirmation?.action === 'activate'
            ? t('The new version becomes authoritative for future requests.')
            : t('Future requests will no longer use this active rule.')
        }
        confirmText={
          confirmation?.action === 'activate' ? t('Activate') : t('Retire')
        }
        destructive={confirmation?.action === 'retire'}
        isLoading={lifecycleMutation.isPending}
        handleConfirm={() => {
          if (confirmation) lifecycleMutation.mutate(confirmation)
        }}
      />
    </>
  )
}
