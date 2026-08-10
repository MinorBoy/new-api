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
import { useState } from 'react'
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
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'

import type { CostAccountingSearch } from '../lib/report'
import { updateRouteMarginSearch } from '../lib/route-margin-catalog'
import type {
  RouteMarginCatalogFacets,
  RouteMarginScenario,
  RouteMarginStatus,
} from '../types'

const STATUS_OPTIONS: Array<{ value: RouteMarginStatus; label: string }> = [
  { value: 'all', label: 'All results' },
  { value: 'eligible', label: 'Eligible' },
  { value: 'ineligible', label: 'Ineligible' },
]

const SCENARIO_OPTIONS: Array<{
  value: RouteMarginScenario
  label: string
}> = [
  { value: 'all', label: 'All scenarios' },
  { value: 'no_video', label: 'No video' },
  { value: 'with_video', label: 'With video' },
]

export function RouteMarginCatalogFilters(props: {
  search: CostAccountingSearch
  facets?: RouteMarginCatalogFacets
  onSearchChange: (search: CostAccountingSearch) => void
  onRefresh: () => void
  onExport: () => void
  exporting: boolean
}) {
  const { t } = useTranslation()
  const [mode, setMode] = useState<'quick' | 'advanced'>('quick')
  const apply = (patch: Partial<CostAccountingSearch>) =>
    props.onSearchChange(updateRouteMarginSearch(props.search, patch))
  const channelOptions = [
    { value: 'all', label: t('All channels') },
    ...(props.facets?.channels ?? []).map((channel) => ({
      value: String(channel.id),
      label: `${channel.name || t('Unknown channel')} · #${channel.id}`,
    })),
  ]
  const modelOptions = [
    { value: 'all', label: t('All models') },
    ...(props.facets?.canonical_models ?? []).map((model) => ({
      value: model,
      label: model,
    })),
  ]
  const resolutionOptions = [
    { value: 'all', label: t('All resolutions') },
    ...(props.facets?.resolutions ?? []).map((resolution) => ({
      value: resolution,
      label: resolution,
    })),
  ]

  return (
    <div className='flex flex-col gap-3 border-b pb-3'>
      <div className='flex flex-wrap items-center justify-between gap-2'>
        <ToggleGroup
          value={[mode]}
          onValueChange={(values) => {
            const next = values[0]
            if (next === 'quick' || next === 'advanced') setMode(next)
          }}
          aria-label={t('Filter mode')}
        >
          <ToggleGroupItem value='quick'>{t('Quick mode')}</ToggleGroupItem>
          <ToggleGroupItem value='advanced'>
            {t('Advanced mode')}
          </ToggleGroupItem>
        </ToggleGroup>
        <div className='flex flex-wrap items-center justify-end gap-2'>
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
            disabled={props.exporting}
            onClick={props.onExport}
          >
            {props.exporting ? (
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
        </div>
      </div>
      <div className='grid gap-2 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-5'>
        <Field>
          <FieldLabel htmlFor='route-margin-minimum'>
            {t('Minimum margin')}
          </FieldLabel>
          <div className='flex items-center gap-1.5'>
            <Input
              key={props.search.marginMinimumPercent ?? 30}
              id='route-margin-minimum'
              type='number'
              min={-100}
              max={100}
              step={0.0001}
              defaultValue={props.search.marginMinimumPercent ?? 30}
              onBlur={(event) => {
                const value = Number(event.target.value)
                if (Number.isFinite(value) && value >= -100 && value <= 100) {
                  apply({ marginMinimumPercent: value })
                }
              }}
              onKeyDown={(event) => {
                if (event.key === 'Enter') event.currentTarget.blur()
              }}
            />
            <span className='text-muted-foreground text-sm'>%</span>
          </div>
        </Field>
        <MarginSelect
          label={t('Status')}
          value={props.search.marginStatus ?? 'all'}
          options={STATUS_OPTIONS.map((option) => ({
            value: option.value,
            label: t(option.label),
          }))}
          onChange={(value) =>
            apply({ marginStatus: value as RouteMarginStatus })
          }
        />
        <Field>
          <FieldLabel>{t('Channel')}</FieldLabel>
          <Combobox
            options={channelOptions}
            value={String(props.search.marginChannelId ?? 'all')}
            ariaLabel={t('Channel')}
            onValueChange={(value) =>
              apply({
                marginChannelId:
                  value && value !== 'all' ? Number(value) : undefined,
              })
            }
          />
        </Field>
        <Field>
          <FieldLabel>{t('Model')}</FieldLabel>
          <Combobox
            options={modelOptions}
            value={props.search.marginModel || 'all'}
            ariaLabel={t('Model')}
            onValueChange={(value) =>
              apply({
                marginModel: value && value !== 'all' ? value : undefined,
              })
            }
          />
        </Field>
        <Field>
          <FieldLabel htmlFor='route-margin-target'>
            {t('Route target')}
          </FieldLabel>
          <Input
            key={props.search.marginRouteTarget ?? ''}
            id='route-margin-target'
            defaultValue={props.search.marginRouteTarget ?? ''}
            onBlur={(event) =>
              apply({ marginRouteTarget: event.target.value || undefined })
            }
            onKeyDown={(event) => {
              if (event.key === 'Enter') event.currentTarget.blur()
            }}
          />
        </Field>
      </div>
      {mode === 'advanced' ? (
        <div className='grid gap-2 border-t pt-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-5'>
          <Field>
            <FieldLabel htmlFor='route-margin-ratio'>
              {t('Group ratio')}
            </FieldLabel>
            <Input
              key={props.search.marginGroupRatio ?? 1}
              id='route-margin-ratio'
              type='number'
              min={0.0001}
              max={100}
              step={0.01}
              defaultValue={props.search.marginGroupRatio ?? 1}
              onBlur={(event) => {
                const value = Number(event.target.value)
                if (Number.isFinite(value) && value > 0 && value <= 100) {
                  apply({ marginGroupRatio: value })
                }
              }}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor='route-margin-duration'>
              {t('Duration')}
            </FieldLabel>
            <Input
              key={props.search.marginDurationSeconds ?? 4}
              id='route-margin-duration'
              type='number'
              min={1}
              max={3600}
              step={1}
              defaultValue={props.search.marginDurationSeconds ?? 4}
              onBlur={(event) => {
                const value = Number(event.target.value)
                if (
                  Number.isSafeInteger(value) &&
                  value >= 1 &&
                  value <= 3600
                ) {
                  apply({ marginDurationSeconds: value })
                }
              }}
            />
          </Field>
          <MarginSelect
            label={t('Resolution')}
            value={props.search.marginResolution || 'all'}
            options={resolutionOptions}
            onChange={(value) =>
              apply({
                marginResolution: value && value !== 'all' ? value : undefined,
              })
            }
          />
          <MarginSelect
            label={t('Material scenario')}
            value={props.search.marginScenario ?? 'all'}
            options={SCENARIO_OPTIONS.map((option) => ({
              value: option.value,
              label: t(option.label),
            }))}
            onChange={(value) =>
              apply({ marginScenario: value as RouteMarginScenario })
            }
          />
          <Field>
            <FieldLabel htmlFor='route-margin-upstream-model'>
              {t('Upstream model')}
            </FieldLabel>
            <Input
              key={props.search.marginUpstreamModel ?? ''}
              id='route-margin-upstream-model'
              defaultValue={props.search.marginUpstreamModel ?? ''}
              onBlur={(event) =>
                apply({ marginUpstreamModel: event.target.value || undefined })
              }
            />
          </Field>
        </div>
      ) : null}
    </div>
  )
}

function MarginSelect(props: {
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
