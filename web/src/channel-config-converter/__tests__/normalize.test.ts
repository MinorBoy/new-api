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
import assert from 'node:assert/strict'
import test from 'node:test'

import {
  canonicalDecimal,
  formulaPreviewMismatch,
  NormalizationError,
  normalizeEntities,
  normalizeEnum,
  normalizeSourceUrl,
} from '../normalize'
import { scanForSecrets } from '../security-scan'
import type { ExtractedEntity } from '../types'

const cell = (value: boolean | number | string | null) => ({
  value,
  formula: null,
  formulaResult: null,
})

test('normalizes enum text, decimal strings, source URLs, and stable entity order', () => {
  assert.equal(normalizeEnum('  Per Request  '), 'per_request')
  assert.equal(canonicalDecimal('0001.2300'), '1.23')
  assert.equal(canonicalDecimal('-0.000'), '0')
  assert.equal(
    normalizeSourceUrl('https://pricing.example/path?token=removed#section'),
    'https://pricing.example/path'
  )

  const entities: ExtractedEntity[] = [
    {
      businessId: 'Z-COST',
      fields: {
        native_unit_price: cell('01.2000'),
        计费倍率: cell('01.000'),
      },
      sourceLocations: [{ sheet: '成本', row: 6, businessId: 'Z-COST' }],
    },
    {
      businessId: 'A-COST',
      fields: { native_unit_price: cell('2.000') },
      sourceLocations: [{ sheet: '成本', row: 5, businessId: 'A-COST' }],
    },
  ]
  assert.deepEqual(
    normalizeEntities(entities).map((entity) => entity.business_id),
    ['A-COST', 'Z-COST']
  )
  assert.equal(normalizeEntities(entities)[1].fields.native_unit_price, '1.2')
  assert.equal(normalizeEntities(entities)[1].fields.计费倍率, '1')
})

test('rejects invalid authoritative decimals instead of rounding or coercing them', () => {
  for (const value of ['NaN', 'Infinity', '-1', '1.000001']) {
    assert.throws(
      () =>
        canonicalDecimal(value, { nonNegative: true, maxFractionDigits: 5 }),
      (error: unknown) => error instanceof NormalizationError
    )
  }
  assert.throws(
    () => canonicalDecimal('0', { positive: true }),
    (error: unknown) => error instanceof NormalizationError
  )
})

test('reports formula preview mismatches without treating cached formulas as authoritative', () => {
  assert.deepEqual(
    formulaPreviewMismatch(
      { value: 4, formula: '=A1*2', formulaResult: 4 },
      '4'
    ),
    null
  )
  assert.deepEqual(
    formulaPreviewMismatch(
      { value: 5, formula: '=A1*2', formulaResult: 5 },
      '4'
    ),
    {
      code: 'COST_NORMALIZATION_MISMATCH',
      formula: '=A1*2',
      preview: '5',
      recomputed: '4',
    }
  )
})

test('finds credential-like field names and values without returning the secret value', () => {
  const findings = scanForSecrets({
    api_key: 'not-returned',
    source_url: 'https://pricing.example/path',
    nested: { authorization: 'Bearer abcdefghijklmnopqrstuvwxyz' },
  })

  assert.deepEqual(
    findings.map((finding) => ({ code: finding.code, path: finding.path })),
    [
      { code: 'SECURITY_SECRET_FIELD', path: '$.api_key' },
      { code: 'SECURITY_SECRET_FIELD', path: '$.nested.authorization' },
      { code: 'SECURITY_SECRET_VALUE', path: '$.nested.authorization' },
    ]
  )
  assert.equal(
    findings.some((finding) => 'value' in finding),
    false
  )
})
