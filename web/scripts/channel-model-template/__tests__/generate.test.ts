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

import { convertWorkbook } from '../../../src/channel-config-converter/conversion'
import { runGenerator } from '../generate'

const sourcePath = fileURLToPath(
  new URL('../__fixtures__/sd-source-v1.xlsx', import.meta.url)
)
const rulesPath = fileURLToPath(
  new URL('../conversion-rules.json', import.meta.url)
)
const basePath = fileURLToPath(
  new URL(
    '../../../src/channel-config-converter/__fixtures__/channel-config-v1-corrected.xlsx',
    import.meta.url
  )
)

test('refuses to overwrite an existing workbook without --force', async () => {
  const directory = await fs.mkdtemp(
    path.join(os.tmpdir(), 'channel-template-generator-')
  )
  const outputPath = path.join(directory, 'template.xlsx')
  try {
    await fs.writeFile(outputPath, 'existing workbook')
    await assert.rejects(
      () =>
        runGenerator([
          '--source',
          sourcePath,
          '--rules',
          rulesPath,
          '--base',
          basePath,
          '--output',
          outputPath,
          '--allow-warnings',
        ]),
      /Output already exists/
    )
  } finally {
    await fs.rm(directory, { recursive: true, force: true })
  }
})

test('writes a workbook and report when warnings are explicitly allowed', async () => {
  const directory = await fs.mkdtemp(
    path.join(os.tmpdir(), 'channel-template-generator-')
  )
  const outputPath = path.join(directory, 'template.xlsx')
  const reportPath = path.join(directory, 'template.report.json')
  const warningSourcePath = path.join(directory, 'source-with-warning.xlsx')
  try {
    const workbook = new ExcelJS.Workbook()
    await workbook.xlsx.readFile(sourcePath)
    const sourceSheet = workbook.getWorksheet('sd')
    assert.ok(sourceSheet)
    for (let row = 4; row <= sourceSheet.rowCount; row += 1) {
      sourceSheet.getRow(row).getCell(1).value = null
      sourceSheet.getRow(row).getCell(6).value = null
    }
    const priceColumn = sourceSheet
      .getRow(2)
      .values.findIndex((value) => String(value).trim() === '单价 元')
    assert.ok(priceColumn > 0)
    sourceSheet.getRow(3).getCell(priceColumn).value = 0
    await workbook.xlsx.writeFile(warningSourcePath)

    const result = await runGenerator([
      '--source',
      warningSourcePath,
      '--rules',
      rulesPath,
      '--base',
      basePath,
      '--output',
      outputPath,
      '--report',
      reportPath,
      '--allow-warnings',
    ])

    assert.equal(result.hasFailures, false)
    assert.ok(
      result.report.issues.some(
        (item) => item.code === 'COST_PRICE_INVALID' && item.severity === 'WARN'
      )
    )
    const bytes = await fs.readFile(outputPath)
    const converted = await convertWorkbook(
      new File([bytes], 'template.xlsx', {
        type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
      })
    )

    assert.equal(converted.document.template_version, '1')
    await fs.access(reportPath)
  } finally {
    await fs.rm(directory, { recursive: true, force: true })
  }
})

test('supports an explicit SD-only generation scope when H3 sheets are present', async () => {
  const directory = await fs.mkdtemp(
    path.join(os.tmpdir(), 'channel-template-generator-sd-only-')
  )
  const outputPath = path.join(directory, 'template.xlsx')
  const reportPath = path.join(directory, 'template.report.json')
  const sourceWithH3Path = path.join(directory, 'source-with-h3.xlsx')
  try {
    const workbook = new ExcelJS.Workbook()
    await workbook.xlsx.readFile(sourcePath)
    const sourceSheet = workbook.getWorksheet('sd')
    assert.ok(sourceSheet)
    for (let row = 4; row <= sourceSheet.rowCount; row += 1) {
      sourceSheet.getRow(row).getCell(1).value = null
      sourceSheet.getRow(row).getCell(6).value = null
    }
    workbook.addWorksheet('h3').addRow(['渠道', '模型ID', '系列'])
    workbook.addWorksheet('h3官价').addRow(['系列', '模型', '分辨率'])
    await workbook.xlsx.writeFile(sourceWithH3Path)

    const result = await runGenerator([
      '--source',
      sourceWithH3Path,
      '--rules',
      rulesPath,
      '--base',
      basePath,
      '--output',
      outputPath,
      '--report',
      reportPath,
      '--allow-warnings',
      '--allow-unsupported-sheets',
    ])

    assert.equal(result.hasFailures, false)
    assert.ok(
      result.report.issues.some(
        (item) =>
          item.code === 'UNSUPPORTED_SOURCE_SHEET' && item.severity === 'WARN'
      )
    )
    await fs.access(outputPath)
  } finally {
    await fs.rm(directory, { recursive: true, force: true })
  }
})
