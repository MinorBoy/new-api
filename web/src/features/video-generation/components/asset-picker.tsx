/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import {
  Alert02Icon,
  ArrowLeft01Icon,
  ArrowRight01Icon,
  Image01Icon,
  ImageNotFound01Icon,
  RefreshIcon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useQuery } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  Alert,
  AlertAction,
  AlertDescription,
  AlertTitle,
} from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'

import { listAssets } from '../../assets/api'
import type { Asset } from '../../assets/types'

const ASSET_PAGE_SIZE = 12

type AssetPickerProps = {
  apiKeyId: number | null
  apiKey: string
  selectedIds: string[]
  limit: number
  onChange: (ids: string[]) => void
}

function statusVariant(status: Asset['status']) {
  if (status === 'active') return 'default' as const
  if (status === 'failed') return 'destructive' as const
  return 'outline' as const
}

export function AssetPicker(props: AssetPickerProps) {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)
  const [failedPreviewIds, setFailedPreviewIds] = useState<string[]>([])
  useEffect(() => {
    setPage(1)
    setFailedPreviewIds([])
  }, [props.apiKeyId])
  const assetsQuery = useQuery({
    queryKey: ['video-generation', 'role-assets', props.apiKeyId, page],
    queryFn: () =>
      listAssets(props.apiKey, { page, pageSize: ASSET_PAGE_SIZE }),
    enabled: Boolean(props.apiKeyId && props.apiKey),
  })
  const assets = assetsQuery.data?.data.items ?? []
  const totalPages = Math.max(
    1,
    Math.ceil(
      (assetsQuery.data?.data.total ?? 0) /
        (assetsQuery.data?.data.page_size ?? ASSET_PAGE_SIZE)
    )
  )

  function toggleAsset(asset: Asset) {
    if (asset.status !== 'active') return
    if (props.selectedIds.includes(asset.id)) {
      props.onChange(props.selectedIds.filter((id) => id !== asset.id))
      return
    }
    if (props.selectedIds.length >= props.limit) return
    props.onChange([...props.selectedIds, asset.id])
  }

  if (!props.apiKeyId || !props.apiKey) {
    return (
      <Empty className='border'>
        <EmptyHeader>
          <EmptyMedia variant='icon'>
            <HugeiconsIcon icon={Image01Icon} strokeWidth={2} />
          </EmptyMedia>
          <EmptyTitle>{t('Select an API key')}</EmptyTitle>
          <EmptyDescription>
            {t('Select an API key to view assets.')}
          </EmptyDescription>
        </EmptyHeader>
      </Empty>
    )
  }

  if (assetsQuery.isError) {
    return (
      <Alert variant='destructive'>
        <HugeiconsIcon icon={Alert02Icon} strokeWidth={2} />
        <AlertTitle>{t('Failed to load assets')}</AlertTitle>
        <AlertDescription>
          {t('The asset library could not be loaded.')}
        </AlertDescription>
        <AlertAction>
          <Button
            type='button'
            size='sm'
            variant='outline'
            aria-label={t('Retry loading assets')}
            onClick={() => void assetsQuery.refetch()}
          >
            <HugeiconsIcon
              icon={RefreshIcon}
              strokeWidth={2}
              data-icon='inline-start'
            />
            {t('Retry')}
          </Button>
        </AlertAction>
      </Alert>
    )
  }

  if (assetsQuery.isPending) {
    return (
      <div className='grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-4'>
        {Array.from({ length: 4 }, (_, index) => (
          <Skeleton key={index} className='aspect-square rounded-lg' />
        ))}
      </div>
    )
  }

  if (assets.length === 0) {
    return (
      <Empty className='border'>
        <EmptyHeader>
          <EmptyMedia variant='icon'>
            <HugeiconsIcon icon={Image01Icon} strokeWidth={2} />
          </EmptyMedia>
          <EmptyTitle>{t('No assets yet')}</EmptyTitle>
          <EmptyDescription>
            {t('Create a role asset before selecting it for video generation.')}
          </EmptyDescription>
        </EmptyHeader>
        <EmptyContent>
          <Button nativeButton={false} render={<a href='/assets' />}>
            {t('Open asset library')}
          </Button>
        </EmptyContent>
      </Empty>
    )
  }

  return (
    <div className='space-y-3'>
      <p className='text-muted-foreground text-xs tabular-nums'>
        {t('Selected {{count}} of {{limit}}', {
          count: props.selectedIds.length,
          limit: props.limit,
        })}
      </p>
      <div className='grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-4'>
        {assets.map((asset) => {
          const selected = props.selectedIds.includes(asset.id)
          const disabled =
            asset.status !== 'active' ||
            (!selected && props.selectedIds.length >= props.limit)
          return (
            <button
              key={asset.id}
              type='button'
              aria-label={t('Select asset {{id}}', { id: asset.id })}
              aria-pressed={selected}
              className={cn(
                'bg-card overflow-hidden rounded-lg border text-left outline-none transition-colors',
                'focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50',
                selected && 'border-primary ring-2 ring-primary/20'
              )}
              disabled={disabled}
              onClick={() => toggleAsset(asset)}
            >
              {failedPreviewIds.includes(asset.id) ? (
                <span
                  role='img'
                  aria-label={t('Preview unavailable')}
                  className='bg-muted text-muted-foreground flex aspect-square w-full items-center justify-center'
                >
                  <HugeiconsIcon
                    icon={ImageNotFound01Icon}
                    strokeWidth={2}
                    className='size-6'
                    aria-hidden='true'
                  />
                </span>
              ) : (
                <img
                  src={asset.url}
                  alt=''
                  className='aspect-square w-full object-cover'
                  onError={() =>
                    setFailedPreviewIds((current) =>
                      current.includes(asset.id)
                        ? current
                        : [...current, asset.id]
                    )
                  }
                />
              )}
              <span className='flex min-w-0 flex-col gap-1 p-2'>
                <span className='font-mono text-[11px] leading-4 break-all'>
                  {asset.id}
                </span>
                <Badge variant={statusVariant(asset.status)}>
                  {t(asset.status)}
                </Badge>
              </span>
            </button>
          )
        })}
      </div>
      {totalPages > 1 ? (
        <div className='flex items-center justify-center gap-2'>
          <Button
            type='button'
            size='icon-sm'
            variant='outline'
            aria-label={t('Previous page')}
            disabled={page <= 1}
            onClick={() => setPage((current) => Math.max(1, current - 1))}
          >
            <HugeiconsIcon icon={ArrowLeft01Icon} strokeWidth={2} />
          </Button>
          <span className='text-muted-foreground min-w-20 text-center text-xs tabular-nums'>
            {t('Page {{page}} of {{total}}', { page, total: totalPages })}
          </span>
          <Button
            type='button'
            size='icon-sm'
            variant='outline'
            aria-label={t('Next page')}
            disabled={page >= totalPages}
            onClick={() =>
              setPage((current) => Math.min(totalPages, current + 1))
            }
          >
            <HugeiconsIcon icon={ArrowRight01Icon} strokeWidth={2} />
          </Button>
        </div>
      ) : null}
    </div>
  )
}
