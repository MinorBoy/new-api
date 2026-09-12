import { api } from '@/lib/api'

import type { HomePageContentResponse } from './types'

// ============================================================================
// Home Page APIs
// ============================================================================

/**
 * Get custom home page content
 * Returns Markdown/HTML content or iframe URL
 */
export async function getHomePageContent(): Promise<HomePageContentResponse> {
  const res = await api.get('/api/home_page_content')
  return res.data
}

/**
 * Get the admin-configured home page composition.
 * Returns 'default' | 'living-system'. Public endpoint — consumed by
 * anonymous visitors on the landing route, parallel to getHomePageContent.
 */
export async function getHomePageStyle(): Promise<HomePageContentResponse> {
  const res = await api.get('/api/home_page_style')
  return res.data
}
