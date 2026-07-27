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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import type { ConfigImportDocument } from '../document'

export interface IssueViewProps {
  issues: ConfigImportDocument['issues']
}

export function IssueView(props: IssueViewProps) {
  const { t } = useTranslation()
  const [severity, setSeverity] = useState('all')
  const issues = props.issues.filter(
    (issue) => severity === 'all' || issue.severity === severity
  )

  return (
    <section aria-labelledby='converter-issues-title'>
      <div
        style={{
          alignItems: 'center',
          display: 'flex',
          gap: 12,
          justifyContent: 'space-between',
          marginBottom: 14,
        }}
      >
        <h2 id='converter-issues-title' style={{ fontSize: 18, margin: 0 }}>
          {t('Issues')}
        </h2>
        <select
          aria-label={t('Issue severity')}
          onChange={(event) => setSeverity(event.target.value)}
          value={severity}
        >
          <option value='all'>{t('All severities')}</option>
          <option value='error'>{t('Error')}</option>
          <option value='warning'>{t('Warning')}</option>
          <option value='info'>{t('Info')}</option>
        </select>
      </div>
      <ul style={{ margin: 0, paddingLeft: 20 }}>
        {issues.map((issue) => (
          <li key={`${issue.code}-${issue.entity_ref ?? ''}`}>
            <strong>{issue.severity.toUpperCase()}</strong> {issue.code}:{' '}
            {issue.message}
          </li>
        ))}
      </ul>
    </section>
  )
}
