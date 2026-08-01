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

For commercial licensing, please contact support@quantumnous.com
*/
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import {
  findEndpointBySlug,
  getCategoryTitle,
  groupEndpointsByCategory,
  filterEndpoints,
} from '../lib/api-endpoints-helpers'
import { apiEndpoints } from '../lib/api-endpoints'

describe('endpoint catalog integrity', () => {
  test('every endpoint has a unique slug, valid method, path, and code samples', () => {
    const slugs = apiEndpoints.map((e) => e.slug)
    assert.equal(new Set(slugs).size, slugs.length, 'slugs must be unique')
    for (const e of apiEndpoints) {
      assert.ok(e.path.startsWith('/'), `path must be absolute: ${e.path}`)
      assert.ok(['GET', 'POST', 'DELETE', 'PUT', 'PATCH'].includes(e.method))
      assert.ok(e.codeSamples.length > 0, `endpoint ${e.slug} needs a code sample`)
      assert.ok(e.title.en && e.summary.en, `endpoint ${e.slug} needs en title/summary`)
    }
  })
})

describe('groupEndpointsByCategory', () => {
  const groups = groupEndpointsByCategory()

  test('returns non-empty groups in display order', () => {
    assert.ok(groups.length > 0)
    // chat should come before images (display order)
    const chatIdx = groups.findIndex((g) => g.id === 'chat')
    const imagesIdx = groups.findIndex((g) => g.id === 'images')
    assert.ok(chatIdx < imagesIdx)
  })

  test('every endpoint appears in exactly one group', () => {
    const grouped = groups.flatMap((g) => g.endpoints)
    assert.equal(grouped.length, apiEndpoints.length)
  })
})

describe('findEndpointBySlug', () => {
  test('finds a known endpoint', () => {
    const e = findEndpointBySlug('chat-completions')
    assert.ok(e)
    assert.equal(e!.path, '/v1/chat/completions')
  })

  test('returns undefined for unknown slug', () => {
    assert.equal(findEndpointBySlug('nope'), undefined)
  })
})

describe('getCategoryTitle', () => {
  test('returns localized category titles', () => {
    assert.equal(getCategoryTitle('chat', 'en'), 'Chat')
    assert.equal(getCategoryTitle('chat', 'zh'), '对话')
  })
})

describe('filterEndpoints', () => {
  test('returns all when query is empty', () => {
    assert.equal(filterEndpoints('').length, apiEndpoints.length)
  })

  test('matches by path', () => {
    const results = filterEndpoints('/v1/chat')
    assert.ok(results.every((e) => e.path.includes('/v1/chat')))
  })

  test('matches by title across locales', () => {
    const zhResults = filterEndpoints('对话')
    assert.ok(zhResults.some((e) => e.slug === 'chat-completions'))
    const enResults = filterEndpoints('embeddings')
    assert.ok(enResults.some((e) => e.slug === 'embeddings'))
  })

  test('is case-insensitive', () => {
    assert.equal(filterEndpoints('CURL').length, 0) // curl appears in code, not titles
    const results = filterEndpoints('CHAT')
    assert.ok(results.length > 0)
  })
})
