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
const CHUNK_RECOVERY_KEY = 'newapi:chunk-recovery'
const CHUNK_RECOVERY_WINDOW_MS = 10_000
const CHUNK_LOAD_ERROR =
  /ChunkLoadError|Loading chunk .+ failed|Failed to fetch dynamically imported module|Importing a module script failed/i

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

function isChunkLoadFailure(value: unknown): boolean {
  if (typeof value === 'string') return CHUNK_LOAD_ERROR.test(value)
  if (!isRecord(value)) return false

  for (const field of ['name', 'message', 'stack'] as const) {
    if (
      typeof value[field] === 'string' &&
      CHUNK_LOAD_ERROR.test(value[field])
    ) {
      return true
    }
  }

  return isChunkLoadFailure(value.error) || isChunkLoadFailure(value.reason)
}

function readRecoveryMarker(target: Window) {
  try {
    return target.sessionStorage.getItem(CHUNK_RECOVERY_KEY)
  } catch {
    return null
  }
}

function clearRecoveryMarker(target: Window) {
  try {
    target.sessionStorage.removeItem(CHUNK_RECOVERY_KEY)
  } catch {
    /* Storage can be unavailable in privacy-restricted browsers. */
  }
}

export function installChunkLoadRecovery(target: Window = window) {
  let recoveryTriggered = false
  let markerTimer: number | undefined

  if (readRecoveryMarker(target) === '1') {
    markerTimer = target.setTimeout(
      () => clearRecoveryMarker(target),
      CHUNK_RECOVERY_WINDOW_MS
    )
  }

  const recover = (value: unknown) => {
    if (
      recoveryTriggered ||
      readRecoveryMarker(target) === '1' ||
      !isChunkLoadFailure(value)
    ) {
      return
    }

    recoveryTriggered = true
    try {
      target.sessionStorage.setItem(CHUNK_RECOVERY_KEY, '1')
    } catch {
      /* Reload still gives the browser a chance to fetch the current build. */
    }
    target.location.reload()
  }

  const onError = (event: ErrorEvent) => recover(event.error ?? event.message)
  const onUnhandledRejection = (event: PromiseRejectionEvent) =>
    recover(event.reason)

  target.addEventListener('error', onError)
  target.addEventListener('unhandledrejection', onUnhandledRejection)

  return () => {
    target.removeEventListener('error', onError)
    target.removeEventListener('unhandledrejection', onUnhandledRejection)
    if (markerTimer !== undefined) target.clearTimeout(markerTimer)
  }
}
