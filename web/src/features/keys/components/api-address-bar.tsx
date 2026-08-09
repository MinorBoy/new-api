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
import { Server, ExternalLink } from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { Button } from '@/components/ui/button'
import { useStatus } from '@/hooks/use-status'
import type { SystemStatus } from '@/features/auth/types'

function extractServerAddress(status: SystemStatus | null): string {
  const fromStatus =
    (status?.server_address as string | undefined) ??
    (status?.serverAddress as string | undefined) ??
    status?.data?.server_address ??
    (status?.data as Record<string, unknown> | undefined)?.serverAddress

  if (fromStatus && typeof fromStatus === 'string') {
    return fromStatus
  }
  if (typeof window !== 'undefined') {
    return window.location.origin
  }
  return ''
}

export function ApiAddressBar() {
  const { t } = useTranslation()
  const { status } = useStatus()
  const address = useMemo(() => extractServerAddress(status), [status])

  const trimmed = address.replace(/\/+$/, '')

  return (
    <div className='bg-muted/30 flex items-center gap-2 rounded-lg border px-3 py-2 sm:gap-3'>
      <span className='bg-primary/10 text-primary flex size-7 shrink-0 items-center justify-center rounded-md'>
        <Server className='size-4' />
      </span>
      <div className='flex min-w-0 flex-1 flex-col gap-0.5 sm:flex-row sm:items-baseline sm:gap-2'>
        <span className='text-muted-foreground shrink-0 text-xs font-medium'>
          {t('API Address')}
        </span>
        <span className='text-foreground/80 truncate font-mono text-xs sm:text-sm'>
          {trimmed}
        </span>
      </div>
      <div className='flex shrink-0 items-center gap-0.5'>
        <CopyButton
          value={trimmed}
          variant='ghost'
          size='sm'
          className='size-7 p-0'
          iconClassName='size-3.5'
          tooltip={t('Copy API Address')}
          aria-label={t('Copy API Address')}
        />
        <Button
          variant='ghost'
          size='sm'
          className='size-7 p-0'
          title={t('Open in New Tab')}
          render={<a href={trimmed} target='_blank' rel='noreferrer' />}
        >
          <ExternalLink className='size-3.5' />
        </Button>
      </div>
    </div>
  )
}
