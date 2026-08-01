import { Link } from '@tanstack/react-router'
import { ChevronLeft, ChevronRight } from 'lucide-react'
import { useEffect } from 'react'
import { useTranslation } from 'react-i18next'

import { DocsMarkdown } from '../lib/docs-markdown'
import { getDocNeighbors } from '../lib/resolve-doc'
import type { DocLocale, ResolvedDoc } from '../types'

/**
 * Reset the scroll position to the top whenever the page slug changes, so
 * navigating between docs doesn't carry over the previous scroll offset.
 */
function useScrollResetOnNavigate(slug: string) {
  useEffect(() => {
    if (typeof window === 'undefined') return
    // Defer until after the new content paints.
    requestAnimationFrame(() => window.scrollTo({ top: 0 }))
  }, [slug])
}

export function DocsContent({
  doc,
  locale,
}: {
  doc: ResolvedDoc
  locale: DocLocale
}) {
  const { t } = useTranslation()
  useScrollResetOnNavigate(doc.slug)
  const { prev, next } = getDocNeighbors(doc.slug, locale)

  return (
    <article className='mx-auto w-full max-w-3xl min-w-0 px-4 py-8 sm:px-6 lg:px-8'>
      <DocsMarkdown source={doc.body} />

      <nav
        aria-label={t('Page navigation')}
        className='border-border/60 mt-12 flex items-center justify-between gap-4 border-t pt-6'
      >
        {prev ? (
          <Link
            to='/docs/$'
            params={{ _splat: prev.slug }}
            className='hover:bg-muted group flex min-w-0 flex-1 flex-col rounded-lg border p-3 transition-colors'
          >
            <span className='text-muted-foreground flex items-center gap-1 text-xs'>
              <ChevronLeft className='size-3.5' />
              {t('Previous')}
            </span>
            <span className='text-foreground truncate text-sm font-medium'>
              {prev.title}
            </span>
          </Link>
        ) : (
          <span className='flex-1' />
        )}
        {next ? (
          <Link
            to='/docs/$'
            params={{ _splat: next.slug }}
            className='hover:bg-muted group flex min-w-0 flex-1 flex-col items-end rounded-lg border p-3 text-right transition-colors'
          >
            <span className='text-muted-foreground flex items-center justify-end gap-1 text-xs'>
              {t('Next')}
              <ChevronRight className='size-3.5' />
            </span>
            <span className='text-foreground truncate text-sm font-medium'>
              {next.title}
            </span>
          </Link>
        ) : (
          <span className='flex-1' />
        )}
      </nav>
    </article>
  )
}
