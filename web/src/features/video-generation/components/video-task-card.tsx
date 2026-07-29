import {
  Check,
  CircleAlert,
  Clock3,
  Copy,
  Film,
  LoaderCircle,
  RefreshCw,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'

import { getVideoTaskResult } from '../lib/task-state'
import type { VideoTaskRecord, VideoTaskStatus } from '../types'

type VideoTaskCardProps = {
  task: VideoTaskRecord
  onRefresh: () => void
  onCopy: (value: string) => void
}

const STATUS_COPY: Record<VideoTaskStatus, string> = {
  queued: 'Queued',
  running: 'Generating',
  succeeded: 'Completed',
  failed: 'Failed',
  expired: 'Expired',
  cancelled: 'Cancelled',
  unknown: 'Unknown',
}

function statusVariant(status: VideoTaskStatus) {
  if (status === 'failed' || status === 'expired' || status === 'cancelled') {
    return 'destructive' as const
  }
  if (status === 'queued' || status === 'running') return 'warning' as const
  return 'secondary' as const
}

function StatusIcon({ status }: { status: VideoTaskStatus }) {
  if (status === 'succeeded') return <Check aria-hidden='true' />
  if (status === 'failed' || status === 'expired' || status === 'cancelled') {
    return <CircleAlert aria-hidden='true' />
  }
  if (status === 'queued') return <Clock3 aria-hidden='true' />
  return <LoaderCircle aria-hidden='true' className='animate-spin' />
}

export function VideoTaskCard(props: VideoTaskCardProps) {
  const { t } = useTranslation()
  const result = props.task.response
    ? getVideoTaskResult(props.task.response)
    : { videoUrl: undefined, lastFrameUrl: undefined }
  const responseJson = props.task.response
    ? JSON.stringify(props.task.response, null, 2)
    : ''

  return (
    <Card className='min-w-0'>
      <CardHeader className='gap-3 border-b pb-4'>
        <div className='flex flex-wrap items-start justify-between gap-3'>
          <div className='min-w-0'>
            <CardTitle className='flex items-center gap-2'>
              <Film
                className='text-primary size-4 shrink-0'
                aria-hidden='true'
              />
              {t('Video task')}
            </CardTitle>
            <CardDescription className='mt-1 font-mono text-xs break-all'>
              {props.task.taskId || t('Waiting for task ID')}
            </CardDescription>
          </div>
          <Badge variant={statusVariant(props.task.status)}>
            <StatusIcon status={props.task.status} />
            {t(STATUS_COPY[props.task.status])}
          </Badge>
        </div>
        <div className='text-muted-foreground flex flex-wrap gap-x-4 gap-y-1 text-xs'>
          <span>{props.task.request.model}</span>
          <span>{props.task.request.duration}s</span>
          <span>{new Date(props.task.createdAt).toLocaleTimeString()}</span>
        </div>
      </CardHeader>

      <CardContent className='grid gap-4 lg:grid-cols-[minmax(0,1.3fr)_minmax(220px,0.7fr)]'>
        {result.videoUrl ? (
          <div className='overflow-hidden rounded-lg border bg-black'>
            <video
              className='aspect-video w-full object-contain'
              controls
              playsInline
              preload='metadata'
              src={result.videoUrl}
            />
          </div>
        ) : (
          <div className='bg-muted/40 text-muted-foreground flex min-h-36 items-center justify-center rounded-lg border text-sm'>
            {props.task.error ||
              t('Video preview will appear when the task completes')}
          </div>
        )}

        <div className='min-w-0 space-y-3'>
          {result.lastFrameUrl && (
            <img
              alt={t('Generated last frame')}
              className='max-h-44 w-full rounded-lg border object-contain'
              loading='lazy'
              src={result.lastFrameUrl}
            />
          )}
          <div className='flex flex-wrap gap-2'>
            <Button
              size='sm'
              type='button'
              variant='outline'
              onClick={props.onRefresh}
            >
              <RefreshCw aria-hidden='true' />
              {t('Refresh')}
            </Button>
            {result.videoUrl && (
              <Button
                size='sm'
                type='button'
                variant='outline'
                onClick={() => props.onCopy(result.videoUrl || '')}
              >
                <Copy aria-hidden='true' />
                {t('Copy video URL')}
              </Button>
            )}
          </div>
          {responseJson && (
            <details className='bg-muted/20 rounded-lg border p-2'>
              <summary className='cursor-pointer text-xs font-medium'>
                {t('Response JSON')}
              </summary>
              <pre className='mt-2 max-h-56 overflow-auto text-[11px] leading-relaxed break-all whitespace-pre-wrap'>
                {responseJson}
              </pre>
            </details>
          )}
        </div>
      </CardContent>
    </Card>
  )
}
