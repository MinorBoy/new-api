import { z } from 'zod'

import { api } from '@/lib/api'

import type {
  GroupCostMode,
  GroupRoutingProfiles,
  GroupRoutingRequirements,
} from './group-routing-requirements'

const groupCostModeSchema = z.enum([
  'per_request',
  'per_duration',
  'per_token',
  'free',
])

export const groupRoutingTargetStatusSchema = z.enum([
  'matched',
  'real_person_mismatch',
  'real_person_unknown',
  'cost_mode_mismatch',
  'cost_rule_missing',
  'excluded',
  'target_disabled',
  'channel_unavailable',
])

export const groupRoutingProfileSummarySchema = z.object({
  models: z.number().int().nonnegative(),
  matched_models: z.number().int().nonnegative(),
  targets: z.number().int().nonnegative(),
  matched_targets: z.number().int().nonnegative(),
  stale_exclusions: z.number().int().nonnegative(),
})

const groupRoutingProfileTargetSchema = z.object({
  model: z.string(),
  channel_id: z.number().int().positive(),
  channel_name: z.string(),
  target_name: z.string(),
  upstream_model: z.string(),
  cost_variant_key: z.string(),
  target_priority: z.number().int(),
  supports_real_person: z.boolean().nullable(),
  cost_mode: groupCostModeSchema.optional(),
  cost_rule_id: z.number().int().positive().optional(),
  cost_rule_version: z.number().int().positive().optional(),
  target_key: z.string(),
  status: groupRoutingTargetStatusSchema,
  issues: z.array(groupRoutingTargetStatusSchema),
})

export const groupRoutingProfileTargetPageSchema = z.object({
  items: z.array(groupRoutingProfileTargetSchema),
  summary: groupRoutingProfileSummarySchema,
  facets: z.object({
    models: z.array(z.string()),
    channels: z.array(
      z.object({
        id: z.number().int().positive(),
        name: z.string(),
      })
    ),
    cost_modes: z.array(groupCostModeSchema),
    statuses: z.array(groupRoutingTargetStatusSchema),
  }),
  page: z.number().int().positive(),
  page_size: z.union([z.literal(25), z.literal(50), z.literal(100)]),
  total: z.number().int().nonnegative(),
})

const apiSuccessSchema = z.object({
  success: z.literal(true),
  message: z.string().optional(),
})

export const groupRoutingProfileTargetPageResponseSchema =
  apiSuccessSchema.extend({
    data: groupRoutingProfileTargetPageSchema,
  })

export const groupRoutingProfileSummariesResponseSchema =
  apiSuccessSchema.extend({
    data: z.record(z.string(), groupRoutingProfileSummarySchema),
  })

export type GroupRoutingTargetStatus = z.infer<
  typeof groupRoutingTargetStatusSchema
>
export type GroupRoutingProfileSummary = z.infer<
  typeof groupRoutingProfileSummarySchema
>
export type GroupRoutingProfileTargetPage = z.infer<
  typeof groupRoutingProfileTargetPageSchema
>
export type GroupRoutingProfileTargetPageResponse = z.infer<
  typeof groupRoutingProfileTargetPageResponseSchema
>
export type GroupRoutingProfileSummariesResponse = z.infer<
  typeof groupRoutingProfileSummariesResponseSchema
>

export type GroupRoutingProfileTargetRequest = {
  group_name: string
  profile: GroupRoutingRequirements
  model?: string
  channel_id?: number
  cost_mode?: GroupCostMode
  status?: GroupRoutingTargetStatus
  page: number
  page_size: 25 | 50 | 100
}

export async function previewGroupRoutingProfileTargets(
  input: GroupRoutingProfileTargetRequest
): Promise<GroupRoutingProfileTargetPageResponse> {
  const response = await api.post(
    '/api/routing-policies/group-profile/targets',
    input
  )
  return groupRoutingProfileTargetPageResponseSchema.parse(response.data)
}

export async function previewGroupRoutingProfileSummaries(
  profiles: GroupRoutingProfiles
): Promise<GroupRoutingProfileSummariesResponse> {
  const response = await api.post(
    '/api/routing-policies/group-profile/summaries',
    { profiles }
  )
  return groupRoutingProfileSummariesResponseSchema.parse(response.data)
}
