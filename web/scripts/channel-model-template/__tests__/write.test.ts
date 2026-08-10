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
import fs from 'node:fs/promises'
import os from 'node:os'
import path from 'node:path'
import test from 'node:test'
import { fileURLToPath } from 'node:url'

import { TextWriter, Uint8ArrayReader, ZipReader } from '@zip.js/zip.js'
import ExcelJS from 'exceljs'

import { convertWorkbook } from '../../../src/channel-config-converter/conversion'
import { buildTemplateData } from '../build'
import { parseRules } from '../rules'
import type { SourceWorkbook } from '../source'
import { writeTemplateWorkbook } from '../write'

const basePath = fileURLToPath(
  new URL(
    '../../../src/channel-config-converter/__fixtures__/channel-config-v1-corrected.xlsx',
    import.meta.url
  )
)

const rules = parseRules({
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
})

const source: SourceWorkbook = {
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
        备注: null,
      },
    },
  ],
  officialPrices: [
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
  ],
}

test('writes a V1 workbook recognized by the existing converter', async () => {
  const directory = await fs.mkdtemp(
    path.join(os.tmpdir(), 'channel-template-write-')
  )
  const outputPath = path.join(directory, 'template.xlsx')
  const reportPath = path.join(directory, 'template.report.json')
  try {
    const data = buildTemplateData(source, rules)
    const result = await writeTemplateWorkbook({
      basePath,
      outputPath,
      reportPath,
      sourcePath: 'source.xlsx',
      rulesPath: 'rules.json',
      rules,
      data,
    })
    const bytes = await fs.readFile(outputPath)
    const converted = await convertWorkbook(
      new File([bytes], 'channel-model-template.xlsx', {
        type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
      })
    )

    assert.equal(result.hasFailures, false)
    assert.equal(converted.document.template_version, '1')
    assert.equal(converted.hasFailures, false)
    assert.equal(converted.document.entities.channels.length, 1)
    assert.deepEqual(
      converted.document.entities.sale_proposals.map((proposal) => ({
        businessID: proposal.business_id,
        billingMode: proposal.billing_mode,
        currency: proposal.currency,
        scenario: proposal.scenario,
        seedanceTokenPrice: proposal.seedance_token_price,
      })),
      [
        {
          businessID: 'SALE-SEEDANCE-2-0-720P-NOV',
          billingMode: 'seedance_tokens',
          currency: 'USD',
          scenario: 'no_video',
          seedanceTokenPrice: {
            price_per_million: '6.301369863013698',
            width: 1280,
            height: 720,
            frame_rate: 24,
            pricing_version: 'official-token-v1',
            source: 'SRC-OFFICIAL-SEEDANCE-2-0!5',
          },
        },
        {
          businessID: 'SALE-SEEDANCE-2-0-720P-VID',
          billingMode: 'seedance_tokens',
          currency: 'USD',
          scenario: 'with_video',
          seedanceTokenPrice: {
            price_per_million: '3.835616438356164',
            width: 1280,
            height: 720,
            frame_rate: 24,
            pricing_version: 'official-token-v1',
            source: 'SRC-OFFICIAL-SEEDANCE-2-0!6',
          },
        },
      ]
    )

    const written = new ExcelJS.Workbook()
    await written.xlsx.readFile(outputPath)
    const saleSheet = written.getWorksheet('官方售价')
    const profitSheet = written.getWorksheet('利润测算')
    assert.ok(saleSheet)
    assert.ok(profitSheet)
    assert.deepEqual(saleSheet.getCell('K5').value, {
      formula: `IFERROR(H5*I5*J5/'参数'!$B$7,"")`,
      result: 21600,
    })
    assert.deepEqual(saleSheet.getCell('L5').value, {
      formula: `IFERROR(G5*K5/'参数'!$B$8,"")`,
      result: data.sales[0]?.nativePerSecond,
    })
    assert.deepEqual(saleSheet.getCell('N5').value, {
      formula: `IFERROR(M5*K5/'参数'!$B$8,"")`,
      result: data.sales[0]?.usdPerSecond,
    })
    assert.deepEqual(profitSheet.getCell('O5').value, {
      formula: `IFERROR(XLOOKUP(B5,'官方售价'!$A$5:$A$504,'官方售价'!$M$5:$M$504)*L5/'参数'!$B$8,"")`,
      result: data.profits[0]?.officialSaleUsd,
    })
    assert.deepEqual(profitSheet.getCell('P5').value, {
      formula: `IF(M5="per_token",N5*L5/'参数'!$B$8,IF(M5="per_duration",N5*F5,N5))`,
      result: data.profits[0]?.channelCostUsd,
    })
    assert.deepEqual(profitSheet.getCell('Q5').value, {
      formula: `IFERROR(O5*D5,"")`,
      result: data.profits[0]?.userRevenueUsd,
    })
    const reader = new ZipReader(
      new Uint8ArrayReader(await fs.readFile(outputPath))
    )
    try {
      const workbookXml = await (
        await reader.getEntries()
      )
        .find((entry) => entry.filename === 'xl/workbook.xml')
        ?.getData(new TextWriter())
      assert.match(workbookXml ?? '', /fullCalcOnLoad="1"/)
      assert.match(workbookXml ?? '', /forceFullCalc="1"/)
    } finally {
      await reader.close()
    }
    await fs.access(reportPath)
  } finally {
    await fs.rm(directory, { recursive: true, force: true })
  }
})

test('writes and converts channel mapping duration independently from the SKU', async () => {
  const directory = await fs.mkdtemp(
    path.join(os.tmpdir(), 'channel-template-mapping-duration-')
  )
  const outputPath = path.join(directory, 'template.xlsx')
  const reportPath = path.join(directory, 'template.report.json')
  try {
    const channelSource = structuredClone(source)
    const model = channelSource.models[0]
    assert.ok(model)
    model.fields.时长范围 = '5-15'
    const data = buildTemplateData(channelSource, rules)
    assert.equal(data.skus[0]?.minDurationSeconds, 4)

    const result = await writeTemplateWorkbook({
      basePath,
      outputPath,
      reportPath,
      sourcePath: 'source.xlsx',
      rulesPath: 'rules.json',
      rules,
      data,
    })
    assert.equal(result.hasFailures, false)

    const written = new ExcelJS.Workbook()
    await written.xlsx.readFile(outputPath)
    const mappingSheet = written.getWorksheet('模型映射')
    assert.ok(mappingSheet)
    assert.equal(mappingSheet.getRow(4).getCell(8).value, '最小时长秒')
    assert.equal(mappingSheet.getRow(4).getCell(9).value, '最大时长秒')
    assert.equal(mappingSheet.getRow(5).getCell(8).value, 5)
    assert.equal(mappingSheet.getRow(5).getCell(9).value, 15)

    const bytes = await fs.readFile(outputPath)
    const converted = await convertWorkbook(
      new File([bytes], 'channel-model-template.xlsx', {
        type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
      })
    )
    const blueprint = converted.document.entities.route_blueprints.find(
      (item) => item.business_id === 'route-blueprint/MAP-CLMM-R3-720'
    )
    assert.ok(blueprint)
    const target = (blueprint.targets as Array<Record<string, unknown>>)[0]
    assert.equal(target?.duration_min, 5)
    assert.equal(target?.duration_max, 15)
  } finally {
    await fs.rm(directory, { recursive: true, force: true })
  }
})

test('writes discrete channel durations as route duration values', async () => {
  const directory = await fs.mkdtemp(
    path.join(os.tmpdir(), 'channel-template-discrete-duration-')
  )
  const outputPath = path.join(directory, 'template.xlsx')
  const reportPath = path.join(directory, 'template.report.json')
  try {
    const channelSource = structuredClone(source)
    const model = channelSource.models[0]
    assert.ok(model)
    model.fields.时长范围 = '5,10,15'
    const data = buildTemplateData(channelSource, rules)

    const result = await writeTemplateWorkbook({
      basePath,
      outputPath,
      reportPath,
      sourcePath: 'source.xlsx',
      rulesPath: 'rules.json',
      rules,
      data,
    })
    assert.equal(result.hasFailures, false)

    const written = new ExcelJS.Workbook()
    await written.xlsx.readFile(outputPath)
    const mappingSheet = written.getWorksheet('模型映射')
    assert.ok(mappingSheet)
    assert.equal(mappingSheet.getRow(4).getCell(10).value, '可用时长秒')
    assert.equal(mappingSheet.getRow(5).getCell(8).value, null)
    assert.equal(mappingSheet.getRow(5).getCell(9).value, null)
    assert.equal(mappingSheet.getRow(5).getCell(10).value, '5,10,15')

    const bytes = await fs.readFile(outputPath)
    const converted = await convertWorkbook(
      new File([bytes], 'channel-model-template.xlsx', {
        type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
      })
    )
    const blueprint = converted.document.entities.route_blueprints.find(
      (item) => item.business_id === 'route-blueprint/MAP-CLMM-R3-720'
    )
    assert.ok(blueprint)
    const target = (blueprint.targets as Array<Record<string, unknown>>)[0]
    assert.deepEqual(target?.duration_values, [5, 10, 15])
    assert.equal(target?.duration_min, undefined)
    assert.equal(target?.duration_max, undefined)
  } finally {
    await fs.rm(directory, { recursive: true, force: true })
  }
})

test('writes a report for a relative output path in a new directory', async () => {
  const directory = await fs.mkdtemp(
    path.join(os.tmpdir(), 'channel-template-relative-write-')
  )
  const outputDirectory = path.join(directory, 'generated')
  const outputPath = path.relative(
    process.cwd(),
    path.join(outputDirectory, 'template.xlsx')
  )
  const reportPath = path.relative(
    process.cwd(),
    path.join(outputDirectory, 'template.report.json')
  )
  try {
    const result = await writeTemplateWorkbook({
      basePath,
      outputPath,
      reportPath,
      sourcePath: 'source.xlsx',
      rulesPath: 'rules.json',
      rules,
      data: buildTemplateData(source, rules),
    })

    assert.equal(result.hasFailures, false)
    await fs.access(outputPath)
    await fs.access(reportPath)
  } finally {
    await fs.rm(directory, { recursive: true, force: true })
  }
})

test('unhides every populated cost and mapping row inherited from the base workbook', async () => {
  const directory = await fs.mkdtemp(
    path.join(os.tmpdir(), 'channel-template-visible-rows-')
  )
  const outputPath = path.join(directory, 'template.xlsx')
  const reportPath = path.join(directory, 'template.report.json')
  try {
    const data = buildTemplateData(source, rules)
    const cost = data.costs[0]
    const mapping = data.mappings[0]
    assert.ok(cost)
    assert.ok(mapping)
    data.costs = Array.from({ length: 220 }, (_, index) => ({
      ...cost,
      businessId: `COST-VISIBLE-${index + 1}`,
    }))
    data.mappings = Array.from({ length: 120 }, (_, index) => ({
      ...mapping,
      businessId: `MAP-VISIBLE-${index + 1}`,
    }))

    const result = await writeTemplateWorkbook({
      basePath,
      outputPath,
      reportPath,
      sourcePath: 'source.xlsx',
      rulesPath: 'rules.json',
      rules,
      data,
    })

    assert.equal(result.hasFailures, false)
    const written = new ExcelJS.Workbook()
    await written.xlsx.readFile(outputPath)
    const costSheet = written.getWorksheet('渠道成本')
    const mappingSheet = written.getWorksheet('模型映射')
    assert.ok(costSheet)
    assert.ok(mappingSheet)
    for (let row = 5; row < data.costs.length + 5; row += 1) {
      assert.equal(costSheet.getRow(row).hidden, false, `渠道成本 row ${row}`)
    }
    for (let row = 5; row < data.mappings.length + 5; row += 1) {
      assert.equal(
        mappingSheet.getRow(row).hidden,
        false,
        `模型映射 row ${row}`
      )
    }
  } finally {
    await fs.rm(directory, { recursive: true, force: true })
  }
})
