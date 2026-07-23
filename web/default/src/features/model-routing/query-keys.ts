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
import type { RoutingPolicyListParams } from './types'

export const routingPolicyQueryKeys = {
  all: ['routing-policies'] as const,
  lists: () => [...routingPolicyQueryKeys.all, 'list'] as const,
  list: (params: RoutingPolicyListParams) =>
    [...routingPolicyQueryKeys.lists(), params] as const,
  details: () => [...routingPolicyQueryKeys.all, 'detail'] as const,
  detail: (id: number) => [...routingPolicyQueryKeys.details(), id] as const,
  candidates: (groupName: string, model: string) =>
    [...routingPolicyQueryKeys.all, 'candidates', groupName, model] as const,
}
