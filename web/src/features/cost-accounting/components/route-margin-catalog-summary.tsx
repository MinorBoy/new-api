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

import { Skeleton } from '@/components/ui/skeleton'

import type { RouteMarginCatalogSummary as Summary } from '../types'

export function RouteMarginCatalogSummary(props: {
  summary?: Summary
  loading: boolean
}) {
  const { t } = useTranslation()
  const metrics = [
    [t('Route targets'), props.summary?.target_count],
    [t('Scenario rows'), props.summary?.scenario_count],
    [t('Eligible targets'), props.summary?.eligible_target_count],
    [t('Fully eligible'), props.summary?.fully_eligible_target_count],
    [t('Partially eligible'), props.summary?.partially_eligible_target_count],
    [t('Ineligible targets'), props.summary?.ineligible_target_count],
    [t('Eligible scenario rows'), props.summary?.eligible_scenario_count],
  ] as const

  return (
    <div className='grid min-h-18 grid-cols-2 divide-x divide-y border-y sm:grid-cols-4 xl:grid-cols-7 xl:divide-y-0'>
      {metrics.map(([label, value]) => (
        <div key={label} className='min-w-0 px-3 py-2.5'>
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
