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
import { useMemo } from 'react'

import { PublicLayout } from '@/components/layout'

import { DocsLayout } from './components/docs-layout'
import { DocsNotFound } from './components/docs-not-found'
import { EndpointDetail } from './components/endpoint-detail'
import { ReferenceHome } from './components/reference-home'
import { extractDocHeadings } from './lib/headings'
import { findEndpointBySlug } from './lib/api-endpoints-helpers'
import { normalizeDocSlug, resolveDoc } from './lib/resolve-doc'
import { useDocLocale } from './lib/use-doc-locale'

const REFERENCE_PREFIX = 'reference/'

/**
 * Resolve the splat into one of three render targets:
 *   - `reference`              → the endpoint catalog landing page
 *   - `reference/<slug>`       → a structured endpoint detail page
 *   - any guide slug           → a Markdown guide page
 *
 * Returns a discriminated result so the caller renders the right shell.
 */
type ResolvedRoute =
  | { kind: 'reference-home' }
  | { kind: 'endpoint'; slug: string }
  | { kind: 'guide'; slug: string }
  | { kind: 'not-found'; slug: string }

function resolveRoute(rawSplat: string | undefined): ResolvedRoute {
  const slug = normalizeDocSlug(rawSplat)

  if (slug === 'reference') {
    return { kind: 'reference-home' }
  }

  if (slug.startsWith(REFERENCE_PREFIX)) {
    const endpointSlug = slug.slice(REFERENCE_PREFIX.length)
    if (endpointSlug && findEndpointBySlug(endpointSlug)) {
      return { kind: 'endpoint', slug: endpointSlug }
    }
    return { kind: 'not-found', slug }
  }

  return { kind: 'guide', slug }
}

/**
 * Route component for `/docs/$`. Resolves the catch-all splat to the
 * appropriate docs surface and renders it inside the docs shell.
 */
export function Docs({ splat }: { splat: string | undefined }) {
  const locale = useDocLocale()
  const route = useMemo(() => resolveRoute(splat), [splat])

  if (route.kind === 'reference-home') {
    return (
      <DocsLayout locale={locale}>
        <ReferenceHome />
      </DocsLayout>
    )
  }

  if (route.kind === 'endpoint') {
    const endpoint = findEndpointBySlug(route.slug)!
    return (
      <DocsLayout locale={locale}>
        <EndpointDetail endpoint={endpoint} />
      </DocsLayout>
    )
  }

  if (route.kind === 'not-found') {
    return (
      <PublicLayout>
        <DocsNotFound slug={route.slug} />
      </PublicLayout>
    )
  }

  // Guide page (Markdown)
  const result = resolveDoc(route.slug, locale)
  if (!result.found) {
    return (
      <PublicLayout>
        <DocsNotFound slug={route.slug} />
      </PublicLayout>
    )
  }

  const headings = extractDocHeadings(result.doc.body)
  return <DocsLayout doc={result.doc} headings={headings} locale={locale} />
}
