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

import { V2WorkbookAdapter } from '../adapters/v2'
import { buildImportDocument } from '../document'
import { WorkbookContractError } from '../schema'
import type { CellSnapshot, WorkbookSnapshot } from '../types'
import { loadWorkbookSnapshot, V2_HEADERS } from '../workbook'

const v2FixturePath = fileURLToPath(
  new URL('../__fixtures__/channel-config-v2-golden.xlsx', import.meta.url)
)
const v2TemplatePath = fileURLToPath(
  new URL('../../../../docs/templates/channel-config-v2.xlsx', import.meta.url)
)

function cell(value: CellSnapshot['value']): CellSnapshot {
  return { value, formula: null, formulaResult: null }
}

function createSheet(
  name: string,
  headers: readonly string[],
  records: Record<string, CellSnapshot['value']>[]
) {
  return {
    name,
    rows: [
      { rowNumber: 1, cells: [] },
      { rowNumber: 2, cells: [] },
      { rowNumber: 3, cells: [] },
      { rowNumber: 4, cells: headers.map(cell) },
      ...records.map((record, index) => ({
        rowNumber: index + 5,
        cells: headers.map((header) => cell(record[header] ?? null)),
      })),
    ],
  }
}

function createV2Snapshot(): WorkbookSnapshot {
  const records: Record<string, Record<string, CellSnapshot['value']>[]> = {
    渠道: [
      {
        channel_ref: 'CH-4STOKEN',
        display_name: 'Example channel',
        currency: 'USD',
        recharge_ratio: 1,
        fee_rate: 0,
        billing_multiplier: 1,
        status_proposal: 'disabled',
        source_ref: 'SRC-EXAMPLE',
        note: 'provider reference only',
      },
    ],
    渠道线路: [
      {
        line_ref: 'example-line',
        channel_ref: 'CH-4STOKEN',
        display_name: 'Example line',
        provider_type_hint: 'openai',
        region: 'global',
        protocol: 'openai',
        supports_real_person: false,
        status_proposal: 'disabled',
        source_ref: 'SRC-EXAMPLE',
        note: 'Do not infer routing rules from this note.',
      },
    ],
    模型SKU: [
      {
        sku_ref: 'SKU-EXAMPLE-720',
        canonical_model: 'seedance-2.0',
        resolution: '720p',
        output_width: 1280,
        output_height: 720,
        frame_rate: 24,
        duration_min: 4,
        duration_max: 15,
        source_ref: 'SRC-EXAMPLE',
      },
    ],
    官方售价: [
      {
        sale_ref: 'SALE-EXAMPLE',
        client_model: 'seedance-2.0',
        sku_ref: 'SKU-EXAMPLE-720',
        scenario: 'no_video',
        billing_mode: 'per_request',
        currency: 'USD',
        native_unit_price: '4',
        status_proposal: 'disabled',
        source_ref: 'SRC-EXAMPLE',
      },
    ],
    渠道成本: [
      {
        cost_rule_ref: 'COST-EXAMPLE',
        line_ref: 'example-line',
        upstream_model: 'video-2.0',
        sku_ref: 'SKU-EXAMPLE-720',
        scenario: 'no_video',
        cost_mode: 'per_request',
        cost_variant_key: 'retail-720',
        route_target_ref: 'ROUTE-EXAMPLE',
        currency: 'USD',
        currency_to_usd_rate: '1',
        billing_multiplier: '1',
        purchase_discount_ratio: '1',
        recharge_exchange_ratio: '1',
        fee_rate: '0',
        native_unit_price: '3',
        normalized_usd_unit_price: '3',
        status_proposal: 'disabled',
        source_ref: 'SRC-EXAMPLE',
        note: 'This note cannot select a route.',
      },
    ],
    路由目标: [
      {
        route_target_ref: 'ROUTE-EXAMPLE',
        canonical_model: 'seedance-2.0',
        client_model: 'seedance-2.0',
        merge_mode: 'merge',
        line_ref: 'example-line',
        upstream_model: 'video-2.0',
        sku_ref: 'SKU-EXAMPLE-720',
        cost_variant_key: 'retail-720',
        output_resolutions: '720p',
        duration_min: 4,
        duration_max: 15,
        reference_min_images: 0,
        reference_min_videos: 0,
        reference_min_audios: 0,
        reference_max_images: 9,
        reference_max_videos: 3,
        reference_max_audios: 3,
        supports_real_person: false,
        priority: 10,
        enabled: false,
      },
    ],
    模型映射: [
      {
        mapping_ref: 'MAP-EXAMPLE',
        client_model: 'seedance-2.0',
        line_ref: 'example-line',
        upstream_model: 'video-2.0',
        sku_ref: 'SKU-EXAMPLE-720',
        route_target_ref: 'ROUTE-EXAMPLE',
        source_ref: 'SRC-EXAMPLE',
        note: 'Audit context only.',
      },
    ],
    来源: [
      {
        source_ref: 'SRC-EXAMPLE',
        name: 'Example source',
        url: 'https://example.invalid/pricing',
        collected_at: '2026-07-26',
        note: 'Source audit record',
      },
    ],
  }

  return {
    sheets: Object.entries(V2_HEADERS).map(([name, headers]) =>
      createSheet(name, headers, records[name] ?? [])
    ),
  }
}

test('v2 adapter consumes explicit channel line and route target columns without note inference', () => {
  const adapter = new V2WorkbookAdapter()
  const extracted = adapter.extract(createV2Snapshot())

  assert.equal(adapter.matches(createV2Snapshot()).matched, true)
  assert.equal(extracted.templateVersion, '2')
  assert.deepEqual(
    extracted.channelLines.map((line) => line.businessId),
    ['example-line']
  )
  assert.deepEqual(
    extracted.routeBlueprints.map((target) => ({
      businessId: target.businessId,
      lineRef: target.lineRef,
      variant: target.fields.cost_variant_key.value,
    })),
    [
      {
        businessId: 'ROUTE-EXAMPLE',
        lineRef: 'example-line',
        variant: 'retail-720',
      },
    ]
  )
  assert.equal(extracted.costRuleDrafts[0].lineRef, 'example-line')
  assert.equal(
    extracted.costRuleDrafts[0].fields.note.value,
    'This note cannot select a route.'
  )
})

test('v2 adapter rejects a route target with a broken explicit line reference', () => {
  const snapshot = createV2Snapshot()
  const routeSheet = snapshot.sheets.find((sheet) => sheet.name === '路由目标')
  assert.ok(routeSheet)
  const lineRefColumn = V2_HEADERS.路由目标.indexOf('line_ref')
  routeSheet.rows[4].cells[lineRefColumn].value = 'missing-line'

  assert.throws(
    () => new V2WorkbookAdapter().extract(snapshot),
    (error: unknown) => {
      assert.ok(error instanceof WorkbookContractError)
      assert.equal(error.code, 'BROKEN_REFERENCE')
      return true
    }
  )
})

test('v2 document preserves explicit line, route target, and cost variant fields', async () => {
  const result = await buildImportDocument({
    extracted: new V2WorkbookAdapter().extract(createV2Snapshot()),
    sourceBytes: new Uint8Array([1, 2, 3]),
    sourceFileName: 'channel-config-v2.xlsx',
  })

  assert.equal(
    result.hasFailures,
    false,
    JSON.stringify(result.document.issues)
  )
  assert.equal(result.hasWarnings, false)
  assert.deepEqual(
    result.document.entities.cost_rule_drafts.map((draft) => ({
      lineRef: draft.line_ref,
      routeTargetRef: draft.route_target_ref,
      variant: draft.cost_variant_key,
    })),
    [
      {
        lineRef: 'example-line',
        routeTargetRef: 'ROUTE-EXAMPLE',
        variant: 'retail-720',
      },
    ]
  )
  assert.deepEqual(
    result.document.entities.route_blueprints.map((blueprint) => ({
      businessId: blueprint.business_id,
      mappings: blueprint.model_mapping_refs,
      target: blueprint.targets,
    })),
    [
      {
        businessId: 'route-blueprint/ROUTE-EXAMPLE',
        mappings: ['MAP-EXAMPLE'],
        target: [
          {
            cost_variant_key: 'retail-720',
            duration_max: 15,
            duration_min: 4,
            enabled: false,
            line_ref: 'example-line',
            output_resolutions: ['720p'],
            priority: 10,
            reference_limits: { images: 9, videos: 3, audios: 3 },
            reference_minimums: { images: 0, videos: 0, audios: 0 },
            route_target_ref: 'ROUTE-EXAMPLE',
            sku_ref: 'SKU-EXAMPLE-720',
            supports_real_person: false,
            upstream_model: 'video-2.0',
          },
        ],
      },
    ]
  )
})

test('generated v2 workbook has explicit disabled routing and no structural conflicts', async () => {
  const bytes = await fs.readFile(v2FixturePath)
  const workbook = await loadWorkbookSnapshot(bytes)
  const adapter = new V2WorkbookAdapter()
  const extracted = adapter.extract(workbook)
  const result = await buildImportDocument({
    extracted,
    sourceBytes: bytes,
    sourceFileName: 'channel-config-v2-golden.xlsx',
  })

  assert.equal(adapter.matches(workbook).matched, true)
  assert.equal(
    result.hasFailures,
    false,
    JSON.stringify(result.document.issues)
  )
  assert.equal(
    result.hasWarnings,
    false,
    JSON.stringify(result.document.issues)
  )
  assert.equal(result.document.entities.channels.length, 9)
  assert.equal(result.document.entities.channel_lines.length, 12)
  assert.equal(result.document.entities.model_skus.length, 9)
  assert.equal(result.document.entities.sale_proposals.length, 16)
  assert.equal(result.document.entities.cost_rule_drafts.length, 98)
  assert.equal(result.document.entities.model_mappings.length, 98)
  assert.equal(result.document.entities.route_blueprints.length, 98)
  assert.equal(result.document.entities.unresolved_variants.length, 0)
  assert.ok(
    result.document.entities.route_blueprints.every((blueprint) =>
      (blueprint.targets as Array<Record<string, unknown>>).every(
        (target) =>
          typeof target.line_ref === 'string' &&
          typeof target.cost_variant_key === 'string' &&
          typeof target.route_target_ref === 'string' &&
          target.enabled === false
      )
    )
  )
  assert.deepEqual(await fs.readFile(v2TemplatePath), bytes)
})
