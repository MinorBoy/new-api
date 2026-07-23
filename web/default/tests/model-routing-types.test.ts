import { describe, expect, test } from 'bun:test'

import {
  fromPolicyResponse,
  routeTargetFormSchema,
  routingPolicyResponseSchema,
  toWriteRequest,
} from '../src/features/model-routing/types'

describe('model routing data contract', () => {
  test('converts UI-only duration and real-person state to the API shape', () => {
    const payload = toWriteRequest({
      group_name: '分组A',
      model: 'doubao-seedance-2-0-260128',
      enabled: false,
      defaults: {
        output_resolution: '720p',
        duration_seconds: 10,
        aspect_ratio: '9:16',
      },
      targets: [
        {
          channel_id: 11,
          channel_name: 'A1',
          name: 'A1 720p',
          upstream_model: 'provider-720p',
          target_priority: 100,
          enabled: true,
          output_resolutions: ['720p'],
          generation_resolution: undefined,
          upscaled: false,
          durations: { mode: 'values', values: [5, 10] },
          aspect_ratios: [],
          reference_limits: { images: 9, videos: 3, audios: 3 },
          supports_real_person: 'unknown',
        },
      ],
    })

    expect(payload.targets[0]?.constraints.durations).toEqual({
      values: [5, 10],
    })
    expect(payload.targets[0]?.constraints).not.toHaveProperty(
      'generation_resolution'
    )
    expect(payload.targets[0]?.constraints.supports_real_person).toBeNull()
  })

  test('restores range mode and tri-state support from an API response', () => {
    const response = routingPolicyResponseSchema.parse({
      success: true,
      data: {
        id: 7,
        group_name: '分组A',
        model: 'doubao-seedance-2-0-260128',
        enabled: true,
        defaults: {
          output_resolution: '1080p',
          duration_seconds: 10,
          aspect_ratio: '16:9',
        },
        targets: [
          {
            id: 21,
            channel_id: 12,
            channel_name: 'A1_copy',
            name: 'upscaled 1080p',
            upstream_model: 'provider-1080p',
            target_priority: 110,
            enabled: true,
            constraints: {
              output_resolutions: ['1080p'],
              generation_resolution: '720p',
              upscaled: true,
              durations: { min: 4, max: 15 },
              aspect_ratios: [],
              reference_limits: { images: 4, videos: 3, audios: 1 },
              supports_real_person: false,
            },
          },
        ],
        created_at: 1,
        updated_at: 2,
      },
    })

    const form = fromPolicyResponse(response.data)
    expect(form.targets[0]?.durations).toEqual({
      mode: 'range',
      values: [],
      min: 4,
      max: 15,
    })
    expect(form.targets[0]?.supports_real_person).toBe('no')
  })

  test('rejects upscaled targets without a distinct generation resolution', () => {
    const result = routeTargetFormSchema.safeParse({
      channel_id: 12,
      name: 'invalid upscale',
      upstream_model: 'provider-1080p',
      target_priority: 100,
      enabled: true,
      output_resolutions: ['1080p'],
      generation_resolution: '1080p',
      upscaled: true,
      durations: { mode: 'range', values: [], min: 4, max: 15 },
      aspect_ratios: [],
      reference_limits: { images: 4, videos: 3, audios: 1 },
      supports_real_person: 'yes',
    })

    expect(result.success).toBe(false)
  })
})
