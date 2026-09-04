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
import { useQuery } from '@tanstack/react-query'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { JsonCodeEditor } from '@/components/json-code-editor'
import {
  Accordion,
  AccordionContent,
  AccordionItem,
  AccordionTrigger,
} from '@/components/ui/accordion'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { api } from '@/lib/api'

import { FormNavigationGuard } from '../components/form-navigation-guard'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'
import {
  getImagePreviewEndpointOptions,
  getImagePreviewOptions,
  normalizeImagePreviewSelection,
  type ImagePreviewSelection,
} from './image-preview-options'
import { getImagePricingCostSummary } from './image-pricing-api'
import {
  calculateExpectedMarginBps,
  flattenImagePricingCatalog,
  formatImagePrice,
  updateCatalogSalePrices,
  validateSalePrice,
  type ImageCatalog,
  type ImagePricingRow,
} from './image-pricing-catalog'
import { countUnsavedImagePricingChanges } from './image-pricing-workbench-utils'
import { parseImageRoutingPolicy } from './image-routing-policy'

type ImagePricingWorkbenchProps = { catalog: string; routing: string }

type Preview = {
  strategy?: string
  sku?: string
  selected_channel_id?: number
  candidates?: Array<{
    channel_id: number
    channel_name?: string
    estimated_cost_usd?: string
    exclusion_reason?: string
  }>
}

function pretty(value: string): string {
  try {
    return JSON.stringify(JSON.parse(value || '{}'), null, 2)
  } catch {
    return value || '{}'
  }
}

function rowWithCost(
  row: ImagePricingRow,
  costs: Map<string, { cost?: string; sourceCount: number }>
): ImagePricingRow {
  const cost = costs.get(row.id)
  return {
    ...row,
    upstreamCostUSD: cost?.cost,
    sourceCount: cost?.sourceCount ?? 0,
    expectedMarginBps: calculateExpectedMarginBps(row.salePriceUSD, cost?.cost),
  }
}

export function ImagePricingWorkbench(props: ImagePricingWorkbenchProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const [catalogText, setCatalogText] = useState(() => pretty(props.catalog))
  const [routingText, setRoutingText] = useState(() => pretty(props.routing))
  const [editedPrices, setEditedPrices] = useState<Record<string, string>>({})
  const [savedCatalogText, setSavedCatalogText] = useState(() =>
    pretty(props.catalog)
  )
  const [savedRoutingText, setSavedRoutingText] = useState(() =>
    pretty(props.routing)
  )
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({})
  const [preview, setPreview] = useState<Preview | null>(null)
  const [previewLoading, setPreviewLoading] = useState(false)
  const [advancedOpen, setAdvancedOpen] = useState<string[]>([])
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
        props.catalog,
        props.routing
      )
    )

  useEffect(() => {
    setCatalogText(pretty(props.catalog))
    setRoutingText(pretty(props.routing))
    setSavedCatalogText(pretty(props.catalog))
    setSavedRoutingText(pretty(props.routing))
    setEditedPrices({})
    setFieldErrors({})
  }, [props.catalog, props.routing])

  const parsedRows = useMemo(() => {
    try {
      return flattenImagePricingCatalog(catalogText)
    } catch {
      return []
    }
  }, [catalogText])
  const modelNames = useMemo(
    () => [...new Set(parsedRows.map((row) => row.model))],
    [parsedRows]
  )
  const costQuery = useQuery({
    queryKey: ['image-pricing-costs', modelNames],
    queryFn: () => getImagePricingCostSummary(modelNames),
    enabled: modelNames.length > 0,
    staleTime: 60_000,
  })
  const costMap = useMemo(() => {
    const map = new Map<string, { cost?: string; sourceCount: number }>()
    for (const item of costQuery.data?.data.items ?? []) {
      map.set(`${item.model}|${item.sku}`, {
        cost: item.known ? item.minimum_cost_usd : undefined,
        sourceCount: item.source_count,
      })
    }
    return map
  }, [costQuery.data])
  const rows = useMemo(
    () =>
      parsedRows.map((row) =>
        rowWithCost(
          { ...row, salePriceUSD: editedPrices[row.id] ?? row.salePriceUSD },
          costMap
        )
      ),
    [costMap, editedPrices, parsedRows]
  )
  const savedPrices = useMemo(
    () =>
      Object.fromEntries(parsedRows.map((row) => [row.id, row.salePriceUSD])),
    [parsedRows]
  )
  const unsavedCount = countUnsavedImagePricingChanges(
    editedPrices,
    savedPrices
  )
  const advancedUnsavedCount =
    Number(pretty(catalogText) !== pretty(savedCatalogText)) +
    Number(pretty(routingText) !== pretty(savedRoutingText))
  const totalUnsavedCount = unsavedCount + advancedUnsavedCount
  const isDirty = totalUnsavedCount > 0
  const previewOptions = useMemo(
    () => getImagePreviewOptions(catalogText, routingText),
    [catalogText, routingText]
  )
  const endpointOptions = useMemo(
    () =>
      getImagePreviewEndpointOptions(
        catalogText,
        previewSelection.model,
        previewSelection.endpoint
      ),
    [catalogText, previewSelection.endpoint, previewSelection.model]
  )

  const beginEdit = (row: ImagePricingRow) => {
    setEditedPrices((current) => ({
      ...current,
      [row.id]: current[row.id] ?? row.salePriceUSD,
    }))
  }
  const commitEdit = (row: ImagePricingRow) => {
    const value = editedPrices[row.id] ?? row.salePriceUSD
    const error = validateSalePrice(value)
    setFieldErrors((current) => ({ ...current, [row.id]: error ?? '' }))
    if (!error) {
      setEditedPrices((current) => ({ ...current, [row.id]: value.trim() }))
    }
  }
  const cancelEdit = (row: ImagePricingRow) => {
    setEditedPrices((current) => {
      const next = { ...current }
      delete next[row.id]
      return next
    })
    setFieldErrors((current) => ({ ...current, [row.id]: '' }))
  }

  const savePrices = async () => {
    const errors: Record<string, string> = {}
    for (const [id, value] of Object.entries(editedPrices)) {
      const error = validateSalePrice(value)
      if (error) errors[id] = error
    }
    if (Object.keys(errors).length > 0) {
      setFieldErrors(errors)
      return
    }
    try {
      const updated = updateCatalogSalePrices(
        JSON.parse(catalogText) as ImageCatalog,
        editedPrices
      )
      const response = await updateOption.mutateAsync({
        key: 'ImageModelCatalog',
        value: JSON.stringify(updated),
      })
      if (!response.success) throw new Error(response.message)
      setCatalogText(pretty(JSON.stringify(updated)))
      setSavedCatalogText(pretty(JSON.stringify(updated)))
      setEditedPrices({})
      setFieldErrors({})
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('Failed to update setting')
      )
    }
  }

  const saveAdvanced = async () => {
    try {
      const parsedCatalog = JSON.parse(catalogText)
      const parsedRouting = parseImageRoutingPolicy(routingText)
      await updateOption.mutateAsync({
        key: 'ImageModelCatalog',
        value: JSON.stringify(parsedCatalog),
      })
      await updateOption.mutateAsync({
        key: 'ImageRoutingPolicy',
        value: JSON.stringify(parsedRouting),
      })
      setSavedCatalogText(pretty(JSON.stringify(parsedCatalog)))
      setSavedRoutingText(pretty(JSON.stringify(parsedRouting)))
      setCatalogText(pretty(JSON.stringify(parsedCatalog)))
      setRoutingText(pretty(JSON.stringify(parsedRouting)))
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('Failed to update setting')
      )
    }
  }

  const runPreview = async () => {
    setPreviewLoading(true)
    try {
      const response = await api.post(
        '/api/routing-policies/image/preview',
        previewSelection
      )
      if (!response.data.success) {
        throw new Error(
          response.data.message || t('Unable to preview image routing')
        )
      }
      setPreview(response.data.data)
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
      <FormNavigationGuard when={isDirty} />
      <div className='flex flex-wrap items-center justify-between gap-3 rounded-md border p-3'>
        <div className='flex flex-wrap gap-4 text-sm'>
          <span>
            {t('SKU count')}: {rows.length}
          </span>
          <span>
            {t('Enabled SKUs')}: {rows.filter((row) => row.enabled).length}
          </span>
          <span>
            {t('Unsaved changes')}: {totalUnsavedCount}
          </span>
        </div>
        <div className='flex gap-2'>
          <Button
            type='button'
            variant='outline'
            onClick={() => window.location.reload()}
            disabled={isDirty}
          >
            {t('Refresh')}
          </Button>
          <Button
            type='button'
            onClick={savePrices}
            disabled={unsavedCount === 0 || updateOption.isPending}
          >
            {t('Save changes')}
          </Button>
        </div>
      </div>
      {costQuery.isError && (
        <p className='text-destructive text-sm'>
          {t('Unable to load image pricing costs')}
        </p>
      )}
      {parsedRows.length === 0 ? (
        <p className='text-destructive text-sm'>
          {t('Image catalog must be valid JSON')}
        </p>
      ) : (
        <div className='w-full overflow-x-auto rounded-md border'>
          <table className='w-full min-w-[900px] text-sm'>
            <thead>
              <tr className='bg-muted/30 border-b text-left'>
                {[
                  t('Model'),
                  t('Endpoint'),
                  t('Resolution tier'),
                  t('Quality'),
                  t('Upstream cost'),
                  t('Sale price'),
                  t('Expected margin'),
                  t('Enabled'),
                ].map((heading) => (
                  <th key={heading} className='px-3 py-2 font-medium'>
                    {heading}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {rows.map((row) => {
                const value = editedPrices[row.id] ?? row.salePriceUSD
                const error = fieldErrors[row.id]
                return (
                  <tr key={row.id} className='border-b last:border-0'>
                    <td className='px-3 py-2 font-medium'>{row.model}</td>
                    <td className='px-3 py-2'>{row.endpoint}</td>
                    <td className='px-3 py-2 font-mono'>{row.tier || '-'}</td>
                    <td className='px-3 py-2'>{row.quality}</td>
                    <td className='px-3 py-2'>
                      {row.upstreamCostUSD
                        ? `$${formatImagePrice(row.upstreamCostUSD)}`
                        : t('Not configured')}
                    </td>
                    <td className='px-3 py-2'>
                      <Input
                        aria-label={`${row.model} ${row.skuKey} sale price`}
                        value={value}
                        className='w-32'
                        aria-invalid={Boolean(error)}
                        disabled={updateOption.isPending}
                        onFocus={() => beginEdit(row)}
                        onChange={(event) =>
                          setEditedPrices((current) => ({
                            ...current,
                            [row.id]: event.target.value,
                          }))
                        }
                        onBlur={() => commitEdit(row)}
                        onKeyDown={(event) => {
                          if (event.key === 'Enter') {
                            event.preventDefault()
                            commitEdit(row)
                          }
                          if (event.key === 'Escape') {
                            event.preventDefault()
                            cancelEdit(row)
                          }
                        }}
                      />
                      {!row.configured && !error && (
                        <span className='text-muted-foreground block text-xs'>
                          {t('Not configured')}
                        </span>
                      )}
                      {error && (
                        <span className='text-destructive text-xs'>
                          {t(error)}
                        </span>
                      )}
                    </td>
                    <td className='px-3 py-2'>
                      {row.expectedMarginBps == null
                        ? t('Unavailable')
                        : `${(row.expectedMarginBps / 100).toFixed(2)}%`}
                    </td>
                    <td className='px-3 py-2'>
                      {row.enabled ? t('Enabled') : t('Disabled')}
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      )}
      <Accordion
        multiple
        value={advancedOpen}
        onValueChange={(value) => setAdvancedOpen(value)}
        className='rounded-md border px-3'
      >
        <AccordionItem value='advanced'>
          <AccordionTrigger>{t('Advanced settings')}</AccordionTrigger>
          <AccordionContent className='space-y-4'>
            <div>
              <label className='mb-2 block text-sm font-medium'>
                {t('Catalog JSON')}
              </label>
              <JsonCodeEditor
                value={catalogText}
                onChange={setCatalogText}
                name='image-catalog'
                heightClassName='h-72 min-h-72'
              />
            </div>
            <div>
              <label className='mb-2 block text-sm font-medium'>
                {t('Routing & acceptance')}
              </label>
              <JsonCodeEditor
                value={routingText}
                onChange={setRoutingText}
                name='image-routing'
                heightClassName='h-52 min-h-52'
              />
            </div>
            <div className='flex flex-wrap gap-2'>
              <Button
                type='button'
                onClick={saveAdvanced}
                disabled={updateOption.isPending}
              >
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
                        catalogText,
                        routingText
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
                        catalogText,
                        routingText
                      )
                    )
                  }
                >
                  {endpointOptions.endpoints.map((endpoint) => (
                    <NativeSelectOption key={endpoint} value={endpoint}>
                      {endpoint}
                    </NativeSelectOption>
                  ))}
                </NativeSelect>
              </label>
              <label className='flex flex-col gap-1 text-sm'>
                <span>{t('Size')}</span>
                <Input
                  aria-label={t('Size')}
                  value={previewSelection.size}
                  placeholder={t('auto or widthxheight')}
                  onChange={(event) =>
                    setPreviewSelection((current) => ({
                      ...current,
                      size: event.target.value,
                    }))
                  }
                />
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
                  {endpointOptions.qualities.map((quality) => (
                    <NativeSelectOption key={quality} value={quality}>
                      {quality}
                    </NativeSelectOption>
                  ))}
                </NativeSelect>
              </label>
            </div>
            {preview && (
              <div className='space-y-2 rounded-md border p-3 text-sm'>
                <div className='font-medium'>{t('Routing preview')}</div>
                <div>
                  {t('Strategy')}: {preview.strategy || '-'} · {t('SKU')}:{' '}
                  {preview.sku || '-'} · {t('Selected channel')}:{' '}
                  {preview.selected_channel_id ?? '-'}
                </div>
                {preview.candidates?.map((candidate) => (
                  <div
                    key={candidate.channel_id}
                    className='flex flex-wrap gap-3'
                  >
                    <span>
                      {candidate.channel_name || candidate.channel_id}
                    </span>
                    <span>{candidate.estimated_cost_usd || '-'}</span>
                    <span>{candidate.exclusion_reason || ''}</span>
                  </div>
                ))}
              </div>
            )}
          </AccordionContent>
        </AccordionItem>
      </Accordion>
    </SettingsSection>
  )
}
