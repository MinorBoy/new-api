/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'

import type { ConfigImportBatchDetail } from '../types'

export interface PublishResultStepProps {
  batch: ConfigImportBatchDetail
  onValidate?: () => Promise<void>
}

export function PublishResultStep(props: PublishResultStepProps) {
  const { t } = useTranslation()
  const published = props.batch.status === 'published'
  const failed = props.batch.status === 'publish_failed'
  let title = t('Publish result')
  if (published) title = t('Published')
  if (failed) title = t('Publish failed')
  const pendingCache = props.batch.issues.some(
    (issue) => issue.code === 'CACHE_REFRESH_PENDING'
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
      {published && <p>{t('The import was published successfully.')}</p>}
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
      {pendingCache && (
        <p className='border border-amber-500/50 bg-amber-500/10 px-3 py-2 text-sm'>
          {t(
            'CACHE_REFRESH_PENDING: configuration changed, but cache refresh is still pending.'
          )}
        </p>
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
