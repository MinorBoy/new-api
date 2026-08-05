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
import { Check, Copy, Eye, Music } from 'lucide-react'
/* eslint-disable react-refresh/only-export-components */
import { useId, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { StatusBadge } from '@/components/status-badge'
import { Avatar, AvatarFallback } from '@/components/ui/avatar'
import { Button } from '@/components/ui/button'
import {
  HoverCard,
  HoverCardContent,
  HoverCardTrigger,
} from '@/components/ui/hover-card'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Separator } from '@/components/ui/separator'
import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'
import { getUserAvatarFallback, getUserAvatarStyle } from '@/lib/avatar'
import { formatLogQuota, formatTimestampToDate } from '@/lib/format'
import { cn } from '@/lib/utils'

import { TASK_ACTIONS, TASK_STATUS } from '../../constants'
import { taskActionMapper, taskStatusMapper } from '../../lib/mappers'
import type { PublicTaskResult, TaskLog } from '../../types'
import {
  AudioPreviewDialog,
  type AudioClip,
} from '../dialogs/audio-preview-dialog'
import { FailReasonDialog } from '../dialogs/fail-reason-dialog'
import { TaskAuditDataDialog } from '../dialogs/task-audit-data-dialog'
import { ModelBadge } from '../model-badge'
import { useUsageLogsContext } from '../usage-logs-provider'
import {
  createDurationColumn,
  createChannelColumn,
  createProgressColumn,
} from './column-helpers'

function parseTaskData(data: unknown): unknown[] {
  if (Array.isArray(data)) return data
  if (typeof data === 'string') {
    try {
      const parsed = JSON.parse(data)
      return Array.isArray(parsed) ? parsed : []
    } catch {
      return []
    }
  }
  return []
}

function projectPublicTaskResult(log: TaskLog): PublicTaskResult | null {
  let data = log.user_response_data
  if (typeof data === 'string') {
    try {
      data = JSON.parse(data)
    } catch {
      return null
    }
  }
  if (!data || typeof data !== 'object' || Array.isArray(data)) return null

  const source = data as Record<string, unknown>
  const sourceUsage =
    source.usage &&
    typeof source.usage === 'object' &&
    !Array.isArray(source.usage)
      ? (source.usage as Record<string, unknown>)
      : {}
  const sourceContent =
    source.content &&
    typeof source.content === 'object' &&
    !Array.isArray(source.content)
      ? (source.content as Record<string, unknown>)
      : {}
  const numberValue = (value: unknown) =>
    typeof value === 'number' && Number.isFinite(value) ? value : 0
  const stringValue = (value: unknown) =>
    typeof value === 'string' ? value : ''
  const taskStatus = String(log.status || '').toUpperCase()
  const sourceStatus = stringValue(source.status)
  let status = 'queued'
  if (taskStatus === TASK_STATUS.SUCCESS) {
    status = 'succeeded'
  } else if (taskStatus === TASK_STATUS.FAILURE) {
    status = 'failed'
  } else if (['queued', 'running', 'cancelled'].includes(sourceStatus)) {
    status = sourceStatus
  }

  const result: PublicTaskResult = {
    id: log.task_id,
    model: log.request_model || '',
    status,
    usage: {
      completion_tokens: numberValue(sourceUsage.completion_tokens),
      total_tokens: numberValue(sourceUsage.total_tokens),
    },
    created_at: numberValue(source.created_at),
    updated_at: numberValue(source.updated_at),
    seed: numberValue(source.seed),
    resolution: stringValue(source.resolution),
    ratio: stringValue(source.ratio),
    duration: numberValue(source.duration),
    framespersecond: numberValue(source.framespersecond),
    service_tier: stringValue(source.service_tier),
    execution_expires_after: numberValue(source.execution_expires_after),
    generate_audio: source.generate_audio === true,
    draft: source.draft === true,
    priority: numberValue(source.priority),
  }

  if (status === 'succeeded') {
    const candidateURL = stringValue(sourceContent.video_url)
    const expectedPath = `/v1/videos/${encodeURIComponent(log.task_id)}/content`
    if (candidateURL === expectedPath) {
      result.content = { video_url: expectedPath }
    } else if (candidateURL) {
      try {
        const parsedURL = new URL(candidateURL, window.location.origin)
        if (
          parsedURL.origin === window.location.origin &&
          parsedURL.pathname === expectedPath
        ) {
          result.content = { video_url: expectedPath }
        }
      } catch {
        // Invalid or non-local URLs are not part of the public task contract.
      }
    }
  } else if (status === 'failed') {
    result.error = { code: 'task_failed', message: 'task failed' }
  }

  return result
}

function AudioPreviewCell({ log }: { log: TaskLog }) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const clips = useMemo(() => {
    const data = parseTaskData(log.data)
    return data.filter(
      (c) =>
        c && typeof c === 'object' && (c as Record<string, unknown>).audio_url
    )
  }, [log.data])

  if (clips.length === 0) return null

  return (
    <>
      <button
        type='button'
        className='group flex items-center gap-1 text-left text-xs'
        onClick={() => setOpen(true)}
      >
        <Music className='text-muted-foreground size-3' />
        <span className='text-foreground leading-snug group-hover:underline'>
          {t('Click to preview audio')}
        </span>
      </button>
      <AudioPreviewDialog
        open={open}
        onOpenChange={setOpen}
        clips={clips as AudioClip[]}
      />
    </>
  )
}

function TaskAuditDataCell({ data, title }: { data: unknown; title: string }) {
  const { t } = useTranslation()
  const triggerId = useId()
  const [previewOpen, setPreviewOpen] = useState(false)
  const [dialogOpen, setDialogOpen] = useState(false)
  const formattedData = useMemo(() => {
    if (typeof data === 'string') {
      try {
        return JSON.stringify(JSON.parse(data), null, 2)
      } catch {
        return data
      }
    }

    return JSON.stringify(data, null, 2) ?? ''
  }, [data])
  const { copiedText, copyToClipboard } = useCopyToClipboard({ notify: false })

  if (data == null || data === '') {
    return <span className='text-muted-foreground/60 text-xs'>-</span>
  }

  return (
    <>
      <HoverCard
        open={previewOpen}
        triggerId={triggerId}
        onOpenChange={(open, eventDetails) => {
          if (eventDetails.reason !== 'trigger-press') {
            setPreviewOpen(open)
          }
        }}
      >
        <HoverCardTrigger
          id={triggerId}
          delay={250}
          closeDelay={150}
          render={
            <button
              type='button'
              className='text-foreground inline-flex max-w-full items-center gap-1 text-xs hover:underline focus-visible:underline'
              onClick={() => {
                setPreviewOpen(false)
                setDialogOpen(true)
              }}
              onFocus={() => setPreviewOpen(true)}
              title={t('View')}
            />
          }
        >
          <Eye className='size-3 shrink-0' aria-hidden='true' />
          <span className='truncate'>{t('View')}</span>
        </HoverCardTrigger>
        <HoverCardContent
          align='start'
          className='w-[min(35rem,calc(100vw-2rem))] overflow-hidden p-0'
        >
          <div className='flex h-10 items-center justify-between gap-3 px-3'>
            <span className='truncate text-sm font-medium'>{t(title)}</span>
            <Button
              variant='ghost'
              size='sm'
              className='size-8 shrink-0 p-0'
              onClick={() => copyToClipboard(formattedData)}
              title={t('Copy to clipboard')}
              aria-label={t('Copy to clipboard')}
            >
              {copiedText === formattedData ? (
                <Check aria-hidden='true' />
              ) : (
                <Copy aria-hidden='true' />
              )}
            </Button>
          </div>
          <Separator />
          <ScrollArea className='h-[min(26rem,calc(100vh-8rem))]'>
            <pre className='overflow-wrap-anywhere min-w-0 p-3 font-mono text-xs leading-relaxed break-all whitespace-pre-wrap'>
              {formattedData}
            </pre>
          </ScrollArea>
        </HoverCardContent>
      </HoverCard>
      <TaskAuditDataDialog
        formattedData={formattedData}
        title={title}
        open={dialogOpen}
        onOpenChange={setDialogOpen}
      />
    </>
  )
}

export function useTaskLogsColumns(isAdmin: boolean): ColumnDef<TaskLog>[] {
  const { t } = useTranslation()
  const requestModelColumn: ColumnDef<TaskLog> = {
    accessorKey: 'request_model',
    header: t('Request Model'),
    cell: ({ row }) => {
      const requestModel = row.original.request_model
      if (!requestModel) {
        return <span className='text-muted-foreground/60 text-xs'>-</span>
      }
      return <ModelBadge modelName={requestModel} />
    },
    size: 180,
  }
  const requestDataColumn: ColumnDef<TaskLog> = {
    accessorKey: 'user_request_data',
    header: t('Request Data'),
    cell: ({ row }) => (
      <TaskAuditDataCell
        data={row.original.user_request_data}
        title='Request Data'
      />
    ),
    size: 150,
  }
  const taskDetailsColumn: ColumnDef<TaskLog> = {
    accessorKey: 'user_response_data',
    header: t('Task Details'),
    cell: ({ row }) => (
      <TaskAuditDataCell
        data={
          isAdmin
            ? row.original.user_response_data
            : projectPublicTaskResult(row.original)
        }
        title='Task Details'
      />
    ),
    size: 180,
  }
  const columns: ColumnDef<TaskLog>[] = [
    {
      accessorKey: 'submit_time',
      header: t('Submit Time'),
      cell: ({ row }) => {
        const log = row.original
        const submitTime = row.getValue('submit_time') as number

        return (
          <div className='flex min-w-0 flex-col gap-0.5'>
            <span className='truncate font-mono text-xs tabular-nums'>
              {formatTimestampToDate(submitTime, 'seconds')}
            </span>
            {log.finish_time ? (
              <span className='text-muted-foreground/60 truncate font-mono text-[11px] tabular-nums'>
                {formatTimestampToDate(log.finish_time, 'seconds')}
              </span>
            ) : (
              <span className='text-muted-foreground/50 text-[11px]'>-</span>
            )}
          </div>
        )
      },
      size: 180,
    },
  ]

  if (isAdmin) {
    columns.push(
      createChannelColumn<TaskLog>({ headerLabel: t('Channel') }),
      {
        accessorKey: 'request_path',
        header: t('Endpoint'),
        cell: ({ row }) => {
          const requestPath = row.original.request_path
          if (!requestPath) {
            return <span className='text-muted-foreground/60 text-xs'>-</span>
          }
          return (
            <span
              className='block max-w-[240px] truncate font-mono text-xs'
              title={requestPath}
            >
              {t('Inbound')}
              {requestPath}
            </span>
          )
        },
        size: 240,
      },
      {
        id: 'user',
        header: t('User'),
        accessorFn: (row) => row.username || row.user_id,
        cell: function UserCell({ row }) {
          const { sensitiveVisible, setSelectedUserId, setUserInfoDialogOpen } =
            useUsageLogsContext()
          const log = row.original
          const displayName = log.username || String(log.user_id || '?')

          return (
            <button
              type='button'
              className='flex items-center gap-1.5 text-left'
              onClick={(e) => {
                e.stopPropagation()
                setSelectedUserId(log.user_id ?? null)
                setUserInfoDialogOpen(true)
              }}
            >
              <Avatar className='ring-border/60 size-6 ring-1 max-sm:hidden'>
                <AvatarFallback
                  className={cn(
                    'text-[11px] font-semibold',
                    !sensitiveVisible && 'bg-muted text-muted-foreground'
                  )}
                  style={
                    sensitiveVisible
                      ? getUserAvatarStyle(displayName)
                      : undefined
                  }
                >
                  {sensitiveVisible ? getUserAvatarFallback(displayName) : '•'}
                </AvatarFallback>
              </Avatar>
              <span className='text-muted-foreground truncate text-sm hover:underline'>
                {sensitiveVisible ? displayName : '••••'}
              </span>
            </button>
          )
        },
      },
      requestModelColumn,
      requestDataColumn,
      {
        accessorKey: 'upstream_response_data',
        header: t('Upstream Response (Create Task)'),
        cell: ({ row }) => (
          <TaskAuditDataCell
            data={row.original.upstream_response_data}
            title='Upstream Response (Create Task)'
          />
        ),
        size: 180,
      },
      taskDetailsColumn
    )
  } else {
    columns.push(requestModelColumn, taskDetailsColumn)
  }

  columns.push(
    {
      accessorKey: 'task_id',
      header: t('Task ID'),
      cell: ({ row }) => {
        const log = row.original
        const taskId = row.getValue('task_id') as string
        if (!taskId) {
          return <span className='text-muted-foreground/60 text-xs'>-</span>
        }
        return (
          <div className='flex max-w-[170px] flex-col gap-0.5'>
            <StatusBadge
              label={taskId}
              copyText={taskId}
              variant='neutral'
              size='sm'
              className='border-border/60 bg-muted/30 !text-foreground max-w-full truncate rounded-md border px-1.5 py-0.5 font-mono'
            />
            <span className='text-muted-foreground/60 truncate text-[11px]'>
              {isAdmin && log.platform ? `${t(log.platform)} · ` : ''}
              {t(taskActionMapper.getLabel(log.action))}
            </span>
          </div>
        )
      },
      meta: { mobileTitle: true },
    },
    createDurationColumn<TaskLog>({
      submitTimeKey: 'submit_time',
      finishTimeKey: 'finish_time',
      unit: 'seconds',
      headerLabel: t('Duration'),
      warningThresholdSec: 300,
    }),
    {
      accessorKey: 'status',
      header: t('Status'),
      cell: ({ row }) => {
        const status = row.getValue('status') as string
        return (
          <StatusBadge
            label={t(taskStatusMapper.getLabel(status, status || 'Submitting'))}
            variant={taskStatusMapper.getVariant(status)}
            size='sm'
            copyable={false}
            className='-ml-1.5'
          />
        )
      },
    },
    createProgressColumn<TaskLog>({ headerLabel: t('Progress') }),
    {
      accessorKey: 'quota',
      header: t('Consumption'),
      cell: ({ row }) => (
        <span className='font-mono text-xs tabular-nums'>
          {formatLogQuota(row.original.quota || 0)}
        </span>
      ),
      size: 120,
    },
    {
      accessorKey: 'fail_reason',
      header: t('Details'),
      cell: function DetailsCell({ row }) {
        const log = row.original
        const failReason = row.getValue('fail_reason') as string
        const status = log.status
        const [dialogOpen, setDialogOpen] = useState(false)

        const isSunoSuccess =
          log.platform === 'suno' && status === TASK_STATUS.SUCCESS
        if (isSunoSuccess) {
          const data = parseTaskData(log.data)
          if (
            data.some(
              (c) =>
                c &&
                typeof c === 'object' &&
                (c as Record<string, unknown>).audio_url
            )
          ) {
            return <AudioPreviewCell log={log} />
          }
        }

        const isVideoTask =
          log.action === TASK_ACTIONS.GENERATE ||
          log.action === TASK_ACTIONS.TEXT_GENERATE ||
          log.action === TASK_ACTIONS.FIRST_TAIL_GENERATE ||
          log.action === TASK_ACTIONS.REFERENCE_GENERATE ||
          log.action === TASK_ACTIONS.REMIX_GENERATE
        const isSuccess = status === TASK_STATUS.SUCCESS
        const isUrl = failReason?.startsWith('http')
        const publicResult = projectPublicTaskResult(log)
        const publicVideoURL = publicResult?.content?.video_url

        if (
          isSuccess &&
          isVideoTask &&
          (publicVideoURL || (isAdmin && isUrl))
        ) {
          const videoUrl = publicVideoURL || `/v1/videos/${log.task_id}/content`
          return (
            <a
              href={videoUrl}
              target='_blank'
              rel='noopener noreferrer'
              className='text-foreground text-xs hover:underline'
            >
              {t('Click to preview video')}
            </a>
          )
        }

        if (!failReason) {
          return <span className='text-muted-foreground/60 text-xs'>-</span>
        }

        return (
          <>
            <button
              type='button'
              className='group flex max-w-[200px] items-center gap-1 text-left text-xs'
              onClick={() => setDialogOpen(true)}
              title={t('Click to view full error message')}
            >
              <span className='truncate leading-snug text-red-600 group-hover:underline dark:text-red-400'>
                {failReason}
              </span>
            </button>
            <FailReasonDialog
              failReason={failReason}
              open={dialogOpen}
              onOpenChange={setDialogOpen}
            />
          </>
        )
      },
      size: 200,
      maxSize: 220,
    }
  )

  return columns
}
