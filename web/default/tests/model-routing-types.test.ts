import { describe, expect, test } from 'bun:test'

import {
  clearUnavailableTargetChannels,
  copyPolicyForm,
  copyTargetForm,
  createEmptyPolicyForm,
  createEmptyTarget,
  fromPolicyResponse,
  normalizeRoutingGroups,
  routingPolicyFormSchema,
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
    expect(payload.targets[0]?.constraints).not.toHaveProperty('upscaled')
    expect(payload.targets[0]?.constraints.supports_real_person).toBeNull()
  })

  test('restores range mode and ignores legacy super-resolution properties', () => {
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
    expect(response.data.targets[0]?.constraints).toEqual({
      output_resolutions: ['1080p'],
      durations: { min: 4, max: 15 },
      aspect_ratios: [],
      reference_limits: { images: 4, videos: 3, audios: 1 },
      supports_real_person: false,
    })
  })

  test('creates independent policy and target defaults', () => {
    const first = createEmptyPolicyForm()
    const second = createEmptyPolicyForm()
    first.targets.push(createEmptyTarget())

    expect(second.targets).toEqual([])
    expect(first.defaults).not.toBe(second.defaults)
    expect(first.targets[0]?.durations).toEqual({
      mode: 'range',
      values: [],
      min: 4,
      max: 15,
    })
  })

  test('requires at least one target when a policy is enabled', () => {
    const policy = createEmptyPolicyForm()
    policy.group_name = '分组A'
    policy.enabled = true

    const result = routingPolicyFormSchema.safeParse(policy)

    expect(result.success).toBe(false)
    if (!result.success) {
      expect(result.error.issues[0]?.path).toEqual(['targets'])
    }
  })

  test('prepares a disabled copy without policy or target ids', () => {
    const response = routingPolicyResponseSchema.parse({
      success: true,
      data: {
        id: 7,
        group_name: '分组A',
        model: 'doubao-seedance-2-0-260128',
        enabled: true,
        defaults: {
          output_resolution: '720p',
          duration_seconds: 10,
          aspect_ratio: '16:9',
        },
        targets: [
          {
            id: 21,
            channel_id: 11,
            channel_name: 'A1',
            name: 'A1 target',
            upstream_model: 'provider-model',
            target_priority: 100,
            enabled: true,
            constraints: {
              output_resolutions: ['720p'],
              durations: { min: 4, max: 15 },
              aspect_ratios: [],
              reference_limits: { images: 9, videos: 3, audios: 3 },
              supports_real_person: true,
            },
          },
        ],
        created_at: 1,
        updated_at: 2,
      },
    })

    const copy = copyPolicyForm(response.data)

    expect(copy.id).toBeUndefined()
    expect(copy.enabled).toBe(false)
    expect(copy.targets[0]?.id).toBeUndefined()
    expect(copy.targets[0]?.name).toBe('A1 target')
  })

  test('copies one target with independent constraints and a new name', () => {
    const source = createEmptyTarget()
    source.id = 21
    source.name = 'A1 target'

    const copy = copyTargetForm(source)
    copy.durations.values.push(10)
    copy.reference_limits.images = 4

    expect(copy.id).toBeUndefined()
    expect(copy.name).toBe('A1 target copy')
    expect(source.durations.values).toEqual([])
    expect(source.reference_limits.images).toBe(9)
  })

  test('clears only target channels missing from the latest candidates', () => {
    const first = createEmptyTarget()
    first.channel_id = 11
    first.channel_name = 'A1'
    const second = createEmptyTarget()
    second.channel_id = 12
    second.channel_name = 'A1_copy'

    const result = clearUnavailableTargetChannels([first, second], [12])

    expect(result[0]?.channel_id).toBe(0)
    expect(result[0]?.channel_name).toBe('')
    expect(result[1]?.channel_id).toBe(12)
    expect(result[1]?.channel_name).toBe('A1_copy')
  })

  test('normalizes, deduplicates, sorts, and filters routing groups', () => {
    expect(
      normalizeRoutingGroups(
        [' group-b ', 'auto', 'Group-A', 'group-b', '', 'AUTO'],
        ''
      )
    ).toEqual(['Group-A', 'group-b'])
  })

  test('preserves the current group when it is missing from the API', () => {
    expect(normalizeRoutingGroups(['default', 'vip'], 'legacy-group')).toEqual([
      'default',
      'legacy-group',
      'vip',
    ])
  })
})
