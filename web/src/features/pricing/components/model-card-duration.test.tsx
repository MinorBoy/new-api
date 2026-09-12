import assert from 'node:assert/strict'
import test from 'node:test'

import { createInstance } from 'i18next'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { I18nextProvider } from 'react-i18next'

import type { PricingModel } from '../types'
import { ModelCard } from './model-card'

const i18n = createInstance()
await i18n.init({
  lng: 'en',
  fallbackLng: 'en',
  resources: { en: { translation: {} } },
})

const durationModel = {
  id: 1,
  model_name: 'video-duration',
  quota_type: 1,
  model_ratio: 0,
  completion_ratio: 0,
  enable_groups: ['default'],
  billing_mode: 'per_duration',
  duration_price: {
    price: 0.25,
    unit: 'minute',
    rounding_step_seconds: 5,
    minimum_duration_seconds: 10,
  },
} satisfies PricingModel

function renderCard(model: PricingModel): string {
  return renderToStaticMarkup(
    createElement(
      I18nextProvider,
      { i18n },
      createElement(ModelCard, {
        model,
        tokenUnit: 'M',
        onClick: () => {},
      })
    )
  )
}

test('duration cards show duration units without a token-unit footer', () => {
  const html = renderCard(durationModel)

  assert.match(html, /\$0\.25/)
  assert.match(html, /\/ minute/)
  assert.equal(html.includes('1M'), false)
  assert.doesNotMatch(
    html,
    /text-muted-foreground whitespace-nowrap"><span class="text-foreground font-mono font-semibold">\$0\.25/
  )
})

test('malformed duration cards stay in duration mode and render safely', () => {
  const html = renderCard({ ...durationModel, duration_price: undefined })

  assert.match(html, /Duration-based/)
  assert.match(html, />-</)
  assert.doesNotMatch(html, /\/ request/)
  assert.equal(html.includes('1M'), false)
})
