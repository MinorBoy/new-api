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
import { t } from 'i18next'
import { z } from 'zod'

export const CANONICAL_SEEDANCE_MODELS = [
  'doubao-seedance-2-0-260128',
  'doubao-seedance-2-0-fast-260128',
  'doubao-seedance-2-0-mini-260615',
  'doubao-seedance-2-5-260628',
] as const

export type CanonicalSeedanceModel = (typeof CANONICAL_SEEDANCE_MODELS)[number]

const SEEDANCE_20_REFERENCE_LIMITS = {
  images: 9,
  videos: 3,
  audios: 3,
} as const
const SEEDANCE_25_REFERENCE_LIMITS = {
  images: 30,
  videos: 10,
  audios: 10,
} as const

export function getSeedanceReferenceLimits(model: CanonicalSeedanceModel) {
  return model === 'doubao-seedance-2-5-260628'
    ? SEEDANCE_25_REFERENCE_LIMITS
    : SEEDANCE_20_REFERENCE_LIMITS
}

export const OUTPUT_RESOLUTIONS = ['480p', '720p', '1080p', '4k'] as const
export const MAX_TASK_DURATION_SECONDS = 3600
export const ASPECT_RATIOS = [
  '16:9',
  '4:3',
  '1:1',
  '3:4',
  '9:16',
  '21:9',
  'adaptive',
] as const
export const INPUT_MODES = [
  'text',
  'first_frame',
  'first_last_frames',
  'omni_reference',
] as const
export const REFERENCE_MODES = [
  'first_last_frames',
  'omni_reference',
  'agentic',
] as const

const resolutionSchema = z.enum(OUTPUT_RESOLUTIONS)
const aspectRatioSchema = z.enum(ASPECT_RATIOS)
const inputModeSchema = z.enum(INPUT_MODES)
const referenceModeSchema = z.enum(REFERENCE_MODES)
const durationValueSchema = z
  .number()
  .int()
  .min(1)
  .max(MAX_TASK_DURATION_SECONDS)

const durationConstraintFormSchema = z
  .object({
    mode: z.enum(['values', 'range']),
    values: z.array(durationValueSchema),
    min: durationValueSchema.optional(),
    max: durationValueSchema.optional(),
  })
  .superRefine((value, ctx) => {
    if (value.mode === 'values' && value.values.length === 0) {
      ctx.addIssue({
        code: 'custom',
        path: ['values'],
        message: 'At least one duration is required',
      })
    }
    if (
      value.mode === 'range' &&
      (value.min === undefined ||
        value.max === undefined ||
        value.min > value.max)
    ) {
      ctx.addIssue({
        code: 'custom',
        path: ['min'],
        message: 'Enter a valid inclusive duration range',
      })
    }
  })

const referenceLimitsSchema = z.object({
  images: z.number().int().min(0).max(SEEDANCE_25_REFERENCE_LIMITS.images),
  videos: z.number().int().min(0).max(SEEDANCE_25_REFERENCE_LIMITS.videos),
  audios: z.number().int().min(0).max(SEEDANCE_25_REFERENCE_LIMITS.audios),
})

type ReferenceLimits = z.infer<typeof referenceLimitsSchema>

type ReferenceRange = {
  reference_minimums: ReferenceLimits
  reference_limits: ReferenceLimits
  reference_total_max?: number | null
  reference_video_audio_total_max?: number | null
  reference_video_total_duration_seconds?: number | null
}

function validateReferenceRange(value: ReferenceRange, ctx: z.RefinementCtx) {
  for (const kind of ['images', 'videos', 'audios'] as const) {
    if (value.reference_minimums[kind] <= value.reference_limits[kind]) {
      continue
    }
    ctx.addIssue({
      code: 'custom',
      path: ['reference_minimums', kind],
      message: 'Minimum cannot exceed maximum',
    })
  }
  const limits = value.reference_limits
  if (
    value.reference_total_max !== undefined &&
    value.reference_total_max !== null &&
    value.reference_total_max > limits.images + limits.videos + limits.audios
  ) {
    ctx.addIssue({
      code: 'custom',
      path: ['reference_total_max'],
      message: 'Total maximum exceeds the individual limits',
    })
  }
  if (
    value.reference_video_audio_total_max !== undefined &&
    value.reference_video_audio_total_max !== null
  ) {
    if (value.reference_video_audio_total_max > limits.videos + limits.audios) {
      ctx.addIssue({
        code: 'custom',
        path: ['reference_video_audio_total_max'],
        message: 'Video and audio maximum exceeds the individual limits',
      })
    }
    if (
      value.reference_total_max !== undefined &&
      value.reference_total_max !== null &&
      value.reference_total_max >
        limits.images + value.reference_video_audio_total_max
    ) {
      ctx.addIssue({
        code: 'custom',
        path: ['reference_total_max'],
        message: 'Aggregate reference limits conflict',
      })
    }
  }
}

function validateSeedanceReferenceLimits(
  model: CanonicalSeedanceModel,
  limits: ReferenceLimits,
  ctx: z.RefinementCtx,
  path: Array<string | number>
) {
  const maximums = getSeedanceReferenceLimits(model)
  for (const kind of ['images', 'videos', 'audios'] as const) {
    if (limits[kind] <= maximums[kind]) {
      continue
    }
    ctx.addIssue({
      code: 'custom',
      path: [...path, kind],
      message: `Reference ${kind} cannot exceed ${maximums[kind]} for this model`,
    })
  }
}

const marginBPSSchema = z.number().int().min(0).max(10_000)

export const costVariantKeySchema = z
  .string()
  .trim()
  .toLowerCase()
  .transform((value) => (value === '' ? 'default' : value))
  .pipe(
    z
      .string()
      .regex(
        /^[a-z0-9][a-z0-9._-]{0,63}$/,
        t(
          'Cost variant key must use 1-64 lowercase letters, digits, dots, hyphens, or underscores'
        )
      )
  )

const costVariantKeyApiSchema = costVariantKeySchema.default('default')

export const routeTargetFormSchema = z
  .object({
    id: z.number().int().positive().optional(),
    channel_id: z.number().int().positive('Channel is required'),
    channel_name: z.string(),
    name: z.string().trim().min(1, 'Target name is required'),
    upstream_model: z.string().trim().min(1, 'Upstream model is required'),
    cost_variant_key: costVariantKeySchema,
    target_priority: z.number().int(),
    minimum_expected_margin_bps: marginBPSSchema.nullable(),
    enabled: z.boolean(),
    output_resolutions: z
      .array(resolutionSchema)
      .min(1, 'At least one output resolution is required'),
    durations: durationConstraintFormSchema,
    aspect_ratios: z.array(aspectRatioSchema),
    input_modes: z
      .array(inputModeSchema)
      .min(1, 'At least one input mode is required'),
    reference_minimums: referenceLimitsSchema,
    reference_limits: referenceLimitsSchema,
    reference_total_max: z.number().int().min(0).nullable(),
    reference_video_audio_total_max: z.number().int().min(0).nullable(),
    reference_video_total_duration_seconds: z.number().int().min(0).nullable(),
    reference_modes: z.array(referenceModeSchema),
    supports_real_person: z.enum(['unknown', 'yes', 'no']),
  })
  .superRefine(validateReferenceRange)

export const routingPolicyFormSchema = z
  .object({
    id: z.number().int().positive().optional(),
    group_name: z.string().trim().min(1, 'Group is required'),
    model: z.enum(CANONICAL_SEEDANCE_MODELS),
    enabled: z.boolean(),
    defaults: z.object({
      output_resolution: resolutionSchema,
      duration_seconds: durationValueSchema,
      aspect_ratio: aspectRatioSchema,
    }),
    targets: z.array(routeTargetFormSchema),
  })
  .superRefine((value, ctx) => {
    if (value.enabled && value.targets.length === 0) {
      ctx.addIssue({
        code: 'custom',
        path: ['targets'],
        message: 'At least one routing target is required when enabled',
      })
    }
    value.targets.forEach((target, index) => {
      validateSeedanceReferenceLimits(
        value.model,
        target.reference_limits,
        ctx,
        ['targets', index, 'reference_limits']
      )
    })
  })

const durationValuesApiSchema = z.object({
  values: z.array(durationValueSchema).min(1),
  min: z.never().optional(),
  max: z.never().optional(),
})

const durationRangeApiSchema = z
  .object({
    values: z.never().optional(),
    min: durationValueSchema,
    max: durationValueSchema,
  })
  .refine((value) => value.min <= value.max, {
    path: ['min'],
    message: 'Enter a valid inclusive duration range',
  })

export const durationConstraintApiSchema = z.union([
  durationValuesApiSchema,
  durationRangeApiSchema,
])

export const routeConstraintsApiSchema = z
  .object({
    output_resolutions: z.array(resolutionSchema).min(1),
    durations: durationConstraintApiSchema,
    aspect_ratios: z.array(aspectRatioSchema).default([]),
    input_modes: z.array(inputModeSchema).default([...INPUT_MODES]),
    reference_minimums: referenceLimitsSchema.default({
      images: 0,
      videos: 0,
      audios: 0,
    }),
    reference_limits: referenceLimitsSchema,
    reference_total_max: z.number().int().min(0).nullable().optional(),
    reference_video_audio_total_max: z
      .number()
      .int()
      .min(0)
      .nullable()
      .optional(),
    reference_video_total_duration_seconds: z
      .number()
      .int()
      .min(0)
      .nullable()
      .optional(),
    reference_modes: z.array(referenceModeSchema).default([]),
    supports_real_person: z.boolean().nullable(),
  })
  .superRefine(validateReferenceRange)

export const routeTargetSchema = z.object({
  id: z.number().int().positive(),
  channel_id: z.number().int().positive(),
  channel_name: z.string(),
  name: z.string(),
  upstream_model: z.string(),
  cost_variant_key: costVariantKeyApiSchema,
  target_priority: z.number().int(),
  minimum_expected_margin_bps: marginBPSSchema.nullable(),
  enabled: z.boolean(),
  constraints: routeConstraintsApiSchema,
})

export const routeTargetWriteRequestSchema = routeTargetSchema
  .omit({ channel_name: true })
  .extend({ id: z.number().int().positive().optional() })

const routingPolicyApiSchema = z.object({
  id: z.number().int().positive(),
  group_name: z.string(),
  model: z.enum(CANONICAL_SEEDANCE_MODELS),
  enabled: z.boolean(),
  defaults: z.object({
    output_resolution: resolutionSchema,
    duration_seconds: durationValueSchema,
    aspect_ratio: aspectRatioSchema,
  }),
  targets: z.array(routeTargetSchema),
  created_at: z.number().int(),
  updated_at: z.number().int(),
})

export const routingPolicySchema = routingPolicyApiSchema.superRefine(
  (value, ctx) => {
    value.targets.forEach((target, index) => {
      validateSeedanceReferenceLimits(
        value.model,
        target.constraints.reference_limits,
        ctx,
        ['targets', index, 'constraints', 'reference_limits']
      )
    })
  }
)

export const routingPolicyWriteRequestSchema = routingPolicyApiSchema
  .omit({ id: true, created_at: true, updated_at: true, targets: true })
  .extend({ targets: z.array(routeTargetWriteRequestSchema) })
  .superRefine((value, ctx) => {
    value.targets.forEach((target, index) => {
      validateSeedanceReferenceLimits(
        value.model,
        target.constraints.reference_limits,
        ctx,
        ['targets', index, 'constraints', 'reference_limits']
      )
    })
  })

const apiSuccessSchema = z.object({
  success: z.literal(true),
  message: z.string().optional(),
})

export const routingGroupResponseSchema = apiSuccessSchema.extend({
  data: z.array(z.string()),
})

export function normalizeRoutingGroups(
  groups: string[],
  currentGroup: string
): string[] {
  const normalized = new Map<string, string>()
  for (const value of groups) {
    const group = value.trim()
    if (group === '' || group.toLowerCase() === 'auto') {
      continue
    }
    const key = group.toLocaleLowerCase()
    if (!normalized.has(key)) {
      normalized.set(key, group)
    }
  }

  const current = currentGroup.trim()
  if (current !== '' && current.toLowerCase() !== 'auto') {
    normalized.set(current.toLocaleLowerCase(), current)
  }

  return [...normalized.values()].sort((left, right) =>
    left.localeCompare(right, undefined, { sensitivity: 'base' })
  )
}

export const routingPolicyResponseSchema = apiSuccessSchema.extend({
  data: routingPolicySchema,
})

export const routingPolicyListResponseSchema = apiSuccessSchema.extend({
  data: z.object({
    items: z.array(routingPolicySchema),
    total: z.number().int().nonnegative(),
    page: z.number().int().positive(),
    page_size: z.number().int().positive(),
  }),
})

export const routingCandidateSchema = z.object({
  id: z.number().int().positive(),
  name: z.string(),
  status: z.number().int(),
  priority: z.number().int(),
  weight: z.number().int().nonnegative(),
})

export const routingCandidateResponseSchema = apiSuccessSchema.extend({
  data: z.array(routingCandidateSchema),
})

export const routingPolicyErrorSchema = z.object({
  success: z.literal(false),
  message: z.string(),
  code: z.string(),
  data: z
    .object({
      field: z.string().optional(),
      target_indexes: z.array(z.number().int().nonnegative()).optional(),
    })
    .optional(),
})

export type RouteTargetFormValues = z.infer<typeof routeTargetFormSchema>
export type RoutingPolicyFormValues = z.infer<typeof routingPolicyFormSchema>
export type RoutingPolicy = z.infer<typeof routingPolicySchema>
export type RoutingPolicyWriteRequest = z.infer<
  typeof routingPolicyWriteRequestSchema
>
export type RoutingPolicyError = z.infer<typeof routingPolicyErrorSchema>
export type RoutingCandidate = z.infer<typeof routingCandidateSchema>

export type RoutingPolicyListParams = {
  group_name?: string
  model?: string
  channel_id?: number
  p?: number
  page_size?: number
}

export function buildRoutingTargetName(input: {
  date: Date
  channelName: string
  model: RoutingPolicyFormValues['model']
  outputResolutions: RouteTargetFormValues['output_resolutions']
  durations: RouteTargetFormValues['durations']
}): string | undefined {
  const channelName = input.channelName.trim()
  if (channelName === '') {
    return undefined
  }

  const resolutionOrder = new Map(
    OUTPUT_RESOLUTIONS.map((resolution, index) => [resolution, index])
  )
  const resolutions = [...new Set(input.outputResolutions)].sort(
    (left, right) =>
      (resolutionOrder.get(left) ?? Number.MAX_SAFE_INTEGER) -
      (resolutionOrder.get(right) ?? Number.MAX_SAFE_INTEGER)
  )
  if (resolutions.length === 0) {
    return undefined
  }

  let duration: string
  if (input.durations.mode === 'values') {
    const values = [...new Set(input.durations.values)].sort(
      (left, right) => left - right
    )
    if (values.length === 0) {
      return undefined
    }
    duration = `${values.join('+')}s`
  } else {
    if (
      input.durations.min === undefined ||
      input.durations.max === undefined
    ) {
      return undefined
    }
    duration = `${input.durations.min}-${input.durations.max}s`
  }

  let speed = 'standard'
  if (input.model.includes('-fast-')) {
    speed = 'fast'
  } else if (input.model.includes('-mini-')) {
    speed = 'mini'
  }

  const year = String(input.date.getFullYear())
  const month = String(input.date.getMonth() + 1).padStart(2, '0')
  const day = String(input.date.getDate()).padStart(2, '0')
  return `${year}${month}${day}-${channelName}-${resolutions.join('+')}-${speed}-${duration}`
}

export function shouldUpdateRoutingTargetName(
  currentName: string,
  previousGeneratedName: string | undefined
): boolean {
  return (
    currentName.trim() === '' ||
    (previousGeneratedName !== undefined &&
      currentName === previousGeneratedName)
  )
}

export function createEmptyPolicyForm(): RoutingPolicyFormValues {
  return {
    group_name: '',
    model: CANONICAL_SEEDANCE_MODELS[0],
    enabled: false,
    defaults: {
      output_resolution: '720p',
      duration_seconds: 10,
      aspect_ratio: '16:9',
    },
    targets: [],
  }
}

export function createEmptyTarget(): RouteTargetFormValues {
  return {
    channel_id: 0,
    channel_name: '',
    name: '',
    upstream_model: '',
    cost_variant_key: 'default',
    target_priority: 0,
    minimum_expected_margin_bps: null,
    enabled: false,
    output_resolutions: ['720p'],
    durations: { mode: 'range', values: [], min: 4, max: 15 },
    aspect_ratios: [],
    input_modes: [...INPUT_MODES],
    reference_minimums: { images: 0, videos: 0, audios: 0 },
    reference_limits: { images: 9, videos: 3, audios: 3 },
    reference_total_max: null,
    reference_video_audio_total_max: null,
    reference_video_total_duration_seconds: null,
    reference_modes: [],
    supports_real_person: 'unknown',
  }
}

export function copyPolicyForm(policy: RoutingPolicy): RoutingPolicyFormValues {
  const copy = fromPolicyResponse(policy)
  return {
    ...copy,
    id: undefined,
    enabled: false,
    targets: copy.targets.map((target) => cloneTargetForm(target, target.name)),
  }
}

export function copyTargetForm(
  target: RouteTargetFormValues
): RouteTargetFormValues {
  return cloneTargetForm(target, `${target.name} copy`)
}

function cloneTargetForm(
  target: RouteTargetFormValues,
  name: string
): RouteTargetFormValues {
  return {
    ...target,
    id: undefined,
    name,
    output_resolutions: [...target.output_resolutions],
    durations: {
      ...target.durations,
      values: [...target.durations.values],
    },
    aspect_ratios: [...target.aspect_ratios],
    input_modes: [...target.input_modes],
    reference_minimums: { ...target.reference_minimums },
    reference_limits: { ...target.reference_limits },
    reference_modes: [...target.reference_modes],
  }
}

export function clearUnavailableTargetChannels(
  targets: RouteTargetFormValues[],
  candidateIDs: number[]
): RouteTargetFormValues[] {
  const available = new Set(candidateIDs)
  return targets.map((target) => {
    if (target.channel_id === 0 || available.has(target.channel_id)) {
      return target
    }
    return { ...target, channel_id: 0, channel_name: '' }
  })
}

export function toWriteRequest(
  value: RoutingPolicyFormValues
): RoutingPolicyWriteRequest {
  const parsed = routingPolicyFormSchema.parse(value)
  return {
    group_name: parsed.group_name,
    model: parsed.model,
    enabled: parsed.enabled,
    defaults: parsed.defaults,
    targets: parsed.targets.map((target) => ({
      id: target.id,
      channel_id: target.channel_id,
      name: target.name,
      upstream_model: target.upstream_model,
      cost_variant_key: target.cost_variant_key,
      target_priority: target.target_priority,
      minimum_expected_margin_bps: target.minimum_expected_margin_bps,
      enabled: target.enabled,
      constraints: {
        output_resolutions: target.output_resolutions,
        durations:
          target.durations.mode === 'values'
            ? { values: target.durations.values }
            : {
                min: target.durations.min as number,
                max: target.durations.max as number,
              },
        aspect_ratios: target.aspect_ratios,
        input_modes: target.input_modes,
        reference_minimums: target.reference_minimums,
        reference_limits: target.reference_limits,
        reference_total_max: target.reference_total_max,
        reference_video_audio_total_max: target.reference_video_audio_total_max,
        reference_video_total_duration_seconds:
          target.reference_video_total_duration_seconds,
        reference_modes: target.reference_modes,
        supports_real_person:
          target.supports_real_person === 'unknown'
            ? null
            : target.supports_real_person === 'yes',
      },
    })),
  }
}

export function fromPolicyResponse(
  policy: RoutingPolicy
): RoutingPolicyFormValues {
  return {
    id: policy.id,
    group_name: policy.group_name,
    model: policy.model,
    enabled: policy.enabled,
    defaults: policy.defaults,
    targets: policy.targets.map((target) => {
      const durations = target.constraints.durations
      const durationForm =
        'values' in durations && durations.values
          ? { mode: 'values' as const, values: durations.values }
          : {
              mode: 'range' as const,
              values: [],
              min: durations.min,
              max: durations.max,
            }
      const supportsRealPerson = target.constraints.supports_real_person
      let supportsRealPersonForm: RouteTargetFormValues['supports_real_person'] =
        'unknown'
      if (supportsRealPerson !== null) {
        supportsRealPersonForm = supportsRealPerson ? 'yes' : 'no'
      }
      return {
        id: target.id,
        channel_id: target.channel_id,
        channel_name: target.channel_name,
        name: target.name,
        upstream_model: target.upstream_model,
        cost_variant_key: target.cost_variant_key,
        target_priority: target.target_priority,
        minimum_expected_margin_bps: target.minimum_expected_margin_bps,
        enabled: target.enabled,
        output_resolutions: target.constraints.output_resolutions,
        durations: durationForm,
        aspect_ratios: target.constraints.aspect_ratios,
        input_modes: target.constraints.input_modes,
        reference_minimums: target.constraints.reference_minimums,
        reference_limits: target.constraints.reference_limits,
        reference_total_max: target.constraints.reference_total_max ?? null,
        reference_video_audio_total_max:
          target.constraints.reference_video_audio_total_max ?? null,
        reference_video_total_duration_seconds:
          target.constraints.reference_video_total_duration_seconds ?? null,
        reference_modes: target.constraints.reference_modes,
        supports_real_person: supportsRealPersonForm,
      }
    }),
  }
}
