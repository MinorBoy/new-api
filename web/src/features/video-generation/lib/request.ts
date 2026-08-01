import type {
  SeedanceVideoRequest,
  VideoGenerationForm,
  VideoMedia,
} from '../types'

export const VIDEO_MEDIA_LIMITS = {
  images: 9,
  videos: 3,
  audios: 3,
} as const

type ApiKeyModelLimit = {
  model_limits_enabled: boolean
  model_limits?: string | null
}

function isHttpUrl(value: string): boolean {
  try {
    const url = new URL(value)
    return url.protocol === 'http:' || url.protocol === 'https:'
  } catch {
    return false
  }
}

function appendMedia(
  content: SeedanceVideoRequest['content'],
  values: string[],
  type: 'image_url' | 'video_url' | 'audio_url'
): void {
  values
    .map((value) => value.trim())
    .filter(Boolean)
    .forEach((url) => {
      if (type === 'image_url') {
        content.push({
          type,
          image_url: { url },
          role: 'reference_image',
        })
      } else if (type === 'video_url') {
        content.push({
          type,
          video_url: { url },
          role: 'reference_video',
        })
      } else {
        content.push({
          type,
          audio_url: { url },
          role: 'reference_audio',
        })
      }
    })
}

export function validateMediaLimits(media: VideoMedia): string | null {
  const entries: Array<[keyof VideoMedia, string]> = [
    ['images', 'Reference images'],
    ['videos', 'Reference videos'],
    ['audios', 'Reference audios'],
  ]

  for (const [kind, label] of entries) {
    if (media[kind].length > VIDEO_MEDIA_LIMITS[kind]) {
      return `${label} cannot exceed ${VIDEO_MEDIA_LIMITS[kind]} items.`
    }
    if (media[kind].some((url) => url.trim() && !isHttpUrl(url.trim()))) {
      return `${label} must use an HTTP(S) URL.`
    }
  }

  return null
}

export function buildVideoRequest(
  form: VideoGenerationForm
): SeedanceVideoRequest {
  const content: SeedanceVideoRequest['content'] = []
  const prompt = form.prompt.trim()
  if (prompt) content.push({ type: 'text', text: prompt })
  appendMedia(content, form.media.images, 'image_url')
  appendMedia(content, form.media.videos, 'video_url')
  appendMedia(content, form.media.audios, 'audio_url')

  const request: SeedanceVideoRequest = {
    model: form.model.trim(),
    content,
    ratio: form.ratio,
    duration: form.duration,
  }

  if (form.resolution.trim()) request.resolution = form.resolution.trim()
  if (form.generateAudio) request.generate_audio = true
  if (form.watermark) request.watermark = true
  if (form.executionExpiresAfter != null) {
    request.execution_expires_after = form.executionExpiresAfter
  }
  if (form.returnLastFrame) request.return_last_frame = true
  if (form.callbackUrl?.trim()) request.callback_url = form.callbackUrl.trim()
  return request
}

export function buildVideoTaskCurl(
  request: SeedanceVideoRequest,
  origin: string
): string {
  const body = JSON.stringify(request, null, 2).replaceAll("'", "'\\''")
  const baseUrl = origin.replace(/\/+$/, '')
  return [
    `curl -X POST '${baseUrl}/api/v3/contents/generations/tasks' \\`,
    "  -H 'Authorization: Bearer <selected-api-key>' \\",
    "  -H 'Content-Type: application/json' \\",
    `  -d '${body}'`,
  ].join('\n')
}

export function filterModelsForApiKey(
  models: string[],
  apiKey: ApiKeyModelLimit
): string[] {
  const modelLimits = apiKey.model_limits?.trim() || ''
  if (!apiKey.model_limits_enabled || !modelLimits) return models
  const allowed = new Set(
    modelLimits
      .split(',')
      .map((model) => model.trim())
      .filter(Boolean)
  )
  return models.filter((model) => allowed.has(model))
}
