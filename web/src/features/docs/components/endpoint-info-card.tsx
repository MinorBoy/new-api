import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'

import type { ApiEndpoint } from '../types'
import { MethodBadge } from './method-badge'

/**
 * The endpoint info card shown at the top of each detail page: a key/value
 * table of the full request URL, method, path, Content-Type, and auth scheme.
 * Mirrors the 4stoken "接口信息" block.
 */
export function EndpointInfoCard({ endpoint }: { endpoint: ApiEndpoint }) {
  const { t } = useTranslation()
  const baseUrl =
    typeof window !== 'undefined'
      ? window.location.origin
      : 'https://<your-domain>'
  const fullUrl = `${baseUrl}${endpoint.path}`

  const rows: Array<{
    label: string
    value: string
    mono?: boolean
    copyable?: boolean
  }> = [
    { label: t('Method'), value: endpoint.method, mono: true },
    { label: t('Path'), value: endpoint.path, mono: true, copyable: true },
    { label: t('Full URL'), value: fullUrl, mono: true, copyable: true },
    { label: t('Content-Type'), value: endpoint.contentType, mono: true },
    { label: t('Authentication'), value: endpoint.auth },
  ]

  return (
    <div className='border-border/60 overflow-hidden rounded-lg border'>
      <dl className='divide-border/60 divide-y'>
        {rows.map((row) => (
          <div
            key={row.label}
            className='hover:bg-muted/30 flex items-center gap-3 px-4 py-2.5 transition-colors'
          >
            <dt className='text-muted-foreground w-28 shrink-0 text-xs font-medium'>
              {row.label}
            </dt>
            <dd
              className={
                'flex min-w-0 flex-1 items-center gap-2 text-sm ' +
                (row.mono ? 'font-mono' : '')
              }
            >
              {row.label === t('Method') ? (
                <MethodBadge method={endpoint.method} />
              ) : (
                <span className='truncate'>{row.value}</span>
              )}
              {row.copyable && (
                <CopyButton
                  value={row.value}
                  variant='ghost'
                  size='icon'
                  className='size-6 opacity-60 hover:opacity-100'
                />
              )}
            </dd>
          </div>
        ))}
      </dl>
    </div>
  )
}
