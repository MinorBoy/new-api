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
import type {
  CellSnapshot,
  ExtractedEntity,
  SnapshotRow,
  WorkbookSnapshot,
  WorksheetSnapshot,
} from './types'

export type WorkbookContractErrorCode =
  | 'BROKEN_REFERENCE'
  | 'DUPLICATE_BUSINESS_ID'
  | 'INVALID_HEADER'
  | 'MISSING_SHEET'
  | 'UNSUPPORTED_TEMPLATE'

export class WorkbookContractError extends Error {
  readonly code: WorkbookContractErrorCode

  constructor(code: WorkbookContractErrorCode, message: string) {
    super(message)
    this.name = 'WorkbookContractError'
    this.code = code
  }
}

export function cellText(cell: CellSnapshot | undefined): string {
  const value = cell?.value
  if (value === null || value === undefined) {
    return ''
  }
  if (value instanceof Date) {
    return value.toISOString()
  }
  return String(value).trim()
}

export function getSheet(
  workbook: WorkbookSnapshot,
  name: string
): WorksheetSnapshot {
  const sheet = workbook.sheets.find((candidate) => candidate.name === name)
  if (!sheet) {
    throw new WorkbookContractError(
      'MISSING_SHEET',
      `Missing worksheet: ${name}`
    )
  }
  return sheet
}

export function assertExactHeaders(
  sheet: WorksheetSnapshot,
  expected: readonly string[]
): void {
  const headers = sheet.rows[3]?.cells.map((cell) => cellText(cell)) ?? []
  if (
    headers.length !== expected.length ||
    headers.some((header, index) => header !== expected[index])
  ) {
    throw new WorkbookContractError(
      'INVALID_HEADER',
      `Worksheet ${sheet.name} does not match its row-four header contract.`
    )
  }
}

export function dataRows(sheet: WorksheetSnapshot): SnapshotRow[] {
  return sheet.rows.slice(4).filter((row) => cellText(row.cells[0]) !== '')
}

export function fieldsFromRow(
  headers: readonly string[],
  row: SnapshotRow
): Record<string, CellSnapshot> {
  return Object.fromEntries(
    headers.map((header, index) => [
      header,
      row.cells[index] ?? { value: null, formula: null, formulaResult: null },
    ])
  )
}

export function assertUniqueBusinessIds(
  sheet: WorksheetSnapshot,
  rows: SnapshotRow[]
): void {
  const ids = new Set<string>()
  for (const row of rows) {
    const businessId = cellText(row.cells[0])
    if (ids.has(businessId)) {
      throw new WorkbookContractError(
        'DUPLICATE_BUSINESS_ID',
        `Worksheet ${sheet.name} contains duplicate business ID: ${businessId}`
      )
    }
    ids.add(businessId)
  }
}

export function assertReference(
  entity: ExtractedEntity,
  field: string,
  targets: Set<string>
): void {
  const reference = cellText(entity.fields[field])
  if (reference === '' || !targets.has(reference)) {
    throw new WorkbookContractError(
      'BROKEN_REFERENCE',
      `${entity.businessId} references an unknown ${field}: ${reference}`
    )
  }
}
