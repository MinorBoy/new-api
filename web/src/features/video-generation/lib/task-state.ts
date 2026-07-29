import type { VideoTaskResponse, VideoTaskStatus } from '../types'

type TaskResult = {
  videoUrl?: string
  lastFrameUrl?: string
}

function getTaskData(response: VideoTaskResponse): VideoTaskResponse {
  return response.data ? { ...response, ...response.data } : response
}

export function getVideoTaskId(response: VideoTaskResponse): string {
  const task = getTaskData(response)
  return task.id || task.task_id || ''
}

export function getVideoTaskStatus(
  response: VideoTaskResponse
): VideoTaskStatus {
  const status = getTaskData(response).status?.toLowerCase()
  switch (status) {
    case 'queued':
    case 'pending':
      return 'queued'
    case 'running':
    case 'processing':
    case 'in_progress':
      return 'running'
    case 'succeeded':
    case 'success':
    case 'completed':
      return 'succeeded'
    case 'failed':
    case 'error':
      return 'failed'
    case 'expired':
      return 'expired'
    case 'cancelled':
    case 'canceled':
      return 'cancelled'
    default:
      return 'unknown'
  }
}

export function getVideoTaskResult(response: VideoTaskResponse): TaskResult {
  const task = getTaskData(response)
  return {
    videoUrl: task.content?.video_url,
    lastFrameUrl: task.content?.last_frame_url,
  }
}
