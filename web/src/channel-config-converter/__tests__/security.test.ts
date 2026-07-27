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
import zlib from 'node:zlib'

import { preflightWorkbook } from '../security'

type ZipFixtureEntry = {
  name: string
  data: string | Uint8Array
  declaredUncompressedSize?: number
}

function makeZip(entries: ZipFixtureEntry[]): Uint8Array {
  const localParts: Buffer[] = []
  const centralParts: Buffer[] = []
  let localOffset = 0

  for (const entry of entries) {
    const name = Buffer.from(entry.name, 'utf8')
    const data = Buffer.from(entry.data)
    const compressed = zlib.deflateRawSync(data)
    const local = Buffer.alloc(30 + name.length)
    local.writeUInt32LE(0x04034b50, 0)
    local.writeUInt16LE(20, 4)
    local.writeUInt16LE(0, 6)
    local.writeUInt16LE(8, 8)
    local.writeUInt32LE(0, 14)
    local.writeUInt32LE(compressed.length, 18)
    local.writeUInt32LE(data.length, 22)
    local.writeUInt16LE(name.length, 26)
    local.writeUInt16LE(0, 28)
    name.copy(local, 30)
    localParts.push(local, compressed)

    const central = Buffer.alloc(46 + name.length)
    central.writeUInt32LE(0x02014b50, 0)
    central.writeUInt16LE(20, 4)
    central.writeUInt16LE(20, 6)
    central.writeUInt16LE(0, 8)
    central.writeUInt16LE(8, 10)
    central.writeUInt32LE(0, 16)
    central.writeUInt32LE(compressed.length, 20)
    central.writeUInt32LE(entry.declaredUncompressedSize ?? data.length, 24)
    central.writeUInt16LE(name.length, 28)
    central.writeUInt16LE(0, 30)
    central.writeUInt16LE(0, 32)
    central.writeUInt32LE(0, 38)
    central.writeUInt32LE(localOffset, 42)
    name.copy(central, 46)
    centralParts.push(central)

    localOffset += local.length + compressed.length
  }

  const localBytes = Buffer.concat(localParts)
  const centralBytes = Buffer.concat(centralParts)
  const end = Buffer.alloc(22)
  end.writeUInt32LE(0x06054b50, 0)
  end.writeUInt16LE(entries.length, 8)
  end.writeUInt16LE(entries.length, 10)
  end.writeUInt32LE(centralBytes.length, 12)
  end.writeUInt32LE(localBytes.length, 16)
  return Buffer.concat([localBytes, centralBytes, end])
}

function makeFile(name: string, bytes: Uint8Array): Blob & { name: string } {
  const copy = new Uint8Array(bytes.byteLength)
  copy.set(bytes)
  const file = new Blob([copy.buffer], {
    type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
  }) as Blob & { name: string }
  Object.defineProperty(file, 'name', { value: name })
  return file
}

async function assertPreflightError(
  file: Blob & { name: string },
  code: string
): Promise<void> {
  await assert.rejects(
    () => preflightWorkbook(file),
    (error: unknown) => {
      assert.equal((error as { code?: string }).code, code)
      return true
    }
  )
}

function safeWorkbook(
  sheetXml = '<worksheet><sheetData><row r="1" /></sheetData></worksheet>'
) {
  return makeZip([
    { name: '[Content_Types].xml', data: '<Types />' },
    { name: 'xl/workbook.xml', data: '<workbook />' },
    { name: 'xl/worksheets/sheet1.xml', data: sheetXml },
  ])
}

test('rejects legacy workbook extensions before archive inspection', async () => {
  for (const name of ['channels.xls', 'channels.xlsm']) {
    await assertPreflightError(
      makeFile(name, safeWorkbook()),
      'UNSUPPORTED_EXTENSION'
    )
  }
})

test('rejects OLE containers even when the extension is xlsx', async () => {
  await assertPreflightError(
    makeFile(
      'channels.xlsx',
      Uint8Array.from([0xd0, 0xcf, 0x11, 0xe0, 0xa1, 0xb1, 0x1a, 0xe1])
    ),
    'OLE_CONTAINER'
  )
})

test('rejects macro and external connection entries', async () => {
  await assertPreflightError(
    makeFile(
      'channels.xlsx',
      makeZip([{ name: 'xl/vbaProject.bin', data: 'macro' }])
    ),
    'MACRO_CONTENT'
  )
  await assertPreflightError(
    makeFile(
      'channels.xlsx',
      makeZip([{ name: 'xl/externalLinks/externalLink1.xml', data: 'link' }])
    ),
    'EXTERNAL_LINK_OR_CONNECTION'
  )
})

test('rejects files larger than the 10 MiB input limit', async () => {
  await assertPreflightError(
    makeFile('channels.xlsx', new Uint8Array(10 * 1024 * 1024 + 1)),
    'FILE_SIZE_LIMIT'
  )
})

test('rejects archives whose declared uncompressed content exceeds 100 MiB', async () => {
  await assertPreflightError(
    makeFile(
      'channels.xlsx',
      makeZip([
        {
          name: 'xl/worksheets/sheet1.xml',
          data: '<worksheet />',
          declaredUncompressedSize: 100 * 1024 * 1024 + 1,
        },
      ])
    ),
    'ZIP_UNCOMPRESSED_SIZE_LIMIT'
  )
})

test('rejects archives with more than 20 worksheet entries', async () => {
  const entries = Array.from({ length: 21 }, (_, index) => ({
    name: `xl/worksheets/sheet${index + 1}.xml`,
    data: '<worksheet />',
  }))
  await assertPreflightError(
    makeFile('channels.xlsx', makeZip(entries)),
    'SHEET_COUNT_LIMIT'
  )
})

test('rejects one worksheet with more than 20,000 rows', async () => {
  const rows = '<row />'.repeat(20_001)
  await assertPreflightError(
    makeFile(
      'channels.xlsx',
      safeWorkbook(`<worksheet><sheetData>${rows}</sheetData></worksheet>`)
    ),
    'ROW_COUNT_LIMIT'
  )
})

test('rejects workbooks with more than 5,000 entity rows', async () => {
  const rows = '<row />'.repeat(5_001)
  await assertPreflightError(
    makeFile(
      'channels.xlsx',
      safeWorkbook(`<worksheet><sheetData>${rows}</sheetData></worksheet>`)
    ),
    'ENTITY_COUNT_LIMIT'
  )
})

test('returns sheet and row counts for a safe workbook', async () => {
  const result = await preflightWorkbook(
    makeFile(
      'channels.xlsx',
      safeWorkbook(
        '<worksheet><sheetData><row r="1" /><row r="2" /></sheetData></worksheet>'
      )
    )
  )
  assert.equal(result.sheetCount, 1)
  assert.equal(result.rowCount, 2)
  assert.equal(result.entityCount, 2)
})
