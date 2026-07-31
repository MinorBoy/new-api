/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the License,
or (at your option) any later version.
*/
import { Plus } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'

import type {
  ConfigImportBatchDetail,
  ConfigImportBinding,
  ConfigImportBindingsRequest,
} from '../types'

export interface ConfigImportChannelCandidate {
  id: number
  name: string
  status: number
}

interface ImportChannelLine {
  lineRef: string
  displayName: string
  sourceSheet: string
  sourceRow?: number
}

interface BindingDraft {
  action: ConfigImportBinding['action']
  channelID?: number
  credentialsConfirmed: boolean
}

export interface ChannelBindingStepProps {
  batch: ConfigImportBatchDetail
  channels: ConfigImportChannelCandidate[]
  createdChannelIDs?: Record<string, number>
  isSaving?: boolean
  onCreateChannel: (lineRef: string) => void
  onSave: (request: ConfigImportBindingsRequest) => Promise<void>
}

function channelLines(batch: ConfigImportBatchDetail): ImportChannelLine[] {
  return batch.items
    .filter((item) => item.entity_type === 'channel_lines')
    .map((item) => {
      let lineRef = item.business_id
      let displayName = item.business_id
      try {
        const value = JSON.parse(item.canonical_json) as Record<string, unknown>
        if (typeof value.line_ref === 'string' && value.line_ref.trim()) {
          lineRef = value.line_ref
        }
        if (
          typeof value.display_name === 'string' &&
          value.display_name.trim()
        ) {
          displayName = value.display_name
        }
      } catch {
        // The backend has already validated this canonical JSON. Fall back to the business ID for a damaged legacy row.
      }
      return {
        lineRef,
        displayName,
        sourceSheet: item.source_sheet,
        sourceRow: item.source_row ?? undefined,
      }
    })
    .sort((left, right) => left.lineRef.localeCompare(right.lineRef))
}

export function ChannelBindingStep(props: ChannelBindingStepProps) {
  const { t } = useTranslation()
  const lines = useMemo(() => channelLines(props.batch), [props.batch])
  const [drafts, setDrafts] = useState<Record<string, BindingDraft>>({})
  const [error, setError] = useState<string>()

  useEffect(() => {
    setDrafts((current) => {
      const next = { ...current }
      for (const line of lines) {
        const createdChannelID = props.createdChannelIDs?.[line.lineRef]
        if (createdChannelID) {
          next[line.lineRef] = {
            action: 'create',
            channelID: createdChannelID,
            credentialsConfirmed: false,
          }
          continue
        }

        if (next[line.lineRef]) continue

        const persisted = props.batch.bindings?.find(
          (binding) => binding.line_ref === line.lineRef
        )
        if (
          !persisted ||
          persisted.action === 'skip' ||
          !persisted.channel_id
        ) {
          continue
        }
        next[line.lineRef] = {
          action: persisted.action as 'bind' | 'create',
          channelID: persisted.channel_id,
          credentialsConfirmed: persisted.credentials_confirmed,
        }
      }
      return next
    })
  }, [lines, props.batch.bindings, props.createdChannelIDs])

  const updateDraft = (lineRef: string, update: Partial<BindingDraft>) => {
    setDrafts((current) => {
      const previous = current[lineRef] ?? {
        action: 'bind' as const,
        credentialsConfirmed: false,
      }
      return { ...current, [lineRef]: { ...previous, ...update } }
    })
  }

  const save = async () => {
    const bindings: ConfigImportBinding[] = []
    for (const line of lines) {
      const draft = drafts[line.lineRef] ?? {
        action: 'bind' as const,
        credentialsConfirmed: false,
      }
      if (draft.action === 'skip') {
        bindings.push({
          line_ref: line.lineRef,
          action: 'skip',
          credentials_confirmed: false,
        })
        continue
      }
      if (!draft.channelID) {
        setError(t('A channel selection is required.'))
        return
      }
      if (!draft.credentialsConfirmed) {
        setError(t('Confirm channel credentials before saving.'))
        return
      }
      bindings.push({
        line_ref: line.lineRef,
        action: draft.action,
        channel_id: draft.channelID,
        credentials_confirmed: draft.credentialsConfirmed,
      })
    }
    setError(undefined)
    try {
      await props.onSave({ bindings })
    } catch {
      setError(t('Binding changes could not be saved.'))
    }
  }

  return (
    <section
      className='space-y-4'
      aria-labelledby='config-import-bindings-title'
    >
      <div className='flex flex-wrap items-center justify-between gap-3 border-b pb-3'>
        <h2
          id='config-import-bindings-title'
          className='text-base font-semibold'
        >
          {t('Channel bindings')}
        </h2>
        <Button onClick={save} disabled={props.isSaving || lines.length === 0}>
          {t('Save bindings')}
        </Button>
      </div>

      {error && (
        <p className='text-destructive text-sm' role='alert'>
          {error}
        </p>
      )}

      <div className='space-y-3'>
        {lines.map((line) => {
          const draft = drafts[line.lineRef] ?? {
            action: 'bind' as const,
            credentialsConfirmed: false,
          }
          return (
            <article
              key={line.lineRef}
              className='grid gap-3 border p-3 lg:grid-cols-[minmax(13rem,1fr)_minmax(14rem,1fr)_auto]'
            >
              <div className='min-w-0'>
                <p className='truncate font-medium'>{line.displayName}</p>
                <p className='text-muted-foreground truncate font-mono text-xs'>
                  {line.lineRef}
                </p>
                <p className='text-muted-foreground text-xs'>
                  {line.sourceSheet}
                  {line.sourceRow ? `:${line.sourceRow}` : ''}
                </p>
              </div>

              <div className='grid gap-2'>
                <div className='flex flex-wrap gap-2'>
                  <Button
                    size='sm'
                    variant={draft.action === 'bind' ? 'default' : 'outline'}
                    onClick={() =>
                      updateDraft(line.lineRef, { action: 'bind' })
                    }
                  >
                    {t('Bind')}
                  </Button>
                  <Button
                    size='sm'
                    variant={draft.action === 'skip' ? 'default' : 'outline'}
                    onClick={() =>
                      updateDraft(line.lineRef, {
                        action: 'skip',
                        channelID: undefined,
                        credentialsConfirmed: false,
                      })
                    }
                  >
                    {t('Skip')}
                  </Button>
                  <Button
                    size='sm'
                    variant='outline'
                    onClick={() => props.onCreateChannel(line.lineRef)}
                  >
                    <Plus className='size-4' aria-hidden='true' />
                    {t('Create channel')}
                  </Button>
                </div>

                {draft.action !== 'skip' && (
                  <>
                    <select
                      className='border-input bg-background h-9 w-full rounded-md border px-2 text-sm'
                      value={draft.channelID?.toString() ?? ''}
                      aria-label={t('Existing channel')}
                      onChange={(event) =>
                        updateDraft(line.lineRef, {
                          channelID:
                            event.target.value === ''
                              ? undefined
                              : Number(event.target.value),
                        })
                      }
                    >
                      <option value=''>{t('Existing channel')}</option>
                      {props.channels.map((channel) => (
                        <option key={channel.id} value={channel.id}>
                          {channel.name} ({channel.id})
                        </option>
                      ))}
                    </select>
                    <label className='flex items-start gap-2 text-sm'>
                      <input
                        type='checkbox'
                        checked={draft.credentialsConfirmed}
                        onChange={(event) =>
                          updateDraft(line.lineRef, {
                            credentialsConfirmed: event.target.checked,
                          })
                        }
                      />
                      <span>
                        {t(
                          'I confirm this channel has been configured with valid credentials.'
                        )}
                      </span>
                    </label>
                  </>
                )}
              </div>
            </article>
          )
        })}
      </div>
    </section>
  )
}
