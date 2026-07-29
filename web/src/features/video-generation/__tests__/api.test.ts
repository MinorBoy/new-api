import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import { getTokenRequestConfig, loadVideoApiKeyPages } from '../api'
import type { ApiKey } from '../../keys/types'

describe('video generation token requests', () => {
  test('uses the selected API key without session refresh', () => {
    assert.deepEqual(getTokenRequestConfig('sk-selected'), {
      authToken: 'sk-selected',
      skipAuthRefresh: true,
      skipErrorHandler: true,
    })
  })

  test('loads enabled API keys from every page', async () => {
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

    const keys = await loadVideoApiKeyPages(async ({ p = 1 }) => {
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
