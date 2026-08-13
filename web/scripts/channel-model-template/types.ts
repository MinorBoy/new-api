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
export type Severity = 'FAIL' | 'WARN' | 'INFO'

export type Issue = {
  code: string
  severity: Severity
  message: string
  sheet?: string
  row?: number
  field?: string
  businessId?: string
  suggestion?: string
}

export type ModelRule = {
  clientModel?: string
  upstreamModel?: string
  outputWidth?: number
  outputHeight?: number
  frameRate?: number
  minDurationSeconds?: number
  maxDurationSeconds?: number
  supportsVideoInput?: boolean
  supportsRealPerson?: boolean
  supportsSuperResolution?: boolean
}

export type RowOverride = {
  clientModel?: string
  minDurationSeconds?: number
  maxDurationSeconds?: number
  maxReferenceTotal?: number
  nativePerMillion?: string
  nativePerRequest?: string
  nativePerSecond?: string
  status?: 'active' | 'draft'
  supportsRealPerson?: boolean
  upstreamModel?: string
}

export type Rules = {
  version: string
  channelCodes: Record<string, string>
  defaults: {
    currency: string
    currencyToUsd: string
    rechargeRatio: string
    purchaseDiscountRatio: string
    tokenDivisor: number
    groupRatio: string
  }
  modelRules: Record<string, ModelRule>
  overrides: Record<string, RowOverride>
}

export type ChannelRow = {
  businessId: string
  name: string
  pricePage: string
  currency: string
  rechargeRatio: string
  feeRate: string
  billingMultiplier: string
  status: 'active' | 'draft'
  strictCostValidation: '是' | '否'
  sourceId: string
  sourceSheet: string
  sourceRow: number
  note: string
}

export type SkuRow = {
  businessId: string
  series: string
  model: string
  version: string
  resolution: string
  outputWidth: number
  outputHeight: number
  frameRate: number
  minDurationSeconds: number
  maxDurationSeconds: number
  ratio: string
  supportsVideoInput: '是' | '否'
  supportsRealPerson: '是' | '否' | '待确认'
  supportsSuperResolution: '是' | '否' | '待确认'
  measurementMethod: string
  status: 'active' | 'draft'
  sourceId: string
  sourceSheet: string
  sourceRow: number
  note: string
}

export type SaleRow = {
  businessId: string
  clientModel: string
  skuCode: string
  scenario: 'no_video' | 'with_video'
  billingMode: 'seedance_tokens' | 'per_token' | 'per_duration'
  currency: string
  nativePerMillion: string
  outputWidth: number
  outputHeight: number
  frameRate: number
  nativePerSecond: string
  usdPerMillion: string
  usdPerSecond: string
  status: 'active' | 'draft'
  sourceId: string
  sourceSheet: string
  sourceRow: number
  note: string
}

export type CostRow = {
  businessId: string
  channelCode: string
  upstreamModel: string
  skuCode: string
  scenario: 'no_video' | 'with_video'
  mode: 'per_duration' | 'per_request' | 'per_token'
  tokenSubMode: string
  meterSource: string
  tokenField: string
  chargeEvent: string
  currency: string
  nativePerRequest: string
  nativePerSecond: string
  nativePerMillion: string
  nativeBasePrice: string
  billingMultiplier: string
  purchaseDiscountRatio: string
  rechargeRatio: string
  feeRate: string
  currencyToUsd: string
  normalizedUsdUnitPrice: string
  unit: string
  status: 'active' | 'draft'
  sourceId: string
  sourceSheet: string
  sourceRow: number
  note: string
}

export type MappingRow = {
  businessId: string
  clientModel: string
  channelCode: string
  upstreamModel: string
  skuCode: string
  defaultScenario: 'no_video' | 'with_video'
  enabled: '是' | '否'
  minDurationSeconds?: number
  maxDurationSeconds?: number
  durationValues?: number[]
  sourceId: string
  sourceSheet: string
  sourceRow: number
  note: string
}

export type ProfitRow = {
  businessId: string
  saleId: string
  costId: string
  groupRatio: string
  inputVideoSeconds: number
  outputVideoSeconds: number
  skuCode: string
  scenario: 'no_video' | 'with_video'
  estimatedTokens: number
  costMode: string
  normalizedUsdUnitPrice: string
  officialSaleUsd: string
  channelCostUsd: string
  userRevenueUsd: string
  grossProfitUsd: string
  grossMargin: string
  costStatus: 'active' | 'draft'
  note: string
}

export type TemplateData = {
  channels: ChannelRow[]
  skus: SkuRow[]
  sales: SaleRow[]
  costs: CostRow[]
  mappings: MappingRow[]
  profits: ProfitRow[]
  sources: Array<{
    businessId: string
    project: string
    valueOrRange: string
    unit: string
    asOf: string
    sourceType: string
    sourceName: string
    reference: string
    owner: string
    note: string
    accessedAt: string
  }>
  issues: Issue[]
}
