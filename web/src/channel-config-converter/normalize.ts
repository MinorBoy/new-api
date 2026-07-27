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
import Decimal from 'decimal.js'

import type { CellSnapshot, ExtractedEntity } from './types'

export class NormalizationError extends Error {
  constructor(message: string) {
    super(message)
    this.name = 'NormalizationError'
  }
}

export type DecimalOptions = {
  maxFractionDigits?: number
  nonNegative?: boolean
  positive?: boolean
}

export type NormalizedEntity = {
  business_id: string
  fields: Record<string, boolean | null | string>
  line_ref?: string
  source_locations: {
    business_id: string
    row: number
    sheet: string
  }[]
}

const DECIMAL_FIELDS = new Set([
  'native_unit_price',
  'price_per_million',
  'recharge_ratio',
  'fee_rate',
  'billing_multiplier',
  '原币按次',
  '原币/秒',
  '原币/1M',
  '原币基础单价',
  '计费倍率',
  '采购折扣',
  '充值兑换比例',
  '手续费率',
  '原币兑USD',
  '标准USD单价',
  'USD/1M',
  'USD/基准秒',
  'Token/基准秒',
  '原币/基准秒',
  '分组倍率',
])
const URL_FIELD = /(?:url|链接|价格页|引用位置)/i

function normalizeCellValue(cell: CellSnapshot): boolean | null | string {
  const value = cell.value
  if (value === null) {
    return null
  }
  if (typeof value === 'boolean') {
    return value
  }
  if (value instanceof Date) {
    return value.toISOString()
  }
  return typeof value === 'string'
    ? value.trim().replaceAll(/\s+/g, ' ')
    : String(value)
}

export function normalizeEnum(value: string): string {
  return value
    .trim()
    .toLowerCase()
    .replaceAll(/[\s-]+/g, '_')
}

export function canonicalDecimal(
  value: number | string,
  options: DecimalOptions = {}
): string {
  let decimal: Decimal
  try {
    decimal = new Decimal(value)
  } catch {
    throw new NormalizationError(`Invalid decimal: ${value}`)
  }
  if (!decimal.isFinite()) {
    throw new NormalizationError(`Decimal must be finite: ${value}`)
  }
  if (options.nonNegative && decimal.isNegative()) {
    throw new NormalizationError(`Decimal must be non-negative: ${value}`)
  }
  if (options.positive && decimal.lessThanOrEqualTo(0)) {
    throw new NormalizationError(`Decimal must be positive: ${value}`)
  }
  if (
    options.maxFractionDigits !== undefined &&
    decimal.decimalPlaces() > options.maxFractionDigits
  ) {
    throw new NormalizationError(`Decimal precision exceeds limit: ${value}`)
  }
  const normalized = decimal.isZero() ? '0' : decimal.toFixed()
  if (!/^-?(0|[1-9][0-9]*)(\.[0-9]+)?$/.test(normalized)) {
    throw new NormalizationError(`Decimal cannot be canonicalized: ${value}`)
  }
  return normalized
}

export function normalizeSourceUrl(value: string): string {
  const url = new URL(value.trim())
  url.search = ''
  url.hash = ''
  return url.toString().replaceAll(/\/$/g, '')
}

export function normalizeEntities(
  entities: ExtractedEntity[]
): NormalizedEntity[] {
  return entities
    .map((entity) => {
      const fields: NormalizedEntity['fields'] = {}
      for (const [name, cell] of Object.entries(entity.fields).sort(
        ([left], [right]) => left.localeCompare(right)
      )) {
        const value = normalizeCellValue(cell)
        if (
          typeof value === 'string' &&
          DECIMAL_FIELDS.has(name) &&
          value !== ''
        ) {
          fields[name] = canonicalDecimal(value, {
            nonNegative: true,
            maxFractionDigits: 12,
          })
        } else if (
          typeof value === 'string' &&
          URL_FIELD.test(name) &&
          /^https?:\/\//i.test(value)
        ) {
          fields[name] = normalizeSourceUrl(value)
        } else {
          fields[name] = value
        }
      }
      return {
        business_id: entity.businessId.trim(),
        fields,
        ...(entity.lineRef ? { line_ref: normalizeEnum(entity.lineRef) } : {}),
        source_locations: entity.sourceLocations
          .map((source) => ({
            business_id: source.businessId,
            row: source.row,
            sheet: source.sheet,
          }))
          .sort((left, right) =>
            `${left.sheet}/${left.row}/${left.business_id}`.localeCompare(
              `${right.sheet}/${right.row}/${right.business_id}`
            )
          ),
      }
    })
    .sort((left, right) => left.business_id.localeCompare(right.business_id))
}

export function formulaPreviewMismatch(
  cell: CellSnapshot,
  recomputed: string
): {
  code: 'COST_NORMALIZATION_MISMATCH'
  formula: string
  preview: string
  recomputed: string
} | null {
  if (!cell.formula) {
    return null
  }
  const preview = cell.formulaResult === null ? '' : String(cell.formulaResult)
  if (preview === recomputed) {
    return null
  }
  return {
    code: 'COST_NORMALIZATION_MISMATCH',
    formula: cell.formula,
    preview,
    recomputed,
  }
}
