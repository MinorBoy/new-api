import { useTranslation } from 'react-i18next'

import { cn } from '@/lib/utils'

import type { ConfigImportStep } from '../lib/batch-state'

export interface ConfigImportStepperProps {
  steps: readonly ConfigImportStep[]
  activeStep: ConfigImportStep
}

export function ConfigImportStepper(props: ConfigImportStepperProps) {
  const { t } = useTranslation()
  return (
    <ol
      className='flex gap-2 overflow-x-auto text-sm'
      aria-label={t('Import steps')}
    >
      {props.steps.map((step, index) => (
        <li
          key={step}
          className={cn(
            'shrink-0 border px-3 py-2',
            step === props.activeStep &&
              'border-primary bg-primary/10 font-medium'
          )}
        >
          <span className='text-muted-foreground mr-2'>{index + 1}</span>
          {t(stepLabel(step))}
        </li>
      ))}
    </ol>
  )
}

function stepLabel(step: ConfigImportStep): string {
  switch (step) {
    case 'upload':
      return 'Import upload'
    case 'channel_binding':
      return 'Channel bindings'
    case 'conflict_resolution':
      return 'Conflict resolution'
    case 'pricing':
      return 'Pricing review'
    case 'routing_diff':
      return 'Routing diff'
    case 'publish_review':
      return 'Publish review'
    case 'activation':
      return 'Route activation'
    case 'publish_result':
      return 'Publish result'
  }
}
