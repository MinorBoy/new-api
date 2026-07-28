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

import { V1WorkbookAdapter } from '../adapters/v1'
import { V2WorkbookAdapter } from '../adapters/v2'
import { buildImportDocument, type ConfigImportDocument } from '../document'
import { buildScopedImportDocument, groupChannelLines } from '../scope'
import { loadWorkbookSnapshot } from '../workbook'

const fixturePath = fileURLToPath(
  new URL('../__fixtures__/channel-config-v1-corrected.xlsx', import.meta.url)
)
const v2FixturePath = fileURLToPath(
  new URL('../__fixtures__/channel-config-v2-golden.xlsx', import.meta.url)
)

async function loadDocument(): Promise<ConfigImportDocument> {
  const bytes = await fs.readFile(fixturePath)
  const snapshot = await loadWorkbookSnapshot(bytes)
  const result = await buildImportDocument({
    extracted: new V1WorkbookAdapter().extract(snapshot),
    sourceBytes: bytes,
    sourceFileName: 'channel-config-v1-corrected.xlsx',
  })
  return result.document
}

async function loadV2Document(): Promise<ConfigImportDocument> {
  const bytes = await fs.readFile(v2FixturePath)
  const snapshot = await loadWorkbookSnapshot(bytes)
  const result = await buildImportDocument({
    extracted: new V2WorkbookAdapter().extract(snapshot),
    sourceBytes: bytes,
    sourceFileName: 'channel-config-v2-golden.xlsx',
  })
  return result.document
}

function entityCounts(
  document: ConfigImportDocument
): ConfigImportDocument['manifest']['counts'] {
  return Object.fromEntries(
    Object.entries(document.entities).map(([name, entities]) => [
      name,
      entities.length,
    ])
  ) as ConfigImportDocument['manifest']['counts']
}

test('groups channel lines by their parent channel', async () => {
  const groups = groupChannelLines(await loadDocument())
  const secure = groups.find(
    (group) => group.channel.business_id === 'CH-SECURE'
  )

  assert.ok(secure)
  assert.deepEqual(
    secure.lines.map((line) => line.line_ref),
    ['secure-discount', 'secure-enterprise', 'secure-overseas']
  )
})

test('selecting secure-enterprise retains only its dependency closure', async () => {
  const scoped = await buildScopedImportDocument(
    await loadDocument(),
    new Set(['secure-enterprise'])
  )

  assert.equal(scoped.canUse, true)
  assert.equal(scoped.selectedGroupCount, 1)
  assert.equal(scoped.selectedLineCount, 1)
  assert.deepEqual(
    scoped.document.entities.channel_lines.map((line) => line.line_ref),
    ['secure-enterprise']
  )
  assert.deepEqual(
    scoped.document.entities.channels.map((channel) => channel.business_id),
    ['CH-SECURE']
  )
  assert.ok(
    scoped.document.entities.cost_rule_drafts.every(
      (draft) => draft.line_ref === 'secure-enterprise'
    )
  )
  assert.ok(
    scoped.document.entities.model_mappings.every(
      (mapping) => mapping.line_ref === 'secure-enterprise'
    )
  )
  assert.ok(
    scoped.document.entities.route_blueprints.every((route) =>
      (route.targets as Array<{ line_ref: string }>).every(
        (target) => target.line_ref === 'secure-enterprise'
      )
    )
  )
})

test('unselected MEGABYAI ambiguity neither appears nor blocks selected Secure scope', async () => {
  const scoped = await buildScopedImportDocument(
    await loadDocument(),
    new Set(['secure-enterprise'])
  )

  assert.equal(scoped.document.entities.unresolved_variants.length, 0)
  assert.equal(
    scoped.document.issues.some(
      (issue) => issue.code === 'COST_VARIANT_AMBIGUOUS'
    ),
    false
  )
  assert.deepEqual(scoped.blockingIssues, [])
})

test('rebuilds manifest counts and payload hashes deterministically', async () => {
  const document = await loadDocument()
  const originalHash = document.manifest.payload_sha256
  const once = await buildScopedImportDocument(
    document,
    new Set(['secure-enterprise'])
  )
  const twice = await buildScopedImportDocument(
    document,
    new Set(['secure-enterprise'])
  )

  assert.deepEqual(once.document.manifest.counts, entityCounts(once.document))
  assert.match(once.document.manifest.payload_sha256, /^[a-f0-9]{64}$/)
  assert.equal(
    once.document.manifest.payload_sha256,
    twice.document.manifest.payload_sha256
  )
  assert.equal(document.manifest.payload_sha256, originalHash)
  assert.deepEqual(once.validationErrors, [])
})

test('rejects empty and unknown selections', async () => {
  const document = await loadDocument()
  const empty = await buildScopedImportDocument(document, new Set())
  const unknown = await buildScopedImportDocument(
    document,
    new Set(['missing-line'])
  )

  assert.deepEqual(empty.validationErrors, ['EMPTY_SELECTION'])
  assert.equal(empty.canUse, false)
  assert.deepEqual(unknown.validationErrors, ['UNKNOWN_LINE_REF'])
  assert.equal(unknown.canUse, false)
})

test('keeps V2 references inside the selected line scope', async () => {
  const document = await loadV2Document()
  const selectedLineRef = document.entities.channel_lines[0].line_ref
  const scoped = await buildScopedImportDocument(
    document,
    new Set([String(selectedLineRef)])
  )

  assert.equal(scoped.canUse, true)
  assert.deepEqual(scoped.validationErrors, [])
  assert.ok(
    scoped.document.entities.cost_rule_drafts.every(
      (cost) => cost.line_ref === String(selectedLineRef)
    )
  )
  assert.ok(
    scoped.document.entities.model_mappings.every(
      (mapping) => mapping.line_ref === String(selectedLineRef)
    )
  )
  assert.ok(
    scoped.document.entities.route_blueprints.every((route) =>
      (route.targets as Array<{ line_ref: string }>).every(
        (target) => target.line_ref === String(selectedLineRef)
      )
    )
  )
})
