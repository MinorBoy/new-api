/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { Copy } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'

import type { ConfigImportBatchDetail } from '../types'

export interface PublishResultStepProps {
  batch: ConfigImportBatchDetail
  onValidate?: () => Promise<void>
  onCopyForBinding?: () => Promise<void>
  onRefreshCache?: () => Promise<void>
  isCopying?: boolean
  isRefreshing?: boolean
}

export function PublishResultStep(props: PublishResultStepProps) {
  const { t } = useTranslation()
  const published = props.batch.status === 'published'
  const activated = published && props.batch.activated_at != null
  const failed = props.batch.status === 'publish_failed'
  let title = t('Publish result')
  if (published) title = t('Published')
  if (failed) title = t('Publish failed')
  const pendingCacheIssue = props.batch.issues.find(
    (issue) =>
      (issue.code === 'CACHE_REFRESH_PENDING' ||
        issue.code === 'ACTIVATION_CACHE_REFRESH_PENDING') &&
      issue.resolution_status === 'open'
  )
  const createdCount = props.batch.items.filter(
    (item) => item.state === 'new'
  ).length
  const changedCount = props.batch.items.filter(
    (item) => item.state === 'changed'
  ).length
  const excludedCount = props.batch.items.filter(
    (item) => item.state === 'excluded'
  ).length

  return (
    <section
      className='space-y-4'
      aria-labelledby='config-import-publish-result-title'
    >
      <h2
        id='config-import-publish-result-title'
        className='text-base font-semibold'
      >
        {title}
      </h2>
      {published && !activated && (
        <p>{t('The import was published successfully.')}</p>
      )}
      {activated && <p>{t('The published configuration is active.')}</p>}
      {published && props.onCopyForBinding && (
        <Button
          type='button'
          variant='outline'
          disabled={props.isCopying}
          aria-busy={props.isCopying}
          onClick={() => void props.onCopyForBinding?.()}
        >
          {props.isCopying && <Spinner data-icon='inline-start' />}
          {!props.isCopying && <Copy data-icon='inline-start' />}
          {t('Copy as new binding batch')}
        </Button>
      )}
      {failed && (
        <div className='border-destructive/50 space-y-3 border px-3 py-3 text-sm'>
          <p>
            {t(
              'The transactional publish failed. Validate again before retrying.'
            )}
          </p>
          <Button
            onClick={() => void props.onValidate?.()}
            disabled={!props.onValidate}
          >
            {t('Retry validation')}
          </Button>
        </div>
      )}
      {pendingCacheIssue && (
        <Alert>
          <AlertTitle>{pendingCacheIssue.code}</AlertTitle>
          <AlertDescription className='flex flex-col items-start gap-3'>
            <p>
              {t(
                'CACHE_REFRESH_PENDING: configuration changed, but cache refresh is still pending.'
              )}
            </p>
            {props.onRefreshCache && (
              <Button
                type='button'
                variant='outline'
                disabled={props.isRefreshing}
                aria-busy={props.isRefreshing}
                onClick={() => void props.onRefreshCache?.()}
              >
                {props.isRefreshing && <Spinner data-icon='inline-start' />}
                {t('Retry cache refresh')}
              </Button>
            )}
          </AlertDescription>
        </Alert>
      )}
      <dl className='grid gap-3 text-sm sm:grid-cols-3'>
        <div className='border px-3 py-2'>
          <dt className='text-muted-foreground'>{t('Created')}</dt>
          <dd className='text-lg font-semibold'>{createdCount}</dd>
        </div>
        <div className='border px-3 py-2'>
          <dt className='text-muted-foreground'>{t('Changed')}</dt>
          <dd className='text-lg font-semibold'>{changedCount}</dd>
        </div>
        <div className='border px-3 py-2'>
          <dt className='text-muted-foreground'>{t('Excluded')}</dt>
          <dd className='text-lg font-semibold'>{excludedCount}</dd>
        </div>
      </dl>
    </section>
  )
}
