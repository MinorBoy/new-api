/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.
*/
import assert from 'node:assert/strict'
import test from 'node:test'

import type { ConfigImportItemDetail } from '../../types'
import { groupItemDiffs } from '../diff'

function item(
  business_id: string,
  entity_type: string,
  state: ConfigImportItemDetail['state'],
  source_sheet: string,
  source_row: number
): ConfigImportItemDetail {
  return {
    id: source_row,
    entity_type,
    business_id,
    entity_hash: `${business_id}-hash`,
    canonical_json: '{}',
    state,
    source_ref: `${source_sheet}!${source_row}`,
    source_sheet,
    source_row,
  }
}

test('groups item diffs by entity and state while sorting business ids', () => {
  const groups = groupItemDiffs([
    item('z-line', 'channel_lines', 'changed', '渠道成本', 8),
    item('a-line', 'channel_lines', 'new', '渠道成本', 4),
    item('b-line', 'channel_lines', 'changed', '渠道成本', 6),
    item('a-channel', 'channels', 'new', '渠道', 2),
  ])

  assert.deepEqual(
    groups.map((group) => `${group.entityType}:${group.state}`),
    ['channel_lines:new', 'channel_lines:changed', 'channels:new']
  )
  assert.deepEqual(
    groups
      .find((group) => group.state === 'changed')
      ?.items.map((entry) => entry.business_id),
    ['b-line', 'z-line']
  )
  assert.equal(groups.at(-1)?.items[0]?.source_sheet, '渠道')
  assert.equal(groups.at(-1)?.items[0]?.source_row, 2)
})
