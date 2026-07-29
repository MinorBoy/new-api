import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import { shouldScheduleNextVideoPoll } from '../lib/polling'

describe('video task polling', () => {
  test('stops automatic polling when a request fails or the page has unmounted', () => {
    assert.equal(
      shouldScheduleNextVideoPoll({
        isCurrent: true,
        status: 'running',
        requestFailed: true,
      }),
      false
    )
    assert.equal(
      shouldScheduleNextVideoPoll({
        isCurrent: false,
        status: 'running',
        requestFailed: false,
      }),
      false
    )
  })

  test('continues only the current non-terminal task', () => {
    assert.equal(
      shouldScheduleNextVideoPoll({
        isCurrent: true,
        status: 'running',
        requestFailed: false,
      }),
      true
    )
    assert.equal(
      shouldScheduleNextVideoPoll({
        isCurrent: true,
        status: 'succeeded',
        requestFailed: false,
      }),
      false
    )
  })
})
