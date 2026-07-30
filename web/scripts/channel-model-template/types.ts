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
