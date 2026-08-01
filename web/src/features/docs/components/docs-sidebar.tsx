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
import type { DocLocale, HttpMethod } from '../types'

/**
 * The id of the nav group whose landing page is the reference catalog. When
 * rendering this group we expand the full endpoint list under the landing link.
 */
const REFERENCE_GROUP_ID = 'api-reference'

/** Compact semantic color for the HTTP verb in the endpoint list. */
const METHOD_TEXT_COLOR: Record<HttpMethod, string> = {
  GET: 'text-emerald-600 dark:text-emerald-400',
  POST: 'text-blue-600 dark:text-blue-400',
  DELETE: 'text-rose-600 dark:text-rose-400',
  PUT: 'text-amber-600 dark:text-amber-400',
  PATCH: 'text-violet-600 dark:text-violet-400',
}

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
    <nav aria-label={t('Documentation')} className='flex flex-col gap-7 text-sm'>
      {docsNavGroups.map((group) => {
        const Icon = group.icon
        const isReferenceGroup = group.id === REFERENCE_GROUP_ID
        return (
          <div key={group.id} className='flex flex-col gap-1.5'>
            <div className='text-muted-foreground/80 flex items-center gap-1.5 px-2 text-[11px] font-semibold tracking-wide'>
              <Icon className='size-3.5' />
              <span>{resolveDocLocale(group.title, locale)}</span>
            </div>

            <ul className='flex flex-col gap-px'>
              {group.pages.map((page) => {
                const isActive = page.slug === activeSlug
                return (
                  <li key={page.slug}>
                    <Link
                      to='/docs/$'
                      params={{ _splat: page.slug }}
                      onClick={onNavigate}
                      className={cn(
                        'group relative block rounded-md py-1.5 pr-2 pl-3 transition-colors',
                        isActive
                          ? 'text-primary font-medium'
                          : 'text-muted-foreground hover:text-foreground'
                      )}
                    >
                      {isActive && (
                        <span className='bg-primary absolute top-1/2 left-0 h-4 w-0.5 -translate-y-1/2 rounded-full' />
                      )}
                      <span
                        className={cn(
                          'rounded-md px-1 py-0.5 -ml-1 transition-colors',
                          isActive
                            ? 'bg-primary/10'
                            : 'group-hover:bg-muted'
                        )}
                      >
                        {resolveDocLocale(page.title, locale)}
                      </span>
                    </Link>
                  </li>
                )
              })}
            </ul>

            {/* Expand the full endpoint list under the API Reference group,
                separated from the guide links by a hairline divider. */}
            {isReferenceGroup && (
              <div className='border-border/60 mt-1 border-t pt-3'>
                <ul className='flex flex-col gap-3.5'>
                  {endpointGroups.map((endpointGroup) => (
                    <li key={endpointGroup.id} className='flex flex-col gap-px'>
                      <div className='text-muted-foreground/60 px-3 text-[10px] font-medium tracking-wide uppercase'>
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
                              'group relative flex items-center gap-2 rounded-md py-1 pr-2 pl-3 text-xs transition-colors',
                              isActive
                                ? 'text-primary font-medium'
                                : 'text-muted-foreground hover:text-foreground'
                            )}
                          >
                            {isActive && (
                              <span className='bg-primary absolute top-1/2 left-0 h-3.5 w-0.5 -translate-y-1/2 rounded-full' />
                            )}
                            <span
                              className={cn(
                                'font-mono text-[10px] font-bold w-9 shrink-0',
                                METHOD_TEXT_COLOR[endpoint.method]
                              )}
                            >
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
              </div>
            )}
          </div>
        )
      })}
    </nav>
  )
}
