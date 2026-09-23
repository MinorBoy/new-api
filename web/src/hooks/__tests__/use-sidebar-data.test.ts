import assert from 'node:assert/strict'
import { test } from 'node:test'

import { openFlyreqStudioWithPreparation } from '../use-sidebar-data'

test('reserves a blank FlyReq tab and loads it once with the prepared URL', async () => {
  const events: string[] = []
  let resolvePreparedUrl: (url: string) => void = () => undefined
  const preparedUrl = new Promise<string>((resolve) => {
    resolvePreparedUrl = resolve
  })
  const popup = {
    location: { href: 'about:blank' },
    close: () => {
      events.push('close')
    },
  } as unknown as Window

  const opening = openFlyreqStudioWithPreparation(
    () => preparedUrl,
    (url) => {
      events.push(`open:${url}`)
      return popup
    },
    'http://localhost:3001',
  )

  // The tab is reserved on about:blank so the studio never boots before the
  // provider payload is ready; a running studio cancels provider navigations.
  assert.deepEqual(events, ['open:about:blank'])
  assert.equal(popup.location.href, 'about:blank')

  resolvePreparedUrl('http://localhost:3001/zh/?provider=configured')
  await opening

  assert.equal(popup.location.href, 'http://localhost:3001/zh/?provider=configured')
  assert.equal(events.includes('close'), false)
})

test('loads the studio shell on the reserved tab when configuration preparation fails', async () => {
  const events: string[] = []
  const popup = {
    location: { href: 'about:blank' },
    close: () => {
      events.push('close')
    },
  } as unknown as Window

  await openFlyreqStudioWithPreparation(
    async () => {
      throw new Error('temporary configuration failure')
    },
    (url) => {
      events.push(`open:${url}`)
      return popup
    },
    'http://127.0.0.1:3001',
  )

  assert.deepEqual(events, ['open:about:blank'])
  assert.equal(popup.location.href, 'http://127.0.0.1:3001/zh/')
  assert.equal(events.includes('close'), false)
})
