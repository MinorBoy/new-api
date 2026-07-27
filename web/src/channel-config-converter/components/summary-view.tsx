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
import { useTranslation } from 'react-i18next'

import type { ConfigImportDocument } from '../document'

export interface SummaryViewProps {
  document: ConfigImportDocument
}

export function SummaryView(props: SummaryViewProps) {
  const { t } = useTranslation()
  const entries = Object.entries(props.document.manifest.counts)

  return (
    <section aria-labelledby='converter-overview-title'>
      <h2
        id='converter-overview-title'
        style={{ fontSize: 18, margin: '0 0 14px' }}
      >
        {t('Overview')}
      </h2>
      <dl
        style={{
          display: 'grid',
          gap: 12,
          gridTemplateColumns: 'repeat(auto-fit, minmax(128px, 1fr))',
          margin: 0,
        }}
      >
        {entries.map(([name, count]) => (
          <div key={name} style={{ border: '1px solid #d7dee8', padding: 12 }}>
            <dt style={{ color: '#526173', fontSize: 12 }}>{t(name)}</dt>
            <dd style={{ fontSize: 22, margin: '4px 0 0' }}>{count}</dd>
          </div>
        ))}
      </dl>
      <p
        style={{
          color: '#526173',
          fontFamily: 'monospace',
          fontSize: 12,
          overflowWrap: 'anywhere',
        }}
      >
        {props.document.manifest.payload_sha256}
      </p>
    </section>
  )
}
