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
import { V1WorkbookAdapter } from './adapters/v1'
import { V2WorkbookAdapter } from './adapters/v2'
import { buildImportDocument, type ImportDocumentResult } from './document'
import { WorkbookContractError } from './schema'
import { preflightWorkbook } from './security'
import { loadWorkbookSnapshot } from './workbook'

export type WorkbookConversion = ImportDocumentResult

export async function convertWorkbook(file: File): Promise<WorkbookConversion> {
  await preflightWorkbook(file)
  const sourceBytes = new Uint8Array(await file.arrayBuffer())
  const snapshot = await loadWorkbookSnapshot(sourceBytes)
  const adapter = [new V2WorkbookAdapter(), new V1WorkbookAdapter()].find(
    (candidate) => candidate.matches(snapshot).matched
  )
  if (!adapter) {
    throw new WorkbookContractError(
      'UNSUPPORTED_TEMPLATE',
      'No supported workbook template matched.'
    )
  }
  return buildImportDocument({
    extracted: adapter.extract(snapshot),
    sourceBytes,
    sourceFileName: file.name,
  })
}
