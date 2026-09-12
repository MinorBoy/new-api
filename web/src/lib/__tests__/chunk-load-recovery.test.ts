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
