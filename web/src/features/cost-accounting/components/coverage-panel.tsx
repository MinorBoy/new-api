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

import { Badge } from '@/components/ui/badge'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

import type { CostCoverageItem } from '../types'

type CoveragePanelProps = {
  items: CostCoverageItem[]
}

function coverageReason(
  reason: string | undefined,
  t: (key: string) => string
) {
  if (reason === 'invalid_model_mapping') return t('Invalid model mapping')
  if (reason === 'missing_or_incompatible_cost_rule') {
    return t('Missing or incompatible cost rule')
  }
  return reason || t('Rule is not covered')
}

export function CoveragePanel(props: CoveragePanelProps) {
  const { t } = useTranslation()
  const uncovered = props.items.filter((item) => !item.covered)
  const coveredCount = props.items.length - uncovered.length

  return (
    <section className='border-border/60 flex flex-col gap-3 border-b pb-4'>
      <div className='flex flex-wrap items-center justify-between gap-2'>
        <div>
          <h3 className='text-sm font-semibold'>{t('Coverage check')}</h3>
          <p className='text-muted-foreground text-xs'>
            {t('Authoritative coverage for enabled channel model mappings.')}
          </p>
        </div>
        <div className='flex items-center gap-2'>
          <Badge variant='secondary'>
            {t('{{count}} covered', { count: coveredCount })}
          </Badge>
          <Badge variant={uncovered.length > 0 ? 'destructive' : 'outline'}>
            {t('{{count}} uncovered', { count: uncovered.length })}
          </Badge>
        </div>
      </div>

      {uncovered.length > 0 ? (
        <div className='max-h-40 overflow-y-auto rounded-md border'>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('Client model')}</TableHead>
                <TableHead>{t('Billable upstream model')}</TableHead>
                <TableHead>cost_variant_key</TableHead>
                <TableHead>{t('Reason')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {uncovered.map((item) => (
                <TableRow
                  key={`${item.origin_model}:${item.predicted_upstream_model}:${item.cost_variant_key || 'default'}`}
                >
                  <TableCell className='font-mono text-xs'>
                    {item.origin_model}
                  </TableCell>
                  <TableCell className='font-mono text-xs'>
                    {item.predicted_upstream_model}
                  </TableCell>
                  <TableCell className='font-mono text-xs'>
                    {item.cost_variant_key || 'default'}
                  </TableCell>
                  <TableCell className='text-muted-foreground max-w-56 whitespace-normal'>
                    {coverageReason(item.reason, t)}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      ) : null}
    </section>
  )
}
