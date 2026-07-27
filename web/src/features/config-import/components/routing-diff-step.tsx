/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'

import type {
  ConfigImportBatchDetail,
  ConfigImportRouteMergeMode,
} from '../types'

interface RouteTarget {
  routeTargetRef: string
  lineRef: string
  upstreamModel: string
  costVariantKey: string
  constraints: string[]
}

interface RouteBlueprint {
  businessID: string
  source: string
  mergeMode: ConfigImportRouteMergeMode
  targets: RouteTarget[]
}

export interface RoutingDiffStepProps {
  batch: ConfigImportBatchDetail
  existingTargets?: Record<string, string[]>
  isStaleBaseline?: boolean
  onReview: (
    routes: Array<{
      business_id: string
      merge_mode: ConfigImportRouteMergeMode
    }>
  ) => void
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

function routeBlueprints(batch: ConfigImportBatchDetail): RouteBlueprint[] {
  return batch.items
    .filter((item) => item.entity_type === 'route_blueprints')
    .map((item) => {
      const blueprint = parseObject(item.canonical_json)
      const mergeMode = blueprint.merge_mode
      const targets = Array.isArray(blueprint.targets) ? blueprint.targets : []
      return {
        businessID: item.business_id,
        source: `${item.source_sheet}${item.source_row ? `:${item.source_row}` : ''}`,
        mergeMode:
          mergeMode === 'replace' || mergeMode === 'skip' ? mergeMode : 'merge',
        targets: targets.flatMap((target) => {
          if (target === null || typeof target !== 'object') return []
          const value = target as Record<string, unknown>
          const constraints = [
            ...(Array.isArray(value.output_resolutions)
              ? value.output_resolutions
              : []),
            ...(Array.isArray(value.duration_values)
              ? value.duration_values.map((duration) => `${duration} seconds`)
              : []),
          ].filter(
            (constraint): constraint is string => typeof constraint === 'string'
          )
          if (
            typeof value.route_target_ref !== 'string' ||
            typeof value.line_ref !== 'string' ||
            typeof value.upstream_model !== 'string' ||
            typeof value.cost_variant_key !== 'string'
          ) {
            return []
          }
          return [
            {
              routeTargetRef: value.route_target_ref,
              lineRef: value.line_ref,
              upstreamModel: value.upstream_model,
              costVariantKey: value.cost_variant_key,
              constraints,
            },
          ]
        }),
      }
    })
}

export function RoutingDiffStep(props: RoutingDiffStepProps) {
  const { t } = useTranslation()
  const blueprints = useMemo(() => routeBlueprints(props.batch), [props.batch])
  const [modes, setModes] = useState<
    Record<string, ConfigImportRouteMergeMode>
  >(() =>
    Object.fromEntries(
      blueprints.map((blueprint) => [blueprint.businessID, blueprint.mergeMode])
    )
  )
  const [confirmed, setConfirmed] = useState<Record<string, boolean>>({})
  const [error, setError] = useState<string>()

  const continueReview = () => {
    const replacement = blueprints.find(
      (blueprint) =>
        modes[blueprint.businessID] === 'replace' &&
        !confirmed[blueprint.businessID]
    )
    if (replacement) {
      setError(t('Confirm every route replacement before continuing.'))
      return
    }
    setError(undefined)
    props.onReview(
      blueprints.map((blueprint) => ({
        business_id: blueprint.businessID,
        merge_mode: modes[blueprint.businessID] ?? blueprint.mergeMode,
      }))
    )
  }

  return (
    <section
      className='space-y-4'
      aria-labelledby='config-import-routing-title'
    >
      <div className='flex flex-wrap items-center justify-between gap-3 border-b pb-3'>
        <h2
          id='config-import-routing-title'
          className='text-base font-semibold'
        >
          {t('Routing diff')}
        </h2>
        <Button onClick={continueReview}>{t('Continue')}</Button>
      </div>
      {props.isStaleBaseline && (
        <p className='border border-amber-500/50 bg-amber-500/10 px-3 py-2 text-sm'>
          {t(
            'Configuration changed since this import was staged. Review the latest diffs before publishing.'
          )}
        </p>
      )}
      {error && (
        <p className='text-destructive text-sm' role='alert'>
          {error}
        </p>
      )}

      <div className='space-y-3'>
        {blueprints.map((blueprint) => {
          const mode = modes[blueprint.businessID] ?? blueprint.mergeMode
          const deletions = props.existingTargets?.[blueprint.businessID] ?? []
          return (
            <article
              key={blueprint.businessID}
              className='space-y-3 border p-3'
            >
              <div className='flex flex-wrap items-center justify-between gap-3'>
                <div>
                  <p className='font-medium'>{blueprint.businessID}</p>
                  <p className='text-muted-foreground text-xs'>
                    {blueprint.source}
                  </p>
                </div>
                <select
                  className='border-input bg-background h-9 rounded-md border px-2 text-sm'
                  aria-label={t('Route merge mode')}
                  value={mode}
                  onChange={(event) =>
                    setModes((current) => ({
                      ...current,
                      [blueprint.businessID]: event.target
                        .value as ConfigImportRouteMergeMode,
                    }))
                  }
                >
                  <option value='merge'>{t('Merge')}</option>
                  <option value='replace'>{t('Replace')}</option>
                  <option value='skip'>{t('Skip')}</option>
                </select>
              </div>

              <div className='overflow-x-auto'>
                <table className='w-full min-w-[42rem] text-left text-sm'>
                  <thead className='bg-muted/50 text-muted-foreground'>
                    <tr>
                      <th className='px-3 py-2 font-medium'>
                        {t('Route target')}
                      </th>
                      <th className='px-3 py-2 font-medium'>
                        {t('Cost variant key')}
                      </th>
                      <th className='px-3 py-2 font-medium'>
                        {t('Constraints')}
                      </th>
                    </tr>
                  </thead>
                  <tbody>
                    {blueprint.targets.map((target) => (
                      <tr key={target.routeTargetRef} className='border-t'>
                        <td className='px-3 py-2'>{target.routeTargetRef}</td>
                        <td className='px-3 py-2'>{target.costVariantKey}</td>
                        <td className='px-3 py-2'>
                          {target.constraints.join(', ') || '--'}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>

              {mode === 'replace' && (
                <div className='space-y-2 border-t pt-3'>
                  <p className='text-sm font-medium'>
                    {t('Targets to delete')}
                  </p>
                  <ul className='text-muted-foreground list-disc pl-5 text-sm'>
                    {deletions.map((target) => (
                      <li key={target}>{target}</li>
                    ))}
                  </ul>
                  <label className='flex items-center gap-2 text-sm'>
                    <input
                      type='checkbox'
                      aria-label={t('Confirm replacement')}
                      checked={confirmed[blueprint.businessID] ?? false}
                      onChange={(event) =>
                        setConfirmed((current) => ({
                          ...current,
                          [blueprint.businessID]: event.target.checked,
                        }))
                      }
                    />
                    {t(
                      'I understand the listed route targets will be deleted.'
                    )}
                  </label>
                </div>
              )}
            </article>
          )
        })}
      </div>
    </section>
  )
}
