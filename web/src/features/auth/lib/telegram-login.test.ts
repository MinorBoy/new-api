import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import { pickTelegramAuthorization } from './telegram-login'

describe('Telegram login authorization', () => {
  test('keeps only fields signed by the Telegram login contract', () => {
    assert.deepEqual(
      pickTelegramAuthorization({
        id: 12345,
        first_name: 'Test',
        last_name: 'User',
        username: 'test_user',
        photo_url: 'https://t.me/i/userpic/320/test.jpg',
        auth_date: 1_900_000_000,
        hash: 'signed-hash',
        lang: 'en',
        admin: true,
        redirect: 'https://attacker.example',
      }),
      {
        id: 12345,
        first_name: 'Test',
        last_name: 'User',
        username: 'test_user',
        photo_url: 'https://t.me/i/userpic/320/test.jpg',
        auth_date: 1_900_000_000,
        hash: 'signed-hash',
        lang: 'en',
      }
    )
  })

  test('rejects incomplete or structurally invalid callbacks', () => {
    assert.equal(pickTelegramAuthorization(null), null)
    assert.equal(
      pickTelegramAuthorization({ auth_date: 1, hash: 'hash' }),
      null
    )
    assert.equal(
      pickTelegramAuthorization({ id: 1, auth_date: 1, hash: '' }),
      null
    )
    assert.equal(
      pickTelegramAuthorization({ id: {}, auth_date: 1, hash: 'hash' }),
      null
    )
  })
})
