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
  channels: 10,
  channelLines: 14,
  modelSkus: 8,
  saleProposals: 16,
  costRuleDrafts: 147,
  modelMappings: 147,
  detectedConflictKeys: 0,
  automaticConflictKeys: 0,
  manualConflictKeys: 0,
} as const

async function loadFixture(): Promise<WorkbookSnapshot> {
  return loadWorkbookSnapshot(await fs.readFile(fixturePath))
}

function cloneSnapshot(snapshot: WorkbookSnapshot): WorkbookSnapshot {
  return structuredClone(snapshot)
}

type SnapshotRow = WorkbookSnapshot['sheets'][number]['rows'][number]

function requiredCell(
  row: SnapshotRow,
  index: number
): SnapshotRow['cells'][number] {
  const cell = row.cells[index]
  assert.ok(cell)
  return cell
}

function expectContractError(callback: () => unknown, code: string): void {
  assert.throws(callback, (error: unknown) => {
    assert.ok(error instanceof WorkbookContractError)
    assert.equal(error.code, code)
    return true
  })
}

test('v1 snapshot preserves the ten fixed sheets and row-four headers', async () => {
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
    [
      'channel-megabyai',
      'megabyai-fast-no-real-person',
      'megabyai-fast-real-person',
    ]
  )

  const mergedScenarioCost = extracted.costRuleDrafts.find(
    (cost) => cost.businessId === 'COST-MEGABYAI-R106-480-REQ'
  )
  assert.ok(mergedScenarioCost)
  assert.equal(mergedScenarioCost.sourceLocations.length, 2)
  assert.equal(mergedScenarioCost.lineRef, 'megabyai-fast-real-person')
})

test('v1 adapter derives Secure and MegaByAI lines from business fields instead of row numbers', async () => {
  const snapshot = cloneSnapshot(await loadFixture())
  const mappingSheet = snapshot.sheets.find(
    (sheet) => sheet.name === '模型映射'
  )
  const costSheet = snapshot.sheets.find((sheet) => sheet.name === '渠道成本')
  assert.ok(mappingSheet)
  assert.ok(costSheet)
  const mappingNoteColumn = V1_HEADERS.模型映射.indexOf('备注')
  const costNoteColumn = V1_HEADERS.渠道成本.indexOf('备注')

  const secureMapping = mappingSheet.rows.find(
    (row) => row.cells[0]?.value === 'MAP-SECURE-R69-720'
  )
  assert.ok(secureMapping)
  requiredCell(secureMapping, 0).value = 'MAP-SECURE-R999-720'
  requiredCell(secureMapping, mappingNoteColumn).value =
    '上游模型分组=video-企业; 真人脸=是'

  const megaMapping = mappingSheet.rows.find(
    (row) => row.cells[0]?.value === 'MAP-MEGABYAI-R106-480'
  )
  assert.ok(megaMapping)
  requiredCell(megaMapping, 0).value = 'MAP-MEGABYAI-R998-480'
  requiredCell(megaMapping, mappingNoteColumn).value =
    '上游模型分组=默认; 真人脸=是'

  for (const row of costSheet.rows.filter((candidate) =>
    String(candidate.cells[0]?.value).startsWith('COST-MEGABYAI-R106-480-REQ')
  )) {
    const businessIdCell = requiredCell(row, 0)
    businessIdCell.value = String(businessIdCell.value).replace('R106', 'R998')
    requiredCell(row, costNoteColumn).value = '上游模型分组=默认; 真人脸=是'
  }
  for (const row of costSheet.rows.filter((candidate) =>
    String(candidate.cells[0]?.value).startsWith('COST-SECURE-R69-720-DUR')
  )) {
    const businessIdCell = requiredCell(row, 0)
    businessIdCell.value = String(businessIdCell.value).replace('R69', 'R999')
    requiredCell(row, costNoteColumn).value =
      '上游模型分组=video-企业; 真人脸=是'
  }

  const extracted = extractWorkbook(snapshot)
  assert.equal(
    extracted.modelMappings.find(
      (mapping) => mapping.businessId === 'MAP-MEGABYAI-R998-480'
    )?.lineRef,
    'megabyai-fast-real-person'
  )
  assert.equal(
    extracted.costRuleDrafts.find(
      (cost) => cost.businessId === 'COST-MEGABYAI-R998-480-REQ'
    )?.lineRef,
    'megabyai-fast-real-person'
  )
  assert.equal(
    extracted.modelMappings.find(
      (mapping) => mapping.businessId === 'MAP-SECURE-R999-720'
    )?.lineRef,
    'secure-enterprise'
  )
  assert.equal(
    extracted.costRuleDrafts.find(
      (cost) => cost.businessId === 'COST-SECURE-R999-720-DUR'
    )?.lineRef,
    'secure-enterprise'
  )
})

test('v1 adapter does not infer channel lines from legacy source row numbers', async () => {
  const snapshot = cloneSnapshot(await loadFixture())
  const mappingSheet = snapshot.sheets.find(
    (sheet) => sheet.name === '模型映射'
  )
  const costSheet = snapshot.sheets.find((sheet) => sheet.name === '渠道成本')
  assert.ok(mappingSheet)
  assert.ok(costSheet)
  const mappingNoteColumn = V1_HEADERS.模型映射.indexOf('备注')
  const costNoteColumn = V1_HEADERS.渠道成本.indexOf('备注')

  const megaMapping = mappingSheet.rows.find(
    (row) => row.cells[0]?.value === 'MAP-MEGABYAI-R106-480'
  )
  assert.ok(megaMapping)
  requiredCell(megaMapping, mappingNoteColumn).value = '原模型=videos-fast'

  for (const row of costSheet.rows.filter((candidate) => {
    const businessId = String(candidate.cells[0]?.value)
    return businessId.startsWith('COST-MEGABYAI-R106-480-REQ')
  })) {
    requiredCell(row, costNoteColumn).value = '原模型=legacy'
  }

  const extracted = extractWorkbook(snapshot)
  assert.equal(
    extracted.modelMappings.find(
      (mapping) => mapping.businessId === 'MAP-MEGABYAI-R106-480'
    )?.lineRef,
    'channel-megabyai'
  )
  assert.equal(
    extracted.costRuleDrafts.find(
      (cost) => cost.businessId === 'COST-MEGABYAI-R106-480-REQ'
    )?.lineRef,
    'channel-megabyai'
  )
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
  const dimensioTarget = targets.get('route-blueprint/MAP-DIMENSIO-R93-720')
  const clmmTarget = targets.get('route-blueprint/MAP-CLMM-R7-720')
  const paipuTarget = targets.get('route-blueprint/MAP-PAIPU-R22-720')
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
    (cost) => cost.business_id === 'COST-PAIPU-R22-720-REQ'
  )
  assert.ok(paipuCost)
  assert.equal(paipuCost.upstream_model, 'lec-seedance-2-0')
  assert.equal(paipuCost.cost_mode, 'per_request')
  assert.equal(paipuCost.enabled, true)
})

test('v1 document uses channel mapping duration instead of shared SKU duration', async () => {
  const snapshot = cloneSnapshot(await loadFixture())
  const mappingSheet = snapshot.sheets.find(
    (sheet) => sheet.name === '模型映射'
  )
  assert.ok(mappingSheet)
  const minDurationColumn = V1_HEADERS.模型映射.indexOf('最小时长秒')
  const maxDurationColumn = V1_HEADERS.模型映射.indexOf('最大时长秒')
  assert.notEqual(minDurationColumn, -1)
  assert.notEqual(maxDurationColumn, -1)
  const mapping = mappingSheet.rows.find(
    (row) => row.cells[0]?.value === 'MAP-SECURE-R69-720'
  )
  assert.ok(mapping)
  requiredCell(mapping, minDurationColumn).value = 5
  requiredCell(mapping, maxDurationColumn).value = 15

  const result = await buildImportDocument({
    extracted: extractWorkbook(snapshot),
    sourceBytes: await fs.readFile(fixturePath),
    sourceFileName: 'channel-config-v1-mapping-duration.xlsx',
  })
  const blueprint = result.document.entities.route_blueprints.find(
    (item) => item.business_id === 'route-blueprint/MAP-SECURE-R69-720'
  )
  assert.ok(blueprint)
  const target = (blueprint.targets as Array<Record<string, unknown>>)[0]

  assert.equal(target?.duration_min, 5)
  assert.equal(target?.duration_max, 15)
})

test('v1 document preserves discrete channel durations on the route target', async () => {
  const snapshot = cloneSnapshot(await loadFixture())
  const extracted = extractWorkbook(snapshot)
  const mapping = extracted.modelMappings.find(
    (item) => item.businessId === 'MAP-SECURE-R69-720'
  )
  assert.ok(mapping)
  mapping.fields['可用时长秒'] = {
    value: '5,10,15',
    formula: null,
    formulaResult: null,
  }

  const result = await buildImportDocument({
    extracted,
    sourceBytes: await fs.readFile(fixturePath),
    sourceFileName: 'channel-config-v1-discrete-duration.xlsx',
  })
  const blueprint = result.document.entities.route_blueprints.find(
    (item) => item.business_id === 'route-blueprint/MAP-SECURE-R69-720'
  )
  assert.ok(blueprint)
  const target = (blueprint.targets as Array<Record<string, unknown>>)[0]

  assert.deepEqual(target?.duration_values, [5, 10, 15])
  assert.equal(target?.duration_min, undefined)
  assert.equal(target?.duration_max, undefined)
})

test('v1 document preserves disabled cost draft status', async () => {
  const snapshot = cloneSnapshot(await loadFixture())
  const costSheet = snapshot.sheets.find((sheet) => sheet.name === '渠道成本')
  assert.ok(costSheet)
  const statusColumn = V1_HEADERS.渠道成本.indexOf('状态')
  const rows = costSheet.rows.filter((candidate) =>
    String(candidate.cells[0]?.value).startsWith('COST-PAIPU-R22-720-REQ')
  )
  assert.ok(rows.length > 0)
  for (const row of rows) requiredCell(row, statusColumn).value = 'draft'

  const result = await buildImportDocument({
    extracted: extractWorkbook(snapshot),
    sourceBytes: await fs.readFile(fixturePath),
    sourceFileName: 'channel-config-v1-corrected.xlsx',
  })
  const draft = result.document.entities.cost_rule_drafts.find(
    (cost) => cost.business_id === 'COST-PAIPU-R22-720-REQ'
  )

  assert.ok(draft)
  assert.equal(draft.enabled, false)
})

test('v1 document preserves task cost accounting contract fields', async () => {
  const snapshot = cloneSnapshot(await loadFixture())
  const costSheet = snapshot.sheets.find((sheet) => sheet.name === '渠道成本')
  assert.ok(costSheet)
  const tokenModeColumn = V1_HEADERS.渠道成本.indexOf('Token子模式')
  const meterSourceColumn = V1_HEADERS.渠道成本.indexOf('计量来源')
  const chargeEventColumn = V1_HEADERS.渠道成本.indexOf('计费事件')
  const rows = costSheet.rows.filter((candidate) =>
    String(candidate.cells[0]?.value).startsWith('COST-LUCEN-R52-480-TOK')
  )
  assert.equal(rows.length, 2)
  for (const row of rows) {
    requiredCell(row, tokenModeColumn).value = 'total_tokens'
    requiredCell(row, meterSourceColumn).value = 'upstream_usage'
    requiredCell(row, chargeEventColumn).value = 'task_succeeded'
  }

  const result = await buildImportDocument({
    extracted: extractWorkbook(snapshot),
    sourceBytes: await fs.readFile(fixturePath),
    sourceFileName: 'channel-config-v1-cost-accounting.xlsx',
  })
  const draft = result.document.entities.cost_rule_drafts.find(
    (cost) => cost.business_id === 'COST-LUCEN-R52-480-TOK'
  )

  assert.ok(draft)
  assert.equal(draft.token_mode, 'total_tokens')
  assert.equal(draft.meter_source, 'upstream_usage')
  assert.equal(draft.charge_event, 'task_succeeded')
})

test('v1 document rejects an unresolved legacy reference limit', async () => {
  const snapshot = await loadFixture()
  const mappingSheet = snapshot.sheets.find(
    (sheet) => sheet.name === '模型映射'
  )
  assert.ok(mappingSheet)
  const mappingRow = mappingSheet.rows.find(
    (row) => row.cells[0]?.value === 'MAP-8YES-R60-480'
  )
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

test('v1 document preserves structured aggregate reference constraints', async () => {
  const snapshot = await loadFixture()
  const mappingSheet = snapshot.sheets.find(
    (sheet) => sheet.name === '模型映射'
  )
  assert.ok(mappingSheet)
  const mappingRow = mappingSheet.rows.find(
    (row) => row.cells[0]?.value === 'MAP-CLMM-R3-720'
  )
  assert.ok(mappingRow)
  const noteColumn = V1_HEADERS.模型映射.indexOf('备注')
  mappingRow.cells[noteColumn].value =
    '原模型=provider; 参考图数=9; 参考视频数=3; 参考音频数=3; 最大素材数=12; 视频音频合计上限=3; 素材模式=first_last_frames,omni_reference; 参考视频总时长上限秒=15; 最小参考图数=1; 原比例=16:9,9:16; 真人脸=是'

  const result = await buildImportDocument({
    extracted: extractWorkbook(snapshot),
    sourceBytes: await fs.readFile(fixturePath),
    sourceFileName: 'channel-config-v1-corrected.xlsx',
  })
  const blueprint = result.document.entities.route_blueprints.find((item) =>
    (item.model_mapping_refs as string[] | undefined)?.includes(
      'MAP-CLMM-R3-720'
    )
  )
  assert.ok(blueprint)
  const target = (blueprint.targets as Array<Record<string, unknown>>)[0]

  assert.equal(target.reference_total_max, 12)
  assert.equal(target.reference_video_audio_total_max, 3)
  assert.equal(target.reference_video_total_duration_seconds, 15)
  assert.deepEqual(target.reference_modes, [
    'first_last_frames',
    'omni_reference',
  ])
  assert.deepEqual(target.aspect_ratios, ['16:9', '9:16'])
  assert.deepEqual(target.input_modes, [
    'first_frame',
    'first_last_frames',
    'omni_reference',
  ])
  assert.deepEqual(target.reference_minimums, {
    images: 1,
    videos: 0,
    audios: 0,
  })
})

test('v1 import document exposes omni reference mode for declared media capacity', async () => {
  const snapshot = await loadFixture()
  const mappingSheet = snapshot.sheets.find(
    (sheet) => sheet.name === '模型映射'
  )
  assert.ok(mappingSheet)
  const mappingRow = mappingSheet.rows.find((row) => row.rowNumber === 5)
  assert.ok(mappingRow)
  const noteColumn = V1_HEADERS.模型映射.indexOf('备注')
  mappingRow.cells[noteColumn].value =
    '原模型=provider; 参考图数=9; 参考视频数=3; 参考音频数=3; 最大素材数=15; 素材模式=; 参考视频总时长上限秒=15; 最小参考图数=0; 原比例=16:9; 真人脸=是'

  const result = await buildImportDocument({
    extracted: extractWorkbook(snapshot),
    sourceBytes: await fs.readFile(fixturePath),
    sourceFileName: 'channel-config-v1-media.xlsx',
  })
  const blueprint = result.document.entities.route_blueprints.find((item) =>
    (item.model_mapping_refs as string[] | undefined)?.includes(
      'MAP-CLMM-R3-720'
    )
  )
  assert.ok(blueprint)
  const target = (blueprint.targets as Array<Record<string, unknown>>)[0]

  assert.deepEqual(target.input_modes, ['text', 'omni_reference'])
})

test('v1 import document uses the reserved YSR channel type IDs', async () => {
  const sourceBytes = await fs.readFile(fixturePath)
  const extracted = extractWorkbook(await loadFixture())
  const sourceChannel = extracted.channels[0]
  assert.ok(sourceChannel)
  const sourceLocation = sourceChannel.sourceLocations[0]
  assert.ok(sourceLocation)
  extracted.channels.push({
    ...sourceChannel,
    businessId: 'CH-OMEGAAI',
    sourceLocations: [
      {
        ...sourceLocation,
        businessId: 'CH-OMEGAAI',
        row: 999,
      },
    ],
  })
  extracted.channels.push({
    ...sourceChannel,
    businessId: 'CH-Z5API',
    sourceLocations: [
      {
        ...sourceLocation,
        businessId: 'CH-Z5API',
        row: 1000,
      },
    ],
  })
  const result = await buildImportDocument({
    extracted,
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
  assert.equal(typesByChannel.get('CH-OMEGAAI'), 208)
  assert.equal(typesByChannel.get('CH-4STOKEN'), 209)
  assert.equal(typesByChannel.get('CH-8YES'), 210)
  assert.equal(typesByChannel.get('CH-Z5API'), 211)
})

test('v1 adapter treats line and SKU price differences as distinct variants and removes Secure unsupported 480p rows', async () => {
  const extracted = extractWorkbook(await loadFixture())

  assert.deepEqual(extracted.unresolvedVariants, [])

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

test('v1 document gives different price contracts distinct stable cost variants', async () => {
  const sourceBytes = await fs.readFile(fixturePath)
  const snapshot = cloneSnapshot(await loadFixture())
  const costSheet = snapshot.sheets.find((sheet) => sheet.name === '渠道成本')
  assert.ok(costSheet)
  const priceColumn = V1_HEADERS.渠道成本.indexOf('原币/秒')
  for (const row of costSheet.rows.filter((candidate) =>
    String(candidate.cells[0]?.value).startsWith('COST-MEGABYAI-R120-720-DUR')
  )) {
    requiredCell(row, priceColumn).value = '0.44'
  }
  const result = await buildImportDocument({
    extracted: extractWorkbook(snapshot),
    sourceBytes,
    sourceFileName: 'channel-config-v1-corrected.xlsx',
  })
  const contracts = new Map<string, string>()

  for (const draft of result.document.entities.cost_rule_drafts) {
    const runtimeKey = [
      draft.line_ref,
      draft.upstream_model,
      draft.cost_variant_key,
    ].join('/')
    const contract = JSON.stringify({
      billing_multiplier: draft.billing_multiplier,
      cost_mode: draft.cost_mode,
      currency: draft.currency,
      currency_to_usd_rate: draft.currency_to_usd_rate,
      fee_rate: draft.fee_rate,
      input_per_million: draft.input_per_million,
      output_per_million: draft.output_per_million,
      completion_per_million: draft.completion_per_million,
      total_per_million: draft.total_per_million,
      price_per_second: draft.price_per_second,
      purchase_discount_ratio: draft.purchase_discount_ratio,
      recharge_exchange_ratio: draft.recharge_exchange_ratio,
      unit_price: draft.unit_price,
    })
    const previous = contracts.get(runtimeKey)
    assert.ok(
      previous === undefined || previous === contract,
      `conflicting price contracts share runtime key ${runtimeKey}`
    )
    contracts.set(runtimeKey, contract)
  }
})

test('v1 adapter rejects conflicting NOV and VID cost contracts before deduplication', async () => {
  const snapshot = cloneSnapshot(await loadFixture())
  const costSheet = snapshot.sheets.find((sheet) => sheet.name === '渠道成本')
  assert.ok(costSheet)
  const priceColumn = V1_HEADERS.渠道成本.indexOf('原币/秒')
  const scenarioRows = costSheet.rows.filter((row) =>
    String(row.cells[0]?.value).startsWith('COST-MEGABYAI-R120-720-DUR')
  )
  assert.equal(scenarioRows.length, 2)
  requiredCell(scenarioRows[0], priceColumn).value = '0.44'

  expectContractError(() => extractWorkbook(snapshot), 'COST_SCENARIO_CONFLICT')
})

test('v1 adapter ignores non-runtime NOV and VID differences', async () => {
  const snapshot = cloneSnapshot(await loadFixture())
  const costSheet = snapshot.sheets.find((sheet) => sheet.name === '渠道成本')
  assert.ok(costSheet)
  const unusedPriceColumn = V1_HEADERS.渠道成本.indexOf('原币按次')
  const scenarioRows = costSheet.rows.filter((row) =>
    String(row.cells[0]?.value).startsWith('COST-MEGABYAI-R120-720-DUR')
  )
  assert.equal(scenarioRows.length, 2)
  requiredCell(scenarioRows[0], unusedPriceColumn).value = '99'

  assert.doesNotThrow(() => extractWorkbook(snapshot))
})

test('v1 adapter rejects Secure costs without a recognized upstream group', async () => {
  const snapshot = cloneSnapshot(await loadFixture())
  const costSheet = snapshot.sheets.find((sheet) => sheet.name === '渠道成本')
  assert.ok(costSheet)
  const noteColumn = V1_HEADERS.渠道成本.indexOf('备注')
  const secureCosts = costSheet.rows.filter((row) =>
    String(row.cells[0]?.value).startsWith('COST-SECURE-R69-720-DUR')
  )
  assert.equal(secureCosts.length, 2)
  for (const secureCost of secureCosts) {
    requiredCell(secureCost, noteColumn).value = '真人脸=是'
  }

  expectContractError(() => extractWorkbook(snapshot), 'SECURE_GROUP_UNRESOLVED')
})

test('v1 document keeps the cost variant stable when supplier price changes', async () => {
  const sourceBytes = await fs.readFile(fixturePath)
  const baseline = await buildImportDocument({
    extracted: extractWorkbook(await loadFixture()),
    sourceBytes,
    sourceFileName: 'channel-config-v1-corrected.xlsx',
  })
  const changedSnapshot = cloneSnapshot(await loadFixture())
  const costSheet = changedSnapshot.sheets.find(
    (sheet) => sheet.name === '渠道成本'
  )
  assert.ok(costSheet)
  const priceColumn = V1_HEADERS.渠道成本.indexOf('原币/秒')
  for (const row of costSheet.rows.filter((candidate) =>
    String(candidate.cells[0]?.value).startsWith('COST-MEGABYAI-R120-720-DUR')
  )) {
    requiredCell(row, priceColumn).value = '0.44'
  }
  const changed = await buildImportDocument({
    extracted: extractWorkbook(changedSnapshot),
    sourceBytes,
    sourceFileName: 'channel-config-v1-corrected.xlsx',
  })
  const businessID = 'COST-MEGABYAI-R120-720-DUR'
  const baselineDraft = baseline.document.entities.cost_rule_drafts.find(
    (draft) => draft.business_id === businessID
  )
  const changedDraft = changed.document.entities.cost_rule_drafts.find(
    (draft) => draft.business_id === businessID
  )
  assert.ok(baselineDraft)
  assert.ok(changedDraft)
  assert.equal(changedDraft.cost_variant_key, baselineDraft.cost_variant_key)
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
