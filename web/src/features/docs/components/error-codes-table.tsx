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
          <tr className='bg-muted/40 border-b'>
            <th className='px-3 py-2 text-left font-semibold whitespace-nowrap'>
              {t('HTTP Status')}
            </th>
            <th className='px-3 py-2 text-left font-semibold'>
              {t('Description')}
            </th>
          </tr>
        </thead>
        <tbody>
          {rows.map((row) => (
            <tr key={row.status} className='border-b last:border-0'>
              <td className='px-3 py-2 align-top font-mono text-xs whitespace-nowrap'>
                {row.status}
              </td>
              <td className='text-muted-foreground px-3 py-2 align-top'>
                {resolveDocLocale(row.description, locale)}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}
