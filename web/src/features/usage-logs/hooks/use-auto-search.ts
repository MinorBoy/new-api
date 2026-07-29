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
import { useCallback, useEffect, useRef } from 'react'

type TimerHandle = unknown

export type AutoSearchController<TValue> = {
  schedule: (value: TValue) => void
  flush: (value: TValue) => void
  cancel: () => void
}

/**
 * Coordinates immediate selections and debounced free-text filtering.
 */
export function createAutoSearchController<TValue>(
  onSearch: (value: TValue) => void,
  delay: number = 350,
  scheduleTimer: (callback: () => void, delayMs: number) => TimerHandle = (
    callback,
    delayMs
  ) => setTimeout(callback, delayMs),
  cancelTimer: (timer: TimerHandle) => void = (timer) =>
    clearTimeout(timer as ReturnType<typeof setTimeout>)
): AutoSearchController<TValue> {
  let timer: TimerHandle | undefined

  const cancel = () => {
    if (timer === undefined) return
    cancelTimer(timer)
    timer = undefined
  }

  return {
    schedule: (value) => {
      cancel()
      timer = scheduleTimer(() => {
        timer = undefined
        onSearch(value)
      }, delay)
    },
    flush: (value) => {
      cancel()
      onSearch(value)
    },
    cancel,
  }
}

export function useAutoSearch<TValue>(
  onSearch: (value: TValue) => void,
  delay: number = 350
): AutoSearchController<TValue> {
  const onSearchRef = useRef(onSearch)
  const controllerRef = useRef<AutoSearchController<TValue> | null>(null)
  const delayRef = useRef(delay)

  useEffect(() => {
    onSearchRef.current = onSearch
  }, [onSearch])

  if (controllerRef.current === null || delayRef.current !== delay) {
    controllerRef.current?.cancel()
    delayRef.current = delay
    controllerRef.current = createAutoSearchController(
      (value) => onSearchRef.current(value),
      delay
    )
  }

  const controller = controllerRef.current

  useEffect(() => () => controller.cancel(), [controller])

  return {
    schedule: useCallback(
      (value: TValue) => controller.schedule(value),
      [controller]
    ),
    flush: useCallback(
      (value: TValue) => controller.flush(value),
      [controller]
    ),
    cancel: useCallback(() => controller.cancel(), [controller]),
  }
}
