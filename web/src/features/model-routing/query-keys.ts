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
  groups: () => [...routingPolicyQueryKeys.all, 'groups'] as const,
}
