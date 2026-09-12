import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import {
  parseTelegramBindCallback,
  postTelegramBindResult,
  startOAuthBindResponseDeadline,
  watchOAuthPopupClosed,
} from './oauth-bind-window'

function fakeTimerRuntime() {
  let callback: (() => void) | undefined
  let delay = 0
  const cancelled: unknown[] = []
  const handle = Symbol('timer')
  return {
    runtime: {
      schedule: (scheduled: () => void, scheduledDelay: number) => {
        callback = scheduled
        delay = scheduledDelay
        return handle
      },
      cancel: (cancelledHandle: unknown) => cancelled.push(cancelledHandle),
    },
    fire: () => callback?.(),
    get delay() {
      return delay
    },
    cancelled,
    handle,
  }
}

describe('OAuth bind popup lifecycle', () => {
  test('parses Telegram success and stable error callbacks', () => {
    assert.deepEqual(
      parseTelegramBindCallback({
        telegram_bind: 'success',
        flow_token: 'flow-success',
      }),
      {
        kind: 'result',
        flowToken: 'flow-success',
        success: true,
      }
    )
    assert.deepEqual(
      parseTelegramBindCallback({
        telegram_bind: 'error',
        flow_token: 'flow-error',
        error_code: 'TELEGRAM_BIND_ALREADY_BOUND',
      }),
      {
        kind: 'result',
        flowToken: 'flow-error',
        success: false,
        code: 'TELEGRAM_BIND_ALREADY_BOUND',
      }
    )
  })

  test('rejects Telegram callbacks without a flow token and ignores descriptions', () => {
    assert.deepEqual(parseTelegramBindCallback({ telegram_bind: 'error' }), {
      kind: 'invalid',
    })
    assert.deepEqual(
      parseTelegramBindCallback({
        telegram_bind: 'error',
        flow_token: 'flow-error',
        error_code: 'UNKNOWN_CODE',
        error_description: 'untrusted message',
      } as Parameters<typeof parseTelegramBindCallback>[0]),
      {
        kind: 'result',
        flowToken: 'flow-error',
        success: false,
        code: 'UNKNOWN_CODE',
      }
    )
    assert.equal(parseTelegramBindCallback({}), null)
  })

  test('posts only complete Telegram bind results to an available opener', () => {
    const messages: Array<{ message: unknown; targetOrigin: string }> = []
    const opener = {
      closed: false,
      postMessage: (message: unknown, targetOrigin: string) => {
        messages.push({ message, targetOrigin })
      },
    } as Pick<Window, 'closed' | 'postMessage'>
    const callback = parseTelegramBindCallback({
      telegram_bind: 'error',
      flow_token: 'flow-error',
      error_code: 'UNKNOWN_CODE',
    })

    assert.equal(
      postTelegramBindResult(callback, opener, 'https://dashboard.example.com'),
      true
    )
    assert.deepEqual(messages, [
      {
        message: {
          type: 'telegram:binding:result',
          flow_token: 'flow-error',
          success: false,
          code: 'UNKNOWN_CODE',
        },
        targetOrigin: 'https://dashboard.example.com',
      },
    ])

    assert.equal(
      postTelegramBindResult(
        { kind: 'invalid' },
        opener,
        'https://example.com'
      ),
      false
    )
    assert.equal(
      postTelegramBindResult(
        callback,
        { ...opener, closed: true },
        'https://example.com'
      ),
      false
    )
    assert.equal(messages.length, 1)
  })

  test('waits 30 seconds for the opener response and can be cancelled', () => {
    const timer = fakeTimerRuntime()
    let timedOut = false
    const cancel = startOAuthBindResponseDeadline(
      () => {
        timedOut = true
      },
      undefined,
      timer.runtime
    )

    assert.equal(timer.delay, 30_000)
    cancel()
    timer.fire()
    assert.equal(timedOut, false)
    assert.deepEqual(timer.cancelled, [timer.handle])
  })

  test('reports a closed popup once and clears its poller', () => {
    const timer = fakeTimerRuntime()
    const popup = { closed: false }
    let closedCount = 0
    watchOAuthPopupClosed(
      popup,
      () => {
        closedCount += 1
      },
      undefined,
      timer.runtime
    )

    assert.equal(timer.delay, 500)
    timer.fire()
    assert.equal(closedCount, 0)
    popup.closed = true
    timer.fire()
    timer.fire()
    assert.equal(closedCount, 1)
    assert.deepEqual(timer.cancelled, [timer.handle])
  })
})
