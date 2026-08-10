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

import ExcelJS from 'exceljs'

import { readSourceWorkbook } from '../source'

const fixturePath = fileURLToPath(
  new URL('../__fixtures__/sd-source-v1.xlsx', import.meta.url)
)
const latestHeaders = [
  '渠道', '充值汇率', '手续费', '计费倍率', '付费模式', '模型ID', '版本',
  '清晰度', '计费方式', '单价 元', '参考图数', '参考视频数', '参考音频数',
  '最大素材数', '视频音频合计上限', '参考视频总时长上限 秒',
  '最小参考图数', '超分', '时长范围', '比例', '视频输入', '过真人脸',
  '素材库', 'NSFW', '协议', '状态', '并发数', '折扣 秒 无V',
  '折扣 秒 含V', '折扣 M 无V', '折扣 M 含V', '接入', '已测',
  '售价', '利润', '上游模型分组', '备注',
]

function headerColumn(sheet: ExcelJS.Worksheet, header: string): number {
  for (let column = 1; column <= sheet.columnCount; column += 1) {
    if (sheet.getRow(2).getCell(column).value === header) return column
  }
  throw new Error(`missing test header: ${header}`)
}

async function withLatestSource(
  run: (sourcePath: string) => Promise<void>,
  mutate?: (sheet: ExcelJS.Worksheet) => void
): Promise<void> {
  const workbook = new ExcelJS.Workbook()
  await workbook.xlsx.readFile(fixturePath)
  const sheet = workbook.getWorksheet('sd')
  assert.ok(sheet)
  for (let rowNumber = 3; rowNumber <= sheet.rowCount; rowNumber += 1) {
    const row = sheet.getRow(rowNumber)
    const value = (column: number) => row.getCell(column).value
    const billingMode = String(value(8) ?? '')
    const priceColumn = billingMode === 'second' ? 9 : billingMode === 'call' ? 10 : 11
    const limit = /^\d{3}$/u.test(String(value(12) ?? ''))
      ? String(value(12))
      : '000'
    const [images, videos, audios] = [...limit].map(Number)
    row.values = [
      undefined,
      ...Array.from({ length: 7 }, (_, index) => value(index + 1)),
      value(13), billingMode, value(priceColumn), images, videos, audios,
      images + videos + audios, null, videos > 0 ? 15 : null, 0,
      ...Array.from({ length: 18 }, (_, index) => value(index + 14)),
      null, value(32),
    ]
  }
  sheet.getRow(2).values = [undefined, ...latestHeaders]
  mutate?.(sheet)
  const tempDirectory = await fs.mkdtemp(path.join(os.tmpdir(), 'channel-source-test-'))
  const sourcePath = path.join(tempDirectory, 'source.xlsx')
  try {
    await workbook.xlsx.writeFile(sourcePath)
    await run(sourcePath)
  } finally {
    await fs.rm(tempDirectory, { recursive: true, force: true })
  }
}

test('reads latest source records with original worksheet and row locations', async () => {
  await withLatestSource(async (sourcePath) => {
    const source = await readSourceWorkbook(sourcePath)
    assert.equal(source.channels.length, 9)
    assert.equal(source.models[0]?.location.sheet, 'sd')
    assert.equal(source.models[0]?.location.row, 3)
    assert.equal(source.officialPrices[0]?.location.sheet, 'sd官价')
  })
})

test('reads the latest billing mode and unified unit price', async () => {
  await withLatestSource(async (sourcePath) => {
    const source = await readSourceWorkbook(sourcePath)
    const model = source.models[0]
    assert.equal(model?.fields.清晰度, '720')
    assert.equal(model?.fields.计费, 'second')
    assert.equal(model?.fields['单价 元'], 1.38)
    assert.equal(model?.fields['元/秒'], undefined)
    assert.equal(model?.fields['元/次'], undefined)
    assert.equal(model?.fields['元/1M'], undefined)
  })
})

test('rejects legacy, mixed, and missing sd pricing headers', async () => {
  for (const mutate of [
    (sheet: ExcelJS.Worksheet) => {
      sheet.getRow(2).getCell(headerColumn(sheet, '单价 元')).value = '元/秒'
    },
    (sheet: ExcelJS.Worksheet) => {
      sheet.getRow(2).getCell(headerColumn(sheet, '参考图数')).value = '元/次'
    },
    (sheet: ExcelJS.Worksheet) => {
      sheet.getRow(2).getCell(headerColumn(sheet, '单价 元')).value = null
    },
  ]) {
    await withLatestSource(async (sourcePath) => {
      await assert.rejects(() => readSourceWorkbook(sourcePath), /sd header mismatch/)
    }, mutate)
  }
})

test('rejects a workbook whose sd header changes', async () => {
  await withLatestSource(async (sourcePath) => {
    await assert.rejects(() => readSourceWorkbook(sourcePath), /sd header mismatch/)
  }, (sheet) => {
    sheet.getRow(2).getCell(headerColumn(sheet, '渠道')).value = '渠道错误'
  })
})

test('reads sd records when an optional column is added before remarks', async () => {
  await withLatestSource(async (sourcePath) => {
    const source = await readSourceWorkbook(sourcePath)
    assert.equal(source.models[0]?.fields.备注, 'remark-a')
  }, (sheet) => {
    const remarkColumn = headerColumn(sheet, '备注')
    sheet.getRow(2).getCell(remarkColumn).value = '新增可选列'
    sheet.getRow(2).getCell(remarkColumn + 1).value = '备注'
    sheet.getRow(3).getCell(remarkColumn + 1).value = 'remark-a'
  })
})

test('reads structured reference formulas from the latest sd sheet', async () => {
  await withLatestSource(async (sourcePath) => {
    const source = await readSourceWorkbook(sourcePath)
    assert.equal(source.models[0]?.fields.参考图数, 9)
    assert.equal(source.models[0]?.fields.最大素材数, 12)
    assert.equal(source.models[0]?.fields.最小参考图数, 1)
  }, (sheet) => {
    const audioColumn = headerColumn(sheet, '参考音频数')
    const totalColumn = headerColumn(sheet, '最大素材数')
    sheet.getRow(3).getCell(audioColumn).value = 0
    sheet.getRow(3).getCell(totalColumn).value = {
      formula: '9+3+0',
      result: 12,
    }
    sheet.getRow(3).getCell(headerColumn(sheet, '最小参考图数')).value = 1
  })
})
