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

import {
  buildModelPricingSubmitData,
  buildPreviewRows,
  createModelPricingFormValues,
  getInitialPricingMode,
  validateDurationPricingValues,
  type LaneKey,
  type ModelPricingFormValues,
  type PricingMode,
} from './model-pricing-core'
import {
  buildModelSnapshots,
  deleteModelPricingFromMaps,
  getPriceDetail,
  getPriceSummary,
  getSnapshotSignature,
  isBasePricingUnset,
  updateModelPricingMaps,
  type ModelPricingSnapshotInput,
} from './model-pricing-snapshots'
import { applyResolutionSelection } from './upstream-ratio-sync-helpers'

const rule = {
  price: 0.25,
  unit: 'minute' as const,
  rounding_step_seconds: 5,
  minimum_duration_seconds: 10,
}

const emptyInput: ModelPricingSnapshotInput = {
  modelPrice: '{}',
  modelRatio: '{}',
  cacheRatio: '{}',
  createCacheRatio: '{}',
  completionRatio: '{}',
  imageRatio: '{}',
  audioRatio: '{}',
  audioCompletionRatio: '{}',
  billingMode: '{}',
  billingExpr: '{}',
  durationPrice: '{}',
}

const emptyLanePrices: Record<LaneKey, string> = {
  completion: '',
  cache: '',
  createCache: '',
  image: '',
  audioInput: '',
  audioOutput: '',
}

const disabledLanes: Record<LaneKey, boolean> = {
  completion: false,
  cache: false,
  createCache: false,
  image: false,
  audioInput: false,
  audioOutput: false,
}

const durationFormValues: ModelPricingFormValues = {
  name: 'video',
  price: '',
  ratio: '',
  cacheRatio: '',
  createCacheRatio: '',
  completionRatio: '',
  imageRatio: '',
  audioRatio: '',
  audioCompletionRatio: '',
  durationPrice: '0.25',
  durationUnit: 'minute',
  roundingStepSeconds: '5',
  minimumDurationSeconds: '10',
}

test('hydrates all duration editor fields and selects duration mode', () => {
  const values = createModelPricingFormValues({
    name: 'video',
    billingMode: 'per_duration',
    durationPrice: rule,
  })

  assert.equal(
    getInitialPricingMode({
      name: 'video',
      billingMode: 'per_duration',
      durationPrice: rule,
    }),
    'per_duration'
  )
  assert.equal(values.durationPrice, '0.25')
  assert.equal(values.durationUnit, 'minute')
  assert.equal(values.roundingStepSeconds, '5')
  assert.equal(values.minimumDurationSeconds, '10')
})

test('uses safe duration defaults for a new pricing rule', () => {
  const values = createModelPricingFormValues()

  assert.equal(values.durationPrice, '')
  assert.equal(values.durationUnit, 'second')
  assert.equal(values.roundingStepSeconds, '1')
  assert.equal(values.minimumDurationSeconds, '0')
})

test('validates duration price and strict integer duration fields', () => {
  const cases: Array<{
    field: 'durationPrice' | 'roundingStepSeconds' | 'minimumDurationSeconds'
    value: string
    expected: string
  }> = [
    {
      field: 'durationPrice',
      value: ' ',
      expected: 'Duration price is required.',
    },
    {
      field: 'durationPrice',
      value: 'Infinity',
      expected: 'Duration price must be zero or greater.',
    },
    {
      field: 'durationPrice',
      value: '-0.01',
      expected: 'Duration price must be zero or greater.',
    },
    {
      field: 'roundingStepSeconds',
      value: '1.5',
      expected: 'Rounding step must be an integer between 1 and 3600.',
    },
    {
      field: 'roundingStepSeconds',
      value: '3601',
      expected: 'Rounding step must be an integer between 1 and 3600.',
    },
    {
      field: 'minimumDurationSeconds',
      value: ' 1',
      expected:
        'Minimum billable duration must be an integer between 0 and 3600.',
    },
    {
      field: 'minimumDurationSeconds',
      value: '-1',
      expected:
        'Minimum billable duration must be an integer between 0 and 3600.',
    },
  ]

  for (const testCase of cases) {
    const errors = validateDurationPricingValues(
      { ...durationFormValues, [testCase.field]: testCase.value },
      (key) => key
    )
    assert.equal(errors[testCase.field], testCase.expected)
  }

  assert.deepEqual(
    validateDurationPricingValues(durationFormValues, (key) => key),
    {}
  )
})

test('rejects whitespace in duration prices without rejecting valid numbers', () => {
  for (const durationPrice of [' 1', '1 ', '1 0']) {
    const errors = validateDurationPricingValues(
      { ...durationFormValues, durationPrice },
      (key) => key
    )
    assert.equal(
      errors.durationPrice,
      'Duration price must be zero or greater.'
    )
  }

  for (const durationPrice of ['0', '0.25']) {
    const errors = validateDurationPricingValues(
      { ...durationFormValues, durationPrice },
      (key) => key
    )
    assert.equal(errors.durationPrice, undefined)
  }
})

test('builds the complete duration rule only in duration mode', () => {
  const durationData = buildModelPricingSubmitData(
    durationFormValues,
    'per_duration',
    '',
    ''
  )

  assert.deepEqual(durationData.durationPrice, rule)

  for (const mode of [
    'per-token',
    'per-request',
    'tiered_expr',
  ] satisfies PricingMode[]) {
    const data = buildModelPricingSubmitData(
      durationFormValues,
      mode,
      'tier("base", 1)',
      ''
    )
    assert.equal(data.durationPrice, undefined)
  }
})

test('previews the complete duration rule', () => {
  const rows = buildPreviewRows(
    durationFormValues,
    'per_duration',
    '',
    '',
    '',
    emptyLanePrices,
    disabledLanes,
    (key) => key
  )

  assert.deepEqual(rows, [
    { key: 'mode', label: 'Mode', value: 'Duration-based' },
    {
      key: 'durationPrice',
      label: 'Duration price',
      value: '$0.25 / minute',
    },
    { key: 'roundingStep', label: 'Rounding step', value: '5 s' },
    {
      key: 'minimumDuration',
      label: 'Minimum billable duration',
      value: '10 s',
    },
  ])
})

test('builds and summarizes a per-duration snapshot', () => {
  const rows = buildModelSnapshots({
    ...emptyInput,
    billingMode: '{"video":"per_duration"}',
    durationPrice: JSON.stringify({ video: rule }),
  })

  assert.equal(rows[0].billingMode, 'per_duration')
  assert.deepEqual(rows[0].durationPrice, rule)
  assert.equal(
    getPriceSummary(rows[0], (key) => key),
    '$0.25 / minute'
  )
  assert.equal(isBasePricingUnset(rows[0]), false)
  assert.equal(
    getPriceDetail(rows[0], (key) => key),
    'Duration-based'
  )
})

test('does not activate duration mode without a structured rule', () => {
  const rows = buildModelSnapshots({
    ...emptyInput,
    billingMode: '{"video":"per_duration"}',
  })

  assert.equal(rows[0].billingMode, 'per-token')
  assert.equal(rows[0].durationPrice, undefined)
  assert.equal(isBasePricingUnset(rows[0]), true)
})

test('includes the complete duration rule in the dirty signature', () => {
  const [first] = buildModelSnapshots({
    ...emptyInput,
    billingMode: '{"video":"per_duration"}',
    durationPrice: JSON.stringify({ video: rule }),
  })
  const [second] = buildModelSnapshots({
    ...emptyInput,
    billingMode: '{"video":"per_duration"}',
    durationPrice: JSON.stringify({
      video: { ...rule, rounding_step_seconds: 10 },
    }),
  })

  assert.notEqual(getSnapshotSignature(first), getSnapshotSignature(second))
})

test('switching from duration writes an explicit ratio override', () => {
  const durationInput = updateModelPricingMaps(emptyInput, {
    name: 'video',
    billingMode: 'per_duration',
    durationPrice: rule,
  })
  const switched = updateModelPricingMaps(durationInput, {
    name: 'video',
    billingMode: 'per-request',
    price: '1.5',
  })

  assert.deepEqual(JSON.parse(switched.durationPrice), {})
  assert.deepEqual(JSON.parse(switched.billingMode), { video: 'ratio' })
  assert.deepEqual(JSON.parse(switched.modelPrice), { video: 1.5 })
})

test('deletion removes the duration rule and explicit mode', () => {
  const durationInput = updateModelPricingMaps(emptyInput, {
    name: 'video',
    billingMode: 'per_duration',
    durationPrice: rule,
  })
  const deleted = deleteModelPricingFromMaps(durationInput, 'video')

  assert.deepEqual(JSON.parse(deleted.durationPrice), {})
  assert.deepEqual(JSON.parse(deleted.billingMode), {})
  assert.deepEqual(buildModelSnapshots(deleted), [])
})

test('empty tiered expressions do not leave a tiered mode behind', () => {
  const updated = updateModelPricingMaps(
    {
      ...emptyInput,
      billingMode: '{"video":"per-token"}',
      billingExpr: '{"video":"old"}',
    },
    {
      name: 'video',
      billingMode: 'tiered_expr',
      billingExpr: '',
      requestRuleExpr: '',
      ratio: '2',
    }
  )

  assert.deepEqual(JSON.parse(updated.billingMode), {})
  assert.deepEqual(JSON.parse(updated.billingExpr), {})
  assert.deepEqual(JSON.parse(updated.modelRatio), { video: 2 })
})

test('duration save preserves auxiliary ratios and clears base conflicts', () => {
  const updated = updateModelPricingMaps(
    {
      ...emptyInput,
      modelPrice: '{"video":9}',
      modelRatio: '{"video":4}',
      billingExpr: '{"video":"tier(\\"base\\", 1)"}',
    },
    {
      name: 'video',
      billingMode: 'per_duration',
      durationPrice: rule,
      cacheRatio: '0.1',
      createCacheRatio: '0.2',
      completionRatio: '0.3',
      imageRatio: '1.5',
      audioRatio: '2.5',
      audioCompletionRatio: '0.4',
    }
  )

  assert.deepEqual(JSON.parse(updated.cacheRatio), { video: 0.1 })
  assert.deepEqual(JSON.parse(updated.createCacheRatio), { video: 0.2 })
  assert.deepEqual(JSON.parse(updated.completionRatio), { video: 0.3 })
  assert.deepEqual(JSON.parse(updated.imageRatio), { video: 1.5 })
  assert.deepEqual(JSON.parse(updated.audioRatio), { video: 2.5 })
  assert.deepEqual(JSON.parse(updated.audioCompletionRatio), { video: 0.4 })
  assert.deepEqual(JSON.parse(updated.modelPrice), {})
  assert.deepEqual(JSON.parse(updated.modelRatio), {})
  assert.deepEqual(JSON.parse(updated.billingExpr), {})
})

test('batch copy carries the full duration rule to every target', () => {
  const updated = updateModelPricingMaps(
    emptyInput,
    {
      name: 'source',
      billingMode: 'per_duration',
      durationPrice: rule,
    },
    ['source', 'copy-a', 'copy-b']
  )

  const rows = buildModelSnapshots(updated)
  assert.deepEqual(
    rows.map((row) => [row.name, row.billingMode, row.durationPrice]),
    [
      ['source', 'per_duration', rule],
      ['copy-a', 'per_duration', rule],
      ['copy-b', 'per_duration', rule],
    ]
  )
})

test('batch copy preserves duration auxiliary ratios for every target', () => {
  const updated = updateModelPricingMaps(
    {
      ...emptyInput,
      modelPrice: '{"source":9,"copy-a":9,"copy-b":9}',
      modelRatio: '{"source":4,"copy-a":4,"copy-b":4}',
      billingExpr: '{"source":"old","copy-a":"old","copy-b":"old"}',
    },
    {
      name: 'source',
      billingMode: 'per_duration',
      durationPrice: rule,
      cacheRatio: '0.1',
      createCacheRatio: '0.2',
      completionRatio: '0.3',
      imageRatio: '1.5',
      audioRatio: '2.5',
      audioCompletionRatio: '0.4',
    },
    ['source', 'copy-a', 'copy-b']
  )
  const expectedCache = { source: 0.1, 'copy-a': 0.1, 'copy-b': 0.1 }
  const expectedCreateCache = { source: 0.2, 'copy-a': 0.2, 'copy-b': 0.2 }
  const expectedCompletion = { source: 0.3, 'copy-a': 0.3, 'copy-b': 0.3 }
  const expectedImage = { source: 1.5, 'copy-a': 1.5, 'copy-b': 1.5 }
  const expectedAudio = { source: 2.5, 'copy-a': 2.5, 'copy-b': 2.5 }
  const expectedAudioCompletion = {
    source: 0.4,
    'copy-a': 0.4,
    'copy-b': 0.4,
  }

  assert.deepEqual(JSON.parse(updated.cacheRatio), expectedCache)
  assert.deepEqual(JSON.parse(updated.createCacheRatio), expectedCreateCache)
  assert.deepEqual(JSON.parse(updated.completionRatio), expectedCompletion)
  assert.deepEqual(JSON.parse(updated.imageRatio), expectedImage)
  assert.deepEqual(JSON.parse(updated.audioRatio), expectedAudio)
  assert.deepEqual(
    JSON.parse(updated.audioCompletionRatio),
    expectedAudioCompletion
  )
  assert.deepEqual(JSON.parse(updated.modelPrice), {})
  assert.deepEqual(JSON.parse(updated.modelRatio), {})
  assert.deepEqual(JSON.parse(updated.billingExpr), {})
})

test('fixed-price sync clears duration state with an explicit ratio mode', () => {
  const resolutions = applyResolutionSelection(
    {
      video: {
        billing_mode: 'per_duration',
        duration_price: rule,
      },
    },
    {
      video: {
        model_price: {
          current: null,
          upstreams: { upstream: 2 },
          confidence: { upstream: true },
        },
      },
    },
    {
      model: 'video',
      ratioType: 'model_price',
      value: 2,
      sourceName: 'upstream',
    }
  )

  assert.deepEqual(resolutions, {
    video: {
      billing_mode: 'ratio',
      model_price: 2,
    },
  })
})

test('duration sync preserves auxiliary ratios and clears base pricing', () => {
  const resolutions = applyResolutionSelection(
    {
      video: {
        model_price: 2,
        model_ratio: 3,
        cache_ratio: 0.1,
        image_ratio: 1.5,
        audio_ratio: 2.5,
      },
    },
    {
      video: {
        duration_price: {
          current: null,
          upstreams: { upstream: rule },
          confidence: { upstream: true },
        },
        billing_mode: {
          current: 'ratio',
          upstreams: { upstream: 'per_duration' },
          confidence: { upstream: true },
        },
      },
    },
    {
      model: 'video',
      ratioType: 'duration_price',
      value: rule,
      sourceName: 'upstream',
    }
  )

  assert.deepEqual(resolutions, {
    video: {
      cache_ratio: 0.1,
      image_ratio: 1.5,
      audio_ratio: 2.5,
      duration_price: rule,
      billing_mode: 'per_duration',
    },
  })
})

test('auxiliary ratio sync does not switch off selected duration billing', () => {
  const resolutions = applyResolutionSelection(
    {
      video: {
        billing_mode: 'per_duration',
        duration_price: rule,
      },
    },
    {
      video: {
        cache_ratio: {
          current: 0.1,
          upstreams: { upstream: 0.2 },
          confidence: { upstream: true },
        },
      },
    },
    {
      model: 'video',
      ratioType: 'cache_ratio',
      value: 0.2,
      sourceName: 'upstream',
    }
  )

  assert.deepEqual(resolutions, {
    video: {
      billing_mode: 'per_duration',
      duration_price: rule,
      cache_ratio: 0.2,
    },
  })
})
