/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'

import {
  getConfigImportBatch,
  publishConfigImport,
  saveConfigImportBindings,
  saveConfigImportResolutions,
  stageConfigImport,
  validateConfigImport,
} from './api'
import { ChannelBindingStep } from './components/channel-binding-step'
import { ConfigImportStepper } from './components/config-import-stepper'
import { ConflictResolutionStep } from './components/conflict-resolution-step'
import { ImportUploadStep } from './components/import-upload-step'
import { PricingStep } from './components/pricing-step'
import { PublishResultStep } from './components/publish-result-step'
import { PublishReviewStep } from './components/publish-review-step'
import { RoutingDiffStep } from './components/routing-diff-step'
import {
  CONFIG_IMPORT_STEPS,
  deriveWizardState,
  type ConfigImportStep,
} from './lib/batch-state'
import type {
  ConfigImportBatchDetail,
  ConfigImportBindingsRequest,
  ConfigImportResolutionsRequest,
} from './types'

export interface ConfigImportWizardProps {
  batch?: ConfigImportBatchDetail
  restoreBatchID?: number
  onLoadBatch?: (id: number) => Promise<ConfigImportBatchDetail>
  onStage?: (id: number) => Promise<ConfigImportBatchDetail>
  onValidate?: (id: number) => Promise<ConfigImportBatchDetail>
  onPublish?: (id: number) => Promise<ConfigImportBatchDetail>
  onSaveBindings?: (
    id: number,
    request: ConfigImportBindingsRequest
  ) => Promise<ConfigImportBatchDetail>
  onSaveResolutions?: (
    id: number,
    request: ConfigImportResolutionsRequest
  ) => Promise<ConfigImportBatchDetail>
}

export function ConfigImportWizard(props: ConfigImportWizardProps) {
  const { t } = useTranslation()
  const [batch, setBatch] = useState<ConfigImportBatchDetail | undefined>(
    props.batch
  )
  const [error, setError] = useState<string>()
  const [isBusy, setIsBusy] = useState(false)
  const [forcedStep, setForcedStep] = useState<ConfigImportStep>()

  useEffect(() => {
    if (!props.restoreBatchID) return
    setIsBusy(true)
    void (props.onLoadBatch ?? getConfigImportBatch)(props.restoreBatchID)
      .then(setBatch)
      .catch(() => setError(t('The import batch could not be restored.')))
      .finally(() => setIsBusy(false))
  }, [props, t])

  const runMutation = async (
    mutation: (id: number) => Promise<ConfigImportBatchDetail>
  ) => {
    if (!batch) return
    setIsBusy(true)
    setError(undefined)
    try {
      setForcedStep(undefined)
      setBatch(await mutation(batch.id))
    } catch (caught) {
      const code =
        caught !== null && typeof caught === 'object' && 'code' in caught
          ? String(caught.code)
          : ''
      if (code === 'STALE_BASE_VERSION') {
        setForcedStep('routing_diff')
      }
      setError(
        caught instanceof Error
          ? caught.message
          : t('The import action failed.')
      )
    } finally {
      setIsBusy(false)
    }
  }

  if (!batch) {
    return <ImportUploadStep disabled={isBusy} onUploaded={setBatch} />
  }

  const state = deriveWizardState(batch)
  const step = forcedStep ?? state.step

  return (
    <section className='space-y-5' aria-labelledby='config-import-wizard-title'>
      <div className='flex flex-wrap items-center justify-between gap-3'>
        <h1 id='config-import-wizard-title' className='text-xl font-semibold'>
          {t('Channel configuration import')}
        </h1>
        <span className='text-muted-foreground font-mono text-xs'>
          #{batch.id}
        </span>
      </div>
      <ConfigImportStepper steps={CONFIG_IMPORT_STEPS} activeStep={step} />
      {error && (
        <p className='text-destructive text-sm' role='alert'>
          {error}
        </p>
      )}

      {step === 'upload' && (
        <ImportUploadStep disabled={isBusy} onUploaded={setBatch} />
      )}
      {step === 'channel_binding' && (
        <ChannelBindingStep
          batch={batch}
          channels={[]}
          onCreateChannel={() => undefined}
          onSave={async (request) => {
            await runMutation((id) =>
              (
                props.onSaveBindings ??
                ((batchID, payload) =>
                  saveConfigImportBindings(batchID, payload))
              )(id, request)
            )
          }}
        />
      )}
      {step === 'conflict_resolution' && (
        <ConflictResolutionStep
          batch={batch}
          onSave={async (request) => {
            await runMutation((id) =>
              (
                props.onSaveResolutions ??
                ((batchID, payload) =>
                  saveConfigImportResolutions(batchID, payload))
              )(id, request)
            )
          }}
        />
      )}
      {step === 'pricing' && <PricingStep batch={batch} />}
      {step === 'routing_diff' && (
        <RoutingDiffStep batch={batch} onReview={() => undefined} />
      )}
      {step === 'publish_review' && (
        <PublishReviewStep
          batch={batch}
          canPublish={state.canPublish}
          isPublishing={isBusy}
          onPublish={async () => {
            const mutation =
              props.onPublish ??
              (async (id: number) => {
                await publishConfigImport(id)
                return getConfigImportBatch(id)
              })
            await runMutation(mutation)
          }}
        />
      )}
      {step === 'publish_result' && (
        <PublishResultStep
          batch={batch}
          onValidate={
            batch.status === 'publish_failed'
              ? async () =>
                  runMutation(props.onValidate ?? validateConfigImport)
              : undefined
          }
        />
      )}

      {batch.status === 'binding' &&
        batch.allowed_actions.includes('stage') && (
          <Button
            onClick={() => void runMutation(props.onStage ?? stageConfigImport)}
            disabled={isBusy}
          >
            {t('Stage import')}
          </Button>
        )}
      {batch.status === 'staged' &&
        batch.allowed_actions.includes('validate') && (
          <Button
            onClick={() =>
              void runMutation(props.onValidate ?? validateConfigImport)
            }
            disabled={isBusy}
          >
            {t('Validate import')}
          </Button>
        )}
    </section>
  )
}
