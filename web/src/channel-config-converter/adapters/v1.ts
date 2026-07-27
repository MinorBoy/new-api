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
  WorkbookContractError,
} from '../schema'
import type {
  AdapterMatch,
  CompatibilityKey,
  ExtractedChannelLine,
  ExtractedEntity,
  ExtractedWorkbook,
  SnapshotRow,
  SourceLocation,
  WorkbookAdapter,
  WorkbookSnapshot,
  WorksheetSnapshot,
} from '../types'
import { V1_HEADERS } from '../workbook'

const V1_SHEET_NAMES = Object.keys(V1_HEADERS)
const MANUAL_CONFLICT_KEY = 'CH-MEGABYAI/videos-standard'

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

function defaultLineRef(channelCode: string): string {
  return `channel-${channelCode.toLowerCase().replace(/^ch-/, '')}`
}

function secureLineRef(businessId: string): string | undefined {
  if (/^COST-SECURE-R66-|^MAP-SECURE-R66-/.test(businessId)) {
    return 'secure-enterprise'
  }
  if (
    /^COST-SECURE-R(?:67|68|69|70|71)-|^MAP-SECURE-R(?:67|68|69|70|71)-/.test(
      businessId
    )
  ) {
    return 'secure-discount'
  }
  if (
    /^COST-SECURE-R(?:72|73|74|75)-|^MAP-SECURE-R(?:72|73|74|75)-/.test(
      businessId
    )
  ) {
    return 'secure-overseas'
  }
  return undefined
}

function megaByAIFastLineRef(businessId: string): string | undefined {
  if (/^(?:COST|MAP)-MEGABYAI-R(?:102|103)-/.test(businessId)) {
    return 'megabyai-fast-real-person'
  }
  if (/^(?:COST|MAP)-MEGABYAI-R(?:104|105)-/.test(businessId)) {
    return 'megabyai-fast-no-real-person'
  }
  return undefined
}

function lineRefFor(
  channelCode: string,
  upstreamModel: string,
  businessId: string
): string | undefined {
  if (channelCode === 'CH-SECURE') {
    return secureLineRef(businessId)
  }
  if (channelCode === 'CH-MEGABYAI') {
    if (upstreamModel === 'videos-fast') {
      return megaByAIFastLineRef(businessId)
    }
    return undefined
  }
  return defaultLineRef(channelCode)
}

function isUnsupportedSecure480(
  entity: ExtractedEntity,
  resolutionBySku: Map<string, string>
): boolean {
  const channelCode = cellText(entity.fields['渠道代码'])
  const upstreamModel = cellText(entity.fields['上游模型'])
  const skuCode = cellText(entity.fields['SKU代码'])
  return (
    channelCode === 'CH-SECURE' &&
    (upstreamModel === 'video-2.0-fast' ||
      upstreamModel === 'video-2.0-mini') &&
    resolutionBySku.get(skuCode) === '480p'
  )
}

function scenarioBaseId(businessId: string): string {
  return businessId.replace(/-(?:NOV|VID)$/, '')
}

function generatedField(value: boolean | string) {
  return { value, formula: null, formulaResult: null }
}

function createChannelLines(
  channels: ExtractedEntity[]
): ExtractedChannelLine[] {
  return channels.flatMap((channel) => {
    const channelRef = channel.businessId
    const sourceLocations = channel.sourceLocations
    if (channelRef === 'CH-SECURE') {
      return ['secure-discount', 'secure-overseas', 'secure-enterprise'].map(
        (businessId) => ({
          businessId,
          channelRef,
          fields: {
            channel_ref: generatedField(channelRef),
            line_ref: generatedField(businessId),
            status_proposal: generatedField('disabled'),
          },
          sourceLocations,
        })
      )
    }
    if (channelRef === 'CH-MEGABYAI') {
      return [
        { businessId: 'megabyai-fast-real-person', supportsRealPerson: true },
        {
          businessId: 'megabyai-fast-no-real-person',
          supportsRealPerson: false,
        },
      ].map((line) => ({
        businessId: line.businessId,
        channelRef,
        supportsRealPerson: line.supportsRealPerson,
        fields: {
          channel_ref: generatedField(channelRef),
          line_ref: generatedField(line.businessId),
          status_proposal: generatedField('disabled'),
          supports_real_person: generatedField(line.supportsRealPerson),
        },
        sourceLocations,
      }))
    }
    const businessId = defaultLineRef(channelRef)
    return [
      {
        businessId,
        channelRef,
        fields: {
          channel_ref: generatedField(channelRef),
          line_ref: generatedField(businessId),
          status_proposal: generatedField('disabled'),
        },
        sourceLocations,
      },
    ]
  })
}

function assertV1Contract(workbook: WorkbookSnapshot): void {
  for (const sheetName of V1_SHEET_NAMES) {
    const sheet = getSheet(workbook, sheetName)
    assertExactHeaders(sheet, V1_HEADERS[sheetName as keyof typeof V1_HEADERS])
  }
  if (workbook.sheets.length !== V1_SHEET_NAMES.length) {
    throw new WorkbookContractError(
      'UNSUPPORTED_TEMPLATE',
      'The workbook contains sheets outside the fixed v1 contract.'
    )
  }
}

function sourceLocationsFor(entities: ExtractedEntity[]): SourceLocation[] {
  return entities.flatMap((entity) => entity.sourceLocations)
}

export class V1WorkbookAdapter implements WorkbookAdapter {
  readonly templateVersion = '1' as const

  matches(workbook: WorkbookSnapshot): AdapterMatch {
    if (workbook.sheets.length !== V1_SHEET_NAMES.length) {
      return { matched: false, reason: 'V1 requires exactly ten worksheets.' }
    }
    for (const sheetName of V1_SHEET_NAMES) {
      const sheet = workbook.sheets.find(
        (candidate) => candidate.name === sheetName
      )
      if (!sheet) {
        return { matched: false, reason: `Missing worksheet: ${sheetName}` }
      }
      const expectedHeaders = V1_HEADERS[sheetName as keyof typeof V1_HEADERS]
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
    assertV1Contract(workbook)

    const sourceSheet = getSheet(workbook, '来源')
    const channelSheet = getSheet(workbook, '渠道')
    const skuSheet = getSheet(workbook, '模型SKU')
    const saleSheet = getSheet(workbook, '官方售价')
    const costSheet = getSheet(workbook, '渠道成本')
    const mappingSheet = getSheet(workbook, '模型映射')

    const sourceRows = dataRows(sourceSheet)
    const channelRows = dataRows(channelSheet)
    const skuRows = dataRows(skuSheet)
    const saleRows = dataRows(saleSheet)
    const costRows = dataRows(costSheet)
    const mappingRows = dataRows(mappingSheet)

    for (const [sheet, rows] of [
      [sourceSheet, sourceRows],
      [channelSheet, channelRows],
      [skuSheet, skuRows],
      [saleSheet, saleRows],
      [costSheet, costRows],
      [mappingSheet, mappingRows],
    ] as const) {
      assertUniqueBusinessIds(sheet, rows)
    }

    const sources = sourceRows.map((row) =>
      entityFromRow(sourceSheet, V1_HEADERS.来源, row)
    )
    const channels = channelRows.map((row) =>
      entityFromRow(channelSheet, V1_HEADERS.渠道, row)
    )
    const rawSkus = skuRows.map((row) =>
      entityFromRow(skuSheet, V1_HEADERS.模型SKU, row)
    )
    const rawSales = saleRows.map((row) =>
      entityFromRow(saleSheet, V1_HEADERS.官方售价, row)
    )
    const rawCosts = costRows.map((row) =>
      entityFromRow(costSheet, V1_HEADERS.渠道成本, row)
    )
    const rawMappings = mappingRows.map((row) =>
      entityFromRow(mappingSheet, V1_HEADERS.模型映射, row)
    )

    const sourceIds = new Set(sources.map((entity) => entity.businessId))
    const channelIds = new Set(channels.map((entity) => entity.businessId))
    const skuIds = new Set(rawSkus.map((entity) => entity.businessId))
    for (const channel of channels) {
      assertReference(channel, '来源ID', sourceIds)
    }
    for (const sku of rawSkus) {
      assertReference(sku, '来源ID', sourceIds)
    }
    for (const sale of rawSales) {
      assertReference(sale, 'SKU代码', skuIds)
      assertReference(sale, '来源ID', sourceIds)
    }
    for (const cost of rawCosts) {
      assertReference(cost, '渠道代码', channelIds)
      assertReference(cost, 'SKU代码', skuIds)
      assertReference(cost, '来源ID', sourceIds)
    }
    for (const mapping of rawMappings) {
      assertReference(mapping, '渠道代码', channelIds)
      assertReference(mapping, 'SKU代码', skuIds)
      assertReference(mapping, '来源ID', sourceIds)
    }

    const modelSkus = rawSkus
    const saleProposals = rawSales.filter(
      (sale) => cellText(sale.fields['状态']) === 'active'
    )
    const resolutionBySku = new Map(
      rawSkus.map((sku) => [sku.businessId, cellText(sku.fields['分辨率档位'])])
    )

    const costsByScenarioBase = new Map<string, ExtractedEntity[]>()
    for (const cost of rawCosts) {
      if (isUnsupportedSecure480(cost, resolutionBySku)) {
        continue
      }
      const baseId = scenarioBaseId(cost.businessId)
      const group = costsByScenarioBase.get(baseId) ?? []
      group.push(cost)
      costsByScenarioBase.set(baseId, group)
    }
    const costRuleDrafts = [...costsByScenarioBase.entries()].map(
      ([businessId, candidates]) => {
        const representative = candidates[0]
        const channelCode = cellText(representative.fields['渠道代码'])
        const upstreamModel = cellText(representative.fields['上游模型'])
        return {
          ...representative,
          businessId,
          lineRef: lineRefFor(channelCode, upstreamModel, businessId),
          sourceLocations: sourceLocationsFor(candidates),
        }
      }
    )
    const modelMappings = rawMappings
      .filter((mapping) => !isUnsupportedSecure480(mapping, resolutionBySku))
      .map((mapping) => ({
        ...mapping,
        lineRef: lineRefFor(
          cellText(mapping.fields['渠道代码']),
          cellText(mapping.fields['上游模型']),
          mapping.businessId
        ),
      }))

    const conflicts = new Map<string, ExtractedEntity[]>()
    for (const cost of costRuleDrafts) {
      const key = `${cellText(cost.fields['渠道代码'])}/${cellText(cost.fields['上游模型'])}`
      const candidates = conflicts.get(key) ?? []
      candidates.push(cost)
      conflicts.set(key, candidates)
    }
    const compatibilityKeys: CompatibilityKey[] = [...conflicts.entries()]
      .filter(([, candidates]) => {
        const prices = new Set(
          candidates.map((candidate) =>
            cellText(candidate.fields['原币基础单价'])
          )
        )
        return prices.size > 1
      })
      .map(([businessId, candidates]) => ({
        businessId,
        automatic: businessId !== MANUAL_CONFLICT_KEY,
        sourceLocations: sourceLocationsFor(candidates),
      }))
      .sort((left, right) => left.businessId.localeCompare(right.businessId))

    const unresolvedVariants = compatibilityKeys
      .filter((key) => !key.automatic)
      .map((key) => ({
        businessId: key.businessId,
        fields: {
          channel_code: generatedField('CH-MEGABYAI'),
          upstream_model: generatedField('videos-standard'),
        },
        sourceLocations: key.sourceLocations,
      }))

    return {
      templateVersion: this.templateVersion,
      channels,
      channelLines: createChannelLines(channels),
      modelSkus,
      saleProposals,
      costRuleDrafts,
      modelMappings,
      routeBlueprints: [],
      sources,
      unresolvedVariants,
      compatibilityKeys,
    }
  }
}

export function extractWorkbook(workbook: WorkbookSnapshot): ExtractedWorkbook {
  return new V1WorkbookAdapter().extract(workbook)
}
