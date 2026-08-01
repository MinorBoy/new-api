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

// ---------------------------------------------------------------------------
// Structured API reference (endpoint catalog)
// ---------------------------------------------------------------------------

export type HttpMethod = 'GET' | 'POST' | 'DELETE' | 'PUT' | 'PATCH'

/**
 * The upstream protocol family an endpoint belongs to. Drives the protocol
 * badge (OpenAI / Anthropic / Gemini / Gateway) and groups endpoints in the
 * sidebar.
 */
export type EndpointProtocol =
  | 'openai'
  | 'anthropic'
  | 'gemini'
  | 'gateway'
  | 'ark'
  | 'mj'
  | 'suno'

/**
 * Functional grouping for the reference catalog and sidebar sub-sections.
 */
export type EndpointCategory =
  | 'chat'
  | 'embeddings'
  | 'images'
  | 'audio'
  | 'video'
  | 'rerank'
  | 'moderation'
  | 'models'

/** Whether a request parameter is mandatory. */
export type ParamRequired = 'yes' | 'no' | 'conditional'

/**
 * One row of a request/response parameter table.
 *
 * `name` uses dotted/bracket notation for nested fields, e.g. `messages[].role`
 * or `usage.prompt_tokens`, mirroring how 4stoken documents nested params.
 */
export type ParamField = {
  name: string
  type: string
  required: ParamRequired
  description: LocalizedText
}

/** One row of an HTTP error-code table for an endpoint. */
export type ErrorCodeRow = {
  status: number
  description: LocalizedText
}

/** A code sample in a specific language. `highlight` is the CodeMirror lang. */
export type CodeSample = {
  lang: string
  label: string
  highlight: string
  code: string
}

/**
 * A structured API endpoint, the unit of the reference catalog.
 *
 * `slug` is the URL segment under `/docs/reference/`
 * (e.g. `chat-completions` → `/docs/reference/chat-completions`). Path and
 * method mirror the real relay routes in `common/endpoint_defaults.go` /
 * `router/relay-router.go`.
 */
export type ApiEndpoint = {
  slug: string
  method: HttpMethod
  path: string
  protocol: EndpointProtocol
  category: EndpointCategory
  title: LocalizedText
  summary: LocalizedText
  auth: string
  contentType: string
  requestParams?: ParamField[]
  responseParams?: ParamField[]
  errorCodes?: ErrorCodeRow[]
  codeSamples: CodeSample[]
}

/** A category group of endpoints for the reference home and sidebar. */
export type EndpointCategoryGroup = {
  id: EndpointCategory
  title: LocalizedText
  endpoints: ApiEndpoint[]
}

