import { docsHomeSlug, flatDocEntries } from '../manifest'
import type { DocLocale, LocalizedText, ResolvedDoc } from '../types'

/**
 * Resolve a localized value with English fallback. Anything not explicitly
 * translated renders in English rather than vanishing.
 */
export function resolveDocLocale(
  value: LocalizedText | undefined,
  locale: DocLocale
): string {
  if (!value) {
    return ''
  }
  if (locale === 'zh') {
    return value.zh ?? value.en
  }
  return value.en
}

/**
 * Normalize a raw splat (from the catch-all route `/docs/$`) into a slug.
 * Trims leading/trailing slashes and collapses to a single path, e.g.
 * `clients/curl/` → `clients/curl`. An empty/undefined splat resolves to the
 * docs home slug.
 */
export function normalizeDocSlug(splat: string | undefined): string {
  if (!splat) {
    return docsHomeSlug
  }
  return splat.split('/').filter(Boolean).join('/').toLowerCase()
}

export type ResolveDocResult =
  | { found: true; doc: ResolvedDoc }
  | { found: false }

/**
 * Look up a page by its slug across the flat nav tree and resolve its strings
 * for the requested locale. Returns `{ found: false }` when the slug does not
 * match any page so the caller can render a not-found state.
 */
export function resolveDoc(slug: string, locale: DocLocale): ResolveDocResult {
  const normalized = normalizeDocSlug(slug)
  const entry = flatDocEntries.find((e) => e.page.slug === normalized)

  if (!entry) {
    return { found: false }
  }

  const { page, groupIndex, pageIndex } = entry
  const body =
    locale === 'zh' ? (page.content.zh ?? page.content.en) : page.content.en

  return {
    found: true,
    doc: {
      slug: page.slug,
      title: resolveDocLocale(page.title, locale),
      body,
      groupIndex,
      pageIndex,
    },
  }
}

export type DocNeighbor = {
  slug: string
  title: string
}

/**
 * Compute the previous and next pages in sidebar order for prev/next nav links.
 * Wraps across group boundaries: the page after the last page is the first page
 * of the next group, and the page before the first page of a group is the last
 * page of the previous group.
 */
export function getDocNeighbors(
  slug: string,
  locale: DocLocale
): { prev: DocNeighbor | null; next: DocNeighbor | null } {
  const normalized = normalizeDocSlug(slug)
  const currentIndex = flatDocEntries.findIndex(
    (e) => e.page.slug === normalized
  )

  if (currentIndex === -1) {
    return { prev: null, next: null }
  }

  const prevEntry = flatDocEntries[currentIndex - 1]
  const nextEntry = flatDocEntries[currentIndex + 1]

  const toNeighbor = (
    entry: (typeof flatDocEntries)[number] | undefined
  ): DocNeighbor | null => {
    if (!entry) {
      return null
    }
    return {
      slug: entry.page.slug,
      title: resolveDocLocale(entry.page.title, locale),
    }
  }

  return {
    prev: toNeighbor(prevEntry),
    next: toNeighbor(nextEntry),
  }
}
