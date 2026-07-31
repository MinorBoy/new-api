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
import { fileURLToPath } from 'node:url'

import { buildTemplateData } from '../build'
import { parseRules } from '../rules'
import { readSourceWorkbook, type SourceWorkbook } from '../source'

const sourceFixturePath = fileURLToPath(
  new URL('../__fixtures__/sd-source-v1.xlsx', import.meta.url)
)

const rulesInput = {
  version: '1',
  channelCodes: { '1': 'CH-CLMM' },
  defaults: {
    currency: 'CNY',
    currencyToUsd: '0.136986301369863',
    rechargeRatio: '1',
    purchaseDiscountRatio: '1',
    tokenDivisor: 1024,
    groupRatio: '1',
  },
  modelRules: {
    'seedance 720p-fast': {
      clientModel: 'seedance-2.0',
      upstreamModel: 'seedance-2.0-fast',
      outputWidth: 1280,
      outputHeight: 720,
      frameRate: 24,
      minDurationSeconds: 4,
      maxDurationSeconds: 15,
    },
  },
  overrides: {},
}
const rules = parseRules(rulesInput)

function sourceWithOfficialPrice(includePrice = true): SourceWorkbook {
  return {
    channels: [
      {
        location: { sheet: 'channel', row: 3 },
        fields: {
          渠道: 1,
          名称: 'clmm',
          链接: 'https://clmm-mall.top/pricing',
          'Base Url': 'https://clmm-mall.top',
        },
      },
    ],
    models: [
      {
        location: { sheet: 'sd', row: 3 },
        fields: {
          渠道: 1,
          充值汇率: '1:1',
          手续费: null,
          计费倍率: 1,
          付费模式: '余额',
          模型ID: 'seedance 720p-fast',
          版本: 'fast',
          计费: 'second',
          '元/秒': 1.38,
          '元/次': null,
          '元/1M': null,
          素材限制: 933,
          清晰度: '720',
          超分: '否',
          时长范围: '4-15',
          比例: '9:16',
          视频输入: '是',
          过真人脸: '是',
          状态: '正常',
          协议: '自有',
          备注: null,
        },
      },
    ],
    officialPrices: includePrice
      ? [
          {
            location: { sheet: 'sd官价', row: 7 },
            fields: {
              模型: 'seedance-2.0',
              版本: '标准',
              分辨率: 720,
              '不含视频 元/M': 46,
              '包含视频 元/M': 28,
              帧率: 24,
              长边: 1280,
              短边: 720,
              '不含视频 元/秒': 0.994,
              '包含视频 元/秒': 0.605,
              备注: null,
            },
          },
        ]
      : [],
  }
}

test('maps a second-priced source row to a per-duration USD cost', () => {
  const output = buildTemplateData(sourceWithOfficialPrice(), rules)
  const cost = output.costs.find(
    (item) => item.businessId === 'COST-CLMM-R3-720-DUR-NOV'
  )

  assert.ok(cost)
  assert.equal(cost.mode, 'per_duration')
  assert.equal(cost.nativePerSecond, '1.38')
  assert.equal(cost.normalizedUsdUnitPrice, '0.18904109589041094')
  assert.equal(cost.status, 'active')
})

test('keeps a source row without an official SKU as draft', () => {
  const output = buildTemplateData(sourceWithOfficialPrice(false), rules)
  const cost = output.costs[0]

  assert.equal(cost?.status, 'draft')
  assert.equal(output.issues[0]?.code, 'SKU_UNRESOLVED')
  assert.equal(output.issues[0]?.severity, 'WARN')
})

test('keeps a source row without a material limit as draft', () => {
  const source = sourceWithOfficialPrice()
  source.models[0]!.fields.素材限制 = null

  const output = buildTemplateData(source, rules)

  assert.equal(output.costs[0]?.status, 'draft')
  assert.ok(
    output.issues.some(
      (item) =>
        item.code === 'MATERIAL_LIMIT_UNRESOLVED' && item.severity === 'WARN'
    )
  )
})

test('matches a Seedance source family to an official Seedance SKU', async () => {
  const source = await readSourceWorkbook(sourceFixturePath)
  const output = buildTemplateData(source, rules)

  assert.ok(output.skus.some((sku) => sku.model === 'seedance-2.0'))
  assert.equal(
    output.costs.find((cost) => cost.businessId === 'COST-CLMM-R3-720-DUR-NOV')
      ?.status,
    'active'
  )
})

test('infers a Seedance official model from a source model family', () => {
  const output = buildTemplateData(
    sourceWithOfficialPrice(),
    parseRules({ ...rulesInput, modelRules: {} })
  )

  assert.equal(
    output.costs.find((cost) => cost.businessId === 'COST-CLMM-R3-720-DUR-NOV')
      ?.status,
    'active'
  )
})

test('uses the configured exchange rate for official sale previews', () => {
  const output = buildTemplateData(
    sourceWithOfficialPrice(),
    parseRules({
      ...rulesInput,
      defaults: { ...rulesInput.defaults, currencyToUsd: '0.2' },
    })
  )

  assert.equal(
    output.sales.find((sale) => sale.scenario === 'no_video')?.usdPerMillion,
    '9.2'
  )
})

test('uses a registered source ID for generated cost rows', () => {
  const output = buildTemplateData(sourceWithOfficialPrice(), rules)

  assert.equal(
    output.issues.some((item) => item.code === 'COST_SOURCE_UNRESOLVED'),
    false
  )
})

test('applies a typed price override from the rules file', () => {
  const output = buildTemplateData(
    sourceWithOfficialPrice(),
    parseRules({
      ...rulesInput,
      overrides: { '3': { nativePerSecond: '2' } },
    })
  )

  assert.equal(
    output.costs.find((cost) => cost.businessId === 'COST-CLMM-R3-720-DUR-NOV')
      ?.nativePerSecond,
    '2'
  )
})

test('writes a confirmed real-person override to the mapping audit note', () => {
  const output = buildTemplateData(
    sourceWithOfficialPrice(),
    parseRules({
      ...rulesInput,
      overrides: { '3': { supportsRealPerson: false } },
    })
  )

  assert.match(output.mappings[0]?.note ?? '', /真人脸=否/)
})
