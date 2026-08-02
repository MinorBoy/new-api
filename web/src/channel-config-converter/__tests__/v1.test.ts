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

import { extractWorkbook, V1WorkbookAdapter } from '../adapters/v1'
import { buildImportDocument } from '../document'
import { WorkbookContractError } from '../schema'
import type { WorkbookSnapshot } from '../types'
import { V1_HEADERS, loadWorkbookSnapshot } from '../workbook'

const fixturePath = fileURLToPath(
  new URL('../__fixtures__/channel-config-v1-corrected.xlsx', import.meta.url)
)

const expectedCounts = {
  channels: 9,
  channelLines: 12,
  modelSkus: 9,
  saleProposals: 16,
  costRuleDrafts: 121,
  modelMappings: 121,
  detectedConflictKeys: 17,
  automaticConflictKeys: 16,
  manualConflictKeys: 1,
} as const

async function loadFixture(): Promise<WorkbookSnapshot> {
  return loadWorkbookSnapshot(await fs.readFile(fixturePath))
}

function cloneSnapshot(snapshot: WorkbookSnapshot): WorkbookSnapshot {
  return structuredClone(snapshot)
}

function expectContractError(callback: () => unknown, code: string): void {
  assert.throws(callback, (error: unknown) => {
    assert.ok(error instanceof WorkbookContractError)
    assert.equal(error.code, code)
    return true
  })
}

test('v1 snapshot preserves the ten fixed sheets, row-four headers, and formula cache', async () => {
  const snapshot = await loadFixture()

  assert.deepEqual(
    snapshot.sheets.map((sheet) => sheet.name),
    Object.keys(V1_HEADERS)
  )
  for (const [sheetName, expectedHeaders] of Object.entries(V1_HEADERS)) {
    const sheet = snapshot.sheets.find(
      (candidate) => candidate.name === sheetName
    )
    assert.ok(sheet)
    assert.deepEqual(
      sheet.rows[3].cells.map((cell) => cell.value),
      expectedHeaders
    )
  }

  const formulaCell = snapshot.sheets
    .find((sheet) => sheet.name === '渠道成本')
    ?.rows.flatMap((row) => row.cells)
    .find((cell) => cell.formula !== null)
  assert.ok(formulaCell)
  assert.equal(typeof formulaCell.formula, 'string')
  assert.equal(typeof formulaCell.formulaResult, 'number')
})

test('v1 adapter extracts the corrected fixture baseline and stable line contracts', async () => {
  const extracted = extractWorkbook(await loadFixture())

  assert.equal(extracted.templateVersion, '1')
  assert.equal(extracted.channels.length, expectedCounts.channels)
  assert.equal(extracted.channelLines.length, expectedCounts.channelLines)
  assert.equal(extracted.modelSkus.length, expectedCounts.modelSkus)
  assert.equal(extracted.saleProposals.length, expectedCounts.saleProposals)
  assert.equal(extracted.costRuleDrafts.length, expectedCounts.costRuleDrafts)
  assert.equal(extracted.modelMappings.length, expectedCounts.modelMappings)
  assert.equal(
    extracted.compatibilityKeys.length,
    expectedCounts.detectedConflictKeys
  )
  assert.equal(
    extracted.compatibilityKeys.filter((key) => key.automatic).length,
    expectedCounts.automaticConflictKeys
  )
  assert.equal(
    extracted.unresolvedVariants.length,
    expectedCounts.manualConflictKeys
  )

  assert.deepEqual(
    extracted.channelLines
      .filter((line) => line.channelRef === 'CH-SECURE')
      .map((line) => line.businessId)
      .sort(),
    ['secure-discount', 'secure-enterprise', 'secure-overseas']
  )
  assert.deepEqual(
    extracted.channelLines
      .filter((line) => line.channelRef === 'CH-MEGABYAI')
      .map((line) => line.businessId)
      .sort(),
    ['megabyai-fast-no-real-person', 'megabyai-fast-real-person']
  )

  const mergedScenarioCost = extracted.costRuleDrafts.find(
    (cost) => cost.businessId === 'COST-MEGABYAI-R102-480-REQ'
  )
  assert.ok(mergedScenarioCost)
  assert.equal(mergedScenarioCost.sourceLocations.length, 2)
  assert.equal(mergedScenarioCost.lineRef, 'megabyai-fast-real-person')
})

test('v1 document decodes compact reference limits into route target constraints', async () => {
  const result = await buildImportDocument({
    extracted: extractWorkbook(await loadFixture()),
    sourceBytes: await fs.readFile(fixturePath),
    sourceFileName: 'channel-config-v1-corrected.xlsx',
  })

  const targets = new Map(
    result.document.entities.route_blueprints.map((blueprint) => [
      blueprint.business_id,
      (blueprint.targets as Array<Record<string, unknown>>)[0],
    ])
  )
  const dimensioTarget = targets.get('route-blueprint/MAP-DIMENSIO-R83-720')
  const clmmTarget = targets.get('route-blueprint/MAP-CLMM-R7-480')
  const paipuTarget = targets.get('route-blueprint/MAP-PAIPU-R14-720')
  assert.ok(dimensioTarget)
  assert.ok(clmmTarget)
  assert.ok(paipuTarget)
  assert.deepEqual(dimensioTarget.reference_limits, {
    images: 9,
    videos: 3,
    audios: 3,
  })
  assert.deepEqual(dimensioTarget.reference_minimums, {
    images: 0,
    videos: 0,
    audios: 0,
  })
  assert.deepEqual(clmmTarget.reference_limits, {
    images: 4,
    videos: 3,
    audios: 1,
  })
  assert.deepEqual(paipuTarget.reference_limits, {
    images: 9,
    videos: 0,
    audios: 0,
  })
  assert.equal(paipuTarget.upstream_model, 'lec-seedance-2-0')

  const paipuCost = result.document.entities.cost_rule_drafts.find(
    (cost) => cost.business_id === 'COST-PAIPU-R14-720-REQ'
  )
  assert.ok(paipuCost)
  assert.equal(paipuCost.upstream_model, 'lec-seedance-2-0')
  assert.equal(paipuCost.cost_mode, 'per_request')
})

test('v1 document rejects an unresolved legacy reference limit', async () => {
  const snapshot = await loadFixture()
  const mappingSheet = snapshot.sheets.find(
    (sheet) => sheet.name === '模型映射'
  )
  assert.ok(mappingSheet)
  const mappingRow = mappingSheet.rows.find((row) => row.rowNumber === 59)
  assert.ok(mappingRow)
  const noteColumn = V1_HEADERS.模型映射.indexOf('备注')
  mappingRow.cells[noteColumn].value = '源行=sd!60；时长=4-15'
  const result = await buildImportDocument({
    extracted: extractWorkbook(snapshot),
    sourceBytes: await fs.readFile(fixturePath),
    sourceFileName: 'channel-config-v1-corrected.xlsx',
  })

  assert.equal(result.hasFailures, true)
  assert.equal(
    result.document.entities.route_blueprints.some(
      (blueprint) =>
        blueprint.business_id === 'route-blueprint/MAP-8YES-R60-480'
    ),
    false
  )
  assert.equal(
    result.document.entities.cost_rule_drafts.some(
      (draft) => draft.route_target_ref === 'route-target/MAP-8YES-R60-480'
    ),
    false
  )
  assert.equal(
    result.document.entities.model_mappings.some(
      (mapping) => mapping.business_id === 'MAP-8YES-R60-480'
    ),
    true
  )
  assert.ok(
    result.document.issues.some(
      (issue) =>
        issue.code === 'ROUTE_REFERENCE_LIMITS_UNRESOLVED' &&
        issue.severity === 'error' &&
        issue.entity_ref === 'MAP-8YES-R60-480'
    )
  )
})

test('v1 import document uses the reserved YSR channel type IDs', async () => {
  const sourceBytes = await fs.readFile(fixturePath)
  const result = await buildImportDocument({
    extracted: extractWorkbook(await loadFixture()),
    sourceBytes,
    sourceFileName: 'channel-config-v1-corrected.xlsx',
  })
  const typesByChannel = new Map(
    result.document.entities.channels.map((channel) => [
      channel.business_id,
      channel['channel_type'],
    ])
  )

  assert.equal(typesByChannel.get('CH-DIMENSIO'), 200)
  assert.equal(typesByChannel.get('CH-MEGABYAI'), 204)
  assert.equal(typesByChannel.get('CH-SECURE'), 207)
  assert.equal(typesByChannel.get('CH-4STOKEN'), 209)
  assert.equal(typesByChannel.get('CH-8YES'), 210)
})

test('v1 adapter preserves the one manual MegaByAI conflict and removes Secure unsupported 480p rows', async () => {
  const extracted = extractWorkbook(await loadFixture())

  assert.deepEqual(
    extracted.unresolvedVariants.map((variant) => variant.businessId),
    ['CH-MEGABYAI/videos-standard']
  )

  for (const entity of [
    ...extracted.costRuleDrafts,
    ...extracted.modelMappings,
  ]) {
    const upstreamModel = entity.fields['上游模型']?.value
    const sku = entity.fields['SKU代码']?.value
    assert.notDeepEqual(
      [upstreamModel, sku],
      ['video-2.0-fast', 'SKU-SD20-FAST-480']
    )
    assert.notDeepEqual(
      [upstreamModel, sku],
      ['video-2.0-mini', 'SKU-SD20-MINI-480']
    )
  }
})

test('v1 adapter rejects missing sheets, changed headers, duplicate IDs, and broken references', async () => {
  const fixture = await loadFixture()

  const missingSheet = cloneSnapshot(fixture)
  missingSheet.sheets = missingSheet.sheets.filter(
    (sheet) => sheet.name !== '来源'
  )
  expectContractError(() => extractWorkbook(missingSheet), 'MISSING_SHEET')

  const changedHeader = cloneSnapshot(fixture)
  const costSheet = changedHeader.sheets.find(
    (sheet) => sheet.name === '渠道成本'
  )
  assert.ok(costSheet)
  costSheet.rows[3].cells[0].value = '错误成本ID'
  expectContractError(() => extractWorkbook(changedHeader), 'INVALID_HEADER')

  const duplicateId = cloneSnapshot(fixture)
  const channelSheet = duplicateId.sheets.find((sheet) => sheet.name === '渠道')
  assert.ok(channelSheet)
  channelSheet.rows[5].cells[0].value = channelSheet.rows[4].cells[0].value
  expectContractError(
    () => extractWorkbook(duplicateId),
    'DUPLICATE_BUSINESS_ID'
  )

  const brokenReference = cloneSnapshot(fixture)
  const saleSheet = brokenReference.sheets.find(
    (sheet) => sheet.name === '官方售价'
  )
  assert.ok(saleSheet)
  saleSheet.rows[4].cells[2].value = 'SKU-MISSING'
  expectContractError(
    () => extractWorkbook(brokenReference),
    'BROKEN_REFERENCE'
  )
})

test('v1 adapter advertises only the fixed v1 contract', async () => {
  const adapter = new V1WorkbookAdapter()
  const snapshot = await loadFixture()

  assert.equal(adapter.matches(snapshot).matched, true)
  snapshot.sheets.push({ name: '渠道线路', rows: [] })
  assert.equal(adapter.matches(snapshot).matched, false)
})
