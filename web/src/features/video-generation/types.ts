export type VideoMedia = {
  images: string[]
  videos: string[]
  audios: string[]
}

export type VideoImageSource = 'url' | 'asset'

export type VideoGenerationForm = {
  model: string
  prompt: string
  imageSource: VideoImageSource
  assetIds: string[]
  media: VideoMedia
  resolution: string
  ratio: string
  duration: number
  executionExpiresAfter?: number
  generateAudio: boolean
  watermark: boolean
  returnLastFrame: boolean
  callbackUrl?: string
}

export type VideoContentItem =
  | { type: 'text'; text: string }
  | {
      type: 'image_url'
      image_url: { url: string }
      role: 'reference_image'
    }
  | {
      type: 'video_url'
      video_url: { url: string }
      role: 'reference_video'
    }
  | {
      type: 'audio_url'
      audio_url: { url: string }
      role: 'reference_audio'
    }

export type SeedanceVideoRequest = {
  model: string
  content: VideoContentItem[]
  resolution?: string
  ratio: string
  duration: number
  execution_expires_after?: number
  generate_audio?: boolean
  watermark?: boolean
  return_last_frame?: boolean
  callback_url?: string
}

export type VideoTaskStatus =
  | 'queued'
  | 'running'
  | 'succeeded'
  | 'failed'
  | 'expired'
  | 'cancelled'
  | 'unknown'

export type VideoTaskResponse = {
  id?: string
  task_id?: string
  status?: string
  model?: string
  content?: {
    video_url?: string
    last_frame_url?: string
  }
  data?: {
    id?: string
    status?: string
    content?: {
      video_url?: string
      last_frame_url?: string
    }
  }
  error?: { message?: string }
  message?: string
  [key: string]: unknown
}

export type VideoTaskRecord = {
  clientId: string
  taskId: string
  status: VideoTaskStatus
  request: SeedanceVideoRequest
  createdAt: number
  apiKey: string
  response?: VideoTaskResponse
  error?: string
}
