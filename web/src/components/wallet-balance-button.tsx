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
import { useQuery } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { Wallet } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { getSelf } from '@/lib/api'
import { formatQuota } from '@/lib/format'

import { Button } from './ui/button'
import { Skeleton } from './ui/skeleton'

type WalletBalanceButtonProps = {
  quota?: number
  loading: boolean
  onOpenWallet: () => void
}

type SelfResponse = {
  success?: boolean
  data?: {
    quota?: number
  }
}

export function WalletBalanceButton({
  quota,
  loading,
  onOpenWallet,
}: WalletBalanceButtonProps) {
  const { t } = useTranslation()
  const hasQuota = quota !== undefined && Number.isFinite(quota)
  const formattedQuota = hasQuota ? formatQuota(quota) : undefined
  const accessibleLabel = formattedQuota
    ? `${t('Current Balance')}: ${formattedQuota}`
    : t('Current Balance')

  return (
    <Button
      variant='ghost'
      size='sm'
      className='size-7 px-0 sm:w-auto sm:min-w-20 sm:px-2'
      data-testid='wallet-balance-button'
      data-header-control='wallet-balance'
      aria-label={accessibleLabel}
      title={accessibleLabel}
      onClick={onOpenWallet}
    >
      <Wallet className='size-3.5' data-icon='inline-start' />
      {loading ? (
        <Skeleton
          data-testid='wallet-balance-loading'
          className='hidden h-3.5 w-12 sm:block'
        />
      ) : formattedQuota ? (
        <span
          data-testid='wallet-balance-value'
          className='hidden font-mono tabular-nums sm:inline'
        >
          {formattedQuota}
        </span>
      ) : null}
    </Button>
  )
}

export function WalletBalanceLink() {
  const navigate = useNavigate()
  const { data: quota, isLoading } = useQuery({
    queryKey: ['user', 'wallet-balance'],
    queryFn: async () => {
      const response = (await getSelf()) as SelfResponse
      const currentQuota = response.data?.quota
      if (!response.success || typeof currentQuota !== 'number') {
        throw new Error('current user balance is unavailable')
      }
      return currentQuota
    },
    staleTime: 0,
    refetchInterval: 60_000,
    refetchOnWindowFocus: true,
    retry: false,
  })

  return (
    <WalletBalanceButton
      quota={quota}
      loading={isLoading}
      onOpenWallet={() => navigate({ to: '/wallet' })}
    />
  )
}
