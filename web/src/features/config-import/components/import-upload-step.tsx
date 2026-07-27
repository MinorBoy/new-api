/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the License,
or (at your option) any later version.
*/
import { Loader2, Upload } from 'lucide-react'
import { useState, type ChangeEvent } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'

import { uploadConfigImport } from '../api'
import type { ConfigImportBatchDetail } from '../types'

const MAX_IMPORT_FILE_BYTES = 10 * 1024 * 1024

export interface ImportUploadStepProps {
  disabled?: boolean
  onUpload?: (document: unknown) => Promise<ConfigImportBatchDetail>
  onUploaded: (batch: ConfigImportBatchDetail) => void
}

export function ImportUploadStep(props: ImportUploadStepProps) {
  const { t } = useTranslation()
  const [error, setError] = useState<string>()
  const [isUploading, setIsUploading] = useState(false)
  const [latestBatch, setLatestBatch] = useState<ConfigImportBatchDetail>()

  const handleFileChange = async (event: ChangeEvent<HTMLInputElement>) => {
    const file = event.currentTarget.files?.[0]
    event.currentTarget.value = ''
    setError(undefined)
    if (!file) return

    if (!file.name.toLowerCase().endsWith('.json')) {
      setError(t('Select a JSON file'))
      return
    }
    if (file.size > MAX_IMPORT_FILE_BYTES) {
      setError(t('The JSON file is too large'))
      return
    }

    setIsUploading(true)
    try {
      const document = JSON.parse(await file.text()) as unknown
      const batch = await (props.onUpload ?? uploadConfigImport)(document)
      setLatestBatch(batch)
      props.onUploaded(batch)
    } catch {
      setError(t('The selected JSON file could not be imported.'))
    } finally {
      setIsUploading(false)
    }
  }

  return (
    <section className='space-y-4' aria-labelledby='config-import-upload-title'>
      <div className='flex flex-wrap items-center justify-between gap-3 border-b pb-3'>
        <h2 id='config-import-upload-title' className='text-base font-semibold'>
          {t('Import upload')}
        </h2>
        <Button render={<label />} disabled={props.disabled || isUploading}>
          {isUploading ? (
            <Loader2 className='size-4 animate-spin' aria-hidden='true' />
          ) : (
            <Upload className='size-4' aria-hidden='true' />
          )}
          {t('Select JSON')}
          <input
            className='sr-only'
            type='file'
            accept='application/json,.json'
            aria-label={t('Import configuration JSON')}
            disabled={props.disabled || isUploading}
            onChange={handleFileChange}
          />
        </Button>
      </div>

      {error && (
        <p className='text-destructive text-sm' role='alert'>
          {error}
        </p>
      )}

      {latestBatch && (
        <dl className='grid gap-x-6 gap-y-3 text-sm sm:grid-cols-2 lg:grid-cols-4'>
          <div>
            <dt className='text-muted-foreground'>{t('Payload SHA-256')}</dt>
            <dd className='font-mono text-xs break-all'>
              {latestBatch.payload_sha256}
            </dd>
          </div>
          <div>
            <dt className='text-muted-foreground'>{t('Schema version')}</dt>
            <dd>{latestBatch.schema_version}</dd>
          </div>
          <div>
            <dt className='text-muted-foreground'>{t('Template version')}</dt>
            <dd>{latestBatch.template_version}</dd>
          </div>
          <div>
            <dt className='text-muted-foreground'>{t('Issues')}</dt>
            <dd>{latestBatch.issue_count}</dd>
          </div>
        </dl>
      )}
    </section>
  )
}
