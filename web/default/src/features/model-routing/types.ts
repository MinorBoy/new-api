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
import { z } from 'zod'

export const CANONICAL_SEEDANCE_MODELS = [
  'doubao-seedance-2-0-260128',
  'doubao-seedance-2-0-fast-260128',
  'doubao-seedance-2-0-mini-260615',
] as const

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

const resolutionSchema = z.enum(OUTPUT_RESOLUTIONS)
const aspectRatioSchema = z.enum(ASPECT_RATIOS)
const durationValueSchema = z
  .number()
  .int()
  .min(1)
  .max(MAX_TASK_DURATION_SECONDS)

const durationConstraintFormSchema = z
  .object({
    mode: z.enum(['values', 'range']),
    values: z.array(durationValueSchema).default([]),
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
  images: z.number().int().min(0).max(9),
  videos: z.number().int().min(0).max(3),
  audios: z.number().int().min(0).max(3),
})

export const routeTargetFormSchema = z
  .object({
    id: z.number().int().positive().optional(),
    channel_id: z.number().int().positive('Channel is required'),
    channel_name: z.string().default(''),
    name: z.string().trim().min(1, 'Target name is required'),
    upstream_model: z.string().trim().min(1, 'Upstream model is required'),
    target_priority: z.number().int(),
    enabled: z.boolean(),
    output_resolutions: z
      .array(resolutionSchema)
      .min(1, 'At least one output resolution is required'),
    generation_resolution: resolutionSchema.optional(),
    upscaled: z.boolean(),
    durations: durationConstraintFormSchema,
    aspect_ratios: z.array(aspectRatioSchema).default([]),
    reference_limits: referenceLimitsSchema,
    supports_real_person: z.enum(['unknown', 'yes', 'no']),
  })
  .superRefine((value, ctx) => {
    if (value.upscaled) {
      if (
        value.output_resolutions.length !== 1 ||
        value.generation_resolution === undefined ||
        value.generation_resolution === value.output_resolutions[0]
      ) {
        ctx.addIssue({
          code: 'custom',
          path: ['generation_resolution'],
          message:
            'Upscaled targets require one distinct generation resolution',
        })
      }
      return
    }
    if (value.generation_resolution !== undefined) {
      ctx.addIssue({
        code: 'custom',
        path: ['generation_resolution'],
        message: 'Native targets cannot set a generation resolution',
      })
    }
  })

export const routingPolicyFormSchema = z.object({
  id: z.number().int().positive().optional(),
  group_name: z.string().trim().min(1, 'Group is required'),
  model: z.enum(CANONICAL_SEEDANCE_MODELS),
  enabled: z.boolean(),
  defaults: z.object({
    output_resolution: resolutionSchema,
    duration_seconds: durationValueSchema,
    aspect_ratio: aspectRatioSchema,
  }),
  targets: z.array(routeTargetFormSchema).default([]),
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

export const routeConstraintsApiSchema = z.object({
  output_resolutions: z.array(resolutionSchema).min(1),
  generation_resolution: resolutionSchema.optional(),
  upscaled: z.boolean(),
  durations: durationConstraintApiSchema,
  aspect_ratios: z.array(aspectRatioSchema).default([]),
  reference_limits: referenceLimitsSchema,
  supports_real_person: z.boolean().nullable(),
})

export const routeTargetSchema = z.object({
  id: z.number().int().positive(),
  channel_id: z.number().int().positive(),
  channel_name: z.string(),
  name: z.string(),
  upstream_model: z.string(),
  target_priority: z.number().int(),
  enabled: z.boolean(),
  constraints: routeConstraintsApiSchema,
})

export const routeTargetWriteRequestSchema = routeTargetSchema.omit({
  id: true,
  channel_name: true,
})

export const routingPolicySchema = z.object({
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

export const routingPolicyWriteRequestSchema = routingPolicySchema
  .omit({ id: true, created_at: true, updated_at: true, targets: true })
  .extend({ targets: z.array(routeTargetWriteRequestSchema) })

const apiSuccessSchema = z.object({
  success: z.literal(true),
  message: z.string().optional(),
})

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
      channel_id: target.channel_id,
      name: target.name,
      upstream_model: target.upstream_model,
      target_priority: target.target_priority,
      enabled: target.enabled,
      constraints: {
        output_resolutions: target.output_resolutions,
        ...(target.upscaled && target.generation_resolution
          ? { generation_resolution: target.generation_resolution }
          : {}),
        upscaled: target.upscaled,
        durations:
          target.durations.mode === 'values'
            ? { values: target.durations.values }
            : {
                min: target.durations.min as number,
                max: target.durations.max as number,
              },
        aspect_ratios: target.aspect_ratios,
        reference_limits: target.reference_limits,
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
        target_priority: target.target_priority,
        enabled: target.enabled,
        output_resolutions: target.constraints.output_resolutions,
        generation_resolution: target.constraints.generation_resolution,
        upscaled: target.constraints.upscaled,
        durations: durationForm,
        aspect_ratios: target.constraints.aspect_ratios,
        reference_limits: target.constraints.reference_limits,
        supports_real_person: supportsRealPersonForm,
      }
    }),
  }
}
