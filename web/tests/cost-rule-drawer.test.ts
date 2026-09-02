import { describe, expect, test } from 'bun:test'

import { costRuleRequestPath } from '../src/features/cost-accounting/components/cost-rule-drawer'

describe('cost rule request path', () => {
  test('uses the image generations path for per-image rules', () => {
    expect(costRuleRequestPath('per_image', false)).toBe(
      '/v1/images/generations'
    )
  })

  test('keeps the standard path for non-image rules', () => {
    expect(costRuleRequestPath('per_request', false)).toBe(
      '/v1/chat/completions'
    )
  })
})
