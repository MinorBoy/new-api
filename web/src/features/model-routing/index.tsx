import { useState } from 'react'

import { RoutingPoliciesTable } from './components/routing-policies-table'
import {
  RoutingPolicyDialogs,
  type RoutingPolicyDialogState,
} from './components/routing-policy-dialogs'
import { RoutingPolicyDrawer } from './components/routing-policy-drawer'
import type { RoutingPolicy } from './types'

type EditingPolicyState = RoutingPolicy | 'create' | null

export function ModelRouting() {
  const [editingPolicy, setEditingPolicy] = useState<EditingPolicyState>(null)
  const [copyingPolicy, setCopyingPolicy] = useState<RoutingPolicy | null>(null)
  const [dialogState, setDialogState] = useState<RoutingPolicyDialogState>(null)

  const closeDrawer = () => {
    setEditingPolicy(null)
    setCopyingPolicy(null)
  }

  return (
    <>
      <RoutingPoliciesTable
        onCreate={() => {
          setCopyingPolicy(null)
          setEditingPolicy('create')
        }}
        onEdit={(policy) => {
          setCopyingPolicy(null)
          setEditingPolicy(policy)
        }}
        onCopy={(policy) => {
          setEditingPolicy(null)
          setCopyingPolicy(policy)
        }}
        onToggleStatus={(policy) => setDialogState({ kind: 'status', policy })}
        onDelete={(policy) => setDialogState({ kind: 'delete', policy })}
      />

      <RoutingPolicyDrawer
        open={editingPolicy !== null || copyingPolicy !== null}
        editingPolicy={
          editingPolicy !== null && editingPolicy !== 'create'
            ? editingPolicy
            : null
        }
        copyingPolicy={copyingPolicy}
        onOpenChange={(open) => {
          if (!open) {
            closeDrawer()
          }
        }}
      />

      <RoutingPolicyDialogs
        state={dialogState}
        onClose={() => setDialogState(null)}
      />
    </>
  )
}
