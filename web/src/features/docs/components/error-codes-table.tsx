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

import { cn } from '@/lib/utils'

import { resolveDocLocale } from '../lib/resolve-doc'
import { useDocLocale } from '../lib/use-doc-locale'
import type { ErrorCodeRow } from '../types'

/**
 * HTTP error-code table: 状态码 | 说明. Reused on every endpoint detail page
 * and on the standalone error-codes guide.
 */
export function ErrorCodesTable({
  rows,
  className,
}: {
  rows: ErrorCodeRow[]
  className?: string
}) {
  const { t } = useTranslation()
  const locale = useDocLocale()

  return (
    <div className={cn('w-full overflow-x-auto', className)}>
      <table className='w-full border-collapse text-sm'>
        <thead>
          <tr className='border-b bg-muted/40'>
            <th className='px-3 py-2 text-left font-semibold whitespace-nowrap'>
              {t('HTTP Status')}
            </th>
            <th className='px-3 py-2 text-left font-semibold'>{t('Description')}</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((row) => (
            <tr key={row.status} className='border-b last:border-0'>
              <td className='px-3 py-2 align-top font-mono text-xs whitespace-nowrap'>
                {row.status}
              </td>
              <td className='px-3 py-2 align-top text-muted-foreground'>
                {resolveDocLocale(row.description, locale)}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}
