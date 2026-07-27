/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the License,
or (at your option) any later version.
*/
import { RotateCcw } from 'lucide-react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'

import type { ConfigImportBatchSummary } from '../types'

export interface ImportBatchListProps {
  batches: ConfigImportBatchSummary[]
  isLoading?: boolean
  onResume: (id: number) => void
}

export function ImportBatchList(props: ImportBatchListProps) {
  const { t } = useTranslation()
  let content: ReactNode

  if (props.isLoading) {
    content = <p className='text-muted-foreground text-sm'>{t('Loading')}</p>
  } else if (props.batches.length === 0) {
    content = (
      <p className='text-muted-foreground text-sm'>
        {t('No import batches found')}
      </p>
    )
  } else {
    content = (
      <div className='overflow-x-auto border'>
        <table className='w-full min-w-[42rem] text-left text-sm'>
          <thead className='bg-muted/50 text-muted-foreground'>
            <tr>
              <th className='px-3 py-2 font-medium'>{t('Batch')}</th>
              <th className='px-3 py-2 font-medium'>{t('Status')}</th>
              <th className='px-3 py-2 font-medium'>{t('Template version')}</th>
              <th className='px-3 py-2 font-medium'>{t('Issues')}</th>
              <th className='w-24 px-3 py-2 font-medium'>
                <span className='sr-only'>{t('Actions')}</span>
              </th>
            </tr>
          </thead>
          <tbody>
            {props.batches.map((batch) => (
              <tr key={batch.id} className='border-t'>
                <td className='px-3 py-2 font-mono'>{batch.id}</td>
                <td className='px-3 py-2'>{batch.status}</td>
                <td className='px-3 py-2'>{batch.template_version}</td>
                <td className='px-3 py-2'>{batch.issue_count}</td>
                <td className='px-3 py-2 text-right'>
                  <Button
                    size='icon'
                    variant='ghost'
                    aria-label={t('Resume import batch')}
                    title={t('Resume import batch')}
                    onClick={() => props.onResume(batch.id)}
                  >
                    <RotateCcw className='size-4' aria-hidden='true' />
                  </Button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    )
  }

  return (
    <section
      className='space-y-3'
      aria-labelledby='config-import-batch-list-title'
    >
      <h2
        id='config-import-batch-list-title'
        className='text-base font-semibold'
      >
        {t('Available import batches')}
      </h2>
      {content}
    </section>
  )
}
