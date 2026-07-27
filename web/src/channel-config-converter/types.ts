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
export type CellValue = boolean | Date | number | string | null

export type CellSnapshot = {
  value: CellValue
  formula: string | null
  formulaResult: CellValue
}

export type SnapshotRow = {
  rowNumber: number
  cells: CellSnapshot[]
}

export type WorksheetSnapshot = {
  name: string
  rows: SnapshotRow[]
}

export type WorkbookSnapshot = {
  sheets: WorksheetSnapshot[]
}

export type SourceLocation = {
  sheet: string
  row: number
  businessId: string
}

export type AdapterMatch = {
  matched: boolean
  reason?: string
}

export type ExtractedEntity = {
  businessId: string
  fields: Record<string, CellSnapshot>
  sourceLocations: SourceLocation[]
  lineRef?: string
}

export type ExtractedChannelLine = ExtractedEntity & {
  channelRef: string
  supportsRealPerson?: boolean
}

export type CompatibilityKey = {
  businessId: string
  automatic: boolean
  sourceLocations: SourceLocation[]
}

export type ExtractedWorkbook = {
  templateVersion: '1' | '2'
  channels: ExtractedEntity[]
  channelLines: ExtractedChannelLine[]
  modelSkus: ExtractedEntity[]
  saleProposals: ExtractedEntity[]
  costRuleDrafts: ExtractedEntity[]
  modelMappings: ExtractedEntity[]
  routeBlueprints: ExtractedEntity[]
  sources: ExtractedEntity[]
  unresolvedVariants: ExtractedEntity[]
  compatibilityKeys: CompatibilityKey[]
}

export interface WorkbookAdapter {
  readonly templateVersion: '1' | '2'

  matches(workbook: WorkbookSnapshot): AdapterMatch
  extract(workbook: WorkbookSnapshot): ExtractedWorkbook
}
