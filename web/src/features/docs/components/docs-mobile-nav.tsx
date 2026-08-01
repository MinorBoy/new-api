import { Menu } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from '@/components/ui/sheet'

import type { DocLocale } from '../types'
import { DocsSidebar } from './docs-sidebar'

/**
 * Mobile-only sidebar trigger. Opens a left Sheet containing the full docs
 * navigation tree; selecting a link closes the sheet.
 */
export function DocsMobileNav({
  activeSlug,
  locale,
}: {
  activeSlug: string
  locale: DocLocale
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)

  return (
    <Sheet open={open} onOpenChange={setOpen}>
      <SheetTrigger
        render={
          <Button
            variant='outline'
            size='icon'
            aria-label={t('Open navigation')}
          >
            <Menu className='size-5' />
          </Button>
        }
      />
      <SheetContent side='left' className='w-72 p-0'>
        <SheetHeader className='border-b'>
          <SheetTitle>{t('Documentation')}</SheetTitle>
        </SheetHeader>
        <div className='overflow-y-auto p-4'>
          <DocsSidebar
            activeSlug={activeSlug}
            locale={locale}
            onNavigate={() => setOpen(false)}
          />
        </div>
      </SheetContent>
    </Sheet>
  )
}
