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

function withoutEntityHash(
  value: Record<string, unknown>
): Record<string, unknown> {
  const { entity_hash: _entityHash, ...rest } = value
  return rest
}

async function sha256(value: unknown): Promise<string> {
  const bytes = new TextEncoder().encode(canonicalize(value))
  const digest = await crypto.subtle.digest('SHA-256', bytes)
  return Array.from(new Uint8Array(digest), (byte) =>
    byte.toString(16).padStart(2, '0')
  ).join('')
}

export async function hashEntity(
  entity: Record<string, unknown>
): Promise<string> {
  return sha256(withoutEntityHash(entity))
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
      normalizedEntities[type] = candidates
        .filter(
          (candidate): candidate is Record<string, unknown> =>
            typeof candidate === 'object' &&
            candidate !== null &&
            !Array.isArray(candidate)
        )
        .map(withoutEntityHash)
        .sort((left, right) =>
          String(left.business_id).localeCompare(String(right.business_id))
        )
    }
  }
  return sha256({
    ...(typeof payload.kind === 'string' ? { kind: payload.kind } : {}),
    ...(typeof payload.schema_version === 'number'
      ? { schema_version: payload.schema_version }
      : {}),
    ...(typeof payload.template_version === 'string'
      ? { template_version: payload.template_version }
      : {}),
    entities: normalizedEntities,
  })
}
