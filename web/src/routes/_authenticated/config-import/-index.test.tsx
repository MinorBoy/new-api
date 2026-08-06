/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the License,
or (at your option) any later version.
*/
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
