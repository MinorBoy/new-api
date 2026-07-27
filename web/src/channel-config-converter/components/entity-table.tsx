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
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

export interface EntityTableProps {
  entities: Record<string, unknown>[]
  title: string
}

function sourceLocation(entity: Record<string, unknown>): string {
  const sheet = typeof entity.sheet === 'string' ? entity.sheet : ''
  const row = typeof entity.row === 'number' ? entity.row : 0
  return sheet && row > 0 ? `${sheet}!${row}` : ''
}

export function EntityTable(props: EntityTableProps) {
  const { t } = useTranslation()
  const [filter, setFilter] = useState('')
  const items = useMemo(() => {
    const query = filter.trim().toLowerCase()
    if (!query) return props.entities
    return props.entities.filter((entity) =>
      JSON.stringify(entity).toLowerCase().includes(query)
    )
  }, [filter, props.entities])

  return (
    <section aria-labelledby={`${props.title}-title`}>
      <div
        style={{
          alignItems: 'center',
          display: 'flex',
          gap: 12,
          justifyContent: 'space-between',
          marginBottom: 14,
        }}
      >
        <h2 id={`${props.title}-title`} style={{ fontSize: 18, margin: 0 }}>
          {t(props.title)}
        </h2>
        <input
          aria-label={t('Filter entities')}
          onChange={(event) => setFilter(event.target.value)}
          placeholder={t('Filter entities')}
          value={filter}
        />
      </div>
      <div
        style={{
          border: '1px solid #d7dee8',
          maxHeight: 460,
          overflow: 'auto',
        }}
      >
        <table
          style={{
            borderCollapse: 'collapse',
            minWidth: '100%',
            width: '100%',
          }}
        >
          <thead>
            <tr>
              <th style={{ textAlign: 'left' }}>{t('Business ID')}</th>
              <th style={{ textAlign: 'left' }}>{t('Source row')}</th>
              <th style={{ textAlign: 'left' }}>{t('Details')}</th>
            </tr>
          </thead>
          <tbody>
            {items.map((entity) => (
              <tr key={String(entity.business_id)}>
                <td>{String(entity.business_id)}</td>
                <td>{sourceLocation(entity)}</td>
                <td>
                  <code>{JSON.stringify(entity)}</code>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </section>
  )
}
