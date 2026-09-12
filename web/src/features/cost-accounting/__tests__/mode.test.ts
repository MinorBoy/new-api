import assert from 'node:assert/strict'
import test from 'node:test'

import { isCostAccountingMode } from '../lib/mode'

test('accepts tracking without changing the strict mode contract', () => {
  assert.equal(isCostAccountingMode('disabled'), true)
  assert.equal(isCostAccountingMode('tracking'), true)
  assert.equal(isCostAccountingMode('strict'), true)
  assert.equal(isCostAccountingMode('unknown'), false)
})
