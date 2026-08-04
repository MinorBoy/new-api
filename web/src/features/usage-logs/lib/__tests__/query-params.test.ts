/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import assert from 'node:assert/strict'
import { test } from 'node:test'

import { buildSearchParams } from '../filter'
import { buildQueryParams } from '../query-params'
import { buildTaskApiParams } from '../utils'

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
