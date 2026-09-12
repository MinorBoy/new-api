import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'

import { deleteRoutingPolicy, updateRoutingPolicyStatus } from '../api'
import { routingPolicyQueryKeys } from '../query-keys'
import type { RoutingPolicy } from '../types'
import { getApiErrorPayload } from './routing-policy-drawer'

export type RoutingPolicyDialogState =
  | { kind: 'status'; policy: RoutingPolicy }
  | { kind: 'delete'; policy: RoutingPolicy }
  | null

type RoutingPolicyDialogsProps = {
  state: RoutingPolicyDialogState
  onClose: () => void
}

export function RoutingPolicyDialogs(props: RoutingPolicyDialogsProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const mutation = useMutation({
    mutationFn: async (state: Exclude<RoutingPolicyDialogState, null>) => {
      if (state.kind === 'delete') {
        return deleteRoutingPolicy(state.policy.id)
      }
      return updateRoutingPolicyStatus(state.policy.id, !state.policy.enabled)
    },
    onSuccess: async (_data, state) => {
      await queryClient.invalidateQueries({
        queryKey: routingPolicyQueryKeys.all,
      })
      toast.success(
        t(
          state.kind === 'delete'
            ? 'Routing policy deleted'
            : 'Routing policy status updated'
        )
      )
      props.onClose()
    },
    onError: (error) => {
      const payload = getApiErrorPayload(error)
      toast.error(t(payload?.message ?? 'Routing policy operation failed'))
    },
  })

  const state = props.state
  const open = state !== null
  const handleOpenChange = (nextOpen: boolean) => {
    if (!nextOpen && !mutation.isPending) {
      props.onClose()
    }
  }

  if (!state) {
    return null
  }

  if (state.kind === 'delete') {
    return (
      <ConfirmDialog
        open={open}
        onOpenChange={handleOpenChange}
        title={t('Delete routing policy')}
        desc={t(
          'Delete the routing policy for group "{{group}}" and model "{{model}}"? All targets in this policy will also be removed.',
          {
            group: state.policy.group_name,
            model: state.policy.model,
          }
        )}
        confirmText={t('Delete')}
        destructive
        isLoading={mutation.isPending}
        handleConfirm={() => mutation.mutate(state)}
      />
    )
  }

  const enabling = !state.policy.enabled
  return (
    <ConfirmDialog
      open={open}
      onOpenChange={handleOpenChange}
      title={t(enabling ? 'Enable routing policy' : 'Disable routing policy')}
      desc={
        enabling
          ? t(
              'Enable capability routing for group "{{group}}" and model "{{model}}"?',
              {
                group: state.policy.group_name,
                model: state.policy.model,
              }
            )
          : t(
              'Disable this policy? New requests will return to legacy routing; the policy and targets will be kept.'
            )
      }
      confirmText={t(enabling ? 'Enable' : 'Disable')}
      isLoading={mutation.isPending}
      handleConfirm={() => mutation.mutate(state)}
    />
  )
}
