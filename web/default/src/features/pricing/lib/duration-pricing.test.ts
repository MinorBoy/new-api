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

import { QUOTA_TYPES, SORT_OPTIONS } from '../constants'
import type { PricingModel } from '../types'
import { filterByQuotaType, sortModels } from './filters'
import { isDurationBasedModel } from './model-helpers'
import {
  formatDurationPrice,
  formatFixedPrice,
  formatRequestPrice,
} from './price'

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

test('identifies only models with duration billing metadata', () => {
  assert.equal(isDurationBasedModel(durationModel), true)
  assert.equal(
    isDurationBasedModel({ ...durationModel, billing_mode: 'ratio' }),
    false
  )
  assert.equal(
    isDurationBasedModel({ ...durationModel, duration_price: undefined }),
    false
  )
})

test('formats duration prices with group and recharge adjustments', () => {
  const groupedModel: PricingModel = {
    ...durationModel,
    enable_groups: ['default', 'vip'],
    group_ratio: { default: 1, vip: 2 },
  }

  assert.equal(formatDurationPrice(groupedModel), '$0.25')
  assert.equal(formatDurationPrice(groupedModel, false, 1, 1, 'vip'), '$0.5')
  assert.equal(formatDurationPrice(groupedModel, true, 0.5, 1, 'vip'), '$0.25')
  assert.equal(
    formatDurationPrice({ ...groupedModel, duration_price: undefined }),
    '-'
  )
})

test('does not format duration models as per-request prices', () => {
  assert.equal(formatRequestPrice(durationModel), '-')
  assert.equal(
    formatFixedPrice(durationModel, 'default', false, 1, 1, { default: 1 }),
    '-'
  )
})

test('filters duration models separately from per-request models', () => {
  const requestModel: PricingModel = {
    ...durationModel,
    id: 2,
    model_name: 'image-request',
    billing_mode: 'ratio',
    duration_price: undefined,
    model_price: 0.5,
  }
  const tokenModel: PricingModel = {
    ...requestModel,
    id: 3,
    model_name: 'chat-token',
    quota_type: 0,
    model_ratio: 1,
  }
  const models = [durationModel, requestModel, tokenModel]

  assert.equal(QUOTA_TYPES.DURATION, 'duration')
  assert.deepEqual(filterByQuotaType(models, QUOTA_TYPES.DURATION), [
    durationModel,
  ])
  assert.deepEqual(filterByQuotaType(models, QUOTA_TYPES.REQUEST), [
    requestModel,
  ])
})

test('sorts duration models by their duration unit price', () => {
  const lowerDurationPrice: PricingModel = {
    ...durationModel,
    id: 2,
    model_name: 'lower-duration',
    model_price: 100,
    duration_price: { ...durationModel.duration_price, price: 0.1 },
  }
  const higherDurationPrice: PricingModel = {
    ...durationModel,
    id: 3,
    model_name: 'higher-duration',
    model_price: 0,
    duration_price: { ...durationModel.duration_price, price: 0.4 },
  }

  assert.deepEqual(
    sortModels(
      [higherDurationPrice, lowerDurationPrice],
      SORT_OPTIONS.PRICE_LOW
    ).map((model) => model.model_name),
    ['lower-duration', 'higher-duration']
  )
})
