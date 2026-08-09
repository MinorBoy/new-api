import assert from 'node:assert/strict'
import { test } from 'node:test'

import { AxiosHeaders } from 'axios'

import { api } from '@/lib/api'

import { updateGroupSettings } from '../../api'

test('updates the complete group settings snapshot in one request', async () => {
  let capturedUrl = ''
  let capturedBody: unknown
  const previousAdapter = api.defaults.adapter
  api.defaults.adapter = async (config) => {
    capturedUrl = config.url ?? ''
    capturedBody = JSON.parse(String(config.data))
    return {
      data: { success: true, message: '' },
      status: 200,
      statusText: 'OK',
      headers: new AxiosHeaders(),
      config,
    }
  }

  try {
    await updateGroupSettings({
      group_ratio: '{"default":1}',
      group_status: '{"default":true}',
      topup_group_ratio: '{}',
      user_usable_groups: '{}',
      group_group_ratio: '{}',
      auto_groups: '[]',
      default_use_auto_group: false,
      group_special_usable_group: '{}',
      group_routing_requirements: '{}',
    })

    assert.equal(capturedUrl, '/api/option/group-settings')
    assert.deepEqual(capturedBody, {
      group_ratio: '{"default":1}',
      group_status: '{"default":true}',
      topup_group_ratio: '{}',
      user_usable_groups: '{}',
      group_group_ratio: '{}',
      auto_groups: '[]',
      default_use_auto_group: false,
      group_special_usable_group: '{}',
      group_routing_requirements: '{}',
    })
  } finally {
    api.defaults.adapter = previousAdapter
  }
})
