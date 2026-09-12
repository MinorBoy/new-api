import { createFileRoute, redirect } from '@tanstack/react-router'
import { z } from 'zod'

import { ConfigImportWizard } from '@/features/config-import'
import {
  ADMIN_PERMISSION_ACTIONS,
  ADMIN_PERMISSION_RESOURCES,
  hasPermission,
} from '@/lib/admin-permissions'
import { useAuthStore } from '@/stores/auth-store'

const configImportSearchSchema = z.object({
  batch: z.number().int().positive().optional().catch(undefined),
})

export function requireConfigImportRead() {
  const { auth } = useAuthStore.getState()
  if (
    !hasPermission(
      auth.user,
      ADMIN_PERMISSION_RESOURCES.CONFIG_IMPORT,
      ADMIN_PERMISSION_ACTIONS.READ
    )
  ) {
    throw redirect({ to: '/403' })
  }
}

export const Route = createFileRoute('/_authenticated/config-import/')({
  beforeLoad: requireConfigImportRead,
  validateSearch: configImportSearchSchema,
  component: ConfigImportRoute,
})

function ConfigImportRoute() {
  const { batch } = Route.useSearch()
  return <ConfigImportWizard restoreBatchID={batch} />
}
