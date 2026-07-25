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
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Loader2, Scale } from 'lucide-react'
import { useEffect } from 'react'
import { useForm, useWatch } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'

import {
  sideDrawerContentClassName,
  sideDrawerFooterClassName,
  sideDrawerFormClassName,
  sideDrawerHeaderClassName,
  SideDrawerSection,
  SideDrawerSectionHeader,
} from '@/components/drawer-layout'
import { Button } from '@/components/ui/button'
import {
  Field,
  FieldError,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Sheet,
  SheetClose,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Textarea } from '@/components/ui/textarea'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'

import {
  costAccountingQueryKeys,
  reconcileCostAttempt,
  reconcileCostRevenue,
} from '../api'
import type {
  CostAccountingAttemptLedger,
  CostAccountingRequestLedger,
  CostMeter,
  CostRuleConfigV1,
  ReconcileCostAttemptRequest,
  ReconcileCostRevenueRequest,
} from '../types'

export type CostReconcileTarget =
  | { kind: 'attempt'; attempt: CostAccountingAttemptLedger }
  | { kind: 'revenue'; request: CostAccountingRequestLedger }

type CostReconcileDrawerProps = {
  open: boolean
  target: CostReconcileTarget | null
  onOpenChange: (open: boolean) => void
}

const optionalInteger = z.union([
  z.literal(''),
  z.string().regex(/^(?:0|[1-9]\d*)$/, 'Must be a non-negative integer'),
])

const reconcileFormSchema = z.object({
  action: z.enum(['settle', 'confirm_zero']),
  reason: z.string().trim().min(1, 'Reconciliation reason is required'),
  duration_seconds: optionalInteger,
  input_tokens: optionalInteger,
  output_tokens: optionalInteger,
  completion_tokens: optionalInteger,
  total_tokens: optionalInteger,
  final_user_quota: optionalInteger,
})

type ReconcileFormValues = z.infer<typeof reconcileFormSchema>

function parseJSON<T>(value: string): Partial<T> {
  if (!value.trim()) {
    return {}
  }
  try {
    const parsed = JSON.parse(value) as unknown
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
      return {}
    }
    return parsed as Partial<T>
  } catch {
    return {}
  }
}

function textValue(value: number | undefined): string {
  return value === undefined ? '' : String(value)
}

function defaultValues(
  target: CostReconcileTarget | null
): ReconcileFormValues {
  const meter =
    target?.kind === 'attempt'
      ? parseJSON<CostMeter>(target.attempt.actual_meter_json)
      : {}
  return {
    action: 'settle',
    reason: '',
    duration_seconds: meter.duration_seconds ?? '',
    input_tokens: textValue(meter.input_tokens),
    output_tokens: textValue(meter.output_tokens),
    completion_tokens: textValue(meter.completion_tokens),
    total_tokens: textValue(meter.total_tokens),
    final_user_quota:
      target?.kind === 'revenue'
        ? textValue(target.request.final_user_quota)
        : '',
  }
}

function numberValue(value: string, name: string): number | undefined {
  if (value === '') return undefined
  const parsed = Number(value)
  if (!Number.isSafeInteger(parsed)) {
    throw new Error(`${name} is too large`)
  }
  return parsed
}

function attemptRequest(
  attempt: CostAccountingAttemptLedger,
  values: ReconcileFormValues
): ReconcileCostAttemptRequest {
  if (values.action === 'confirm_zero') {
    return { action: 'confirm_zero', reason: values.reason }
  }

  const config = parseJSON<CostRuleConfigV1>(attempt.rule_config_json)
  let meter: CostMeter | undefined
  if (
    attempt.cost_mode === 'per_duration' &&
    attempt.meter_source !== 'validated_request'
  ) {
    const duration = numberValue(values.duration_seconds, 'Duration')
    if (duration === undefined) throw new Error('Duration is required')
    meter = {
      source: attempt.meter_source,
      duration_seconds: String(duration),
    }
  }
  if (attempt.cost_mode === 'per_token') {
    meter = { source: attempt.meter_source }
    if (config.token_mode === 'total_tokens') {
      const total = numberValue(values.total_tokens, 'Total token count')
      if (total === undefined) throw new Error('Total token count is required')
      meter.total_tokens = total
    } else if (config.token_mode === 'completion_tokens') {
      const completion = numberValue(
        values.completion_tokens,
        'Completion token count'
      )
      if (completion === undefined) {
        throw new Error('Completion token count is required')
      }
      meter.completion_tokens = completion
    } else {
      const input = numberValue(values.input_tokens, 'Input token count')
      const output = numberValue(values.output_tokens, 'Output token count')
      if (input === undefined || output === undefined) {
        throw new Error('Input and output token counts are required')
      }
      meter.input_tokens = input
      meter.output_tokens = output
    }
  }
  return { action: 'settle', meter, reason: values.reason }
}

function revenueRequest(
  values: ReconcileFormValues
): ReconcileCostRevenueRequest {
  if (values.action === 'confirm_zero') {
    return {
      action: 'confirm_zero',
      final_user_quota: 0,
      reason: values.reason,
    }
  }
  const finalQuota = numberValue(values.final_user_quota, 'Final user quota')
  if (finalQuota === undefined) throw new Error('Final user quota is required')
  return {
    action: 'settle',
    final_user_quota: finalQuota,
    reason: values.reason,
  }
}

export function CostReconcileDrawer(props: CostReconcileDrawerProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const form = useForm<ReconcileFormValues>({
    resolver: zodResolver(reconcileFormSchema),
    defaultValues: defaultValues(props.target),
  })
  const action = useWatch({ control: form.control, name: 'action' })

  useEffect(() => {
    if (props.open) form.reset(defaultValues(props.target))
  }, [form, props.open, props.target])

  const mutation = useMutation({
    mutationFn: async (values: ReconcileFormValues) => {
      if (!props.target) throw new Error('Reconciliation target is required')
      if (props.target.kind === 'attempt') {
        return reconcileCostAttempt(
          props.target.attempt.id,
          attemptRequest(props.target.attempt, values)
        )
      }
      return reconcileCostRevenue(
        props.target.request.id,
        revenueRequest(values)
      )
    },
    onSuccess: async () => {
      const requestID =
        props.target?.kind === 'attempt'
          ? props.target.attempt.cost_request_id
          : props.target?.request.id
      await Promise.all([
        queryClient.invalidateQueries({
          queryKey: requestID
            ? costAccountingQueryKeys.request(requestID)
            : costAccountingQueryKeys.requests(),
        }),
        queryClient.invalidateQueries({
          queryKey: costAccountingQueryKeys.anomalies(),
        }),
        queryClient.invalidateQueries({
          queryKey: costAccountingQueryKeys.reports(),
        }),
      ])
      toast.success(t('Cost reconciliation saved'))
      props.onOpenChange(false)
    },
    onError: (error) => {
      toast.error(
        error instanceof Error
          ? t(error.message)
          : t('Cost reconciliation failed')
      )
    },
  })

  const attempt = props.target?.kind === 'attempt' ? props.target.attempt : null
  const config = attempt
    ? parseJSON<CostRuleConfigV1>(attempt.rule_config_json)
    : {}
  const showDuration =
    action === 'settle' &&
    attempt?.cost_mode === 'per_duration' &&
    attempt.meter_source !== 'validated_request'
  const showToken = action === 'settle' && attempt?.cost_mode === 'per_token'
  const showRevenueQuota =
    action === 'settle' && props.target?.kind === 'revenue'

  const handleSubmit = form.handleSubmit((values) => {
    try {
      if (props.target?.kind === 'attempt') {
        attemptRequest(props.target.attempt, values)
      } else {
        revenueRequest(values)
      }
      mutation.mutate(values)
    } catch (error) {
      toast.error(
        error instanceof Error ? t(error.message) : t('Invalid input')
      )
    }
  })

  return (
    <Sheet open={props.open} onOpenChange={props.onOpenChange}>
      <SheetContent className={sideDrawerContentClassName('sm:max-w-lg')}>
        <SheetHeader className={sideDrawerHeaderClassName()}>
          <SheetTitle>{t('Reconcile cost accounting')}</SheetTitle>
          <SheetDescription>
            {props.target?.kind === 'attempt'
              ? `${t('Attempt')} #${props.target.attempt.attempt_no}`
              : `${t('Request')} #${props.target?.request.id ?? '-'}`}
          </SheetDescription>
        </SheetHeader>

        <form
          id='cost-reconcile-form'
          className={sideDrawerFormClassName()}
          onSubmit={handleSubmit}
        >
          <SideDrawerSection>
            <SideDrawerSectionHeader
              title={t('Reconciliation action')}
              description={t(
                'Settlement recalculates from the frozen snapshot; confirm zero records an explicit no-charge outcome.'
              )}
              icon={<Scale aria-hidden='true' />}
              iconTone='warning'
            />

            <Field>
              <FieldLabel>{t('Action')}</FieldLabel>
              <ToggleGroup
                value={[action]}
                onValueChange={(selection) => {
                  const next = selection[0]
                  if (next === 'settle' || next === 'confirm_zero') {
                    form.setValue('action', next, { shouldValidate: true })
                  }
                }}
                disabled={mutation.isPending}
              >
                <ToggleGroupItem value='settle'>{t('Settle')}</ToggleGroupItem>
                <ToggleGroupItem value='confirm_zero'>
                  {t('Confirm zero')}
                </ToggleGroupItem>
              </ToggleGroup>
            </Field>

            {showDuration ? (
              <Field data-invalid={!!form.formState.errors.duration_seconds}>
                <FieldLabel htmlFor='reconcile-duration-seconds'>
                  {t('Duration seconds')}
                </FieldLabel>
                <Input
                  id='reconcile-duration-seconds'
                  inputMode='numeric'
                  aria-invalid={!!form.formState.errors.duration_seconds}
                  {...form.register('duration_seconds')}
                />
                <FieldError
                  errors={
                    form.formState.errors.duration_seconds
                      ? [
                          {
                            message: t(
                              form.formState.errors.duration_seconds.message ??
                                'Invalid input'
                            ),
                          },
                        ]
                      : undefined
                  }
                />
              </Field>
            ) : null}

            {showToken ? (
              <FieldGroup className='grid gap-4 sm:grid-cols-2'>
                {config.token_mode === 'total_tokens' ? (
                  <MeterInput
                    id='reconcile-total-tokens'
                    label={t('Total tokens')}
                    name='total_tokens'
                    form={form}
                  />
                ) : null}
                {config.token_mode === 'completion_tokens' ? (
                  <MeterInput
                    id='reconcile-completion-tokens'
                    label={t('Completion tokens')}
                    name='completion_tokens'
                    form={form}
                  />
                ) : null}
                {config.token_mode !== 'total_tokens' &&
                config.token_mode !== 'completion_tokens' ? (
                  <>
                    <MeterInput
                      id='reconcile-input-tokens'
                      label={t('Input tokens')}
                      name='input_tokens'
                      form={form}
                    />
                    <MeterInput
                      id='reconcile-output-tokens'
                      label={t('Output tokens')}
                      name='output_tokens'
                      form={form}
                    />
                  </>
                ) : null}
              </FieldGroup>
            ) : null}

            {showRevenueQuota ? (
              <MeterInput
                id='reconcile-final-user-quota'
                label={t('Final user quota')}
                name='final_user_quota'
                form={form}
              />
            ) : null}

            <Field data-invalid={!!form.formState.errors.reason}>
              <FieldLabel htmlFor='reconcile-reason'>{t('Reason')}</FieldLabel>
              <Textarea
                id='reconcile-reason'
                rows={4}
                aria-invalid={!!form.formState.errors.reason}
                placeholder={t('Document the evidence used for this repair')}
                {...form.register('reason')}
              />
              <FieldError
                errors={
                  form.formState.errors.reason
                    ? [
                        {
                          message: t(
                            form.formState.errors.reason.message ??
                              'Reconciliation reason is required'
                          ),
                        },
                      ]
                    : undefined
                }
              />
            </Field>
          </SideDrawerSection>
        </form>

        <SheetFooter className={sideDrawerFooterClassName()}>
          <SheetClose
            render={
              <Button
                type='button'
                variant='outline'
                disabled={mutation.isPending}
              />
            }
          >
            {t('Cancel')}
          </SheetClose>
          <Button
            type='submit'
            form='cost-reconcile-form'
            disabled={mutation.isPending || !props.target}
          >
            {mutation.isPending ? (
              <Loader2
                data-icon='inline-start'
                className='animate-spin'
                aria-hidden='true'
              />
            ) : null}
            {t('Reconcile')}
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  )
}

function MeterInput(props: {
  id: string
  label: string
  name:
    | 'input_tokens'
    | 'output_tokens'
    | 'completion_tokens'
    | 'total_tokens'
    | 'final_user_quota'
  form: ReturnType<typeof useForm<ReconcileFormValues>>
}) {
  const { t } = useTranslation()
  const error = props.form.formState.errors[props.name]
  return (
    <Field data-invalid={!!error}>
      <FieldLabel htmlFor={props.id}>{props.label}</FieldLabel>
      <Input
        id={props.id}
        inputMode='numeric'
        aria-invalid={!!error}
        {...props.form.register(props.name)}
      />
      <FieldError
        errors={
          error
            ? [
                {
                  message: t(error.message ?? 'Must be a non-negative integer'),
                },
              ]
            : undefined
        }
      />
    </Field>
  )
}
