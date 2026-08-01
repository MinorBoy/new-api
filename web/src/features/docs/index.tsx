import { useMemo } from 'react'

import { PublicLayout } from '@/components/layout'

import { DocsLayout } from './components/docs-layout'
import { DocsNotFound } from './components/docs-not-found'
import { extractDocHeadings } from './lib/headings'
import { normalizeDocSlug, resolveDoc } from './lib/resolve-doc'
import { useDocLocale } from './lib/use-doc-locale'

/**
 * Route component for `/docs/$`. Resolves the catch-all splat to a documentation
 * page and renders the three-column docs shell. Unknown slugs render a
 * documentation-specific not-found state inside the public layout.
 */
export function Docs({ splat }: { splat: string | undefined }) {
  const locale = useDocLocale()
  const slug = normalizeDocSlug(splat)
  const result = useMemo(() => resolveDoc(slug, locale), [slug, locale])

  if (!result.found) {
    return (
      <PublicLayout>
        <DocsNotFound slug={slug} />
      </PublicLayout>
    )
  }

  const headings = extractDocHeadings(result.doc.body)

  return <DocsLayout doc={result.doc} headings={headings} locale={locale} />
}
