import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import { resolveLegacyRoute } from './legacy-route'

describe('legacy frontend route migration', () => {
  test('maps former public and console routes to their current destinations', () => {
    const routes = {
      '/login': '/sign-in',
      '/forbidden': '/403',
      '/console': '/dashboard',
      '/console/models': '/models',
      '/console/deployment': '/models/deployments',
      '/console/subscription': '/subscriptions',
      '/console/channel': '/channels',
      '/console/token': '/keys',
      '/console/playground': '/playground',
      '/console/redemption': '/redemption-codes',
      '/console/user': '/users',
      '/console/personal': '/profile',
      '/console/log': '/usage-logs',
      '/console/midjourney': '/usage-logs/drawing',
      '/console/task': '/usage-logs/task',
      '/console/chat/42': '/chat/42',
    }

    for (const [source, target] of Object.entries(routes)) {
      assert.equal(resolveLegacyRoute(source), target)
    }
  })

  test('preserves search and hash while applying route-specific behavior', () => {
    assert.equal(
      resolveLegacyRoute('/login?redirect=%2Fkeys#continue'),
      '/sign-in?redirect=%2Fkeys#continue'
    )
    assert.equal(
      resolveLegacyRoute('/console/topup?source=email#orders'),
      '/wallet?source=email#orders'
    )
  })

  test('maps legacy settings tabs and retains unrelated parameters', () => {
    const settingsTabs = {
      operation: '/system-settings/operations/behavior',
      dashboard: '/system-settings/content/dashboard',
      chats: '/system-settings/content/chat',
      drawing: '/system-settings/content/drawing',
      payment: '/system-settings/billing/payment',
      ratio: '/system-settings/billing/model-pricing',
      ratelimit: '/system-settings/security/rate-limit',
      models: '/system-settings/models/global',
      'model-deployment': '/system-settings/models/model-deployment',
      performance: '/system-settings/operations/performance',
      system: '/system-settings/site/system-info',
      other: '/system-settings/site/system-info',
    }

    for (const [tab, target] of Object.entries(settingsTabs)) {
      assert.equal(
        resolveLegacyRoute(`/console/setting?tab=${tab}&from=bookmark#form`),
        `${target}?tab=${tab}&from=bookmark#form`
      )
    }
    assert.equal(
      resolveLegacyRoute('/console/setting?tab=unknown'),
      '/system-settings?tab=unknown'
    )
  })

  test('safely redirects unknown console locations without touching new routes', () => {
    assert.equal(
      resolveLegacyRoute('/console/removed?page=2#old'),
      '/dashboard?page=2#old'
    )
    assert.equal(resolveLegacyRoute('/dashboard'), null)
    assert.equal(resolveLegacyRoute('/api/status'), null)
  })
})
