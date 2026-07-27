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

import { FileBlob, SpreadsheetFile } from '@oai/artifact-tool'

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
  return contents.map((content) => {
    if (!content.name.endsWith('.xml') && !content.name.endsWith('.rels')) {
      return content
    }
    const text = content.data
      .toString('utf8')
      .replace(tokenPattern, (token) => replacements.get(token) ?? token)
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
  await fs.writeFile(generatedPath, outputBytes)
  await fs.writeFile(
    expectedCountsPath,
    `${JSON.stringify(expectedCounts, null, 2)}\n`
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
  }

  console.log(JSON.stringify(metadata, null, 2))
  console.log('COST_INSPECTION')
  console.log(costInspection.ndjson)
  console.log('MAPPING_INSPECTION')
  console.log(mappingInspection.ndjson)
}

await main()
