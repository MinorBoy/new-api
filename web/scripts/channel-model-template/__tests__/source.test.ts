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

async function withMutatedFixture(
  mutate: (workbook: ExcelJS.Workbook) => void,
  assertWorkbook: (filePath: string) => Promise<void>
): Promise<void> {
  const workbook = new ExcelJS.Workbook()
  await workbook.xlsx.readFile(fixturePath)
  mutate(workbook)
  const tempDirectory = await fs.mkdtemp(
    path.join(os.tmpdir(), 'channel-source-test-')
  )
  const sourcePath = path.join(tempDirectory, 'source.xlsx')
  try {
    await workbook.xlsx.writeFile(sourcePath)
    await assertWorkbook(sourcePath)
  } finally {
    await fs.rm(tempDirectory, { recursive: true, force: true })
  }
}

test('reads source records with original worksheet and row locations', async () => {
  const source = await readSourceWorkbook(fixturePath)

  assert.equal(source.channels.length, 9)
  assert.equal(source.models[0]?.location.sheet, 'sd')
  assert.equal(source.models[0]?.location.row, 3)
  assert.equal(source.officialPrices[0]?.location.sheet, 'sd官价')
})

test('accepts migrated channel economics and records optional price sheets', async () => {
  await withMutatedFixture(
    (workbook) => {
      const channel = workbook.getWorksheet('channel')
      assert.ok(channel)
      channel.getCell('E2').value = '充值汇率'
      channel.getCell('F2').value = '手续费'
      channel.getCell('G2').value = '计费倍率'
      channel.getCell('E3').value = '1:1'
      channel.getCell('F3').value = '3%'
      channel.getCell('G3').value = 1.25

      const sd = workbook.getWorksheet('sd')
      assert.ok(sd)
      for (const column of ['B', 'C', 'D']) {
        sd.getCell(`${column}2`).value = null
      }

      const h3 = workbook.addWorksheet('h3')
      h3.addRow([
        '渠道',
        '模型ID',
        '系列',
        '版本',
        '清晰度',
        '计费方式',
        '单价 元',
      ])
      h3.addRow([2, 'lec-minimax-h3', 'h3', '标准', '768p', 'second', 0.15])
      const h3Official = workbook.addWorksheet('h3官价')
      h3Official.addRow([
        '系列',
        '模型',
        '版本',
        '分辨率',
        '输入素材',
        '素材价格 元/秒',
        '输入素材',
        '素材价格 元/秒',
        '输出价格 元/秒',
      ])
      h3Official.addRow(['h3', 'MiniMax-H3', '标准', '768P', '视频', 0.5, '图片', 0.2, 0.5])
    },
    async (sourcePath) => {
      const source = await readSourceWorkbook(sourcePath)
      assert.equal(source.channels[0]?.fields.计费倍率, 1.25)
      assert.equal(source.channels[0]?.fields.手续费, '3%')
      assert.deepEqual(source.additionalSheets, ['h3', 'h3官价'])
    }
  )
})

test('accepts the latest channel API URL header as the canonical link field', async () => {
  await withMutatedFixture(
    (workbook) => {
      const channel = workbook.getWorksheet('channel')
      assert.ok(channel)
      channel.getCell('C2').value = '模型数据 API URL'
    },
    async (sourcePath) => {
      const source = await readSourceWorkbook(sourcePath)
      assert.equal(
        source.channels[0]?.fields.链接,
        'https://clmm-mall.top/pricing'
      )
    }
  )
})

test('rejects duplicate channel economics left on sd after migration', async () => {
  await withMutatedFixture(
    (workbook) => {
      const channel = workbook.getWorksheet('channel')
      assert.ok(channel)
      channel.getCell('E2').value = '充值汇率'
      channel.getCell('F2').value = '手续费'
      channel.getCell('G2').value = '计费倍率'
    },
    async (sourcePath) => {
      await assert.rejects(
        () => readSourceWorkbook(sourcePath),
        /sd header mismatch; migrated=.*充值汇率.*手续费.*计费倍率/
      )
    }
  )
})

test('reads the latest series and unit-price contract and skips series-only group rows', async () => {
  const source = await readSourceWorkbook(fixturePath)
  const model = source.models[0]
  const official = source.officialPrices[0]

  assert.equal(model?.fields.系列, 2)
  assert.equal(model?.fields.清晰度, '720')
  assert.equal(model?.fields.计费, 'second')
  assert.equal(model?.fields['单价 元'], 1.38)
  assert.equal(model?.fields.视频输入, undefined)
  assert.equal(
    source.models.some((record) => record.fields.系列 === 2.5),
    false
  )
  assert.equal(official?.fields.系列, 2)
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
  await withMutatedFixture(
    (workbook) => {
      const sheet = workbook.getWorksheet('sd')
      assert.ok(sheet)
      sheet.getCell('AM2').value = '可选扩展'
      sheet.getCell('AN2').value = '备注'
      sheet.getCell('AM3').value = 'extra-a'
      sheet.getCell('AN3').value = 'remark-a'
    },
    async (sourcePath) => {
      const source = await readSourceWorkbook(sourcePath)
      assert.equal(source.models[0]?.fields.备注, 'remark-a')
    }
  )
})

test('reads structured reference contract columns from the updated sd sheet', async () => {
  await withMutatedFixture(
    (workbook) => {
      const sheet = workbook.getWorksheet('sd')
      assert.ok(sheet)
      sheet.getCell('L3').value = 9
      sheet.getCell('M3').value = 3
      sheet.getCell('N3').value = 3
      sheet.getCell('O3').numFmt = 'General'
      sheet.getCell('O3').value = { formula: 'SUM(L3:N3)', result: 15 }
      sheet.getCell('P3').value = 3
      sheet.getCell('R3').value = 15
      sheet.getCell('S3').value = 1
    },
    async (sourcePath) => {
      const source = await readSourceWorkbook(sourcePath)
      assert.equal(source.models[0]?.fields.参考图数, 9)
      assert.equal(source.models[0]?.fields.最大素材数, 15)
      assert.equal(source.models[0]?.fields.素材模式, undefined)
      assert.equal(source.models[0]?.fields.最小参考图数, 1)
    }
  )
})

test('rejects forbidden legacy sd headers', async () => {
  for (const forbiddenHeader of ['元/秒', '元/次', '元/1M', '视频输入']) {
    await withMutatedFixture(
      (workbook) => {
        const sheet = workbook.getWorksheet('sd')
        assert.ok(sheet)
        sheet.getCell('Q2').value = forbiddenHeader
      },
      async (sourcePath) => {
        await assert.rejects(
          () => readSourceWorkbook(sourcePath),
          new RegExp(`sd header mismatch; forbidden=.*${forbiddenHeader}`)
        )
      }
    )
  }
})

test('rejects missing latest sd headers', async () => {
  for (const [cell, header] of [
    ['G2', '系列'],
    ['K2', '单价 元'],
  ] as const) {
    await withMutatedFixture(
      (workbook) => {
        const sheet = workbook.getWorksheet('sd')
        assert.ok(sheet)
        sheet.getCell(cell).value = null
      },
      async (sourcePath) => {
        await assert.rejects(
          () => readSourceWorkbook(sourcePath),
          new RegExp(`sd header mismatch; missing=.*${header}`)
        )
      }
    )
  }
})

test('rejects invalid model series with the exact worksheet row', async () => {
  for (const value of [null, 0, -1, 'invalid']) {
    await withMutatedFixture(
      (workbook) => {
        const sheet = workbook.getWorksheet('sd')
        assert.ok(sheet)
        sheet.getCell('G3').value = value
      },
      async (sourcePath) => {
        await assert.rejects(
          () => readSourceWorkbook(sourcePath),
          /sd series invalid at row 3/
        )
      }
    )
  }
})

test('rejects invalid official-price series with the exact worksheet row', async () => {
  for (const value of [null, 0, -1, 'invalid']) {
    await withMutatedFixture(
      (workbook) => {
        const sheet = workbook.getWorksheet('sd官价')
        assert.ok(sheet)
        sheet.getCell('A7').value = value
      },
      async (sourcePath) => {
        await assert.rejects(
          () => readSourceWorkbook(sourcePath),
          /sd官价 series invalid at row 7/
        )
      }
    )
  }
})
