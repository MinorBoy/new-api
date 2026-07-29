import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import {
  getVideoTaskId,
  getVideoTaskStatus,
  getVideoTaskResult,
} from '../lib/task-state'

describe('video task response normalization', () => {
  test('normalizes an ARK task response', () => {
    const response = {
      id: 'task-1',
      status: 'succeeded',
      content: { video_url: 'https://cdn.example/video.mp4' },
    }

    assert.equal(getVideoTaskId(response), 'task-1')
    assert.equal(getVideoTaskStatus(response), 'succeeded')
    assert.deepEqual(getVideoTaskResult(response), {
      videoUrl: 'https://cdn.example/video.mp4',
      lastFrameUrl: undefined,
    })
  })

  test('normalizes nested data and in-progress statuses', () => {
    const response = {
      data: {
        id: 'task-2',
        status: 'in_progress',
        content: { last_frame_url: 'https://cdn.example/frame.jpg' },
      },
    }

    assert.equal(getVideoTaskId(response), 'task-2')
    assert.equal(getVideoTaskStatus(response), 'running')
    assert.deepEqual(getVideoTaskResult(response), {
      videoUrl: undefined,
      lastFrameUrl: 'https://cdn.example/frame.jpg',
    })
  })
})
