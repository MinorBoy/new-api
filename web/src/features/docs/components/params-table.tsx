import { useTranslation } from 'react-i18next'

import { cn } from '@/lib/utils'

import { resolveDocLocale } from '../lib/resolve-doc'
import { useDocLocale } from '../lib/use-doc-locale'
import type { ParamField, ParamRequired } from '../types'

const REQUIRED_LABEL: Record<
  ParamRequired,
  { en: string; zh: string; cls: string }
> = {
  yes: { en: 'Yes', zh: '是', cls: 'text-rose-600 dark:text-rose-400' },
  no: { en: 'No', zh: '否', cls: 'text-muted-foreground' },
  conditional: {
    en: 'Conditional',
    zh: '条件',
    cls: 'text-amber-600 dark:text-amber-400',
  },
}

/**
 * A parameter table for request or response fields, 4stoken-style:
 * 参数 | 类型 | 必填 | 说明. Nested fields use dotted/bracket notation.
 */
export function ParamsTable({
  fields,
  requiredHeader,
  className,
}: {
  fields: ParamField[]
  /** Column header for the third column: "必填" for requests, "返回" for responses. */
  requiredHeader: string
  className?: string
}) {
  const { t } = useTranslation()
  const locale = useDocLocale()

  if (fields.length === 0) {
    return (
      <p className='text-muted-foreground text-sm'>{t('No parameters.')}</p>
    )
  }

  return (
    <div className={cn('w-full overflow-x-auto', className)}>
      <table className='w-full border-collapse text-sm'>
        <thead>
          <tr className='bg-muted/40 border-b'>
            <th className='px-3 py-2 text-left font-semibold'>
              {t('Parameter')}
            </th>
            <th className='px-3 py-2 text-left font-semibold'>{t('Type')}</th>
            <th className='px-3 py-2 text-left font-semibold whitespace-nowrap'>
              {requiredHeader}
            </th>
            <th className='px-3 py-2 text-left font-semibold'>
              {t('Description')}
            </th>
          </tr>
        </thead>
        <tbody>
          {fields.map((field) => {
            const req = REQUIRED_LABEL[field.required]
            return (
              <tr key={field.name} className='border-b last:border-0'>
                <td className='px-3 py-2 align-top font-mono text-xs whitespace-nowrap'>
                  {field.name}
                </td>
                <td className='text-muted-foreground px-3 py-2 align-top font-mono text-xs whitespace-nowrap'>
                  {field.type}
                </td>
                <td
                  className={cn(
                    'px-3 py-2 align-top text-xs font-medium',
                    req.cls
                  )}
                >
                  {resolveDocLocale(req, locale)}
                </td>
                <td className='text-muted-foreground px-3 py-2 align-top'>
                  {resolveDocLocale(field.description, locale)}
                </td>
              </tr>
            )
          })}
        </tbody>
      </table>
    </div>
  )
}
