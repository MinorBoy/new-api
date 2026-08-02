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
import test from 'node:test'
import { fileURLToPath } from 'node:url'

import { convertWorkbook } from '../conversion'
import { WorkbookPreflightError } from '../security'

const fixturePath = fileURLToPath(
  new URL('../__fixtures__/channel-config-v1-corrected.xlsx', import.meta.url)
)

test('converts the v1 fixture into a channel configuration import document', async () => {
  const fixtureBytes = await fs.readFile(fixturePath)
  const file = new File(
    [new Uint8Array(fixtureBytes).buffer],
    'channel-config-v1-corrected.xlsx',
    {
      type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
    }
  )

  const result = await convertWorkbook(file)

  assert.equal(result.document.kind, 'new-api.channel-config-import')
  assert.equal(result.document.template_version, '1')
  assert.equal(result.document.entities.channel_lines.length, 14)
})

test('rejects invalid local workbook content during preflight', async () => {
  const file = new File(
    [Uint8Array.from([0xd0, 0xcf, 0x11, 0xe0, 0xa1, 0xb1, 0x1a, 0xe1])],
    'channels.xlsx',
    {
      type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
    }
  )

  await assert.rejects(
    () => convertWorkbook(file),
    (error: unknown) => error instanceof WorkbookPreflightError
  )
})
