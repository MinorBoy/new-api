import { z } from 'zod'

const policySchema = z
  .object({
    strategy: z.enum(['manual', 'lowest_cost', 'cost_weighted']),
    minimum_expected_margin_bps: z.number().int().min(0).max(10000).optional(),
    cost_tolerance_bps: z.number().int().min(0).max(10000).optional(),
    require_compatibility_test: z.boolean().optional(),
  })
  .superRefine((policy, ctx) => {
    if (
      policy.cost_tolerance_bps !== undefined &&
      policy.strategy !== 'cost_weighted'
    ) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        path: ['cost_tolerance_bps'],
        message: 'cost_tolerance_bps is only valid for cost_weighted strategy',
      })
    }
  })

const routingPolicySchema = z.object({
  version: z.literal(1),
  default: policySchema,
  groups: z
    .record(z.string(), z.record(z.string(), policySchema))
    .optional(),
})

export function parseImageRoutingPolicy(value: string): unknown {
  const parsed: unknown = JSON.parse(value)
  return routingPolicySchema.parse(parsed)
}
