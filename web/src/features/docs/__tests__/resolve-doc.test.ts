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
  test('resolves the home slug to the overview page', () => {
    const result = resolveDoc(docsHomeSlug, 'en')
    assert.equal(result.found, true)
    if (result.found) {
      assert.equal(result.doc.slug, 'overview')
      assert.equal(result.doc.title, 'Overview')
      // English body must be a non-empty string from the markdown import.
      assert.ok(result.doc.body.length > 0)
    }
  })

  test('falls back to English body when the zh source is missing', () => {
    const result = resolveDoc('overview', 'zh')
    assert.equal(result.found, true)
    if (result.found) {
      assert.equal(result.doc.title, '使用概览')
      // The zh content is authored, so it should differ from a placeholder.
      assert.ok(result.doc.body.length > 0)
    }
  })

  test('returns the localized title per locale', () => {
    const en = resolveDoc('endpoints', 'en')
    const zh = resolveDoc('endpoints', 'zh')
    assert.equal(en.found, true)
    assert.equal(zh.found, true)
    if (en.found && zh.found) {
      assert.equal(en.doc.title, 'Endpoints')
      assert.equal(zh.doc.title, '接口地址')
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
    // error-codes is the last page of the api-reference group; next is the
    // first page of the clients group.
    const { prev, next } = getDocNeighbors('error-codes', 'en')
    assert.equal(prev?.slug, 'pricing')
    assert.equal(next?.slug, 'clients/curl')
  })

  test('first page has no previous neighbor', () => {
    const { prev } = getDocNeighbors(docsHomeSlug, 'en')
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
  test('documents the three public Seedance IDs without internal IDs', () => {
    const publicModels = [
      'doubao-seedance-2-0-260128',
      'doubao-seedance-2-0-fast-260128',
      'doubao-seedance-2-0-mini-260615',
    ]

    for (const locale of ['en', 'zh'] as const) {
      const result = resolveDoc('pricing', locale)
      assert.equal(result.found, true)
      if (!result.found) continue

      for (const model of publicModels) {
        assert.match(result.doc.body, new RegExp(`\\b${model}\\b`))
      }
      assert.doesNotMatch(result.doc.body, /doubao-seedance-2-0-mini-260128/)
      assert.doesNotMatch(result.doc.body, /(?:mg|bb|lec|jimeng)-seedance/i)
    }
  })

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
})
