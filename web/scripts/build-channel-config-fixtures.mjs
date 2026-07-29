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
import crypto from 'node:crypto'
import fs from 'node:fs/promises'
import os from 'node:os'
import path from 'node:path'
import zlib from 'node:zlib'

import { FileBlob, SpreadsheetFile, Workbook } from '@oai/artifact-tool'

import { extractWorkbook } from '../src/channel-config-converter/adapters/v1.ts'
import {
  buildImportDocument,
  serializeImportDocument,
} from '../src/channel-config-converter/document.ts'
import {
  loadWorkbookSnapshot,
  V2_HEADERS,
} from '../src/channel-config-converter/workbook.ts'

const projectRoot = path.resolve(import.meta.dirname, '..', '..')
const outputDir = path.join(
  projectRoot,
  'outputs',
  '019f9dbb-4e5d-7933-8531-d38e417ec068'
)
const fixturePath = path.join(
  projectRoot,
  'web',
  'src',
  'channel-config-converter',
  '__fixtures__',
  'channel-config-v1-corrected.xlsx'
)
const expectedCountsPath = path.join(
  projectRoot,
  'web',
  'src',
  'channel-config-converter',
  '__fixtures__',
  'v1-expected-counts.json'
)
const v2TemplatePath = path.join(
  projectRoot,
  'docs',
  'templates',
  'channel-config-v2.xlsx'
)
const v2GoldenPath = path.join(
  projectRoot,
  'web',
  'src',
  'channel-config-converter',
  '__fixtures__',
  'channel-config-v2-golden.xlsx'
)
const v1ImportFixturePath = path.join(
  projectRoot,
  'e2e',
  'testdata',
  'channel-config-v1.json'
)

const expectedCounts = {
  channels: 9,
  channel_lines: 12,
  model_skus: 9,
  sale_proposals: 16,
  cost_rule_drafts: 121,
  model_mappings: 121,
  detected_conflict_keys: 17,
  automatic_conflict_keys: 16,
  manual_conflict_keys: 1,
  manual_conflict_business_id: 'CH-MEGABYAI/videos-standard',
}

const corrections = [
  {
    stableCostId: 'COST-MEGABYAI-R102-480-REQ',
    mappingId: 'MAP-MEGABYAI-R102-480',
    sourceRow: 'sd!102',
    upstreamModel: 'videos-fast',
    sku: 'SKU-SD20-FAST-480',
    priceCny: 3,
    supportsRealPerson: true,
  },
  {
    stableCostId: 'COST-MEGABYAI-R103-720-REQ',
    mappingId: 'MAP-MEGABYAI-R103-720',
    sourceRow: 'sd!103',
    upstreamModel: 'videos-fast',
    sku: 'SKU-SD20-FAST-720',
    priceCny: 4,
    supportsRealPerson: true,
  },
  {
    stableCostId: 'COST-MEGABYAI-R104-480-REQ',
    mappingId: 'MAP-MEGABYAI-R104-480',
    sourceRow: 'sd!104',
    upstreamModel: 'videos-fast',
    sku: 'SKU-SD20-FAST-480',
    priceCny: 1.2,
    supportsRealPerson: false,
  },
  {
    stableCostId: 'COST-MEGABYAI-R105-720-REQ',
    mappingId: 'MAP-MEGABYAI-R105-720',
    sourceRow: 'sd!105',
    upstreamModel: 'videos-fast',
    sku: 'SKU-SD20-FAST-720',
    priceCny: 1.6,
    supportsRealPerson: false,
  },
]

const toBooleanLabel = (value) => (value ? '是' : '否')

function buildCostNote(correction) {
  const capability = toBooleanLabel(correction.supportsRealPerson)
  return [
    `源行=${correction.sourceRow}`,
    `原模型=${correction.upstreamModel}`,
    '时长=4-15',
    '比例=不限',
    '视频输入=是',
    `真人脸=${capability}`,
    '超分=否',
    '素材库=否',
    'NSFW=否',
    '协议=自有',
    '素材限制=933',
    '原表状态为空，已导入为draft',
    `v1修正：confirmed_real_person=${String(correction.supportsRealPerson)}；原币按次=${correction.priceCny} CNY`,
  ].join('；')
}

function buildMappingNote(correction) {
  const capability = toBooleanLabel(correction.supportsRealPerson)
  return [
    `源行=${correction.sourceRow}`,
    '时长=4-15',
    '比例=不限',
    '视频输入=是',
    `真人脸=${capability}`,
    '超分=否',
    '素材库=否',
    'NSFW=否',
    '协议=自有',
    '原状态=空',
    '素材限制=933',
    '原表状态为空，默认停用待确认',
    `v1修正：confirmed_real_person=${String(correction.supportsRealPerson)}；原币按次=${correction.priceCny} CNY`,
  ].join('；')
}

async function sha256File(filePath) {
  const bytes = await fs.readFile(filePath)
  return {
    bytes,
    sha256: crypto
      .createHash('sha256')
      .update(bytes)
      .digest('hex')
      .toUpperCase(),
  }
}

async function findSourceFile() {
  const entries = await fs.readdir(outputDir, { withFileTypes: true })
  const candidates = entries
    .filter(
      (entry) =>
        entry.isFile() &&
        entry.name.toLowerCase().endsWith('.xlsx') &&
        entry.name.includes('-') &&
        !entry.name.includes('v1-')
    )
    .map((entry) => entry.name)
  if (candidates.length !== 1) {
    throw new Error(
      `Expected exactly one source workbook, found: ${candidates.join(', ')}`
    )
  }
  return path.join(outputDir, candidates[0])
}

function findRowsByStableCostId(sheet, stableCostId) {
  const usedRange = sheet.getUsedRange()
  const values = usedRange?.values ?? []
  const rows = []
  for (let index = 4; index < values.length; index += 1) {
    const id = values[index]?.[0]
    if (
      typeof id === 'string' &&
      (id === stableCostId || id.startsWith(`${stableCostId}-`))
    ) {
      rows.push({ index, id })
    }
  }
  return rows
}

function findRowByStableId(sheet, stableId) {
  const usedRange = sheet.getUsedRange()
  const values = usedRange?.values ?? []
  for (let index = 4; index < values.length; index += 1) {
    if (values[index]?.[0] === stableId) {
      return { index, id: stableId }
    }
  }
  return null
}

function setCellValue(sheet, rowIndex, columnIndex, value) {
  sheet.getCell(rowIndex, columnIndex).values = [[value]]
}

function setCellFormula(sheet, rowIndex, columnIndex, formula) {
  sheet.getCell(rowIndex, columnIndex).formulas = [[formula]]
}

function readUInt16(bytes, offset) {
  return bytes.readUInt16LE(offset)
}

function readUInt32(bytes, offset) {
  return bytes.readUInt32LE(offset)
}

function writeUInt16(bytes, offset, value) {
  bytes.writeUInt16LE(value, offset)
}

function writeUInt32(bytes, offset, value) {
  bytes.writeUInt32LE(value, offset)
}

function findEndOfCentralDirectory(bytes) {
  for (let offset = bytes.length - 22; offset >= 0; offset -= 1) {
    if (readUInt32(bytes, offset) === 0x06054b50) return offset
  }
  throw new Error('Invalid XLSX ZIP: end of central directory not found')
}

function canonicalizeRelationshipIds(contents) {
  const tokenPattern = /\bR[0-9a-f]{16}\b/gi
  const replacements = new Map()
  const commentIdentifierPattern =
    /[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/gi
  const commentReplacements = new Map()
  let nextId = 1
  for (const content of contents) {
    if (!content.name.endsWith('.rels')) {
      continue
    }
    for (const token of content.data.toString('utf8').match(tokenPattern) ??
      []) {
      if (!replacements.has(token)) {
        replacements.set(token, `R${String(nextId).padStart(16, '0')}`)
        nextId += 1
      }
    }
  }
  for (const content of contents) {
    if (!/xl\/(comments|persons|threadedcomments)\//i.test(content.name)) {
      continue
    }
    for (const token of content.data
      .toString('utf8')
      .match(commentIdentifierPattern) ?? []) {
      if (!commentReplacements.has(token)) {
        commentReplacements.set(
          token,
          `00000000-0000-0000-0000-${String(commentReplacements.size + 1).padStart(12, '0')}`
        )
      }
    }
  }
  return contents.map((content) => {
    if (!content.name.endsWith('.xml') && !content.name.endsWith('.rels')) {
      return content
    }
    const text = content.data
      .toString('utf8')
      .replaceAll(tokenPattern, (token) => replacements.get(token) ?? token)
      .replaceAll(
        commentIdentifierPattern,
        (token) => commentReplacements.get(token) ?? token
      )
      .replaceAll(/\bdT="[^"]+"/g, 'dT="2000-01-01T00:00:00Z"')
    return { ...content, data: Buffer.from(text, 'utf8') }
  })
}

function canonicalizeXlsxBytes(inputBytes) {
  const bytes = Buffer.from(inputBytes)
  const endOfCentralDirectory = findEndOfCentralDirectory(bytes)
  const entryCount = readUInt16(bytes, endOfCentralDirectory + 10)
  const centralDirectoryOffset = readUInt32(bytes, endOfCentralDirectory + 16)
  const contents = []
  let centralOffset = centralDirectoryOffset

  for (let index = 0; index < entryCount; index += 1) {
    if (readUInt32(bytes, centralOffset) !== 0x02014b50) {
      throw new Error(`Invalid XLSX ZIP: central entry ${index} is malformed`)
    }
    const nameLength = readUInt16(bytes, centralOffset + 28)
    const extraLength = readUInt16(bytes, centralOffset + 30)
    const commentLength = readUInt16(bytes, centralOffset + 32)
    const compressedSize = readUInt32(bytes, centralOffset + 20)
    const uncompressedSize = readUInt32(bytes, centralOffset + 24)
    const localHeaderOffset = readUInt32(bytes, centralOffset + 42)
    const name = bytes
      .subarray(centralOffset + 46, centralOffset + 46 + nameLength)
      .toString('utf8')
    const localNameLength = readUInt16(bytes, localHeaderOffset + 26)
    const localExtraLength = readUInt16(bytes, localHeaderOffset + 28)
    const dataOffset =
      localHeaderOffset + 30 + localNameLength + localExtraLength
    const compressedData = bytes.subarray(
      dataOffset,
      dataOffset + compressedSize
    )
    const method = readUInt16(bytes, centralOffset + 10)
    const data =
      method === 0
        ? Buffer.from(compressedData)
        : zlib.inflateRawSync(compressedData)
    if (data.length !== uncompressedSize) {
      throw new Error(`Invalid XLSX ZIP: ${name} has an unexpected size`)
    }
    contents.push({
      name,
      method,
      data,
      centralHeader: Buffer.from(
        bytes.subarray(
          centralOffset,
          centralOffset + 46 + nameLength + extraLength + commentLength
        )
      ),
      localHeader: Buffer.from(bytes.subarray(localHeaderOffset, dataOffset)),
    })
    centralOffset += 46 + nameLength + extraLength + commentLength
  }

  const normalizedContents = canonicalizeRelationshipIds(contents)
  const localParts = []
  const centralParts = []
  let nextLocalOffset = 0
  const fixedDosTime = 0
  const fixedDosDate = 33

  for (const content of normalizedContents) {
    const compressedData =
      content.method === 0
        ? content.data
        : zlib.deflateRawSync(content.data, {
            level: 9,
            memLevel: 8,
            strategy: zlib.constants.Z_DEFAULT_STRATEGY,
          })
    const crc = crc32(content.data)
    const localHeader = Buffer.from(content.localHeader)
    writeUInt16(localHeader, 10, fixedDosTime)
    writeUInt16(localHeader, 12, fixedDosDate)
    writeUInt32(localHeader, 14, crc)
    writeUInt32(localHeader, 18, compressedData.length)
    writeUInt32(localHeader, 22, content.data.length)
    localParts.push(localHeader, compressedData)

    const centralHeader = Buffer.from(content.centralHeader)
    writeUInt16(centralHeader, 12, fixedDosTime)
    writeUInt16(centralHeader, 14, fixedDosDate)
    writeUInt32(centralHeader, 16, crc)
    writeUInt32(centralHeader, 20, compressedData.length)
    writeUInt32(centralHeader, 24, content.data.length)
    writeUInt32(centralHeader, 42, nextLocalOffset)
    centralParts.push(centralHeader)
    nextLocalOffset += localHeader.length + compressedData.length
  }

  const localBytes = Buffer.concat(localParts)
  const centralBytes = Buffer.concat(centralParts)
  const end = Buffer.alloc(22)
  writeUInt32(end, 0, 0x06054b50)
  writeUInt16(end, 4, 0)
  writeUInt16(end, 6, 0)
  writeUInt16(end, 8, normalizedContents.length)
  writeUInt16(end, 10, normalizedContents.length)
  writeUInt32(end, 12, centralBytes.length)
  writeUInt32(end, 16, localBytes.length)
  writeUInt16(end, 20, 0)
  return Buffer.concat([localBytes, centralBytes, end])
}

function xlsxEntryHashes(inputBytes) {
  const bytes = Buffer.from(inputBytes)
  const endOfCentralDirectory = findEndOfCentralDirectory(bytes)
  const entryCount = readUInt16(bytes, endOfCentralDirectory + 10)
  const centralDirectoryOffset = readUInt32(bytes, endOfCentralDirectory + 16)
  const hashes = new Map()
  let centralOffset = centralDirectoryOffset
  for (let index = 0; index < entryCount; index += 1) {
    const nameLength = readUInt16(bytes, centralOffset + 28)
    const extraLength = readUInt16(bytes, centralOffset + 30)
    const commentLength = readUInt16(bytes, centralOffset + 32)
    const compressedSize = readUInt32(bytes, centralOffset + 20)
    const localHeaderOffset = readUInt32(bytes, centralOffset + 42)
    const name = bytes
      .subarray(centralOffset + 46, centralOffset + 46 + nameLength)
      .toString('utf8')
    const localNameLength = readUInt16(bytes, localHeaderOffset + 26)
    const localExtraLength = readUInt16(bytes, localHeaderOffset + 28)
    const dataOffset =
      localHeaderOffset + 30 + localNameLength + localExtraLength
    const compressedData = bytes.subarray(
      dataOffset,
      dataOffset + compressedSize
    )
    const method = readUInt16(bytes, centralOffset + 10)
    const data =
      method === 0
        ? Buffer.from(compressedData)
        : zlib.inflateRawSync(compressedData)
    hashes.set(name, crypto.createHash('sha256').update(data).digest('hex'))
    centralOffset += 46 + nameLength + extraLength + commentLength
  }
  return hashes
}

function differingXlsxEntries(firstBytes, secondBytes) {
  const first = xlsxEntryHashes(firstBytes)
  const second = xlsxEntryHashes(secondBytes)
  return [...new Set([...first.keys(), ...second.keys()])]
    .filter((name) => first.get(name) !== second.get(name))
    .sort()
}

function crc32(bytes) {
  let crc = 0xffffffff
  for (const byte of bytes) {
    crc ^= byte
    for (let bit = 0; bit < 8; bit += 1) {
      crc = (crc >>> 1) ^ (0xedb88320 & -(crc & 1))
    }
  }
  return (crc ^ 0xffffffff) >>> 0
}

function patchCorrectedRows(costSheet, mappingSheet) {
  const patchedCostRows = []
  const patchedMappingRows = []

  for (const correction of corrections) {
    const costRows = findRowsByStableCostId(costSheet, correction.stableCostId)
    if (costRows.length !== 2) {
      throw new Error(
        `${correction.stableCostId} must resolve to NOV and VID rows; found ${costRows.length}`
      )
    }

    const mappingRow = findRowByStableId(mappingSheet, correction.mappingId)
    if (!mappingRow) {
      throw new Error(`${correction.mappingId} was not found by stable ID`)
    }

    for (const { index, id } of costRows) {
      const excelRow = index + 1
      setCellValue(costSheet, index, 11, correction.priceCny)
      setCellFormula(
        costSheet,
        index,
        14,
        `=IF(F${excelRow}="per_request",L${excelRow},IF(F${excelRow}="per_duration",M${excelRow},IF(F${excelRow}="per_token",N${excelRow},0)))`
      )
      setCellFormula(
        costSheet,
        index,
        20,
        `=O${excelRow}*P${excelRow}*Q${excelRow}*R${excelRow}*(1+S${excelRow})*T${excelRow}`
      )
      setCellValue(costSheet, index, 26, buildCostNote(correction))
      patchedCostRows.push({ id, excelRow, priceCny: correction.priceCny })
    }

    setCellValue(
      mappingSheet,
      mappingRow.index,
      10,
      buildMappingNote(correction)
    )
    patchedMappingRows.push({
      id: mappingRow.id,
      excelRow: mappingRow.index + 1,
    })
  }

  return { patchedCostRows, patchedMappingRows }
}

function scanFormulaErrors(workbook) {
  const errorPattern = /#REF!|#DIV\/0!|#VALUE!|#NAME\?|#N\/A/i
  const errors = []
  for (const sheet of workbook.worksheets.items) {
    const usedRange = sheet.getUsedRange()
    if (!usedRange) continue
    const values = usedRange.values ?? []
    const formulas = usedRange.formulas ?? []
    for (let row = 0; row < values.length; row += 1) {
      for (let col = 0; col < (values[row]?.length ?? 0); col += 1) {
        const value = values[row][col]
        const formula = formulas[row]?.[col]
        if (
          (typeof value === 'string' && errorPattern.test(value)) ||
          (typeof formula === 'string' && errorPattern.test(formula))
        ) {
          errors.push({
            sheet: sheet.name,
            row: row + 1,
            col: col + 1,
            value,
            formula,
          })
        }
      }
    }
  }
  return errors
}

async function renderSheets(workbook) {
  const renderDir = await fs.mkdtemp(
    path.join(os.tmpdir(), 'new-api-channel-config-')
  )
  const renders = []
  for (const sheet of workbook.worksheets.items) {
    const preview = await workbook.render({
      sheetName: sheet.name,
      autoCrop: 'all',
      scale: 1,
      format: 'png',
    })
    const fileName = `${String(sheet.index + 1).padStart(2, '0')}.png`
    const filePath = path.join(renderDir, fileName)
    await fs.writeFile(filePath, new Uint8Array(await preview.arrayBuffer()))
    renders.push({ sheet: sheet.name, path: filePath })
  }
  return { renderDir, renders }
}

function extractedText(entity, ...names) {
  for (const name of names) {
    const value = entity.fields[name]?.value
    if (value === null || value === undefined) continue
    if (value instanceof Date) return value.toISOString().slice(0, 10)
    const text = String(value).trim()
    if (text !== '') return text
  }
  return ''
}

function extractedNumber(entity, fallback, ...names) {
  const value = Number(extractedText(entity, ...names))
  return Number.isFinite(value) ? value : fallback
}

function columnName(index) {
  let value = index
  let name = ''
  while (value > 0) {
    const remainder = (value - 1) % 26
    name = String.fromCharCode(65 + remainder) + name
    value = Math.floor((value - 1) / 26)
  }
  return name
}

function pairingKey(entity) {
  return [
    entity.lineRef ?? extractedText(entity, 'line_ref'),
    extractedText(entity, 'upstream_model', '上游模型'),
    extractedText(entity, 'sku_ref', 'SKU代码'),
  ].join('\u0000')
}

function costVariant(sku) {
  return (
    extractedText(sku, 'resolution', '分辨率档位')
      .trim()
      .toLowerCase()
      .replaceAll(/[^a-z0-9._-]/g, '-') || 'default'
  )
}

function auditNote(entity) {
  const location = entity.sourceLocations[0]
  if (!location) return `原始业务ID=${entity.businessId}`
  return `原始业务ID=${entity.businessId}; 来源定位=${location.sheet}!${location.row}`
}

function compactReferenceLimits(entity) {
  const match = extractedText(entity, '备注').match(
    /素材限制=(\d{3})(?:$|[；;])/u
  )
  if (!match) {
    return null
  }
  const [images, videos, audios] = match[1].split('').map(Number)
  return { images, videos, audios }
}

function addStructuredSheet(workbook, name, headers, rows, tableName) {
  const sheet = workbook.worksheets.add(name)
  const lastColumn = columnName(headers.length)
  const lastRow = Math.max(rows.length + 4, 5)
  sheet.showGridLines = false
  sheet.mergeCells(`A1:${lastColumn}1`)
  sheet.getRange('A1').values = [[`渠道模型成本与利润配置模板 v2 - ${name}`]]
  sheet.getRange('A2').values = [
    [
      '结构化字段是发布依据；备注和公式仅用于审计与预览。所有新增渠道、线路和路由默认禁用。',
    ],
  ]
  sheet.getRange(`A4:${lastColumn}4`).values = [headers]
  if (rows.length > 0) {
    sheet.getRange(`A5:${lastColumn}${rows.length + 4}`).values = rows
  }
  sheet.getRange(`A1:${lastColumn}1`).format = {
    fill: '#183B56',
    font: { bold: true, color: '#FFFFFF', size: 14 },
    horizontalAlignment: 'left',
  }
  sheet.getRange(`A2:${lastColumn}2`).format = {
    font: { color: '#526173', italic: true },
    wrapText: true,
  }
  sheet.getRange(`A4:${lastColumn}4`).format = {
    fill: '#1E6A8A',
    font: { bold: true, color: '#FFFFFF' },
    horizontalAlignment: 'center',
    wrapText: true,
    borders: { preset: 'outside', style: 'thin', color: '#D7DEE8' },
  }
  sheet.getRange(`A5:${lastColumn}${lastRow}`).format = {
    borders: { preset: 'insideHorizontal', style: 'thin', color: '#E6ECF2' },
    verticalAlignment: 'center',
  }
  sheet.getRange(`A1:${lastColumn}${lastRow}`).format.columnWidth = 16
  for (const [index, header] of headers.entries()) {
    const column = columnName(index + 1)
    const range = sheet.getRange(`${column}1:${column}${lastRow}`)
    if (header === 'note') {
      range.format.columnWidth = 42
      sheet.getRange(`${column}5:${column}${lastRow}`).format.wrapText = true
    } else if (
      [
        'channel_ref',
        'client_model',
        'canonical_model',
        'cost_rule_ref',
        'line_ref',
        'mapping_ref',
        'route_target_ref',
        'sku_ref',
        'upstream_model',
      ].includes(header)
    ) {
      range.format.columnWidth = 26
    } else if (header === 'display_name') {
      range.format.columnWidth = 30
    }
  }
  sheet.getRange(`A1:${lastColumn}1`).format.rowHeight = 25
  sheet.getRange(`A2:${lastColumn}2`).format.rowHeight = 34
  sheet.getRange(`A4:${lastColumn}4`).format.rowHeight = 36
  sheet.getRange(`A5:A${lastRow}`).format.numberFormat = '@'
  sheet.freezePanes.freezeRows(4)
  if (rows.length > 0) {
    sheet.tables.add(`A4:${lastColumn}${rows.length + 4}`, true, tableName)
  }
  return sheet
}

function applyListValidation(sheet, column, rowCount, values) {
  if (rowCount === 0) return
  sheet.getRange(`${column}5:${column}${rowCount + 4}`).dataValidation = {
    rule: { type: 'list', values },
  }
}

function normalizedPairs(extracted) {
  const costsByKey = new Map()
  const mappingsByKey = new Map()
  for (const cost of extracted.costRuleDrafts) {
    if (!cost.lineRef) continue
    const values = costsByKey.get(pairingKey(cost)) ?? []
    values.push(cost)
    costsByKey.set(pairingKey(cost), values)
  }
  for (const mapping of extracted.modelMappings) {
    if (!mapping.lineRef) continue
    const values = mappingsByKey.get(pairingKey(mapping)) ?? []
    values.push(mapping)
    mappingsByKey.set(pairingKey(mapping), values)
  }
  const pairs = []
  for (const [key, costs] of costsByKey) {
    const mappings = mappingsByKey.get(key) ?? []
    costs.sort((left, right) => left.businessId.localeCompare(right.businessId))
    mappings.sort((left, right) =>
      left.businessId.localeCompare(right.businessId)
    )
    if (costs.length !== mappings.length) {
      throw new Error(
        `V2 source rows cannot pair costs and mappings for ${key}`
      )
    }
    for (const [index, cost] of costs.entries()) {
      if (!compactReferenceLimits(mappings[index])) continue
      pairs.push({ cost, mapping: mappings[index] })
    }
  }
  return pairs.sort((left, right) =>
    left.mapping.businessId.localeCompare(right.mapping.businessId)
  )
}

function buildStructuredV2Workbook(extracted) {
  const workbook = Workbook.create()
  const channelByRef = new Map(
    extracted.channels.map((channel) => [channel.businessId, channel])
  )
  const skuByRef = new Map(
    extracted.modelSkus.map((sku) => [sku.businessId, sku])
  )
  const pairs = normalizedPairs(extracted)

  const sourceRows = extracted.sources.map((source) => [
    source.businessId,
    extractedText(source, '来源名称', '项目'),
    extractedText(source, '引用位置').startsWith('http')
      ? extractedText(source, '引用位置')
      : '',
    extractedText(source, '访问日期', '截止日期'),
    auditNote(source),
  ])
  const channelRows = extracted.channels.map((channel) => [
    channel.businessId,
    extractedText(channel, '渠道名称'),
    extractedText(channel, '默认币种') || 'USD',
    extractedNumber(channel, 1, '充值兑换比例'),
    extractedNumber(channel, 0, '手续费率'),
    extractedNumber(channel, 1, '计费倍率'),
    'disabled',
    extractedText(channel, '来源ID'),
    auditNote(channel),
  ])
  const lineRows = extracted.channelLines.map((line) => {
    const channel = channelByRef.get(line.channelRef)
    const supportsRealPerson = line.supportsRealPerson ?? null
    return [
      line.businessId,
      line.channelRef,
      `${extractedText(channel, '渠道名称')} / ${line.businessId}`,
      line.channelRef.replace(/^CH-/, '').toLowerCase(),
      'global',
      'task',
      supportsRealPerson,
      'disabled',
      extractedText(channel, '来源ID'),
      '由 v1 已确认分组转换；连接凭据必须在导入向导中填写。',
    ]
  })
  const skuRows = extracted.modelSkus.map((sku) => [
    sku.businessId,
    extractedText(sku, '模型'),
    extractedText(sku, '分辨率档位'),
    extractedNumber(sku, 0, '输出宽'),
    extractedNumber(sku, 0, '输出高'),
    extractedNumber(sku, 0, '帧率'),
    extractedNumber(sku, 0, '最小时长秒'),
    extractedNumber(sku, 0, '最大时长秒'),
    extractedText(sku, '来源ID'),
  ])
  const saleRows = extracted.saleProposals.map((sale) => [
    sale.businessId,
    extractedText(sale, '客户端模型'),
    extractedText(sale, 'SKU代码'),
    extractedText(sale, '定价场景'),
    extractedText(sale, '计费模式'),
    extractedText(sale, '币种'),
    extractedText(sale, '原币/1M', '原币/基准秒'),
    'disabled',
    extractedText(sale, '来源ID'),
  ])
  const costRows = pairs.map(({ cost, mapping }) => {
    const sku = skuByRef.get(extractedText(cost, 'SKU代码'))
    const mode = extractedText(cost, '成本模式')
    let nativePriceField = '原币/1M'
    if (mode === 'per_request') {
      nativePriceField = '原币按次'
    } else if (mode === 'per_duration') {
      nativePriceField = '原币/秒'
    }
    const nativePrice = extractedText(cost, nativePriceField, '原币基础单价')
    return [
      `COST-V2/${mapping.businessId}`,
      cost.lineRef,
      extractedText(cost, '上游模型'),
      extractedText(cost, 'SKU代码'),
      extractedText(cost, '定价场景'),
      mode,
      costVariant(sku),
      `ROUTE-${mapping.businessId}`,
      extractedText(cost, '币种'),
      extractedNumber(cost, 1, '原币兑USD'),
      extractedNumber(cost, 1, '计费倍率'),
      extractedNumber(cost, 1, '采购折扣'),
      extractedNumber(cost, 1, '充值兑换比例'),
      extractedNumber(cost, 0, '手续费率'),
      Number(nativePrice) || 0,
      null,
      'disabled',
      extractedText(cost, '来源ID'),
      auditNote(cost),
    ]
  })
  const routeRows = pairs.map(({ mapping }) => {
    const sku = skuByRef.get(extractedText(mapping, 'SKU代码'))
    const line = extracted.channelLines.find(
      (candidate) => candidate.businessId === mapping.lineRef
    )
    const referenceLimits = compactReferenceLimits(mapping)
    if (!referenceLimits) {
      throw new Error(`${mapping.businessId} 缺少有效素材限制`)
    }
    return [
      `ROUTE-${mapping.businessId}`,
      extractedText(sku, '模型'),
      extractedText(mapping, '客户端模型'),
      'merge',
      mapping.lineRef,
      extractedText(mapping, '上游模型'),
      extractedText(mapping, 'SKU代码'),
      costVariant(sku),
      extractedText(sku, '分辨率档位'),
      extractedNumber(sku, 0, '最小时长秒'),
      extractedNumber(sku, 0, '最大时长秒'),
      0,
      0,
      0,
      referenceLimits.images,
      referenceLimits.videos,
      referenceLimits.audios,
      line?.supportsRealPerson ?? null,
      100,
      false,
    ]
  })
  const mappingRows = pairs.map(({ mapping }) => [
    mapping.businessId,
    extractedText(mapping, '客户端模型'),
    mapping.lineRef,
    extractedText(mapping, '上游模型'),
    extractedText(mapping, 'SKU代码'),
    `ROUTE-${mapping.businessId}`,
    extractedText(mapping, '来源ID'),
    auditNote(mapping),
  ])

  const channelSheet = addStructuredSheet(
    workbook,
    '渠道',
    V2_HEADERS.渠道,
    channelRows,
    'V2Channels'
  )
  const lineSheet = addStructuredSheet(
    workbook,
    '渠道线路',
    V2_HEADERS.渠道线路,
    lineRows,
    'V2ChannelLines'
  )
  addStructuredSheet(
    workbook,
    '模型SKU',
    V2_HEADERS.模型SKU,
    skuRows,
    'V2ModelSKUs'
  )
  const saleSheet = addStructuredSheet(
    workbook,
    '官方售价',
    V2_HEADERS.官方售价,
    saleRows,
    'V2SaleProposals'
  )
  const costSheet = addStructuredSheet(
    workbook,
    '渠道成本',
    V2_HEADERS.渠道成本,
    costRows,
    'V2CostRules'
  )
  const routeSheet = addStructuredSheet(
    workbook,
    '路由目标',
    V2_HEADERS.路由目标,
    routeRows,
    'V2RouteTargets'
  )
  addStructuredSheet(
    workbook,
    '模型映射',
    V2_HEADERS.模型映射,
    mappingRows,
    'V2ModelMappings'
  )
  addStructuredSheet(workbook, '来源', V2_HEADERS.来源, sourceRows, 'V2Sources')

  const costLastRow = costRows.length + 4
  costSheet.getRange(`P5:P${costLastRow}`).formulas = costRows.map(
    (_, index) => [
      `=IFERROR(O${index + 5}*J${index + 5}*K${index + 5}*L${index + 5}*M${index + 5}*(1+N${index + 5}),0)`,
    ]
  )
  costSheet.getRange(`J5:P${costLastRow}`).format.numberFormat = '0.000000'
  applyListValidation(channelSheet, 'G', channelRows.length, ['disabled'])
  applyListValidation(lineSheet, 'G', lineRows.length, ['true', 'false'])
  applyListValidation(lineSheet, 'H', lineRows.length, ['disabled'])
  applyListValidation(saleSheet, 'H', saleRows.length, ['disabled'])
  applyListValidation(costSheet, 'F', costRows.length, [
    'free',
    'per_request',
    'per_duration',
    'per_token',
  ])
  applyListValidation(costSheet, 'Q', costRows.length, ['disabled'])
  applyListValidation(routeSheet, 'D', routeRows.length, [
    'merge',
    'replace',
    'skip',
  ])
  applyListValidation(routeSheet, 'R', routeRows.length, ['true', 'false'])
  applyListValidation(routeSheet, 'T', routeRows.length, ['false'])

  const salesUSDBySKU = new Map(
    extracted.saleProposals.map((sale) => [
      extractedText(sale, 'SKU代码'),
      extractedNumber(sale, 0, 'USD/1M', 'USD/基准秒'),
    ])
  )
  const profitRows = pairs.map(({ mapping }) => [
    `COST-V2/${mapping.businessId}`,
    null,
    salesUSDBySKU.get(extractedText(mapping, 'SKU代码')) ?? 0,
    null,
    null,
  ])
  const profitSheet = addStructuredSheet(
    workbook,
    '利润测算',
    [
      '成本规则ID',
      '规范成本USD',
      '官方售价USD（参考）',
      '预估毛利润USD',
      '预估毛利率',
    ],
    profitRows,
    'V2ProfitPreview'
  )
  const profitLastRow = profitRows.length + 4
  profitSheet.getRange(`B5:B${profitLastRow}`).formulas = profitRows.map(
    (_, index) => [`='渠道成本'!P${index + 5}`]
  )
  profitSheet.getRange(`D5:D${profitLastRow}`).formulas = profitRows.map(
    (_, index) => [`=IFERROR(C${index + 5}-B${index + 5},0)`]
  )
  profitSheet.getRange(`E5:E${profitLastRow}`).formulas = profitRows.map(
    (_, index) => [`=IFERROR(D${index + 5}/C${index + 5},0)`]
  )
  profitSheet.getRange(`B5:D${profitLastRow}`).format.numberFormat = '0.000000'
  profitSheet.getRange(`E5:E${profitLastRow}`).format.numberFormat = '0.0%'

  const validationRows = [
    [
      '成本线路引用',
      null,
      null,
      `渠道成本!B5:B${costLastRow}`,
      '所有成本必须绑定结构化线路。',
    ],
    [
      '禁用路由目标',
      null,
      null,
      `路由目标!T5:T${routeRows.length + 4}`,
      '新路由目标必须保持禁用。',
    ],
    [
      '成本变体',
      null,
      null,
      `渠道成本!G5:G${costLastRow}`,
      '每条成本必须包含成本变体。',
    ],
  ]
  const validationSheet = addStructuredSheet(
    workbook,
    '校验',
    ['检查项', '错误数（公式）', '状态（公式）', '修复位置', '说明'],
    validationRows,
    'V2Checks'
  )
  validationSheet.getRange('B5:B7').formulas = [
    [`=COUNTBLANK('渠道成本'!$B$5:$B$${costLastRow})`],
    [`=COUNTIF('路由目标'!$T$5:$T$${routeRows.length + 4},TRUE)`],
    [`=COUNTBLANK('渠道成本'!$G$5:$G$${costLastRow})`],
  ]
  validationSheet.getRange('C5:C7').formulas = [
    ['=IF(B5=0,"PASS","FAIL")'],
    ['=IF(B6=0,"PASS","FAIL")'],
    ['=IF(B7=0,"PASS","FAIL")'],
  ]
  workbook.comments.setSelf({ displayName: 'User' })
  workbook.comments.addThread(
    { cell: costSheet.getRange('P4') },
    '由原币价格、汇率、倍率、折扣、充值比例和手续费公式计算；导入服务会重新计算。'
  )
  workbook.comments.addThread(
    { cell: profitSheet.getRange('E4') },
    '利润测算仅用于预览，不作为发布时的权威价格。'
  )
  workbook.comments.addThread(
    { cell: validationSheet.getRange('B4') },
    '错误数由公式生成；发布前应为零。'
  )

  return workbook
}

async function exportWorkbookBytes(workbook) {
  const directory = await fs.mkdtemp(path.join(os.tmpdir(), 'new-api-v2-xlsx-'))
  const filePath = path.join(directory, 'template.xlsx')
  const output = await SpreadsheetFile.exportXlsx(workbook)
  await output.save(filePath)
  const bytes = await fs.readFile(filePath)
  await fs.rm(directory, { recursive: true, force: true })
  return bytes
}

async function buildV2Template() {
  const extracted = extractWorkbook(
    await loadWorkbookSnapshot(await fs.readFile(fixturePath))
  )
  const firstWorkbook = buildStructuredV2Workbook(extracted)
  const secondWorkbook = buildStructuredV2Workbook(extracted)
  const firstBytes = canonicalizeXlsxBytes(
    await exportWorkbookBytes(firstWorkbook)
  )
  const secondBytes = canonicalizeXlsxBytes(
    await exportWorkbookBytes(secondWorkbook)
  )
  if (!firstBytes.equals(secondBytes)) {
    throw new Error(
      `Structured V2 template is not deterministic: ${differingXlsxEntries(firstBytes, secondBytes).join(', ')}`
    )
  }
  const formulaErrors = scanFormulaErrors(firstWorkbook)
  if (formulaErrors.length > 0) {
    throw new Error(
      `V2 workbook formula errors found: ${JSON.stringify(formulaErrors)}`
    )
  }
  const inspections = await Promise.all([
    firstWorkbook.inspect({
      kind: 'table',
      range: '渠道成本!A1:S12',
      include: 'values,formulas',
      tableMaxRows: 12,
      tableMaxCols: 20,
    }),
    firstWorkbook.inspect({
      kind: 'table',
      range: '路由目标!A1:T12',
      include: 'values,formulas',
      tableMaxRows: 12,
      tableMaxCols: 20,
    }),
    firstWorkbook.inspect({
      kind: 'match',
      searchTerm: '#REF!|#DIV/0!|#VALUE!|#NAME\\?|#N/A',
      options: { useRegex: true, maxResults: 100 },
      summary: 'V2 formula error scan',
    }),
  ])
  const { renderDir, renders } = await renderSheets(firstWorkbook)
  await fs.mkdir(path.dirname(v2TemplatePath), { recursive: true })
  await fs.mkdir(path.dirname(v2GoldenPath), { recursive: true })
  await fs.writeFile(v2TemplatePath, firstBytes)
  await fs.writeFile(v2GoldenPath, firstBytes)
  return {
    formulaErrors,
    inspection: inspections.map((result) => result.ndjson),
    renderDir,
    renders,
    sha256: crypto.createHash('sha256').update(firstBytes).digest('hex'),
  }
}

async function main() {
  const sourcePath = await findSourceFile()
  const sourceBefore = await sha256File(sourcePath)
  const sourceName = path.basename(sourcePath)
  const generatedName = sourceName.replace(/-[^-]+\.xlsx$/i, '-v1-修正版.xlsx')
  const generatedPath = path.join(outputDir, generatedName)

  const workbook = await SpreadsheetFile.importXlsx(
    await FileBlob.load(sourcePath)
  )
  const costSheet = workbook.worksheets.getItemAt(5)
  const mappingSheet = workbook.worksheets.getItemAt(6)
  const patchResult = patchCorrectedRows(costSheet, mappingSheet)

  const costInspection = await workbook.inspect({
    kind: 'region',
    sheetId: costSheet.name,
    range: 'A192:AB199',
    maxChars: 12000,
  })
  const mappingInspection = await workbook.inspect({
    kind: 'region',
    sheetId: mappingSheet.name,
    range: 'A107:L110',
    maxChars: 12000,
  })
  const formulaErrors = scanFormulaErrors(workbook)
  if (formulaErrors.length > 0) {
    throw new Error(
      `Workbook formula errors found: ${JSON.stringify(formulaErrors)}`
    )
  }

  await fs.mkdir(path.dirname(fixturePath), { recursive: true })
  const output = await SpreadsheetFile.exportXlsx(workbook)
  await output.save(fixturePath)
  const outputBytes = canonicalizeXlsxBytes(await fs.readFile(fixturePath))
  await fs.writeFile(fixturePath, outputBytes)
  if (process.env.SKIP_GENERATED_WORKBOOK_OUTPUT !== '1') {
    await fs.writeFile(generatedPath, outputBytes)
  }
  await fs.writeFile(
    expectedCountsPath,
    `${JSON.stringify(expectedCounts, null, 2)}\n`
  )
  const v1Bytes = await fs.readFile(fixturePath)
  const v1Document = await buildImportDocument({
    extracted: extractWorkbook(await loadWorkbookSnapshot(v1Bytes)),
    sourceBytes: v1Bytes,
    sourceFileName: path.basename(fixturePath),
  })
  if (v1Document.hasFailures) {
    throw new Error(
      `Corrected V1 fixture has conversion failures: ${JSON.stringify(v1Document.document.issues)}`
    )
  }
  await fs.mkdir(path.dirname(v1ImportFixturePath), { recursive: true })
  await fs.writeFile(
    v1ImportFixturePath,
    serializeImportDocument(v1Document.document),
    'utf8'
  )

  const sourceAfter = await sha256File(sourcePath)
  if (
    sourceBefore.sha256 !== sourceAfter.sha256 ||
    !sourceBefore.bytes.equals(sourceAfter.bytes)
  ) {
    throw new Error(
      `Source workbook changed: before=${sourceBefore.sha256}, after=${sourceAfter.sha256}`
    )
  }

  const v2Result = await buildV2Template()
  const { renderDir, renders } = await renderSheets(workbook)
  await fs.rm(`${fixturePath}.inspect.ndjson`, { force: true })
  const metadata = {
    source_sha256: sourceBefore.sha256,
    source_file: sourceName,
    generated_file: generatedName,
    fixture_file: path.relative(projectRoot, fixturePath),
    sheets_rendered: renders.length,
    render_dir: renderDir,
    patched_cost_rows: patchResult.patchedCostRows,
    patched_mapping_rows: patchResult.patchedMappingRows,
    source_unchanged: true,
    formula_errors: formulaErrors,
    v2: {
      template_file: path.relative(projectRoot, v2TemplatePath),
      golden_file: path.relative(projectRoot, v2GoldenPath),
      formula_errors: v2Result.formulaErrors,
      renders: v2Result.renders,
      render_dir: v2Result.renderDir,
      sha256: v2Result.sha256,
    },
    v1_import_fixture: path.relative(projectRoot, v1ImportFixturePath),
  }

  console.log(JSON.stringify(metadata, null, 2))
  console.log('COST_INSPECTION')
  console.log(costInspection.ndjson)
  console.log('MAPPING_INSPECTION')
  console.log(mappingInspection.ndjson)
  console.log('V2_INSPECTION')
  console.log(v2Result.inspection.join('\n'))
}

await main()
