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
import { canonicalize } from 'json-canonicalize'

function compareStrings(left: string, right: string): number {
  if (left < right) return -1
  if (left > right) return 1
  return 0
}

function withoutEntityHash(
  value: Record<string, unknown>
): Record<string, unknown> {
  const { entity_hash: _entityHash, ...rest } = value
  return rest
}

function canonicalContractValue(value: unknown): unknown {
  if (Array.isArray(value)) {
    const normalized = value.map(canonicalContractValue)
    if (normalized.every((item) => typeof item === 'string')) {
      return normalized.sort((left, right) =>
        compareStrings(String(left), String(right))
      )
    }
    if (normalized.every((item) => typeof item === 'number')) {
      return normalized.sort((left, right) => Number(left) - Number(right))
    }
    for (const key of ['business_id', 'route_target_ref']) {
      if (
        normalized.every(
          (item) =>
            typeof item === 'object' &&
            item !== null &&
            !Array.isArray(item) &&
            typeof (item as Record<string, unknown>)[key] === 'string'
        )
      ) {
        return normalized.sort((left, right) =>
          compareStrings(
            String((left as Record<string, unknown>)[key]),
            String((right as Record<string, unknown>)[key])
          )
        )
      }
    }
    return normalized
  }
  if (typeof value === 'object' && value !== null) {
    return Object.fromEntries(
      Object.entries(value).map(([key, child]) => [
        key,
        canonicalContractValue(child),
      ])
    )
  }
  return value
}

async function sha256(
  value: unknown,
  normalizeContract = false
): Promise<string> {
  const bytes = new TextEncoder().encode(
    canonicalize(normalizeContract ? canonicalContractValue(value) : value)
  )
  const digest = await crypto.subtle.digest('SHA-256', bytes)
  return Array.from(new Uint8Array(digest), (byte) =>
    byte.toString(16).padStart(2, '0')
  ).join('')
}

export async function hashEntity(
  entity: Record<string, unknown>
): Promise<string> {
  return sha256(withoutEntityHash(entity), true)
}

function sortStringArrayField(
  entity: Record<string, unknown>,
  key: string
): void {
  const values = entity[key]
  if (
    !Array.isArray(values) ||
    !values.every((value) => typeof value === 'string')
  ) {
    return
  }
  entity[key] = [...values].sort(compareStrings)
}

function sortNumberArrayField(
  entity: Record<string, unknown>,
  key: string
): void {
  const values = entity[key]
  if (
    !Array.isArray(values) ||
    !values.every((value) => typeof value === 'number')
  ) {
    return
  }
  entity[key] = [...values].sort((left, right) => left - right)
}

function canonicalPayloadEntity(
  type: string,
  candidate: Record<string, unknown>
): Record<string, unknown> {
  const entity = structuredClone(candidate)
  if (type === 'model_skus') {
    for (const key of ['output_resolutions', 'aspect_ratios', 'input_modes']) {
      sortStringArrayField(entity, key)
    }
    sortNumberArrayField(entity, 'duration_values')
  }
  if (type === 'route_blueprints') {
    sortStringArrayField(entity, 'model_mapping_refs')
    const targets = entity.targets
    if (Array.isArray(targets)) {
      const normalizedTargets = targets
        .filter(
          (target): target is Record<string, unknown> =>
            typeof target === 'object' &&
            target !== null &&
            !Array.isArray(target)
        )
        .map((target) => structuredClone(target))
      for (const target of normalizedTargets) {
        for (const key of [
          'output_resolutions',
          'aspect_ratios',
          'input_modes',
        ]) {
          sortStringArrayField(target, key)
        }
        sortNumberArrayField(target, 'duration_values')
      }
      normalizedTargets.sort((left, right) => {
        for (const key of ['route_target_ref', 'line_ref', 'sku_ref']) {
          const compared = compareStrings(
            String(left[key] ?? ''),
            String(right[key] ?? '')
          )
          if (compared !== 0) return compared
        }
        return 0
      })
      entity.targets = normalizedTargets
    }
  }
  return entity
}

export async function hashPayload(
  payload: Record<string, unknown>
): Promise<string> {
  const entities = payload.entities
  const normalizedEntities: Record<string, unknown[]> = {}
  if (
    typeof entities === 'object' &&
    entities !== null &&
    !Array.isArray(entities)
  ) {
    for (const [type, candidates] of Object.entries(entities)) {
      if (!Array.isArray(candidates)) {
        continue
      }
      if (type === 'group_routing_requirements' && candidates.length === 0) {
        continue
      }
      normalizedEntities[type] = candidates
        .filter(
          (candidate): candidate is Record<string, unknown> =>
            typeof candidate === 'object' &&
            candidate !== null &&
            !Array.isArray(candidate)
        )
        .map((candidate) => canonicalPayloadEntity(type, candidate))
        .sort((left, right) =>
          compareStrings(String(left.business_id), String(right.business_id))
        )
    }
  }
  return sha256({ entities: normalizedEntities })
}
