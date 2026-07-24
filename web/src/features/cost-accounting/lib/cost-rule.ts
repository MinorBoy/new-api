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

import type { CostRuleConfigV1, CostRuleFormValues } from '../types'

const NANO_USD_SCALE = 1_000_000_000n
const PPM_PER_PERCENT = 10_000n
const CANONICAL_INTEGER = /^-?(?:0|[1-9]\d*)$/
const CANONICAL_DECIMAL = /^(?:0|[1-9]\d*)(?:\.\d*[1-9])?$/

const positiveDecimalSchema = z
  .string()
  .regex(CANONICAL_DECIMAL, 'Must be a canonical decimal')
  .refine((value) => value !== '0', 'Must be greater than zero')
const nonNegativeDecimalSchema = z
  .string()
  .regex(CANONICAL_DECIMAL, 'Must be a canonical non-negative decimal')
const paidFieldSchemas = {
  currency: z
    .string()
    .trim()
    .min(1, 'Currency is required')
    .transform((value) => value.toUpperCase()),
  billing_multiplier: positiveDecimalSchema,
  purchase_discount_ratio: positiveDecimalSchema,
  recharge_exchange_ratio: positiveDecimalSchema,
  fee_rate: nonNegativeDecimalSchema,
  currency_to_usd_rate: positiveDecimalSchema,
  charge_event: z.enum([
    'response_succeeded',
    'submit_accepted',
    'task_succeeded',
  ]),
} as const
const tokenPaidFieldSchemas = {
  ...paidFieldSchemas,
  charge_event: z.enum(['response_succeeded', 'task_succeeded']),
} as const

const freeCostRuleFormSchema = z.strictObject({
  cost_mode: z.literal('free'),
  zero_cost_reason: z.string().trim().min(1, 'A free rule requires a reason'),
})
const perRequestCostRuleFormSchema = z.strictObject({
  cost_mode: z.literal('per_request'),
  ...paidFieldSchemas,
  unit_price: positiveDecimalSchema,
})
const perDurationCostRuleFormSchema = z
  .strictObject({
    cost_mode: z.literal('per_duration'),
    ...paidFieldSchemas,
    meter_source: z.enum(['validated_request', 'upstream_actual']),
    price_per_second: positiveDecimalSchema,
  })
  .refine(
    (values) =>
      values.charge_event !== 'submit_accepted' ||
      values.meter_source === 'validated_request',
    {
      message:
        'Submit-accepted duration rules require validated-request metering',
      path: ['meter_source'],
    }
  )
const totalTokenCostRuleFormSchema = z.strictObject({
  cost_mode: z.literal('per_token'),
  ...tokenPaidFieldSchemas,
  meter_source: z.enum(['upstream_usage', 'local_usage']),
  token_mode: z.literal('total_tokens'),
  total_per_million: positiveDecimalSchema,
})
const completionTokenCostRuleFormSchema = z.strictObject({
  cost_mode: z.literal('per_token'),
  ...tokenPaidFieldSchemas,
  meter_source: z.enum(['upstream_usage', 'local_usage']),
  token_mode: z.literal('completion_tokens'),
  completion_per_million: positiveDecimalSchema,
})
const splitTokenCostRuleFormSchema = z.strictObject({
  cost_mode: z.literal('per_token'),
  ...tokenPaidFieldSchemas,
  meter_source: z.enum(['upstream_usage', 'local_usage']),
  token_mode: z.literal('input_output'),
  input_per_million: positiveDecimalSchema,
  output_per_million: positiveDecimalSchema,
})

export const costRuleFormSchema = z.union([
  freeCostRuleFormSchema,
  perRequestCostRuleFormSchema,
  perDurationCostRuleFormSchema,
  totalTokenCostRuleFormSchema,
  completionTokenCostRuleFormSchema,
  splitTokenCostRuleFormSchema,
])

function parseInteger(value: string, name: string): bigint {
  if (!CANONICAL_INTEGER.test(value)) {
    throw new Error(`${name} must be a canonical integer string`)
  }
  return BigInt(value)
}

function groupInteger(value: bigint): string {
  return value.toString().replaceAll(/\B(?=(\d{3})+(?!\d))/g, ',')
}

export function formatNanoUSD(value: string): string {
  const nanoUSD = parseInteger(value, 'nano USD')
  const negative = nanoUSD < 0n
  const absolute = negative ? -nanoUSD : nanoUSD
  const whole = absolute / NANO_USD_SCALE
  const fractional = (absolute % NANO_USD_SCALE)
    .toString()
    .padStart(9, '0')
    .replace(/0+$/, '')
  const amount = `$${groupInteger(whole)}${fractional ? `.${fractional}` : ''}`
  return negative ? `-${amount}` : amount
}

export function addNanoUSD(...values: string[]): string {
  return values
    .reduce((total, value) => total + parseInteger(value, 'nano USD'), 0n)
    .toString()
}

export function formatMarginPPM(value: string | null): string {
  if (value === null) return 'N/A'

  const ppm = parseInteger(value, 'margin PPM')
  const negative = ppm < 0n
  const absolute = negative ? -ppm : ppm
  const whole = absolute / PPM_PER_PERCENT
  const fractional = (absolute % PPM_PER_PERCENT)
    .toString()
    .padStart(4, '0')
    .replace(/0+$/, '')
  const percentage = `${groupInteger(whole)}${fractional ? `.${fractional}` : ''}%`
  return negative ? `-${percentage}` : percentage
}

export function parseCostRuleForm(
  formValues: CostRuleFormValues
): CostRuleConfigV1 {
  const values = costRuleFormSchema.parse(formValues)

  if (values.cost_mode === 'free') {
    return {
      zero_cost_reason: values.zero_cost_reason,
      normalized_usd_prices: {},
    }
  }

  const paidFields = {
    currency: values.currency,
    billing_multiplier: values.billing_multiplier,
    purchase_discount_ratio: values.purchase_discount_ratio,
    recharge_exchange_ratio: values.recharge_exchange_ratio,
    fee_rate: values.fee_rate,
    currency_to_usd_rate: values.currency_to_usd_rate,
    charge_event: values.charge_event,
  }

  if (values.cost_mode === 'per_request') {
    return {
      ...paidFields,
      unit_price: values.unit_price,
      normalized_usd_prices: {},
    }
  }

  if (values.cost_mode === 'per_duration') {
    return {
      ...paidFields,
      meter_source: values.meter_source,
      price_per_second: values.price_per_second,
      normalized_usd_prices: {},
    }
  }

  if (values.token_mode === 'total_tokens') {
    return {
      ...paidFields,
      meter_source: values.meter_source,
      token_mode: 'total_tokens',
      total_per_million: values.total_per_million,
      normalized_usd_prices: {},
    }
  }
  if (values.token_mode === 'completion_tokens') {
    return {
      ...paidFields,
      meter_source: values.meter_source,
      token_mode: 'completion_tokens',
      completion_per_million: values.completion_per_million,
      normalized_usd_prices: {},
    }
  }
  return {
    ...paidFields,
    meter_source: values.meter_source,
    token_mode: 'input_output',
    input_per_million: values.input_per_million,
    output_per_million: values.output_per_million,
    normalized_usd_prices: {},
  }
}
