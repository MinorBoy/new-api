/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.
*/
import type { ConfigImportBatchDetail, ConfigImportBatchStatus } from '../types'

export const CONFIG_IMPORT_STEPS = [
  'upload',
  'channel_binding',
  'conflict_resolution',
  'pricing',
  'routing_diff',
  'publish_review',
  'publish_result',
] as const

export type ConfigImportStep = (typeof CONFIG_IMPORT_STEPS)[number]

export interface ConfigImportWizardState {
  step: ConfigImportStep
  canGoBack: boolean
  canContinue: boolean
  canPublish: boolean
  blockingCodes: string[]
}

function stepForStatus(status: ConfigImportBatchStatus): ConfigImportStep {
  switch (status) {
    case 'validating':
      return 'upload'
    case 'binding':
      return 'channel_binding'
    case 'blocked':
      return 'upload'
    case 'staged':
      return 'conflict_resolution'
    case 'ready':
    case 'publishing':
      return 'publish_review'
    case 'publish_failed':
    case 'published':
      return 'publish_result'
  }
}

function openBlockingCodes(batch: ConfigImportBatchDetail): string[] {
  return [
    ...new Set(
      batch.issues
        .filter(
          (issue) =>
            issue.resolution_status !== 'resolved' &&
            issue.resolution_status !== 'excluded' &&
            (issue.severity === 'warning' || issue.severity === 'error')
        )
        .map((issue) => issue.code)
    ),
  ].sort()
}

export function deriveWizardState(
  batch: ConfigImportBatchDetail
): ConfigImportWizardState {
  const step = stepForStatus(batch.status)
  const blockingCodes = openBlockingCodes(batch)
  const terminal =
    batch.status === 'published' || batch.status === 'publish_failed'
  const canPublish =
    batch.status === 'ready' &&
    batch.allowed_actions.includes('publish') &&
    blockingCodes.length === 0
  const canContinue =
    !terminal &&
    !['blocked', 'validating', 'publishing'].includes(batch.status) &&
    batch.allowed_actions.length > 0 &&
    blockingCodes.length === 0

  return {
    step,
    canGoBack: step !== 'upload' && !terminal,
    canContinue,
    canPublish,
    blockingCodes,
  }
}
