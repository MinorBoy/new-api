import { ArrowLeft01Icon, ArrowRight01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

const CATALOG_PAGE_SIZES = [25, 50, 100] as const

export function SupplierCostCatalogPagination(props: {
  page: number
  pageSize: 25 | 50 | 100
  total: number
  onPageChange: (page: number) => void
  onPageSizeChange: (size: 25 | 50 | 100) => void
}) {
  const { t } = useTranslation()
  const pageCount = Math.max(1, Math.ceil(props.total / props.pageSize))
  const pageSizeValues = CATALOG_PAGE_SIZES.map((size) => ({
    label: String(size),
    value: String(size),
  }))
  return (
    <div className='flex flex-wrap items-center justify-between gap-3 border-t pt-3 text-sm'>
      <span className='text-muted-foreground'>
        {t('{{count}} rows', { count: props.total })}
      </span>
      <div className='flex items-center gap-2'>
        <Select
          items={pageSizeValues}
          value={String(props.pageSize)}
          onValueChange={(value) => {
            const parsed = Number(value)
            if (parsed === 25 || parsed === 50 || parsed === 100) {
              props.onPageSizeChange(parsed)
            }
          }}
        >
          <SelectTrigger aria-label={t('Rows per page')}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent align='end'>
            <SelectGroup>
              {CATALOG_PAGE_SIZES.map((size) => (
                <SelectItem key={size} value={String(size)}>
                  {size}
                </SelectItem>
              ))}
            </SelectGroup>
          </SelectContent>
        </Select>
        <span className='min-w-24 text-center font-mono text-xs'>
          {props.page} / {pageCount}
        </span>
        <Button
          type='button'
          variant='outline'
          size='icon-sm'
          aria-label={t('Previous page')}
          disabled={props.page <= 1}
          onClick={() => props.onPageChange(props.page - 1)}
        >
          <HugeiconsIcon icon={ArrowLeft01Icon} strokeWidth={2} />
        </Button>
        <Button
          type='button'
          variant='outline'
          size='icon-sm'
          aria-label={t('Next page')}
          disabled={props.page >= pageCount}
          onClick={() => props.onPageChange(props.page + 1)}
        >
          <HugeiconsIcon icon={ArrowRight01Icon} strokeWidth={2} />
        </Button>
      </div>
    </div>
  )
}
