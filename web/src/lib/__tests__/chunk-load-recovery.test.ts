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

import { installChunkLoadRecovery } from '../chunk-load-recovery'

type Listener = (event: Record<string, unknown>) => void

function createTarget(marker = '') {
  const listeners = new Map<string, Listener>()
  const storage = new Map<string, string>(
    marker ? [['newapi:chunk-recovery', marker]] : []
  )
  let reloads = 0

  const target = {
    sessionStorage: {
      getItem: (key: string) => storage.get(key) ?? null,
      setItem: (key: string, value: string) => storage.set(key, value),
      removeItem: (key: string) => storage.delete(key),
    },
    location: {
      reload: () => {
        reloads += 1
      },
    },
    addEventListener: (type: string, listener: Listener) =>
      listeners.set(type, listener),
    removeEventListener: (type: string) => listeners.delete(type),
    setTimeout: () => 1,
    clearTimeout: () => undefined,
  }

  return {
    target: target as unknown as Window,
    emit: (type: string, event: Record<string, unknown>) =>
      listeners.get(type)?.(event),
    get reloads() {
      return reloads
    },
    get marker() {
      return storage.get('newapi:chunk-recovery') ?? null
    },
  }
}

test('reloads once when a stale dynamic chunk fails to load', () => {
  const harness = createTarget()
  const cleanup = installChunkLoadRecovery(harness.target)

  harness.emit('error', {
    error: { name: 'ChunkLoadError', message: 'Loading chunk 344 failed' },
  })
  harness.emit('unhandledrejection', {
    reason: new Error('Failed to fetch dynamically imported module'),
  })

  assert.equal(harness.reloads, 1)
  assert.equal(harness.marker, '1')
  cleanup()
})

test('does not reload again when the recovery marker is already set', () => {
  const harness = createTarget('1')
  const cleanup = installChunkLoadRecovery(harness.target)

  harness.emit('error', { message: 'Loading chunk 12 failed' })

  assert.equal(harness.reloads, 0)
  cleanup()
})
