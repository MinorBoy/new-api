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
*/
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
