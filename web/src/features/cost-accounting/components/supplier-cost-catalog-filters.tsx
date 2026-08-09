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
import { Download04Icon, RefreshIcon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Combobox } from '@/components/ui/combobox'
import { Field, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Spinner } from '@/components/ui/spinner'

import { updateCatalogSearch } from '../lib/catalog'
import type { CostAccountingSearch } from '../lib/report'
import type { CostCatalogFacets, CostMode, CostRuleStatus } from '../types'

const COST_MODES: Array<{ value: CostMode | 'all'; label: string }> = [
  { value: 'all', label: 'All cost modes' },
  { value: 'free', label: 'Free' },
  { value: 'per_request', label: 'Per request' },
  { value: 'per_duration', label: 'Per duration' },
  { value: 'per_token', label: 'Per token' },
]

const COST_STATUSES: Array<{
  value: CostRuleStatus | 'all'
  label: string
}> = [
  { value: 'active', label: 'Active' },
  { value: 'draft', label: 'Draft' },
  { value: 'retired', label: 'Retired' },
  { value: 'all', label: 'All statuses' },
]

export function SupplierCostCatalogFilters(props: {
  search: CostAccountingSearch
  facets?: CostCatalogFacets
  onSearchChange: (search: CostAccountingSearch) => void
  onRefresh: () => void
  onExport: (scope: 'filtered' | 'all') => void
  exportingScope: 'filtered' | 'all' | null
}) {
  const { t } = useTranslation()
  const [model, setModel] = useState(props.search.catalogModel ?? '')
  useEffect(
    () => setModel(props.search.catalogModel ?? ''),
    [props.search.catalogModel]
  )

  const apply = (patch: Partial<CostAccountingSearch>) =>
    props.onSearchChange(updateCatalogSearch(props.search, patch))
  const channelOptions = [
    { value: 'all', label: t('All channels') },
    ...(props.facets?.channels ?? []).map((channel) => ({
      value: String(channel.id),
      label: `${channel.name || t('Unknown channel')} · #${channel.id}`,
    })),
  ]
  const currencyValues = ['all', ...(props.facets?.currencies ?? [])]
  const sourceValues = ['all', ...(props.facets?.sources ?? [])]

  return (
    <div className='flex flex-col gap-3 border-b pb-3'>
      <div className='grid gap-2 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-6'>
        <Field>
          <FieldLabel>{t('Channel')}</FieldLabel>
          <Combobox
            options={channelOptions}
            value={String(props.search.catalogChannelId ?? 'all')}
            ariaLabel={t('Channel')}
            onValueChange={(value) =>
              apply({
                catalogChannelId:
                  value && value !== 'all' ? Number(value) : undefined,
              })
            }
          />
        </Field>
        <Field>
          <FieldLabel htmlFor='supplier-cost-model'>
            {t('Billable upstream model')}
          </FieldLabel>
          <Input
            id='supplier-cost-model'
            value={model}
            onChange={(event) => setModel(event.target.value)}
            onBlur={() => apply({ catalogModel: model || undefined })}
            onKeyDown={(event) => {
              if (event.key === 'Enter') event.currentTarget.blur()
            }}
          />
        </Field>
        <CatalogSelect
          label={t('Cost mode')}
          value={props.search.catalogCostMode ?? 'all'}
          options={COST_MODES.map((item) => ({
            value: item.value,
            label: t(item.label),
          }))}
          onChange={(value) =>
            apply({
              catalogCostMode:
                value === 'all' ? undefined : (value as CostMode),
            })
          }
        />
        <CatalogSelect
          label={t('Status')}
          value={props.search.catalogStatus ?? 'active'}
          options={COST_STATUSES.map((item) => ({
            value: item.value,
            label: t(item.label),
          }))}
          onChange={(value) =>
            apply({ catalogStatus: value as CostRuleStatus | 'all' })
          }
        />
        <CatalogSelect
          label={t('Currency')}
          value={props.search.catalogCurrency ?? 'all'}
          options={currencyValues.map((value) => ({
            value,
            label: value === 'all' ? t('All currencies') : value,
          }))}
          onChange={(value) =>
            apply({ catalogCurrency: value === 'all' ? undefined : value })
          }
        />
        <CatalogSelect
          label={t('Source')}
          value={props.search.catalogSource ?? 'all'}
          options={sourceValues.map((value) => ({
            value,
            label: value === 'all' ? t('All sources') : value,
          }))}
          onChange={(value) =>
            apply({ catalogSource: value === 'all' ? undefined : value })
          }
        />
      </div>
      <div className='flex flex-wrap items-center justify-end gap-2'>
        <Button
          type='button'
          variant='ghost'
          onClick={() =>
            apply({
              catalogChannelId: undefined,
              catalogModel: undefined,
              catalogCostMode: undefined,
              catalogStatus: 'active',
              catalogCurrency: undefined,
              catalogSource: undefined,
              catalogPageSize: 50,
              catalogSort: 'channel_name',
              catalogOrder: 'asc',
            })
          }
        >
          {t('Reset filters')}
        </Button>
        <Button type='button' variant='outline' onClick={props.onRefresh}>
          <HugeiconsIcon
            icon={RefreshIcon}
            data-icon='inline-start'
            strokeWidth={2}
          />
          {t('Refresh')}
        </Button>
        <Button
          type='button'
          variant='outline'
          disabled={props.exportingScope === 'filtered'}
          onClick={() => props.onExport('filtered')}
        >
          {props.exportingScope === 'filtered' ? (
            <Spinner data-icon='inline-start' />
          ) : (
            <HugeiconsIcon
              icon={Download04Icon}
              data-icon='inline-start'
              strokeWidth={2}
            />
          )}
          {t('Export current results')}
        </Button>
        <Button
          type='button'
          variant='outline'
          disabled={props.exportingScope === 'all'}
          onClick={() => props.onExport('all')}
        >
          {props.exportingScope === 'all' ? (
            <Spinner data-icon='inline-start' />
          ) : (
            <HugeiconsIcon
              icon={Download04Icon}
              data-icon='inline-start'
              strokeWidth={2}
            />
          )}
          {t('Export all supplier costs')}
        </Button>
      </div>
    </div>
  )
}

function CatalogSelect(props: {
  label: string
  value: string
  options: Array<{ value: string; label: string }>
  onChange: (value: string) => void
}) {
  return (
    <Field>
      <FieldLabel>{props.label}</FieldLabel>
      <Select
        items={props.options}
        value={props.value}
        onValueChange={(value) => value && props.onChange(value)}
      >
        <SelectTrigger className='w-full' aria-label={props.label}>
          <SelectValue />
        </SelectTrigger>
        <SelectContent align='start'>
          <SelectGroup>
            {props.options.map((option) => (
              <SelectItem key={option.value} value={option.value}>
                {option.label}
              </SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
    </Field>
  )
}
