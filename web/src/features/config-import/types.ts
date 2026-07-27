/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.
*/
import { z } from 'zod'

export const configImportBatchStatusSchema = z.enum([
  'validating',
  'blocked',
  'binding',
  'staged',
  'ready',
  'publishing',
  'published',
  'publish_failed',
])

export const configImportItemStateSchema = z.enum([
  'new',
  'unchanged',
  'changed',
  'conflict',
  'excluded',
])

export const configImportIssueSeveritySchema = z.enum([
  'info',
  'warning',
  'error',
])

export const configImportBindingActionSchema = z.enum([
  'create',
  'bind',
  'skip',
])
export const configImportResolutionActionSchema = z.enum([
  'use_import',
  'keep_existing',
  'exclude',
  'split_line',
  'bind_variant',
])

export const configImportRouteMergeModeSchema = z.enum([
  'replace',
  'merge',
  'skip',
])

const entityCountsSchema = z.object({
  channels: z.number().int().nonnegative(),
  channel_lines: z.number().int().nonnegative(),
  model_skus: z.number().int().nonnegative(),
  sale_proposals: z.number().int().nonnegative(),
  cost_rule_drafts: z.number().int().nonnegative(),
  model_mappings: z.number().int().nonnegative(),
  route_blueprints: z.number().int().nonnegative(),
  sources: z.number().int().nonnegative(),
  unresolved_variants: z.number().int().nonnegative(),
})

export const configImportItemSchema = z
  .object({
    id: z.number().int().positive(),
    entity_type: z.string().min(1),
    business_id: z.string().min(1),
    entity_hash: z.string().min(1),
    canonical_json: z.string(),
    state: configImportItemStateSchema,
    source_ref: z.string().min(1),
    source_sheet: z.string(),
    source_row: z.number().int().positive().nullish(),
    materialized_type: z.string().nullish(),
    materialized_id: z.number().int().positive().nullish(),
    conflict_reason: z.string().nullish(),
    exclusion_reason: z.string().nullish(),
  })
  .strict()

export const configImportIssueSchema = z
  .object({
    id: z.number().int().positive(),
    severity: configImportIssueSeveritySchema,
    code: z.string().min(1),
    entity_type: z.string().nullish(),
    business_id: z.string().nullish(),
    sheet: z.string().nullish(),
    row: z.number().int().positive().nullish(),
    field: z.string().nullish(),
    message: z.string().min(1),
    suggestion: z.string().nullish(),
    resolution_status: z.string().min(1),
  })
  .strict()

export const configImportBatchSummarySchema = z
  .object({
    id: z.number().int().positive(),
    schema_version: z.number().int().positive(),
    template_version: z.string().min(1),
    source_sha256: z.string().min(1),
    payload_sha256: z.string().min(1),
    status: configImportBatchStatusSchema,
    created_by: z.number().int().positive(),
    item_counts: entityCountsSchema,
    issue_count: z.number().int().nonnegative(),
    allowed_actions: z.array(z.string()),
    created_at: z.number().int(),
    updated_at: z.number().int(),
  })
  .strict()

export const configImportBatchDetailSchema =
  configImportBatchSummarySchema.extend({
    items: z.array(configImportItemSchema),
    issues: z.array(configImportIssueSchema),
  })

export const configImportBindingSchema = z
  .object({
    line_ref: z.string().min(1),
    action: configImportBindingActionSchema,
    channel_id: z.number().int().positive().optional(),
    credentials_confirmed: z.boolean(),
    reason: z.string().optional(),
  })
  .strict()
  .superRefine((value, ctx) => {
    if (value.action === 'skip') {
      if (value.channel_id !== undefined) {
        ctx.addIssue({
          code: 'custom',
          path: ['channel_id'],
          message: 'Skipped bindings cannot select a channel',
        })
      }
      if (value.credentials_confirmed) {
        ctx.addIssue({
          code: 'custom',
          path: ['credentials_confirmed'],
          message: 'Skipped bindings cannot confirm credentials',
        })
      }
      if (!value.reason?.trim()) {
        ctx.addIssue({
          code: 'custom',
          path: ['reason'],
          message: 'A reason is required when skipping a binding',
        })
      }
      return
    }

    if (value.channel_id === undefined) {
      ctx.addIssue({
        code: 'custom',
        path: ['channel_id'],
        message: 'A channel is required for this binding action',
      })
    }
    if (value.reason !== undefined) {
      ctx.addIssue({
        code: 'custom',
        path: ['reason'],
        message: 'Bound channels cannot include a skip reason',
      })
    }
  })

export const configImportResolutionSchema = z
  .object({
    item_business_id: z.string().min(1),
    action: configImportResolutionActionSchema,
  })
  .strict()

export const configImportUploadRequestSchema = z.object({
  document: z.record(z.string(), z.unknown()),
})

export const configImportBindingsRequestSchema = z
  .object({
    bindings: z.array(configImportBindingSchema).min(1),
  })
  .strict()

export const configImportResolutionsRequestSchema = z
  .object({
    batch_ref: z.string().optional(),
    resolutions: z.array(configImportResolutionSchema).min(1),
  })
  .strict()

export const configImportStageRequestSchema = z
  .object({
    batch_ref: z.string().optional(),
    excluded_business_ids: z.array(z.string()).optional(),
  })
  .strict()

export const configImportListParamsSchema = z
  .object({
    page: z.number().int().positive().optional(),
    page_size: z.number().int().positive().max(100).optional(),
  })
  .strict()

export const configImportBatchPageSchema = z.object({
  items: z.array(configImportBatchSummarySchema),
  total: z.number().int().nonnegative(),
  page: z.number().int().positive(),
  page_size: z.number().int().positive(),
})

export const configImportResponseSchema = z.object({
  success: z.literal(true),
  message: z.string().optional(),
})

export const configImportBatchDetailResponseSchema =
  configImportResponseSchema.extend({
    data: configImportBatchDetailSchema,
  })

export const configImportBatchPageResponseSchema =
  configImportResponseSchema.extend({
    data: configImportBatchPageSchema,
  })

export const configImportPublishResponseSchema =
  configImportResponseSchema.extend({
    data: z.object({
      batch_id: z.number().int().positive(),
      status: z.literal('published'),
    }),
  })

export type ConfigImportBatchStatus = z.infer<
  typeof configImportBatchStatusSchema
>
export type ConfigImportItemState = z.infer<typeof configImportItemStateSchema>
export type ConfigImportIssueSeverity = z.infer<
  typeof configImportIssueSeveritySchema
>
export type ConfigImportBindingAction = z.infer<
  typeof configImportBindingActionSchema
>
export type ConfigImportResolutionAction = z.infer<
  typeof configImportResolutionActionSchema
>
export type ConfigImportRouteMergeMode = z.infer<
  typeof configImportRouteMergeModeSchema
>
export type ConfigImportEntityCounts = z.infer<typeof entityCountsSchema>
export type ConfigImportItemDetail = z.infer<typeof configImportItemSchema>
export type ConfigImportIssueDetail = z.infer<typeof configImportIssueSchema>
export type ConfigImportBatchSummary = z.infer<
  typeof configImportBatchSummarySchema
>
export type ConfigImportBatchDetail = z.infer<
  typeof configImportBatchDetailSchema
>
export type ConfigImportBinding = z.infer<typeof configImportBindingSchema>
export type ConfigImportResolution = z.infer<
  typeof configImportResolutionSchema
>
export type ConfigImportBatchPage = z.infer<typeof configImportBatchPageSchema>
export type ConfigImportListParams = z.infer<
  typeof configImportListParamsSchema
>
export type ConfigImportBindingsRequest = z.infer<
  typeof configImportBindingsRequestSchema
>
export type ConfigImportResolutionsRequest = z.infer<
  typeof configImportResolutionsRequestSchema
>
export type ConfigImportStageRequest = z.infer<
  typeof configImportStageRequestSchema
>
