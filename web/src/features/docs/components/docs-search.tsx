import { useNavigate } from '@tanstack/react-router'
import { Search } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  CommandDialog,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from '@/components/ui/command'

import { resolveDocLocale } from '../lib/resolve-doc'
import { useDocLocale } from '../lib/use-doc-locale'
import { flatDocEntries } from '../manifest'

/**
 * Docs-scoped search trigger. Opens a command dialog listing every page grouped
 * by category; cmdk filters by title as the user types. Selecting an item
 * navigates to that doc.
 */
export function DocsSearch() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [open, setOpen] = useState(false)
  const locale = useDocLocale()

  const groups = useMemo(
    () =>
      flatDocEntries.reduce<
        Array<{
          groupId: string
          groupTitle: string
          items: typeof flatDocEntries
        }>
      >((acc, entry) => {
        let bucket = acc.find((g) => g.groupId === entry.group.id)
        if (!bucket) {
          bucket = {
            groupId: entry.group.id,
            groupTitle: resolveDocLocale(entry.group.title, locale),
            items: [],
          }
          acc.push(bucket)
        }
        bucket.items.push(entry)
        return acc
      }, []),
    [locale]
  )

  const goTo = (slug: string) => {
    setOpen(false)
    void navigate({ to: '/docs/$', params: { _splat: slug } })
  }

  return (
    <>
      <Button
        variant='outline'
        className='text-muted-foreground w-full justify-start gap-2'
        onClick={() => setOpen(true)}
      >
        <Search className='size-4' />
        <span>{t('Search docs')}</span>
      </Button>
      <CommandDialog open={open} onOpenChange={setOpen}>
        <CommandInput placeholder={t('Search documentation')} />
        <CommandList>
          <CommandEmpty>{t('No results found.')}</CommandEmpty>
          {groups.map((group) => (
            <CommandGroup key={group.groupId} heading={group.groupTitle}>
              {group.items.map(({ page }) => (
                <CommandItem
                  key={page.slug}
                  value={`${resolveDocLocale(page.title, locale)} ${page.slug}`}
                  onSelect={() => goTo(page.slug)}
                >
                  {resolveDocLocale(page.title, locale)}
                </CommandItem>
              ))}
            </CommandGroup>
          ))}
        </CommandList>
      </CommandDialog>
    </>
  )
}
