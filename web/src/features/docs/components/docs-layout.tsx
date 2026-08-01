import { PublicLayout } from '@/components/layout'

import type { DocHeading } from '../lib/headings'
import { docsNavGroups } from '../manifest'
import type { DocLocale, ResolvedDoc } from '../types'
import { DocsCategoryTabs } from './docs-category-tabs'
import { DocsContent } from './docs-content'
import { DocsMobileNav } from './docs-mobile-nav'
import { DocsSearch } from './docs-search'
import { DocsSidebar } from './docs-sidebar'
import { OnThisPage } from './on-this-page'

/**
 * Three-column documentation shell:
 *   - top:    category switcher (sticky under the public header)
 *   - left:   docs sidebar (sticky on desktop, Sheet on mobile)
 *   - center: reading column with prev/next nav
 *   - right:  on-this-page outline (desktop only)
 *
 * Wrapped in `<PublicLayout showMainContainer={false}>` so the docs area owns
 * its full-bleed layout while still inheriting the site header, theme switch,
 * and language switcher.
 */
export function DocsLayout({
  doc,
  headings,
  locale,
}: {
  doc: ResolvedDoc
  headings: DocHeading[]
  locale: DocLocale
}) {
  const activeGroup = docsNavGroups.find((group) =>
    group.pages.some((page) => page.slug === doc.slug)
  )

  return (
    <PublicLayout showMainContainer={false}>
      <DocsCategoryTabs
        activeGroupId={activeGroup?.id ?? null}
        locale={locale}
      />

      <div className='mx-auto flex max-w-screen-2xl gap-0 lg:gap-8'>
        {/* Left rail */}
        <aside className='hidden w-64 shrink-0 lg:block'>
          <div className='sticky top-32 max-h-[calc(100vh-9rem)] overflow-y-auto py-8 pr-4'>
            <DocsSearch />
            <div className='mt-6'>
              <DocsSidebar activeSlug={doc.slug} locale={locale} />
            </div>
          </div>
        </aside>

        {/* Center column */}
        <div className='min-w-0 flex-1'>
          {/* Mobile nav trigger — visible only on small screens */}
          <div className='flex items-center gap-2 px-4 py-3 lg:hidden'>
            <DocsMobileNav activeSlug={doc.slug} locale={locale} />
          </div>
          <DocsContent doc={doc} locale={locale} />
        </div>

        {/* Right rail */}
        <aside className='hidden w-56 shrink-0 xl:block'>
          <div className='sticky top-32 max-h-[calc(100vh-9rem)] overflow-y-auto py-8'>
            <OnThisPage headings={headings} />
          </div>
        </aside>
      </div>
    </PublicLayout>
  )
}
