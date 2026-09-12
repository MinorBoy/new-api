import assert from 'node:assert/strict'
import test from 'node:test'

import { createAutoSearchController } from '../use-auto-search'

test('submits only the latest typed filter value after the debounce period', () => {
  const submitted: string[] = []
  const timers = new Map<number, () => void>()
  let nextTimerId = 1
  const controller = createAutoSearchController(
    (value: string) => submitted.push(value),
    350,
    (callback) => {
      const timerId = nextTimerId++
      timers.set(timerId, callback)
      return timerId
    },
    (timerId) => timers.delete(timerId as number)
  )

  controller.schedule('task_a')
  controller.schedule('task_ab')

  assert.equal(timers.size, 1)
  const timer = timers.values().next().value as (() => void) | undefined
  assert.ok(timer)
  timer()

  assert.deepEqual(submitted, ['task_ab'])
})

test('submits an explicit selection immediately and cancels queued text input', () => {
  const submitted: string[] = []
  const timers = new Map<number, () => void>()
  let nextTimerId = 1
  const controller = createAutoSearchController(
    (value: string) => submitted.push(value),
    350,
    (callback) => {
      const timerId = nextTimerId++
      timers.set(timerId, callback)
      return timerId
    },
    (timerId) => timers.delete(timerId as number)
  )

  controller.schedule('draft')
  controller.flush('selected')

  assert.deepEqual(submitted, ['selected'])
  assert.equal(timers.size, 0)
})
