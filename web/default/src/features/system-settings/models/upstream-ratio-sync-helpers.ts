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
import type { RatioType, RatioValue } from '../types'
import {
  MODELS_DEV_PRESET_ID,
  MODELS_DEV_PRESET_NAME,
  OFFICIAL_CHANNEL_ID,
  OFFICIAL_CHANNEL_NAME,
  RATIO_TYPE_OPTIONS,
} from './constants'
import type { DurationPrice } from './model-pricing-core'

export type RatioDifferenceEntry = {
  current: RatioValue | null
  upstreams: Record<string, RatioValue | 'same'>
  confidence: Record<string, boolean>
}

export type ModelRow = {
  key: string
  model: string
  ratioTypes: Partial<Record<RatioType, RatioDifferenceEntry>>
  billingConflict: boolean
}

export type ResolutionsMap = Record<string, Record<string, RatioValue>>

export type ResolutionSelection = {
  model: string
  ratioType: RatioType
  value: RatioValue
  sourceName: string
}

export type ResolvedResolutionSelection = ResolutionSelection & {
  ratioType: RatioType
}

export type ResolutionRemoval = {
  model: string
  ratioType: RatioType
}

export type ResolutionRemovalPlan = Map<string, Set<RatioType>>

export const RATIO_SYNC_FIELDS: RatioType[] = [
  'model_ratio',
  'completion_ratio',
  'cache_ratio',
  'create_cache_ratio',
  'image_ratio',
  'audio_ratio',
  'audio_completion_ratio',
]

export const SYNC_FIELD_ORDER: RatioType[] = [
  ...RATIO_SYNC_FIELDS,
  'model_price',
  'billing_mode',
  'billing_expr',
  'duration_price',
]

export const NUMERIC_SYNC_FIELDS = new Set<string>([
  ...RATIO_SYNC_FIELDS,
  'model_price',
])

const DURATION_COMPATIBLE_SYNC_FIELDS = new Set<RatioType>(
  RATIO_SYNC_FIELDS.filter((ratioType) => ratioType !== 'model_ratio')
)

function isDurationCompatibleSyncField(ratioType: string): boolean {
  return DURATION_COMPATIBLE_SYNC_FIELDS.has(ratioType as RatioType)
}

export function getSyncFieldLabel(
  ratioType: string,
  t: (key: string) => string
): string {
  const opt = RATIO_TYPE_OPTIONS.find((o) => o.value === ratioType)
  if (opt) return t(opt.label)
  return ratioType
}

export function getOrderedRatioTypes(
  ratioTypes: Partial<Record<RatioType, RatioDifferenceEntry>>,
  filter?: string
): RatioType[] {
  const keys = Object.keys(ratioTypes) as RatioType[]
  const ordered = [
    ...SYNC_FIELD_ORDER.filter((f) => keys.includes(f)),
    ...keys.filter((f) => !SYNC_FIELD_ORDER.includes(f)),
  ]
  if (!filter || filter === '__all__') return ordered
  return ordered.filter((f) => f === filter)
}

export function getPreferredSyncField(
  ratioTypes: Partial<Record<RatioType, RatioDifferenceEntry>>,
  ratioType: RatioType,
  sourceName: string
): RatioType {
  const exprValue = ratioTypes.billing_expr?.upstreams?.[sourceName]
  if (
    ratioType !== 'billing_expr' &&
    exprValue !== null &&
    exprValue !== undefined &&
    exprValue !== 'same'
  ) {
    return 'billing_expr'
  }
  const durationValue = ratioTypes.duration_price?.upstreams?.[sourceName]
  if (
    ratioType !== 'duration_price' &&
    !isDurationCompatibleSyncField(ratioType) &&
    isSelectableUpstreamValue(durationValue)
  ) {
    return 'duration_price'
  }
  return ratioType
}

export function getVisibleRatioTypesForSource(
  ratioTypes: Partial<Record<RatioType, RatioDifferenceEntry>>,
  sourceName: string,
  filter?: string
): RatioType[] {
  return getOrderedRatioTypes(ratioTypes, filter).filter(
    (ratioType) =>
      getPreferredSyncField(ratioTypes, ratioType, sourceName) === ratioType
  )
}

export function getAlignedRatioTypes(
  ratioTypes: Partial<Record<RatioType, RatioDifferenceEntry>>,
  sourceNames: string[],
  filter?: string
): RatioType[] {
  const ordered = getOrderedRatioTypes(ratioTypes, filter)
  if (sourceNames.length === 0) return ordered

  const visible = new Set<RatioType>()
  sourceNames.forEach((sourceName) => {
    getVisibleRatioTypesForSource(ratioTypes, sourceName, filter).forEach(
      (ratioType) => visible.add(ratioType)
    )
  })

  return ordered.filter((ratioType) => visible.has(ratioType))
}

export function getBillingCategory(
  ratioType: string
): 'price' | 'ratio' | 'duration' | 'tiered' {
  if (ratioType === 'model_price') return 'price'
  if (ratioType === 'duration_price') return 'duration'
  if (ratioType === 'billing_mode' || ratioType === 'billing_expr') {
    return 'tiered'
  }
  return 'ratio'
}

export function isSelectableUpstreamValue(
  value: RatioValue | 'same' | null | undefined
): value is RatioValue {
  return value !== null && value !== undefined && value !== 'same'
}

export function getUpstreamDisplayName(sourceName: string): string {
  const synthesizedPresets = [
    { name: OFFICIAL_CHANNEL_NAME, id: OFFICIAL_CHANNEL_ID },
    { name: MODELS_DEV_PRESET_NAME, id: MODELS_DEV_PRESET_ID },
  ]

  for (const preset of synthesizedPresets) {
    if (sourceName === `${preset.name}(${preset.id})`) {
      return preset.name
    }
  }

  return sourceName
}

export function isSelectedResolutionValue(
  resolutions: ResolutionsMap,
  model: string,
  ratioType: RatioType,
  upstreamValue: RatioValue | 'same' | null | undefined
): boolean {
  if (!isSelectableUpstreamValue(upstreamValue)) return false

  const selectedValue = resolutions[model]?.[ratioType]
  if (selectedValue === undefined) return false

  if (NUMERIC_SYNC_FIELDS.has(ratioType)) {
    const selectedNumber = Number(selectedValue)
    const upstreamNumber = Number(upstreamValue)
    return (
      Number.isFinite(selectedNumber) &&
      Number.isFinite(upstreamNumber) &&
      selectedNumber === upstreamNumber
    )
  }

  if (typeof selectedValue === 'object' || typeof upstreamValue === 'object') {
    return JSON.stringify(selectedValue) === JSON.stringify(upstreamValue)
  }
  return selectedValue === upstreamValue
}

export function deleteResolutionField(
  resolutions: ResolutionsMap,
  model: string,
  ratioType: RatioType
): ResolutionsMap {
  return applyResolutionRemovals(resolutions, [{ model, ratioType }])
}

function getDraftModelResolution(
  drafts: Map<string, Record<string, RatioValue>>,
  resolutions: ResolutionsMap,
  model: string
): Record<string, RatioValue> {
  const existingDraft = drafts.get(model)
  if (existingDraft) return existingDraft

  const draft = resolutions[model] ? { ...resolutions[model] } : {}
  drafts.set(model, draft)
  return draft
}

function applyResolutionSelectionToDraft(
  drafts: Map<string, Record<string, RatioValue>>,
  resolutions: ResolutionsMap,
  differences: Record<string, Partial<Record<RatioType, RatioDifferenceEntry>>>,
  selection: ResolutionSelection
) {
  const modelDiffs = differences[selection.model]
  const preferredType = getPreferredSyncField(
    modelDiffs || {},
    selection.ratioType,
    selection.sourceName
  )
  const preferredValue =
    preferredType === selection.ratioType
      ? selection.value
      : (modelDiffs?.[preferredType]?.upstreams?.[selection.sourceName] ??
        selection.value)

  const finalType = preferredType
  const finalValue = preferredValue as RatioValue
  const category = getBillingCategory(finalType)
  const newModelRes = getDraftModelResolution(
    drafts,
    resolutions,
    selection.model
  )
  const isAuxiliaryRatioSelection =
    category === 'ratio' && isDurationCompatibleSyncField(finalType)

  Object.keys(newModelRes).forEach((rt) => {
    if (isAuxiliaryRatioSelection) return
    if (category === 'duration') {
      if (rt === 'model_price' || rt === 'model_ratio') {
        delete newModelRes[rt]
      }
      return
    }
    if (
      category !== 'tiered' &&
      getBillingCategory(rt) !== 'tiered' &&
      getBillingCategory(rt) !== category
    ) {
      delete newModelRes[rt]
    }
  })

  if (
    category === 'price' ||
    (category === 'ratio' && !isAuxiliaryRatioSelection)
  ) {
    delete newModelRes['billing_expr']
    delete newModelRes['duration_price']
    newModelRes['billing_mode'] = 'ratio'
  } else if (category === 'duration') {
    delete newModelRes['billing_expr']
  } else if (category === 'tiered') {
    delete newModelRes['duration_price']
  }

  newModelRes[finalType] = finalValue

  if (category === 'tiered' && modelDiffs) {
    const modeVal = modelDiffs.billing_mode?.upstreams?.[selection.sourceName]
    const exprVal = modelDiffs.billing_expr?.upstreams?.[selection.sourceName]
    if (modeVal !== undefined && modeVal !== null && modeVal !== 'same') {
      newModelRes['billing_mode'] = modeVal
    } else if (finalType === 'billing_expr') {
      newModelRes['billing_mode'] = 'tiered_expr'
    }
    if (exprVal !== undefined && exprVal !== null && exprVal !== 'same') {
      newModelRes['billing_expr'] = exprVal
    }
  }
  if (category === 'duration' && modelDiffs) {
    newModelRes['billing_mode'] = 'per_duration'
    const durationValue =
      modelDiffs.duration_price?.upstreams?.[selection.sourceName]
    if (isSelectableUpstreamValue(durationValue)) {
      newModelRes['duration_price'] = durationValue
    }
  }
}

export function resolveResolutionSelection(
  differences: Record<string, Partial<Record<RatioType, RatioDifferenceEntry>>>,
  selection: ResolutionSelection
): ResolvedResolutionSelection {
  const modelDiffs = differences[selection.model]
  const preferredType = getPreferredSyncField(
    modelDiffs || {},
    selection.ratioType,
    selection.sourceName
  )
  const preferredValue =
    preferredType === selection.ratioType
      ? selection.value
      : (modelDiffs?.[preferredType]?.upstreams?.[selection.sourceName] ??
        selection.value)

  return {
    ...selection,
    ratioType: preferredType,
    value: preferredValue as RatioValue,
  }
}

export function getEffectiveResolutionSelections(
  differences: Record<string, Partial<Record<RatioType, RatioDifferenceEntry>>>,
  selections: ResolutionSelection[]
): ResolvedResolutionSelection[] {
  const effectiveByKey = new Map<string, ResolvedResolutionSelection>()

  selections.forEach((selection) => {
    const resolved = resolveResolutionSelection(differences, selection)
    const category = getBillingCategory(resolved.ratioType)

    if (category !== 'tiered') {
      for (const [key, existing] of effectiveByKey) {
        const durationCompatible =
          (resolved.ratioType === 'duration_price' &&
            isDurationCompatibleSyncField(existing.ratioType)) ||
          (existing.ratioType === 'duration_price' &&
            isDurationCompatibleSyncField(resolved.ratioType))
        if (
          existing.model === resolved.model &&
          getBillingCategory(existing.ratioType) !== 'tiered' &&
          getBillingCategory(existing.ratioType) !== category &&
          !durationCompatible
        ) {
          effectiveByKey.delete(key)
        }
      }
    }

    effectiveByKey.set(`${resolved.model}\u0000${resolved.ratioType}`, resolved)
  })

  return [...effectiveByKey.values()]
}

export function applyResolutionSelections(
  resolutions: ResolutionsMap,
  differences: Record<string, Partial<Record<RatioType, RatioDifferenceEntry>>>,
  selections: ResolutionSelection[]
): ResolutionsMap {
  if (selections.length === 0) return resolutions

  const next = { ...resolutions }
  const drafts = new Map<string, Record<string, RatioValue>>()

  selections.forEach((selection) => {
    applyResolutionSelectionToDraft(drafts, resolutions, differences, selection)
  })

  drafts.forEach((draft, model) => {
    if (Object.keys(draft).length === 0) {
      delete next[model]
    } else {
      next[model] = draft
    }
  })

  return next
}

export function applyResolutionSelection(
  resolutions: ResolutionsMap,
  differences: Record<string, Partial<Record<RatioType, RatioDifferenceEntry>>>,
  selection: ResolutionSelection
): ResolutionsMap {
  return applyResolutionSelections(resolutions, differences, [selection])
}

export function applyResolutionRemovals(
  resolutions: ResolutionsMap,
  removals: ResolutionRemoval[]
): ResolutionsMap {
  if (removals.length === 0) return resolutions

  const plan: ResolutionRemovalPlan = new Map()
  removals.forEach((removal) => {
    const ratioTypes = plan.get(removal.model)
    if (ratioTypes) {
      ratioTypes.add(removal.ratioType)
    } else {
      plan.set(removal.model, new Set([removal.ratioType]))
    }
  })

  return applyResolutionRemovalPlan(resolutions, plan)
}

export function applyResolutionRemovalPlan(
  resolutions: ResolutionsMap,
  plan: ResolutionRemovalPlan
): ResolutionsMap {
  if (plan.size === 0) return resolutions

  const next = { ...resolutions }

  plan.forEach((ratioTypes, model) => {
    const current = resolutions[model]
    if (!current) return

    const draft = { ...current }
    ratioTypes.forEach((ratioType) => {
      delete draft[ratioType]
      if (ratioType === 'billing_expr') delete draft['billing_mode']
      if (ratioType === 'duration_price') delete draft['billing_mode']
      if (ratioType === 'billing_mode') {
        delete draft['billing_expr']
        delete draft['duration_price']
      }
    })
    const hasBasePricing =
      draft['model_price'] !== undefined || draft['model_ratio'] !== undefined
    if (!hasBasePricing && draft['billing_mode'] === 'ratio') {
      delete draft['billing_mode']
    }
    if (Object.keys(draft).length === 0) {
      delete next[model]
    } else {
      next[model] = draft
    }
  })

  return next
}

export function isDurationPrice(value: unknown): value is DurationPrice {
  if (!value || typeof value !== 'object') return false
  const candidate = value as Partial<DurationPrice>
  return (
    typeof candidate.price === 'number' &&
    (candidate.unit === 'second' || candidate.unit === 'minute') &&
    typeof candidate.rounding_step_seconds === 'number' &&
    typeof candidate.minimum_duration_seconds === 'number'
  )
}

export function formatSyncValue(
  value: RatioValue,
  t: (key: string) => string
): string {
  if (!isDurationPrice(value)) return String(value)
  return `$${value.price} / ${t(value.unit)} · ${t('Rounding step')}: ${value.rounding_step_seconds}s · ${t('Minimum billable duration')}: ${value.minimum_duration_seconds}s`
}
