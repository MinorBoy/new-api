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
  officialPrices: [
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
  ],
}

test('writes a V1 workbook recognized by the existing converter', async () => {
  const directory = await fs.mkdtemp(
    path.join(os.tmpdir(), 'channel-template-write-')
  )
  const outputPath = path.join(directory, 'template.xlsx')
  const reportPath = path.join(directory, 'template.report.json')
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
