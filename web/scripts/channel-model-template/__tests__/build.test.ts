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

import Decimal from 'decimal.js'

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
          系列: 2,
          版本: 'fast',
          计费: 'second',
          '单价 元': 1.38,
          参考图数: 9,
          参考视频数: 3,
          参考音频数: 3,
          最大素材数: 12,
          视频音频合计上限: 3,
          '参考视频总时长上限 秒': 15,
          最小参考图数: 1,
          清晰度: '720',
          超分: '否',
          时长范围: '4-15',
          比例: '9:16',
          过真人脸: '是',
          状态: '正常',
          协议: '自有',
          上游模型分组: '默认',
          备注: null,
        },
      },
    ],
    officialPrices: includePrice
      ? [
          {
            location: { sheet: 'sd官价', row: 7 },
            fields: {
              系列: 2,
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

function firstSourceModel(
  source: SourceWorkbook
): SourceWorkbook['models'][number] {
  const model = source.models[0]
  assert.ok(model)
  return model
}

function ffLinkSource(): SourceWorkbook {
  const source = sourceWithOfficialPrice()
  const sourceModel = firstSourceModel(source)
  source.channels[0] = {
    location: { sheet: 'channel', row: 17 },
    fields: {
      渠道: 15,
      名称: 'fflink',
      链接: 'https://api.fflink.top',
      'Base Url': 'https://api.fflink.top',
    },
  }
  const rows = [
    { model: 'seedance-2.0', version: '标准', resolution: '480p', price: 0.12 },
    { model: 'seedance-2.0', version: '标准', resolution: '720p', price: 0.25 },
    { model: 'seedance-2.0', version: '标准', resolution: '1080p', price: 0.6 },
    {
      model: 'seedance-2.0-fast',
      version: 'fast',
      resolution: '480p',
      price: 0.1,
    },
    {
      model: 'seedance-2.0-fast',
      version: 'fast',
      resolution: '720p',
      price: 0.2,
    },
    {
      model: 'seedance-2.0-mini',
      version: 'mini',
      resolution: '720p',
      price: 0.17,
    },
  ]
  const dimensions: Record<string, [number, number]> = {
    '480p': [864, 496],
    '720p': [1280, 720],
    '1080p': [1920, 1080],
  }
  source.models = rows.map((row, index) => ({
    location: { sheet: 'sd', row: 221 + index },
    fields: {
      ...sourceModel.fields,
      渠道: 15,
      模型ID: row.model,
      版本: row.version,
      清晰度: row.resolution,
      '单价 元': row.price,
      参考图数: 4,
      参考视频数: 3,
      参考音频数: 1,
      最大素材数: 8,
      视频音频合计上限: null,
      '参考视频总时长上限 秒': 15,
      最小参考图数: 0,
      时长范围: '4-15',
      比例: 'auto',
      过真人脸: '否',
      上游模型分组: '默认',
    },
  }))
  source.officialPrices = rows.map((row, index) => ({
    location: { sheet: 'sd官价', row: 7 + index },
    fields: {
      系列: 2,
      模型: row.model,
      版本: row.version,
      分辨率: row.resolution,
      '不含视频 元/M': 46,
      '包含视频 元/M': 28,
      帧率: 24,
      长边: dimensions[row.resolution]?.[0],
      短边: dimensions[row.resolution]?.[1],
      '不含视频 元/秒': 1,
      '包含视频 元/秒': 1,
      备注: null,
    },
  }))
  return source
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
  assert.equal(cost.chargeEvent, 'task_succeeded')
})

test('maps all latest billing modes from one unit price field', () => {
  const cases = [
    ['second', 'per_duration', 'nativePerSecond'],
    ['call', 'per_request', 'nativePerRequest'],
    ['token', 'per_token', 'nativePerMillion'],
  ] as const

  for (const [billingMode, expectedMode, priceField] of cases) {
    const source = sourceWithOfficialPrice()
    const model = firstSourceModel(source)
    model.fields.计费 = billingMode
    model.fields['单价 元'] = 2

    const cost = buildTemplateData(source, rules).costs[0]

    assert.equal(cost?.mode, expectedMode)
    assert.equal(cost?.[priceField], '2')
  }
})

test('keeps a source row without an official SKU as draft', () => {
  const output = buildTemplateData(sourceWithOfficialPrice(false), rules)
  const cost = output.costs[0]

  assert.equal(cost?.status, 'draft')
  assert.equal(output.issues[0]?.code, 'SKU_UNRESOLVED')
  assert.equal(output.issues[0]?.severity, 'WARN')
})

test('keeps every invalid unit price as a disabled draft', () => {
  for (const value of [null, 0, -1, 'invalid']) {
    const source = sourceWithOfficialPrice()
    firstSourceModel(source).fields['单价 元'] = value

    const output = buildTemplateData(source, rules)

    assert.equal(output.costs[0]?.status, 'draft')
    assert.equal(output.mappings[0]?.enabled, '否')
    assert.ok(
      output.issues.some(
        (item) => item.code === 'COST_PRICE_INVALID' && item.severity === 'WARN'
      )
    )
  }
})

test('rejects an unsupported latest billing mode', () => {
  const source = sourceWithOfficialPrice()
  firstSourceModel(source).fields.计费 = 'minute'

  const output = buildTemplateData(source, rules)

  assert.ok(
    output.issues.some(
      (item) => item.code === 'COST_MODE_UNKNOWN' && item.severity === 'FAIL'
    )
  )
})

test('keeps a source row without structured reference limits as draft', () => {
  const source = sourceWithOfficialPrice()
  firstSourceModel(source).fields.最大素材数 = null

  const output = buildTemplateData(source, rules)

  assert.equal(output.costs[0]?.status, 'draft')
  assert.ok(
    output.issues.some(
      (item) =>
        item.code === 'REFERENCE_CONTRACT_UNRESOLVED' &&
        item.severity === 'WARN' &&
        item.message.length > 0
    )
  )
})

test('writes the structured reference contract to the mapping audit note', () => {
  const output = buildTemplateData(sourceWithOfficialPrice(), rules)
  const note = output.mappings[0]?.note ?? ''

  assert.match(note, /参考图数=9/)
  assert.match(note, /参考视频数=3/)
  assert.match(note, /参考音频数=3/)
  assert.match(note, /最大素材数=12/)
  assert.match(note, /视频音频合计上限=3/)
  assert.match(note, /素材模式=/)
  assert.match(note, /上游模型分组=默认/)
  assert.match(note, /参考视频总时长上限秒=15/)
  assert.match(note, /最小参考图数=1/)
})

test('normalizes supplier 2160p rows to the runtime 4k resolution', () => {
  const source = sourceWithOfficialPrice()
  firstSourceModel(source).fields.清晰度 = '2160p'

  const output = buildTemplateData(source, rules)

  assert.match(output.mappings[0]?.skuCode ?? '', /4K/u)
})

test('caps aggregate reference totals by the combined video and audio limit', () => {
  const source = sourceWithOfficialPrice()
  firstSourceModel(source).fields.最大素材数 = 15
  firstSourceModel(source).fields.视频音频合计上限 = 3

  const output = buildTemplateData(source, rules)
  const note = output.mappings[0]?.note ?? ''

  assert.equal(output.costs[0]?.status, 'active')
  assert.match(note, /最大素材数=12/)
})

test('ignores a reference video duration when the row allows no videos', () => {
  const source = sourceWithOfficialPrice()
  firstSourceModel(source).fields.参考视频数 = 0

  const output = buildTemplateData(source, rules)
  const note = output.mappings[0]?.note ?? ''

  assert.equal(output.costs[0]?.status, 'active')
  assert.match(note, /参考视频总时长上限秒=0/)
})

test('derives video input support from the reference-video count', () => {
  for (const [count, expected] of [
    [0, '否'],
    [3, '是'],
  ] as const) {
    const source = sourceWithOfficialPrice()
    firstSourceModel(source).fields.参考视频数 = count

    const output = buildTemplateData(source, rules)

    assert.equal(output.skus[0]?.supportsVideoInput, expected)
  }
})

test('matches official prices within the same Seedance series', () => {
  const source = sourceWithOfficialPrice()
  const model = firstSourceModel(source)
  const seriesTwoPrice = source.officialPrices[0]
  assert.ok(seriesTwoPrice)
  model.fields.系列 = 2.5
  seriesTwoPrice.fields.系列 = 2
  seriesTwoPrice.fields.模型 = 'seedance-2.5'
  seriesTwoPrice.fields['不含视频 元/M'] = 11
  seriesTwoPrice.fields['包含视频 元/M'] = 7
  const seriesTwoPointFivePrice = structuredClone(seriesTwoPrice)
  seriesTwoPointFivePrice.location.row = 8
  seriesTwoPointFivePrice.fields.系列 = 2.5
  seriesTwoPointFivePrice.fields['不含视频 元/M'] = 55
  seriesTwoPointFivePrice.fields['包含视频 元/M'] = 33
  source.officialPrices.push(seriesTwoPointFivePrice)

  const output = buildTemplateData(
    source,
    parseRules({
      ...rulesInput,
      modelRules: {
        'seedance 720p-fast': {
          ...rulesInput.modelRules['seedance 720p-fast'],
          clientModel: 'seedance-2.5',
        },
      },
    })
  )

  assert.equal(output.skus[0]?.series, '2.5')
  assert.equal(
    output.sales.find((sale) => sale.scenario === 'no_video')?.nativePerMillion,
    '55'
  )
})

test('derives Dimensio reference modes and aspect ratios from the verified contract', () => {
  const source = sourceWithOfficialPrice()
  firstSourceModel(source).fields.渠道 = 7
  firstSourceModel(source).fields.比例 = 'auto'

  const output = buildTemplateData(source, rules)
  const note = output.mappings[0]?.note ?? ''

  assert.match(note, /素材模式=first_last_frames,omni_reference,agentic/)
  assert.match(note, /归一化比例=1:1,4:3,3:4,16:9,9:16,21:9/)
})

test('derives Secure overseas reference modes and aspect ratios from the verified contract', () => {
  const source = sourceWithOfficialPrice()
  firstSourceModel(source).fields.渠道 = 5
  firstSourceModel(source).fields.上游模型分组 = 'video-海外'
  firstSourceModel(source).fields.比例 = 'auto'

  const output = buildTemplateData(source, rules)
  const note = output.mappings[0]?.note ?? ''

  assert.match(note, /素材模式=first_last_frames,omni_reference/)
  assert.match(note, /归一化比例=1:1,4:3,3:4,16:9,9:16,21:9/)
})

test('accepts documented Cangyuan 431 limits and derives its media modes', () => {
  const source = sourceWithOfficialPrice()
  const channel = source.channels[0]
  const model = firstSourceModel(source)
  assert.ok(channel)
  channel.fields.渠道 = 6
  model.fields.渠道 = 6
  model.fields.模型ID = 'seedance-2.0'
  model.fields.参考图数 = 4
  model.fields.参考视频数 = 3
  model.fields.参考音频数 = 1
  model.fields.最大素材数 = 8
  model.fields.视频音频合计上限 = null
  model.fields.最小参考图数 = 0
  model.fields.比例 = 'auto'

  const output = buildTemplateData(
    source,
    parseRules({
      ...rulesInput,
      channelCodes: { ...rulesInput.channelCodes, '6': 'CH-CANGYUANSUANLI' },
      modelRules: {},
    })
  )

  assert.equal(
    output.issues.some(
      (item) =>
        item.severity === 'FAIL' && item.code.startsWith('CHANNEL_CONTRACT_')
    ),
    false
  )
  assert.match(
    output.mappings[0]?.note ?? '',
    /素材模式=first_last_frames,omni_reference/
  )
})

test('accepts documented Cangyuan sd5 shared media limits', () => {
  const source = sourceWithOfficialPrice()
  const channel = source.channels[0]
  const model = firstSourceModel(source)
  assert.ok(channel)
  channel.fields.渠道 = 6
  model.fields.渠道 = 6
  model.fields.模型ID = 'sd5-seedance-2.0-fast'
  model.fields.参考图数 = 9
  model.fields.参考视频数 = 3
  model.fields.参考音频数 = 3
  model.fields.最大素材数 = 12
  model.fields.视频音频合计上限 = 3
  model.fields.最小参考图数 = 0
  model.fields.比例 = 'auto'

  const output = buildTemplateData(
    source,
    parseRules({
      ...rulesInput,
      channelCodes: { ...rulesInput.channelCodes, '6': 'CH-CANGYUANSUANLI' },
      modelRules: {},
    })
  )

  assert.equal(
    output.issues.some(
      (item) =>
        item.severity === 'FAIL' && item.code.startsWith('CHANNEL_CONTRACT_')
    ),
    false
  )
})

test('applies the Cangyuan sd5 shared media protocol when the source omits the aggregate field', () => {
  const source = sourceWithOfficialPrice()
  const channel = source.channels[0]
  const model = firstSourceModel(source)
  assert.ok(channel)
  channel.fields.渠道 = 6
  model.fields.渠道 = 6
  model.fields.模型ID = 'sd5-seedance-2.0'
  model.fields.参考图数 = 9
  model.fields.参考视频数 = 3
  model.fields.参考音频数 = 3
  model.fields.最大素材数 = 12
  model.fields.视频音频合计上限 = null
  model.fields.最小参考图数 = 0
  model.fields.比例 = 'auto'

  const output = buildTemplateData(
    source,
    parseRules({
      ...rulesInput,
      channelCodes: { ...rulesInput.channelCodes, '6': 'CH-CANGYUANSUANLI' },
      modelRules: {},
    })
  )

  assert.equal(
    output.issues.some(
      (item) =>
        item.severity === 'FAIL' && item.code.startsWith('CHANNEL_CONTRACT_')
    ),
    false
  )
  assert.match(output.mappings[0]?.note ?? '', /视频音频合计上限=3/)
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

test('interprets an Excel date in the duration range as month-day bounds', () => {
  const source = sourceWithOfficialPrice()
  firstSourceModel(source).fields.时长范围 = new Date(
    '2026-04-15T00:00:00.000Z'
  )

  const output = buildTemplateData(
    source,
    parseRules({ ...rulesInput, modelRules: {} })
  )
  const sku = output.skus.find((item) => item.model === 'seedance-2.0')

  assert.equal(sku?.minDurationSeconds, 4)
  assert.equal(sku?.maxDurationSeconds, 15)
})

test('treats a single duration value as an exact channel duration', () => {
  const source = sourceWithOfficialPrice()
  firstSourceModel(source).fields.时长范围 = '15'

  const output = buildTemplateData(
    source,
    parseRules({ ...rulesInput, modelRules: {} })
  )

  assert.equal(output.skus[0]?.minDurationSeconds, 15)
  assert.equal(output.skus[0]?.maxDurationSeconds, 15)
  assert.equal(output.mappings[0]?.minDurationSeconds, 15)
  assert.equal(output.mappings[0]?.maxDurationSeconds, 15)
})

test('preserves discrete channel durations without widening them into a range', () => {
  const source = sourceWithOfficialPrice()
  firstSourceModel(source).fields.时长范围 = '5,10,15'

  const output = buildTemplateData(
    source,
    parseRules({ ...rulesInput, modelRules: {} })
  )

  assert.deepEqual(output.mappings[0]?.durationValues, [5, 10, 15])
  assert.equal(output.mappings[0]?.minDurationSeconds, undefined)
  assert.equal(output.mappings[0]?.maxDurationSeconds, undefined)
})

test('aggregates SKU duration while preserving each channel mapping duration', () => {
  const source = sourceWithOfficialPrice()
  const first = firstSourceModel(source)
  first.fields.时长范围 = '5-15'
  const second = structuredClone(first)
  second.location.row = 4
  second.fields.时长范围 = '4-15'
  source.models.push(second)

  const output = buildTemplateData(
    source,
    parseRules({ ...rulesInput, modelRules: {} })
  )

  assert.equal(output.skus[0]?.minDurationSeconds, 4)
  assert.equal(output.skus[0]?.maxDurationSeconds, 15)
  assert.deepEqual(
    output.mappings.map((mapping) => ({
      row: mapping.sourceRow,
      min: mapping.minDurationSeconds,
      max: mapping.maxDurationSeconds,
    })),
    [
      { row: 3, min: 5, max: 15 },
      { row: 4, min: 4, max: 15 },
    ]
  )
})

test('blocks verified channel contract conflicts before workbook generation', () => {
  const contractRules = parseRules({
    ...rulesInput,
    channelCodes: {
      '1': 'CH-CLMM',
      '4': 'CH-8YES',
      '5': 'CH-SECURE',
      '6': 'CH-CANGYUANSUANLI',
      '8': 'CH-MEGABYAI',
      '15': 'CH-FFLINK',
    },
    modelRules: {},
  })
  const tests = [
    {
      name: 'Cangyuan reference limit overflow',
      channel: 6,
      mutate: (model: SourceWorkbook['models'][number]) => {
        model.fields.模型ID = 'seedance-2.0'
        model.fields.参考图数 = 5
        model.fields.参考视频数 = 3
        model.fields.参考音频数 = 1
        model.fields.最大素材数 = 9
        model.fields.视频音频合计上限 = null
        model.fields.最小参考图数 = 0
      },
      code: 'CHANNEL_CONTRACT_REFERENCES',
    },
    {
      name: '8yes model resolution mismatch',
      channel: 4,
      mutate: (model: SourceWorkbook['models'][number]) => {
        model.fields.模型ID = 'videos-mini-480p'
        model.fields.清晰度 = '720p'
      },
      code: 'CHANNEL_CONTRACT_RESOLUTION',
    },
    {
      name: 'MegaByAI unsupported resolution',
      channel: 8,
      mutate: (model: SourceWorkbook['models'][number]) => {
        model.fields.模型ID = 'videos-standard'
        model.fields.清晰度 = '1440p'
      },
      code: 'CHANNEL_CONTRACT_RESOLUTION',
    },
    {
      name: 'Secure enterprise four second duration',
      channel: 5,
      mutate: (model: SourceWorkbook['models'][number]) => {
        model.fields.模型ID = 'video-2.0-pro'
        model.fields.清晰度 = '720p'
        model.fields.上游模型分组 = 'video-企业'
        model.fields.时长范围 = '4-15'
        model.fields.参考视频数 = 0
        model.fields.最大素材数 = 12
      },
      code: 'CHANNEL_CONTRACT_DURATION',
    },
    {
      name: 'FYLink unknown model',
      channel: 15,
      mutate: (model: SourceWorkbook['models'][number]) => {
        model.fields.模型ID = 'seedance-unknown'
        model.fields.清晰度 = '720p'
        model.fields.参考图数 = 4
        model.fields.参考视频数 = 3
        model.fields.参考音频数 = 1
        model.fields.最大素材数 = 8
        model.fields.最小参考图数 = 0
      },
      code: 'CHANNEL_CONTRACT_MODEL',
    },
    {
      name: 'FYLink fast 1080p',
      channel: 15,
      mutate: (model: SourceWorkbook['models'][number]) => {
        model.fields.模型ID = 'seedance-2.0-fast'
        model.fields.清晰度 = '1080p'
        model.fields.参考图数 = 4
        model.fields.参考视频数 = 3
        model.fields.参考音频数 = 1
        model.fields.最大素材数 = 8
        model.fields.最小参考图数 = 0
      },
      code: 'CHANNEL_CONTRACT_RESOLUTION',
    },
    {
      name: 'FYLink reference overflow',
      channel: 15,
      mutate: (model: SourceWorkbook['models'][number]) => {
        model.fields.模型ID = 'seedance-2.0'
        model.fields.清晰度 = '720p'
        model.fields.参考图数 = 5
        model.fields.参考视频数 = 3
        model.fields.参考音频数 = 1
        model.fields.最大素材数 = 9
        model.fields.最小参考图数 = 0
      },
      code: 'CHANNEL_CONTRACT_REFERENCES',
    },
    {
      name: 'FYLink 1080p fifteen second duration',
      channel: 15,
      mutate: (model: SourceWorkbook['models'][number]) => {
        model.fields.模型ID = 'seedance-2.0'
        model.fields.清晰度 = '1080p'
        model.fields.参考图数 = 4
        model.fields.参考视频数 = 3
        model.fields.参考音频数 = 1
        model.fields.最大素材数 = 8
        model.fields.最小参考图数 = 0
      },
      code: 'CHANNEL_CONTRACT_DURATION',
    },
  ]

  for (const testCase of tests) {
    const source = sourceWithOfficialPrice()
    const channel = source.channels[0]
    const model = source.models[0]
    assert.ok(channel)
    assert.ok(model)
    channel.fields.渠道 = testCase.channel
    model.fields.渠道 = testCase.channel
    testCase.mutate(model)

    const output = buildTemplateData(source, contractRules)

    assert.ok(
      output.issues.some(
        (item) => item.code === testCase.code && item.severity === 'FAIL'
      ),
      testCase.name
    )
  }
})

test('generates FYLink mappings and applies the R223 duration override', () => {
  const output = buildTemplateData(
    ffLinkSource(),
    parseRules({
      ...rulesInput,
      channelCodes: { '15': 'CH-FFLINK' },
      modelRules: {},
      overrides: { '15/R223': { maxDurationSeconds: 12 } },
    })
  )
  const mappings = output.mappings.filter(
    (mapping) => mapping.channelCode === 'CH-FFLINK'
  )
  const costs = output.costs.filter((cost) => cost.channelCode === 'CH-FFLINK')

  assert.equal(mappings.length, 6)
  assert.equal(costs.length, 12)
  assert.equal(
    costs.every((cost) => cost.mode === 'per_duration'),
    true
  )
  assert.deepEqual(
    [...new Set(costs.map((cost) => cost.nativePerSecond))].sort(),
    ['0.1', '0.12', '0.17', '0.2', '0.25', '0.6']
  )
  const standard1080p = mappings.find((mapping) => mapping.sourceRow === 223)
  assert.ok(standard1080p)
  assert.equal(standard1080p.sourceSheet, 'sd')
  assert.equal(standard1080p.minDurationSeconds, 4)
  assert.equal(standard1080p.maxDurationSeconds, 12)
  assert.equal(
    output.issues.some(
      (item) =>
        item.code === 'ROW_DURATION_OVERRIDE' &&
        item.row === 223 &&
        item.severity === 'WARN'
    ),
    true
  )
  assert.equal(
    output.issues.some((item) => item.severity === 'FAIL'),
    false
  )
})

test('applies the verified Mikoto Sora reference total override', () => {
  const source = sourceWithOfficialPrice()
  const model = firstSourceModel(source)
  model.location.row = 211
  model.fields.渠道 = 13
  model.fields.模型ID = 'sora-v3-pro'
  model.fields.清晰度 = '720p'
  model.fields.参考图数 = 9
  model.fields.参考视频数 = 3
  model.fields.参考音频数 = 3
  model.fields.最大素材数 = 15
  model.fields.视频音频合计上限 = null
  model.fields.最小参考图数 = 0
  const output = buildTemplateData(
    source,
    parseRules({
      ...rulesInput,
      channelCodes: { '13': 'CH-MIKOTO' },
      modelRules: {
        'sora-v3-pro': {
          clientModel: 'seedance-2.0',
          upstreamModel: 'sora-v3-pro',
        },
      },
      overrides: { '13/R211': { maxReferenceTotal: 12 } },
    })
  )

  assert.equal(
    output.issues.some(
      (item) =>
        item.code === 'CHANNEL_CONTRACT_REFERENCES' && item.severity === 'FAIL'
    ),
    false
  )
  assert.match(output.mappings[0]?.note ?? '', /最大素材数=12/)
})

test('accepts MegaByAI 1080p and 4k resolutions from the source contract', () => {
  for (const resolutionValue of ['1080p', '4k']) {
    const source = sourceWithOfficialPrice()
    const channel = source.channels[0]
    const model = source.models[0]
    const officialPrice = source.officialPrices[0]
    assert.ok(channel)
    assert.ok(model)
    assert.ok(officialPrice)
    channel.fields.渠道 = 8
    model.fields.渠道 = 8
    model.fields.模型ID = 'videos-standard'
    model.fields.清晰度 = resolutionValue
    officialPrice.fields.分辨率 = resolutionValue

    const output = buildTemplateData(
      source,
      parseRules({
        ...rulesInput,
        channelCodes: { '8': 'CH-MEGABYAI' },
        modelRules: {},
      })
    )

    assert.equal(
      output.issues.some((item) => item.code === 'CHANNEL_CONTRACT_RESOLUTION'),
      false,
      resolutionValue
    )
    assert.equal(
      output.costs.every((cost) => cost.status === 'active'),
      true,
      resolutionValue
    )
    assert.equal(output.mappings[0]?.enabled, '是', resolutionValue)
  }
})

test('quarantines an explicit draft contract conflict without blocking the workbook', () => {
  const source = sourceWithOfficialPrice()
  const channel = source.channels[0]
  const model = source.models[0]
  assert.ok(channel)
  assert.ok(model)
  channel.fields.渠道 = 6
  model.fields.渠道 = 6
  model.fields.模型ID = 'seedance-2.0'
  model.fields.参考图数 = 5
  model.fields.参考视频数 = 3
  model.fields.参考音频数 = 1
  model.fields.最大素材数 = 9
  model.fields.视频音频合计上限 = null
  model.fields.最小参考图数 = 0

  const output = buildTemplateData(
    source,
    parseRules({
      ...rulesInput,
      channelCodes: { '6': 'CH-CANGYUANSUANLI' },
      modelRules: {},
      overrides: { '6/R3': { status: 'draft' } },
    })
  )

  assert.equal(
    output.issues.some(
      (item) =>
        item.code === 'CHANNEL_CONTRACT_REFERENCES' && item.severity === 'FAIL'
    ),
    false
  )
  assert.equal(
    output.issues.some(
      (item) =>
        item.code === 'CHANNEL_CONTRACT_REFERENCES' && item.severity === 'WARN'
    ),
    true
  )
  assert.equal(
    output.costs.every((cost) => cost.status === 'draft'),
    true
  )
  assert.equal(output.mappings[0]?.enabled, '否')
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

test('keeps exact official USD per-million token prices for both scenarios', () => {
  const output = buildTemplateData(sourceWithOfficialPrice(), rules)
  const tokensPerSecond = new Decimal(1280).mul(720).mul(24).div(1024)

  assert.deepEqual(
    output.sales.map((sale) => ({
      scenario: sale.scenario,
      billingMode: sale.billingMode,
      nativePerSecond: sale.nativePerSecond,
      usdPerMillion: sale.usdPerMillion,
      usdPerSecond: sale.usdPerSecond,
    })),
    [
      {
        scenario: 'no_video',
        billingMode: 'seedance_tokens',
        nativePerSecond: '0.9936',
        usdPerMillion: '6.301369863013698',
        usdPerSecond: new Decimal('6.301369863013698')
          .mul(tokensPerSecond)
          .div(1_000_000)
          .toFixed(),
      },
      {
        scenario: 'with_video',
        billingMode: 'seedance_tokens',
        nativePerSecond: '0.6048',
        usdPerMillion: '3.835616438356164',
        usdPerSecond: new Decimal('3.835616438356164')
          .mul(tokensPerSecond)
          .div(1_000_000)
          .toFixed(),
      },
    ]
  )
})

test('calculates official sale preview from total tokens instead of treating USD per million as a request price', () => {
  const output = buildTemplateData(sourceWithOfficialPrice(), rules)
  const sale = output.sales.find((item) => item.scenario === 'with_video')
  const profit = output.profits.find((item) => item.scenario === 'with_video')

  assert.ok(sale)
  assert.ok(profit)
  const expected = new Decimal(sale.usdPerMillion)
    .mul(profit.estimatedTokens)
    .div(1_000_000)
  assert.equal(profit.officialSaleUsd, expected.toFixed())
  assert.notEqual(profit.officialSaleUsd, sale.usdPerMillion)
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

test('maps wxart seedance2.5 rows to the official seedance-2.5 family', () => {
  const source = sourceWithOfficialPrice()
  const channel = source.channels[0]
  const model = source.models[0]
  const official = source.officialPrices[0]
  assert.ok(channel)
  assert.ok(model)
  assert.ok(official)
  channel.fields.渠道 = 17
  channel.fields.名称 = 'wxart'
  channel.fields['Base Url'] = 'https://api.wxart.space'
  model.fields.渠道 = 17
  model.fields.模型ID = 'seedance2.5'
  model.fields.系列 = 2.5
  model.fields.版本 = '标准'
  model.fields.清晰度 = '720p'
  model.fields.参考图数 = 30
  model.fields.参考视频数 = 10
  model.fields.参考音频数 = 10
  model.fields.最大素材数 = 50
  model.fields['参考视频总时长上限 秒'] = 30
  model.fields.时长范围 = '4-30'
  official.fields.系列 = 2.5
  official.fields.模型 = 'seedance-2.5'
  official.fields.分辨率 = '720p'

  const output = buildTemplateData(
    source,
    parseRules({
      ...rulesInput,
      channelCodes: { '17': 'CH-WXART' },
      modelRules: { 'seedance2.5': { clientModel: 'seedance-2.5' } },
    })
  )

  assert.equal(output.skus[0]?.model, 'seedance-2.5')
  assert.equal(output.skus[0]?.status, 'active')
  assert.equal(output.sales.length, 2)
  assert.equal(output.mappings[0]?.clientModel, 'seedance-2.5')
  assert.equal(output.issues.some((item) => item.code === 'SALE_UNRESOLVED'), false)
})

test('uses the same draft SKU identity when a source model has a cross-series label', () => {
  const source = sourceWithOfficialPrice()
  const channel = source.channels[0]
  const model = source.models[0]
  const official = source.officialPrices[0]
  assert.ok(channel)
  assert.ok(model)
  assert.ok(official)
  channel.fields.渠道 = 7
  model.fields.渠道 = 7
  model.fields.模型ID = 'pxv-seedance-2.0-fast'
  model.fields.系列 = 2.5
  model.fields.版本 = '标准'
  model.fields.清晰度 = '480p'
  official.fields.系列 = 2
  official.fields.模型 = 'seedance-2.0-fast'
  official.fields.版本 = 'fast'
  official.fields.分辨率 = '480p'

  const output = buildTemplateData(
    source,
    parseRules({
      ...rulesInput,
      channelCodes: { '7': 'CH-DIMENSIO' },
      modelRules: {},
    })
  )
  const sku = output.skus.find((item) => item.model === 'pxv-seedance-2.0-fast')
  const cost = output.costs.find((item) => item.sourceRow === 3)

  assert.ok(sku)
  assert.ok(cost)
  assert.equal(cost.skuCode, sku.businessId)
  assert.equal(
    output.issues.some(
      (item) =>
        item.code === 'COST_SKU_UNRESOLVED' &&
        item.businessId === cost.businessId
    ),
    false
  )
})
