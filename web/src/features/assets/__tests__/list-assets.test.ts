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

import { createAsset, listAssets, refreshAsset } from '../api'
import type { Asset } from '../types'

test('passes picker pagination to the asset list endpoint', async () => {
  const originalAdapter = api.defaults.adapter
  let seenParams: unknown
  let seenAuthToken: unknown
  let seenSkipAuthRefresh: unknown
  api.defaults.adapter = async (config) => {
    seenParams = config.params
    seenAuthToken = config.authToken
    seenSkipAuthRefresh = config.skipAuthRefresh
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
    await listAssets('sk-selected', { page: 2, pageSize: 12 })
    assert.deepEqual(seenParams, { type: 'image', page: 2, page_size: 12 })
    assert.equal(seenAuthToken, 'sk-selected')
    assert.equal(seenSkipAuthRefresh, true)
  } finally {
    api.defaults.adapter = originalAdapter
  }
})

test('uses the selected API key for create and refresh requests', async () => {
  const originalAdapter = api.defaults.adapter
  const requests: Array<{ method?: string; authToken?: string }> = []
  api.defaults.adapter = async (config) => {
    requests.push({ method: config.method, authToken: config.authToken })
    return {
      data: {
        success: true,
        data: {
          id: 'asset-test',
          type: 'image',
          url: 'https://example.com/character.png',
          status: 'processing',
          provider: 'secure',
          created_at: 1_700_000_000,
          updated_at: 1_700_000_000,
        },
      },
      status: 200,
      statusText: 'OK',
      headers: {},
      config,
    }
  }
  const asset: Asset = {
    id: 'asset-test',
    type: 'image',
    url: 'https://example.com/character.png',
    status: 'processing',
    provider: 'secure',
    created_at: 1_700_000_000,
    updated_at: 1_700_000_000,
  }

  try {
    await createAsset('sk-selected', asset.url)
    await refreshAsset('sk-selected', asset)
    assert.deepEqual(requests, [
      { method: 'post', authToken: 'sk-selected' },
      { method: 'post', authToken: 'sk-selected' },
    ])
  } finally {
    api.defaults.adapter = originalAdapter
  }
})
