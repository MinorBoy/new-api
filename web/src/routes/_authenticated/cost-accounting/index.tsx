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
import { createFileRoute, redirect } from '@tanstack/react-router'
import z from 'zod'

import { CostAccounting } from '@/features/cost-accounting'
import {
  ADMIN_PERMISSION_ACTIONS,
  ADMIN_PERMISSION_RESOURCES,
  hasPermission,
} from '@/lib/admin-permissions'
import { useAuthStore } from '@/stores/auth-store'

const costAccountingSearchSchema = z.object({
  tab: z
    .enum(['profit', 'catalog', 'route-margin', 'anomalies'])
    .optional()
    .catch('profit'),
  timeBasis: z
    .enum(['profit_recognized_at', 'requested_at'])
    .optional()
    .catch('profit_recognized_at'),
  startTime: z.number().optional().catch(undefined),
  endTime: z.number().optional().catch(undefined),
  channelId: z.number().optional().catch(undefined),
  billableModel: z.string().optional().catch(''),
  originModel: z.string().optional().catch(''),
  userGroup: z.string().optional().catch(''),
  usingGroup: z.string().optional().catch(''),
  billingSource: z.string().optional().catch(''),
  status: z.string().optional().catch(''),
  catalogChannelId: z.number().int().positive().optional().catch(undefined),
  catalogModel: z.string().optional().catch(''),
  catalogCostMode: z
    .enum(['free', 'per_request', 'per_image', 'per_duration', 'per_token'])
    .optional()
    .catch(undefined),
  catalogStatus: z
    .enum(['active', 'draft', 'retired', 'all'])
    .optional()
    .catch('active'),
  catalogCurrency: z.string().optional().catch(''),
  catalogSource: z.string().optional().catch(''),
  catalogPage: z.number().int().positive().optional().catch(1),
  catalogPageSize: z
    .union([z.literal(25), z.literal(50), z.literal(100)])
    .optional()
    .catch(50),
  catalogSort: z
    .enum([
      'channel_name',
      'channel_id',
      'billable_upstream_model',
      'cost_variant_key',
      'status',
      'version',
      'cost_mode',
      'source',
      'effective_from',
    ])
    .optional()
    .catch('channel_name'),
  catalogOrder: z.enum(['asc', 'desc']).optional().catch('asc'),
  marginMinimumPercent: z.number().min(-100).max(100).optional().catch(30),
  marginDurationSeconds: z.number().int().min(1).max(3600).optional().catch(4),
  marginGroupRatio: z.number().positive().max(100).optional().catch(1),
  marginScenario: z
    .enum(['all', 'no_video', 'with_video'])
    .optional()
    .catch('all'),
  marginChannelId: z.number().int().positive().optional().catch(undefined),
  marginModel: z.string().max(191).optional().catch(''),
  marginUpstreamModel: z.string().max(191).optional().catch(''),
  marginRouteTarget: z.string().max(191).optional().catch(''),
  marginResolution: z.string().max(191).optional().catch(''),
  marginStatus: z
    .enum(['all', 'eligible', 'ineligible'])
    .optional()
    .catch('all'),
  marginPage: z.number().int().positive().optional().catch(1),
  marginPageSize: z
    .union([z.literal(25), z.literal(50), z.literal(100)])
    .optional()
    .catch(50),
  marginSort: z
    .enum([
      'target_name',
      'channel_name',
      'upstream_model',
      'gross_margin_ppm',
      'estimated_profit_nano_usd',
    ])
    .optional()
    .catch('gross_margin_ppm'),
  marginOrder: z.enum(['asc', 'desc']).optional().catch('desc'),
})

export function requireCostAccountingRead() {
  const { auth } = useAuthStore.getState()
  if (
    !hasPermission(
      auth.user,
      ADMIN_PERMISSION_RESOURCES.COST_ACCOUNTING,
      ADMIN_PERMISSION_ACTIONS.READ
    )
  ) {
    throw redirect({ to: '/403' })
  }
}

export const Route = createFileRoute('/_authenticated/cost-accounting/')({
  beforeLoad: requireCostAccountingRead,
  validateSearch: costAccountingSearchSchema,
  component: CostAccounting,
})
