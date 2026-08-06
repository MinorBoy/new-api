/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'

import type { ConfigImportBatchDetail } from '../types'

export interface ActivationStepProps {
  batch: ConfigImportBatchDetail
  canActivate: boolean
  isActivating?: boolean
  onActivate: () => Promise<void>
}

export function ActivationStep(props: ActivationStepProps) {
  const { t } = useTranslation()
  const [confirmed, setConfirmed] = useState(false)
  const [isSubmitting, setIsSubmitting] = useState(false)
  const [error, setError] = useState<string>()
  const preview = props.batch.activation_preview
  const busy = props.isActivating || isSubmitting
  const disabled =
    !props.canActivate ||
    !preview?.ready ||
    preview.blockers.length > 0 ||
    !confirmed ||
    busy
  const counts = [
    [t('Channels to enable'), preview?.channel_count ?? 0],
    [t('Policies to enable'), preview?.policy_count ?? 0],
    [t('Targets to enable'), preview?.target_count ?? 0],
    [t('Targets to retire'), preview?.retire_target_count ?? 0],
  ] as const

  const activate = async () => {
    setError(undefined)
    setIsSubmitting(true)
    try {
      await props.onActivate()
    } catch (caught) {
      setError(
        caught instanceof Error ? caught.message : t('Activation failed.')
      )
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <section
      className='flex flex-col gap-5'
      aria-labelledby='config-import-activation-title'
    >
      <h2
        id='config-import-activation-title'
        className='text-base font-semibold'
      >
        {t('Activation review')}
      </h2>

      <dl className='grid grid-cols-2 gap-3 sm:grid-cols-4'>
        {counts.map(([label, value]) => (
          <div
            key={label}
            className='flex min-h-20 min-w-0 flex-col justify-between border px-3 py-3'
            data-activation-count
          >
            <dt className='text-muted-foreground text-xs'>{label}</dt>
            <dd className='text-xl font-semibold tabular-nums'>{value}</dd>
          </div>
        ))}
      </dl>

      {(!preview?.ready || preview.blockers.length > 0) && (
        <section className='flex flex-col gap-3 border px-3 py-3'>
          <div className='flex flex-col gap-1'>
            <h3 className='text-sm font-semibold'>
              {t('Activation blockers')}
            </h3>
            <p className='text-muted-foreground text-sm'>
              {t('This batch is published but cannot be activated.')}
            </p>
          </div>
          {preview && preview.blockers.length > 0 && (
            <ul className='flex flex-col gap-2'>
              {preview.blockers.map((blocker) => (
                <li
                  key={`${blocker.code}:${blocker.channel_id ?? ''}:${blocker.line_ref ?? ''}:${blocker.route_target_ref ?? ''}`}
                  className='min-w-0 border px-3 py-3 text-sm'
                >
                  <p className='font-mono text-xs font-semibold break-all'>
                    {blocker.code}
                  </p>
                  <p className='mt-1 break-words'>{blocker.message}</p>
                  {(blocker.channel_id ||
                    blocker.line_ref ||
                    blocker.route_target_ref) && (
                    <dl className='text-muted-foreground mt-2 grid min-w-0 gap-1 text-xs sm:grid-cols-2'>
                      {blocker.channel_id && (
                        <div className='flex min-w-0 gap-1'>
                          <dt>{t('Channel ID')}:</dt>
                          <dd className='font-mono'>{blocker.channel_id}</dd>
                        </div>
                      )}
                      {blocker.line_ref && (
                        <div className='flex min-w-0 gap-1'>
                          <dt>{t('Line reference')}:</dt>
                          <dd className='min-w-0 font-mono break-all'>
                            {blocker.line_ref}
                          </dd>
                        </div>
                      )}
                      {blocker.route_target_ref && (
                        <div className='flex min-w-0 gap-1 sm:col-span-2'>
                          <dt className='shrink-0'>{t('Route target')}:</dt>
                          <dd
                            className='min-w-0 font-mono break-all'
                            data-route-target-ref
                          >
                            {blocker.route_target_ref}
                          </dd>
                        </div>
                      )}
                    </dl>
                  )}
                </li>
              ))}
            </ul>
          )}
        </section>
      )}

      {error && (
        <Alert variant='destructive'>
          <AlertTitle>{t('Activation failed.')}</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}

      <label className='flex items-start gap-2 border px-3 py-3 text-sm'>
        <input
          type='checkbox'
          className='mt-0.5 size-4 shrink-0'
          aria-label={t('Confirm activation')}
          checked={confirmed}
          disabled={busy || !preview?.ready || preview.blockers.length > 0}
          onChange={(event) => setConfirmed(event.currentTarget.checked)}
        />
        <span>
          {t('I confirm this published batch is ready to become active.')}
        </span>
      </label>

      <div>
        <Button
          type='button'
          disabled={disabled}
          aria-busy={busy}
          onClick={() => void activate()}
        >
          {busy && <Spinner data-icon='inline-start' />}
          {t('Activate import')}
        </Button>
      </div>
    </section>
  )
}
