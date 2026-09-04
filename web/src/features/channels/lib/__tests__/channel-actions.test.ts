import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import { normalizeChannelGroups } from '../channel-actions'

describe('normalizeChannelGroups', () => {
  test('trims, removes empty values, and preserves first-seen order', () => {
    assert.deepEqual(
      normalizeChannelGroups([' default ', '', 'vip', 'default', ' vip ']),
      ['default', 'vip']
    )
  })

  test('returns an empty list when no group is selected', () => {
    assert.deepEqual(normalizeChannelGroups([' ', '\t']), [])
  })
})
