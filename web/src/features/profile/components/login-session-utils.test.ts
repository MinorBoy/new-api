import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import type { TFunction } from 'i18next'

import { loginMethodLabel, sessionDevice } from './login-session-utils'

const translate = ((key: string) => key) as TFunction

describe('login session presentation', () => {
  test('labels built-in and provider OAuth login methods', () => {
    assert.equal(loginMethodLabel('password', translate), 'Password')
    assert.equal(
      loginMethodLabel('2fa', translate),
      'Two-factor Authentication'
    )
    assert.equal(loginMethodLabel('oauth:github', translate), 'OAuth · GitHub')
    assert.equal(
      loginMethodLabel('oauth:custom-provider', translate),
      'OAuth · custom-provider'
    )
  })

  test('derives a stable browser and operating-system label', () => {
    assert.equal(
      sessionDevice(
        'Mozilla/5.0 (Macintosh; Intel Mac OS X) AppleWebKit Safari/605.1.15',
        'Unknown device',
        'Browser'
      ),
      'Safari · macOS'
    )
    assert.equal(
      sessionDevice('', 'Unknown device', 'Browser'),
      'Unknown device'
    )
  })
})
