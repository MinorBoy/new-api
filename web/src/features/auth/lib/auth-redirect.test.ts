import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import type { AuthUser } from '@/stores/auth-store'

import { getSavedLanguage, sanitizeAuthRedirect } from './auth-redirect'

const origin = 'https://dashboard.example.com'

describe('authentication redirect validation', () => {
  test('preserves safe internal paths, search parameters, and fragments', () => {
    assert.equal(
      sanitizeAuthRedirect('/console?tab=usage#recent', origin),
      '/console?tab=usage#recent'
    )
    assert.equal(
      sanitizeAuthRedirect(
        'https://dashboard.example.com/dashboard?tab=quota#daily',
        origin
      ),
      '/dashboard?tab=quota#daily'
    )
  })

  test('rejects external and ambiguously parsed redirect targets', () => {
    const unsafeTargets: unknown[] = [
      undefined,
      '',
      'dashboard',
      '//attacker.example/path',
      'https://attacker.example/path',
      'javascript:alert(1)',
      '/\\attacker.example/path',
      'https:\\attacker.example/path',
    ]

    for (const target of unsafeTargets) {
      assert.equal(sanitizeAuthRedirect(target, origin), null)
    }
  })

  test('rejects invalid or non-HTTP application origins', () => {
    assert.equal(sanitizeAuthRedirect('/dashboard', 'not-an-origin'), null)
    assert.equal(sanitizeAuthRedirect('/dashboard', 'file:///tmp/app'), null)
  })
})

describe('saved authentication language', () => {
  const user: AuthUser = { id: 1, username: 'user', role: 1 }

  test('prefers the explicit user language', () => {
    assert.equal(
      getSavedLanguage({
        ...user,
        language: 'ja',
        setting: { language: 'fr' },
      }),
      'ja'
    )
  })

  test('reads object and JSON string settings', () => {
    assert.equal(
      getSavedLanguage({ ...user, setting: { language: 'fr' } }),
      'fr'
    )
    assert.equal(
      getSavedLanguage({ ...user, setting: '{"language":"ru"}' }),
      'ru'
    )
  })

  test('ignores malformed and non-string setting languages', () => {
    assert.equal(getSavedLanguage({ ...user, setting: '{' }), undefined)
    assert.equal(
      getSavedLanguage({ ...user, setting: { language: 123 } }),
      undefined
    )
  })
})
