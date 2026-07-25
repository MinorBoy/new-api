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
import { Calculator, Loader2 } from 'lucide-react'
import { useEffect, useState } from 'react'
import {
  useForm,
  useWatch,
  type FieldPath,
  type Resolver,
  type UseFormReturn,
} from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  SideDrawerSection,
  SideDrawerSectionHeader,
  sideDrawerContentClassName,
  sideDrawerFooterClassName,
  sideDrawerFormClassName,
  sideDrawerHeaderClassName,
} from '@/components/drawer-layout'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field'
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Sheet,
  SheetClose,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'

import type { Channel } from '../../channels/types'
import { TASK_ONLY_CHANNEL_TYPES } from '../../channels/constants'
import {
  costAccountingQueryKeys,
  createCostRule,
  previewCostAccounting,
  updateCostRule,
} from '../api'
import {
  costRuleFormSchema,
  formatMarginPPM,
  formatNanoUSD,
  parseCostRuleForm,
} from '../lib/cost-rule'
import type {
  CostMode,
  CostPreviewRequest,
  CostRule,
  CostRuleFormValues,
} from '../types'

type CostRuleDrawerProps = {
  open: boolean
  channel: Channel
  billableModel: string
  originModel: string
  rule: CostRule | null
  canWrite: boolean
  onOpenChange: (open: boolean) => void
}

type CostRuleFormSeed = Partial<{
  currency: string
  billing_multiplier: string
  purchase_discount_ratio: string
  recharge_exchange_ratio: string
  fee_rate: string
  currency_to_usd_rate: string
  charge_event: 'response_succeeded' | 'submit_accepted' | 'task_succeeded'
  meter_source:
    | 'validated_request'
    | 'upstream_actual'
    | 'upstream_usage'
    | 'local_usage'
  token_mode: 'total_tokens' | 'completion_tokens' | 'input_output'
  unit_price: string
  price_per_second: string
  total_per_million: string
  completion_per_million: string
  input_per_million: string
  output_per_million: string
  zero_cost_reason: string
}>

type SelectOption = { value: string; label: string }

const paidDefaults = {
  currency: 'USD',
  billing_multiplier: '1',
  purchase_discount_ratio: '1',
  recharge_exchange_ratio: '1',
  fee_rate: '0',
  currency_to_usd_rate: '1',
  charge_event: 'response_succeeded' as const,
}

function createCostRuleFormValues(
  mode: CostMode,
  seed: CostRuleFormSeed = {}
): CostRuleFormValues {
  if (mode === 'free') {
    return {
      cost_mode: 'free',
      zero_cost_reason: seed.zero_cost_reason ?? '',
    }
  }

  const paid = {
    ...paidDefaults,
    currency: seed.currency ?? paidDefaults.currency,
    billing_multiplier:
      seed.billing_multiplier ?? paidDefaults.billing_multiplier,
    purchase_discount_ratio:
      seed.purchase_discount_ratio ?? paidDefaults.purchase_discount_ratio,
    recharge_exchange_ratio:
      seed.recharge_exchange_ratio ?? paidDefaults.recharge_exchange_ratio,
    fee_rate: seed.fee_rate ?? paidDefaults.fee_rate,
    currency_to_usd_rate:
      seed.currency_to_usd_rate ?? paidDefaults.currency_to_usd_rate,
    charge_event: seed.charge_event ?? paidDefaults.charge_event,
  }
  if (mode === 'per_request') {
    return {
      cost_mode: 'per_request',
      ...paid,
      unit_price: seed.unit_price ?? '1',
    }
  }
  if (mode === 'per_duration') {
    return {
      cost_mode: 'per_duration',
      ...paid,
      meter_source:
        seed.meter_source === 'validated_request' ||
        seed.meter_source === 'upstream_actual'
          ? seed.meter_source
          : 'validated_request',
      price_per_second: seed.price_per_second ?? '0.01',
    }
  }

  const tokenPaid = {
    ...paid,
    charge_event:
      paid.charge_event === 'submit_accepted'
        ? ('response_succeeded' as const)
        : paid.charge_event,
    meter_source:
      seed.meter_source === 'upstream_usage' ||
      seed.meter_source === 'local_usage'
        ? seed.meter_source
        : ('upstream_usage' as const),
  }
  if (seed.token_mode === 'completion_tokens') {
    return {
      cost_mode: 'per_token',
      ...tokenPaid,
      token_mode: 'completion_tokens',
      completion_per_million: seed.completion_per_million ?? '1',
    }
  }
  if (seed.token_mode === 'input_output') {
    return {
      cost_mode: 'per_token',
      ...tokenPaid,
      token_mode: 'input_output',
      input_per_million: seed.input_per_million ?? '1',
      output_per_million: seed.output_per_million ?? '1',
    }
  }
  return {
    cost_mode: 'per_token',
    ...tokenPaid,
    token_mode: 'total_tokens',
    total_per_million: seed.total_per_million ?? '1',
  }
}

function costRuleFormValues(
  rule: CostRule | null,
  taskOnly: boolean
): CostRuleFormValues {
  if (!rule) {
    return createCostRuleFormValues(
      'per_request',
      taskOnly ? { charge_event: 'task_succeeded' } : {}
    )
  }
  return createCostRuleFormValues(rule.cost_mode, rule.config)
}

function CostRuleTextField(props: {
  form: UseFormReturn<CostRuleFormValues>
  name: FieldPath<CostRuleFormValues>
  label: string
  disabled: boolean
  type?: 'text' | 'number'
}) {
  return (
    <FormField
      control={props.form.control}
      name={props.name}
      render={({ field }) => (
        <FormItem>
          <FormLabel>{props.label}</FormLabel>
          <FormControl>
            <Input
              {...field}
              type={props.type ?? 'text'}
              value={typeof field.value === 'string' ? field.value : ''}
              disabled={props.disabled}
              autoComplete='off'
            />
          </FormControl>
          <FormMessage />
        </FormItem>
      )}
    />
  )
}

function CostRuleSelectField(props: {
  form: UseFormReturn<CostRuleFormValues>
  name: FieldPath<CostRuleFormValues>
  label: string
  options: SelectOption[]
  disabled: boolean
}) {
  return (
    <FormField
      control={props.form.control}
      name={props.name}
      render={({ field }) => (
        <FormItem>
          <FormLabel>{props.label}</FormLabel>
          <Select
            items={props.options}
            value={typeof field.value === 'string' ? field.value : ''}
            onValueChange={field.onChange}
            disabled={props.disabled}
          >
            <FormControl>
              <SelectTrigger className='w-full'>
                <SelectValue />
              </SelectTrigger>
            </FormControl>
            <SelectContent align='start'>
              <SelectGroup>
                {props.options.map((option) => (
                  <SelectItem key={option.value} value={option.value}>
                    {option.label}
                  </SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
          <FormMessage />
        </FormItem>
      )}
    />
  )
}

function parseSampleInteger(value: string, label: string): number {
  if (!/^(?:0|[1-9]\d*)$/.test(value)) {
    throw new Error(`${label} must be a non-negative integer`)
  }
  const parsed = Number(value)
  if (!Number.isSafeInteger(parsed)) {
    throw new Error(`${label} is too large`)
  }
  return parsed
}

function buildPreviewRequest(
  props: CostRuleDrawerProps,
  values: CostRuleFormValues,
  samples: {
    userGroup: string
    duration: string
    inputTokens: string
    outputTokens: string
  }
): CostPreviewRequest {
  const request: CostPreviewRequest = {
    origin_model: props.originModel || props.billableModel,
    user_group: samples.userGroup.trim(),
    relay_mode: 1,
    request_path: '/v1/chat/completions',
    cost_mode: values.cost_mode,
    config: parseCostRuleForm(values),
    meter: { source: '' },
  }
  if (!request.user_group) {
    throw new Error('User group is required')
  }
  if (values.cost_mode === 'per_duration') {
    const duration = parseSampleInteger(samples.duration, 'Duration')
    if (duration === 0) throw new Error('Duration must be greater than zero')
    request.duration_seconds = duration
    request.meter = {
      source: values.meter_source,
      duration_seconds: samples.duration,
    }
  }
  if (values.cost_mode === 'per_token') {
    const input = parseSampleInteger(samples.inputTokens, 'Input token count')
    const output = parseSampleInteger(
      samples.outputTokens,
      'Output token count'
    )
    request.usage = {
      prompt_tokens: input,
      completion_tokens: output,
      total_tokens: input + output,
      input_tokens: input,
      output_tokens: output,
    }
    request.meter = { source: values.meter_source }
    if (values.token_mode === 'total_tokens') {
      request.meter.total_tokens = input + output
    } else if (values.token_mode === 'completion_tokens') {
      request.meter.completion_tokens = output
    } else {
      request.meter.input_tokens = input
      request.meter.output_tokens = output
    }
  }
  return request
}

export function CostRuleDrawer(props: CostRuleDrawerProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const isEditingDraft = props.rule?.status === 'draft'
  const taskOnly = TASK_ONLY_CHANNEL_TYPES.has(props.channel.type)
  const form = useForm<CostRuleFormValues>({
    resolver: zodResolver(costRuleFormSchema) as Resolver<CostRuleFormValues>,
    defaultValues: costRuleFormValues(props.rule, taskOnly),
  })
  const costMode = useWatch({ control: form.control, name: 'cost_mode' })
  const tokenMode = useWatch({ control: form.control, name: 'token_mode' })
  const [note, setNote] = useState(props.rule?.note ?? '')
  const [userGroup, setUserGroup] = useState(
    props.channel.group.split(',')[0] || 'default'
  )
  const [duration, setDuration] = useState('10')
  const [inputTokens, setInputTokens] = useState('1000')
  const [outputTokens, setOutputTokens] = useState('500')

  useEffect(() => {
    if (!props.open) return
    form.reset(costRuleFormValues(props.rule, taskOnly))
    setNote(props.rule?.note ?? '')
  }, [form, props.open, props.rule, taskOnly])

  const invalidateCostQueries = async () => {
    await Promise.all([
      queryClient.invalidateQueries({
        queryKey: costAccountingQueryKeys.rules(),
      }),
      queryClient.invalidateQueries({
        queryKey: costAccountingQueryKeys.coverages(),
      }),
    ])
  }

  const saveMutation = useMutation({
    mutationFn: async (values: CostRuleFormValues) => {
      const config = parseCostRuleForm(values)
      if (isEditingDraft && props.rule) {
        return updateCostRule(props.rule.id, {
          cost_mode: values.cost_mode,
          config,
          note,
        })
      }
      return createCostRule({
        channel_id: props.channel.id,
        billable_upstream_model: props.billableModel,
        cost_mode: values.cost_mode,
        config,
        note,
      })
    },
    onSuccess: async () => {
      await invalidateCostQueries()
      toast.success(
        t(isEditingDraft ? 'Cost rule updated' : 'Cost draft saved')
      )
      props.onOpenChange(false)
    },
    onError: () => toast.error(t('Failed to save cost rule')),
  })

  const previewMutation = useMutation({
    mutationFn: (values: CostRuleFormValues) =>
      previewCostAccounting(
        buildPreviewRequest(props, values, {
          userGroup,
          duration,
          inputTokens,
          outputTokens,
        })
      ),
    onError: (error) => {
      toast.error(
        error instanceof Error ? t(error.message) : t('Failed to preview cost')
      )
    },
  })

  const handleModeChange = (selection: string[]) => {
    const nextMode = selection[0] as CostMode | undefined
    if (!nextMode || nextMode === costMode) return
    form.reset(
      createCostRuleFormValues(
        nextMode,
        form.getValues() as unknown as CostRuleFormSeed
      )
    )
  }

  const chargeEventOptions: SelectOption[] = [
    { value: 'response_succeeded', label: t('Response succeeded') },
    { value: 'submit_accepted', label: t('Submit accepted') },
    { value: 'task_succeeded', label: t('Task succeeded') },
  ]
  const durationMeterOptions: SelectOption[] = [
    { value: 'validated_request', label: t('Validated request') },
    { value: 'upstream_actual', label: t('Upstream actual') },
  ]
  const tokenMeterOptions: SelectOption[] = [
    { value: 'upstream_usage', label: t('Upstream usage') },
    { value: 'local_usage', label: t('Local usage') },
  ]
  const tokenModeOptions: SelectOption[] = [
    { value: 'total_tokens', label: t('Total tokens') },
    { value: 'completion_tokens', label: t('Completion tokens') },
    { value: 'input_output', label: t('Input and output tokens') },
  ]
  const disabled = !props.canWrite || saveMutation.isPending
  const preview = previewMutation.data?.data
  let drawerTitle = 'Create cost rule'
  if (props.rule) drawerTitle = 'Create cost rule version'
  if (isEditingDraft) drawerTitle = 'Edit cost rule draft'

  return (
    <Sheet open={props.open} onOpenChange={props.onOpenChange}>
      <SheetContent className={sideDrawerContentClassName('sm:max-w-2xl')}>
        <SheetHeader className={sideDrawerHeaderClassName()}>
          <SheetTitle>{t(drawerTitle)}</SheetTitle>
          <SheetDescription>
            {props.channel.name} · {props.billableModel}
          </SheetDescription>
        </SheetHeader>

        <Form {...form}>
          <form
            id='cost-rule-form'
            className={sideDrawerFormClassName()}
            onSubmit={form.handleSubmit((values) =>
              saveMutation.mutate(values)
            )}
          >
            <SideDrawerSection>
              <SideDrawerSectionHeader
                title={t('Supplier cost rule')}
                description={t(
                  'Configure the supplier charge event and exact source price.'
                )}
              />

              <FormItem>
                <FormLabel>{t('Cost mode')}</FormLabel>
                <ToggleGroup
                  value={[costMode]}
                  onValueChange={handleModeChange}
                  disabled={disabled}
                  className='grid grid-cols-2 sm:grid-cols-4'
                >
                  <ToggleGroupItem value='free'>{t('Free')}</ToggleGroupItem>
                  <ToggleGroupItem value='per_request'>
                    {t('Per request')}
                  </ToggleGroupItem>
                  <ToggleGroupItem value='per_duration'>
                    {t('Per duration')}
                  </ToggleGroupItem>
                  <ToggleGroupItem value='per_token'>
                    {t('Per token')}
                  </ToggleGroupItem>
                </ToggleGroup>
              </FormItem>

              {costMode === 'free' ? (
                <CostRuleTextField
                  form={form}
                  name='zero_cost_reason'
                  label={t('Free cost reason')}
                  disabled={disabled}
                />
              ) : (
                <>
                  <div className='grid gap-4 sm:grid-cols-2'>
                    <CostRuleTextField
                      form={form}
                      name='currency'
                      label={t('Currency')}
                      disabled={disabled}
                    />
                    <CostRuleSelectField
                      form={form}
                      name='charge_event'
                      label={t('Charge event')}
                      options={chargeEventOptions}
                      disabled={disabled}
                    />
                  </div>

                  {costMode === 'per_request' ? (
                    <CostRuleTextField
                      form={form}
                      name='unit_price'
                      label={t('Unit price')}
                      disabled={disabled}
                    />
                  ) : null}
                  {costMode === 'per_duration' ? (
                    <div className='grid gap-4 sm:grid-cols-2'>
                      <CostRuleTextField
                        form={form}
                        name='price_per_second'
                        label={t('Price per second')}
                        disabled={disabled}
                      />
                      <CostRuleSelectField
                        form={form}
                        name='meter_source'
                        label={t('Meter source')}
                        options={durationMeterOptions}
                        disabled={disabled}
                      />
                    </div>
                  ) : null}
                  {costMode === 'per_token' ? (
                    <>
                      <div className='grid gap-4 sm:grid-cols-2'>
                        <CostRuleSelectField
                          form={form}
                          name='meter_source'
                          label={t('Meter source')}
                          options={tokenMeterOptions}
                          disabled={disabled}
                        />
                        <CostRuleSelectField
                          form={form}
                          name='token_mode'
                          label={t('Token mode')}
                          options={tokenModeOptions}
                          disabled={disabled}
                        />
                      </div>
                      {tokenMode === 'total_tokens' ? (
                        <CostRuleTextField
                          form={form}
                          name='total_per_million'
                          label={t('Price per 1M total tokens')}
                          disabled={disabled}
                        />
                      ) : null}
                      {tokenMode === 'completion_tokens' ? (
                        <CostRuleTextField
                          form={form}
                          name='completion_per_million'
                          label={t('Price per 1M completion tokens')}
                          disabled={disabled}
                        />
                      ) : null}
                      {tokenMode === 'input_output' ? (
                        <div className='grid gap-4 sm:grid-cols-2'>
                          <CostRuleTextField
                            form={form}
                            name='input_per_million'
                            label={t('Input price per 1M tokens')}
                            disabled={disabled}
                          />
                          <CostRuleTextField
                            form={form}
                            name='output_per_million'
                            label={t('Output price per 1M tokens')}
                            disabled={disabled}
                          />
                        </div>
                      ) : null}
                    </>
                  ) : null}

                  <div className='grid gap-4 sm:grid-cols-2'>
                    <CostRuleTextField
                      form={form}
                      name='billing_multiplier'
                      label={t('Billing multiplier')}
                      disabled={disabled}
                    />
                    <CostRuleTextField
                      form={form}
                      name='purchase_discount_ratio'
                      label={t('Purchase discount ratio')}
                      disabled={disabled}
                    />
                    <CostRuleTextField
                      form={form}
                      name='recharge_exchange_ratio'
                      label={t('Recharge exchange ratio')}
                      disabled={disabled}
                    />
                    <CostRuleTextField
                      form={form}
                      name='fee_rate'
                      label={t('Fee rate')}
                      disabled={disabled}
                    />
                    <CostRuleTextField
                      form={form}
                      name='currency_to_usd_rate'
                      label={t('Currency to USD rate')}
                      disabled={disabled}
                    />
                  </div>
                </>
              )}

              <Field>
                <FieldLabel htmlFor='cost-rule-note'>{t('Note')}</FieldLabel>
                <Input
                  id='cost-rule-note'
                  value={note}
                  onChange={(event) => setNote(event.target.value)}
                  disabled={disabled}
                />
              </Field>
            </SideDrawerSection>

            <SideDrawerSection>
              <div className='flex items-start justify-between gap-3'>
                <SideDrawerSectionHeader
                  title={t('Estimated preview')}
                  description={t(
                    'Preview billing revenue, supplier cost, gross profit, and margin without writing ledger data.'
                  )}
                />
                <Badge variant='outline'>{t('Estimated')}</Badge>
              </div>

              <FieldGroup className='grid gap-4 sm:grid-cols-2'>
                <Field>
                  <FieldLabel htmlFor='cost-preview-group'>
                    {t('User group')}
                  </FieldLabel>
                  <Input
                    id='cost-preview-group'
                    value={userGroup}
                    onChange={(event) => setUserGroup(event.target.value)}
                  />
                </Field>
                {costMode === 'per_duration' ? (
                  <Field>
                    <FieldLabel htmlFor='cost-preview-duration'>
                      {t('Duration seconds')}
                    </FieldLabel>
                    <Input
                      id='cost-preview-duration'
                      inputMode='numeric'
                      value={duration}
                      onChange={(event) => setDuration(event.target.value)}
                    />
                  </Field>
                ) : null}
                {costMode === 'per_token' ? (
                  <>
                    <Field>
                      <FieldLabel htmlFor='cost-preview-input-tokens'>
                        {t('Input tokens')}
                      </FieldLabel>
                      <Input
                        id='cost-preview-input-tokens'
                        inputMode='numeric'
                        value={inputTokens}
                        onChange={(event) => setInputTokens(event.target.value)}
                      />
                    </Field>
                    <Field>
                      <FieldLabel htmlFor='cost-preview-output-tokens'>
                        {t('Output tokens')}
                      </FieldLabel>
                      <Input
                        id='cost-preview-output-tokens'
                        inputMode='numeric'
                        value={outputTokens}
                        onChange={(event) =>
                          setOutputTokens(event.target.value)
                        }
                      />
                    </Field>
                  </>
                ) : null}
              </FieldGroup>

              <Button
                type='button'
                variant='outline'
                disabled={previewMutation.isPending}
                onClick={() =>
                  void form.handleSubmit((values) =>
                    previewMutation.mutate(values)
                  )()
                }
              >
                {previewMutation.isPending ? (
                  <Loader2
                    data-icon='inline-start'
                    className='animate-spin'
                    aria-hidden='true'
                  />
                ) : (
                  <Calculator data-icon='inline-start' aria-hidden='true' />
                )}
                {t('Preview cost')}
              </Button>

              {preview ? (
                <dl className='border-border/60 grid grid-cols-2 gap-x-4 gap-y-3 border-t pt-4 text-sm sm:grid-cols-4'>
                  <div>
                    <dt className='text-muted-foreground text-xs'>
                      {t('Billed revenue equivalent')}
                    </dt>
                    <dd className='mt-1 font-mono'>
                      {formatNanoUSD(preview.revenue_nano_usd)}
                    </dd>
                  </div>
                  <div>
                    <dt className='text-muted-foreground text-xs'>
                      {t('Supplier cost')}
                    </dt>
                    <dd className='mt-1 font-mono'>
                      {formatNanoUSD(preview.cost_nano_usd)}
                    </dd>
                  </div>
                  <div>
                    <dt className='text-muted-foreground text-xs'>
                      {t('Billed gross profit')}
                    </dt>
                    <dd className='mt-1 font-mono'>
                      {formatNanoUSD(preview.profit_nano_usd)}
                    </dd>
                  </div>
                  <div>
                    <dt className='text-muted-foreground text-xs'>
                      {t('Gross margin')}
                    </dt>
                    <dd className='mt-1 font-mono'>
                      {formatMarginPPM(preview.margin_ppm ?? null)}
                    </dd>
                  </div>
                </dl>
              ) : null}
            </SideDrawerSection>
          </form>
        </Form>

        <SheetFooter className={sideDrawerFooterClassName()}>
          <SheetClose
            render={
              <Button
                type='button'
                variant='outline'
                disabled={saveMutation.isPending}
              />
            }
          >
            {t('Cancel')}
          </SheetClose>
          {props.canWrite ? (
            <Button
              type='submit'
              form='cost-rule-form'
              disabled={saveMutation.isPending}
            >
              {saveMutation.isPending ? (
                <Loader2
                  data-icon='inline-start'
                  className='animate-spin'
                  aria-hidden='true'
                />
              ) : null}
              {t(isEditingDraft ? 'Save changes' : 'Save draft')}
            </Button>
          ) : null}
        </SheetFooter>
      </SheetContent>
    </Sheet>
  )
}
