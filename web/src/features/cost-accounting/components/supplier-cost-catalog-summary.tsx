import { useTranslation } from 'react-i18next'

import { Skeleton } from '@/components/ui/skeleton'

import type { CostCatalogSummary } from '../types'

export function SupplierCostCatalogSummary(props: {
  summary?: CostCatalogSummary
  loading: boolean
}) {
  const { t } = useTranslation()
  const metrics = [
    [t('Channel count'), props.summary?.channel_count],
    [t('Active rules'), props.summary?.active_rule_count],
    [t('Draft rules'), props.summary?.draft_rule_count],
    [t('Retired rules'), props.summary?.retired_rule_count],
  ] as const

  return (
    <div className='grid min-h-18 grid-cols-2 divide-x divide-y border-y lg:grid-cols-4 lg:divide-y-0'>
      {metrics.map(([label, value]) => (
        <div key={label} className='min-w-0 px-3 py-2.5 sm:px-4'>
          <p className='text-muted-foreground truncate text-xs'>{label}</p>
          {props.loading ? (
            <Skeleton className='mt-2 h-5 w-14' />
          ) : (
            <p className='mt-1 font-mono text-lg font-semibold tabular-nums'>
              {value ?? 0}
            </p>
          )}
        </div>
      ))}
    </div>
  )
}
