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
import type {
  ApiEndpoint,
  EndpointCategory,
  EndpointCategoryGroup,
} from '../types'
import { resolveDocLocale } from './resolve-doc'
import type { DocLocale } from '../types'
import { apiEndpoints } from './api-endpoints'

/**
 * Catalog category display order and titles. Endpoints are grouped by these
 * categories in both the reference home and the sidebar.
 */
const CATEGORY_TITLES: Record<EndpointCategory, { en: string; zh?: string }> = {
  chat: { en: 'Chat', zh: '对话' },
  embeddings: { en: 'Embeddings', zh: '向量' },
  images: { en: 'Images', zh: '图像' },
  audio: { en: 'Audio', zh: '音频' },
  video: { en: 'Video', zh: '视频' },
  rerank: { en: 'Rerank', zh: '重排序' },
  moderation: { en: 'Moderation', zh: '内容审核' },
  models: { en: 'Models', zh: '模型' },
}

/** Display order for categories in the catalog. */
const CATEGORY_ORDER: EndpointCategory[] = [
  'chat',
  'embeddings',
  'images',
  'audio',
  'video',
  'rerank',
  'moderation',
  'models',
]

/**
 * Group endpoints by category in display order, skipping empty categories.
 */
export function groupEndpointsByCategory(
  endpoints: ApiEndpoint[] = apiEndpoints
): EndpointCategoryGroup[] {
  return CATEGORY_ORDER.map((category) => ({
    id: category,
    title: CATEGORY_TITLES[category],
    endpoints: endpoints.filter((e) => e.category === category),
  })).filter((group) => group.endpoints.length > 0)
}

/**
 * Resolve a localized category title.
 */
export function getCategoryTitle(
  category: EndpointCategory,
  locale: DocLocale
): string {
  return resolveDocLocale(CATEGORY_TITLES[category], locale)
}

/**
 * Look up an endpoint by its slug (the segment after `/docs/reference/`).
 */
export function findEndpointBySlug(
  slug: string,
  endpoints: ApiEndpoint[] = apiEndpoints
): ApiEndpoint | undefined {
  return endpoints.find((e) => e.slug === slug)
}

/**
 * Case-insensitive filter of endpoints by title (any locale), path, or summary.
 * Used by the reference home search box.
 */
export function filterEndpoints(
  query: string,
  endpoints: ApiEndpoint[] = apiEndpoints
): ApiEndpoint[] {
  const q = query.trim().toLowerCase()
  if (!q) return endpoints
  return endpoints.filter((e) => {
    const hay = [
      e.title.en,
      e.title.zh ?? '',
      e.summary.en,
      e.summary.zh ?? '',
      e.path,
      e.slug,
      e.protocol,
    ]
      .join(' ')
      .toLowerCase()
    return hay.includes(q)
  })
}

/**
 * The default reference landing slug shown at `/docs` when no splat is given.
 */
export const referenceHomeSlug = 'reference'
