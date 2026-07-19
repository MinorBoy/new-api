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
import {
  combineBillingExpr,
  splitBillingExprAndRequestRules,
} from '@/features/pricing/lib/billing-expr'

import { safeJsonParse } from '../utils/json-parser'
import type { DurationPrice, ModelRatioData } from './model-pricing-core'
import { formatPricingNumber } from './pricing-format'

export type ModelPricingSnapshotInput = {
  modelPrice: string
  modelRatio: string
  cacheRatio: string
  createCacheRatio: string
  completionRatio: string
  imageRatio: string
  audioRatio: string
  audioCompletionRatio: string
  billingMode: string
  billingExpr: string
  durationPrice: string
}

export type ModelPricingSnapshot = {
  name: string
  price?: string
  ratio?: string
  cacheRatio?: string
  createCacheRatio?: string
  completionRatio?: string
  imageRatio?: string
  audioRatio?: string
  audioCompletionRatio?: string
  billingMode?: string
  billingExpr?: string
  requestRuleExpr?: string
  durationPrice?: DurationPrice
  hasConflict: boolean
}

export type ModelRow = ModelPricingSnapshot & {
  saved?: ModelPricingSnapshot
  draft?: ModelPricingSnapshot
  isDraftChanged: boolean
  isDraftDeleted: boolean
  isDraftNew: boolean
}

export const hasPricingValue = (value?: string) =>
  value !== undefined && value !== ''

export const isBasePricingUnset = (snapshot?: ModelPricingSnapshot) =>
  !snapshot ||
  (snapshot.billingMode !== 'tiered_expr' &&
    snapshot.billingMode !== 'per_duration' &&
    !hasPricingValue(snapshot.price) &&
    !hasPricingValue(snapshot.ratio))

const toNumberOrNull = (value?: string) => {
  if (!hasPricingValue(value)) return null
  const num = Number(value)
  return Number.isFinite(num) ? num : null
}

const ratioToPrice = (ratio?: string, denominator?: string) => {
  const ratioNumber = toNumberOrNull(ratio)
  const denominatorNumber = denominator ? toNumberOrNull(denominator) : 2
  if (ratioNumber === null || denominatorNumber === null) return ''
  return formatPricingNumber(ratioNumber * denominatorNumber)
}

export const getModeLabel = (mode?: string) => {
  if (mode === 'per-request') return 'Per-request'
  if (mode === 'per_duration') return 'Per-duration'
  if (mode === 'tiered_expr') return 'Expression'
  return 'Per-token'
}

export const getModeVariant = (
  mode?: string
): 'warning' | 'info' | 'success' => {
  if (mode === 'per-request') return 'warning'
  if (mode === 'per_duration') return 'success'
  if (mode === 'tiered_expr') return 'info'
  return 'success'
}

const getExpressionSummary = (
  row: ModelPricingSnapshot,
  t: (key: string) => string
) => {
  const tierCount = (row.billingExpr?.match(/tier\(/g) || []).length
  if (tierCount > 0) {
    return `${t('Tiered pricing')} · ${tierCount} ${t('tiers')}`
  }
  return t('Expression pricing')
}

export const getPriceSummary = (
  row: ModelPricingSnapshot,
  t: (key: string) => string
) => {
  if (row.billingMode === 'tiered_expr') {
    return getExpressionSummary(row, t)
  }
  if (row.billingMode === 'per-request') {
    return row.price ? `$${row.price} / ${t('request')}` : t('Unset price')
  }
  if (row.billingMode === 'per_duration') {
    const rule = row.durationPrice
    return rule ? `$${rule.price} / ${t(rule.unit)}` : t('Unset price')
  }

  const inputPrice = ratioToPrice(row.ratio)
  if (!inputPrice) return t('Unset price')

  const extraCount = [
    row.completionRatio,
    row.cacheRatio,
    row.createCacheRatio,
    row.imageRatio,
    row.audioRatio,
    row.audioCompletionRatio,
  ].filter(hasPricingValue).length

  return extraCount > 0
    ? `${t('Input')} $${inputPrice} · ${extraCount} ${t('extras')}`
    : `${t('Input')} $${inputPrice}`
}

export const getPriceDetail = (
  row: ModelPricingSnapshot,
  t: (key: string) => string
) => {
  if (row.billingMode === 'tiered_expr') {
    return row.requestRuleExpr
      ? t('Includes request rules')
      : t('Expression based')
  }
  if (row.billingMode === 'per-request') {
    return t('Fixed request price')
  }

  const inputPrice = ratioToPrice(row.ratio)
  if (!inputPrice) return t('No base input price')

  const details = [
    row.completionRatio &&
      `${t('Output')} $${ratioToPrice(row.completionRatio, inputPrice)}`,
    row.cacheRatio &&
      `${t('Cache')} $${ratioToPrice(row.cacheRatio, inputPrice)}`,
    row.createCacheRatio &&
      `${t('Cache write')} $${ratioToPrice(row.createCacheRatio, inputPrice)}`,
  ]
    .filter(Boolean)
    .slice(0, 2)

  return details.length > 0 ? details.join(' · ') : t('Base input price only')
}

export const buildModelSnapshots = ({
  modelPrice,
  modelRatio,
  cacheRatio,
  createCacheRatio,
  completionRatio,
  imageRatio,
  audioRatio,
  audioCompletionRatio,
  billingMode,
  billingExpr,
  durationPrice,
}: ModelPricingSnapshotInput): ModelPricingSnapshot[] => {
  const priceMap = safeJsonParse<Record<string, number>>(modelPrice, {
    fallback: {},
    context: 'model prices',
  })
  const ratioMap = safeJsonParse<Record<string, number>>(modelRatio, {
    fallback: {},
    context: 'model ratios',
  })
  const cacheMap = safeJsonParse<Record<string, number>>(cacheRatio, {
    fallback: {},
    context: 'cache ratios',
  })
  const createCacheMap = safeJsonParse<Record<string, number>>(
    createCacheRatio,
    { fallback: {}, context: 'create cache ratios' }
  )
  const completionMap = safeJsonParse<Record<string, number>>(completionRatio, {
    fallback: {},
    context: 'completion ratios',
  })
  const imageMap = safeJsonParse<Record<string, number>>(imageRatio, {
    fallback: {},
    context: 'image ratios',
  })
  const audioMap = safeJsonParse<Record<string, number>>(audioRatio, {
    fallback: {},
    context: 'audio ratios',
  })
  const audioCompletionMap = safeJsonParse<Record<string, number>>(
    audioCompletionRatio,
    { fallback: {}, context: 'audio completion ratios' }
  )
  const billingModeMap = safeJsonParse<Record<string, string>>(billingMode, {
    fallback: {},
    context: 'billing mode',
  })
  const billingExprMap = safeJsonParse<Record<string, string>>(billingExpr, {
    fallback: {},
    context: 'billing expression',
  })
  const durationPriceMap = safeJsonParse<Record<string, DurationPrice>>(
    durationPrice,
    { fallback: {}, context: 'duration prices' }
  )

  const modelNames = new Set([
    ...Object.keys(priceMap),
    ...Object.keys(ratioMap),
    ...Object.keys(cacheMap),
    ...Object.keys(createCacheMap),
    ...Object.keys(completionMap),
    ...Object.keys(imageMap),
    ...Object.keys(audioMap),
    ...Object.keys(audioCompletionMap),
    ...Object.keys(billingModeMap),
    ...Object.keys(billingExprMap),
    ...Object.keys(durationPriceMap),
  ])

  return [...modelNames].map((name) => {
    const price = priceMap[name]?.toString() || ''
    const ratio = ratioMap[name]?.toString() || ''
    const cache = cacheMap[name]?.toString() || ''
    const createCache = createCacheMap[name]?.toString() || ''
    const completion = completionMap[name]?.toString() || ''
    const image = imageMap[name]?.toString() || ''
    const audio = audioMap[name]?.toString() || ''
    const audioCompletion = audioCompletionMap[name]?.toString() || ''

    const modeForModel = billingModeMap[name]
    const durationRule = durationPriceMap[name]
    if (modeForModel === 'per_duration' && durationRule) {
      return {
        name,
        billingMode: 'per_duration',
        durationPrice: durationRule,
        price,
        ratio,
        cacheRatio: cache,
        createCacheRatio: createCache,
        completionRatio: completion,
        imageRatio: image,
        audioRatio: audio,
        audioCompletionRatio: audioCompletion,
        hasConflict: false,
      }
    }
    if (modeForModel === 'tiered_expr') {
      const fullExpr = billingExprMap[name] || ''
      const { billingExpr: pureExpr, requestRuleExpr } =
        splitBillingExprAndRequestRules(fullExpr)
      return {
        name,
        billingMode: 'tiered_expr',
        billingExpr: pureExpr,
        requestRuleExpr,
        price,
        ratio,
        cacheRatio: cache,
        createCacheRatio: createCache,
        completionRatio: completion,
        imageRatio: image,
        audioRatio: audio,
        audioCompletionRatio: audioCompletion,
        hasConflict: false,
      }
    }

    return {
      name,
      price,
      ratio,
      cacheRatio: cache,
      createCacheRatio: createCache,
      completionRatio: completion,
      imageRatio: image,
      audioRatio: audio,
      audioCompletionRatio: audioCompletion,
      billingMode: price !== '' ? 'per-request' : 'per-token',
      hasConflict:
        price !== '' &&
        (ratio !== '' ||
          completion !== '' ||
          cache !== '' ||
          createCache !== '' ||
          image !== '' ||
          audio !== '' ||
          audioCompletion !== ''),
    }
  })
}

export const getSnapshotSignature = (snapshot?: ModelPricingSnapshot) => {
  if (!snapshot) return ''
  return JSON.stringify({
    price: snapshot.price || '',
    ratio: snapshot.ratio || '',
    cacheRatio: snapshot.cacheRatio || '',
    createCacheRatio: snapshot.createCacheRatio || '',
    completionRatio: snapshot.completionRatio || '',
    imageRatio: snapshot.imageRatio || '',
    audioRatio: snapshot.audioRatio || '',
    audioCompletionRatio: snapshot.audioCompletionRatio || '',
    billingMode: snapshot.billingMode || 'per-token',
    billingExpr: snapshot.billingExpr || '',
    requestRuleExpr: snapshot.requestRuleExpr || '',
    durationPrice: snapshot.durationPrice || null,
  })
}

type PricingMaps = {
  price: Record<string, number>
  ratio: Record<string, number>
  cache: Record<string, number>
  createCache: Record<string, number>
  completion: Record<string, number>
  image: Record<string, number>
  audio: Record<string, number>
  audioCompletion: Record<string, number>
  billingMode: Record<string, string>
  billingExpr: Record<string, string>
  durationPrice: Record<string, DurationPrice>
}

function parsePricingMaps(input: ModelPricingSnapshotInput): PricingMaps {
  return {
    price: safeJsonParse(input.modelPrice, { fallback: {}, silent: true }),
    ratio: safeJsonParse(input.modelRatio, { fallback: {}, silent: true }),
    cache: safeJsonParse(input.cacheRatio, { fallback: {}, silent: true }),
    createCache: safeJsonParse(input.createCacheRatio, {
      fallback: {},
      silent: true,
    }),
    completion: safeJsonParse(input.completionRatio, {
      fallback: {},
      silent: true,
    }),
    image: safeJsonParse(input.imageRatio, { fallback: {}, silent: true }),
    audio: safeJsonParse(input.audioRatio, { fallback: {}, silent: true }),
    audioCompletion: safeJsonParse(input.audioCompletionRatio, {
      fallback: {},
      silent: true,
    }),
    billingMode: safeJsonParse(input.billingMode, {
      fallback: {},
      silent: true,
    }),
    billingExpr: safeJsonParse(input.billingExpr, {
      fallback: {},
      silent: true,
    }),
    durationPrice: safeJsonParse(input.durationPrice, {
      fallback: {},
      silent: true,
    }),
  }
}

function serializePricingMaps(maps: PricingMaps): ModelPricingSnapshotInput {
  return {
    modelPrice: JSON.stringify(maps.price, null, 2),
    modelRatio: JSON.stringify(maps.ratio, null, 2),
    cacheRatio: JSON.stringify(maps.cache, null, 2),
    createCacheRatio: JSON.stringify(maps.createCache, null, 2),
    completionRatio: JSON.stringify(maps.completion, null, 2),
    imageRatio: JSON.stringify(maps.image, null, 2),
    audioRatio: JSON.stringify(maps.audio, null, 2),
    audioCompletionRatio: JSON.stringify(maps.audioCompletion, null, 2),
    billingMode: JSON.stringify(maps.billingMode, null, 2),
    billingExpr: JSON.stringify(maps.billingExpr, null, 2),
    durationPrice: JSON.stringify(maps.durationPrice, null, 2),
  }
}

function deletePricingMapEntries(maps: PricingMaps, name: string) {
  delete maps.price[name]
  delete maps.ratio[name]
  delete maps.cache[name]
  delete maps.createCache[name]
  delete maps.completion[name]
  delete maps.image[name]
  delete maps.audio[name]
  delete maps.audioCompletion[name]
  delete maps.billingMode[name]
  delete maps.billingExpr[name]
  delete maps.durationPrice[name]
}

function setNumericPricingValue(
  target: Record<string, number>,
  name: string,
  value?: string
) {
  if (!value) return
  const parsed = Number(value)
  if (Number.isFinite(parsed)) target[name] = parsed
}

export function updateModelPricingMaps(
  input: ModelPricingSnapshotInput,
  data: ModelRatioData,
  targetNames: string[] = [data.name]
): ModelPricingSnapshotInput {
  const maps = parsePricingMaps(input)

  targetNames.forEach((name) => {
    deletePricingMapEntries(maps, name)

    if (data.billingMode === 'per_duration' && data.durationPrice) {
      maps.billingMode[name] = 'per_duration'
      maps.durationPrice[name] = data.durationPrice
      return
    }

    if (data.billingMode === 'tiered_expr') {
      maps.billingMode[name] = 'tiered_expr'
      const combined = combineBillingExpr(
        data.billingExpr || '',
        data.requestRuleExpr || ''
      )
      if (combined) maps.billingExpr[name] = combined
      setNumericPricingValue(maps.price, name, data.price)
      setNumericPricingValue(maps.ratio, name, data.ratio)
      setNumericPricingValue(maps.cache, name, data.cacheRatio)
      setNumericPricingValue(maps.createCache, name, data.createCacheRatio)
      setNumericPricingValue(maps.completion, name, data.completionRatio)
      setNumericPricingValue(maps.image, name, data.imageRatio)
      setNumericPricingValue(maps.audio, name, data.audioRatio)
      setNumericPricingValue(
        maps.audioCompletion,
        name,
        data.audioCompletionRatio
      )
      return
    }

    maps.billingMode[name] = 'ratio'
    if (data.billingMode === 'per-request') {
      setNumericPricingValue(maps.price, name, data.price)
      return
    }

    setNumericPricingValue(maps.ratio, name, data.ratio)
    setNumericPricingValue(maps.cache, name, data.cacheRatio)
    setNumericPricingValue(maps.createCache, name, data.createCacheRatio)
    setNumericPricingValue(maps.completion, name, data.completionRatio)
    setNumericPricingValue(maps.image, name, data.imageRatio)
    setNumericPricingValue(maps.audio, name, data.audioRatio)
    setNumericPricingValue(
      maps.audioCompletion,
      name,
      data.audioCompletionRatio
    )
  })

  return serializePricingMaps(maps)
}

export function deleteModelPricingFromMaps(
  input: ModelPricingSnapshotInput,
  name: string
): ModelPricingSnapshotInput {
  const maps = parsePricingMaps(input)
  deletePricingMapEntries(maps, name)
  return serializePricingMaps(maps)
}
