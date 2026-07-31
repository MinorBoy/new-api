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
import fs from 'node:fs/promises'

import ExcelJS from 'exceljs'

export const CHANNEL_HEADERS = ['渠道', '名称', '链接', 'Base Url'] as const
export const SD_HEADERS = [
  '渠道',
  '充值汇率',
  '手续费',
  '计费倍率',
  '付费模式',
  '模型ID',
  '版本',
  '计费',
  '元/秒',
  '元/次',
  '元/1M',
  '素材限制',
  '清晰度',
  '超分',
  '时长范围',
  '比例',
  '视频输入',
  '过真人脸',
  '素材库',
  'NSFW',
  '协议',
  '状态',
  '并发数',
  '折扣 秒 无V',
  '折扣 秒 含V',
  '折扣 M 无V',
  '折扣 M 含V',
  '接入',
  '已测',
  '售价',
  '利润',
  '备注',
] as const
export const OFFICIAL_PRICE_HEADERS = [
  '模型',
  '版本',
  '分辨率',
  '不含视频 元/M',
  '包含视频 元/M',
  '帧率',
  '长边',
  '短边',
  '不含视频 元/秒',
  '包含视频 元/秒',
  '备注',
] as const

export type SourceValue = boolean | Date | number | string | null

export type SourceLocation = {
  sheet: string
  row: number
}

export type SourceRecord = {
  fields: Record<string, SourceValue>
  location: SourceLocation
}

export type SourceWorkbook = {
  channels: SourceRecord[]
  models: SourceRecord[]
  officialPrices: SourceRecord[]
}

const MAX_FILE_BYTES = 10 * 1024 * 1024
const MAX_SHEETS = 20
const MAX_ROWS_PER_SHEET = 20_000
const REQUIRED_SHEETS = ['channel', 'sd', 'sd官价'] as const

function cellValue(value: unknown): SourceValue {
  if (
    value === null ||
    typeof value === 'boolean' ||
    typeof value === 'number' ||
    typeof value === 'string' ||
    value instanceof Date
  ) {
    return value
  }
  if (typeof value === 'object' && value !== null && 'text' in value) {
    const text = value.text
    return typeof text === 'string' ? text : null
  }
  return null
}

function cellText(value: SourceValue): string {
  if (value === null) return ''
  if (value instanceof Date) return value.toISOString()
  return String(value).trim()
}

function rowIsBlank(row: ExcelJS.Row, columns: readonly number[]): boolean {
  for (const column of columns) {
    if (cellText(cellValue(row.getCell(column).value)) !== '') return false
  }
  return true
}

function readHeaders(
  sheet: ExcelJS.Worksheet,
  rowNumber: number,
  headers: readonly string[],
  name: string
): number[] {
  const headerRow = sheet.getRow(rowNumber)
  const columns = headers.map((header) => {
    let matchedColumn = 0
    for (let column = 1; column <= sheet.columnCount; column += 1) {
      if (cellText(cellValue(headerRow.getCell(column).value)) !== header) {
        continue
      }
      if (matchedColumn !== 0) {
        throw new Error(`${name} header mismatch`)
      }
      matchedColumn = column
    }
    return matchedColumn
  })
  if (columns.some((column) => column === 0)) {
    throw new Error(`${name} header mismatch`)
  }
  return columns
}

function readRecords(
  sheet: ExcelJS.Worksheet,
  headers: readonly string[],
  columns: readonly number[],
  headerRow: number
): SourceRecord[] {
  const records: SourceRecord[] = []
  for (
    let rowNumber = headerRow + 1;
    rowNumber <= sheet.rowCount;
    rowNumber += 1
  ) {
    const row = sheet.getRow(rowNumber)
    if (rowIsBlank(row, columns)) continue
    const fields = Object.fromEntries(
      headers.map((header, index) => [
        header,
        cellValue(row.getCell(columns[index]!).value),
      ])
    )
    records.push({
      fields,
      location: { sheet: sheet.name, row: rowNumber },
    })
  }
  return records
}

function validateWorkbookShape(workbook: ExcelJS.Workbook): void {
  if (workbook.worksheets.length > MAX_SHEETS) {
    throw new Error(`workbook has more than ${MAX_SHEETS} worksheets`)
  }
  const sheetNames = workbook.worksheets.map((sheet) => sheet.name)
  const missing = REQUIRED_SHEETS.filter((name) => !sheetNames.includes(name))
  const unexpected = sheetNames.filter(
    (name) => !(REQUIRED_SHEETS as readonly string[]).includes(name)
  )
  if (missing.length > 0 || unexpected.length > 0) {
    throw new Error(
      `source worksheets mismatch; missing=${missing.join(',')}; unexpected=${unexpected.join(',')}`
    )
  }
  for (const sheet of workbook.worksheets) {
    if (sheet.rowCount > MAX_ROWS_PER_SHEET) {
      throw new Error(`${sheet.name} has more than ${MAX_ROWS_PER_SHEET} rows`)
    }
  }
}

export async function readSourceWorkbook(
  filePath: string
): Promise<SourceWorkbook> {
  const file = await fs.readFile(filePath)
  if (file.length > MAX_FILE_BYTES) {
    throw new Error(`source workbook exceeds ${MAX_FILE_BYTES} bytes`)
  }
  if (file.length < 4 || file[0] !== 0x50 || file[1] !== 0x4b) {
    throw new Error('source workbook is not an xlsx archive')
  }

  const workbook = new ExcelJS.Workbook()
  await workbook.xlsx.load(file)
  validateWorkbookShape(workbook)

  const channel = workbook.getWorksheet('channel')
  const models = workbook.getWorksheet('sd')
  const officialPrices = workbook.getWorksheet('sd官价')
  if (!channel || !models || !officialPrices) {
    throw new Error('source worksheets are unavailable')
  }
  const channelColumns = readHeaders(channel, 2, CHANNEL_HEADERS, 'channel')
  const modelColumns = readHeaders(models, 2, SD_HEADERS, 'sd')
  const officialPriceColumns = readHeaders(
    officialPrices,
    6,
    OFFICIAL_PRICE_HEADERS,
    'sd官价'
  )
  return {
    channels: readRecords(channel, CHANNEL_HEADERS, channelColumns, 2),
    models: readRecords(models, SD_HEADERS, modelColumns, 2),
    officialPrices: readRecords(
      officialPrices,
      OFFICIAL_PRICE_HEADERS,
      officialPriceColumns,
      6
    ),
  }
}

export { cellText }
