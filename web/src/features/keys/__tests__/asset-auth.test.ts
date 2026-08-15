/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.
*/
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import { getApiKeyRequestConfig, loadEnabledApiKeyPages } from '../api'
import type { ApiKey } from '../types'

describe('API key authenticated requests', () => {
  test('builds a request that cannot fall back to dashboard auth', () => {
    assert.deepEqual(getApiKeyRequestConfig('sk-selected'), {
      authToken: 'sk-selected',
      skipAuthRefresh: true,
      skipErrorHandler: true,
    })
  })

  test('loads every page and keeps only enabled keys', async () => {
    const pages: number[] = []
    const makeKey = (id: number, status = 1): ApiKey => ({
      id,
      name: `Key ${id}`,
      key: `sk-${id}`,
      status,
      remain_quota: 0,
      used_quota: 0,
      unlimited_quota: true,
      expired_time: -1,
      created_time: 0,
      accessed_time: 0,
      group: 'default',
      cross_group_retry: false,
      model_limits_enabled: false,
      model_limits: '',
      allow_ips: '',
    })

    const keys = await loadEnabledApiKeyPages(async ({ p = 1 }) => {
      pages.push(p)
      return {
        success: true,
        data: {
          items: [makeKey(p, p === 2 ? 2 : 1)],
          total: 201,
          page: p,
          page_size: 100,
        },
      }
    })

    assert.deepEqual(pages, [1, 2, 3])
    assert.deepEqual(
      keys.map((key) => key.id),
      [1, 3]
    )
  })
})
