import type { TFunction } from 'i18next'
import { z } from 'zod'

import { VIDEO_MEDIA_LIMITS } from './request'

function optionalHttpUrl(t: TFunction) {
  return z.string().refine(
    (value) => {
      if (!value.trim()) return true
      try {
        const url = new URL(value)
        return url.protocol === 'http:' || url.protocol === 'https:'
      } catch {
        return false
      }
    },
    { message: t('Use a valid HTTP(S) URL') }
  )
}

export function createVideoGenerationSchema(t: TFunction) {
  return z
    .object({
      model: z.string().trim().min(1, t('Select a model')),
      prompt: z.string().max(4000, t('Prompt cannot exceed 4000 characters')),
      media: z.object({
        images: z
          .array(optionalHttpUrl(t))
          .max(
            VIDEO_MEDIA_LIMITS.images,
            t('Up to 9 reference images are allowed')
          ),
        videos: z
          .array(optionalHttpUrl(t))
          .max(
            VIDEO_MEDIA_LIMITS.videos,
            t('Up to 3 reference videos are allowed')
          ),
        audios: z
          .array(optionalHttpUrl(t))
          .max(
            VIDEO_MEDIA_LIMITS.audios,
            t('Up to 3 reference audio files are allowed')
          ),
      }),
      resolution: z.string(),
      ratio: z.string().min(1),
      duration: z
        .number()
        .refine((value) => value === -1 || (value >= 4 && value <= 15), {
          message: t('Duration must be -1 or between 4 and 15 seconds'),
        }),
      executionExpiresAfter: z
        .union([
          z
            .number()
            .min(3600, t('Task expiry must be at least 3600 seconds'))
            .max(259200, t('Task expiry cannot exceed 259200 seconds')),
          z.nan(),
        ])
        .optional()
        .transform((value) => (Number.isNaN(value) ? undefined : value)),
      generateAudio: z.boolean(),
      watermark: z.boolean(),
      returnLastFrame: z.boolean(),
      callbackUrl: optionalHttpUrl(t).optional(),
    })
    .superRefine((value, context) => {
      const hasMedia = Object.values(value.media).some((items) =>
        items.some((item) => item.trim())
      )
      if (!value.prompt.trim() && !hasMedia) {
        context.addIssue({
          code: 'custom',
          path: ['prompt'],
          message: t('Enter a prompt or at least one reference URL'),
        })
      }
    })
}
