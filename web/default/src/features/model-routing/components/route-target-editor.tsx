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
import { Copy, Plus, Trash2, X } from 'lucide-react'
import type { UseFormReturn } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Combobox } from '@/components/ui/combobox'
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'

import {
  ASPECT_RATIOS,
  MAX_TASK_DURATION_SECONDS,
  OUTPUT_RESOLUTIONS,
  type RoutingCandidate,
  type RoutingPolicyFormValues,
} from '../types'

type RouteTargetEditorProps = {
  form: UseFormReturn<RoutingPolicyFormValues>
  index: number
  candidates: RoutingCandidate[]
  candidatesLoading: boolean
  canRemove: boolean
  onCopy: () => void
  onRemove: () => void
}

function numericValue(value: string): number {
  return value === '' ? 0 : Number(value)
}

const ROUTING_SELECTED_CLASS =
  'data-[state=on]:border-primary data-[state=on]:bg-primary/15 data-[state=on]:text-primary dark:data-[state=on]:bg-primary/20'

export function RouteTargetEditor(props: RouteTargetEditorProps) {
  const { t } = useTranslation()
  const target = props.form.watch(`targets.${props.index}`)
  const durationValues = target?.durations.values ?? []
  const candidateOptions = props.candidates.map((candidate) => ({
    value: String(candidate.id),
    label: `${candidate.name} (#${candidate.id}) · ${
      candidate.status === 1 ? t('Enabled') : t('Disabled')
    } · P${candidate.priority} · W${candidate.weight}`,
  }))

  const setDurationMode = (mode: 'values' | 'range') => {
    if (mode === target?.durations.mode) {
      return
    }
    if (mode === 'values') {
      props.form.setValue(`targets.${props.index}.durations`, {
        mode,
        values: [5, 10, 15],
        min: undefined,
        max: undefined,
      })
      return
    }
    props.form.setValue(`targets.${props.index}.durations`, {
      mode,
      values: [],
      min: 4,
      max: 15,
    })
  }

  const updateDurationValue = (durationIndex: number, value: number) => {
    if (
      durationValues.some(
        (duration, index) => index !== durationIndex && duration === value
      )
    ) {
      return
    }
    const next = [...durationValues]
    next[durationIndex] = value
    props.form.setValue(`targets.${props.index}.durations.values`, next, {
      shouldValidate: true,
    })
  }

  const removeDurationValue = (durationIndex: number) => {
    props.form.setValue(
      `targets.${props.index}.durations.values`,
      durationValues.filter((_, index) => index !== durationIndex),
      { shouldValidate: true }
    )
  }

  const addDurationValue = () => {
    const largest = durationValues.length > 0 ? Math.max(...durationValues) : 0
    const next = Math.min(MAX_TASK_DURATION_SECONDS, Math.max(1, largest + 5))
    if (durationValues.includes(next)) {
      return
    }
    props.form.setValue(
      `targets.${props.index}.durations.values`,
      [...durationValues, next],
      { shouldValidate: true }
    )
  }

  return (
    <div className='flex flex-col gap-5 rounded-md border p-3 sm:p-4'>
      <div className='flex min-w-0 items-center justify-between gap-2'>
        <p className='min-w-0 truncate text-sm font-medium'>
          {target?.name || `${t('Routing target')} ${props.index + 1}`}
        </p>
        <TooltipProvider>
          <div className='flex shrink-0 items-center gap-1'>
            <Tooltip>
              <TooltipTrigger
                render={
                  <Button
                    type='button'
                    variant='ghost'
                    size='icon-lg'
                    aria-label={t('Copy')}
                    title={t('Copy')}
                    onClick={props.onCopy}
                  />
                }
              >
                <Copy aria-hidden='true' />
              </TooltipTrigger>
              <TooltipContent>{t('Copy')}</TooltipContent>
            </Tooltip>
            <Tooltip>
              <TooltipTrigger
                render={
                  <Button
                    type='button'
                    variant='ghost'
                    size='icon-lg'
                    aria-label={t('Delete')}
                    title={t('Delete')}
                    disabled={!props.canRemove}
                    onClick={props.onRemove}
                  />
                }
              >
                <Trash2 aria-hidden='true' />
              </TooltipTrigger>
              <TooltipContent>{t('Delete')}</TooltipContent>
            </Tooltip>
          </div>
        </TooltipProvider>
      </div>

      <div className='grid gap-4 lg:grid-cols-2'>
        <FormField
          control={props.form.control}
          name={`targets.${props.index}.channel_id`}
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('Channel')}</FormLabel>
              <Combobox
                options={candidateOptions}
                value={field.value > 0 ? String(field.value) : ''}
                onValueChange={(value) => {
                  const channelID = Number(value)
                  const candidate = props.candidates.find(
                    (item) => item.id === channelID
                  )
                  field.onChange(channelID)
                  props.form.setValue(
                    `targets.${props.index}.channel_name`,
                    candidate?.name ?? ''
                  )
                }}
                placeholder={t('Select channel')}
                searchPlaceholder={t('Search channels...')}
                emptyText={t(
                  'No channels declare this group and canonical model'
                )}
                className='w-full'
              />
              {!props.candidatesLoading && props.candidates.length === 0 && (
                <p className='text-muted-foreground text-xs'>
                  {t('No channels declare this group and canonical model')}
                </p>
              )}
              <FormMessage />
            </FormItem>
          )}
        />

        <FormField
          control={props.form.control}
          name={`targets.${props.index}.target_priority`}
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('Target priority')}</FormLabel>
              <FormControl>
                <Input
                  type='number'
                  value={field.value}
                  onChange={(event) =>
                    field.onChange(numericValue(event.target.value))
                  }
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />

        <FormField
          control={props.form.control}
          name={`targets.${props.index}.name`}
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('Name')}</FormLabel>
              <FormControl>
                <Input {...field} />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />

        <FormField
          control={props.form.control}
          name={`targets.${props.index}.upstream_model`}
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('Upstream model')}</FormLabel>
              <FormControl>
                <Input className='font-mono text-xs' {...field} />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
      </div>

      <FormField
        control={props.form.control}
        name={`targets.${props.index}.enabled`}
        render={({ field }) => (
          <FormItem className='flex items-center justify-between gap-3'>
            <FormLabel>{t('Enabled')}</FormLabel>
            <FormControl>
              <Switch checked={field.value} onCheckedChange={field.onChange} />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />

      <FormField
        control={props.form.control}
        name={`targets.${props.index}.output_resolutions`}
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('Output resolutions')}</FormLabel>
            <div className='grid grid-cols-2 gap-2 sm:grid-cols-4'>
              {OUTPUT_RESOLUTIONS.map((resolution) => (
                <label
                  key={resolution}
                  className={cn(
                    'flex min-h-9 cursor-pointer items-center gap-2 rounded-md border px-3 py-2 text-sm transition-colors',
                    field.value.includes(resolution) &&
                      'border-primary bg-primary/10 text-primary dark:bg-primary/15'
                  )}
                >
                  <Checkbox
                    checked={field.value.includes(resolution)}
                    onCheckedChange={(checked) => {
                      const next = checked
                        ? [...field.value, resolution]
                        : field.value.filter((value) => value !== resolution)
                      field.onChange(next)
                    }}
                  />
                  <span>{resolution}</span>
                </label>
              ))}
            </div>
            <FormMessage />
          </FormItem>
        )}
      />

      <FormField
        control={props.form.control}
        name={`targets.${props.index}.durations.mode`}
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('Allowed generation durations')}</FormLabel>
            <FormControl>
              <ToggleGroup
                value={[field.value]}
                onValueChange={(values) => {
                  const next = values.find((value) => value !== field.value)
                  if (next === 'values' || next === 'range') {
                    setDurationMode(next)
                  }
                }}
                variant='outline'
                className='w-full'
              >
                <ToggleGroupItem
                  value='values'
                  className={cn('flex-1', ROUTING_SELECTED_CLASS)}
                >
                  {t('Discrete values')}
                </ToggleGroupItem>
                <ToggleGroupItem
                  value='range'
                  className={cn('flex-1', ROUTING_SELECTED_CLASS)}
                >
                  {t('Range')}
                </ToggleGroupItem>
              </ToggleGroup>
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />

      {target?.durations.mode === 'values' ? (
        <FormItem>
          <FormLabel>{t('Duration values')}</FormLabel>
          <div className='flex flex-wrap gap-2'>
            {durationValues.map((duration, durationIndex) => (
              <div
                key={duration}
                className='flex h-8 items-center rounded-md border'
              >
                <Input
                  type='number'
                  min={1}
                  max={MAX_TASK_DURATION_SECONDS}
                  value={duration}
                  onChange={(event) =>
                    updateDurationValue(
                      durationIndex,
                      numericValue(event.target.value)
                    )
                  }
                  aria-label={t('Duration')}
                  className='h-7 w-20 border-0 shadow-none'
                />
                <Button
                  type='button'
                  variant='ghost'
                  size='icon-xs'
                  aria-label={t('Delete duration')}
                  onClick={() => removeDurationValue(durationIndex)}
                >
                  <X aria-hidden='true' />
                </Button>
              </div>
            ))}
            <Button
              type='button'
              variant='outline'
              size='sm'
              onClick={addDurationValue}
            >
              <Plus data-icon='inline-start' aria-hidden='true' />
              {t('Add duration')}
            </Button>
          </div>
          <FormMessage />
        </FormItem>
      ) : (
        <div className='grid gap-4 sm:grid-cols-2'>
          {(['min', 'max'] as const).map((bound) => (
            <FormField
              key={bound}
              control={props.form.control}
              name={`targets.${props.index}.durations.${bound}`}
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    {t(bound === 'min' ? 'Minimum' : 'Maximum')}
                  </FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={1}
                      max={MAX_TASK_DURATION_SECONDS}
                      value={field.value ?? ''}
                      onChange={(event) =>
                        field.onChange(
                          event.target.value === ''
                            ? undefined
                            : Number(event.target.value)
                        )
                      }
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
          ))}
        </div>
      )}

      <FormField
        control={props.form.control}
        name={`targets.${props.index}.aspect_ratios`}
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('Aspect ratios')}</FormLabel>
            <FormControl>
              <ToggleGroup
                multiple
                value={field.value}
                onValueChange={field.onChange}
                variant='outline'
                spacing={1}
                className='flex w-full flex-wrap justify-start'
              >
                {ASPECT_RATIOS.map((ratio) => (
                  <ToggleGroupItem
                    key={ratio}
                    value={ratio}
                    className={ROUTING_SELECTED_CLASS}
                  >
                    {ratio}
                  </ToggleGroupItem>
                ))}
              </ToggleGroup>
            </FormControl>
            {field.value.length === 0 && (
              <p className='text-muted-foreground text-xs'>{t('Any ratio')}</p>
            )}
            <FormMessage />
          </FormItem>
        )}
      />

      <div className='grid gap-4 sm:grid-cols-3'>
        {(['images', 'videos', 'audios'] as const).map((kind) => {
          const maximum = kind === 'images' ? 9 : 3
          return (
            <FormField
              key={kind}
              control={props.form.control}
              name={`targets.${props.index}.reference_limits.${kind}`}
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t(`Reference ${kind}`)}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={0}
                      max={maximum}
                      value={field.value}
                      onChange={(event) =>
                        field.onChange(numericValue(event.target.value))
                      }
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
          )
        })}
      </div>

      <FormField
        control={props.form.control}
        name={`targets.${props.index}.supports_real_person`}
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('Require real person')}</FormLabel>
            <FormControl>
              <ToggleGroup
                value={[field.value]}
                onValueChange={(values) => {
                  const next = values.find((value) => value !== field.value)
                  if (next === 'unknown' || next === 'yes' || next === 'no') {
                    field.onChange(next)
                  }
                }}
                variant='outline'
                className='w-full'
              >
                <ToggleGroupItem
                  value='unknown'
                  className={cn('flex-1', ROUTING_SELECTED_CLASS)}
                >
                  {t('Unknown')}
                </ToggleGroupItem>
                <ToggleGroupItem
                  value='yes'
                  className={cn('flex-1', ROUTING_SELECTED_CLASS)}
                >
                  {t('Supported')}
                </ToggleGroupItem>
                <ToggleGroupItem
                  value='no'
                  className={cn('flex-1', ROUTING_SELECTED_CLASS)}
                >
                  {t('Not supported')}
                </ToggleGroupItem>
              </ToggleGroup>
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
    </div>
  )
}
