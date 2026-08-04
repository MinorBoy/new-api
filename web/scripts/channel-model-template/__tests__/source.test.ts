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
const repositorySourcePath = fileURLToPath(
  new URL('../../../../docs/new-channels/sd收录.xlsx', import.meta.url)
)

test('reads source records with original worksheet and row locations', async () => {
  const source = await readSourceWorkbook(fixturePath)

  assert.equal(source.channels.length, 9)
  assert.equal(source.models[0]?.location.sheet, 'sd')
  assert.equal(source.models[0]?.location.row, 3)
  assert.equal(source.officialPrices[0]?.location.sheet, 'sd官价')
})

test('reads the renamed billing and model resolution columns from the latest source sheet', async () => {
  const source = await readSourceWorkbook(repositorySourcePath)
  const channel = source.channels.find(
    (record) => record.fields.渠道 === 11
  )
  const model = source.models.find((record) => record.fields.渠道 === 11)

  assert.equal(channel?.fields.名称, 'z5')
  assert.equal(model?.fields.清晰度, '720p')
  assert.equal(model?.fields.计费, 'call')
})

test('rejects a workbook whose sd header changes', async () => {
  const workbook = new ExcelJS.Workbook()
  await workbook.xlsx.readFile(fixturePath)
  const sheet = workbook.getWorksheet('sd')
  assert.ok(sheet)
  sheet.getCell('A2').value = '渠道错误'

  const tempDirectory = await fs.mkdtemp(
    path.join(os.tmpdir(), 'channel-source-test-')
  )
  const invalidPath = path.join(tempDirectory, 'invalid.xlsx')
  try {
    await workbook.xlsx.writeFile(invalidPath)
    await assert.rejects(
      () => readSourceWorkbook(invalidPath),
      /sd header mismatch/
    )
  } finally {
    await fs.rm(tempDirectory, { recursive: true, force: true })
  }
})

test('reads sd records when an optional column is added before remarks', async () => {
  const workbook = new ExcelJS.Workbook()
  await workbook.xlsx.readFile(fixturePath)
  const sheet = workbook.getWorksheet('sd')
  assert.ok(sheet)
  sheet.getCell('AF2').value = '上游模型分组'
  sheet.getCell('AG2').value = '备注'
  sheet.getCell('AF3').value = 'group-a'
  sheet.getCell('AG3').value = 'remark-a'

  const tempDirectory = await fs.mkdtemp(
    path.join(os.tmpdir(), 'channel-source-test-')
  )
  const sourcePath = path.join(tempDirectory, 'source.xlsx')
  try {
    await workbook.xlsx.writeFile(sourcePath)
    const source = await readSourceWorkbook(sourcePath)

    assert.equal(source.models[0]?.fields.备注, 'remark-a')
  } finally {
    await fs.rm(tempDirectory, { recursive: true, force: true })
  }
})

test('reads structured reference contract columns from the updated sd sheet', async () => {
  const workbook = new ExcelJS.Workbook()
  await workbook.xlsx.readFile(fixturePath)
  const sheet = workbook.getWorksheet('sd')
  assert.ok(sheet)
  for (let column = 12; column <= 40; column += 1) {
    sheet.getRow(2).getCell(column).value = null
  }
  const headers = new Map([
    [12, '参考图数'],
    [13, '参考视频数'],
    [14, '参考音频数'],
    [15, '最大素材数'],
    [16, '视频音频合计上限'],
    [18, '参考视频总时长上限 秒'],
    [19, '最小参考图数'],
    [20, '清晰度'],
    [21, '超分'],
    [22, '时长范围'],
    [23, '比例'],
    [24, '视频输入'],
    [25, '过真人脸'],
    [26, '素材库'],
    [27, 'NSFW'],
    [28, '协议'],
    [29, '状态'],
    [30, '并发数'],
    [31, '折扣 秒 无V'],
    [32, '折扣 秒 含V'],
    [33, '折扣 M 无V'],
    [34, '折扣 M 含V'],
    [35, '接入'],
    [36, '已测'],
    [37, '售价'],
    [38, '利润'],
    [39, '上游模型分组'],
    [40, '备注'],
  ])
  headers.forEach((header, column) => {
    sheet.getRow(2).getCell(column).value = header
  })
  const values = new Map<number, string | number>([
    [12, 9],
    [13, 3],
    [14, 3],
    [15, 12],
    [16, 3],
    [18, 15],
    [19, 1],
  ])
  values.forEach((value, column) => {
    sheet.getRow(3).getCell(column).value = value
  })
  sheet.getRow(3).getCell(15).numFmt = 'General'
  sheet.getRow(3).getCell(15).value = { formula: 'SUM(L3:N3)', result: 12 }

  const tempDirectory = await fs.mkdtemp(
    path.join(os.tmpdir(), 'channel-source-test-')
  )
  const sourcePath = path.join(tempDirectory, 'source.xlsx')
  try {
    await workbook.xlsx.writeFile(sourcePath)
    const source = await readSourceWorkbook(sourcePath)

    assert.equal(source.models[0]?.fields.参考图数, 9)
    assert.equal(source.models[0]?.fields.最大素材数, 12)
    assert.equal(source.models[0]?.fields.素材模式, undefined)
    assert.equal(source.models[0]?.fields.最小参考图数, 1)
  } finally {
    await fs.rm(tempDirectory, { recursive: true, force: true })
  }
})
