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
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { cn } from '@/lib/utils'

import { groupEndpointsByCategory } from '../lib/api-endpoints-helpers'
import { resolveDocLocale } from '../lib/resolve-doc'
import { docsNavGroups } from '../manifest'
import type { DocLocale } from '../types'

/**
 * The id of the nav group whose landing page is the reference catalog. When
 * rendering this group we expand the full endpoint list under the landing link.
 */
const REFERENCE_GROUP_ID = 'api-reference'

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
  // Endpoint sub-groups shown only inside the API Reference section.
  const endpointGroups = groupEndpointsByCategory()

  return (
    <nav aria-label={t('Documentation')} className='space-y-6 text-sm'>
      {docsNavGroups.map((group) => {
        const Icon = group.icon
        const isReferenceGroup = group.id === REFERENCE_GROUP_ID
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

            {/* Expand the full endpoint list under the API Reference group. */}
            {isReferenceGroup && (
              <ul className='mt-2 space-y-3 border-l pl-2'>
                {endpointGroups.map((endpointGroup) => (
                  <li key={endpointGroup.id} className='space-y-0.5'>
                    <div className='text-muted-foreground/70 px-2 text-[10px] font-semibold tracking-wide uppercase'>
                      {resolveDocLocale(endpointGroup.title, locale)}
                    </div>
                    {endpointGroup.endpoints.map((endpoint) => {
                      const endpointSlug = `reference/${endpoint.slug}`
                      const isActive = endpointSlug === activeSlug
                      return (
                        <Link
                          key={endpoint.slug}
                          to='/docs/$'
                          params={{ _splat: endpointSlug }}
                          onClick={onNavigate}
                          className={cn(
                            'flex items-center gap-1.5 rounded-md px-2 py-1 text-xs transition-colors',
                            isActive
                              ? 'bg-primary/10 text-primary font-medium'
                              : 'text-muted-foreground hover:text-foreground hover:bg-muted'
                          )}
                        >
                          <span className='font-mono text-[10px] font-bold opacity-70'>
                            {endpoint.method}
                          </span>
                          <span className='truncate'>
                            {resolveDocLocale(endpoint.title, locale)}
                          </span>
                        </Link>
                      )
                    })}
                  </li>
                ))}
              </ul>
            )}
          </div>
        )
      })}
    </nav>
  )
}
