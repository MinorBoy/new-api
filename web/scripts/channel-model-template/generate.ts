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
import fs from 'node:fs/promises'
import path from 'node:path'

import { buildTemplateData } from './build'
import { parseRules } from './rules'
import { readSourceWorkbook } from './source'
import { writeTemplateWorkbook, type WriteTemplateResult } from './write'

type GeneratorOptions = {
  allowWarnings: boolean
  allowUnsupportedSheets: boolean
  basePath: string
  force: boolean
  outputPath: string
  reportPath: string
  rulesPath: string
  sourcePath: string
}

const VALUE_FLAGS = new Set([
  '--source',
  '--rules',
  '--base',
  '--output',
  '--report',
])
const BOOLEAN_FLAGS = new Set([
  '--allow-warnings',
  '--allow-unsupported-sheets',
  '--force',
])

function defaultReportPath(outputPath: string): string {
  const parsed = path.parse(outputPath)
  return path.join(parsed.dir, `${parsed.name}.report.json`)
}

function parseArguments(args: string[]): GeneratorOptions {
  const values = new Map<string, string>()
  const booleans = new Set<string>()
  for (let index = 0; index < args.length; index += 1) {
    const flag = args[index]
    if (VALUE_FLAGS.has(flag)) {
      const value = args[index + 1]
      if (!value || value.startsWith('--') || values.has(flag)) {
        throw new Error(`Invalid ${flag} argument`)
      }
      values.set(flag, value)
      index += 1
      continue
    }
    if (BOOLEAN_FLAGS.has(flag)) {
      if (booleans.has(flag)) throw new Error(`Duplicate ${flag} argument`)
      booleans.add(flag)
      continue
    }
    throw new Error(`Unknown argument: ${flag}`)
  }
  const sourcePath = values.get('--source')
  const rulesPath = values.get('--rules')
  const basePath = values.get('--base')
  const outputPath = values.get('--output')
  if (!sourcePath || !rulesPath || !basePath || !outputPath) {
    throw new Error('--source, --rules, --base, and --output are required')
  }
  if (path.extname(sourcePath).toLowerCase() !== '.xlsx') {
    throw new Error('--source must point to an .xlsx file')
  }
  if (path.extname(basePath).toLowerCase() !== '.xlsx') {
    throw new Error('--base must point to an .xlsx file')
  }
  if (path.extname(outputPath).toLowerCase() !== '.xlsx') {
    throw new Error('--output must end in .xlsx')
  }
  return {
    allowWarnings: booleans.has('--allow-warnings'),
    allowUnsupportedSheets: booleans.has('--allow-unsupported-sheets'),
    basePath,
    force: booleans.has('--force'),
    outputPath,
    reportPath: values.get('--report') ?? defaultReportPath(outputPath),
    rulesPath,
    sourcePath,
  }
}

async function outputExists(outputPath: string): Promise<boolean> {
  try {
    await fs.access(outputPath)
    return true
  } catch {
    return false
  }
}

export async function runGenerator(
  args: string[]
): Promise<WriteTemplateResult> {
  const options = parseArguments(args)
  if (!options.force && (await outputExists(options.outputPath))) {
    throw new Error(`Output already exists: ${options.outputPath}`)
  }
  const [source, rulesFile] = await Promise.all([
    readSourceWorkbook(options.sourcePath),
    fs.readFile(options.rulesPath, 'utf8'),
  ])
  const rules = parseRules(JSON.parse(rulesFile) as unknown)
  const data = buildTemplateData(source, rules)
  const scopedData = options.allowUnsupportedSheets
    ? {
        ...data,
        issues: data.issues.map((item) =>
          item.code === 'UNSUPPORTED_SOURCE_SHEET' && item.severity === 'FAIL'
            ? {
                ...item,
                severity: 'WARN' as const,
                suggestion:
                  '本次显式使用 SD-only 范围；H3 数据保留在源表，未生成或发布。',
              }
            : item
        ),
      }
    : data
  const hasWarnings = scopedData.issues.some(
    (item) => item.severity === 'WARN'
  )
  const writableData =
    hasWarnings && !options.allowWarnings
      ? {
          ...scopedData,
          issues: [
            ...scopedData.issues,
            {
              code: 'WARNINGS_REQUIRE_ACKNOWLEDGEMENT',
              severity: 'FAIL' as const,
              message:
                'Warnings require --allow-warnings before the template can be written.',
              suggestion:
                'Resolve the source or rule issue, or explicitly generate a draft workbook.',
            },
          ],
        }
      : scopedData
  return writeTemplateWorkbook({
    basePath: options.basePath,
    outputPath: options.outputPath,
    reportPath: options.reportPath,
    sourcePath: options.sourcePath,
    rulesPath: options.rulesPath,
    rules,
    data: writableData,
  })
}

if (import.meta.main) {
  try {
    const result = await runGenerator(process.argv.slice(2))
    console.log(
      JSON.stringify(
        {
          hasFailures: result.hasFailures,
          output: result.report.output,
          report: result.report,
        },
        null,
        2
      )
    )
    if (result.hasFailures) process.exitCode = 1
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error))
    process.exitCode = 1
  }
}
