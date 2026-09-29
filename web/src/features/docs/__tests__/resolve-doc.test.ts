import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import {
  getDocNeighbors,
  normalizeDocSlug,
  resolveDoc,
  resolveDocLocale,
} from '../lib/resolve-doc'
import { docsHomeSlug, flatDocEntries } from '../manifest'

describe('normalizeDocSlug', () => {
  test('falls back to the docs home slug for empty input', () => {
    assert.equal(normalizeDocSlug(undefined), docsHomeSlug)
    assert.equal(normalizeDocSlug(''), docsHomeSlug)
  })

  test('trims slashes and lowercases', () => {
    assert.equal(normalizeDocSlug('Quickstart/'), 'quickstart')
    assert.equal(normalizeDocSlug('/clients/curl/'), 'clients/curl')
  })
})

describe('resolveDoc', () => {
  test('resolves the home slug to the reference catalog entry', () => {
    const result = resolveDoc(docsHomeSlug, 'en')
    assert.equal(result.found, true)
    if (result.found) {
      assert.equal(result.doc.slug, 'reference')
      assert.equal(result.doc.title, 'Endpoints')
    }
  })

  test('resolves a guide page with localized content', () => {
    const result = resolveDoc('overview', 'zh')
    assert.equal(result.found, true)
    if (result.found) {
      assert.equal(result.doc.title, '使用概览')
      // The zh content is authored, so it should differ from a placeholder.
      assert.ok(result.doc.body.length > 0)
    }
  })

  test('returns the localized title per locale', () => {
    const en = resolveDoc('pricing', 'en')
    const zh = resolveDoc('pricing', 'zh')
    assert.equal(en.found, true)
    assert.equal(zh.found, true)
    if (en.found && zh.found) {
      assert.equal(en.doc.title, 'Models & Pricing')
      assert.equal(zh.doc.title, '模型与计费')
    }
  })

  test('reports not-found for an unknown slug', () => {
    const result = resolveDoc('does-not-exist', 'en')
    assert.equal(result.found, false)
  })

  test('resolves nested slugs under subfolders', () => {
    const result = resolveDoc('clients/curl', 'en')
    assert.equal(result.found, true)
    if (result.found) {
      assert.equal(result.doc.slug, 'clients/curl')
    }
  })
})

describe('getDocNeighbors', () => {
  test('links across group boundaries in sidebar order', () => {
    // minimax-h3-video is the last page of the api-reference group; next is
    // the first page of the clients group.
    const { prev, next } = getDocNeighbors('minimax-h3-video', 'en')
    assert.equal(prev?.slug, 'error-codes')
    assert.equal(next?.slug, 'clients/curl')
  })

  test('first page has no previous neighbor', () => {
    // overview is the first entry in the flat sidebar order.
    const { prev } = getDocNeighbors('overview', 'en')
    assert.equal(prev, null)
  })

  test('last page has no next neighbor', () => {
    const { next } = getDocNeighbors('billing-rules', 'en')
    assert.equal(next, null)
  })

  test('returns null neighbors for an unknown slug', () => {
    const { prev, next } = getDocNeighbors('missing', 'en')
    assert.equal(prev, null)
    assert.equal(next, null)
  })
})

describe('resolveDocLocale', () => {
  test('returns the zh value when present', () => {
    assert.equal(
      resolveDocLocale({ en: 'Overview', zh: '使用概览' }, 'zh'),
      '使用概览'
    )
  })

  test('falls back to English when zh is absent', () => {
    assert.equal(resolveDocLocale({ en: 'Overview' }, 'zh'), 'Overview')
    assert.equal(resolveDocLocale({ en: 'Overview' }, 'en'), 'Overview')
  })

  test('returns empty string for undefined input', () => {
    assert.equal(resolveDocLocale(undefined, 'en'), '')
  })
})

describe('public documentation boundary', () => {
  test('keeps non-Seedance providers and protocols in public documentation', () => {
    const bodies = flatDocEntries
      .flatMap((entry) => [entry.page.content.en, entry.page.content.zh ?? ''])
      .join('\n')

    for (const expected of [
      'gpt-4o-mini',
      'Anthropic',
      '/v1/chat/completions',
    ]) {
      assert.ok(
        bodies.includes(expected),
        `must continue to publish ${expected}`
      )
    }
  })

  test('publishes the MiniMax H3 video guide with canonical model only', () => {
    const entry = flatDocEntries.find(
      ({ page }) => page.slug === 'minimax-h3-video'
    )
    assert.ok(entry, 'minimax-h3-video page must be registered')
    const zh = entry.page.content.zh ?? ''
    const en = entry.page.content.en ?? ''
    // Downstream docs teach the single canonical client model and the unified
    // video entry; per-channel upstream IDs stay private to routing.
    for (const expected of ['MiniMax-H3', '/v1/video/generations']) {
      assert.ok(zh.includes(expected), `zh guide must mention ${expected}`)
      assert.ok(en.includes(expected), `en guide must mention ${expected}`)
    }
    for (const upstream of [
      'minimax-h3-vip',
      'lec-minimax-h3',
      'mm2-minimax-h3',
    ]) {
      assert.ok(!zh.includes(upstream), `zh guide must not leak ${upstream}`)
      assert.ok(!en.includes(upstream), `en guide must not leak ${upstream}`)
    }
  })
})
