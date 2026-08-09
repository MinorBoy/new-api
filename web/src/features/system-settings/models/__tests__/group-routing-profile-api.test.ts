import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import { AxiosHeaders } from 'axios'

import { api } from '@/lib/api'

import {
  groupRoutingProfileTargetPageResponseSchema,
  previewGroupRoutingProfileSummaries,
  previewGroupRoutingProfileTargets,
} from '../group-routing-profile-api'

const targetPage = {
  items: [
    {
      model: 'doubao-seedance-2-0-260128',
      channel_id: 23,
      channel_name: 'Supplier A',
      target_name: 'duration-primary',
      upstream_model: 'vendor-video',
      cost_variant_key: 'default',
      target_priority: 100,
      supports_real_person: true,
      cost_mode: 'per_duration',
      cost_rule_id: 42,
      cost_rule_version: 2,
      target_key: 'grt_target',
      status: 'matched',
      issues: [],
    },
  ],
  summary: {
    models: 1,
    matched_models: 1,
    targets: 1,
    matched_targets: 1,
    stale_exclusions: 0,
  },
  facets: {
    models: ['doubao-seedance-2-0-260128'],
    channels: [{ id: 23, name: 'Supplier A' }],
    cost_modes: ['per_duration'],
    statuses: ['matched'],
  },
  page: 1,
  page_size: 25,
  total: 1,
}

describe('group routing profile API contract', () => {
  test('parses the safe paginated target view', () => {
    const response = groupRoutingProfileTargetPageResponseSchema.parse({
      success: true,
      data: targetPage,
    })

    assert.equal(response.data.items[0]?.target_key, 'grt_target')
    assert.equal(response.data.items[0]?.cost_rule_id, 42)
    assert.equal(response.data.total, 1)
  })

  test('posts unsaved profiles to the two preview endpoints', async () => {
    const requests: Array<{ url?: string; data?: string }> = []
    const previousAdapter = api.defaults.adapter
    api.defaults.adapter = async (config) => {
      requests.push({ url: config.url, data: config.data })
      const data = config.url?.endsWith('/targets')
        ? { success: true, data: targetPage }
        : {
            success: true,
            data: {
              客户A: targetPage.summary,
            },
          }
      return {
        data,
        status: 200,
        statusText: 'OK',
        headers: new AxiosHeaders(),
        config,
      }
    }

    try {
      const profile = {
        status: 'draft' as const,
        routing_source: 'default' as const,
      }
      await previewGroupRoutingProfileTargets({
        group_name: '客户A',
        profile,
        page: 1,
        page_size: 25,
      })
      const summaries = await previewGroupRoutingProfileSummaries({
        客户A: profile,
      })

      assert.deepEqual(
        requests.map((request) => request.url),
        [
          '/api/routing-policies/group-profile/targets',
          '/api/routing-policies/group-profile/summaries',
        ]
      )
      assert.equal(summaries.data.客户A?.matched_targets, 1)
    } finally {
      api.defaults.adapter = previousAdapter
    }
  })
})
