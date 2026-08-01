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
import type { LucideIcon } from 'lucide-react'

/**
 * Localized text. Falls back to the English value when a language is missing.
 * Keep the keys in sync with `resolveDocLocale` in `lib/resolve-doc.ts`.
 */
export type LocalizedText = {
  en: string
  zh?: string
}

/**
 * A single documentation page.
 *
 * `slug` is the URL segment under `/docs/` (e.g. `quickstart` → `/docs/quickstart`).
 * Nested slugs use `/` separators (`clients/curl` → `/docs/clients/curl`).
 */
export type DocPage = {
  slug: string
  title: LocalizedText
  /** Raw markdown source keyed by language code (`en` required, `zh` optional). */
  content: { en: string; zh?: string }
  /** Sort order within its category (ascending). */
  order: number
}

/**
 * A sidebar group — a category of related pages.
 */
export type DocNavGroup = {
  id: string
  title: LocalizedText
  icon: LucideIcon
  pages: DocPage[]
}

export type DocLocale = 'en' | 'zh'

/** A resolved page for rendering, with the active locale's strings applied. */
export type ResolvedDoc = {
  slug: string
  title: string
  /** Markdown source for the active locale (already language-fallback resolved). */
  body: string
  /** group id + index, used for prev/next navigation. */
  groupIndex: number
  pageIndex: number
}
