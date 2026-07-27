/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'

import type { ConfigImportBatchDetail } from '../types'
import { IssueList } from './issue-list'

export interface PublishReviewStepProps {
  batch: ConfigImportBatchDetail
  canPublish: boolean
  isPublishing?: boolean
  onPublish: () => Promise<void>
}

function unresolvedIssues(batch: ConfigImportBatchDetail) {
  return batch.issues.filter(
    (issue) =>
      (issue.severity === 'warning' || issue.severity === 'error') &&
      issue.resolution_status !== 'resolved' &&
      issue.resolution_status !== 'excluded'
  )
}

export function PublishReviewStep(props: PublishReviewStepProps) {
  const { t } = useTranslation()
  const [confirmed, setConfirmed] = useState(false)
  const [error, setError] = useState<string>()
  const openIssues = useMemo(() => unresolvedIssues(props.batch), [props.batch])
  const blocked = !props.canPublish || openIssues.length > 0

  const publish = async () => {
    if (blocked || !confirmed) return
    setError(undefined)
    try {
      await props.onPublish()
    } catch (caught) {
      const code =
        caught !== null && typeof caught === 'object' && 'code' in caught
          ? String(caught.code)
          : ''
      if (code === 'STALE_BASE_VERSION') {
        setError(
          t('The baseline is stale. Return to routing diffs and review again.')
        )
      } else if (caught instanceof Error) {
        setError(caught.message)
      } else {
        setError(t('The import could not be published.'))
      }
      throw caught
    }
  }

  return (
    <section
      className='space-y-4'
      aria-labelledby='config-import-publish-review-title'
    >
      <div className='flex flex-wrap items-center justify-between gap-3 border-b pb-3'>
        <div>
          <h2
            id='config-import-publish-review-title'
            className='text-base font-semibold'
          >
            {t('Publish review')}
          </h2>
          <p className='text-muted-foreground text-sm'>
            {t('Review the publish order and affected configuration.')}
          </p>
        </div>
        <Button
          onClick={publish}
          disabled={blocked || !confirmed || props.isPublishing}
        >
          {t('Publish import')}
        </Button>
      </div>

      {error && (
        <p className='text-destructive text-sm' role='alert'>
          {error}
        </p>
      )}

      <dl className='grid gap-3 text-sm sm:grid-cols-2 lg:grid-cols-4'>
        {Object.entries(props.batch.item_counts).map(([key, value]) => (
          <div key={key} className='border px-3 py-2'>
            <dt className='text-muted-foreground'>{key}</dt>
            <dd className='text-lg font-semibold'>{value}</dd>
          </div>
        ))}
      </dl>

      <div className='space-y-2 border px-3 py-3 text-sm'>
        <p className='font-medium'>{t('Publish order')}</p>
        <ol className='list-decimal space-y-1 pl-5'>
          <li>{t('Validate all staged proposals')}</li>
          <li>{t('Activate cost drafts')}</li>
          <li>{t('Apply routing and pricing configuration')}</li>
          <li>{t('Refresh configuration caches')}</li>
        </ol>
      </div>

      <IssueList issues={openIssues} />

      <label className='flex items-start gap-2 text-sm'>
        <input
          type='checkbox'
          aria-label={t('Confirm publish')}
          checked={confirmed}
          onChange={(event) => setConfirmed(event.target.checked)}
        />
        <span>{t('I confirm this import is ready to publish.')}</span>
      </label>
    </section>
  )
}
