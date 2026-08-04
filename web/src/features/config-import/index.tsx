/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { getChannels } from '@/features/channels/api'
import { ChannelsProvider } from '@/features/channels/components/channels-provider'
import { ChannelMutateDrawer } from '@/features/channels/components/drawers/channel-mutate-drawer'
import { listRoutingGroups } from '@/features/model-routing/api'

import {
  getConfigImportBatch,
  publishConfigImport,
  saveConfigImportBindings,
  saveConfigImportPricingReview,
  saveConfigImportResolutions,
  saveConfigImportRouteReviews,
  stageConfigImport,
  uploadConfigImport,
  validateConfigImport,
} from './api'
import {
  ChannelBindingStep,
  type ConfigImportChannelCandidate,
} from './components/channel-binding-step'
import { ConfigImportStepper } from './components/config-import-stepper'
import { ConflictResolutionStep } from './components/conflict-resolution-step'
import { ImportSourceStep } from './components/import-source-step'
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
  ConfigImportPricingReviewRequest,
  ConfigImportResolutionsRequest,
  ConfigImportRouteReviewsRequest,
} from './types'

export interface ConfigImportWizardProps {
  batch?: ConfigImportBatchDetail
  restoreBatchID?: number
  onLoadBatch?: (id: number) => Promise<ConfigImportBatchDetail>
  onUpload?: (document: unknown) => Promise<ConfigImportBatchDetail>
  onStage?: (id: number) => Promise<ConfigImportBatchDetail>
  onValidate?: (id: number) => Promise<ConfigImportBatchDetail>
  onPublish?: (id: number) => Promise<ConfigImportBatchDetail>
  onSaveBindings?: (
    id: number,
    request: ConfigImportBindingsRequest
  ) => Promise<ConfigImportBatchDetail>
  onSavePricingReview?: (
    id: number,
    request: ConfigImportPricingReviewRequest
  ) => Promise<ConfigImportBatchDetail>
  onSaveResolutions?: (
    id: number,
    request: ConfigImportResolutionsRequest
  ) => Promise<ConfigImportBatchDetail>
  onSaveRouteReviews?: (
    id: number,
    request: ConfigImportRouteReviewsRequest
  ) => Promise<ConfigImportBatchDetail>
  onLoadChannels?: () => Promise<ConfigImportChannelCandidate[]>
  onLoadPricingGroups?: () => Promise<string[]>
  currentPricingValues?: Record<string, string>
}

export function ConfigImportWizard(props: ConfigImportWizardProps) {
  const { t } = useTranslation()
  const [batch, setBatch] = useState<ConfigImportBatchDetail | undefined>(
    props.batch
  )
  const [error, setError] = useState<string>()
  const [isBusy, setIsBusy] = useState(false)
  const [forcedStep, setForcedStep] = useState<ConfigImportStep>()
  const [reviewStep, setReviewStep] = useState<ConfigImportStep>()
  const [channels, setChannels] = useState<ConfigImportChannelCandidate[]>([])
  const [pricingGroups, setPricingGroups] = useState<string[]>(['default'])
  const [createdChannelIDs, setCreatedChannelIDs] = useState<
    Record<string, number>
  >({})
  const [creatingLineRef, setCreatingLineRef] = useState<string>()
  const onLoadChannels = props.onLoadChannels
  const onLoadPricingGroups = props.onLoadPricingGroups

  const loadChannels = useCallback(async () => {
    if (onLoadChannels) return onLoadChannels()

    const response = await getChannels({ p: 1, page_size: 1000 })
    if (!response.success) {
      throw new Error(response.message || 'Unable to load channels')
    }
    return (response.data?.items ?? []).map(({ id, name, status }) => ({
      id,
      name,
      status,
    }))
  }, [onLoadChannels])

  const loadPricingGroups = useCallback(async () => {
    if (onLoadPricingGroups) return onLoadPricingGroups()
    const response = await listRoutingGroups()
    return response.data
  }, [onLoadPricingGroups])

  useEffect(() => {
    if (!props.restoreBatchID) return
    setIsBusy(true)
    void (props.onLoadBatch ?? getConfigImportBatch)(props.restoreBatchID)
      .then(setBatch)
      .catch(() => setError(t('The import batch could not be restored.')))
      .finally(() => setIsBusy(false))
  }, [props, t])

  useEffect(() => {
    if (!batch) {
      setChannels([])
      return
    }

    let cancelled = false
    void loadChannels()
      .then((loaded) => {
        if (!cancelled) setChannels(loaded)
      })
      .catch(() => {
        if (!cancelled) setError(t('The import action failed.'))
      })

    return () => {
      cancelled = true
    }
  }, [batch, loadChannels, t])

  useEffect(() => {
    if (!batch) {
      setPricingGroups(['default'])
      return
    }

    let cancelled = false
    void loadPricingGroups()
      .then((loaded) => {
        if (!cancelled) setPricingGroups(loaded)
      })
      .catch(() => {
        if (!cancelled) setError(t('The import action failed.'))
      })

    return () => {
      cancelled = true
    }
  }, [batch, loadPricingGroups, t])

  useEffect(() => {
    setCreatedChannelIDs({})
    setCreatingLineRef(undefined)
  }, [batch?.id])

  const runMutation = async (
    mutation: (id: number) => Promise<ConfigImportBatchDetail>
  ): Promise<ConfigImportBatchDetail | undefined> => {
    if (!batch) return undefined
    setIsBusy(true)
    setError(undefined)
    try {
      setForcedStep(undefined)
      const updated = await mutation(batch.id)
      setBatch(updated)
      return updated
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
    return undefined
  }

  if (!batch) {
    return (
      <ImportSourceStep
        disabled={isBusy}
        onUpload={props.onUpload ?? uploadConfigImport}
        onUploaded={setBatch}
      />
    )
  }

  const state = deriveWizardState(batch)
  const step = forcedStep ?? reviewStep ?? state.step

  return (
    <section
      className='min-h-0 flex-1 space-y-5 overflow-auto p-6'
      aria-labelledby='config-import-wizard-title'
    >
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
        <>
          <ChannelBindingStep
            batch={batch}
            channels={channels}
            createdChannelIDs={createdChannelIDs}
            onCreateChannel={(lineRef) => setCreatingLineRef(lineRef)}
            onSave={async (request) => {
              const updated = await runMutation((id) =>
                (
                  props.onSaveBindings ??
                  ((batchID, payload) =>
                    saveConfigImportBindings(batchID, payload))
                )(id, request)
              )
              if (updated) {
                const staged = await runMutation(
                  props.onStage ?? stageConfigImport
                )
                if (staged) {
                  setReviewStep(
                    staged.items.some((item) => item.state === 'conflict')
                      ? 'conflict_resolution'
                      : 'pricing'
                  )
                }
              }
            }}
          />
          {creatingLineRef && (
            <ChannelsProvider>
              <ChannelMutateDrawer
                open
                initialDisabled
                onOpenChange={(open) => {
                  if (!open) setCreatingLineRef(undefined)
                }}
                onCreated={(channelIDs) => {
                  const channelID = channelIDs[0]
                  const lineRef = creatingLineRef
                  if (!channelID || !lineRef) return
                  setCreatedChannelIDs((current) => ({
                    ...current,
                    [lineRef]: channelID,
                  }))
                  setCreatingLineRef(undefined)
                  void runMutation((id) =>
                    (
                      props.onSaveBindings ??
                      ((batchID, payload) =>
                        saveConfigImportBindings(batchID, payload))
                    )(id, {
                      bindings: [
                        {
                          line_ref: lineRef,
                          action: 'create',
                          channel_id: channelID,
                          credentials_confirmed: false,
                        },
                      ],
                    })
                  )
                  void loadChannels()
                    .then(setChannels)
                    .catch(() => setError(t('The import action failed.')))
                }}
              />
            </ChannelsProvider>
          )}
        </>
      )}
      {step === 'conflict_resolution' && (
        <ConflictResolutionStep
          batch={batch}
          onSave={async (request) => {
            const updated = await runMutation((id) =>
              (
                props.onSaveResolutions ??
                ((batchID, payload) =>
                  saveConfigImportResolutions(batchID, payload))
              )(id, request)
            )
            if (updated) {
              const staged = await runMutation(
                props.onStage ?? stageConfigImport
              )
              if (staged) setReviewStep('pricing')
            }
          }}
          onContinue={() => setReviewStep('pricing')}
        />
      )}
      {step === 'pricing' && (
        <PricingStep
          batch={batch}
          currentValues={props.currentPricingValues}
          availableGroups={pricingGroups}
          onContinue={async (selectedGroups) => {
            const updated = await runMutation((id) =>
              (
                props.onSavePricingReview ??
                ((batchID, payload) =>
                  saveConfigImportPricingReview(batchID, payload))
              )(id, { selected_groups: selectedGroups })
            )
            if (updated) {
              const staged = await runMutation(
                props.onStage ?? stageConfigImport
              )
              if (staged) setReviewStep('routing_diff')
            }
          }}
        />
      )}
      {step === 'routing_diff' && (
        <RoutingDiffStep
          batch={batch}
          onReview={async (reviews) => {
            const updated = await runMutation((id) =>
              (
                props.onSaveRouteReviews ??
                ((batchID, payload) =>
                  saveConfigImportRouteReviews(batchID, payload))
              )(id, {
                reviews: reviews.map(({ business_id, merge_mode }) => ({
                  item_business_id: business_id,
                  merge_mode,
                })),
              })
            )
            if (updated) {
              const validated = await runMutation(
                props.onValidate ?? validateConfigImport
              )
              if (validated) setReviewStep('publish_review')
            }
          }}
        />
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
            const published = await runMutation(mutation)
            if (published) setReviewStep(undefined)
          }}
        />
      )}
      {step === 'publish_result' && (
        <PublishResultStep
          batch={batch}
          onValidate={
            batch.status === 'publish_failed'
              ? async () => {
                  await runMutation(props.onValidate ?? validateConfigImport)
                }
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
