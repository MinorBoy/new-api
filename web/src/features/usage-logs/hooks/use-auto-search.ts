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
