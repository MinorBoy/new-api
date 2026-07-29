import type { VideoTaskStatus } from '../types'

const TERMINAL_VIDEO_TASK_STATUSES = new Set<VideoTaskStatus>([
  'succeeded',
  'failed',
  'expired',
  'cancelled',
])

type NextVideoPollOptions = {
  isCurrent: boolean
  status: VideoTaskStatus
  requestFailed: boolean
}

export function isTerminalVideoTaskStatus(status: VideoTaskStatus): boolean {
  return TERMINAL_VIDEO_TASK_STATUSES.has(status)
}

export function shouldScheduleNextVideoPoll(
  options: NextVideoPollOptions
): boolean {
  return (
    options.isCurrent &&
    !options.requestFailed &&
    !isTerminalVideoTaskStatus(options.status)
  )
}
