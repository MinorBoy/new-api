/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { useTranslation } from 'react-i18next'

import type { ConfigImportIssueDetail } from '../types'

export interface IssueListProps {
  issues: ConfigImportIssueDetail[]
}

export function IssueList(props: IssueListProps) {
  const { t } = useTranslation()

  if (props.issues.length === 0) return null

  return (
    <ul className='divide-y border' aria-label={t('Import issues')}>
      {props.issues.map((issue) => (
        <li key={issue.id} className='space-y-1 px-3 py-2 text-sm'>
          <div className='flex flex-wrap items-center gap-2'>
            <span className='font-medium'>{issue.code}</span>
            <span className='text-muted-foreground'>{issue.severity}</span>
            <span className='text-muted-foreground'>
              {issue.resolution_status}
            </span>
          </div>
          <p>{issue.message}</p>
          {issue.sheet && (
            <p className='text-muted-foreground text-xs'>
              {issue.sheet}
              {issue.row ? `:${issue.row}` : ''}
            </p>
          )}
        </li>
      ))}
    </ul>
  )
}
