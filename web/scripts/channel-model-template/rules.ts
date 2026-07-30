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

import type { ModelRule, RowOverride, Rules } from './types'

const CREDENTIAL_FIELD = /^(?:api[_-]?key|token|authorization|cookie|secret)$/iu

function asRecord(value: unknown, name: string): Record<string, unknown> {
  if (typeof value !== 'object' || value === null || Array.isArray(value)) {
    throw new Error(`${name} must be an object`)
  }
  return value as Record<string, unknown>
}

function asString(value: unknown, name: string): string {
  if (typeof value !== 'string' || value.trim() === '') {
    throw new Error(`${name} must be a non-empty string`)
  }
  return value.trim()
}

function canonicalPositiveDecimal(value: unknown, name: string): string {
  const source = asString(value, name)
  let decimal: Decimal
  try {
    decimal = new Decimal(source)
  } catch {
    throw new Error(`${name} must be a decimal string`)
  }
  if (!decimal.isFinite() || decimal.lte(0)) {
    throw new Error(`${name} must be a positive decimal string`)
  }
  return decimal.toFixed()
}

function asPositiveInteger(value: unknown, name: string): number {
  if (typeof value !== 'number' || !Number.isSafeInteger(value) || value <= 0) {
    throw new Error(`${name} must be a positive integer`)
  }
  return value
}

function rejectCredentialFields(value: unknown): void {
  if (Array.isArray(value)) {
    for (const item of value) rejectCredentialFields(item)
    return
  }
  if (typeof value !== 'object' || value === null) return
  for (const [key, child] of Object.entries(value)) {
    if (CREDENTIAL_FIELD.test(key)) {
      throw new Error(`credential-like field: ${key}`)
    }
    rejectCredentialFields(child)
  }
}

function freeze<T>(value: T): T {
  if (Array.isArray(value)) {
    for (const item of value) freeze(item)
  } else if (typeof value === 'object' && value !== null) {
    for (const item of Object.values(value)) freeze(item)
    Object.freeze(value)
  }
  return value
}

function parseModelRules(value: unknown): Record<string, ModelRule> {
  const entries = asRecord(value, 'modelRules')
  return Object.fromEntries(
    Object.entries(entries).map(([key, item]) => [
      key,
      asRecord(item, `modelRules.${key}`) as ModelRule,
    ])
  )
}

function parseOverrides(value: unknown): Record<string, RowOverride> {
  const entries = asRecord(value, 'overrides')
  return Object.fromEntries(
    Object.entries(entries).map(([key, item]) => [
      key,
      asRecord(item, `overrides.${key}`) as RowOverride,
    ])
  )
}

export function parseRules(value: unknown): Rules {
  rejectCredentialFields(value)
  const input = asRecord(value, 'rules')
  const channelCodes = asRecord(input.channelCodes, 'channelCodes')
  const defaults = asRecord(input.defaults, 'defaults')
  const normalizedCodes = Object.fromEntries(
    Object.entries(channelCodes).map(([sourceId, code]) => {
      const normalized = asString(code, `channelCodes.${sourceId}`)
      if (!/^CH-[A-Z0-9-]+$/u.test(normalized)) {
        throw new Error(`channelCodes.${sourceId} must match CH-`)
      }
      return [sourceId, normalized]
    })
  )
  const rules: Rules = {
    version: asString(input.version, 'version'),
    channelCodes: normalizedCodes,
    defaults: {
      currency: asString(defaults.currency, 'defaults.currency').toUpperCase(),
      currencyToUsd: canonicalPositiveDecimal(
        defaults.currencyToUsd,
        'defaults.currencyToUsd'
      ),
      rechargeRatio: canonicalPositiveDecimal(
        defaults.rechargeRatio,
        'defaults.rechargeRatio'
      ),
      purchaseDiscountRatio: canonicalPositiveDecimal(
        defaults.purchaseDiscountRatio,
        'defaults.purchaseDiscountRatio'
      ),
      tokenDivisor: asPositiveInteger(
        defaults.tokenDivisor,
        'defaults.tokenDivisor'
      ),
      groupRatio: canonicalPositiveDecimal(
        defaults.groupRatio,
        'defaults.groupRatio'
      ),
    },
    modelRules: parseModelRules(input.modelRules),
    overrides: parseOverrides(input.overrides),
  }
  return freeze(rules)
}
