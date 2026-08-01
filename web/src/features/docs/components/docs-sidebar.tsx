import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { cn } from '@/lib/utils'

import { resolveDocLocale } from '../lib/resolve-doc'
import { docsNavGroups } from '../manifest'
import type { DocLocale } from '../types'

export function DocsSidebar({
  activeSlug,
  locale,
  onNavigate,
}: {
  activeSlug: string
  locale: DocLocale
  onNavigate?: () => void
}) {
  const { t } = useTranslation()

  return (
    <nav aria-label={t('Documentation')} className='space-y-6 text-sm'>
      {docsNavGroups.map((group) => {
        const Icon = group.icon
        return (
          <div key={group.id} className='space-y-1'>
            <div className='text-muted-foreground flex items-center gap-2 px-2 text-xs font-semibold tracking-wide uppercase'>
              <Icon className='size-3.5' />
              <span>{resolveDocLocale(group.title, locale)}</span>
            </div>
            <ul className='space-y-0.5'>
              {group.pages.map((page) => {
                const isActive = page.slug === activeSlug
                return (
                  <li key={page.slug}>
                    <Link
                      to='/docs/$'
                      params={{ _splat: page.slug }}
                      onClick={onNavigate}
                      className={cn(
                        'block rounded-md px-2 py-1.5 transition-colors',
                        isActive
                          ? 'bg-primary/10 text-primary font-medium'
                          : 'text-muted-foreground hover:text-foreground hover:bg-muted'
                      )}
                    >
                      {resolveDocLocale(page.title, locale)}
                    </Link>
                  </li>
                )
              })}
            </ul>
          </div>
        )
      })}
    </nav>
  )
}
