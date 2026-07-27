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
  assertExactHeaders,
  assertReference,
  assertUniqueBusinessIds,
  cellText,
  dataRows,
  fieldsFromRow,
  getSheet,
} from '../schema'
import type {
  AdapterMatch,
  ExtractedChannelLine,
  ExtractedEntity,
  ExtractedWorkbook,
  SnapshotRow,
  WorkbookAdapter,
  WorkbookSnapshot,
  WorksheetSnapshot,
} from '../types'
import { V2_HEADERS } from '../workbook'

const V2_SHEET_NAMES = Object.keys(V2_HEADERS)

function entityFromRow(
  sheet: WorksheetSnapshot,
  headers: readonly string[],
  row: SnapshotRow,
  lineRef?: string
): ExtractedEntity {
  const businessId = cellText(row.cells[0])
  return {
    businessId,
    fields: fieldsFromRow(headers, row),
    sourceLocations: [
      {
        sheet: sheet.name,
        row: row.rowNumber,
        businessId,
      },
    ],
    lineRef,
  }
}

function assertV2Contract(workbook: WorkbookSnapshot): void {
  for (const sheetName of V2_SHEET_NAMES) {
    const sheet = getSheet(workbook, sheetName)
    assertExactHeaders(sheet, V2_HEADERS[sheetName as keyof typeof V2_HEADERS])
  }
}

function ids(entities: ExtractedEntity[]): Set<string> {
  return new Set(entities.map((entity) => entity.businessId))
}

export class V2WorkbookAdapter implements WorkbookAdapter {
  readonly templateVersion = '2' as const

  matches(workbook: WorkbookSnapshot): AdapterMatch {
    for (const sheetName of V2_SHEET_NAMES) {
      const sheet = workbook.sheets.find(
        (candidate) => candidate.name === sheetName
      )
      if (!sheet) {
        return { matched: false, reason: `Missing worksheet: ${sheetName}` }
      }
      const expectedHeaders = V2_HEADERS[sheetName as keyof typeof V2_HEADERS]
      const headers = sheet.rows[3]?.cells.map((cell) => cellText(cell)) ?? []
      if (
        headers.length !== expectedHeaders.length ||
        headers.some((header, index) => header !== expectedHeaders[index])
      ) {
        return { matched: false, reason: `Invalid header: ${sheetName}` }
      }
    }
    return { matched: true }
  }

  extract(workbook: WorkbookSnapshot): ExtractedWorkbook {
    assertV2Contract(workbook)

    const sourceSheet = getSheet(workbook, '来源')
    const channelSheet = getSheet(workbook, '渠道')
    const lineSheet = getSheet(workbook, '渠道线路')
    const skuSheet = getSheet(workbook, '模型SKU')
    const saleSheet = getSheet(workbook, '官方售价')
    const costSheet = getSheet(workbook, '渠道成本')
    const routeSheet = getSheet(workbook, '路由目标')
    const mappingSheet = getSheet(workbook, '模型映射')

    const sourceRows = dataRows(sourceSheet)
    const channelRows = dataRows(channelSheet)
    const lineRows = dataRows(lineSheet)
    const skuRows = dataRows(skuSheet)
    const saleRows = dataRows(saleSheet)
    const costRows = dataRows(costSheet)
    const routeRows = dataRows(routeSheet)
    const mappingRows = dataRows(mappingSheet)
    for (const [sheet, rows] of [
      [sourceSheet, sourceRows],
      [channelSheet, channelRows],
      [lineSheet, lineRows],
      [skuSheet, skuRows],
      [saleSheet, saleRows],
      [costSheet, costRows],
      [routeSheet, routeRows],
      [mappingSheet, mappingRows],
    ] as const) {
      assertUniqueBusinessIds(sheet, rows)
    }

    const sources = sourceRows.map((row) =>
      entityFromRow(sourceSheet, V2_HEADERS.来源, row)
    )
    const channels = channelRows.map((row) =>
      entityFromRow(channelSheet, V2_HEADERS.渠道, row)
    )
    const rawLines = lineRows.map((row) =>
      entityFromRow(lineSheet, V2_HEADERS.渠道线路, row)
    )
    const channelLines: ExtractedChannelLine[] = rawLines.map((line) => ({
      ...line,
      channelRef: cellText(line.fields.channel_ref),
      supportsRealPerson:
        typeof line.fields.supports_real_person.value === 'boolean'
          ? line.fields.supports_real_person.value
          : undefined,
    }))
    const modelSkus = skuRows.map((row) =>
      entityFromRow(skuSheet, V2_HEADERS.模型SKU, row)
    )
    const saleProposals = saleRows.map((row) =>
      entityFromRow(saleSheet, V2_HEADERS.官方售价, row)
    )
    const routeBlueprints = routeRows.map((row) => {
      const entity = entityFromRow(routeSheet, V2_HEADERS.路由目标, row)
      return { ...entity, lineRef: cellText(entity.fields.line_ref) }
    })
    const costRuleDrafts = costRows.map((row) => {
      const entity = entityFromRow(costSheet, V2_HEADERS.渠道成本, row)
      return { ...entity, lineRef: cellText(entity.fields.line_ref) }
    })
    const modelMappings = mappingRows.map((row) => {
      const entity = entityFromRow(mappingSheet, V2_HEADERS.模型映射, row)
      return { ...entity, lineRef: cellText(entity.fields.line_ref) }
    })

    const sourceIds = ids(sources)
    const channelIds = ids(channels)
    const lineIds = ids(channelLines)
    const skuIds = ids(modelSkus)
    const routeIds = ids(routeBlueprints)
    for (const channel of channels) {
      assertReference(channel, 'source_ref', sourceIds)
    }
    for (const line of channelLines) {
      assertReference(line, 'channel_ref', channelIds)
      assertReference(line, 'source_ref', sourceIds)
    }
    for (const sku of modelSkus) {
      assertReference(sku, 'source_ref', sourceIds)
    }
    for (const sale of saleProposals) {
      assertReference(sale, 'sku_ref', skuIds)
      assertReference(sale, 'source_ref', sourceIds)
    }
    for (const route of routeBlueprints) {
      assertReference(route, 'line_ref', lineIds)
      assertReference(route, 'sku_ref', skuIds)
    }
    for (const cost of costRuleDrafts) {
      assertReference(cost, 'line_ref', lineIds)
      assertReference(cost, 'sku_ref', skuIds)
      assertReference(cost, 'route_target_ref', routeIds)
      assertReference(cost, 'source_ref', sourceIds)
    }
    for (const mapping of modelMappings) {
      assertReference(mapping, 'line_ref', lineIds)
      assertReference(mapping, 'sku_ref', skuIds)
      assertReference(mapping, 'route_target_ref', routeIds)
      assertReference(mapping, 'source_ref', sourceIds)
    }

    return {
      templateVersion: this.templateVersion,
      channels,
      channelLines,
      modelSkus,
      saleProposals,
      costRuleDrafts,
      modelMappings,
      routeBlueprints,
      sources,
      unresolvedVariants: [],
      compatibilityKeys: [],
    }
  }
}
