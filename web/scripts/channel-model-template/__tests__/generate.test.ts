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
import assert from 'node:assert/strict'
import fs from 'node:fs/promises'
import os from 'node:os'
import path from 'node:path'
import test from 'node:test'
import { fileURLToPath } from 'node:url'

import { convertWorkbook } from '../../../src/channel-config-converter/conversion'
import { runGenerator } from '../generate'

const sourcePath = fileURLToPath(
  new URL('../__fixtures__/sd-source-v1.xlsx', import.meta.url)
)
const rulesPath = fileURLToPath(
  new URL('../conversion-rules.json', import.meta.url)
)
const basePath = fileURLToPath(
  new URL(
    '../../../src/channel-config-converter/__fixtures__/channel-config-v1-corrected.xlsx',
    import.meta.url
  )
)

test('refuses to overwrite an existing workbook without --force', async () => {
  const directory = await fs.mkdtemp(
    path.join(os.tmpdir(), 'channel-template-generator-')
  )
  const outputPath = path.join(directory, 'template.xlsx')
  try {
    await fs.writeFile(outputPath, 'existing workbook')
    await assert.rejects(
      () =>
        runGenerator([
          '--source',
          sourcePath,
          '--rules',
          rulesPath,
          '--base',
          basePath,
          '--output',
          outputPath,
          '--allow-warnings',
        ]),
      /Output already exists/
    )
  } finally {
    await fs.rm(directory, { recursive: true, force: true })
  }
})

test('writes a workbook and report when warnings are explicitly allowed', async () => {
  const directory = await fs.mkdtemp(
    path.join(os.tmpdir(), 'channel-template-generator-')
  )
  const outputPath = path.join(directory, 'template.xlsx')
  const reportPath = path.join(directory, 'template.report.json')
  try {
    const result = await runGenerator([
      '--source',
      sourcePath,
      '--rules',
      rulesPath,
      '--base',
      basePath,
      '--output',
      outputPath,
      '--report',
      reportPath,
      '--allow-warnings',
    ])

    assert.equal(result.hasFailures, false)
    assert.ok(result.report.issues.some((item) => item.severity === 'WARN'))
    const bytes = await fs.readFile(outputPath)
    const converted = await convertWorkbook(
      new File([bytes], 'template.xlsx', {
        type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
      })
    )

    assert.equal(converted.document.template_version, '1')
    await fs.access(reportPath)
  } finally {
    await fs.rm(directory, { recursive: true, force: true })
  }
})
