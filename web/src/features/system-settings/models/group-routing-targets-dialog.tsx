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
  Ban,
  ChevronLeft,
  ChevronRight,
  LoaderCircle,
  RotateCcw,
  Trash2,
} from 'lucide-react'
import { useMemo, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

import {
  previewGroupRoutingProfileTargets,
  type GroupRoutingProfileTargetPage,
  type GroupRoutingProfileTargetRequest,
  type GroupRoutingTargetStatus,
} from './group-routing-profile-api'
import {
  parseGroupRoutingProfiles,
  removeStaleGroupRoutingExclusions,
  toggleGroupRoutingTargetExclusion,
  type GroupCostMode,
} from './group-routing-requirements'

type GroupRoutingTargetsDialogProps = {
  open: boolean
  groupName: string
  groupRoutingRequirements: string
  disabled?: boolean
  onOpenChange: (open: boolean) => void
  onChange: (value: string) => void
}

type Target = GroupRoutingProfileTargetPage['items'][number]
type PageSize = 25 | 50 | 100

const pageSizes: PageSize[] = [25, 50, 100]
const costModes: GroupCostMode[] = [
  'per_request',
  'per_duration',
  'per_token',
  'free',
]
const targetStatuses: GroupRoutingTargetStatus[] = [
  'matched',
  'real_person_mismatch',
  'real_person_unknown',
  'cost_mode_mismatch',
  'cost_rule_missing',
  'excluded',
  'target_disabled',
  'channel_unavailable',
]

export function GroupRoutingTargetsDialog(
  props: GroupRoutingTargetsDialogProps
) {
  const { t } = useTranslation()
  const [modelFilter, setModelFilter] = useState('')
  const [channelFilter, setChannelFilter] = useState('all')
  const [costModeFilter, setCostModeFilter] = useState('all')
  const [statusFilter, setStatusFilter] = useState('all')
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState<PageSize>(25)
  const [cleaningStaleExclusions, setCleaningStaleExclusions] = useState(false)
  const [cleanupError, setCleanupError] = useState(false)

  const profile = useMemo(() => {
    try {
      return parseGroupRoutingProfiles(props.groupRoutingRequirements)[
        props.groupName
      ]
    } catch {
      return undefined
    }
  }, [props.groupName, props.groupRoutingRequirements])

  const request = useMemo<GroupRoutingProfileTargetRequest | undefined>(() => {
    if (!profile?.routing_source) return undefined
    return {
      group_name: props.groupName,
      profile,
      model: modelFilter.trim() || undefined,
      channel_id: channelFilter === 'all' ? undefined : Number(channelFilter),
      cost_mode:
        costModeFilter === 'all'
          ? undefined
          : (costModeFilter as GroupCostMode),
      status:
        statusFilter === 'all'
          ? undefined
          : (statusFilter as GroupRoutingTargetStatus),
      page,
      page_size: pageSize,
    }
  }, [
    channelFilter,
    costModeFilter,
    modelFilter,
    page,
    pageSize,
    profile,
    props.groupName,
    statusFilter,
  ])

  const targetsQuery = useQuery({
    queryKey: ['group-routing-profile-targets', request],
    queryFn: async () => {
      if (!request) throw new Error('Routing profile is not available')
      return previewGroupRoutingProfileTargets(request)
    },
    enabled: props.open && request !== undefined,
    placeholderData: (previous) => previous,
  })
  const result = targetsQuery.data?.data
  const excludedTargetKeys = new Set(profile?.excluded_target_keys ?? [])
  const totalPages = result
    ? Math.max(1, Math.ceil(result.total / result.page_size))
    : 1

  const toggleExclusion = (target: Target, excluded: boolean) => {
    props.onChange(
      toggleGroupRoutingTargetExclusion(
        props.groupRoutingRequirements,
        props.groupName,
        target.target_key,
        excluded
      )
    )
  }

  let targetCatalog: ReactNode
  if (targetsQuery.isError) {
    targetCatalog = (
      <Alert className='m-4'>
        <AlertTriangle aria-hidden='true' />
        <AlertTitle>{t('Routing preview unavailable')}</AlertTitle>
        <AlertDescription>
          {t('The adapted target catalog could not be loaded.')}
        </AlertDescription>
      </Alert>
    )
  } else if (targetsQuery.isPending && !result) {
    targetCatalog = (
      <div className='text-muted-foreground flex min-h-48 items-center justify-center gap-2'>
        <LoaderCircle className='size-4 animate-spin' aria-hidden='true' />
        {t('Loading targets')}
      </div>
    )
  } else if (!result || result.items.length === 0) {
    targetCatalog = (
      <div className='text-muted-foreground flex min-h-48 items-center justify-center px-4 text-center'>
        {t('No targets match the current filters.')}
      </div>
    )
  } else {
    targetCatalog = (
      <>
        <div className='hidden min-w-[1100px] lg:block'>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('Model')}</TableHead>
                <TableHead>{t('Channel')}</TableHead>
                <TableHead>{t('Upstream model')}</TableHead>
                <TableHead>{t('Cost variant')}</TableHead>
                <TableHead>{t('Real-person capability')}</TableHead>
                <TableHead>{t('Cost mode')}</TableHead>
                <TableHead>{t('Status')}</TableHead>
                <TableHead className='text-right'>{t('Actions')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {result.items.map((target) => (
                <TargetTableRow
                  key={target.target_key}
                  target={target}
                  excluded={excludedTargetKeys.has(target.target_key)}
                  disabled={props.disabled}
                  onToggleExclusion={toggleExclusion}
                />
              ))}
            </TableBody>
          </Table>
        </div>
        <div className='divide-y lg:hidden'>
          {result.items.map((target) => (
            <TargetMobileRow
              key={target.target_key}
              target={target}
              excluded={excludedTargetKeys.has(target.target_key)}
              disabled={props.disabled}
              onToggleExclusion={toggleExclusion}
            />
          ))}
        </div>
      </>
    )
  }

  const updateFilter = (setter: (value: string) => void, value: string) => {
    setter(value)
    setPage(1)
  }

  const cleanStaleExclusions = async () => {
    if (!profile || cleaningStaleExclusions) return
    setCleaningStaleExclusions(true)
    setCleanupError(false)
    try {
      const baseRequest: GroupRoutingProfileTargetRequest = {
        group_name: props.groupName,
        profile,
        page: 1,
        page_size: 100,
      }
      const firstPage = await previewGroupRoutingProfileTargets(baseRequest)
      const pageCount = Math.ceil(firstPage.data.total / 100)
      const remainingPages = await Promise.all(
        Array.from({ length: Math.max(0, pageCount - 1) }, (_, index) =>
          previewGroupRoutingProfileTargets({
            ...baseRequest,
            page: index + 2,
          })
        )
      )
      const liveTargetKeys = [firstPage, ...remainingPages].flatMap(
        (response) => response.data.items.map((target) => target.target_key)
      )
      props.onChange(
        removeStaleGroupRoutingExclusions(
          props.groupRoutingRequirements,
          props.groupName,
          liveTargetKeys
        )
      )
    } catch {
      setCleanupError(true)
    } finally {
      setCleaningStaleExclusions(false)
    }
  }

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className='flex max-h-[88vh] w-[min(96vw,1400px)] max-w-none flex-col overflow-hidden p-0'>
        <DialogHeader className='border-b px-5 py-4 pr-12'>
          <DialogTitle>
            {t('Adapted routing targets: {{group}}', {
              group: props.groupName,
            })}
          </DialogTitle>
          <DialogDescription>
            {t(
              'Material constraints, duration, resolution, strict cost, and margin are evaluated at request time.'
            )}
          </DialogDescription>
        </DialogHeader>

        <div className='grid min-h-0 flex-1 grid-rows-[auto_auto_auto_minmax(0,1fr)_auto] gap-3 px-5 pb-4'>
          <div className='grid gap-2 pt-1 sm:grid-cols-2 lg:grid-cols-4'>
            <Input
              value={modelFilter}
              onChange={(event) =>
                updateFilter(setModelFilter, event.target.value)
              }
              placeholder={t('Filter by model')}
              aria-label={t('Filter by model')}
            />
            <TargetFilterSelect
              value={channelFilter}
              label={t('Filter by channel')}
              allLabel={t('All channels')}
              onValueChange={(value) => updateFilter(setChannelFilter, value)}
              options={(result?.facets.channels ?? []).map((channel) => ({
                value: String(channel.id),
                label: `${channel.name} #${channel.id}`,
              }))}
            />
            <TargetFilterSelect
              value={costModeFilter}
              label={t('Filter by cost mode')}
              allLabel={t('All cost modes')}
              onValueChange={(value) => updateFilter(setCostModeFilter, value)}
              options={costModes.map((costMode) => ({
                value: costMode,
                label: costModeLabel(costMode, t),
              }))}
            />
            <TargetFilterSelect
              value={statusFilter}
              label={t('Filter by status')}
              allLabel={t('All statuses')}
              onValueChange={(value) => updateFilter(setStatusFilter, value)}
              options={targetStatuses.map((status) => ({
                value: status,
                label: targetStatusLabel(status, t),
              }))}
            />
          </div>

          <div className='flex min-h-8 flex-wrap items-center gap-2'>
            {result ? (
              <>
                <Badge variant='outline'>
                  {t('Matched models: {{matched}}/{{total}}', {
                    matched: result.summary.matched_models,
                    total: result.summary.models,
                  })}
                </Badge>
                <Badge variant='outline'>
                  {t('Matched targets: {{matched}}/{{total}}', {
                    matched: result.summary.matched_targets,
                    total: result.summary.targets,
                  })}
                </Badge>
                {result.summary.stale_exclusions > 0 ? (
                  <div className='flex items-center gap-2'>
                    <Badge variant='warning'>
                      {t('{{count}} stale exclusions', {
                        count: result.summary.stale_exclusions,
                      })}
                    </Badge>
                    <Button
                      type='button'
                      size='sm'
                      variant='outline'
                      disabled={props.disabled || cleaningStaleExclusions}
                      onClick={() => void cleanStaleExclusions()}
                      aria-label={t('Clean stale exclusions')}
                    >
                      {cleaningStaleExclusions ? (
                        <LoaderCircle
                          className='animate-spin'
                          aria-hidden='true'
                        />
                      ) : (
                        <Trash2 aria-hidden='true' />
                      )}
                      {t('Clean stale exclusions')}
                    </Button>
                  </div>
                ) : null}
              </>
            ) : null}
          </div>
          {cleanupError ? (
            <Alert variant='destructive'>
              <AlertTriangle aria-hidden='true' />
              <AlertTitle>{t('Unable to clean stale exclusions')}</AlertTitle>
              <AlertDescription>
                {t('The stale exclusions were not changed. Try again later.')}
              </AlertDescription>
            </Alert>
          ) : null}

          <div className='min-h-0 overflow-auto rounded-md border'>
            {targetCatalog}
          </div>

          <div className='flex flex-wrap items-center justify-between gap-3'>
            <TargetFilterSelect
              value={String(pageSize)}
              label={t('Rows per page')}
              allLabel=''
              includeAll={false}
              onValueChange={(value) => {
                setPageSize(Number(value) as PageSize)
                setPage(1)
              }}
              options={pageSizes.map((size) => ({
                value: String(size),
                label: t('{{count}} rows', { count: size }),
              }))}
            />
            <div className='flex items-center gap-2'>
              <span className='text-muted-foreground text-sm tabular-nums'>
                {t('Page {{page}} of {{total}}', {
                  page,
                  total: totalPages,
                })}
              </span>
              <Button
                type='button'
                size='icon-sm'
                variant='outline'
                disabled={page <= 1}
                onClick={() => setPage((current) => Math.max(1, current - 1))}
                aria-label={t('Previous page')}
              >
                <ChevronLeft aria-hidden='true' />
              </Button>
              <Button
                type='button'
                size='icon-sm'
                variant='outline'
                disabled={page >= totalPages}
                onClick={() =>
                  setPage((current) => Math.min(totalPages, current + 1))
                }
                aria-label={t('Next page')}
              >
                <ChevronRight aria-hidden='true' />
              </Button>
            </div>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  )
}

type TargetFilterSelectProps = {
  value: string
  label: string
  allLabel: string
  includeAll?: boolean
  options: { value: string; label: string }[]
  onValueChange: (value: string) => void
}

function TargetFilterSelect(props: TargetFilterSelectProps) {
  return (
    <Select
      value={props.value}
      onValueChange={(value) => {
        if (value) props.onValueChange(value)
      }}
    >
      <SelectTrigger className='w-full' aria-label={props.label}>
        <SelectValue placeholder={props.label} />
      </SelectTrigger>
      <SelectContent>
        {props.includeAll !== false ? (
          <SelectItem value='all'>{props.allLabel}</SelectItem>
        ) : null}
        {props.options.map((option) => (
          <SelectItem key={option.value} value={option.value}>
            {option.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}

type TargetRowProps = {
  target: Target
  excluded: boolean
  disabled?: boolean
  onToggleExclusion: (target: Target, excluded: boolean) => void
}

function TargetTableRow(props: TargetRowProps) {
  const { t } = useTranslation()
  return (
    <TableRow>
      <TableCell className='font-medium'>{props.target.model}</TableCell>
      <TableCell>
        {props.target.channel_name} #{props.target.channel_id}
      </TableCell>
      <TableCell>{props.target.upstream_model}</TableCell>
      <TableCell>{props.target.cost_variant_key || '-'}</TableCell>
      <TableCell>
        {realPersonCapabilityLabel(props.target.supports_real_person, t)}
      </TableCell>
      <TableCell>
        {props.target.cost_mode
          ? costModeLabel(props.target.cost_mode, t)
          : t('Unknown')}
      </TableCell>
      <TableCell>
        <TargetStatus target={props.target} />
      </TableCell>
      <TableCell className='text-right'>
        <TargetExclusionButton {...props} />
      </TableCell>
    </TableRow>
  )
}

function TargetMobileRow(props: TargetRowProps) {
  const { t } = useTranslation()
  return (
    <div className='space-y-2 p-3'>
      <div className='flex items-start justify-between gap-3'>
        <div className='min-w-0'>
          <p className='truncate font-medium'>{props.target.model}</p>
          <p className='text-muted-foreground truncate text-xs'>
            {props.target.channel_name} #{props.target.channel_id}
          </p>
        </div>
        <TargetStatus target={props.target} />
      </div>
      <dl className='grid grid-cols-[auto_minmax(0,1fr)] gap-x-3 gap-y-1 text-xs'>
        <dt className='text-muted-foreground'>{t('Upstream model')}</dt>
        <dd className='truncate'>{props.target.upstream_model}</dd>
        <dt className='text-muted-foreground'>{t('Cost mode')}</dt>
        <dd>
          {props.target.cost_mode
            ? costModeLabel(props.target.cost_mode, t)
            : t('Unknown')}
        </dd>
        <dt className='text-muted-foreground'>{t('Real-person capability')}</dt>
        <dd>
          {realPersonCapabilityLabel(props.target.supports_real_person, t)}
        </dd>
      </dl>
      <TargetExclusionButton {...props} />
    </div>
  )
}

function TargetStatus({ target }: { target: Target }) {
  const { t } = useTranslation()
  const issues = target.issues.filter((issue) => issue !== target.status)
  return (
    <div className='flex max-w-64 flex-wrap gap-1'>
      <Badge variant={target.status === 'matched' ? 'default' : 'outline'}>
        {targetStatusLabel(target.status, t)}
      </Badge>
      {issues.map((issue) => (
        <Badge key={issue} variant='warning'>
          {targetStatusLabel(issue, t)}
        </Badge>
      ))}
    </div>
  )
}

function TargetExclusionButton(props: TargetRowProps) {
  const { t } = useTranslation()
  if (props.excluded || props.target.status === 'excluded') {
    return (
      <Button
        type='button'
        size='sm'
        variant='outline'
        disabled={props.disabled}
        onClick={() => props.onToggleExclusion(props.target, false)}
        aria-label={t('Restore target')}
      >
        <RotateCcw aria-hidden='true' />
        {t('Restore')}
      </Button>
    )
  }
  if (props.target.status !== 'matched') return null
  return (
    <Button
      type='button'
      size='sm'
      variant='outline'
      disabled={props.disabled}
      onClick={() => props.onToggleExclusion(props.target, true)}
      aria-label={t('Exclude target')}
    >
      <Ban aria-hidden='true' />
      {t('Exclude')}
    </Button>
  )
}

function realPersonCapabilityLabel(
  value: boolean | null,
  t: (key: string) => string
): string {
  if (value === true) return t('Supported')
  if (value === false) return t('Not supported')
  return t('Unknown')
}

function costModeLabel(mode: GroupCostMode, t: (key: string) => string) {
  return {
    per_request: t('Per request'),
    per_duration: t('Per duration'),
    per_token: t('Per token'),
    free: t('Free'),
  }[mode]
}

function targetStatusLabel(
  status: GroupRoutingTargetStatus,
  t: (key: string) => string
) {
  return {
    matched: t('Matched'),
    real_person_mismatch: t('Real-person capability mismatch'),
    real_person_unknown: t('Real-person capability unknown'),
    cost_mode_mismatch: t('Cost mode mismatch'),
    cost_rule_missing: t('Cost rule missing'),
    excluded: t('Excluded'),
    target_disabled: t('Target disabled'),
    channel_unavailable: t('Channel unavailable'),
  }[status]
}
