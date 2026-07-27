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
import { Input } from '@/components/ui/input'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'

import type {
  ConfigImportBatchDetail,
  ConfigImportResolution,
  ConfigImportResolutionsRequest,
} from '../types'
import { IssueList } from './issue-list'

interface ResolutionDraft {
  action?: Extract<
    ConfigImportResolution['action'],
    'split_line' | 'bind_variant' | 'exclude'
  >
  lineRef: string
  costVariantKey: string
  routeTargetRef: string
  reason: string
}

interface RouteVariantOption {
  costVariantKey: string
  routeTargetRef: string
  lineRef: string
}

export interface ConflictResolutionStepProps {
  batch: ConfigImportBatchDetail
  isSaving?: boolean
  onSave: (request: ConfigImportResolutionsRequest) => Promise<void>
  onContinue?: () => void
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

function lineReferences(batch: ConfigImportBatchDetail): string[] {
  if (!batch.bindings?.length) {
    return batch.items
      .filter((item) => item.entity_type === 'channel_lines')
      .map((item) => parseObject(item.canonical_json).line_ref)
      .filter((lineRef): lineRef is string => typeof lineRef === 'string')
      .sort()
  }
  const boundLines = new Set(
    (batch.bindings ?? [])
      .filter(
        (binding) =>
          binding.action !== 'skip' &&
          binding.channel_id !== null &&
          binding.channel_id !== undefined &&
          binding.credentials_confirmed
      )
      .map((binding) => binding.line_ref)
  )
  return batch.items
    .filter((item) => item.entity_type === 'channel_lines')
    .map((item) => parseObject(item.canonical_json).line_ref)
    .filter(
      (lineRef): lineRef is string =>
        typeof lineRef === 'string' && boundLines.has(lineRef)
    )
    .sort()
}

function routeVariantOptions(
  batch: ConfigImportBatchDetail
): RouteVariantOption[] {
  return batch.items
    .filter((item) => item.entity_type === 'route_blueprints')
    .flatMap((item) => {
      const targets = parseObject(item.canonical_json).targets
      if (!Array.isArray(targets)) return []
      return targets.flatMap((target) => {
        if (target === null || typeof target !== 'object') return []
        const values = target as Record<string, unknown>
        if (
          typeof values.cost_variant_key !== 'string' ||
          typeof values.route_target_ref !== 'string'
        ) {
          return []
        }
        return [
          {
            costVariantKey: values.cost_variant_key,
            routeTargetRef: values.route_target_ref,
            lineRef: typeof values.line_ref === 'string' ? values.line_ref : '',
          },
        ]
      })
    })
    .sort((left, right) =>
      `${left.costVariantKey}:${left.routeTargetRef}`.localeCompare(
        `${right.costVariantKey}:${right.routeTargetRef}`
      )
    )
}

export function ConflictResolutionStep(props: ConflictResolutionStepProps) {
  const { t } = useTranslation()
  const conflicts = useMemo(
    () => props.batch.items.filter((item) => item.state === 'conflict'),
    [props.batch.items]
  )
  const lines = useMemo(() => lineReferences(props.batch), [props.batch])
  const routeOptions = useMemo(
    () => routeVariantOptions(props.batch),
    [props.batch]
  )
  const [drafts, setDrafts] = useState<Record<string, ResolutionDraft>>({})
  const [error, setError] = useState<string>()

  const updateDraft = (
    businessID: string,
    update: Partial<ResolutionDraft>
  ) => {
    setDrafts((current) => ({
      ...current,
      [businessID]: {
        ...(current[businessID] ?? {
          action: undefined,
          lineRef: '',
          costVariantKey: '',
          routeTargetRef: '',
          reason: '',
        }),
        ...update,
      },
    }))
  }

  const save = async () => {
    const resolutions: ConfigImportResolution[] = []
    for (const item of conflicts) {
      const draft = drafts[item.business_id]
      if (!draft?.action) {
        setError(t('Select a resolution action for every conflict.'))
        return
      }
      if (draft.action === 'split_line') {
        if (!draft.lineRef) {
          setError(t('Select a line reference for every split.'))
          return
        }
        resolutions.push({
          item_business_id: item.business_id,
          action: 'split_line',
          line_ref: draft.lineRef,
        })
        continue
      }
      if (draft.action === 'bind_variant') {
        if (!draft.costVariantKey || !draft.routeTargetRef) {
          setError(
            t('Select a cost variant and route target for every binding.')
          )
          return
        }
        resolutions.push({
          item_business_id: item.business_id,
          action: 'bind_variant',
          cost_variant_key: draft.costVariantKey,
          route_target_ref: draft.routeTargetRef,
        })
        continue
      }
      if (!draft.reason.trim()) {
        setError(t('An exclusion reason is required.'))
        return
      }
      resolutions.push({
        item_business_id: item.business_id,
        action: 'exclude',
        reason: draft.reason.trim(),
      })
    }
    setError(undefined)
    try {
      await props.onSave({ resolutions })
    } catch {
      setError(t('Resolution changes could not be saved.'))
    }
  }

  return (
    <section
      className='space-y-4'
      aria-labelledby='config-import-conflicts-title'
    >
      <div className='flex flex-wrap items-center justify-between gap-3 border-b pb-3'>
        <div>
          <h2
            id='config-import-conflicts-title'
            className='text-base font-semibold'
          >
            {t('Conflict resolution')}
          </h2>
          <p className='text-muted-foreground text-sm'>
            {t('Resolve every warning before publishing.')}
          </p>
        </div>
        <Button
          onClick={save}
          disabled={props.isSaving || conflicts.length === 0}
        >
          {t('Save resolutions')}
        </Button>
        {props.onContinue && conflicts.length === 0 && (
          <Button variant='outline' onClick={props.onContinue}>
            {t('Continue')}
          </Button>
        )}
      </div>

      {error && (
        <p className='text-destructive text-sm' role='alert'>
          {error}
        </p>
      )}

      <IssueList
        issues={props.batch.issues.filter(
          (issue) =>
            issue.severity === 'warning' && issue.resolution_status === 'open'
        )}
      />

      <div className='space-y-3'>
        {conflicts.map((item) => {
          const draft = drafts[item.business_id] ?? {
            lineRef: '',
            costVariantKey: '',
            routeTargetRef: '',
            reason: '',
          }
          const evidence = parseObject(item.canonical_json)
          const itemLineRef =
            typeof evidence.line_ref === 'string' ? evidence.line_ref : ''
          const matchingRoutes = routeOptions.filter(
            (route) =>
              (!itemLineRef ||
                !route.lineRef ||
                route.lineRef === itemLineRef) &&
              (!draft.costVariantKey ||
                route.costVariantKey === draft.costVariantKey)
          )
          return (
            <article
              key={item.id}
              className='grid gap-3 border p-3 lg:grid-cols-[minmax(16rem,1fr)_minmax(20rem,1fr)]'
            >
              <div className='min-w-0 space-y-1'>
                <p className='font-medium'>{item.business_id}</p>
                <p className='text-muted-foreground text-xs'>
                  {item.source_sheet}
                  {item.source_row ? `:${item.source_row}` : ''}
                </p>
                {typeof evidence.reason === 'string' && (
                  <p className='text-muted-foreground text-sm'>
                    {evidence.reason}
                  </p>
                )}
                {item.conflict_reason && (
                  <p className='text-muted-foreground text-sm'>
                    {item.conflict_reason}
                  </p>
                )}
              </div>

              <div className='grid gap-2'>
                <ToggleGroup
                  value={draft.action ? [draft.action] : []}
                  onValueChange={(selection) =>
                    updateDraft(item.business_id, {
                      action: selection[0] as ResolutionDraft['action'],
                      lineRef: '',
                      costVariantKey: '',
                      routeTargetRef: '',
                      reason: '',
                    })
                  }
                  aria-label={t('Select resolution action')}
                >
                  <ToggleGroupItem value='split_line' size='sm'>
                    {t('Split line')}
                  </ToggleGroupItem>
                  <ToggleGroupItem value='bind_variant' size='sm'>
                    {t('Bind variant')}
                  </ToggleGroupItem>
                  <ToggleGroupItem value='exclude' size='sm'>
                    {t('Exclude')}
                  </ToggleGroupItem>
                </ToggleGroup>

                {draft.action === 'split_line' && (
                  <select
                    className='border-input bg-background h-9 w-full rounded-md border px-2 text-sm'
                    aria-label={t('Line reference')}
                    value={draft.lineRef}
                    onChange={(event) =>
                      updateDraft(item.business_id, {
                        lineRef: event.target.value,
                      })
                    }
                  >
                    <option value=''>{t('Select line reference')}</option>
                    {lines.map((line) => (
                      <option key={line} value={line}>
                        {line}
                      </option>
                    ))}
                  </select>
                )}

                {draft.action === 'bind_variant' && (
                  <>
                    <select
                      className='border-input bg-background h-9 w-full rounded-md border px-2 text-sm'
                      aria-label={t('Cost variant key')}
                      value={draft.costVariantKey}
                      onChange={(event) =>
                        updateDraft(item.business_id, {
                          costVariantKey: event.target.value,
                          routeTargetRef: '',
                        })
                      }
                    >
                      <option value=''>{t('Select cost variant key')}</option>
                      {[
                        ...new Set(
                          routeOptions
                            .filter(
                              (route) =>
                                !itemLineRef ||
                                !route.lineRef ||
                                route.lineRef === itemLineRef
                            )
                            .map((route) => route.costVariantKey)
                        ),
                      ].map((key) => (
                        <option key={key} value={key}>
                          {key}
                        </option>
                      ))}
                    </select>
                    <select
                      className='border-input bg-background h-9 w-full rounded-md border px-2 text-sm'
                      aria-label={t('Route target reference')}
                      value={draft.routeTargetRef}
                      onChange={(event) =>
                        updateDraft(item.business_id, {
                          routeTargetRef: event.target.value,
                        })
                      }
                    >
                      <option value=''>
                        {t('Select route target reference')}
                      </option>
                      {matchingRoutes.map((route) => (
                        <option
                          key={route.routeTargetRef}
                          value={route.routeTargetRef}
                        >
                          {route.routeTargetRef}
                        </option>
                      ))}
                    </select>
                  </>
                )}

                {draft.action === 'exclude' && (
                  <Input
                    aria-label={t('Exclusion reason')}
                    value={draft.reason}
                    onChange={(event) =>
                      updateDraft(item.business_id, {
                        reason: event.target.value,
                      })
                    }
                    onInput={(event) =>
                      updateDraft(item.business_id, {
                        reason: event.currentTarget.value,
                      })
                    }
                  />
                )}
              </div>
            </article>
          )
        })}
      </div>
    </section>
  )
}
