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
export type NanoUSD = string

export interface CostAccountingApiResponse<T> {
  success: boolean
  message: string
  data: T
}

export type CostAccountingMode = 'disabled' | 'strict'
export type CostMode = 'free' | 'per_request' | 'per_duration' | 'per_token'
export type CostRuleStatus = 'draft' | 'active' | 'retired'
export type CostRevenueStatus =
  | 'pending'
  | 'settled'
  | 'confirmed_zero'
  | 'revenue_failed'
export type CostProfitStatus =
  | 'complete'
  | 'incomplete_cost'
  | 'incomplete_revenue'
export type CostChargeEvent =
  | 'response_succeeded'
  | 'submit_accepted'
  | 'task_succeeded'
export type CostMeterSource =
  | 'validated_request'
  | 'upstream_actual'
  | 'upstream_usage'
  | 'local_usage'
export type DurationCostMeterSource = Extract<
  CostMeterSource,
  'validated_request' | 'upstream_actual'
>
export type TokenCostMeterSource = Extract<
  CostMeterSource,
  'upstream_usage' | 'local_usage'
>
export type CostTokenMode =
  | 'total_tokens'
  | 'completion_tokens'
  | 'input_output'
export type CostAttemptStatus =
  | 'prepared'
  | 'dispatching'
  | 'not_dispatched'
  | 'awaiting_meter'
  | 'settled'
  | 'confirmed_zero'
  | 'cost_unknown'
  | 'settlement_failed'
export type CostReconciliationStatus = 'none' | 'reconciled'

export interface CostRulePricesV1 {
  unit_price?: string
  price_per_second?: string
  total_per_million?: string
  completion_per_million?: string
  input_per_million?: string
  output_per_million?: string
}

export interface CostRuleConfigV1 extends CostRulePricesV1 {
  currency?: string
  billing_multiplier?: string
  purchase_discount_ratio?: string
  recharge_exchange_ratio?: string
  fee_rate?: string
  currency_to_usd_rate?: string
  zero_cost_reason?: string
  charge_event?: CostChargeEvent
  meter_source?: CostMeterSource
  token_mode?: CostTokenMode
  normalized_usd_prices: CostRulePricesV1
}

interface PaidCostRuleFormFields {
  currency: string
  billing_multiplier: string
  purchase_discount_ratio: string
  recharge_exchange_ratio: string
  fee_rate: string
  currency_to_usd_rate: string
  charge_event: CostChargeEvent
}

export interface FreeCostRuleFormValues {
  cost_mode: 'free'
  zero_cost_reason: string
}

export type PerRequestCostRuleFormValues = PaidCostRuleFormFields & {
  cost_mode: 'per_request'
  unit_price: string
}

export type PerDurationCostRuleFormValues = PaidCostRuleFormFields & {
  cost_mode: 'per_duration'
  meter_source: DurationCostMeterSource
  price_per_second: string
}

type TokenCostRuleFormFields = Omit<PaidCostRuleFormFields, 'charge_event'> & {
  charge_event: Exclude<CostChargeEvent, 'submit_accepted'>
}

export type TotalTokenCostRuleFormValues = TokenCostRuleFormFields & {
  cost_mode: 'per_token'
  meter_source: TokenCostMeterSource
  token_mode: 'total_tokens'
  total_per_million: string
}

export type CompletionTokenCostRuleFormValues = TokenCostRuleFormFields & {
  cost_mode: 'per_token'
  meter_source: TokenCostMeterSource
  token_mode: 'completion_tokens'
  completion_per_million: string
}

export type SplitTokenCostRuleFormValues = TokenCostRuleFormFields & {
  cost_mode: 'per_token'
  meter_source: TokenCostMeterSource
  token_mode: 'input_output'
  input_per_million: string
  output_per_million: string
}

export type CostRuleFormValues =
  | FreeCostRuleFormValues
  | PerRequestCostRuleFormValues
  | PerDurationCostRuleFormValues
  | TotalTokenCostRuleFormValues
  | CompletionTokenCostRuleFormValues
  | SplitTokenCostRuleFormValues

export interface CostAccountingSettings {
  mode: CostAccountingMode
}

export interface CostRule {
  id: number
  channel_id: number
  billable_upstream_model: string
  version: number
  status: CostRuleStatus
  cost_mode: CostMode
  schema_version: number
  config: CostRuleConfigV1
  source: string
  note: string
  created_by: number
  activated_by: number
  effective_from?: number
  effective_to?: number
  created_at: number
  updated_at: number
}

export interface CostRuleWriteRequest {
  channel_id: number
  billable_upstream_model: string
  cost_mode: CostMode
  config: CostRuleConfigV1
  note?: string
  request_path?: string
  task_platform?: 'suno' | 'mj'
}

export type CostRuleUpdateRequest = Omit<
  CostRuleWriteRequest,
  'channel_id' | 'billable_upstream_model'
>

export interface CostRuleListParams {
  channel_id?: number
  billable_upstream_model?: string
}

export interface CostRuleValidationResult {
  valid: true
  config: CostRuleConfigV1
}

export interface CostMeter {
  source: CostMeterSource | ''
  duration_seconds?: string
  input_tokens?: number
  output_tokens?: number
  completion_tokens?: number
  total_tokens?: number
}

export interface CostPreviewUsage {
  prompt_tokens?: number
  completion_tokens?: number
  total_tokens?: number
  input_tokens?: number
  output_tokens?: number
  generated_images?: number
  input_images?: number
  [key: string]: unknown
}

export interface CostPreviewTokenMeta {
  token_type?: 'text_number' | 'tokenizer' | 'image'
  combine_text?: string
  tools_count?: number
  name_count?: number
  messages_count?: number
  files?: unknown[]
  max_tokens?: number
  image_ratio?: number
  billing_ratios?: Record<string, number>
}

export interface CostExpressionRequestInput {
  headers?: Record<string, string>
  body?: Record<string, unknown>
}

export interface CostPreviewRequest {
  origin_model: string
  user_group: string
  relay_mode: number
  request_path?: string
  usage?: CostPreviewUsage
  token_meta?: CostPreviewTokenMeta
  duration_seconds?: number
  expression_request_input?: CostExpressionRequestInput
  cost_mode: CostMode
  config: CostRuleConfigV1
  meter: CostMeter
}

export interface CostPreviewResult {
  estimated: boolean
  original_cost: string
  revenue_nano_usd: NanoUSD
  cost_nano_usd: NanoUSD
  profit_nano_usd: NanoUSD
  margin_ppm?: string
}

export interface CostCoverageParams {
  channel_id?: number
  origin_model?: string
  billable_upstream_model?: string
}

export interface CostCoverageItem {
  channel_id: number
  origin_model: string
  predicted_upstream_model: string
  covered: boolean
  reason?: string
}

export interface CostAccountingRequestLedger {
  id: number
  request_id: string
  task_id?: string
  upstream_task_id?: string
  user_id: number
  token_id: number
  user_group: string
  using_group: string
  origin_model_name: string
  billing_source: string
  subscription_id: number
  subscription_plan_id: number
  final_user_quota?: number
  quota_per_unit_snapshot: string
  billed_revenue_equivalent_nano_usd?: NanoUSD
  confirmed_cost_nano_usd: NanoUSD
  attempt_count: number
  winning_attempt_id?: number
  billed_gross_profit_nano_usd?: NanoUSD
  gross_margin_ppm?: string
  revenue_status: CostRevenueStatus
  profit_status: CostProfitStatus
  failure_code: string
  requested_at: number
  revenue_settled_at?: number
  profit_recognized_at?: number
  created_at: number
  updated_at: number
}

export interface CostAccountingAttemptLedger {
  id: number
  cost_request_id: number
  attempt_no: number
  channel_id: number
  channel_name: string
  channel_type: number
  predicted_upstream_model: string
  billable_upstream_model: string
  rule_id: number
  rule_version: number
  cost_mode: CostMode
  schema_version: number
  rule_config_json: string
  charge_event: CostChargeEvent | ''
  meter_source: CostMeterSource | ''
  billable_request_count: number
  request_meter_json: string
  actual_meter_json: string
  original_cost: string
  cost_nano_usd?: NanoUSD
  upstream_accepted: boolean
  http_status: number
  result_code: string
  failure_code: string
  status: CostAttemptStatus
  reconciliation_status: CostReconciliationStatus
  prepared_at: number
  dispatching_at?: number
  accepted_at?: number
  terminal_at?: number
  settled_at?: number
  created_at: number
  updated_at: number
}

export interface CostAccountingAudit {
  id: number
  cost_request_id: number
  cost_attempt_id?: number
  admin_id: number
  old_state: string
  new_state: string
  meter_json: string
  rule_id: number
  rule_version: number
  old_amount_nano_usd?: NanoUSD
  new_amount_nano_usd?: NanoUSD
  reason: string
  created_at: number
}

export interface CostRequestAttempt {
  attempt: CostAccountingAttemptLedger
  winning: boolean
}

export interface CostRequestDetail {
  request: CostAccountingRequestLedger
  attempts: CostRequestAttempt[]
  audits: CostAccountingAudit[]
}

export interface CostAnomaly {
  kind: string
  request: CostAccountingRequestLedger
  attempt?: CostAccountingAttemptLedger
  occurred_at: number
}

export interface CostAnomalyParams {
  page?: number
  page_size?: number
  kind?: string
  channel_id?: number
  start_time?: number
  end_time?: number
}

export interface CostAnomalyPage {
  items: CostAnomaly[]
  total: number
  page: number
  page_size: number
}

export type ReconcileCostAttemptRequest =
  | { action: 'settle'; meter?: CostMeter; reason: string }
  | { action: 'confirm_zero'; meter?: CostMeter; reason: string }

export type ReconcileCostRevenueRequest =
  | { action: 'settle'; final_user_quota: number; reason: string }
  | { action: 'confirm_zero'; final_user_quota?: 0; reason: string }

export interface CostReportParams {
  time_basis?: 'profit_recognized_at' | 'requested_at'
  start_time?: number
  end_time?: number
  channel_id?: number
  billable_upstream_model?: string
  origin_model?: string
  user_group?: string
  using_group?: string
  billing_source?: string
  status?: string
}

export interface CostProfitSummary {
  realized_revenue_nano_usd: NanoUSD
  realized_cost_nano_usd: NanoUSD
  realized_profit_nano_usd: NanoUSD
  gross_margin_ppm?: string
  known_incomplete_cost_nano_usd: NanoUSD
  complete_request_count: number
  negative_profit_request_count: number
  retry_attempt_count: number
  awaiting_meter_count: number
  unknown_cost_count: number
  settlement_failed_count: number
  revenue_failed_count: number
}

export interface CostProfitBreakdown extends CostProfitSummary {
  channel_id: number
  channel_name: string
  billable_upstream_model: string
  attempt_count: number
}
