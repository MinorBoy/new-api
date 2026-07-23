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
