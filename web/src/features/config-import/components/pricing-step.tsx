/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import type { ConfigImportBatchDetail, ConfigImportItemDetail } from '../types'
import { DiffTable, type DiffTableColumn } from './diff-table'

interface PricingRow {
  key: string
  proposalID: string
  field: string
  current: string
  proposed: string
  recomputed: string
  margin: string
  severity: string
  source: string
}

export interface PricingStepProps {
  batch: ConfigImportBatchDetail
  currentValues?: Record<string, string>
  onSelectedGroupsChange?: (groups: string[]) => void
}

function parseObject(value: string): Record<string, unknown> {
  try {
    const parsed = JSON.parse(value)
    return parsed !== null && typeof parsed === 'object'
      ? (parsed as Record<string, unknown>)
      : {}
  } catch {
    return {}
  }
}

function source(item: ConfigImportItemDetail): string {
  return `${item.source_sheet}${item.source_row ? `:${item.source_row}` : ''}`
}

function pricingRows(
  batch: ConfigImportBatchDetail,
  currentValues: Record<string, string>
): PricingRow[] {
  const recomputed = batch.items
    .filter((item) => item.entity_type === 'cost_rule_drafts')
    .map((item) => parseObject(item.canonical_json))
    .flatMap((item) =>
      Object.entries(item)
        .filter(([key]) => key.startsWith('normalized_usd_'))
        .map(([, value]) => (typeof value === 'string' ? value : ''))
    )
  return batch.items
    .filter((item) => item.entity_type === 'sale_proposals')
    .flatMap((item) => {
      const proposal = parseObject(item.canonical_json)
      const margin =
        typeof proposal.margin_ratio === 'string' ? proposal.margin_ratio : '--'
      const fields = [
        'unit_price',
        'price_per_unit',
        'input_per_million',
        'output_per_million',
        'completion_per_million',
        'total_per_million',
      ]
      return fields.flatMap((field) => {
        const proposed = proposal[field]
        if (typeof proposed !== 'string') return []
        return [
          {
            key: `${item.business_id}:${field}`,
            proposalID: item.business_id,
            field,
            current: currentValues[`${item.business_id}:${field}`] ?? '--',
            proposed,
            recomputed: recomputed[0] ?? '--',
            margin,
            severity:
              batch.issues.find(
                (issue) => issue.business_id === item.business_id
              )?.severity ?? 'info',
            source: source(item),
          },
        ]
      })
    })
}

function pricingGroups(batch: ConfigImportBatchDetail): string[] {
  const groups = new Set<string>(['default'])
  for (const item of batch.items) {
    if (item.entity_type !== 'sale_proposals') continue
    const proposal = parseObject(item.canonical_json)
    if (Array.isArray(proposal.selected_groups)) {
      for (const group of proposal.selected_groups) {
        if (typeof group === 'string') groups.add(group)
      }
    }
    if (proposal.group_prices && typeof proposal.group_prices === 'object') {
      for (const group of Object.keys(proposal.group_prices)) groups.add(group)
    }
  }
  return [...groups].sort((left, right) => {
    if (left === 'default') return -1
    if (right === 'default') return 1
    return left.localeCompare(right)
  })
}

export function PricingStep(props: PricingStepProps) {
  const { t } = useTranslation()
  const groups = useMemo(() => pricingGroups(props.batch), [props.batch])
  const initialGroups = useMemo(() => new Set(groups), [groups])
  const [selectedGroups, setSelectedGroups] = useState(initialGroups)
  const rows = useMemo(
    () => pricingRows(props.batch, props.currentValues ?? {}),
    [props.batch, props.currentValues]
  )
  const columns: DiffTableColumn<PricingRow>[] = [
    { id: 'field', header: t('Field'), cell: (row) => row.field },
    { id: 'current', header: t('Current'), cell: (row) => row.current },
    { id: 'proposed', header: t('Proposed'), cell: (row) => row.proposed },
    {
      id: 'recomputed',
      header: t('Server recomputation'),
      cell: (row) => row.recomputed,
    },
    { id: 'margin', header: t('Margin'), cell: (row) => row.margin },
    { id: 'severity', header: t('Severity'), cell: (row) => row.severity },
    { id: 'source', header: t('Source'), cell: (row) => row.source },
  ]

  const toggleGroup = (group: string, checked: boolean) => {
    setSelectedGroups((current) => {
      const next = new Set(current)
      if (checked) next.add(group)
      else next.delete(group)
      const values = groups.filter((value) => next.has(value))
      props.onSelectedGroupsChange?.(values)
      return next
    })
  }

  return (
    <section
      className='space-y-4'
      aria-labelledby='config-import-pricing-title'
    >
      <div className='border-b pb-3'>
        <h2
          id='config-import-pricing-title'
          className='text-base font-semibold'
        >
          {t('Pricing review')}
        </h2>
      </div>
      <fieldset className='flex flex-wrap gap-x-4 gap-y-2'>
        <legend className='mb-2 text-sm font-medium'>{t('Groups')}</legend>
        {groups.map((group) => (
          <label key={group} className='flex items-center gap-2 text-sm'>
            <input
              type='checkbox'
              aria-label={group}
              checked={selectedGroups.has(group)}
              onChange={(event) => toggleGroup(group, event.target.checked)}
            />
            {group}
          </label>
        ))}
      </fieldset>
      <DiffTable
        columns={columns}
        rows={rows}
        getRowKey={(row) => row.key}
        emptyMessage={t('No pricing proposals found')}
      />
    </section>
  )
}
