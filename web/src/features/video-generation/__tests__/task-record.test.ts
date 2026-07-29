import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import { appendTaskRecord, updateTaskRecord } from '../lib/task-record'
import type { VideoTaskRecord } from '../types'

const record = (clientId: string): VideoTaskRecord => ({
  clientId,
  taskId: '',
  status: 'queued',
  request: {
    model: 'seedance',
    content: [],
    ratio: '16:9',
    duration: 11,
    generate_audio: true,
    watermark: false,
  },
  createdAt: 1,
  apiKey: 'secret',
})

describe('video task records', () => {
  test('appends independent records without replacing an existing task', () => {
    const first = record('one')
    const second = record('two')
    assert.deepEqual(appendTaskRecord([first], second), [first, second])
  })

  test('updates only the selected task record', () => {
    const result = updateTaskRecord([record('one'), record('two')], 'two', {
      taskId: 'remote-2',
      status: 'succeeded',
    })
    assert.equal(result[0]?.taskId, '')
    assert.equal(result[1]?.taskId, 'remote-2')
    assert.equal(result[1]?.status, 'succeeded')
  })
})
