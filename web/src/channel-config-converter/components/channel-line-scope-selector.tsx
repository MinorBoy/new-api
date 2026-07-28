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
import { type KeyboardEvent, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'

import type { ChannelLineGroup, ScopedImportDocumentResult } from '../scope'

export interface ChannelLineScopeSelectorProps {
  disabled?: boolean
  groups: ChannelLineGroup[]
  onSelectionChange: (lineRefs: Set<string>) => void
  selectedLineRefs: ReadonlySet<string>
  summary: Pick<
    ScopedImportDocumentResult,
    | 'blockingIssues'
    | 'document'
    | 'selectedGroupCount'
    | 'selectedLineCount'
    | 'validationErrors'
    | 'warnings'
  >
}

function field(entity: Record<string, unknown>, name: string): string {
  const value = entity[name]
  return typeof value === 'string' ? value : ''
}

function toggleOnSpace(event: KeyboardEvent, onToggle: () => void): void {
  if (event.key !== ' ') return
  event.preventDefault()
  onToggle()
}

export function ChannelLineScopeSelector(props: ChannelLineScopeSelectorProps) {
  const { t } = useTranslation()
  const [query, setQuery] = useState('')
  const normalizedQuery = query.trim().toLocaleLowerCase()
  const visibleGroups = useMemo(() => {
    if (normalizedQuery === '') return props.groups
    return props.groups.flatMap((group) => {
      const channelValues = [
        group.channel.business_id,
        field(group.channel, 'display_name'),
      ].join(' ')
      const groupMatches = channelValues
        .toLocaleLowerCase()
        .includes(normalizedQuery)
      const lines = group.lines.filter((line) => {
        if (groupMatches) return true
        return [
          line.business_id,
          field(line, 'display_name'),
          field(line, 'line_ref'),
        ]
          .join(' ')
          .toLocaleLowerCase()
          .includes(normalizedQuery)
      })
      return lines.length === 0 ? [] : [{ ...group, lines }]
    })
  }, [normalizedQuery, props.groups])

  const allLineRefs = useMemo(
    () =>
      props.groups.flatMap((group) =>
        group.lines.map((line) => field(line, 'line_ref'))
      ),
    [props.groups]
  )
  const routeTargetCount =
    props.summary.document.entities.route_blueprints.reduce(
      (count, route) =>
        count + (Array.isArray(route.targets) ? route.targets.length : 0),
      0
    )
  const summaryItems = [
    [t('Selected groups'), props.summary.selectedGroupCount],
    [t('Selected lines'), props.summary.selectedLineCount],
    [
      t('Channel costs'),
      props.summary.document.manifest.counts.cost_rule_drafts,
    ],
    [
      t('Model mappings'),
      props.summary.document.manifest.counts.model_mappings,
    ],
    [t('Route targets'), routeTargetCount],
    [t('Model SKUs'), props.summary.document.manifest.counts.model_skus],
    [t('Sale pricing'), props.summary.document.manifest.counts.sale_proposals],
  ] as const

  const changeGroup = (group: ChannelLineGroup, checked: boolean) => {
    const next = new Set(props.selectedLineRefs)
    for (const line of group.lines) {
      const lineRef = field(line, 'line_ref')
      if (checked) next.add(lineRef)
      else next.delete(lineRef)
    }
    props.onSelectionChange(next)
  }

  const changeLine = (lineRef: string, checked: boolean) => {
    const next = new Set(props.selectedLineRefs)
    if (checked) next.add(lineRef)
    else next.delete(lineRef)
    props.onSelectionChange(next)
  }

  return (
    <section
      className='flex flex-col gap-4'
      aria-labelledby='channel-line-scope-title'
    >
      <div className='flex flex-wrap items-center justify-between gap-3 border-b pb-3'>
        <div>
          <h2 id='channel-line-scope-title' className='text-base font-semibold'>
            {t('Select channel lines')}
          </h2>
          <p className='text-muted-foreground text-sm'>
            {t(
              'Conversion runs in this browser. The workbook is not uploaded.'
            )}
          </p>
        </div>
        <div className='flex flex-wrap gap-2'>
          <Button
            disabled={props.disabled || allLineRefs.length === 0}
            onClick={() => props.onSelectionChange(new Set(allLineRefs))}
            type='button'
            variant='outline'
          >
            {t('Select all channel lines')}
          </Button>
          <Button
            disabled={props.disabled || props.selectedLineRefs.size === 0}
            onClick={() => props.onSelectionChange(new Set())}
            type='button'
            variant='outline'
          >
            {t('Clear channel line selection')}
          </Button>
        </div>
      </div>

      <div className='grid gap-5 lg:grid-cols-[minmax(0,1fr)_18rem]'>
        <div className='flex min-w-0 flex-col gap-3'>
          <Input
            aria-label={t('Search channel groups and lines')}
            disabled={props.disabled}
            onInput={(event) => setQuery(event.currentTarget.value)}
            placeholder={t('Search channel groups and lines')}
            value={query}
          />
          <div className='divide-y rounded-lg border'>
            {visibleGroups.map((group) => {
              const selectedCount = group.lines.filter((line) =>
                props.selectedLineRefs.has(field(line, 'line_ref'))
              ).length
              const groupChecked = selectedCount === group.lines.length
              const groupIndeterminate =
                selectedCount > 0 && selectedCount < group.lines.length
              const groupName = field(group.channel, 'display_name')
              return (
                <div
                  className='flex flex-col gap-2 p-3'
                  key={group.channel.business_id}
                >
                  <div className='flex items-center gap-2 font-medium'>
                    <Checkbox
                      aria-label={t('Select all lines in {{group}}', {
                        group: groupName,
                      })}
                      checked={groupChecked}
                      disabled={props.disabled}
                      indeterminate={groupIndeterminate}
                      onCheckedChange={(checked) =>
                        changeGroup(group, checked === true)
                      }
                      onKeyDown={(event) =>
                        toggleOnSpace(event, () =>
                          changeGroup(group, !groupChecked)
                        )
                      }
                    />
                    <span className='truncate'>{groupName}</span>
                    <span className='text-muted-foreground font-mono text-xs'>
                      {selectedCount}/{group.lines.length}
                    </span>
                  </div>
                  <div className='grid gap-2 pl-6'>
                    {group.lines.map((line) => {
                      const lineRef = field(line, 'line_ref')
                      const lineName = field(line, 'display_name')
                      return (
                        <div
                          className='flex min-w-0 items-center gap-2 text-sm'
                          key={line.business_id}
                        >
                          <Checkbox
                            aria-label={lineName}
                            checked={props.selectedLineRefs.has(lineRef)}
                            disabled={props.disabled}
                            onCheckedChange={(checked) =>
                              changeLine(lineRef, checked === true)
                            }
                            onKeyDown={(event) =>
                              toggleOnSpace(event, () =>
                                changeLine(
                                  lineRef,
                                  !props.selectedLineRefs.has(lineRef)
                                )
                              )
                            }
                          />
                          <span className='truncate'>{lineName}</span>
                          <span className='text-muted-foreground truncate font-mono text-xs'>
                            {lineRef}
                          </span>
                        </div>
                      )
                    })}
                  </div>
                </div>
              )
            })}
            {visibleGroups.length === 0 && (
              <p className='text-muted-foreground p-3 text-sm'>
                {t('No channel lines are selected.')}
              </p>
            )}
          </div>
        </div>

        <aside className='border-l pl-5 max-lg:border-t max-lg:border-l-0 max-lg:pt-4 max-lg:pl-0'>
          <h3 className='text-sm font-semibold'>{t('Selected scope')}</h3>
          <dl className='mt-3 grid grid-cols-2 gap-x-4 gap-y-2 text-sm'>
            {summaryItems.map(([label, value]) => (
              <div className='contents' key={label}>
                <dt className='text-muted-foreground'>{label}</dt>
                <dd className='text-right font-mono'>{value}</dd>
              </div>
            ))}
          </dl>
          {props.summary.warnings.length > 0 && (
            <p className='mt-4 text-sm'>
              {t('Scoped warnings')}: {props.summary.warnings.length}
            </p>
          )}
          {(props.summary.blockingIssues.length > 0 ||
            props.summary.validationErrors.length > 0) && (
            <p className='text-destructive mt-2 text-sm' role='alert'>
              {t('Scoped errors')}:{' '}
              {props.summary.blockingIssues.length +
                props.summary.validationErrors.length}
            </p>
          )}
        </aside>
      </div>
    </section>
  )
}
