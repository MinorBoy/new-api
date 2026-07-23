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
/* eslint-disable react-refresh/only-export-components */
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { isAxiosError } from 'axios'
import { Loader2, Plus } from 'lucide-react'
import { useEffect } from 'react'
import {
  useFieldArray,
  useForm,
  useWatch,
  type FieldPath,
  type UseFormReturn,
} from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  SideDrawerSection,
  sideDrawerContentClassName,
  sideDrawerFooterClassName,
  sideDrawerFormClassName,
  sideDrawerHeaderClassName,
} from '@/components/drawer-layout'
import { Button } from '@/components/ui/button'
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import {
  Combobox,
  ComboboxContent,
  ComboboxEmpty,
  ComboboxInput,
  ComboboxItem,
  ComboboxList,
} from '@/components/ui/combobox'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
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
import { Switch } from '@/components/ui/switch'

import {
  createRoutingPolicy,
  listRoutingGroups,
  listRoutingCandidates,
  updateRoutingPolicy,
} from '../api'
import { routingPolicyQueryKeys } from '../query-keys'
import {
  ASPECT_RATIOS,
  CANONICAL_SEEDANCE_MODELS,
  MAX_TASK_DURATION_SECONDS,
  OUTPUT_RESOLUTIONS,
  clearUnavailableTargetChannels,
  copyPolicyForm,
  copyTargetForm,
  createEmptyPolicyForm,
  createEmptyTarget,
  fromPolicyResponse,
  normalizeRoutingGroups,
  routingPolicyErrorSchema,
  routingPolicyFormSchema,
  toWriteRequest,
  type RoutingPolicy,
  type RoutingCandidate,
  type RoutingPolicyError,
  type RoutingPolicyFormValues,
} from '../types'
import { RouteTargetEditor } from './route-target-editor'

type RoutingPolicyDrawerProps = {
  open: boolean
  editingPolicy: RoutingPolicy | null
  copyingPolicy: RoutingPolicy | null
  onOpenChange: (open: boolean) => void
}

const EMPTY_CANDIDATES: RoutingCandidate[] = []

export function getApiErrorPayload(
  error: unknown
): RoutingPolicyError | undefined {
  if (!isAxiosError(error)) {
    return undefined
  }
  const parsed = routingPolicyErrorSchema.safeParse(error.response?.data)
  return parsed.success ? parsed.data : undefined
}

export function applyRoutingPolicyError(
  error: unknown,
  form: UseFormReturn<RoutingPolicyFormValues>
): string | undefined {
  const payload = getApiErrorPayload(error)
  if (payload?.code === 'routing_target_overlap') {
    for (const index of payload.data?.target_indexes ?? []) {
      form.setError(`targets.${index}.target_priority`, {
        type: 'server',
        message: 'This target overlaps another target at the same priority',
      })
    }
    return undefined
  }
  if (payload?.data?.field) {
    form.setError(payload.data.field as FieldPath<RoutingPolicyFormValues>, {
      type: 'server',
      message: payload.message,
    })
    return undefined
  }
  return payload?.message ?? 'Failed to save routing policy'
}

export function RoutingPolicyDrawer(props: RoutingPolicyDrawerProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const isEditing = props.editingPolicy !== null
  const form = useForm<RoutingPolicyFormValues>({
    resolver: zodResolver(routingPolicyFormSchema),
    defaultValues: createEmptyPolicyForm(),
  })
  const targets = useFieldArray({ control: form.control, name: 'targets' })
  const groupName = useWatch({ control: form.control, name: 'group_name' })
  const modelName = useWatch({ control: form.control, name: 'model' })
  const enabled = useWatch({ control: form.control, name: 'enabled' })

  const candidatesQuery = useQuery({
    queryKey: routingPolicyQueryKeys.candidates(groupName, modelName),
    queryFn: () => listRoutingCandidates(groupName, modelName),
    enabled: groupName.length > 0 && modelName.length > 0 && props.open,
  })
  const groupsQuery = useQuery({
    queryKey: routingPolicyQueryKeys.groups(),
    queryFn: listRoutingGroups,
    enabled: props.open,
  })
  const candidates = candidatesQuery.data?.data
  const groupOptions = normalizeRoutingGroups(
    groupsQuery.data?.data ?? [],
    groupName
  )

  useEffect(() => {
    if (!props.open) {
      return
    }
    if (props.editingPolicy) {
      form.reset(fromPolicyResponse(props.editingPolicy))
      return
    }
    if (props.copyingPolicy) {
      form.reset(copyPolicyForm(props.copyingPolicy))
      return
    }
    form.reset(createEmptyPolicyForm())
  }, [form, props.copyingPolicy, props.editingPolicy, props.open])

  useEffect(() => {
    if (!candidates) {
      return
    }
    const current = form.getValues('targets')
    const next = clearUnavailableTargetChannels(
      current,
      candidates.map((candidate) => candidate.id)
    )
    if (next.some((target, index) => target !== current[index])) {
      form.setValue('targets', next, { shouldValidate: true })
    }
  }, [candidates, form])

  const saveMutation = useMutation({
    mutationFn: async (values: RoutingPolicyFormValues) => {
      const payload = toWriteRequest(values)
      if (isEditing && props.editingPolicy) {
        return updateRoutingPolicy(props.editingPolicy.id, payload)
      }
      return createRoutingPolicy(payload)
    },
    onSuccess: async () => {
      await queryClient.invalidateQueries({
        queryKey: routingPolicyQueryKeys.all,
      })
      toast.success(
        t(isEditing ? 'Routing policy updated' : 'Routing policy created')
      )
      props.onOpenChange(false)
    },
    onError: (error) => {
      const message = applyRoutingPolicyError(error, form)
      if (message) {
        toast.error(t(message))
      }
    },
  })

  const handleOpenChange = (open: boolean) => {
    if (!open && !saveMutation.isPending) {
      props.onOpenChange(false)
    }
  }

  const handleSubmit = (values: RoutingPolicyFormValues) => {
    form.clearErrors()
    const selectableGroups = normalizeRoutingGroups(
      groupsQuery.data?.data ?? [],
      isEditing || props.copyingPolicy ? values.group_name : ''
    )
    if (
      !groupsQuery.isSuccess ||
      !selectableGroups.includes(values.group_name)
    ) {
      form.setError('group_name', {
        type: 'validate',
        message: groupsQuery.isError
          ? 'Failed to load groups'
          : 'Group is required',
      })
      return
    }
    saveMutation.mutate(values)
  }

  const targetsError = form.formState.errors.targets?.message

  return (
    <Sheet open={props.open} onOpenChange={handleOpenChange}>
      <SheetContent className={sideDrawerContentClassName('sm:max-w-3xl')}>
        <SheetHeader className={sideDrawerHeaderClassName()}>
          <SheetTitle>
            {isEditing ? t('Edit routing policy') : t('Create routing policy')}
          </SheetTitle>
          <SheetDescription>
            {t(
              'Route one canonical Seedance model to compatible upstream targets.'
            )}
          </SheetDescription>
        </SheetHeader>

        <Form {...form}>
          <form
            id='routing-policy-form'
            onSubmit={form.handleSubmit(handleSubmit)}
            className={sideDrawerFormClassName()}
          >
            <SideDrawerSection>
              <div className='grid gap-4 sm:grid-cols-2'>
                <FormField
                  control={form.control}
                  name='group_name'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Group')}</FormLabel>
                      <Combobox
                        value={field.value || null}
                        onValueChange={(value) => field.onChange(value ?? '')}
                        items={groupOptions}
                        disabled={groupsQuery.isLoading}
                      >
                        <FormControl>
                          <ComboboxInput
                            placeholder={
                              groupsQuery.isLoading
                                ? t('Loading groups...')
                                : t('Search groups...')
                            }
                            aria-label={t('Group')}
                          >
                            <ComboboxContent>
                              <ComboboxList>
                                {(group: string) => (
                                  <ComboboxItem key={group} value={group}>
                                    {group}
                                  </ComboboxItem>
                                )}
                              </ComboboxList>
                              <ComboboxEmpty>
                                {t('No groups available')}
                              </ComboboxEmpty>
                            </ComboboxContent>
                          </ComboboxInput>
                        </FormControl>
                      </Combobox>
                      {groupsQuery.isSuccess && groupOptions.length === 0 && (
                        <p className='text-muted-foreground text-xs'>
                          {t('No groups available')}
                        </p>
                      )}
                      {groupsQuery.isError && (
                        <div className='flex items-center justify-between gap-2'>
                          <p className='text-destructive text-xs'>
                            {t('Failed to load groups')}
                          </p>
                          <Button
                            type='button'
                            variant='ghost'
                            size='sm'
                            onClick={() => void groupsQuery.refetch()}
                          >
                            {t('Retry')}
                          </Button>
                        </div>
                      )}
                      <FormMessage />
                    </FormItem>
                  )}
                />

                <FormField
                  control={form.control}
                  name='model'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Canonical model')}</FormLabel>
                      <Select
                        value={field.value}
                        onValueChange={field.onChange}
                      >
                        <FormControl>
                          <SelectTrigger className='w-full font-mono text-xs'>
                            <SelectValue />
                          </SelectTrigger>
                        </FormControl>
                        <SelectContent align='start'>
                          {CANONICAL_SEEDANCE_MODELS.map((model) => (
                            <SelectItem
                              key={model}
                              value={model}
                              className='font-mono text-xs'
                            >
                              {model}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>

              <FormField
                control={form.control}
                name='enabled'
                render={({ field }) => (
                  <FormItem className='flex items-center justify-between gap-3'>
                    <FormLabel>{t('Enabled')}</FormLabel>
                    <FormControl>
                      <Switch
                        checked={field.value}
                        onCheckedChange={field.onChange}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <div className='grid gap-4 sm:grid-cols-3'>
                <FormField
                  control={form.control}
                  name='defaults.output_resolution'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Default resolution')}</FormLabel>
                      <Select
                        value={field.value}
                        onValueChange={field.onChange}
                      >
                        <FormControl>
                          <SelectTrigger className='w-full'>
                            <SelectValue />
                          </SelectTrigger>
                        </FormControl>
                        <SelectContent align='start'>
                          {OUTPUT_RESOLUTIONS.map((resolution) => (
                            <SelectItem key={resolution} value={resolution}>
                              {resolution}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                <FormField
                  control={form.control}
                  name='defaults.duration_seconds'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Default duration')}</FormLabel>
                      <FormControl>
                        <Input
                          type='number'
                          min={1}
                          max={MAX_TASK_DURATION_SECONDS}
                          value={field.value}
                          onChange={(event) =>
                            field.onChange(Number(event.target.value))
                          }
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                <FormField
                  control={form.control}
                  name='defaults.aspect_ratio'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Default aspect ratio')}</FormLabel>
                      <Select
                        value={field.value}
                        onValueChange={field.onChange}
                      >
                        <FormControl>
                          <SelectTrigger className='w-full'>
                            <SelectValue />
                          </SelectTrigger>
                        </FormControl>
                        <SelectContent align='start'>
                          {ASPECT_RATIOS.map((ratio) => (
                            <SelectItem key={ratio} value={ratio}>
                              {ratio}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>
            </SideDrawerSection>

            <section className='flex flex-col gap-4'>
              <div className='flex items-center justify-between gap-3'>
                <div className='min-w-0'>
                  <h3 className='text-sm font-semibold'>
                    {t('Routing targets')}
                  </h3>
                  <p className='text-muted-foreground text-xs'>
                    {t(
                      'Each channel may define multiple targets with different capability ranges.'
                    )}
                  </p>
                </div>
                <Button
                  type='button'
                  variant='outline'
                  size='sm'
                  onClick={() => targets.append(createEmptyTarget())}
                >
                  <Plus data-icon='inline-start' aria-hidden='true' />
                  {t('Add target')}
                </Button>
              </div>

              {targets.fields.map((target, index) => (
                <RouteTargetEditor
                  key={target.id}
                  form={form}
                  index={index}
                  candidates={candidates ?? EMPTY_CANDIDATES}
                  candidatesLoading={candidatesQuery.isLoading}
                  canRemove={!enabled || targets.fields.length > 1}
                  onCopy={() => {
                    const source = form.getValues(`targets.${index}`)
                    targets.insert(index + 1, copyTargetForm(source))
                  }}
                  onRemove={() => targets.remove(index)}
                />
              ))}

              {targets.fields.length === 0 && (
                <p className='text-muted-foreground py-6 text-center text-sm'>
                  {t('No routing targets configured')}
                </p>
              )}
              {targetsError && (
                <p className='text-destructive text-sm'>{t(targetsError)}</p>
              )}
            </section>
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
          <Button
            type='submit'
            form='routing-policy-form'
            disabled={saveMutation.isPending}
          >
            {saveMutation.isPending && (
              <Loader2
                data-icon='inline-start'
                className='animate-spin'
                aria-hidden='true'
              />
            )}
            {t('Save policy')}
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  )
}
