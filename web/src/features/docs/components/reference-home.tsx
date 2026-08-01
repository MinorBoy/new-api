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
import { KeyRound, Link2, Search, Wallet } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { useDocLocale } from '../lib/use-doc-locale'
import {
  filterEndpoints,
  getCategoryTitle,
  groupEndpointsByCategory,
} from '../lib/api-endpoints-helpers'
import { EndpointCard } from './endpoint-card'

/**
 * The API reference landing page: a top info strip (Base URL / auth / pricing),
 * a client-side filter input, and the endpoint catalog grouped by category.
 *
 * The filter is plain front-end state (no cmdk/portal) to avoid the dialog
 * crashes seen with the previous global search.
 */
export function ReferenceHome() {
  const { t } = useTranslation()
  const locale = useDocLocale()
  const [query, setQuery] = useState('')

  const groups = useMemo(() => {
    const filtered = filterEndpoints(query)
    return groupEndpointsByCategory(filtered)
  }, [query])

  const baseUrl =
    typeof window !== 'undefined' ? window.location.origin : 'https://<your-domain>'

  const infoCards = [
    {
      icon: Link2,
      label: t('Base URL'),
      value: baseUrl,
      mono: true,
    },
    {
      icon: KeyRound,
      label: t('Authentication'),
      value: 'Bearer Token',
    },
    {
      icon: Wallet,
      label: t('Billing'),
      value: t('View Models & Pricing'),
      to: '/pricing',
    },
  ]

  return (
    <div className='mx-auto w-full max-w-5xl px-4 py-8 sm:px-6 lg:px-8'>
      <h1 className='text-2xl font-bold tracking-tight'>{t('API Reference')}</h1>
      <p className='text-muted-foreground mt-2'>
        {t('Browse all available API endpoints. Click any endpoint for full parameter details and code samples.')}
      </p>

      {/* Info strip */}
      <div className='mt-6 grid gap-3 sm:grid-cols-3'>
        {infoCards.map((card) => {
          const Icon = card.icon
          const content = (
            <>
              <div className='text-muted-foreground flex items-center gap-1.5 text-xs font-medium'>
                <Icon className='size-3.5' />
                {card.label}
              </div>
              <div
                className={
                  'mt-1 truncate text-sm font-semibold ' + (card.mono ? 'font-mono' : '')
                }
              >
                {card.value}
              </div>
            </>
          )
          return card.to ? (
            <Link
              key={card.label}
              to={card.to as '/pricing'}
              className='border-border/60 hover:border-primary/40 hover:bg-muted/30 block rounded-lg border p-3 transition-colors'
            >
              {content}
            </Link>
          ) : (
            <div
              key={card.label}
              className='border-border/60 rounded-lg border p-3'
            >
              {content}
            </div>
          )
        })}
      </div>

      {/* Filter */}
      <div className='mt-6 flex items-center gap-2'>
        <div className='relative flex-1'>
          <Search className='text-muted-foreground pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2' />
          <input
            type='text'
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder={t('Filter endpoints by name or path…')}
            className='border-input focus-visible:border-ring focus-visible:ring-ring/50 bg-background h-9 w-full rounded-md border py-2 pr-3 pl-9 text-sm outline-none focus-visible:ring-[3px]'
          />
        </div>
      </div>

      {/* Catalog grouped by category */}
      <div className='mt-8 space-y-8'>
        {groups.map((group) => (
          <section key={group.id}>
            <h2 className='mb-3 text-sm font-semibold tracking-wide uppercase'>
              {getCategoryTitle(group.id, locale)}
              <span className='text-muted-foreground ml-2 font-normal'>
                {group.endpoints.length}
              </span>
            </h2>
            <div className='grid gap-3 sm:grid-cols-2'>
              {group.endpoints.map((endpoint) => (
                <EndpointCard key={endpoint.slug} endpoint={endpoint} />
              ))}
            </div>
          </section>
        ))}
        {groups.length === 0 && (
          <p className='text-muted-foreground py-12 text-center text-sm'>
            {t('No endpoints match your filter.')}
          </p>
        )}
      </div>
    </div>
  )
}
