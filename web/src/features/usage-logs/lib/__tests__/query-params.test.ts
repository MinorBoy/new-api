import assert from 'node:assert/strict'
import { test } from 'node:test'

import { buildSearchParams } from '../filter'
import { buildQueryParams } from '../query-params'
import { buildApiParams, buildTaskApiParams } from '../utils'

test('buildQueryParams preserves zero while omitting empty values', () => {
  const params = buildQueryParams({
    page: 0,
    empty: '',
    absent: undefined,
    nullable: null,
    model: 'seedance',
  })

  assert.deepEqual(
    [...params.entries()],
    [
      ['page', '0'],
      ['model', 'seedance'],
    ]
  )
})

test('task filters map to stable URL search fields', () => {
  const params = buildSearchParams(
    {
      taskId: 'task-1',
      channel: '40',
      status: 'SUCCESS',
      requestModel: 'doubao-seedance',
      userId: '10',
    },
    'task'
  )

  assert.deepEqual(params, {
    channel: '40',
    filter: 'task-1',
    status: 'SUCCESS',
    requestModel: 'doubao-seedance',
    userId: '10',
  })
})

test('task URL fields map to admin API query parameters before fetching', () => {
  const params = buildTaskApiParams({
    page: 2,
    pageSize: 50,
    isAdmin: true,
    searchParams: {
      channel: '40',
      filter: 'task-1',
      status: 'SUCCESS',
      requestModel: 'doubao-seedance',
      userId: '10',
      startTime: 100_000,
      endTime: 200_000,
    },
  })

  assert.deepEqual(params, {
    p: 2,
    page_size: 50,
    channel_id: '40',
    task_id: 'task-1',
    status: 'SUCCESS',
    request_model: 'doubao-seedance',
    user_id: '10',
    start_timestamp: 100,
    end_timestamp: 200,
  })
})

test('task API parameters omit the admin-only user filter in self view', () => {
  const params = buildTaskApiParams({
    page: 1,
    pageSize: 20,
    isAdmin: false,
    searchParams: { userId: '10' },
  })

  assert.equal(params.user_id, undefined)
})

test('common self API parameters omit every supplier dimension', () => {
  const params = buildApiParams({
    page: 1,
    pageSize: 20,
    isAdmin: false,
    searchParams: {
      model: 'public-model',
      token: 'user-token',
      group: 'internal-group',
      channel: '40',
      username: 'alice',
      requestId: 'req-public',
      upstreamRequestId: 'upstream-secret',
    },
  })

  assert.equal(params.model_name, 'public-model')
  assert.equal(params.token_name, 'user-token')
  assert.equal(params.request_id, 'req-public')
  assert.equal(params.group, undefined)
  assert.equal(params.channel, undefined)
  assert.equal(params.username, undefined)
  assert.equal(params.upstream_request_id, undefined)
})

test('common administrator API parameters retain audit dimensions', () => {
  const params = buildApiParams({
    page: 1,
    pageSize: 20,
    isAdmin: true,
    searchParams: {
      group: 'internal-group',
      channel: '40',
      username: 'alice',
      upstreamRequestId: 'upstream-secret',
    },
  })

  assert.equal(params.group, 'internal-group')
  assert.equal(params.channel, 40)
  assert.equal(params.username, 'alice')
  assert.equal(params.upstream_request_id, 'upstream-secret')
})
