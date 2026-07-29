/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { useEffect, useMemo, useState } from 'react'
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
  availableGroups?: string[]
  onSelectedGroupsChange?: (groups: string[]) => void
  onContinue?: (groups: string[]) => unknown | Promise<unknown>
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

function pricingProposal(stored: Record<string, unknown>) {
  const staged =
    stored.staged_proposal !== null &&
    typeof stored.staged_proposal === 'object'
      ? (stored.staged_proposal as Record<string, unknown>)
      : {}
  const proposalValue =
    staged.proposal && typeof staged.proposal === 'object'
      ? staged.proposal
      : stored
  return proposalValue !== null && typeof proposalValue === 'object'
    ? (proposalValue as Record<string, unknown>)
    : stored
}

function pricingRows(
  batch: ConfigImportBatchDetail,
  currentValues: Record<string, string>
): PricingRow[] {
  const targetToSKU = new Map<string, string>()
  for (const item of batch.items) {
    if (item.entity_type !== 'route_blueprints') continue
    const blueprint = parseObject(item.canonical_json)
    if (!Array.isArray(blueprint.targets)) continue
    for (const target of blueprint.targets) {
      if (target === null || typeof target !== 'object') continue
      const values = target as Record<string, unknown>
      if (
        typeof values.route_target_ref === 'string' &&
        typeof values.sku_ref === 'string'
      ) {
        targetToSKU.set(values.route_target_ref, values.sku_ref)
      }
    }
  }
  const recomputedBySKU = new Map<string, Record<string, string>>()
  const fallbackRecomputed: Record<string, string>[] = []
  for (const item of batch.items.filter(
    (item) => item.entity_type === 'cost_rule_drafts'
  )) {
    const cost = parseObject(item.canonical_json)
    const routeTargetRef = cost.route_target_ref
    const sku =
      typeof routeTargetRef === 'string' ? targetToSKU.get(routeTargetRef) : ''
    const values: Record<string, string> = {}
    for (const [key, value] of Object.entries(cost)) {
      if (key.startsWith('normalized_usd_') && typeof value === 'string') {
        values[key] = value
      }
    }
    fallbackRecomputed.push(values)
    if (!sku) continue
    recomputedBySKU.set(sku, {
      ...recomputedBySKU.get(sku),
      ...values,
    })
  }
  return batch.items
    .filter((item) => item.entity_type === 'sale_proposals')
    .flatMap((item) => {
      const stored = parseObject(item.canonical_json)
      const proposal = pricingProposal(stored)
      const margin =
        typeof proposal.margin_ratio === 'string' ? proposal.margin_ratio : '--'
      const recomputed = recomputedBySKU.get(
        typeof proposal.model_sku_ref === 'string' ? proposal.model_sku_ref : ''
      )
      const serverValues =
        recomputed ??
        (fallbackRecomputed.length === 1 ? fallbackRecomputed[0] : undefined)
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
            recomputed:
              serverValues?.[
                `normalized_usd_${
                  field === 'unit_price' || field === 'price_per_unit'
                    ? 'unit_price'
                    : field.replace('_per_million', '_per_million')
                }`
              ] ?? '--',
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

function pricingGroups(
  batch: ConfigImportBatchDetail,
  availableGroups: string[]
): string[] {
  const groups = new Set<string>(['default', ...availableGroups])
  for (const item of batch.items) {
    if (item.entity_type !== 'sale_proposals') continue
    const proposal = pricingProposal(parseObject(item.canonical_json))
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

function selectedPricingGroups(batch: ConfigImportBatchDetail): Set<string> {
  const selected = new Set<string>()
  for (const item of batch.items) {
    if (item.entity_type !== 'sale_proposals') continue
    const proposal = pricingProposal(parseObject(item.canonical_json))
    if (!Array.isArray(proposal.selected_groups)) continue
    for (const group of proposal.selected_groups) {
      if (typeof group === 'string') selected.add(group)
    }
  }
  if (selected.size === 0) selected.add('default')
  return selected
}

export function PricingStep(props: PricingStepProps) {
  const { t } = useTranslation()
  const groups = useMemo(
    () => pricingGroups(props.batch, props.availableGroups ?? []),
    [props.availableGroups, props.batch]
  )
  const initialGroups = useMemo(
    () => selectedPricingGroups(props.batch),
    [props.batch]
  )
  const [selectedGroups, setSelectedGroups] = useState(initialGroups)
  const rows = useMemo(
    () => pricingRows(props.batch, props.currentValues ?? {}),
    [props.batch, props.currentValues]
  )
  useEffect(() => setSelectedGroups(new Set(initialGroups)), [initialGroups])
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

  const continueReview = () => {
    const values = groups.filter((group) => selectedGroups.has(group))
    return props.onContinue?.(values)
  }

  return (
    <section
      className='space-y-4'
      aria-labelledby='config-import-pricing-title'
    >
      <div className='flex items-center justify-between border-b pb-3'>
        <h2
          id='config-import-pricing-title'
          className='text-base font-semibold'
        >
          {t('Pricing review')}
        </h2>
        {props.onContinue && (
          <button
            type='button'
            className='border-input hover:bg-muted rounded-md border px-3 py-2 text-sm'
            disabled={selectedGroups.size === 0}
            onClick={continueReview}
          >
            {t('Continue')}
          </button>
        )}
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
