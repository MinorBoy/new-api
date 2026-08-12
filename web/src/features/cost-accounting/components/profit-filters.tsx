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
import { FilterX, Search } from 'lucide-react'
import {
  useEffect,
  useState,
  type CompositionEvent,
  type HTMLAttributes,
  type HTMLInputTypeAttribute,
} from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Combobox } from '@/components/ui/combobox'
import type { ComboboxInputOption } from '@/components/ui/combobox-input'
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
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import dayjs from '@/lib/dayjs'

import type { ProfitFilterOptions } from '../hooks/use-profit-filter-options'
import type { CostAccountingSearch } from '../lib/report'

type ProfitFiltersProps = {
  search: CostAccountingSearch
  onChange: (search: CostAccountingSearch) => void
  filterOptions: ProfitFilterOptions
}

type FilterDraft = {
  startTime: string
  endTime: string
  channelId: string
  billableModel: string
  originModel: string
  userGroup: string
  usingGroup: string
  billingSource: string
  status: string
}

function optionalText(value: string | undefined): string | undefined {
  const trimmed = value?.trim() ?? ''
  return trimmed || undefined
}

function dateInput(value: number | undefined): string {
  return value ? dayjs.unix(value).format('YYYY-MM-DDTHH:mm') : ''
}

function timestamp(value: string): number | undefined {
  if (!value) return undefined
  const parsed = dayjs(value)
  return parsed.isValid() ? parsed.unix() : undefined
}

function draftFromSearch(search: CostAccountingSearch): FilterDraft {
  return {
    startTime: dateInput(search.startTime),
    endTime: dateInput(search.endTime),
    channelId: search.channelId ? String(search.channelId) : '',
    billableModel: search.billableModel ?? '',
    originModel: search.originModel ?? '',
    userGroup: search.userGroup ?? '',
    usingGroup: search.usingGroup ?? '',
    billingSource: search.billingSource ?? '',
    status: search.status ?? '',
  }
}

export function ProfitFilters(props: ProfitFiltersProps) {
  const { t } = useTranslation()
  const [draft, setDraft] = useState<FilterDraft>(() =>
    draftFromSearch(props.search)
  )

  useEffect(() => {
    setDraft(draftFromSearch(props.search))
  }, [props.search])

  const updateDraft = (field: keyof FilterDraft, value: string) => {
    setDraft((current) => ({ ...current, [field]: value }))
  }

  const apply = () => {
    const channelID = Number(draft.channelId)
    props.onChange({
      ...props.search,
      startTime: timestamp(draft.startTime),
      endTime: timestamp(draft.endTime),
      channelId:
        draft.channelId && Number.isInteger(channelID) && channelID > 0
          ? channelID
          : undefined,
      billableModel: optionalText(draft.billableModel),
      originModel: optionalText(draft.originModel),
      userGroup: optionalText(draft.userGroup),
      usingGroup: optionalText(draft.usingGroup),
      billingSource: optionalText(draft.billingSource),
      status: optionalText(draft.status),
    })
  }

  const reset = () => {
    const next: CostAccountingSearch = {
      tab: props.search.tab,
      timeBasis: props.search.timeBasis ?? 'profit_recognized_at',
    }
    setDraft(draftFromSearch(next))
    props.onChange(next)
  }

  return (
    <div className='border-border/60 flex flex-col gap-3 border-y py-3'>
      <div className='flex flex-wrap items-end gap-3'>
        <Field className='w-full sm:w-auto'>
          <FieldLabel>{t('Time basis')}</FieldLabel>
          <ToggleGroup
            value={[props.search.timeBasis ?? 'profit_recognized_at']}
            onValueChange={(selection) => {
              const value = selection[0]
              if (
                value === 'profit_recognized_at' ||
                value === 'requested_at'
              ) {
                props.onChange({ ...props.search, timeBasis: value })
              }
            }}
          >
            <ToggleGroupItem value='profit_recognized_at'>
              {t('Profit recognized')}
            </ToggleGroupItem>
            <ToggleGroupItem value='requested_at'>
              {t('Requested')}
            </ToggleGroupItem>
          </ToggleGroup>
        </Field>
        <TextFilter
          id='profit-start-time'
          label={t('Start time')}
          value={draft.startTime}
          type='datetime-local'
          onChange={(value) => updateDraft('startTime', value)}
        />
        <TextFilter
          id='profit-end-time'
          label={t('End time')}
          value={draft.endTime}
          type='datetime-local'
          onChange={(value) => updateDraft('endTime', value)}
        />
        <ComboboxFilter
          id='profit-channel-id'
          label={t('Channel ID')}
          value={draft.channelId}
          options={props.filterOptions.channels}
          className='w-28'
          onChange={(value) => updateDraft('channelId', value)}
        />
        <ComboboxFilter
          id='profit-billable-model'
          label={t('Billable upstream model')}
          value={draft.billableModel}
          options={props.filterOptions.billableModels}
          className='w-52'
          onChange={(value) => updateDraft('billableModel', value)}
        />
        <ComboboxFilter
          id='profit-origin-model'
          label={t('Origin model')}
          value={draft.originModel}
          options={props.filterOptions.originModels}
          className='w-44'
          onChange={(value) => updateDraft('originModel', value)}
        />
        <ComboboxFilter
          id='profit-user-group'
          label={t('User group')}
          value={draft.userGroup}
          options={props.filterOptions.userGroups}
          className='w-32'
          onChange={(value) => updateDraft('userGroup', value)}
        />
        <ComboboxFilter
          id='profit-using-group'
          label={t('Using group')}
          value={draft.usingGroup}
          options={props.filterOptions.usingGroups}
          className='w-32'
          onChange={(value) => updateDraft('usingGroup', value)}
        />
        <Field className='w-40'>
          <FieldLabel>{t('Billing source')}</FieldLabel>
          <Select
            value={draft.billingSource || 'all'}
            onValueChange={(value) =>
              updateDraft('billingSource', value === 'all' ? '' : (value ?? ''))
            }
          >
            <SelectTrigger className='w-full'>
              <SelectValue />
            </SelectTrigger>
            <SelectContent align='start'>
              <SelectGroup>
                <SelectItem value='all'>{t('All sources')}</SelectItem>
                <SelectItem value='wallet'>{t('Wallet')}</SelectItem>
                <SelectItem value='subscription'>
                  {t('Subscription')}
                </SelectItem>
              </SelectGroup>
            </SelectContent>
          </Select>
        </Field>
        <Field className='w-44'>
          <FieldLabel>{t('Profit status')}</FieldLabel>
          <Select
            value={draft.status || 'all'}
            onValueChange={(value) =>
              updateDraft('status', value === 'all' ? '' : (value ?? ''))
            }
          >
            <SelectTrigger className='w-full'>
              <SelectValue />
            </SelectTrigger>
            <SelectContent align='start'>
              <SelectGroup>
                <SelectItem value='all'>{t('All statuses')}</SelectItem>
                <SelectItem value='complete'>{t('Profit complete')}</SelectItem>
                <SelectItem value='incomplete_cost'>
                  {t('Incomplete cost')}
                </SelectItem>
                <SelectItem value='incomplete_revenue'>
                  {t('Incomplete revenue')}
                </SelectItem>
              </SelectGroup>
            </SelectContent>
          </Select>
        </Field>
      </div>
      <div className='flex justify-end gap-2'>
        <Button type='button' variant='ghost' onClick={reset}>
          <FilterX data-icon='inline-start' aria-hidden='true' />
          {t('Reset filters')}
        </Button>
        <Button type='button' onClick={apply}>
          <Search data-icon='inline-start' aria-hidden='true' />
          {t('Apply filters')}
        </Button>
      </div>
    </div>
  )
}

function TextFilter(props: {
  id: string
  label: string
  value: string
  onChange: (value: string) => void
  type?: HTMLInputTypeAttribute
  inputMode?: HTMLAttributes<HTMLInputElement>['inputMode']
  className?: string
}) {
  return (
    <Field className={props.className ?? 'w-48'}>
      <FieldLabel htmlFor={props.id}>{props.label}</FieldLabel>
      <Input
        id={props.id}
        type={props.type}
        inputMode={props.inputMode}
        value={props.value}
        onChange={(event) => props.onChange(event.target.value)}
      />
    </Field>
  )
}

function ComboboxFilter(props: {
  id: string
  label: string
  value: string
  options: ComboboxInputOption[]
  onChange: (value: string) => void
  className?: string
}) {
  const [isComposing, setIsComposing] = useState(false)

  return (
    <Field className={props.className ?? 'w-48'}>
      <FieldLabel htmlFor={props.id}>{props.label}</FieldLabel>
      <Combobox
        id={props.id}
        ariaLabel={props.label}
        options={props.options}
        value={props.value}
        onValueChange={(value) => {
          if (!isComposing) props.onChange(value ?? '')
        }}
        onCompositionStart={() => setIsComposing(true)}
        onCompositionEnd={(event: CompositionEvent<HTMLInputElement>) => {
          setIsComposing(false)
          props.onChange(event.currentTarget.value)
        }}
        allowCustomValue
        openOnFocus
        className='w-full'
      />
    </Field>
  )
}
