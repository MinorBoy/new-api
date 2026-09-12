import { useMutation } from '@tanstack/react-query'
import { Check, Save } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

import { upsertImageCostMatrix } from '../api'
import {
  buildImageCostMatrix,
  IMAGE_COST_QUALITIES,
  IMAGE_COST_TIERS,
  type ImageCostQuality,
  type ImageCostTier,
} from '../lib/image-cost-matrix'
import type { CostRule } from '../types'

type ImageCostMatrixProps = {
  channelID: number
  billableModel: string
  endpoint: 'generations' | 'edits'
  rules: CostRule[]
  canWrite: boolean
  onSaved: () => Promise<void>
}

function ruleStatus(rule: CostRule | null, t: (key: string) => string) {
  if (!rule) return t('Not configured')
  return rule.status === 'active' ? t('Active') : t('Draft')
}

export function ImageCostMatrix(props: ImageCostMatrixProps) {
  const { t } = useTranslation()
  const cells = useMemo(
    () => buildImageCostMatrix(props.endpoint, props.rules),
    [props.endpoint, props.rules]
  )
  const [prices, setPrices] = useState<Record<string, string>>({})
  const [initialPrices, setInitialPrices] = useState<Record<string, string>>({})
  const [activate, setActivate] = useState(true)

  useEffect(() => {
    const next = Object.fromEntries(
      cells.map((cell) => [cell.key, cell.unitPrice])
    )
    setPrices(next)
    setInitialPrices(next)
  }, [cells])

  const saveMutation = useMutation({
    mutationFn: () => {
      const entries = cells
        .filter(
          (cell) => prices[cell.key]?.trim() !== initialPrices[cell.key]?.trim()
        )
        .filter((cell) => prices[cell.key]?.trim() !== '')
        .map((cell) => ({
          cost_variant_key: cell.key,
          unit_price: prices[cell.key].trim(),
        }))
      if (entries.length === 0) throw new Error(t('No image cost changes'))
      return upsertImageCostMatrix({
        channel_id: props.channelID,
        billable_upstream_model: props.billableModel,
        endpoint: props.endpoint,
        entries,
        activate,
      })
    },
    onSuccess: async () => {
      setInitialPrices({ ...prices })
      await props.onSaved()
      toast.success(
        t(
          activate
            ? 'Image costs saved and activated'
            : 'Image cost drafts saved'
        )
      )
    },
    onError: (error) => {
      toast.error(
        error instanceof Error ? error.message : t('Failed to save image costs')
      )
    },
  })

  const hasChanges = cells.some(
    (cell) => prices[cell.key]?.trim() !== initialPrices[cell.key]?.trim()
  )
  const cellByTier = (tier: ImageCostTier, quality: ImageCostQuality) =>
    cells.find((cell) => cell.tier === tier && cell.quality === quality)

  return (
    <section className='flex flex-col gap-3 rounded-md border p-4'>
      <div className='flex flex-wrap items-start justify-between gap-3'>
        <div>
          <h3 className='text-sm font-semibold'>
            {t('Image supplier cost matrix')}
          </h3>
          <p className='text-muted-foreground text-xs'>
            {props.billableModel} ·{' '}
            {props.endpoint === 'edits' ? t('Edits') : t('Generations')} ·{' '}
            {t('USD per image')}
          </p>
        </div>
        {props.canWrite ? (
          <div className='flex items-center gap-3'>
            <label className='text-muted-foreground flex items-center gap-2 text-xs'>
              <Checkbox
                checked={activate}
                onCheckedChange={(checked) => setActivate(checked === true)}
              />
              {t('Activate after saving')}
            </label>
            <Button
              type='button'
              size='sm'
              disabled={!hasChanges || saveMutation.isPending}
              onClick={() => saveMutation.mutate()}
            >
              <Save data-icon='inline-start' aria-hidden='true' />
              {t('Save matrix')}
            </Button>
          </div>
        ) : null}
      </div>
      <div className='overflow-x-auto rounded-md border'>
        <Table className='min-w-[560px]'>
          <TableHeader>
            <TableRow>
              <TableHead>{t('Resolution tier')}</TableHead>
              {IMAGE_COST_QUALITIES.map((quality) => (
                <TableHead key={quality}>{quality}</TableHead>
              ))}
            </TableRow>
          </TableHeader>
          <TableBody>
            {IMAGE_COST_TIERS.map((tier) => (
              <TableRow key={tier}>
                <TableHead className='font-mono text-xs'>{tier}</TableHead>
                {IMAGE_COST_QUALITIES.map((quality) => {
                  const cell = cellByTier(tier, quality)
                  if (!cell) return <TableCell key={quality}>-</TableCell>
                  return (
                    <TableCell key={quality} className='min-w-40'>
                      <div className='flex flex-col gap-1'>
                        <Input
                          aria-label={`${tier} ${quality}`}
                          inputMode='decimal'
                          placeholder='USD'
                          value={prices[cell.key] ?? ''}
                          disabled={!props.canWrite || saveMutation.isPending}
                          onChange={(event) =>
                            setPrices((current) => ({
                              ...current,
                              [cell.key]: event.target.value,
                            }))
                          }
                        />
                        <div className='flex items-center gap-1 text-[11px]'>
                          <Badge
                            variant={
                              cell.rule?.status === 'active'
                                ? 'default'
                                : 'secondary'
                            }
                          >
                            {ruleStatus(cell.rule, t)}
                          </Badge>
                          {cell.rule ? (
                            <Check
                              className='size-3 text-emerald-600'
                              aria-hidden='true'
                            />
                          ) : null}
                        </div>
                      </div>
                    </TableCell>
                  )
                })}
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
      <p className='text-muted-foreground text-xs'>
        {t(
          'Leave a cell unchanged to keep its current rule. Empty new cells are ignored.'
        )}
      </p>
    </section>
  )
}
