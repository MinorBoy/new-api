/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.
*/
import assert from 'node:assert/strict'
import test from 'node:test'

import { api } from '@/lib/api'

import { listAssets } from '../api'

test('passes picker pagination to the asset list endpoint', async () => {
  const originalAdapter = api.defaults.adapter
  let seenParams: unknown
  api.defaults.adapter = async (config) => {
    seenParams = config.params
    return {
      data: {
        success: true,
        data: { items: [], total: 0, page: 2, page_size: 12 },
      },
      status: 200,
      statusText: 'OK',
      headers: {},
      config,
    }
  }

  try {
    await listAssets({ page: 2, pageSize: 12 })
    assert.deepEqual(seenParams, { type: 'image', page: 2, page_size: 12 })
  } finally {
    api.defaults.adapter = originalAdapter
  }
})
