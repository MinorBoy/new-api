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
import path from 'node:path'

import {
  TextReader,
  TextWriter,
  Uint8ArrayReader,
  Uint8ArrayWriter,
  ZipReader,
  ZipWriter,
} from '@zip.js/zip.js'
import Decimal from 'decimal.js'
import ExcelJS from 'exceljs'

import { V1_HEADERS } from '../../src/channel-config-converter/workbook'
import type { Issue, Rules, TemplateData } from './types'

type CellValue = boolean | Date | number | string | null
type ManagedSheet = keyof typeof V1_HEADERS

export type ConversionReport = {
  converterVersion: string
  source: { path: string; sha256: string }
  rules: { path: string; sha256: string; version: string }
  base: { path: string; sha256: string }
  generatedAt: string
  counts: Record<string, number>
  issues: Issue[]
  output: { path: string; sha256: string }
}

export type WriteTemplateInput = {
  basePath: string
  outputPath: string
  reportPath: string
  sourcePath: string
  rulesPath: string
  rules: Rules
  data: TemplateData
}

export type WriteTemplateResult = {
  hasFailures: boolean
  report: ConversionReport
}

function normalizeArtifactToolXml(fileName: string, value: string): string {
  const normalized = value.replaceAll('<x:', '<').replaceAll('</x:', '</')
  if (fileName.startsWith('xl/worksheets/_rels/')) {
    return normalized.replaceAll(
      /<Relationship\b(?=[^>]*Type="[^"]*\/(?:comments|threadedComment)")[^>]*\/>/giu,
      ''
    )
  }
  if (!fileName.startsWith('xl/worksheets/')) return normalized
  const tablePartsStart = normalized.indexOf('<tableParts')
  const tablePartsEnd = normalized.indexOf('</tableParts>')
  if (tablePartsStart < 0 || tablePartsEnd < tablePartsStart) return normalized
  return (
    normalized.slice(0, tablePartsStart) + normalized.slice(tablePartsEnd + 13)
  )
}

async function prepareForExcelJs(input: Uint8Array): Promise<Uint8Array> {
  const reader = new ZipReader(new Uint8ArrayReader(input), {
    useWebWorkers: false,
  })
  const writer = new ZipWriter(new Uint8ArrayWriter(), {
    useWebWorkers: false,
  })
  try {
    for (const entry of await reader.getEntries()) {
      if (
        entry.directory ||
        entry.filename.startsWith('xl/tables/') ||
        entry.filename.startsWith('xl/comments') ||
        entry.filename.startsWith('xl/threadedcomments/') ||
        entry.filename.startsWith('xl/persons/')
      ) {
        continue
      }
      if (entry.filename.endsWith('.xml') || entry.filename.endsWith('.rels')) {
        const xml = await entry.getData(new TextWriter())
        await writer.add(
          entry.filename,
          new TextReader(normalizeArtifactToolXml(entry.filename, xml))
        )
      } else {
        await writer.add(
          entry.filename,
          new Uint8ArrayReader(await entry.getData(new Uint8ArrayWriter()))
        )
      }
    }
    return await writer.close()
  } finally {
    await reader.close()
  }
}

function cellText(value: unknown): string {
  if (value === null || value === undefined) return ''
  if (value instanceof Date) return value.toISOString()
  if (typeof value === 'object' && value !== null && 'text' in value) {
    const text = value.text
    return typeof text === 'string' ? text.trim() : ''
  }
  return String(value).trim()
}

function verifyBase(workbook: ExcelJS.Workbook): void {
  const sheetNames = Object.keys(V1_HEADERS)
  if (workbook.worksheets.length !== sheetNames.length) {
    throw new Error('V1 base worksheet count mismatch')
  }
  for (const name of sheetNames) {
    const sheet = workbook.getWorksheet(name)
    if (!sheet) throw new Error(`V1 base is missing sheet: ${name}`)
    const headers = V1_HEADERS[name as ManagedSheet]
    for (const [index, header] of headers.entries()) {
      if (cellText(sheet.getRow(4).getCell(index + 1).value) !== header) {
        throw new Error(`V1 base header mismatch: ${name}`)
      }
    }
  }
}

function clearDataRows(sheet: ExcelJS.Worksheet, columnCount: number): void {
  for (let row = 5; row <= sheet.rowCount; row += 1) {
    for (let column = 1; column <= columnCount; column += 1) {
      sheet.getRow(row).getCell(column).value = null
    }
  }
}

function writeRows(
  sheet: ExcelJS.Worksheet,
  headers: readonly string[],
  rows: CellValue[][]
): void {
  clearDataRows(sheet, headers.length)
  for (const [rowOffset, values] of rows.entries()) {
    const row = sheet.getRow(rowOffset + 5)
    for (const [columnOffset, value] of values.entries()) {
      row.getCell(columnOffset + 1).value = value
    }
  }
}

function channelRows(data: TemplateData): CellValue[][] {
  return data.channels.map((item) => [
    item.businessId,
    item.name,
    item.pricePage,
    item.currency,
    item.rechargeRatio,
    item.feeRate,
    item.billingMultiplier,
    '余额',
    item.status === 'active' ? '正常' : 'draft',
    null,
    item.strictCostValidation,
    new Date(),
    item.sourceId,
    item.note,
    null,
  ])
}

function skuRows(data: TemplateData): CellValue[][] {
  return data.skus.map((item) => [
    item.businessId,
    item.model,
    item.version,
    item.resolution,
    item.outputWidth,
    item.outputHeight,
    item.frameRate,
    item.minDurationSeconds,
    item.maxDurationSeconds,
    item.ratio,
    item.supportsVideoInput,
    item.supportsRealPerson,
    item.supportsSuperResolution,
    item.measurementMethod,
    item.status,
    item.sourceId,
    item.note,
    null,
  ])
}

function saleRows(data: TemplateData): CellValue[][] {
  return data.sales.map((item) => [
    item.businessId,
    item.clientModel,
    item.skuCode,
    item.scenario,
    item.billingMode,
    item.currency,
    item.nativePerMillion,
    item.outputWidth,
    item.outputHeight,
    item.frameRate,
    null,
    item.nativePerSecond,
    null,
    null,
    new Date(),
    null,
    item.status,
    item.sourceId,
    item.note,
    null,
  ])
}

function costRows(data: TemplateData): CellValue[][] {
  return data.costs.map((item) => [
    item.businessId,
    item.channelCode,
    item.upstreamModel,
    item.skuCode,
    item.scenario,
    item.mode,
    item.tokenSubMode,
    item.meterSource,
    item.tokenField,
    item.chargeEvent,
    item.currency,
    item.nativePerRequest,
    item.nativePerSecond,
    item.nativePerMillion,
    item.nativeBasePrice,
    item.billingMultiplier,
    item.purchaseDiscountRatio,
    item.rechargeRatio,
    item.feeRate,
    item.currencyToUsd,
    null,
    item.unit,
    new Date(),
    null,
    item.status,
    item.sourceId,
    item.note,
    null,
  ])
}

function mappingRows(data: TemplateData): CellValue[][] {
  return data.mappings.map((item) => [
    item.businessId,
    item.clientModel,
    item.channelCode,
    item.upstreamModel,
    item.skuCode,
    item.defaultScenario,
    item.enabled,
    new Date(),
    null,
    item.sourceId,
    item.note,
    null,
  ])
}

function profitRows(data: TemplateData): CellValue[][] {
  return data.profits.map((item) => [
    item.businessId,
    item.saleId,
    item.costId,
    item.groupRatio,
    item.inputVideoSeconds,
    item.outputVideoSeconds,
    item.skuCode,
    item.scenario,
    null,
    null,
    null,
    null,
    item.costMode,
    null,
    null,
    null,
    null,
    null,
    null,
    item.costStatus,
    item.note,
  ])
}

function sourceRows(data: TemplateData): CellValue[][] {
  return data.sources.map((item) => [
    item.businessId,
    item.project,
    item.valueOrRange,
    item.unit,
    item.asOf,
    item.sourceType,
    item.sourceName,
    item.reference,
    item.owner,
    item.note,
    item.accessedAt,
    'OK',
  ])
}

function issueRows(issues: Issue[]): CellValue[][] {
  const failures = issues.filter((item) => item.severity === 'FAIL').length
  const warnings = issues.filter((item) => item.severity === 'WARN').length
  return [
    [
      '转换FAIL',
      failures,
      failures === 0 ? 'PASS' : 'FAIL',
      '源表或规则文件',
      'FAIL 会阻止正式输出。',
    ],
    [
      '转换WARN',
      warnings,
      warnings === 0 ? 'PASS' : 'WARN',
      '输出草稿行',
      'WARN 对应实体保持 draft。',
    ],
    ['公式引用', 0, 'PASS', '全工作簿', '打开 Excel 时执行完整重算。'],
  ]
}

function setFormula(
  sheet: ExcelJS.Worksheet,
  address: string,
  formula: string
): void {
  sheet.getCell(address).value = { formula }
}

function writeFormulas(workbook: ExcelJS.Workbook, data: TemplateData): void {
  const channels = workbook.getWorksheet('渠道')
  const skus = workbook.getWorksheet('模型SKU')
  const sales = workbook.getWorksheet('官方售价')
  const costs = workbook.getWorksheet('渠道成本')
  const mappings = workbook.getWorksheet('模型映射')
  const profits = workbook.getWorksheet('利润测算')
  if (!channels || !skus || !sales || !costs || !mappings || !profits) {
    throw new Error('V1 formula sheets are unavailable')
  }
  for (let row = 5; row < data.channels.length + 5; row += 1) {
    setFormula(
      channels,
      `O${row}`,
      `IF(A${row}="","",IF(COUNTIF($A$5:$A$504,A${row})>1,"错误:重复渠道代码",IF(OR(B${row}="",C${row}="",D${row}="",E${row}<=0,F${row}<0,G${row}<=0,H${row}="",I${row}=""),"错误:必填项","OK")))`
    )
  }
  for (let row = 5; row < data.skus.length + 5; row += 1) {
    setFormula(
      skus,
      `R${row}`,
      `IF(A${row}="","",IF(COUNTIF($A$5:$A$504,A${row})>1,"错误:重复SKU代码",IF(OR(B${row}="",C${row}="",D${row}="",E${row}<=0,F${row}<=0,G${row}<=0,H${row}<=0,I${row}<H${row},N${row}="",O${row}=""),"错误:必填项","OK")))`
    )
  }
  for (let row = 5; row < data.sales.length + 5; row += 1) {
    setFormula(
      sales,
      `K${row}`,
      `IFERROR(H${row}*I${row}*J${row}/'参数'!$B$7,"")`
    )
    setFormula(sales, `M${row}`, `IFERROR(G${row}*'参数'!$B$6,"")`)
    setFormula(sales, `N${row}`, `IFERROR(L${row}*'参数'!$B$6,"")`)
    setFormula(
      sales,
      `T${row}`,
      `IF(A${row}="","",IF(COUNTIF($A$5:$A$504,A${row})>1,"错误:重复售价ID",IF(OR(B${row}="",C${row}="",D${row}="",E${row}="",F${row}="",G${row}<=0,Q${row}="",R${row}=""),"错误:必填项","OK")))`
    )
  }
  for (let row = 5; row < data.costs.length + 5; row += 1) {
    setFormula(
      costs,
      `U${row}`,
      `IFERROR(O${row}*P${row}*Q${row}/R${row}*(1+S${row})*T${row},"")`
    )
    setFormula(
      costs,
      `AB${row}`,
      `IF(A${row}="","",IF(COUNTIF($A$5:$A$1004,A${row})>1,"错误:重复成本规则ID",IF(OR(B${row}="",C${row}="",D${row}="",E${row}="",F${row}="",K${row}="",O${row}<=0,P${row}<=0,Q${row}<=0,R${row}<=0,S${row}<0,T${row}<=0,Y${row}="",Z${row}=""),"错误:必填项","OK")))`
    )
  }
  for (let row = 5; row < data.mappings.length + 5; row += 1) {
    setFormula(
      mappings,
      `L${row}`,
      `IF(A${row}="","",IF(COUNTIF($A$5:$A$1004,A${row})>1,"错误:重复映射ID",IF(OR(B${row}="",C${row}="",D${row}="",E${row}="",F${row}="",G${row}="",J${row}=""),"错误:必填项","OK")))`
    )
  }
  for (let row = 5; row < data.profits.length + 5; row += 1) {
    setFormula(
      profits,
      `I${row}`,
      `IFERROR(XLOOKUP(G${row},'模型SKU'!$A$5:$A$504,'模型SKU'!$E$5:$E$504),"")`
    )
    setFormula(
      profits,
      `J${row}`,
      `IFERROR(XLOOKUP(G${row},'模型SKU'!$A$5:$A$504,'模型SKU'!$F$5:$F$504),"")`
    )
    setFormula(
      profits,
      `K${row}`,
      `IFERROR(XLOOKUP(G${row},'模型SKU'!$A$5:$A$504,'模型SKU'!$G$5:$G$504),"")`
    )
    setFormula(
      profits,
      `L${row}`,
      `IFERROR((E${row}+F${row})*I${row}*J${row}*K${row}/'参数'!$B$7,0)`
    )
    setFormula(
      profits,
      `N${row}`,
      `IFERROR(XLOOKUP(C${row},'渠道成本'!$A$5:$A$1004,'渠道成本'!$U$5:$U$1004),"")`
    )
    setFormula(
      profits,
      `O${row}`,
      `IFERROR(XLOOKUP(B${row},'官方售价'!$A$5:$A$504,'官方售价'!$M$5:$M$504),"")`
    )
    setFormula(
      profits,
      `P${row}`,
      `IF(M${row}="per_token",N${row}*L${row}/'参数'!$B$8,N${row})`
    )
    setFormula(profits, `Q${row}`, `IFERROR(O${row}*D${row},"")`)
    setFormula(profits, `R${row}`, `IFERROR(Q${row}-P${row},"")`)
    setFormula(profits, `S${row}`, `IFERROR(R${row}/Q${row},"")`)
  }
}

function staticFormulaIssues(workbook: ExcelJS.Workbook): Issue[] {
  const issues: Issue[] = []
  for (const sheet of workbook.worksheets) {
    sheet.eachRow((row) => {
      row.eachCell((cell) => {
        const value = cell.value
        if (typeof value === 'object' && value !== null && 'formula' in value) {
          const formula = value.formula
          if (typeof formula === 'string' && formula.includes('#REF!')) {
            issues.push({
              code: 'FORMULA_REFERENCE_INVALID',
              severity: 'FAIL',
              message: `Broken formula reference in ${sheet.name}!${cell.address}.`,
              sheet: sheet.name,
              field: cell.address,
            })
          }
        }
      })
    })
  }
  return issues
}

async function hashPath(filePath: string): Promise<string> {
  try {
    return crypto
      .createHash('sha256')
      .update(await fs.readFile(filePath))
      .digest('hex')
  } catch {
    return 'unavailable'
  }
}

function counts(data: TemplateData): Record<string, number> {
  return {
    channels: data.channels.length,
    modelSkus: data.skus.length,
    saleProposals: data.sales.length,
    costRules: data.costs.length,
    modelMappings: data.mappings.length,
    profitScenarios: data.profits.length,
    sources: data.sources.length,
  }
}

export async function writeTemplateWorkbook(
  input: WriteTemplateInput
): Promise<WriteTemplateResult> {
  const baseBytes = await fs.readFile(input.basePath)
  const workbook = new ExcelJS.Workbook()
  await workbook.xlsx.load(await prepareForExcelJs(baseBytes))
  verifyBase(workbook)
  const issues = [...input.data.issues]
  const hasInputFailures = issues.some((item) => item.severity === 'FAIL')
  const generatedAt = new Date().toISOString()
  const report: ConversionReport = {
    converterVersion: `channel-model-template/${input.rules.version}`,
    source: {
      path: input.sourcePath,
      sha256: await hashPath(input.sourcePath),
    },
    rules: {
      path: input.rulesPath,
      sha256: await hashPath(input.rulesPath),
      version: input.rules.version,
    },
    base: { path: input.basePath, sha256: await hashPath(input.basePath) },
    generatedAt,
    counts: counts(input.data),
    issues,
    output: { path: input.outputPath, sha256: 'not-written' },
  }
  if (hasInputFailures) {
    await fs.mkdir(path.dirname(input.reportPath), { recursive: true })
    await fs.writeFile(input.reportPath, `${JSON.stringify(report, null, 2)}\n`)
    return { hasFailures: true, report }
  }

  const parameters = workbook.getWorksheet('参数')
  const instructions = workbook.getWorksheet('使用说明')
  const checks = workbook.getWorksheet('校验')
  if (!parameters || !instructions || !checks) {
    throw new Error('V1 output sheets are unavailable')
  }
  parameters.getCell('B5').value = Number(
    new Decimal(input.rules.defaults.currencyToUsd).pow(-1).toFixed()
  )
  parameters.getCell('B6').value = { formula: '=1/B5' }
  parameters.getCell('B7').value = input.rules.defaults.tokenDivisor
  parameters.getCell('B9').value = Number(input.rules.defaults.groupRatio)
  parameters.getCell('B10').value = `v1-generator-${input.rules.version}`
  parameters.getCell('B11').value = new Date(generatedAt)
  instructions.getCell('B30').value = input.data.costs.length / 2
  instructions.getCell('B31').value = input.data.channels.length
  instructions.getCell('B32').value = input.data.skus.length
  instructions.getCell('B33').value = input.data.sales.length
  instructions.getCell('B34').value = input.data.costs.length
  instructions.getCell('B35').value = input.data.mappings.length
  instructions.getCell('B36').value = input.data.profits.length

  writeRows(
    workbook.getWorksheet('渠道')!,
    V1_HEADERS.渠道,
    channelRows(input.data)
  )
  writeRows(
    workbook.getWorksheet('模型SKU')!,
    V1_HEADERS.模型SKU,
    skuRows(input.data)
  )
  writeRows(
    workbook.getWorksheet('官方售价')!,
    V1_HEADERS.官方售价,
    saleRows(input.data)
  )
  writeRows(
    workbook.getWorksheet('渠道成本')!,
    V1_HEADERS.渠道成本,
    costRows(input.data)
  )
  writeRows(
    workbook.getWorksheet('模型映射')!,
    V1_HEADERS.模型映射,
    mappingRows(input.data)
  )
  writeRows(
    workbook.getWorksheet('利润测算')!,
    V1_HEADERS.利润测算,
    profitRows(input.data)
  )
  writeRows(
    workbook.getWorksheet('来源')!,
    V1_HEADERS.来源,
    sourceRows(input.data)
  )
  writeRows(checks, V1_HEADERS.校验, issueRows(issues))
  writeFormulas(workbook, input.data)
  workbook.calcProperties.fullCalcOnLoad = true
  workbook.calcProperties.forceFullCalc = true

  issues.push(...staticFormulaIssues(workbook))
  report.issues = issues
  if (issues.some((item) => item.severity === 'FAIL')) {
    await fs.mkdir(path.dirname(input.reportPath), { recursive: true })
    await fs.writeFile(input.reportPath, `${JSON.stringify(report, null, 2)}\n`)
    return { hasFailures: true, report }
  }

  await fs.mkdir(path.dirname(input.outputPath), { recursive: true })
  await workbook.xlsx.writeFile(input.outputPath)
  report.output.sha256 = await hashPath(input.outputPath)
  await fs.mkdir(path.dirname(input.reportPath), { recursive: true })
  await fs.writeFile(input.reportPath, `${JSON.stringify(report, null, 2)}\n`)
  return { hasFailures: false, report }
}
