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
import { PublicLayout } from '@/components/layout'

import type { DocHeading } from '../lib/headings'
import type { DocLocale, ResolvedDoc } from '../types'
import { DocsContent } from './docs-content'
import { DocsMobileNav } from './docs-mobile-nav'
import { DocsSidebar } from './docs-sidebar'
import { OnThisPage } from './on-this-page'

type DocsLayoutProps = {
  locale: DocLocale
} & (
  | {
      /** Guide page to render in the reading column with prev/next nav. */
      doc: ResolvedDoc
      headings: DocHeading[]
      children?: never
    }
  | {
      /** Custom content for the reference catalog / endpoint detail pages. */
      children: React.ReactNode
      doc?: never
      headings?: never
    }
)

/**
 * Two-column documentation shell:
 *   - left:   docs sidebar (sticky on desktop, Sheet on mobile)
 *   - center: reading column (guide Markdown OR custom reference content)
 *   - right:  on-this-page outline (guide pages only, desktop)
 *
 * Wrapped in `<PublicLayout showMainContainer={false}>` so the docs area owns
 * its full-bleed layout while still inheriting the site header, theme switch,
 * and language switcher.
 */
export function DocsLayout({ doc, headings, locale, children }: DocsLayoutProps) {
  // The active sidebar slug: guide pages use the doc slug; reference/endpoint
  // pages highlight the reference catalog entry.
  const activeSlug = doc?.slug ?? 'reference'

  return (
    <PublicLayout showMainContainer={false}>
      <div className='mx-auto flex max-w-screen-2xl gap-0 lg:gap-8'>
        {/* Left rail */}
        <aside className='hidden w-64 shrink-0 lg:block'>
          <div className='sticky top-16 max-h-[calc(100vh-5rem)] overflow-y-auto py-8 pr-4'>
            <div className='mt-6'>
              <DocsSidebar activeSlug={activeSlug} locale={locale} />
            </div>
          </div>
        </aside>

        {/* Center column */}
        <div className='min-w-0 flex-1'>
          {/* Mobile nav trigger — visible only on small screens */}
          <div className='flex items-center gap-2 px-4 py-3 lg:hidden'>
            <DocsMobileNav activeSlug={activeSlug} locale={locale} />
          </div>
          {doc ? (
            <DocsContent doc={doc} locale={locale} />
          ) : (
            children
          )}
        </div>

        {/* Right rail — on-this-page only makes sense for guide pages */}
        {doc && headings && headings.length > 0 && (
          <aside className='hidden w-56 shrink-0 xl:block'>
            <div className='sticky top-16 max-h-[calc(100vh-5rem)] overflow-y-auto py-8'>
              <OnThisPage headings={headings} />
            </div>
          </aside>
        )}
      </div>
    </PublicLayout>
  )
}
