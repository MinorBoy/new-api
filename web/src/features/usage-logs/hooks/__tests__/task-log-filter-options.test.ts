import assert from 'node:assert/strict'
import { test } from 'node:test'

import {
  buildTaskLogFilterOptionsQueryKey,
  normalizeTaskLogFilterOptions,
} from '../use-task-log-filter-options'

test('task filter options query key depends only on scope and time range', () => {
  const key = buildTaskLogFilterOptionsQueryKey(true, {
    start_timestamp: 100,
    end_timestamp: 400,
  })

  assert.deepEqual(key, ['usage-log-filter-options', 'task', true, 100, 400])
})

test('task filter options are deduplicated and sorted for stable dropdowns', () => {
  const options = normalizeTaskLogFilterOptions({
    channels: [
      { id: 40, name: '' },
      { id: 29, name: 'paipu' },
      { id: 40, name: 'stale duplicate' },
    ],
    statuses: ['SUCCESS', 'FAILURE', 'SUCCESS'],
    request_models: ['model-b', 'model-a', 'model-b'],
    users: [
      { id: 11, username: 'bob' },
      { id: 10, username: 'alice' },
      { id: 11, username: 'bob' },
    ],
  })

  assert.deepEqual(options.channelOptions, [
    { value: '29', label: '29 - paipu' },
    { value: '40', label: '40' },
  ])
  assert.deepEqual(options.statusOptions, ['FAILURE', 'SUCCESS'])
  assert.deepEqual(options.requestModelOptions, [
    { value: 'model-a', label: 'model-a' },
    { value: 'model-b', label: 'model-b' },
  ])
  assert.deepEqual(options.userOptions, [
    { value: '10', label: '10 - alice' },
    { value: '11', label: '11 - bob' },
  ])
})
