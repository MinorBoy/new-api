import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { cn } from '@/lib/utils'

import type { DocHeading } from '../lib/headings'

/**
 * Right-rail table of contents for the current page. Tracks which H2/H3 is in
 * view via IntersectionObserver and highlights it. Renders nothing when there
 * are fewer than two headings.
 */
export function OnThisPage({ headings }: { headings: DocHeading[] }) {
  const { t } = useTranslation()
  const [activeId, setActiveId] = useState<string>(headings[0]?.id ?? '')

  // Re-run when the heading list changes (page navigation).
  useEffect(() => {
    if (headings.length === 0) {
      return
    }
    setActiveId(headings[0]?.id ?? '')

    const elements = headings
      .map((h) => document.querySelector<HTMLElement>(`#${CSS.escape(h.id)}`))
      .filter((el): el is HTMLElement => el !== null)

    if (elements.length === 0) {
      return
    }

    const observer = new IntersectionObserver(
      (entries) => {
        // Pick the topmost intersecting heading to stay stable while scrolling.
        const visible = entries
          .filter((entry) => entry.isIntersecting)
          .sort((a, b) => a.boundingClientRect.top - b.boundingClientRect.top)
        if (visible[0]) {
          setActiveId(visible[0].target.id)
        }
      },
      { rootMargin: '-80px 0px -70% 0px', threshold: 0 }
    )

    elements.forEach((el) => observer.observe(el))
    return () => observer.disconnect()
  }, [headings])

  const items = useMemo(
    () => headings.filter((h) => h.level === 2 || h.level === 3),
    [headings]
  )

  if (items.length < 2) {
    return null
  }

  return (
    <nav aria-label={t('On this page')} className='space-y-2 text-sm'>
      <p className='text-muted-foreground px-3 text-xs font-semibold tracking-wide uppercase'>
        {t('On this page')}
      </p>
      <ul className='space-y-0.5 border-l'>
        {items.map((heading) => (
          <li key={heading.id}>
            <a
              href={`#${heading.id}`}
              className={cn(
                'hover:text-foreground block border-l-2 py-1 transition-colors',
                heading.level === 3 ? 'pl-6' : 'pl-3',
                activeId === heading.id
                  ? 'text-primary border-primary font-medium'
                  : 'text-muted-foreground border-transparent'
              )}
            >
              {heading.text}
            </a>
          </li>
        ))}
      </ul>
    </nav>
  )
}
