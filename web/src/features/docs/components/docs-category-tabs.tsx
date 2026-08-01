import { Link } from '@tanstack/react-router'

import { cn } from '@/lib/utils'

import { resolveDocLocale } from '../lib/resolve-doc'
import { docsNavGroups } from '../manifest'
import type { DocLocale } from '../types'

/**
 * Horizontal category switcher pinned under the header. Selecting a category
 * navigates to its first page; the category containing the active page is
 * highlighted. Mirrors the top-level tabs on reference docs sites.
 */
export function DocsCategoryTabs({
  activeGroupId,
  locale,
}: {
  activeGroupId: string | null
  locale: DocLocale
}) {
  return (
    <div className='border-border/60 bg-background/95 supports-backdrop-filter:bg-background/80 sticky top-16 z-30 border-b backdrop-blur'>
      <div className='mx-auto flex max-w-screen-2xl items-center gap-1 overflow-x-auto px-4 py-2'>
        {docsNavGroups.map((group) => {
          const firstPage = group.pages[0]
          if (!firstPage) {
            return null
          }
          const isActive = group.id === activeGroupId
          return (
            <Link
              key={group.id}
              to='/docs/$'
              params={{ _splat: firstPage.slug }}
              className={cn(
                'rounded-full px-3 py-1.5 text-sm font-medium whitespace-nowrap transition-colors',
                isActive
                  ? 'bg-primary text-primary-foreground'
                  : 'text-muted-foreground hover:text-foreground hover:bg-muted'
              )}
            >
              {resolveDocLocale(group.title, locale)}
            </Link>
          )
        })}
      </div>
    </div>
  )
}
