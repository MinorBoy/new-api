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

test('reads source records with original worksheet and row locations', async () => {
  const source = await readSourceWorkbook(fixturePath)

  assert.equal(source.channels.length, 9)
  assert.equal(source.models[0]?.location.sheet, 'sd')
  assert.equal(source.models[0]?.location.row, 3)
  assert.equal(source.officialPrices[0]?.location.sheet, 'sd官价')
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
