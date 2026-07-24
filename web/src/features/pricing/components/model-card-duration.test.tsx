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
