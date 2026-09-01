import { zodResolver } from '@hookform/resolvers/zod'
import { useEffect, useMemo, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'

import { JsonCodeEditor } from '@/components/json-code-editor'
import { Button } from '@/components/ui/button'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { api } from '@/lib/api'

import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'
import {
  getImagePreviewEndpointOptions,
  getImagePreviewOptions,
  normalizeImagePreviewSelection,
  type ImagePreviewSelection,
} from './image-preview-options'
import { parseImageRoutingPolicy } from './image-routing-policy'

type ImageSettingsCardProps = {
  catalog: string
  routing: string
}

const jsonObject = (message: string) =>
  z.string().superRefine((value, ctx) => {
    try {
      const parsed = JSON.parse(value)
      if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
        ctx.addIssue({ code: z.ZodIssueCode.custom, message })
      }
    } catch {
      ctx.addIssue({ code: z.ZodIssueCode.custom, message })
    }
  })

const imageSettingsSchema = z.object({
  catalog: jsonObject('Image catalog must be a JSON object'),
  routing: jsonObject('Image routing policy must be a JSON object'),
})

type ImageSettingsValues = z.infer<typeof imageSettingsSchema>

function pretty(value: string): string {
  try {
    return JSON.stringify(JSON.parse(value || '{}'), null, 2)
  } catch {
    return value || '{}'
  }
}

export function ImageSettingsCard({
  catalog,
  routing,
}: ImageSettingsCardProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const [preview, setPreview] = useState<{
    strategy?: string
    sku?: string
    selected_channel_id?: number
    candidates?: Array<{
      channel_id: number
      channel_name?: string
      estimated_cost_usd?: string
      estimated_revenue_usd?: string
      exclusion_reason?: string
    }>
  } | null>(null)
  const [previewLoading, setPreviewLoading] = useState(false)
  const [previewSelection, setPreviewSelection] =
    useState<ImagePreviewSelection>(() =>
      normalizeImagePreviewSelection(
        {
          group: 'default',
          model: 'gpt-image-1',
          endpoint: 'generations',
          size: '1024x1024',
          quality: 'medium',
          response_format: 'b64_json',
          n: 1,
        },
        catalog,
        routing
      )
    )
  const form = useForm<ImageSettingsValues>({
    resolver: zodResolver(imageSettingsSchema),
    defaultValues: { catalog: pretty(catalog), routing: pretty(routing) },
  })

  useEffect(() => {
    form.reset({ catalog: pretty(catalog), routing: pretty(routing) })
  }, [catalog, form, routing])

  const previewOptions = useMemo(
    () => getImagePreviewOptions(catalog, routing),
    [catalog, routing]
  )

  const selectedEndpointOptions = useMemo(
    () =>
      getImagePreviewEndpointOptions(
        catalog,
        previewSelection.model,
        previewSelection.endpoint
      ),
    [catalog, previewSelection.endpoint, previewSelection.model]
  )

  useEffect(() => {
    setPreviewSelection((current) =>
      normalizeImagePreviewSelection(current, catalog, routing)
    )
  }, [catalog, routing])

const save = async (values: ImageSettingsValues) => {
	const parsedCatalog = JSON.parse(values.catalog)
	const parsedRouting = parseImageRoutingPolicy(values.routing)
	await updateOption.mutateAsync({
		key: 'ImageModelCatalog',
		value: JSON.stringify(parsedCatalog),
	})
	await updateOption.mutateAsync({
		key: 'ImageRoutingPolicy',
		value: JSON.stringify(parsedRouting),
    })
  }

  const runPreview = async () => {
    setPreviewLoading(true)
    try {
      const response = await api.post(
        '/api/routing-policies/image/preview',
        previewSelection
      )
      const data = response.data
      if (!data.success) {
        throw new Error(data.message || t('Unable to preview image routing'))
      }
      setPreview(data.data)
    } catch (error) {
      toast.error(
        error instanceof Error
          ? error.message
          : t('Unable to preview image routing')
      )
    } finally {
      setPreviewLoading(false)
    }
  }

  return (
    <SettingsSection title={t('Image Models & Routing')}>
      <Form {...form}>
        <form onSubmit={form.handleSubmit(save)} className='space-y-6'>
          <FormField
            control={form.control}
            name='catalog'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Global image model catalog')}</FormLabel>
                <FormControl>
                  <JsonCodeEditor
                    value={field.value}
                    onChange={field.onChange}
                    name={field.name}
                    onBlur={field.onBlur}
                    textareaRef={field.ref}
                    disabled={updateOption.isPending}
                    heightClassName='h-72 min-h-72 max-h-[32rem]'
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'Configure public image models, capabilities, SKUs, and user sale prices.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name='routing'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Image routing policy')}</FormLabel>
                <FormControl>
                  <JsonCodeEditor
                    value={field.value}
                    onChange={field.onChange}
                    name={field.name}
                    onBlur={field.onBlur}
                    textareaRef={field.ref}
                    disabled={updateOption.isPending}
                    heightClassName='h-52 min-h-52 max-h-[28rem]'
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'Use manual, lowest_cost, or cost_weighted routing; cost_weighted keeps near-cheapest channels within a configurable tolerance.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
          <div className='flex flex-wrap items-center gap-2'>
            <Button type='submit' disabled={updateOption.isPending}>
              {t('Save image settings')}
            </Button>
            <Button
              type='button'
              variant='outline'
              onClick={runPreview}
              disabled={previewLoading}
            >
              {t('Preview image routing')}
            </Button>
          </div>
          <div className='grid gap-3 sm:grid-cols-2 lg:grid-cols-4'>
            <label className='flex flex-col gap-1 text-sm'>
              <span>{t('Group')}</span>
              <NativeSelect
                aria-label={t('Group')}
                value={previewSelection.group}
                onChange={(event) =>
                  setPreviewSelection((current) => ({
                    ...current,
                    group: event.target.value,
                  }))
                }
              >
                {previewOptions.groups.map((group) => (
                  <NativeSelectOption key={group} value={group}>
                    {group}
                  </NativeSelectOption>
                ))}
              </NativeSelect>
            </label>
            <label className='flex flex-col gap-1 text-sm'>
              <span>{t('Model')}</span>
              <NativeSelect
                aria-label={t('Model')}
                value={previewSelection.model}
                onChange={(event) =>
                  setPreviewSelection(
                    normalizeImagePreviewSelection(
                      { ...previewSelection, model: event.target.value },
                      catalog,
                      routing
                    )
                  )
                }
              >
                {previewOptions.models.map((model) => (
                  <NativeSelectOption key={model} value={model}>
                    {model}
                  </NativeSelectOption>
                ))}
              </NativeSelect>
            </label>
            <label className='flex flex-col gap-1 text-sm'>
              <span>{t('Endpoint')}</span>
              <NativeSelect
                aria-label={t('Endpoint')}
                value={previewSelection.endpoint}
                onChange={(event) =>
                  setPreviewSelection(
                    normalizeImagePreviewSelection(
                      { ...previewSelection, endpoint: event.target.value },
                      catalog,
                      routing
                    )
                  )
                }
              >
                {selectedEndpointOptions.endpoints.map((endpoint) => (
                  <NativeSelectOption key={endpoint} value={endpoint}>
                    {endpoint}
                  </NativeSelectOption>
                ))}
              </NativeSelect>
            </label>
            <label className='flex flex-col gap-1 text-sm'>
              <span>{t('Size')}</span>
              <NativeSelect
                aria-label={t('Size')}
                value={previewSelection.size}
                onChange={(event) =>
                  setPreviewSelection((current) => ({
                    ...current,
                    size: event.target.value,
                  }))
                }
              >
                {selectedEndpointOptions.sizes.map((size) => (
                  <NativeSelectOption key={size} value={size}>
                    {size}
                  </NativeSelectOption>
                ))}
              </NativeSelect>
            </label>
            <label className='flex flex-col gap-1 text-sm'>
              <span>{t('Quality')}</span>
              <NativeSelect
                aria-label={t('Quality')}
                value={previewSelection.quality}
                onChange={(event) =>
                  setPreviewSelection((current) => ({
                    ...current,
                    quality: event.target.value,
                  }))
                }
              >
                {selectedEndpointOptions.qualities.map((quality) => (
                  <NativeSelectOption key={quality} value={quality}>
                    {quality}
                  </NativeSelectOption>
                ))}
              </NativeSelect>
            </label>
            <label className='flex flex-col gap-1 text-sm'>
              <span>{t('Response format')}</span>
              <NativeSelect
                aria-label={t('Response format')}
                value={previewSelection.response_format}
                onChange={(event) =>
                  setPreviewSelection((current) => ({
                    ...current,
                    response_format: event.target.value,
                  }))
                }
              >
                {selectedEndpointOptions.responseFormats.map((format) => (
                  <NativeSelectOption key={format} value={format}>
                    {format}
                  </NativeSelectOption>
                ))}
              </NativeSelect>
            </label>
            <label className='flex flex-col gap-1 text-sm'>
              <span>{t('Image count')}</span>
              <input
                aria-label={t('Image count')}
                className='border-input h-8 rounded-lg border bg-transparent px-2 text-sm'
                type='number'
                min={1}
                max={10}
                value={previewSelection.n}
                onChange={(event) =>
                  setPreviewSelection((current) => ({
                    ...current,
                    n: Number(event.target.value) || 1,
                  }))
                }
              />
            </label>
          </div>
          {preview && (
            <div className='border-border/60 bg-muted/20 space-y-2 rounded-md border p-3 text-sm'>
              <div className='font-medium'>{t('Routing preview')}</div>
              <div className='text-muted-foreground'>
                {t('Strategy')}: {preview.strategy || '-'} · {t('SKU')}:{' '}
                {preview.sku || '-'} · {t('Selected channel')}:{' '}
                {preview.selected_channel_id ?? '-'}
              </div>
              {preview.candidates?.map((candidate) => (
                <div
                  key={candidate.channel_id}
                  className='flex flex-wrap gap-x-3 gap-y-1'
                >
                  <span>{candidate.channel_name || candidate.channel_id}</span>
                  <span className='text-muted-foreground'>
                    {candidate.estimated_cost_usd || '-'}
                  </span>
                  <span className='text-muted-foreground'>
                    {candidate.exclusion_reason || ''}
                  </span>
                </div>
              ))}
            </div>
          )}
        </form>
      </Form>
    </SettingsSection>
  )
}
