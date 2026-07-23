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
import { z } from 'zod'

import { api } from '@/lib/api'

import {
  routingCandidateResponseSchema,
  routingGroupResponseSchema,
  routingPolicyListResponseSchema,
  routingPolicyResponseSchema,
  type RoutingPolicyListParams,
  type RoutingPolicyWriteRequest,
} from './types'

export async function listRoutingPolicies(params: RoutingPolicyListParams) {
  const response = await api.get('/api/routing-policies', { params })
  return routingPolicyListResponseSchema.parse(response.data)
}

export async function getRoutingPolicy(id: number) {
  const response = await api.get(`/api/routing-policies/${id}`)
  return routingPolicyResponseSchema.parse(response.data)
}

export async function listRoutingCandidates(groupName: string, model: string) {
  const response = await api.get('/api/routing-policies/candidates', {
    params: { group_name: groupName, model },
  })
  return routingCandidateResponseSchema.parse(response.data)
}

export async function listRoutingGroups() {
  const response = await api.get('/api/group/')
  return routingGroupResponseSchema.parse(response.data)
}

export async function createRoutingPolicy(payload: RoutingPolicyWriteRequest) {
  const response = await api.post('/api/routing-policies', payload, {
    skipBusinessError: true,
  })
  return routingPolicyResponseSchema.parse(response.data)
}

export async function updateRoutingPolicy(
  id: number,
  payload: RoutingPolicyWriteRequest
) {
  const response = await api.put(`/api/routing-policies/${id}`, payload, {
    skipBusinessError: true,
  })
  return routingPolicyResponseSchema.parse(response.data)
}

export async function updateRoutingPolicyStatus(id: number, enabled: boolean) {
  const response = await api.post(
    `/api/routing-policies/${id}/status`,
    { enabled },
    { skipBusinessError: true }
  )
  return routingPolicyResponseSchema.parse(response.data)
}

export async function deleteRoutingPolicy(id: number) {
  const response = await api.delete(`/api/routing-policies/${id}`, {
    skipBusinessError: true,
  })
  return z
    .object({ success: z.literal(true) })
    .passthrough()
    .parse(response.data)
}
