import assert from 'node:assert/strict'
import test from 'node:test'

import { Route } from './index'

test('accepts a positive batch identifier for restoring an import', () => {
  const validator = Route.options.validateSearch as
    | { parse: (value: unknown) => unknown }
    | undefined

  assert.ok(validator)
  assert.deepEqual(validator.parse({ batch: 21 }), { batch: 21 })
})
