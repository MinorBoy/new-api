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
import {
  BlobReader,
  TextWriter,
  ZipReader,
  type FileEntry,
} from '@zip.js/zip.js'

export const SECURITY_LIMITS = {
  maxFileBytes: 10 * 1024 * 1024,
  maxUncompressedBytes: 100 * 1024 * 1024,
  maxSheets: 20,
  maxRowsPerSheet: 20_000,
  maxEntities: 5_000,
} as const

export type PreflightErrorCode =
  | 'UNSUPPORTED_EXTENSION'
  | 'OLE_CONTAINER'
  | 'MACRO_CONTENT'
  | 'EXTERNAL_LINK_OR_CONNECTION'
  | 'FILE_SIZE_LIMIT'
  | 'ZIP_UNCOMPRESSED_SIZE_LIMIT'
  | 'SHEET_COUNT_LIMIT'
  | 'ROW_COUNT_LIMIT'
  | 'ENTITY_COUNT_LIMIT'

export class WorkbookPreflightError extends Error {
  readonly code: PreflightErrorCode

  constructor(code: PreflightErrorCode, message?: string) {
    super(message ?? code)
    this.name = 'WorkbookPreflightError'
    this.code = code
  }
}

export type WorkbookPreflightResult = {
  sheetCount: number
  rowCount: number
  entityCount: number
}

type WorkbookFile = Blob & { name?: string }

const OLE_SIGNATURE = Uint8Array.from([
  0xd0, 0xcf, 0x11, 0xe0, 0xa1, 0xb1, 0x1a, 0xe1,
])

function hasPrefix(bytes: Uint8Array, prefix: Uint8Array): boolean {
  return prefix.every((value, index) => bytes[index] === value)
}

function isWorksheetEntry(name: string): boolean {
  return /^xl\/worksheets\/[^/]+\.xml$/i.test(name)
}

function isMacroEntry(name: string): boolean {
  return (
    /(^|\/)vbaProject\.bin$/i.test(name) || /(^|\/)vbaData\.xml$/i.test(name)
  )
}

function isExternalEntry(name: string): boolean {
  return (
    /(^|\/)externalLinks?(\/|\.)/i.test(name) ||
    /(^|\/)connections?(\/|\.)/i.test(name)
  )
}

function countRows(xml: string): number {
  return (xml.match(/<row(?:\s|>)/gi) ?? []).length
}

export async function preflightWorkbook(
  file: WorkbookFile
): Promise<WorkbookPreflightResult> {
  const fileName = file.name?.toLowerCase() ?? ''
  if (!fileName.endsWith('.xlsx')) {
    throw new WorkbookPreflightError(
      'UNSUPPORTED_EXTENSION',
      'Only .xlsx workbooks are supported.'
    )
  }

  if (file.size > SECURITY_LIMITS.maxFileBytes) {
    throw new WorkbookPreflightError(
      'FILE_SIZE_LIMIT',
      'The workbook exceeds the input size limit.'
    )
  }

  const bytes = new Uint8Array(
    await file.slice(0, OLE_SIGNATURE.length).arrayBuffer()
  )
  if (hasPrefix(bytes, OLE_SIGNATURE)) {
    throw new WorkbookPreflightError(
      'OLE_CONTAINER',
      'Legacy OLE containers are not supported.'
    )
  }

  const reader = new ZipReader(new BlobReader(file), {
    checkSignature: false,
    useWebWorkers: false,
  })

  try {
    const entries = await reader.getEntries()
    let uncompressedBytes = 0
    const worksheetEntries = entries.filter(
      (entry): entry is FileEntry =>
        !entry.directory && isWorksheetEntry(entry.filename)
    )

    for (const entry of entries) {
      if (isMacroEntry(entry.filename)) {
        throw new WorkbookPreflightError(
          'MACRO_CONTENT',
          'Macro-enabled workbook content is not supported.'
        )
      }
      if (isExternalEntry(entry.filename)) {
        throw new WorkbookPreflightError(
          'EXTERNAL_LINK_OR_CONNECTION',
          'External links and connections are not supported.'
        )
      }
      if (
        typeof entry.uncompressedSize === 'number' &&
        Number.isFinite(entry.uncompressedSize)
      ) {
        uncompressedBytes += entry.uncompressedSize
      }
    }

    if (uncompressedBytes > SECURITY_LIMITS.maxUncompressedBytes) {
      throw new WorkbookPreflightError(
        'ZIP_UNCOMPRESSED_SIZE_LIMIT',
        'The workbook expands beyond the archive size limit.'
      )
    }

    if (worksheetEntries.length > SECURITY_LIMITS.maxSheets) {
      throw new WorkbookPreflightError(
        'SHEET_COUNT_LIMIT',
        'The workbook contains too many worksheets.'
      )
    }

    let rowCount = 0
    for (const entry of worksheetEntries) {
      const xml = await entry.getData(new TextWriter())
      const sheetRows = countRows(xml)
      if (sheetRows > SECURITY_LIMITS.maxRowsPerSheet) {
        throw new WorkbookPreflightError(
          'ROW_COUNT_LIMIT',
          'A worksheet contains too many rows.'
        )
      }
      rowCount += sheetRows
      if (rowCount > SECURITY_LIMITS.maxEntities) {
        throw new WorkbookPreflightError(
          'ENTITY_COUNT_LIMIT',
          'The workbook contains too many entity rows.'
        )
      }
    }

    return {
      sheetCount: worksheetEntries.length,
      rowCount,
      entityCount: rowCount,
    }
  } finally {
    await reader.close()
  }
}
