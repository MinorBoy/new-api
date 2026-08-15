import type { TFunction } from 'i18next'
import { z } from 'zod'

import { getVideoMediaLimits, supportsRoleAssets } from './request'

const PROJECT_ASSET_ID_PATTERN =
  /^asset-(?:[0-9a-f]{32}|[0-9]{14}-[a-z0-9]{5})$/

const MEDIA_LABELS = {
  images: 'Reference images',
  videos: 'Reference videos',
  audios: 'Reference audios',
} as const

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
      imageSource: z.enum(['url', 'asset']),
      assetIds: z.array(z.string()),
      media: z.object({
        images: z.array(optionalHttpUrl(t)),
        videos: z.array(optionalHttpUrl(t)),
        audios: z.array(optionalHttpUrl(t)),
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
      const limits = getVideoMediaLimits(value.model)
      for (const kind of ['images', 'videos', 'audios'] as const) {
        if (value.imageSource === 'asset' && kind !== 'audios') continue
        if (value.media[kind].length <= limits[kind]) continue
        context.addIssue({
          code: 'custom',
          path: ['media', kind],
          message: t('{{kind}} cannot exceed {{count}} items', {
            kind: t(MEDIA_LABELS[kind]),
            count: limits[kind],
          }),
        })
      }

      if (value.imageSource === 'asset') {
        if (!supportsRoleAssets(value.model)) {
          context.addIssue({
            code: 'custom',
            path: ['imageSource'],
            message: t('This model does not support role assets'),
          })
        }
        if (value.assetIds.length > 9) {
          context.addIssue({
            code: 'custom',
            path: ['assetIds'],
            message: t('Role assets cannot exceed {{count}} items', {
              count: 9,
            }),
          })
        }
        value.assetIds.forEach((assetId, index) => {
          if (PROJECT_ASSET_ID_PATTERN.test(assetId)) return
          context.addIssue({
            code: 'custom',
            path: ['assetIds', index],
            message: t('Use a valid project asset ID'),
          })
        })
      }

      const hasAudio = value.media.audios.some((item) => item.trim())
      const hasMedia =
        hasAudio ||
        (value.imageSource === 'asset'
          ? value.assetIds.length > 0
          : value.media.images.some((item) => item.trim()) ||
            value.media.videos.some((item) => item.trim()))
      if (!value.prompt.trim() && !hasMedia) {
        context.addIssue({
          code: 'custom',
          path: ['prompt'],
          message: t('Enter a prompt or at least one reference'),
        })
      }
    })
}
