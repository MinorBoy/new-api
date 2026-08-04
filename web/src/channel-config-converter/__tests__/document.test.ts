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

import { extractWorkbook } from '../adapters/v1'
import { buildImportDocument } from '../document'
import { scanForSecrets } from '../security-scan'
import { loadWorkbookSnapshot } from '../workbook'

const fixturePath = fileURLToPath(
  new URL('../__fixtures__/channel-config-v1-corrected.xlsx', import.meta.url)
)

test('builds the corrected v1 import document without unresolved contracts', async () => {
  const bytes = await fs.readFile(fixturePath)
  const extracted = extractWorkbook(await loadWorkbookSnapshot(bytes))

  const result = await buildImportDocument({
    extracted,
    sourceBytes: bytes,
    sourceFileName: 'channel-config-v1-corrected.xlsx',
  })

  assert.equal(result.document.kind, 'new-api.channel-config-import')
  assert.equal(result.document.template_version, '1')
  assert.equal(result.document.entities.channels.length, 10)
  assert.equal(result.document.entities.channel_lines.length, 14)
  assert.equal(result.document.entities.model_skus.length, 8)
  assert.equal(result.document.entities.sale_proposals.length, 16)

  for (const proposal of result.document.entities.sale_proposals) {
    assert.equal('enabled' in proposal, false)
  }

  assert.equal(result.document.entities.cost_rule_drafts.length, 147)
  assert.equal(result.document.entities.model_mappings.length, 147)
  assert.equal(result.document.entities.route_blueprints.length, 147)
  assert.equal(result.document.entities.unresolved_variants.length, 0)
  assert.deepEqual(result.document.issues, [])
  assert.equal(result.hasFailures, false)
  assert.equal(result.hasWarnings, false)
  assert.equal(
    result.document.entities.model_mappings.some(
      (mapping) => mapping.business_id === 'MAP-8YES-R60-480'
    ),
    true
  )
  assert.equal(
    result.document.entities.route_blueprints.some(
      (blueprint) =>
        blueprint.business_id === 'route-blueprint/MAP-8YES-R60-480'
    ),
    true
  )
  assert.equal(scanForSecrets(result.document).length, 0)
  assert.match(result.document.manifest.payload_sha256, /^[a-f0-9]{64}$/)
  const businessIDs = new Set<string>()
  for (const collection of Object.values(result.document.entities)) {
    for (const entity of collection) {
      assert.equal(
        businessIDs.has(entity.business_id),
        false,
        `duplicate business ID: ${entity.business_id}`
      )
      businessIDs.add(entity.business_id)
    }
  }
  assert.ok(
    result.document.entities.route_blueprints.every((blueprint) =>
      (blueprint.targets as Array<{ enabled: boolean }>).every(
        (target) => target.enabled === false
      )
    )
  )
})

test('builds explicit Seedance official token sale contracts from USD per million prices', async () => {
  const bytes = await fs.readFile(fixturePath)
  const extracted = extractWorkbook(await loadWorkbookSnapshot(bytes))
  const expectedPrices = new Map([
    ['SALE-SEEDANCE-2-0-1080P-NOV', '6.3'],
    ['SALE-SEEDANCE-2-0-1080P-VID', '3.8'],
  ])

  for (const sale of extracted.saleProposals) {
    const price = expectedPrices.get(sale.businessId)
    if (price) {
      sale.fields['USD/1M'] = {
        value: price,
        formula: null,
        formulaResult: null,
      }
    }
  }

  const result = await buildImportDocument({
    extracted,
    sourceBytes: bytes,
    sourceFileName: 'channel-config-v1-corrected.xlsx',
  })

  for (const [businessID, price] of expectedPrices) {
    const proposal = result.document.entities.sale_proposals.find(
      (item) => item.business_id === businessID
    )
    assert.ok(proposal)
    assert.equal(proposal.billing_mode, 'seedance_tokens')
    assert.equal(
      proposal.scenario,
      businessID.endsWith('-NOV') ? 'no_video' : 'with_video'
    )
    assert.equal(proposal.resolution, '1080p')

    const tokenPrice = proposal.seedance_token_price as Record<string, unknown>
    assert.equal(tokenPrice.price_per_million, price)
    assert.equal(tokenPrice.width, 1920)
    assert.equal(tokenPrice.height, 1080)
    assert.equal(tokenPrice.frame_rate, 24)
    assert.equal(tokenPrice.pricing_version, 'official-token-v1')
    assert.equal('duration_price' in proposal, false)
  }
})
